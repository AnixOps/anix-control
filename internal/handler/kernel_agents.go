package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentPKIHandler administers agent enrollment: one-time credentials that
// enroll a new node's agent (node-ops-service.md, section 5.3). Routes sit
// behind admin authentication and the audit log.
type AgentPKIHandler struct {
	pki func() (*agentpki.Service, error)
	// db is the kernel database (the super administrator check of the
	// credential rotation); nil reads database.Get.
	db func() *gorm.DB
}

// NewAgentPKIHandler builds the agent PKI from the loaded configuration on
// each request.
func NewAgentPKIHandler() *AgentPKIHandler {
	return &AgentPKIHandler{
		pki: func() (*agentpki.Service, error) {
			return agentpki.FromConfig(config.Get(), database.Get())
		},
		db: database.Get,
	}
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

// Rotation lifetimes: the new enrollment credential lives one hour unless
// the request says otherwise, like an install token (H18), and at most
// seven days.
const (
	defaultRotationTTL = time.Hour
	minRotationTTL     = time.Minute
)

type rotateAgentCredentialsRequest struct {
	// Node is "proxy-<id>" or "forward-<id>".
	Node string `json:"node" binding:"required"`
	// TTLSeconds is the new enrollment credential's lifetime: one hour by
	// default, from 60 to 604800 (7 days).
	TTLSeconds int64 `json:"ttl_seconds"`
	// Reason is recorded in the audit entry; at most 200 bytes (UTF-8).
	Reason string `json:"reason"`
}

type rotatedAgentCredentials struct {
	Node string `json:"node"`
	// Revoked counts the Agent certificates, enrollments and forward link
	// certificates the rotation revoked.
	Revoked    agentpki.ActiveCredentials `json:"revoked"`
	Enrollment model.AgentEnrollment      `json:"enrollment"`
	ExpiresAt  time.Time                  `json:"expires_at"`
	// Credential is the new one-time enrollment credential, returned once;
	// Control stores only its SHA-256.
	Credential string `json:"credential"`
}

// RotateCredentials rotates a node's Agent credentials: it revokes the
// node's Agent certificates and enrollments and issues a fresh one-time enrollment credential so the
// Agent enrolls again. Super administrators only; the audit entries are
// written by agentpki (this route is outside the AuditLog middleware).
//
// RotateCredentials godoc
// @Summary Rotate a node's Agent credentials
// @Description Revokes every Agent certificate, enrollment (unused enrollment credentials included) and forward link certificate of the node and returns a fresh one-time anixagt_ enrollment credential, shown once, for the Agent to enroll again (install script with --reset). Never dials the node, so it works while the node is offline; open Agent streams end at their next heartbeat. Repeating it leaves exactly one valid enrollment credential, the last. Super administrators only. Audited as agent_credentials_rotate and agent_enrollment_token_issue.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body rotateAgentCredentialsRequest true "node (proxy-<id> or forward-<id>), ttl_seconds (60 to 604800, default 3600), reason (at most 200 bytes, UTF-8)"
// @Success 201 {object} map[string]any "data.credential is shown once"
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v4/kernel/agents/rotate-credentials [post]
func (h *AgentPKIHandler) RotateCredentials(c *gin.Context) {
	db := h.database()
	ok, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may rotate a node's Agent credentials")
		return
	}
	var request rotateAgentCredentialsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	node, err := agentcontrol.ParseAgentNode(request.Node)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return
	}
	ttl := defaultRotationTTL
	if request.TTLSeconds != 0 {
		ttl = time.Duration(request.TTLSeconds) * time.Second
	}
	if request.TTLSeconds < 0 || ttl < minRotationTTL || ttl > agentpki.MaxEnrollmentCredentialTTL {
		kernelError(c, http.StatusBadRequest, "invalid_request", "ttl_seconds must be between 60 and 604800 (7 days)")
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if len(reason) > agentpki.MaxRotationReasonLength {
		kernelError(c, http.StatusBadRequest, "invalid_request", "reason may not exceed 200 characters")
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
	rotate := agentpki.RotateRequest{
		Node: node, TTL: ttl, CreatedBy: kernelActorID(c), Actor: c.GetString("email"), IP: c.ClientIP(), Reason: reason,
	}
	rotation, err := pki.RotateCredentials(c.Request.Context(), rotate)
	switch {
	case errors.Is(err, agentpki.ErrInvalidNode):
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
		return
	case errors.Is(err, agentpki.ErrNodeDisabled):
		kernelError(c, http.StatusConflict, "node_disabled", "the node is disabled: its Agent credentials were revoked when it was disabled; enable it before issuing a new credential")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	var expires time.Time
	if rotation.Enrollment.ExpiresAt != nil {
		expires = *rotation.Enrollment.ExpiresAt
	}
	slog.Info("agent credentials rotated", "component", "agent-pki", "node", node.String(), "actor_id", rotate.CreatedBy,
		"revoked_certificates", rotation.Revoked.Certificates)
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusCreated, rotatedAgentCredentials{
		Node: node.String(), Revoked: rotation.Revoked,
		Enrollment: rotation.Enrollment, ExpiresAt: expires, Credential: rotation.Credential,
	})
}

func (h *AgentPKIHandler) database() *gorm.DB {
	if h.db != nil {
		return h.db()
	}
	return database.Get()
}
