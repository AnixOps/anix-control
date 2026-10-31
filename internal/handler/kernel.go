package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	compatv2 "github.com/AnixOps/anix-control/v5/internal/compat/v2"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxControlPluginRequestBody = 1 << 20
const defaultControlPluginHostRequestTimeout = 30 * time.Second
const defaultControlPluginHostWebSocketSessionTimeout = 24 * time.Hour

type KernelHandler struct {
	db                 *gorm.DB
	controlPluginHosts pluginhost.Manager
}

// accessGroupDetailResponse keeps the access-control management view on a
// single authoritative Kernel read. Memberships intentionally expose only the
// small admin-facing identity projection; credentials and user profile data do
// not belong to the access-group contract.
type accessGroupDetailResponse struct {
	Group          model.AccessGroup        `json:"group"`
	Users          []accessGroupUserSummary `json:"users"`
	Plans          []accessGroupPlanSummary `json:"plans"`
	ResourceGrants []model.ResourceGrant    `json:"resource_grants"`
	QuotaPolicies  []model.QuotaPolicy      `json:"quota_policies"`
}

type accessGroupUserSummary struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
}

type accessGroupPlanSummary struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func NewKernelHandler() *KernelHandler {
	return &KernelHandler{db: database.Get(), controlPluginHosts: pluginhost.DefaultManager()}
}

// NewV2RouteCache returns the verified-route cache that one router's unary
// and WebSocket v2 gateways share, so a signed release artifact is verified
// once rather than per request and per gateway.
func NewV2RouteCache() *compatv2.VerifiedRouteCache {
	return compatv2.NewVerifiedRouteCache(0)
}

// NewV2CompatibilityGateway builds the unary v2 compatibility gateway. A
// missing or invalid trust root remains a resolver error, so the gateway fails
// closed rather than using legacy code.
func NewV2CompatibilityGateway(cfg *config.Config, cache *compatv2.VerifiedRouteCache) *compatv2.Gateway {
	publicKey := ""
	requestBodyLimit := config.DefaultControlHostPayloadBytes
	if cfg != nil {
		publicKey = cfg.Plugins.OfficialPublicKey
		requestBodyLimit = cfg.Plugins.ControlHostRequestBodyLimit()
	}
	return &compatv2.Gateway{
		Registry:         compatv2.NewRegistry(compatv2.NewVerifiedRouteSourceWithCache(database.Get(), publicKey, cache)),
		Dispatcher:       pluginhost.DefaultManager(),
		Timeout:          controlPluginHostRequestTimeout(cfg),
		RequestBodyLimit: requestBodyLimit,
	}
}

// NewV2WebSocketGateway exposes the optional streaming capability only when
// the current manager explicitly implements it.
func NewV2WebSocketGateway(cfg *config.Config, cache *compatv2.VerifiedRouteCache) *compatv2.WebSocketGateway {
	publicKey := ""
	if cfg != nil {
		publicKey = cfg.Plugins.OfficialPublicKey
	}
	var dispatcher pluginhost.WebSocketManager
	if manager, ok := pluginhost.DefaultManager().(pluginhost.WebSocketManager); ok {
		dispatcher = manager
	}
	return &compatv2.WebSocketGateway{
		Registry:       compatv2.NewRegistry(compatv2.NewVerifiedRouteSourceWithCache(database.Get(), publicKey, cache)),
		Dispatcher:     dispatcher,
		Timeout:        controlPluginHostRequestTimeout(cfg),
		SessionTimeout: controlPluginHostWebSocketSessionTimeout(cfg),
	}
}

func controlPluginHostRequestTimeout(cfg *config.Config) time.Duration {
	if cfg == nil {
		return defaultControlPluginHostRequestTimeout
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(cfg.Plugins.ControlHostRequestTimeout))
	if err != nil || parsed <= 0 {
		return defaultControlPluginHostRequestTimeout
	}
	return parsed
}

func controlPluginHostWebSocketSessionTimeout(cfg *config.Config) time.Duration {
	if cfg == nil {
		return defaultControlPluginHostWebSocketSessionTimeout
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(cfg.Plugins.ControlHostWebSocketSessionTimeout))
	if err != nil || parsed <= 0 {
		return defaultControlPluginHostWebSocketSessionTimeout
	}
	return parsed
}

func kernelData(c *gin.Context, status int, data any) { c.JSON(status, gin.H{"data": data}) }
func kernelError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func kernelDBError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		kernelError(c, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	kernelError(c, http.StatusInternalServerError, "database_error", "database operation failed")
}

func parseKernelID(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_id", name+" must be a positive integer")
		return 0, false
	}
	return uint(id), true
}

func (h *KernelHandler) accessGroupExists(groupID uint) (bool, error) {
	var count int64
	err := h.db.Model(&model.AccessGroup{}).Where("id = ?", groupID).Count(&count).Error
	return count > 0, err
}

func kernelActorID(c *gin.Context) uint {
	value, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return id
	case uint32:
		return uint(id)
	case int:
		if id > 0 {
			return uint(id)
		}
	case float64:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

func kernelActorIsAdmin(c *gin.Context) bool {
	value, ok := c.Get("is_admin")
	return ok && value == true
}

func kernelRequestID(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if requestID, ok := value.(string); ok && strings.TrimSpace(requestID) != "" {
			return requestID
		}
	}
	if requestID := strings.TrimSpace(c.GetHeader("X-Request-ID")); requestID != "" {
		return requestID
	}
	return uuid.NewString()
}

func controlOperationIdempotencyKey(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "control-plugin:" + hex.EncodeToString(digest[:])
}

func enqueueControlPluginOperation(tx *gorm.DB, installation model.PluginInstallation, kind, targetVersion string, requestIdentity ...string) (*model.KernelOperation, error) {
	identityParts := []string{
		strconv.FormatUint(uint64(installation.ID), 10), kind, targetVersion,
		strconv.FormatInt(installation.LifecycleGeneration, 10),
		strconv.FormatInt(installation.ConfigRevision, 10), strconv.FormatBool(installation.Enabled),
	}
	identityParts = append(identityParts, requestIdentity...)
	return enqueueControlPluginOperationWithKey(tx, installation, kind, targetVersion, controlOperationIdempotencyKey(identityParts...))
}

