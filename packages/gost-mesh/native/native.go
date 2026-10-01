// Package native implements the gost-mesh package's routes in the package
// itself:
//   - the administrator's gost API connection test, which calls the gost API
//     of the host and port the administrator names, with the token the
//     administrator gives, and reads no table;
//   - the NodeX runtime status and diagnosis, which read the NodeX control
//     plane's address, shared token and timeout through the kernel's
//     KernelSettings contract (namespace nodex; the token, a secret, in
//     clear with kernel.settings.nodex.secrets.v1) and call NodeX's health
//     and runtime status endpoints with that token from the package host.
//
// Responses are byte-compatible with the legacy handlers
// (internal/tests/gostmeshcompat). A host without KernelSettings leaves the
// NodeX routes legacy.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/gin-gonic/gin/binding"
)

// Service holds what the native routes need.
type Service struct {
	// Settings is the kernel's KernelSettings; without it the NodeX status
	// and diagnosis have no native handler and stay legacy.
	Settings Settings
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"gost.admin.forward.test_connection.post": s.TestConnection,
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
// outcome is the answer's data; only a request that does not bind is an
// error.
func (s *Service) TestConnection(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req TestGostConnectionRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(bindError(err))
	}
	client := newClient(fmt.Sprintf("http://%s:%d", req.Host, req.APIPort), req.APIToken)
	if err := client.healthCheck(ctx); err != nil {
		return s.panel(map[string]any{"success": false, "message": err.Error()})
	}
	services, err := client.getServices(ctx)
	if err != nil {
		return s.panel(map[string]any{"success": false, "message": "API connected but failed to get services: " + err.Error()})
	}
	return s.panel(map[string]any{"success": true, "message": "Connection successful", "service_count": services.Count})
}
