package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newKernelHandlerTestDB(t *testing.T, extra ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, service.MigrateKernelModelsForTest(db))
	if len(extra) > 0 {
		require.NoError(t, db.AutoMigrate(extra...))
	}
	return db
}

func TestNewKernelHandlerUsesDefaultPluginHostManager(t *testing.T) {
	manager, err := pluginhost.NewManager(pluginhost.ManagerConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	previous := pluginhost.DefaultManager()
	pluginhost.SetDefaultManager(manager)
	t.Cleanup(func() { pluginhost.SetDefaultManager(previous) })

	var handler *KernelHandler
	require.NotPanics(t, func() { handler = NewKernelHandler() })
	require.Same(t, manager, handler.controlPluginHosts)
}

func TestSetPluginOperationResponseHeadersIncludesDependencyApplyChain(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	applyFirst := model.KernelOperation{
		ID: "plan-operation-1", IdempotencyKey: "plan-key-1", Kind: "plugin.install",
		LifecyclePlanID: "plan-1", LifecyclePlanPhase: "apply", LifecyclePlanSequence: 1, State: "pending",
	}
	applyRoot := model.KernelOperation{
		ID: "plan-operation-2", IdempotencyKey: "plan-key-2", Kind: "plugin.enable",
		LifecyclePlanID: "plan-1", LifecyclePlanPhase: "apply", LifecyclePlanSequence: 2, State: "pending",
	}
	rollback := model.KernelOperation{
		ID: "plan-operation-3", IdempotencyKey: "plan-key-3", Kind: "plugin.disable",
		LifecyclePlanID: "plan-1", LifecyclePlanPhase: "rollback", LifecyclePlanSequence: 3, State: "pending",
	}
	require.NoError(t, db.Create(&applyFirst).Error)
	require.NoError(t, db.Create(&applyRoot).Error)
	require.NoError(t, db.Create(&rollback).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	require.NoError(t, setPluginOperationResponseHeaders(context, db, applyRoot))
	require.Equal(t, applyRoot.ID, recorder.Header().Get("X-AnixOps-Operation-ID"))
	require.Equal(t, applyFirst.ID+","+applyRoot.ID, recorder.Header().Get("X-AnixOps-Operation-Chain"))
}

func TestControlPluginHostRequestTimeoutUsesConfiguredValue(t *testing.T) {
	timeout := controlPluginHostRequestTimeout(&config.Config{Plugins: config.PluginConfig{ControlHostRequestTimeout: "2s"}})
	require.Equal(t, 2*time.Second, timeout)
}

func TestControlPluginHostWebSocketSessionTimeoutUsesConfiguredValue(t *testing.T) {
	timeout := controlPluginHostWebSocketSessionTimeout(&config.Config{Plugins: config.PluginConfig{ControlHostWebSocketSessionTimeout: "6h"}})
	require.Equal(t, 6*time.Hour, timeout)
}

func TestControlOperationIdempotencyAdvancesWithLifecycleGeneration(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	require.NoError(t, db.Create(&model.Plugin{
		ID: "generation-test", Name: "Generation Test", Publisher: "AnixOps", Official: true,
	}).Error)
	manifest := service.PluginManifest{
		ID: "generation-test", Name: "Generation Test", Version: "1.0.0", APIVersion: "v1",
		Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256: strings.Repeat("a", sha256.Size*2),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}).Error)
	installation := model.PluginInstallation{
		PluginID: "generation-test", Target: "control", DesiredVersion: "1.0.0",
		State: "pending", Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&installation).Error)

	first, err := enqueueControlPluginOperation(db, installation, "plugin.enable", installation.DesiredVersion)
	require.NoError(t, err)
	replayed, err := enqueueControlPluginOperation(db, installation, "plugin.enable", installation.DesiredVersion)
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)

	installation.LifecycleGeneration++
	require.NoError(t, db.Save(&installation).Error)
	second, err := enqueueControlPluginOperation(db, installation, "plugin.enable", installation.DesiredVersion)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.NotEqual(t, first.IdempotencyKey, second.IdempotencyKey)
}

