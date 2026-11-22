package grpc

import (
	"context"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// revisionTestAgent is one Agent session on the test environment's
// listener that acknowledges every desired operation it receives.
type revisionTestAgent struct {
	t         *testing.T
	stream    agentv1pb.AgentControlService_ControlStreamClient
	nodeID    uint32
	sessionID string
	received  chan *agentv1pb.DesiredOperation
}

func connectRevisionTestAgent(t *testing.T, environment *agentControlTestEnvironment, ctx context.Context, revision uint64, capabilities ...string) *revisionTestAgent {
	t.Helper()
	stream, err := agentv1pb.NewAgentControlServiceClient(environment.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(uint32(environment.node.ID))
	hello.Revision = revision
	for _, capability := range capabilities {
		hello.GetHello().Capabilities = append(hello.GetHello().Capabilities, &agentv1pb.Capability{Name: capability, Version: "v1"})
	}
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHelloAck())
	agent := &revisionTestAgent{
		t: t, stream: stream, nodeID: uint32(environment.node.ID),
		sessionID: message.GetHelloAck().GetSessionId(), received: make(chan *agentv1pb.DesiredOperation, 16),
	}
	go agent.serve()
	return agent
}

func (a *revisionTestAgent) serve() {
	for {
		message, err := a.stream.Recv()
		if err != nil {
			return
		}
		desired := message.GetDesiredOperation()
		if desired == nil {
			continue
		}
		a.received <- desired
		_ = a.stream.Send(&agentv1pb.AgentToControl{
			RequestId: "ack-" + desired.GetOperationId(), NodeId: a.nodeID, Revision: desired.GetRevision(), SentAtUnixMs: time.Now().UnixMilli(),
			Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
				OperationId: desired.GetOperationId(), Accepted: true, AcceptedAtUnixMs: time.Now().UnixMilli(),
				SessionId: a.sessionID, Revision: desired.GetRevision(),
			}},
		})
	}
}

// newRevisionTestEnvironment is the Agent Control test environment with
// the kernel schema and the durable revision store, as the server wires it.
func newRevisionTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newAgentControlTestEnvironment(t)
	require.NoError(t, service.MigrateKernelModelsForTest(database.GetDB()))
	environment.manager.UseRevisionStore(NewDatabaseRevisionStore(database.GetDB))
	return environment
}

func storedNodeRevision(t *testing.T, nodeID uint) int64 {
	t.Helper()
	var cursor model.NodeOperationRevision
	require.NoError(t, database.GetDB().First(&cursor, "node_id = ?", nodeID).Error)
	return cursor.DesiredRevision
}

// Concurrent one-off operations on one node reach the in-memory guard in
// allocation order: none is refused as not newer than another.
func TestAgentControlConcurrentOneOffOperationsAllocateInOrder(t *testing.T) {
	environment := newRevisionTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 10*time.Second)
	defer cancel()
	nodeID := uint32(environment.node.ID)
	agent := connectRevisionTestAgent(t, environment, ctx, 0)

	const operations = 8
	errs := make(chan error, operations)
	for index := range operations {
		go func() {
			_, err := environment.manager.DispatchOperation(ctx, nodeID, &agentv1pb.DesiredOperation{
				OperationId: "concurrent-" + string(rune('a'+index)), Kind: "agent.ping",
			})
			errs <- err
		}()
	}
	seen := map[uint64]bool{}
	for range operations {
		select {
		case desired := <-agent.received:
			assert.False(t, seen[desired.GetRevision()], "revision %d sent twice", desired.GetRevision())
			seen[desired.GetRevision()] = true
		case <-ctx.Done():
			t.Fatal("timed out waiting for concurrent operations")
		}
	}
	for range operations {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, int64(operations), storedNodeRevision(t, environment.node.ID))
}

// Without a database the store reports no durable allocator and the
// manager keeps its in-memory counter.
func TestDatabaseRevisionStoreWithoutDatabaseFallsBack(t *testing.T) {
	store := NewDatabaseRevisionStore(func() *gorm.DB { return nil })
	revision, ok, err := store.AllocateRevision(context.Background(), 1, 4)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Zero(t, revision)
	require.NoError(t, store.RaiseRevision(context.Background(), 1, 9))
}
