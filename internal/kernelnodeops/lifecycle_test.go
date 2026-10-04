package kernelnodeops

import (
	"context"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var (
	pending    = kernelnodeopsv1.OperationState_OPERATION_STATE_PENDING
	running    = kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING
	succeeded  = kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED
	failed     = kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED
	cancelled  = kernelnodeopsv1.OperationState_OPERATION_STATE_CANCELLED
	timedOut   = kernelnodeopsv1.OperationState_OPERATION_STATE_TIMED_OUT
	superseded = kernelnodeopsv1.OperationState_OPERATION_STATE_SUPERSEDED
)

func nodeSyncResult(hash string) *kernelnodeopsv1.OperationResult {
	return &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_NodeSync{NodeSync: &kernelnodeopsv1.NodeSyncResult{
		Channel: kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, ConfigHash: hash, Changed: true, Revision: 7,
	}}}
}

// An operation moves pending, dispatching, running and then to a terminal
// state that never changes; every change is an event, and a result that
// arrives after the end is evidence only.
func TestTheLifecycleEndsOnceAndForAll(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindNodeSync, executor)
		h.start(t)
		client := h.client(protocolHost, allFamilies())

		submitted := submit(t, client, "node.sync:proxy-1:d", syncNode(proxyKind, 1, true))
		operationID := submitted.GetOperation().GetOperationId()
		run := executor.next(t)
		require.Equal(t, operationID, run.OperationID)
		require.Equal(t, "protocol-runtime", run.PackageID)
		require.EqualValues(t, 1, run.Attempt)
		require.True(t, run.Operation.GetSyncNode().GetForce())
		require.Equal(t, []*kernelnodeopsv1.NodeRef{nodeRef(proxyKind, 1)}, run.Targets)
		accepted := eventually(t, get(t, client, operationID), inState(running))
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_KERNEL, accepted.GetChannel())
		require.NotZero(t, accepted.GetAcceptedAtUnixMs())
		require.Zero(t, accepted.GetFinishedAtUnixMs())

		executor.release <- Succeeded(nodeSyncResult("abc")).WithChannel(kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL)
		ended := eventually(t, get(t, client, operationID), inState(succeeded))
		require.Equal(t, "abc", ended.GetResult().GetNodeSync().GetConfigHash())
		require.EqualValues(t, 7, ended.GetResult().GetNodeSync().GetRevision())
		require.NotZero(t, ended.GetFinishedAtUnixMs())
		require.Nil(t, ended.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_KERNEL, ended.GetChannel(), "the channel accepted first stays")
		require.Equal(t, []string{statePending, stateDispatching, stateRunning, stateSucceeded}, h.states(t, operationID))

		// Terminal is monotonic: no transition leaves it, and a late outcome
		// is kept as evidence.
		applied, err := h.engine.end(context.Background(), operationID, activeStates, ending{state: stateFailed})
		require.NoError(t, err)
		require.False(t, applied)
		h.engine.complete(context.Background(), context.Background(), run, Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, "late", false))
		var row model.KernelNodeOperation
		require.NoError(t, db.Where("operation_id = ?", operationID).Take(&row).Error)
		require.Contains(t, row.Evidence, `"late"`)
		require.Contains(t, row.Evidence, `"failed"`)
		require.Equal(t, succeeded, get(t, client, operationID)().GetState())
	})
}

// endTx never moves a terminal state: every terminal state refuses every
// other.
func TestTerminalStatesNeverChange(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindAgentOperation, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		terminal := []string{stateSucceeded, stateFailed, stateCancelled, stateTimedOut, stateSuperseded}
		for index, from := range terminal {
			response := submit(t, client, "agent.operation:"+from, agentOperation(1, "agent.ping"))
			id := response.GetOperation().GetOperationId()
			applied, err := h.engine.end(context.Background(), id, activeStates, ending{state: from})
			require.NoError(t, err)
			require.True(t, applied)
			for _, to := range terminal {
				// Even a caller that names the terminal state as a source
				// cannot move it: the transitions name active states only.
				applied, err := h.engine.end(context.Background(), id, activeStates, ending{state: to})
				require.NoError(t, err)
				require.False(t, applied, "%s -> %s", from, to)
			}
			require.NoError(t, h.engine.cancel(context.Background(), id, "late"))
			require.NoError(t, h.engine.requeue(context.Background(), id))
			var row model.KernelNodeOperation
			require.NoError(t, db.Where("operation_id = ?", id).Take(&row).Error)
			require.Equal(t, from, row.State, "case %d", index)
			require.Nil(t, row.CancelRequestedAt)
		}
	})
}

