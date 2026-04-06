package service

import (
	"context"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ForwardGostStatsWorkerTestSuite struct {
	ServiceTestSuite
	worker *ForwardGostStatsWorker
}

func (s *ForwardGostStatsWorkerTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.ForwardTunnel{},
		&model.Forward{},
		&model.ForwardTrafficCursor{},
	)
}

func (s *ForwardGostStatsWorkerTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.worker = NewForwardGostStatsWorker(database.Get())
}

func (s *ForwardGostStatsWorkerTestSuite) TestRunOnce_NoActiveForwardsReturnsNil() {
	activeForwards, err := s.worker.runOnce(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, activeForwards)
}

func TestForwardGostStatsWorker(t *testing.T) {
	suite.Run(t, new(ForwardGostStatsWorkerTestSuite))
}

func TestNewForwardGostStatsWorker_NormalizesIdleIntervalFromEnv(t *testing.T) {
	t.Setenv(forwardGostStatsPollIntervalEnvVar, "45s")
	t.Setenv(forwardGostStatsIdlePollIntervalEnvVar, "10s")
	t.Setenv(forwardGostStatsErrorLogIntervalEnvVar, "3m")

	worker := NewForwardGostStatsWorker(&gorm.DB{})
	assert.Equal(t, 45*time.Second, worker.interval)
	assert.Equal(t, 45*time.Second, worker.idleInterval)
	assert.Equal(t, 3*time.Minute, worker.errorLogger.interval)
}
