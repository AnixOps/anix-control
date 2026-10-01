package kernelnodeops

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
)

// AgentSources are the live agent sessions the executors dispatch on and
// the agent session RPCs read: the Agent Control streams of every node
// kind, and the legacy WebSocket. Either may be nil in a kernel that does
// not hold it.
type AgentSources struct {
	Streams    agentstreams.Streams
	WebSockets agentstreams.WebSockets
}

// SourcesFunc returns the sources at the time of a call, so that a source
// registered after the executors were (the WebSocket handler is built with
// the router) is found when an operation runs.
type SourcesFunc func() AgentSources

var (
	defaultSourcesMu sync.RWMutex
	defaultSources   AgentSources
)

// UseAgentStreams makes streams the kernel's Agent Control streams for the
// default sources; cmd/server calls it once the listener exists.
func UseAgentStreams(streams agentstreams.Streams) {
	defaultSourcesMu.Lock()
	defer defaultSourcesMu.Unlock()
	defaultSources.Streams = streams
}

// UseWebSocketAgents makes agents the kernel's WebSocket agents for the
// default sources; the router calls it with the agent handler.
func UseWebSocketAgents(agents agentstreams.WebSockets) {
	defaultSourcesMu.Lock()
	defer defaultSourcesMu.Unlock()
	defaultSources.WebSockets = agents
}

// DefaultAgentSources returns the sources registered with UseAgentStreams
// and UseWebSocketAgents, as they are now.
func DefaultAgentSources() AgentSources {
	defaultSourcesMu.RLock()
	defer defaultSourcesMu.RUnlock()
	return defaultSources
}

// Timeouts of the stream dispatches, as the legacy routes bound them.
const (
	// DefaultAgentAckTimeout bounds the wait for an agent's acknowledgement
	// when the operation names no timeout.
	DefaultAgentAckTimeout = 10 * time.Second
	// MaxAgentAckTimeout caps the acknowledgement wait an operation asks for.
	MaxAgentAckTimeout = 60 * time.Second
	// cancelDeliveryTimeout bounds the cancellation sent to an agent when
	// an operation is cancelled while it runs.
	cancelDeliveryTimeout = 2 * time.Second
)

// agentOperationDeadlineText is how an agent reports that an operation
// passed its deadline (internal/grpc.agentOperationDeadlineText).
const agentOperationDeadlineText = "operation deadline exceeded"

// NewTaskID returns a task or operation id as the legacy routes generate
// them.
func NewTaskID() string {
	return "task-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// agentNodeOf converts a contract node reference to a stream node.
func agentNodeOf(ref *kernelnodeopsv1.NodeRef) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: nodeKindNames[ref.GetKind()], ID: uint32(ref.GetId())} // #nosec G115 -- ids are checked against maxID (32-bit).
}

func proxyAgentNode(id uint64) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)} // #nosec G115 -- ids are checked against maxID (32-bit).
}

// ackTimeout clamps an operation's acknowledgement wait.
func ackTimeout(seconds uint32) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 {
		return DefaultAgentAckTimeout
	}
	if timeout > MaxAgentAckTimeout {
		return MaxAgentAckTimeout
	}
	return timeout
}

// agentAck converts a stream acknowledgement to the contract's.
func agentAck(ack *agentv1pb.OperationAck) *kernelnodeopsv1.AgentAck {
	if ack == nil {
		return nil
	}
	return &kernelnodeopsv1.AgentAck{
		Accepted: ack.GetAccepted(), Error: ack.GetError(), AcceptedAtUnixMs: ack.GetAcceptedAtUnixMs(),
		SessionId: ack.GetSessionId(), Revision: ack.GetRevision(),
	}
}

// observedKey names the operation an agent reports on.
type observedKey struct {
	node        agentcontrol.AgentNode
	operationID string
}

// follower fans the observed states of one Streams out to the executors
// waiting for an operation's terminal state. One follower subscribes per
// Streams, once.
type follower struct {
	mu      sync.Mutex
	waiters map[observedKey][]chan *agentv1pb.ObservedState
}

var (
	followersMu sync.Mutex
	followers   = map[agentstreams.Streams]*follower{}
)

// followerFor returns the follower of streams, subscribing on first use.
func followerFor(streams agentstreams.Streams) *follower {
	followersMu.Lock()
	defer followersMu.Unlock()
	if f, ok := followers[streams]; ok {
		return f
	}
	f := &follower{waiters: map[observedKey][]chan *agentv1pb.ObservedState{}}
	followers[streams] = f
	streams.OnObserved(f.deliver)
	return f
}

// await returns a channel that receives every observed state of the
// operation until stop is called. Registering before the dispatch means
// no state is missed, however fast the agent answers.
func (f *follower) await(node agentcontrol.AgentNode, operationID string) (<-chan *agentv1pb.ObservedState, func()) {
	key := observedKey{node: node, operationID: operationID}
	ch := make(chan *agentv1pb.ObservedState, 16)
	f.mu.Lock()
	f.waiters[key] = append(f.waiters[key], ch)
	f.mu.Unlock()
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		waiters := f.waiters[key]
		for i, waiter := range waiters {
			if waiter == ch {
				f.waiters[key] = append(waiters[:i:i], waiters[i+1:]...)
				break
			}
		}
		if len(f.waiters[key]) == 0 {
			delete(f.waiters, key)
		}
	}
}

