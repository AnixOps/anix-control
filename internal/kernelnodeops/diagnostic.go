package kernelnodeops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

const (
	// AgentDiagnosticTaskType is the type of every task an administrator
	// sends to an agent: a whitelisted diagnostic action.
	AgentDiagnosticTaskType = "diagnostic"
	// AgentDiagnosticOperation is the Agent Control operation that carries
	// a diagnostic task on the stream; its payload is {"task": task}, the
	// WebSocket's task.assign payload. An agent that runs it advertises
	// the capability of the same name.
	AgentDiagnosticOperation = "agent.diagnostic"
)

// AgentDiagnosticRequest is one diagnostic task for a proxy node's agent,
// as POST /admin/agent/tasks and /admin/agent/execute take it.
type AgentDiagnosticRequest struct {
	NodeID uint
	Action string
	Params map[string]any
	// Timeout is the task's timeout as the agent receives it.
	Timeout int
}

// DiagnosticValidationError refuses a task the whitelist does not allow.
type DiagnosticValidationError struct {
	Err error
}

func (e *DiagnosticValidationError) Error() string { return e.Err.Error() }
func (e *DiagnosticValidationError) Unwrap() error { return e.Err }

// AgentDiagnostic is what RunAgentDiagnostic did.
type AgentDiagnostic struct {
	Task agentstreams.DiagnosticTask
	// Row is the task's row as the legacy routes answer it: reloaded after
	// the dispatch, or as created when the task could not be sent.
	Row      *model.AgentDiagnosticTask
	Dispatch agentstreams.DiagnosticDispatch
}

// RunAgentDiagnostic validates a diagnostic task against the kernel's
// whitelist (its parameters normalized), records it in
// v2_agent_diagnostic_task, sends it over transport and records whether it
// went out, as the legacy task and execute routes do. A refused task is a
// DiagnosticValidationError and records nothing.
func RunAgentDiagnostic(ctx context.Context, db *gorm.DB, request AgentDiagnosticRequest, transport agentstreams.DiagnosticTransport) (*AgentDiagnostic, error) {
	normalized, err := service.ValidateAgentDiagnosticTask(request.Action, request.Params)
	if err != nil {
		return nil, &DiagnosticValidationError{Err: err}
	}
	if db == nil {
		return nil, errors.New("diagnostic task service unavailable")
	}
	if transport == nil {
		return nil, errors.New("node websocket unavailable")
	}
	tasks := service.NewAgentDiagnosticTaskService(db)
	task := agentstreams.DiagnosticTask{ID: NewTaskID(), Type: AgentDiagnosticTaskType, Action: request.Action, Params: normalized, Timeout: request.Timeout}
	row, err := tasks.CreateTask(task.ID, request.NodeID, request.Action, normalized)
	if err != nil {
		return nil, err
	}
	run := &AgentDiagnostic{Task: task, Row: row}
	run.Dispatch = transport.DispatchDiagnostic(ctx, task)
	if !run.Dispatch.Sent() {
		_ = tasks.MarkStatus(task.ID, model.AgentDiagnosticTaskStatusFailed)
		return run, nil
	}
	_ = tasks.MarkStatus(task.ID, model.AgentDiagnosticTaskStatusDispatched)
	run.Row, _ = tasks.GetTask(task.ID)
	return run, nil
}

// Bounds of a legacy route's diagnostic on the stream
// (RunAgentDiagnosticOnStream).
const (
	// defaultDiagnosticTimeout is a task's timeout when the request names
	// none, as the whitelist's tasks run.
	defaultDiagnosticTimeout = 30 * time.Second
	// maxDiagnosticTimeout caps the timeout a request asks for.
	maxDiagnosticTimeout = 10 * time.Minute
)

// diagnosticResultGrace is how long after the task's timeout its result
// is still awaited. A variable so tests can shorten it.
var diagnosticResultGrace = time.Minute

// SetDiagnosticResultGraceForTest sets diagnosticResultGrace and returns
// the function that restores it.
func SetDiagnosticResultGraceForTest(grace time.Duration) (restore func()) {
	previous := diagnosticResultGrace
	diagnosticResultGrace = grace
	return func() { diagnosticResultGrace = previous }
}

