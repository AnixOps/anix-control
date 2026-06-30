package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type stubForwardRuntimeNodeXClient struct {
	calls     []nodeXForwardExecuteRequest
	executeFn func(context.Context, nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error)
}

func (c *stubForwardRuntimeNodeXClient) Execute(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	c.calls = append(c.calls, req)
	if c.executeFn != nil {
		return c.executeFn(ctx, req)
	}
	return &nodeXForwardExecuteResult{
		Status:  model.ForwardRuntimeJobStatusSuccess,
		Message: "ok",
	}, nil
}

type PanelForwardRuntimeServiceTestSuite struct {
	ServiceTestSuite
	svc *PanelForwardRuntimeService
}

func (s *PanelForwardRuntimeServiceTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
		&model.SpeedLimit{},
	)
}

func (s *PanelForwardRuntimeServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	pathExists = func(raw string) bool { return true }
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_speed_limit")
	db.Exec("DELETE FROM v2_forward_tunnel")
	db.Exec("DELETE FROM v2_forward_node")
	s.svc = NewPanelForwardRuntimeService(db)
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
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXModeConfigKey,
		"",
		"bool",
		forwardRuntimeConfigGroup,
		"reset NodeX mode",
	))
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_DispatchesPanelForwardRequestAndPersistsSuccessJob() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)

	node := &model.ForwardNode{
		Name:     "Ingress Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "198.51.100.10",
		Port:     22,
		APIPort:  18080,
		APIToken: "node-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Forward Tunnel",
		InNodeID:      node.ID,
		Protocol:      "both",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "127.0.0.1",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:        100,
		UserName:      "runtime-user@example.com",
		Name:          "Panel Forward",
		TunnelID:      tunnel.ID,
		InPort:        19090,
		RemoteAddr:    "example.com:443",
		InterfaceName: "eth0",
		Strategy:      "round",
		Status:        model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	client := &stubForwardRuntimeNodeXClient{
		executeFn: func(_ context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
			assert.Equal(s.T(), nodeXForwardResourceTypePanelForward, req.ResourceType)
			assert.Equal(s.T(), model.ForwardRuntimeBackendGost, req.Backend)
			assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, req.Action)
			if assert.NotNil(s.T(), req.PanelForward) {
				assert.Equal(s.T(), forward.ID, req.PanelForward.Forward.ID)
				assert.Equal(s.T(), forward.InPort, req.PanelForward.Forward.InPort)
				assert.Equal(s.T(), forward.RemoteAddr, req.PanelForward.Forward.RemoteAddr)
				assert.Equal(s.T(), tunnel.ID, req.PanelForward.Tunnel.ID)
				assert.Equal(s.T(), "both", req.PanelForward.Tunnel.Protocol)
				assert.Equal(s.T(), node.ID, req.PanelForward.IngressNode.ID)
				assert.Equal(s.T(), node.APIPort, req.PanelForward.IngressNode.APIPort)
				assert.Equal(s.T(), node.APIToken, req.PanelForward.IngressNode.APIToken)
			}
			return &nodeXForwardExecuteResult{
				Backend: model.ForwardRuntimeBackendGost,
				Status:  model.ForwardRuntimeJobStatusSuccess,
				Message: "nodex synchronized",
				Result:  "applied",
			}, nil
		},
	}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Len(s.T(), client.calls, 1)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, result.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, result.Status)
	assert.Equal(s.T(), "nodex synchronized", result.Message)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Order("id ASC").Find(&jobs).Error)
	if assert.Len(s.T(), jobs, 1) {
		assert.Equal(s.T(), model.ForwardRuntimeBackendGost, jobs[0].Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, jobs[0].Action)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, jobs[0].Status)
		if assert.NotNil(s.T(), jobs[0].NodeID) {
			assert.Equal(s.T(), node.ID, *jobs[0].NodeID)
		}
		assert.NotNil(s.T(), jobs[0].StartedAt)
		assert.NotNil(s.T(), jobs[0].CompletedAt)
		assert.Equal(s.T(), "applied", jobs[0].Result)
		assert.Contains(s.T(), jobs[0].Payload, `"resourceType":"panel_forward"`)
		assert.Contains(s.T(), jobs[0].Payload, `"backend":"gost"`)
	}
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_GostDeleteWithoutIngressNodeSkipsRemoteExecutionAndPersistsAuditJob() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)

	tunnel := &model.ForwardTunnel{
		Name:   "Missing Ingress Tunnel",
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     101,
		UserName:   "runtime-delete@example.com",
		Name:       "Delete Forward",
		TunnelID:   tunnel.ID,
		InPort:     20001,
		RemoteAddr: "example.net:8443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionDelete, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, result.Status)
	assert.True(s.T(), strings.Contains(result.Message, "skipped because ingress node is not configured"))
	assert.Empty(s.T(), client.calls)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Order("id ASC").Find(&jobs).Error)
	if assert.Len(s.T(), jobs, 1) {
		assert.Equal(s.T(), model.ForwardRuntimeBackendGost, jobs[0].Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionDelete, jobs[0].Action)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, jobs[0].Status)
		assert.Nil(s.T(), jobs[0].NodeID)
		assert.NotNil(s.T(), jobs[0].StartedAt)
		assert.NotNil(s.T(), jobs[0].CompletedAt)
		assert.Empty(s.T(), jobs[0].Error)
		assert.Contains(s.T(), jobs[0].Result, "skipped because ingress node is not configured")
		assert.Contains(s.T(), jobs[0].Payload, `"resourceType":"panel_forward"`)
		assert.Contains(s.T(), jobs[0].Payload, `"backend":"gost"`)
	}
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_GostWithoutNodeXBaseURLFails() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)
	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"",
		"string",
		forwardRuntimeConfigGroup,
		"clear test NodeX runtime URL",
	))

	node := &model.ForwardNode{
		Name:     "Fallback Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.50",
		Port:     22,
		APIPort:  19080,
		APIToken: "fallback-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Fallback Tunnel",
		InNodeID:      node.ID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     111,
		UserName:   "runtime-fallback@example.com",
		Name:       "Fallback Forward",
		TunnelID:   tunnel.ID,
		InPort:     21001,
		RemoteAddr: "fallback.example.com:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.Error(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, result.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, result.Backend)
	assert.Empty(s.T(), client.calls)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXBaseURLConfigKey)

	var jobCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Count(&jobCount).Error)
	assert.Zero(s.T(), jobCount, "NodeX execution should not enqueue jobs when base URL is missing")
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_GostWithoutNodeXTokenFails() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendGost)
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
		"",
		"string",
		forwardRuntimeConfigGroup,
		"clear test NodeX runtime token",
	))

	node := &model.ForwardNode{
		Name:     "Fallback Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.50",
		Port:     22,
		APIPort:  19080,
		APIToken: "fallback-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Fallback Tunnel",
		InNodeID:      node.ID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:     111,
		UserName:   "runtime-fallback@example.com",
		Name:       "Fallback Forward",
		TunnelID:   tunnel.ID,
		InPort:     21001,
		RemoteAddr: "fallback.example.com:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.Error(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, result.Status)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, result.Backend)
	assert.Empty(s.T(), client.calls)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXTokenConfigKey)

	var jobCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Count(&jobCount).Error)
	assert.Zero(s.T(), jobCount, "NodeX execution should not enqueue jobs when token is missing")
}

