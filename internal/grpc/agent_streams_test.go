package grpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// AgentStreams is the kernel's view of the Agent Control streams by node
// kind (agentstreams.Streams): proxy and forward nodes of the same id have
// sessions of their own, dispatches reach the right one, and every error
// wraps its agentstreams sentinel while keeping the manager's text.

// streamClient is a scripted agent on one open stream: it records the
// desired operations it receives and answers them as told.
type streamClient struct {
	t         *testing.T
	node      agentcontrol.AgentNode
	stream    agentv1pb.AgentControlService_ControlStreamClient
	sessionID string
	cancel    context.CancelFunc
	received  chan *agentv1pb.DesiredOperation
	ack       bool
}

// openStreamClient sends a Hello advertising capabilities and reads the
// HelloAck, then a heartbeat so the session is registered before it
// returns.
func openStreamClient(t *testing.T, ctx context.Context, conn *grpc.ClientConn, node agentcontrol.AgentNode, ack bool, capabilities ...string) *streamClient {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := &agentv1pb.Hello{Protocol: AgentProtocolVersion, AgentVersion: "streams-test", InstanceId: "instance-" + node.String()}
	for _, capability := range capabilities {
		hello.Capabilities = append(hello.Capabilities, &agentv1pb.Capability{Name: capability, Version: agentcontrol.CapabilityVersionV1})
	}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "hello", NodeId: node.ID, SentAtUnixMs: time.Now().UnixMilli(), Payload: &agentv1pb.AgentToControl_Hello{Hello: hello},
	}))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHelloAck())
	client := &streamClient{t: t, node: node, stream: stream, sessionID: message.GetHelloAck().GetSessionId(), cancel: cancel, received: make(chan *agentv1pb.DesiredOperation, 16), ack: ack}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "heartbeat", NodeId: node.ID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: client.sessionID}},
	}))
	go client.serve()
	t.Cleanup(cancel)
	return client
}

func (c *streamClient) serve() {
	for {
		message, err := c.stream.Recv()
		if err != nil {
			return
		}
		desired := message.GetDesiredOperation()
		if desired == nil {
			continue
		}
		c.received <- desired
		if !c.ack {
			continue
		}
		_ = c.stream.Send(&agentv1pb.AgentToControl{
			RequestId: "ack-" + desired.GetOperationId(), NodeId: c.node.ID, Revision: desired.GetRevision(), SentAtUnixMs: time.Now().UnixMilli(),
			Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
				OperationId: desired.GetOperationId(), Accepted: true, SessionId: c.sessionID, Revision: desired.GetRevision(), AcceptedAtUnixMs: time.Now().UnixMilli(),
			}},
		})
	}
}

// next returns the desired operation with id the agent receives; what the
// manager replays from earlier sessions of the same node id is skipped.
func (c *streamClient) next(id string) *agentv1pb.DesiredOperation {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case desired := <-c.received:
			if desired.GetOperationId() == id {
				return desired
			}
		case <-deadline:
			c.t.Fatalf("agent %s received no operation %q", c.node, id)
			return nil
		}
	}
}

// none requires that no operation with id reaches the agent within a
// moment.
func (c *streamClient) none(id string) {
	c.t.Helper()
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case desired := <-c.received:
			require.NotEqual(c.t, id, desired.GetOperationId(), "agent %s received %q", c.node, id)
		case <-deadline:
			return
		}
	}
}

// observe reports a terminal state for an operation.
func (c *streamClient) observe(desired *agentv1pb.DesiredOperation, phase agentv1pb.ObservedPhase) {
	c.t.Helper()
	require.NoError(c.t, c.stream.Send(&agentv1pb.AgentToControl{
		RequestId: "observed-" + desired.GetOperationId(), NodeId: c.node.ID, Revision: desired.GetRevision(), SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: &agentv1pb.ObservedState{
			OperationId: desired.GetOperationId(), Revision: desired.GetRevision(), Phase: phase, StateJson: []byte(`{"ok":true}`),
			ObservedAtUnixMs: time.Now().UnixMilli(), SessionId: c.sessionID,
		}},
	}))
}

