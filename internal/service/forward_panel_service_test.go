package service

import (
	"context"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PanelForwardServiceTestSuite struct {
	ServiceTestSuite
	svc           *PanelForwardService
	runtimeClient *stubNodeXForwardRuntimeClient
}

type stubNodeXForwardRuntimeClient struct {
	calls         int
	lastRequest   *nodeXForwardExecuteRequest
	requests      []nodeXForwardExecuteRequest
	executeResult *nodeXForwardExecuteResult
	executeErr    error
}

func (c *stubNodeXForwardRuntimeClient) Execute(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	_ = ctx
	c.calls++
	reqCopy := req
	if req.PanelForward != nil {
		panelForward := *req.PanelForward
		reqCopy.PanelForward = &panelForward
	}
	if req.LegacyRule != nil {
		legacyRule := *req.LegacyRule
		reqCopy.LegacyRule = &legacyRule
	}
	if req.AnsibleRuntime != nil {
		ansibleRuntime := *req.AnsibleRuntime
		reqCopy.AnsibleRuntime = &ansibleRuntime
	}
	c.lastRequest = &reqCopy
	c.requests = append(c.requests, reqCopy)
	if c.executeResult != nil || c.executeErr != nil {
		return c.executeResult, c.executeErr
	}

	message := "gost runtime synchronized"
	if req.Backend == model.ForwardRuntimeBackendIptablesAnsible {
		message = "ansible runtime applied"
	}
	return &nodeXForwardExecuteResult{
		Backend: req.Backend,
		Status:  model.ForwardRuntimeJobStatusSuccess,
		Message: message,
	}, nil
}

func (s *PanelForwardServiceTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
	)
}

func (s *PanelForwardServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_forward_tunnel")
	s.svc = NewPanelForwardService(db)
	s.runtimeClient = &stubNodeXForwardRuntimeClient{}
	s.svc.runtimeService.client = s.runtimeClient
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

func (s *PanelForwardServiceTestSuite) TestCreateForward_RejectsExpiredTunnelPermission() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-expired-perm@example.com",
		Password:       "hash",
		Token:          "panel-forward-expired-perm-token",
		UUID:           "panel-forward-expired-perm-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Expired Permission Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	expiredAt := time.Now().UnixMilli() - 1000
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
		ExpTime:  expiredAt,
	}).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Denied by Expired Permission",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_RejectsUserTrafficExhausted() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-user-traffic-exhausted@example.com",
		Password:       "hash",
		Token:          "panel-forward-user-traffic-exhausted-token",
		UUID:           "panel-forward-user-traffic-exhausted-uuid",
		TransferEnable: 1024,
		U:              1024,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "User Traffic Exhausted Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Traffic Exhausted Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "user total traffic exhausted")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_RejectsTunnelTrafficExhausted() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-tunnel-traffic-exhausted@example.com",
		Password:       "hash",
		Token:          "panel-forward-tunnel-traffic-exhausted-token",
		UUID:           "panel-forward-tunnel-traffic-exhausted-uuid",
		TransferEnable: 2 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Tunnel Traffic Exhausted Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Flow:     1,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Consumed Forward",
		TunnelID:   tunnel.ID,
		InPort:     10001,
		RemoteAddr: "used.example:443",
		InFlow:     bytesPerGiB / 2,
		OutFlow:    bytesPerGiB / 2,
		Status:     model.ForwardStatusPaused,
	}).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Tunnel Traffic Exhausted Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tunnel traffic exhausted")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_RejectsTunnelForwardQuota() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-tunnel-quota@example.com",
		Password:       "hash",
		Token:          "panel-forward-tunnel-quota-token",
		UUID:           "panel-forward-tunnel-quota-uuid",
		TransferEnable: bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Tunnel Quota Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Num:      1,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Existing Tunnel Forward",
		TunnelID:   tunnel.ID,
		InPort:     10001,
		RemoteAddr: "used.example:443",
		Status:     model.ForwardStatusPaused,
	}).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Quota Exceeded Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tunnel forward quota exceeded")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestSetForwardStatus_ResumeRejectsTunnelTrafficExhausted() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-resume-traffic@example.com",
		Password:       "hash",
		Token:          "panel-forward-resume-traffic-token",
		UUID:           "panel-forward-resume-traffic-uuid",
		TransferEnable: 2 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Resume Traffic Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Flow:     1,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	forward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Resume Me",
		TunnelID:   tunnel.ID,
		InPort:     10001,
		RemoteAddr: "resume.example:443",
		InFlow:     bytesPerGiB / 2,
		OutFlow:    bytesPerGiB / 2,
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Model(forward).Update("status", model.ForwardStatusPaused).Error)

	err := s.svc.SetForwardStatus(user.ID, false, forward.ID, model.ForwardStatusActive)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tunnel traffic exhausted")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, record.Status)
}

