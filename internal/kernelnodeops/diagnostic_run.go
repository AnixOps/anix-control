package kernelnodeops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// The synchronous node diagnostics of the administrator API
// (POST /api/v4/kernel/nodes/:id/diagnostics): one whitelisted task of the
// agent.diagnostic operation, sent on the node's Agent Control stream, and
// the Agent's result answered within the request.

const (
	// DefaultNodeDiagnosticTimeout is a diagnostic's time when the request
	// names none.
	DefaultNodeDiagnosticTimeout = 30 * time.Second
	// MaxNodeDiagnosticTimeout is the longest a request may wait.
	MaxNodeDiagnosticTimeout = 120 * time.Second
	// nodeDiagnosticAckTimeout bounds the wait for the Agent's
	// acknowledgement, inside the diagnostic's time.
	nodeDiagnosticAckTimeout = 10 * time.Second
	// MaxNodeDiagnosticListLimit bounds the task history a request reads.
	MaxNodeDiagnosticListLimit = 50
	// nodeDiagnosticListOutput bounds the output each task of a history
	// carries (its tail: logs end with what happened last).
	nodeDiagnosticListOutput = 8 << 10
)

// Why a node cannot run diagnostics now (DiagnosticUnavailableError.Reason).
const (
	// DiagnosticOffline: the node's Agent holds no Agent Control stream.
	DiagnosticOffline = "offline"
	// DiagnosticUnsupported: the Agent is connected and does not advertise
	// agent.diagnostic (and diag.v1 for the forward checks).
	DiagnosticUnsupported = "unsupported"
	// DiagnosticPendingApproval: the node joined a group that requires
	// approval and waits for it.
	DiagnosticPendingApproval = "pending_approval"
)

// NodeDiagnosticRequest is one diagnostic of an administrator.
type NodeDiagnosticRequest struct {
	Node   agentcontrol.AgentNode
	Action string
	Params map[string]any
	// Timeout bounds the whole run, from the dispatch to the result; zero
	// is DefaultNodeDiagnosticTimeout and more than MaxNodeDiagnosticTimeout
	// is clamped to it.
	Timeout time.Duration
}

// NodeDiagnosticResult is what the Agent answered.
type NodeDiagnosticResult struct {
	TaskID string `json:"task_id"`
	Node   string `json:"node"`
	Action string `json:"action"`
	// Params are the task's parameters as the whitelist normalized them.
	Params     map[string]any `json:"params"`
	Success    bool           `json:"success"`
	Output     string         `json:"output"`
	Error      string         `json:"error,omitempty"`
	DurationMS int64          `json:"duration_ms"`
	// Result is a forward check's structured answer; absent for the
	// generic actions.
	Result json.RawMessage `json:"result,omitempty"`
}

// DiagnosticUnavailableError means the node cannot run the task now.
type DiagnosticUnavailableError struct {
	Reason  string
	Message string
}

func (e *DiagnosticUnavailableError) Error() string { return e.Message }

// DiagnosticTimeoutError means the Agent gave no answer in time.
type DiagnosticTimeoutError struct {
	// TaskID is the task that was sent (its row, for a proxy node).
	TaskID  string
	Timeout time.Duration
	// Acknowledged is true when the Agent accepted the task and reported
	// no end.
	Acknowledged bool
}

func (e *DiagnosticTimeoutError) Error() string {
	if e.Acknowledged {
		return fmt.Sprintf("the Agent did not report a result within %d seconds", int(e.Timeout/time.Second))
	}
	return "the Agent did not acknowledge the task in time"
}

// DiagnosticAgentError means the Agent refused the task or the stream
// failed while sending it.
type DiagnosticAgentError struct {
	TaskID  string
	Message string
}

func (e *DiagnosticAgentError) Error() string { return e.Message }

// NodeDiagnosticSupport is what a node's live Agent session runs.
type NodeDiagnosticSupport struct {
	// Connected: the node's Agent holds an Agent Control stream.
	Connected    bool
	AgentVersion string
	// Diagnostics: the Agent advertises agent.diagnostic (service_status,
	// service_restart, log_tail).
	Diagnostics bool
	// ForwardChecks: it also advertises diag.v1 (the forward checks).
	ForwardChecks bool
}

