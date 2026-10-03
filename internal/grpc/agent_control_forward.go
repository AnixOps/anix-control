package grpc

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/wire"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// Forwarding on the Agent Control stream (forward.v1; forward-sdk.md
// section 8, PROTOCOL.md "Forwarding", sdk/forward/wire):
//
//   - Negotiation. The HelloAck lists forward.v1 for a proxy or forward
//     node when the Hello lists it with a valid node_capabilities attribute
//     and Control serves the session config.v1, which carries the state. A
//     forward node is then offered package-reports.v1 too, for its reports.
//   - Hello. kernelforward.RecordHello records the node's capabilities and
//     its negotiated flag (and clears the flag for a Hello without
//     forward.v1), replanning when the capabilities changed, before the
//     Hello's configuration push: the first snapshot already carries the
//     node's state.
//   - State. The node's configuration is anixops.nodeconfig/v2 while its
//     flag is set (kernelforward.NodeConfigMember). Every plan that moves a
//     node's generation pushes the node's configuration at once
//     (pushForwardStates); the minute refresh catches anything missed.
//   - Heartbeat. A forward node's session that negotiated forward.v1 is
//     asked to heartbeat every 60 seconds (forward-sdk.md section 8.4).
//   - Reports. A PackageReport with plugin_id "forward" and kind
//     "forward.report" is the node's NodeForwardReport: accepted only on a
//     session that negotiated forward.v1, never authorized by a plugin
//     release, checked by wire.DecodeReport for the stream's node, stored as
//     the node's latest and metered by kernelforward.RecordReport.

// forwardHeartbeatIntervalSeconds is the heartbeat a forward node's
// forward.v1 session is asked for (forward-sdk.md section 8.4: one
// long-lived stream and a low-frequency heartbeat, decided by default with
// F3a). A certificate revoked while the stream is open ends it at the next
// heartbeat, so within a minute on these sessions.
const forwardHeartbeatIntervalSeconds = uint32(60)

// forwardHelloTimeout bounds recording a Hello, its plan included.
const forwardHelloTimeout = 30 * time.Second

// forwardCapabilities decodes the node capabilities of a Hello that lists
// forward.v1: nil when it does not, or when its attribute is malformed (the
// session then does not negotiate forward.v1).
func forwardCapabilities(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) (*wireCapabilities, bool) {
	caps, listed, err := wire.NodeCapabilitiesFromHello(agent, node.String())
	if !listed {
		return nil, false
	}
	if err != nil {
		return &wireCapabilities{err: err}, true
	}
	return &wireCapabilities{caps: caps}, true
}

// servesForward tells whether the HelloAck advertises forward.v1.
func (s *AgentControlGRPCServer) servesForward(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	if !s.servesConfig(node, agent) {
		return false
	}
	caps, listed := forwardCapabilities(node, agent)
	return listed && caps.err == nil
}

// heartbeatInterval is the interval the HelloAck asks the session for.
func (s *AgentControlGRPCServer) heartbeatInterval(node agentcontrol.AgentNode, forwardNegotiated bool) uint32 {
	if node.Kind == agentcontrol.NodeKindForward && forwardNegotiated {
		return forwardHeartbeatIntervalSeconds
	}
	return s.heartbeatIntervalSeconds
}

// recordForwardHello records the session's forwarding capabilities, or that
// it has none. Errors are logged: the session keeps the node's last
// recorded state.
func (s *AgentControlGRPCServer) recordForwardHello(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode, hello *agentv1pb.Hello) {
	db := databaseForAgentChecks()
	if db == nil {
		return
	}
	// The Hello is recorded whole even when the stream ends meanwhile: a
	// plan it starts runs to its commit or rollback.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forwardHelloTimeout)
	defer cancel()
	caps, listed := forwardCapabilities(node, hello.GetCapabilities())
	if listed && caps.err != nil {
		slog.Warn("agent forward: the Hello's node capabilities were refused; forward.v1 is not served", "component", "agent-control",
			"node", node.String(), "error", caps.err)
	}
	var recorded *forwardv1.NodeCapabilities
	if connection.forwardNegotiated && caps != nil {
		recorded = caps.caps
	}
	_, _, err := kernelforward.New(db).RecordHello(ctx, node, recorded, hello.GetAgentVersion())
	if err != nil {
		slog.Warn("agent forward: the Hello could not be recorded", "component", "agent-control", "node", node.String(), "error", err)
	}
}