func enqueueControlPluginOperationWithKey(tx *gorm.DB, installation model.PluginInstallation, kind, targetVersion, idempotencyKey string) (*model.KernelOperation, error) {
	configuration, err := service.GetPluginConfiguration(tx, installation.ID)
	if err != nil {
		return nil, err
	}
	var existing model.KernelOperation
	if err := tx.First(&existing, "idempotency_key = ?", idempotencyKey).Error; err == nil {
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Minute)
	operation, _, err := service.CreateKernelOperation(tx, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: idempotencyKey,
		PluginID: installation.PluginID, TargetVersion: targetVersion, Kind: kind,
		ConfigJSON: configuration.ConfigJSON, DeadlineAt: &deadline,
	})
	return operation, err
}

func (h *KernelHandler) ListPlugins(c *gin.Context) {
	var rows []model.Plugin
	if err := h.db.Order("id").Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func (h *KernelHandler) ListExtensions(c *gin.Context) {
	var publicKey ed25519.PublicKey
	cfg := config.Get()
	if cfg != nil && strings.TrimSpace(cfg.Plugins.OfficialPublicKey) != "" {
		parsed, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
		if err != nil {
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_invalid", err.Error())
			return
		}
		publicKey = parsed
	}
	extensions, err := service.ListEnabledWebUIExtensionsForActor(
		h.db,
		publicKey,
		kernelActorID(c),
		kernelActorIsAdmin(c),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPluginTrustRootRequired):
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
		case errors.Is(err, service.ErrExtensionCatalogIntegrity):
			kernelError(c, http.StatusConflict, "extension_catalog_invalid", err.Error())
		default:
			kernelDBError(c, err)
		}
		return
	}
	kernelData(c, http.StatusOK, extensions)
}

func (h *KernelHandler) PluginRouteGateway(c *gin.Context) {
	if c.Param("plugin_id") == forwardPackageID && !h.forwardKernelChecks(c) {
		return
	}
	h.dispatchPluginControlRoute(c, c.Param("plugin_id"), c.Request.URL.Path)
}

// dispatchPluginControlRoute resolves requestPath among pluginID's manifest
// control routes, checks the caller's permission and dispatches the
// request to the package host with requestPath as its path.
func (h *KernelHandler) dispatchPluginControlRoute(c *gin.Context, pluginID, requestPath string) {
	cfg := config.Get()
	if cfg == nil || strings.TrimSpace(cfg.Plugins.OfficialPublicKey) == "" {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", "official plugin public key is not configured")
		return
	}
	publicKey, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_invalid", err.Error())
		return
	}
	resolution, err := service.ResolvePluginControlRoute(
		h.db,
		publicKey,
		pluginID,
		requestPath,
		kernelActorID(c),
		kernelActorIsAdmin(c),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPluginRouteNotFound):
			kernelError(c, http.StatusNotFound, "plugin_route_not_found", err.Error())
		case errors.Is(err, service.ErrPluginRouteForbidden):
			kernelError(c, http.StatusForbidden, "plugin_route_forbidden", err.Error())
		case errors.Is(err, service.ErrPluginTrustRootRequired):
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
		case errors.Is(err, service.ErrExtensionCatalogIntegrity):
			kernelError(c, http.StatusConflict, "plugin_route_integrity_failed", err.Error())
		case errors.Is(err, service.ErrPluginArtifactRequired):
			kernelError(c, http.StatusConflict, "plugin_artifact_missing", err.Error())
		default:
			kernelDBError(c, err)
		}
		return
	}
	if h.controlPluginHosts == nil {
		kernelError(c, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		return
	}
	bridgeRouteID := service.PluginControlBridgeRouteID(resolution.PluginID, resolution.MatchedRoute)
	if bridgeRouteID == "" {
		kernelError(c, http.StatusConflict, "plugin_route_integrity_failed", "plugin control route is invalid")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxControlPluginRequestBody+1))
	if err != nil {
		kernelError(c, http.StatusBadRequest, "plugin_request_invalid", "plugin request body could not be read")
		return
	}
	if len(body) > maxControlPluginRequestBody {
		kernelError(c, http.StatusRequestEntityTooLarge, "plugin_request_too_large", "plugin request body exceeds 1 MiB")
		return
	}
	// super_admin is set only where a gateway resolved it (the forward
	// package's list answers, F5b D7); other packages never see it.
	var superAdmin *bool
	if value, ok := c.Get(pluginPrincipalSuperAdminKey); ok {
		if allowed, isBool := value.(bool); isBool {
			superAdmin = &allowed
		}
	}
	principal, err := json.Marshal(struct {
		ActorID    uint   `json:"actor_id"`
		Admin      bool   `json:"admin"`
		Plugin     string `json:"plugin_id"`
		SuperAdmin *bool  `json:"super_admin,omitempty"`
	}{
		ActorID: kernelActorID(c), Admin: kernelActorIsAdmin(c), Plugin: resolution.PluginID, SuperAdmin: superAdmin,
	})
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "plugin_host_unavailable", "plugin request principal could not be encoded")
		return
	}
	deadline := time.Now().Add(controlPluginHostRequestTimeout(cfg))
	if requestDeadline, ok := c.Request.Context().Deadline(); ok && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	dispatchContext, cancelDispatch := context.WithDeadline(c.Request.Context(), deadline)
	defer cancelDispatch()
	response, err := h.controlPluginHosts.Dispatch(dispatchContext, pluginhost.DispatchInput{
		PackageID: resolution.PluginID, Version: resolution.Version, Generation: resolution.Generation,
		RequestID: kernelRequestID(c), IdempotencyKey: c.GetHeader("Idempotency-Key"), RouteID: bridgeRouteID,
		Method: c.Request.Method, Body: body, PrincipalJSON: principal,
		Metadata: pluginhost.RequestMetadata{Path: requestPath, Query: cloneKernelQuery(c.Request.URL.Query())}, Deadline: deadline,
	})
	if err != nil {
		switch {
		case errors.Is(err, pluginhost.ErrResponseTooLarge):
			kernelError(c, http.StatusBadGateway, "plugin_response_too_large", "plugin response exceeds its limit")
		case errors.Is(err, pluginhost.ErrHostIncompatible):
			kernelError(c, http.StatusBadGateway, "plugin_host_incompatible", "plugin host is incompatible")
		default:
			kernelError(c, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		}
		return
	}
	if response.StatusCode < http.StatusContinue || response.StatusCode > 599 {
		kernelError(c, http.StatusBadGateway, "plugin_host_incompatible", "plugin host returned an invalid response")
		return
	}
	for _, header := range response.Headers {
		if header.Name != "" {
			c.Header(header.Name, header.Value)
		}
	}
	c.Data(int(response.StatusCode), c.GetHeader("Content-Type"), response.Body)
}

