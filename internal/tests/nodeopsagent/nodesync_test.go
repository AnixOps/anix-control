package nodeopsagent

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A sync of a node whose agent is on the Control stream stores the desired
// configuration and pushes node.reload through the stream; a repeat with
// the same configuration keeps the revision and pushes only when forced; a
// changed configuration grows the revision and pushes again.
func TestSyncNodeOnTheControlStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{})
		client := f.client()

		first := terminal(t, client, "node.sync:proxy-1:a", syncSpec(f.proxyNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, first.GetState(), first.GetError())
		result := first.GetResult().GetNodeSync()
		require.NotNil(t, result)
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, result.GetChannel())
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, first.GetChannel())
		assert.True(t, result.GetChanged(), "the first build is a change")
		assert.Equal(t, uint64(1), result.GetConfigRevision())
		assert.Len(t, result.GetConfigHash(), 64)
		assert.NotEmpty(t, result.GetAgentOperationId())
		assert.Equal(t, uint64(1), result.GetRevision(), "the stream's first revision")
		assert.Equal(t, uint64(1), first.GetNodeRevision())
		require.NotNil(t, result.GetAck())
		assert.True(t, result.GetAck().GetAccepted())
		assert.Equal(t, agent.SessionID(), result.GetAck().GetSessionId())

		received := agent.Received()
		require.Len(t, received, 1)
		assert.Equal(t, kernelnodeops.NodeReloadOperation, received[0].GetKind())
		assert.Equal(t, result.GetAgentOperationId(), received[0].GetOperationId())

		row, found := f.desired(f.proxyNode())
		require.True(t, found)
		assert.Equal(t, uint64(1), row.Revision)
		assert.Equal(t, result.GetConfigHash(), row.ConfigHash)
		assert.Equal(t, kernelnodeops.DesiredConfigFormat, row.Format)
		assert.Contains(t, row.ConfigJSON, protocolHost)
		assert.Contains(t, row.ConfigJSON, realityKey, "the desired configuration holds the node's secrets, in the protected table")

		// The same configuration: no change, no push.
		second := terminal(t, client, "node.sync:proxy-1:b", syncSpec(f.proxyNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, second.GetState(), second.GetError())
		assert.False(t, second.GetResult().GetNodeSync().GetChanged())
		assert.Equal(t, uint64(1), second.GetResult().GetNodeSync().GetConfigRevision())
		assert.Equal(t, result.GetConfigHash(), second.GetResult().GetNodeSync().GetConfigHash(), "the same configuration has the same hash")
		assert.Empty(t, second.GetResult().GetNodeSync().GetAgentOperationId(), "nothing was pushed")
		assert.Len(t, agent.Received(), 1)

		// Forced: no change, pushed anyway.
		forced := terminal(t, client, "node.sync:proxy-1:c", syncSpec(f.proxyNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, forced.GetState(), forced.GetError())
		assert.False(t, forced.GetResult().GetNodeSync().GetChanged())
		assert.NotEmpty(t, forced.GetResult().GetNodeSync().GetAgentOperationId())
		assert.Equal(t, uint64(2), forced.GetResult().GetNodeSync().GetRevision())
		assert.Len(t, agent.Received(), 2)

		// A changed protocol: the revision grows and the push goes out.
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocol.ID).Update("port", 8443).Error)
		changed := terminal(t, client, "node.sync:proxy-1:d", syncSpec(f.proxyNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, changed.GetState(), changed.GetError())
		assert.True(t, changed.GetResult().GetNodeSync().GetChanged())
		assert.Equal(t, uint64(2), changed.GetResult().GetNodeSync().GetConfigRevision())
		assert.NotEqual(t, result.GetConfigHash(), changed.GetResult().GetNodeSync().GetConfigHash())
		assert.Len(t, agent.Received(), 3)
		row, _ = f.desired(f.proxyNode())
		assert.Equal(t, uint64(2), row.Revision)
	})
}

// A node without a stream keeps its stored configuration for its next
// periodic pull: nothing is pushed and the operation succeeds on
// LEGACY_PULL.
func TestSyncNodeOnTheLegacyTransports(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()
		operation := terminal(t, client, "node.sync:proxy-1:legacy", syncSpec(f.proxyNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result := operation.GetResult().GetNodeSync()
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_LEGACY_PULL, result.GetChannel())
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_LEGACY_PULL, operation.GetChannel())
		assert.True(t, result.GetChanged())
		assert.Equal(t, uint64(1), result.GetConfigRevision())
		assert.Empty(t, result.GetAgentOperationId())
		assert.Nil(t, result.GetAck())
		_, found := f.desired(f.proxyNode())
		assert.True(t, found, "the configuration is stored for the pull")

		// The forward node of the same id has its own configuration.
		forward := terminal(t, client, "node.sync:forward-1:legacy", syncSpec(f.forwardNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, forward.GetState(), forward.GetError())
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_LEGACY_PULL, forward.GetResult().GetNodeSync().GetChannel())
		row, found := f.desired(f.forwardNode())
		require.True(t, found)
		assert.Equal(t, uint64(1), row.Revision)
		assert.NotEqual(t, result.GetConfigHash(), row.ConfigHash)
		assert.Contains(t, row.ConfigJSON, `"legacy_rules"`)
		assert.NotContains(t, row.ConfigJSON, forwardToken, "a forward node's configuration never carries its token")
	})
}

