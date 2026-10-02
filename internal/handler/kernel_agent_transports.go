package handler

import (
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentTransportsHandler serves the agent transport inventory
// (node-ops-service.md, section 5.6): which channel each node's agent was
// last seen on, before agent_control.mtls becomes required.
type AgentTransportsHandler struct {
	policy agenttransport.Policy
	db     func() *gorm.DB
	live   func() *agenttransport.Recorder
}

// NewAgentTransportsHandler reads the kernel database and this process's
// live sightings under policy, the router's agent_control.
func NewAgentTransportsHandler(policy agenttransport.Policy) *AgentTransportsHandler {
	return &AgentTransportsHandler{policy: policy, db: database.Get, live: agenttransport.Default}
}

// List godoc
// @Summary Agent transport inventory
// @Description Lists every proxy and forward node with the transport its agent was last seen on (mtls-stream, apikey-stream, http-legacy, websocket, clean-agent, uniproxy, v2board-grpc), its agent version, its newest valid agent certificate and when it was last seen. legacy_only=true keeps the nodes agent_control.mtls: required would refuse.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param legacy_only query bool false "only nodes on a legacy AnixOps Agent channel"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/transports [get]
func (h *AgentTransportsHandler) List(c *gin.Context) {
	legacyOnly := false
	if raw := c.Query("legacy_only"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", "legacy_only must be true or false")
			return
		}
		legacyOnly = parsed
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	inventory, err := agenttransport.Build(c.Request.Context(), db, h.policy, agenttransport.Options{
		LegacyOnly: legacyOnly, Live: h.live(),
	})
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, inventory)
}
