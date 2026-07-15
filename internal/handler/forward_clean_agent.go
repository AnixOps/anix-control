package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

func (h *ForwardCleanAgentHandler) InstallScript(c *gin.Context) {
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, buildForwardCleanAgentInstallScript(resolveCleanAgentPublicURL(c)))
}

func (h *ForwardCleanAgentHandler) ListAgents(c *gin.Context) {
	agents, err := h.service.ListAgents()
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, gin.H{
		"list":  agents,
		"total": len(agents),
	})
}

func (h *ForwardCleanAgentHandler) CreateAgentToken(c *gin.Context) {
	var req service.ForwardCleanAgentCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "invalid request body")
		return
	}
	result, err := h.service.CreateToken(req)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, result)
}

func (h *ForwardCleanAgentHandler) RevokeAgent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		panelError(c, "invalid agent id")
		return
	}
	if err := h.service.RevokeAgent(uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "agent not found")
			return
		}
		panelError(c, err.Error())
		return
	}
	panelSuccess(c, true)
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

func resolveCleanAgentPublicURL(c *gin.Context) string {
	if cfg := config.Get(); cfg != nil {
		if publicURL := strings.TrimRight(strings.TrimSpace(cfg.ForwardRuntime.CleanAgent.PublicURL), "/"); publicURL != "" {
			return publicURL
		}
	}

	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

func defaultCleanAgentHeartbeatInterval() int {
	if cfg := config.Get(); cfg != nil && cfg.ForwardRuntime.CleanAgent.HeartbeatIntervalSeconds > 0 {
		return cfg.ForwardRuntime.CleanAgent.HeartbeatIntervalSeconds
	}
	return 10
}

func buildForwardCleanAgentInstallScript(publicURL string) string {
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if publicURL == "" {
		publicURL = "https://panel.example.com"
	}
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

PANEL_URL="${PANEL_URL:-%s}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
NODE_ID="${NODE_ID:-}"
CONFIG_DIR="/etc/v2board-forward-agent"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
BIN="/usr/local/bin/v2forward-agent"
SERVICE_FILE="/etc/systemd/system/v2forward-agent.service"

if [ -z "${AGENT_TOKEN}" ]; then
  echo "AGENT_TOKEN is required. Create it from /api/v2/admin/forward/agents first." >&2
  exit 1
fi

if [ ! -x "${BIN}" ]; then
  echo "${BIN} is missing. Build or place the clean-room v2forward-agent binary before enabling the service." >&2
  exit 1
fi

install -d -m 0750 "${CONFIG_DIR}"
cat > "${CONFIG_FILE}" <<EOF
panel_url: "${PANEL_URL}"
agent_token: "${AGENT_TOKEN}"
node_id: "${NODE_ID}"
heartbeat_interval: 10s
action_timeout: 120s
EOF
chmod 0600 "${CONFIG_FILE}"

cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=AnixOps Agent Forward Compatibility Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN} -c ${CONFIG_FILE}
Restart=always
RestartSec=3
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now v2forward-agent
systemctl status v2forward-agent --no-pager
`, publicURL)
}
