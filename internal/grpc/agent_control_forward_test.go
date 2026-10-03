package grpc

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/wire"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newForwardTestEnvironment is the config environment with the kernel's
// forwarding tables.
func newForwardTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newConfigTestEnvironment(t)
	requireAutoMigrate(t, append(model.KernelForwardModels(), &model.ForwardNode{})...)
	return environment
}

func forwardNodeCapabilities() *forwardv1.NodeCapabilities {
	return &forwardv1.NodeCapabilities{
		KernelVersion: "6.12", Cgroup: "v2", AgentVersion: "4.2.0",
		Engines: []*forwardv1.EngineCapabilities{{
			Engine: forwardv1.Engine_ENGINE_NFTABLES, Version: "nft 1.1.3", Available: true, Udp: true, Ipv6: true,
			Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
			LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
			BandwidthLimit: true, Quota: true, MaxConns: true,
		}},
	}
}

// forwardHello is what an F3b Agent lists: config.v1, package-reports.v1
// and forward.v1 with its capabilities.
func forwardHello(t *testing.T) []*agentv1pb.Capability {
	t.Helper()
	forward, err := wire.HelloCapability(forwardNodeCapabilities())
	require.NoError(t, err)
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityPackageReports, Version: agentcontrol.CapabilityVersionV1},
		forward,
	}
}

func configCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityPackageReports, Version: agentcontrol.CapabilityVersionV1},
	}
}

// forwardMember decodes a snapshot's forwarding state; found is false for a
// snapshot without one.
func forwardMember(t *testing.T, snapshot *agentv1pb.ConfigSnapshot) (*forwardv1.NodeForwardState, bool) {
	t.Helper()
	state, found, err := wire.StateFromNodeConfig(snapshot.GetFormat(), snapshot.GetConfigJson())
	require.NoError(t, err)
	if !found {
		var document map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(snapshot.GetConfigJson(), &document))
		assert.NotContains(t, document, wire.NodeConfigMember)
	}
	return state, found
}

// The HelloAck lists forward.v1 only with config.v1 and valid capabilities,
// for proxy and forward nodes; a forward node is offered package-reports.v1
// with it, and asked for a 60-second heartbeat.
func TestAgentControlForwardNegotiation(t *testing.T) {
	server := NewAgentControlGRPCServer(NewAgentControlManager())
	forwardNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}
	proxyNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	listed := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability, name string) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), name, agentcontrol.CapabilityVersionV1)
	}
	hello := forwardHello(t)
	for _, node := range []agentcontrol.AgentNode{forwardNode, proxyNode} {
		assert.True(t, listed(node, hello, agentcontrol.CapabilityForward), node.String())
		assert.True(t, listed(node, hello, agentcontrol.CapabilityPackageReports), node.String())
		assert.False(t, listed(node, configCapabilities(), agentcontrol.CapabilityForward), "a legacy Agent: %s", node)
	}
	assert.False(t, listed(forwardNode, configCapabilities(), agentcontrol.CapabilityPackageReports), "no forward.v1, no package reports for a forward node")
	withoutConfig := []*agentv1pb.Capability{hello[0], hello[2], hello[3]}
	assert.False(t, listed(forwardNode, withoutConfig, agentcontrol.CapabilityForward), "the state rides config.v1")
	malformed := []*agentv1pb.Capability{hello[0], hello[1], hello[2], {Name: agentcontrol.CapabilityForward, Version: agentcontrol.CapabilityVersionV1,
		Attributes: map[string]string{wire.AttributeNodeCapabilities: `{"node_ref":"forward-2"}`}}}
	assert.False(t, listed(forwardNode, malformed, agentcontrol.CapabilityForward), "capabilities naming another node")
	unknownKind := agentcontrol.AgentNode{Kind: "edge", ID: 1}
	assert.False(t, listed(unknownKind, hello, agentcontrol.CapabilityForward))

	assert.EqualValues(t, 60, server.heartbeatInterval(forwardNode, true))
	assert.EqualValues(t, 20, server.heartbeatInterval(forwardNode, false))
	assert.EqualValues(t, 20, server.heartbeatInterval(proxyNode, true), "proxy nodes keep the default")
}

