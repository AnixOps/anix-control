package grpc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNetworkTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newConfigTestEnvironment(t)
	requireAutoMigrate(t, &model.NetworkModel{}, &model.NetworkTransportKey{}, &model.NetworkDevice{}, &model.OperationLog{}, &model.ForwardNode{})
	return environment
}

// networkHello is what a network.v1 Agent lists: config.v1,
// package-reports.v1 and network.v1 with its capabilities.
func networkHello(t *testing.T, node agentcontrol.AgentNode) []*agentv1pb.Capability {
	t.Helper()
	capability, err := network.HelloCapability(&networkv1.DeviceCapabilities{DeviceRef: node.String(), Kinds: []*networkv1.KindSupport{
		{Component: networkv1.Component_COMPONENT_INGRESS, Kind: network.IngressDirect, Versions: []uint32{1}},
		{Component: networkv1.Component_COMPONENT_EGRESS, Kind: network.EgressNATDirect, Versions: []uint32{1}},
	}})
	require.NoError(t, err)
	return append(configCapabilities(), capability)
}

func networkMember(t *testing.T, snapshot *agentv1pb.ConfigSnapshot) (*networkv1.NetworkPlan, bool) {
	t.Helper()
	plan, found, err := network.PlanFromNodeConfig(snapshot.GetConfigJson())
	require.NoError(t, err)
	return plan, found
}

// The HelloAck lists network.v1 only with config.v1 and valid capabilities;
// a forward node is offered package-reports.v1 with it.
func TestAgentControlNetworkNegotiation(t *testing.T) {
	server := NewAgentControlGRPCServer(NewAgentControlManager())
	forwardNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}
	proxyNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	listed := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability, name string) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), name, agentcontrol.CapabilityVersionV1)
	}
	for _, node := range []agentcontrol.AgentNode{forwardNode, proxyNode} {
		hello := networkHello(t, node)
		assert.True(t, listed(node, hello, agentcontrol.CapabilityNetwork), node.String())
		assert.True(t, listed(node, hello, agentcontrol.CapabilityPackageReports), node.String())
		assert.False(t, listed(node, configCapabilities(), agentcontrol.CapabilityNetwork), node.String())
		withoutConfig := []*agentv1pb.Capability{hello[0], hello[2], hello[3]}
		assert.False(t, listed(node, withoutConfig, agentcontrol.CapabilityNetwork), "the plan rides config.v1")
	}
	other := networkHello(t, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 2})
	assert.False(t, listed(proxyNode, other, agentcontrol.CapabilityNetwork), "capabilities naming another device")
}

// An Agent that negotiates network.v1 gets the member "network" at once
// (generation 0 while the model gives it nothing); a model write that
// gives it a plan pushes a new snapshot; its NetworkStatus is recorded and
// its public keys kept; reconnecting without network.v1 drops the member.
func TestAgentControlNetworkPlanAndStatus(t *testing.T) {
	env := newNetworkTestEnvironment(t)
	node := env.agentNode()
	nodeID := uint32(node.ID)

	session := openConfigSession(t, env, 0, networkHello(t, node)...)
	require.True(t, agentcontrol.HasCapabilityVersion(session.helloAck.ServerCapabilities, agentcontrol.CapabilityNetwork, agentcontrol.CapabilityVersionV1))
	first := session.next().GetConfig()
	require.NotNil(t, first)
	plan, found := networkMember(t, first)
	require.True(t, found)
	assert.Equal(t, node.String(), plan.GetDeviceRef())
	assert.Zero(t, plan.GetGeneration())

	before := snapshotsSent(configTriggerNetwork)
	_, err := networkmodel.New(database.GetDB()).Replace(context.Background(), &networkmodel.Document{
		Lines: []networkmodel.Line{{ID: "ln-1", Node: node.String(), Interface: "eth0", Families: []string{"v4"}}},
		Ingresses: []networkmodel.Ingress{{ID: "in-1", Line: "ln-1", Kind: "direct", Audience: "user", Family: "v4",
			Listen: networkmodel.Endpoint{Address: "0.0.0.0", Port: 443}, Protocols: []string{"vless"}}},
		Egresses: []networkmodel.Egress{{ID: "eg-1", Line: "ln-1", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "auto"}},
		Paths:    []networkmodel.Path{{ID: "pa-1", Ingress: "in-1", Egress: "eg-1"}},
	}, nil, networkmodel.Actor{})
	require.NoError(t, err)
	pushed := session.next().GetConfig()
	require.NotNil(t, pushed, "the model write pushed the device's configuration")
	assert.Greater(t, pushed.GetConfigRevision(), first.GetConfigRevision())
	plan, found = networkMember(t, pushed)
	require.True(t, found)
	assert.EqualValues(t, 1, plan.GetGeneration())
	require.Len(t, plan.GetPaths(), 1)
	require.NoError(t, network.Check(plan))
	assert.Equal(t, before+1, snapshotsSent(configTriggerNetwork))

	report, err := network.StatusReport(&networkv1.NetworkStatus{
		DeviceRef: node.String(), Generation: 1, PlanHash: plan.GetHash(), ObservedAtUnixMs: time.Now().UnixMilli(),
		Components: []*networkv1.ComponentStatus{{Component: networkv1.Component_COMPONENT_EGRESS, Id: "eg-1", State: networkv1.ComponentState_COMPONENT_STATE_APPLIED}},
		PublicKeys: []*networkv1.PublicKey{{Kind: network.TransportAnxLine, Key: "anx1-key"}},
	})
	require.NoError(t, err)
	accepted := readPackageReportCounts().results[packageReportAccepted]
	session.send(packageReportMessage("", nodeID, report))
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck(), "never acknowledged")
	assert.Equal(t, accepted+1, readPackageReportCounts().results[packageReportAccepted])
	status, err := networkmodel.New(database.GetDB()).Status(context.Background(), node.String())
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, plan.GetHash(), status.GetPlanHash())
	var device model.NetworkDevice
	require.NoError(t, database.GetDB().First(&device, "node_ref = ?", node.String()).Error)
	keys := map[string]string{}
	require.NoError(t, json.Unmarshal([]byte(device.PublicKeysJSON), &keys))
	assert.Equal(t, "anx1-key", keys[network.TransportAnxLine])

	other, err := network.StatusReport(&networkv1.NetworkStatus{DeviceRef: "proxy-" + strconv.FormatUint(uint64(nodeID)+1, 10)})
	require.NoError(t, err)
	bad := readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload]
	session.send(packageReportMessage("", nodeID, other))
	session.heartbeat()
	session.untilHeartbeatAck()
	assert.Equal(t, bad+1, readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload], "a status naming another device")
	require.NoError(t, session.stream.CloseSend())

	legacy := openConfigSession(t, env, pushed.GetConfigRevision(), configCapabilities()...)
	fallback := legacy.next().GetConfig()
	require.NotNil(t, fallback)
	_, found = networkMember(t, fallback)
	assert.False(t, found, "without network.v1 the member is gone")
	unnegotiated := readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated]
	legacy.send(packageReportMessage("", nodeID, report))
	legacy.heartbeat()
	legacy.untilHeartbeatAck()
	assert.Equal(t, unnegotiated+1, readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated])
	require.NoError(t, legacy.stream.CloseSend())
}