func cloneKernelQuery(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return map[string][]string{}
	}
	result := make(map[string][]string, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func (h *KernelHandler) ListPluginReleases(c *gin.Context) {
	var rows []model.PluginRelease
	query := h.db.Order("plugin_id, published_at DESC")
	if pluginID := strings.TrimSpace(c.Query("plugin_id")); pluginID != "" {
		query = query.Where("plugin_id = ?", pluginID)
	}
	if err := query.Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func (h *KernelHandler) GetPluginReleaseArtifact(c *gin.Context) {
	releaseID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	artifact, err := service.GetPluginArtifact(h.db, releaseID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, artifact)
}

func (h *KernelHandler) ServePluginWebUIAsset(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("plugin_id"))
	version := strings.TrimSpace(c.Param("version"))
	bundleSHA256 := strings.ToLower(strings.TrimSpace(c.Param("sha256")))
	filename := strings.TrimSpace(c.Param("filename"))
	cfg := config.Get()
	if cfg == nil || strings.TrimSpace(cfg.Plugins.OfficialPublicKey) == "" {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", "official plugin public key is not configured")
		return
	}
	publicKey, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_invalid", err.Error())
		return
	}
	asset, err := service.ResolveActivePluginWebUIAsset(
		h.db,
		publicKey,
		kernelActorID(c),
		kernelActorIsAdmin(c),
		pluginID,
		version,
		bundleSHA256,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPluginRouteForbidden):
			kernelError(c, http.StatusForbidden, "extension_asset_forbidden", err.Error())
		case errors.Is(err, service.ErrPluginTrustRootRequired):
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
		case errors.Is(err, service.ErrExtensionCatalogIntegrity):
			kernelError(c, http.StatusConflict, "extension_asset_integrity_failed", err.Error())
		case errors.Is(err, service.ErrPluginArtifactRequired):
			kernelError(c, http.StatusConflict, "plugin_artifact_missing", err.Error())
		default:
			kernelDBError(c, err)
		}
		return
	}
	if filename == "" || filename != path.Base(asset.BundlePath) {
		kernelError(c, http.StatusNotFound, "not_found", "webui asset not found")
		return
	}
	// Authorization and installation state are rechecked for every fetch. Do
	// not let a browser reuse an immutable response after access is revoked or
	// the installation is disabled.
	c.Header("Cache-Control", "private, no-store")
	c.Header("ETag", `"`+asset.BundleSHA256+`"`)
	c.Data(http.StatusOK, asset.ContentType, asset.Data)
}

func (h *KernelHandler) ListPluginInstallations(c *gin.Context) {
	var rows []model.PluginInstallation
	if err := h.db.Order("plugin_id, target").Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	response := make([]service.PluginInstallationStatus, 0, len(rows))
	for _, row := range rows {
		response = append(response, service.PublicPluginInstallation(row))
	}
	kernelData(c, http.StatusOK, response)
}

func (h *KernelHandler) GetPluginInstallationConfiguration(c *gin.Context) {
	installationID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	configuration, err := service.GetPluginConfiguration(h.db, installationID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, configuration)
}