func (s *PanelForwardRuntimeServiceTestSuite) TestResolveBackend_NodeXModeOverridesLegacyBackend() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXModeConfigKey,
		"false",
		"bool",
		forwardRuntimeConfigGroup,
		"disable NodeX mode",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendGost,
		"string",
		forwardRuntimeConfigGroup,
		"legacy backend",
	))

	backend, err := s.svc.resolveBackend()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, backend)

	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeNodeXModeConfigKey,
		"true",
		"bool",
		forwardRuntimeConfigGroup,
		"enable NodeX mode",
	))
	assert.NoError(s.T(), configSvc.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendIptablesAnsible,
		"string",
		forwardRuntimeConfigGroup,
		"legacy backend",
	))

	backend, err = s.svc.resolveBackend()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_UsesStoredForwardRuntimeBackendBeforeGlobalConfig() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)

	node := &model.ForwardNode{
		Name:     "Stored Backend Ingress",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "198.51.100.44",
		Port:     22,
		APIPort:  18081,
		APIToken: "stored-backend-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:     "Stored Backend Tunnel",
		InNodeID: node.ID,
		InIP:     node.Host,
		Type:     1,
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:         201,
		UserName:       "stored-backend@example.com",
		Name:           "Stored Backend Forward",
		TunnelID:       tunnel.ID,
		InPort:         10443,
		RemoteAddr:     "example.com:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendGost,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	client := &stubForwardRuntimeNodeXClient{
		executeFn: func(_ context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
			assert.Equal(s.T(), model.ForwardRuntimeBackendGost, req.Backend)
			return &nodeXForwardExecuteResult{
				Backend: model.ForwardRuntimeBackendGost,
				Status:  model.ForwardRuntimeJobStatusSuccess,
				Message: "stored backend synchronized",
				Result:  "stored backend applied",
			}, nil
		},
	}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionUpdate, forward, tunnel)
	assert.NoError(s.T(), err)
	if assert.NotNil(s.T(), result) {
		assert.Equal(s.T(), model.ForwardRuntimeBackendGost, result.Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, result.Status)
	}
	assert.Len(s.T(), client.calls, 1)

	var jobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Order("id ASC").Find(&jobs).Error)
	if assert.Len(s.T(), jobs, 1) {
		assert.Equal(s.T(), model.ForwardRuntimeBackendGost, jobs[0].Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionUpdate, jobs[0].Action)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, jobs[0].Status)
		assert.NotNil(s.T(), jobs[0].NodeID)
		assert.Equal(s.T(), node.ID, *jobs[0].NodeID)
		assert.Contains(s.T(), jobs[0].Result, "stored backend applied")
	}
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_IptablesAnsibleQueuesLocalJobWithoutNodeX() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)

	node := &model.ForwardNode{
		Name:     "Async Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.10",
		Port:     22,
		APIPort:  19080,
		APIToken: "async-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Async Tunnel",
		InNodeID:      node.ID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:         102,
		UserName:       "runtime-async@example.com",
		Name:           "Async Forward",
		TunnelID:       tunnel.ID,
		InPort:         21001,
		RemoteAddr:     "async.example.com:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      "/etc/ansible/hosts",
			ApplyPlaybook:  "/opt/ansible/apply.yml",
			RemovePlaybook: "/opt/ansible/remove.yml",
			Become:         true,
			ExtraVars: map[string]any{
				"manage_with": "iptables",
			},
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime",
	))

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Empty(s.T(), client.calls)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, result.Status)
	assert.Equal(s.T(), "ansible runtime queued for local executor", result.Message)
	assert.True(s.T(), result.Async)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	// iptables 已下线: 输入 iptables_ansible 被归一化为 nftables_ansible
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, job.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, job.Status)
	assert.Nil(s.T(), job.StartedAt)
	assert.Nil(s.T(), job.CompletedAt)
	assert.Equal(s.T(), "", job.Error)
	assert.NotContains(s.T(), job.Payload, "\"ansibleRuntime\"")
	assert.Contains(s.T(), job.Payload, "\"inventory\":\"/etc/ansible/hosts\"")
	assert.Contains(s.T(), job.Payload, "\"playbook\":\"/opt/ansible/apply.yml\"")

	var payload panelForwardAnsibleRuntimePayload
	assert.NoError(s.T(), json.Unmarshal([]byte(job.Payload), &payload))
	assert.Equal(s.T(), "/etc/ansible/hosts", payload.Inventory)
	assert.Equal(s.T(), "/opt/ansible/apply.yml", payload.Playbook)
	assert.Equal(s.T(), forward.ID, payload.Forward.ID)
	assert.Equal(s.T(), node.ID, payload.Node.ID)
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_AttachesLimiterToRuntimeRequest() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)

	node := &model.ForwardNode{
		Name:     "Limiter Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.15",
		Port:     22,
		APIPort:  19500,
		APIToken: "limiter-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:          "Limiter Tunnel",
		InNodeID:      node.ID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	speedLimit := &model.SpeedLimit{
		Name:        "25M",
		Speed:       200,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	assert.NoError(s.T(), db.Create(speedLimit).Error)

	forward := &model.Forward{
		UserID:         103,
		UserName:       "runtime-limiter@example.com",
		Name:           "Limiter Forward",
		TunnelID:       tunnel.ID,
		InPort:         22001,
		RemoteAddr:     "limiter.example.com:443",
		InterfaceName:  "eth0",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   forward.UserID,
		TunnelID: tunnel.ID,
		SpeedID:  &speedLimit.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	configSvc := NewSystemConfigService(db)
	tempDir := s.T().TempDir()
	inventoryPath := filepath.Join(tempDir, "inventory.ini")
	applyPlaybookPath := filepath.Join(tempDir, "apply.yml")
	removePlaybookPath := filepath.Join(tempDir, "remove.yml")
	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[forward_nodes]\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(applyPlaybookPath, []byte("---\n- hosts: all\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(removePlaybookPath, []byte("---\n- hosts: all\n"), 0o600))
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      inventoryPath,
			ApplyPlaybook:  applyPlaybookPath,
			RemovePlaybook: removePlaybookPath,
			WorkingDir:     tempDir,
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime",
	))

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Empty(s.T(), client.calls)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, result.Status)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	assert.Contains(s.T(), job.Payload, "\"limiter\"")
	assert.Contains(s.T(), job.Payload, "\"speedId\":"+fmt.Sprintf("%d", speedLimit.ID))

	var payload panelForwardAnsibleRuntimePayload
	assert.NoError(s.T(), json.Unmarshal([]byte(job.Payload), &payload))
	if assert.NotNil(s.T(), payload.Limiter) {
		assert.Equal(s.T(), speedLimit.ID, payload.Limiter.SpeedID)
		assert.Equal(s.T(), speedLimit.Name, payload.Limiter.Name)
		assert.Equal(s.T(), speedLimit.Speed, payload.Limiter.Speed)
	}
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_IptablesAnsibleQueuesLocalJobWithExecutionNodeFromOutNode() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)

	outNode := &model.ForwardNode{
		Name:     "Out Node",
		Type:     model.ForwardNodeTypeExit,
		Host:     "198.51.100.20",
		Port:     22,
		APIPort:  19180,
		APIToken: "out-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(outNode).Error)
	outNodeID := outNode.ID

	tunnel := &model.ForwardTunnel{
		Name:          "Out Execution Tunnel",
		Type:          1,
		OutNodeID:     &outNodeID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:         104,
		UserName:       "runtime-out@example.com",
		Name:           "Execution Forward",
		TunnelID:       tunnel.ID,
		InPort:         23001,
		RemoteAddr:     "execution.example.com:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      "/var/ansible/hosts",
			ApplyPlaybook:  "/tmp/ansible/apply.yml",
			RemovePlaybook: "/tmp/ansible/remove.yml",
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime with out node",
	))

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, result.Status)
	assert.Empty(s.T(), client.calls)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	// iptables 已下线: 归一化为 nftables_ansible
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, job.Backend)
	if assert.NotNil(s.T(), job.NodeID) {
		assert.Equal(s.T(), outNode.ID, *job.NodeID)
	}

	var payload panelForwardAnsibleRuntimePayload
	assert.NoError(s.T(), json.Unmarshal([]byte(job.Payload), &payload))
	assert.Equal(s.T(), outNode.ID, payload.Node.ID)
	assert.Equal(s.T(), "/var/ansible/hosts", payload.Inventory)
	assert.Equal(s.T(), "/tmp/ansible/apply.yml", payload.Playbook)
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_IptablesAnsibleRejectsTunnelTypeNotSupported() {
	db := database.Get()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendIptablesAnsible)

	node := &model.ForwardNode{
		Name:     "Unsupported Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.5",
		Port:     22,
		APIPort:  19200,
		APIToken: "unsupported-token",
		Enabled:  true,
	}
	assert.NoError(s.T(), db.Create(node).Error)
	nodeID := node.ID

	tunnel := &model.ForwardTunnel{
		Name:          "Unsupported Tunnel",
		Type:          2,
		InNodeID:      nodeID,
		OutNodeID:     &nodeID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		UserID:         105,
		UserName:       "runtime-unsupported@example.com",
		Name:           "Unsupported Forward",
		TunnelID:       tunnel.ID,
		InPort:         24001,
		RemoteAddr:     "unsupported.example.com:443",
		Status:         model.ForwardStatusActive,
		InterfaceName:  "eth1",
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      "/var/ansible/hosts",
			ApplyPlaybook:  "/tmp/ansible/apply.yml",
			RemovePlaybook: "/tmp/ansible/remove.yml",
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime type guard",
	))

	client := &stubForwardRuntimeNodeXClient{}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.Error(s.T(), err)
	if assert.NotNil(s.T(), result) {
		// iptables 已下线: 归一化为 nftables_ansible
		assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, result.Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, result.Status)
	}
	assert.Contains(s.T(), err.Error(), "type 1")
	assert.Empty(s.T(), client.calls)

	var jobCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Count(&jobCount).Error)
	assert.Zero(s.T(), jobCount)
}

