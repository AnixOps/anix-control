package service

import (
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// forwardRuntimeJobActiveForwardIndex is the partial unique index that
// allows one active (pending or running) runtime job per forward. The
// schema baseline creates it (internal/database/migrations); a write that
// violates it is an active-job conflict.
const forwardRuntimeJobActiveForwardIndex = "idx_forward_runtime_job_active_forward"

// MigrateForwardRuntimeJobsForTest creates the runtime job table with its
// partial unique index on a test's own database, as the schema baseline
// does in Control's.
func MigrateForwardRuntimeJobsForTest(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&model.ForwardRuntimeJob{}); err != nil {
		return err
	}
	return db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (forward_id) WHERE forward_id IS NOT NULL AND status IN (%d, %d)",
		forwardRuntimeJobActiveForwardIndex,
		model.ForwardRuntimeJob{}.TableName(),
		model.ForwardRuntimeJobStatusPending,
		model.ForwardRuntimeJobStatusRunning,
	)).Error
}