func TestAgentStreamsDispatchToEachNodeKind(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	streams := GetAgentStreams()
	proxy, forward := l.proxyNode(), l.forwardNode()
	require.Equal(t, proxy.ID, forward.ID, "the two nodes share an id")

	// Before any session: not connected, nothing observed, no sessions of
	// these nodes.
	_, connected := streams.Session(proxy)
	assert.False(t, connected)
	_, observed := streams.Observed(forward)
	assert.False(t, observed)
	_, err := streams.Dispatch(ctx, proxy, &agentv1pb.DesiredOperation{OperationId: "none", Kind: "agent.ping"})
	require.ErrorIs(t, err, agentstreams.ErrNotConnected)
	assert.Equal(t, fmt.Sprintf("agent node %d is not connected", proxy.ID), err.Error(), "the manager's text is kept")

	proxyClient := openStreamClient(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), l.dial(t, nil), proxy, true, "agent.ping", "operation.cancel")
	certificate, _, err := enrollForTest(t, nodeCredentials(ctx, forward, l.forwardToken), l.dial(t, nil), "")
	require.NoError(t, err)
	forwardClient := openStreamClient(t, ctx, l.dial(t, certificate), forward, true, "agent.ping", "node.reload", "operation.cancel")

	// Sessions, with the identity each authenticated by.
	proxySession, ok := streams.Session(proxy)
	require.True(t, ok)
	assert.Equal(t, proxyClient.sessionID, proxySession.SessionID)
	assert.Equal(t, agentstreams.IdentityAPIKey, proxySession.Identity)
	assert.Equal(t, agentstreams.TransportControlStream, proxySession.Transport)
	assert.Equal(t, "streams-test", proxySession.AgentVersion)
	assert.ElementsMatch(t, []string{"agent.ping", "operation.cancel"}, proxySession.Capabilities)
	forwardSession, ok := streams.Session(forward)
	require.True(t, ok)
	assert.Equal(t, forwardClient.sessionID, forwardSession.SessionID)
	assert.Equal(t, "spiffe://anixops/test/agent/"+forward.String(), forwardSession.Identity)
	assert.Equal(t, forward, forwardSession.Node)
	var listed []agentcontrol.AgentNode
	for _, session := range streams.Sessions() {
		if session.Node.ID == proxy.ID {
			listed = append(listed, session.Node)
		}
	}
	assert.ElementsMatch(t, []agentcontrol.AgentNode{proxy, forward}, listed)

	// Observed states fan out with their node kind.
	var mu sync.Mutex
	var seen []agentcontrol.AgentNode
	streams.OnObserved(func(node agentcontrol.AgentNode, observed *agentv1pb.ObservedState) {
		mu.Lock()
		defer mu.Unlock()
		if observed.GetOperationId() == "ping-forward" {
			seen = append(seen, node)
		}
	})
	streams.OnObserved(nil)

	// A dispatch to the forward node reaches its session, not the proxy's.
	ack, err := streams.Dispatch(ctx, forward, &agentv1pb.DesiredOperation{OperationId: "ping-forward", Kind: "agent.ping"})
	require.NoError(t, err)
	assert.True(t, ack.GetAccepted())
	assert.Equal(t, forwardClient.sessionID, ack.GetSessionId())
	desired := forwardClient.next("ping-forward")
	proxyClient.none("ping-forward")
	forwardClient.observe(desired, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == 1
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, forward, seen[0])
	state, ok := streams.Observed(forward)
	require.True(t, ok)
	assert.Equal(t, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, state.GetPhase())
	_, ok = streams.Observed(proxy)
	assert.False(t, ok, "the proxy node of the same id observed nothing")

	// A dispatch to the proxy node, then its cancellation.
	ack, err = streams.Dispatch(ctx, proxy, &agentv1pb.DesiredOperation{OperationId: "ping-proxy", Kind: "agent.ping"})
	require.NoError(t, err)
	assert.Equal(t, proxyClient.sessionID, ack.GetSessionId())
	desired = proxyClient.next("ping-proxy")
	require.NoError(t, streams.Cancel(ctx, proxy, desired.GetOperationId(), desired.GetRevision()))
	cancellation := proxyClient.next("ping-proxy")
	assert.Equal(t, "operation.cancel", cancellation.GetKind())
	forwardClient.none("ping-proxy")
	proxyClient.observe(desired, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED)

	// A cancellation of an operation the kernel no longer holds is still
	// delivered at the revision given (the recovery after a restart); a
	// kind the proxy session does not advertise is refused.
	require.NoError(t, streams.Cancel(ctx, proxy, "forgotten", desired.GetRevision()+1))
	assert.Equal(t, "operation.cancel", proxyClient.next("forgotten").GetKind())
	_, err = streams.Dispatch(ctx, proxy, &agentv1pb.DesiredOperation{OperationId: "reload-proxy", Kind: "node.reload"})
	require.ErrorIs(t, err, agentstreams.ErrCapabilityMissing)
	assert.Equal(t, fmt.Sprintf("agent node %d does not advertise capability %q", proxy.ID, "node.reload"), err.Error())
}

