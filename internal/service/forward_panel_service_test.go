package service

import (
	"context"
	"fmt"
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
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{},
		&model.SpeedLimit{},
	)
}

func (s *PanelForwardServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward_traffic_cursor")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_speed_limit")
	db.Exec("DELETE FROM v2_forward_tunnel")
	db.Exec("DELETE FROM v2_forward_node")
	s.svc = NewPanelForwardService(db)
	s.runtimeClient = &stubNodeXForwardRuntimeClient{}
	s.svc.runtimeService.client = s.runtimeClient
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

func (s *PanelForwardServiceTestSuite) createForwardNode(name, host string, status int) *model.ForwardNode {
	return s.createForwardNodeOfType(name, host, status, model.ForwardNodeTypeRelay)
}

func (s *PanelForwardServiceTestSuite) createForwardNodeOfType(name, host string, status int, nodeType string) *model.ForwardNode {
	node := &model.ForwardNode{
		Name:    name,
		Type:    nodeType,
		Host:    host,
		Port:    20001,
		Status:  status,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(node).Error)
	return node
}

func float64Ptr(value float64) *float64 {
	return &value
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

func (s *PanelForwardServiceTestSuite) TestListAdminTunnels_ReturnsFluxCompatibleDTOsInNameOrder() {
	db := database.Get()

	outNodeID := uint(22)
	first := &model.ForwardTunnel{
		Name:          "Beta Tunnel",
		InNodeID:      11,
		OutNodeID:     &outNodeID,
		InIP:          "10.10.10.1",
		OutIP:         "10.10.10.2",
		Type:          2,
		Flow:          2,
		TrafficRatio:  1.5,
		InterfaceName: "eth1",
		Protocol:      "tls",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	second := &model.ForwardTunnel{
		Name:          "Alpha Tunnel",
		InNodeID:      33,
		InIP:          "10.20.20.1",
		OutIP:         "10.20.20.1",
		Type:          1,
		Flow:          1,
		TrafficRatio:  2,
		Protocol:      "",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusDisabled,
	}
	assert.NoError(s.T(), db.Create(first).Error)
	assert.NoError(s.T(), db.Create(second).Error)

	items, err := s.svc.ListAdminTunnels()
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 2)
	assert.Equal(s.T(), "Alpha Tunnel", items[0].Name)
	assert.Equal(s.T(), second.ID, items[0].ID)
	assert.Equal(s.T(), second.InNodeID, items[0].InNodeID)
	assert.Equal(s.T(), second.Type, items[0].Type)
	assert.Equal(s.T(), second.Flow, items[0].Flow)
	assert.Equal(s.T(), second.TrafficRatio, items[0].TrafficRatio)
	assert.Equal(s.T(), second.Status, items[0].Status)
	assert.Equal(s.T(), "Beta Tunnel", items[1].Name)
	assert.Equal(s.T(), first.OutNodeID, items[1].OutNodeID)
	assert.Equal(s.T(), first.OutIP, items[1].OutIP)
	assert.Equal(s.T(), first.Protocol, items[1].Protocol)
	assert.NotZero(s.T(), items[1].CreatedTime)
	assert.NotZero(s.T(), items[1].UpdatedTime)
}

func (s *PanelForwardServiceTestSuite) TestCreateTunnel_SucceedsWithResolvedNodeIPsAndDefaults() {
	inNode := s.createForwardNode("Ingress Node", "10.0.0.1", model.ForwardNodeStatusOnline)
	outNode := s.createForwardNodeOfType("Egress Node", "10.0.0.2", model.ForwardNodeStatusOnline, model.ForwardNodeTypeExit)

	item, err := s.svc.CreateTunnel(PanelTunnelInput{
		Name:      "Create Tunnel",
		InNodeID:  inNode.ID,
		OutNodeID: &outNode.ID,
		Type:      2,
		Flow:      1,
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), inNode.ID, item.InNodeID)
	assert.Equal(s.T(), &outNode.ID, item.OutNodeID)
	assert.Equal(s.T(), "10.0.0.1", item.InIP)
	assert.Equal(s.T(), "10.0.0.2", item.OutIP)
	assert.Equal(s.T(), "tls", item.Protocol)
	assert.Equal(s.T(), 1.0, item.TrafficRatio)
	assert.Equal(s.T(), "[::]", item.TCPListenAddr)
	assert.Equal(s.T(), "[::]", item.UDPListenAddr)

	var record model.ForwardTunnel
	assert.NoError(s.T(), database.Get().First(&record, item.ID).Error)
	assert.Equal(s.T(), inNode.ID, record.InNodeID)
	assert.Equal(s.T(), &outNode.ID, record.OutNodeID)
	assert.Equal(s.T(), "10.0.0.1", record.InIP)
	assert.Equal(s.T(), "10.0.0.2", record.OutIP)
	assert.Equal(s.T(), 1.0, record.TrafficRatio)
	assert.Equal(s.T(), "tls", record.Protocol)
}

func (s *PanelForwardServiceTestSuite) TestCreateTunnel_RejectsDuplicateNameAndInvalidType2Nodes() {
	db := database.Get()
	inNode := s.createForwardNode("Ingress Node", "10.0.0.3", model.ForwardNodeStatusOnline)
	outNode := s.createForwardNodeOfType("Egress Node", "10.0.0.4", model.ForwardNodeStatusOnline, model.ForwardNodeTypeExit)
	wrongRelayOutNode := s.createForwardNode("Relay Out Node", "10.0.0.5", model.ForwardNodeStatusOnline)
	assert.NoError(s.T(), db.Create(&model.ForwardTunnel{
		Name:   "Duplicate Tunnel",
		InIP:   "127.0.0.1",
		OutIP:  "127.0.0.1",
		Status: model.ForwardTunnelStatusActive,
	}).Error)

	_, err := s.svc.CreateTunnel(PanelTunnelInput{
		Name:     "Duplicate Tunnel",
		InNodeID: inNode.ID,
		Type:     1,
		Flow:     1,
	})
	assert.Error(s.T(), err)

	_, err = s.svc.CreateTunnel(PanelTunnelInput{
		Name:     "Missing Out Node",
		InNodeID: inNode.ID,
		Type:     2,
		Flow:     1,
	})
	assert.Error(s.T(), err)

	_, err = s.svc.CreateTunnel(PanelTunnelInput{
		Name:      "Same Node Tunnel",
		InNodeID:  inNode.ID,
		OutNodeID: &inNode.ID,
		Type:      2,
		Flow:      1,
	})
	assert.Error(s.T(), err)

	_, err = s.svc.CreateTunnel(PanelTunnelInput{
		Name:      "Wrong Out Type Tunnel",
		InNodeID:  inNode.ID,
		OutNodeID: &wrongRelayOutNode.ID,
		Type:      2,
		Flow:      1,
	})
	assert.Error(s.T(), err)

	var count int64
	assert.NoError(s.T(), db.Model(&model.ForwardTunnel{}).Where("name IN ?", []string{"Missing Out Node", "Same Node Tunnel", "Wrong Out Type Tunnel"}).Count(&count).Error)
	assert.Zero(s.T(), count)

	_, err = s.svc.CreateTunnel(PanelTunnelInput{
		Name:      "Valid Type2 Tunnel",
		InNodeID:  inNode.ID,
		OutNodeID: &outNode.ID,
		Type:      2,
		Flow:      1,
	})
	assert.NoError(s.T(), err)
}

func (s *PanelForwardServiceTestSuite) TestCreateTunnel_AnsibleExecNodeOnly() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)
	defer setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)
	defer setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)

	execNode := s.createForwardNode("Ansible Execution Node", "10.0.0.77", model.ForwardNodeStatusOnline)
	outNodeID := execNode.ID

	tunnel, err := s.svc.CreateTunnel(PanelTunnelInput{
		Name:          "Ansible Exec Only Tunnel",
		Flow:          1,
		Type:          1,
		InNodeID:      0,
		OutNodeID:     &outNodeID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "127.0.0.1",
		TrafficRatio:  float64Ptr(1),
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), tunnel)

	var record model.ForwardTunnel
	assert.NoError(s.T(), db.First(&record, tunnel.ID).Error)
	assert.Equal(s.T(), uint(0), record.InNodeID)
	assert.NotNil(s.T(), record.OutNodeID)
	assert.Equal(s.T(), execNode.ID, *record.OutNodeID)
}

