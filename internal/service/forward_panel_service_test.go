package service

import (
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PanelForwardServiceTestSuite struct {
	ServiceTestSuite
	svc *PanelForwardService
}

func (s *PanelForwardServiceTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
	)
}

func (s *PanelForwardServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_forward_tunnel")
	s.svc = NewPanelForwardService(db)
}

func (s *PanelForwardServiceTestSuite) TestListTunnels_AdminGetsAllActive() {
	db := database.Get()

	activeA := &model.ForwardTunnel{Name: "Tunnel A", InIP: "1.1.1.1", Status: model.ForwardTunnelStatusActive}
	activeB := &model.ForwardTunnel{Name: "Tunnel B", InIP: "2.2.2.2", Status: model.ForwardTunnelStatusActive}
	disabled := &model.ForwardTunnel{Name: "Tunnel C", InIP: "3.3.3.3", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(activeA).Error)
	assert.NoError(s.T(), db.Create(activeB).Error)
	assert.NoError(s.T(), db.Create(disabled).Error)
	assert.NoError(s.T(), db.Model(disabled).Update("status", model.ForwardTunnelStatusDisabled).Error)

	items, err := s.svc.ListTunnels(0, true)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 2)
	assert.Equal(s.T(), "Tunnel A", items[0].Name)
	assert.Equal(s.T(), "Tunnel B", items[1].Name)
}

func (s *PanelForwardServiceTestSuite) TestListTunnels_UserGetsAuthorizedActiveOnly() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-user@example.com",
		Password:       "hash",
		Token:          "panel-forward-user-token",
		UUID:           "panel-forward-user-uuid",
		TransferEnable: 1073741824,
	}
	otherUser := &model.User{
		Email:          "panel-forward-other@example.com",
		Password:       "hash",
		Token:          "panel-forward-other-token",
		UUID:           "panel-forward-other-uuid",
		TransferEnable: 1073741824,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(otherUser).Error)

	authorizedActive := &model.ForwardTunnel{Name: "Authorized Active", InIP: "10.0.0.1", Status: model.ForwardTunnelStatusActive}
	authorizedDisabled := &model.ForwardTunnel{Name: "Authorized Disabled", InIP: "10.0.0.2", Status: model.ForwardTunnelStatusActive}
	otherUsersTunnel := &model.ForwardTunnel{Name: "Other User Tunnel", InIP: "10.0.0.3", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(authorizedActive).Error)
	assert.NoError(s.T(), db.Create(authorizedDisabled).Error)
	assert.NoError(s.T(), db.Create(otherUsersTunnel).Error)
	assert.NoError(s.T(), db.Model(authorizedDisabled).Update("status", model.ForwardTunnelStatusDisabled).Error)

	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: authorizedActive.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: authorizedDisabled.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", user.ID, authorizedDisabled.ID).
		Update("status", model.ForwardUserTunnelStatusDisabled).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   otherUser.ID,
		TunnelID: otherUsersTunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	items, err := s.svc.ListTunnels(user.ID, false)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 1)
	assert.Equal(s.T(), authorizedActive.ID, items[0].ID)
	assert.Equal(s.T(), "Authorized Active", items[0].Name)
}

func (s *PanelForwardServiceTestSuite) TestListTunnels_UserWithoutPermissionGetsEmptyList() {
	db := database.Get()

	tunnel := &model.ForwardTunnel{Name: "Unassigned Tunnel", InIP: "10.10.10.10", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	items, err := s.svc.ListTunnels(999999, false)
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), items)
}

func (s *PanelForwardServiceTestSuite) TestListTunnels_UserGetsAuthorizedTunnelEvenIfPermissionDisabled() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-disabled-permission@example.com",
		Password:       "hash",
		Token:          "panel-forward-disabled-permission-token",
		UUID:           "panel-forward-disabled-permission-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:     "Disabled Permission Tunnel",
		InIP:     "10.20.30.40",
		Type:     2,
		Protocol: "udp",
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", user.ID, tunnel.ID).
		Update("status", model.ForwardUserTunnelStatusDisabled).Error)

	items, err := s.svc.ListTunnels(user.ID, false)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 1)
	assert.Equal(s.T(), tunnel.ID, items[0].ID)
	assert.Equal(s.T(), tunnel.InIP, items[0].IP)
	assert.Equal(s.T(), tunnel.InIP, items[0].InIP)
	assert.Equal(s.T(), tunnel.Type, items[0].Type)
	assert.Equal(s.T(), tunnel.Protocol, items[0].Protocol)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_RequiresUserTunnelPermission() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-create@example.com",
		Password:       "hash",
		Token:          "panel-forward-create-token",
		UUID:           "panel-forward-create-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Create Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Denied Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)

	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", user.ID, tunnel.ID).
		Update("status", model.ForwardUserTunnelStatusDisabled).Error)

	_, err = s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Still Denied Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
	assert.NoError(s.T(), db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", user.ID, tunnel.ID).
		Update("status", model.ForwardUserTunnelStatusActive).Error)

	item, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Allowed Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), tunnel.ID, item.TunnelID)
	assert.Equal(s.T(), user.ID, item.UserID)
}

func TestPanelForwardService(t *testing.T) {
	suite.Run(t, new(PanelForwardServiceTestSuite))
}