func setForwardRuntimeBackendForTest(t *testing.T, db *gorm.DB, backend string) {
	t.Helper()
	configSvc := NewSystemConfigService(db)
	assert.NoError(t, configSvc.Set(
		forwardRuntimeBackendConfigKey,
		backend,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	))
	localBackend := ""
	if isForwardRuntimeLocalAnsibleBackend(backend) {
		localBackend = backend
	}
	assert.NoError(t, configSvc.Set(
		forwardRuntimeLocalBackendConfigKey,
		localBackend,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime local backend",
	))
}

func (s *PanelForwardRuntimeServiceTestSuite) TestForwardRuntimeExecutor_RegistryAndNodeRoles() {
	// 注册表覆盖全部 backend, 且 iptables 兜底指向 nftables executor
	gostExec, ok := s.svc.forwardRuntimeExecutor(model.ForwardRuntimeBackendGost)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), forwardNodeRoleIngress, gostExec.nodeRole())

	nftExec, ok := s.svc.forwardRuntimeExecutor(model.ForwardRuntimeBackendNftablesAnsible)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), forwardNodeRoleExecution, nftExec.nodeRole())

	iptExec, ok := s.svc.forwardRuntimeExecutor(model.ForwardRuntimeBackendIptablesAnsible)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, iptExec.backend()) // 兜底

	agentExec, ok := s.svc.forwardRuntimeExecutor(model.ForwardRuntimeBackendCleanAgent)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), forwardNodeRoleExecution, agentExec.nodeRole())

	// 未知 backend
	_, ok = s.svc.forwardRuntimeExecutor("does-not-exist")
	assert.False(s.T(), ok)
}

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_UnsupportedBackendReturnsError() {
	// 强制注入空注册表, 模拟未知 backend 不 panic、返回友好错误
	svc := &PanelForwardRuntimeService{
		db:            database.Get(),
		configService: NewSystemConfigService(database.Get()),
		client:        &stubForwardRuntimeNodeXClient{},
		executors:     map[string]forwardRuntimeExecutor{}, // 非 nil 空表, 不触发惰性初始化
	}
	forward := &model.Forward{
		UserID:         1,
		Name:           "bad-backend",
		TunnelID:       1,
		InPort:         40001,
		RemoteAddr:     "10.0.0.1:80",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendGost,
	}
	tunnel := &model.ForwardTunnel{Name: "t", Type: 1, Protocol: "tcp", InNodeID: 1, Status: 1}

	result, err := svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "unsupported forward runtime backend")
	if assert.NotNil(s.T(), result) {
		assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, result.Status)
	}
}

