package nodeopsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// Configuration push on the Agent Control stream (A2-3,
// node-ops-service.md section 5.5): ConfigSnapshot from the desired
// configuration, the Hello reconcile, and node.sync ending on the agent's
// ConfigStatus.

// configCapabilities is what an agent with config.v1 advertises.
func configCapabilities() []string {
	return append(append([]string(nil), fakeagent.DefaultCapabilities...), agentcontrol.CapabilityConfig)
}

// configStatus returns the node's recorded configuration status.
func (f *fixture) configStatus(node agentcontrol.AgentNode) (model.KernelNodeConfigStatus, bool) {
	f.t.Helper()
	row, found, err := kernelnodeops.LoadConfigStatus(context.Background(), f.db, node)
	require.NoError(f.t, err)
	return row, found
}

// awaitConfigStatus polls until the node's recorded status matches.
func (f *fixture) awaitConfigStatus(node agentcontrol.AgentNode, match func(model.KernelNodeConfigStatus) bool) model.KernelNodeConfigStatus {
	f.t.Helper()
	deadline := time.Now().Add(awaitTimeout)
	var last model.KernelNodeConfigStatus
	for time.Now().Before(deadline) {
		row, found := f.configStatus(node)
		if found && match(row) {
			return row
		}
		last = row
		time.Sleep(settleInterval)
	}
	require.Failf(f.t, "configuration status not reached", "last: %+v", last)
	return last
}

func hasCapability(capabilities []*agentv1pb.Capability, name string) bool {
	return agentcontrol.HasCapabilityVersion(capabilities, name, agentcontrol.CapabilityVersionV1)
}

// config.v1 is negotiated only with an agent that lists it, for proxy and
// forward nodes. An agent without it keeps getting node.reload and no
// snapshot; one with it gets a snapshot, and node.sync pushes the snapshot
// instead of node.reload and ends on the agent's ConfigStatus.
func TestConfigPushNegotiation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		client := f.client()

		old := f.proxyAgent(fakeagent.Script{})
		assert.False(t, hasCapability(old.HelloAck.GetServerCapabilities(), agentcontrol.CapabilityConfig), "an agent without config.v1 is not offered it")
		reload := terminal(t, client, "node.sync:old", syncSpec(f.proxyNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, reload.GetState(), reload.GetError())
		require.Len(t, old.Received(), 1)
		assert.Equal(t, kernelnodeops.NodeReloadOperation, old.Received()[0].GetKind(), "an old agent still gets node.reload")
		assert.Empty(t, old.Snapshots())
		old.Disconnect()

		agent := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities()})
		assert.True(t, hasCapability(agent.HelloAck.GetServerCapabilities(), agentcontrol.CapabilityConfig))
		require.Len(t, agent.Snapshots(), 1, "the Hello reconcile sent the desired configuration")
		row, found := f.desired(f.proxyNode())
		require.True(t, found)
		assert.Equal(t, row.Revision, agent.Snapshots()[0].GetConfigRevision())
		assert.Equal(t, row.ConfigHash, agent.Snapshots()[0].GetConfigHash())
		assert.Equal(t, kernelnodeops.DesiredConfigFormat, agent.Snapshots()[0].GetFormat())
		assert.Equal(t, row.ConfigJSON, string(agent.Snapshots()[0].GetConfigJson()))

		synced := terminal(t, client, "node.sync:config", syncSpec(f.proxyNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, synced.GetState(), synced.GetError())
		result := synced.GetResult().GetNodeSync()
		assert.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, result.GetChannel())
		assert.Empty(t, result.GetAgentOperationId(), "no node.reload")
		assert.True(t, result.GetSnapshot(), "the configuration went out as a snapshot")
		assert.Equal(t, row.Revision, result.GetRevision())
		assert.Equal(t, row.Revision, synced.GetNodeRevision())
		require.NotNil(t, result.GetAck())
		assert.True(t, result.GetAck().GetAccepted(), "the ack is the agent's ConfigStatus")
		assert.Equal(t, agent.SessionID(), result.GetAck().GetSessionId())
		assert.Empty(t, agent.Received(), "a config.v1 agent is not sent node.reload")
		assert.Len(t, agent.Snapshots(), 2)

		status := f.awaitConfigStatus(f.proxyNode(), func(row model.KernelNodeConfigStatus) bool { return row.AppliedRevision > 0 })
		assert.Equal(t, model.ConfigVerdictApplied, status.Verdict)
		assert.Equal(t, row.Revision, status.AppliedRevision)
		assert.Equal(t, row.ConfigHash, status.AppliedHash)
		assert.Equal(t, agent.SessionID(), status.SessionID)

		// Unchanged and applied: a sync that is not forced pushes nothing.
		quiet := terminal(t, client, "node.sync:quiet", syncSpec(f.proxyNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, quiet.GetState(), quiet.GetError())
		assert.Len(t, agent.Snapshots(), 2)

		// A forward node's agent negotiates it too.
		forward := f.forwardAgent(fakeagent.Script{Capabilities: configCapabilities()})
		assert.True(t, hasCapability(forward.HelloAck.GetServerCapabilities(), agentcontrol.CapabilityConfig))
		require.Len(t, forward.Snapshots(), 1)
		assert.Contains(t, string(forward.Snapshots()[0].GetConfigJson()), `"legacy_rules"`)
		assert.NotContains(t, string(forward.Snapshots()[0].GetConfigJson()), forwardToken)
		forwardSync := terminal(t, client, "node.sync:forward-config", syncSpec(f.forwardNode(), true))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, forwardSync.GetState(), forwardSync.GetError())
		assert.Equal(t, forward.SessionID(), forwardSync.GetResult().GetNodeSync().GetAck().GetSessionId())
		assert.Empty(t, forward.Received())
	})
}

