package grpc

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestRequireCallerNodeBindsTheNodeKind: proxy and forward node ids
// overlap, so a forward node never acts for the proxy node with its id.
func TestRequireCallerNodeBindsTheNodeKind(t *testing.T) {
	ctx := context.Background()
	forward := context.WithValue(ctx, grpcCallerKey{}, grpcCaller{node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 5}})
	assert.Equal(t, codes.PermissionDenied, status.Code(requireCallerNode(forward, 5)))

	proxy := withNodeCaller(ctx, 5)
	assert.NoError(t, requireCallerNode(proxy, 5))
	assert.Equal(t, codes.PermissionDenied, status.Code(requireCallerNode(proxy, 6)))
	assert.NoError(t, requireCallerNode(withAdminCaller(ctx), 6))
	assert.Equal(t, codes.Unauthenticated, status.Code(requireCallerNode(ctx, 5)))
}