func (s *PanelForwardServiceTestSuite) TestValidatePanelTunnelCreateInput_AnsibleRejectsType2() {
	outNodeID := uint(2)
	err := validatePanelTunnelCreateInput(PanelTunnelInput{
		Name:      "Validator Type2",
		InNodeID:  1,
		OutNodeID: &outNodeID,
		Type:      2,
		Flow:      1,
	}, 1, model.ForwardRuntimeBackendIptablesAnsible)
	if assert.Error(s.T(), err) {
		assert.Contains(s.T(), err.Error(), "Ansible 转发模式仅支持端口转发")
	}
}

func (s *PanelForwardServiceTestSuite) TestUpdateTunnel_UpdatesEditableFieldsOnly() {
	db := database.Get()
	outNodeID := uint(44)
	record := &model.ForwardTunnel{
		Name:          "Editable Tunnel",
		InNodeID:      11,
		OutNodeID:     &outNodeID,
		InIP:          "10.30.30.1",
		OutIP:         "10.30.30.2",
		Type:          2,
		Flow:          1,
		TrafficRatio:  1,
		InterfaceName: "eth0",
		Protocol:      "tls",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(record).Error)

	item, err := s.svc.UpdateTunnel(PanelTunnelUpdateInput{
		ID:            record.ID,
		Name:          "Updated Tunnel",
		Flow:          2,
		TrafficRatio:  float64Ptr(3.5),
		InterfaceName: "eth2",
		Protocol:      "ws",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "127.0.0.1",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), "Updated Tunnel", item.Name)
	assert.Equal(s.T(), 2, item.Flow)
	assert.Equal(s.T(), 3.5, item.TrafficRatio)
	assert.Equal(s.T(), "eth2", item.InterfaceName)
	assert.Equal(s.T(), "ws", item.Protocol)
	assert.Equal(s.T(), "0.0.0.0", item.TCPListenAddr)
	assert.Equal(s.T(), "127.0.0.1", item.UDPListenAddr)

	var updated model.ForwardTunnel
	assert.NoError(s.T(), db.First(&updated, record.ID).Error)
	assert.Equal(s.T(), record.InNodeID, updated.InNodeID)
	assert.Equal(s.T(), record.OutNodeID, updated.OutNodeID)
	assert.Equal(s.T(), record.InIP, updated.InIP)
	assert.Equal(s.T(), record.OutIP, updated.OutIP)
	assert.Equal(s.T(), record.Type, updated.Type)
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestUpdateTunnel_RuntimeFieldChangeResyncsActiveForwards() {
	db := database.Get()
	inNode := s.createForwardNode("Ingress Node", "10.0.0.6", model.ForwardNodeStatusOnline)
	outNode := s.createForwardNodeOfType("Egress Node", "10.0.0.7", model.ForwardNodeStatusOnline, model.ForwardNodeTypeExit)
	user := &model.User{
		Email:          "panel-forward-runtime-sync@example.com",
		Password:       "hash",
		Token:          "panel-forward-runtime-sync-token",
		UUID:           "panel-forward-runtime-sync-uuid",
		TransferEnable: 1073741824,
	}
	assert.NoError(s.T(), db.Create(user).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Runtime Sync Tunnel",
		InNodeID:      inNode.ID,
		OutNodeID:     &outNode.ID,
		InIP:          inNode.Host,
		OutIP:         outNode.Host,
		Type:          2,
		Flow:          1,
		TrafficRatio:  1,
		InterfaceName: "eth0",
		Protocol:      "tls",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)
	forward := &model.Forward{
		UserID:        user.ID,
		UserName:      user.Email,
		Name:          "Active Forward",
		TunnelID:      tunnel.ID,
		InPort:        12001,
		RemoteAddr:    "example.com:443",
		InterfaceName: "eth0",
		Strategy:      "fifo",
		Status:        model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	item, err := s.svc.UpdateTunnel(PanelTunnelUpdateInput{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		Flow:          tunnel.Flow,
		TrafficRatio:  float64Ptr(tunnel.TrafficRatio),
		InterfaceName: "eth9",
		Protocol:      "ws",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "[::1]",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), 1, s.runtimeClient.calls)
	assert.NotNil(s.T(), s.runtimeClient.lastRequest)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionUpdate, s.runtimeClient.lastRequest.Action)
	assert.NotNil(s.T(), s.runtimeClient.lastRequest.PanelForward)
	assert.Equal(s.T(), forward.ID, s.runtimeClient.lastRequest.PanelForward.Forward.ID)
	assert.Equal(s.T(), "tcp", s.runtimeClient.lastRequest.PanelForward.Tunnel.Protocol)
	assert.Equal(s.T(), "eth9", s.runtimeClient.lastRequest.PanelForward.Tunnel.InterfaceName)

	var updated model.ForwardTunnel
	assert.NoError(s.T(), db.First(&updated, tunnel.ID).Error)
	assert.Equal(s.T(), "ws", updated.Protocol)
	assert.Equal(s.T(), "eth9", updated.InterfaceName)
}

func (s *PanelForwardServiceTestSuite) TestDeleteTunnel_RejectsWhenReferencedAndDeletesWhenUnused() {
	db := database.Get()

	forwardTunnel := &model.ForwardTunnel{Name: "Forward Referenced Tunnel", InIP: "10.40.40.1", OutIP: "10.40.40.1", Status: model.ForwardTunnelStatusActive}
	permissionTunnel := &model.ForwardTunnel{Name: "Permission Referenced Tunnel", InIP: "10.40.40.2", OutIP: "10.40.40.2", Status: model.ForwardTunnelStatusActive}
	unusedTunnel := &model.ForwardTunnel{Name: "Unused Tunnel", InIP: "10.40.40.3", OutIP: "10.40.40.3", Status: model.ForwardTunnelStatusActive}
	user := &model.User{
		Email:          "panel-forward-delete@example.com",
		Password:       "hash",
		Token:          "panel-forward-delete-token",
		UUID:           "panel-forward-delete-uuid",
		TransferEnable: 1073741824,
	}
	assert.NoError(s.T(), db.Create(forwardTunnel).Error)
	assert.NoError(s.T(), db.Create(permissionTunnel).Error)
	assert.NoError(s.T(), db.Create(unusedTunnel).Error)
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Using Forward",
		TunnelID:   forwardTunnel.ID,
		InPort:     13001,
		RemoteAddr: "example.com:8443",
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: permissionTunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	err := s.svc.DeleteTunnel(forwardTunnel.ID)
	assert.Error(s.T(), err)

	err = s.svc.DeleteTunnel(permissionTunnel.ID)
	assert.Error(s.T(), err)

	err = s.svc.DeleteTunnel(unusedTunnel.ID)
	assert.NoError(s.T(), err)

	var count int64
	assert.NoError(s.T(), db.Model(&model.ForwardTunnel{}).Where("id = ?", unusedTunnel.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *PanelForwardServiceTestSuite) TestCreateTunnel_AllowsOfflineForwardNodes() {
	inNode := s.createForwardNode("Offline In Node", "10.0.0.6", model.ForwardNodeStatusOffline)
	outNode := s.createForwardNodeOfType("Offline Out Node", "10.0.0.7", model.ForwardNodeStatusOffline, model.ForwardNodeTypeExit)

	tunnel, err := s.svc.CreateTunnel(PanelTunnelInput{
		Name:      "Offline Friendly Tunnel",
		InNodeID:  inNode.ID,
		OutNodeID: &outNode.ID,
		Type:      2,
		Flow:      1,
	})
	assert.NoError(s.T(), err)
	if assert.NotNil(s.T(), tunnel) {
		assert.Equal(s.T(), inNode.ID, tunnel.InNodeID)
		assert.Equal(s.T(), outNode.ID, *tunnel.OutNodeID)
		assert.Equal(s.T(), model.ForwardTunnelStatusActive, tunnel.Status)
		assert.Equal(s.T(), "Offline Friendly Tunnel", tunnel.Name)
	}
}

func (s *PanelForwardServiceTestSuite) TestDeleteForward_RequiresForceWhenActive() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-delete-active@example.com",
		Password:       "hash",
		Token:          "panel-forward-delete-active-token",
		UUID:           "panel-forward-delete-active-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Delete Active Tunnel",
		InIP:   "10.60.0.1",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Delete Active Forward",
		TunnelID:   tunnel.ID,
		InPort:     14001,
		RemoteAddr: "delete-active.example.com:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	err := s.svc.DeleteForward(user.ID, true, forward.ID, false)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "强制删除")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Equal(s.T(), int64(1), count)
}

func (s *PanelForwardServiceTestSuite) TestDeleteForward_ForceDeletesRecordWhenRuntimeSyncFails() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-force-delete@example.com",
		Password:       "hash",
		Token:          "panel-forward-force-delete-token",
		UUID:           "panel-forward-force-delete-uuid",
		TransferEnable: 1073741824,
	}
	node := &model.ForwardNode{
		Name:     "Force Delete Ingress",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.60",
		Port:     22,
		APIPort:  19090,
		APIToken: "force-delete-token",
		Status:   model.ForwardNodeStatusOnline,
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Force Delete Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:         user.ID,
		UserName:       user.Email,
		Name:           "Force Delete Forward",
		TunnelID:       tunnel.ID,
		InPort:         14002,
		RemoteAddr:     "force-delete.example.com:443",
		Status:         model.ForwardStatusPaused,
		RuntimeBackend: model.ForwardRuntimeBackendGost,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   100,
		DownloadTotal: 200,
	}).Error)

	s.runtimeClient.executeErr = assert.AnError

	err := s.svc.DeleteForward(user.ID, true, forward.ID, true)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, s.runtimeClient.calls)

	var forwardCount int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&forwardCount).Error)
	assert.Zero(s.T(), forwardCount)

	var cursorCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardTrafficCursor{}).Where("forward_id = ?", forward.ID).Count(&cursorCount).Error)
	assert.Zero(s.T(), cursorCount)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Where("forward_id = ?", forward.ID).Find(&jobs).Error)
	if assert.Len(s.T(), jobs, 1) {
		assert.Equal(s.T(), model.ForwardRuntimeJobActionDelete, jobs[0].Action)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, jobs[0].Status)
		assert.Contains(s.T(), jobs[0].Error, assert.AnError.Error())
	}
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

