// Package agentstreams is the kernel's view of its live agent sessions: the
// Agent Control streams, by node kind, and the legacy agent WebSocket. It is
// the seam between the KernelNodeOps executors (internal/kernelnodeops),
// which dispatch node operations, and the listeners that hold the sessions
// (internal/grpc, internal/handler). It carries no credential: a session is
// described by its node, transport, version and revisions, never by the
// key or certificate that authenticated it.
package agentstreams

import (
	"context"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
)

// Transport is the connection an agent session uses.
type Transport string

const (
	// TransportControlStream is the Agent Control gRPC stream.
	TransportControlStream Transport = "control-stream"
	// TransportWebSocket is the legacy agent WebSocket.
	TransportWebSocket Transport = "websocket"
)

// Identity values of a session authenticated without a certificate.
const (
	// IdentityAPIKey is a stream authenticated by the node's API key or
	// forward token; a certificate's identity is its SPIFFE ID.
	IdentityAPIKey = "api-key"
)

// Session is one live agent session, without its credentials.
type Session struct {
	Node         agentcontrol.AgentNode
	Transport    Transport
	SessionID    string
	AgentVersion string
	InstanceID   string
	Capabilities []string
	ConnectedAt  time.Time
	LastSeen     time.Time
	// DesiredRevision and ObservedRevision are the stream's operation
	// revisions; zero on the WebSocket.
	DesiredRevision  uint64
	ObservedRevision uint64
	// System is what a WebSocket agent reported about its host.
	System map[string]any
	// Identity is the session's authenticated identity: the agent's SPIFFE
	// ID, or IdentityAPIKey.
	Identity string
}

// Errors a Streams implementation reports from Dispatch and Cancel. They
// are wrapped, so callers test them with errors.Is.
var (
	// ErrNotConnected: the node has no stream session.
	ErrNotConnected = errors.New("agent is not connected")
	// ErrCapabilityMissing: the session does not advertise the operation
	// kind.
	ErrCapabilityMissing = errors.New("agent does not advertise the capability")
	// ErrSessionClosed: the session ended before the agent acknowledged.
	ErrSessionClosed = errors.New("agent session closed before the acknowledgement")
	// ErrOperationPending: the operation id is already in flight on the
	// session.
	ErrOperationPending = errors.New("agent operation is already pending")
)

// ObservedHandler receives every observed state an agent reports on its
// stream, for the node that reported it.
type ObservedHandler func(node agentcontrol.AgentNode, observed *agentv1pb.ObservedState)

// Streams is the Agent Control streams of every node kind: the dispatcher
// of node-ops-service.md section 3.8 takes node kinds, since proxy and
// forward node ids overlap.
type Streams interface {
	// Session returns the node's current stream session.
	Session(node agentcontrol.AgentNode) (Session, bool)
	// Sessions lists every stream session, in no particular order.
	Sessions() []Session
	// Observed returns the last observed state the node's agent reported.
	Observed(node agentcontrol.AgentNode) (*agentv1pb.ObservedState, bool)
	// Dispatch sends a desired operation on the node's stream and waits for
	// the agent's acknowledgement, or for ctx to end. A revision of zero
	// takes the node's next one; the acknowledgement carries it.
	Dispatch(ctx context.Context, node agentcontrol.AgentNode, operation *agentv1pb.DesiredOperation) (*agentv1pb.OperationAck, error)
	// Cancel asks the agent to stop an operation it holds.
	Cancel(ctx context.Context, node agentcontrol.AgentNode, operationID string, revision uint64) error
	// OnObserved registers a handler for every observed state, of every
	// node kind.
	OnObserved(handler ObservedHandler)
}

// DiagnosticTask is one whitelisted diagnostic action as it is sent to an
// agent, on the WebSocket ("task.assign", and the legacy "task" message)
// and on the stream (an "agent.diagnostic" operation).
type DiagnosticTask struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Action  string         `json:"action"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// Ack is an agent's acknowledgement of a dispatched task, as the
// transport reported it.
type Ack struct {
	Accepted   bool
	Error      string
	AcceptedAt time.Time
	SessionID  string
	Revision   uint64
}

// DiagnosticDispatch is what a transport did with a task.
type DiagnosticDispatch struct {
	// MessageID identifies the dispatch on the transport: the WebSocket
	// message id, or the stream operation id.
	MessageID string
	// AckReceived is true when the agent acknowledged the task.
	AckReceived bool
	Ack         Ack
	// RawAck is the transport's own acknowledgement object, for answers
	// that show it as the legacy routes do.
	RawAck any
	// DispatchError is why the acknowledged dispatch failed.
	DispatchError error
	// LegacyFallback is true when, after DispatchError, the task went out
	// in the legacy task message without an acknowledgement.
	LegacyFallback bool
	// FallbackError is why the legacy fallback failed too.
	FallbackError error
}

// Sent reports whether the task reached the agent in some form.
func (d DiagnosticDispatch) Sent() bool {
	return d.DispatchError == nil || d.LegacyFallback
}

// DiagnosticTransport carries a diagnostic task to one node's agent.
type DiagnosticTransport interface {
	DispatchDiagnostic(ctx context.Context, task DiagnosticTask) DiagnosticDispatch
}

// WebSockets is the legacy agent WebSocket, as the kernel reads it.
type WebSockets interface {
	// Sessions lists the connected WebSocket agents.
	Sessions() []Session
	// Monitor returns the last monitor snapshot a proxy node's agent posted.
	Monitor(nodeID uint) (system map[string]any, receivedAt time.Time, ok bool)
	// DiagnosticTransport returns the WebSocket of a connected proxy node's
	// agent, for diagnostic tasks.
	DiagnosticTransport(nodeID uint) (DiagnosticTransport, bool)
}
