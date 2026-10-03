package kernelnodeops

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Engine defaults (node-ops-service.md sections 3.4, 3.5 and 3.9).
const (
	DefaultPollInterval   = time.Second
	DefaultTimeout        = 10 * time.Minute
	DefaultMaxRunning     = 16
	DefaultPackageQuota   = 256
	DefaultNodeQuota      = 32
	OperationRetention    = 90 * 24 * time.Hour
	EventRetention        = 7 * 24 * time.Hour
	pruneInterval         = time.Hour
	pruneBatch            = 500
	shutdownGrace         = 5 * time.Second
	recordTimeout         = 10 * time.Second
	fanOutRequestIDPrefix = "fanout:"
)

// ErrOperationEnded is returned by Run.Accept for an operation that ended
// meanwhile (cancelled, timed out): its executor should stop.
var ErrOperationEnded = errors.New("node operation has ended")

// Engine is the kernel's KernelNodeOps engine: the ledger, the dispatcher
// that hands pending operations to their executors, and the change
// notifications that waits and watches follow. One engine runs its
// dispatcher per database (Control is a single replica, section 8 R6).
type Engine struct {
	DB *gorm.DB
	// Executors serve the operation kinds; nil serves none.
	Executors *Registry
	// PollInterval is how often the dispatcher looks for work without a
	// wake-up; DefaultPollInterval when zero.
	PollInterval time.Duration
	// Timeout is an operation's deadline after its submission;
	// DefaultTimeout when zero.
	Timeout time.Duration
	// MaxRunning bounds the executors running at once.
	MaxRunning int
	// PackageQuota and NodeQuota bound the operations that have not ended,
	// per package and per target node.
	PackageQuota int
	NodeQuota    int
	// Now defaults to time.Now; it stamps the ledger.
	Now func() time.Time
	// Secrets holds the sealed handles of the requests in flight; nil
	// selects sealedsecrets.DefaultStore, which the v2 gateway seals into.
	Secrets *sealedsecrets.Store

	sealed  sealedState
	notify  hub
	wakeMu  sync.Mutex
	wake    chan struct{}
	mu      sync.Mutex
	running map[string]*activeRun
	runs    sync.WaitGroup
}

type activeRun struct {
	run      *Run
	cancel   context.CancelFunc
	deadline time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return DefaultTimeout
}

func (e *Engine) pollInterval() time.Duration {
	if e.PollInterval > 0 {
		return e.PollInterval
	}
	return DefaultPollInterval
}

func (e *Engine) maxRunning() int {
	if e.MaxRunning > 0 {
		return e.MaxRunning
	}
	return DefaultMaxRunning
}

func (e *Engine) packageQuota() int {
	if e.PackageQuota > 0 {
		return e.PackageQuota
	}
	return DefaultPackageQuota
}

func (e *Engine) nodeQuota() int {
	if e.NodeQuota > 0 {
		return e.NodeQuota
	}
	return DefaultNodeQuota
}

func (e *Engine) wakeChannel() chan struct{} {
	e.wakeMu.Lock()
	defer e.wakeMu.Unlock()
	if e.wake == nil {
		e.wake = make(chan struct{}, 1)
	}
	return e.wake
}

// Wake makes the dispatcher look for work now.
func (e *Engine) Wake() {
	select {
	case e.wakeChannel() <- struct{}{}:
	default:
	}
}

// changed tells waiters and watchers that operations changed.
func (e *Engine) changed() {
	e.notify.broadcast()
}

// engines holds the process's engine per database.
var engines sync.Map

// EngineFor returns the process's engine for db, serving DefaultExecutors.
// The local package bridge and the module listener share it, and the
// kernel's singleton workers run its dispatcher.
func EngineFor(db *gorm.DB) *Engine {
	if engine, ok := engines.Load(db); ok {
		return engine.(*Engine)
	}
	engine, _ := engines.LoadOrStore(db, &Engine{DB: db, Executors: DefaultExecutors})
	return engine.(*Engine)
}

