package kernelnodeops

import (
	"context"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// fanOutForwards is a tunnel fan-out's executor: one forward.apply SYNC per
// forward id.
func fanOutForwards(ids ...uint64) Executor {
	return ExecutorFunc(func(context.Context, *Run) Outcome {
		children := make([]*kernelnodeopsv1.OperationSpec, 0, len(ids))
		for _, id := range ids {
			children = append(children, applyForward(id, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC))
		}
		return FanOut(children...)
	})
}

func children(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, parentID string) []*kernelnodeopsv1.Operation {
	t.Helper()
	list, err := client.ListOperations(context.Background(), &kernelnodeopsv1.ListOperationsRequest{Limit: 500})
	require.NoError(t, err)
	var found []*kernelnodeopsv1.Operation
	for _, operation := range list.GetOperations() {
		if operation.GetParentOperationId() == parentID {
			found = append(found, operation)
		}
	}
	return found
}

// A fan-out creates one child per forward; the parent counts them and ends
// with the last: SUCCEEDED when none failed, FAILED with the counts
// otherwise. Children are not counted against the quotas.
func TestFanOutCountsItsChildren(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.PackageQuota = 2; e.NodeQuota = 2 })
		h.serve(KindForwardTunnel, fanOutForwards(40, 41))
		executor := newHeld()
		h.serve(KindForwardApply, executor)
		h.serve(KindDiagnoseTunnel, newScript(succeed))
		h.start(t)
		client := h.client(forwardHost, allFamilies())

		parent := submit(t, client, "forward.tunnel:30:d", applyTunnel(30)).GetOperation()
		require.NotNil(t, parent.GetFanOut(), "a fan-out answers its counts from the start")
		running := eventually(t, get(t, client, parent.GetOperationId()), inState(running))
		require.Equal(t, &kernelnodeopsv1.FanOut{Total: 2, Pending: 2}, running.GetFanOut())
		executor.next(t)
		executor.next(t)
		kids := children(t, client, parent.GetOperationId())
		require.Len(t, kids, 2)
		for _, kid := range kids {
			require.Equal(t, KindForwardApply, kid.GetKind())
			require.Equal(t, parent.GetDeadlineUnixMs(), kid.GetDeadlineUnixMs())
			require.Contains(t, kid.GetRequestId(), parent.GetOperationId())
		}
		// The two running children count against neither quota: the parent
		// and this diagnosis are the package's two, and node 10's two.
		diagnosis := submit(t, client, "diagnose.tunnel:30:d", sampleOperation(KindDiagnoseTunnel)).GetOperation()
		eventually(t, get(t, client, diagnosis.GetOperationId()), inState(succeeded))

		executor.release <- Succeeded(nil)
		executor.release <- Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED, "nodex refused", true)
		ended := eventually(t, get(t, client, parent.GetOperationId()), inState(failed))
		require.Equal(t, &kernelnodeopsv1.FanOut{Total: 2, Succeeded: 1, Failed: 1}, ended.GetFanOut())
		require.Equal(t, "1 of 2 operations failed", ended.GetError().GetMessage())
		require.Equal(t, []string{statePending, stateDispatching, stateRunning, stateFailed}, h.states(t, parent.GetOperationId()))

		again := submit(t, client, "forward.tunnel:30:e", applyTunnel(30)).GetOperation()
		executor.next(t)
		executor.next(t)
		executor.release <- Succeeded(nil)
		executor.release <- Succeeded(nil)
		ended = eventually(t, get(t, client, again.GetOperationId()), inState(succeeded))
		require.Equal(t, &kernelnodeopsv1.FanOut{Total: 2, Succeeded: 2}, ended.GetFanOut())
		require.Nil(t, ended.GetError())
	})
}

// A fan-out with nothing to do succeeds at once; a child whose target is
// gone is recorded FAILED (TARGET_GONE) and counted.
func TestFanOutEdges(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindForwardTunnel, ExecutorFunc(func(_ context.Context, run *Run) Outcome {
			if run.Operation.GetApplyTunnel().GetTunnelId() == 31 {
				return FanOut()
			}
			return fanOutForwards(40, 99).Execute(context.Background(), run)
		}))
		h.serve(KindForwardSyncBackend, ExecutorFunc(func(context.Context, *Run) Outcome {
			return FanOut(syncNode(proxyKind, 1, false))
		}))
		h.serve(KindForwardApply, newScript(succeed))
		h.start(t)
		client := h.client(forwardHost, allFamilies())

		empty := submit(t, client, "forward.tunnel:31", applyTunnel(31)).GetOperation()
		ended := eventually(t, get(t, client, empty.GetOperationId()), inState(succeeded))
		require.Equal(t, &kernelnodeopsv1.FanOut{}, ended.GetFanOut())

		gone := submit(t, client, "forward.tunnel:30", applyTunnel(30)).GetOperation()
		ended = eventually(t, get(t, client, gone.GetOperationId()), inState(failed))
		require.Equal(t, &kernelnodeopsv1.FanOut{Total: 2, Succeeded: 1, Failed: 1}, ended.GetFanOut())
		var goneChild *kernelnodeopsv1.Operation
		for _, kid := range children(t, client, gone.GetOperationId()) {
			if kid.GetState() == failed {
				goneChild = kid
			}
		}
		require.NotNil(t, goneChild)
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, goneChild.GetError().GetCode())

		// A fan-out may create operations of its own family only.
		crossing := submit(t, client, "forward.sync_backend", sampleOperation(KindForwardSyncBackend)).GetOperation()
		ended = eventually(t, get(t, client, crossing.GetOperationId()), inState(failed))
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, ended.GetError().GetCode())
		require.Empty(t, children(t, client, crossing.GetOperationId()))
	})
}

// Cancelling a fan-out cancels its children; the parent ends CANCELLED
// with its last child.
func TestCancellingAFanOutCancelsItsChildren(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.MaxRunning = 2 })
		h.serve(KindForwardTunnel, fanOutForwards(40, 41))
		executor := newHeld()
		h.serve(KindForwardApply, executor)
		h.start(t)
		client := h.client(forwardHost, allFamilies())
		parent := submit(t, client, "forward.tunnel:30:cancel", applyTunnel(30)).GetOperation()
		executor.next(t)
		kids := children(t, client, parent.GetOperationId())
		require.Len(t, kids, 2)

		answered, err := client.CancelOperation(context.Background(), &kernelnodeopsv1.CancelOperationRequest{OperationId: parent.GetOperationId(), Reason: "tunnel deleted"})
		require.NoError(t, err)
		ended := eventually(t, get(t, client, parent.GetOperationId()), inState(cancelled))
		require.Equal(t, "operation cancelled: tunnel deleted", ended.GetError().GetMessage())
		require.Equal(t, uint32(2), ended.GetFanOut().GetFailed())
		require.Contains(t, []kernelnodeopsv1.OperationState{running, cancelled}, answered.GetOperation().GetState())
		for _, kid := range kids {
			require.Equal(t, cancelled, get(t, client, kid.GetOperationId())().GetState())
		}
	})
}

// The fan-out's request ids are the kernel's: a package cannot claim one.
func TestFanOutRequestIDsAreReserved(t *testing.T) {
	_, err := checkRequestID("fanout:abc:0")
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	id, err := checkRequestID("  forward.tunnel:30:d  ")
	require.NoError(t, err)
	require.Equal(t, "forward.tunnel:30:d", id)
}
