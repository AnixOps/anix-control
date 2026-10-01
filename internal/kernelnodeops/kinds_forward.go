package kernelnodeops

import (
	"context"
	"errors"
	"fmt"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// DefaultJobPoll is how often a forward.apply on a job backend reads its
// runtime job while it waits for the executor on the node, or the local
// Ansible runner, to end it.
const DefaultJobPoll = 250 * time.Millisecond

// Forward executes the forward family's kinds (node-ops-service.md section
// 3.3, NO-7): ApplyForward, ApplyTunnel, SyncForwardBackend and
// ApplyLegacyRule. Each runs the kernel's implementation of the legacy
// route (internal/service), which the legacy handlers run too, over the
// executors the kernel already has: NodeX for the gost backend and the
// legacy rules, the local Ansible job executor and the clean agent job
// queue. The operation carries ids, never a payload: the kernel reads the
// rows when it executes, so a repeat converges on the latest state.
//
// Credentials never leave the kernel. A job's stored payload carries no
// token; the gost executor resolves the ingress node's token when it sends
// the request to NodeX, and only for the node's pinned endpoint (section
// 3.8): an address that is not the pin ends ENDPOINT_UNCONFIRMED.
type Forward struct {
	// JobPoll is how often a job backend's job is read; DefaultJobPoll when
	// zero.
	JobPoll time.Duration
	// Now stamps the results; time.Now when nil.
	Now func() time.Time
}

func (f *Forward) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Forward) jobPoll() time.Duration {
	if f.JobPoll > 0 {
		return f.JobPoll
	}
	return DefaultJobPoll
}

// Register serves the forward kinds on registry.
func (f *Forward) Register(registry *Registry) error {
	for _, served := range []struct {
		kind     string
		executor ExecutorFunc
	}{
		{KindForwardApply, f.applyForward},
		{KindForwardTunnel, f.applyTunnel},
		{KindForwardSyncBackend, f.syncBackend},
		{KindForwardLegacyRule, f.applyLegacyRule},
	} {
		if err := registry.Register(served.kind, served.executor); err != nil {
			return err
		}
	}
	return nil
}

