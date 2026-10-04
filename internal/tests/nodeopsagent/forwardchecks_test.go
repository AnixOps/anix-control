package nodeopsagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forwardCheckCapabilities are an Agent that runs the forward checks.
var forwardCheckCapabilities = append(append([]string(nil), fakeagent.DefaultCapabilities...), "diag")

func (f *fixture) forwardChecks() kernelnodeops.ForwardChecks {
	return kernelnodeops.ForwardChecks{Sources: func() kernelnodeops.AgentSources {
		return kernelnodeops.AgentSources{Streams: f.control.Streams}
	}}
}

// The route diagnosis's forward checks ride agent.diagnostic on a forward
// node's stream: the parameters normalized, the check's result decoded
// from the Agent's state, and no task row written.
func TestForwardChecksOnTheStream(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		checks := f.forwardChecks()
		assert.Equal(t, kernelforward.NodeVantage{Note: "the node's Agent is not connected"}, checks.Vantage(f.forwardNode()))
		_, err := checks.Check(context.Background(), f.forwardNode(), "forward.listen", map[string]any{"route_id": "r1", "hop_index": 0})
		require.ErrorIs(t, err, kernelforward.ErrCheckUnavailable)

		agent := f.forwardAgent(fakeagent.Script{Capabilities: forwardCheckCapabilities, AgentVersion: "4.2.1", Result: func(*agentv1pb.DesiredOperation) []byte {
			return []byte(`{"success":true,"output":"1 upstream reachable","duration_ms":4,"result":{"check":"forward.connect","route_id":"r1","hop_index":1,
				"generation":7,"status":"ok","items":[{"target":"198.51.100.10:443","protocol":"tcp","status":"ok","rtt_us":900}]}}`)
		}})
		vantage := checks.Vantage(f.forwardNode())
		assert.True(t, vantage.Connected)
		assert.True(t, vantage.Capable)

		result, err := checks.Check(context.Background(), f.forwardNode(), "forward.connect", map[string]any{
			"route_id": "r1", "hop_index": int64(1), "generation": int64(7), "timeout_ms": int64(50000), "target_policy": "public_only", "extra": "dropped",
		})
		require.NoError(t, err)
		assert.Equal(t, kernelforward.CheckOK, result.Status)
		require.Len(t, result.Items, 1)
		assert.EqualValues(t, 900, result.Items[0].RTTMicro)
		assert.NotEmpty(t, result.OperationID)

		received := agent.Received()
		require.Len(t, received, 1)
		assert.Equal(t, kernelnodeops.AgentDiagnosticOperation, received[0].GetKind())
		assert.Equal(t, result.OperationID, received[0].GetOperationId())
		assert.NotZero(t, received[0].GetDeadlineUnixMs())
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
		assert.Equal(t, "diagnostic", payload.Task.Type)
		assert.Equal(t, "forward.connect", payload.Task.Action)
		assert.Equal(t, map[string]any{"route_id": "r1", "hop_index": float64(1), "generation": float64(7), "timeout_ms": float64(10000), "target_policy": "public_only"},
			payload.Task.Params, "unknown parameters dropped, the timeout clamped")
		assert.Equal(t, 10, payload.Task.Timeout)
		var rows int64
		require.NoError(t, f.db.Model(&model.AgentDiagnosticTask{}).Count(&rows).Error)
		assert.Zero(t, rows, "probes are not administrator tasks")

		// Refused before anything is sent.
		_, err = checks.Check(context.Background(), f.forwardNode(), "forward.connect", map[string]any{"route_id": "r1", "hop_index": 9})
		require.ErrorContains(t, err, "hop_index")
		_, err = checks.Check(context.Background(), f.forwardNode(), "service_restart", map[string]any{"service": "gost"})
		require.ErrorContains(t, err, "not a forward diagnostic check")
		assert.Len(t, agent.Received(), 1)
	})
}

// An Agent without diag.v1 does not take the checks; an Agent that fails
// a check, answers without a result or never answers is reported as such.
func TestForwardChecksRefusalsAndFailures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		checks := f.forwardChecks()
		old := f.forwardAgent(fakeagent.Script{AgentVersion: "4.1.9"})
		vantage := checks.Vantage(f.forwardNode())
		assert.True(t, vantage.Connected)
		assert.False(t, vantage.Capable)
		assert.Contains(t, vantage.Note, "diag.v1")
		params := map[string]any{"route_id": "r1", "hop_index": 0, "timeout_ms": 200}
		_, err := checks.Check(context.Background(), f.forwardNode(), "forward.listen", params)
		require.ErrorIs(t, err, kernelforward.ErrCheckUnavailable)
		assert.Empty(t, old.Received())
		old.Disconnect()

		agent := f.forwardAgent(fakeagent.Script{Capabilities: forwardCheckCapabilities, Hold: true})
		require.Eventually(t, func() bool { return checks.Vantage(f.forwardNode()).Capable }, awaitTimeout, settleInterval)
		done := make(chan error, 1)
		go func() {
			_, err := checks.Check(context.Background(), f.forwardNode(), "forward.listen", params)
			done <- err
		}()
		held, err := agent.AwaitKind(awaitTimeout, kernelnodeops.AgentDiagnosticOperation)
		require.NoError(t, err)
		require.NoError(t, agent.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, "unsupported diagnostic action", nil))
		require.ErrorContains(t, <-done, "unsupported diagnostic action")

		go func() {
			_, err := checks.Check(context.Background(), f.forwardNode(), "forward.listen", params)
			done <- err
		}()
		held, err = agent.Await(awaitTimeout, func(operation *agentv1pb.DesiredOperation) bool {
			return operation.GetOperationId() != held.GetOperationId()
		})
		require.NoError(t, err)
		require.NoError(t, agent.Complete(held, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "", map[string]any{"success": true, "output": "active"}))
		require.ErrorContains(t, <-done, "without a forward check result")

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_, err = checks.Check(ctx, f.forwardNode(), "forward.listen", params)
		require.True(t, errors.Is(err, context.DeadlineExceeded), err)
	})
}
