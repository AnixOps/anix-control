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
	svc           *SpeedLimitService
	runtimeClient *stubNodeXForwardRuntimeClient
}

func (s *SpeedLimitServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	s.svc = NewSpeedLimitService(db)
	s.runtimeClient = &stubNodeXForwardRuntimeClient{}
	s.svc.forwardService.runtimeService.client = s.runtimeClient
	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"http://127.0.0.1:18080",
		"string",
		forwardRuntimeConfigGroup,
		"test NodeX runtime URL",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXTokenConfigKey,
		"test-nodex-token",
		"string",
		forwardRuntimeConfigGroup,
		"test NodeX runtime token",
	))
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

func (s *SpeedLimitServiceTestSuite) TestUpdate_ResyncsAssignedActiveForwards() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)

	user := &model.User{
		Email:          "speed-limit-resync@example.com",
		Password:       "hash",
		Token:          "speed-limit-resync-token",
		UUID:           "speed-limit-resync-uuid",
		TransferEnable: 10 * bytesPerGiB,
	}
	node := &model.ForwardNode{
		Name:     "Speed Limit Resync Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.40",
		APIPort:  19510,
		APIToken: "speed-limit-resync-api-token",
		Status:   model.ForwardNodeStatusOnline,
		Enabled:  true,
	}
	tunnel := &model.ForwardTunnel{
		Name:          "Speed Limit Resync Tunnel",
		InNodeID:      0,
		InIP:          "203.0.113.41",
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnel.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnel).Error)

	record, err := s.svc.Create(SpeedLimitInput{
		Name:       "10M",
		Speed:      80,
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

	active := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Speed Limit Active",
		TunnelID:   tunnel.ID,
		InPort:     13201,
		RemoteAddr: "speed-limit-active.example:443",
		Status:     model.ForwardStatusActive,
	}
	paused := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Speed Limit Paused",
		TunnelID:   tunnel.ID,
		InPort:     13202,
		RemoteAddr: "speed-limit-paused.example:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(active).Error)
	assert.NoError(s.T(), db.Create(paused).Error)
	assert.NoError(s.T(), db.Model(paused).Update("status", model.ForwardStatusPaused).Error)

	updated, err := s.svc.Update(SpeedLimitUpdateInput{
		ID: record.ID,
		SpeedLimitInput: SpeedLimitInput{
			Name:       "20M",
			Speed:      160,
			TunnelID:   tunnel.ID,
			TunnelName: tunnel.Name,
		},
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(160), updated.Speed)

	assert.Len(s.T(), s.runtimeClient.requests, 1)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionUpdate, s.runtimeClient.requests[0].Action)
	if assert.NotNil(s.T(), s.runtimeClient.requests[0].PanelForward) && assert.NotNil(s.T(), s.runtimeClient.requests[0].PanelForward.Limiter) {
		assert.Equal(s.T(), record.ID, s.runtimeClient.requests[0].PanelForward.Limiter.SpeedID)
		assert.Equal(s.T(), int64(160), s.runtimeClient.requests[0].PanelForward.Limiter.Speed)
	}
}

func (s *SpeedLimitServiceTestSuite) TestUpdate_RejectsChangingTunnelWhenAssigned() {
	db := database.Get()

	user := &model.User{
		Email:          "speed-limit-change-tunnel@example.com",
		Password:       "hash",
		Token:          "speed-limit-change-tunnel-token",
		UUID:           "speed-limit-change-tunnel-uuid",
		TransferEnable: bytesPerGiB,
	}
	tunnelA := &model.ForwardTunnel{
		Name:   "Speed Tunnel A",
		InIP:   "10.60.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:   "Speed Tunnel B",
		InIP:   "10.60.0.2",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	record, err := s.svc.Create(SpeedLimitInput{
		Name:       "Assigned Tunnel Rule",
		Speed:      120,
		TunnelID:   tunnelA.ID,
		TunnelName: tunnelA.Name,
	})
	assert.NoError(s.T(), err)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		SpeedID:  &record.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	_, err = s.svc.Update(SpeedLimitUpdateInput{
		ID: record.ID,
		SpeedLimitInput: SpeedLimitInput{
			Name:       "Assigned Tunnel Rule",
			Speed:      120,
			TunnelID:   tunnelB.ID,
			TunnelName: tunnelB.Name,
		},
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "cannot change tunnel")
}

func TestSpeedLimitService(t *testing.T) {
	suite.Run(t, new(SpeedLimitServiceTestSuite))
}
