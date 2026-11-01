package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	controlgrpc "github.com/AnixOps/anix-control/v5/internal/grpc"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/AnixOps/anix-control/v5/internal/service"
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
	// sessions are this process's live Agent Control stream sessions, for
	// the kernel each plan node reported; nil reads none.
	sessions func() []agentstreams.Session
}

// NewAgentUpgradesHandler reads the loaded configuration and database on
// each request.
func NewAgentUpgradesHandler() *AgentUpgradesHandler {
	return &AgentUpgradesHandler{db: database.Get, config: config.Get, sessions: controlgrpc.GetAgentStreams().Sessions}
}

type startAgentUpgradeRequest struct {
	// TargetVersion defaults to this Control's Agent release (H25).
	TargetVersion string               `json:"target_version"`
	Batches       []agentupgrade.Batch `json:"batches"`
	Exclude       agentupgrade.Exclude `json:"exclude"`
	// Nodes, when set, upgrade exactly these nodes at once (manual
	// upgrade); batches and exclude must then be empty.
	Nodes []string `json:"nodes"`
	// KernelOnly upgrades only the anx-node kernel of Nodes: the target is
	// the Agent release they run now (they must run one), so each Agent
	// keeps its binary and installs the newest kernel Control holds.
	// TargetVersion must then be empty.
	KernelOnly bool   `json:"kernel_only"`
	Reason     string `json:"reason"`
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

// settings resolves the Agent install settings of the running Control, as
// every campaign does: the https address nodes reach, and the Agent
// release it pins (agent_install.agent_version, else the release this
// Control was built with, else its own version, H25).
// status is the HTTP status of the error: 503 without a loaded
// configuration, 409 when the settings do not resolve.
func (h *AgentUpgradesHandler) settings(c *gin.Context) (cfg *config.Config, settings agentinstall.Settings, status int, err error) {
	cfg = h.config()
	if cfg == nil {
		return nil, agentinstall.Settings{}, http.StatusServiceUnavailable, errors.New("the configuration is not loaded")
	}
	install := cfg.AgentInstall
	panel := cfg.Panel()
	controlURL := panel.PublicURL
	if controlURL == "" {
		controlURL = requestorigin.Resolve(c.Request).BaseURL()
	}
	settings, err = agentinstall.ResolveSettings(agentinstall.SettingsInput{
		ControlURL: controlURL, GRPCTarget: panel.GRPCTarget, GRPCPort: panel.GRPCPort,
		AgentVersion: install.AgentVersion, PinnedAgentVersion: AgentRelease, ReleaseVersion: ReleaseVersion, ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
	})
	if err != nil {
		return cfg, agentinstall.Settings{}, http.StatusConflict, err
	}
	return cfg, settings, 0, nil
}

// Start godoc
// @Summary Start a staged Agent upgrade
// @Description Pushes an Agent release to every enabled node whose Agent was seen on the Agent Control stream, in batches (default 5%, 25%, 100% of the nodes, at least 30 minutes each; canaries first by node name hash), as the agent.upgrade operation to Agents that negotiated upgrade.v1. With nodes (proxy-<id>, forward-<id>, each eligible) it is a manual upgrade instead: exactly those nodes in one batch with no minimum duration; batches and exclude must be empty. With kernel_only (and nodes, no target_version) only the anx-node kernel is upgraded: the target is the Agent release those nodes run now, so each Agent keeps its binary and installs the newest anx-node release in agent_install.artifact_dir. A batch rolls back when more than 5% of its offered nodes fail or do not reconnect with the target version within 10 minutes (H19). The release must be in agent_install.artifact_dir with its .sig files and SHA256SUMS(.sig), verified with plugins.official_public_key. Super administrators only; one campaign at a time.
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body startAgentUpgradeRequest true "target_version (default this Control's Agent release), batches, exclude {nodes, tags}, nodes (manual upgrade), kernel_only, reason"
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
	cfg, settings, status, err := h.settings(c)
	if err != nil {
		kernelError(c, status, "agent_install_unconfigured", err.Error())
		return
	}
	install := cfg.AgentInstall
	target := strings.TrimSpace(request.TargetVersion)
	if request.KernelOnly {
		if target != "" {
			kernelError(c, http.StatusBadRequest, "invalid_request", "a kernel-only upgrade keeps the nodes' Agent release: leave target_version empty")
			return
		}
		target, err = agentupgrade.KernelOnlyTarget(c.Request.Context(), h.db(), request.Nodes)
		if err != nil {
			h.campaignError(c, err)
			return
		}
	}
	if target == "" {
		target = settings.AgentVersion
	}
	artifacts, err := agentinstall.UpgradeArtifacts(install.ArtifactDir, target, cfg.Plugins.OfficialPublicKey, settings.ControlURL)
	if err != nil {
		if request.KernelOnly {
			err = fmt.Errorf("a kernel-only upgrade still sends the nodes' own Agent release %s, which must be in agent_install.artifact_dir: %w", target, err)
		}
		kernelError(c, http.StatusConflict, "agent_release_unverified", err.Error())
		return
	}
	// The worker offers the newest anx-node release with the upgrade; a
	// release that is there but incomplete is refused here.
	kernels, err := agentinstall.UpgradeKernels(install.ArtifactDir, settings.ControlURL)
	if err != nil {
		kernelError(c, http.StatusConflict, "kernel_release_unverified", err.Error())
		return
	}
	reason := request.Reason
	if request.KernelOnly {
		if len(kernels) == 0 {
			kernelError(c, http.StatusConflict, "kernel_release_missing", "agent_install.artifact_dir holds no anx-node release to install")
			return
		}
		if strings.TrimSpace(reason) == "" {
			reason = "kernel upgrade: anx-node " + kernels[0].Version
		}
	}
	campaign, err := h.service().Start(c.Request.Context(), agentupgrade.StartRequest{
		TargetVersion: target, ControlVersion: ReleaseVersion, Artifacts: artifacts, Batches: request.Batches,
		Exclude: request.Exclude, Nodes: request.Nodes, Reason: reason, Actor: h.actor(c),
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

// Plan godoc
// @Summary What an Agent upgrade would do now
// @Description The Agent release a campaign pushes by default (agent_install.agent_version, else the Agent release this Control was built with, else its own release), this Control's release, how many nodes a campaign would include now (enabled nodes whose Agent was seen on the Agent Control stream) and which (nodes: node_ref, name, agent_version, kernel_version reported by a live session, for a manual upgrade), the default batches with the rules of H19 (first batch at most 5%, each batch at least 30 minutes, a batch rolls back above 5% failures or after 10 minutes without a reconnect), the newest anx-node release agent_install.artifact_dir holds (kernel_version, which every campaign installs; empty for none), the active campaign if there is one, and whether the caller may start one (can_start: super administrators). When the https address nodes reach Control is not configured, configured is false with unconfigured_reason and target_version empty. Administrators read it; Start verifies the release files.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/upgrade-plan [get]
func (h *AgentUpgradesHandler) Plan(c *gin.Context) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	plan := gin.H{
		"target_version": "", "control_version": "v" + strings.TrimPrefix(strings.TrimSpace(ReleaseVersion), "v"),
		"configured": true, "unconfigured_reason": "",
		"batches": agentupgrade.DefaultBatches(), "min_batch_seconds": int(agentupgrade.MinBatchDuration / time.Second),
		"first_batch_max_percent": agentupgrade.FirstBatchMaxPercent, "failure_threshold_percent": agentupgrade.FailureThresholdPercent,
		"reconnect_timeout_seconds": int(agentupgrade.ReconnectTimeout / time.Second),
	}
	if _, settings, _, err := h.settings(c); err != nil {
		plan["configured"], plan["unconfigured_reason"] = false, err.Error()
	} else {
		plan["target_version"] = settings.AgentVersion
	}
	// kernel_version is the anx-node release a campaign installs (the
	// newest one agent_install.artifact_dir holds), "" for none; a
	// kernel-only upgrade needs it.
	plan["kernel_version"] = ""
	if cfg := h.config(); cfg != nil {
		plan["kernel_version"] = agentinstall.LatestKernelVersion(cfg.AgentInstall.ArtifactDir)
	}
	nodes, err := agentupgrade.Candidates(c.Request.Context(), db, agentupgrade.Exclude{})
	if err != nil {
		kernelDBError(c, err)
		return
	}
	plan["eligible_nodes"] = len(nodes)
	described, err := agentupgrade.Describe(c.Request.Context(), db, nodes)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if h.sessions != nil {
		kernels := map[string]string{}
		for _, session := range h.sessions() {
			kernels[session.Node.String()] = session.KernelVersion
		}
		for index := range described {
			described[index].KernelVersion = kernels[described[index].NodeRef]
		}
	}
	plan["nodes"] = described
	var active []string
	if err := db.WithContext(c.Request.Context()).Model(&model.AgentUpgradeCampaign{}).Where("active_slot IS NOT NULL").Limit(1).Pluck("id", &active).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	plan["active_campaign_id"] = ""
	if len(active) > 0 {
		plan["active_campaign_id"] = active[0]
	}
	super, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	plan["can_start"] = super
	kernelData(c, http.StatusOK, plan)
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
