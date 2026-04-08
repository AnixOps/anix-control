package service

import (
	"context"
	"testing"
	"time"

	appconfig "github.com/anixops/v2board/internal/config"
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
	appconfig.Set(nil)
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