// A forward node's agent streams by certificate in the forward manager:
// its sync is pushed there, and never to the proxy node of the same id.
func TestSyncNodeOfAForwardNodeOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		proxyAgent := f.proxyAgent(fakeagent.Script{})
		forwardAgent := f.forwardAgent(fakeagent.Script{})
		client := f.client()

		operation := terminal(t, client, "node.sync:forward-1:stream", syncSpec(f.forwardNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result := operation.GetResult().GetNodeSync()
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, result.GetChannel())
		assert.Equal(t, forwardAgent.SessionID(), result.GetAck().GetSessionId())
		require.Len(t, forwardAgent.Received(), 1)
		assert.Equal(t, kernelnodeops.NodeReloadOperation, forwardAgent.Received()[0].GetKind())
		assert.Empty(t, proxyAgent.Received(), "the proxy node of the same id received nothing")
		_, found := f.desired(f.forwardNode())
		assert.True(t, found)
	})
}

// The agent's answers decide the outcome: a refusal fails the operation
// with AGENT_REJECTED, and a reported failure with the agent's message.
func TestSyncNodeFollowsTheAgentsAnswers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		refusing := f.proxyAgent(fakeagent.Script{Ack: func(*agentv1pb.DesiredOperation) (bool, string) { return false, "reload is disabled on this node" }})
		client := f.client()
		refused := terminal(t, client, "node.sync:proxy-1:refused", syncSpec(f.proxyNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, refused.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_AGENT_REJECTED, refused.GetError().GetCode())
		assert.Equal(t, "reload is disabled on this node", refused.GetError().GetMessage())
		assert.False(t, refused.GetResult().GetNodeSync().GetAck().GetAccepted(), "the refusal is in the result")
		assert.Equal(t, uint64(1), refused.GetResult().GetNodeSync().GetConfigRevision(), "the configuration was stored before the push")
		refusing.Disconnect()

		failing := f.proxyAgent(fakeagent.Script{Hold: true})
		var pushed *agentv1pb.DesiredOperation
		done := make(chan *kernelnodeopsv1.Operation, 1)
		go func() { done <- terminal(t, client, "node.sync:proxy-1:failing", syncSpec(f.proxyNode(), true)) }()
		var err error
		pushed, err = failing.AwaitKind(awaitTimeout, kernelnodeops.NodeReloadOperation)
		require.NoError(t, err)
		require.NoError(t, failing.Complete(pushed, agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, "xray refused the configuration", nil))
		failed := <-done
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, failed.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, failed.GetError().GetCode())
		assert.Equal(t, "xray refused the configuration", failed.GetError().GetMessage())
		assert.True(t, failed.GetResult().GetNodeSync().GetAck().GetAccepted())
	})
}

// Targets that do not exist are NOT_FOUND at submission and record
// nothing; a node deleted after submission ends TARGET_GONE.
func TestSyncNodeTargets(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()
		_, err := submit(t, client, "node.sync:proxy-999", syncSpec(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 999}, false), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		assert.Equal(t, codes.NotFound, status.Code(err))

		// Record while the node exists, delete it, then let the engine run.
		f.start()
		pending, err := submit(t, client, "node.sync:proxy-1:gone", syncSpec(f.proxyNode(), false), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		require.NoError(t, f.db.Delete(&model.NodeProtocol{}, f.protocol.ID).Error)
		require.NoError(t, f.db.Delete(&model.Node{}, f.proxy.ID).Error)
		ended := awaitTerminalState(t, client, pending.GetOperation().GetOperationId())
		if ended.GetState() == kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED {
			assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, ended.GetError().GetCode())
		} else {
			// The engine ran it before the deletion.
			assert.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, ended.GetState())
		}
	})
}

// awaitTerminalState polls until the operation ended, whatever the state.
func awaitTerminalState(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, operationID string) *kernelnodeopsv1.Operation {
	t.Helper()
	deadline := time.Now().Add(awaitTimeout)
	for time.Now().Before(deadline) {
		response, err := client.GetOperation(context.Background(), &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: operationID}})
		require.NoError(t, err)
		switch response.GetOperation().GetState() {
		case kernelnodeopsv1.OperationState_OPERATION_STATE_PENDING, kernelnodeopsv1.OperationState_OPERATION_STATE_DISPATCHING, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING:
			time.Sleep(settleInterval)
		default:
			return response.GetOperation()
		}
	}
	require.Fail(t, "the operation did not end")
	return nil
}
