package service

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
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

func (c *stubNodeXForwardRuntimeClient) Translate(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error) {
	_ = ctx
	return &nodeXBridgeAgentTask{
		TaskID: fmt.Sprintf("forward-runtime-job-%d", sourceJobID),
		NodeID: nodeID,
		Type:   "forward",
		Action: payload.Action,
	}, nil
}

func (s *PanelForwardServiceTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	s.Require().NoError(database.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{},
		&model.SpeedLimit{},
	))
}

func (s *PanelForwardServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	pathExists = func(raw string) bool { return true }
	// A user's forward targets are resolved (validateUserForwardTargets):
	// the tests' names resolve to a public documentation address without
	// asking DNS, unless a test answers otherwise.
	previousLookup := probeLookup
	probeLookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.80")}, nil
	}
	s.T().Cleanup(func() { probeLookup = previousLookup })
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_port_binding")
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
	node := s.createForwardNode("Admin Tunnel Relay", "10.0.0.9", model.ForwardNodeStatusOnline)

	activeA := &model.ForwardTunnel{Name: "Tunnel A", InNodeID: node.ID, InIP: "1.1.1.1", Status: model.ForwardTunnelStatusActive}
	activeB := &model.ForwardTunnel{Name: "Tunnel B", InNodeID: node.ID, InIP: "2.2.2.2", Status: model.ForwardTunnelStatusActive}
	disabled := &model.ForwardTunnel{Name: "Tunnel C", InNodeID: node.ID, InIP: "3.3.3.3", Status: model.ForwardTunnelStatusActive}
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
	node := s.createForwardNode("Authorized Active Relay", "10.0.0.10", model.ForwardNodeStatusOnline)

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

	authorizedActive := &model.ForwardTunnel{Name: "Authorized Active", InNodeID: node.ID, InIP: "10.0.0.1", Type: 1, Status: model.ForwardTunnelStatusActive}
	authorizedDisabled := &model.ForwardTunnel{Name: "Authorized Disabled", InNodeID: node.ID, InIP: "10.0.0.2", Type: 1, Status: model.ForwardTunnelStatusActive}
	otherUsersTunnel := &model.ForwardTunnel{Name: "Other User Tunnel", InNodeID: node.ID, InIP: "10.0.0.3", Type: 1, Status: model.ForwardTunnelStatusActive}
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
	node := s.createForwardNode("Disabled Permission Relay", "10.20.30.10", model.ForwardNodeStatusOnline)

	user := &model.User{
		Email:          "panel-forward-disabled-permission@example.com",
		Password:       "hash",
		Token:          "panel-forward-disabled-permission-token",
		UUID:           "panel-forward-disabled-permission-uuid",
		TransferEnable: 1073741824,
	}
	tunnel := &model.ForwardTunnel{
		Name:     "Disabled Permission Tunnel",
		InNodeID: node.ID,
		InIP:     "10.20.30.40",
		Type:     1,
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

func (s *PanelForwardServiceTestSuite) TestListTunnels_FiltersIncompatibleWithCurrentRuntimeBackend() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)

	relayNode := s.createForwardNode("Filter Relay", "10.20.0.10", model.ForwardNodeStatusOnline)
	exitNode := s.createForwardNodeOfType("Filter Exit", "10.20.0.11", model.ForwardNodeStatusOnline, model.ForwardNodeTypeExit)
	relayNodeID := relayNode.ID
	exitNodeID := exitNode.ID

	gostPortTunnel := &model.ForwardTunnel{
		Name:     "Gost Port Tunnel",
		InNodeID: relayNode.ID,
		InIP:     relayNode.Host,
		Type:     1,
		Protocol: "tcp",
		Status:   model.ForwardTunnelStatusActive,
	}
	gostNodeXTunnel := &model.ForwardTunnel{
		Name:      "Gost NodeX Tunnel",
		InNodeID:  relayNode.ID,
		OutNodeID: &exitNodeID,
		InIP:      relayNode.Host,
		OutIP:     exitNode.Host,
		Type:      2,
		Protocol:  "tcp",
		Status:    model.ForwardTunnelStatusActive,
	}
	ansibleExecOnlyTunnel := &model.ForwardTunnel{
		Name:      "Ansible Exec Tunnel",
		InNodeID:  0,
		OutNodeID: &relayNodeID,
		InIP:      relayNode.Host,
		Type:      1,
		Protocol:  "tcp",
		Status:    model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(gostPortTunnel).Error)
	assert.NoError(s.T(), db.Create(gostNodeXTunnel).Error)
	assert.NoError(s.T(), db.Create(ansibleExecOnlyTunnel).Error)

	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendGost,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	gostItems, err := s.svc.ListTunnels(0, true)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), gostItems, 2)
	assert.Equal(s.T(), "Gost NodeX Tunnel", gostItems[0].Name)
	assert.Equal(s.T(), "Gost Port Tunnel", gostItems[1].Name)

	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendIptablesAnsible,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	ansibleItems, err := s.svc.ListTunnels(0, true)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), ansibleItems, 2)
	assert.Equal(s.T(), "Ansible Exec Tunnel", ansibleItems[0].Name)
	assert.Equal(s.T(), "Gost Port Tunnel", ansibleItems[1].Name)
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
		assert.Contains(s.T(), err.Error(), "ansible 转发模式仅支持端口转发")
	}
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

