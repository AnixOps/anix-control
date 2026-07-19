package router

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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/identitybridge"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type v2TestHost struct {
	lastRouteID string
}

type v2PackageRouteCatalogRow struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Owner     string `json:"owner"`
	RouteID   string `json:"route_id"`
	Transport string `json:"transport"`
}

func loadV2PackageRouteCatalog(t *testing.T) []v2PackageRouteCatalogRow {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(sourceFile), "..", "..", "config", "v2-package-route-catalog.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var routes []v2PackageRouteCatalogRow
	require.NoError(t, json.Unmarshal(raw, &routes))
	require.NotEmpty(t, routes)
	return routes
}

func (h *v2TestHost) Start(context.Context, pluginhost.ArtifactRef, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (h *v2TestHost) Dispatch(_ context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	h.lastRouteID = input.RouteID
	if strings.HasPrefix(input.RouteID, "identity.") {
		return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`)}, nil
	}
	return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{"items":[]}`)}, nil
}

func (h *v2TestHost) Health(context.Context, string, string, uint64) (pluginhost.HostHealth, error) {
	return pluginhost.HostHealth{}, pluginhost.ErrHostUnavailable
}

func (h *v2TestHost) Drain(context.Context, string, string, uint64, time.Time) error {
	return pluginhost.ErrHostUnavailable
}

