package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
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

// pendingCleanAgentJob queues a clean agent job for a node; its payload
// stands for the node's API token that real payloads carry.
func (s *ForwardCleanAgentServiceTestSuite) pendingCleanAgentJob(nodeID uint) *model.ForwardRuntimeJob {
	job := &model.ForwardRuntimeJob{
		Backend: model.ForwardRuntimeBackendCleanAgent, Action: model.ForwardRuntimeJobActionSync,
		NodeID: uintPtr(nodeID), Status: model.ForwardRuntimeJobStatusPending,
		Payload: fmt.Sprintf(`{"node":%d}`, nodeID),
	}
	s.Require().NoError(database.Get().Create(job).Error)
	return job
}

func (s *ForwardCleanAgentServiceTestSuite) claimedJobIDs(agentID uint, token string) []uint {
	actions, err := s.svc.Heartbeat(ForwardCleanAgentHeartbeatInput{AgentID: agentID, Token: token})
	s.Require().NoError(err)
	ids := []uint{}
	for _, action := range actions {
		ids = append(ids, action.JobID)
	}
	return ids
}

// A clean agent's registration set its node to the body's nodeId, so the
// token of an agent issued for one node could register under any other and
// claim that node's jobs, whose payloads carry the node's API token. A token
// is now bound to the node it was issued for.
func (s *ForwardCleanAgentServiceTestSuite) TestAgentIssuedForANodeCannotRegisterUnderAnother() {
	db := database.Get()
	nodeA, _, _ := s.createCleanAgentForwardFixtures()
	nodeB := &model.ForwardNode{Name: "victim-node", Type: model.ForwardNodeTypeRelay, Host: "203.0.113.11", Port: 22, Enabled: true}
	s.Require().NoError(db.Create(nodeB).Error)
	jobA := s.pendingCleanAgentJob(nodeA.ID)
	jobB := s.pendingCleanAgentJob(nodeB.ID)

	issued, err := s.svc.CreateToken(ForwardCleanAgentCreateInput{Name: "agent-a", NodeID: &nodeA.ID})
	s.Require().NoError(err)

	agent, err := s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token, NodeID: &nodeB.ID, Hostname: "attacker"})
	s.Require().ErrorIs(err, ErrForwardCleanAgentNodeMismatch)
	s.Nil(agent)
	var stored model.ForwardCleanAgent
	s.Require().NoError(db.First(&stored, issued.Agent.ID).Error)
	s.Require().NotNil(stored.NodeID)
	s.Equal(nodeA.ID, *stored.NodeID, "the refused registration moved the agent")
	s.Empty(stored.Hostname, "the refused registration changed the agent")
	s.Equal(model.ForwardCleanAgentStatusOffline, stored.Status)

	// Its own node, or no node, registers it; it claims its node's jobs only.
	agent, err = s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token, NodeID: &nodeA.ID})
	s.Require().NoError(err)
	s.Equal(nodeA.ID, *agent.NodeID)
	agent, err = s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token})
	s.Require().NoError(err)
	s.Equal(nodeA.ID, *agent.NodeID)
	s.Equal([]uint{jobA.ID}, s.claimedJobIDs(agent.ID, issued.Token))

	var pending model.ForwardRuntimeJob
	s.Require().NoError(db.First(&pending, jobB.ID).Error)
	s.Equal(model.ForwardRuntimeJobStatusPending, pending.Status, "another node's job was claimed")
	s.Nil(pending.AgentID)
}

// A token issued without a node is bound by its first registration that
// names one, and cannot move afterwards.
func (s *ForwardCleanAgentServiceTestSuite) TestUnboundAgentIsBoundByItsFirstRegistration() {
	db := database.Get()
	nodeA, _, _ := s.createCleanAgentForwardFixtures()
	nodeB := &model.ForwardNode{Name: "victim-node", Type: model.ForwardNodeTypeRelay, Host: "203.0.113.11", Port: 22, Enabled: true}
	s.Require().NoError(db.Create(nodeB).Error)
	jobA := s.pendingCleanAgentJob(nodeA.ID)
	jobB := s.pendingCleanAgentJob(nodeB.ID)

	issued, err := s.svc.CreateToken(ForwardCleanAgentCreateInput{Name: "unbound"})
	s.Require().NoError(err)
	s.Nil(issued.Agent.NodeID)

	// A registration without a node leaves it unbound.
	agent, err := s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token})
	s.Require().NoError(err)
	s.Nil(agent.NodeID)

	agent, err = s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token, NodeID: &nodeA.ID})
	s.Require().NoError(err)
	s.Require().NotNil(agent.NodeID)
	s.Equal(nodeA.ID, *agent.NodeID)

	_, err = s.svc.Register(ForwardCleanAgentRegisterInput{Token: issued.Token, NodeID: &nodeB.ID})
	s.Require().ErrorIs(err, ErrForwardCleanAgentNodeMismatch)
	s.Equal([]uint{jobA.ID}, s.claimedJobIDs(agent.ID, issued.Token))

	var pending model.ForwardRuntimeJob
	s.Require().NoError(db.First(&pending, jobB.ID).Error)
	s.Equal(model.ForwardRuntimeJobStatusPending, pending.Status, "another node's job was claimed")
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
