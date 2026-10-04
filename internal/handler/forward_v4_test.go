package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardV4ControlPath(t *testing.T) {
	for in, want := range map[string]string{
		"/api/v4/forward":             "/api/v4/plugins/forward",
		"/api/v4/forward/routes":      "/api/v4/plugins/forward/routes",
		"/api/v4/forward/routes/x/go": "/api/v4/plugins/forward/routes/x/go",
	} {
		got, ok := forwardV4ControlPath(in)
		require.True(t, ok, in)
		require.Equal(t, want, got)
	}
	for _, in := range []string{"/api/v4/forwarding", "/api/v4/plugins/forward/routes"} {
		_, ok := forwardV4ControlPath(in)
		require.False(t, ok, in)
	}
}

// installForwardPackage registers a signed forward release with its v4
// control route and a healthy control installation.
func installForwardPackage(t *testing.T, edition string) (*KernelHandler, *capturingPluginHostManager) {
	t.Helper()
	db := newKernelHandlerTestDB(t, &model.User{})
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{
		App: config.AppConfig{Edition: edition},
		Plugins: config.PluginConfig{
			OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), ControlExecutionEnabled: true, ControlHostRequestTimeout: "2s",
		},
	})
	t.Cleanup(func() { config.Set(previousConfig) })
	artifact := []byte("forward-control-artifact")
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "forward", Name: "Forward", Version: "4.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
		Permissions:   []string{"forward.view", service.PluginAPIPermission("forward")},
		ControlRoutes: []string{"/api/v4/plugins/forward/*"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: "forward", Target: "control", DesiredVersion: "4.0.0", ObservedVersion: "4.0.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 3,
	}).Error)
	// User 7 is a super administrator, user 8 a staff administrator.
	require.NoError(t, db.Create(&model.User{ID: 7, Email: "root@example.test", Password: "x", Token: "t7", UUID: "u7", IsAdmin: 1}).Error)
	require.NoError(t, db.Create(&model.User{ID: 8, Email: "staff@example.test", Password: "x", Token: "t8", UUID: "u8", IsAdmin: 1, IsStaff: 1}).Error)
	hosts := &capturingPluginHostManager{output: pluginhost.DispatchOutput{
		StatusCode: 200, Body: []byte(`{"data":{}}`), Headers: []pluginhost.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}}
	return &KernelHandler{db: db, controlPluginHosts: hosts}, hosts
}

func asAdmin(userID uint) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("is_admin", true)
	}
}

// The forward package's v4 API reaches the package as its control route,
// with the canonical path; DELETE needs a super administrator.
func TestForwardGatewayDispatchesTheControlRoute(t *testing.T) {
	kernel, hosts := installForwardPackage(t, config.EditionCommunity)
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/forward/routes?owner=admin", "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(8))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, service.PluginControlBridgeRouteID("forward", "/api/v4/plugins/forward/*"), hosts.input.RouteID)
	require.Equal(t, "/api/v4/plugins/forward/routes", hosts.input.Metadata.Path)
	require.Equal(t, []string{"admin"}, hosts.input.Metadata.Query["owner"])
	require.EqualValues(t, 3, hosts.input.Generation)

	hosts.input = pluginhost.DispatchInput{}
	recorder = performKernelHandlerRequestWithSetup(t, http.MethodDelete, "/api/v4/forward/routes/01J", "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(8))
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "super_admin_required")
	require.Empty(t, hosts.input.RouteID, "a refused delete never reaches the package")

	recorder = performKernelHandlerRequestWithSetup(t, http.MethodDelete, "/api/v4/forward/nodes/forward-3", "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(7))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, http.MethodDelete, hosts.input.Method)
	require.Equal(t, "/api/v4/plugins/forward/nodes/forward-3", hosts.input.Metadata.Path)
}

