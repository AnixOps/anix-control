package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type PeriodicResetTestSuite struct {
	ServiceTestSuite
}

func TestPeriodicResets(t *testing.T) {
	suite.Run(t, new(PeriodicResetTestSuite))
}

func (s *PeriodicResetTestSuite) TestNodeMonthlyResetZeroesCountersOnlyOncePerDay() {
	db := database.Get()
	node := &model.Node{Name: "once-a-day-node", Host: "198.51.100.10", MonthlyResetDay: 6, MonthlyUpload: 10, MonthlyDownload: 20}
	require.NoError(s.T(), db.Create(node).Error)

	worker := NewNodeMonthlyResetWorker(db)
	resetDay := time.Date(2026, 3, 6, 0, 0, 10, 0, time.Local)
	require.NoError(s.T(), worker.RunOnce(resetDay))
	s.assertNodeCounters(node.ID, 0, 0)

	require.NoError(s.T(), db.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]any{"monthly_upload": 30, "monthly_download": 40}).Error)
	require.NoError(s.T(), worker.RunOnce(resetDay.Add(12*time.Hour)))
	s.assertNodeCounters(node.ID, 30, 40)

	require.NoError(s.T(), worker.RunOnce(time.Date(2026, 4, 6, 0, 0, 10, 0, time.Local)))
	s.assertNodeCounters(node.ID, 0, 0)
}

func (s *PeriodicResetTestSuite) TestClaimDailyRunRollsBackWithTheJob() {
	db := database.Get()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	err := db.Transaction(func(tx *gorm.DB) error {
		claimed, err := claimDailyRun(tx, "scheduler.test.last_day", now)
		require.NoError(s.T(), err)
		require.True(s.T(), claimed)
		return assert.AnError
	})
	require.ErrorIs(s.T(), err, assert.AnError)

	claimed, err := claimDailyRun(db, "scheduler.test.last_day", now)
	require.NoError(s.T(), err)
	assert.True(s.T(), claimed, "a failed job must not consume the day")
	claimed, err = claimDailyRun(db, "scheduler.test.last_day", now)
	require.NoError(s.T(), err)
	assert.False(s.T(), claimed)
}

func (s *PeriodicResetTestSuite) assertNodeCounters(id uint, upload, download int64) {
	var node model.Node
	require.NoError(s.T(), database.Get().First(&node, id).Error)
	assert.EqualValues(s.T(), upload, node.MonthlyUpload)
	assert.EqualValues(s.T(), download, node.MonthlyDownload)
}