func (s *PanelForwardServiceTestSuite) TestUpdateForward_AdminChangingTunnelRevalidatesTargetUserGrant() {
	db := database.Get()

	admin := &model.User{
		Email:          "panel-forward-admin-update@example.com",
		Password:       "hash",
		Token:          "panel-forward-admin-update-token",
		UUID:           "panel-forward-admin-update-uuid",
		TransferEnable: bytesPerGiB,
		IsAdmin:        1,
	}
	user := &model.User{
		Email:          "panel-forward-admin-target@example.com",
		Password:       "hash",
		Token:          "panel-forward-admin-target-token",
		UUID:           "panel-forward-admin-target-uuid",
		TransferEnable: bytesPerGiB,
	}
	tunnelA := &model.ForwardTunnel{
		Name:   "Admin Update Tunnel A",
		InIP:   "10.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:   "Admin Update Tunnel B",
		InIP:   "10.0.0.2",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(admin).Error)
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	forward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Admin Managed Forward",
		TunnelID:   tunnelA.ID,
		InPort:     10001,
		RemoteAddr: "admin.example:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	inPort := forward.InPort

	_, err := s.svc.UpdateForward(admin.ID, true, PanelForwardUpdateInput{
		ID:         forward.ID,
		UserID:     user.ID,
		Name:       forward.Name,
		TunnelID:   tunnelB.ID,
		InPort:     &inPort,
		RemoteAddr: forward.RemoteAddr,
		Strategy:   "fifo",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no active tunnel permission")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, forward.ID).Error)
	assert.Equal(s.T(), tunnelA.ID, record.TunnelID)
}

func (s *PanelForwardServiceTestSuite) TestRemoveUserTunnel_CascadeDeleteUserTunnelForwards() {
	db := database.Get()

	userA := &model.User{
		Email:          "panel-forward-remove-user-a@example.com",
		Password:       "hash",
		Token:          "panel-forward-remove-user-a-token",
		UUID:           "panel-forward-remove-user-a-uuid",
		TransferEnable: 1073741824,
	}
	userB := &model.User{
		Email:          "panel-forward-remove-user-b@example.com",
		Password:       "hash",
		Token:          "panel-forward-remove-user-b-token",
		UUID:           "panel-forward-remove-user-b-uuid",
		TransferEnable: 1073741824,
	}
	node := &model.ForwardNode{Name: "Cascade Ingress", Host: "10.0.0.10", Status: model.ForwardNodeStatusOnline, Enabled: true}
	tunnelA := &model.ForwardTunnel{Name: "Cascade Tunnel A", InNodeID: 0, InIP: "10.0.0.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Cascade Tunnel B", InIP: "10.0.0.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(userA).Error)
	assert.NoError(s.T(), db.Create(userB).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnelA.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	perm := &model.ForwardUserTunnel{
		UserID:   userA.ID,
		TunnelID: tunnelA.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(perm).Error)

	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "A-1",
		TunnelID:   tunnelA.ID,
		InPort:     10001,
		RemoteAddr: "example.com:443",
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "A-2",
		TunnelID:   tunnelA.ID,
		InPort:     10002,
		RemoteAddr: "example.org:443",
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "A-OtherTunnel",
		TunnelID:   tunnelB.ID,
		InPort:     11001,
		RemoteAddr: "other.example:443",
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     userB.ID,
		UserName:   userB.Email,
		Name:       "B-SameTunnel",
		TunnelID:   tunnelA.ID,
		InPort:     12001,
		RemoteAddr: "another.example:443",
		Status:     model.ForwardStatusPaused,
	}).Error)

	assert.NoError(s.T(), s.svc.RemoveUserTunnel(perm.ID))
	assert.Len(s.T(), s.runtimeClient.requests, 2)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionDelete, s.runtimeClient.requests[0].Action)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionDelete, s.runtimeClient.requests[1].Action)
	assert.Equal(s.T(), "A-1", s.runtimeClient.requests[0].PanelForward.Forward.Name)
	assert.Equal(s.T(), "A-2", s.runtimeClient.requests[1].PanelForward.Forward.Name)

	var remaining int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).
		Where("user_id = ? AND tunnel_id = ?", userA.ID, tunnelA.ID).
		Count(&remaining).Error)
	assert.Equal(s.T(), int64(0), remaining)

	assert.NoError(s.T(), db.Model(&model.Forward{}).
		Where("user_id = ? AND tunnel_id = ?", userA.ID, tunnelB.ID).
		Count(&remaining).Error)
	assert.Equal(s.T(), int64(1), remaining)

	assert.NoError(s.T(), db.Model(&model.Forward{}).
		Where("user_id = ? AND tunnel_id = ?", userB.ID, tunnelA.ID).
		Count(&remaining).Error)
	assert.Equal(s.T(), int64(1), remaining)

	var permCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardUserTunnel{}).Where("id = ?", perm.ID).Count(&permCount).Error)
	assert.Equal(s.T(), int64(0), permCount)
}

