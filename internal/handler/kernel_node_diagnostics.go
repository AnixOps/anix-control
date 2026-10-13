package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The audit of node diagnostics: the audit middleware skips /api/v4/kernel,
// so the handler writes its own operation log rows (module node_diagnostic,
// action node_diagnostic_<action>), one per call, before the task is sent.
const (
	nodeDiagnosticAuditModule  = "node_diagnostic"
	nodeDiagnosticAuditRefused = "node_diagnostic_refused"
	// nodeDiagnosticWriteSlack is how long after a diagnostic's own time
	// the response may still be written.
	nodeDiagnosticWriteSlack = 15 * time.Second
	nodeDiagnosticListLimit  = 10
)

// NodeDiagnosticsHandler is the administrator API of node diagnostics (5.0
// node page, 诊断): the whitelisted actions of the Agent's agent.diagnostic
// operation (service status, restart, log tail, and the forward checks) run
// on a node's Agent Control stream, with the result answered in the
// request.
type NodeDiagnosticsHandler struct {
	db       func() *gorm.DB
	sources  func() kernelnodeops.AgentSources
	inflight sync.Map
}

// NewNodeDiagnosticsHandler reads the loaded database and the kernel's live
// Agent Control streams on each request.
func NewNodeDiagnosticsHandler() *NodeDiagnosticsHandler {
	return &NodeDiagnosticsHandler{db: database.Get, sources: kernelnodeops.DefaultAgentSources}
}

type nodeDiagnosticRequest struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params"`
	// TimeoutSeconds bounds the wait for the result: 1 to 120, default 30.
	TimeoutSeconds *int `json:"timeout_seconds"`
}

// nodeDiagnosticAudit is the operation log row of one call.
type nodeDiagnosticAudit struct {
	db      *gorm.DB
	id      uint
	content map[string]any
}

// begin writes the row of a call, as refused until it ends. A call whose
// row cannot be written is not run.
func (h *NodeDiagnosticsHandler) begin(c *gin.Context, db *gorm.DB, node agentcontrol.AgentNode, action string) (*nodeDiagnosticAudit, error) {
	name := nodeDiagnosticAuditRefused
	if _, generic := service.AgentDiagnosticActions[action]; generic || service.IsForwardDiagnosticCheck(action) {
		name = "node_diagnostic_" + strings.ReplaceAll(action, ".", "_")
	}
	content := map[string]any{"node": node.String(), "action": truncateText(action, 64), "outcome": "requested"}
	audit := &nodeDiagnosticAudit{db: db, content: content}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var userID *uint
	if id := kernelActorID(c); id != 0 {
		userID = &id
	}
	targetType, targetID := "proxy_node", uint(node.ID)
	if node.Kind == agentcontrol.NodeKindForward {
		targetType = "forward_node"
	}
	row := model.OperationLog{
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: name, Module: nodeDiagnosticAuditModule,
		TargetType: targetType, TargetID: &targetID, Content: string(encoded), IP: truncateText(c.ClientIP(), 45), Status: 2,
	}
	if err := db.WithContext(c.Request.Context()).Create(&row).Error; err != nil {
		return nil, err
	}
	audit.id = row.ID
	return audit, nil
}

// end records how the call ended; extra is merged into the content (never
// the output of the node).
func (a *nodeDiagnosticAudit) end(outcome string, success bool, extra map[string]any) {
	if a == nil {
		return
	}
	a.content["outcome"] = outcome
	for key, value := range extra {
		a.content[key] = value
	}
	encoded, err := json.Marshal(a.content)
	if err != nil {
		return
	}
	status := 2
	if success {
		status = 1
	}
	// The caller may be gone: the row is finished whatever the request's
	// context says.
	err = a.db.WithContext(context.Background()).Model(&model.OperationLog{}).Where("id = ?", a.id).
		Updates(map[string]any{"content": string(encoded), "status": status}).Error
	if err != nil {
		slog.Warn("node diagnostic audit row not finished", slog.Uint64("id", uint64(a.id)), slog.String("error", err.Error()))
	}
}

func isNotAllowed(err error) bool {
	var notAllowed *kernelnodeops.ActionNotAllowedError
	return errors.As(err, &notAllowed)
}

