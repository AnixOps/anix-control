package service

import (
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

const forwardRuntimeJobActiveForwardIndex = "idx_forward_runtime_job_active_forward"

// EnsureForwardRuntimeJobSchema keeps runtime job invariants that GORM cannot
// express with model tags, especially the partial unique index for active jobs.
func EnsureForwardRuntimeJobSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&model.ForwardRuntimeJob{}); err != nil {
		return err
	}
	if err := reconcileDuplicateForwardRuntimeJobsInProgress(db); err != nil {
		return err
	}
	return createForwardRuntimeJobActiveForwardIndex(db)
}

func reconcileDuplicateForwardRuntimeJobsInProgress(db *gorm.DB) error {
	type duplicateGroup struct {
		ForwardID uint
		Count     int64
	}

	var duplicates []duplicateGroup
	if err := db.Model(&model.ForwardRuntimeJob{}).
		Select("forward_id, COUNT(*) AS count").
		Where("forward_id IS NOT NULL AND status IN ?", forwardRuntimeJobInProgressStatuses()).
		Group("forward_id").
		Having("COUNT(*) > 1").
		Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) == 0 {
		return nil
	}

	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, duplicate := range duplicates {
			var jobs []model.ForwardRuntimeJob
			if err := tx.Where("forward_id = ? AND status IN ?", duplicate.ForwardID, forwardRuntimeJobInProgressStatuses()).
				Order("CASE WHEN status = 1 THEN 0 ELSE 1 END ASC, id DESC").
				Find(&jobs).Error; err != nil {
				return err
			}
			if len(jobs) <= 1 {
				continue
			}

			staleIDs := make([]uint, 0, len(jobs)-1)
			for _, job := range jobs[1:] {
				staleIDs = append(staleIDs, job.ID)
			}
			if err := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id IN ?", staleIDs).
				Updates(map[string]any{
					"status":       model.ForwardRuntimeJobStatusFailed,
					"error":        "superseded by newer active runtime job during schema repair",
					"completed_at": &now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func createForwardRuntimeJobActiveForwardIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex(&model.ForwardRuntimeJob{}, forwardRuntimeJobActiveForwardIndex) {
		return nil
	}

	switch db.Name() {
	case "sqlite", "postgres":
		return db.Exec(fmt.Sprintf(
			"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (forward_id) WHERE forward_id IS NOT NULL AND status IN (%d, %d)",
			forwardRuntimeJobActiveForwardIndex,
			model.ForwardRuntimeJob{}.TableName(),
			model.ForwardRuntimeJobStatusPending,
			model.ForwardRuntimeJobStatusRunning,
		)).Error
	default:
		return fmt.Errorf("unsupported database dialect for forward runtime job active index: %s", db.Name())
	}
}

func forwardRuntimeJobInProgressStatuses() []int {
	return []int{
		model.ForwardRuntimeJobStatusPending,
		model.ForwardRuntimeJobStatusRunning,
	}
}