// NodeDiagnosticSupportOf reads the node's live session in sources.
func NodeDiagnosticSupportOf(sources AgentSources, node agentcontrol.AgentNode) NodeDiagnosticSupport {
	if sources.Streams == nil {
		return NodeDiagnosticSupport{}
	}
	session, ok := sources.Streams.Session(node)
	if !ok {
		return NodeDiagnosticSupport{}
	}
	diagnostics := hasCapability(session.Capabilities, AgentDiagnosticOperation)
	return NodeDiagnosticSupport{
		Connected: true, AgentVersion: session.AgentVersion, Diagnostics: diagnostics,
		ForwardChecks: diagnostics && hasCapability(session.Capabilities, agentcontrol.CapabilityDiag),
	}
}

// ClampNodeDiagnosticTimeout applies the default and the bound.
func ClampNodeDiagnosticTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultNodeDiagnosticTimeout
	}
	return min(timeout, MaxNodeDiagnosticTimeout)
}

// ActionNotAllowedError is the validation error of an action outside the
// diagnostic whitelist.
type ActionNotAllowedError struct {
	Action string
}

func (e *ActionNotAllowedError) Error() string {
	return fmt.Sprintf("action %q is not in the diagnostic whitelist", e.Action)
}

// ValidateNodeDiagnostic checks a task against the whitelist the
// administrator API serves: the generic actions of
// service.AgentDiagnosticActions and the forward checks of
// service.ForwardDiagnosticChecks. It answers the parameters normalized
// (only the action's own, clamped) and whether the action is a forward
// check. Every error is a *DiagnosticValidationError.
func ValidateNodeDiagnostic(action string, params map[string]any) (normalized map[string]any, forwardCheck bool, err error) {
	if service.IsForwardDiagnosticCheck(action) {
		normalized, err = service.ValidateForwardDiagnosticCheck(action, params)
		if err != nil {
			return nil, true, &DiagnosticValidationError{Err: err}
		}
		return normalized, true, nil
	}
	if _, ok := service.AgentDiagnosticActions[action]; !ok {
		return nil, false, &DiagnosticValidationError{Err: &ActionNotAllowedError{Action: action}}
	}
	normalized, err = service.ValidateAgentDiagnosticTask(action, params)
	if err != nil {
		return nil, false, &DiagnosticValidationError{Err: err}
	}
	return normalized, false, nil
}

// NodeDiagnosticMutates reports whether a whitelisted action changes the
// node (service_restart).
func NodeDiagnosticMutates(action string) bool {
	return service.AgentDiagnosticActions[action].Mutating
}