// handleForwardReport records a forward report. Like every package report
// it is never acknowledged, and a refusal leaves the stream open.
func (s *AgentControlGRPCServer) handleForwardReport(connection *AgentControlConnection, node agentcontrol.AgentNode, report *agentv1pb.PackageReport) {
	refuse := func(reason string, err error) {
		agentPackageReportMetrics.refused(reason)
		slog.Warn("refused agent forward report", "component", "agent-control", "node", node.String(), "reason", reason, "error", err)
	}
	if !connection.forwardNegotiated {
		refuse(service.PackageReportRefusedUnnegotiated, nil)
		return
	}
	if report.GetVersion() != wire.ReportVersion {
		refuse(service.PackageReportRefusedInvalid, nil)
		return
	}
	if err := agentcontrol.ValidatePackageReportPayloadSize(report.GetPayloadJson()); err != nil {
		refuse(service.PackageReportRefusedOversize, err)
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
	decoded, err := wire.DecodeReport(report.GetPayloadJson(), node.String())
	if err != nil {
		refuse(service.PackageReportRefusedBadPayload, err)
		return
	}
	result, err := kernelforward.New(databaseForAgentChecks()).RecordReport(context.Background(), node, decoded, observedAt, receivedAt)
	switch {
	case err == nil && result.Stored:
		agentPackageReportMetrics.result(packageReportAccepted)
	case err == nil:
		agentPackageReportMetrics.result(packageReportSuperseded)
	case isInvalidForwardReport(err):
		refuse(service.PackageReportRefusedBadPayload, err)
	default:
		agentPackageReportMetrics.result(packageReportUnrecorded)
		slog.Warn("failed to record agent forward report", "component", "agent-control", "node", node.String(), "error", err)
	}
}

// wireCapabilities is a Hello's decoded node capabilities, or why they
// were refused.
type wireCapabilities struct {
	caps *forwardv1.NodeCapabilities
	err  error
}

func isInvalidForwardReport(err error) bool { return errors.Is(err, kernelforward.ErrInvalidReport) }

// forwardStateManagers are the managers whose sessions a plan's state
// change reaches: every AgentControlGRPCServer's.
var (
	forwardStateOnce     sync.Once
	forwardStateMu       sync.Mutex
	forwardStateManagers = map[*AgentControlManager]string{}
)

// followForwardStates makes plans that move a node's generation push the
// node's configuration on the managers' sessions.
func followForwardStates(proxy, forward *AgentControlManager) {
	forwardStateMu.Lock()
	if proxy != nil {
		forwardStateManagers[proxy] = agentcontrol.NodeKindProxy
	}
	if forward != nil {
		forwardStateManagers[forward] = agentcontrol.NodeKindForward
	}
	forwardStateMu.Unlock()
	forwardStateOnce.Do(func() { kernelforward.OnStateChange(pushForwardStates) })
}

// pushForwardStates pushes the configuration of every changed node whose
// session negotiated forward.v1, each in its own goroutine so a slow stream
// never holds up the plan's caller. sendConfig never sends a revision older
// than one the session was sent, so concurrent pushes keep the newest.
func pushForwardStates(nodes []agentcontrol.AgentNode) {
	forwardStateMu.Lock()
	var targets []*AgentControlConnection
	var targetNodes []agentcontrol.AgentNode
	for manager, kind := range forwardStateManagers {
		for _, node := range nodes {
			if node.Kind != kind {
				continue
			}
			manager.mu.RLock()
			connection := manager.connections[node.ID]
			manager.mu.RUnlock()
			if connection != nil && connection.configNegotiated && connection.forwardNegotiated {
				targets = append(targets, connection)
				targetNodes = append(targetNodes, node)
			}
		}
	}
	forwardStateMu.Unlock()
	for i, connection := range targets {
		go func(connection *AgentControlConnection, node agentcontrol.AgentNode) {
			// The build is not cut off halfway when the stream ends; the
			// send then fails and the next session's Hello reconciles.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(connection.stream.Context()), forwardHelloTimeout)
			defer cancel()
			if err := pushDesiredConfig(ctx, connection, node, configTriggerForward); err != nil {
				slog.Debug("agent forward: the configuration push failed", "component", "agent-control", "node", node.String(), "error", err)
			}
		}(connection, targetNodes[i])
	}
}