func performKernelHandlerRequest(t *testing.T, method, path, body string, route string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	return performKernelHandlerRequestWithSetup(t, method, path, body, route, handler, nil)
}

func performKernelHandlerRequestWithSetup(t *testing.T, method, path, body string, route string, handler gin.HandlerFunc, setup func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, route, func(c *gin.Context) {
		if setup != nil {
			setup(c)
		}
		handler(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func kernelHandlerWebUIPackage(t *testing.T, manifest *service.PluginManifest, source string) []byte {
	t.Helper()
	bundleBytes := []byte(source)
	bundleDigest := sha256.Sum256(bundleBytes)
	manifest.WebUI.Bundle.SHA256 = hex.EncodeToString(bundleDigest[:])
	manifest.FrontendSHA256 = manifest.WebUI.Bundle.SHA256
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	bundleWriter, err := writer.Create(manifest.WebUI.Bundle.Path)
	require.NoError(t, err)
	_, err = bundleWriter.Write(bundleBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	artifact := buf.Bytes()
	artifactDigest := sha256.Sum256(artifact)
	manifest.ArtifactSHA256 = hex.EncodeToString(artifactDigest[:])
	return artifact
}

func TestKernelCreateAccessGroupPreservesExplicitDisabledState(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	handler := (&KernelHandler{db: db}).CreateAccessGroup
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/access-groups", `{"scope_id":"forward","name":"disabled","enabled":false}`, "/access-groups", handler)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	var group model.AccessGroup
	require.NoError(t, db.First(&group, "scope_id = ? AND name = ?", "forward", "disabled").Error)
	require.False(t, group.Enabled)
}

func TestKernelGetAccessGroupDetailReturnsOnlyMembershipIdentityAndPolicies(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.User{}, &model.Plan{})
	group := model.AccessGroup{ScopeID: "forward", Name: "canary-operators", Description: "Canary access", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	user := model.User{Email: "operator@example.com", Password: "not-returned", Token: "user-token", UUID: "user-uuid"}
	plan := model.Plan{Name: "Canary plan"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: user.ID}).Error)
	require.NoError(t, db.Create(&model.AccessGroupPlan{GroupID: group.ID, PlanID: plan.ID}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: "machine-telemetry",
		Permissions: `["machine-telemetry.api"]`,
	}).Error)
	require.NoError(t, db.Create(&model.QuotaPolicy{GroupID: group.ID, Key: "machine-telemetry.rate", PolicyJSON: `{"requests_per_minute":60}`}).Error)

	path := "/access-groups/" + strconv.FormatUint(uint64(group.ID), 10)
	recorder := performKernelHandlerRequest(t, http.MethodGet, path, "", "/access-groups/:id", (&KernelHandler{db: db}).GetAccessGroupDetail)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var payload struct {
		Data accessGroupDetailResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, group.ID, payload.Data.Group.ID)
	require.Equal(t, []accessGroupUserSummary{{ID: user.ID, Email: user.Email}}, payload.Data.Users)
	require.Equal(t, []accessGroupPlanSummary{{ID: plan.ID, Name: plan.Name}}, payload.Data.Plans)
	require.Len(t, payload.Data.ResourceGrants, 1)
	require.Equal(t, group.ID, payload.Data.ResourceGrants[0].GroupID)
	require.Len(t, payload.Data.QuotaPolicies, 1)
	require.Equal(t, "machine-telemetry.rate", payload.Data.QuotaPolicies[0].Key)
	require.NotContains(t, recorder.Body.String(), user.Password)
	require.NotContains(t, recorder.Body.String(), user.Token)

	notFound := performKernelHandlerRequest(t, http.MethodGet, "/access-groups/999", "", "/access-groups/:id", (&KernelHandler{db: db}).GetAccessGroupDetail)
	require.Equal(t, http.StatusNotFound, notFound.Code, notFound.Body.String())
}

func TestKernelListExtensionsReturnsEmptyCatalogWithoutTrustRoot(t *testing.T) {
	previousConfig := config.Get()
	config.Set(nil)
	t.Cleanup(func() { config.Set(previousConfig) })
	db := newKernelHandlerTestDB(t)
	handler := (&KernelHandler{db: db}).ListExtensions
	recorder := performKernelHandlerRequest(t, http.MethodGet, "/extensions", "", "/extensions", handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"data":[]}`, recorder.Body.String())
}

func TestKernelCreateResourceGrantValidatesPluginNamespace(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	group := model.AccessGroup{ScopeID: "grant-ui", Name: "operators", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	handler := (&KernelHandler{db: db}).CreateResourceGrant

	valid := fmt.Sprintf(`{"group_id":%d,"resource_type":"%s","resource_id":"grant-ui","permissions":"[\"grant-ui.view\"]"}`, group.ID, service.PluginAPIGrantResourceType)
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/resource-grants", valid, "/resource-grants", handler)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	crossNamespace := fmt.Sprintf(`{"group_id":%d,"resource_type":"%s","resource_id":"grant-ui-2","permissions":"[\"grant-ui.view\",\"other-ui.escape\"]"}`, group.ID, service.PluginAPIGrantResourceType)
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/resource-grants", crossNamespace, "/resource-grants", handler)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "invalid_permissions")

	emptyEffective := fmt.Sprintf(`{"group_id":%d,"resource_type":"%s","resource_id":"grant-ui-3","permissions":"{\"grant-ui-3.view\":false}"}`, group.ID, service.PluginAPIGrantResourceType)
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/resource-grants", emptyEffective, "/resource-grants", handler)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "invalid_permissions")

	var grants []model.ResourceGrant
	require.NoError(t, db.Where("resource_type = ?", service.PluginAPIGrantResourceType).Find(&grants).Error)
	require.Len(t, grants, 1)
}

func TestKernelUpdatePluginInstallationConfigurationUsesSignedSchema(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}})
	t.Cleanup(func() { config.Set(previousConfig) })
	manifest := service.PluginManifest{
		ID: "handler-config", Name: "Handler Config", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: strings.Repeat("a", 64),
		ConfigSchema: []byte(`{"type":"object","required":["port"],"properties":{"port":{"type":"integer"}}}`),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	installation := model.PluginInstallation{PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, State: "disabled"}
	require.NoError(t, db.Create(&installation).Error)

	handler := (&KernelHandler{db: db}).UpdatePluginInstallationConfiguration
	path := "/plugin-installations/" + strconv.FormatUint(uint64(installation.ID), 10) + "/config"
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, `{"config":{"port":443},"expected_revision":0}`, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"revision":1`)

	recorder = performKernelHandlerRequest(t, http.MethodPut, path, `{"config":{"port":"443"},"expected_revision":1}`, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "invalid_plugin_configuration")
}

