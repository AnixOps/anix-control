package service

import (
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestEnsureForwardRuntimeJobSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runtime-job.db")), &gorm.Config{})
	assert.NoError(t, err)

	assert.NoError(t, EnsureForwardRuntimeJobSchema(db))
	assert.True(t, db.Migrator().HasTable(&model.ForwardRuntimeJob{}))
	assert.True(t, db.Migrator().HasIndex(&model.ForwardRuntimeJob{}, forwardRuntimeJobActiveForwardIndex))
}

func TestEnsureForwardRuntimeJobSchema_RepairsDuplicateActiveJobsBeforeIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runtime-job.db")), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&model.ForwardRuntimeJob{}))

	forwardID := uint(42)
	jobs := []model.ForwardRuntimeJob{
		{Backend: model.ForwardRuntimeBackendNftablesAnsible, Action: model.ForwardRuntimeJobActionPause, ForwardID: &forwardID, Status: model.ForwardRuntimeJobStatusPending},
		{Backend: model.ForwardRuntimeBackendNftablesAnsible, Action: model.ForwardRuntimeJobActionResume, ForwardID: &forwardID, Status: model.ForwardRuntimeJobStatusRunning},
		{Backend: model.ForwardRuntimeBackendNftablesAnsible, Action: model.ForwardRuntimeJobActionPause, ForwardID: &forwardID, Status: model.ForwardRuntimeJobStatusPending},
	}
	assert.NoError(t, db.Create(&jobs).Error)

	assert.NoError(t, EnsureForwardRuntimeJobSchema(db))

	var activeJobs []model.ForwardRuntimeJob
	assert.NoError(t, db.Where("forward_id = ? AND status IN ?", forwardID, forwardRuntimeJobInProgressStatuses()).
		Order("id ASC").
		Find(&activeJobs).Error)
	if assert.Len(t, activeJobs, 1) {
		assert.Equal(t, jobs[1].ID, activeJobs[0].ID)
		assert.Equal(t, model.ForwardRuntimeJobStatusRunning, activeJobs[0].Status)
	}

	var failedCount int64
	assert.NoError(t, db.Model(&model.ForwardRuntimeJob{}).
		Where("forward_id = ? AND status = ? AND error LIKE ?", forwardID, model.ForwardRuntimeJobStatusFailed, "%schema repair%").
		Count(&failedCount).Error)
	assert.Equal(t, int64(2), failedCount)

	err = db.Create(&model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionPause,
		ForwardID: &forwardID,
		Status:    model.ForwardRuntimeJobStatusPending,
	}).Error
	assert.Error(t, err)
	assert.True(t, isForwardRuntimeJobActiveConflictError(err))

	assert.NoError(t, db.Create(&model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionPause,
		ForwardID: &forwardID,
		Status:    model.ForwardRuntimeJobStatusSuccess,
	}).Error)
}
