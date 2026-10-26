package kernelnodeops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
)

// The Reality target probe of the agent.diagnostic operation
// (sdk/api/agent/v1/PROTOCOL.md, "Reality target probe"): a proxy node's
// Agent that lists the capability reality.probe resolves a host name with
// the node's resolver, completes a TLS handshake and sends GET /, and
// answers whether the site can serve as a REALITY target from that node.
const (
	RealityProbeAction     = "reality.probe"
	RealityProbeCapability = "reality.probe"
)

// ErrRealityProbeUnavailable is the error of a probe the node's Agent
// cannot run: not connected, or it does not list reality.probe.
var ErrRealityProbeUnavailable = errors.New("the node's Agent does not run reality.probe")

// RealityProbes runs reality.probe on the Agent Control stream of a proxy
// node. Like the forward checks, a probe is not an administrator task: it
// is not recorded in v2_agent_diagnostic_task; the caller audits the call.
type RealityProbes struct {
	// Sources are the live sessions; DefaultAgentSources when nil.
	Sources SourcesFunc
}

func (p RealityProbes) streams() agentstreams.Streams {
	if p.Sources == nil {
		return DefaultAgentSources().Streams
	}
	return p.Sources().Streams
}

// RealityProbeSupport says whether a node's Agent runs the probe now.
type RealityProbeSupport struct {
	Connected    bool   `json:"connected"`
	Capable      bool   `json:"capable"`
	AgentVersion string `json:"agent_version,omitempty"`
}

// Support answers whether node's Agent is connected and runs the probe.
func (p RealityProbes) Support(node agentcontrol.AgentNode) RealityProbeSupport {
	streams := p.streams()
	if streams == nil {
		return RealityProbeSupport{}
	}
	session, ok := streams.Session(node)
	if !ok {
		return RealityProbeSupport{}
	}
	return RealityProbeSupport{
		Connected: true, AgentVersion: session.AgentVersion,
		Capable: hasCapability(session.Capabilities, AgentDiagnosticOperation) && hasCapability(session.Capabilities, RealityProbeCapability),
	}
}

// Probe runs reality.probe for serverName:port on node, the Agent bounding
// it by timeout, and answers the Agent's result (the probe's JSON) until
// ctx ends.
func (p RealityProbes) Probe(ctx context.Context, node agentcontrol.AgentNode, serverName string, port int, timeout time.Duration) (json.RawMessage, error) {
	streams := p.streams()
	if streams == nil {
		return nil, ErrRealityProbeUnavailable
	}
	if !p.Support(node).Capable {
		return nil, ErrRealityProbeUnavailable
	}
	timeoutMS := timeout.Milliseconds()
	deadline := time.Now().Add(timeout + 2*time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	id := newRealityProbeID()
	params := map[string]any{"server_name": serverName, "port": port, "timeout_ms": timeoutMS}
	task := agentstreams.DiagnosticTask{ID: id, Type: AgentDiagnosticTaskType, Action: RealityProbeAction, Params: params, Timeout: int((timeoutMS + 999) / 1000)}
	payload, err := json.Marshal(map[string]any{"task": task})
	if err != nil {
		return nil, fmt.Errorf("encode reality probe: %w", err)
	}
	desired := &agentv1pb.DesiredOperation{OperationId: id, Kind: AgentDiagnosticOperation, PayloadJson: payload, DeadlineUnixMs: deadline.UnixMilli()}
	dispatch := dispatchOnStream(ctx, streams, node, desired, min(forwardCheckAckTimeout, time.Until(deadline)))
	if dispatch.Err != nil {
		if errors.Is(dispatch.Err, agentstreams.ErrNotConnected) || errors.Is(dispatch.Err, agentstreams.ErrCapabilityMissing) ||
			errors.Is(dispatch.Err, agentstreams.ErrSessionClosed) {
			return nil, fmt.Errorf("%w: %v", ErrRealityProbeUnavailable, dispatch.Err)
		}
		return nil, dispatch.Err
	}
	if !dispatch.Ack.GetAccepted() {
		dispatch.release()
		message := dispatch.Ack.GetError()
		if message == "" {
			message = "the Agent rejected the probe"
		}
		return nil, errors.New(message)
	}
	observed, err := dispatch.awaitTerminal(ctx, streams, node)
	if err != nil {
		return nil, err
	}
	return decodeRealityProbe(observed)
}

// decodeRealityProbe reads the probe's result from the Agent's terminal
// state.
func decodeRealityProbe(observed *agentv1pb.ObservedState) (json.RawMessage, error) {
	switch observed.GetPhase() {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return nil, errors.New("the Agent superseded the probe")
	default:
		if observed.GetMessage() == agentOperationDeadlineText {
			return nil, context.DeadlineExceeded
		}
		message := observed.GetMessage()
		if message == "" {
			message = "the Agent reported that the probe failed"
		}
		return nil, errors.New(message)
	}
	var state struct {
		Error  string          `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(observed.GetStateJson(), &state); err != nil {
		return nil, fmt.Errorf("the Agent's answer is not a diagnostic result: %w", err)
	}
	if len(state.Result) == 0 || string(state.Result) == "null" {
		message := state.Error
		if message == "" {
			message = "the Agent answered without a probe result"
		}
		return nil, errors.New(message)
	}
	return state.Result, nil
}

func newRealityProbeID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "rprobe-" + hex.EncodeToString(raw[:])
}