func (s *PanelForwardServiceTestSuite) TestDeleteForward_ForceDeleteIgnoresRuntimeFailureAndRemovesRecord() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendGost,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"http://127.0.0.1:18080",
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime nodex base url",
	))

	node := s.createForwardNode("Delete Force Ingress", "198.51.100.60", model.ForwardNodeStatusOnline)
	node.APIPort = 19090
	node.APIToken = "delete-force-token"
	assert.NoError(s.T(), db.Save(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Delete Force Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     1,
		UserName:   "force-delete@example.com",
		Name:       "Force Delete Forward",
		TunnelID:   tunnel.ID,
		InPort:     22001,
		RemoteAddr: "example.com:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Model(forward).Update("status", model.ForwardStatusPaused).Error)

	s.runtimeClient.executeErr = assert.AnError

	err := s.svc.DeleteForward(0, true, forward.ID, true)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, s.runtimeClient.calls)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *PanelForwardServiceTestSuite) TestDeleteForward_NonForceDeleteReturnsRuntimeFailureAndKeepsRecord() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendGost,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"http://127.0.0.1:18080",
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime nodex base url",
	))

	node := s.createForwardNode("Delete NonForce Ingress", "198.51.100.61", model.ForwardNodeStatusOnline)
	node.APIPort = 19091
	node.APIToken = "delete-nonforce-token"
	assert.NoError(s.T(), db.Save(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Delete NonForce Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     1,
		UserName:   "nonforce-delete@example.com",
		Name:       "NonForce Delete Forward",
		TunnelID:   tunnel.ID,
		InPort:     22002,
		RemoteAddr: "example.com:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Model(forward).Update("status", model.ForwardStatusPaused).Error)

	s.runtimeClient.executeErr = assert.AnError

	err := s.svc.DeleteForward(0, true, forward.ID, false)
	assert.ErrorIs(s.T(), err, assert.AnError)
	assert.Equal(s.T(), 1, s.runtimeClient.calls)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Equal(s.T(), int64(1), count)
}

func (s *PanelForwardServiceTestSuite) TestDeleteForward_NonForceDeleteRejectsActiveForwardBeforeRuntimeCall() {
	db := database.Get()

	forward := &model.Forward{
		UserID:     1,
		UserName:   "active-delete@example.com",
		Name:       "Active Delete Forward",
		TunnelID:   1,
		InPort:     22003,
		RemoteAddr: "example.com:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	err := s.svc.DeleteForward(0, true, forward.ID, false)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "请先暂停或使用强制删除")
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Equal(s.T(), int64(1), count)
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

func (s *PanelForwardServiceTestSuite) TestUpdateUserTunnel_SpeedChangeResyncsAffectedActiveForwards() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)

	user := &model.User{
		Email:          "panel-forward-speed-change@example.com",
		Password:       "hash",
		Token:          "panel-forward-speed-change-token",
		UUID:           "panel-forward-speed-change-uuid",
		TransferEnable: 10 * bytesPerGiB,
	}
	node := &model.ForwardNode{
		Name:     "Speed Change Ingress",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "198.51.100.30",
		APIPort:  19091,
		APIToken: "speed-change-token",
		Status:   model.ForwardNodeStatusOnline,
		Enabled:  true,
	}
	tunnel := &model.ForwardTunnel{
		Name:          "Speed Change Tunnel",
		InNodeID:      0,
		InIP:          "198.51.100.31",
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnel.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnel).Error)

	initialLimit := &model.SpeedLimit{
		Name:        "10M",
		Speed:       80,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	updatedLimit := &model.SpeedLimit{
		Name:        "20M",
		Speed:       160,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	assert.NoError(s.T(), db.Create(initialLimit).Error)
	assert.NoError(s.T(), db.Create(updatedLimit).Error)

	permission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Flow:     100,
		Num:      10,
		SpeedID:  &initialLimit.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permission).Error)

	activeA := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Speed-Active-A",
		TunnelID:   tunnel.ID,
		InPort:     13101,
		RemoteAddr: "speed-a.example:443",
		Status:     model.ForwardStatusActive,
	}
	activeB := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Speed-Active-B",
		TunnelID:   tunnel.ID,
		InPort:     13102,
		RemoteAddr: "speed-b.example:443",
		Status:     model.ForwardStatusActive,
	}
	paused := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Speed-Paused",
		TunnelID:   tunnel.ID,
		InPort:     13103,
		RemoteAddr: "speed-paused.example:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(s.T(), db.Create(activeA).Error)
	assert.NoError(s.T(), db.Create(activeB).Error)
	assert.NoError(s.T(), db.Create(paused).Error)
	assert.NoError(s.T(), db.Model(paused).Update("status", model.ForwardStatusPaused).Error)

	err := s.svc.UpdateUserTunnel(PanelUserTunnelUpdateInput{
		ID:            permission.ID,
		Flow:          permission.Flow,
		Num:           permission.Num,
		FlowResetTime: permission.FlowResetTime,
		ExpTime:       permission.ExpTime,
		Status:        model.ForwardUserTunnelStatusActive,
		SpeedID:       &updatedLimit.ID,
	})
	assert.NoError(s.T(), err)

	assert.Len(s.T(), s.runtimeClient.requests, 2)
	for _, req := range s.runtimeClient.requests {
		assert.Equal(s.T(), model.ForwardRuntimeJobActionUpdate, req.Action)
		if assert.NotNil(s.T(), req.PanelForward) && assert.NotNil(s.T(), req.PanelForward.Limiter) {
			assert.Equal(s.T(), updatedLimit.ID, req.PanelForward.Limiter.SpeedID)
			assert.Equal(s.T(), updatedLimit.Speed, req.PanelForward.Limiter.Speed)
		}
	}

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permission.ID).Error)
	if assert.NotNil(s.T(), reloadedPermission.SpeedID) {
		assert.Equal(s.T(), updatedLimit.ID, *reloadedPermission.SpeedID)
	}
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