// init serves the forward kinds on DefaultExecutors, after kinds.go's init
// registered the kinds.
func init() {
	if err := (&Forward{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

// forwardActions maps the contract's actions to the runtime's. A forced
// deletion is a deletion that succeeds whatever the node answers.
var forwardActions = map[kernelnodeopsv1.ForwardAction]string{
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE:       model.ForwardRuntimeJobActionCreate,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE:       model.ForwardRuntimeJobActionUpdate,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_DELETE:       model.ForwardRuntimeJobActionDelete,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_FORCE_DELETE: model.ForwardRuntimeJobActionDelete,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_PAUSE:        model.ForwardRuntimeJobActionPause,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_RESUME:       model.ForwardRuntimeJobActionResume,
	kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC:         model.ForwardRuntimeJobActionSync,
}

// backendChannel is the channel a runtime backend carries an operation on.
func backendChannel(backend string) kernelnodeopsv1.Channel {
	switch backend {
	case model.ForwardRuntimeBackendGost:
		return kernelnodeopsv1.Channel_CHANNEL_NODEX
	case model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeBackendIptablesAnsible:
		return kernelnodeopsv1.Channel_CHANNEL_LOCAL_ANSIBLE
	case model.ForwardRuntimeBackendCleanAgent:
		return kernelnodeopsv1.Channel_CHANNEL_CLEAN_AGENT_JOB
	}
	return kernelnodeopsv1.Channel_CHANNEL_UNSPECIFIED
}

// applyOutcome classifies an error of the runtime.
func applyOutcome(ctx context.Context, err error, result *kernelnodeopsv1.OperationResult) Outcome {
	switch {
	case ctx.Err() != nil:
		return Cancelled("the forward change stopped before the node answered").WithResult(result)
	case errors.Is(err, service.ErrForwardNodeEndpointUnconfirmed):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_ENDPOINT_UNCONFIRMED, err.Error(), false).WithResult(result)
	case errors.Is(err, service.ErrPanelForwardNotFound), errors.Is(err, service.ErrPanelTunnelNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, err.Error(), false).WithResult(result)
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, err.Error(), true).WithResult(result)
}

func forwardApplyResult(backend string, status int, message string, jobID uint, at time.Time) *kernelnodeopsv1.OperationResult {
	return &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_ForwardApply{ForwardApply: &kernelnodeopsv1.ForwardApplyResult{
		Backend: backend, RuntimeStatus: int32(status), Message: message, RuntimeJobId: uint64(jobID), SyncedAtUnixMs: at.UnixMilli(), // #nosec G115 -- a runtime status is 0..3.
	}}}
}

// applyForward applies one forward on its node: synchronously through NodeX
// on the gost backend, or as a job the local Ansible executor or the
// node's clean agent runs, which the operation then follows to its end.
func (f *Forward) applyForward(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetApplyForward()
	db := run.engine.DB
	panel := service.NewPanelForwardService(db)
	record, err := panel.GetForwardWithTunnel(nodeID(op.GetForwardId()))
	if err != nil {
		return applyOutcome(ctx, err, nil)
	}
	action, known := forwardActions[op.GetAction()]
	if !known {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "the forward action is not one the runtime applies", false)
	}
	backend, err := panel.RuntimeService().BackendForForward(record)
	if err != nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "resolving the forward runtime backend failed", true)
	}
	channel := backendChannel(backend)
	if channel == kernelnodeopsv1.Channel_CHANNEL_NODEX {
		// NodeX takes the change in the call itself; a job backend takes
		// it when the job is queued, below.
		if outcome, ok := begin(ctx, run, channel); !ok {
			return outcome
		}
	} else if ctx.Err() != nil {
		return Cancelled("the operation ended before it ran").WithChannel(channel)
	}
	nameTargetTokens(run)

	outcome, err := panel.ApplyForwardRuntime(ctx, record, action)
	if outcome == nil {
		return applyOutcome(ctx, err, nil)
	}
	acceptance := Acceptance{Channel: backendChannel(outcome.Backend), ForwardRuntimeJobID: uint64(outcome.JobID)}
	if acceptErr := run.Accept(ctx, acceptance); acceptErr != nil {
		if !errors.Is(acceptErr, ErrOperationEnded) {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "recording the runtime job failed", true).WithChannel(acceptance.Channel)
		}
		if outcome.Async {
			return Cancelled("the operation ended while its runtime job was queued; the job runs on").WithChannel(acceptance.Channel).
				WithResult(forwardApplyResult(outcome.Backend, outcome.Status, outcome.Message, outcome.JobID, f.now()))
		}
	}
	result := forwardApplyResult(outcome.Backend, outcome.Status, outcome.Message, outcome.JobID, f.now())
	if err != nil {
		if op.GetAction() == kernelnodeopsv1.ForwardAction_FORWARD_ACTION_FORCE_DELETE && ctx.Err() == nil {
			// The package may delete the forward whatever the node answered.
			return Succeeded(result)
		}
		return applyOutcome(ctx, err, result)
	}
	if !outcome.Async {
		return Succeeded(result)
	}
	return f.awaitJob(ctx, db, outcome, result)
}

