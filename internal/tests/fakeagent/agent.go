package fakeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// DefaultCapabilities are what an agent advertises unless its script says
// otherwise: the operations of today's agent and the diagnostic operation
// of NO-6.
var DefaultCapabilities = []string{"agent.control", "operation.cancel", "agent.ping", "node.reload", "users.reload", "agent.diagnostic"}

// Script says how the agent answers what Control sends.
type Script struct {
	// Capabilities advertised in Hello; DefaultCapabilities when nil.
	Capabilities []string
	// Ack decides the acknowledgement of a desired operation; every
	// operation is accepted when nil.
	Ack func(operation *agentv1pb.DesiredOperation) (accepted bool, reason string)
	// Hold keeps the agent from reporting a terminal state on its own: the
	// test ends the operation with Complete, or lets the deadline pass.
	Hold bool
	// Silent makes the agent receive operations without acknowledging
	// them, as an agent cut off before its answer.
	Silent bool
	// Result is the state_json of the SUCCEEDED state; {"ok":true} when
	// nil.
	Result func(operation *agentv1pb.DesiredOperation) []byte
	// ObservedRevision is what Hello reports as the agent's revision.
	ObservedRevision uint64
	// AgentVersion defaults to "fake-agent".
	AgentVersion string
	// ConfigRevision is what Hello reports as the configuration revision
	// the agent applied (config.v1).
	ConfigRevision uint64
	// Config decides the ConfigStatus answering a ConfigSnapshot: applied,
	// with the snapshot's revision and hash, when nil.
	Config func(snapshot *agentv1pb.ConfigSnapshot) (applied bool, reason string)
	// HoldConfig keeps the agent from answering snapshots on its own: the
	// test answers with SendConfigStatus.
	HoldConfig bool
}

// Agent is one scripted agent session.
type Agent struct {
	t      testing.TB
	Node   agentcontrol.AgentNode
	script Script
	stream agentv1pb.AgentControlService_ControlStreamClient
	cancel context.CancelFunc

	HelloAck *agentv1pb.HelloAck

	mu            sync.Mutex
	received      []*agentv1pb.DesiredOperation
	arrivals      chan *agentv1pb.DesiredOperation
	heartbeatAcks chan *agentv1pb.HeartbeatAck
	snapshots     []*agentv1pb.ConfigSnapshot
	configs       chan *agentv1pb.ConfigSnapshot
	sendMu        sync.Mutex
	exited        chan struct{}
	exitError     error
	sequence      int
}

// ConnectWithKey opens a stream for a proxy node authenticated by its API
// key (the legacy credential).
func ConnectWithKey(t testing.TB, conn *grpc.ClientConn, nodeID uint32, apiKey string, script Script) *Agent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ctx = metadata.AppendToOutgoingContext(ctx, "x-node-id", strconv.FormatUint(uint64(nodeID), 10), "x-api-key", apiKey)
	return connect(t, ctx, cancel, conn, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: nodeID}, script)
}

// ConnectWithCertificate opens a stream for node over a connection that
// presents its agent certificate (Control.Dial with Control.Enroll).
func ConnectWithCertificate(t testing.TB, conn *grpc.ClientConn, node agentcontrol.AgentNode, script Script) *Agent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	return connect(t, ctx, cancel, conn, node, script)
}

func connect(t testing.TB, ctx context.Context, cancel context.CancelFunc, conn *grpc.ClientConn, node agentcontrol.AgentNode, script Script) *Agent {
	t.Helper()
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	require.NoError(t, err)
	agent := &Agent{
		t: t, Node: node, script: script, stream: stream, cancel: cancel, arrivals: make(chan *agentv1pb.DesiredOperation, 64),
		heartbeatAcks: make(chan *agentv1pb.HeartbeatAck, 4), exited: make(chan struct{}),
		configs: make(chan *agentv1pb.ConfigSnapshot, 64),
	}
	capabilities := script.Capabilities
	if capabilities == nil {
		capabilities = DefaultCapabilities
	}
	hello := &agentv1pb.Hello{Protocol: agentcontrol.ProtocolV1, AgentVersion: script.AgentVersion, InstanceId: "fake-" + node.String(), ConfigRevision: script.ConfigRevision}
	if hello.AgentVersion == "" {
		hello.AgentVersion = "fake-agent"
	}
	for _, capability := range capabilities {
		hello.Capabilities = append(hello.Capabilities, &agentv1pb.Capability{Name: capability, Version: agentcontrol.CapabilityVersionV1})
	}
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "hello", NodeId: node.ID, Revision: script.ObservedRevision, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Hello{Hello: hello},
	}))
	message, err := stream.Recv()
	require.NoError(t, err)
	agent.HelloAck = message.GetHelloAck()
	require.NotNil(t, agent.HelloAck, "the first message is the HelloAck")
	go agent.serve()
	t.Cleanup(agent.Disconnect)
	// Control registers the session after it sent the HelloAck; a
	// heartbeat answered proves the session is registered and replayed.
	require.NoError(t, agent.Heartbeat())
	select {
	case <-agent.heartbeatAcks:
	case <-time.After(10 * time.Second):
		t.Fatalf("agent %s: no heartbeat acknowledgement", node)
	}
	return agent
}