func TestAgentStreamsReplacedSessionClosesThePendingDispatch(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, false)
	ctx := testContext(t)
	streams := GetAgentStreams()
	proxy := l.proxyNode()

	silent := openStreamClient(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), l.dial(t, nil), proxy, false, "agent.ping")
	result := make(chan error, 1)
	go func() {
		_, err := streams.Dispatch(ctx, proxy, &agentv1pb.DesiredOperation{OperationId: "pending", Kind: "agent.ping"})
		result <- err
	}()
	silent.next("pending")

	// A newer session for the same node replaces the first: the pending
	// dispatch ends without an acknowledgement.
	replacement := openStreamClient(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), l.dial(t, nil), proxy, true, "agent.ping")
	select {
	case err := <-result:
		require.ErrorIs(t, err, agentstreams.ErrSessionClosed)
		assert.Equal(t, "agent connection closed before operation ACK", err.Error())
	case <-ctx.Done():
		t.Fatal("the pending dispatch did not end")
	}
	session, ok := streams.Session(proxy)
	require.True(t, ok)
	assert.Equal(t, replacement.sessionID, session.SessionID)

	// The replacement is replayed the retained operation; an operation id
	// still pending on it is refused as such.
	replayed := replacement.next("pending")
	_, err := streams.Dispatch(ctx, proxy, &agentv1pb.DesiredOperation{OperationId: "pending", Kind: "agent.ping"})
	require.ErrorIs(t, err, agentstreams.ErrOperationPending)
	assert.Contains(t, err.Error(), `"pending" is already desired`)
	replacement.observe(replayed, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED)

	// The context's own end is passed through unwrapped.
	ended, cancel := context.WithCancel(ctx)
	cancel()
	_, err = streams.Dispatch(ended, proxy, &agentv1pb.DesiredOperation{OperationId: "late", Kind: "agent.ping"})
	require.ErrorIs(t, err, context.Canceled)
	var wrapped classified
	assert.False(t, errors.As(err, &wrapped))
}

// failingStream is a registered connection whose sends fail.
type failingStream struct {
	grpc.ServerStream
}

func (failingStream) Send(*agentv1pb.ControlToAgent) error {
	return errors.New("transport: broken pipe")
}
func (failingStream) Recv() (*agentv1pb.AgentToControl, error) {
	return nil, errors.New("transport: broken pipe")
}

