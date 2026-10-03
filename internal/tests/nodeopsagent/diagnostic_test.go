package nodeopsagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A diagnostic goes to an agent that advertises agent.diagnostic over the
// stream: the whitelist normalizes the parameters, the task row is written
// before the dispatch, and the agent's report completes it.
func TestAgentDiagnosticOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Result: func(*agentv1pb.DesiredOperation) []byte {
			return []byte(`{"success":true,"output":"active (running)","duration_ms":12}`)
		}})
		client := f.client()
		operation := terminal(t, client, "agent.diag:1", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost","extra":"dropped"}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, operation.GetChannel())
		result := operation.GetResult().GetAgentDiagnostic()
		require.NotNil(t, result)
		assert.True(t, result.GetAckReceived())
		assert.False(t, result.GetLegacyFallback())
		assert.Empty(t, result.GetDispatchError())
		assert.Equal(t, result.GetTaskId(), result.GetMessageId())
		assert.Equal(t, agent.SessionID(), result.GetAck().GetSessionId())

		received := agent.Received()
		require.Len(t, received, 1)
		assert.Equal(t, kernelnodeops.AgentDiagnosticOperation, received[0].GetKind())
		assert.Equal(t, result.GetTaskId(), received[0].GetOperationId())
		var payload struct {
			Task struct {
				ID      string         `json:"id"`
				Type    string         `json:"type"`
				Action  string         `json:"action"`
				Params  map[string]any `json:"params"`
				Timeout int            `json:"timeout"`
			} `json:"task"`
		}
		require.NoError(t, json.Unmarshal(received[0].GetPayloadJson(), &payload))
		assert.Equal(t, result.GetTaskId(), payload.Task.ID)
		assert.Equal(t, "diagnostic", payload.Task.Type)
		assert.Equal(t, "service_status", payload.Task.Action)
		assert.Equal(t, map[string]any{"service": "gost"}, payload.Task.Params, "the whitelist drops unknown parameters")
		assert.Equal(t, 30, payload.Task.Timeout)

		var row model.AgentDiagnosticTask
		require.NoError(t, f.db.Where("task_id = ?", result.GetTaskId()).First(&row).Error)
		assert.Equal(t, model.AgentDiagnosticTaskStatusCompleted, row.Status)
		assert.True(t, row.Success)
		assert.Equal(t, "active (running)", row.Output)
		assert.Equal(t, int64(12), row.DurationMS)
		assert.JSONEq(t, `{"service":"gost"}`, row.Params)
	})
}

// Without a stream that runs diagnostics, the task goes to the node's
// WebSocket as the legacy route sends it, with its fallback; without any
// agent the node is offline. A refused action records nothing.
func TestAgentDiagnosticOnTheWebSocketAndRefusals(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()
		offline := terminal(t, client, "agent.diag:offline", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, offline.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, offline.GetError().GetCode())
		assert.Equal(t, "node offline", offline.GetError().GetMessage())

		// A stream agent without the capability does not take diagnostics;
		// the WebSocket does.
		f.proxyAgent(fakeagent.Script{Capabilities: []string{"agent.control", "agent.ping"}})
		transport := f.websockets.connect(f.proxy.ID, acknowledged)
		acked := terminal(t, client, "agent.diag:ws", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, acked.GetState(), acked.GetError())
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_WEBSOCKET, acked.GetChannel())
		result := acked.GetResult().GetAgentDiagnostic()
		assert.True(t, result.GetAckReceived())
		assert.Equal(t, "msg-"+result.GetTaskId(), result.GetMessageId())
		require.Len(t, transport.received(), 1)
		assert.Equal(t, result.GetTaskId(), transport.received()[0].ID)
		var row model.AgentDiagnosticTask
		require.NoError(t, f.db.Where("task_id = ?", result.GetTaskId()).First(&row).Error)
		assert.Equal(t, model.AgentDiagnosticTaskStatusDispatched, row.Status, "the WebSocket agent reports its result later")

		f.websockets.connect(f.proxy.ID, fallenBack)
		fallback := terminal(t, client, "agent.diag:fallback", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, fallback.GetState(), fallback.GetError())
		assert.True(t, fallback.GetResult().GetAgentDiagnostic().GetLegacyFallback())
		assert.False(t, fallback.GetResult().GetAgentDiagnostic().GetAckReceived())
		assert.Equal(t, "ack timeout after 3s", fallback.GetResult().GetAgentDiagnostic().GetDispatchError())

		f.websockets.connect(f.proxy.ID, undeliverable)
		lost := terminal(t, client, "agent.diag:lost", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, lost.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, lost.GetError().GetCode())
		assert.Contains(t, lost.GetError().GetMessage(), "legacy fallback: websocket: close sent")
		var lostRow model.AgentDiagnosticTask
		require.NoError(t, f.db.Where("task_id = ?", lost.GetResult().GetAgentDiagnostic().GetTaskId()).First(&lostRow).Error)
		assert.Equal(t, model.AgentDiagnosticTaskStatusFailed, lostRow.Status)

		refused := terminal(t, client, "agent.diag:refused", diagnosticSpec(f.proxy.ID, "rm_rf", `{}`))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, refused.GetState())
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, refused.GetError().GetCode())
		assert.Equal(t, `action "rm_rf" is not in the diagnostic whitelist`, refused.GetError().GetMessage())
		var refusedRows int64
		require.NoError(t, f.db.Model(&model.AgentDiagnosticTask{}).Where("action = ?", "rm_rf").Count(&refusedRows).Error)
		assert.Zero(t, refusedRows)
	})
}