// Run dispatches operations until ctx ends: it first recovers the
// operations a previous process left started, then on every tick ends the
// operations past their deadline, passes cancellations to their executors
// and starts pending operations, and prunes the ledger hourly.
func (e *Engine) Run(ctx context.Context) {
	if e.DB == nil {
		return
	}
	recovery, cancelRecovery := statementContext(ctx)
	if err := e.recoverStarted(recovery); err != nil && ctx.Err() == nil {
		log.Printf("Kernel node operations: recovery failed: %v", err)
	}
	cancelRecovery()
	var pruned time.Time
	ticker := time.NewTicker(e.pollInterval())
	defer ticker.Stop()
	for {
		if err := e.tick(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Kernel node operations: dispatch failed: %v", err)
		}
		if time.Since(pruned) >= pruneInterval {
			pruning, cancelPruning := statementContext(ctx)
			if _, err := e.Prune(pruning, e.now()); err != nil && ctx.Err() == nil {
				log.Printf("Kernel node operations: prune failed: %v", err)
			}
			cancelPruning()
			pruned = time.Now()
		}
		select {
		case <-ctx.Done():
			e.awaitRuns(shutdownGrace)
			return
		case <-ticker.C:
		case <-e.wakeChannel():
		}
	}
}

// statementContext is ctx for the dispatcher's own statements: they run
// to completion when ctx ends, bounded by recordTimeout (see tick).
func statementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
}

func (e *Engine) awaitRuns(grace time.Duration) {
	done := make(chan struct{})
	go func() {
		e.runs.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(grace):
	}
}

func (e *Engine) tick(ctx context.Context) error {
	// A tick's statements are short and run to completion when the
	// dispatcher stops, bounded by recordTimeout, instead of being
	// interrupted: the SQLite driver's interrupt on a cancelled context
	// races its connection close (a data race in glebarez/go-sqlite).
	// Executors still run under ctx.
	statements, cancel := statementContext(ctx)
	defer cancel()
	if err := e.expire(statements); err != nil {
		return err
	}
	if err := e.passCancellations(statements); err != nil {
		return err
	}
	return e.claim(ctx, statements)
}

// expire ends the operations past their deadline TIMED_OUT, pending or
// started, and stops their executors. A result that arrives later is
// recorded as evidence only.
func (e *Engine) expire(ctx context.Context) error {
	var due []model.KernelNodeOperation
	if err := e.DB.WithContext(ctx).Where("state IN ? AND deadline_at <= ?", activeStates, e.now()).
		Order("id").Limit(100).Find(&due).Error; err != nil {
		return err
	}
	for i := range due {
		applied, err := e.end(ctx, due[i].OperationID, activeStates, ending{state: stateTimedOut, err: &kernelnodeopsv1.OperationError{
			Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, Message: "the operation's deadline passed", Retryable: true,
		}})
		if err != nil {
			return err
		}
		if applied {
			e.stopExpired(due[i].OperationID)
		}
	}
	return nil
}

// stopExpired stops the executor of an operation that passed its
// deadline. Its context carries that deadline, so once the deadline has
// passed by the wall clock the context ends DeadlineExceeded by itself;
// cancelling it here would race that timer and could hand the executor
// context.Canceled instead. Only a run whose context deadline is still
// ahead (the ledger's clock, Engine.Now, ran ahead of it) is cancelled.
func (e *Engine) stopExpired(operationID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	active, ok := e.running[operationID]
	if !ok {
		return
	}
	if active.deadline.IsZero() || time.Now().Before(active.deadline) {
		active.cancel()
	}
}

// passCancellations cancels the executors of operations cancelled through
// another process's CancelOperation.
func (e *Engine) passCancellations(ctx context.Context) error {
	e.mu.Lock()
	ids := make([]string, 0, len(e.running))
	for id := range e.running {
		ids = append(ids, id)
	}
	e.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	var cancelled []string
	if err := e.DB.WithContext(ctx).Model(&model.KernelNodeOperation{}).
		Where("operation_id IN ? AND (cancel_requested_at IS NOT NULL OR state NOT IN ?)", ids, startedStates).
		Pluck("operation_id", &cancelled).Error; err != nil {
		return err
	}
	for _, id := range cancelled {
		e.stop(id)
	}
	return nil
}

func (e *Engine) isRunning(operationID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.running[operationID]
	return ok
}