// The Hello reconcile: an agent that reports the desired revision is sent
// nothing once the kernel recorded that revision as applied, and one snapshot
// before (its ConfigStatus may have been lost with its earlier session); one
// that reports another revision, or none, is sent exactly one snapshot.
func TestConfigHelloReconcile(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		row, _, err := kernelnodeops.RefreshDesiredConfig(context.Background(), f.db, f.proxyNode(), time.Now())
		require.NoError(t, err)
		recorded := false
		for _, test := range []struct {
			name     string
			revision uint64
			applied  bool
			want     int
		}{
			{"same revision, applied never recorded", row.Revision, false, 1},
			{"older revision", row.Revision - 1, false, 1},
			{"newer revision", row.Revision + 5, false, 1},
			{"no revision", 0, false, 1},
			{"same revision, recorded as applied", row.Revision, true, 0},
		} {
			if test.applied && !recorded {
				verdict, err := kernelnodeops.RecordConfigStatus(context.Background(), f.db, f.proxyNode(), "earlier-session",
					&agentv1pb.ConfigStatus{ConfigRevision: row.Revision, ConfigHash: row.ConfigHash, Applied: true}, time.Now())
				require.NoError(t, err)
				require.Equal(t, model.ConfigVerdictApplied, verdict)
				recorded = true
			}
			agent := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities(), ConfigRevision: test.revision, HoldConfig: true})
			// connect waited for a heartbeat answer, which Control sends
			// after the reconcile.
			assert.Len(t, agent.Snapshots(), test.want, test.name)
			for _, snapshot := range agent.Snapshots() {
				assert.Equal(t, row.Revision, snapshot.GetConfigRevision(), test.name)
				assert.Equal(t, row.ConfigHash, snapshot.GetConfigHash(), test.name)
			}
			agent.Disconnect()
		}
		stored, _ := f.desired(f.proxyNode())
		assert.Equal(t, row.Revision, stored.Revision, "reconciles do not move an unchanged revision")
	})
}

