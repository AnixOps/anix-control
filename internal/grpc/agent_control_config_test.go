package grpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newConfigTestEnvironment is the Agent Control test environment with the
// desired configuration tables.
func newConfigTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newAgentControlTestEnvironment(t)
	requireAutoMigrate(t, append(model.KernelNodeOperationModels(), &model.NodeProtocol{})...)
	return environment
}

func (e *agentControlTestEnvironment) agentNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(e.node.ID)}
}

// desiredConfig builds and stores the environment node's desired
// configuration.
func (e *agentControlTestEnvironment) desiredConfig(t *testing.T) model.KernelNodeDesiredConfig {
	t.Helper()
	row, _, err := kernelnodeops.RefreshDesiredConfig(context.Background(), database.GetDB(), e.agentNode(), time.Now())
	require.NoError(t, err)
	return row
}

// configSession is a stream that negotiated config.v1 only.
type configSession struct {
	t        *testing.T
	env      *agentControlTestEnvironment
	stream   agentv1pb.AgentControlService_ControlStreamClient
	helloAck *agentv1pb.HelloAck
	sequence int
}

// openConfigSession sends a Hello with capabilities (agent.ping and
// config.v1 when nil) and configRevision, and returns the session.
func openConfigSession(t *testing.T, env *agentControlTestEnvironment, configRevision uint64, capabilities ...*agentv1pb.Capability) *configSession {
	t.Helper()
	if capabilities == nil {
		capabilities = []*agentv1pb.Capability{
			{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
			{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1},
		}
	}
	ctx, cancel := context.WithTimeout(env.authContext(context.Background()), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := agentv1pb.NewAgentControlServiceClient(env.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(env.node.ID))
	hello.GetHello().Capabilities = capabilities
	hello.GetHello().ConfigRevision = configRevision
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHelloAck())
	return &configSession{t: t, env: env, stream: stream, helloAck: message.GetHelloAck()}
}

func (s *configSession) send(message *agentv1pb.AgentToControl) {
	s.t.Helper()
	s.sequence++
	message.RequestId = "request-" + strings.Repeat("x", s.sequence)
	message.NodeId = uint32(s.env.node.ID)
	message.SentAtUnixMs = time.Now().UnixMilli()
	require.NoError(s.t, s.stream.Send(message))
}

// heartbeat sends a heartbeat; its answer proves everything sent before
// was handled.
func (s *configSession) heartbeat() {
	s.t.Helper()
	s.send(&agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: s.helloAck.SessionId}}})
}

func (s *configSession) status(configStatus *agentv1pb.ConfigStatus) {
	s.t.Helper()
	s.send(&agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_ConfigStatus{ConfigStatus: configStatus}})
}

// next returns the next message.
func (s *configSession) next() *agentv1pb.ControlToAgent {
	s.t.Helper()
	message, err := s.stream.Recv()
	require.NoError(s.t, err)
	return message
}

// untilHeartbeatAck returns the snapshots received before the next
// heartbeat acknowledgement.
func (s *configSession) untilHeartbeatAck() []*agentv1pb.ConfigSnapshot {
	s.t.Helper()
	var snapshots []*agentv1pb.ConfigSnapshot
	for {
		message := s.next()
		if message.GetHeartbeatAck() != nil {
			return snapshots
		}
		require.NotNil(s.t, message.GetConfig(), "unexpected payload %T", message.Payload)
		snapshots = append(snapshots, message.GetConfig())
	}
}

func snapshotsSent(trigger string) uint64 { return agentConfigMetrics.snapshots[trigger].Load() }
func statusesReceived(result string) uint64 {
	return agentConfigMetrics.statuses[result].Load()
}

// HelloAck lists config.v1 only to an agent that lists it.
func TestAgentControlConfigNegotiation(t *testing.T) {
	env := newConfigTestEnvironment(t)
	withConfig := openConfigSession(t, env, 0)
	assert.True(t, agentcontrol.HasCapabilityVersion(withConfig.helloAck.ServerCapabilities, agentcontrol.CapabilityConfig, agentcontrol.CapabilityVersionV1))
	require.NoError(t, withConfig.stream.CloseSend())

	without := openConfigSession(t, env, 7, &agentv1pb.Capability{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1})
	assert.False(t, agentcontrol.HasCapabilityVersion(without.helloAck.ServerCapabilities, agentcontrol.CapabilityConfig, agentcontrol.CapabilityVersionV1))
	without.heartbeat()
	assert.Empty(t, without.untilHeartbeatAck(), "no snapshot without config.v1")

	server := NewAgentControlGRPCServer(NewAgentControlManager())
	config := []*agentv1pb.Capability{{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1}}
	assert.True(t, server.servesConfig(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, config), "forward nodes have a desired configuration")
	assert.False(t, server.servesConfig(agentcontrol.AgentNode{Kind: "other", ID: 1}, config))
	assert.False(t, server.servesConfig(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, []*agentv1pb.Capability{{Name: agentcontrol.CapabilityConfig, Version: "v2"}}))
}

