package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultForwardCleanAgentActionTimeout = 120 * time.Second

// ForwardAgentBridgeWorker claims pending clean_agent runtime jobs, asks NodeX to
// translate the panel payload into a legacy AgentTask, and persists the durable
// task_id ↔ runtime_job mapping so the agent-task dispatch path can hand it to the
// target agent. It only touches backend='clean_agent'; nftables_ansible stays on the
// local ansible executor and gost stays on the synchronous NodeX execute path.
type ForwardAgentBridgeWorker struct {
	db               *gorm.DB
	nodex            forwardRuntimeNodeXExecutor
	pollInterval     time.Duration
	idlePollInterval time.Duration
	batchSize        int
	actionTimeout    time.Duration
	errorLogger      *forwardBackgroundErrorLogger
}

func NewForwardAgentBridgeWorker(db *gorm.DB) *ForwardAgentBridgeWorker {
	settings := loadForwardRuntimeJobExecutorSettings()
	return &ForwardAgentBridgeWorker{
		db:               db,
		nodex:            newNodeXForwardRuntimeClient(NewSystemConfigService(db)),
		pollInterval:     settings.PollInterval,
		idlePollInterval: settings.IdlePollInterval,
		batchSize:        settings.BatchSize,
		actionTimeout:    loadForwardCleanAgentActionTimeout(),
		errorLogger:      newForwardBackgroundErrorLogger(settings.ErrorLogInterval),
	}
}

func (w *ForwardAgentBridgeWorker) queryDB() *gorm.DB {
	if w.db == nil {
		return nil
	}
	return w.db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}

func (w *ForwardAgentBridgeWorker) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.requeueStaleJobs(); err != nil {
		w.errorLogger.Logf("requeue", "forward bridge worker requeue failed: %v", err)
	} else {
		w.errorLogger.Clear("requeue")
	}

	nextDelay := time.Duration(0)
	for {
		if !waitForwardBackgroundCycle(ctx, nextDelay) {
			return
		}

		processed, err := w.runPendingJobs(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.errorLogger.Logf("cycle", "forward bridge worker cycle failed: %v", err)
			nextDelay = w.pollInterval
			continue
		}
		w.errorLogger.Clear("cycle")

		if processed == 0 {
			nextDelay = w.idlePollInterval
			continue
		}
		nextDelay = w.pollInterval
	}
}

func (w *ForwardAgentBridgeWorker) RunPendingJobs(ctx context.Context) error {
	_, err := w.runPendingJobs(ctx)
	return err
}

func (w *ForwardAgentBridgeWorker) runPendingJobs(ctx context.Context) (int, error) {
	if err := w.expireTimedOutBridgeTasks(time.Now()); err != nil {
		return 0, err
	}

	processedCount := 0
	for {
		processed, err := w.processNext(ctx)
		if err != nil {
			return processedCount, err
		}
		if !processed {
			return processedCount, nil
		}
		processedCount++
	}
}

func (w *ForwardAgentBridgeWorker) processNext(ctx context.Context) (bool, error) {
	var jobs []model.ForwardRuntimeJob
	if err := w.queryDB().
		Where("backend = ? AND status = ?", model.ForwardRuntimeBackendCleanAgent, model.ForwardRuntimeJobStatusPending).
		Order("id ASC").
		Limit(w.batchSize).
		Find(&jobs).Error; err != nil {
		return false, err
	}

	for idx := range jobs {
		claimed, err := w.claimJob(&jobs[idx])
		if err != nil {
			return false, err
		}
		if !claimed {
			continue
		}
		return true, w.dispatchClaimedJob(ctx, &jobs[idx])
	}

	return false, nil
}

func (w *ForwardAgentBridgeWorker) claimJob(job *model.ForwardRuntimeJob) (bool, error) {
	now := time.Now()
	claimed := false
	if err := w.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ForwardRuntimeJob{}).
			Where("id = ? AND status = ?", job.ID, model.ForwardRuntimeJobStatusPending).
			Updates(map[string]any{
				"status":     model.ForwardRuntimeJobStatusRunning,
				"started_at": &now,
				"claimed_at": &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		claimed = true
		return updateForwardRuntimeRunningStateTx(tx, job, &now)
	}); err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}
	job.Status = model.ForwardRuntimeJobStatusRunning
	job.StartedAt = &now
	job.ClaimedAt = &now
	return true, nil
}