func TestPanelForwardRuntimeService(t *testing.T) {
	suite.Run(t, new(PanelForwardRuntimeServiceTestSuite))
}

func (s *PanelForwardRuntimeServiceTestSuite) TestValidateAnsiblePaths_RejectsMissingInventory() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	svc := &PanelForwardRuntimeService{db: db, configService: configSvc}
	pathExists = func(raw string) bool {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return false
		}
		_, err := os.Stat(trimmed)
		return err == nil
	}

	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXModeConfigKey, "false", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeLocalBackendConfigKey, model.ForwardRuntimeBackendNftablesAnsible, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleInventoryConfigKey, "/nonexistent/inventory.ini", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleApplyPlaybookConfigKey, "/opt/ansible/apply.yml", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleRemovePlaybookConfigKey, "/opt/ansible/remove.yml", "string", forwardRuntimeConfigGroup, "test"))

	err := svc.validateAnsiblePaths(model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeJobActionCreate)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "inventory file not found")
}

func (s *PanelForwardRuntimeServiceTestSuite) TestValidateAnsiblePaths_RejectsMissingPlaybook() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	svc := &PanelForwardRuntimeService{db: db, configService: configSvc}
	pathExists = func(raw string) bool {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return false
		}
		_, err := os.Stat(trimmed)
		return err == nil
	}

	tempDir := s.T().TempDir()
	inventoryPath := tempDir + "/inventory.ini"
	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[nodes]\n"), 0o600))

	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXModeConfigKey, "false", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeLocalBackendConfigKey, model.ForwardRuntimeBackendNftablesAnsible, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleInventoryConfigKey, inventoryPath, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleApplyPlaybookConfigKey, "/nonexistent/apply.yml", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleRemovePlaybookConfigKey, "/nonexistent/remove.yml", "string", forwardRuntimeConfigGroup, "test"))

	err := svc.validateAnsiblePaths(model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeJobActionCreate)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "playbook file not found")
}