// The Hello reconcile sends one snapshot when the agent's revision is not
// the desired one, and nothing when it is.
func TestAgentControlConfigHelloReconcile(t *testing.T) {
	env := newConfigTestEnvironment(t)
	row := env.desiredConfig(t)
	for _, test := range []struct {
		name     string
		revision uint64
		want     int
	}{
		{"same", row.Revision, 0},
		{"none", 0, 1},
		{"other", row.Revision + 3, 1},
	} {
		before := snapshotsSent(configTriggerHello)
		session := openConfigSession(t, env, test.revision)
		session.heartbeat()
		snapshots := session.untilHeartbeatAck()
		require.Len(t, snapshots, test.want, test.name)
		for _, snapshot := range snapshots {
			assert.Equal(t, row.Revision, snapshot.ConfigRevision)
			assert.Equal(t, row.ConfigHash, snapshot.ConfigHash)
			assert.Equal(t, kernelnodeops.DesiredConfigFormat, snapshot.Format)
			assert.Equal(t, row.ConfigJSON, string(snapshot.ConfigJson))
		}
		assert.Equal(t, before+uint64(test.want), snapshotsSent(configTriggerHello), test.name)
		require.NoError(t, session.stream.CloseSend())
	}

	// A node whose configuration cannot be built is sent nothing; the
	// stream stays open.
	require.NoError(t, database.GetDB().Migrator().DropTable(&model.NodeProtocol{}))
	session := openConfigSession(t, env, 0)
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck())
}