func (h *v2TestHost) Stop(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func (h *v2TestHost) Rollback(context.Context, string, string, uint64) error {
	return pluginhost.ErrHostUnavailable
}

func testHost(t *testing.T) *v2TestHost {
	t.Helper()
	host := &v2TestHost{}
	previous := pluginhost.DefaultManager()
	pluginhost.SetDefaultManager(host)
	t.Cleanup(func() { pluginhost.SetDefaultManager(previous) })
	return host
}

func seedV2KnowledgePackage(t *testing.T, cfg *config.Config) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	cfg.Plugins.ControlExecutionEnabled = true
	config.Set(cfg)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))

	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[]}`)
	routes := []byte(`{"api_version":"v2","package_id":"knowledge","routes":[{"method":"GET","legacy_path":"/api/v2/user/knowledge","package_route":"knowledge.article.list","envelope":"data"},{"method":"GET","legacy_path":"/api/v2/admin/ws/monitor","package_route":"telemetry.monitor.ws","envelope":"websocket","transport":"websocket"}]}`)
	artifact := v2TestPackage(t, map[string][]byte{
		"bin/control-host":      entrypoint,
		"compat/v2-routes.json": routes,
		"migrations/index.json": migrations,
	})
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256(routes)
	manifest := service.PluginManifest{
		ID: "knowledge", Name: "Knowledge", Version: "4.0.0", APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256:    hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint: &service.PluginEntrypoint{Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:])},
		Migrations:        &service.PluginMigrations{Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:])},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:]),
		},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(database.GetDB(), string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(database.GetDB(), release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.PluginInstallation{
		PluginID: "knowledge", Target: "control", DesiredVersion: "4.0.0", ObservedVersion: "4.0.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)
	return publicKey, privateKey
}

func seedV2IdentityPackage(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[]}`)
	routes := []byte(`{"api_version":"v2","package_id":"identity-platform","routes":[{"method":"POST","legacy_path":"/api/v2/login","package_route":"identity.auth.login","envelope":"panel"},{"method":"POST","legacy_path":"/api/v2/register","package_route":"identity.auth.register","envelope":"panel"}]}`)
	artifact := v2TestPackage(t, map[string][]byte{
		"bin/control-host": entrypoint, "compat/v2-routes.json": routes, "migrations/index.json": migrations,
	})
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256(routes)
	manifest := service.PluginManifest{
		ID: "identity-platform", Name: "Identity Platform", Version: "4.0.0", APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256:    hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint: &service.PluginEntrypoint{Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:])},
		Migrations:        &service.PluginMigrations{Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:])},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:]),
		},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(database.GetDB(), string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(database.GetDB(), release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.PluginInstallation{
		PluginID: "identity-platform", Target: "control", DesiredVersion: "4.0.0", ObservedVersion: "4.0.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)
}

func seedV2TaskSixPackage(t *testing.T, packageID, name, routes string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[]}`)
	artifact := v2TestPackage(t, map[string][]byte{
		"bin/control-host": entrypoint, "compat/v2-routes.json": []byte(routes), "migrations/index.json": migrations,
	})
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256([]byte(routes))
	manifest := service.PluginManifest{
		ID: packageID, Name: name, Version: "4.0.0", APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256:    hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint: &service.PluginEntrypoint{Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:])},
		Migrations:        &service.PluginMigrations{Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:])},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:]),
		},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(database.GetDB(), string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(database.GetDB(), release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.PluginInstallation{
		PluginID: packageID, Target: "control", DesiredVersion: "4.0.0", ObservedVersion: "4.0.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)
}

func seedV2TaskSixPackages(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	seedV2TaskSixPackage(t, "ticket", "Ticket", `{"api_version":"v2","package_id":"ticket","routes":[{"method":"GET","legacy_path":"/api/v2/user/ticket","package_route":"ticket.user.ticket.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "plan", "Plan", `{"api_version":"v2","package_id":"plan","routes":[{"method":"GET","legacy_path":"/api/v2/user/plan","package_route":"plan.user.plan.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "notification", "Notification", `{"api_version":"v2","package_id":"notification","routes":[{"method":"GET","legacy_path":"/api/v2/user/notifications","package_route":"notification.user.notifications.get","envelope":"data"}]}`, publicKey, privateKey)
}

func seedV2TaskSevenPackages(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	seedV2TaskSixPackage(t, "order", "Order", `{"api_version":"v2","package_id":"order","routes":[{"method":"GET","legacy_path":"/api/v2/user/order","package_route":"order.user.order.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "payment", "Payment", `{"api_version":"v2","package_id":"payment","routes":[{"method":"GET","legacy_path":"/api/v2/payment/methods","package_route":"payment.payment.methods.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "subscription", "Subscription", `{"api_version":"v2","package_id":"subscription","routes":[{"method":"GET","legacy_path":"/api/v2/user/subscription","package_route":"subscription.user.subscription.get","envelope":"data"}]}`, publicKey, privateKey)
}

func seedV2TaskEightPackages(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) {
	t.Helper()
	seedV2TaskSixPackage(t, "machine-telemetry", "Machine Telemetry", `{"api_version":"v2","package_id":"machine-telemetry","routes":[{"method":"GET","legacy_path":"/api/v2/admin/dashboard","package_route":"telemetry.admin.dashboard.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "proxy-node", "Proxy Node", `{"api_version":"v2","package_id":"proxy-node","routes":[{"method":"GET","legacy_path":"/api/v2/admin/nodes","package_route":"proxy.admin.nodes.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "protocol-runtime", "Protocol Runtime", `{"api_version":"v2","package_id":"protocol-runtime","routes":[{"method":"GET","legacy_path":"/api/v2/agent/tasks","package_route":"protocol.agent.tasks.get","envelope":"raw"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "wireguard", "WireGuard", `{"api_version":"v2","package_id":"wireguard","routes":[{"method":"POST","legacy_path":"/api/v2/admin/wireguard/keypair","package_route":"wireguard.admin.wireguard.keypair.post","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "forward", "Forward", `{"api_version":"v2","package_id":"forward","routes":[{"method":"GET","legacy_path":"/api/v2/user/forward/rules","package_route":"forward.user.forward.rules.get","envelope":"data"}]}`, publicKey, privateKey)
	seedV2TaskSixPackage(t, "gost-mesh", "Gost Mesh", `{"api_version":"v2","package_id":"gost-mesh","routes":[{"method":"GET","legacy_path":"/api/v2/admin/forward/nodex/status","package_route":"gost.admin.forward.nodex.status.get","envelope":"data"}]}`, publicKey, privateKey)
}

func v2TestPackage(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"bin/control-host", "compat/v2-routes.json", "migrations/index.json"} {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write(files[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func requestV2(t *testing.T, router *gin.Engine, cfg *config.Config, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := utils.GenerateToken(1, "user@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	return requestV2WithToken(t, router, method, path, token)
}

func requestV2Admin(t *testing.T, router *gin.Engine, cfg *config.Config, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	return requestV2WithToken(t, router, method, path, token)
}

func requestV2WithToken(t *testing.T, router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func disablePackage(t *testing.T, packageID string) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&model.PluginInstallation{}).
		Where("plugin_id = ? AND target = ?", packageID, "control").Update("enabled", false).Error)
}

func setupV2PackageRouter(t *testing.T) (*gin.Engine, *config.Config, *v2TestHost) {
	t.Helper()
	cache.InitMemory()
	require.NoError(t, database.Close())
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}))

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{Secret: "test-jwt-secret", Expire: 86400},
		App: config.AppConfig{APIToken: "test-api-token", SubscribePath: "s", TrafficLogEnable: true},
	}
	host := testHost(t)
	publicKey, privateKey := seedV2KnowledgePackage(t, cfg)
	seedV2IdentityPackage(t, publicKey, privateKey)
	seedV2TaskSixPackages(t, publicKey, privateKey)
	seedV2TaskSevenPackages(t, publicKey, privateKey)
	seedV2TaskEightPackages(t, publicKey, privateKey)
	router := gin.New()
	Setup(router, cfg)
	return router, cfg, host
}

func TestV2TaskSixRepresentativeRoutesUsePackageHosts(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	for _, test := range []struct {
		path    string
		routeID string
	}{
		{path: "/api/v2/user/knowledge", routeID: "knowledge.article.list"},
		{path: "/api/v2/user/ticket", routeID: "ticket.user.ticket.get"},
		{path: "/api/v2/user/plan", routeID: "plan.user.plan.get"},
		{path: "/api/v2/user/notifications", routeID: "notification.user.notifications.get"},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			response := requestV2(t, router, cfg, http.MethodGet, test.path)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, test.routeID, host.lastRouteID)
		})
	}
}

func TestV2TaskSevenRepresentativeRoutesUsePackageHosts(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	for _, test := range []struct {
		path    string
		routeID string
	}{
		{path: "/api/v2/user/order", routeID: "order.user.order.get"},
		{path: "/api/v2/payment/methods", routeID: "payment.payment.methods.get"},
		{path: "/api/v2/user/subscription", routeID: "subscription.user.subscription.get"},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			response := requestV2(t, router, cfg, http.MethodGet, test.path)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, test.routeID, host.lastRouteID)
		})
	}
}

func TestV2TaskEightRepresentativeRoutesUsePackageHosts(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	for _, test := range []struct {
		method  string
		path    string
		routeID string
		admin   bool
	}{
		{method: http.MethodGet, path: "/api/v2/admin/dashboard", routeID: "telemetry.admin.dashboard.get", admin: true},
		{method: http.MethodGet, path: "/api/v2/admin/nodes", routeID: "proxy.admin.nodes.get", admin: true},
		{method: http.MethodGet, path: "/api/v2/agent/tasks", routeID: "protocol.agent.tasks.get"},
		{method: http.MethodPost, path: "/api/v2/admin/wireguard/keypair", routeID: "wireguard.admin.wireguard.keypair.post", admin: true},
		{method: http.MethodGet, path: "/api/v2/user/forward/rules", routeID: "forward.user.forward.rules.get"},
		{method: http.MethodGet, path: "/api/v2/admin/forward/nodex/status", routeID: "gost.admin.forward.nodex.status.get", admin: true},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			var response *httptest.ResponseRecorder
			if test.admin {
				response = requestV2Admin(t, router, cfg, test.method, test.path)
			} else {
				response = requestV2(t, router, cfg, test.method, test.path)
			}
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, test.routeID, host.lastRouteID)
		})
	}
}

