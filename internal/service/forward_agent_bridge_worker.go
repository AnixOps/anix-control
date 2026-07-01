package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
	result := w.db.Model(&model.ForwardRuntimeJob{}).
		Where("id = ? AND status = ?", job.ID, model.ForwardRuntimeJobStatusPending).
		Updates(map[string]any{
			"status":     model.ForwardRuntimeJobStatusRunning,
			"started_at": &now,
			"claimed_at": &now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
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
		if err := db.Model(&model.ForwardRuntimeJob{}).
			Where("id = ? AND status = ?", jobs[idx].ID, model.ForwardRuntimeJobStatusRunning).
			Updates(map[string]any{
				"status":     model.ForwardRuntimeJobStatusPending,
				"started_at": nil,
				"claimed_at": nil,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}