// RunAgentDiagnosticOnStream runs a diagnostic task of the legacy admin
// routes (POST /admin/agent/tasks and /admin/agent/execute) on the node's
// Agent Control stream, as the agent.diagnostic executor runs it: when the
// node's agent is connected on a stream of sources and advertises
// agent.diagnostic. ok is false otherwise, and nothing was recorded. The
// task row is written as RunAgentDiagnostic writes it; the answer is the
// agent's acknowledgement. After an accepted acknowledgement the result is
// awaited in the background, until the task's timeout plus a minute, and
// recorded in the row as the executor records it (the agent API's result
// report); without one in time the row is marked failed.
func RunAgentDiagnosticOnStream(ctx context.Context, db *gorm.DB, sources AgentSources, request AgentDiagnosticRequest) (*AgentDiagnostic, bool, error) {
	if sources.Streams == nil || request.NodeID == 0 || uint64(request.NodeID) > math.MaxUint32 {
		return nil, false, nil
	}
	node := proxyAgentNode(uint64(request.NodeID))
	session, connected := sources.Streams.Session(node)
	if !connected || !hasCapability(session.Capabilities, AgentDiagnosticOperation) {
		return nil, false, nil
	}
	timeout := defaultDiagnosticTimeout
	if request.Timeout > 0 {
		timeout = min(time.Duration(request.Timeout)*time.Second, maxDiagnosticTimeout)
	}
	deadline := time.Now().Add(timeout + diagnosticResultGrace)
	stream := &streamDiagnosticTransport{streams: sources.Streams, node: node, deadline: deadline, timeout: ackTimeout(uint32(timeout / time.Second))}
	run, err := RunAgentDiagnostic(ctx, db, request, stream)
	if err != nil || stream.dispatch == nil {
		return run, true, err
	}
	if !run.Dispatch.AckReceived {
		stream.dispatch.release()
		return run, true, nil
	}
	go func() {
		waitCtx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		observed, err := stream.dispatch.awaitTerminal(waitCtx, sources.Streams, node)
		if err != nil {
			_ = service.NewAgentDiagnosticTaskService(db).MarkStatus(run.Task.ID, model.AgentDiagnosticTaskStatusFailed)
			return
		}
		completeDiagnosticTask(db, run.Task, node, observed)
	}()
	return run, true, nil
}

// streamDiagnosticTransport carries a task on the node's Agent Control
// stream as an agent.diagnostic operation.
type streamDiagnosticTransport struct {
	streams  agentstreams.Streams
	node     agentcontrol.AgentNode
	deadline time.Time
	timeout  time.Duration
	dispatch *streamDispatch
}

func (t *streamDiagnosticTransport) DispatchDiagnostic(ctx context.Context, task agentstreams.DiagnosticTask) agentstreams.DiagnosticDispatch {
	payload, err := json.Marshal(map[string]any{"task": task})
	if err != nil {
		return agentstreams.DiagnosticDispatch{MessageID: task.ID, DispatchError: fmt.Errorf("encode task: %w", err)}
	}
	desired := &agentv1pb.DesiredOperation{OperationId: task.ID, Kind: AgentDiagnosticOperation, PayloadJson: payload, DeadlineUnixMs: t.deadline.UnixMilli()}
	t.dispatch = dispatchOnStream(ctx, t.streams, t.node, desired, t.timeout)
	dispatch := agentstreams.DiagnosticDispatch{MessageID: task.ID}
	if t.dispatch.Err != nil {
		dispatch.DispatchError = t.dispatch.Err
		return dispatch
	}
	ack := t.dispatch.Ack
	dispatch.RawAck = ack
	dispatch.Ack = agentstreams.Ack{
		Accepted: ack.GetAccepted(), Error: ack.GetError(), SessionID: ack.GetSessionId(), Revision: ack.GetRevision(),
	}
	if ack.GetAcceptedAtUnixMs() != 0 {
		dispatch.Ack.AcceptedAt = time.UnixMilli(ack.GetAcceptedAtUnixMs())
	}
	if !ack.GetAccepted() {
		if ack.GetError() == "" {
			dispatch.DispatchError = fmt.Errorf("agent nack for operation %s", task.ID)
		} else {
			dispatch.DispatchError = fmt.Errorf("agent nack: %s", ack.GetError())
		}
		return dispatch
	}
	dispatch.AckReceived = true
	return dispatch
}

// agentDiagnosticExecutor serves agent.diagnostic: on the node's Agent
// Control stream when its agent advertises the operation, else on the
// legacy WebSocket with its fallback (section 6.3).
type agentDiagnosticExecutor struct {
	sources SourcesFunc
}

