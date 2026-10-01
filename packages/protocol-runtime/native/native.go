// Package native implements three of the protocol-runtime package's routes in
// the package itself: the protocol templates (static data), and the
// administrator's diagnostic task history and task detail on the kernel's
// v2_agent_diagnostic_task table, adopted in place
// (kernel.storage.adopt:v2_agent_diagnostic_task). The kernel keeps writing
// that table (creating a task, the agents' polls and results), so a route can
// switch between legacy and native at any time. Responses are byte-compatible
// with the legacy handlers (internal/tests/protocolruntimecompat).
//
// The other seventeen routes stay bridged; the reasons are in the host's
// bridgedRoutes (packages/protocol-runtime/control/service.go). In short:
// node protocols live in the protected v2_node_protocol (Reality and
// WireGuard private keys, and the rows the kernel builds every node's
// configuration from without validating them again), the agent routes
// authenticate node credentials and keep the agents' live connections in the
// kernel's memory, and the Agent Control routes dispatch over the kernel's
// gRPC control streams.
package native

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// v2_agent_diagnostic_task table is visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"protocol.admin.protocol_templates.get":  s.ProtocolTemplates,
		"protocol.admin.agent.tasks.get":         s.ListDiagnosticTasks,
		"protocol.admin.agent.tasks.task_id.get": s.GetDiagnosticTask,
	}
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

// jsonAnswer is the legacy handlers' c.JSON(code, body): encoding/json, as
// gin uses, with gin's content type.
func jsonAnswer(code int, body any) (pluginhostsdk.NativeResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: uint32(code), Body: encoded, // #nosec G115 -- HTTP status codes.
		Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}

// errorAnswer is the legacy handlers' c.JSON(code, gin.H{"error": message}).
func errorAnswer(code int, message string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"error": message})
}

func internalError(err error) (pluginhostsdk.NativeResponse, error) {
	return errorAnswer(http.StatusInternalServerError, err.Error())
}

// query is gin's c.Query on the forwarded query string.
func query(request pluginhostsdk.NativeRequest, key string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}
