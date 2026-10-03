package grpc

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamAgent is a fake agent that implements every data-plane capability
// on one Agent Control stream: it sorts what Control sends by kind.
type streamAgent struct {
	t       *testing.T
	stream  agentv1pb.AgentControlService_ControlStreamClient
	node    uint32
	session string

	sendMu   sync.Mutex
	messages map[string]chan *agentv1pb.ControlToAgent
	done     chan error
}

func newStreamAgent(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient, node uint32, session string) *streamAgent {
	agent := &streamAgent{t: t, stream: stream, node: node, session: session, messages: map[string]chan *agentv1pb.ControlToAgent{}, done: make(chan error, 1)}
	for _, kind := range []string{"config", "users", "heartbeat_ack", "report_ack", "maintenance_ack", "desired_operation"} {
		agent.messages[kind] = make(chan *agentv1pb.ControlToAgent, 64)
	}
	go func() {
		for {
			message, err := stream.Recv()
			if err != nil {
				agent.done <- err
				return
			}
			var kind string
			switch message.Payload.(type) {
			case *agentv1pb.ControlToAgent_Config:
				kind = "config"
			case *agentv1pb.ControlToAgent_Users:
				kind = "users"
			case *agentv1pb.ControlToAgent_HeartbeatAck:
				kind = "heartbeat_ack"
			case *agentv1pb.ControlToAgent_ReportAck:
				kind = "report_ack"
			case *agentv1pb.ControlToAgent_MaintenanceAck:
				kind = "maintenance_ack"
			case *agentv1pb.ControlToAgent_DesiredOperation:
				kind = "desired_operation"
			default:
				agent.done <- io.ErrUnexpectedEOF
				return
			}
			agent.messages[kind] <- message
		}
	}()
	return agent
}

func (a *streamAgent) send(requestID string, revision uint64, message *agentv1pb.AgentToControl) {
	a.t.Helper()
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	message.RequestId, message.NodeId, message.Revision, message.SentAtUnixMs = requestID, a.node, revision, time.Now().UnixMilli()
	require.NoError(a.t, a.stream.Send(message))
}

func (a *streamAgent) await(kind string) *agentv1pb.ControlToAgent {
	a.t.Helper()
	select {
	case message := <-a.messages[kind]:
		return message
	case err := <-a.done:
		a.t.Fatalf("the stream ended while awaiting %s: %v", kind, err)
	case <-time.After(10 * time.Second):
		a.t.Fatalf("no %s within 10s", kind)
	}
	return nil
}

