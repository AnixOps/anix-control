package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

type ForwardCleanAgentHandler struct {
	service *service.ForwardCleanAgentService
}

func NewForwardCleanAgentHandler() *ForwardCleanAgentHandler {
	return &ForwardCleanAgentHandler{
		service: service.NewForwardCleanAgentService(database.Get()),
	}
}

func (h *ForwardCleanAgentHandler) Register(c *gin.Context) {
	var req service.ForwardCleanAgentRegisterInput
	if err := bindOptionalCleanAgentJSON(c, &req); err != nil {
		cleanAgentError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	h.fillAgentAuth(c, &req.Token, nil)
	if strings.TrimSpace(req.PublicIP) == "" {
		req.PublicIP = c.ClientIP()
	}

	agent, err := h.service.Register(req)
	if err != nil {
		h.handleAgentError(c, err)
		return
	}
	cleanAgentSuccess(c, gin.H{
		"agentId":           agent.ID,
		"nodeId":            agent.NodeID,
		"heartbeatInterval": defaultCleanAgentHeartbeatInterval(),
	})
}

func (h *ForwardCleanAgentHandler) Heartbeat(c *gin.Context) {
	var req service.ForwardCleanAgentHeartbeatInput
	if err := bindOptionalCleanAgentJSON(c, &req); err != nil {
		cleanAgentError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	h.fillAgentAuth(c, &req.Token, &req.AgentID)
	if strings.TrimSpace(req.PublicIP) == "" {
		req.PublicIP = c.ClientIP()
	}

	actions, err := h.service.Heartbeat(req)
	if err != nil {
		h.handleAgentError(c, err)
		return
	}
	cleanAgentSuccess(c, gin.H{
		"actions":           actions,
		"heartbeatInterval": defaultCleanAgentHeartbeatInterval(),
	})
}

func (h *ForwardCleanAgentHandler) Report(c *gin.Context) {
	var req service.ForwardCleanAgentReportInput
	if err := c.ShouldBindJSON(&req); err != nil {
		cleanAgentError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	h.fillAgentAuth(c, &req.Token, &req.AgentID)

	if err := h.service.Report(req); err != nil {
		h.handleAgentError(c, err)
		return
	}
	cleanAgentSuccess(c, true)
}

func (h *ForwardCleanAgentHandler) fillAgentAuth(c *gin.Context, token *string, agentID *uint) {
	if token != nil && strings.TrimSpace(*token) == "" {
		*token = strings.TrimSpace(c.GetHeader("X-Agent-Token"))
	}
	if agentID != nil && *agentID == 0 {
		if raw := strings.TrimSpace(c.GetHeader("X-Agent-ID")); raw != "" {
			if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
				*agentID = uint(parsed)
			}
		}
	}
}

func (h *ForwardCleanAgentHandler) handleAgentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrForwardCleanAgentUnauthorized):
		cleanAgentError(c, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, service.ErrForwardCleanAgentRevoked):
		cleanAgentError(c, http.StatusForbidden, "agent revoked")
	case errors.Is(err, service.ErrForwardCleanAgentNodeMismatch):
		cleanAgentError(c, http.StatusForbidden, "agent is bound to another node")
	default:
		cleanAgentError(c, http.StatusOK, err.Error())
	}
}

func cleanAgentSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": data,
	})
}

func cleanAgentError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{
		"code": -1,
		"msg":  msg,
		"data": nil,
	})
}

func bindOptionalCleanAgentJSON(c *gin.Context, req any) error {
	if err := c.ShouldBindJSON(req); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func defaultCleanAgentHeartbeatInterval() int {
	if cfg := config.Get(); cfg != nil && cfg.ForwardRuntime.CleanAgent.HeartbeatIntervalSeconds > 0 {
		return cfg.ForwardRuntime.CleanAgent.HeartbeatIntervalSeconds
	}
	return 10
}
