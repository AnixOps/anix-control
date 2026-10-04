package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/requestorigin"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentUpgradesHandler serves staged Agent upgrade campaigns
// (forward-sdk.md section 9, O4): super administrators start, pause,
// resume and abort them; administrators read them. The worker in the
// singleton-worker process drives them (internal/agentupgrade).
type AgentUpgradesHandler struct {
	db     func() *gorm.DB
	config func() *config.Config
}

// NewAgentUpgradesHandler reads the loaded configuration and database on
// each request.
func NewAgentUpgradesHandler() *AgentUpgradesHandler {
	return &AgentUpgradesHandler{db: database.Get, config: config.Get}
}

type startAgentUpgradeRequest struct {
	// TargetVersion defaults to this Control's Agent release (H25).
	TargetVersion string               `json:"target_version"`
	Batches       []agentupgrade.Batch `json:"batches"`
	Exclude       agentupgrade.Exclude `json:"exclude"`
	Reason        string               `json:"reason"`
}

type abortAgentUpgradeRequest struct {
	// Rollback rolls the current batch back before the campaign ends.
	Rollback bool `json:"rollback"`
}

func (h *AgentUpgradesHandler) service() *agentupgrade.Service {
	return &agentupgrade.Service{DB: h.db()}
}

func (h *AgentUpgradesHandler) requireSuperAdmin(c *gin.Context, action string) bool {
	ok, err := service.IsSuperAdmin(h.db(), kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may "+action+" Agent upgrades")
		return false
	}
	return true
}

func (h *AgentUpgradesHandler) actor(c *gin.Context) agentupgrade.Actor {
	return agentupgrade.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

// Start godoc
// @Summary Start a staged Agent upgrade
// @Description Pushes an Agent release to every enabled node whose Agent was seen on the Agent Control stream, in batches (default 5%, 25%, 100% of the nodes, at least 30 minutes each; canaries first by node name hash), as the agent.upgrade operation to Agents that negotiated upgrade.v1. A batch rolls back when more than 5% of its offered nodes fail or do not reconnect with the target version within 10 minutes (H19). The release must be in agent_install.artifact_dir with its .sig files and SHA256SUMS(.sig), verified with plugins.official_public_key. Super administrators only; one campaign at a time.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body startAgentUpgradeRequest true "target_version (default this Control's Agent release), batches, exclude {nodes, tags}, reason"
// @Success 201 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades [post]
func (h *AgentUpgradesHandler) Start(c *gin.Context) {
	if !h.requireSuperAdmin(c, "start") {
		return
	}
	var request startAgentUpgradeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg := h.config()
	if cfg == nil {
		kernelError(c, http.StatusServiceUnavailable, "agent_install_unconfigured", "the configuration is not loaded")
		return
	}
	install := cfg.AgentInstall
	controlURL := strings.TrimRight(strings.TrimSpace(install.PublicURL), "/")
	if controlURL == "" {
		controlURL = requestorigin.Resolve(c.Request).BaseURL()
	}
	settings, err := agentinstall.ResolveSettings(agentinstall.SettingsInput{
		ControlURL: controlURL, GRPCTarget: install.GRPCTarget, GRPCPort: cfg.GRPC.Port,
		AgentVersion: install.AgentVersion, ReleaseVersion: ReleaseVersion, ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
	})
	if err != nil {
		kernelError(c, http.StatusConflict, "agent_install_unconfigured", err.Error())
		return
	}
	target := strings.TrimSpace(request.TargetVersion)
	if target == "" {
		target = settings.AgentVersion
	}
	artifacts, err := agentinstall.UpgradeArtifacts(install.ArtifactDir, target, cfg.Plugins.OfficialPublicKey, settings.ControlURL)
	if err != nil {
		kernelError(c, http.StatusConflict, "agent_release_unverified", err.Error())
		return
	}
	campaign, err := h.service().Start(c.Request.Context(), agentupgrade.StartRequest{
		TargetVersion: target, ControlVersion: ReleaseVersion, Artifacts: artifacts, Batches: request.Batches,
		Exclude: request.Exclude, Reason: request.Reason, Actor: h.actor(c),
	})
	if err != nil {
		h.campaignError(c, err)
		return
	}
	view, err := h.service().Get(c.Request.Context(), campaign.ID)
	if err != nil {
		h.campaignError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, view)
}

// List godoc
// @Summary List Agent upgrade campaigns
// @Description The latest Agent upgrade campaigns, newest first, each with its batches and nodes counted by state (pending, offered, upgrading, succeeded, failed, rolled_back, skipped).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param limit query int false "at most 100, default 20"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades [get]
func (h *AgentUpgradesHandler) List(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	campaigns, err := h.service().List(c.Request.Context(), limit)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"campaigns": campaigns})
}

