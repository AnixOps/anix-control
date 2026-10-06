package grpc

import (
	"context"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
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

func (a *revisionTestAgent) next(operationID string) *agentv1pb.DesiredOperation {
	a.t.Helper()
	select {
	case desired := <-a.received:
		require.Equal(a.t, operationID, desired.GetOperationId())
		return desired
	case <-time.After(5 * time.Second):
		a.t.Fatalf("the agent received no operation %q", operationID)
		return nil
	}
}

// newRevisionTestEnvironment is the Agent Control test environment with
// the kernel schema and the durable revision store, as the server wires it.
func newRevisionTestEnvironment(t *testing.T) *agentControlTestEnvironment {
	t.Helper()
	environment := newAgentControlTestEnvironment(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))
	environment.manager.UseRevisionStore(NewDatabaseRevisionStore(database.GetDB))
	return environment
}

func storedNodeRevision(t *testing.T, nodeID uint) int64 {
	t.Helper()
	var cursor model.NodeOperationRevision
	require.NoError(t, database.GetDB().First(&cursor, "node_id = ?", nodeID).Error)
	return cursor.DesiredRevision
}

func createDurableTestOperation(t *testing.T, environment *agentControlTestEnvironment, id string) *model.KernelOperation {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(database.GetDB(), model.KernelOperation{
		ID: id, IdempotencyKey: "revision-test:" + id, NodeID: &environment.node.ID,
		PluginID: "machine-telemetry", TargetVersion: "1.1.0", Kind: "plugin.configure",
		ConfigJSON: `{"interval_seconds":10}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	return operation
}

// One-off stream operations (agent.ping here; agent.diagnostic, the forward
// checks, node.reload and users.reload alike) used to take their revisions
// from the manager's in-memory counter while durable plugin operations took
// the next revision of v3_kernel_node_operation_revision. After two one-off
// operations the durable one was created at revision 1 and refused:
// "revision 1 is not newer than 2". Both kinds now allocate from the cursor.
func TestAgentControlDurableOperationAfterOneOffOperationsIsDispatched(t *testing.T) {
	environment := newRevisionTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 10*time.Second)
	defer cancel()
	nodeID := uint32(environment.node.ID)
	agent := connectRevisionTestAgent(t, environment, ctx, 0, "plugin.configure")

	for index, operationID := range []string{"ping-1", "ping-2"} {
		ack, err := environment.manager.DispatchOperation(ctx, nodeID, &agentv1pb.DesiredOperation{OperationId: operationID, Kind: "agent.ping"})
		require.NoError(t, err)
		require.True(t, ack.GetAccepted())
		desired := agent.next(operationID)
		assert.Equal(t, uint64(index+1), desired.GetRevision())
	}
	assert.Equal(t, int64(2), storedNodeRevision(t, environment.node.ID), "one-off operations allocate from the durable cursor")

	operation := createDurableTestOperation(t, environment, "4c1d8a2e-6f0b-4b8e-9d55-1f0e5a8c7b31")
	require.Equal(t, int64(3), operation.Revision, "the durable operation is newer than the one-off operations")

	bridge, err := NewKernelOperationBridge(database.GetDB(), environment.manager)
	require.NoError(t, err)
	count, err := bridge.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	desired := agent.next(operation.ID)
	assert.Equal(t, uint64(3), desired.GetRevision())
	var stored model.KernelOperation
	require.NoError(t, database.GetDB().First(&stored, "id = ?", operation.ID).Error)
	assert.Equal(t, "running", stored.State)
	assert.Empty(t, stored.LastError)

	// A one-off operation after the durable one is newer still.
	_, err = environment.manager.DispatchOperation(ctx, nodeID, &agentv1pb.DesiredOperation{OperationId: "ping-3", Kind: "agent.ping"})
	require.NoError(t, err)
	assert.Equal(t, uint64(4), agent.next("ping-3").GetRevision())
	assert.Equal(t, int64(4), storedNodeRevision(t, environment.node.ID))
}

// After a Control restart the in-memory counter starts at zero and the
// Agent's last revision can be above the durable cursor. Its Hello raises
// the cursor, so the next durable and one-off revisions are above it; a
// lower Hello revision never lowers the cursor.
func TestAgentControlHelloRaisesTheDurableRevisionCursor(t *testing.T) {
	environment := newRevisionTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 10*time.Second)
	defer cancel()
	nodeID := uint32(environment.node.ID)

	require.NoError(t, service.RaiseNodeOperationRevision(database.GetDB(), environment.node.ID, 3))
	agent := connectRevisionTestAgent(t, environment, ctx, 8, "plugin.configure")
	assert.Equal(t, int64(8), storedNodeRevision(t, environment.node.ID), "the Hello revision raises the cursor")

	operation := createDurableTestOperation(t, environment, "8f2b6c1a-0d3e-4a7f-b5c9-2e6d4f8a1b03")
	require.Equal(t, int64(9), operation.Revision)
	bridge, err := NewKernelOperationBridge(database.GetDB(), environment.manager)
	require.NoError(t, err)
	count, err := bridge.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	assert.Equal(t, uint64(9), agent.next(operation.ID).GetRevision())

	_, err = environment.manager.DispatchOperation(ctx, nodeID, &agentv1pb.DesiredOperation{OperationId: "ping-after-raise", Kind: "agent.ping"})
	require.NoError(t, err)
	assert.Equal(t, uint64(10), agent.next("ping-after-raise").GetRevision())

	// A reconnect reporting an older revision leaves the cursor alone.
	connectRevisionTestAgent(t, environment, ctx, 2, "plugin.configure")
	assert.Equal(t, int64(10), storedNodeRevision(t, environment.node.ID))
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

// A durable operation takes its revision when it is created, a one-off
// operation when it is sent. A one-off operation sent between the two used to
// leave the durable one behind the manager's last revision: it was refused as
// "revision N is not newer than M", stayed dispatching and every retry failed
// the same way. The bridge now renumbers the operation (and the pending ones
// behind it, keeping their order) above the cursor and sends it.
func TestAgentControlDurableOperationCreatedBeforeOneOffOperationIsRenumbered(t *testing.T) {
	environment := newRevisionTestEnvironment(t)
	ctx, cancel := context.WithTimeout(environment.authContext(context.Background()), 10*time.Second)
	defer cancel()
	nodeID := uint32(environment.node.ID)
	agent := connectRevisionTestAgent(t, environment, ctx, 0, "plugin.configure")

	first := createDurableTestOperation(t, environment, "5a7e2c90-3b1d-4f6a-8c24-9d0e1b3a5f71")
	second := createDurableTestOperation(t, environment, "6b8f3da1-4c2e-4a7b-9d35-0e1f2c4b6a82")
	require.Equal(t, int64(1), first.Revision)
	require.Equal(t, int64(2), second.Revision)

	ack, err := environment.manager.DispatchOperation(ctx, nodeID, &agentv1pb.DesiredOperation{OperationId: "ping-between", Kind: "agent.ping"})
	require.NoError(t, err)
	require.True(t, ack.GetAccepted())
	require.Equal(t, uint64(3), agent.next("ping-between").GetRevision(), "the one-off operation is sent after both durable ones were created")

	bridge, err := NewKernelOperationBridge(database.GetDB(), environment.manager)
	require.NoError(t, err)
	count, err := bridge.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "the first durable operation is dispatched")
	assert.Equal(t, uint64(4), agent.next(first.ID).GetRevision(), "it is renumbered above the one-off operation")

	var storedFirst, storedSecond model.KernelOperation
	require.NoError(t, database.GetDB().First(&storedFirst, "id = ?", first.ID).Error)
	require.NoError(t, database.GetDB().First(&storedSecond, "id = ?", second.ID).Error)
	assert.Equal(t, "running", storedFirst.State)
	assert.Equal(t, int64(4), storedFirst.Revision)
	assert.Empty(t, storedFirst.LastError)
	assert.Equal(t, "pending", storedSecond.State, "the second waits for the first")
	assert.Equal(t, int64(5), storedSecond.Revision, "the pending operation behind it keeps its order")
	assert.Equal(t, int64(5), storedNodeRevision(t, environment.node.ID))
}