// The community edition answers the commercial prefixes of
// config/editions.json as routes that do not exist; the commercial one
// hands them to the package.
func TestForwardGatewayHidesCommercialPathsInCommunity(t *testing.T) {
	kernel, hosts := installForwardPackage(t, config.EditionCommunity)
	for _, path := range []string{"/api/v4/forward/self/routes", "/api/v4/forward/plans", "/api/v4/forward/multipliers/1"} {
		recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, path, "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(7))
		require.Equal(t, http.StatusNotFound, recorder.Code, path)
		require.Contains(t, recorder.Body.String(), "plugin_route_not_found")
	}
	require.Empty(t, hosts.input.RouteID)

	commercial, commercialHosts := installForwardPackage(t, config.EditionCommercial)
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/forward/self/routes", "", "/api/v4/forward/*route", commercial.ForwardGateway, asAdmin(7))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "/api/v4/plugins/forward/self/routes", commercialHosts.input.Metadata.Path)
}

// The list answers learn the super administrator rule through the
// principal (F5b D7): super_admin is true for a super administrator, false
// for staff, and absent on every other request.
func TestForwardGatewayTellsTheListsWhetherTheCallerMayDelete(t *testing.T) {
	kernel, hosts := installForwardPackage(t, config.EditionCommunity)
	principal := func() map[string]any {
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(hosts.input.PrincipalJSON, &decoded))
		return decoded
	}
	for _, path := range []string{"/api/v4/forward/routes", "/api/v4/forward/nodes?kind=proxy", "/api/v4/forward/ansible-machines"} {
		recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, path, "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(7))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, true, principal()["super_admin"], path)
		recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, path, "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(8))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, false, principal()["super_admin"], path)
	}
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/forward/routes/01J", "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(7))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, principal(), "super_admin")
	recorder = performKernelHandlerRequestWithSetup(t, http.MethodPost, "/api/v4/forward/routes", `{}`, "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(7))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, principal(), "super_admin")
}

// DNS provider writes carry credentials: they need a super administrator,
// while reads and binding writes stay with every administrator (L2).
func TestForwardGatewayDNSProviderWritesNeedSuperAdmin(t *testing.T) {
	kernel, hosts := installForwardPackage(t, config.EditionCommunity)
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/api/v4/forward/dns/providers"},
		{http.MethodPut, "/api/v4/forward/dns/providers/3"},
		{http.MethodDelete, "/api/v4/forward/dns/providers/3"},
		{http.MethodDelete, "/api/v4/forward/dns/bindings/3"},
	} {
		hosts.input = pluginhost.DispatchInput{}
		recorder := performKernelHandlerRequestWithSetup(t, request.method, request.path, `{}`, "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(8))
		require.Equal(t, http.StatusForbidden, recorder.Code, request.path)
		require.Contains(t, recorder.Body.String(), "super_admin_required")
		require.Empty(t, hosts.input.RouteID, "a refused write never reaches the package")
	}
	for _, request := range []struct {
		method, path string
		user         uint
	}{
		{http.MethodGet, "/api/v4/forward/dns/providers", 8},
		{http.MethodPost, "/api/v4/forward/dns/bindings", 8},
		{http.MethodGet, "/api/v4/forward/routes/01J/dns", 8},
		{http.MethodPost, "/api/v4/forward/dns/providers", 7},
		{http.MethodPut, "/api/v4/forward/dns/providers/3", 7},
	} {
		recorder := performKernelHandlerRequestWithSetup(t, request.method, request.path, `{}`, "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(request.user))
		require.Equal(t, http.StatusOK, recorder.Code, request.path)
	}
	require.False(t, forwardV4NeedsSuperAdmin(http.MethodPost, "/api/v4/forward/dns/providersx"))
	require.True(t, forwardV4NeedsSuperAdmin(http.MethodPatch, "/api/v4/forward/dns/providers/1"))
}

// The package's own spelling, /api/v4/plugins/forward/*, keeps the kernel's
// forward checks: super administrator writes and the hidden commercial
// prefixes cannot be reached around ForwardGateway.
func TestPluginRouteGatewayKeepsTheForwardChecks(t *testing.T) {
	kernel, hosts := installForwardPackage(t, config.EditionCommunity)
	const route = "/api/v4/plugins/:plugin_id/*route"
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/api/v4/plugins/forward/dns/providers"},
		{http.MethodPut, "/api/v4/plugins/forward/dns/providers/3"},
		{http.MethodDelete, "/api/v4/plugins/forward/routes/01J"},
	} {
		hosts.input = pluginhost.DispatchInput{}
		recorder := performKernelHandlerRequestWithSetup(t, request.method, request.path, `{}`, route, kernel.PluginRouteGateway, asAdmin(8))
		require.Equal(t, http.StatusForbidden, recorder.Code, request.path)
		require.Empty(t, hosts.input.RouteID, "a refused write never reaches the package")
	}
	recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/plugins/forward/self/routes", "", route, kernel.PluginRouteGateway, asAdmin(7))
	require.Equal(t, http.StatusNotFound, recorder.Code)
	for _, user := range []uint{7, 8} {
		recorder = performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/plugins/forward/dns/providers", "", route, kernel.PluginRouteGateway, asAdmin(user))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}
	recorder = performKernelHandlerRequestWithSetup(t, http.MethodPost, "/api/v4/plugins/forward/dns/providers", `{}`, route, kernel.PluginRouteGateway, asAdmin(7))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "/api/v4/plugins/forward/dns/providers", hosts.input.Metadata.Path)
}
