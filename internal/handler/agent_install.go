package handler

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/branding"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/requestorigin"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ReleaseVersion is the running Control's version (set at startup from the
// build); the Agent it installs carries the same number (H25).
var ReleaseVersion = branding.DefaultVersion

// Install token lifetimes (H18): one hour by default, at most seven days.
const (
	defaultInstallTokenTTL = time.Hour
	minInstallTokenTTL     = time.Minute
)

// AgentInstallHandler is one-command node onboarding (forward-sdk.md,
// section 9): install tokens for the node page and the public install
// script, its signature, the release metadata and the Agent release.
type AgentInstallHandler struct {
	pki    func() (*agentpki.Service, error)
	config func() *config.Config
	db     func() *gorm.DB
}

// NewAgentInstallHandler reads the loaded configuration and database on
// each request.
func NewAgentInstallHandler() *AgentInstallHandler {
	return &AgentInstallHandler{
		pki: func() (*agentpki.Service, error) {
			return agentpki.FromConfig(config.Get(), database.Get())
		},
		config: config.Get,
		db:     database.Get,
	}
}

type createInstallTokenRequest struct {
	// Node is "proxy-<id>" or "forward-<id>".
	Node string `json:"node" binding:"required"`
	// TTLSeconds defaults to one hour; from 60 to 604800 (7 days).
	TTLSeconds int64 `json:"ttl_seconds"`
}

type installTokenResponse struct {
	Enrollment model.AgentEnrollment `json:"enrollment"`
	// Credential is the single-use token, returned once; Control stores
	// only its SHA-256.
	Credential   string                 `json:"credential"`
	Node         string                 `json:"node"`
	ExpiresAt    time.Time              `json:"expires_at"`
	AgentVersion string                 `json:"agent_version"`
	Commands     []agentinstall.Command `json:"commands"`
	Script       installScriptInfo      `json:"script"`
}

type installScriptInfo struct {
	URL          string `json:"url"`
	SignatureURL string `json:"signature_url"`
	// Signed is true when Control serves a valid release signature.
	Signed bool `json:"signed"`
}

// CreateInstallToken issues a single-use enrollment token bound to a node
// and returns the install command for every mirror. Super administrators
// only; agentpki writes the token's audit entry.
func (h *AgentInstallHandler) CreateInstallToken(c *gin.Context) {
	db := h.db()
	ok, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may issue install tokens")
		return
	}
	var request createInstallTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	node, err := agentcontrol.ParseAgentNode(request.Node)
	if err != nil || !agentinstall.ValidNode(node.String()) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return
	}
	ttl := defaultInstallTokenTTL
	if request.TTLSeconds != 0 {
		ttl = time.Duration(request.TTLSeconds) * time.Second
	}
	if request.TTLSeconds < 0 || ttl < minInstallTokenTTL || ttl > agentpki.MaxEnrollmentCredentialTTL {
		kernelError(c, http.StatusBadRequest, "invalid_request", "ttl_seconds must be between 60 and 604800 (7 days)")
		return
	}
	cfg := h.config()
	settings, err := h.settings(c, cfg)
	if err != nil {
		kernelError(c, http.StatusConflict, "agent_install_unconfigured", err.Error())
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
		Node: node, TTL: ttl, CreatedBy: kernelActorID(c), Actor: c.GetString("email"), IP: c.ClientIP(),
	})
	switch {
	case errors.Is(err, agentpki.ErrInvalidNode):
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist or is disabled")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	var expires time.Time
	if row.ExpiresAt != nil {
		expires = *row.ExpiresAt
	}
	_, signatureErr := agentinstall.LoadSignature(cfg.AgentInstall.SignatureFile, cfg.Plugins.OfficialPublicKey)
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusCreated, installTokenResponse{
		Enrollment: row, Credential: credential, Node: node.String(), ExpiresAt: expires,
		AgentVersion: settings.AgentVersion, Commands: settings.Commands(node.String(), credential),
		Script: installScriptInfo{
			URL: settings.ControlURL + "/install.sh", SignatureURL: settings.ControlURL + "/install.sh.sig", Signed: signatureErr == nil,
		},
	})
}

// Script serves the install script, byte for byte the signed release asset.
func (h *AgentInstallHandler) Script(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", agentinstall.Script)
}

// Signature serves the script's release signature once it verifies with
// the official key: a stale or foreign file is never served.
func (h *AgentInstallHandler) Signature(c *gin.Context) {
	cfg := h.config()
	signature, err := agentinstall.LoadSignature(cfg.AgentInstall.SignatureFile, cfg.Plugins.OfficialPublicKey)
	if err != nil {
		kernelError(c, http.StatusNotFound, "signature_unavailable",
			"this Control has no release signature for its install script; verify the agent-install.sh release asset and its .sig from GitHub releases instead")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", signature)
}

// Metadata serves /install/agent.env: the Agent release, the gRPC target,
// the mirrors and, when Control holds the release, its digests.
func (h *AgentInstallHandler) Metadata(c *gin.Context) {
	settings, err := h.settings(c, h.config())
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "agent_install_unconfigured", err.Error())
		return
	}
	env, err := settings.Metadata().Env()
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "agent_install_unconfigured", err.Error())
		return
	}
	// The answer depends on the request's origin when public_url is unset.
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(env))
}

// Artifact serves an Agent release asset from agent_install.artifact_dir
// (the control mirror).
func (h *AgentInstallHandler) Artifact(c *gin.Context) {
	cfg := h.config()
	path, err := agentinstall.ArtifactPath(cfg.AgentInstall.ArtifactDir, c.Param("tag"), c.Param("asset"))
	if err != nil {
		kernelError(c, http.StatusNotFound, "not_found", "no such Agent release asset on this Control")
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		kernelError(c, http.StatusNotFound, "not_found", "no such Agent release asset on this Control")
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.File(path)
}

// settings resolves the onboarding settings of this Control.
func (h *AgentInstallHandler) settings(c *gin.Context, cfg *config.Config) (agentinstall.Settings, error) {
	if cfg == nil {
		return agentinstall.Settings{}, errors.New("the configuration is not loaded")
	}
	install := cfg.AgentInstall
	controlURL := strings.TrimRight(strings.TrimSpace(install.PublicURL), "/")
	if controlURL == "" {
		controlURL = requestorigin.Resolve(c.Request).BaseURL()
	}
	return agentinstall.ResolveSettings(agentinstall.SettingsInput{
		ControlURL: controlURL, GRPCTarget: install.GRPCTarget, GRPCPort: cfg.GRPC.Port,
		AgentVersion: install.AgentVersion, ReleaseVersion: ReleaseVersion,
		ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
	})
}