func (s *PanelForwardServiceTestSuite) TestUpdateUserTunnel_DisablePausesAffectedActiveForwards() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-disable-user@example.com",
		Password:       "hash",
		Token:          "panel-forward-disable-user-token",
		UUID:           "panel-forward-disable-user-uuid",
		TransferEnable: 1073741824,
	}
	node := &model.ForwardNode{Name: "Disable Ingress", Host: "10.10.10.1", Status: model.ForwardNodeStatusOnline, Enabled: true}
	tunnelA := &model.ForwardTunnel{Name: "Disable Tunnel A", InNodeID: 0, InIP: "10.10.10.10", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Disable Tunnel B", InIP: "10.10.10.11", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnelA.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	perm := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		Flow:     100,
		Num:      10,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(perm).Error)

	active := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Disable-Active",
		TunnelID:   tunnelA.ID,
		InPort:     13001,
		RemoteAddr: "disable.example:443",
		Status:     model.ForwardStatusActive,
	}
	alreadyPaused := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Disable-Paused",
		TunnelID:   tunnelA.ID,
		InPort:     13002,
		RemoteAddr: "paused.example:443",
		Status:     model.ForwardStatusPaused,
	}
	otherTunnel := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Disable-OtherTunnel",
		TunnelID:   tunnelB.ID,
		InPort:     13003,
		RemoteAddr: "other.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(active).Error)
	assert.NoError(s.T(), db.Create(alreadyPaused).Error)
	assert.NoError(s.T(), db.Model(alreadyPaused).Update("status", model.ForwardStatusPaused).Error)
	assert.NoError(s.T(), db.Create(otherTunnel).Error)

	err := s.svc.UpdateUserTunnel(PanelUserTunnelUpdateInput{
		ID:            perm.ID,
		Flow:          perm.Flow,
		Num:           perm.Num,
		FlowResetTime: perm.FlowResetTime,
		ExpTime:       perm.ExpTime,
		Status:        model.ForwardUserTunnelStatusDisabled,
		SpeedID:       perm.SpeedID,
	})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), s.runtimeClient.requests, 1)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionPause, s.runtimeClient.requests[0].Action)
	assert.Equal(s.T(), active.Name, s.runtimeClient.requests[0].PanelForward.Forward.Name)

	var activeRecord model.Forward
	assert.NoError(s.T(), db.First(&activeRecord, active.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, activeRecord.Status)

	var pausedRecord model.Forward
	assert.NoError(s.T(), db.First(&pausedRecord, alreadyPaused.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, pausedRecord.Status)

	var otherRecord model.Forward
	assert.NoError(s.T(), db.First(&otherRecord, otherTunnel.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusActive, otherRecord.Status)
}

func (s *PanelForwardServiceTestSuite) TestListUserTunnels_ReturnsJoinedFields() {
	db := database.Get()

	speed := int64(2048)
	speedID := uint(11)
	user := &model.User{
		Email:          "panel-forward-list-user-tunnels@example.com",
		Password:       "hash",
		Token:          "panel-forward-list-user-tunnels-token",
		UUID:           "panel-forward-list-user-tunnels-uuid",
		TransferEnable: 1073741824,
		SpeedLimit:     &speed,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "List Detail Tunnel",
		InIP:   "10.10.10.10",
		Flow:   2,
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.SpeedLimit{
		ID:          speedID,
		Name:        "speed-11",
		Speed:       4096,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}).Error)

	perm := &model.ForwardUserTunnel{
		UserID:        user.ID,
		TunnelID:      tunnel.ID,
		Flow:          100,
		Num:           5,
		InFlow:        3000,
		OutFlow:       4000,
		FlowResetTime: 7,
		ExpTime:       time.Now().Add(24 * time.Hour).UnixMilli(),
		SpeedID:       &speedID,
		Status:        model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(perm).Error)

	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Traffic-1",
		TunnelID:   tunnel.ID,
		InPort:     13001,
		RemoteAddr: "foo.example:443",
		InFlow:     1000,
		OutFlow:    2000,
		Status:     model.ForwardStatusPaused,
	}).Error)

	items, err := s.svc.ListUserTunnels(PanelUserTunnelQueryInput{UserID: user.ID})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 1)
	assert.Equal(s.T(), perm.ID, items[0].ID)
	assert.Equal(s.T(), user.ID, items[0].UserID)
	assert.Equal(s.T(), tunnel.ID, items[0].TunnelID)
	assert.Equal(s.T(), int64(100), items[0].Flow)
	assert.Equal(s.T(), 5, items[0].Num)
	assert.Equal(s.T(), int64(7), items[0].FlowResetTime)
	assert.Equal(s.T(), perm.ExpTime, items[0].ExpTime)
	assert.Equal(s.T(), &speedID, items[0].SpeedID)
	assert.Equal(s.T(), "speed-11", items[0].SpeedLimitName)
	assert.Equal(s.T(), int64(4096), items[0].Speed)
	assert.Equal(s.T(), tunnel.Name, items[0].TunnelName)
	assert.Equal(s.T(), tunnel.Flow, items[0].TunnelFlow)
	assert.Equal(s.T(), int64(3000), items[0].InFlow)
	assert.Equal(s.T(), int64(4000), items[0].OutFlow)
	assert.Equal(s.T(), model.ForwardUserTunnelStatusActive, items[0].Status)
}