func TestKernelServesVerifiedWebUIAsset(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}})
	t.Cleanup(func() { config.Set(previousConfig) })

	source := `export const anixopsExtension = {}; export default {};`
	manifest := service.PluginManifest{
		ID: "handler-ui", Name: "Handler UI", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, Permissions: []string{"handler-ui.view"},
		ConfigSchema: []byte(`{"type":"object"}`),
		WebUI: &service.PluginWebUI{
			Bundle:      service.PluginWebUIBundle{Path: "webui/index.mjs"},
			Permissions: []string{"handler-ui.view"},
			Menus: []service.PluginWebUIMenu{{
				ID: "handler-ui.main", Parent: "services", Label: "Handler UI", Icon: "box",
				Route: "/admin/extensions/handler-ui", Permission: "handler-ui.view", Order: 10,
			}},
			Routes: []service.PluginWebUIRoute{{
				ID: "handler-ui.main", Path: "/admin/extensions/handler-ui", Export: "default", Permission: "handler-ui.view",
			}},
		},
	}
	artifact := kernelHandlerWebUIPackage(t, &manifest, source)
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)

	assetURL := service.PluginWebUIAssetURL(manifest.ID, manifest.Version, manifest.WebUI.Bundle.SHA256, manifest.WebUI.Bundle.Path)
	setupAdmin := func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("is_admin", true)
	}
	recorder := performKernelHandlerRequestWithSetup(
		t, http.MethodGet, assetURL, "",
		"/api/v3/extensions/:plugin_id/:version/webui/:sha256/:filename",
		(&KernelHandler{db: db}).ServePluginWebUIAsset,
		setupAdmin,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, source, recorder.Body.String())
	require.Equal(t, `"`+manifest.WebUI.Bundle.SHA256+`"`, recorder.Header().Get("ETag"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/javascript")

	require.NoError(t, db.Model(&installation).Update("enabled", false).Error)
	recorder = performKernelHandlerRequestWithSetup(
		t, http.MethodGet, assetURL, "",
		"/api/v3/extensions/:plugin_id/:version/webui/:sha256/:filename",
		(&KernelHandler{db: db}).ServePluginWebUIAsset,
		setupAdmin,
	)
	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())

	require.NoError(t, db.Model(&installation).Updates(map[string]any{"enabled": true, "state": "healthy"}).Error)
	group := model.AccessGroup{ScopeID: manifest.ID, Name: "asset-viewers", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: manifest.ID,
		Permissions: `["handler-ui.view"]`,
	}).Error)
	recorder = performKernelHandlerRequestWithSetup(
		t, http.MethodGet, assetURL, "",
		"/api/v3/extensions/:plugin_id/:version/webui/:sha256/:filename",
		(&KernelHandler{db: db}).ServePluginWebUIAsset,
		setupAdmin,
	)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "extension_asset_forbidden")

	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: 7}).Error)
	recorder = performKernelHandlerRequestWithSetup(
		t, http.MethodGet, assetURL, "",
		"/api/v3/extensions/:plugin_id/:version/webui/:sha256/:filename",
		(&KernelHandler{db: db}).ServePluginWebUIAsset,
		setupAdmin,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func TestKernelPluginRouteGatewayAuthorizesInstalledSignedRoutes(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("handler-plugin-api-artifact")
	artifactDigest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "handler-api", Name: "Handler API", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		Permissions:   []string{service.PluginAPIPermission("handler-api")},
		ControlRoutes: []string{"/api/v3/plugins/handler-api/status"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, ObservedVersion: manifest.Version,
		State: "healthy", Enabled: true, LifecycleGeneration: 1,
	}).Error)

	handler := (&KernelHandler{db: db}).PluginRouteGateway
	setupAdmin := func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("is_admin", true)
	}
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/status", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_host_unavailable")

	group := model.AccessGroup{ScopeID: manifest.ID, Name: "operators", Enabled: true}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: manifest.ID,
		Permissions: `{"` + service.PluginAPIPermission(manifest.ID) + `":true}`,
	}).Error)

	recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/status", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_route_forbidden")

	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: 7}).Error)
	recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/status", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_host_unavailable")

	recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/missing", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_route_not_found")
}

