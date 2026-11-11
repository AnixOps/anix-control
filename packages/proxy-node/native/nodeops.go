package native

import (
	"context"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// submit records one operation and waits as asked; wait bounds the wait.
// The request binding goes with it: the kernel resolves the request's
// sealed handles, and mints the handles its answer shows, only for it.
func (s *Service) submit(ctx context.Context, request pluginhostsdk.NativeRequest, requestID string, spec *kernelnodeopsv1.OperationSpec, mode kernelnodeopsv1.WaitMode, wait time.Duration) (*kernelnodeopsv1.Operation, error) {
	submission := &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: requestID, Operation: spec, Wait: mode, WaitTimeoutMs: uint32(wait / time.Millisecond), // #nosec G115 -- at most a minute.
	}
	if len(request.Binding) > 0 {
		submission.Request = &kernelnodeopsv1.RequestBinding{BridgeCapability: request.Binding}
	}
	response, err := s.NodeOps.SubmitOperation(ctx, submission)
	if err != nil {
		return nil, err
	}
	return response.GetOperation(), nil
}

// succeeded reports whether an operation ended SUCCEEDED.
func succeeded(operation *kernelnodeopsv1.Operation) bool {
	return operation.GetState() == kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED
}

// proxyNode is a proxy node reference.
func proxyNode(id uint) *kernelnodeopsv1.NodeRef {
	return &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: uint64(id)}
}