// RunNodeDiagnostic validates the task against the whitelist (the generic
// actions of service.AgentDiagnosticActions and the forward checks of
// service.ForwardDiagnosticChecks, parameters normalized), sends it on the
// node's Agent Control stream and waits for the Agent's result until the
// request's time ends or ctx does.
//
// The generic actions of a proxy node are recorded in
// v2_agent_diagnostic_task like the legacy routes' tasks. A forward node's
// tasks and the forward checks (probes, not tasks of a proxy node's table)
// are not; the caller audits the call.
//
// Errors: *DiagnosticValidationError (refused, nothing sent),
// *DiagnosticUnavailableError (offline, no capability, pending approval),
// *DiagnosticTimeoutError, *DiagnosticAgentError, and ctx's own error when
// the caller went away (the Agent is asked to cancel the task).
func RunNodeDiagnostic(ctx context.Context, db *gorm.DB, sources AgentSources, request NodeDiagnosticRequest) (*NodeDiagnosticResult, error) {
	if !request.Node.Valid() {
		return nil, &DiagnosticValidationError{Err: errors.New("node must be proxy-<id> or forward-<id>")}
	}
	normalized, forwardCheck, err := ValidateNodeDiagnostic(request.Action, request.Params)
	if err != nil {
		return nil, err
	}

	support := NodeDiagnosticSupportOf(sources, request.Node)
	switch {
	case !support.Connected:
		return nil, &DiagnosticUnavailableError{Reason: DiagnosticOffline, Message: "the node's Agent is not connected"}
	case !support.Diagnostics:
		return nil, &DiagnosticUnavailableError{Reason: DiagnosticUnsupported, Message: "the node's Agent does not advertise agent.diagnostic" + versionNote(support)}
	case forwardCheck && !support.ForwardChecks:
		return nil, &DiagnosticUnavailableError{Reason: DiagnosticUnsupported, Message: "the node's Agent does not advertise diag.v1, the forward checks" + versionNote(support)}
	}

	timeout := ClampNodeDiagnosticTimeout(request.Timeout)
	started := time.Now()
	deadline := started.Add(timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	id := newDiagnosticID()
	// The Agent runs a generic task for the whole time it was given; a
	// forward check keeps its own timeout_ms.
	taskSeconds := int(math.Ceil(timeout.Seconds()))
	if forwardCheck {
		if timeoutMS, ok := normalized["timeout_ms"].(int64); ok {
			taskSeconds = int((timeoutMS + 999) / 1000)
		}
	}
	task := agentstreams.DiagnosticTask{ID: id, Type: AgentDiagnosticTaskType, Action: request.Action, Params: normalized, Timeout: taskSeconds}
	payload, err := json.Marshal(map[string]any{"task": task})
	if err != nil {
		return nil, fmt.Errorf("encode diagnostic task: %w", err)
	}

	var tasks *service.AgentDiagnosticTaskService
	if !forwardCheck && request.Node.Kind == agentcontrol.NodeKindProxy && db != nil {
		tasks = service.NewAgentDiagnosticTaskService(db)
		if _, err := tasks.CreateTask(id, uint(request.Node.ID), request.Action, normalized); err != nil {
			return nil, fmt.Errorf("record diagnostic task: %w", err)
		}
	}
	finish := func(success bool, output, failure string, durationMS int64) {
		if tasks != nil {
			_ = tasks.CompleteTask(id, uint(request.Node.ID), request.Action, success, output, failure, durationMS)
		}
	}
	elapsed := func() int64 { return time.Since(started).Milliseconds() }

	desired := &agentv1pb.DesiredOperation{OperationId: id, Kind: AgentDiagnosticOperation, PayloadJson: payload, DeadlineUnixMs: deadline.UnixMilli()}
	dispatch := dispatchOnStream(ctx, sources.Streams, request.Node, desired, min(nodeDiagnosticAckTimeout, timeout))
	if dispatch.Err != nil {
		err := dispatchFailure(dispatch.Err, id, timeout)
		finish(false, "", err.Error(), elapsed())
		return nil, err
	}
	if !dispatch.Ack.GetAccepted() {
		dispatch.release()
		message := dispatch.Ack.GetError()
		if message == "" {
			message = "the Agent rejected the task"
		}
		finish(false, "", message, elapsed())
		return nil, &DiagnosticAgentError{TaskID: id, Message: message}
	}
	if tasks != nil {
		_ = tasks.MarkStatus(id, model.AgentDiagnosticTaskStatusDispatched)
	}

	observed, err := dispatch.awaitTerminal(ctx, sources.Streams, request.Node)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			timedOut := &DiagnosticTimeoutError{TaskID: id, Timeout: timeout, Acknowledged: true}
			finish(false, "", timedOut.Error(), elapsed())
			return nil, timedOut
		}
		finish(false, "", "the caller went away before the Agent reported", elapsed())
		return nil, err
	}
	if observed.GetPhase() != agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED && observed.GetMessage() == agentOperationDeadlineText {
		timedOut := &DiagnosticTimeoutError{TaskID: id, Timeout: timeout, Acknowledged: true}
		finish(false, "", timedOut.Error(), elapsed())
		return nil, timedOut
	}

	report := decodeDiagnosticReport(observed)
	result := &NodeDiagnosticResult{
		TaskID: id, Node: request.Node.String(), Action: request.Action, Params: normalized,
		Success: report.Success, Output: scrubText(report.Output, nil), Error: scrubText(report.Error, nil), DurationMS: report.DurationMS,
	}
	if len(report.Result) > 0 && string(report.Result) != "null" {
		result.Result = scrubBytes(report.Result, nil)
	}
	finish(report.Success, report.Output, report.Error, report.DurationMS)
	return result, nil
}

func versionNote(support NodeDiagnosticSupport) string {
	if support.AgentVersion == "" {
		return ""
	}
	return " (Agent " + support.AgentVersion + ")"
}

// dispatchFailure maps an error of the dispatch to the diagnostic's.
func dispatchFailure(err error, id string, timeout time.Duration) error {
	switch {
	case errors.Is(err, agentstreams.ErrNotConnected), errors.Is(err, agentstreams.ErrSessionClosed):
		return &DiagnosticUnavailableError{Reason: DiagnosticOffline, Message: "the node's Agent is not connected"}
	case errors.Is(err, agentstreams.ErrCapabilityMissing):
		return &DiagnosticUnavailableError{Reason: DiagnosticUnsupported, Message: "the node's Agent does not advertise agent.diagnostic"}
	case errors.Is(err, agentstreams.ErrPendingApproval):
		return &DiagnosticUnavailableError{Reason: DiagnosticPendingApproval, Message: "the node waits for approval: Control sends it nothing until an administrator approves it"}
	case errors.Is(err, context.DeadlineExceeded):
		return &DiagnosticTimeoutError{TaskID: id, Timeout: timeout}
	case errors.Is(err, context.Canceled):
		// The caller went away while the Agent had not acknowledged yet.
		return context.Canceled
	}
	return &DiagnosticAgentError{TaskID: id, Message: err.Error()}
}

