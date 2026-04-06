package service

import (
	"context"
	"fmt"
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
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_speed_limit")
	db.Exec("DELETE FROM v2_forward_tunnel")
	db.Exec("DELETE FROM v2_forward_node")
	s.svc = NewPanelForwardRuntimeService(db)
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

func (s *PanelForwardRuntimeServiceTestSuite) TestApply_PersistsPendingJobForAsyncNodeXResponse() {
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
		UserID:     102,
		UserName:   "runtime-async@example.com",
		Name:       "Async Forward",
		TunnelID:   tunnel.ID,
		InPort:     21001,
		RemoteAddr: "async.example.com:443",
		Status:     model.ForwardStatusActive,
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
			ExtraVars: map[string]interface{}{
				"manage_with": "iptables",
			},
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime",
	))

	client := &stubForwardRuntimeNodeXClient{
		executeFn: func(_ context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
			assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, req.Backend)
			if assert.NotNil(s.T(), req.AnsibleRuntime) {
				assert.Equal(s.T(), "/etc/ansible/hosts", req.AnsibleRuntime.Inventory)
				assert.Equal(s.T(), "/opt/ansible/apply.yml", req.AnsibleRuntime.Playbook)
				assert.Equal(s.T(), forward.ID, req.AnsibleRuntime.Forward.ID)
				assert.Equal(s.T(), node.ID, req.AnsibleRuntime.Node.ID)
			}
			return &nodeXForwardExecuteResult{
				Backend: model.ForwardRuntimeBackendIptablesAnsible,
				Status:  model.ForwardRuntimeJobStatusPending,
				Message: "accepted",
				Async:   true,
			}, nil
		},
	}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, result.Status)
	assert.Equal(s.T(), "accepted", result.Message)
	assert.True(s.T(), result.Async)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, job.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, job.Status)
	assert.NotNil(s.T(), job.StartedAt)
	assert.Nil(s.T(), job.CompletedAt)
	assert.Equal(s.T(), "", job.Error)
	assert.Contains(s.T(), job.Payload, "\"ansibleRuntime\"")
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
		UserID:        103,
		UserName:      "runtime-limiter@example.com",
		Name:          "Limiter Forward",
		TunnelID:      tunnel.ID,
		InPort:        22001,
		RemoteAddr:    "limiter.example.com:443",
		InterfaceName: "eth0",
		Status:        model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   forward.UserID,
		TunnelID: tunnel.ID,
		SpeedID:  &speedLimit.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	configSvc := NewSystemConfigService(db)
	assert.NoError(s.T(), configSvc.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      "/etc/ansible/hosts",
			ApplyPlaybook:  "/opt/ansible/apply.yml",
			RemovePlaybook: "/opt/ansible/remove.yml",
		},
		forwardRuntimeConfigGroup,
		"test ansible runtime",
	))

	client := &stubForwardRuntimeNodeXClient{
		executeFn: func(_ context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
			if assert.NotNil(s.T(), req.PanelForward) && assert.NotNil(s.T(), req.PanelForward.Limiter) {
				assert.Equal(s.T(), speedLimit.ID, req.PanelForward.Limiter.SpeedID)
				assert.Equal(s.T(), speedLimit.Name, req.PanelForward.Limiter.Name)
				assert.Equal(s.T(), speedLimit.Speed, req.PanelForward.Limiter.Speed)
			}
			if assert.NotNil(s.T(), req.AnsibleRuntime) && assert.NotNil(s.T(), req.AnsibleRuntime.Limiter) {
				assert.Equal(s.T(), speedLimit.ID, req.AnsibleRuntime.Limiter.SpeedID)
				assert.Equal(s.T(), speedLimit.Speed, req.AnsibleRuntime.Limiter.Speed)
			}
			return &nodeXForwardExecuteResult{
				Backend: model.ForwardRuntimeBackendIptablesAnsible,
				Status:  model.ForwardRuntimeJobStatusSuccess,
				Message: "limiter applied",
			}, nil
		},
	}
	s.svc.client = client

	result, err := s.svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, result.Status)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	assert.Contains(s.T(), job.Payload, "\"limiter\"")
	assert.Contains(s.T(), job.Payload, "\"speedId\":"+fmt.Sprintf("%d", speedLimit.ID))
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
}

func TestPanelForwardRuntimeService(t *testing.T) {
	suite.Run(t, new(PanelForwardRuntimeServiceTestSuite))
}
