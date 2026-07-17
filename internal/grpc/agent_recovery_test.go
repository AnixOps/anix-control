package grpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/v3/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// terminalReplayStream models the Agent's completed-operation cache: an
// exact operation identity is acknowledged and immediately returned as a
// terminal observation. It is intentionally wired through the real
// AgentControlManager and KernelOperationBridge rather than a bridge stub.
type terminalReplayStream struct {
	grpc.ServerStream
	manager    *AgentControlManager
	connection *AgentControlConnection
	mu         sync.Mutex
	sent       []*agentv1pb.ControlToAgent
}

func (s *terminalReplayStream) Send(message *agentv1pb.ControlToAgent) error {
	s.mu.Lock()
	s.sent = append(s.sent, message)
	s.mu.Unlock()
	desired := message.GetDesiredOperation()
	if desired == nil {
		return nil
	}
	if s.manager == nil || s.connection == nil {
		return errors.New("terminal replay stream is not connected")
	}
	if err := s.manager.resolveAck(s.connection, &agentv1pb.OperationAck{
		OperationId:      desired.OperationId,
		Accepted:         true,
		AcceptedAtUnixMs: time.Now().UnixMilli(),
		SessionId:        s.connection.SessionID,
		Revision:         desired.Revision,
	}); err != nil {
		return err
	}
	return s.manager.recordObserved(s.connection, &agentv1pb.ObservedState{
		OperationId:      desired.OperationId,
		Revision:         desired.Revision,
		Phase:            agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED,
		ObservedAtUnixMs: time.Now().UnixMilli(),
		SessionId:        s.connection.SessionID,
	})
}

func (s *terminalReplayStream) Recv() (*agentv1pb.AgentToControl, error) {
	return nil, io.EOF
}

func TestKernelOperationBridgeReplaysSameRevisionThroughManagerAndAgentTerminal(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "manager-agent-replay-node", Host: "127.0.0.20", APIKey: "manager-agent-replay-key"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "8e0d5c90-67d9-4d77-bf8b-58b4f4e2a9df", IdempotencyKey: "manager-agent-replay-1", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	// Simulate a Control restart after the operation was sent but before its
	// receipt/terminal observation was persisted.
	dispatchedAt := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{
		"state": "running", "session_id": "control-before-restart", "dispatched_at": dispatchedAt,
	}).Error)

	manager := NewAgentControlManager()
	nodeID := uint32(node.ID)
	manager.reconcileDesiredRevision(nodeID, uint64(operation.Revision))
	connection := &AgentControlConnection{
		NodeID:       nodeID,
		SessionID:    "control-after-restart",
		Capabilities: []*agentv1pb.Capability{{Name: "plugin.health", Version: "v1"}},
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
	}
	stream := &terminalReplayStream{manager: manager, connection: connection}
	connection.stream = stream
	manager.register(connection)

	bridge, err := NewKernelOperationBridge(db, manager)
	require.NoError(t, err)
	count, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)

	stream.mu.Lock()
	require.Len(t, stream.sent, 1)
	replayed := stream.sent[0].GetDesiredOperation()
	stream.mu.Unlock()
	require.NotNil(t, replayed)
	require.Equal(t, operation.ID, replayed.OperationId)
	require.Equal(t, uint64(operation.Revision), replayed.Revision)

	var stored model.KernelOperation
	require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
	require.Equal(t, "succeeded", stored.State)

	// A normal caller cannot claim the observed revision for a different
	// operation identity. Only the durable recovery entry point can do so.
	_, err = manager.DispatchOperation(context.Background(), nodeID, &agentv1pb.DesiredOperation{
		OperationId: "different-operation-at-old-revision", Kind: "plugin.health", Revision: uint64(operation.Revision),
	})
	require.ErrorContains(t, err, "not newer")
}