func (s *PanelForwardServiceTestSuite) TestListUserTunnels_UsesAscendingRelationOrder() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-list-order@example.com",
		Password:       "hash",
		Token:          "panel-forward-list-order-token",
		UUID:           "panel-forward-list-order-uuid",
		TransferEnable: 1073741824,
	}
	tunnelA := &model.ForwardTunnel{Name: "Order Tunnel A", InIP: "10.0.2.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Order Tunnel B", InIP: "10.0.2.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	first := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnelA.ID, Status: model.ForwardUserTunnelStatusActive}
	second := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnelB.ID, Status: model.ForwardUserTunnelStatusActive}
	assert.NoError(s.T(), db.Create(first).Error)
	assert.NoError(s.T(), db.Create(second).Error)

	items, err := s.svc.ListUserTunnels(PanelUserTunnelQueryInput{UserID: user.ID})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 2)
	assert.Equal(s.T(), first.ID, items[0].ID)
	assert.Equal(s.T(), second.ID, items[1].ID)
}

func (s *PanelForwardServiceTestSuite) TestResetUserTunnelTraffic_ResetsPermissionAndMatchingForwardFlowsOnly() {
	db := database.Get()

	userA := &model.User{
		Email:          "panel-forward-reset-user-a@example.com",
		Password:       "hash",
		Token:          "panel-forward-reset-user-a-token",
		UUID:           "panel-forward-reset-user-a-uuid",
		TransferEnable: 1073741824,
	}
	userB := &model.User{
		Email:          "panel-forward-reset-user-b@example.com",
		Password:       "hash",
		Token:          "panel-forward-reset-user-b-token",
		UUID:           "panel-forward-reset-user-b-uuid",
		TransferEnable: 1073741824,
	}
	tunnelA := &model.ForwardTunnel{Name: "Reset Flow Tunnel A", InIP: "10.0.1.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Reset Flow Tunnel B", InIP: "10.0.1.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(userA).Error)
	assert.NoError(s.T(), db.Create(userB).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	perm := &model.ForwardUserTunnel{
		UserID:   userA.ID,
		TunnelID: tunnelA.ID,
		InFlow:   1234,
		OutFlow:  5678,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(perm).Error)

	target := &model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "Target Reset Forward",
		TunnelID:   tunnelA.ID,
		InPort:     14001,
		RemoteAddr: "target.example:443",
		InFlow:     1234,
		OutFlow:    5678,
		Status:     model.ForwardStatusPaused,
	}
	sameUserOtherTunnel := &model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "Same User Other Tunnel",
		TunnelID:   tunnelB.ID,
		InPort:     14002,
		RemoteAddr: "other-tunnel.example:443",
		InFlow:     111,
		OutFlow:    222,
		Status:     model.ForwardStatusPaused,
	}
	otherUserSameTunnel := &model.Forward{
		UserID:     userB.ID,
		UserName:   userB.Email,
		Name:       "Other User Same Tunnel",
		TunnelID:   tunnelA.ID,
		InPort:     14003,
		RemoteAddr: "other-user.example:443",
		InFlow:     333,
		OutFlow:    444,
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(target).Error)
	assert.NoError(s.T(), db.Create(sameUserOtherTunnel).Error)
	assert.NoError(s.T(), db.Create(otherUserSameTunnel).Error)

	assert.NoError(s.T(), s.svc.ResetUserTunnelTraffic(perm.ID))

	var reloadedTarget model.Forward
	assert.NoError(s.T(), db.First(&reloadedTarget, target.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedTarget.InFlow)
	assert.Equal(s.T(), int64(0), reloadedTarget.OutFlow)

	var reloadedSameUserOtherTunnel model.Forward
	assert.NoError(s.T(), db.First(&reloadedSameUserOtherTunnel, sameUserOtherTunnel.ID).Error)
	assert.Equal(s.T(), int64(111), reloadedSameUserOtherTunnel.InFlow)
	assert.Equal(s.T(), int64(222), reloadedSameUserOtherTunnel.OutFlow)

	var reloadedOtherUserSameTunnel model.Forward
	assert.NoError(s.T(), db.First(&reloadedOtherUserSameTunnel, otherUserSameTunnel.ID).Error)
	assert.Equal(s.T(), int64(333), reloadedOtherUserSameTunnel.InFlow)
	assert.Equal(s.T(), int64(444), reloadedOtherUserSameTunnel.OutFlow)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, perm.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedPermission.InFlow)
	assert.Equal(s.T(), int64(0), reloadedPermission.OutFlow)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_RejectsTunnelTrafficExhaustedFromPermissionCounters() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-permission-counter@example.com",
		Password:       "hash",
		Token:          "panel-forward-permission-counter-token",
		UUID:           "panel-forward-permission-counter-uuid",
		TransferEnable: 2 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Permission Counter Tunnel",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Flow:     1,
		InFlow:   bytesPerGiB / 2,
		OutFlow:  bytesPerGiB / 2,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	_, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Blocked by Permission Counter",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "tunnel traffic exhausted")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestAssignUserTunnel_RejectsSpeedLimitFromOtherTunnel() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-assign-speed-limit@example.com",
		Password:       "hash",
		Token:          "panel-forward-assign-speed-limit-token",
		UUID:           "panel-forward-assign-speed-limit-uuid",
		TransferEnable: 1073741824,
	}
	tunnelA := &model.ForwardTunnel{Name: "Assign Speed Tunnel A", InIP: "10.50.0.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Assign Speed Tunnel B", InIP: "10.50.0.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	speedLimit := &model.SpeedLimit{
		Name:        "Other Tunnel Speed",
		Speed:       512,
		TunnelID:    tunnelB.ID,
		TunnelName:  tunnelB.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	assert.NoError(s.T(), db.Create(speedLimit).Error)

	err := s.svc.AssignUserTunnel(PanelUserTunnelInput{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		SpeedID:  &speedLimit.ID,
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "does not belong to tunnel")
}

func (s *PanelForwardServiceTestSuite) TestUpdateUserTunnel_RejectsMissingSpeedLimit() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-update-missing-speed-limit@example.com",
		Password:       "hash",
		Token:          "panel-forward-update-missing-speed-limit-token",
		UUID:           "panel-forward-update-missing-speed-limit-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{Name: "Update Speed Tunnel", InIP: "10.50.0.3", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	perm := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(perm).Error)

	missingID := uint(99999)
	err := s.svc.UpdateUserTunnel(PanelUserTunnelUpdateInput{
		ID:      perm.ID,
		Status:  model.ForwardUserTunnelStatusActive,
		SpeedID: &missingID,
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "speed limit not found")
}

func (s *PanelForwardServiceTestSuite) TestListRuntimeJobsFilters() {
	db := database.Get()

	jobs := []model.ForwardRuntimeJob{
		{Backend: model.ForwardRuntimeBackendGost, Action: model.ForwardRuntimeJobActionCreate, ForwardID: uintPtr(1), Status: model.ForwardRuntimeJobStatusPending},
		{Backend: model.ForwardRuntimeBackendIptablesAnsible, Action: model.ForwardRuntimeJobActionCreate, ForwardID: uintPtr(2), Status: model.ForwardRuntimeJobStatusSuccess},
		{Backend: model.ForwardRuntimeBackendIptablesAnsible, Action: model.ForwardRuntimeJobActionDelete, ForwardID: uintPtr(1), Status: model.ForwardRuntimeJobStatusPending},
	}
	for _, job := range jobs {
		assert.NoError(s.T(), db.Create(&job).Error)
	}

	status := model.ForwardRuntimeJobStatusPending
	forwardID := uint(1)
	filter := PanelRuntimeJobFilter{
		Backend:   model.ForwardRuntimeBackendIptablesAnsible,
		Status:    &status,
		ForwardID: &forwardID,
		Limit:     5,
	}

	results, err := s.svc.ListRuntimeJobs(filter)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), results, 1)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionDelete, results[0].Action)

	filter.Limit = 1
	results, err = s.svc.ListRuntimeJobs(filter)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), results, 1)

	filter.ForwardID = nil
	filter.Status = nil
	filter.Limit = 2
	results, err = s.svc.ListRuntimeJobs(filter)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), results, 2)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, results[0].Backend)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, results[1].Backend)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_IptablesAnsibleExecutesViaNodeX() {
	db := database.Get()

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendIptablesAnsible,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      "/etc/ansible/hosts",
			ApplyPlaybook:  "/opt/ansible/iptables-forward-apply.yml",
			RemovePlaybook: "/opt/ansible/iptables-forward-remove.yml",
			Become:         true,
			ExtraVars: map[string]interface{}{
				"manage_with": "iptables",
			},
		},
		forwardRuntimeConfigGroup,
		"iptables ansible runtime config",
	))

	user := &model.User{
		Email:          "panel-forward-ansible@example.com",
		Password:       "hash",
		Token:          "panel-forward-ansible-token",
		UUID:           "panel-forward-ansible-uuid",
		TransferEnable: 1073741824,
	}
	node := &model.ForwardNode{
		Name:    "Ansible Ingress",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.10.10.10",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	tunnel := &model.ForwardTunnel{
		Name:     "Ansible Tunnel",
		InNodeID: node.ID,
		InIP:     "10.10.10.10",
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnel.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	item, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Ansible Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, item.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, item.RuntimeStatus)
	assert.Equal(s.T(), "ansible runtime applied", item.RuntimeMessage)
	assert.Equal(s.T(), 1, s.runtimeClient.calls)
	if assert.NotNil(s.T(), s.runtimeClient.lastRequest) {
		assert.Equal(s.T(), nodeXForwardResourceTypePanelForward, s.runtimeClient.lastRequest.ResourceType)
		assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, s.runtimeClient.lastRequest.Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, s.runtimeClient.lastRequest.Action)
		assert.Equal(s.T(), tunnel.ID, s.runtimeClient.lastRequest.PanelForward.Tunnel.ID)
	}

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, item.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusActive, record.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, record.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, record.RuntimeStatus)
	assert.NotNil(s.T(), record.RuntimeLastSyncAt)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Order("id ASC").Find(&jobs).Error)
	assert.Len(s.T(), jobs, 1)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, jobs[0].Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, jobs[0].Action)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, jobs[0].Status)
	assert.Contains(s.T(), jobs[0].Payload, `"resourceType":"panel_forward"`)
	assert.Contains(s.T(), jobs[0].Payload, `"backend":"iptables_ansible"`)
	assert.NotNil(s.T(), jobs[0].StartedAt)
	assert.NotNil(s.T(), jobs[0].CompletedAt)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_DefaultGostMarksRuntimeFailureWithoutIngressNode() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-gost-fail@example.com",
		Password:       "hash",
		Token:          "panel-forward-gost-fail-token",
		UUID:           "panel-forward-gost-fail-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Gost Tunnel Missing Node",
		InIP:   "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	item, err := s.svc.CreateForward(user.ID, false, PanelForwardInput{
		Name:       "Gost Failure Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), model.ForwardStatusError, item.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, item.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, item.RuntimeStatus)
	assert.Contains(s.T(), item.RuntimeMessage, "ingress node is not configured")

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, item.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusError, record.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, record.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, record.RuntimeStatus)
	assert.NotNil(s.T(), record.RuntimeLastSyncAt)

	var jobCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Count(&jobCount).Error)
	assert.Equal(s.T(), int64(0), jobCount)
}

func TestPanelForwardService(t *testing.T) {
	suite.Run(t, new(PanelForwardServiceTestSuite))
}
