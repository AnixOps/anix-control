package grpc

import (
	"context"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// grpcCaller is who the authentication interceptors authenticated a call
// as: a node (its own API key or its agent client certificate) or an
// administrator (the global grpc.api_token or an administrator's JWT).
type grpcCaller struct {
	// node is the authenticated node. Proxy and forward node ids overlap,
	// so the kind is part of the caller.
	node  agentcontrol.AgentNode
	admin bool
}

type grpcCallerKey struct{}

// withNodeCaller records that the call is authenticated as the proxy node
// (v2_node) nodeID: its requests may name only that node.
func withNodeCaller(ctx context.Context, nodeID uint32) context.Context {
	return context.WithValue(ctx, grpcCallerKey{}, grpcCaller{node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: nodeID}})
}

// withAdminCaller records that the call is authenticated as an
// administrator: it may act for any node.
func withAdminCaller(ctx context.Context) context.Context {
	return context.WithValue(ctx, grpcCallerKey{}, grpcCaller{admin: true})
}

// requireCallerNode tells whether the call may act for the proxy node
// (v2_node) a request names. A node may act only for itself
// (PermissionDenied otherwise; a forward node never, even with the same
// numeric id) and an administrator for any node. A call without an
// authenticated caller is refused: every node-scoped handler is behind the
// authentication interceptors.
func requireCallerNode(ctx context.Context, nodeID uint32) error {
	caller, found := ctx.Value(grpcCallerKey{}).(grpcCaller)
	switch {
	case !found:
		return status.Error(codes.Unauthenticated, "call is not authenticated")
	case caller.admin:
		return nil
	case caller.node.Kind != agentcontrol.NodeKindProxy || caller.node.ID != nodeID:
		return status.Error(codes.PermissionDenied, "request node_id does not match the authenticated node")
	}
	return nil
}
