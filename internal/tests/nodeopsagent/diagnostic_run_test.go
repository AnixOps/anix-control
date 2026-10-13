package nodeopsagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *fixture) sources() kernelnodeops.AgentSources {
	return kernelnodeops.AgentSources{Streams: f.control.Streams}
}

// The administrator API's diagnostic waits for the Agent's result on the
// stream: a proxy node's generic action is recorded and completed, a
// forward node's check answers its structured result without a row, and
// the Agent is sent the whitelist's normalized parameters.
func TestRunNodeDiagnosticOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		ctx := context.Background()
		_, err := kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), kernelnodeops.NodeDiagnosticRequest{
			Node: f.proxyNode(), Action: "service_status", Params: map[string]any{"service": "gost"},
		})
		var unavailable *kernelnodeops.DiagnosticUnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, kernelnodeops.DiagnosticOffline, unavailable.Reason)
		var rows int64
		require.NoError(t, f.db.Model(&model.AgentDiagnosticTask{}).Count(&rows).Error)
		assert.Zero(t, rows, "a node that cannot be reached records nothing")

		agent := f.proxyAgent(fakeagent.Script{Result: func(*agentv1pb.DesiredOperation) []byte {
			return []byte(`{"success":true,"output":"active (running)","duration_ms":12}`)
		}})
		result, err := kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), kernelnodeops.NodeDiagnosticRequest{
			Node: f.proxyNode(), Action: "log_tail", Params: map[string]any{"service": "gost", "lines": 99999, "extra": "dropped"}, Timeout: 20 * time.Second,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "active (running)", result.Output)
		assert.EqualValues(t, 12, result.DurationMS)
		assert.Equal(t, map[string]any{"service": "gost", "lines": 1000}, result.Params)

		received := agent.Received()
		require.Len(t, received, 1)
		assert.Equal(t, kernelnodeops.AgentDiagnosticOperation, received[0].GetKind())
		assert.Equal(t, result.TaskID, received[0].GetOperationId())
		var payload struct {
			Task struct {
				Type    string         `json:"type"`
				Action  string         `json:"action"`
				Params  map[string]any `json:"params"`
				Timeout int            `json:"timeout"`
			} `json:"task"`
		}
		require.NoError(t, json.Unmarshal(received[0].GetPayloadJson(), &payload))
		assert.Equal(t, "log_tail", payload.Task.Action)
		assert.Equal(t, 20, payload.Task.Timeout)
		assert.Equal(t, map[string]any{"service": "gost", "lines": float64(1000)}, payload.Task.Params)
		var row model.AgentDiagnosticTask
		require.NoError(t, f.db.Where("task_id = ?", result.TaskID).First(&row).Error)
		assert.Equal(t, model.AgentDiagnosticTaskStatusCompleted, row.Status)
		assert.Equal(t, f.proxy.ID, row.NodeID)
		assert.Equal(t, "active (running)", row.Output)
		agent.Disconnect()

		// A forward node: the forward checks need diag.v1, answer the
		// structured result and keep no task row.
		old := f.forwardAgent(fakeagent.Script{})
		_, err = kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), kernelnodeops.NodeDiagnosticRequest{
			Node: f.forwardNode(), Action: "forward.listen", Params: map[string]any{"route_id": "r1", "hop_index": 0},
		})
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, kernelnodeops.DiagnosticUnsupported, unavailable.Reason)
		assert.Contains(t, unavailable.Message, "diag.v1")
		assert.Empty(t, old.Received())
		old.Disconnect()

		f.forwardAgent(fakeagent.Script{Capabilities: forwardCheckCapabilities, Result: func(*agentv1pb.DesiredOperation) []byte {
			return []byte(`{"success":true,"output":"listening","duration_ms":2,"result":{"check":"forward.listen","route_id":"r1","hop_index":0,"status":"ok","items":[]}}`)
		}})
		checked, err := kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), kernelnodeops.NodeDiagnosticRequest{
			Node: f.forwardNode(), Action: "forward.listen", Params: map[string]any{"route_id": "r1", "hop_index": 0},
		})
		require.NoError(t, err)
		assert.True(t, checked.Success)
		assert.JSONEq(t, `{"check":"forward.listen","route_id":"r1","hop_index":0,"status":"ok","items":[]}`, string(checked.Result))
		require.NoError(t, f.db.Model(&model.AgentDiagnosticTask{}).Count(&rows).Error)
		assert.EqualValues(t, 1, rows, "only the proxy node's task has a row")
	})
}

// An Agent that refuses the task, one that never reports and a caller that
// goes away end the task: the row is failed, and the error says why.
func TestRunNodeDiagnosticFailures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		ctx := context.Background()
		request := kernelnodeops.NodeDiagnosticRequest{Node: f.proxyNode(), Action: "service_status", Params: map[string]any{"service": "gost"}}

		f.proxyAgent(fakeagent.Script{Ack: func(*agentv1pb.DesiredOperation) (bool, string) { return false, "busy" }})
		_, err := kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), request)
		var refused *kernelnodeops.DiagnosticAgentError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, "busy", refused.Message)
		row := awaitTaskStatus(t, f, refused.TaskID, model.AgentDiagnosticTaskStatusFailed)
		assert.Equal(t, "busy", row.Error)

		agent := f.proxyAgent(fakeagent.Script{Hold: true})
		request.Timeout = time.Second
		started := time.Now()
		_, err = kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), request)
		var timedOut *kernelnodeops.DiagnosticTimeoutError
		require.ErrorAs(t, err, &timedOut)
		assert.True(t, timedOut.Acknowledged)
		assert.Less(t, time.Since(started), 5*time.Second)
		awaitTaskStatus(t, f, timedOut.TaskID, model.AgentDiagnosticTaskStatusFailed)

		// The Agent's own deadline report is a timeout too.
		request.Timeout = 30 * time.Second
		done := make(chan error, 1)
		go func() {
			_, err := kernelnodeops.RunNodeDiagnostic(ctx, f.db, f.sources(), request)
			done <- err
		}()
		held, err := agent.Await(awaitTimeout, func(operation *agentv1pb.DesiredOperation) bool {
			return operation.GetKind() == kernelnodeops.AgentDiagnosticOperation && operation.GetOperationId() != timedOut.TaskID
		})
		require.NoError(t, err)
		require.NoError(t, agent.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, "operation deadline exceeded", nil))
		require.ErrorAs(t, <-done, &timedOut)

		// A caller that goes away is the context's own error.
		callerCtx, cancel := context.WithCancel(ctx)
		go func() {
			_, err := kernelnodeops.RunNodeDiagnostic(callerCtx, f.db, f.sources(), request)
			done <- err
		}()
		_, err = agent.Await(awaitTimeout, func(operation *agentv1pb.DesiredOperation) bool {
			return operation.GetKind() == kernelnodeops.AgentDiagnosticOperation && operation.GetOperationId() != held.GetOperationId() && operation.GetOperationId() != timedOut.TaskID
		})
		require.NoError(t, err)
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}