// stop cancels a running executor's context.
func (e *Engine) stop(operationID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if active, ok := e.running[operationID]; ok {
		active.cancel()
	}
}

// claim starts pending operations, oldest first, at most one per resource
// at a time and at most MaxRunning at once.
func (e *Engine) claim(ctx, statements context.Context) error {
	e.mu.Lock()
	free := e.maxRunning() - len(e.running)
	e.mu.Unlock()
	if free <= 0 {
		return nil
	}
	db := e.DB.WithContext(statements)
	var pending []model.KernelNodeOperation
	if err := db.Where("state = ?", statePending).Order("id").Limit(free * 4).Find(&pending).Error; err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	var busyKeys []string
	if err := db.Model(&model.KernelNodeOperation{}).Where("state IN ? AND resource_key <> ''", startedStates).
		Distinct().Pluck("resource_key", &busyKeys).Error; err != nil {
		return err
	}
	busy := make(map[string]bool, len(busyKeys))
	for _, key := range busyKeys {
		busy[key] = true
	}
	for i := range pending {
		if free == 0 || ctx.Err() != nil {
			// A stopping dispatcher starts nothing more.
			break
		}
		op := pending[i]
		if op.ResourceKey != "" && busy[op.ResourceKey] {
			continue
		}
		claimed, err := e.claimOne(statements, &op)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if op.ResourceKey != "" {
			busy[op.ResourceKey] = true
		}
		free--
		e.dispatch(ctx, statements, op)
	}
	return nil
}

func (e *Engine) claimOne(ctx context.Context, op *model.KernelNodeOperation) (bool, error) {
	claimed := false
	err := e.write(ctx, func(tx *gorm.DB) error {
		claimed = false
		now := e.now()
		result := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND state = ?", op.OperationID, statePending).
			Updates(map[string]any{"state": stateDispatching, "attempt": gorm.Expr("attempt + 1"), "updated_at": now})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		op.State = stateDispatching
		op.Attempt++
		claimed = true
		return appendEvent(tx, op, now)
	})
	if claimed {
		e.changed()
	}
	return claimed, err
}

// dispatch hands a claimed operation to its executor. A kind without one
// (a kernel rolled back past the release that served it) ends FAILED at
// once: an operation never waits for an executor that is not there.
func (e *Engine) dispatch(ctx, statements context.Context, op model.KernelNodeOperation) {
	executor, served := e.Executors.Lookup(op.Kind)
	if !served {
		e.endLogged(op.OperationID, startedStates, ending{state: stateFailed, err: &kernelnodeopsv1.OperationError{
			Code:    kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL,
			Message: fmt.Sprintf("operation kind %q has no executor in this kernel", op.Kind), Retryable: true,
		}})
		return
	}
	operation, err := decodeOperation(op.Operation)
	var targets map[string][]*kernelnodeopsv1.NodeRef
	if err == nil {
		targets, err = loadTargets(e.DB.WithContext(statements), []string{op.OperationID})
	}
	if err != nil {
		e.endLogged(op.OperationID, startedStates, ending{state: stateFailed, err: &kernelnodeopsv1.OperationError{
			Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, Message: "the stored operation cannot be read", Retryable: false,
		}})
		return
	}
	run := &Run{
		OperationID: op.OperationID, RequestID: op.RequestID, PackageID: op.PackageID, Kind: op.Kind,
		ParentOperationID: op.ParentOperationID, Operation: operation, Targets: targets[op.OperationID],
		Attempt: op.Attempt, Deadline: op.DeadlineAt, Request: e.boundRequest(op.PackageID, op.RequestID), engine: e,
	}
	runCtx, cancel := context.WithDeadline(ctx, op.DeadlineAt)
	e.mu.Lock()
	if e.running == nil {
		e.running = map[string]*activeRun{}
	}
	e.running[op.OperationID] = &activeRun{run: run, cancel: cancel, deadline: op.DeadlineAt}
	e.mu.Unlock()
	if op.CancelRequestedAt != nil {
		cancel()
	}
	e.runs.Add(1)
	go func() {
		defer e.runs.Done()
		defer func() {
			cancel()
			e.mu.Lock()
			delete(e.running, op.OperationID)
			e.mu.Unlock()
			e.Wake()
		}()
		outcome := execute(runCtx, executor, run)
		e.complete(ctx, runCtx, run, outcome)
	}()
}