// A legacy Agent (config.v1 without forward.v1) keeps
// anixops.nodeconfig/v1 and gets no forwarding. An Agent that negotiates
// forward.v1 joins the inventory and gets anixops.nodeconfig/v2 with its
// state at once; a plan that moves its generation pushes a new snapshot;
// reconnecting without forward.v1 falls back to v1.
func TestAgentControlForwardConfigPush(t *testing.T) {
	env := newForwardTestEnvironment(t)
	node := env.agentNode()

	legacy := openConfigSession(t, env, 0, configCapabilities()...)
	assert.False(t, agentcontrol.HasCapabilityVersion(legacy.helloAck.ServerCapabilities, agentcontrol.CapabilityForward, agentcontrol.CapabilityVersionV1))
	assert.EqualValues(t, 20, legacy.helloAck.HeartbeatIntervalSeconds)
	first := legacy.next().GetConfig()
	require.NotNil(t, first)
	assert.Equal(t, kernelnodeops.DesiredConfigFormat, first.GetFormat())
	_, found := forwardMember(t, first)
	assert.False(t, found, "a legacy Agent never receives forwarding")
	var inventory int64
	require.NoError(t, database.GetDB().Model(&model.KernelForwardNode{}).Count(&inventory).Error)
	assert.Zero(t, inventory, "a legacy Agent does not join the inventory")
	require.NoError(t, legacy.stream.CloseSend())

	session := openConfigSession(t, env, first.GetConfigRevision(), forwardHello(t)...)
	require.True(t, agentcontrol.HasCapabilityVersion(session.helloAck.ServerCapabilities, agentcontrol.CapabilityForward, agentcontrol.CapabilityVersionV1))
	assert.EqualValues(t, 20, session.helloAck.HeartbeatIntervalSeconds, "a proxy node keeps its heartbeat")
	snapshot := session.next().GetConfig()
	require.NotNil(t, snapshot)
	assert.Equal(t, wire.NodeConfigFormat, snapshot.GetFormat())
	assert.Greater(t, snapshot.GetConfigRevision(), first.GetConfigRevision())
	state, found := forwardMember(t, snapshot)
	require.True(t, found)
	assert.Equal(t, node.String(), state.GetNodeRef())
	assert.EqualValues(t, 1, state.GetGeneration(), "the Hello's plan: an empty state")
	assert.Empty(t, state.GetHops())
	connection, connected := env.manager.Connection(node.ID)
	require.True(t, connected)
	assert.Contains(t, connection.ServerCapabilities, "forward.v1")

	before := snapshotsSent(configTriggerForward)
	forward := &kernelforward.Service{DB: database.GetDB(), Cluster: func() string { return "test" }}
	route, err := forward.CreateRoute(context.Background(), "create-1", &forwardv1.Route{
		Owner:   "admin",
		Listen:  &forwardv1.Listen{Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops:    []*forwardv1.Hop{{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{node.String()}}},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 443}},
	})
	require.NoError(t, err)
	pushed := session.next().GetConfig()
	require.NotNil(t, pushed, "the plan pushed the node's configuration")
	assert.Equal(t, snapshot.GetConfigRevision()+1, pushed.GetConfigRevision())
	state, found = forwardMember(t, pushed)
	require.True(t, found)
	assert.EqualValues(t, 2, state.GetGeneration())
	require.Len(t, state.GetHops(), 1)
	assert.Equal(t, route.GetId(), state.GetHops()[0].GetRouteId())
	assert.Equal(t, before+1, snapshotsSent(configTriggerForward))
	// The agent answers as for any snapshot.
	session.status(&agentv1pb.ConfigStatus{ConfigRevision: pushed.GetConfigRevision(), ConfigHash: pushed.GetConfigHash(), Applied: true})
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck())
	require.NoError(t, session.stream.CloseSend())

	again := openConfigSession(t, env, pushed.GetConfigRevision(), configCapabilities()...)
	fallback := again.next().GetConfig()
	require.NotNil(t, fallback, "the format changed, so the revision moved")
	assert.Equal(t, kernelnodeops.DesiredConfigFormat, fallback.GetFormat())
	_, found = forwardMember(t, fallback)
	assert.False(t, found)
	require.NoError(t, again.stream.CloseSend())
}

func forwardReport(t *testing.T, nodeRef string, observed time.Time, counters ...*forwardv1.Counters) *agentv1pb.PackageReport {
	t.Helper()
	report, err := wire.Report(&forwardv1.NodeForwardReport{
		NodeRef: nodeRef, Generation: 1, Applied: true, ObservedAtUnixMs: observed.UnixMilli(), Counters: counters,
	})
	require.NoError(t, err)
	return report
}