func TestV2WebSocketRoutesRegisterExactPackageBridgeOperations(t *testing.T) {
	_, _, _ = setupV2PackageRouter(t)
	for _, test := range []struct {
		packageID string
		routeID   string
	}{
		{packageID: "machine-telemetry", routeID: "telemetry.admin.ws.monitor.get"},
		{packageID: "proxy-node", routeID: "proxy.node.ws.get"},
		{packageID: "protocol-runtime", routeID: "protocol.agent.ws.get"},
	} {
		t.Run(test.routeID, func(t *testing.T) {
			operation, ok := packagebridge.DefaultRouteRegistry().ResolveWebSocket(test.packageID, test.routeID, test.routeID)
			require.True(t, ok)
			require.NotNil(t, operation)
		})
	}
}

func TestPackageRouteWrappersAlwaysUseTheirGenericGateways(t *testing.T) {
	gateway := func(*gin.Context) {}
	legacy := func(*gin.Context) {}

	httpRoute := registeredPackageRoute(gateway, "knowledge", "knowledge.wrapper.gateway.test", legacy)
	require.Equal(t, reflect.ValueOf(gateway).Pointer(), reflect.ValueOf(httpRoute).Pointer())

	webSocketRoute := registeredPackageWebSocketRoute(gateway, "proxy-node", "proxy.wrapper.gateway.test", legacy, nil)
	require.Equal(t, reflect.ValueOf(gateway).Pointer(), reflect.ValueOf(webSocketRoute).Pointer())

	calledGateway := false
	calledLegacy := false
	preflightRoute := registeredPackageWebSocketRoute(
		func(*gin.Context) { calledGateway = true },
		"protocol-runtime",
		"protocol.wrapper.gateway.test",
		func(*gin.Context) { calledLegacy = true },
		func(*gin.Context) bool { return true },
	)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v2/agent/ws", nil)
	preflightRoute(context)
	require.True(t, calledGateway)
	require.False(t, calledLegacy)
}