// execute runs an executor, turning a panic into a failure.
func execute(ctx context.Context, executor Executor, run *Run) (outcome Outcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the operation's executor failed", true)
		}
	}()
	return executor.Execute(ctx, run)
}

// complete records an executor's outcome. When the operation ended
// meanwhile (its deadline passed, or it was cancelled while pending in a
// fan-out), the outcome is evidence only: a terminal state never changes.
func (e *Engine) complete(engineCtx, runCtx context.Context, run *Run, outcome Outcome) {
	if engineCtx.Err() != nil && outcome.kind == outcomeCancelled {
		// The kernel is stopping: leave the operation started; the next
		// dispatcher recovers it.
		return
	}
	secrets := run.usedSecrets()
	if outcome.kind == outcomeFanOut {
		if err := e.fanOut(run, outcome.children); err != nil {
			log.Printf("Kernel node operations: fan-out of %s failed: %v", run.OperationID, err)
		}
		return
	}
	defer e.dropBinding(run.PackageID, run.RequestID)
	// The ledger keeps a result without its handles; the submitting call,
	// bound to the request they were minted for, is answered them.
	stored, revealed := withoutHandles(scrubResult(outcome.result, secrets))
	if revealed != nil && run.Request != nil {
		e.keepReveal(run.OperationID, revealed, run.Request)
	}
	end := ending{result: stored, err: scrubError(outcome.err, secrets), channel: channelName(outcome.channel)}
	switch outcome.kind {
	case outcomeSucceeded:
		end.state = stateSucceeded
	case outcomeFailed:
		end.state = stateFailed
	case outcomeCancelled:
		end.state = stateCancelled
	default:
		end.state = stateFailed
		end.err = &kernelnodeopsv1.OperationError{Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, Message: "the operation's executor returned no outcome", Retryable: true}
	}
	if end.state != stateSucceeded && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		message := "the operation's deadline passed"
		if end.err.GetMessage() != "" {
			message += ": " + end.err.GetMessage()
		}
		end.state, end.err = stateTimedOut, &kernelnodeopsv1.OperationError{
			Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, Message: message, Retryable: true,
		}
	}
	applied, err := e.endFor(run.OperationID, startedStates, end)
	if err != nil {
		log.Printf("Kernel node operations: recording %s failed: %v", run.OperationID, err)
		return
	}
	if !applied {
		e.recordEvidence(run.OperationID, end)
	}
}

func scrubError(err *kernelnodeopsv1.OperationError, secrets []string) *kernelnodeopsv1.OperationError {
	if err == nil {
		return nil
	}
	copied := proto.Clone(err).(*kernelnodeopsv1.OperationError)
	copied.Message = scrubText(copied.GetMessage(), secrets)
	return copied
}

// recordEvidence keeps an outcome that arrived after the operation ended.
func (e *Engine) recordEvidence(operationID string, end ending) {
	evidence := map[string]any{"state": end.state, "at": e.now().UTC().Format(time.RFC3339Nano)}
	if end.result != nil {
		if encoded, err := storeJSON.Marshal(end.result); err == nil && len(encoded) <= maxResultBytes {
			evidence["result"] = string(encoded)
		}
	}
	if end.err != nil {
		evidence["error"] = map[string]any{"code": errorCodeName(end.err.GetCode()), "message": end.err.GetMessage()}
	}
	encoded, err := encodeJSON(evidence)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	if err := e.DB.WithContext(ctx).Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND evidence = ''", operationID).
		Updates(map[string]any{"evidence": encoded, "updated_at": e.now()}).Error; err != nil {
		log.Printf("Kernel node operations: recording evidence of %s failed: %v", operationID, err)
	}
}

// end moves an operation to a terminal state in its own transaction.
func (e *Engine) end(ctx context.Context, operationID string, from []string, end ending) (bool, error) {
	applied := false
	err := e.write(ctx, func(tx *gorm.DB) error {
		applied = false
		op, found, err := loadOperation(tx, operationID)
		if err != nil || !found {
			return err
		}
		applied, err = e.endTx(tx, &op, from, end)
		return err
	})
	if applied {
		e.changed()
	}
	return applied, err
}