// A failure the agent reports on the stream fails the operation and the
// task row.
func TestAgentDiagnosticFailureReport(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Hold: true})
		client := f.client()
		done := make(chan *kernelnodeopsv1.Operation, 1)
		go func() {
			done <- terminal(t, client, "agent.diag:failed", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`))
		}()
		held, err := agent.AwaitKind(awaitTimeout, kernelnodeops.AgentDiagnosticOperation)
		require.NoError(t, err)
		require.NoError(t, agent.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, "systemctl: unit not found",
			map[string]any{"success": false, "error": "unit gost.service not found", "duration_ms": 3}))
		failed := <-done
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, failed.GetState())
		assert.Equal(t, "systemctl: unit not found", failed.GetError().GetMessage())
		var row model.AgentDiagnosticTask
		require.NoError(t, f.db.Where("task_id = ?", held.GetOperationId()).First(&row).Error)
		assert.Equal(t, model.AgentDiagnosticTaskStatusFailed, row.Status)
		assert.Equal(t, "unit gost.service not found", row.Error)
		assert.Equal(t, int64(3), row.DurationMS)
	})
}

// awaitTaskStatus waits until the task's row reaches status.
func awaitTaskStatus(t *testing.T, f *fixture, taskID, status string) model.AgentDiagnosticTask {
	t.Helper()
	var row model.AgentDiagnosticTask
	require.Eventually(t, func() bool {
		return f.db.Where("task_id = ?", taskID).First(&row).Error == nil && row.Status == status
	}, 10*time.Second, 10*time.Millisecond, "task %s never became %s (last %q)", taskID, status, row.Status)
	return row
}

// The legacy admin routes (POST /admin/agent/tasks, /admin/agent/execute)
// reach a stream agent that advertises agent.diagnostic: the answer is the
// acknowledgement, and the agent's report completes the row later. A node
// without such a stream is not served there; a refused action records
// nothing; a result that never comes fails the row.
func TestLegacyDiagnosticRouteOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		ctx := context.Background()
		sources := kernelnodeops.AgentSources{Streams: f.control.Streams}
		request := kernelnodeops.AgentDiagnosticRequest{NodeID: f.proxy.ID, Action: "service_status", Params: map[string]any{"service": "gost"}}

		_, onStream, err := kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, request)
		require.NoError(t, err)
		assert.False(t, onStream, "no agent is connected")
		_, onStream, _ = kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, kernelnodeops.AgentSources{}, request)
		assert.False(t, onStream, "no streams")

		f.proxyAgent(fakeagent.Script{Capabilities: []string{"agent.control", "agent.ping"}})
		_, onStream, err = kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, request)
		require.NoError(t, err)
		assert.False(t, onStream, "the agent does not run diagnostics")

		agent := f.proxyAgent(fakeagent.Script{Result: func(*agentv1pb.DesiredOperation) []byte {
			return []byte(`{"success":true,"output":"active (running)","duration_ms":7}`)
		}})
		run, onStream, err := kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, request)
		require.NoError(t, err)
		require.True(t, onStream)
		assert.True(t, run.Dispatch.AckReceived)
		assert.NoError(t, run.Dispatch.DispatchError)
		assert.Equal(t, agent.SessionID(), run.Dispatch.Ack.SessionID)
		row := awaitTaskStatus(t, f, run.Task.ID, model.AgentDiagnosticTaskStatusCompleted)
		assert.True(t, row.Success)
		assert.Equal(t, "active (running)", row.Output)

		_, onStream, err = kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, kernelnodeops.AgentDiagnosticRequest{NodeID: f.proxy.ID, Action: "rm -rf /"})
		assert.True(t, onStream)
		var refused *kernelnodeops.DiagnosticValidationError
		assert.ErrorAs(t, err, &refused)

		// An agent that refuses the task: not sent, the row failed.
		f.proxyAgent(fakeagent.Script{Ack: func(*agentv1pb.DesiredOperation) (bool, string) { return false, "busy" }})
		run, onStream, err = kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, request)
		require.NoError(t, err)
		require.True(t, onStream)
		assert.Error(t, run.Dispatch.DispatchError)
		awaitTaskStatus(t, f, run.Task.ID, model.AgentDiagnosticTaskStatusFailed)

		// An agent that never reports: the row fails after the timeout.
		defer kernelnodeops.SetDiagnosticResultGraceForTest(0)()
		f.proxyAgent(fakeagent.Script{Hold: true})
		timed := request
		timed.Timeout = 1
		run, onStream, err = kernelnodeops.RunAgentDiagnosticOnStream(ctx, f.db, sources, timed)
		require.NoError(t, err)
		require.True(t, onStream)
		assert.True(t, run.Dispatch.AckReceived)
		awaitTaskStatus(t, f, run.Task.ID, model.AgentDiagnosticTaskStatusFailed)
	})
}
