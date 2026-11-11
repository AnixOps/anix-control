package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// agentDiagnosticTaskType is the type of every task an administrator sends
// to an agent: a whitelisted diagnostic action (ValidateAgentDiagnosticTask).
const agentDiagnosticTaskType = kernelnodeops.AgentDiagnosticTaskType

// AgentHandler is the administrators' view of the connected Agents: their
// Agent Control stream sessions and the diagnostic tasks sent to them.
type AgentHandler struct {
	db            *gorm.DB
	diagnosticSvc *service.AgentDiagnosticTaskService
	// streams reads the Agent Control stream sessions at each call; nil
	// reads the kernel's default sources.
	streams func() agentstreams.Streams
}

// NewAgentHandler 创建 Handler
func NewAgentHandler() *AgentHandler {
	db := database.Get()
	return &AgentHandler{
		db:            db,
		diagnosticSvc: service.NewAgentDiagnosticTaskService(db),
	}
}

// sources are the agent sources the handler reads now.
func (h *AgentHandler) sources() kernelnodeops.AgentSources {
	if h.streams != nil {
		return kernelnodeops.AgentSources{Streams: h.streams()}
	}
	return kernelnodeops.DefaultAgentSources()
}

// ========== 管理接口 ==========

// CreateTask godoc
// @Summary 创建任务
// @Description 管理员向节点下发任务
// @Tags 管理端 Agent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTaskRequest true "任务信息"
// @Success 200 {object} map[string]any
// @Router /admin/agent/tasks [post]
func (h *AgentHandler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if taskReqValue, ok := c.Get("task_request"); ok {
		taskReq, ok := taskReqValue.(CreateTaskRequest)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_request payload"})
			return
		}
		req = taskReq
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	// The task goes to the agent with its type, so the type is fixed like
	// the action: only diagnostic tasks, whatever the body says.
	if strings.TrimSpace(req.Type) != agentDiagnosticTaskType {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("task type %q is not allowed: only %q tasks are sent to agents", req.Type, agentDiagnosticTaskType)})
		return
	}
	req.Type = agentDiagnosticTaskType

	// The task goes to the agent on its Agent Control stream
	// (agent.diagnostic), as the KernelNodeOps agent.diagnostic executor
	// sends it.
	run, onStream, err := kernelnodeops.RunAgentDiagnosticOnStream(c.Request.Context(), h.db, h.sources(),
		kernelnodeops.AgentDiagnosticRequest{NodeID: req.NodeID, Action: req.Action, Params: req.Params, Timeout: req.Timeout})
	if !onStream {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node offline"})
		return
	}
	answerStreamDiagnostic(c, req.NodeID, run, err)
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	NodeID  uint           `json:"node_id" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Action  string         `json:"action" binding:"required"`
	Params  map[string]any `json:"params"`
	Timeout int            `json:"timeout"`
}

// ListAgents godoc
// @Summary 获取在线 Agent 列表
// @Description 管理员获取所有在线的 Agent
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /admin/agent/list [get]
func (h *AgentHandler) ListAgents(c *gin.Context) {
	var sessions []agentstreams.Session
	if streams := h.sources().Streams; streams != nil {
		sessions = streams.Sessions()
	}
	kernelnodeops.SortAgentSessions(sessions)
	now := time.Now()
	agents := make([]json.RawMessage, 0, len(sessions))
	for _, session := range sessions {
		agents = append(agents, kernelnodeops.AgentListEntryJSON(session, now))
	}

	panelSuccess(c, gin.H{"agents": agents})
}

// GetMonitor godoc
// @Summary Get latest monitor payload from agent node
// @Description Monitor snapshots came from the retired agent REST channel; Agents on the Agent Control stream report no snapshot, so this answers 404 for every node.
// @Tags 管理端 Agent
// @Produce json
// @Security BearerAuth
// @Param node_id query int true "Node ID"
// @Failure 404 {object} map[string]any
// @Router /admin/agent/monitor [get]
func (h *AgentHandler) GetMonitor(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Query("node_id"), 10, 32)
	if err != nil || nodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid node_id is required"})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "monitor data not found"})
}

// answerStreamDiagnostic answers a diagnostic task sent on the node's Agent
// Control stream: as the WebSocket answer, at the agent's acknowledgement;
// the agent's result reaches the task's row later.
func answerStreamDiagnostic(c *gin.Context, nodeID uint, run *kernelnodeops.AgentDiagnostic, err error) {
	if err != nil {
		var refused *kernelnodeops.DiagnosticValidationError
		if errors.As(err, &refused) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dispatch := run.Dispatch
	if dispatch.DispatchError != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "send failed", "task_id": run.Task.ID, "message_id": dispatch.MessageID,
			"dispatch_err": dispatch.DispatchError.Error(), "channel": "agent_control", "data": json.RawMessage(kernelnodeops.AgentTaskJSON(run.Row)),
		})
		return
	}
	panelSuccess(c, gin.H{
		"message": "task sent", "task_id": run.Task.ID, "node_id": nodeID, "success": true, "output": "task dispatched",
		"duration_ms": int64(0), "message_id": dispatch.MessageID, "ack_received": true, "ack": json.RawMessage(kernelnodeops.AgentAckJSON(dispatch.RawAck)),
		"channel": "agent_control", "data": json.RawMessage(kernelnodeops.AgentTaskJSON(run.Row)),
	})
}
