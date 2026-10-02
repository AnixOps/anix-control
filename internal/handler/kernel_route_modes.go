package handler

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RouteModeHandler lists and switches the route modes of v2 Control
// packages (legacy, shadow, native). Any administrator may read them; only
// a super administrator (service.IsSuperAdmin) may switch them.
type RouteModeHandler struct {
	db        func() *gorm.DB
	publicKey func() (ed25519.PublicKey, error)
	hosts     func() []pluginhost.HostStats
	// catalog overrides the embedded package extraction map in tests.
	catalog map[string]service.RouteCatalogEntry
}

// NewRouteModeHandler reads the database, the official package trust root
// and the package hosts' reports on each request.
func NewRouteModeHandler() *RouteModeHandler {
	return &RouteModeHandler{
		db: database.Get,
		publicKey: func() (ed25519.PublicKey, error) {
			cfg := config.Get()
			if cfg == nil || strings.TrimSpace(cfg.Plugins.OfficialPublicKey) == "" {
				return nil, service.ErrPluginTrustRootRequired
			}
			return service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
		},
		hosts: func() []pluginhost.HostStats {
			if provider, ok := pluginhost.DefaultManager().(pluginhost.HostStatsProvider); ok {
				return provider.Stats()
			}
			return nil
		},
	}
}

// RouteModeChangeBody switches routes of a package. No routes means the
// whole package. Switching to native needs confirm and a reason.
type RouteModeChangeBody struct {
	PackageID string   `json:"package_id" binding:"required"`
	Routes    []string `json:"routes"`
	Mode      string   `json:"mode" binding:"required"`
	Reason    string   `json:"reason"`
	Confirm   bool     `json:"confirm"`
}

// RouteModeRollbackBody returns a whole package to legacy.
type RouteModeRollbackBody struct {
	PackageID string `json:"package_id" binding:"required"`
	Reason    string `json:"reason"`
}

func (h *RouteModeHandler) admin(c *gin.Context, needKey bool) (*service.RouteModeAdmin, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	admin := &service.RouteModeAdmin{DB: db.WithContext(c.Request.Context()), Catalog: h.catalog}
	publicKey, err := h.publicKey()
	if err != nil {
		if needKey {
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
			return nil, false
		}
		publicKey = nil
	}
	admin.PublicKey = publicKey
	return admin, true
}

// hostObservations parses the route reports in the package hosts' health
// details by package and route.
func (h *RouteModeHandler) hostObservations() map[string]map[string]service.RouteHostObservation {
	if h.hosts == nil {
		return nil
	}
	observations := map[string]map[string]service.RouteHostObservation{}
	for _, stats := range h.hosts() {
		if stats.HealthDetailsJSON == "" {
			continue
		}
		var details packageHostHealthDetails
		if err := json.Unmarshal([]byte(stats.HealthDetailsJSON), &details); err != nil {
			continue
		}
		for route, detail := range details.Routes {
			if observations[stats.PackageID] == nil {
				observations[stats.PackageID] = map[string]service.RouteHostObservation{}
			}
			observations[stats.PackageID][route] = service.RouteHostObservation{
				Mode: detail.Mode, Effective: detail.Effective, NativeTotal: detail.Native, NativeErrors: detail.NativeErrors,
				ShadowTotal: detail.Shadow, ShadowMismatch: detail.ShadowMismatch, ShadowErrors: detail.ShadowErrors,
				ShadowSkipped: detail.ShadowSkipped,
			}
		}
	}
	return observations
}

func (h *RouteModeHandler) superAdmin(c *gin.Context, db *gorm.DB) (bool, bool) {
	ok, err := service.IsSuperAdmin(db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false, false
	}
	return ok, true
}

func (h *RouteModeHandler) requireSuperAdmin(c *gin.Context, db *gorm.DB) bool {
	ok, done := h.superAdmin(c, db)
	if !done {
		return false
	}
	if !ok {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may switch route modes")
		return false
	}
	return true
}

func routeModeActor(c *gin.Context, db *gorm.DB) service.RouteModeActor {
	userID := kernelActorID(c)
	return service.RouteModeActor{
		UserID: userID, Name: service.RouteModeActorName(db, userID),
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	}
}

func routeModeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrRouteModeConfirmationRequired):
		kernelError(c, http.StatusBadRequest, "route_mode_confirmation_required", "switching to native needs confirm: true and a reason")
	case errors.Is(err, service.ErrPluginConfigurationConflict):
		kernelError(c, http.StatusConflict, "configuration_revision_conflict", err.Error())
	case errors.Is(err, service.ErrRouteModeRejected):
		kernelError(c, http.StatusBadRequest, "route_mode_rejected", err.Error())
	case errors.Is(err, service.ErrRouteModePackageNotInstalled):
		kernelError(c, http.StatusNotFound, "package_not_installed", err.Error())
	case errors.Is(err, service.ErrPluginTrustRootRequired):
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
	default:
		kernelError(c, http.StatusInternalServerError, "route_mode_failed", err.Error())
	}
}

// List godoc
// @Summary List package route modes
// @Description Every v2 Control package's compatibility routes with their configured and effective mode, the modes each may switch to, and the running host's native and shadow counters. can_switch tells whether the caller may switch them.
// @Tags Kernel route modes
// @Produce json
// @Security BearerAuth
// @Param package_id query string false "Package id"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/kernel/route-modes [get]
func (h *RouteModeHandler) List(c *gin.Context) {
	admin, ok := h.admin(c, false)
	if !ok {
		return
	}
	canSwitch, ok := h.superAdmin(c, admin.DB)
	if !ok {
		return
	}
	packages, err := admin.List(c.Request.Context(), strings.TrimSpace(c.Query("package_id")), h.hostObservations())
	if err != nil {
		routeModeError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"can_switch": canSwitch, "packages": packages})
}

// Set godoc
// @Summary Switch package route modes
// @Description Switches the named routes of a package, or every route that may switch, to legacy, shadow or native. Super administrators only. Switching to native needs confirm: true and a reason. Every switch is audited and recorded in the revision history.
// @Tags Kernel route modes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body RouteModeChangeBody true "Route mode change"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v4/kernel/route-modes [post]
func (h *RouteModeHandler) Set(c *gin.Context) {
	admin, ok := h.admin(c, true)
	if !ok || !h.requireSuperAdmin(c, admin.DB) {
		return
	}
	var body RouteModeChangeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := admin.Set(c.Request.Context(), service.RouteModeChangeRequest{
		PackageID: strings.TrimSpace(body.PackageID), Routes: body.Routes, Mode: strings.TrimSpace(body.Mode),
		Reason: body.Reason, Confirm: body.Confirm,
	}, routeModeActor(c, admin.DB))
	if err != nil {
		routeModeError(c, err)
		return
	}
	kernelData(c, http.StatusOK, result)
}

// Rollback godoc
// @Summary Roll a package back to legacy route modes
// @Description Returns every route of a package to legacy in one change, except identity group A, which the identity rollback switches. Super administrators only; no confirmation is needed and the reason is optional.
// @Tags Kernel route modes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body RouteModeRollbackBody true "Rollback"
// @Success 200 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/kernel/route-modes/rollback [post]
func (h *RouteModeHandler) Rollback(c *gin.Context) {
	admin, ok := h.admin(c, true)
	if !ok || !h.requireSuperAdmin(c, admin.DB) {
		return
	}
	var body RouteModeRollbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := admin.Rollback(c.Request.Context(), strings.TrimSpace(body.PackageID), body.Reason, routeModeActor(c, admin.DB))
	if err != nil {
		routeModeError(c, err)
		return
	}
	kernelData(c, http.StatusOK, result)
}

// Revisions godoc
// @Summary List route mode revisions
// @Description The latest route mode switches, newest first, of one package or of all.
// @Tags Kernel route modes
// @Produce json
// @Security BearerAuth
// @Param package_id query string false "Package id"
// @Param limit query int false "At most this many revisions (default 100, at most 1000)"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/route-modes/revisions [get]
func (h *RouteModeHandler) Revisions(c *gin.Context) {
	admin, ok := h.admin(c, false)
	if !ok {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			kernelError(c, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	revisions, err := admin.Revisions(c.Request.Context(), strings.TrimSpace(c.Query("package_id")), limit)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"revisions": revisions})
}