func TestKernelMachineTelemetryPluginRouteFailsClosedWithoutHost(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("machine-telemetry-control-artifact")
	artifactDigest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: plugincontrol.MachineTelemetryPluginID, Name: "Machine Telemetry", Version: plugincontrol.MachineTelemetryVersion,
		APIVersion: "v1", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		Permissions:    []string{"machine-telemetry.view", service.PluginAPIPermission(plugincontrol.MachineTelemetryPluginID)},
		ControlRoutes:  []string{plugincontrol.MachineTelemetryStatusRoute},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Node{
		Name: "telemetry-node", Host: "10.10.0.1", APIKey: "telemetry-node-key", Status: model.NodeStatusOnline, LastCheckAt: &now,
		RuntimeHealthy: true, CPUUsage: 23.5, MemoryUsage: 50, DiskUsage: 70,
	}).Error)

	// Installed and healthy, the way the start-up bootstrap leaves a
	// built-in package, but this Control runs no package host.
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, ObservedVersion: manifest.Version,
		State: "healthy", Enabled: true, LifecycleGeneration: 1,
	}).Error)

	handler := (&KernelHandler{db: db}).PluginRouteGateway
	setupAdmin := func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("is_admin", true)
	}
	recorder := performKernelHandlerRequestWithSetup(
		t, http.MethodGet, plugincontrol.MachineTelemetryStatusRoute, "",
		"/api/v3/plugins/:plugin_id/*route", handler, setupAdmin,
	)
	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_host_unavailable")

	recorder = performKernelHandlerRequestWithSetup(
		t, http.MethodPost, plugincontrol.MachineTelemetryStatusRoute, `{}`,
		"/api/v3/plugins/:plugin_id/*route", handler, setupAdmin,
	)
	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_host_unavailable")
}

