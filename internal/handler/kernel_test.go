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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newKernelHandlerTestDB(t *testing.T, extra ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, service.EnsureKernelSchema(db))
	if len(extra) > 0 {
		require.NoError(t, db.AutoMigrate(extra...))
	}
	return db
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

	registry, err := plugincontrol.NewRegistry(plugincontrol.NewGostMeshExecutor(db))
	require.NoError(t, err)
	handler := (&KernelHandler{db: db, controlPluginExecutors: registry}).UpdatePluginInstallationConfiguration
	path := "/plugin-installations/" + strconv.FormatUint(uint64(installation.ID), 10) + "/config"
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, `{"config":{"port":443},"expected_revision":0}`, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"revision":1`)

	recorder = performKernelHandlerRequest(t, http.MethodPut, path, `{"config":{"port":"443"},"expected_revision":1}`, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "invalid_plugin_configuration")
}

func gostMeshHandlerConfiguration(exitTable int) string {
	return fmt.Sprintf(`{
		"api_version":"anixops.gost-mesh/v1",
		"apply":true,
		"rollback_on_exit":true,
		"tunnels":[{
			"id":"mesh-exit-quic",
			"role":"exit",
			"transport":"quic",
			"listen":{"address":"0.0.0.0","port":443},
			"tun":{"name":"anxquicx","address":"172.31.66.1/30","peer_address":"172.31.66.2","port":18421,"mtu":1280},
			"routing":{"source_cidrs":[],"route_cidrs":["10.66.0.0/24"],"table":%d,"priority":0},
			"tls":{"server_name":"","ca_file":"/run/anixops/secrets/mesh-ca.pem","cert_file":"/run/anixops/secrets/mesh-server.pem","key_file":"/run/anixops/secrets/mesh-server-key.pem"},
			"wss_path":"",
			"health":{"enabled":false,"target":"","source_address":"","interval_seconds":15,"timeout_seconds":3,"failure_threshold":3,"restart_delay_seconds":3,"restart_limit":10}
		}]
	}`, exitTable)
}

func TestKernelUpdateGostMeshConfigurationEnforcesExactSemanticValidator(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}})
	t.Cleanup(func() { config.Set(previousConfig) })

	manifest := service.PluginManifest{
		ID: plugincontrol.GostMeshPluginID, Name: "GOST Mesh", Version: plugincontrol.GostMeshVersion,
		APIVersion: "v1", Publisher: "AnixOps", Targets: []string{"agent"},
		ArtifactSHA256: strings.Repeat("b", 64), ConfigSchema: []byte(`{"type":"object"}`),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	installation := model.PluginInstallation{PluginID: manifest.ID, Target: "agent", DesiredVersion: manifest.Version, State: "disabled"}
	require.NoError(t, db.Create(&installation).Error)
	registry, err := plugincontrol.NewRegistry(plugincontrol.NewGostMeshExecutor(db))
	require.NoError(t, err)
	handler := (&KernelHandler{db: db, controlPluginExecutors: registry}).UpdatePluginInstallationConfiguration
	path := "/plugin-installations/" + strconv.FormatUint(uint64(installation.ID), 10) + "/config"

	invalidBody := `{"config":` + gostMeshHandlerConfiguration(100) + `,"expected_revision":0}`
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, invalidBody, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "invalid_plugin_configuration")
	require.Contains(t, recorder.Body.String(), "routing.table")
	var stored model.PluginInstallation
	require.NoError(t, db.First(&stored, installation.ID).Error)
	require.Zero(t, stored.ConfigRevision)
	var configurationCount int64
	require.NoError(t, db.Model(&model.PluginConfiguration{}).Where("installation_id = ?", installation.ID).Count(&configurationCount).Error)
	require.Zero(t, configurationCount)

	validBody := `{"config":` + gostMeshHandlerConfiguration(0) + `,"expected_revision":0}`
	recorder = performKernelHandlerRequest(t, http.MethodPut, path, validBody, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"revision":1`)

	manifest.Version = "1.0.1"
	manifest.ArtifactSHA256 = strings.Repeat("c", 64)
	canonical, err = service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	require.NoError(t, db.Model(&installation).Update("desired_version", manifest.Version).Error)
	versionBoundBody := `{"config":` + gostMeshHandlerConfiguration(0) + `,"expected_revision":1}`
	recorder = performKernelHandlerRequest(t, http.MethodPut, path, versionBoundBody, "/plugin-installations/:id/config", handler)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_configuration_validator_unavailable")
}