// ConfigStatus is recorded with the kernel's verdict, handed to the
// streams' handlers, and counted; only a verified applied status moves the
// applied revision. A malformed status ends the stream.
func TestAgentControlConfigStatus(t *testing.T) {
	env := newConfigTestEnvironment(t)
	streams := NewAgentStreams(env.manager, NewAgentControlManager())
	var mu sync.Mutex
	var reports []agentstreams.ConfigStatusReport
	streams.OnConfigStatus(func(report agentstreams.ConfigStatusReport) {
		mu.Lock()
		reports = append(reports, report)
		mu.Unlock()
	})
	streams.OnConfigStatus(nil)

	session := openConfigSession(t, env, 0)
	snapshot := session.next().GetConfig()
	require.NotNil(t, snapshot)
	assert.True(t, streams.ConfigNegotiated(env.agentNode()))

	load := func() model.KernelNodeConfigStatus {
		row, found, err := kernelnodeops.LoadConfigStatus(context.Background(), database.GetDB(), env.agentNode())
		require.NoError(t, err)
		require.True(t, found)
		return row
	}
	steps := []struct {
		status  *agentv1pb.ConfigStatus
		verdict string
		applied uint64
	}{
		{&agentv1pb.ConfigStatus{ConfigRevision: snapshot.ConfigRevision, ConfigHash: strings.Repeat("f", 64), Applied: true}, model.ConfigVerdictMismatch, 0},
		{&agentv1pb.ConfigStatus{ConfigRevision: snapshot.ConfigRevision + 1, ConfigHash: snapshot.ConfigHash, Applied: true}, model.ConfigVerdictMismatch, 0},
		{&agentv1pb.ConfigStatus{ConfigRevision: snapshot.ConfigRevision, ConfigHash: snapshot.ConfigHash, Error: "core refused it"}, model.ConfigVerdictFailed, 0},
		{&agentv1pb.ConfigStatus{ConfigRevision: snapshot.ConfigRevision, ConfigHash: snapshot.ConfigHash, Applied: true}, model.ConfigVerdictApplied, snapshot.ConfigRevision},
	}
	for i, step := range steps {
		before := statusesReceived(step.verdict)
		session.status(step.status)
		session.heartbeat()
		assert.Empty(t, session.untilHeartbeatAck())
		row := load()
		assert.Equal(t, step.verdict, row.Verdict, i)
		assert.Equal(t, step.applied, row.AppliedRevision, i)
		assert.Equal(t, step.status.ConfigRevision, row.ReportedRevision, i)
		assert.Equal(t, step.status.Error, row.ReportedError, i)
		assert.Equal(t, session.helloAck.SessionId, row.SessionID)
		assert.Equal(t, before+1, statusesReceived(step.verdict), i)
	}
	mu.Lock()
	require.Len(t, reports, len(steps))
	for i, report := range reports {
		assert.Equal(t, env.agentNode(), report.Node)
		assert.Equal(t, session.helloAck.SessionId, report.SessionID)
		assert.Equal(t, steps[i].verdict, report.Verdict)
	}
	mu.Unlock()

	// After a newer desired revision, the applied one is stale; an applied
	// stale status keeps the applied revision.
	require.NoError(t, database.GetDB().Model(&model.Node{}).Where("id = ?", env.node.ID).Update("name", "renamed").Error)
	newer := env.desiredConfig(t)
	require.Greater(t, newer.Revision, snapshot.ConfigRevision)
	lagging, err := kernelnodeops.ConfigLaggingNodes(context.Background(), database.GetDB())
	require.NoError(t, err)
	assert.Equal(t, int64(1), lagging)
	session.status(&agentv1pb.ConfigStatus{ConfigRevision: snapshot.ConfigRevision, ConfigHash: snapshot.ConfigHash, Applied: true})
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck())
	assert.Equal(t, model.ConfigVerdictStale, load().Verdict)
	assert.Equal(t, snapshot.ConfigRevision, load().AppliedRevision)

	// The database cannot record it: handed on without a verdict, counted
	// unrecorded, the stream stays open.
	require.NoError(t, database.GetDB().Migrator().DropTable(&model.KernelNodeConfigStatus{}))
	before := statusesReceived(configStatusUnrecorded)
	session.status(&agentv1pb.ConfigStatus{ConfigRevision: newer.Revision, ConfigHash: newer.ConfigHash, Applied: true})
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck())
	assert.Equal(t, before+1, statusesReceived(configStatusUnrecorded))
	mu.Lock()
	assert.Empty(t, reports[len(reports)-1].Verdict)
	mu.Unlock()

	// Malformed: no revision, no payload.
	for _, malformed := range []*agentv1pb.ConfigStatus{{ConfigHash: "abc", Applied: true}, nil} {
		session := openConfigSession(t, env, newer.Revision)
		session.status(malformed)
		_, err := session.stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

// failingControlStream is a server stream whose sends fail.
type failingControlStream struct {
	grpc.ServerStream
}

func (failingControlStream) Send(*agentv1pb.ControlToAgent) error {
	return errors.New("stream is gone")
}
func (failingControlStream) Recv() (*agentv1pb.AgentToControl, error) {
	return nil, errors.New("stream is gone")
}
func (failingControlStream) Context() context.Context { return context.Background() }

// AgentStreams.PushConfig sends to a session that negotiated config.v1,
// never a revision older than one it sent, and reports the errors the
// kernel's node sync maps.
func TestAgentStreamsPushConfig(t *testing.T) {
	env := newConfigTestEnvironment(t)
	row := env.desiredConfig(t)
	streams := NewAgentStreams(env.manager, nil)
	snapshot := kernelnodeops.ConfigSnapshotOf(row)
	ctx := context.Background()

	_, _, err := streams.PushConfig(ctx, env.agentNode(), snapshot)
	assert.ErrorIs(t, err, agentstreams.ErrNotConnected)
	_, _, err = streams.PushConfig(ctx, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, snapshot)
	assert.ErrorIs(t, err, agentstreams.ErrNotConnected, "no forward manager")
	assert.False(t, streams.ConfigNegotiated(env.agentNode()))

	old := openConfigSession(t, env, 0, &agentv1pb.Capability{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1})
	old.heartbeat()
	old.untilHeartbeatAck()
	assert.False(t, streams.ConfigNegotiated(env.agentNode()))
	_, _, err = streams.PushConfig(ctx, env.agentNode(), snapshot)
	assert.ErrorIs(t, err, agentstreams.ErrCapabilityMissing)
	require.NoError(t, old.stream.CloseSend())

	session := openConfigSession(t, env, row.Revision)
	session.heartbeat()
	require.Empty(t, session.untilHeartbeatAck())
	require.True(t, streams.ConfigNegotiated(env.agentNode()))

	before := snapshotsSent(configTriggerSync)
	sessionID, sent, err := streams.PushConfig(ctx, env.agentNode(), snapshot)
	require.NoError(t, err)
	assert.True(t, sent, "a sync pushes the revision the agent has too")
	assert.Equal(t, session.helloAck.SessionId, sessionID)
	assert.Equal(t, row.Revision, session.next().GetConfig().GetConfigRevision())
	assert.Equal(t, before+1, snapshotsSent(configTriggerSync), "counted before PushConfig returned")

	newer := kernelnodeops.ConfigSnapshotOf(row)
	newer.ConfigRevision = row.Revision + 1
	_, sent, err = streams.PushConfig(ctx, env.agentNode(), newer)
	require.NoError(t, err)
	assert.True(t, sent)
	assert.Equal(t, newer.ConfigRevision, session.next().GetConfig().GetConfigRevision())
	_, sent, err = streams.PushConfig(ctx, env.agentNode(), snapshot)
	require.NoError(t, err)
	assert.False(t, sent, "never older than what the session was sent")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err = streams.PushConfig(cancelled, env.agentNode(), snapshot)
	assert.ErrorIs(t, err, context.Canceled)

	// A session whose stream is gone.
	broken := &AgentControlConnection{NodeID: uint32(env.node.ID), SessionID: "broken", stream: failingControlStream{}, configNegotiated: true}
	_, err = broken.sendConfig(newer, true, configTriggerSync)
	require.Error(t, err)
	server := NewAgentControlGRPCServer(env.manager)
	assert.Error(t, pushDesiredConfig(ctx, broken, env.agentNode(), configTriggerHello))
	setConfigRefreshInterval(time.Millisecond)
	stop := server.startConfigRefresh(ctx, broken, env.agentNode())
	time.Sleep(50 * time.Millisecond)
	stop()
	setConfigRefreshInterval(defaultConfigRefreshInterval)
	forward := NewAgentStreams(nil, env.manager)
	env.manager.mu.Lock()
	current := env.manager.connections[uint32(env.node.ID)]
	env.manager.connections[uint32(env.node.ID)] = broken
	env.manager.mu.Unlock()
	_, _, err = forward.PushConfig(ctx, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(env.node.ID)}, kernelnodeops.ConfigSnapshotOf(row))
	assert.ErrorIs(t, err, agentstreams.ErrNotConnected)
	env.manager.mu.Lock()
	env.manager.connections[uint32(env.node.ID)] = current
	env.manager.mu.Unlock()
}

