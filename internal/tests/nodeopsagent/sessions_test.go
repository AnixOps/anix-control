package nodeopsagent

import (
	"context"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
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
		client := f.client()
		ctx := context.Background()

		all, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{})
		require.NoError(t, err)
		require.Len(t, all.GetSessions(), 2)
		forward, proxyStream := all.GetSessions()[0], all.GetSessions()[1]
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
		assert.Equal(t, "spiffe://anixops/test/agent/"+f.proxyNode().String(), proxyStream.GetIdentity())

		streams, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{Transport: kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_CONTROL_STREAM})
		require.NoError(t, err)
		assert.Len(t, streams.GetSessions(), 2)
		sockets, err := client.ListAgentSessions(ctx, &kernelnodeopsv1.ListAgentSessionsRequest{Transport: kernelnodeopsv1.AgentTransport_AGENT_TRANSPORT_WEBSOCKET})
		require.NoError(t, err)
		assert.Empty(t, sockets.GetSessions(), "the WebSocket transport was retired")

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
		assert.False(t, monitor.GetFound(), "monitor snapshots came from the retired REST channel")

		// Disconnected: not connected once Control noticed the stream end.
		proxyAgent.Disconnect()
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
		client := f.client()
		ctx := context.Background()
		var answers []proto.Message
		answers = append(answers, terminal(t, client, "secrets:sync-proxy", syncSpec(f.proxyNode(), true)))
		answers = append(answers, terminal(t, client, "secrets:sync-forward", syncSpec(f.forwardNode(), true)))
		answers = append(answers, terminal(t, client, "secrets:ping", agentOperationSpec(f.proxy.ID, "agent.ping", "", 5)))
		answers = append(answers, terminal(t, client, "secrets:diag", diagnosticSpec(f.proxy.ID, "service_status", `{"service":"anix-agent"}`)))
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
			assert.NotContains(t, text, realityKey, "%T carries a credential", answer)
		}
	})
}
