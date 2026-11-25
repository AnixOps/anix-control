package grpc

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

// The intranet mesh on the Agent Control stream (mesh.v1;
// docs/design/mesh-regions/agent-contract.md, internal/meshregion):
//
//   - Negotiation. While the feature "mesh" runs, the HelloAck lists
//     mesh.v1 for a proxy or forward node whose Hello lists it, and
//     package-reports.v1 then for a forward node too.
//   - Configuration. The node's configuration document carries the member
//     "mesh" while the node is in an enabled region, negotiated or not. A
//     region or membership change pushes the configuration of every node
//     it may touch at once (pushMesh); the minute refresh catches anything
//     missed.
//   - Status. A PackageReport with plugin_id "mesh" and kind "mesh.status"
//     is accepted only on a session that negotiated mesh.v1, at version
//     v1, and recorded by meshregion.RecordStatus.

// configTriggerMesh: a mesh region or membership changed.
const configTriggerMesh = "mesh"

// servesMesh tells whether the HelloAck advertises mesh.v1.
func servesMesh(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return features.Enabled(meshregion.Feature) &&
		(node.Kind == agentcontrol.NodeKindProxy || node.Kind == agentcontrol.NodeKindForward) &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityMesh, agentcontrol.CapabilityVersionV1)
}

// handleMeshStatus records a mesh status. Like every package report it is
// never acknowledged, and a refusal leaves the stream open.
func handleMeshStatus(connection *AgentControlConnection, node agentcontrol.AgentNode, report *agentv1pb.PackageReport) {
	refuse := func(reason string, err error) {
		agentPackageReportMetrics.refused(reason)
		slog.Warn("refused agent mesh status", "component", "agent-control", "node", node.String(), "reason", reason, "error", err)
	}
	switch {
	case !connection.meshNegotiated:
		refuse(service.PackageReportRefusedUnnegotiated, nil)
		return
	case report.GetVersion() != meshregion.StatusVersion:
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
	ctx, cancel := context.WithTimeout(context.Background(), forwardHelloTimeout)
	defer cancel()
	if err := meshregion.RecordStatus(ctx, databaseForAgentChecks(), node.String(), report.GetPayloadJson(), observedAt, receivedAt); err != nil {
		if errors.Is(err, meshregion.ErrInvalid) {
			refuse(service.PackageReportRefusedBadPayload, err)
			return
		}
		agentPackageReportMetrics.result(packageReportUnrecorded)
		slog.Warn("failed to record agent mesh status", "component", "agent-control", "node", node.String(), "error", err)
		return
	}
	agentPackageReportMetrics.result(packageReportAccepted)
}

var (
	meshOnce     sync.Once
	meshMu       sync.Mutex
	meshManagers = map[*AgentControlManager]string{}
)

// followMesh makes mesh changes push the touched nodes' configurations on
// the managers' sessions.
func followMesh(proxy, forward *AgentControlManager) {
	meshMu.Lock()
	if proxy != nil {
		meshManagers[proxy] = agentcontrol.NodeKindProxy
	}
	if forward != nil {
		meshManagers[forward] = agentcontrol.NodeKindForward
	}
	meshMu.Unlock()
	meshOnce.Do(func() { meshregion.OnChange(pushMesh) })
}

// pushMesh pushes the configuration of every node on a session that
// negotiated config.v1, each in its own goroutine.
func pushMesh(nodes []agentcontrol.AgentNode) {
	meshMu.Lock()
	var targets []*AgentControlConnection
	var targetNodes []agentcontrol.AgentNode
	for manager, kind := range meshManagers {
		for _, node := range nodes {
			if node.Kind != kind {
				continue
			}
			manager.mu.RLock()
			connection := manager.connections[node.ID]
			manager.mu.RUnlock()
			if connection != nil && connection.configNegotiated {
				targets = append(targets, connection)
				targetNodes = append(targetNodes, node)
			}
		}
	}
	meshMu.Unlock()
	for i, connection := range targets {
		go func(connection *AgentControlConnection, node agentcontrol.AgentNode) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(connection.stream.Context()), forwardHelloTimeout)
			defer cancel()
			if err := pushDesiredConfig(ctx, connection, node, configTriggerMesh); err != nil {
				slog.Debug("agent mesh: the configuration push failed", "component", "agent-control", "node", node.String(), "error", err)
			}
		}(connection, targetNodes[i])
	}
}