// The refresher rebuilds the desired configuration and sends a snapshot
// when its revision moved.
func TestAgentControlConfigRefresh(t *testing.T) {
	env := newConfigTestEnvironment(t)
	setConfigRefreshInterval(20 * time.Millisecond)
	t.Cleanup(func() { setConfigRefreshInterval(defaultConfigRefreshInterval) })
	row := env.desiredConfig(t)
	session := openConfigSession(t, env, row.Revision)
	before := snapshotsSent(configTriggerRefresh)
	require.NoError(t, database.GetDB().Model(&model.Node{}).Where("id = ?", env.node.ID).Update("host", "192.0.2.10").Error)
	snapshot := session.next().GetConfig()
	require.NotNil(t, snapshot)
	assert.Equal(t, row.Revision+1, snapshot.ConfigRevision)
	assert.Contains(t, string(snapshot.ConfigJson), "192.0.2.10")
	require.Eventually(t, func() bool { return snapshotsSent(configTriggerRefresh) == before+1 }, 5*time.Second, 5*time.Millisecond)
}

// The metrics: snapshots by trigger, statuses by result and the lagging
// nodes; no gauge when the tables are missing.
func TestWriteAgentConfigPrometheus(t *testing.T) {
	env := newConfigTestEnvironment(t)
	row := env.desiredConfig(t)
	require.NoError(t, database.GetDB().Create(&model.KernelNodeConfigStatus{
		NodeKind: agentcontrol.NodeKindProxy, NodeID: uint64(env.node.ID), Verdict: model.ConfigVerdictStale,
		ReportedRevision: row.Revision, ReportedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	agentConfigMetrics.status("")
	var body strings.Builder
	WriteAgentConfigPrometheus(&body)
	text := body.String()
	for _, want := range []string{
		`anixops_agent_config_snapshots_sent_total{trigger="hello"}`,
		`anixops_agent_config_snapshots_sent_total{trigger="sync"}`,
		`anixops_agent_config_snapshots_sent_total{trigger="refresh"}`,
		`anixops_agent_config_statuses_total{result="applied"}`,
		`anixops_agent_config_statuses_total{result="mismatch"}`,
		`anixops_agent_config_statuses_total{result="unrecorded"}`,
		"anixops_agent_config_lagging_nodes 1\n",
	} {
		assert.Contains(t, text, want)
	}
	agentConfigMetrics.sent("unknown")
	agentConfigMetrics.status("unknown")

	require.NoError(t, database.GetDB().Migrator().DropTable(&model.KernelNodeConfigStatus{}))
	body.Reset()
	WriteAgentConfigPrometheus(&body)
	assert.NotContains(t, body.String(), "anixops_agent_config_lagging_nodes")
	assert.Contains(t, body.String(), "anixops_agent_config_statuses_total")
}
