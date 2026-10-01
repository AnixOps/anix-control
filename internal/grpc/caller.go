package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// grpcCaller is who the authentication interceptors authenticated a call
// as: a node (its own API key) or an administrator (the global
// grpc.api_token or an administrator's JWT).
type grpcCaller struct {
	nodeID uint32
	admin  bool
}

type grpcCallerKey struct{}

// withNodeCaller records that the call is authenticated as the node: its
// requests may name only that node.
func withNodeCaller(ctx context.Context, nodeID uint32) context.Context {
	return context.WithValue(ctx, grpcCallerKey{}, grpcCaller{nodeID: nodeID})
}

// withAdminCaller records that the call is authenticated as an
// administrator: it may act for any node.
func withAdminCaller(ctx context.Context) context.Context {
	return context.WithValue(ctx, grpcCallerKey{}, grpcCaller{admin: true})
}

// requireCallerNode tells whether the call may act for the node a request
// names. A node may act only for itself (PermissionDenied otherwise) and an
// administrator for any node. A call without an authenticated caller is
// refused: every node-scoped handler is behind the authentication
// interceptors.
func requireCallerNode(ctx context.Context, nodeID uint32) error {
	caller, found := ctx.Value(grpcCallerKey{}).(grpcCaller)
	switch {
	case !found:
		return status.Error(codes.Unauthenticated, "call is not authenticated")
	case caller.admin:
		return nil
	case caller.nodeID != nodeID:
		return status.Error(codes.PermissionDenied, "request node_id does not match the authenticated node")
	}
	return nil
}