// Failures, panics and executors that return nothing end FAILED with a
// reason code; a kind whose executor is gone when it is dispatched (a
// kernel rolled back past it) fails at once instead of waiting.
func TestFailuresEndWithAReasonCode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindAgentOperation, newScript(func(context.Context, *Run) Outcome {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, "agent is not connected", true)
		}))
		h.serve(KindDiagnoseNodeStats, newScript(func(context.Context, *Run) Outcome { panic("boom") }))
		h.serve(KindRegKeyIssue, newScript(func(context.Context, *Run) Outcome { return Outcome{} }))
		h.start(t)
		client := h.client(forwardHost, allFamilies())

		offline := submit(t, client, "agent:offline", agentOperation(1, "agent.ping")).GetOperation()
		ended := eventually(t, get(t, client, offline.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, ended.GetError().GetCode())
		require.Equal(t, "agent is not connected", ended.GetError().GetMessage())
		require.True(t, ended.GetError().GetRetryable())

		panicked := submit(t, client, "stats:panic", sampleOperation(KindDiagnoseNodeStats)).GetOperation()
		ended = eventually(t, get(t, client, panicked.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, ended.GetError().GetCode())

		empty := submit(t, client, "regkey:empty", sampleOperation(KindRegKeyIssue)).GetOperation()
		ended = eventually(t, get(t, client, empty.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, ended.GetError().GetCode())

		// Recorded while the kind had an executor, dispatched by a kernel
		// without one.
		gone := newHarness(t, db)
		gone.serve(KindNodeSync, newScript(succeed))
		proxy := gone.client(proxyHost, allFamilies())
		recorded := submit(t, proxy, "node.sync:rolled-back", syncNode(proxyKind, 2, false)).GetOperation()
		ended = eventually(t, get(t, proxy, recorded.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, ended.GetError().GetCode())
		require.Contains(t, ended.GetError().GetMessage(), "no executor")
		require.Equal(t, []string{statePending, stateDispatching, stateFailed}, h.states(t, recorded.GetOperationId()))
	})
}

// A started operation that passes its deadline ends TIMED_OUT and its
// executor is stopped; a success that arrives later is evidence only and
// does not reverse the decision.
func TestDeadlinesEndStartedOperationsTimedOut(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.Timeout = 300 * time.Millisecond })
		lateRelease := make(chan struct{})
		stopped := make(chan error, 1)
		stubborn := newScript(func(ctx context.Context, run *Run) Outcome {
			<-ctx.Done()
			stopped <- ctx.Err()
			<-lateRelease
			return Succeeded(nodeSyncResult("late"))
		})
		h.serve(KindNodeSync, stubborn)
		h.start(t)
		client := h.client(protocolHost, allFamilies())

		first := submit(t, client, "node.sync:proxy-1:slow", syncNode(proxyKind, 1, true)).GetOperation()
		<-stubborn.started
		require.ErrorIs(t, <-stopped, context.DeadlineExceeded)
		ended := eventually(t, get(t, client, first.GetOperationId()), inState(timedOut))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, ended.GetError().GetCode())
		require.True(t, ended.GetError().GetRetryable())

		close(lateRelease)
		require.Eventually(t, func() bool {
			var row model.KernelNodeOperation
			return db.Where("operation_id = ?", first.GetOperationId()).Take(&row).Error == nil && row.Evidence != ""
		}, 10*time.Second, 10*time.Millisecond)
		var row model.KernelNodeOperation
		require.NoError(t, db.Where("operation_id = ?", first.GetOperationId()).Take(&row).Error)
		require.Contains(t, row.Evidence, `"succeeded"`)
		require.Contains(t, row.Evidence, "late")
		require.Equal(t, stateTimedOut, row.State)
		require.Empty(t, row.Result)
		require.Equal(t, timedOut, get(t, client, first.GetOperationId())().GetState())
	})
}