func (h *KernelHandler) UpdatePluginInstallationConfiguration(c *gin.Context) {
	installationID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Config           json.RawMessage `json:"config" binding:"required"`
		ExpectedRevision *int64          `json:"expected_revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(req.Config) > 1<<20 {
		kernelError(c, http.StatusRequestEntityTooLarge, "config_too_large", "plugin config exceeds 1 MiB")
		return
	}
	if req.ExpectedRevision != nil && *req.ExpectedRevision < 0 {
		kernelError(c, http.StatusBadRequest, "invalid_revision", "expected_revision must not be negative")
		return
	}
	cfg := config.Get()
	if cfg == nil || strings.TrimSpace(cfg.Plugins.OfficialPublicKey) == "" {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", "official plugin public key is not configured")
		return
	}
	publicKey, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
	if err != nil {
		kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_invalid", err.Error())
		return
	}
	var queuedAgentOperations []*model.KernelOperation
	// Agent plugin dispatch is off until M4: the assignments record the new
	// revision and no Agent operation is queued.
	dispatchEnabled := agentPluginDispatchEnabled()
	configuration, err := service.UpdatePluginConfigurationWithValidatorAndHook(
		h.db, publicKey, installationID, string(req.Config), req.ExpectedRevision, kernelActorID(c), nil,
		func(tx *gorm.DB, installation model.PluginInstallation, configuration model.PluginConfiguration) error {
			if installation.Target != "agent" {
				return nil
			}
			operations, err := service.SyncAgentInstallationAssignments(tx, installation, configuration.Revision, dispatchEnabled, time.Now())
			queuedAgentOperations = operations
			return err
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPluginConfigurationConflict):
			kernelError(c, http.StatusConflict, "configuration_revision_conflict", err.Error())
		case errors.Is(err, service.ErrPluginTrustRootRequired):
			kernelError(c, http.StatusServiceUnavailable, "plugin_trust_root_unconfigured", err.Error())
		case errors.Is(err, gorm.ErrRecordNotFound):
			kernelError(c, http.StatusNotFound, "not_found", "plugin installation not found")
		default:
			kernelError(c, http.StatusUnprocessableEntity, "invalid_plugin_configuration", err.Error())
		}
		return
	}
	var updatedInstallation model.PluginInstallation
	if err := h.db.First(&updatedInstallation, installationID).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if updatedInstallation.Target == "agent" {
		if err := service.RefreshAgentPluginInstallationObservedState(h.db, updatedInstallation.PluginID); err != nil {
			kernelDBError(c, err)
			return
		}
	}
	var queuedOperation *model.KernelOperation
	if updatedInstallation.Target == "control" && updatedInstallation.Enabled {
		queuedOperation, err = enqueueControlPluginOperation(h.db, updatedInstallation, "plugin.configure", updatedInstallation.DesiredVersion)
		if err != nil {
			kernelError(c, http.StatusConflict, "plugin_operation_rejected", err.Error())
			return
		}
	}
	if queuedOperation != nil {
		if err := setPluginOperationResponseHeaders(c, h.db, *queuedOperation); err != nil {
			kernelDBError(c, err)
			return
		}
	} else if len(queuedAgentOperations) > 0 {
		ids := make([]string, 0, len(queuedAgentOperations))
		for _, operation := range queuedAgentOperations {
			ids = append(ids, operation.ID)
		}
		c.Header("X-AnixOps-Operation-ID", ids[0])
		c.Header("X-AnixOps-Operation-Chain", strings.Join(ids, ","))
	}
	kernelData(c, http.StatusOK, configuration)
}

// PluginInstallationAction restarts a built-in package's host: the only
// action left (single-binary design, section 4.3). Installing, upgrading,
// rolling back, enabling and disabling are what every start does to the
// built-in packages, so no administrator action does them.
func (h *KernelHandler) PluginInstallationAction(c *gin.Context) {
	installationID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Action         string `json:"action" binding:"required"`
		IdempotencyKey string `json:"idempotency_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.Action != "restart" {
		kernelError(c, http.StatusBadRequest, "unsupported_plugin_action",
			"the only package action is restart: Control installs, upgrades and enables its built-in packages at every start")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > 160 || strings.ContainsAny(req.IdempotencyKey, "\x00\r\n") {
		kernelError(c, http.StatusBadRequest, "invalid_idempotency_key", "idempotency_key is invalid")
		return
	}
	var installation model.PluginInstallation
	var operation *model.KernelOperation
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&installation, installationID).Error; err != nil {
			return err
		}
		if installation.Target != "control" {
			return errors.New("installation is not a Control target")
		}
		if err := service.LockPluginInstallationTarget(tx, installation.Target); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&installation, installationID).Error; err != nil {
			return err
		}
		// A restart is an enable of the running generation.
		const kind = "plugin.enable"
		stableKey := controlOperationIdempotencyKey("action", strconv.FormatUint(uint64(installation.ID), 10), req.IdempotencyKey)
		var existing model.KernelOperation
		if err := tx.First(&existing, "idempotency_key = ?", stableKey).Error; err == nil {
			if existing.Kind != kind {
				return errors.New("idempotency_key is already bound to a different plugin action")
			}
			operation = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !installation.Enabled {
			return errors.New("disabled installation cannot be restarted")
		}
		var release model.PluginRelease
		if err := tx.First(&release, "plugin_id = ? AND version = ?", installation.PluginID, installation.DesiredVersion).Error; err != nil {
			return errors.New("verified target release not found")
		}
		if err := service.ValidatePluginInstallationPlan(tx, release, "control", true, installation.ID); err != nil {
			return err
		}
		queued, err := enqueueControlPluginOperationWithKey(tx, installation, kind, installation.DesiredVersion, stableKey)
		operation = queued
		return err
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			kernelError(c, http.StatusNotFound, "not_found", "plugin installation not found")
		case errors.Is(err, service.ErrPluginDependencyUnsatisfied):
			kernelError(c, http.StatusConflict, "plugin_dependency_unsatisfied", err.Error())
		case errors.Is(err, service.ErrPluginConflict):
			kernelError(c, http.StatusConflict, "plugin_conflict", err.Error())
		case errors.Is(err, service.ErrPluginArtifactRequired):
			kernelError(c, http.StatusConflict, "plugin_artifact_missing", err.Error())
		default:
			kernelError(c, http.StatusConflict, "plugin_action_rejected", err.Error())
		}
		return
	}
	if err := setPluginOperationResponseHeaders(c, h.db, *operation); err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusAccepted, gin.H{"installation": service.PublicPluginInstallation(installation), "operation": service.PublicKernelOperation(*operation)})
}

// setPluginOperationResponseHeaders exposes both the user-visible operation
// and the complete apply chain for dependency-aware Control lifecycle plans.
// A plain operation is represented as a one-item chain so clients can consume
// the header without branching on the action implementation.
func setPluginOperationResponseHeaders(c *gin.Context, db *gorm.DB, operation model.KernelOperation) error {
	if strings.TrimSpace(operation.ID) == "" {
		return errors.New("plugin operation id is missing")
	}
	c.Header("X-AnixOps-Operation-ID", operation.ID)
	ids := []string{operation.ID}
	if planID := strings.TrimSpace(operation.LifecyclePlanID); planID != "" {
		var chain []model.KernelOperation
		if err := db.Select("id").Where("lifecycle_plan_id = ? AND lifecycle_plan_phase = ?", planID, "apply").
			Order("lifecycle_plan_sequence ASC").Find(&chain).Error; err != nil {
			return fmt.Errorf("load plugin lifecycle operation chain: %w", err)
		}
		if len(chain) > 0 {
			ids = ids[:0]
			for _, item := range chain {
				if strings.TrimSpace(item.ID) != "" {
					ids = append(ids, item.ID)
				}
			}
		}
	}
	c.Header("X-AnixOps-Operation-Chain", strings.Join(ids, ","))
	return nil
}

