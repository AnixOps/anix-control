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
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/requestorigin"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ReleaseVersion is the running Control's version (set at startup from the
// build).
var ReleaseVersion = branding.DefaultVersion

// AgentRelease is the Agent release tag this Control was released with
// (config/AGENT_RELEASE, set at startup from the release build's -X
// main.agentRelease). The Agent it installs is that release, unless
// agent_install.agent_version says otherwise: a Control patch release can
// ship with an unchanged Agent, so the Agent repository need not have a tag
// with the Control's number. Empty in a development build, where the
// Control's version is used (H25).
var AgentRelease string

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
// and returns the install command for every mirror (-u <control> -t
// <anixagt_...>: the token names its node). It enrolls an existing node, for
// moving a node to a new machine or after a credential rotation; new
// machines join a node group instead. Super administrators only; agentpki
// writes the token's audit entry.
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
		AgentVersion: settings.AgentVersion, Commands: settings.Commands(credential),
		Script: installScriptInfo{
			URL: settings.ControlURL + "/install.sh", SignatureURL: settings.ControlURL + "/install.sh.sig", Signed: signatureErr == nil,
		},
	})
}

// Readiness godoc
// @Summary Check that an install command can work
// @Description Checks, without any network call, what a node needs from this Control before an install command is worth copying: agent_install.public_url (or the address this request came in on) is https; the gRPC target resolves from the settings and the listener is on with TLS; module_runtime.ca_kek is usable (join tokens are sealed with it and node certificates signed by the CA it protects); agent_control.mtls is not off; the Agent release of the pinned version is served by this Control or has a source; the install script's release signature verifies when one is configured. ready is false when a check of level error fails; a failed warning does not block the command. Each check is {id, ok, level, message, fix}. Any administrator.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/agents/install-readiness [get]
func (h *AgentInstallHandler) Readiness(c *gin.Context) {
	cfg := h.config()
	if cfg == nil {
		kernelError(c, http.StatusServiceUnavailable, "config_unavailable", "the configuration is not loaded")
		return
	}
	input := agentinstall.ReadinessInput{
		PublicURL: cfg.AgentInstall.PublicURL, RequestOrigin: requestorigin.Resolve(c.Request).BaseURL(),
		GRPCTarget: cfg.AgentInstall.GRPCTarget, GRPCEnabled: cfg.GRPC.Enable, GRPCPort: cfg.GRPC.Port,
		GRPCTLSCertFile: cfg.GRPC.TLSCertFile, GRPCTLSKeyFile: cfg.GRPC.TLSKeyFile,
		BuiltinPKI: cfg.ModuleRuntime.PKIOrDefault() == config.ModulePKIBuiltin, CAKEK: strings.TrimSpace(cfg.ModuleRuntime.CAKEK) != "",
		MTLSMode:      cfg.AgentControl.MTLSOrDefault(),
		SignatureFile: cfg.AgentInstall.SignatureFile, OfficialPublicKey: cfg.Plugins.OfficialPublicKey,
		AgentVersion: cfg.AgentInstall.AgentVersion, ReleaseVersion: ReleaseVersion,
		ArtifactDir: cfg.AgentInstall.ArtifactDir, CNMirrorURL: cfg.AgentInstall.CNMirrorURL,
	}
	if input.CAKEK {
		if _, err := modulepki.ParseKEK(cfg.ModuleRuntime.CAKEK); err != nil {
			input.CAKEKProblem = err.Error()
		}
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, agentinstall.CheckReadiness(input))
}

// Script serves the install script, byte for byte the signed release asset.
func (h *AgentInstallHandler) Script(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", agentinstall.Script)
}

// ClientIP serves /install/ip: the caller's address as Control sees it,
// as text. The install script asks over IPv4 and IPv6 and hands the answers
// to the join (JoinRequest.public_addresses), because the gRPC peer address
// is a proxy's when Control sits behind one. gin's ClientIP applies
// server.trusted_proxies (internal/requestorigin): a forwarded address is
// taken only from a trusted proxy.
func (h *AgentInstallHandler) ClientIP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.String(http.StatusOK, "%s\n", c.ClientIP())
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
		AgentVersion: install.AgentVersion, PinnedAgentVersion: AgentRelease, ReleaseVersion: ReleaseVersion,
		ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
	})
}
