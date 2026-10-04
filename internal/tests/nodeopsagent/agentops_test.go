package nodeopsagent

import (
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// An Agent Control operation goes out on the stream, is answered at the
// acknowledgement (wait ACCEPTED) and ends with the agent's terminal state.
func TestAgentOperationOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Result: func(*agentv1pb.DesiredOperation) []byte { return []byte(`{"node_id":1,"tag":"fake"}`) }})
		client := f.client()

		accepted, err := submit(t, client, "agent.op:ping-1", agentOperationSpec(f.proxy.ID, "agent.ping", "manual-ping-1", 5), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
		require.NoError(t, err)
		require.True(t, accepted.GetApplied())
		assert.Contains(t, []kernelnodeopsv1.OperationState{kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED},
			accepted.GetOperation().GetState(), "ACCEPTED answers once the agent acknowledged")
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, accepted.GetOperation().GetChannel())
		assert.Equal(t, uint64(1), accepted.GetOperation().GetNodeRevision())
		// The result so far is recorded with the acceptance: the
		// acknowledgement the legacy route answers at once.
		soFar := accepted.GetOperation().GetResult().GetAgentOperation()
		require.NotNil(t, soFar, "an ACCEPTED wait answers the acknowledgement")
		assert.Equal(t, "manual-ping-1", soFar.GetAgentOperationId())
		assert.True(t, soFar.GetAck().GetAccepted())
		assert.NotZero(t, soFar.GetDeadlineUnixMs())

		ended := awaitState(t, client, accepted.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
		result := ended.GetResult().GetAgentOperation()
		require.NotNil(t, result)
		assert.Equal(t, "manual-ping-1", result.GetAgentOperationId())
		assert.Equal(t, uint64(1), result.GetRevision())
		assert.True(t, result.GetAck().GetAccepted())
		assert.Equal(t, agent.SessionID(), result.GetAck().GetSessionId())

		received := agent.Received()
		require.Len(t, received, 1)
		assert.Equal(t, "agent.ping", received[0].GetKind())
		assert.Equal(t, "manual-ping-1", received[0].GetOperationId())
		assert.JSONEq(t, `{"source":"test"}`, string(received[0].GetPayloadJson()))

		// A repeat of the request id answers the receipt, without a new push.
		again, err := submit(t, client, "agent.op:ping-1", agentOperationSpec(f.proxy.ID, "agent.ping", "manual-ping-1", 5), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		assert.False(t, again.GetApplied())
		assert.Len(t, agent.Received(), 1)
	})
}

// What the agent cannot or will not do fails with the matching code; an
// agent that is not there fails NODE_OFFLINE.
func TestAgentOperationRefusals(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()
		offline := terminal(t, client, "agent.op:offline", agentOperationSpec(f.proxy.ID, "agent.ping", "", 2))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, offline.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, offline.GetError().GetCode())
		assert.True(t, offline.GetError().GetRetryable())

		agent := f.proxyAgent(fakeagent.Script{
			Capabilities: []string{"agent.control", "agent.ping", "operation.cancel"},
			Ack:          func(*agentv1pb.DesiredOperation) (bool, string) { return false, "operation queue is full" },
		})
		missing := terminal(t, client, "agent.op:missing", agentOperationSpec(f.proxy.ID, "users.reload", "", 2))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, missing.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_CAPABILITY_MISSING, missing.GetError().GetCode())
		assert.Contains(t, missing.GetError().GetMessage(), `does not advertise capability "users.reload"`)
		assert.Empty(t, agent.Received())

		rejected := terminal(t, client, "agent.op:rejected", agentOperationSpec(f.proxy.ID, "agent.ping", "", 2))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, rejected.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_AGENT_REJECTED, rejected.GetError().GetCode())
		assert.Equal(t, "operation queue is full", rejected.GetError().GetMessage())
		assert.False(t, rejected.GetResult().GetAgentOperation().GetAck().GetAccepted())
		assert.Len(t, agent.Received(), 1)

		_, err := submit(t, client, "agent.op:kind", agentOperationSpec(f.proxy.ID, "plugin.install", "", 2), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "only the three kinds are accepted")
	})
}