// Heartbeat sends a heartbeat at the agent's observed revision.
func (a *Agent) Heartbeat() error {
	return a.send(&agentv1pb.AgentToControl{
		Revision: a.script.ObservedRevision,
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{
			SessionId: a.SessionID(), ObservedRevision: a.script.ObservedRevision,
		}},
	})
}

// SessionID is the session Control assigned.
func (a *Agent) SessionID() string { return a.HelloAck.GetSessionId() }

func (a *Agent) send(message *agentv1pb.AgentToControl) error {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	a.sequence++
	if message.RequestId == "" {
		message.RequestId = fmt.Sprintf("%s-%d", a.Node, a.sequence)
	}
	message.NodeId = a.Node.ID
	message.SentAtUnixMs = time.Now().UnixMilli()
	return a.stream.Send(message)
}

// serve answers what Control sends until the stream ends.
func (a *Agent) serve() {
	defer close(a.exited)
	for {
		message, err := a.stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				a.mu.Lock()
				a.exitError = err
				a.mu.Unlock()
			}
			return
		}
		if ack := message.GetHeartbeatAck(); ack != nil {
			select {
			case a.heartbeatAcks <- ack:
			default:
			}
			continue
		}
		if snapshot := message.GetConfig(); snapshot != nil {
			if err := a.receiveConfig(snapshot); err != nil {
				return
			}
			continue
		}
		desired := message.GetDesiredOperation()
		if desired == nil {
			continue
		}
		a.mu.Lock()
		a.received = append(a.received, proto.Clone(desired).(*agentv1pb.DesiredOperation))
		a.mu.Unlock()
		select {
		case a.arrivals <- desired:
		default:
		}
		if desired.GetKind() == "operation.cancel" {
			if err := a.ack(desired, true, ""); err != nil {
				return
			}
			if err := a.observe(desired, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED, "cancelled by Control", nil); err != nil {
				return
			}
			continue
		}
		if a.script.Silent {
			continue
		}
		accepted, reason := true, ""
		if a.script.Ack != nil {
			accepted, reason = a.script.Ack(desired)
		}
		if err := a.ack(desired, accepted, reason); err != nil {
			return
		}
		if !accepted || a.script.Hold {
			continue
		}
		if err := a.observe(desired, agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING, "", nil); err != nil {
			return
		}
		var result []byte
		if a.script.Result != nil {
			result = a.script.Result(desired)
		} else {
			result = []byte(`{"ok":true}`)
		}
		if err := a.observe(desired, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, "", result); err != nil {
			return
		}
	}
}

func (a *Agent) ack(desired *agentv1pb.DesiredOperation, accepted bool, reason string) error {
	return a.send(&agentv1pb.AgentToControl{
		Revision: desired.GetRevision(),
		Payload: &agentv1pb.AgentToControl_OperationAck{OperationAck: &agentv1pb.OperationAck{
			OperationId: desired.GetOperationId(), Accepted: accepted, Error: reason, AcceptedAtUnixMs: time.Now().UnixMilli(),
			SessionId: a.SessionID(), Revision: desired.GetRevision(),
		}},
	})
}

func (a *Agent) observe(desired *agentv1pb.DesiredOperation, phase agentv1pb.ObservedPhase, message string, state []byte) error {
	return a.send(&agentv1pb.AgentToControl{
		Revision: desired.GetRevision(),
		Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: &agentv1pb.ObservedState{
			OperationId: desired.GetOperationId(), Revision: desired.GetRevision(), Phase: phase, Message: message,
			StateJson: state, ObservedAtUnixMs: time.Now().UnixMilli(), SessionId: a.SessionID(),
		}},
	})
}