func (h *KernelHandler) ListServiceScopes(c *gin.Context) {
	var rows []model.ServiceScope
	if err := h.db.Order("id").Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func (h *KernelHandler) ListAccessGroups(c *gin.Context) {
	var rows []model.AccessGroup
	query := h.db.Order("scope_id, name")
	if scope := c.Query("scope_id"); scope != "" {
		query = query.Where("scope_id = ?", scope)
	}
	if err := query.Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// GetAccessGroupDetail returns the associated identities and policy records
// needed to safely administer one access group. The underlying models stay
// normalized so the allow-union resolver remains the single authorization
// implementation rather than a client-side reconstruction.
func (h *KernelHandler) GetAccessGroupDetail(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}

	var group model.AccessGroup
	if err := h.db.First(&group, id).Error; err != nil {
		kernelDBError(c, err)
		return
	}

	response := accessGroupDetailResponse{
		Group:          group,
		Users:          []accessGroupUserSummary{},
		Plans:          []accessGroupPlanSummary{},
		ResourceGrants: []model.ResourceGrant{},
		QuotaPolicies:  []model.QuotaPolicy{},
	}

	var userMemberships []model.AccessGroupUser
	if err := h.db.Where("group_id = ?", id).Order("user_id").Find(&userMemberships).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if len(userMemberships) > 0 {
		userIDs := make([]uint, 0, len(userMemberships))
		for _, membership := range userMemberships {
			userIDs = append(userIDs, membership.UserID)
		}
		var users []model.User
		if err := h.db.Select("id", "email").Where("id IN ?", userIDs).Order("id").Find(&users).Error; err != nil {
			kernelDBError(c, err)
			return
		}
		for _, user := range users {
			response.Users = append(response.Users, accessGroupUserSummary{ID: user.ID, Email: user.Email})
		}
	}

	var planMemberships []model.AccessGroupPlan
	if err := h.db.Where("group_id = ?", id).Order("plan_id").Find(&planMemberships).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if len(planMemberships) > 0 {
		planIDs := make([]uint, 0, len(planMemberships))
		for _, membership := range planMemberships {
			planIDs = append(planIDs, membership.PlanID)
		}
		var plans []model.Plan
		if err := h.db.Select("id", "name").Where("id IN ?", planIDs).Order("id").Find(&plans).Error; err != nil {
			kernelDBError(c, err)
			return
		}
		for _, plan := range plans {
			response.Plans = append(response.Plans, accessGroupPlanSummary{ID: plan.ID, Name: plan.Name})
		}
	}

	if err := h.db.Where("group_id = ?", id).Order("resource_type, resource_id, id").Find(&response.ResourceGrants).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if err := h.db.Where("group_id = ?", id).Order("key, id").Find(&response.QuotaPolicies).Error; err != nil {
		kernelDBError(c, err)
		return
	}

	kernelData(c, http.StatusOK, response)
}

func (h *KernelHandler) CreateAccessGroup(c *gin.Context) {
	var req struct {
		ScopeID     string `json:"scope_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ScopeID) == "" || strings.TrimSpace(req.Name) == "" {
		kernelError(c, http.StatusBadRequest, "invalid_request", "scope_id and name are required")
		return
	}
	row := model.AccessGroup{
		ScopeID: strings.TrimSpace(req.ScopeID), Name: strings.TrimSpace(req.Name),
		Description: req.Description, Enabled: true,
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	var count int64
	if err := h.db.Model(&model.ServiceScope{}).Where("id = ?", row.ScopeID).Count(&count).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if count == 0 {
		kernelError(c, http.StatusBadRequest, "scope_not_found", "service scope does not exist")
		return
	}
	if err := h.db.Create(&row).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, row)
}

func (h *KernelHandler) UpdateAccessGroup(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name, Description string
		Enabled           *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	updates["description"] = req.Description
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	result := h.db.Model(&model.AccessGroup{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		kernelDBError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		kernelError(c, http.StatusNotFound, "not_found", "access group not found")
		return
	}
	var row model.AccessGroup
	_ = h.db.First(&row, id).Error
	kernelData(c, http.StatusOK, row)
}

func (h *KernelHandler) DeleteAccessGroup(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&model.AccessGroupUser{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.AccessGroupPlan{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.ResourceGrant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.QuotaPolicy{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&model.AccessGroup{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *KernelHandler) AddGroupUser(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		UserID uint `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, 400, "invalid_request", err.Error())
		return
	}
	exists, err := h.accessGroupExists(groupID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !exists {
		kernelError(c, http.StatusNotFound, "not_found", "access group not found")
		return
	}
	var userCount int64
	if err := h.db.Model(&model.User{}).Where("id = ?", req.UserID).Count(&userCount).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if userCount == 0 {
		kernelError(c, http.StatusBadRequest, "user_not_found", "user does not exist")
		return
	}
	row := model.AccessGroupUser{GroupID: groupID, UserID: req.UserID}
	if err := h.db.FirstOrCreate(&row, model.AccessGroupUser{GroupID: groupID, UserID: req.UserID}).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, row)
}

func (h *KernelHandler) RemoveGroupUser(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	userID, ok := parseKernelID(c, "user_id")
	if !ok {
		return
	}
	if err := h.db.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&model.AccessGroupUser{}).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	c.Status(204)
}