// node.sync on a config.v1 agent ends on the agent's ConfigStatus: a status
// for an older revision or with another hash is recorded and does not end
// it; applied:false fails it with the agent's error; applied ends it.
func TestSyncNodeEndsOnConfigStatus(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities(), HoldConfig: true})
		_, err := agent.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err, "the Hello reconcile")
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocol.ID).Update("port", 8443).Error)
		client := f.client()

		pending, err := submit(t, client, "node.sync:status-1", syncSpec(f.proxyNode(), false), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		operationID := pending.GetOperation().GetOperationId()
		snapshot, err := agent.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err)
		running := awaitState(t, client, operationID, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING)
		assert.Equal(t, snapshot.GetConfigRevision(), running.GetNodeRevision())

		// A stale hash at the snapshot's revision, and an older revision:
		// recorded, not an answer.
		require.NoError(t, agent.SendConfigStatus(&agentv1pb.ConfigStatus{ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: strings.Repeat("0", 64), Applied: true}))
		mismatch := f.awaitConfigStatus(f.proxyNode(), func(row model.KernelNodeConfigStatus) bool { return row.Verdict == model.ConfigVerdictMismatch })
		assert.Zero(t, mismatch.AppliedRevision, "a mismatched hash is never applied")
		require.NoError(t, agent.SendConfigStatus(&agentv1pb.ConfigStatus{ConfigRevision: snapshot.GetConfigRevision() - 1, ConfigHash: snapshot.GetConfigHash(), Applied: true}))
		stale := f.awaitConfigStatus(f.proxyNode(), func(row model.KernelNodeConfigStatus) bool { return row.Verdict == model.ConfigVerdictStale })
		assert.Zero(t, stale.AppliedRevision)
		still, err := client.GetOperation(context.Background(), &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: operationID}})
		require.NoError(t, err)
		assert.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, still.GetOperation().GetState(), "neither status ends the operation")

		// The agent could not apply it: FAILED with its error.
		require.NoError(t, agent.SendConfigStatus(&agentv1pb.ConfigStatus{
			ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: snapshot.GetConfigHash(), Error: "xray refused the configuration",
			ErrorCode: agentcontrol.ConfigErrorCodeApplyFailed,
		}))
		failed := awaitState(t, client, operationID, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED)
		assert.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, failed.GetError().GetCode())
		assert.Equal(t, "config_apply_failed: xray refused the configuration", failed.GetError().GetMessage(), "the agent's code leads the message")
		assert.False(t, failed.GetResult().GetNodeSync().GetAck().GetAccepted())
		row, _ := f.configStatus(f.proxyNode())
		assert.Equal(t, model.ConfigVerdictFailed, row.Verdict)
		assert.Equal(t, "xray refused the configuration", row.ReportedError)
		assert.Equal(t, agentcontrol.ConfigErrorCodeApplyFailed, row.ReportedErrorCode)
		assert.Zero(t, row.AppliedRevision)

		// Forced again, and applied: SUCCEEDED, the applied revision moves.
		pending, err = submit(t, client, "node.sync:status-2", syncSpec(f.proxyNode(), true), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		snapshot, err = agent.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err)
		require.NoError(t, agent.SendConfigStatus(&agentv1pb.ConfigStatus{ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: snapshot.GetConfigHash(), Applied: true}))
		succeeded := awaitState(t, client, pending.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
		assert.True(t, succeeded.GetResult().GetNodeSync().GetAck().GetAccepted())
		row, _ = f.configStatus(f.proxyNode())
		assert.Equal(t, snapshot.GetConfigRevision(), row.AppliedRevision)
		assert.Equal(t, snapshot.GetConfigHash(), row.AppliedHash)
		lagging, err := kernelnodeops.ConfigLaggingNodes(context.Background(), f.db)
		require.NoError(t, err)
		assert.Zero(t, lagging)
	})
}