// Forward reports: accepted only on a forward.v1 session, checked for the
// stream's node, metered, never acknowledged; every refusal keeps the
// stream open.
func TestAgentControlForwardReports(t *testing.T) {
	env := newForwardTestEnvironment(t)
	node := env.agentNode()
	nodeID := uint32(node.ID)
	session := openConfigSession(t, env, 0, forwardHello(t)...)
	session.next() // the Hello's snapshot
	forward := &kernelforward.Service{DB: database.GetDB(), Cluster: func() string { return "test" }}
	route, err := forward.CreateRoute(context.Background(), "create-1", &forwardv1.Route{
		Owner:   "admin",
		Listen:  &forwardv1.Listen{Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops:    []*forwardv1.Hop{{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{node.String()}}},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 443}},
	})
	require.NoError(t, err)
	session.next() // the plan's snapshot
	id := route.GetId()
	now := time.Now()

	counts := readPackageReportCounts()
	send := func(report *agentv1pb.PackageReport) {
		t.Helper()
		session.send(packageReportMessage("", nodeID, report))
		session.heartbeat()
		assert.Empty(t, session.untilHeartbeatAck(), "never acknowledged")
	}
	send(forwardReport(t, node.String(), now, &forwardv1.Counters{RouteId: id, CounterEpoch: "e1", UpBytes: 100, DownBytes: 200}))
	after := readPackageReportCounts()
	assert.Equal(t, counts.results[packageReportAccepted]+1, after.results[packageReportAccepted])
	stats, err := forward.RouteStats(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.EqualValues(t, 200, stats[0].GetDownBytes())
	latest, found, err := forward.LatestReport(context.Background(), node.String())
	require.NoError(t, err)
	require.True(t, found)
	assert.EqualValues(t, 1, latest.GetGeneration())

	// An older report is superseded; refusals by reason.
	send(forwardReport(t, node.String(), now.Add(-time.Minute)))
	assert.Equal(t, after.results[packageReportSuperseded]+1, readPackageReportCounts().results[packageReportSuperseded])
	refusals := map[string]*agentv1pb.PackageReport{}
	wrongVersion := forwardReport(t, node.String(), now)
	wrongVersion.Version = "v2"
	refusals[service.PackageReportRefusedInvalid] = wrongVersion
	refusals[service.PackageReportRefusedFuture] = forwardReport(t, node.String(), now.Add(time.Hour))
	refusals[service.PackageReportRefusedBadPayload] = forwardReport(t, "proxy-"+strconv.FormatUint(uint64(nodeID)+1, 10), now)
	oversize := forwardReport(t, node.String(), now)
	oversize.PayloadJson = []byte(`{"node_ref":"` + strings.Repeat("x", agentcontrol.MaxPackageReportPayloadBytes) + `"}`)
	refusals[service.PackageReportRefusedOversize] = oversize
	for reason, report := range refusals {
		before := readPackageReportCounts().reasons[reason]
		send(report)
		assert.Equal(t, before+1, readPackageReportCounts().reasons[reason], reason)
	}
	// A counter beyond the ledger's range is a bad payload too.
	overflow := forwardReport(t, node.String(), now.Add(time.Second), &forwardv1.Counters{RouteId: id, CounterEpoch: "e2", UpBytes: 1 << 63})
	before := readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload]
	send(overflow)
	assert.Equal(t, before+1, readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload])
	// The database failing: unrecorded, and the stream stays open.
	require.NoError(t, database.GetDB().Migrator().DropTable(&model.KernelForwardNodeReport{}))
	unrecorded := readPackageReportCounts().results[packageReportUnrecorded]
	send(forwardReport(t, node.String(), now.Add(2*time.Second)))
	assert.Equal(t, unrecorded+1, readPackageReportCounts().results[packageReportUnrecorded])
	require.NoError(t, session.stream.CloseSend())

	// A session without forward.v1 is refused the report, and the stream
	// stays open.
	legacy := openConfigSession(t, env, 0, configCapabilities()...)
	legacy.next()
	before = readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated]
	legacy.send(packageReportMessage("", nodeID, forwardReport(t, node.String(), now)))
	legacy.heartbeat()
	for {
		if message := legacy.next(); message.GetHeartbeatAck() != nil {
			break
		}
	}
	assert.Equal(t, before+1, readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated])
	require.NoError(t, legacy.stream.CloseSend())
}

// The configuration push counters include the forward trigger.
func TestAgentConfigPrometheusForwardTrigger(t *testing.T) {
	var body strings.Builder
	WriteAgentConfigPrometheus(&body)
	assert.Contains(t, body.String(), `anixops_agent_config_snapshots_sent_total{trigger="forward"}`)
}
