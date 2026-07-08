package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardCleanAgentServiceTestSuite struct {
	ServiceTestSuite
	svc *ForwardCleanAgentService
}

func (s *ForwardCleanAgentServiceTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	s.Require().NoError(database.AutoMigrate(
		&model.User{},
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.ForwardTrafficCursor{},
		&model.ForwardRuntimeJob{},
		&model.ForwardCleanAgent{},
	))
}

func (s *ForwardCleanAgentServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward_clean_agent")
	db.Exec("DELETE FROM v2_forward_port_binding")
	db.Exec("DELETE FROM v2_forward_traffic_cursor")
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_forward_tunnel")
	db.Exec("DELETE FROM v2_forward_node")
	db.Exec("DELETE FROM v2_user")
	s.svc = NewForwardCleanAgentService(db)
}

func (s *ForwardCleanAgentServiceTestSuite) TestRegisterHeartbeatAndReportSuccess() {
	db := database.Get()
	node, tunnel, forward := s.createCleanAgentForwardFixtures()

	tokenResult, err := s.svc.CreateToken(ForwardCleanAgentCreateInput{
		Name:   "relay-agent",
		NodeID: &node.ID,
	})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), tokenResult.Token)
	assert.Equal(s.T(), node.ID, *tokenResult.Agent.NodeID)

	agent, err := s.svc.Register(ForwardCleanAgentRegisterInput{
		Token:        tokenResult.Token,
		NodeID:       &node.ID,
		Name:         "relay-agent-registered",
		Version:      "0.1.0",
		Hostname:     "relay-1",
		OS:           "linux",
		Arch:         "amd64",
		Capabilities: []string{"nftables"},
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardCleanAgentStatusOnline, agent.Status)
	assert.NotNil(s.T(), agent.LastSeen)

	payload := nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       model.ForwardRuntimeJobActionCreate,
		PanelForward: &nodeXPanelForwardRequest{
			Forward: nodeXPanelForwardPayload{
				ID:         forward.ID,
				UserID:     forward.UserID,
				Name:       forward.Name,
				InPort:     forward.InPort,
				RemoteAddr: forward.RemoteAddr,
				Status:     forward.Status,
			},
			Tunnel: nodeXPanelTunnelPayload{
				ID:       tunnel.ID,
				Name:     tunnel.Name,
				InNodeID: node.ID,
				Protocol: "tcp",
			},
			IngressNode: nodeXForwardNodePayload{
				ID:   node.ID,
				Name: node.Name,
				Host: node.Host,
				Port: node.Port,
			},
		},
	}
	payloadData, err := json.Marshal(payload)
	assert.NoError(s.T(), err)

	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       model.ForwardRuntimeJobActionCreate,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       uintPtr(node.ID),
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      string(payloadData),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	actions, err := s.svc.Heartbeat(ForwardCleanAgentHeartbeatInput{
		AgentID: agent.ID,
		Token:   tokenResult.Token,
		Version: "0.1.1",
	})
	assert.NoError(s.T(), err)
	if assert.Len(s.T(), actions, 1) {
		assert.Equal(s.T(), job.ID, actions[0].JobID)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, actions[0].Action)
		assert.JSONEq(s.T(), string(payloadData), string(actions[0].Payload))
	}

	var claimed model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&claimed, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, claimed.Status)
	assert.NotNil(s.T(), claimed.AgentID)
	assert.Equal(s.T(), agent.ID, *claimed.AgentID)
	assert.NotNil(s.T(), claimed.ClaimedAt)

	var runningForward model.Forward
	assert.NoError(s.T(), db.First(&runningForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, runningForward.RuntimeStatus)
	assert.Equal(s.T(), "clean_agent runtime running", runningForward.RuntimeMessage)
	assert.NotNil(s.T(), runningForward.RuntimeLastSyncAt)

	success := true
	err = s.svc.Report(ForwardCleanAgentReportInput{
		AgentID:  agent.ID,
		Token:    tokenResult.Token,
		JobID:    job.ID,
		Success:  &success,
		Result:   "applied",
		Upload:   123,
		Download: 456,
	})
	assert.NoError(s.T(), err)

	var completed model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&completed, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, completed.Status)
	assert.Equal(s.T(), "applied", completed.Result)
	assert.Empty(s.T(), completed.Error)
	assert.NotNil(s.T(), completed.CompletedAt)

	var updatedForward model.Forward
	assert.NoError(s.T(), db.First(&updatedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendCleanAgent, updatedForward.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, updatedForward.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusActive, updatedForward.Status)
	assert.Equal(s.T(), int64(456), updatedForward.InFlow)
	assert.Equal(s.T(), int64(123), updatedForward.OutFlow)

	var updatedUser model.User
	assert.NoError(s.T(), db.First(&updatedUser, forward.UserID).Error)
	assert.Equal(s.T(), int64(123), updatedUser.U)
	assert.Equal(s.T(), int64(456), updatedUser.D)
}