// A sync that is not forced, of a configuration that did not change, still
// pushes it to an agent that has not applied it.
func TestSyncNodePushesToALaggingAgent(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities(), HoldConfig: true})
		hello, err := agent.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err)
		client := f.client()
		pending, err := submit(t, client, "node.sync:lagging", syncSpec(f.proxyNode(), false), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		snapshot, err := agent.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err, "the agent has not applied the stored configuration")
		assert.Equal(t, hello.GetConfigRevision(), snapshot.GetConfigRevision())
		require.NoError(t, agent.SendConfigStatus(&agentv1pb.ConfigStatus{ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: snapshot.GetConfigHash(), Applied: true}))
		succeeded := awaitState(t, client, pending.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
		assert.False(t, succeeded.GetResult().GetNodeSync().GetChanged())
	})
}

// An agent that reconnects while a node.sync waits for its ConfigStatus is
// sent the snapshot again by the Hello reconcile, and its answer on the new
// session ends the operation.
func TestSyncNodeSurvivesAReconnect(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		first := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities(), HoldConfig: true})
		helloSnapshot, err := first.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err)
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocol.ID).Update("port", 9443).Error)
		client := f.client()
		pending, err := submit(t, client, "node.sync:reconnect", syncSpec(f.proxyNode(), false), kernelnodeopsv1.WaitMode_WAIT_MODE_NONE)
		require.NoError(t, err)
		snapshot, err := first.AwaitSnapshot(awaitTimeout)
		require.NoError(t, err)
		awaitState(t, client, pending.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING)
		first.Disconnect()

		// The agent comes back still at the configuration it had.
		second := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities(), ConfigRevision: helloSnapshot.GetConfigRevision()})
		require.Len(t, second.Snapshots(), 1)
		assert.Equal(t, snapshot.GetConfigRevision(), second.Snapshots()[0].GetConfigRevision())
		succeeded := awaitState(t, client, pending.GetOperation().GetOperationId(), kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED)
		assert.Equal(t, second.SessionID(), succeeded.GetResult().GetNodeSync().GetAck().GetSessionId())
	})
}

// uniProxyAnswer is the UniProxy configuration answer for the node.
func uniProxyAnswer(t *testing.T, nodeID uint, nodeType string) (int, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	target := "/api/v1/server/UniProxy/config?node_id=" + strconv.FormatUint(uint64(nodeID), 10)
	if nodeType != "" {
		target += "&node_type=" + nodeType
	}
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	handler.NewUniProxyHandler().GetConfig(c)
	return recorder.Code, recorder.Body.Bytes()
}

// canonical decodes JSON into a generic value, for equality.
func canonical(t *testing.T, raw []byte) any {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}

func reencode(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

// v2boardAnswer is the v2board NodeService.GetConfig answer for the node
// and node type, through the real listener with the node's key.
func (f *fixture) v2boardAnswer(t *testing.T, nodeType string) *pb.NodeConfigResponse {
	t.Helper()
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-node-id", strconv.FormatUint(uint64(f.proxy.ID), 10), "x-api-key", proxyKey, "x-node-type", nodeType)
	response, err := pb.NewNodeServiceClient(f.control.Dial(t, nil)).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(f.proxy.ID)}) // #nosec G115 -- a test id.
	require.NoError(t, err)
	return response
}