// dispatchClaimedJob translates the payload via NodeX and persists the bridge mapping.
// On translate failure the job is marked failed (never silently left running or falsely
// marked success). On success the job stays running until the agent reports back.
func (w *ForwardAgentBridgeWorker) dispatchClaimedJob(ctx context.Context, job *model.ForwardRuntimeJob) error {
	if job.NodeID == nil || *job.NodeID == 0 {
		return w.finishJobFailure(job, "clean_agent job missing execution node")
	}

	var payload nodeXForwardExecuteRequest
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return w.finishJobFailure(job, "invalid clean_agent payload: "+err.Error())
	}

	task, err := w.nodex.Translate(ctx, job.ID, *job.NodeID, payload)
	if err != nil {
		return w.finishJobFailure(job, "NodeX translate failed: "+err.Error())
	}

	paramsJSON := "{}"
	if len(task.Params) > 0 {
		if encoded, marshalErr := json.Marshal(task.Params); marshalErr == nil {
			paramsJSON = string(encoded)
		}
	}

	taskType := task.Type
	if taskType == "" {
		taskType = "forward"
	}
	action := task.Action
	if action == "" {
		action = job.Action
	}

	mapping := model.ForwardAgentBridgeTask{
		TaskID:       task.TaskID,
		RuntimeJobID: job.ID,
		NodeID:       *job.NodeID,
		ForwardID:    job.ForwardID,
		Action:       action,
		Type:         taskType,
		Params:       paramsJSON,
		Status:       model.ForwardAgentBridgeTaskStatusPending,
	}

	// Upsert keyed on runtime_job_id so a requeued job re-translating stays idempotent.
	return w.queryDB().Transaction(func(tx *gorm.DB) error {
		var existing model.ForwardAgentBridgeTask
		err := tx.Where("runtime_job_id = ?", job.ID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&mapping).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&model.ForwardAgentBridgeTask{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"task_id":    mapping.TaskID,
				"node_id":    mapping.NodeID,
				"forward_id": mapping.ForwardID,
				"action":     mapping.Action,
				"type":       mapping.Type,
				"params":     mapping.Params,
				"status":     model.ForwardAgentBridgeTaskStatusPending,
				"dispatched": false,
			}).Error
	})
}

func (w *ForwardAgentBridgeWorker) finishJobFailure(job *model.ForwardRuntimeJob, message string) error {
	now := time.Now()
	if err := w.db.Model(&model.ForwardRuntimeJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":       model.ForwardRuntimeJobStatusFailed,
			"error":        message,
			"completed_at": &now,
		}).Error; err != nil {
		return err
	}
	return updateForwardRuntimeStateTx(w.queryDB(), job, model.ForwardRuntimeJobStatusFailed, message, failedForwardStatusForAction(job.Action), &now)
}

func (w *ForwardAgentBridgeWorker) expireTimedOutBridgeTasks(now time.Time) error {
	db := w.queryDB()
	if db == nil {
		return nil
	}

	timeout := w.actionTimeout
	if timeout <= 0 {
		timeout = defaultForwardCleanAgentActionTimeout
	}
	cutoff := now.Add(-timeout)

	var mappings []model.ForwardAgentBridgeTask
	if err := db.
		Where("status IN ? AND updated_at <= ?", []string{
			model.ForwardAgentBridgeTaskStatusPending,
			model.ForwardAgentBridgeTaskStatusDispatched,
		}, cutoff).
		Order("id ASC").
		Find(&mappings).Error; err != nil {
		return err
	}

	for idx := range mappings {
		mapping := mappings[idx]
		message := fmt.Sprintf("clean_agent task timed out after %s without result", timeout.Round(time.Second))
		if err := db.Transaction(func(tx *gorm.DB) error {
			var current model.ForwardAgentBridgeTask
			if err := tx.
				Where("id = ? AND status IN ? AND updated_at <= ?", mapping.ID, []string{
					model.ForwardAgentBridgeTaskStatusPending,
					model.ForwardAgentBridgeTaskStatusDispatched,
				}, cutoff).
				First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}

			var job model.ForwardRuntimeJob
			if err := tx.
				Where("id = ? AND backend = ? AND status = ?", current.RuntimeJobID, model.ForwardRuntimeBackendCleanAgent, model.ForwardRuntimeJobStatusRunning).
				First(&job).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}

			if err := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id = ? AND status = ?", job.ID, model.ForwardRuntimeJobStatusRunning).
				Updates(map[string]any{
					"status":       model.ForwardRuntimeJobStatusFailed,
					"error":        message,
					"completed_at": &now,
				}).Error; err != nil {
				return err
			}
			if err := updateForwardRuntimeStateTx(tx, &job, model.ForwardRuntimeJobStatusFailed, message, failedForwardStatusForAction(job.Action), &now); err != nil {
				return err
			}
			return tx.Model(&model.ForwardAgentBridgeTask{}).
				Where("id = ? AND status IN ?", current.ID, []string{
					model.ForwardAgentBridgeTaskStatusPending,
					model.ForwardAgentBridgeTaskStatusDispatched,
				}).
				Updates(map[string]any{
					"status": model.ForwardAgentBridgeTaskStatusFailed,
				}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

// requeueStaleJobs recovers clean_agent jobs left running by a crash/restart.
// A running job that already has a bridge mapping was dispatched and is still awaiting
// an agent report, so it stays running. A running job with no mapping crashed before or
// during translation, so it is reset to pending for a fresh translate.
func (w *ForwardAgentBridgeWorker) requeueStaleJobs() error {
	db := w.queryDB()
	if db == nil {
		return nil
	}

	var jobs []model.ForwardRuntimeJob
	if err := db.
		Where("backend = ? AND status = ?", model.ForwardRuntimeBackendCleanAgent, model.ForwardRuntimeJobStatusRunning).
		Find(&jobs).Error; err != nil {
		return err
	}

	for idx := range jobs {
		var count int64
		if err := db.Model(&model.ForwardAgentBridgeTask{}).
			Where("runtime_job_id = ?", jobs[idx].ID).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		now := time.Now()
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id = ? AND status = ?", jobs[idx].ID, model.ForwardRuntimeJobStatusRunning).
				Updates(map[string]any{
					"status":     model.ForwardRuntimeJobStatusPending,
					"started_at": nil,
					"claimed_at": nil,
				}).Error; err != nil {
				return err
			}
			return requeueForwardRuntimeStateTx(tx, &jobs[idx], &now)
		}); err != nil {
			return err
		}
	}
	return nil
}