func (f *follower) deliver(node agentcontrol.AgentNode, observed *agentv1pb.ObservedState) {
	if observed == nil {
		return
	}
	f.mu.Lock()
	waiters := append([]chan *agentv1pb.ObservedState(nil), f.waiters[observedKey{node: node, operationID: observed.GetOperationId()}]...)
	f.mu.Unlock()
	for _, waiter := range waiters {
		select {
		case waiter <- observed:
		default:
			// A waiter that is 16 states behind has stopped reading.
		}
	}
}

// streamDispatch is one operation sent on a node's stream: its
// acknowledgement, and the observed states that follow it.
type streamDispatch struct {
	Desired  *agentv1pb.DesiredOperation
	Ack      *agentv1pb.OperationAck
	Err      error
	observed <-chan *agentv1pb.ObservedState
	stop     func()
}

// dispatchOnStream sends desired on the node's stream and waits for the
// acknowledgement for at most ackTimeout, or until ctx ends. The observed
// states are followed from before the send, so awaitTerminal sees every
// one.
func dispatchOnStream(ctx context.Context, streams agentstreams.Streams, node agentcontrol.AgentNode, desired *agentv1pb.DesiredOperation, timeout time.Duration) *streamDispatch {
	d := &streamDispatch{Desired: desired}
	if streams == nil {
		d.Err = errors.New("agent control manager is unavailable")
		d.stop = func() {}
		return d
	}
	d.observed, d.stop = followerFor(streams).await(node, desired.GetOperationId())
	ackCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ack, err := streams.Dispatch(ackCtx, node, desired)
	if err != nil {
		d.Err = err
		d.stop()
		return d
	}
	d.Ack = ack
	if ack.GetRevision() > 0 {
		desired.Revision = ack.GetRevision()
	}
	return d
}

// release stops following the operation; a legacy route that answers at
// the acknowledgement calls it.
func (d *streamDispatch) release() {
	if d.stop != nil {
		d.stop()
	}
}

// awaitTerminal waits for the operation's terminal observed state until
// ctx ends. When ctx is cancelled (not past its deadline) the agent is
// asked to cancel the operation, best effort.
func (d *streamDispatch) awaitTerminal(ctx context.Context, streams agentstreams.Streams, node agentcontrol.AgentNode) (*agentv1pb.ObservedState, error) {
	defer d.release()
	for {
		select {
		case observed := <-d.observed:
			if d.Ack.GetRevision() != 0 && observed.GetRevision() != d.Ack.GetRevision() {
				continue
			}
			if terminalObservedPhase(observed.GetPhase()) {
				return observed, nil
			}
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				cancelCtx, cancel := context.WithTimeout(context.Background(), cancelDeliveryTimeout)
				_ = streams.Cancel(cancelCtx, node, d.Desired.GetOperationId(), d.Desired.GetRevision())
				cancel()
			}
			return nil, ctx.Err()
		}
	}
}

func terminalObservedPhase(phase agentv1pb.ObservedPhase) bool {
	switch phase {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return true
	}
	return false
}

// streamFailure maps a dispatch error to an outcome.
func streamFailure(err error) Outcome {
	message := err.Error()
	switch {
	case errors.Is(err, agentstreams.ErrNotConnected), errors.Is(err, agentstreams.ErrSessionClosed):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, message, true)
	case errors.Is(err, agentstreams.ErrCapabilityMissing):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_CAPABILITY_MISSING, message, false)
	case errors.Is(err, context.DeadlineExceeded):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, "the agent did not acknowledge in time: "+message, true)
	case errors.Is(err, context.Canceled):
		return Cancelled("")
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, message, true)
}

// awaitFailure maps the end of a wait for the terminal state to an
// outcome: the engine records a deadline as TIMED_OUT.
func awaitFailure(err error) Outcome {
	if errors.Is(err, context.Canceled) {
		return Cancelled("the operation was cancelled while the agent ran it")
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, "the agent did not report an end before the deadline", true)
}

// rejected is the outcome of an acknowledgement that refused the operation.
func rejected(ack *agentv1pb.OperationAck) Outcome {
	message := ack.GetError()
	if message == "" {
		message = "the agent rejected the operation"
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_AGENT_REJECTED, message, true)
}

// observedOutcome maps an agent's terminal observed state to an outcome.
func observedOutcome(observed *agentv1pb.ObservedState, result *kernelnodeopsv1.OperationResult) Outcome {
	message := observed.GetMessage()
	switch observed.GetPhase() {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
		return Succeeded(result)
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		if message == "" {
			message = fmt.Sprintf("the agent superseded the operation at revision %d", observed.GetRevision())
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, message, true).WithResult(result)
	}
	if message == agentOperationDeadlineText {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, message, true).WithResult(result)
	}
	if message == "" {
		message = "the agent reported that the operation failed"
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, message, true).WithResult(result)
}