func TestAllCataloguedV2RoutesResolveThroughTheirPackageBridge(t *testing.T) {
	_, cfg, _ := setupV2PackageRouter(t)
	identityAllowlist, err := identitybridge.NewAllowlist(cfg)
	require.NoError(t, err)

	identityRoutes := 0
	bridgedRoutes := 0
	for _, route := range loadV2PackageRouteCatalog(t) {
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			if route.Owner == "identity-platform" {
				identityRoutes++
				require.True(t, identityAllowlist.Allows(route.Owner, route.RouteID, route.RouteID))
				return
			}
			bridgedRoutes++
			if route.Transport == "websocket" {
				operation, ok := packagebridge.DefaultRouteRegistry().ResolveWebSocket(route.Owner, route.RouteID, route.RouteID)
				require.True(t, ok)
				require.NotNil(t, operation)
				return
			}
			operation, ok := packagebridge.DefaultRouteRegistry().Resolve(route.Owner, route.RouteID, route.RouteID)
			require.True(t, ok)
			require.NotNil(t, operation)
		})
	}
	require.Equal(t, 45, identityRoutes)
	require.Equal(t, 247, bridgedRoutes)
}

func TestV2PackageRouterFixturesAreIsolated(t *testing.T) {
	_, _, _ = setupV2PackageRouter(t)
	_, _, _ = setupV2PackageRouter(t)
}

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(t *testing.T) (*gin.Engine, *config.Config) {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)

	// 迁移必要的表
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}))

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken:         "test-api-token",
			SubscribePath:    "s",
			TrafficLogEnable: true,
		},
	}
	config.Set(cfg)

	r := gin.New()
	Setup(r, cfg)

	return r, cfg
}

func teardownTestRouter(t *testing.T) {
	t.Helper()
	require.NoError(t, database.Close())
}

func TestSetup_HealthEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestSetup_V3KernelAccessGroupsRequireAdminAndPersistScope(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))

	payload, err := json.Marshal(gin.H{"scope_id": "forward", "name": "canary", "enabled": true})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v3/access-groups", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	unauthenticated := httptest.NewRecorder()
	r.ServeHTTP(unauthenticated, request)
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, "/api/v3/access-groups", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	created := httptest.NewRecorder()
	r.ServeHTTP(created, request)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v3/access-groups?scope_id=forward", nil)
	listRequest.Header.Set("Authorization", "Bearer "+token)
	listed := httptest.NewRecorder()
	r.ServeHTTP(listed, listRequest)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), "canary")
}

