package grpc_test

import (
	"testing"

	agentplugin "github.com/AnixOps/anix-control/sdk/plugincontrol"
	"github.com/stretchr/testify/require"
)

func TestAgentPluginPackageCrossRepoContract(t *testing.T) {
	operation := agentplugin.Operation{
		ID:             "agent-package-operation-42",
		PackageID:      "wireguard",
		PackageVersion: "4.0.0",
		Generation:     9,
		IdempotencyKey: "agent-package-key-42",
		Kind:           "configure",
		ConfigJSON:     []byte(`{"interface":"wg0"}`),
	}
	require.NoError(t, agentplugin.ValidateOperation(operation))

	status := agentplugin.RuntimeStatus{
		PackageID: operation.PackageID, PackageVersion: operation.PackageVersion,
		Generation: operation.Generation, Healthy: true, ObservedHash: "wireguard-config-sha256",
	}
	require.Equal(t, operation.PackageID, status.PackageID)
	require.Equal(t, operation.PackageVersion, status.PackageVersion)
	require.Equal(t, operation.Generation, status.Generation)
	require.True(t, status.Healthy)
}
