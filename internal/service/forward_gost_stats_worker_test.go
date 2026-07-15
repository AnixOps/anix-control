package service

import (
	"context"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
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
	s.Require().NoError(database.AutoMigrate(
		&model.ForwardTunnel{},
		&model.Forward{},
		&model.ForwardTrafficCursor{},
	))
}

func (s *ForwardGostStatsWorkerTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	appconfig.Set(nil)
	s.worker = NewForwardGostStatsWorker(database.Get())
}

func (s *ForwardGostStatsWorkerTestSuite) TestRunOnce_NoActiveForwardsReturnsNil() {
	activeForwards, err := s.worker.runOnce(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, activeForwards)
}

func (s *ForwardGostStatsWorkerTestSuite) TestStart_StopsOnContextCancellationDuringIdleDelay() {
	s.worker.interval = time.Hour
	s.worker.idleInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.worker.Start(ctx)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		s.T().Fatal("gost stats worker did not stop after context cancellation")
	}
}

func TestForwardGostStatsWorker(t *testing.T) {
	suite.Run(t, new(ForwardGostStatsWorkerTestSuite))
}

func TestNewForwardGostStatsWorker_LoadsPollingSettingsFromConfig(t *testing.T) {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			GostStats: appconfig.ForwardRuntimeGostStatsConfig{
				PollInterval:     "45s",
				IdlePollInterval: "10s",
				ErrorLogInterval: "3m",
			},
		},
	})
	defer appconfig.Set(nil)

	worker := NewForwardGostStatsWorker(&gorm.DB{})
	assert.Equal(t, 45*time.Second, worker.interval)
	assert.Equal(t, 45*time.Second, worker.idleInterval)
	assert.Equal(t, 3*time.Minute, worker.errorLogger.interval)
}