func TestAgentStreamsSendFailuresAndUnknownKinds(t *testing.T) {
	proxyManager, forwardManager := NewAgentControlManager(), NewAgentControlManager()
	streams := NewAgentStreams(proxyManager, forwardManager)
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 7}
	connection := &AgentControlConnection{
		NodeID: node.ID, SessionID: "session-broken", AgentVersion: "1.0.0", InstanceID: "broken",
		Capabilities: []*agentv1pb.Capability{{Name: "agent.ping", Version: "v1"}, {Name: "operation.cancel", Version: "v1"}},
		ConnectedAt:  time.Now(), LastSeen: time.Now(), stream: failingStream{},
	}
	proxyManager.register(connection)
	t.Cleanup(func() { proxyManager.unregister(connection) })

	session, ok := streams.Session(node)
	require.True(t, ok)
	assert.Equal(t, agentstreams.IdentityAPIKey, session.Identity, "a connection without an identity is an API key session")
	assert.Equal(t, []string{"agent.ping", "operation.cancel"}, session.Capabilities)

	// A send that fails is the manager's error, unclassified.
	_, err := streams.Dispatch(context.Background(), node, &agentv1pb.DesiredOperation{OperationId: "broken", Kind: "agent.ping"})
	require.Error(t, err)
	assert.Equal(t, "send desired operation: transport: broken pipe", err.Error())
	for _, sentinel := range []error{agentstreams.ErrNotConnected, agentstreams.ErrCapabilityMissing, agentstreams.ErrSessionClosed, agentstreams.ErrOperationPending} {
		assert.False(t, errors.Is(err, sentinel), "%v", sentinel)
	}

	// A node kind without a manager is not connected.
	unknown := agentcontrol.AgentNode{Kind: "satellite", ID: 7}
	_, ok = streams.Session(unknown)
	assert.False(t, ok)
	_, ok = streams.Observed(unknown)
	assert.False(t, ok)
	_, err = streams.Dispatch(context.Background(), unknown, &agentv1pb.DesiredOperation{OperationId: "x", Kind: "agent.ping"})
	require.ErrorIs(t, err, agentstreams.ErrNotConnected)
	assert.Equal(t, "agent node satellite-7 has no stream of its kind", err.Error())
	err = streams.Cancel(context.Background(), unknown, "x", 1)
	require.ErrorIs(t, err, agentstreams.ErrNotConnected)
	err = streams.Cancel(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 7}, "x", 1)
	require.ErrorIs(t, err, agentstreams.ErrNotConnected)
	assert.Equal(t, "agent node 7 is not connected", err.Error())
	assert.Empty(t, NewAgentStreams(nil, nil).Sessions())
	NewAgentStreams(nil, nil).OnObserved(func(agentcontrol.AgentNode, *agentv1pb.ObservedState) {})
}

func TestClassifyKeepsTheManagersText(t *testing.T) {
	assert.NoError(t, classify(nil))
	for text, sentinel := range map[string]error{
		"agent node 3 is not connected":                           agentstreams.ErrNotConnected,
		"agent node 3 connection changed during cancellation":     agentstreams.ErrNotConnected,
		`agent node 3 does not advertise capability "agent.ping"`: agentstreams.ErrCapabilityMissing,
		"agent connection closed before operation ACK":            agentstreams.ErrSessionClosed,
		`operation "a" is already pending`:                        agentstreams.ErrOperationPending,
		`operation "a" is already desired`:                        agentstreams.ErrOperationPending,
	} {
		err := classify(errors.New(text))
		require.ErrorIs(t, err, sentinel, text)
		assert.Equal(t, text, err.Error())
		var wrapped classified
		require.True(t, errors.As(err, &wrapped))
		assert.Equal(t, text, errors.Unwrap(wrapped).Error())
	}
	other := errors.New("revision 2 is not newer than 3")
	assert.Same(t, other, classify(other))
	assert.ErrorIs(t, classify(context.DeadlineExceeded), context.DeadlineExceeded)
}
