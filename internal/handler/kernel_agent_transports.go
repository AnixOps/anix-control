package handler

import (
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/database"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentTransportsHandler serves the agent transport inventory
// (node-ops-service.md, section 5.6): which channel each node's agent was
// last seen on, and whether agent_control.mtls: required (the default from
// v4.2) would refuse an enabled node.
type AgentTransportsHandler struct {
	policy   agenttransport.Policy
	db       func() *gorm.DB
	live     func() *agenttransport.Recorder
	sessions func() []agentstreams.Session
}

// NewAgentTransportsHandler reads the kernel database, this process's live
// sightings and its live Agent Control stream sessions under policy, the
// router's agent_control.
func NewAgentTransportsHandler(policy agenttransport.Policy) *AgentTransportsHandler {
	return &AgentTransportsHandler{policy: policy, db: database.Get, live: agenttransport.Default, sessions: controlgrpc.GetAgentStreams().Sessions}
}

// List godoc
// @Summary Agent transport inventory
// @Description Lists every proxy and forward node with the transport its agent was last seen on (mtls-stream, apikey-stream, http-legacy, websocket, clean-agent, uniproxy, v2board-grpc), its agent version, its newest valid agent certificate, when it was last seen and its live Agent Control stream session (authentication mtls or api-key, certificate serial, expiry and SAN, negotiated capabilities). legacy_only=true keeps the nodes on a legacy AnixOps Agent channel. summary.ready_for_required (with required_reasons and required_blockers, computed over every node) is false while agent_control.mtls: required, the default from v4.2, would refuse an enabled node: a legacy one or one that never enrolled.
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
		LegacyOnly: legacyOnly, Live: h.live(), Sessions: h.sessions,
	})
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, inventory)
}