func (s *PanelForwardServiceTestSuite) TestUploadFluxForwardFlow_AccumulatesForwardUserAndUserTunnelWithRatioAndFlow() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-flow-upload@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-upload-token",
		UUID:           "panel-forward-flow-upload-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:         "Flow Upload Tunnel",
		InIP:         "198.51.100.10",
		Flow:         2,
		TrafficRatio: 1.5,
		Status:       model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	permission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permission).Error)

	forward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Flux Upload Forward",
		TunnelID:   tunnel.ID,
		InPort:     15001,
		RemoteAddr: "upload.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	err := s.svc.UploadFluxForwardFlow(PanelForwardFlowData{
		N: forwardFlowServiceName(forward.ID, user.ID, permission.ID),
		U: 100,
		D: 200,
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), int64(600), reloadedForward.InFlow)
	assert.Equal(s.T(), int64(300), reloadedForward.OutFlow)

	var reloadedUser model.User
	assert.NoError(s.T(), db.First(&reloadedUser, user.ID).Error)
	assert.Equal(s.T(), int64(300), reloadedUser.U)
	assert.Equal(s.T(), int64(600), reloadedUser.D)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permission.ID).Error)
	assert.Equal(s.T(), int64(600), reloadedPermission.InFlow)
	assert.Equal(s.T(), int64(300), reloadedPermission.OutFlow)
}

