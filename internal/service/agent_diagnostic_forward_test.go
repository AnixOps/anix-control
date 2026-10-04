package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The forward checks are the route diagnosis's alone: the administrator
// routes and the KernelNodeOps agent.diagnostic kind refuse them.
func TestForwardChecksAreNotAdministratorTasks(t *testing.T) {
	for action := range ForwardDiagnosticChecks {
		_, err := ValidateAgentDiagnosticTask(action, map[string]any{"route_id": "r", "hop_index": 0})
		require.ErrorContains(t, err, "only by the forward route diagnosis", action)
	}
	_, err := ValidateAgentDiagnosticTask(ForwardCheckConnect, map[string]any{})
	require.Error(t, err)
	_, err = ValidateAgentDiagnosticTask(AgentDiagnosticActionServiceStatus, map[string]any{"service": "gost"})
	require.NoError(t, err, "the generic whitelist is unchanged")
}

func TestValidateForwardDiagnosticCheck(t *testing.T) {
	normalized, err := ValidateForwardDiagnosticCheck(ForwardCheckListen, map[string]any{
		"route_id": "01JF1A000000000000000000ZZ", "hop_index": float64(2), "generation": float64(12), "upstream": "1.2.3.4:5", "target_policy": "allow_private",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"route_id": "01JF1A000000000000000000ZZ", "hop_index": int64(2), "generation": int64(12), "timeout_ms": int64(ForwardCheckDefaultTimeoutMS)},
		normalized, "listen takes no upstream or policy")

	normalized, err = ValidateForwardDiagnosticCheck(ForwardCheckUDPProbe, map[string]any{
		"route_id": "r1", "hop_index": 0, "timeout_ms": 5, "upstream": "[2001:db8::1]:53",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"route_id": "r1", "hop_index": int64(0), "timeout_ms": int64(ForwardCheckMinTimeoutMS), "upstream": "[2001:db8::1]:53",
		"target_policy": ForwardTargetPolicyPublicOnly}, normalized)

	for name, params := range map[string]map[string]any{
		"no route":         {"hop_index": 0},
		"route spaces":     {"route_id": "a b", "hop_index": 0},
		"no hop":           {"route_id": "r"},
		"hop too large":    {"route_id": "r", "hop_index": 8},
		"fractional hop":   {"route_id": "r", "hop_index": 1.5},
		"negative gen":     {"route_id": "r", "hop_index": 0, "generation": -1},
		"bad timeout":      {"route_id": "r", "hop_index": 0, "timeout_ms": "1s"},
		"upstream no port": {"route_id": "r", "hop_index": 0, "upstream": "1.2.3.4"},
		"upstream port 0":  {"route_id": "r", "hop_index": 0, "upstream": "1.2.3.4:0"},
		"bad policy":       {"route_id": "r", "hop_index": 0, "target_policy": "anything"},
	} {
		_, err := ValidateForwardDiagnosticCheck(ForwardCheckConnect, params)
		assert.Error(t, err, name)
	}
	_, err = ValidateForwardDiagnosticCheck("service_status", map[string]any{"service": "gost"})
	require.Error(t, err)
	assert.True(t, IsForwardDiagnosticCheck(ForwardCheckPortConflict))
	assert.False(t, IsForwardDiagnosticCheck(AgentDiagnosticActionLogTail))
}