func (h *KernelHandler) AddGroupPlan(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		PlanID uint `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, 400, "invalid_request", err.Error())
		return
	}
	exists, err := h.accessGroupExists(groupID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !exists {
		kernelError(c, http.StatusNotFound, "not_found", "access group not found")
		return
	}
	var planCount int64
	if err := h.db.Model(&model.Plan{}).Where("id = ?", req.PlanID).Count(&planCount).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	if planCount == 0 {
		kernelError(c, http.StatusBadRequest, "plan_not_found", "plan does not exist")
		return
	}
	row := model.AccessGroupPlan{GroupID: groupID, PlanID: req.PlanID}
	if err := h.db.FirstOrCreate(&row, model.AccessGroupPlan{GroupID: groupID, PlanID: req.PlanID}).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, row)
}

func (h *KernelHandler) RemoveGroupPlan(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	planID, ok := parseKernelID(c, "plan_id")
	if !ok {
		return
	}
	if err := h.db.Where("group_id = ? AND plan_id = ?", groupID, planID).Delete(&model.AccessGroupPlan{}).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	c.Status(204)
}

func (h *KernelHandler) ListResourceGrants(c *gin.Context) {
	var rows []model.ResourceGrant
	query := h.db.Order("group_id, resource_type, resource_id")
	if id := c.Query("group_id"); id != "" {
		query = query.Where("group_id = ?", id)
	}
	if err := query.Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, rows)
}

func (h *KernelHandler) CreateResourceGrant(c *gin.Context) {
	var row model.ResourceGrant
	if err := c.ShouldBindJSON(&row); err != nil || row.GroupID == 0 || row.ResourceType == "" || row.ResourceID == "" {
		kernelError(c, 400, "invalid_request", "group_id, resource_type and resource_id are required")
		return
	}
	row.ID = 0
	if !jsonObjectOrArray(row.Permissions) {
		kernelError(c, 400, "invalid_permissions", "permissions must be a JSON object or array")
		return
	}
	if row.ResourceType == service.PluginAPIGrantResourceType {
		if err := service.ValidatePluginAPIGrantPermissions(row.ResourceID, row.Permissions); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_permissions", err.Error())
			return
		}
	}
	exists, err := h.accessGroupExists(row.GroupID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !exists {
		kernelError(c, http.StatusBadRequest, "group_not_found", "access group does not exist")
		return
	}
	if err := h.db.Create(&row).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 201, row)
}

func jsonObjectOrArray(value string) bool {
	value = strings.TrimSpace(value)
	isObject := strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}")
	isArray := strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]")
	if !isObject && !isArray {
		return false
	}
	return json.Valid([]byte(value))
}

func (h *KernelHandler) DeleteResourceGrant(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	result := h.db.Delete(&model.ResourceGrant{}, id)
	if result.Error != nil {
		kernelDBError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		kernelError(c, 404, "not_found", "grant not found")
		return
	}
	c.Status(204)
}

func (h *KernelHandler) ListQuotaPolicies(c *gin.Context) {
	var rows []model.QuotaPolicy
	query := h.db.Order("group_id, key")
	if groupID := c.Query("group_id"); groupID != "" {
		query = query.Where("group_id = ?", groupID)
	}
	if err := query.Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func (h *KernelHandler) UpsertQuotaPolicy(c *gin.Context) {
	var req model.QuotaPolicy
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupID == 0 || strings.TrimSpace(req.Key) == "" || !jsonObjectOrArray(req.PolicyJSON) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "group_id, key and a JSON policy are required")
		return
	}
	exists, err := h.accessGroupExists(req.GroupID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	if !exists {
		kernelError(c, http.StatusBadRequest, "group_not_found", "access group does not exist")
		return
	}
	var row model.QuotaPolicy
	err = h.db.Where("group_id = ? AND key = ?", req.GroupID, req.Key).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		kernelDBError(c, err)
		return
	}
	row.GroupID, row.Key, row.PolicyJSON = req.GroupID, req.Key, req.PolicyJSON
	if err := h.db.Save(&row).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}

func (h *KernelHandler) DeleteQuotaPolicy(c *gin.Context) {
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	result := h.db.Delete(&model.QuotaPolicy{}, id)
	if result.Error != nil {
		kernelDBError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		kernelError(c, http.StatusNotFound, "not_found", "quota policy not found")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *KernelHandler) ResolveAccess(c *gin.Context) {
	userID64, err := strconv.ParseUint(c.Query("user_id"), 10, 32)
	if err != nil || userID64 == 0 {
		kernelError(c, 400, "invalid_user", "user_id is required")
		return
	}
	var planID *uint
	if raw := c.Query("plan_id"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || parsed == 0 {
			kernelError(c, 400, "invalid_plan", "plan_id must be an integer")
			return
		}
		value := uint(parsed)
		planID = &value
	}
	scopeID := strings.TrimSpace(c.Query("scope_id"))
	if scopeID == "" {
		kernelError(c, http.StatusBadRequest, "invalid_scope", "scope_id is required")
		return
	}
	result, err := service.ResolveEffectiveAccess(h.db, uint(userID64), planID, scopeID)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, result)
}

func (h *KernelHandler) ListAssignments(c *gin.Context) {
	nodeID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var rows []model.NodeServiceAssignment
	query := h.db.Where("node_id = ?", nodeID).Order("service_scope, plugin_id, role")
	if err := query.Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, 200, rows)
}

func (h *KernelHandler) UpsertAssignment(c *gin.Context) {
	nodeID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var req struct {
		ServiceScope          string `json:"service_scope"`
		PluginID              string `json:"plugin_id"`
		Role                  string `json:"role"`
		DesiredVersion        string `json:"desired_version"`
		DesiredConfigRevision *int64 `json:"desired_config_revision"`
		Enabled               *bool  `json:"enabled"`
		RolloutGroup          string `json:"rollout_group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ServiceScope) == "" || strings.TrimSpace(req.PluginID) == "" || strings.TrimSpace(req.Role) == "" {
		kernelError(c, 400, "invalid_request", "service_scope, plugin_id and role are required")
		return
	}
	req.ServiceScope = strings.TrimSpace(req.ServiceScope)
	req.PluginID = strings.TrimSpace(req.PluginID)
	req.Role = strings.TrimSpace(req.Role)
	req.DesiredVersion = strings.TrimSpace(req.DesiredVersion)
	if req.DesiredConfigRevision != nil && *req.DesiredConfigRevision < 0 {
		kernelError(c, http.StatusBadRequest, "invalid_revision", "desired_config_revision cannot be negative")
		return
	}
	var row model.NodeServiceAssignment
	var queued []*model.KernelOperation
	// Agent plugin dispatch is off until M4: the lifecycle records the
	// desired state and no Agent operation is queued.
	dispatchEnabled := agentPluginDispatchEnabled()
	err := service.WithAgentLifecycleTransaction(h.db, func(tx *gorm.DB) error {
		row = model.NodeServiceAssignment{}
		queued = nil
		var nodeCount, scopeCount, pluginCount int64
		if err := tx.Model(&model.Node{}).Where("id = ?", nodeID).Count(&nodeCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ServiceScope{}).Where("id = ?", req.ServiceScope).Count(&scopeCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Plugin{}).Where("id = ? AND official = ?", req.PluginID, true).Count(&pluginCount).Error; err != nil {
			return err
		}
		if nodeCount == 0 || scopeCount == 0 || pluginCount == 0 {
			return errors.New("invalid_reference")
		}
		if req.DesiredVersion != "" {
			var release model.PluginRelease
			if err := tx.First(&release, "plugin_id = ? AND version = ?", req.PluginID, req.DesiredVersion).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("release_not_found")
				}
				return err
			}
			if err := service.ValidatePluginReleaseTarget(release, "agent"); err != nil {
				return fmt.Errorf("release_invalid: %w", err)
			}
		}
		if err := service.LockPluginInstallationTarget(tx, "agent"); err != nil {
			return err
		}
		if _, err := service.LockNodePluginLifecycleTx(tx, nodeID, req.PluginID); err != nil {
			return err
		}

		find := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"node_id = ? AND service_scope = ? AND plugin_id = ? AND role = ?", nodeID, req.ServiceScope, req.PluginID, req.Role,
		).First(&row)
		if find.Error != nil && !errors.Is(find.Error, gorm.ErrRecordNotFound) {
			return find.Error
		}
		isNew := row.ID == 0
		oldVersion, oldConfigRevision, oldEnabled, oldDeletePending := row.DesiredVersion, row.DesiredConfigRevision, row.Enabled, row.DeletePending
		row.NodeID, row.ServiceScope, row.PluginID, row.Role = nodeID, req.ServiceScope, req.PluginID, req.Role
		row.DesiredVersion, row.RolloutGroup = req.DesiredVersion, strings.TrimSpace(req.RolloutGroup)
		row.DeletePending = false
		if req.DesiredConfigRevision != nil {
			row.DesiredConfigRevision = *req.DesiredConfigRevision
		} else if isNew && req.DesiredVersion != "" {
			var installation model.PluginInstallation
			if err := tx.First(&installation, "plugin_id = ? AND target = ?", req.PluginID, "agent").Error; err == nil && installation.DesiredVersion == req.DesiredVersion {
				configuration, configErr := service.GetPluginConfiguration(tx, installation.ID)
				if configErr != nil {
					return configErr
				}
				row.DesiredConfigRevision = configuration.Revision
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if req.Enabled != nil {
			row.Enabled = *req.Enabled
		} else if isNew {
			row.Enabled = true
		}
		if row.DesiredVersion == "" {
			row.Enabled = false
		}
		if isNew || oldVersion != row.DesiredVersion || oldConfigRevision != row.DesiredConfigRevision || oldEnabled != row.Enabled || oldDeletePending {
			row.LifecycleGeneration++
			if row.LifecycleGeneration <= 0 {
				row.LifecycleGeneration = 1
			}
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		lifecycle, _, err := service.SyncNodePluginLifecycle(tx, row.NodeID, row.PluginID, true, time.Now())
		if err != nil {
			return err
		}
		if !dispatchEnabled {
			return nil
		}
		chain, err := service.QueueNodePluginLifecycle(tx, lifecycle, time.Now())
		if err != nil {
			return err
		}
		for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable, chain.Disable} {
			if operation != nil {
				queued = append(queued, operation)
			}
		}
		return nil
	})
	if err != nil {
		switch {
		case err.Error() == "invalid_reference":
			kernelError(c, http.StatusBadRequest, "invalid_reference", "node, scope or official plugin does not exist")
		case err.Error() == "release_not_found":
			kernelError(c, http.StatusBadRequest, "release_not_found", "verified agent plugin release not found")
		case strings.HasPrefix(err.Error(), "release_invalid:"):
			kernelError(c, http.StatusConflict, "release_invalid", strings.TrimSpace(strings.TrimPrefix(err.Error(), "release_invalid:")))
		case errors.Is(err, service.ErrPluginArtifactRequired), errors.Is(err, gorm.ErrRecordNotFound):
			kernelError(c, http.StatusConflict, "plugin_artifact_missing", err.Error())
		case errors.Is(err, service.ErrAgentPluginReconcileNotReady):
			kernelError(c, http.StatusConflict, "agent_plugin_reconcile_not_ready", err.Error())
		case errors.Is(err, service.ErrAgentPluginAssignmentConflict):
			kernelError(c, http.StatusConflict, "agent_plugin_assignment_conflict", err.Error())
		case errors.Is(err, service.ErrAgentPluginReleaseIntegrity):
			kernelError(c, http.StatusConflict, "plugin_release_integrity_failed", err.Error())
		default:
			kernelDBError(c, err)
		}
		return
	}
	if err := service.RefreshAgentPluginInstallationObservedState(h.db, req.PluginID); err != nil {
		kernelDBError(c, err)
		return
	}
	if len(queued) > 0 {
		ids := make([]string, 0, len(queued))
		for _, operation := range queued {
			ids = append(ids, operation.ID)
		}
		c.Header("X-AnixOps-Operation-ID", ids[0])
		c.Header("X-AnixOps-Operation-Chain", strings.Join(ids, ","))
	}
	kernelData(c, 200, row)
}

