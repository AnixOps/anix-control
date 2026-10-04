package nodeopsagent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	grpcserver "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
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

// seedPluginRelease registers a signed Agent plugin release for a durable
// plugin operation.
func (f *fixture) seedPluginRelease() {
	f.t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(f.t, err)
	canonical, err := service.CanonicalPluginManifest(service.PluginManifest{
		ID: "machine-telemetry", Name: "Machine Telemetry", Version: "1.1.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: []string{"telemetry.read"},
	})
	require.NoError(f.t, err)
	_, err = service.RegisterPluginRelease(f.db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(f.t, err)
}

// One-off stream operations and durable plugin operations share the
// node's durable revision cursor: a plugin operation created after one-off
// operations is newer than them and reaches the Agent (it used to be
// refused as "revision N is not newer than M"), and an Agent reconnecting
// with a higher revision raises the cursor.
func TestOneOffAndDurableOperationsShareTheNodeRevisionCursor(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		capabilities := append(append([]string(nil), fakeagent.DefaultCapabilities...), "plugin.configure")
		agent := f.proxyAgent(fakeagent.Script{Capabilities: capabilities})
		client := f.client()

		first := pingRevision(t, client, f, "revision-ping-1")
		second := pingRevision(t, client, f, "revision-ping-2")
		require.Greater(t, second, first)
		assert.Equal(t, int64(second), f.storedRevision(), "one-off operations allocate from the durable cursor") // #nosec G115 -- a test revision.

		f.seedPluginRelease()
		deadline := time.Now().Add(time.Minute)
		operation, _, err := service.CreateKernelOperation(f.db, model.KernelOperation{
			ID: "0b7e3c52-9a41-4d6f-8e2b-5c1a9f3d7e60", IdempotencyKey: "revision-cursor-configure", NodeID: &f.proxy.ID,
			PluginID: "machine-telemetry", TargetVersion: "1.1.0", Kind: "plugin.configure",
			ConfigJSON: `{"interval_seconds":10}`, DeadlineAt: &deadline,
		})
		require.NoError(t, err)
		require.Equal(t, int64(second)+1, operation.Revision) // #nosec G115 -- a test revision.

		bridge, err := grpcserver.NewKernelOperationBridge(f.db, grpcserver.GetAgentControlManager())
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), awaitTimeout)
		defer cancel()
		count, err := bridge.RunOnce(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		desired, err := agent.Await(awaitTimeout, func(desired *agentv1pb.DesiredOperation) bool { return desired.GetOperationId() == operation.ID })
		require.NoError(t, err)
		assert.Equal(t, uint64(operation.Revision), desired.GetRevision()) // #nosec G115 -- a positive revision.
		var stored model.KernelOperation
		require.NoError(t, f.db.First(&stored, "id = ?", operation.ID).Error)
		assert.Contains(t, []string{"running", "succeeded"}, stored.State)
		assert.NotContains(t, stored.LastError, "is not newer than")

		// A reconnect reporting a revision above the cursor (an Agent that
		// outlived a Control restart) raises it.
		agent.Disconnect()
		f.proxyAgent(fakeagent.Script{Capabilities: capabilities, ObservedRevision: 40})
		assert.Equal(t, int64(40), f.storedRevision())
		assert.Equal(t, uint64(41), pingRevision(t, client, f, "revision-ping-3"))
	})
}