func (x agentDiagnosticExecutor) Execute(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetRunAgentDiagnostic()
	node := proxyAgentNode(op.GetNodeId())
	var params map[string]any
	if len(op.GetParamsJson()) > 0 {
		if err := json.Unmarshal(op.GetParamsJson(), &params); err != nil {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "params_json is not a JSON object", false)
		}
	}
	sources := x.sources()
	var transport agentstreams.DiagnosticTransport
	channel := kernelnodeopsv1.Channel_CHANNEL_UNSPECIFIED
	var stream *streamDiagnosticTransport
	if sources.Streams != nil {
		if session, ok := sources.Streams.Session(node); ok && hasCapability(session.Capabilities, AgentDiagnosticOperation) {
			stream = &streamDiagnosticTransport{streams: sources.Streams, node: node, deadline: run.Deadline, timeout: ackTimeout(op.GetTimeoutSeconds())}
			transport, channel = stream, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL
		}
	}
	if transport == nil && sources.WebSockets != nil {
		if ws, ok := sources.WebSockets.DiagnosticTransport(uint(node.ID)); ok {
			transport, channel = ws, kernelnodeopsv1.Channel_CHANNEL_AGENT_WEBSOCKET
		}
	}
	if transport == nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, "node offline", true)
	}
	diagnostic, err := RunAgentDiagnostic(ctx, run.DB(), AgentDiagnosticRequest{
		NodeID: uint(node.ID), Action: op.GetAction(), Params: params, Timeout: int(op.GetTimeoutSeconds()),
	}, transport)
	if err != nil {
		var refused *DiagnosticValidationError
		if errors.As(err, &refused) {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the task could not be recorded: "+err.Error(), true)
	}
	dispatch := diagnostic.Dispatch
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_AgentDiagnostic{AgentDiagnostic: &kernelnodeopsv1.AgentDiagnosticResult{
		TaskId: diagnostic.Task.ID, MessageId: dispatch.MessageID, AckReceived: dispatch.AckReceived, LegacyFallback: dispatch.LegacyFallback,
	}}}
	if dispatch.DispatchError != nil {
		result.GetAgentDiagnostic().DispatchError = dispatch.DispatchError.Error()
	}
	if dispatch.AckReceived || dispatch.Ack.SessionID != "" || dispatch.Ack.Error != "" {
		result.GetAgentDiagnostic().Ack = &kernelnodeopsv1.AgentAck{
			Accepted: dispatch.Ack.Accepted, Error: dispatch.Ack.Error, SessionId: dispatch.Ack.SessionID, Revision: dispatch.Ack.Revision,
		}
		if !dispatch.Ack.AcceptedAt.IsZero() {
			result.GetAgentDiagnostic().Ack.AcceptedAtUnixMs = dispatch.Ack.AcceptedAt.UnixMilli()
		}
	}
	if !dispatch.Sent() {
		err := dispatch.DispatchError
		if dispatch.FallbackError != nil {
			err = fmt.Errorf("%w; legacy fallback: %v", err, dispatch.FallbackError)
		}
		return streamFailure(err).WithResult(result).WithChannel(channel)
	}
	if err := run.Accept(ctx, Acceptance{Channel: channel, NodeRevision: dispatch.Ack.Revision}); err != nil {
		return ended(err)
	}
	if stream == nil || stream.dispatch == nil {
		// The WebSocket agent reports the result through the agent API.
		return Succeeded(result)
	}
	defer stream.dispatch.release()
	if !dispatch.AckReceived {
		return rejected(stream.dispatch.Ack).WithResult(result)
	}
	observed, err := stream.dispatch.awaitTerminal(ctx, sources.Streams, node)
	if err != nil {
		return awaitFailure(err).WithResult(result)
	}
	completeDiagnosticTask(run.DB(), diagnostic.Task, node, observed)
	return observedOutcome(observed, result)
}

// completeDiagnosticTask records the result an agent reported on the
// stream in the task's row, as the agent API's result report does.
func completeDiagnosticTask(db *gorm.DB, task agentstreams.DiagnosticTask, node agentcontrol.AgentNode, observed *agentv1pb.ObservedState) {
	var report struct {
		Success    bool   `json:"success"`
		Output     string `json:"output"`
		Error      string `json:"error"`
		DurationMS int64  `json:"duration_ms"`
	}
	succeeded := observed.GetPhase() == agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED
	if len(observed.GetStateJson()) > 0 && json.Unmarshal(observed.GetStateJson(), &report) == nil {
		report.Success = report.Success && succeeded
	} else {
		report.Success = succeeded
		report.Error = observed.GetMessage()
	}
	if !report.Success && report.Error == "" {
		report.Error = observed.GetMessage()
	}
	_ = service.NewAgentDiagnosticTaskService(db).CompleteTask(task.ID, uint(node.ID), task.Action, report.Success, report.Output, report.Error, report.DurationMS)
}

func hasCapability(capabilities []string, name string) bool {
	for _, capability := range capabilities {
		if capability == name {
			return true
		}
	}
	return false
}