func (s *PanelForwardServiceTestSuite) TestApplyForwardTrafficSnapshots_TracksDeltaCursorAndCounterReset() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-flow-snapshot@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-snapshot-token",
		UUID:           "panel-forward-flow-snapshot-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:   "Snapshot Tunnel",
		InIP:   "198.51.100.11",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	permission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permission).Error)

	forward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Snapshot Forward",
		TunnelID:   tunnel.ID,
		InPort:     15002,
		RemoteAddr: "snapshot.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   1000,
		DownloadTotal: 2000,
	}}))
	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   1200,
		DownloadTotal: 2500,
	}}))
	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   100,
		DownloadTotal: 150,
	}}))

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), int64(2650), reloadedForward.InFlow)
	assert.Equal(s.T(), int64(1300), reloadedForward.OutFlow)

	var reloadedUser model.User
	assert.NoError(s.T(), db.First(&reloadedUser, user.ID).Error)
	assert.Equal(s.T(), int64(1300), reloadedUser.U)
	assert.Equal(s.T(), int64(2650), reloadedUser.D)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permission.ID).Error)
	assert.Equal(s.T(), int64(2650), reloadedPermission.InFlow)
	assert.Equal(s.T(), int64(1300), reloadedPermission.OutFlow)

	var cursor model.ForwardTrafficCursor
	assert.NoError(s.T(), db.Where("forward_id = ? AND backend = ?", forward.ID, model.ForwardRuntimeBackendGost).First(&cursor).Error)
	assert.Equal(s.T(), int64(100), cursor.UploadTotal)
	assert.Equal(s.T(), int64(150), cursor.DownloadTotal)

	var cursorCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardTrafficCursor{}).Count(&cursorCount).Error)
	assert.Equal(s.T(), int64(1), cursorCount)
	assert.Equal(s.T(), 0, s.runtimeClient.calls)
}

