package service

import (
	"errors"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// EnsureForwardBridgeSchema creates the durable clean_agent bridge mapping table if it
// does not exist. Called unconditionally at startup (including production) because
// AutoMigrate only runs in dev/test. Idempotent and only touches this new table.
func EnsureForwardBridgeSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if db.Migrator().HasTable(&model.ForwardAgentBridgeTask{}) {
		return nil
	}
	return db.AutoMigrate(&model.ForwardAgentBridgeTask{})
}

// ForwardAgentBridgeService handles result write-back for clean_agent bridge jobs.
// It is shared by the agent result handler so that job/forward state transitions stay
// identical to the local ansible executor semantics.
type ForwardAgentBridgeService struct {
	db *gorm.DB
}

func NewForwardAgentBridgeService(db *gorm.DB) *ForwardAgentBridgeService {
	return &ForwardAgentBridgeService{db: db}
}

func (s *ForwardAgentBridgeService) queryDB() *gorm.DB {
	if s.db == nil {
		return nil
	}
	return s.db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}

// LookupBridgeTask returns the durable bridge mapping for a given agent task_id.
// Returns (nil, nil) when the task_id is not a bridge task (e.g. an admin memory task).
func (s *ForwardAgentBridgeService) LookupBridgeTask(taskID string) (*model.ForwardAgentBridgeTask, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	db := s.queryDB()
	if db == nil {
		return nil, nil
	}
	var task model.ForwardAgentBridgeTask
	err := db.Where("task_id = ?", taskID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// CompleteJob writes an agent-reported result back to the runtime job, the forward
// runtime state, and marks the bridge mapping completed. It is idempotent: a mapping
// already in the completed state is a no-op (duplicate reports return alreadyDone=true).
func (s *ForwardAgentBridgeService) CompleteJob(taskID string, success bool, output, errMsg string) (alreadyDone bool, err error) {
	db := s.queryDB()
	if db == nil {
		return false, errors.New("forward agent bridge service has no database")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var mapping model.ForwardAgentBridgeTask
		if err := tx.Where("task_id = ?", taskID).First(&mapping).Error; err != nil {
			return err
		}

		if mapping.Status == model.ForwardAgentBridgeTaskStatusCompleted {
			alreadyDone = true
			return nil
		}

		var job model.ForwardRuntimeJob
		if err := tx.First(&job, mapping.RuntimeJobID).Error; err != nil {
			return err
		}

		now := time.Now()
		if success {
			if err := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id = ?", job.ID).
				Updates(map[string]any{
					"status":       model.ForwardRuntimeJobStatusSuccess,
					"result":       strings.TrimSpace(output),
					"error":        "",
					"completed_at": &now,
				}).Error; err != nil {
				return err
			}
			if err := updateForwardRuntimeStateTx(tx, &job, model.ForwardRuntimeJobStatusSuccess, cleanAgentBridgeSuccessMessage(job.Action), successForwardStatusForAction(job.Action), &now); err != nil {
				return err
			}
		} else {
			trimmed := strings.TrimSpace(errMsg)
			if trimmed == "" {
				trimmed = "clean_agent task execution failed"
			}
			if err := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id = ?", job.ID).
				Updates(map[string]any{
					"status":       model.ForwardRuntimeJobStatusFailed,
					"result":       strings.TrimSpace(output),
					"error":        trimmed,
					"completed_at": &now,
				}).Error; err != nil {
				return err
			}
			if err := updateForwardRuntimeStateTx(tx, &job, model.ForwardRuntimeJobStatusFailed, trimmed, failedForwardStatusForAction(job.Action), &now); err != nil {
				return err
			}
		}

		return tx.Model(&model.ForwardAgentBridgeTask{}).
			Where("id = ?", mapping.ID).
			Updates(map[string]any{
				"status": model.ForwardAgentBridgeTaskStatusCompleted,
			}).Error
	})

	return alreadyDone, err
}

// updateForwardRuntimeStateTx mirrors PanelForwardRuntimeJobExecutor.updateForwardRuntimeState
// but runs inside a caller-provided transaction.
func updateForwardRuntimeStateTx(tx *gorm.DB, job *model.ForwardRuntimeJob, runtimeStatus int, message string, forwardStatus *int, syncedAt *time.Time) error {
	if job.ForwardID == nil || *job.ForwardID == 0 {
		return nil
	}
	updates := map[string]any{
		"runtime_backend":      job.Backend,
		"runtime_status":       runtimeStatus,
		"runtime_message":      strings.TrimSpace(message),
		"runtime_last_sync_at": syncedAt,
	}
	if forwardStatus != nil {
		updates["status"] = *forwardStatus
	}
	return tx.Model(&model.Forward{}).Where("id = ?", *job.ForwardID).Updates(updates).Error
}

func cleanAgentBridgeSuccessMessage(action string) string {
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return "clean_agent runtime removed"
	}
	return "clean_agent runtime synchronized"
}