// endFor is end on a context of its own: an outcome is recorded even while
// the kernel stops.
func (e *Engine) endFor(operationID string, from []string, end ending) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	return e.end(ctx, operationID, from, end)
}

func (e *Engine) endLogged(operationID string, from []string, end ending) {
	if _, err := e.endFor(operationID, from, end); err != nil {
		log.Printf("Kernel node operations: ending %s failed: %v", operationID, err)
	}
}

// accept moves a dispatched operation to RUNNING and records its channel
// and links.
func (e *Engine) accept(ctx context.Context, run *Run, acceptance Acceptance) error {
	accepted := false
	err := e.write(ctx, func(tx *gorm.DB) error {
		accepted = false
		op, found, err := loadOperation(tx, run.OperationID)
		if err != nil {
			return err
		}
		if !found || Terminal(op.State) {
			return ErrOperationEnded
		}
		now := e.now()
		updates := map[string]any{"state": stateRunning, "updated_at": now}
		if op.AcceptedAt == nil {
			updates["accepted_at"] = now
		}
		if channel := channelName(acceptance.Channel); channel != "" {
			updates["channel"] = channel
		}
		if acceptance.NodeRevision != 0 {
			updates["node_revision"] = acceptance.NodeRevision
		}
		if acceptance.KernelOperationID != "" {
			updates["kernel_operation_id"] = acceptance.KernelOperationID
		}
		if acceptance.ForwardRuntimeJobID != 0 {
			updates["forward_runtime_job_id"] = acceptance.ForwardRuntimeJobID
		}
		result := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND state IN ?", op.OperationID, startedStates).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOperationEnded
		}
		if op.State == stateRunning {
			return nil
		}
		op.State = stateRunning
		accepted = true
		return appendEvent(tx, &op, now)
	})
	if accepted {
		e.changed()
	}
	return err
}

// fanOut records a fan-out's children and moves the parent to RUNNING; it
// ends with its last child. A child whose target is gone is recorded
// FAILED (TARGET_GONE) and counted.
func (e *Engine) fanOut(run *Run, children []*kernelnodeopsv1.OperationSpec) error {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	ended := false
	err := e.write(ctx, func(tx *gorm.DB) error {
		ended = false
		parent, found, err := loadOperation(tx, run.OperationID)
		if err != nil || !found || Terminal(parent.State) {
			return err
		}
		if parent.CancelRequestedAt != nil {
			ended, err = e.endTx(tx, &parent, startedStates, fanOutEnding(&parent))
			return err
		}
		parentKind := kinds[parent.Kind]
		var failed uint32
		now := e.now()
		for index, child := range children {
			childKind, err := kindOf(child)
			if err == nil && (childKind.fanOut || childKind.family != parentKind.family) {
				err = fmt.Errorf("a %s operation may not create %s operations", parent.Kind, childKind.name)
			}
			if err == nil {
				err = childKind.check(child)
			}
			if err != nil {
				ended, err = e.endTx(tx, &parent, startedStates, ending{state: stateFailed, err: &kernelnodeopsv1.OperationError{
					Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, Message: "the fan-out created an invalid operation: " + errorText(err),
				}})
				return err
			}
			gone, err := e.recordChild(tx, &parent, index, childKind, child, now)
			if err != nil {
				return err
			}
			if gone {
				failed++
			}
		}
		total := uint32(len(children)) // #nosec G115 -- a fan-out creates far fewer than 2^32 operations.
		updates := map[string]any{
			"state": stateRunning, "fan_out_total": total, "fan_out_failed": failed, "fan_out_succeeded": 0,
			"fan_out_pending": total - failed, "updated_at": now,
		}
		if parent.AcceptedAt == nil {
			updates["accepted_at"] = now
		}
		result := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND state IN ?", parent.OperationID, startedStates).Updates(updates)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if parent.State != stateRunning {
			parent.State = stateRunning
			if err := appendEvent(tx, &parent, now); err != nil {
				return err
			}
		}
		parent.FanOutTotal, parent.FanOutFailed, parent.FanOutPending = total, failed, total-failed
		if parent.FanOutPending == 0 {
			_, err := e.endTx(tx, &parent, []string{stateRunning}, fanOutEnding(&parent))
			return err
		}
		return nil
	})
	if err == nil || ended {
		e.changed()
		e.Wake()
	}
	return err
}

