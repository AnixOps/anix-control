package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/google/uuid"
)

// maxWait is the longest wait KernelNodeOps allows.
const maxWait = 60 * time.Second

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

// requestID names an operation of a request: prefix, the request's
// Idempotency-Key, else its X-Request-ID, else a random value, and, when
// fresh is set, a random part for an operation a retried request must run
// again; at most the kernel's 128 bytes.
func (s *Service) requestID(request pluginhostsdk.NativeRequest, prefix string, fresh bool) string {
	token := ""
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		if value := header(request, name); value != "" {
			token = value
			break
		}
	}
	if token == "" {
		if s.NewToken != nil {
			token = s.NewToken()
		} else {
			token = uuid.NewString()
		}
	}
	if fresh {
		var raw [8]byte
		_, _ = rand.Read(raw[:])
		token += ":" + hex.EncodeToString(raw[:])
	}
	id := prefix + ":" + token
	if len(id) > 128 {
		id = id[:128]
	}
	return id
}

// header is the request's first value of a forwarded header, trimmed.
func header(request pluginhostsdk.NativeRequest, name string) string {
	for key, values := range request.Metadata.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

// succeeded reports whether an operation ended SUCCEEDED.
func succeeded(operation *kernelnodeopsv1.Operation) bool {
	return operation.GetState() == kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED
}

// proxyNode is a proxy node reference.
func proxyNode(id uint) *kernelnodeopsv1.NodeRef {
	return &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: uint64(id)}
}