func (s *ForwardCleanAgentServiceTestSuite) TestReportDeleteSuccessRemovesForwardRecords() {
	db := database.Get()
	node, tunnel, forward := s.createCleanAgentForwardFixtures()

	tokenResult, err := s.svc.CreateToken(ForwardCleanAgentCreateInput{
		Name:   "delete-agent",
		NodeID: &node.ID,
	})
	assert.NoError(s.T(), err)
	agent, err := s.svc.Register(ForwardCleanAgentRegisterInput{
		Token: tokenResult.Token,
		Name:  "delete-agent",
	})
	assert.NoError(s.T(), err)

	assert.NoError(s.T(), db.Create(&model.ForwardPortBinding{
		ForwardID:  forward.ID,
		NodeID:     node.ID,
		Transport:  "tcp",
		ListenAddr: "0.0.0.0",
		InPort:     forward.InPort,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendCleanAgent,
		UploadTotal:   10,
		DownloadTotal: 20,
	}).Error)

	payload := nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       model.ForwardRuntimeJobActionDelete,
	}
	payloadData, err := json.Marshal(payload)
	assert.NoError(s.T(), err)
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       model.ForwardRuntimeJobActionDelete,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       uintPtr(node.ID),
		AgentID:      uintPtr(agent.ID),
		Status:       model.ForwardRuntimeJobStatusRunning,
		Payload:      string(payloadData),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	success := true
	err = s.svc.Report(ForwardCleanAgentReportInput{
		AgentID: agent.ID,
		Token:   tokenResult.Token,
		JobID:   job.ID,
		Success: &success,
		Result:  "removed",
	})
	assert.NoError(s.T(), err)

	var completed model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&completed, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, completed.Status)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardPortBinding{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardTrafficCursor{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *ForwardCleanAgentServiceTestSuite) TestRevokedAgentCannotHeartbeat() {
	node, _, _ := s.createCleanAgentForwardFixtures()

	tokenResult, err := s.svc.CreateToken(ForwardCleanAgentCreateInput{
		Name:   "revoked-agent",
		NodeID: &node.ID,
	})
	assert.NoError(s.T(), err)

	agent, err := s.svc.Register(ForwardCleanAgentRegisterInput{
		Token:  tokenResult.Token,
		NodeID: &node.ID,
	})
	assert.NoError(s.T(), err)

	assert.NoError(s.T(), s.svc.RevokeAgent(agent.ID))
	_, err = s.svc.Heartbeat(ForwardCleanAgentHeartbeatInput{
		AgentID: agent.ID,
		Token:   tokenResult.Token,
	})
	assert.ErrorIs(s.T(), err, ErrForwardCleanAgentRevoked)
}

func (s *ForwardCleanAgentServiceTestSuite) TestRuntimeApplyQueuesCleanAgentJob() {
	db := database.Get()
	node, tunnel, forward := s.createCleanAgentForwardFixtures()
	setForwardRuntimeBackendForTest(s.T(), db, model.ForwardRuntimeBackendCleanAgent)

	runtimeSvc := NewPanelForwardRuntimeService(db)
	result, err := runtimeSvc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), result)
	assert.Equal(s.T(), model.ForwardRuntimeBackendCleanAgent, result.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, result.Status)
	assert.True(s.T(), result.Async)

	var job model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Last(&job).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendCleanAgent, job.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, job.Status)
	assert.Nil(s.T(), job.StartedAt)
	assert.Nil(s.T(), job.CompletedAt)
	if assert.NotNil(s.T(), job.NodeID) {
		assert.Equal(s.T(), node.ID, *job.NodeID)
	}

	var payload nodeXForwardExecuteRequest
	assert.NoError(s.T(), json.Unmarshal([]byte(job.Payload), &payload))
	assert.Equal(s.T(), model.ForwardRuntimeBackendCleanAgent, payload.Backend)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, payload.Action)
	if assert.NotNil(s.T(), payload.PanelForward) {
		assert.Equal(s.T(), forward.ID, payload.PanelForward.Forward.ID)
		assert.Equal(s.T(), tunnel.ID, payload.PanelForward.Tunnel.ID)
		assert.Equal(s.T(), node.ID, payload.PanelForward.IngressNode.ID)
	}
}

func (s *ForwardCleanAgentServiceTestSuite) createCleanAgentForwardFixtures() (*model.ForwardNode, *model.ForwardTunnel, *model.Forward) {
	db := database.Get()

	user := &model.User{
		Email:          "clean-agent-user@example.com",
		Token:          "clean-agent-user-token",
		UUID:           "clean-agent-user-uuid",
		TransferEnable: 1 << 40,
	}
	assert.NoError(s.T(), db.Create(user).Error)

	node := &model.ForwardNode{
		Name:    "clean-agent-node",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "203.0.113.10",
		Port:    22,
		Enabled: true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:     "clean-agent-tunnel",
		Type:     1,
		InNodeID: node.ID,
		OutNodeID: func() *uint {
			id := node.ID
			return &id
		}(),
		InIP:   node.Host,
		OutIP:  node.Host,
		Status: model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	assert.NoError(s.T(), db.Create(&model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: tunnel.ID,
		Status:   model.ForwardUserTunnelStatusActive,
	}).Error)

	forward := &model.Forward{
		UserID:         user.ID,
		UserName:       user.Email,
		Name:           "clean-agent-forward",
		TunnelID:       tunnel.ID,
		InPort:         20001,
		RemoteAddr:     "127.0.0.1:8080",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendCleanAgent,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	return node, tunnel, forward
}

func TestForwardCleanAgentService(t *testing.T) {
	suite.Run(t, new(ForwardCleanAgentServiceTestSuite))
}