func (s *PanelForwardServiceTestSuite) TestUploadFluxForwardFlow_RejectsMismatchedServiceTuple() {
	db := database.Get()

	userA := &model.User{
		Email:          "panel-forward-flow-tuple-a@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-tuple-a-token",
		UUID:           "panel-forward-flow-tuple-a-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	userB := &model.User{
		Email:          "panel-forward-flow-tuple-b@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-tuple-b-token",
		UUID:           "panel-forward-flow-tuple-b-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	tunnelA := &model.ForwardTunnel{
		Name:         "Flow Tuple Tunnel A",
		InIP:         "198.51.100.20",
		Flow:         1,
		TrafficRatio: 1,
		Status:       model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:         "Flow Tuple Tunnel B",
		InIP:         "198.51.100.21",
		Flow:         1,
		TrafficRatio: 1,
		Status:       model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(userA).Error)
	assert.NoError(s.T(), db.Create(userB).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	permissionA := &model.ForwardUserTunnel{
		UserID:   userA.ID,
		TunnelID: tunnelA.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	permissionOtherTunnel := &model.ForwardUserTunnel{
		UserID:   userA.ID,
		TunnelID: tunnelB.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(permissionA).Error)
	assert.NoError(s.T(), db.Create(permissionOtherTunnel).Error)

	forward := &model.Forward{
		UserID:     userA.ID,
		UserName:   userA.Email,
		Name:       "Flux Tuple Forward",
		TunnelID:   tunnelA.ID,
		InPort:     15021,
		RemoteAddr: "tuple.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	err := s.svc.UploadFluxForwardFlow(PanelForwardFlowData{
		N: forwardFlowServiceName(forward.ID, userB.ID, permissionA.ID),
		U: 100,
		D: 200,
	})
	assert.ErrorContains(s.T(), err, "forward flow user mismatch")

	err = s.svc.UploadFluxForwardFlow(PanelForwardFlowData{
		N: forwardFlowServiceName(forward.ID, userA.ID, permissionOtherTunnel.ID),
		U: 100,
		D: 200,
	})
	assert.ErrorContains(s.T(), err, "forward flow user tunnel mismatch")

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedForward.InFlow)
	assert.Equal(s.T(), int64(0), reloadedForward.OutFlow)

	var reloadedUser model.User
	assert.NoError(s.T(), db.First(&reloadedUser, userA.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedUser.U)
	assert.Equal(s.T(), int64(0), reloadedUser.D)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permissionA.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedPermission.InFlow)
	assert.Equal(s.T(), int64(0), reloadedPermission.OutFlow)
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
		Name:         "Snapshot Tunnel",
		InIP:         "198.51.100.11",
		Flow:         1,
		TrafficRatio: 1,
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

func (s *PanelForwardServiceTestSuite) TestApplyForwardTrafficSnapshots_AppliesTunnelRatioAndFlowToDeltas() {
	db := database.Get()

	user := &model.User{
		Email:          "panel-forward-flow-snapshot-ratio@example.com",
		Password:       "hash",
		Token:          "panel-forward-flow-snapshot-ratio-token",
		UUID:           "panel-forward-flow-snapshot-ratio-uuid",
		TransferEnable: 20 * bytesPerGiB,
	}
	tunnel := &model.ForwardTunnel{
		Name:         "Snapshot Ratio Tunnel",
		InIP:         "198.51.100.22",
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
		Name:       "Snapshot Ratio Forward",
		TunnelID:   tunnel.ID,
		InPort:     15022,
		RemoteAddr: "snapshot-ratio.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   100,
		DownloadTotal: 200,
	}}))
	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   150,
		DownloadTotal: 260,
	}}))
	assert.NoError(s.T(), s.svc.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendGost,
		UploadTotal:   10,
		DownloadTotal: 20,
	}}))

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), int64(840), reloadedForward.InFlow)
	assert.Equal(s.T(), int64(480), reloadedForward.OutFlow)

	var reloadedUser model.User
	assert.NoError(s.T(), db.First(&reloadedUser, user.ID).Error)
	assert.Equal(s.T(), int64(480), reloadedUser.U)
	assert.Equal(s.T(), int64(840), reloadedUser.D)

	var reloadedPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedPermission, permission.ID).Error)
	assert.Equal(s.T(), int64(840), reloadedPermission.InFlow)
	assert.Equal(s.T(), int64(480), reloadedPermission.OutFlow)

	var cursor model.ForwardTrafficCursor
	assert.NoError(s.T(), db.Where("forward_id = ? AND backend = ?", forward.ID, model.ForwardRuntimeBackendGost).First(&cursor).Error)
	assert.Equal(s.T(), int64(10), cursor.UploadTotal)
	assert.Equal(s.T(), int64(20), cursor.DownloadTotal)
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

func TestPanelForwardService(t *testing.T) {
	suite.Run(t, new(PanelForwardServiceTestSuite))
}

func forwardFlowServiceName(forwardID, userID, userTunnelID uint) string {
	return fmt.Sprintf("%d_%d_%d", forwardID, userID, userTunnelID)
}
