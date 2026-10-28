package grpc

import (
	"context"
	"log/slog"
	"sync"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

// The device network model on the Agent Control stream (network.v1;
// docs/design/network-model, sdk/network):
//
//   - Negotiation. The HelloAck lists network.v1 for a proxy or forward
//     node when the Hello lists it with a valid device_capabilities
//     attribute and Control serves the session config.v1, which carries the
//     plan. A forward node is then offered package-reports.v1 too, for its
//     status.
//   - Hello. networkmodel.RecordHello records the device's capabilities and
//     its negotiated flag (and clears the flag for a Hello without
//     network.v1) before the Hello's configuration push.
//   - Plan. While the flag is set, the device's configuration document
//     carries the member "network" (networkmodel.NodeConfigMember). A write
//     that moves a device's plan generation pushes its configuration at
//     once (pushNetworkPlans); the minute refresh catches anything missed.
//   - Status. A PackageReport with plugin_id "network" and kind
//     "network.status" is the device's NetworkStatus: accepted only on a
//     session that negotiated network.v1, checked by network.DecodeStatus
//     for the stream's node and recorded by networkmodel.RecordStatus (a new
//     public key replans the paths waiting for it).

// networkCapabilities decodes the device capabilities of a Hello that lists
// network.v1; listed is false when it does not.
func networkCapabilities(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) (caps *networkv1.DeviceCapabilities, listed bool, err error) {
	return network.DeviceCapabilitiesFromHello(agent, node.String())
}

// servesNetwork tells whether the HelloAck advertises network.v1.
func (s *AgentControlGRPCServer) servesNetwork(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	if !s.servesConfig(node, agent) {
		return false
	}
	_, listed, err := networkCapabilities(node, agent)
	return listed && err == nil
}

// recordNetworkHello records the session's network capabilities, or that
// it has none. Errors are logged: the session keeps the device's last
// recorded flag.
func (s *AgentControlGRPCServer) recordNetworkHello(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode, hello *agentv1pb.Hello) {
	db := databaseForAgentChecks()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forwardHelloTimeout)
	defer cancel()
	caps, listed, err := networkCapabilities(node, hello.GetCapabilities())
	if listed && err != nil {
		slog.Warn("agent network: the Hello's device capabilities were refused; network.v1 is not served", "component", "agent-control",
			"node", node.String(), "error", err)
	}
	if !connection.networkNegotiated {
		caps = nil
	}
	if err := networkmodel.New(db).RecordHello(ctx, node, caps); err != nil {
		slog.Warn("agent network: the Hello could not be recorded", "component", "agent-control", "node", node.String(), "error", err)
	}
}

// handleNetworkStatus records a network status. Like every package report
// it is never acknowledged, and a refusal leaves the stream open.
func (s *AgentControlGRPCServer) handleNetworkStatus(connection *AgentControlConnection, node agentcontrol.AgentNode, report *agentv1pb.PackageReport) {
	refuse := func(reason string, err error) {
		agentPackageReportMetrics.refused(reason)
		slog.Warn("refused agent network status", "component", "agent-control", "node", node.String(), "reason", reason, "error", err)
	}
	if !connection.networkNegotiated {
		refuse(service.PackageReportRefusedUnnegotiated, nil)
		return
	}
	if report.GetVersion() != network.StatusVersion {
		refuse(service.PackageReportRefusedInvalid, nil)
		return
	}
	receivedAt := time.Now()
	observedAt := receivedAt
	if report.GetObservedAtUnixMs() > 0 {
		observedAt = time.UnixMilli(report.GetObservedAtUnixMs())
	}
	if observedAt.After(receivedAt.Add(time.Minute)) {
		refuse(service.PackageReportRefusedFuture, nil)
		return
	}
	status, err := network.DecodeStatus(report.GetPayloadJson(), node.String())
	if err != nil {
		refuse(service.PackageReportRefusedBadPayload, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), forwardHelloTimeout)
	defer cancel()
	if err := networkmodel.New(databaseForAgentChecks()).RecordStatus(ctx, node, status, observedAt); err != nil {
		agentPackageReportMetrics.result(packageReportUnrecorded)
		slog.Warn("failed to record agent network status", "component", "agent-control", "node", node.String(), "error", err)
		return
	}
	agentPackageReportMetrics.result(packageReportAccepted)
}

var (
	networkPlanOnce     sync.Once
	networkPlanMu       sync.Mutex
	networkPlanManagers = map[*AgentControlManager]string{}
)

// followNetworkPlans makes writes that move a device's plan generation push
// the device's configuration on the managers' sessions.
func followNetworkPlans(proxy, forward *AgentControlManager) {
	networkPlanMu.Lock()
	if proxy != nil {
		networkPlanManagers[proxy] = agentcontrol.NodeKindProxy
	}
	if forward != nil {
		networkPlanManagers[forward] = agentcontrol.NodeKindForward
	}
	networkPlanMu.Unlock()
	networkPlanOnce.Do(func() { networkmodel.OnPlanChange(pushNetworkPlans) })
}

// pushNetworkPlans pushes the configuration of every changed device whose
// session negotiated network.v1, each in its own goroutine.
func pushNetworkPlans(nodes []agentcontrol.AgentNode) {
	networkPlanMu.Lock()
	var targets []*AgentControlConnection
	var targetNodes []agentcontrol.AgentNode
	for manager, kind := range networkPlanManagers {
		for _, node := range nodes {
			if node.Kind != kind {
				continue
			}
			manager.mu.RLock()
			connection := manager.connections[node.ID]
			manager.mu.RUnlock()
			if connection != nil && connection.configNegotiated && connection.networkNegotiated {
				targets = append(targets, connection)
				targetNodes = append(targetNodes, node)
			}
		}
	}
	networkPlanMu.Unlock()
	for i, connection := range targets {
		go func(connection *AgentControlConnection, node agentcontrol.AgentNode) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(connection.stream.Context()), forwardHelloTimeout)
			defer cancel()
			if err := pushDesiredConfig(ctx, connection, node, configTriggerNetwork); err != nil {
				slog.Debug("agent network: the configuration push failed", "component", "agent-control", "node", node.String(), "error", err)
			}
		}(connection, targetNodes[i])
	}
}