func truncateText(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func nodeExists(db *gorm.DB, node agentcontrol.AgentNode) (bool, error) {
	var table any = &model.Node{}
	if node.Kind == agentcontrol.NodeKindForward {
		table = &model.ForwardNode{}
	}
	var count int64
	if err := db.Model(table).Where("id = ?", node.ID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Run godoc
// @Summary Run a diagnostic on a node
// @Description Runs one whitelisted diagnostic on the node's Agent over the Agent Control stream and answers its result within the request. Generic actions: service_status, service_restart (super administrators only) and log_tail (params service "gost", lines up to 1000); forward checks forward.listen, forward.port_conflict, forward.connect and forward.udp_probe (params route_id, hop_index, optional generation, timeout_ms, upstream, target_policy) when the Agent advertises diag.v1. timeout_seconds is 1 to 120, default 30. 200 with success false means the Agent ran the task and it failed; 409 node_offline, diagnostics_unsupported or node_pending_approval when the Agent cannot take it; 409 diagnostic_busy while another diagnostic of the node runs; 504 diagnostic_timeout when the Agent reports no result in time. Every call is recorded in the operation log (module node_diagnostic).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Param request body nodeDiagnosticRequest true "action, params, timeout_seconds"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/diagnostics [post]
func (h *NodeDiagnosticsHandler) Run(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	var request nodeDiagnosticRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	action := strings.TrimSpace(request.Action)
	if action == "" {
		kernelError(c, http.StatusBadRequest, "invalid_request", "action is required")
		return
	}
	var timeout time.Duration
	if request.TimeoutSeconds != nil {
		seconds := *request.TimeoutSeconds
		if seconds < 1 || time.Duration(seconds)*time.Second > kernelnodeops.MaxNodeDiagnosticTimeout {
			kernelError(c, http.StatusBadRequest, "invalid_request", "timeout_seconds must be between 1 and "+
				strconv.Itoa(int(kernelnodeops.MaxNodeDiagnosticTimeout/time.Second)))
			return
		}
		timeout = time.Duration(seconds) * time.Second
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	audit, err := h.begin(c, db, node, action)
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "audit_unavailable", "the operation log could not be written; nothing was sent")
		return
	}

	normalized, _, err := kernelnodeops.ValidateNodeDiagnostic(action, request.Params)
	if err != nil {
		audit.end("refused", false, map[string]any{"reason": truncateText(err.Error(), 200)})
		if isNotAllowed(err) {
			kernelError(c, http.StatusBadRequest, "action_not_allowed", err.Error())
		} else {
			kernelError(c, http.StatusBadRequest, "invalid_params", err.Error())
		}
		return
	}
	audit.content["params"] = normalized

	if kernelnodeops.NodeDiagnosticMutates(action) {
		super, err := service.IsSuperAdmin(db, kernelActorID(c))
		if err != nil {
			audit.end("error", false, nil)
			kernelDBError(c, err)
			return
		}
		if !super {
			audit.end("forbidden", false, nil)
			kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may run "+action+" on a node")
			return
		}
	}
	exists, err := nodeExists(db, node)
	if err != nil {
		audit.end("error", false, nil)
		kernelDBError(c, err)
		return
	}
	if !exists {
		audit.end("node_not_found", false, nil)
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
		return
	}
	if _, busy := h.inflight.LoadOrStore(node.String(), struct{}{}); busy {
		audit.end("busy", false, nil)
		kernelError(c, http.StatusConflict, "diagnostic_busy", "another diagnostic of this node is running: wait for it to finish")
		return
	}
	defer h.inflight.Delete(node.String())

	wait := kernelnodeops.ClampNodeDiagnosticTimeout(timeout)
	// The server's write timeout (server.write_timeout) may be shorter than
	// the wait: this response is allowed to take it.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(wait + nodeDiagnosticWriteSlack))

	result, err := kernelnodeops.RunNodeDiagnostic(c.Request.Context(), db, h.sources(), kernelnodeops.NodeDiagnosticRequest{
		Node: node, Action: action, Params: request.Params, Timeout: timeout,
	})
	if err != nil {
		h.fail(c, audit, err)
		return
	}
	outcome := "completed"
	if !result.Success {
		outcome = "failed"
	}
	audit.end(outcome, result.Success, map[string]any{"task_id": result.TaskID, "success": result.Success, "duration_ms": result.DurationMS})
	kernelData(c, http.StatusOK, result)
}

// fail answers a diagnostic that gave no result.
func (h *NodeDiagnosticsHandler) fail(c *gin.Context, audit *nodeDiagnosticAudit, err error) {
	var validation *kernelnodeops.DiagnosticValidationError
	var unavailable *kernelnodeops.DiagnosticUnavailableError
	var timedOut *kernelnodeops.DiagnosticTimeoutError
	var agentError *kernelnodeops.DiagnosticAgentError
	switch {
	case errors.As(err, &validation):
		audit.end("refused", false, map[string]any{"reason": truncateText(err.Error(), 200)})
		kernelError(c, http.StatusBadRequest, "invalid_params", err.Error())
	case errors.As(err, &unavailable):
		code := map[string]string{
			kernelnodeops.DiagnosticOffline:         "node_offline",
			kernelnodeops.DiagnosticUnsupported:     "diagnostics_unsupported",
			kernelnodeops.DiagnosticPendingApproval: "node_pending_approval",
		}[unavailable.Reason]
		if code == "" {
			code = "diagnostics_unavailable"
		}
		audit.end(code, false, nil)
		kernelError(c, http.StatusConflict, code, unavailable.Message)
	case errors.As(err, &timedOut):
		audit.end("timeout", false, map[string]any{"task_id": timedOut.TaskID, "acknowledged": timedOut.Acknowledged})
		kernelError(c, http.StatusGatewayTimeout, "diagnostic_timeout", timedOut.Error())
	case errors.As(err, &agentError):
		audit.end("agent_error", false, map[string]any{"task_id": agentError.TaskID, "reason": truncateText(agentError.Message, 200)})
		kernelError(c, http.StatusBadGateway, "agent_error", agentError.Message)
	case errors.Is(err, context.Canceled):
		audit.end("canceled", false, nil)
		c.Abort()
	default:
		audit.end("error", false, nil)
		kernelError(c, http.StatusInternalServerError, "internal_error", "the diagnostic could not be run")
	}
}

// List godoc
// @Summary What a node can diagnose, and its latest diagnostics
// @Description The actions the node's Agent runs now (from its Agent Control session: connected, agent_version, diagnostics for agent.diagnostic, forward_checks for diag.v1), the services a task may name, the time bounds, whether the caller may restart services (can_restart), and the newest recorded tasks with their results (limit 1 to 50, default 10; a proxy node's generic actions only: forward nodes and forward checks keep no history, only the operation log).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Param limit query int false "tasks to list, 1 to 50"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/diagnostics [get]
func (h *NodeDiagnosticsHandler) List(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	limit := nodeDiagnosticListLimit
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > kernelnodeops.MaxNodeDiagnosticListLimit {
			kernelError(c, http.StatusBadRequest, "invalid_request", "limit must be between 1 and "+strconv.Itoa(kernelnodeops.MaxNodeDiagnosticListLimit))
			return
		}
		limit = parsed
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	exists, err := nodeExists(db, node)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !exists {
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
		return
	}
	super, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	tasks, err := kernelnodeops.ListNodeDiagnosticTasks(db, node, limit)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	support := kernelnodeops.NodeDiagnosticSupportOf(h.sources(), node)
	kernelData(c, http.StatusOK, gin.H{
		"node":           node.String(),
		"connected":      support.Connected,
		"agent_version":  support.AgentVersion,
		"diagnostics":    support.Diagnostics,
		"forward_checks": support.ForwardChecks,
		"actions":        kernelnodeops.NodeDiagnosticActions(support),
		"services":       service.AgentDiagnosticServices(),
		"log_tail_max":   service.AgentDiagnosticLogTailMaxLines,
		"timeout_seconds": gin.H{
			"default": int(kernelnodeops.DefaultNodeDiagnosticTimeout / time.Second),
			"max":     int(kernelnodeops.MaxNodeDiagnosticTimeout / time.Second),
		},
		"can_restart": super,
		"history":     node.Kind == agentcontrol.NodeKindProxy,
		"tasks":       tasks,
	})
}