// An operation still pending at its deadline (no dispatcher took it) ends
// TIMED_OUT without running: nothing waits forever.
func TestDeadlinesEndPendingOperationsTimedOut(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.Timeout = 50 * time.Millisecond })
		executor := newScript(succeed)
		h.serve(KindNodeSync, executor)
		client := h.client(protocolHost, allFamilies())
		operation := submit(t, client, "node.sync:never-ran", syncNode(proxyKind, 1, false)).GetOperation()
		time.Sleep(100 * time.Millisecond)
		h.start(t)
		ended := eventually(t, get(t, client, operation.GetOperationId()), inState(timedOut))
		require.Zero(t, ended.GetAttempt(), "it never ran")
		require.Empty(t, executor.started)
		require.Equal(t, []string{statePending, stateTimedOut}, h.states(t, operation.GetOperationId()))
	})
}

// An executor that fails because its deadline passed is recorded
// TIMED_OUT, with its message.
func TestAFailureAfterTheDeadlineIsATimeout(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.Timeout = 200 * time.Millisecond; e.PollInterval = time.Hour })
		h.serve(KindNodeSync, newScript(func(ctx context.Context, run *Run) Outcome {
			<-ctx.Done()
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, "no acknowledgement", true)
		}))
		h.start(t)
		client := h.client(protocolHost, allFamilies())
		operation := submit(t, client, "node.sync:deadline", syncNode(proxyKind, 1, false)).GetOperation()
		ended := eventually(t, get(t, client, operation.GetOperationId()), inState(timedOut))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, ended.GetError().GetCode())
		require.Contains(t, ended.GetError().GetMessage(), "no acknowledgement")
	})
}

// SubmitOperation waits as asked: ACCEPTED until the channel took the
// operation, TERMINAL until it ended, each bounded by wait_timeout_ms.
func TestWaitModes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindAgentOperation, executor)
		h.start(t)
		client := h.client(protocolHost, allFamilies())
		ctx := context.Background()

		accepted, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "agent:accepted", Operation: agentOperation(1, "agent.ping"), Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED,
		})
		require.NoError(t, err)
		require.Equal(t, running, accepted.GetOperation().GetState())
		executor.next(t)

		begun := time.Now()
		bounded, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "agent:accepted", Operation: agentOperation(1, "agent.ping"), Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL,
			WaitTimeoutMs: 150,
		})
		require.NoError(t, err)
		require.False(t, bounded.GetApplied())
		require.Equal(t, running, bounded.GetOperation().GetState(), "answered as it is when the wait ends")
		require.GreaterOrEqual(t, time.Since(begun), 150*time.Millisecond)

		go func() {
			time.Sleep(50 * time.Millisecond)
			executor.release <- Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_AgentOperation{
				AgentOperation: &kernelnodeopsv1.AgentOperationResult{AgentOperationId: "op-1"},
			}})
		}()
		terminal, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "agent:accepted", Operation: agentOperation(1, "agent.ping"), Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL,
		})
		require.NoError(t, err)
		require.Equal(t, succeeded, terminal.GetOperation().GetState())
		require.Equal(t, "op-1", terminal.GetOperation().GetResult().GetAgentOperation().GetAgentOperationId())
		require.Equal(t, MaxWait, waitTimeout(0))
		require.Equal(t, MaxWait, waitTimeout(600000), "at most 60 s")
	})
}

// A pending operation is cancelled at once; a running one through its
// executor's context; an ended one is answered unchanged. An executor that
// reports its outcome first keeps it.
func TestCancellation(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindNodeSync, executor)
		h.start(t)
		client := h.client(protocolHost, allFamilies())
		ctx := context.Background()

		started := submit(t, client, "node.sync:proxy-1:a", syncNode(proxyKind, 1, true)).GetOperation()
		executor.next(t)
		eventually(t, get(t, client, started.GetOperationId()), inState(running))
		queued := submit(t, client, "node.sync:proxy-1:b", syncNode(proxyKind, 1, true)).GetOperation()

		cancelledQueued, err := client.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: queued.GetOperationId(), Reason: "replaced"})
		require.NoError(t, err)
		require.Equal(t, cancelled, cancelledQueued.GetOperation().GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, cancelledQueued.GetOperation().GetError().GetCode())
		require.Equal(t, "operation cancelled: replaced", cancelledQueued.GetOperation().GetError().GetMessage())
		require.Zero(t, cancelledQueued.GetOperation().GetAttempt())

		cancelledRunning, err := client.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: started.GetOperationId()})
		require.NoError(t, err)
		require.Equal(t, cancelled, cancelledRunning.GetOperation().GetState(), "the executor stopped within the wait")
		require.Equal(t, []string{statePending, stateDispatching, stateRunning, stateCancelled}, h.states(t, started.GetOperationId()))

		again, err := client.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: started.GetOperationId()})
		require.NoError(t, err)
		require.Equal(t, cancelled, again.GetOperation().GetState())
	})
}

