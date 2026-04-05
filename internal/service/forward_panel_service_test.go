package service

import (
	"testing"
	"time"

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
	tunnelA := &model.ForwardTunnel{Name: "Cascade Tunnel A", InIP: "10.0.0.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Cascade Tunnel B", InIP: "10.0.0.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(userA).Error)
	assert.NoError(s.T(), db.Create(userB).Error)
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

	perm := &model.ForwardUserTunnel{
		UserID:        user.ID,
		TunnelID:      tunnel.ID,
		Flow:          100,
		Num:           5,
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
	assert.Equal(s.T(), speed, items[0].Speed)
	assert.Equal(s.T(), tunnel.Name, items[0].TunnelName)
	assert.Equal(s.T(), tunnel.Flow, items[0].TunnelFlow)
	assert.Equal(s.T(), int64(1000), items[0].InFlow)
	assert.Equal(s.T(), int64(2000), items[0].OutFlow)
	assert.Equal(s.T(), model.ForwardUserTunnelStatusActive, items[0].Status)
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

func (s *PanelForwardServiceTestSuite) TestCreateForward_IptablesAnsibleQueuesRuntimeJob() {
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
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, item.RuntimeStatus)
	assert.Contains(s.T(), item.RuntimeMessage, "queued ansible runtime job")

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, item.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusActive, record.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, record.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, record.RuntimeStatus)
	assert.NotNil(s.T(), record.RuntimeLastSyncAt)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Order("id ASC").Find(&jobs).Error)
	assert.Len(s.T(), jobs, 1)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, jobs[0].Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, jobs[0].Action)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, jobs[0].Status)
	assert.Contains(s.T(), jobs[0].Payload, "/opt/ansible/iptables-forward-apply.yml")
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
