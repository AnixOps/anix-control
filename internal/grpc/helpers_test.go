package grpc

import (
	"testing"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/assert"
)

func TestAgentControlManagerRemovePendingOnlyRemovesMatchingWaiter(t *testing.T) {
	manager := NewAgentControlManager()
	key := agentOperationKey{nodeID: 3, operationID: "op-1"}
	current := make(chan *agentv1pb.OperationAck, 1)
	stale := make(chan *agentv1pb.OperationAck, 1)
	manager.pending[key] = current

	manager.removePending(key, stale)
	assert.Equal(t, current, manager.pending[key], "a stale waiter must not remove the current one")

	manager.removePending(key, current)
	_, exists := manager.pending[key]
	assert.False(t, exists)
}
