package pluginhostsdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/v2compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routerBridgeFake struct {
	mu        sync.Mutex
	config    packagebridgesdk.PackageConfig
	configErr error
	response  packagebridgesdk.Response
	invoked   []string
}

func (b *routerBridgeFake) Invoke(_ context.Context, _ []byte, operation string, _ []byte) (packagebridgesdk.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.invoked = append(b.invoked, operation)
	return b.response, nil
}

func (b *routerBridgeFake) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.config, b.configErr
}

func (b *routerBridgeFake) setConfig(modes map[string]string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config, b.configErr = packagebridgesdk.PackageConfig{Revision: 2, ConfigHash: "hash", RouteModes: modes}, err
}

func (b *routerBridgeFake) invocations() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.invoked...)
}

const legacyBody = `{"code":0,"data":[{"id":1}],"msg":"操作成功","ts":1}`

func newTestRouter(t *testing.T, bridge *routerBridgeFake, native map[string]NativeHandler) *Router {
	t.Helper()
	if bridge.response.StatusCode == 0 {
		bridge.response = packagebridgesdk.Response{StatusCode: 200, Body: []byte(legacyBody)}
	}
	router, err := NewRouter(RouterConfig{PackageID: "knowledge", LeaseID: "lease-1", Bridge: bridge, Native: native, ShadowConcurrency: 1})
	require.NoError(t, err)
	return router
}

func dispatch(t *testing.T, router *Router, route, method string) (DispatchResponse, error) {
	t.Helper()
	return router.Dispatch(context.Background(), DispatchRequest{
		RouteID: route, Method: method, BridgeCapability: make([]byte, 32),
		PrincipalJSON: []byte(`{"actor_id":42,"admin":true,"package_id":"knowledge"}`),
		Metadata:      RequestMetadata{PathParams: map[string]string{"id": "1"}},
	})
}

func healthDetails(t *testing.T, router *Router) routerHealthDetails {
	t.Helper()
	health, err := router.Health(context.Background())
	require.NoError(t, err)
	require.True(t, health.Healthy)
	var details routerHealthDetails
	require.NoError(t, json.Unmarshal([]byte(health.DetailsJSON), &details))
	return details
}

func matchingNative(context.Context, NativeRequest) (NativeResponse, error) {
	return PanelJSON(v2compat.PanelSuccess([]map[string]int{{"id": 1}}, time.Now()))
}

func TestRouterDefaultsToLegacyUntilConfigured(t *testing.T) {
	bridge := &routerBridgeFake{configErr: packagebridgesdk.ErrSessionOperationUnsupported}
	nativeCalls := 0
	router := newTestRouter(t, bridge, map[string]NativeHandler{"knowledge.article.list": func(context.Context, NativeRequest) (NativeResponse, error) {
		nativeCalls++
		return NativeResponse{}, nil
	}})

	response, err := dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	assert.Equal(t, legacyBody, string(response.ResponseBody))

	router.Refresh(context.Background())
	_, err = dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	assert.Zero(t, nativeCalls, "an older kernel without session operations keeps every route legacy")
	assert.Equal(t, "unsupported", healthDetails(t, router).Config.Status)
	assert.Equal(t, []string{"knowledge.article.list", "knowledge.article.list"}, bridge.invocations())
}

func TestRouterServesNativeRoutesWithTheKernelPrincipal(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.article.list": RouteModeNative}, nil)
	var seen NativeRequest
	router := newTestRouter(t, bridge, map[string]NativeHandler{"knowledge.article.list": func(_ context.Context, request NativeRequest) (NativeResponse, error) {
		seen = request
		return NativeResponse{Body: []byte(`{"native":true}`)}, nil
	}})
	router.Refresh(context.Background())

	response, err := dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	assert.EqualValues(t, 200, response.StatusCode, "a zero status defaults to 200")
	assert.Equal(t, `{"native":true}`, string(response.ResponseBody))
	assert.Equal(t, Principal{ActorID: 42, Admin: true, PackageID: "knowledge"}, seen.Principal)
	assert.Equal(t, "1", seen.Metadata.PathParams["id"])
	assert.Empty(t, bridge.invocations(), "native routes never reach the legacy handler")
	details := healthDetails(t, router)
	assert.Equal(t, "ok", details.Config.Status)
	assert.Equal(t, int64(2), details.Config.Revision)
	assert.EqualValues(t, 1, details.Routes["knowledge.article.list"].Native)
}

func TestRouterFallsBackToLegacyWithoutANativeImplementation(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.admin.knowledge.post": RouteModeNative}, nil)
	router := newTestRouter(t, bridge, nil)
	router.Refresh(context.Background())

	_, err := dispatch(t, router, "knowledge.admin.knowledge.post", "POST")
	require.NoError(t, err)
	assert.Equal(t, []string{"knowledge.admin.knowledge.post"}, bridge.invocations())
	route := healthDetails(t, router).Routes["knowledge.admin.knowledge.post"]
	assert.Equal(t, RouteModeNative, route.Mode)
	assert.Equal(t, RouteModeLegacy, route.Effective)
	assert.True(t, route.Unsupported)
}

