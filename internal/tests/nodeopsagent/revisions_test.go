package nodeopsagent

import (
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedRevision is the node's durable revision cursor
// (v3_kernel_node_operation_revision).
func (f *fixture) storedRevision() int64 {
	f.t.Helper()
	var cursor model.NodeOperationRevision
	require.NoError(f.t, f.db.First(&cursor, "node_id = ?", f.proxy.ID).Error)
	return cursor.DesiredRevision
}

// pingRevision runs one agent.ping one-off operation to its end and
// returns the revision it went out at.
func pingRevision(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, f *fixture, operationID string) uint64 {
	t.Helper()
	accepted, err := submit(t, client, "agent.op:"+operationID, agentOperationSpec(f.proxy.ID, "agent.ping", operationID, 5), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
	require.NoError(t, err)
	ended := awaitState(t, client, accepted.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
	return ended.GetResult().GetAgentOperation().GetRevision()
}

// One-off stream operations allocate from the node's durable revision
// cursor, and an Agent reconnecting with a higher revision raises it.
func TestOneOffOperationsUseTheNodeRevisionCursor(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{})
		client := f.client()

		first := pingRevision(t, client, f, "revision-ping-1")
		second := pingRevision(t, client, f, "revision-ping-2")
		require.Greater(t, second, first)
		assert.Equal(t, int64(second), f.storedRevision(), "one-off operations allocate from the durable cursor") // #nosec G115 -- a test revision.

		// A reconnect reporting a revision above the cursor (an Agent that
		// outlived a Control restart) raises it.
		agent.Disconnect()
		f.proxyAgent(fakeagent.Script{ObservedRevision: 40})
		assert.Equal(t, int64(40), f.storedRevision())
		assert.Equal(t, uint64(41), pingRevision(t, client, f, "revision-ping-3"))
	})
}
