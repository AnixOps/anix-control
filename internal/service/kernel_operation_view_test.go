package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestPublicKernelOperationHidesExecutionPayload(t *testing.T) {
	nodeID := uint(7)
	now := time.Now().UTC().Truncate(time.Microsecond)
	operation := model.KernelOperation{
		ID: "operation-safe-view", NodeID: &nodeID, PluginID: "nftables-forward", TargetVersion: "1.2.0",
		Kind: "plugin.configure", Revision: 4, ConfigHash: strings.Repeat("a", 64), State: "failed",
		SessionID: "session-must-not-leak", ConfigJSON: `{"token":"config-must-not-leak"}`,
		ResultJSON: `{"agent_token":"result-must-not-leak"}`, LastError: "agent-error-must-not-leak",
		CreatedAt: now, UpdatedAt: now,
	}

	view := PublicKernelOperation(operation)
	require.True(t, view.HasResult)
	require.True(t, view.HasError)
	payload, err := json.Marshal(view)
	require.NoError(t, err)
	for _, value := range []string{"session-must-not-leak", "config-must-not-leak", "result-must-not-leak", "agent-error-must-not-leak"} {
		require.NotContains(t, string(payload), value)
	}
	for _, field := range []string{`"config":`, `"result":`, `"session_id":`, `"last_error":`} {
		require.NotContains(t, string(payload), field)
	}
}

func TestPublicPluginInstallationHidesRawExecutorError(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	installation := model.PluginInstallation{
		ID: 9, PluginID: "nftables-forward", Target: "agent", DesiredVersion: "1.2.0",
		State: "failed", Enabled: true, LifecycleGeneration: 3, ConfigRevision: 4,
		LastError: "agent runtime token must-not-leak", CreatedAt: now, UpdatedAt: now,
	}

	view := PublicPluginInstallation(installation)
	require.True(t, view.HasError)
	require.Equal(t, "plugin installation failed", view.LastError)
	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "agent runtime token must-not-leak")
	require.Contains(t, string(payload), `"last_error":"plugin installation failed"`)
}