func (h *KernelHandler) DeleteAssignment(c *gin.Context) {
	nodeID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	id, ok := parseKernelID(c, "assignment_id")
	if !ok {
		return
	}
	var assignment model.NodeServiceAssignment
	var queued []*model.KernelOperation
	// Agent plugin dispatch is off until M4: the lifecycle records the
	// desired state and no Agent operation is queued.
	dispatchEnabled := agentPluginDispatchEnabled()
	err := service.WithAgentLifecycleTransaction(h.db, func(tx *gorm.DB) error {
		assignment = model.NodeServiceAssignment{}
		queued = nil
		var identity struct {
			NodeID   uint
			PluginID string
		}
		if err := tx.Model(&model.NodeServiceAssignment{}).Select("node_id, plugin_id").
			First(&identity, "id = ? AND node_id = ?", id, nodeID).Error; err != nil {
			return err
		}
		if err := service.LockPluginInstallationTarget(tx, "agent"); err != nil {
			return err
		}
		if _, err := service.LockNodePluginLifecycleTx(tx, identity.NodeID, identity.PluginID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&assignment, "id = ? AND node_id = ?", id, nodeID).Error; err != nil {
			return err
		}
		assignment.Enabled = false
		assignment.DeletePending = true
		if err := tx.Save(&assignment).Error; err != nil {
			return err
		}
		lifecycle, _, err := service.SyncNodePluginLifecycle(tx, assignment.NodeID, assignment.PluginID, false, time.Now())
		if err != nil {
			return err
		}
		if !dispatchEnabled {
			return service.PurgeCompletedAssignmentDeletes(tx, lifecycle, nil)
		}
		chain, err := service.QueueNodePluginLifecycle(tx, lifecycle, time.Now())
		if err != nil {
			return err
		}
		for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable, chain.Disable} {
			if operation != nil {
				queued = append(queued, operation)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			kernelError(c, http.StatusNotFound, "not_found", "assignment not found")
			return
		}
		kernelError(c, http.StatusConflict, "assignment_delete_rejected", err.Error())
		return
	}
	if err := service.RefreshAgentPluginInstallationObservedState(h.db, assignment.PluginID); err != nil {
		kernelDBError(c, err)
		return
	}
	if len(queued) > 0 {
		ids := make([]string, 0, len(queued))
		for _, operation := range queued {
			ids = append(ids, operation.ID)
		}
		c.Header("X-AnixOps-Operation-ID", ids[0])
		c.Header("X-AnixOps-Operation-Chain", strings.Join(ids, ","))
	}
	kernelData(c, http.StatusAccepted, assignment)
}

func (h *KernelHandler) ListOperations(c *gin.Context) {
	if _, err := service.ExpireKernelOperations(h.db, time.Now()); err != nil {
		kernelDBError(c, err)
		return
	}
	var rows []model.KernelOperation
	if err := h.db.Order("created_at DESC").Limit(500).Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	response := make([]service.KernelOperationStatus, 0, len(rows))
	planIDs := make([]string, 0)
	seenPlans := make(map[string]struct{})
	for _, row := range rows {
		if planID := strings.TrimSpace(row.LifecyclePlanID); planID != "" {
			if _, seen := seenPlans[planID]; !seen {
				seenPlans[planID] = struct{}{}
				planIDs = append(planIDs, planID)
			}
		}
	}
	chains := make(map[string]string, len(planIDs))
	if len(planIDs) > 0 {
		var chainRows []model.KernelOperation
		if err := h.db.Select("id", "lifecycle_plan_id", "lifecycle_plan_phase", "lifecycle_plan_sequence").
			Where("lifecycle_plan_id IN ? AND lifecycle_plan_phase = ?", planIDs, "apply").
			Order("lifecycle_plan_id, lifecycle_plan_sequence ASC").Find(&chainRows).Error; err != nil {
			kernelDBError(c, err)
			return
		}
		chainIDs := make(map[string][]string, len(planIDs))
		for _, row := range chainRows {
			if strings.TrimSpace(row.ID) != "" {
				chainIDs[row.LifecyclePlanID] = append(chainIDs[row.LifecyclePlanID], row.ID)
			}
		}
		for planID, ids := range chainIDs {
			chains[planID] = strings.Join(ids, ",")
		}
	}
	for _, row := range rows {
		view := service.PublicKernelOperation(row)
		if chain := chains[strings.TrimSpace(row.LifecyclePlanID)]; chain != "" {
			view.OperationChain = chain
		}
		response = append(response, view)
	}
	kernelData(c, 200, response)
}

func (h *KernelHandler) CreateOperation(c *gin.Context) {
	var req struct {
		OperationID    string          `json:"operation_id" binding:"required"`
		IdempotencyKey string          `json:"idempotency_key" binding:"required"`
		NodeID         *uint           `json:"node_id"`
		PluginID       string          `json:"plugin_id" binding:"required"`
		TargetVersion  string          `json:"target_version" binding:"required"`
		Kind           string          `json:"kind" binding:"required"`
		Revision       int64           `json:"revision"`
		Config         json.RawMessage `json:"config" binding:"required"`
		ConfigHash     string          `json:"config_hash"`
		DeadlineAt     *time.Time      `json:"deadline_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	operation, reused, err := service.CreateKernelOperation(h.db, model.KernelOperation{
		ID: req.OperationID, IdempotencyKey: req.IdempotencyKey,
		NodeID: req.NodeID, PluginID: req.PluginID, TargetVersion: req.TargetVersion,
		Kind: req.Kind, Revision: req.Revision, ConfigJSON: string(req.Config), ConfigHash: req.ConfigHash, DeadlineAt: req.DeadlineAt,
	})
	if err != nil {
		kernelError(c, http.StatusUnprocessableEntity, "invalid_operation", err.Error())
		return
	}
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
	}
	kernelData(c, status, gin.H{"operation": service.PublicKernelOperation(*operation), "reused": reused})
}

func (h *KernelHandler) CancelOperation(c *gin.Context) {
	operation, err := service.CancelKernelOperation(h.db, c.Param("id"), time.Now())
	if err != nil {
		if errors.Is(err, service.ErrInvalidKernelOperationID) {
			kernelError(c, http.StatusBadRequest, "invalid_id", err.Error())
			return
		}
		if errors.Is(err, service.ErrKernelOperationNotPending) {
			kernelError(c, http.StatusConflict, "operation_not_cancellable", err.Error())
			return
		}
		kernelDBError(c, err)
		return
	}
	if err := setPluginOperationResponseHeaders(c, h.db, *operation); err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusAccepted, service.PublicKernelOperation(*operation))
}

// agentPluginDispatchEnabled reads service.AgentPluginDispatchEnabled (off
// until M4, no setting); the tests replace it to keep the dispatch paths
// covered.
var agentPluginDispatchEnabled = func() bool { return service.AgentPluginDispatchEnabled }