func newDiagnosticID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "diag-" + hex.EncodeToString(raw[:])
}

// NodeDiagnosticTask is one recorded task of a proxy node, with its
// result as the Agent reported it (scrubbed like every node text).
type NodeDiagnosticTask struct {
	TaskID  string         `json:"task_id"`
	Action  string         `json:"action"`
	Params  map[string]any `json:"params,omitempty"`
	Status  string         `json:"status"`
	Success bool           `json:"success"`
	Output  string         `json:"output,omitempty"`
	// OutputTruncated is true when Output is only the last part of what
	// the Agent answered.
	OutputTruncated bool      `json:"output_truncated,omitempty"`
	Error           string    `json:"error,omitempty"`
	DurationMS      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ListNodeDiagnosticTasks lists the newest recorded tasks of a proxy node.
// A forward node has none: the table is keyed by proxy node id.
func ListNodeDiagnosticTasks(db *gorm.DB, node agentcontrol.AgentNode, limit int) ([]NodeDiagnosticTask, error) {
	if limit <= 0 || limit > MaxNodeDiagnosticListLimit {
		limit = 10
	}
	if db == nil || node.Kind != agentcontrol.NodeKindProxy || node.ID == 0 {
		return []NodeDiagnosticTask{}, nil
	}
	rows, err := service.NewAgentDiagnosticTaskService(db).ListTasks(uint(node.ID), limit)
	if err != nil {
		return nil, err
	}
	tasks := make([]NodeDiagnosticTask, 0, len(rows))
	for _, row := range rows {
		task := NodeDiagnosticTask{
			TaskID: row.TaskID, Action: row.Action, Status: row.Status, Success: row.Success,
			Output: row.Output, Error: scrubText(row.Error, nil), DurationMS: row.DurationMS, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		if len(task.Output) > nodeDiagnosticListOutput {
			task.Output, task.OutputTruncated = task.Output[len(task.Output)-nodeDiagnosticListOutput:], true
		}
		task.Output = scrubText(task.Output, nil)
		if row.Params != "" {
			_ = json.Unmarshal([]byte(row.Params), &task.Params)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// NodeDiagnosticAction describes one action a node's Agent runs.
type NodeDiagnosticAction struct {
	Action string `json:"action"`
	// Mutating actions change the node (service_restart): super
	// administrators only.
	Mutating bool     `json:"mutating"`
	Params   []string `json:"params"`
	// Forward is true for the forward checks.
	Forward bool `json:"forward"`
}

// NodeDiagnosticServices lists the services the node's generic actions
// run on: a proxy node's Agent itself. Nil when the node runs none (a
// forward node, whose gost went with the v4.1 forwarding runtime).
func NodeDiagnosticServices(node agentcontrol.AgentNode, support NodeDiagnosticSupport) []string {
	if !support.Diagnostics || node.Kind != agentcontrol.NodeKindProxy {
		return nil
	}
	return []string{service.AgentDiagnosticServiceAgent}
}

// NodeDiagnosticActions lists the actions a node with support may run, the
// generic ones first, each group by name.
func NodeDiagnosticActions(support NodeDiagnosticSupport) []NodeDiagnosticAction {
	var actions []NodeDiagnosticAction
	if support.Diagnostics {
		for action, spec := range service.AgentDiagnosticActions {
			actions = append(actions, NodeDiagnosticAction{Action: action, Mutating: spec.Mutating, Params: append([]string(nil), spec.Params...)})
		}
	}
	if support.ForwardChecks {
		for action, spec := range service.ForwardDiagnosticChecks {
			actions = append(actions, NodeDiagnosticAction{Action: action, Mutating: spec.Mutating, Params: append([]string(nil), spec.Params...), Forward: true})
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Forward != actions[j].Forward {
			return !actions[i].Forward
		}
		return actions[i].Action < actions[j].Action
	})
	return actions
}