func (s *PanelForwardRuntimeServiceTestSuite) TestValidateAnsiblePaths_PassesWhenFilesExist() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	svc := &PanelForwardRuntimeService{db: db, configService: configSvc}

	tempDir := s.T().TempDir()
	inventoryPath := tempDir + "/inventory.ini"
	applyPlaybookPath := tempDir + "/apply.yml"
	removePlaybookPath := tempDir + "/remove.yml"
	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[nodes]\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(applyPlaybookPath, []byte("---\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(removePlaybookPath, []byte("---\n"), 0o600))

	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXModeConfigKey, "false", "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeLocalBackendConfigKey, model.ForwardRuntimeBackendNftablesAnsible, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleInventoryConfigKey, inventoryPath, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleApplyPlaybookConfigKey, applyPlaybookPath, "string", forwardRuntimeConfigGroup, "test"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeAnsibleRemovePlaybookConfigKey, removePlaybookPath, "string", forwardRuntimeConfigGroup, "test"))

	err := svc.validateAnsiblePaths(model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeJobActionCreate)
	assert.NoError(s.T(), err)

	err = svc.validateAnsiblePaths(model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeJobActionDelete)
	assert.NoError(s.T(), err)
}

func (s *PanelForwardRuntimeServiceTestSuite) TestSyncForwardsToBackend_SyncsMismatchedForwards() {
	db := database.Get()
	configSvc := NewSystemConfigService(db)
	stubClient := &stubForwardRuntimeNodeXClient{
		executeFn: func(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
			return &nodeXForwardExecuteResult{
				Status:  model.ForwardRuntimeJobStatusSuccess,
				Message: "synced",
			}, nil
		},
	}
	svc := &PanelForwardRuntimeService{db: db, configService: configSvc, client: stubClient}

	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXModeConfigKey, "true", "string", forwardRuntimeConfigGroup, "test"))

	// Create a forward on a different backend
	forward := &model.Forward{
		UserID:         1,
		UserName:       "sync-user",
		Name:           "Sync Forward",
		TunnelID:       1,
		InPort:         20001,
		RemoteAddr:     "10.0.0.1:80",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	// Create a tunnel for the forward
	tunnel := &model.ForwardTunnel{
		Name:      "Sync Tunnel",
		Type:      1,
		Protocol:  "tcp",
		InNodeID:  1,
		Status:    1,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)
	forward.TunnelID = tunnel.ID
	db.Save(forward)

	// Create a forward node for the tunnel
	node := &model.ForwardNode{
		Name:     "sync-node",
		Host:     "10.0.0.1",
		Port:     80,
		APIPort:  9000,
		APIToken: "token",
	}
	assert.NoError(s.T(), db.Create(node).Error)
	tunnel.InNodeID = node.ID
	db.Save(tunnel)

	synced, failed, err := svc.SyncForwardsToBackend(model.ForwardRuntimeBackendGost)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, synced)
	assert.Equal(s.T(), 0, failed)

	var updated model.Forward
	assert.NoError(s.T(), db.First(&updated, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, updated.RuntimeBackend)
}
