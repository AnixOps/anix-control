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
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// ForwardChecks runs the forward checks of the agent.diagnostic operation
// (service.ForwardDiagnosticChecks) for the kernel's route diagnosis
// (kernelforward.DiagnoseRoute, forward-sdk.md section 7.6): on the Agent
// Control stream of a proxy or forward node whose Agent advertises
// agent.diagnostic and diag.v1. A check is the operation as the legacy
// routes send it ({"task": task}) with a forward action; it waits for the
// Agent's terminal state and decodes the check's result from it.
//
// The checks are probes, not administrator tasks: they are not recorded in
// v2_agent_diagnostic_task (keyed by proxy node) nor in the KernelNodeOps
// ledger. The diagnosis endpoint that runs them is audited.
type ForwardChecks struct {
	// Sources are the live sessions; DefaultAgentSources when nil.
	Sources SourcesFunc
}

var _ kernelforward.NodeChecker = ForwardChecks{}

// forwardCheckAckTimeout bounds the wait for the Agent's acknowledgement.
const forwardCheckAckTimeout = 5 * time.Second

func (c ForwardChecks) streams() agentstreams.Streams {
	if c.Sources == nil {
		return DefaultAgentSources().Streams
	}
	return c.Sources().Streams
}

// Vantage answers whether node's Agent is connected and runs the checks.
func (c ForwardChecks) Vantage(node agentcontrol.AgentNode) kernelforward.NodeVantage {
	streams := c.streams()
	if streams == nil {
		return kernelforward.NodeVantage{Note: "this Control process holds no Agent Control streams"}
	}
	session, ok := streams.Session(node)
	if !ok {
		return kernelforward.NodeVantage{Note: "the node's Agent is not connected"}
	}
	if !forwardCheckCapable(session.Capabilities) {
		note := "the node's Agent does not advertise agent.diagnostic and diag.v1"
		if session.AgentVersion != "" {
			note += " (Agent " + session.AgentVersion + ")"
		}
		return kernelforward.NodeVantage{Connected: true, Note: note}
	}
	return kernelforward.NodeVantage{Connected: true, Capable: true}
}

func forwardCheckCapable(capabilities []string) bool {
	return hasCapability(capabilities, AgentDiagnosticOperation) && hasCapability(capabilities, agentcontrol.CapabilityDiag)
}

// forwardCheckState is the Agent's terminal state of a forward check: the
// generic diagnostic answer plus the check's result.
type forwardCheckState struct {
	Success bool                       `json:"success"`
	Output  string                     `json:"output"`
	Error   string                     `json:"error"`
	Result  *kernelforward.CheckResult `json:"result"`
}

// Check runs one forward check on node and waits for its result until ctx
// ends.
func (c ForwardChecks) Check(ctx context.Context, node agentcontrol.AgentNode, action string, params map[string]any) (*kernelforward.CheckResult, error) {
	normalized, err := service.ValidateForwardDiagnosticCheck(action, params)
	if err != nil {
		return nil, err
	}
	streams := c.streams()
	if streams == nil {
		return nil, kernelforward.ErrCheckUnavailable
	}
	if session, ok := streams.Session(node); !ok || !forwardCheckCapable(session.Capabilities) {
		return nil, kernelforward.ErrCheckUnavailable
	}
	timeoutMS, _ := normalized["timeout_ms"].(int64)
	deadline := time.Now().Add(time.Duration(timeoutMS)*time.Millisecond + 2*time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	id := newForwardCheckID()
	task := agentstreams.DiagnosticTask{ID: id, Type: AgentDiagnosticTaskType, Action: action, Params: normalized, Timeout: int((timeoutMS + 999) / 1000)}
	payload, err := json.Marshal(map[string]any{"task": task})
	if err != nil {
		return nil, fmt.Errorf("encode forward check: %w", err)
	}
	desired := &agentv1pb.DesiredOperation{OperationId: id, Kind: AgentDiagnosticOperation, PayloadJson: payload, DeadlineUnixMs: deadline.UnixMilli()}
	dispatch := dispatchOnStream(ctx, streams, node, desired, min(forwardCheckAckTimeout, time.Until(deadline)))
	if dispatch.Err != nil {
		if errors.Is(dispatch.Err, agentstreams.ErrNotConnected) || errors.Is(dispatch.Err, agentstreams.ErrCapabilityMissing) ||
			errors.Is(dispatch.Err, agentstreams.ErrSessionClosed) {
			return nil, fmt.Errorf("%w: %v", kernelforward.ErrCheckUnavailable, dispatch.Err)
		}
		return nil, dispatch.Err
	}
	if !dispatch.Ack.GetAccepted() {
		dispatch.release()
		message := dispatch.Ack.GetError()
		if message == "" {
			message = "the Agent rejected the check"
		}
		return nil, errors.New(message)
	}
	observed, err := dispatch.awaitTerminal(ctx, streams, node)
	if err != nil {
		return nil, err
	}
	return decodeForwardCheck(id, observed)
}

// decodeForwardCheck reads a check's result from the Agent's terminal
// state.
func decodeForwardCheck(id string, observed *agentv1pb.ObservedState) (*kernelforward.CheckResult, error) {
	switch observed.GetPhase() {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		return nil, errors.New("the Agent superseded the check")
	default:
		if observed.GetMessage() == agentOperationDeadlineText {
			return nil, context.DeadlineExceeded
		}
		message := observed.GetMessage()
		if message == "" {
			message = "the Agent reported that the check failed"
		}
		return nil, errors.New(message)
	}
	var state forwardCheckState
	if err := json.Unmarshal(observed.GetStateJson(), &state); err != nil {
		return nil, fmt.Errorf("the Agent's answer is not a diagnostic result: %w", err)
	}
	if state.Result == nil {
		message := state.Error
		if message == "" {
			message = "the Agent answered without a forward check result"
		}
		return nil, errors.New(message)
	}
	state.Result.OperationID = id
	return state.Result, nil
}

func newForwardCheckID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "fwdiag-" + hex.EncodeToString(raw[:])
}