// Complete reports a terminal state for an operation the script held.
// state may be nil; a JSON value is sent as the state.
func (a *Agent) Complete(operation *agentv1pb.DesiredOperation, phase agentv1pb.ObservedPhase, message string, state any) error {
	var encoded []byte
	if state != nil {
		var err error
		if encoded, err = json.Marshal(state); err != nil {
			return err
		}
	}
	return a.observe(operation, phase, message, encoded)
}

// Report sends a raw observed state, for reports a replaced session
// would send.
func (a *Agent) Report(observed *agentv1pb.ObservedState) error {
	return a.send(&agentv1pb.AgentToControl{Revision: observed.GetRevision(), Payload: &agentv1pb.AgentToControl_ObservedState{ObservedState: observed}})
}

// receiveConfig records a snapshot and answers it per the script.
func (a *Agent) receiveConfig(snapshot *agentv1pb.ConfigSnapshot) error {
	a.mu.Lock()
	a.snapshots = append(a.snapshots, proto.Clone(snapshot).(*agentv1pb.ConfigSnapshot))
	a.mu.Unlock()
	select {
	case a.configs <- snapshot:
	default:
	}
	if a.script.HoldConfig {
		return nil
	}
	applied, reason := true, ""
	if a.script.Config != nil {
		applied, reason = a.script.Config(snapshot)
	}
	return a.SendConfigStatus(&agentv1pb.ConfigStatus{
		ConfigRevision: snapshot.GetConfigRevision(), ConfigHash: snapshot.GetConfigHash(), Applied: applied, Error: reason,
	})
}

// SendConfigStatus sends a ConfigStatus as the agent.
func (a *Agent) SendConfigStatus(status *agentv1pb.ConfigStatus) error {
	return a.send(&agentv1pb.AgentToControl{Payload: &agentv1pb.AgentToControl_ConfigStatus{ConfigStatus: status}})
}

// Snapshots returns every ConfigSnapshot the agent received, in order.
func (a *Agent) Snapshots() []*agentv1pb.ConfigSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*agentv1pb.ConfigSnapshot, len(a.snapshots))
	for i, snapshot := range a.snapshots {
		out[i] = proto.Clone(snapshot).(*agentv1pb.ConfigSnapshot)
	}
	return out
}

// AwaitSnapshot returns the next ConfigSnapshot the agent receives, within
// timeout.
func (a *Agent) AwaitSnapshot(timeout time.Duration) (*agentv1pb.ConfigSnapshot, error) {
	select {
	case snapshot := <-a.configs:
		return snapshot, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("agent %s received no configuration snapshot within %s", a.Node, timeout)
	}
}

// Received returns every desired operation the agent received, in order.
func (a *Agent) Received() []*agentv1pb.DesiredOperation {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*agentv1pb.DesiredOperation, len(a.received))
	for i, operation := range a.received {
		out[i] = proto.Clone(operation).(*agentv1pb.DesiredOperation)
	}
	return out
}

// Await returns the next desired operation that match accepts, within
// timeout.
func (a *Agent) Await(timeout time.Duration, match func(*agentv1pb.DesiredOperation) bool) (*agentv1pb.DesiredOperation, error) {
	deadline := time.After(timeout)
	for {
		select {
		case operation := <-a.arrivals:
			if match == nil || match(operation) {
				return operation, nil
			}
		case <-deadline:
			return nil, fmt.Errorf("agent %s received no matching operation within %s", a.Node, timeout)
		}
	}
}

// AwaitKind waits for the next operation of kind.
func (a *Agent) AwaitKind(timeout time.Duration, kind string) (*agentv1pb.DesiredOperation, error) {
	return a.Await(timeout, func(operation *agentv1pb.DesiredOperation) bool { return operation.GetKind() == kind })
}

// Disconnect closes the stream from the agent's side.
func (a *Agent) Disconnect() {
	a.cancel()
	<-a.exited
}

// Exited reports when and why the stream ended on the agent's side: nil
// for a clean end, else the server's status.
func (a *Agent) Exited(timeout time.Duration) (bool, error) {
	select {
	case <-a.exited:
		a.mu.Lock()
		defer a.mu.Unlock()
		return true, a.exitError
	case <-time.After(timeout):
		return false, nil
	}
}