func errorText(err error) string {
	if s, ok := status.FromError(err); ok {
		return s.Message()
	}
	return err.Error()
}

// recordChild writes one child of a fan-out. It reports whether the
// child's target was gone, so that it was recorded FAILED.
func (e *Engine) recordChild(tx *gorm.DB, parent *model.KernelNodeOperation, index int, k *kind, spec *kernelnodeopsv1.OperationSpec, now time.Time) (bool, error) {
	operation := canonical(spec)
	dg, err := digest(k.name, operation)
	if err != nil {
		return false, err
	}
	encoded, err := encodeOperation(operation)
	if err != nil {
		return false, err
	}
	resolved, resolveErr := k.resolve(tx, operation)
	gone := status.Code(resolveErr) == codes.NotFound
	if resolveErr != nil && !gone {
		return false, resolveErr
	}
	row := model.KernelNodeOperation{
		OperationID: newOperationID(), RequestID: fmt.Sprintf("%s%s:%d", fanOutRequestIDPrefix, parent.OperationID, index),
		Digest: dg, PackageID: parent.PackageID, PackageGeneration: parent.PackageGeneration, SubmittedBy: parent.SubmittedBy,
		Family: families[k.family].name, Kind: k.name, ResourceKey: resolved.resource, State: statePending,
		ParentOperationID: parent.OperationID, Operation: encoded, CreatedAt: now, UpdatedAt: now, DeadlineAt: parent.DeadlineAt,
	}
	if gone {
		row.State, row.FinishedAt = stateFailed, &now
		row.ErrorCode = errorCodeName(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE)
		row.ErrorMessage = errorText(resolveErr)
	} else if err := e.supersede(tx, k, operation, resolved.resource, parent.OperationID); err != nil {
		return false, err
	}
	return gone, insertOperation(tx, &row, resolved.targets, now)
}

