package grpc

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentSessionMetricsKeepsOnlyAgentHealth(t *testing.T) {
	assert.Nil(t, agentSessionMetrics(nil))
	assert.Nil(t, agentSessionMetrics(map[string]float64{"plugin.machine-telemetry.cpu": 1, "agent_other_x": 1}))
	kept := agentSessionMetrics(map[string]float64{
		"agent_control_reconnects_total": 2, "agent_identity_enrolled": 1, "agent_dataplane_apply_failures_total": 4,
		"agent_dataplane_nan": math.NaN(), "agent_dataplane_inf": math.Inf(1), "agent_control_Upper": 1, "plugin.x": 1,
	})
	assert.Equal(t, map[string]float64{
		"agent_control_reconnects_total": 2, "agent_identity_enrolled": 1, "agent_dataplane_apply_failures_total": 4,
	}, kept)

	many := map[string]float64{}
	for index := 0; index < maxAgentSessionMetrics+10; index++ {
		many["agent_dataplane_m"+strconv.Itoa(index)] = float64(index)
	}
	assert.Len(t, agentSessionMetrics(many), maxAgentSessionMetrics)

	// A heartbeat without them keeps the last set.
	connection := &AgentControlConnection{}
	at := time.Now()
	connection.recordAgentMetrics(map[string]float64{"agent_control_up": 1}, at)
	connection.recordAgentMetrics(map[string]float64{"plugin.x": 1}, at.Add(time.Minute))
	connection.stateMu.RLock()
	metrics, reportedAt := connection.agentMetricsCopyLocked()
	connection.stateMu.RUnlock()
	assert.Equal(t, map[string]float64{"agent_control_up": 1}, metrics)
	require.NotNil(t, reportedAt)
	assert.True(t, reportedAt.Equal(at))
	metrics["agent_control_up"] = 9
	assert.Equal(t, float64(1), connection.agentMetrics["agent_control_up"], "a copy")
}