func (s *PanelForwardServiceTestSuite) TestUploadFluxForwardFlow_UserTrafficExhaustedPausesAllUserForwards() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-flow-user-pause@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-user-pause-token",
		UUID:           "panel-forward-flow-user-pause-uuid",
		TransferEnable: 1000,
	}
	node := &model.ForwardNode{
		Name:    "User Pause Ingress",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "203.0.113.10",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	tunnelA := &model.ForwardTunnel{
		Name:     "User Pause Tunnel A",
		InNodeID: 0,
		InIP:     "203.0.113.20",
		Flow:     1,
		Status:   model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:     "User Pause Tunnel B",
		InNodeID: 0,
		InIP:     "203.0.113.21",
		Flow:     1,
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnelA.InNodeID = node.ID
	tunnelB.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	permission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		Flow:     10,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permission).Error)

	first := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Pause User Forward A",
		TunnelID:   tunnelA.ID,
		InPort:     15003,
		RemoteAddr: "user-a.example:443",
		Status:     model.ForwardStatusActive,
	}
	second := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Pause User Forward B",
		TunnelID:   tunnelB.ID,
		InPort:     15004,
		RemoteAddr: "user-b.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(first).Error)
	assert.NoError(s.T(), db.Create(second).Error)

	err := s.svc.UploadFluxForwardFlow(PanelForwardFlowData{
		N: forwardFlowServiceName(first.ID, user.ID, permission.ID),
		U: 600,
		D: 400,
	})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), s.runtimeClient.requests, 2)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionPause, s.runtimeClient.requests[0].Action)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionPause, s.runtimeClient.requests[1].Action)

	var reloadedFirst model.Forward
	assert.NoError(s.T(), db.First(&reloadedFirst, first.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedFirst.Status)

	var reloadedSecond model.Forward
	assert.NoError(s.T(), db.First(&reloadedSecond, second.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedSecond.Status)

	var reloadedUser model.User
	assert.NoError(s.T(), db.First(&reloadedUser, user.ID).Error)
	assert.Equal(s.T(), int64(600), reloadedUser.U)
	assert.Equal(s.T(), int64(400), reloadedUser.D)
}

func (s *PanelForwardServiceTestSuite) TestUploadFluxForwardFlow_TunnelTrafficExhaustedPausesOnlyAffectedTunnelForwards() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-flow-tunnel-pause@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-tunnel-pause-token",
		UUID:           "panel-forward-flow-tunnel-pause-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	node := &model.ForwardNode{
		Name:    "Tunnel Pause Ingress",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "203.0.113.11",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	tunnelA := &model.ForwardTunnel{
		Name:     "Tunnel Pause A",
		InNodeID: 0,
		InIP:     "203.0.113.30",
		Flow:     1,
		Status:   model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:     "Tunnel Pause B",
		InNodeID: 0,
		InIP:     "203.0.113.31",
		Flow:     1,
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)
	tunnelA.InNodeID = node.ID
	tunnelB.InNodeID = node.ID
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	permission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnelA.ID,
		Flow:     1,
		InFlow:   bytesPerGiB - 400,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permission).Error)

	first := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Pause Tunnel Forward A1",
		TunnelID:   tunnelA.ID,
		InPort:     15005,
		RemoteAddr: "tunnel-a1.example:443",
		InFlow:     bytesPerGiB - 400,
		Status:     model.ForwardStatusActive,
	}
	second := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Pause Tunnel Forward A2",
		TunnelID:   tunnelA.ID,
		InPort:     15006,
		RemoteAddr: "tunnel-a2.example:443",
		Status:     model.ForwardStatusActive,
	}
	otherTunnel := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Pause Tunnel Forward B",
		TunnelID:   tunnelB.ID,
		InPort:     15007,
		RemoteAddr: "tunnel-b.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(first).Error)
	assert.NoError(s.T(), db.Create(second).Error)
	assert.NoError(s.T(), db.Create(otherTunnel).Error)

	err := s.svc.UploadFluxForwardFlow(PanelForwardFlowData{
		N: forwardFlowServiceName(first.ID, user.ID, permission.ID),
		D: 500,
	})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), s.runtimeClient.requests, 2)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionPause, s.runtimeClient.requests[0].Action)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionPause, s.runtimeClient.requests[1].Action)

	var reloadedFirst model.Forward
	assert.NoError(s.T(), db.First(&reloadedFirst, first.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedFirst.Status)

	var reloadedSecond model.Forward
	assert.NoError(s.T(), db.First(&reloadedSecond, second.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedSecond.Status)

	var reloadedOtherTunnel model.Forward
	assert.NoError(s.T(), db.First(&reloadedOtherTunnel, otherTunnel.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusActive, reloadedOtherTunnel.Status)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permission.ID).Error)
	assert.Equal(s.T(), int64(bytesPerGiB+100), reloadedPermission.InFlow)
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

func (s *PanelForwardServiceTestSuite) TestCreateForward_IptablesAnsibleQueuesLocalExecutorJob() {
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
	assert.Equal(s.T(), "ansible runtime queued for local executor", item.RuntimeMessage)
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

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
	assert.Contains(s.T(), jobs[0].Payload, `"action":"create"`)
	assert.Contains(s.T(), jobs[0].Payload, `"inventory":"/etc/ansible/hosts"`)
	assert.Contains(s.T(), jobs[0].Payload, `"playbook":"/opt/ansible/iptables-forward-apply.yml"`)
	assert.Contains(s.T(), jobs[0].Payload, `"forward":{"id":`)
	assert.Nil(s.T(), jobs[0].StartedAt)
	assert.Nil(s.T(), jobs[0].CompletedAt)
}

func (s *PanelForwardServiceTestSuite) TestCreateForward_DefaultGostWithoutNodeXBaseURLMarksRuntimeFailure() {
	db := database.Get()

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"",
		"string",
		forwardRuntimeConfigGroup,
		"clear test NodeX runtime URL",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendGost,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))

	user := &model.User{
		Email:          "panel-forward-gost-fallback@example.com",
		Password:       "hash",
		Token:          "panel-forward-gost-fallback-token",
		UUID:           "panel-forward-gost-fallback-uuid",
		TransferEnable: 1073741824,
	}
	node := &model.ForwardNode{
		Name:     "Fallback Ingress",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "10.10.20.10",
		APIPort:  19080,
		APIToken: "fallback-token",
		Status:   model.ForwardNodeStatusOnline,
		Enabled:  true,
	}
	tunnel := &model.ForwardTunnel{
		Name:     "Fallback Tunnel",
		InNodeID: node.ID,
		InIP:     "10.10.20.10",
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(node).Error)

	var persistedUser model.User
	assert.NoError(s.T(), db.Where("email = ?", user.Email).First(&persistedUser).Error)
	var persistedNode model.ForwardNode
	assert.NoError(s.T(), db.Where("name = ?", node.Name).First(&persistedNode).Error)

	tunnel.InNodeID = persistedNode.ID
	assert.NoError(s.T(), db.Create(tunnel).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   persistedUser.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	item, err := s.svc.CreateForward(persistedUser.ID, false, PanelForwardInput{
		Name:       "Gost Fallback Forward",
		TunnelID:   tunnel.ID,
		RemoteAddr: "example.com:443",
	})
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), item)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, item.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, item.RuntimeStatus)
	assert.Contains(s.T(), item.RuntimeMessage, forwardRuntimeNodeXBaseURLConfigKey)
	assert.Equal(s.T(), 0, s.runtimeClient.calls)

	var record model.Forward
	assert.NoError(s.T(), db.First(&record, item.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, record.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, record.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusError, record.Status)

	var jobCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Count(&jobCount).Error)
	assert.Zero(s.T(), jobCount)
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

func forwardFlowServiceName(forwardID, userID, userTunnelID uint) string {
	return fmt.Sprintf("%d_%d_%d", forwardID, userID, userTunnelID)
}