func TestSetup_V3KernelAccessGroupDetailUsesStaticResolveRouteAndProtectedDetailRoute(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}, &model.Plan{}))

	group := model.AccessGroup{ScopeID: "forward", Name: "detail", Enabled: true}
	user := model.User{Email: "detail@example.com", Token: "detail-token", UUID: "detail-uuid"}
	plan := model.Plan{Name: "Detail plan"}
	require.NoError(t, database.GetDB().Create(&group).Error)
	require.NoError(t, database.GetDB().Create(&user).Error)
	require.NoError(t, database.GetDB().Create(&plan).Error)
	require.NoError(t, database.GetDB().Create(&model.AccessGroupUser{GroupID: group.ID, UserID: user.ID}).Error)
	require.NoError(t, database.GetDB().Create(&model.AccessGroupPlan{GroupID: group.ID, PlanID: plan.ID}).Error)

	request := httptest.NewRequest(http.MethodGet, "/api/v3/access-groups/"+strconv.FormatUint(uint64(group.ID), 10), nil)
	unauthenticated := httptest.NewRecorder()
	r.ServeHTTP(unauthenticated, request)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+token)
	detail := httptest.NewRecorder()
	r.ServeHTTP(detail, request)
	require.Equal(t, http.StatusOK, detail.Code, detail.Body.String())
	require.Contains(t, detail.Body.String(), "detail@example.com")
	require.Contains(t, detail.Body.String(), "Detail plan")

	resolveRequest := httptest.NewRequest(http.MethodGet, "/api/v3/access-groups/resolve?user_id=1&scope_id=forward", nil)
	resolveRequest.Header.Set("Authorization", "Bearer "+token)
	resolved := httptest.NewRecorder()
	r.ServeHTTP(resolved, resolveRequest)
	require.Equal(t, http.StatusOK, resolved.Code, resolved.Body.String())
}

func TestSetup_V3KernelOperationIsIdempotent(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))
	manifestJSON, err := service.CanonicalPluginManifest(service.PluginManifest{
		ID: "subscription", Name: "Subscription", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.PluginRelease{
		PluginID: "subscription", Version: "1.0.0", APIVersion: "v1", ManifestJSON: string(manifestJSON),
		ArtifactSHA256: strings.Repeat("a", 64), Signature: "test",
	}).Error)
	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	payload, err := json.Marshal(gin.H{
		"operation_id": "73c38025-6e13-4af8-a3bc-f3204bbf7cee", "idempotency_key": "router-operation-1",
		"plugin_id": "subscription", "target_version": "1.0.0", "kind": "plugin.health",
		"revision": 1, "config": gin.H{}, "deadline_at": time.Now().Add(time.Minute).UTC(),
	})
	require.NoError(t, err)
	for attempt, expectedStatus := range []int{http.StatusAccepted, http.StatusOK} {
		request := httptest.NewRequest(http.MethodPost, "/api/v3/operations", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		require.Equalf(t, expectedStatus, recorder.Code, "attempt %d: %s", attempt, recorder.Body.String())
	}
}

func TestSetup_V3ExtensionsRequiresAdminAndReturnsEmptyCatalog(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))

	request := httptest.NewRequest(http.MethodGet, "/api/v3/extensions", nil)
	unauthenticated := httptest.NewRecorder()
	r.ServeHTTP(unauthenticated, request)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodGet, "/api/v3/extensions", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"data":[]}`, recorder.Body.String())
}

func TestSetupRegistersV4PluginGateway(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/v4/plugins/knowledge/articles", nil)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
}

func TestV2KnowledgeListUsesPackageEnvelope(t *testing.T) {
	router, cfg, host := setupV2PackageRouter(t)
	defer teardownTestRouter(t)

	response := requestV2(t, router, cfg, http.MethodGet, "/api/v2/user/knowledge")
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"items":[]}}`, response.Body.String())
	require.Equal(t, "knowledge.article.list", host.lastRouteID)
}

func TestV2LoginUsesIdentityPlatformPackagePanelEnvelope(t *testing.T) {
	router, _, host := setupV2PackageRouter(t)
	defer teardownTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v2/login", strings.NewReader(`{"email":"u@example.test","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "identity-login-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"code":0,"msg":"操作成功","ts":1,"data":{"token":"identity-host"}}`, recorder.Body.String())
	require.Equal(t, "identity.auth.login", host.lastRouteID)

	disablePackage(t, "identity-platform")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, requestV2LoginRequest(t))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "package_unavailable")
}

func requestV2LoginRequest(t *testing.T) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/login", strings.NewReader(`{"email":"u@example.test","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestV2GatewayFailsClosedWhenPackageIsDisabled(t *testing.T) {
	router, cfg, _ := setupV2PackageRouter(t)
	defer teardownTestRouter(t)
	disablePackage(t, "knowledge")

	response := requestV2(t, router, cfg, http.MethodGet, "/api/v2/user/knowledge")
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "legacy")
}