// Get godoc
// @Summary Get an Agent upgrade campaign
// @Description One campaign with its artifacts, batches and every node in canary order with its state, versions, timestamps and error code.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "campaign id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades/{id} [get]
func (h *AgentUpgradesHandler) Get(c *gin.Context) {
	view, err := h.service().Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.campaignError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// Pause godoc
// @Summary Pause an Agent upgrade campaign
// @Description No node is offered the upgrade and no batch starts; nodes already offered are still evaluated and a failing batch still rolls back. Super administrators only.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "campaign id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades/{id}/pause [post]
func (h *AgentUpgradesHandler) Pause(c *gin.Context) {
	if !h.requireSuperAdmin(c, "pause") {
		return
	}
	h.answer(c, func(s *agentupgrade.Service) error {
		_, err := s.Pause(c.Request.Context(), c.Param("id"), h.actor(c))
		return err
	})
}

// Resume godoc
// @Summary Resume an Agent upgrade campaign
// @Description Continues a paused campaign; the paused time does not count towards the batch's minimum duration. Super administrators only.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "campaign id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades/{id}/resume [post]
func (h *AgentUpgradesHandler) Resume(c *gin.Context) {
	if !h.requireSuperAdmin(c, "resume") {
		return
	}
	h.answer(c, func(s *agentupgrade.Service) error {
		_, err := s.Resume(c.Request.Context(), c.Param("id"), h.actor(c))
		return err
	})
}

// Abort godoc
// @Summary Abort an Agent upgrade campaign
// @Description Ends the campaign. With rollback the upgraded nodes of the current batch are told to reinstate their previous release first. Super administrators only.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "campaign id"
// @Param request body abortAgentUpgradeRequest false "rollback"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrades/{id}/abort [post]
func (h *AgentUpgradesHandler) Abort(c *gin.Context) {
	if !h.requireSuperAdmin(c, "abort") {
		return
	}
	var request abortAgentUpgradeRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	h.answer(c, func(s *agentupgrade.Service) error {
		_, err := s.Abort(c.Request.Context(), c.Param("id"), h.actor(c), request.Rollback)
		return err
	})
}

func (h *AgentUpgradesHandler) answer(c *gin.Context, change func(*agentupgrade.Service) error) {
	s := h.service()
	if err := change(s); err != nil {
		h.campaignError(c, err)
		return
	}
	view, err := s.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.campaignError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

func (h *AgentUpgradesHandler) campaignError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agentupgrade.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "no such Agent upgrade campaign")
	case errors.Is(err, agentupgrade.ErrActiveCampaign):
		kernelError(c, http.StatusConflict, "agent_upgrade_active", "another Agent upgrade campaign is active: let it end, or abort it")
	case errors.Is(err, agentupgrade.ErrState):
		kernelError(c, http.StatusConflict, "agent_upgrade_state", err.Error())
	case errors.Is(err, agentupgrade.ErrNoNodes):
		kernelError(c, http.StatusConflict, "agent_upgrade_no_nodes", err.Error())
	case errors.Is(err, agentupgrade.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		kernelDBError(c, err)
	}
}