func TestPluginRouteGatewayDoesNotUseRegistryFallback(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("knowledge-control-artifact")
	artifactDigest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "knowledge", Name: "Knowledge", Version: "4.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		Permissions:   []string{service.PluginAPIPermission("knowledge")},
		ControlRoutes: []string{"/api/v4/plugins/knowledge/articles"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, ObservedVersion: manifest.Version,
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)

	handler := (&KernelHandler{db: db}).PluginRouteGateway
	response := performKernelHandlerRequestWithSetup(
		t, http.MethodGet, "/api/v4/plugins/knowledge/articles", "", "/api/v4/plugins/:plugin_id/*route", handler,
		func(c *gin.Context) {
			c.Set("user_id", uint(7))
			c.Set("is_admin", true)
		},
	)

	require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "plugin_host_unavailable")
}

func TestPluginRouteGatewayDispatchesResolvedGenerationToHost(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), ControlHostRequestTimeout: "2s",
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("knowledge-control-artifact")
	artifactDigest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "knowledge", Name: "Knowledge", Version: "4.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		Permissions:   []string{service.PluginAPIPermission("knowledge")},
		ControlRoutes: []string{"/api/v4/plugins/knowledge/articles"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version, ObservedVersion: manifest.Version,
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)

	hosts := &capturingPluginHostManager{output: pluginhost.DispatchOutput{
		StatusCode: 202, Body: []byte(`{"accepted":true}`), Headers: []pluginhost.Header{{Name: "Content-Type", Value: "application/vnd.anixops.plugin+json"}},
	}}
	handler := (&KernelHandler{db: db, controlPluginHosts: hosts}).PluginRouteGateway
	response := performKernelHandlerRequestWithSetup(
		t, http.MethodPost, "/api/v4/plugins/knowledge/articles", `{"title":"new"}`, "/api/v4/plugins/:plugin_id/*route", handler,
		func(c *gin.Context) {
			c.Set("user_id", uint(7))
			c.Set("is_admin", true)
			c.Set("request_id", "request-1")
			c.Request.Header.Set("Idempotency-Key", "idempotency-1")
		},
	)

	require.Equal(t, 202, response.Code, response.Body.String())
	require.JSONEq(t, `{"accepted":true}`, response.Body.String())
	require.Equal(t, "application/vnd.anixops.plugin+json", response.Header().Get("Content-Type"))
	require.Equal(t, "request-1", hosts.input.RequestID)
	require.Equal(t, "idempotency-1", hosts.input.IdempotencyKey)
	require.EqualValues(t, 7, hosts.input.Generation)
	require.Equal(t, manifest.ID, hosts.input.PackageID)
	require.Equal(t, manifest.Version, hosts.input.Version)
	require.Equal(t, service.PluginControlBridgeRouteID(manifest.ID, manifest.ControlRoutes[0]), hosts.input.RouteID)
	require.Equal(t, manifest.ControlRoutes[0], hosts.input.Metadata.Path)
	require.Empty(t, hosts.input.Metadata.Query)
	require.JSONEq(t, `{"actor_id":7,"admin":true,"plugin_id":"knowledge"}`, string(hosts.input.PrincipalJSON))
	require.True(t, hosts.hasContextDeadline)
	require.WithinDuration(t, hosts.input.Deadline, hosts.contextDeadline, 50*time.Millisecond)
}

type capturingPluginHostManager struct {
	input              pluginhost.DispatchInput
	output             pluginhost.DispatchOutput
	err                error
	hasContextDeadline bool
	contextDeadline    time.Time
}

