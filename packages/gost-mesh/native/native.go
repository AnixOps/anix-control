// Package native implements the gost-mesh package's routes in the package
// itself:
//   - the administrator's gost API connection test, which the kernel runs
//     (KernelNodeOps diagnose.forward_backend): the host and port the
//     administrator names and the sealed handle of the token the kernel's
//     gateway substituted are submitted with the request binding, and the
//     kernel dials; the package never sees the token;
//   - the NodeX runtime status and diagnosis, which read the NodeX control
//     plane's address, shared token and timeout through the kernel's
//     KernelSettings contract (namespace nodex; the token, a secret, in
//     clear with kernel.settings.nodex.secrets.v1) and call NodeX's health
//     and runtime status endpoints with that token from the package host.
//
// Responses are byte-compatible with the legacy handlers
// (internal/tests/gostmeshcompat). A host without KernelSettings leaves the
// NodeX routes legacy; one without KernelNodeOps leaves the connection test
// legacy.
package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/gin-gonic/gin/binding"
)

// TestConnectionRouteID is the connection test's route id.
const TestConnectionRouteID = "gost.admin.forward.test_connection.post"

// connectionTestWait bounds how long the host waits for the kernel's dial:
// the kernel's gost client times out each of its two calls after 10 s.
const connectionTestWait = 30 * time.Second

// Service holds what the native routes need.
type Service struct {
	// Settings is the kernel's KernelSettings; without it the NodeX status
	// and diagnosis have no native handler and stay legacy.
	Settings Settings
	// NodeOps is the kernel's KernelNodeOps; without it the connection
	// test has no native handler and stays legacy.
	NodeOps kernelnodeopsv1.KernelNodeOpsClient
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{}
	if s.NodeOps != nil {
		handlers[TestConnectionRouteID] = s.TestConnection
	}
	if s.Settings != nil {
		handlers[NodeXStatusRouteID] = s.NodeXStatus
		handlers[NodeXDoctorRouteID] = s.NodeXDoctor
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// TestGostConnectionRequest is the kernel's handler.TestGostConnectionRequest;
// its name appears in binding errors.
type TestGostConnectionRequest struct {
	Host     string `json:"host" binding:"required"`
	APIPort  int    `json:"api_port" binding:"required"`
	APIToken string `json:"api_token"`
}

// bindError is the message of the legacy handler's binding error. A body
// that is not a JSON object fails to decode as a whole, and encoding/json
// then names the Go type with its package: the kernel's is
// handler.TestGostConnectionRequest. Every other message is the same.
func bindError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Struct == "" && typeErr.Field == "" {
		return strings.Replace(err.Error(), "native.TestGostConnectionRequest", "handler.TestGostConnectionRequest", 1)
	}
	return err.Error()
}

// TestConnection is POST /api/v2/admin/forward/test-connection: whether the
// gost API at host:api_port answers its service list with the token. The
// kernel dials (diagnose.forward_backend) with the token the gateway sealed
// out of the request; the outcome is the answer's data, as the legacy
// handler answers it. Only a request that does not bind is an error. A
// shadow run has no binding, so it cannot submit, and a kernel refusal
// leaves the request to the legacy handler (ErrNativeUnavailable).
func (s *Service) TestConnection(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req TestGostConnectionRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(bindError(err))
	}
	if s.NodeOps == nil || len(request.Binding) == 0 {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	operation := &kernelnodeopsv1.TestForwardBackend{Host: req.Host, ApiPort: int32(req.APIPort)} // #nosec G115 -- the kernel answers an out-of-range port as the legacy handler does.
	if req.APIToken != "" {
		if !v2compat.IsSealedHandle(req.APIToken) {
			// A value the gateway did not seal (the placeholder) cannot be
			// sent in clear.
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
		operation.Token = &kernelnodeopsv1.SecretRef{Handle: req.APIToken}
	}
	requestID, err := connectionTestRequestID()
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	response, err := s.NodeOps.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: requestID,
		Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_TestForwardBackend{TestForwardBackend: operation}},
		Wait:      kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: uint32(connectionTestWait / time.Millisecond),
		Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: request.Binding},
	})
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	result := response.GetOperation()
	if result.GetState() != kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED {
		message := result.GetError().GetMessage()
		if message == "" {
			message = "the connection test did not complete"
		}
		return s.panelError(message)
	}
	test := result.GetResult().GetForwardBackendTest()
	if !test.GetSuccess() {
		return s.panel(map[string]any{"success": false, "message": test.GetMessage()})
	}
	return s.panel(map[string]any{"success": true, "message": test.GetMessage(), "service_count": int(test.GetServiceCount())})
}

// connectionTestRequestID is a fresh request id: a connection test is
// never repeated by intent.
func connectionTestRequestID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "diagnose.forward_backend:" + hex.EncodeToString(raw[:]), nil
}
