package nodeopsagent

import (
	"context"
	"strings"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The session RPCs answer the live sessions of both transports and both
// node kinds, with the identity each authenticated by and never a
// credential; they need the agents capability.
func TestAgentSessionRPCs(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		proxyAgent := f.proxyAgent(fakeagent.Script{AgentVersion: "2.0.0-proxy"})
		forwardAgent := f.forwardAgent(fakeagent.Script{AgentVersion: "2.0.0-forward"})
		f.websockets.connect(f.proxy.ID, acknowledged)
		f.websockets.monitor(f.proxy.ID, map[string]any{"cpu": 12.5, "token": "monitor-secret-0123456789"})
		client := f.client()
		ctx := context.Background()

		all, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{})
		require.NoError(t, err)
		require.Len(t, all.GetSessions(), 3)
		forward, proxyStream, proxyWS := all.GetSessions()[0], all.GetSessions()[1], all.GetSessions()[2]
		assert.Equal(t, kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, forward.GetNode().GetKind())
		assert.Equal(t, uint64(f.forward.ID), forward.GetNode().GetId())
		assert.Equal(t, kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM, forward.GetTransport())
		assert.Equal(t, forwardAgent.SessionID(), forward.GetSessionId())
		assert.Equal(t, "2.0.0-forward", forward.GetAgentVersion())
		assert.Equal(t, "spiffe://anixops/test/agent/"+f.forwardNode().String(), forward.GetIdentity())
		assert.Contains(t, forward.GetCapabilities(), "node.reload")
		assert.NotZero(t, forward.GetConnectedAtUnixMs())

		assert.Equal(t, kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, proxyStream.GetNode().GetKind())
		assert.Equal(t, kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM, proxyStream.GetTransport())
		assert.Equal(t, proxyAgent.SessionID(), proxyStream.GetSessionId())
		assert.Equal(t, "api-key", proxyStream.GetIdentity())
		assert.Equal(t, kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET, proxyWS.GetTransport())
		assert.Equal(t, "ws-agent", proxyWS.GetAgentVersion())
		assert.Contains(t, string(proxyWS.GetSystemJson()), `"os":"linux"`)
		assert.Contains(t, string(proxyWS.GetSystemJson()), `"password":"`+service.NodeSecretPlaceholder+`"`, "what an agent reports is scrubbed")

		streams, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{Transport: kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM})
		require.NoError(t, err)
		assert.Len(t, streams.GetSessions(), 2)
		sockets, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{Transport: kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET})
		require.NoError(t, err)
		assert.Len(t, sockets.GetSessions(), 1)

		// GetAgentSession: the proxy node's stream, with the last observed
		// state once the agent reported one.
		session, err := client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(f.proxy.ID)})
		require.NoError(t, err)
		assert.True(t, session.GetConnected())
		assert.Equal(t, proxyAgent.SessionID(), session.GetSession().GetSessionId())
		assert.Nil(t, session.GetObserved())
		ping := terminal(t, client, "agent.op:observed", agentOperationSpec(f.proxy.ID, "agent.ping", "observed-1", 5))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, ping.GetState(), ping.GetError())
		session, err = client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(f.proxy.ID)})
		require.NoError(t, err)
		require.NotNil(t, session.GetObserved())
		assert.Equal(t, "observed-1", session.GetObserved().GetAgentOperationId())
		assert.Equal(t, "SUCCEEDED", session.GetObserved().GetPhase())
		assert.Equal(t, uint64(1), session.GetObserved().GetRevision())
		assert.Equal(t, uint64(1), session.GetSession().GetObservedRevision())

		_, err = client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: 999})
		assert.Equal(t, codes.NotFound, status.Code(err))
		_, err = client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		monitor, err := client.GetAgentMonitor(ctx, &kernelnodeopsv1.GetAgentMonitorRequest{NodeId: uint64(f.proxy.ID)})
		require.NoError(t, err)
		assert.True(t, monitor.GetFound())
		assert.NotZero(t, monitor.GetReceivedAtUnixMs())
		assert.Contains(t, string(monitor.GetSnapshotJson()), `"cpu":12.5`)
		assert.NotContains(t, string(monitor.GetSnapshotJson()), "monitor-secret")

		// Disconnected: not connected once Control noticed the stream end.
		proxyAgent.Disconnect()
		f.websockets.disconnect(f.proxy.ID)
		deadline := time.Now().Add(awaitTimeout)
		for {
			session, err = client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(f.proxy.ID)})
			require.NoError(t, err)
			if !session.GetConnected() || time.Now().After(deadline) {
				break
			}
			time.Sleep(settleInterval)
		}
		assert.False(t, session.GetConnected())
		assert.Nil(t, session.GetSession())

		// The agents family is required.
		without := f.clientWithout(grants{families: map[string]bool{service.CapabilityNodeOpsNodeConfig: true}})
		_, err = without.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = without.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(f.proxy.ID)})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = without.GetAgentMonitor(ctx, &kernelnodeopsv1.GetAgentMonitorRequest{NodeId: uint64(f.proxy.ID)})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

// GetCapabilities lists the three kinds this kernel executes.
func TestGetCapabilitiesListsTheKinds(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		capabilities, err := f.client().GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		assert.Equal(t, []string{kernelnodeops.KindAgentDiagnostic, kernelnodeops.KindAgentOperation, kernelnodeops.KindNodeSync}, capabilities.GetKinds())
	})
}

// No answer of the operations or the session RPCs carries a credential:
// the node's API key, the forward token, the protocol's Reality key or
// what an agent reported under a secret key.
func TestAnswersCarryNoSecret(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		f.proxyAgent(fakeagent.Script{})
		f.forwardAgent(fakeagent.Script{})
		f.websockets.connect(f.proxy.ID, acknowledged)
		f.websockets.monitor(f.proxy.ID, map[string]any{"api_key": proxyKey, "token": forwardToken, "private_key": realityKey})
		client := f.client()
		ctx := context.Background()
		var answers []proto.Message
		answers = append(answers, terminal(t, client, "secrets:sync-proxy", syncSpec(f.proxyNode(), true)))
		answers = append(answers, terminal(t, client, "secrets:sync-forward", syncSpec(f.forwardNode(), true)))
		answers = append(answers, terminal(t, client, "secrets:ping", agentOperationSpec(f.proxy.ID, "agent.ping", "", 5)))
		answers = append(answers, terminal(t, client, "secrets:diag", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"gost"}`)))
		sessions, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{})
		require.NoError(t, err)
		answers = append(answers, sessions)
		session, err := client.GetAgentSession(ctx, &kernelnodeopsv1.GetAgentSessionRequest{NodeId: uint64(f.proxy.ID)})
		require.NoError(t, err)
		answers = append(answers, session)
		monitor, err := client.GetAgentMonitor(ctx, &kernelnodeopsv1.GetAgentMonitorRequest{NodeId: uint64(f.proxy.ID)})
		require.NoError(t, err)
		answers = append(answers, monitor)
		listed, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{})
		require.NoError(t, err)
		answers = append(answers, listed)

		for _, answer := range answers {
			encoded, err := protojson.Marshal(answer)
			require.NoError(t, err)
			text := string(encoded)
			for _, secret := range []string{proxyKey, forwardToken, realityKey} {
				assert.NotContains(t, text, secret, "%T carries a credential", answer)
			}
		}
		assert.True(t, strings.Contains(string(monitor.GetSnapshotJson()), service.NodeSecretPlaceholder))
	})
}