func (m *capturingPluginHostManager) Start(context.Context, pluginhost.ArtifactRef, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (m *capturingPluginHostManager) Dispatch(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	m.input = input
	m.contextDeadline, m.hasContextDeadline = ctx.Deadline()
	return m.output, m.err
}

func (*capturingPluginHostManager) Health(context.Context, string, string, uint64) (pluginhost.HostHealth, error) {
	return pluginhost.HostHealth{}, pluginhost.ErrHostUnavailable
}

func (*capturingPluginHostManager) Drain(context.Context, string, string, uint64, time.Time) error {
	return pluginhost.ErrHostUnavailable
}

func (*capturingPluginHostManager) Stop(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (*capturingPluginHostManager) Rollback(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

// Restart is the only package action: install, update, rollback, enable
// and disable are what every start does to the built-in packages.
func TestKernelControlPluginActionsAreRestartOnly(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	const pluginID = "restart-control"
	for _, version := range []string{"1.0.0", "2.0.0"} {
		artifact := []byte("restart-control-" + version)
		digest := sha256.Sum256(artifact)
		manifest := service.PluginManifest{
			ID: pluginID, Name: "Restart Control", Version: version, APIVersion: "v1", Publisher: "AnixOps",
			Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
		}
		canonical, canonicalErr := service.CanonicalPluginManifest(manifest)
		require.NoError(t, canonicalErr)
		release, registerErr := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
		require.NoError(t, registerErr)
		_, storeErr := service.StorePluginArtifact(db, release.ID, artifact)
		require.NoError(t, storeErr)
	}
	installation := model.PluginInstallation{
		PluginID: pluginID, Target: "control", DesiredVersion: "2.0.0", ObservedVersion: "2.0.0",
		PreviousVersion: "1.0.0", State: "healthy", Enabled: true, LifecycleGeneration: 3, LastError: "control-error-must-not-leak",
	}
	require.NoError(t, db.Create(&installation).Error)
	path := "/plugin-installations/" + strconv.FormatUint(uint64(installation.ID), 10) + "/actions"
	handler := (&KernelHandler{db: db}).PluginInstallationAction

	for _, action := range []string{"rollback", "update", "enable", "disable", "install", "health", "inspect"} {
		body := `{"action":"` + action + `","target_version":"1.0.0","idempotency_key":"operator-` + action + `"}`
		recorder := performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
		require.Equal(t, http.StatusBadRequest, recorder.Code, action+": "+recorder.Body.String())
		requireKernelErrorCode(t, recorder.Body.Bytes(), "unsupported_plugin_action")
	}
	var count int64
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.Zero(t, count, "a refused action queues nothing")
	var unchanged model.PluginInstallation
	require.NoError(t, db.First(&unchanged, installation.ID).Error)
	require.Equal(t, installation.DesiredVersion, unchanged.DesiredVersion)
	require.Equal(t, installation.LifecycleGeneration, unchanged.LifecycleGeneration)
	require.True(t, unchanged.Enabled)

	body := `{"action":"restart","idempotency_key":"operator-restart-1"}`
	recorder := performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	operationID := recorder.Header().Get("X-AnixOps-Operation-ID")
	require.NotEmpty(t, operationID)
	require.Equal(t, operationID, recorder.Header().Get("X-AnixOps-Operation-Chain"))
	require.Contains(t, recorder.Body.String(), `"kind":"plugin.enable"`)
	require.Contains(t, recorder.Body.String(), `"target_version":"2.0.0"`)
	require.NotContains(t, recorder.Body.String(), "control-error-must-not-leak")
	require.NoError(t, db.First(&unchanged, installation.ID).Error)
	require.Equal(t, installation.LifecycleGeneration, unchanged.LifecycleGeneration, "a restart keeps the generation")

	// The same client identity returns the same operation.
	recorder = performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.Equal(t, operationID, recorder.Header().Get("X-AnixOps-Operation-ID"))
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("plugin_id = ?", pluginID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// A disabled installation is not restarted.
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("id = ?", installation.ID).Update("enabled", false).Error)
	recorder = performKernelHandlerRequest(t, http.MethodPost, path, `{"action":"restart","idempotency_key":"operator-restart-2"}`, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "plugin_action_rejected")
}

func TestKernelCreateTopologyRequiresExistingScope(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	handler := (&KernelHandler{db: db}).CreateTopology
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/topologies", `{"name":"invalid","service_scope":"missing"}`, "/topologies", handler)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "scope_not_found")

	var count int64
	require.NoError(t, db.Model(&model.Topology{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestKernelTopologyDeploymentPlanStatusAndExecutionGate(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	previousConfig := config.Get()
	config.Set(nil)
	t.Cleanup(func() { config.Set(previousConfig) })
	node := model.Node{Name: "topology-handler-node", Host: "127.0.0.1", APIKey: "topology-handler-key"}
	require.NoError(t, db.Create(&node).Error)
	pluginID := "topology-handler-runtime"
	require.NoError(t, db.Create(&model.Plugin{ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true}).Error)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}).Error)
	topology := model.Topology{Name: "topology-handler", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: pluginID, Role: "entry", DesiredVersion: "1.0.0", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: pluginID, Role: "entry", ConfigJSON: `{}`,
	}).Error)

	handler := &KernelHandler{db: db}
	planBody := `{"topology_id":` + strconv.FormatUint(uint64(topology.ID), 10) + `,"revision_id":` + strconv.FormatUint(uint64(revision.ID), 10) + `}`
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/deployments", planBody, "/deployments", handler.PlanDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	var deployment model.TopologyDeployment
	require.NoError(t, db.First(&deployment, "topology_id = ?", topology.ID).Error)
	var steps []model.TopologyDeploymentStep
	require.NoError(t, db.Where("deployment_id = ?", deployment.ID).Find(&steps).Error)
	require.Len(t, steps, 1)

	path := "/deployments/" + strconv.FormatUint(uint64(deployment.ID), 10)
	recorder = performKernelHandlerRequest(t, http.MethodGet, path, "", "/deployments/:id", handler.GetDeploymentStatus)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"steps"`)

	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/apply", "", "/deployments/:id/apply", handler.ApplyDeployment)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "topology_execution_disabled")

	enableTopologyExecution(t)
	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/apply", "", "/deployments/:id/apply", handler.ApplyDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.NoError(t, db.First(&deployment, deployment.ID).Error)
	require.Equal(t, "applying", deployment.State)

	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/rollback", "", "/deployments/:id/rollback", handler.RollbackDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.NoError(t, db.First(&deployment, deployment.ID).Error)
	require.Equal(t, "rollback_requested", deployment.State)
}

func TestKernelDeploymentStatusAndObservedStatesDoNotExposeKernelOrAgentSecrets(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	deployment := model.TopologyDeployment{
		TopologyID: 41, RevisionID: 42, State: "failed", FailurePolicy: "stop_and_rollback", CreatedBy: 1,
		LastError: "deployment-error-token-must-not-leak", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&deployment).Error)
	nodeID := uint(7)
	step := model.TopologyDeploymentStep{
		DeploymentID: deployment.ID, VertexID: 1, VertexKey: "entry", NodeID: nodeID,
		PluginID: "test-plugin", Role: "entry", TargetVersion: "1.0.0", ApplyOrder: 1,
		ApplyAction: "configure_enable", ConfigJSON: `{"token":"topology-config-token-must-not-leak"}`,
		RollbackMode: "restore", RollbackConfigJSON: `{"secret":"rollback-config-secret-must-not-leak"}`,
		State: "failed", LastError: "step-error-token-must-not-leak", ConfigureOperationID: "status-secret-operation",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&model.KernelOperation{
		ID: "status-secret-operation", IdempotencyKey: "status-secret-idempotency", NodeID: &nodeID,
		PluginID: "test-plugin", TargetVersion: "1.0.0", Kind: "plugin.configure", State: "failed",
		TopologyDeploymentID: &deployment.ID, TopologyStepID: &step.ID, TopologyRevision: 42,
		ConfigJSON: `{"token":"operation-config-token-must-not-leak"}`,
		ResultJSON: `{"agent_secret":"agent-result-secret-must-not-leak"}`,
		LastError:  "agent-last-error-token-must-not-leak", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyObservedState{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 42, ObservedRevision: 41,
		State: "failed", HealthJSON: `{"agent_token":"observed-health-token-must-not-leak"}`,
		LastError: "observed-last-error-token-must-not-leak", UpdatedAt: now,
	}).Error)

	handler := &KernelHandler{db: db}
	statusPath := "/deployments/" + strconv.FormatUint(uint64(deployment.ID), 10)
	statusRecorder := performKernelHandlerRequest(t, http.MethodGet, statusPath, "", "/deployments/:id", handler.GetDeploymentStatus)
	require.Equal(t, http.StatusOK, statusRecorder.Code, statusRecorder.Body.String())
	listRecorder := performKernelHandlerRequest(t, http.MethodGet, "/deployments", "", "/deployments", handler.ListDeployments)
	require.Equal(t, http.StatusOK, listRecorder.Code, listRecorder.Body.String())
	observedRecorder := performKernelHandlerRequest(t, http.MethodGet, "/observed-states?deployment_id="+strconv.FormatUint(uint64(deployment.ID), 10), "", "/observed-states", handler.ListObservedStates)
	require.Equal(t, http.StatusOK, observedRecorder.Code, observedRecorder.Body.String())

	for _, response := range []string{statusRecorder.Body.String(), listRecorder.Body.String(), observedRecorder.Body.String()} {
		for _, secret := range []string{
			"deployment-error-token-must-not-leak", "topology-config-token-must-not-leak",
			"rollback-config-secret-must-not-leak", "step-error-token-must-not-leak",
			"operation-config-token-must-not-leak", "agent-result-secret-must-not-leak",
			"agent-last-error-token-must-not-leak", "observed-health-token-must-not-leak",
			"observed-last-error-token-must-not-leak",
		} {
			require.NotContains(t, response, secret)
		}
	}

	statusPayload := statusRecorder.Body.String()
	for _, field := range []string{`"config":`, `"rollback_config":`, `"health":`, `"result":`} {
		require.NotContains(t, statusPayload, field)
	}
	require.Contains(t, statusPayload, `"last_error":"deployment failed"`)
	require.Contains(t, statusPayload, `"last_error":"plugin configuration failed"`)
	require.Contains(t, observedRecorder.Body.String(), `"last_error":"observed state failed"`)
}

func TestKernelAssignmentPreservesDisabledStateWhenEnabledIsOmitted(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	node := model.Node{Name: "node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	handler := (&KernelHandler{db: db}).UpsertAssignment
	path := "/nodes/" + strconv.FormatUint(uint64(node.ID), 10) + "/assignments"
	route := "/nodes/:id/assignments"
	body := `{"service_scope":"forward","plugin_id":"machine-telemetry","role":"entry","enabled":false}`
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, body, route, handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body = `{"service_scope":"forward","plugin_id":"machine-telemetry","role":"entry"}`
	recorder = performKernelHandlerRequest(t, http.MethodPut, path, body, route, handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var assignment model.NodeServiceAssignment
	require.NoError(t, db.First(&assignment, "node_id = ?", node.ID).Error)
	require.False(t, assignment.Enabled)
}

func TestKernelCancelOperationMapsInvalidAndTerminalState(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	handler := (&KernelHandler{db: db}).CancelOperation
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/operations/not-a-uuid/cancel", "", "/operations/:id/cancel", handler)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())

	deadline := time.Now().Add(time.Minute)
	operation := model.KernelOperation{
		ID: "b8d056d5-dd35-475f-a45a-61f744613547", IdempotencyKey: "terminal-handler", SessionID: "session",
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.health", Revision: 1,
		ConfigHash: strings.Repeat("a", 64), State: "completed", DeadlineAt: &deadline,
	}
	require.NoError(t, db.Create(&operation).Error)
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/operations/"+operation.ID+"/cancel", "", "/operations/:id/cancel", handler)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "operation_not_cancellable")
}