// awaitJob follows a queued job to its end: the operation ends with the
// job's recorded status and message. A job still queued when the operation
// is stopped stays queued: the executor on the node runs it, and the next
// operation on the forward converges on the result.
func (f *Forward) awaitJob(ctx context.Context, db *gorm.DB, outcome *service.ForwardRuntimeOutcome, result *kernelnodeopsv1.OperationResult) Outcome {
	ticker := time.NewTicker(f.jobPoll())
	defer ticker.Stop()
	for {
		var job model.ForwardRuntimeJob
		err := db.WithContext(ctx).First(&job, outcome.JobID).Error
		switch {
		case ctx.Err() != nil:
			return Cancelled("the runtime job is still queued; it runs on, and the next operation on the forward converges").WithResult(result)
		case errors.Is(err, gorm.ErrRecordNotFound):
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the runtime job disappeared before it ended", true).WithResult(result)
		case err != nil:
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "reading the runtime job failed", true).WithResult(result)
		}
		switch job.Status {
		case model.ForwardRuntimeJobStatusSuccess:
			message := job.Result
			if message == "" {
				message = outcome.Message
			}
			return Succeeded(forwardApplyResult(job.Backend, job.Status, message, job.ID, f.now()))
		case model.ForwardRuntimeJobStatusFailed:
			message := job.Error
			if message == "" {
				message = "the runtime job failed"
			}
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, message, true).WithResult(forwardApplyResult(job.Backend, job.Status, message, job.ID, f.now()))
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

// applyTunnel re-applies the active forwards of a tunnel, as the legacy
// tunnel update does, one child forward.apply UPDATE per forward.
func (f *Forward) applyTunnel(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetApplyTunnel()
	ids, err := service.NewPanelForwardService(run.engine.DB).ActiveTunnelForwardIDs(nodeID(op.GetTunnelId()))
	if err != nil {
		if ctx.Err() != nil {
			return Cancelled("the tunnel change stopped before its forwards were listed")
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "listing the tunnel's forwards failed", true)
	}
	children := make([]*kernelnodeopsv1.OperationSpec, 0, len(ids))
	for _, id := range ids {
		children = append(children, &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyForward{
			ApplyForward: &kernelnodeopsv1.ApplyForward{ForwardId: uint64(id), Action: kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE},
		}})
	}
	return FanOut(children...)
}

// syncBackend moves every active forward not on the target backend to it
// and re-applies each, as the administrator's backend sync does: the row
// records the target first, and one child forward.apply SYNC per forward
// applies it there.
func (f *Forward) syncBackend(ctx context.Context, run *Run) Outcome {
	runtime := service.NewPanelForwardService(run.engine.DB).RuntimeService()
	target := run.Operation.GetSyncForwardBackend().GetBackend()
	if target == "" {
		inForce, err := runtime.ResolveBackend()
		if err != nil {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		}
		target = inForce
	}
	target, _ = service.NormalizeForwardRuntimeBackend(target)
	forwards, err := runtime.ForwardsToMoveToBackend(target)
	if err != nil {
		if ctx.Err() != nil {
			return Cancelled("the backend sync stopped before the forwards were listed")
		}
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "listing the forwards failed", true)
	}
	children := make([]*kernelnodeopsv1.OperationSpec, 0, len(forwards))
	for i := range forwards {
		if err := runtime.MoveForwardToBackend(forwards[i].ID, target); err != nil {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, fmt.Sprintf("recording the backend of forward %d failed", forwards[i].ID), true)
		}
		children = append(children, &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyForward{
			ApplyForward: &kernelnodeopsv1.ApplyForward{ForwardId: uint64(forwards[i].ID), Action: kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC},
		}})
	}
	return FanOut(children...)
}

// applyLegacyRule pushes a legacy rule to its relay and exit nodes through
// NodeX, with both nodes' tokens resolved at send time for their pinned
// endpoints.
func (f *Forward) applyLegacyRule(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetApplyLegacyRule()
	db := run.engine.DB
	rules := service.NewForwardRuleService(db, service.NewForwardNodeService(db))
	rule, err := rules.GetByID(nodeID(op.GetRuleId()))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("legacy rule %d not found", op.GetRuleId()), false)
	}
	if err != nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "loading the legacy rule failed", true)
	}
	action, known := forwardActions[op.GetAction()]
	if !known {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "the rule action is not one the runtime applies", false)
	}
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_NODEX); !ok {
		return outcome
	}
	nameTargetTokens(run)
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_LegacyRule{LegacyRule: &kernelnodeopsv1.LegacyRuleResult{}}}
	if err := rules.ApplyRuntime(ctx, rule, action); err != nil {
		result.GetLegacyRule().Message = err.Error()
		return applyOutcome(ctx, err, result)
	}
	result.GetLegacyRule().AppliedOnRelay, result.GetLegacyRule().AppliedOnExit = true, true
	result.GetLegacyRule().Message = "legacy rule " + action + " applied through NodeX"
	return Succeeded(result)
}