// The snapshot carries exactly what the legacy pulls give the node:
// legacy_pull holds the UniProxy answer for no node type and for each type
// the node serves, byte for byte as JSON; each protocol's config is what
// the v2board GetConfig of that type renders. Both with protocols only and
// with a raw configuration (which UniProxy answers instead of the
// protocols, and GetConfig ignores). A walk of every secret-named key of
// the snapshot finds no value that one of those pulls does not send, and
// the node's credentials never appear.
func TestConfigSnapshotParityWithTheLegacyPull(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		agent := f.proxyAgent(fakeagent.Script{Capabilities: configCapabilities()})
		require.Len(t, agent.Snapshots(), 1)
		check := func(snapshot *agentv1pb.ConfigSnapshot, types []string) {
			t.Helper()
			var document map[string]any
			require.NoError(t, json.Unmarshal(snapshot.GetConfigJson(), &document))
			pull, ok := document["legacy_pull"].(map[string]any)
			require.True(t, ok, "the snapshot carries the legacy pull")
			var legacy []string
			code, body := uniProxyAnswer(t, f.proxy.ID, "")
			require.Equal(t, http.StatusOK, code, string(body))
			assert.Equal(t, canonical(t, body), canonical(t, reencode(t, pull["default"])), "default")
			legacy = append(legacy, string(body))
			byType, _ := pull["types"].(map[string]any)
			assert.Len(t, byType, len(types))
			for _, nodeType := range types {
				code, body := uniProxyAnswer(t, f.proxy.ID, nodeType)
				require.Equal(t, http.StatusOK, code, string(body))
				assert.Equal(t, canonical(t, body), canonical(t, reencode(t, byType[nodeType])), nodeType)
				legacy = append(legacy, string(body))
			}
			// Each protocol's config against the v2board GetConfig.
			protocols, _ := document["protocols"].([]any)
			require.Len(t, protocols, 1)
			config := protocols[0].(map[string]any)["config"].(map[string]any)
			response := f.v2boardAnswer(t, config["node_type"].(string))
			legacy = append(legacy, response.String())
			assert.Equal(t, config["node_type"], response.GetNodeType())
			assert.Equal(t, config["host"], response.GetHost())
			assert.Equal(t, config["server_name"], response.GetServerName())
			assert.Equal(t, config["network"], response.GetNetwork())
			assert.Equal(t, config["flow"], response.GetFlow())
			assert.EqualValues(t, config["server_port"], response.GetServerPort())
			assert.EqualValues(t, config["tls"], response.GetTls())
			tlsSettings, _ := config["tls_settings"].(map[string]any)
			assert.Equal(t, service.StringifyConfigMap(tlsSettings), response.GetTlsSettings())

			// Secret walk: every value under a secret-named key is one a
			// legacy pull sends the node.
			walkSecrets(document, func(key, value string) {
				found := false
				for _, body := range legacy {
					if strings.Contains(body, value) {
						found = true
					}
				}
				assert.True(t, found, "secret %q of the snapshot is not in the legacy pull", key)
			})
			for _, credential := range []string{proxyKey, forwardToken, f.proxy.Secret} {
				if credential != "" {
					assert.NotContains(t, string(snapshot.GetConfigJson()), credential)
				}
			}
		}
		check(agent.Snapshots()[0], []string{"vless"})
		assert.Contains(t, string(agent.Snapshots()[0].GetConfigJson()), realityKey, "the protocol's key, as UniProxy sends it")

		// A raw configuration replaces the protocols in the UniProxy pull.
		raw := `{"type":"trojan","node_type":"trojan","server_port":7443,"password":"fake-raw-trojan-password-0123"}`
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("raw_config", raw).Error)
		synced := terminal(t, f.client(), "node.sync:raw", syncSpec(f.proxyNode(), false))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, synced.GetState(), synced.GetError())
		snapshots := agent.Snapshots()
		require.Len(t, snapshots, 2)
		check(snapshots[1], []string{"trojan"})
		assert.Contains(t, string(snapshots[1].GetConfigJson()), "fake-raw-trojan-password-0123")
	})
}

// walkSecrets calls found for every non-empty string under a key
// service.IsNodeSecretKey names.
func walkSecrets(value any, found func(key, value string)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if text, ok := child.(string); ok && text != "" && service.IsNodeSecretKey(key) {
				found(key, text)
				continue
			}
			walkSecrets(child, found)
		}
	case []any:
		for _, child := range typed {
			walkSecrets(child, found)
		}
	}
}
