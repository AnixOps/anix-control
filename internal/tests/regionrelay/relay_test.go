// Package regionrelay runs a node's control stream through the region relay
// (cmd/anix-relay, docs/design/region-relay): the relay copies the TCP
// connection byte for byte behind a PROXY v2 header, and TLS with the node's
// client certificate runs end to end between the node and Control.
package regionrelay

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/model"
	relay "github.com/AnixOps/anix-control/v5/internal/regionrelay"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// The node of these tests: an id of its own, apart from the other tests'
// nodes (the Agent Control managers are the process's).
const relayNodeID = 4200

func startRelay(t *testing.T, upstream string, proxyProtocol bool) string {
	t.Helper()
	forward, err := relay.ListenForward(relay.ForwardConfig{
		Name: "control", Listen: "127.0.0.1:0", Upstreams: []string{upstream}, ProxyProtocol: proxyProtocol,
		MaxConnections: 16, DialTimeout: 5 * time.Second, KeepAlive: 30 * time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	go func() { _ = forward.Serve() }()
	t.Cleanup(func() { _ = forward.Close() })
	return forward.Addr().String()
}

func TestRelayForwardsTheControlStreamEndToEnd(t *testing.T) {
	models := []any{&model.Node{}, &model.OperationLog{}}
	packagecompat.ForEachKernelDatabase(t, models, func(t *testing.T, db *gorm.DB) {
		cache.InitMemory()
		require.NoError(t, service.MigrateKernelModelsForTest(db))
		require.NoError(t, db.Create(&model.Node{ID: relayNodeID, Name: "relay-node", Host: "198.51.100.2", Port: 443,
			Status: model.NodeStatusOnline}).Error)

		control := fakeagent.StartControl(t, db)
		node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: relayNodeID}
		certificate := control.Enroll(t, node)
		relayed := fakeagent.StartRelayed(t, control)

		t.Run("the node's certificate opens the stream through the relay", func(t *testing.T) {
			address := startRelay(t, relayed.Addr, true)
			agent := fakeagent.ConnectWithCertificate(t, relayed.DialVia(t, address, certificate), node, fakeagent.Script{})
			assert.NotNil(t, agent.HelloAck)
		})

		t.Run("a relay source without the PROXY header is closed", func(t *testing.T) {
			address := startRelay(t, relayed.Addr, false)
			requireNoStream(t, relayed.DialVia(t, address, certificate), node)
		})

		t.Run("a direct connection from a relay source without the header is closed", func(t *testing.T) {
			requireNoStream(t, relayed.Dial(t, certificate), node)
		})
	})
}

// requireNoStream expects the connection itself to fail: the listener closes
// it before TLS, so no stream and no HelloAck exist.
func requireNoStream(t *testing.T, conn *grpc.ClientConn, node agentcontrol.AgentNode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	if err != nil {
		return
	}
	_ = stream.Send(&agentv1pb.AgentToControl{
		RequestId: "hello", NodeId: node.ID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Hello{Hello: &agentv1pb.Hello{
			Protocol: agentcontrol.ProtocolV1, AgentVersion: "fake-agent", InstanceId: "fake-" + node.String(),
		}},
	})
	message, err := stream.Recv()
	require.Error(t, err)
	assert.Nil(t, message.GetHelloAck())
}