func TestWebSocketGatewayDoesNotInvokeLegacyHandler(t *testing.T) {
	router, cfg, _ := setupV2PackageRouter(t)
	defer teardownTestRouter(t)
	disablePackage(t, "knowledge")

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/admin/ws/monitor", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "legacy")
}

func TestSetup_MetricsEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetup_CORS(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// OPTIONS with Origin header — should echo back the origin (default: allow all)
	req, _ := http.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID")
}

func TestSetup_SecurityHeadersAndRequestID(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "0", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, "none", w.Header().Get("X-Permitted-Cross-Domain-Policies"))
	assert.Contains(t, w.Header().Get("Permissions-Policy"), "camera=()")
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", w.Header().Get("Strict-Transport-Security"))
}

func TestSetup_RequestIDHonorsInboundHeader(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "external-trace-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, "external-trace-id", w.Header().Get("X-Request-ID"))
}

func TestSetup_RegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_LoginEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/login", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_NodeRegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/node/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_SubscribeEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// 创建测试用户
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid-1234",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	req, _ := http.NewRequest("GET", "/s/test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return some response, not 404 (route is registered)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_LegacySubscribeEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	user := &model.User{
		Email:          "legacy-subscribe@example.com",
		Token:          "legacy-test-token",
		UUID:           "legacy-test-uuid",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	req, _ := http.NewRequest("GET", "/api/v1/client/subscribe?token=legacy-test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_AdminEndpoints_RequireAuth(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test admin endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/dashboard"},
		{"GET", "/api/v2/admin/users"},
		{"GET", "/api/v2/admin/nodes"},
		{"GET", "/api/v2/admin/orders"},
		{"GET", "/api/v2/admin/plans"},
		{"GET", "/api/v2/admin/agent/monitor"},
		{"GET", "/api/v2/admin/agent/tasks/task-1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_UserEndpoints_RequireAuth(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test user endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/user/profile"},
		{"GET", "/api/v2/user/dashboard"},
		{"GET", "/api/v2/user/subscription"},
		{"GET", "/api/v2/user/plan"},
		{"GET", "/api/v2/user/order"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_UniProxyEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test UniProxy endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/server/UniProxy/config"},
		{"GET", "/api/v2/server/UniProxy/user"},
		{"POST", "/api/v2/server/UniProxy/push"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_PaymentEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Public payment endpoints
	t.Run("payment methods", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v2/payment/methods", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestSetup_AgentEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Agent public endpoints
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v2/agent/register"},
		{"POST", "/api/v2/agent/heartbeat"},
		{"GET", "/api/v2/agent/tasks"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should not return 404
			assert.NotEqual(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestSetup_TelegramWebhook(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/telegram/webhook", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should not return 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_CustomSubscribePath(t *testing.T) {
	cache.InitMemory()
	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, database.Close())
	}()

	// 迁移用户表用于订阅测试
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}, &model.Plan{}))

	// 创建测试用户
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid-1234",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken:      "test-api-token",
			SubscribePath: "custom-sub",
		},
	}

	r := gin.New()
	Setup(r, cfg)

	req, _ := http.NewRequest("GET", "/custom-sub/test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should not return 404 - route is registered
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_AdminForwardEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/forward/ansible-machines"},
		{"GET", "/api/v2/admin/forward/nodes"},
		{"GET", "/api/v2/admin/forward/rules"},
		{"GET", "/api/v2/admin/forward/stats"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminPaymentGatewayEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/payment/gateways"},
		{"GET", "/api/v2/admin/payment/stats"},
		{"GET", "/api/v2/admin/payment/records"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminTelegramEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/telegram/users"},
		{"PUT", "/api/v2/admin/telegram/users/1/notify"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			body := strings.NewReader("{}")
			if ep.method == "GET" {
				body = strings.NewReader("")
			}
			req, _ := http.NewRequest(ep.method, ep.path, body)
			if ep.method == "PUT" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminSystemEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/system/configs"},
		{"GET", "/api/v2/admin/system/backup/config"},
		{"GET", "/api/v2/admin/system/backups"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}