func TestRouterShadowAnswersFromLegacyAndCountsMismatches(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.article.list": RouteModeShadow, "knowledge.user.knowledge.id.get": RouteModeShadow}, nil)
	router := newTestRouter(t, bridge, map[string]NativeHandler{
		"knowledge.article.list": matchingNative,
		"knowledge.user.knowledge.id.get": func(context.Context, NativeRequest) (NativeResponse, error) {
			return PanelJSON(v2compat.PanelSuccess(nil, time.Now()))
		},
	})
	router.Refresh(context.Background())

	response, err := dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	assert.Equal(t, legacyBody, string(response.ResponseBody), "shadow always answers from legacy")
	require.Eventually(t, func() bool { return healthDetails(t, router).Routes["knowledge.article.list"].Shadow == 1 }, time.Second, 5*time.Millisecond)
	assert.Zero(t, healthDetails(t, router).Routes["knowledge.article.list"].ShadowMismatch, "ts differences are ignored")

	_, err = dispatch(t, router, "knowledge.user.knowledge.id.get", "GET")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return healthDetails(t, router).Routes["knowledge.user.knowledge.id.get"].ShadowMismatch == 1
	}, time.Second, 5*time.Millisecond)
	assert.NotZero(t, healthDetails(t, router).Routes["knowledge.user.knowledge.id.get"].LastMismatch)
}

func TestRouterShadowSkipsWritesAndBusySlots(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.article.list": RouteModeShadow, "knowledge.admin.knowledge.post": RouteModeShadow}, nil)
	release := make(chan struct{})
	nativeCalls := 0
	router := newTestRouter(t, bridge, map[string]NativeHandler{
		"knowledge.article.list": func(ctx context.Context, request NativeRequest) (NativeResponse, error) {
			<-release
			return matchingNative(ctx, request)
		},
		"knowledge.admin.knowledge.post": func(context.Context, NativeRequest) (NativeResponse, error) {
			nativeCalls++
			return NativeResponse{}, nil
		},
	})
	router.Refresh(context.Background())

	_, err := dispatch(t, router, "knowledge.admin.knowledge.post", "POST")
	require.NoError(t, err)
	_, err = dispatch(t, router, "knowledge.article.list", "GET") // occupies the only slot
	require.NoError(t, err)
	_, err = dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	close(release)
	require.Eventually(t, func() bool { return healthDetails(t, router).Routes["knowledge.article.list"].Shadow == 1 }, time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 1, healthDetails(t, router).Routes["knowledge.article.list"].ShadowSkipped)
	assert.Zero(t, nativeCalls, "shadow never runs a write")
}

func TestRouterRecoversNativeAndShadowPanics(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.article.list": RouteModeNative, "knowledge.user.knowledge.id.get": RouteModeShadow}, nil)
	panics := func(context.Context, NativeRequest) (NativeResponse, error) { panic("boom") }
	router := newTestRouter(t, bridge, map[string]NativeHandler{"knowledge.article.list": panics, "knowledge.user.knowledge.id.get": panics})
	router.Refresh(context.Background())

	_, err := dispatch(t, router, "knowledge.article.list", "GET")
	require.ErrorIs(t, err, ErrNativeRoutePanicked)
	_, err = dispatch(t, router, "knowledge.user.knowledge.id.get", "GET")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return healthDetails(t, router).Routes["knowledge.user.knowledge.id.get"].ShadowErrors == 1
	}, time.Second, 5*time.Millisecond)
}

func TestRouterKeepsTheLastModesWhenPollingFails(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.article.list": RouteModeNative}, nil)
	router := newTestRouter(t, bridge, map[string]NativeHandler{"knowledge.article.list": matchingNative})
	router.Refresh(context.Background())

	bridge.setConfig(nil, errors.New("bridge unavailable"))
	router.Refresh(context.Background())
	_, effective := router.Mode("knowledge.article.list")
	assert.Equal(t, RouteModeNative, effective)
	assert.Equal(t, "error", healthDetails(t, router).Config.Status)

	bridge.setConfig(nil, packagebridgesdk.ErrHostFenced)
	router.Refresh(context.Background())
	_, effective = router.Mode("knowledge.article.list")
	assert.Equal(t, RouteModeNative, effective)
	assert.Equal(t, "fenced", healthDetails(t, router).Config.Status)

	bridge.setConfig(map[string]string{}, nil)
	router.Refresh(context.Background())
	_, effective = router.Mode("knowledge.article.list")
	assert.Equal(t, RouteModeLegacy, effective, "switching back to legacy takes effect on the next poll")
}

func TestRouterAllowlistAndMigrationMapping(t *testing.T) {
	bridge := &routerBridgeFake{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"checkpoint":"c","validation_digest":"d","complete":true}`)}}
	router, err := NewRouter(RouterConfig{
		PackageID: "identity-platform", Bridge: bridge,
		AllowRoute:         func(route string) bool { return route == "identity.auth.login" },
		MigrationOperation: func(id string) (string, bool) { return "migration.identity-platform.001", id == "001" },
	})
	require.NoError(t, err)

	_, err = dispatch(t, router, "identity.admin.users.list", "GET")
	require.Error(t, err)
	_, err = router.Migrate(context.Background(), MigrationRequest{MigrationID: "002", BridgeCapability: make([]byte, 32)})
	require.Error(t, err)
	migration, err := router.Migrate(context.Background(), MigrationRequest{MigrationID: "001", BridgeCapability: make([]byte, 32)})
	require.NoError(t, err)
	assert.True(t, migration.Complete)
	assert.Equal(t, []string{"migration.identity-platform.001"}, bridge.invocations())

	generic := newTestRouter(t, &routerBridgeFake{response: bridge.response}, nil)
	_, err = generic.Migrate(context.Background(), MigrationRequest{MigrationID: "001_x", BridgeCapability: make([]byte, 32)})
	require.NoError(t, err)
}

func TestNilRouterIsUnavailable(t *testing.T) {
	var router *Router
	health, err := router.Health(context.Background())
	require.NoError(t, err)
	assert.False(t, health.Healthy)
	_, err = router.Dispatch(context.Background(), DispatchRequest{RouteID: "x"})
	require.Error(t, err)
}