// An executor that does not honour a cancellation and reports a success
// keeps it: the channel reported first.
func TestAnOutcomeReportedBeforeTheCancellationStands(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		release := make(chan struct{})
		stubborn := newScript(func(ctx context.Context, run *Run) Outcome {
			if err := run.Accept(ctx, Acceptance{Channel: kernelnodeopsv1.Channel_CHANNEL_NODEX}); err != nil {
				return Cancelled("")
			}
			<-release
			return Succeeded(nil)
		})
		h.serve(KindForwardApply, stubborn)
		h.start(t)
		forward := h.client(forwardHost, allFamilies())
		operation := submit(t, forward, "forward.apply:40:stubborn", applyForward(40, forwardAction)).GetOperation()
		<-stubborn.started
		eventually(t, get(t, forward, operation.GetOperationId()), inState(running))
		answered, err := forward.CancelOperation(context.Background(), &kernelnodeopsv1.CancelOperationRequest{OperationId: operation.GetOperationId()})
		require.NoError(t, err)
		require.Equal(t, running, answered.GetOperation().GetState(), "answered as it is after the cancel wait")
		close(release)
		ended := eventually(t, get(t, forward, operation.GetOperationId()), inState(succeeded, cancelled))
		require.Equal(t, succeeded, ended.GetState())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_NODEX, ended.GetChannel())
	})
}

// Cancellation reaches an executor in another process through the ledger.
func TestCancellationThroughTheLedger(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindNodeSync, executor)
		h.start(t)
		client := h.client(protocolHost, allFamilies())
		operation := submit(t, client, "node.sync:remote-cancel", syncNode(proxyKind, 1, true)).GetOperation()
		executor.next(t)
		eventually(t, get(t, client, operation.GetOperationId()), inState(running))
		// Another kernel process's engine records the request.
		other := newHarness(t, db)
		require.NoError(t, other.engine.cancel(context.Background(), operation.GetOperationId(), "from elsewhere"))
		ended := eventually(t, get(t, client, operation.GetOperationId()), inState(cancelled))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, ended.GetError().GetCode())
	})
}

// The kernel runs one operation per resource at a time; a newer
// level-triggered operation replaces one still pending (SUPERSEDED), but
// only a deletion replaces a pending deletion.
func TestResourcesRunOneAtATimeAndPendingOperationsAreSuperseded(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindForwardApply, executor)
		h.serve(KindNodeSync, executor)
		h.start(t)
		forward := h.client(forwardHost, allFamilies())

		first := submit(t, forward, "forward.apply:40:1", applyForward(40, forwardAction)).GetOperation()
		executor.next(t)
		update := submit(t, forward, "forward.apply:40:2", applyForward(40, forwardAction)).GetOperation()
		pause := submit(t, forward, "forward.apply:40:3", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_PAUSE)).GetOperation()
		require.Equal(t, superseded, get(t, forward, update.GetOperationId())().GetState())
		require.Nil(t, get(t, forward, update.GetOperationId())().GetError())
		require.Equal(t, pending, get(t, forward, pause.GetOperationId())().GetState(), "one operation per forward at a time")
		deletion := submit(t, forward, "forward.apply:40:4", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_DELETE)).GetOperation()
		require.Equal(t, superseded, get(t, forward, pause.GetOperationId())().GetState())
		resume := submit(t, forward, "forward.apply:40:5", applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_RESUME)).GetOperation()
		require.Equal(t, pending, get(t, forward, deletion.GetOperationId())().GetState(), "a captured deletion is not replaced by a change")
		other := submit(t, forward, "forward.apply:41:1", applyForward(41, forwardAction)).GetOperation()
		executor.next(t)
		eventually(t, get(t, forward, other.GetOperationId()), inState(running))

		executor.release <- Succeeded(nil)
		executor.release <- Succeeded(nil)
		eventually(t, get(t, forward, first.GetOperationId()), inState(succeeded))
		executor.next(t)
		require.Equal(t, running, eventually(t, get(t, forward, deletion.GetOperationId()), inState(running)).GetState())
		require.Equal(t, pending, get(t, forward, resume.GetOperationId())().GetState())

		// A forced sync is replaced only by a forced one.
		sync := h.client(protocolHost, allFamilies())
		executor.release <- Succeeded(nil)
		executor.next(t)
		blocker := submit(t, sync, "node.sync:proxy-2:0", syncNode(proxyKind, 2, false)).GetOperation()
		executor.next(t)
		forced := submit(t, sync, "node.sync:proxy-2:1", syncNode(proxyKind, 2, true)).GetOperation()
		plain := submit(t, sync, "node.sync:proxy-2:2", syncNode(proxyKind, 2, false)).GetOperation()
		require.Equal(t, pending, get(t, sync, forced.GetOperationId())().GetState())
		forcedAgain := submit(t, sync, "node.sync:proxy-2:3", syncNode(proxyKind, 2, true)).GetOperation()
		require.Equal(t, superseded, get(t, sync, forced.GetOperationId())().GetState())
		require.Equal(t, superseded, get(t, sync, plain.GetOperationId())().GetState())
		require.Equal(t, pending, get(t, sync, forcedAgain.GetOperationId())().GetState())
		require.Equal(t, running, get(t, sync, blocker.GetOperationId())().GetState())
	})
}