func TestKernelPluginArtifactUploadAndInstallationGate(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey)}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("handler-artifact")
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "handler-artifact", Name: "Handler Artifact", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)

	installHandler := (&KernelHandler{db: db}).UpsertPluginInstallation
	installBody := `{"plugin_id":"handler-artifact","target":"control","desired_version":"1.0.0","enabled":true}`
	recorder := performKernelHandlerRequest(t, http.MethodPut, "/plugin-installations", installBody, "/plugin-installations", installHandler)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_artifact_missing")

	artifactHandler := (&KernelHandler{db: db}).UploadPluginReleaseArtifact
	artifactPath := "/plugin-releases/" + strconv.FormatUint(uint64(release.ID), 10) + "/artifact"
	recorder = performKernelHandlerRequest(t, http.MethodPost, artifactPath, `{"artifact_base64":"dGFtcGVyZWQ="}`, "/plugin-releases/:id/artifact", artifactHandler)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_artifact_rejected")

	goodBody := `{"artifact_base64":"` + base64.StdEncoding.EncodeToString(artifact) + `"}`
	recorder = performKernelHandlerRequest(t, http.MethodPost, artifactPath, goodBody, "/plugin-releases/:id/artifact", artifactHandler)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), service.PluginArtifactStorageKey(manifest.ID, manifest.Version, manifest.ArtifactSHA256))

	recorder = performKernelHandlerRequest(t, http.MethodPut, "/plugin-installations", installBody, "/plugin-installations", installHandler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"state":"pending"`)
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

	assetURL := service.PluginWebUIAssetURL(manifest.ID, manifest.Version, manifest.WebUI.Bundle.SHA256, manifest.WebUI.Bundle.Path)
	recorder := performKernelHandlerRequest(
		t, http.MethodGet, assetURL, "",
		"/api/v3/extensions/:plugin_id/:version/webui/:sha256/:filename",
		(&KernelHandler{db: db}).ServePluginWebUIAsset,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, source, recorder.Body.String())
	require.Equal(t, `"`+manifest.WebUI.Bundle.SHA256+`"`, recorder.Header().Get("ETag"))
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/javascript")
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
		State: "healthy", Enabled: true,
	}).Error)

	handler := (&KernelHandler{db: db}).PluginRouteGateway
	setupAdmin := func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("is_admin", true)
	}
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/status", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusNotImplemented, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_route_not_implemented")

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
	require.Equal(t, http.StatusNotImplemented, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"permission":"handler-api.api"`)

	recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v3/plugins/handler-api/missing", "", "/api/v3/plugins/:plugin_id/*route", handler, setupAdmin)
	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_route_not_found")
}

func TestKernelMachineTelemetryPluginRouteExecutesAfterKernelAdmission(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), ControlExecutionEnabled: true,
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

	registry, err := plugincontrol.NewRegistry(plugincontrol.NewMachineTelemetryExecutor(db))
	require.NoError(t, err)
	installRecorder := performKernelHandlerRequest(
		t, http.MethodPut, "/plugin-installations",
		`{"plugin_id":"machine-telemetry","target":"control","desired_version":"1.0.0","enabled":true}`,
		"/plugin-installations", (&KernelHandler{db: db, controlPluginExecutors: registry}).UpsertPluginInstallation,
	)
	require.Equal(t, http.StatusOK, installRecorder.Code, installRecorder.Body.String())
	require.Contains(t, installRecorder.Body.String(), `"state":"pending"`)
	require.NotEmpty(t, installRecorder.Header().Get("X-AnixOps-Operation-ID"))
	worker, err := plugincontrol.NewOperationWorker(db, registry)
	require.NoError(t, err)
	processed, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, processed)
	var installation model.PluginInstallation
	require.NoError(t, db.First(&installation, "plugin_id = ? AND target = ?", manifest.ID, "control").Error)
	require.Equal(t, "healthy", installation.State)
	require.Equal(t, manifest.Version, installation.ObservedVersion)

	handler := (&KernelHandler{db: db, controlPluginExecutors: registry}).PluginRouteGateway
	setupAdmin := func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("is_admin", true)
	}
	recorder := performKernelHandlerRequestWithSetup(
		t, http.MethodGet, plugincontrol.MachineTelemetryStatusRoute, "",
		"/api/v3/plugins/:plugin_id/*route", handler, setupAdmin,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"plugin_id":"machine-telemetry"`)
	require.Contains(t, recorder.Body.String(), `"name":"telemetry-node"`)
	require.Contains(t, recorder.Body.String(), `"cpu_usage":23.5`)

	recorder = performKernelHandlerRequestWithSetup(
		t, http.MethodPost, plugincontrol.MachineTelemetryStatusRoute, `{}`,
		"/api/v3/plugins/:plugin_id/*route", handler, setupAdmin,
	)
	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "plugin_method_not_allowed")
}

func TestKernelControlPluginRollbackActionQueuesDurableOperation(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), ControlExecutionEnabled: true,
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	const pluginID = "rollback-control"
	for _, version := range []string{"1.0.0", "2.0.0"} {
		artifact := []byte("rollback-control-" + version)
		digest := sha256.Sum256(artifact)
		manifest := service.PluginManifest{
			ID: pluginID, Name: "Rollback Control", Version: version, APIVersion: "v1", Publisher: "AnixOps",
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
		PreviousVersion: "1.0.0", State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)
	path := "/plugin-installations/" + strconv.FormatUint(uint64(installation.ID), 10) + "/actions"
	body := `{"action":"rollback","idempotency_key":"operator-rollback-1"}`
	handler := (&KernelHandler{db: db}).PluginInstallationAction
	recorder := performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	operationID := recorder.Header().Get("X-AnixOps-Operation-ID")
	require.NotEmpty(t, operationID)
	require.Contains(t, recorder.Body.String(), `"kind":"plugin.rollback"`)
	require.Contains(t, recorder.Body.String(), `"target_version":"1.0.0"`)

	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "1.0.0", installation.DesiredVersion)
	require.Equal(t, "2.0.0", installation.ObservedVersion)
	require.Equal(t, "pending", installation.State)
	var operation model.KernelOperation
	require.NoError(t, db.First(&operation, "id = ?", operationID).Error)
	require.Equal(t, "pending", operation.State)

	recorder = performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.Equal(t, operationID, recorder.Header().Get("X-AnixOps-Operation-ID"))
	var count int64
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("plugin_id = ?", pluginID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// Replaying the same client identity after the original rollback completed
	// must return the original operation without interpreting the now-swapped
	// previous_version as a new rollback request.
	require.NoError(t, db.Model(&operation).Updates(map[string]any{"state": "succeeded"}).Error)
	require.NoError(t, db.Model(&installation).Updates(map[string]any{
		"desired_version": "1.0.0", "observed_version": "1.0.0", "previous_version": "2.0.0", "state": "healthy",
	}).Error)
	recorder = performKernelHandlerRequest(t, http.MethodPost, path, body, "/plugin-installations/:id/actions", handler)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.Equal(t, operationID, recorder.Header().Get("X-AnixOps-Operation-ID"))
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "1.0.0", installation.DesiredVersion)
	require.Equal(t, "2.0.0", installation.PreviousVersion)
	require.Equal(t, "healthy", installation.State)
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

	config.Set(&config.Config{Plugins: config.PluginConfig{TopologyExecutionEnabled: true}})
	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/apply", "", "/deployments/:id/apply", handler.ApplyDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.NoError(t, db.First(&deployment, deployment.ID).Error)
	require.Equal(t, "applying", deployment.State)

	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/rollback", "", "/deployments/:id/rollback", handler.RollbackDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.NoError(t, db.First(&deployment, deployment.ID).Error)
	require.Equal(t, "rollback_requested", deployment.State)
}

func TestKernelAssignmentPreservesDisabledStateWhenEnabledIsOmitted(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	node := model.Node{Name: "node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	handler := (&KernelHandler{db: db}).UpsertAssignment
	path := "/nodes/" + strconv.FormatUint(uint64(node.ID), 10) + "/assignments"
	route := "/nodes/:id/assignments"
	body := `{"service_scope":"forward","plugin_id":"nftables-forward","role":"entry","enabled":false}`
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, body, route, handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body = `{"service_scope":"forward","plugin_id":"nftables-forward","role":"entry"}`
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
