// Package edgegrpc runs the node gRPC through an edge proxy
// (grpc.edge_proxies, docs/design/edge-grpc): the proxy vouches for the
// node's certificate in a header, and Control believes that header only
// from the proxy.
package edgegrpc

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The node of these tests: an id of its own, apart from the other tests'
// nodes (the Agent Control managers are the process's).
const (
	edgeNodeID  = 4100
	edgeNodeKey = "edge-test-api-key"
)

func TestEdgeProxyVouchesForTheNodeCertificate(t *testing.T) {
	models := []any{&model.Node{}, &model.OperationLog{}}
	packagecompat.ForEachKernelDatabase(t, models, func(t *testing.T, db *gorm.DB) {
		cache.InitMemory()
		require.NoError(t, service.MigrateKernelModelsForTest(db))
		require.NoError(t, db.Create(&model.Node{ID: edgeNodeID, Name: "edge-node", Host: "198.51.100.1", Port: 443,
			APIKey: edgeNodeKey, APIKeyHash: fakeagent.APIKeyHash(edgeNodeKey), Status: model.NodeStatusOnline}).Error)

		control := fakeagent.StartControl(t, db)
		node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: edgeNodeID}
		// The node enrolls over the TLS listener, as its Agent does at its
		// first start; the certificate then travels through the edge.
		certificate := control.Enroll(t, node)
		edge := fakeagent.StartEdge(t, control)

		t.Run("the proxy's certificate opens the stream", func(t *testing.T) {
			agent := fakeagent.ConnectWithCertificate(t, edge.DialEdge(t, certificate), node, fakeagent.Script{})
			assert.NotNil(t, agent.HelloAck)
		})

		t.Run("the direct TLS listener still takes the certificate in the handshake", func(t *testing.T) {
			agent := fakeagent.ConnectWithCertificate(t, control.Dial(t, certificate), node, fakeagent.Script{})
			assert.NotNil(t, agent.HelloAck)
		})

		t.Run("without a certificate the edge refuses the stream", func(t *testing.T) {
			requireRefused(t, edge.DialEdge(t, nil), node)
		})

		t.Run("a header on a direct TLS connection is not believed", func(t *testing.T) {
			// The client presents no certificate in the handshake; the header
			// names one, but the peer is not an edge proxy.
			requireRefused(t, control.DialForgedHeader(t, certificate), node)
		})
	})
}

// requireRefused opens a control stream for node and expects the Control to
// refuse it: no HelloAck arrives.
func requireRefused(t *testing.T, conn *grpc.ClientConn, node agentcontrol.AgentNode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	require.NoError(t, err)
	_ = stream.Send(&agentv1pb.AgentToControl{
		RequestId: "hello", NodeId: node.ID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Hello{Hello: &agentv1pb.Hello{
			Protocol: agentcontrol.ProtocolV1, AgentVersion: "fake-agent", InstanceId: "fake-" + node.String(),
		}},
	})
	message, err := stream.Recv()
	require.Error(t, err, "the stream is refused")
	// A refusal is an answer with a status; a deadline or a reset is not.
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "refused as unauthenticated: %v", err)
	assert.Nil(t, message.GetHelloAck(), "no HelloAck for an unauthenticated stream")
}
