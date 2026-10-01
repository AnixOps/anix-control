package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
)

// AgentPKIHandler administers agent enrollment: one-time credentials that
// enroll a new node's agent (node-ops-service.md, section 5.3). Routes sit
// behind admin authentication and the audit log.
type AgentPKIHandler struct {
	pki func() (*agentpki.Service, error)
}

// NewAgentPKIHandler builds the agent PKI from the loaded configuration on
// each request.
func NewAgentPKIHandler() *AgentPKIHandler {
	return &AgentPKIHandler{pki: func() (*agentpki.Service, error) {
		return agentpki.FromConfig(config.Get(), database.Get())
	}}
}

type createAgentEnrollmentTokenRequest struct {
	// Node is "proxy-<id>" or "forward-<id>".
	Node string `json:"node" binding:"required"`
	// TTLSeconds defaults to one day and may not exceed seven.
	TTLSeconds int64 `json:"ttl_seconds" binding:"omitempty,min=1"`
}

type createdAgentEnrollmentToken struct {
	Enrollment model.AgentEnrollment `json:"enrollment"`
	// Credential is returned once; the kernel stores only its hash.
	Credential string `json:"credential"`
}

// CreateEnrollmentToken issues a one-time agent enrollment credential bound
// to a node.
func (h *AgentPKIHandler) CreateEnrollmentToken(c *gin.Context) {
	var request createAgentEnrollmentTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	node, err := agentcontrol.ParseAgentNode(request.Node)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return
	}
	if request.TTLSeconds > int64(agentpki.MaxEnrollmentCredentialTTL/time.Second) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "ttl_seconds may not exceed 604800 (7 days)")
		return
	}
	pki, err := h.pki()
	switch {
	case errors.Is(err, agentpki.ErrDisabled):
		kernelError(c, http.StatusConflict, "agent_pki_disabled", err.Error())
		return
	case err != nil:
		kernelError(c, http.StatusServiceUnavailable, "agent_pki_unavailable", "the agent PKI is unavailable")
		return
	}
	credential, row, err := pki.CreateEnrollmentToken(c.Request.Context(), agentpki.TokenRequest{
		Node: node, TTL: time.Duration(request.TTLSeconds) * time.Second, CreatedBy: kernelActorID(c),
		Actor: c.GetString("email"), IP: c.ClientIP(),
	})
	switch {
	case errors.Is(err, agentpki.ErrInvalidNode):
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist or is disabled")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusCreated, createdAgentEnrollmentToken{Enrollment: row, Credential: credential})
}
