package kernelnodeops

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"gorm.io/gorm"
)

// Executor carries out the operations of one kind (node-ops-service.md
// section 3.3). The engine calls Execute once per dispatch, in its own
// goroutine, with a context that ends at the operation's deadline or when
// the operation is cancelled. Execute reports the channel's acceptance
// through Run.Accept and returns the outcome; the engine records it.
//
// An executor must apply all of an operation or nothing it cannot repeat:
// a level-triggered kind may run again after a kernel restart, every other
// kind ends FAILED (retryable) when it was interrupted.
type Executor interface {
	Execute(ctx context.Context, run *Run) Outcome
}

// ExecutorFunc adapts a function to Executor.
type ExecutorFunc func(ctx context.Context, run *Run) Outcome

// Execute calls f.
func (f ExecutorFunc) Execute(ctx context.Context, run *Run) Outcome { return f(ctx, run) }

// Preparer is implemented by executors that must act in the submitting
// call, before the operation is recorded: use the verified request binding,
// resolve sealed handles (Submission.Unseal), capture what a deletion
// removes. Prepare may replace submission.Operation; the canonical form of
// what it leaves is stored and executed, and must hold no sealed handle.
// An error refuses the submission and records nothing; a gRPC status error
// is answered as it is.
type Preparer interface {
	Prepare(ctx context.Context, submission *Submission) error
}

// Submission is an operation in its submitting call.
type Submission struct {
	Host      packagebridge.HostIdentity
	Kind      string
	Operation *kernelnodeopsv1.OperationSpec
	// Binding is the request binding's bridge capability; it is never
	// stored.
	Binding []byte
	// Request is the verified request binding: the live v2 request the
	// package is serving. Nil when the submission carries no binding; a
	// binding that names no live request of the package is refused before
	// Prepare.
	Request *BoundRequest
	Targets []*kernelnodeopsv1.NodeRef

	store *sealedsecrets.Store
	// db is the submitting call's database, for checks a Preparer makes
	// before the operation is recorded.
	db *gorm.DB
}

// Registry maps operation kinds to their executors. A kind without one is
// not served: SubmitOperation answers UNIMPLEMENTED and records nothing.
type Registry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{executors: map[string]Executor{}}
}

// Register serves kind with executor. A kind the contract does not define,
// or one registered before, is an error.
func (r *Registry) Register(kindName string, executor Executor) error {
	if _, known := kinds[kindName]; !known {
		return fmt.Errorf("kernelnodeops: unknown operation kind %q", kindName)
	}
	if executor == nil {
		return fmt.Errorf("kernelnodeops: executor for %q is nil", kindName)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[kindName]; exists {
		return fmt.Errorf("kernelnodeops: operation kind %q already has an executor", kindName)
	}
	r.executors[kindName] = executor
	return nil
}

// Lookup returns kind's executor.
func (r *Registry) Lookup(kindName string) (Executor, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[kindName]
	return executor, ok
}

// Kinds returns the served kinds, sorted.
func (r *Registry) Kinds() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.executors))
	for name := range r.executors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultExecutors is the registry the kernel's engines use. NO-1 serves no
// kind: NO-5 to NO-8 register the executors.
var DefaultExecutors = NewRegistry()

// Run is one dispatch of an operation, as its executor sees it.
type Run struct {
	OperationID       string
	RequestID         string
	PackageID         string
	Kind              string
	ParentOperationID string
	// Operation is the stored canonical operation.
	Operation *kernelnodeopsv1.OperationSpec
	Targets   []*kernelnodeopsv1.NodeRef
	Attempt   uint32
	Deadline  time.Time
	// Request is the submission's verified request binding while its
	// request is live: nil when it carried none, the request ended, or the
	// kernel restarted since. Reveal mints handles for it.
	Request *BoundRequest

	engine  *Engine
	mu      sync.Mutex
	secrets []string
}

// Acceptance is what the channel reported when it took the operation.
type Acceptance struct {
	Channel             kernelnodeopsv1.Channel
	NodeRevision        uint64
	KernelOperationID   string
	ForwardRuntimeJobID uint64
}

// Accept records that the channel took the operation: it moves to RUNNING
// and an ACCEPTED wait answers. Calling it again updates the links.
func (r *Run) Accept(ctx context.Context, acceptance Acceptance) error {
	return r.engine.accept(ctx, r, acceptance)
}

// UseSecret names credential values the operation uses: the kernel replaces
// them with the placeholder in every text of the result, the error and the
// evidence before it stores them.
func (r *Run) UseSecret(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			r.secrets = append(r.secrets, value)
		}
	}
}

// DB returns the kernel database the operation is recorded in.
func (r *Run) DB() *gorm.DB { return r.engine.DB }

// now is the engine's clock.
func (r *Run) now() time.Time { return r.engine.now() }

func (r *Run) usedSecrets() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.secrets...)
}

// outcomeKind is how an operation ended in its executor.
type outcomeKind int

const (
	outcomeSucceeded outcomeKind = iota + 1
	outcomeFailed
	outcomeCancelled
	outcomeFanOut
)

// Outcome is an executor's report: build it with Succeeded, Failed,
// Cancelled or FanOut.
type Outcome struct {
	kind     outcomeKind
	result   *kernelnodeopsv1.OperationResult
	err      *kernelnodeopsv1.OperationError
	channel  kernelnodeopsv1.Channel
	children []*kernelnodeopsv1.OperationSpec
}

// Succeeded ends the operation SUCCEEDED with result (nil for none).
func Succeeded(result *kernelnodeopsv1.OperationResult) Outcome {
	return Outcome{kind: outcomeSucceeded, result: result}
}

// Failed ends the operation FAILED. A code of UNSPECIFIED is recorded as
// BACKEND_FAILED.
func Failed(code kernelnodeopsv1.ErrorCode, message string, retryable bool) Outcome {
	if code == kernelnodeopsv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		code = kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED
	}
	return Outcome{kind: outcomeFailed, err: &kernelnodeopsv1.OperationError{Code: code, Message: message, Retryable: retryable}}
}

// Cancelled ends the operation CANCELLED: the executor stopped it because
// its context was cancelled.
func Cancelled(message string) Outcome {
	if message == "" {
		message = "operation cancelled"
	}
	return Outcome{kind: outcomeCancelled, err: &kernelnodeopsv1.OperationError{
		Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, Message: message,
	}}
}

// FanOut ends a fan-out kind's own work: the engine records one child
// operation per spec, and the operation ends when they all have.
func FanOut(children ...*kernelnodeopsv1.OperationSpec) Outcome {
	return Outcome{kind: outcomeFanOut, children: children}
}

// WithResult attaches a result to a failed or cancelled outcome, for what
// the operation did before it stopped.
func (o Outcome) WithResult(result *kernelnodeopsv1.OperationResult) Outcome {
	o.result = result
	return o
}

// WithChannel records the channel that carried the operation, when the
// executor did not call Run.Accept.
func (o Outcome) WithChannel(channel kernelnodeopsv1.Channel) Outcome {
	o.channel = channel
	return o
}
