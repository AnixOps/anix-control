package service

import (
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SpeedLimitServiceTestSuite struct {
	ServiceTestSuite
	svc *SpeedLimitService
}

func (s *SpeedLimitServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.svc = NewSpeedLimitService(database.Get())
}

func (s *SpeedLimitServiceTestSuite) TestCreateListUpdateDelete() {
	db := database.Get()
	tunnel := &model.ForwardTunnel{
		Name:   "Speed Limit Tunnel",
		InIP:   "10.30.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	record, err := s.svc.Create(SpeedLimitInput{
		Name:       "100M",
		Speed:      100,
		TunnelID:   tunnel.ID,
		TunnelName: tunnel.Name,
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "100M", record.Name)
	assert.Equal(s.T(), int64(100), record.Speed)
	assert.Equal(s.T(), speedLimitStatusActive, record.Status)

	items, err := s.svc.List()
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 1)
	assert.Equal(s.T(), record.ID, items[0].ID)

	updated, err := s.svc.Update(SpeedLimitUpdateInput{
		ID: record.ID,
		SpeedLimitInput: SpeedLimitInput{
			Name:       "200M",
			Speed:      200,
			TunnelID:   tunnel.ID,
			TunnelName: tunnel.Name,
		},
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "200M", updated.Name)
	assert.Equal(s.T(), int64(200), updated.Speed)

	assert.NoError(s.T(), s.svc.Delete(record.ID))

	var count int64
	assert.NoError(s.T(), db.Model(&model.SpeedLimit{}).Count(&count).Error)
	assert.Equal(s.T(), int64(0), count)
}

func (s *SpeedLimitServiceTestSuite) TestCreateRejectsTunnelMismatch() {
	db := database.Get()
	tunnel := &model.ForwardTunnel{
		Name:   "Tunnel-A",
		InIP:   "10.30.0.2",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	_, err := s.svc.Create(SpeedLimitInput{
		Name:       "Mismatch",
		Speed:      100,
		TunnelID:   tunnel.ID,
		TunnelName: "Tunnel-B",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tunnelName does not match")
}

func (s *SpeedLimitServiceTestSuite) TestDeleteRejectsAssignedRule() {
	db := database.Get()

	user := &model.User{
		Email:          "speed-limit-assigned@example.com",
		Password:       "hash",
		Token:          "speed-limit-assigned-token",
		UUID:           "speed-limit-assigned-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Assigned Tunnel",
		InIP:   "10.30.0.3",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	record, err := s.svc.Create(SpeedLimitInput{
		Name:       "Assigned",
		Speed:      300,
		TunnelID:   tunnel.ID,
		TunnelName: tunnel.Name,
	})
	assert.NoError(s.T(), err)

	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		SpeedID:  &record.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	err = s.svc.Delete(record.ID)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "still assigned")
}

func TestSpeedLimitService(t *testing.T) {
	suite.Run(t, new(SpeedLimitServiceTestSuite))
}
