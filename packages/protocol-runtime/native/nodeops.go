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

// ackWait bounds how long a route waits for an operation's acceptance
// beyond the agent's acknowledgement timeout: the kernel dispatches the
// operation, then waits for the acknowledgement for at most that timeout.
const ackWait = 5 * time.Second

// kernelWait bounds a wait for an operation the kernel runs in one
// transaction (secrets.put, protocol.retire).
const kernelWait = 30 * time.Second

// defaultAckTimeout is the kernel's agent acknowledgement timeout when an
// operation names none (kernelnodeops.DefaultAgentAckTimeout), and
// maxAckTimeout its cap.
const (
	defaultAckTimeout = 10 * time.Second
	maxAckTimeout     = 60 * time.Second
)

// submit records one operation and waits as asked; wait bounds the wait.
func (s *Service) submit(ctx context.Context, request pluginhostsdk.NativeRequest, requestID string, spec *kernelnodeopsv1.OperationSpec, mode kernelnodeopsv1.WaitMode, wait time.Duration) (*kernelnodeopsv1.Operation, error) {
	submission := &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: requestID, Operation: spec, Wait: mode, WaitTimeoutMs: uint32(wait / time.Millisecond), // #nosec G115 -- the routes wait at most a minute and a half.
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

// requestToken identifies the HTTP request in the request ids of its
// operations: its Idempotency-Key, else its X-Request-ID, else a fresh
// random value, so a retried request is applied once.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		if value := header(request, name); value != "" {
			return value
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
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

// requestID names one operation of a request: prefix, the request's token
// (cut to fit the kernel's 128 bytes) and, when fresh is set, a random part,
// for an operation a retried request must run again.
func (s *Service) requestID(request pluginhostsdk.NativeRequest, prefix string, fresh bool) string {
	token := s.requestToken(request)
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

// state is an operation's state.
func state(operation *kernelnodeopsv1.Operation) kernelnodeopsv1.OperationState {
	return operation.GetState()
}

// succeeded reports whether an operation ended SUCCEEDED.
func succeeded(operation *kernelnodeopsv1.Operation) bool {
	return state(operation) == kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED
}

// ackTimeout is the kernel's acknowledgement timeout for an operation that
// asks for seconds (kernelnodeops.ackTimeout).
func ackTimeout(seconds uint32) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 {
		return defaultAckTimeout
	}
	return min(timeout, maxAckTimeout)
}

// deadlinePrefix starts the kernel's message for an acknowledgement that
// did not come in time (kernelnodeops.streamFailure); the legacy routes
// answer the dispatch error alone.
const deadlinePrefix = "the agent did not acknowledge in time: "

// dispatchError is the dispatch error an operation that failed before the
// agent's acknowledgement carries, as the legacy routes answer it.
func dispatchError(operation *kernelnodeopsv1.Operation) string {
	message := operation.GetError().GetMessage()
	if operation.GetError().GetCode() == kernelnodeopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED {
		message = strings.TrimPrefix(message, deadlinePrefix)
	}
	return message
}