// legacyCounters is the legacy agent channel counters of the metrics
// endpoint: every path the legacy HTTP, WebSocket and API key stream
// served or refused.
func legacyCounters() string {
	var body strings.Builder
	agenttransport.WritePrometheus(&body)
	var lines []string
	for _, line := range strings.Split(body.String(), "\n") {
		if strings.HasPrefix(line, "anixops_agent_legacy_") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// Under agent_control.mtls: required, an Agent that enrolled with a
// one-time credential and negotiates every data-plane capability on the
// mTLS stream needs no legacy HTTP or WebSocket path: configuration, users,
// heartbeats, runtime health and system status, traffic, logs, maintenance
// events and diagnostic tasks all flow on the stream, with the side effects
// of the legacy paths, and no legacy channel is served or refused.
func TestAgentStreamUnderRequiredNeedsNoLegacyPath(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSRequired, true)
	db := database.Get()
	requireAutoMigrate(t, append(model.KernelNodeOperationModels(), model.KernelForwardModels()...)...)
	requireAutoMigrate(t, &model.Plan{}, &model.User{}, &model.SubscriberChange{}, &model.SubscriberRequest{}, &model.WireGuardPeer{},
		&model.TrafficLog{}, &model.StatServer{}, &model.NodeLog{}, &model.AgentReportBatch{}, &model.ForwardCleanAgent{})
	require.NoError(t, nodesecrets.EnsureSchema(db))
	ctx := testContext(t)
	nodeID := uint32(l.proxy.ID)
	// The listener serves the process's managers, which keep each node's
	// desired and observed operations across sessions: start from none,
	// and leave none to later tests of the same node id.
	forgetNode := func() {
		manager := GetAgentControlManager()
		manager.mu.Lock()
		delete(manager.observed, nodeID)
		delete(manager.desired, nodeID)
		delete(manager.desiredRevision, nodeID)
		manager.mu.Unlock()
	}
	forgetNode()
	t.Cleanup(forgetNode)

	// The node serves plan group 7, with two active subscribers.
	groupID := uint(7)
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", l.proxy.ID).Updates(map[string]any{"group_id": groupID, "last_check_at": 0}).Error)
	future := time.Now().Add(time.Hour).Unix()
	for _, user := range []model.User{
		{ID: 7, Email: "seven@example.test", Token: "token-7", UUID: "uuid-7", TransferEnable: 1 << 40, ExpiredAt: &future, GroupID: &groupID},
		{ID: 8, Email: "eight@example.test", Token: "token-8", UUID: "uuid-8", TransferEnable: 1 << 40, ExpiredAt: &future, GroupID: &groupID},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, subscriber.RecordChangesTx(db, []uint{7, 8}, false, time.Now()))
	legacyBefore := legacyCounters()

	// Enrollment: required accepts only a one-time credential.
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
	require.NoError(t, err)
	certificate, issued, err := enrollForTest(t, ctx, l.dial(t, nil), credential)
	require.NoError(t, err)

	capabilities := []*agentv1pb.Capability{{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1}, {Name: "agent.diagnostic", Version: agentcontrol.CapabilityVersionV1}}
	for _, name := range []string{agentcontrol.CapabilityConfig, agentcontrol.CapabilityUsers, agentcontrol.CapabilityReports, agentcontrol.CapabilityMaintenance} {
		capabilities = append(capabilities, &agentv1pb.Capability{Name: name, Version: agentcontrol.CapabilityVersionV1})
	}
	stream, err := agentv1pb.NewAgentControlServiceClient(l.dial(t, certificate)).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(nodeID)
	hello.GetHello().Capabilities = capabilities
	require.NoError(t, stream.Send(hello))
	first, err := stream.Recv()
	require.NoError(t, err)
	helloAck := first.GetHelloAck()
	require.NotNil(t, helloAck)
	assert.Equal(t, []string{"config.v1", "users.v1", "reports.v1", "maintenance.v1"}, capabilityVersions(helloAck.ServerCapabilities),
		"Control offers exactly what the Agent listed and serves")
	agent := newStreamAgent(t, stream, nodeID, helloAck.SessionId)

	// Configuration (UniProxy config, the WebSocket's config_update).
	snapshot := agent.await("config").GetConfig()
	require.NotZero(t, snapshot.GetConfigRevision())
	agent.send("config-status", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_ConfigStatus{ConfigStatus: &agentv1pb.ConfigStatus{
		ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: snapshot.GetConfigHash(), Applied: true,
	}}})

	// Users (UniProxy user, the WebSocket's user_update and user_ban).
	delta := agent.await("users").GetUsers()
	require.True(t, delta.GetFull())
	require.True(t, delta.GetLastPage())
	var served []uint64
	for _, user := range delta.GetUpserts() {
		served = append(served, user.GetUserId())
	}
	assert.ElementsMatch(t, []uint64{7, 8}, served)

	// Heartbeat (/api/v2/agent/heartbeat, the WebSocket's heartbeat): the
	// node is online.
	agent.send("heartbeat", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: agent.session, UptimeSeconds: 30}}})
	agent.await("heartbeat_ack")

	// System status and runtime health (/api/v2/node/heartbeat and
	// /api/v2/node/runtime-health).
	agent.send("status", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Status{Status: &agentv1pb.NodeStatus{
		CpuUsagePercent: 12.5, MemoryUsagePercent: 40, DiskUsagePercent: 55, UptimeSeconds: 3600,
		RuntimeHealthy: false, RuntimeError: "wg0 is down", ObservedAtUnixMs: time.Now().UnixMilli(),
	}}})

	// Traffic and online IPs (UniProxy push and alive), then logs.
	agent.send("traffic", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Traffic{Traffic: &agentv1pb.TrafficReport{
		BatchId: "node:proxy-1:boot:1", Users: []*agentv1pb.UserTraffic{{UserId: 7, UploadBytes: 100, DownloadBytes: 200}},
		Online: []*agentv1pb.OnlineUser{{UserId: 7, Ips: []string{"203.0.113.7"}}}, WindowEndUnixMs: time.Now().UnixMilli(),
	}}})
	assert.True(t, agent.await("report_ack").GetReportAck().GetApplied())
	agent.send("logs", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Logs{Logs: &agentv1pb.LogBatch{
		BatchId: "node:proxy-1:boot:2", Entries: []*agentv1pb.LogEntry{{Level: "info", Source: "core", Message: "started", LoggedAtUnixMs: time.Now().UnixMilli()}},
	}}})
	assert.True(t, agent.await("report_ack").GetReportAck().GetApplied())

	// The maintenance outbox (the WebSocket's maintenance_events).
	agent.send("maintenance", 0, maintenanceMessage("", nodeID, agentcontrol.MaintenanceSchemaV1, maintenanceEventJSON(t, nodeID, "event-required")))
	maintenance := agent.await("maintenance_ack").GetMaintenanceAck()
	require.Len(t, maintenance.GetEvents(), 1)
	assert.True(t, maintenance.GetEvents()[0].GetPersisted())

	// A diagnostic task (the WebSocket's task.assign, /api/v2/agent/tasks
	// and /result), as the agent.diagnostic executor dispatches it.
	dispatched := make(chan *agentv1pb.OperationAck, 1)
	go func() {
		ack, err := GetAgentStreams().Dispatch(ctx, l.proxyNode(), &agentv1pb.DesiredOperation{
			OperationId: "diagnostic-1", Kind: "agent.diagnostic",
			PayloadJson: []byte(`{"task":{"id":"diagnostic-1","type":"diagnostic","action":"ping","params":{"target":"1.1.1.1"},"timeout":5}}`),
		})
		assert.NoError(t, err)
		dispatched <- ack
	}()
	desired := agent.await("desired_operation").GetDesiredOperation()
	assert.Equal(t, "agent.diagnostic", desired.GetKind())
	agent.send("diagnostic-ack", desired.GetRevision(), &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
		OperationId: desired.GetOperationId(), Accepted: true, AcceptedAtUnixMs: time.Now().UnixMilli(), SessionId: agent.session, Revision: desired.GetRevision(),
	}}})
	select {
	case ack := <-dispatched:
		require.NotNil(t, ack)
		assert.True(t, ack.GetAccepted())
	case <-time.After(10 * time.Second):
		t.Fatal("the diagnostic task was not acknowledged")
	}
	agent.send("diagnostic-done", desired.GetRevision(), &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: &agentv1pb.ObservedState{
		OperationId: desired.GetOperationId(), Revision: desired.GetRevision(), Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		StateJson: []byte(`{"success":true,"output":"pong"}`), ObservedAtUnixMs: time.Now().UnixMilli(), SessionId: agent.session,
	}}})
	// A heartbeat round trip orders every message above before the checks.
	agent.send("heartbeat-2", 0, &agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: agent.session, UptimeSeconds: 60}}})
	agent.await("heartbeat_ack")
	observed, ok := GetAgentStreams().Observed(l.proxyNode())
	require.True(t, ok)
	assert.Equal(t, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, observed.GetPhase())

	// The side effects the legacy paths had.
	var node model.Node
	require.NoError(t, db.First(&node, l.proxy.ID).Error)
	assert.Equal(t, model.NodeStatusOnline, node.Status)
	assert.NotZero(t, node.LastCheckAt)
	assert.InDelta(t, 12.5, node.CPUUsage, 0.001)
	assert.Equal(t, int64(3600), node.Uptime)
	assert.False(t, node.RuntimeHealthy)
	assert.Equal(t, "wg0 is down", node.RuntimeError)
	assert.NotZero(t, node.RuntimeCheckedAt)
	assert.Equal(t, 1, node.OnlineUsers)
	var user model.User
	require.NoError(t, db.First(&user, 7).Error)
	assert.Equal(t, int64(100), user.U)
	assert.Equal(t, int64(200), user.D)
	var logs []model.NodeLog
	require.NoError(t, db.Where("node_id = ?", l.proxy.ID).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, "started", logs[0].Message)
	assert.Equal(t, service.NodeLogSourceMaintenance, logs[1].Source)
	var configStatus model.KernelNodeConfigStatus
	require.NoError(t, db.Where("node_kind = ? AND node_id = ?", agentcontrol.NodeKindProxy, l.proxy.ID).First(&configStatus).Error)
	assert.Equal(t, model.ConfigVerdictApplied, configStatus.Verdict)
	assert.Equal(t, snapshot.GetConfigRevision(), configStatus.AppliedRevision)

	// The session shows how it authenticated and what it negotiated.
	session, ok := GetAgentControlManager().Connection(nodeID)
	require.True(t, ok)
	assert.Equal(t, agentstreams.AuthenticationMTLS, session.Authentication)
	assert.Equal(t, model.AgentTransportMTLSStream, session.Transport)
	assert.Equal(t, issued.GetSpiffeId(), session.Identity)
	require.NotNil(t, session.Certificate)
	assert.Equal(t, issued.GetSerial(), session.Certificate.Serial)
	assert.Equal(t, issued.GetSpiffeId(), session.Certificate.SPIFFEID)
	assert.Equal(t, issued.GetNotAfterUnix(), session.Certificate.NotAfter.Unix())
	assert.Equal(t, []string{"config.v1", "users.v1", "reports.v1", "maintenance.v1"}, session.NegotiatedCapabilities)
	encoded, err := json.Marshal(session)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"authentication":"mtls"`)
	assert.Contains(t, string(encoded), `"negotiated_capabilities":["config.v1","users.v1","reports.v1","maintenance.v1"]`)

	// No legacy channel was served or refused, and the inventory has the
	// node on the mTLS stream only, ready for required.
	assert.Equal(t, legacyBefore, legacyCounters())
	inventory, err := agenttransport.Build(ctx, db, agenttransport.Policy{Mode: config.AgentMTLSRequired}, agenttransport.Options{
		Live: agenttransport.Default(), Sessions: GetAgentStreams().Sessions,
	})
	require.NoError(t, err)
	var entry *agenttransport.NodeTransports
	for index := range inventory.Nodes {
		if inventory.Nodes[index].Node == l.proxyNode().String() {
			entry = &inventory.Nodes[index]
		}
	}
	require.NotNil(t, entry)
	assert.Equal(t, agenttransport.StatusMTLS, entry.Status)
	require.Len(t, entry.Transports, 1, "%+v", entry.Transports)
	assert.Equal(t, model.AgentTransportMTLSStream, entry.Transports[0].Transport)
	require.NotNil(t, entry.Session)
	assert.Equal(t, agent.session, entry.Session.SessionID)
	assert.Equal(t, agentstreams.AuthenticationMTLS, entry.Session.Authentication)
	assert.Equal(t, issued.GetSerial(), entry.Session.Certificate.Serial)
	assert.Equal(t, []string{"config.v1", "users.v1", "reports.v1", "maintenance.v1"}, entry.Session.NegotiatedCapabilities)

	require.NoError(t, stream.CloseSend())
	select {
	case err := <-agent.done:
		assert.ErrorIs(t, err, io.EOF)
	case <-time.After(10 * time.Second):
		t.Fatal("the stream did not end")
	}
}