// Chaos (section 9): the agent disconnects before the acknowledgement,
// after it, and is replaced by a newer session.
func TestAgentOperationAcrossDisconnects(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()

		t.Run("before the acknowledgement", func(t *testing.T) {
			silent := f.proxyAgent(fakeagent.Script{Silent: true})
			done := make(chan *kernelnodeopsv1.Operation, 1)
			go func() {
				done <- terminal(t, client, "agent.op:cut-before-ack", agentOperationSpec(f.proxy.ID, "agent.ping", "cut-1", 5))
			}()
			_, err := silent.AwaitKind(awaitTimeout, "agent.ping")
			require.NoError(t, err)
			silent.Disconnect()
			ended := <-done
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, ended.GetState())
			assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, ended.GetError().GetCode())
			assert.True(t, ended.GetError().GetRetryable())
			assert.Contains(t, ended.GetError().GetMessage(), "closed before operation ACK")
		})

		t.Run("after the acknowledgement, then a reconnect replays", func(t *testing.T) {
			holding := f.proxyAgent(fakeagent.Script{Hold: true})
			accepted, err := submit(t, client, "agent.op:cut-after-ack", agentOperationSpec(f.proxy.ID, "agent.ping", "cut-2", 5), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
			require.NoError(t, err)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, accepted.GetOperation().GetState())
			holding.Disconnect()

			// The reconnected agent receives the retained operation again
			// (replayed at its revision) and completes it.
			reconnected := f.proxyAgent(fakeagent.Script{ObservedRevision: accepted.GetOperation().GetNodeRevision() - 1})
			replayed, err := reconnected.Await(awaitTimeout, func(operation *agentv1pb.DesiredOperation) bool { return operation.GetOperationId() == "cut-2" })
			require.NoError(t, err)
			assert.Equal(t, "cut-2", replayed.GetOperationId())
			assert.Equal(t, accepted.GetOperation().GetNodeRevision(), replayed.GetRevision())
			ended := awaitState(t, client, accepted.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
			assert.Equal(t, holding.SessionID(), ended.GetResult().GetAgentOperation().GetAck().GetSessionId(), "the recorded acknowledgement is the first session's")
			reconnected.Disconnect()
		})

		t.Run("a replaced session's reports are refused", func(t *testing.T) {
			old := f.proxyAgent(fakeagent.Script{Hold: true})
			accepted, err := submit(t, client, "agent.op:replaced", agentOperationSpec(f.proxy.ID, "agent.ping", "replaced-1", 5), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
			require.NoError(t, err)
			held, err := old.AwaitKind(awaitTimeout, "agent.ping")
			require.NoError(t, err)

			replacement := f.proxyAgent(fakeagent.Script{Hold: true, ObservedRevision: held.GetRevision() - 1})
			replayed, err := replacement.Await(awaitTimeout, func(operation *agentv1pb.DesiredOperation) bool { return operation.GetOperationId() == "replaced-1" })
			require.NoError(t, err)
			assert.Equal(t, held.GetRevision(), replayed.GetRevision())

			// The old session reports success: refused, and its stream ends.
			_ = old.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "", map[string]any{"stale": true})
			exited, exitErr := old.Exited(awaitTimeout)
			require.True(t, exited, "the replaced session's stream ends")
			if exitErr != nil {
				assert.Equal(t, codes.Aborted, status.Code(exitErr))
			}
			time.Sleep(5 * settleInterval)
			current := awaitState(t, client, accepted.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING)
			assert.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, current.GetState(), "the stale report changed nothing")

			// The replacement's report ends it.
			require.NoError(t, replacement.Complete(replayed, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "", map[string]any{"ok": true}))
			awaitState(t, client, accepted.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
			replacement.Disconnect()
		})
	})
}

// A cancelled operation is cancelled on the agent too; one the agent never
// ends times out at the deadline, and its late report is evidence only.
func TestAgentOperationCancellationAndDeadline(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		f.timeout = 2 * time.Second
		client := f.client()
		agent := f.proxyAgent(fakeagent.Script{Hold: true})

		accepted, err := submit(t, client, "agent.op:cancel", agentOperationSpec(f.proxy.ID, "agent.ping", "cancel-1", 5), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
		require.NoError(t, err)
		cancelled, err := client.CancelOperation(t.Context(), &kernelnodeopsv1.CancelOperationRequest{OperationId: accepted.GetOperation().GetOperationId(), Reason: "operator"})
		require.NoError(t, err)
		assert.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_CANCELLED, cancelled.GetOperation().GetState())
		cancel, err := agent.AwaitKind(awaitTimeout, "operation.cancel")
		require.NoError(t, err)
		assert.Equal(t, "cancel-1", cancel.GetOperationId())

		late, err := submit(t, client, "agent.op:deadline", agentOperationSpec(f.proxy.ID, "agent.ping", "late-1", 1), kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED)
		require.NoError(t, err)
		held, err := agent.AwaitKind(awaitTimeout, "agent.ping")
		require.NoError(t, err)
		timedOut := awaitState(t, client, late.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_TIMED_OUT)
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, timedOut.GetError().GetCode())
		require.NoError(t, agent.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "", map[string]any{"late": true}))
		time.Sleep(5 * settleInterval)
		still := awaitState(t, client, late.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_TIMED_OUT)
		assert.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_TIMED_OUT, still.GetState(), "a late result does not reverse the deadline")
	})
}