// A kernel that stopped while operations ran leaves them started: the next
// dispatcher runs level-triggered ones again, fails the others (whether they
// applied is unknown), cancels the cancelled ones and lets fan-outs go on.
func TestRecoveryAfterAKernelRestart(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		first := newHarness(t, db)
		executor := newHeld()
		for _, kindName := range []string{KindNodeSync, KindCredentialIssue, KindAgentOperation} {
			first.serve(kindName, executor)
		}
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			first.engine.Run(ctx)
			close(done)
		}()
		client := first.client(proxyHost, allFamilies())
		sync := submit(t, client, "recover:sync", syncNode(proxyKind, 1, true)).GetOperation()
		issue := submit(t, client, "recover:issue", sampleOperation(KindCredentialIssue)).GetOperation()
		agent := submit(t, client, "recover:agent", agentOperation(2, "users.reload")).GetOperation()
		for range 3 {
			executor.next(t)
		}
		for _, id := range []string{sync.GetOperationId(), issue.GetOperationId(), agent.GetOperationId()} {
			eventually(t, get(t, client, id), inState(running))
		}
		require.NoError(t, db.Model(&model.KernelNodeOperation{}).Where("operation_id = ?", agent.GetOperationId()).
			Update("cancel_requested_at", time.Now()).Error)
		// The process stops; its executors see their context end and the
		// engine leaves the operations as they are.
		stop()
		<-done
		for _, id := range []string{sync.GetOperationId(), issue.GetOperationId()} {
			require.Equal(t, running, get(t, client, id)().GetState())
		}

		second := newHarness(t, db)
		rerun := newScript(succeed)
		for _, kindName := range []string{KindNodeSync, KindCredentialIssue, KindAgentOperation} {
			second.serve(kindName, rerun)
		}
		second.start(t)
		resumed := eventually(t, get(t, client, sync.GetOperationId()), inState(succeeded))
		require.EqualValues(t, 2, resumed.GetAttempt(), "dispatched again")
		unknown := eventually(t, get(t, client, issue.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, unknown.GetError().GetCode())
		require.True(t, unknown.GetError().GetRetryable())
		require.Contains(t, unknown.GetError().GetMessage(), "unknown")
		require.Equal(t, cancelled, eventually(t, get(t, client, agent.GetOperationId()), inState(cancelled)).GetState())
	})
}

// GetOperation finds an operation by its id or its request id.
func TestGetOperationBySelector(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		operation := submit(t, client, "node.sync:by-request", syncNode(proxyKind, 1, false)).GetOperation()
		ctx := context.Background()
		byRequest, err := client.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{
			Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "node.sync:by-request"},
		})
		require.NoError(t, err)
		require.Equal(t, operation.GetOperationId(), byRequest.GetOperation().GetOperationId())
		_, err = client.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = client.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{
			Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: "missing"},
		})
		require.Equal(t, codes.NotFound, status.Code(err))
	})
}
