// Package native implements every protocol-runtime route but the agent
// channel in the package itself. Responses are byte-compatible with the
// legacy handlers (internal/tests/protocolruntimecompat).
//
//   - The protocol templates (static data), and the administrator's
//     diagnostic task history and task detail on the kernel's
//     v2_agent_diagnostic_task table, adopted in place
//     (kernel.storage.adopt:v2_agent_diagnostic_task). The kernel keeps
//     writing that table (creating a task, the agents' polls and results).
//   - A node's protocols, their creation, update and deletion, on
//     v2_node_protocol, adopted once the node credential split finalized it
//     (kernel.storage.adopt:v2_node_protocol, a grant the kernel honours only
//     then): its secret positions hold the placeholder and the secrets are
//     the kernel's. The secrets an administrator types reach the package as
//     sealed handles, which it passes to the kernel (PutSecretDocument);
//     the kernel validates and stores them. A deletion retires what the
//     kernel holds for the protocol (RetireProtocol). Until the lease adopts
//     the table, these routes answer from the legacy handler.
//   - Node synchronization, Agent Control status and operations, and the
//     administrator's agent list, monitoring data, task creation and command
//     execution, through the kernel's KernelNodeOps (node.sync,
//     agent.operation, agent.diagnostic and the agent session RPCs): the
//     agents' connections are the kernel's. Without KernelNodeOps they stay
//     legacy.
//
// The agent channel (registration, heartbeat, task poll, result, monitor
// and the WebSocket) is kernel-owned: node credentials checked in the
// kernel, connections and reports in its memory
// (packages/protocol-runtime/control/service.go).
package native

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Leased reports whether the package's storage lease adopts the kernel
	// table, or grants the kernel view, called name
	// (packagestoresdk.Store.Leased); nil leaves the protocol routes
	// legacy.
	Leased func(ctx context.Context, name string) bool
	// NodeOps is the kernel's KernelNodeOps; without it the routes that
	// write protocols or reach agents stay legacy.
	NodeOps kernelnodeopsv1.KernelNodeOpsClient
	// Now defaults to time.Now.
	Now func() time.Time
	// NewToken names a request that carries neither an Idempotency-Key nor
	// an X-Request-ID, in its operations' request ids; it defaults to a
	// random UUID.
	NewToken func() string
}

// Route ids of the routes that need KernelNodeOps.
const (
	ProtocolsRouteID         = "protocol.admin.nodes.id.protocols.get"
	CreateProtocolRouteID    = "protocol.admin.nodes.id.protocols.post"
	UpdateProtocolRouteID    = "protocol.admin.nodes.id.protocols.protocol_id.put"
	DeleteProtocolRouteID    = "protocol.admin.nodes.id.protocols.protocol_id.delete"
	SyncRouteID              = "protocol.admin.nodes.id.sync.post"
	AgentControlRouteID      = "protocol.admin.nodes.id.agent_control.get"
	AgentControlOpsRouteID   = "protocol.admin.nodes.id.agent_control.operations.post"
	AgentListRouteID         = "protocol.admin.agent.list.get"
	AgentMonitorRouteID      = "protocol.admin.agent.monitor.get"
	AgentTasksCreateRouteID  = "protocol.admin.agent.tasks.post"
	AgentExecuteRouteID      = "protocol.admin.agent.execute.post"
	protocolTemplatesRouteID = "protocol.admin.protocol_templates.get"
	diagnosticTasksRouteID   = "protocol.admin.agent.tasks.get"
	diagnosticTaskRouteID    = "protocol.admin.agent.tasks.task_id.get"
)

// Handlers returns the native handlers by route id. A host without
// KernelNodeOps has none for the routes that need it.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		protocolTemplatesRouteID: s.ProtocolTemplates,
		diagnosticTasksRouteID:   s.ListDiagnosticTasks,
		diagnosticTaskRouteID:    s.GetDiagnosticTask,
		ProtocolsRouteID:         s.GetProtocols,
	}
	if s.NodeOps != nil {
		handlers[CreateProtocolRouteID] = s.CreateProtocol
		handlers[UpdateProtocolRouteID] = s.UpdateProtocol
		handlers[DeleteProtocolRouteID] = s.DeleteProtocol
		handlers[SyncRouteID] = s.SyncNode
		handlers[AgentControlRouteID] = s.GetAgentControl
		handlers[AgentControlOpsRouteID] = s.DispatchAgentControlOperation
		handlers[AgentListRouteID] = s.ListAgents
		handlers[AgentMonitorRouteID] = s.GetMonitor
		handlers[AgentTasksCreateRouteID] = s.CreateTask
		handlers[AgentExecuteRouteID] = s.ExecuteCommand
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