// recoverStarted settles the operations a stopped kernel left started. A
// cancelled one ends CANCELLED; a fan-out whose children exist goes on with
// them; a level-triggered one is dispatched again (it converges on the
// current state); any other ends FAILED, retryable, since whether it
// applied is unknown.
func (e *Engine) recoverStarted(ctx context.Context) error {
	var started []model.KernelNodeOperation
	if err := e.DB.WithContext(ctx).Where("state IN ?", startedStates).Order("id").Find(&started).Error; err != nil {
		return err
	}
	for i := range started {
		op := started[i]
		k := kinds[op.Kind]
		var err error
		switch {
		case e.isRunning(op.OperationID):
			// A previous Run of this engine still winds it down.
		case op.CancelRequestedAt != nil:
			_, err = e.end(ctx, op.OperationID, startedStates, ending{state: stateCancelled, err: &kernelnodeopsv1.OperationError{
				Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, Message: cancelMessage(op.CancelReason),
			}})
		case k != nil && k.fanOut && op.State == stateRunning:
		case k != nil && k.levelTriggered():
			err = e.requeue(ctx, op.OperationID)
		default:
			_, err = e.end(ctx, op.OperationID, startedStates, ending{state: stateFailed, err: &kernelnodeopsv1.OperationError{
				Code:    kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL,
				Message: "the kernel stopped while the operation ran; whether it applied is unknown", Retryable: true,
			}})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) requeue(ctx context.Context, operationID string) error {
	requeued := false
	err := e.write(ctx, func(tx *gorm.DB) error {
		requeued = false
		op, found, err := loadOperation(tx, operationID)
		if err != nil || !found {
			return err
		}
		now := e.now()
		result := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND state IN ?", operationID, startedStates).
			Updates(map[string]any{"state": statePending, "updated_at": now})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		op.State = statePending
		requeued = true
		return appendEvent(tx, &op, now)
	})
	if requeued {
		e.changed()
	}
	return err
}

// cancel asks an operation to stop (CancelOperation). A pending operation
// ends CANCELLED at once; a started one has its executor's context
// cancelled and ends CANCELLED, or with the outcome its channel reported
// first. Cancelling a fan-out cancels its children. An ended operation is
// left as it is.
func (e *Engine) cancel(ctx context.Context, operationID, reason string) error {
	var signal []string
	err := e.write(ctx, func(tx *gorm.DB) error {
		signal = signal[:0]
		op, found, err := loadOperation(tx, operationID)
		if err != nil || !found {
			return err
		}
		ids, err := e.cancelTx(tx, &op, reason)
		signal = ids
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range signal {
		e.stop(id)
	}
	e.changed()
	return nil
}

// cancelTx cancels op and, for a fan-out, its children; it returns the
// started operations whose executors must stop.
func (e *Engine) cancelTx(tx *gorm.DB, op *model.KernelNodeOperation, reason string) ([]string, error) {
	switch {
	case Terminal(op.State):
		return nil, nil
	case op.State == statePending:
		_, err := e.endTx(tx, op, []string{statePending}, ending{state: stateCancelled, err: &kernelnodeopsv1.OperationError{
			Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, Message: cancelMessage(reason),
		}})
		return nil, err
	}
	now := e.now()
	if op.CancelRequestedAt == nil {
		if err := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND cancel_requested_at IS NULL", op.OperationID).
			Updates(map[string]any{"cancel_requested_at": now, "cancel_reason": reason, "updated_at": now}).Error; err != nil {
			return nil, err
		}
		op.CancelRequestedAt, op.CancelReason = &now, reason
	}
	signal := []string{op.OperationID}
	var children []model.KernelNodeOperation
	if err := tx.Where("parent_operation_id = ? AND state IN ?", op.OperationID, activeStates).Order("id").Find(&children).Error; err != nil {
		return nil, err
	}
	for i := range children {
		ids, err := e.cancelTx(tx, &children[i], reason)
		if err != nil {
			return nil, err
		}
		signal = append(signal, ids...)
	}
	return signal, nil
}

// waitFor answers an operation once it satisfies done, or as it is when
// timeout passes or ctx ends.
func (e *Engine) waitFor(ctx context.Context, operationID string, timeout time.Duration, done func(*model.KernelNodeOperation) bool) (model.KernelNodeOperation, error) {
	deadline := time.Now().Add(timeout)
	for {
		changed := e.notify.wait()
		op, found, err := loadOperation(e.DB.WithContext(ctx), operationID)
		if err != nil {
			return op, err
		}
		if !found {
			return op, status.Error(codes.NotFound, "operation not found")
		}
		remaining := time.Until(deadline)
		if done(&op) || remaining <= 0 {
			return op, nil
		}
		timer := time.NewTimer(min(remaining, 250*time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return op, nil
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// Prune deletes the operations that ended before the retention and the
// events older than theirs. A request id pruned may be used again.
func (e *Engine) Prune(ctx context.Context, now time.Time) (int64, error) {
	db := e.DB.WithContext(ctx)
	events := db.Where("created_at < ?", now.Add(-EventRetention)).Delete(&model.KernelNodeOperationEvent{})
	if events.Error != nil {
		return 0, events.Error
	}
	var removed int64
	for {
		var ids []string
		if err := db.Model(&model.KernelNodeOperation{}).Where("state NOT IN ? AND finished_at < ?", activeStates, now.Add(-OperationRetention)).
			Order("id").Limit(pruneBatch).Pluck("operation_id", &ids).Error; err != nil {
			return removed, err
		}
		if len(ids) == 0 {
			return removed, nil
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("operation_id IN ?", ids).Delete(&model.KernelNodeOperationTarget{}).Error; err != nil {
				return err
			}
			result := tx.Where("operation_id IN ?", ids).Delete(&model.KernelNodeOperation{})
			removed += result.RowsAffected
			return result.Error
		})
		if err != nil || len(ids) < pruneBatch {
			return removed, err
		}
	}
}

func newOperationID() string { return uuid.NewString() }

// hub wakes everyone waiting for a change.
type hub struct {
	mu      sync.Mutex
	changed chan struct{}
}

func (h *hub) wait() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.changed == nil {
		h.changed = make(chan struct{})
	}
	return h.changed
}

func (h *hub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.changed != nil {
		close(h.changed)
		h.changed = nil
	}
}
