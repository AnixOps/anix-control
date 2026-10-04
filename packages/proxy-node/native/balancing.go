package native

import (
	"context"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// A load balancer's statistics and health check concern the forward nodes,
// which the package reads through kapi_forward_node_v1 (every column but
// the node's API token) and which the kernel checks
// (CheckEndpoints with record_status). Both have always looked at every
// forward node, whatever the load balancer's group.
const forwardNodeView = "kapi_forward_node_v1"

// ForwardNode is the part of a kapi_forward_node_v1 row the statistics
// read.
type ForwardNode struct {
	ID      uint `gorm:"primaryKey"`
	Status  int
	Latency int
	Load    float64
}

// TableName is the kernel view.
func (ForwardNode) TableName() string { return forwardNodeView }

// forwardNodeOnline is the kernel's model.ForwardNodeStatusOnline.
const forwardNodeOnline = 1

// LoadBalancerStats is GET /api/v2/admin/loadbalancers/:id/stats: how many
// forward nodes there are and are online, and the online ones' average
// load and latency.
func (s *Service) LoadBalancerStats(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	lb, err := s.find(ctx, id)
	if err != nil {
		return s.panelError("load balancer not found")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var nodes []ForwardNode
	if err := db.Find(&nodes).Error; err != nil {
		return s.panelError(err.Error())
	}
	online, avgLatency, totalLoad := 0, 0, float64(0)
	for _, node := range nodes {
		if node.Status == forwardNodeOnline {
			online++
			avgLatency += node.Latency
			totalLoad += node.Load
		}
	}
	if online > 0 {
		avgLatency /= online
		totalLoad /= float64(online)
	}
	return s.panel(map[string]any{
		"total_nodes": len(nodes), "online_nodes": online, "avg_load": totalLoad, "avg_latency": avgLatency, "strategy": lb.Strategy,
	})
}

// RunHealthCheck is POST /api/v2/admin/loadbalancers/:id/check: with the
// load balancer's health checks on, the kernel checks every forward node's
// endpoint and records what it found on the node's row (CheckEndpoints
// with record_status), as the forward node check does.
func (s *Service) RunHealthCheck(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	lb, err := s.find(ctx, id)
	if err != nil {
		return s.panelError("load balancer not found")
	}
	if s.NodeOps == nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if !lb.HealthCheck {
		return s.panel(map[string]any{"message": "health check completed"})
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var ids []uint
	if err := db.Model(&ForwardNode{}).Order("id").Pluck("id", &ids).Error; err != nil {
		return s.panelError(err.Error())
	}
	if len(ids) > 0 {
		nodes := make([]*kernelnodeopsv1.NodeRef, len(ids))
		for i, node := range ids {
			nodes[i] = &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, Id: uint64(node)}
		}
		operation, err := s.submit(ctx, request, s.requestID(request, "diagnose.endpoints:loadbalancer", true),
			&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_CheckEndpoints{CheckEndpoints: &kernelnodeopsv1.CheckEndpoints{
				Nodes: nodes, RecordStatus: true,
			}}},
			kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, maxWait)
		if err != nil {
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
		switch operation.GetError().GetCode() {
		case kernelnodeopsv1.ErrorCode_ERROR_CODE_UNSPECIFIED, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE:
			// Checks that failed and nodes deleted meanwhile are the
			// check's findings, as for the kernel's handler.
		default:
			return s.panelError(operation.GetError().GetMessage())
		}
	}
	return s.panel(map[string]any{"message": "health check completed"})
}
