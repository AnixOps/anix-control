package pluginhostsdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
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

// A native handler that cannot serve a request (here: the kernel sent no
// request address) defers to the legacy handler in native mode and is not
// compared in shadow mode.
func TestRouterAnswersFromLegacyWhenTheNativeHandlerIsUnavailable(t *testing.T) {
	bridge := &routerBridgeFake{}
	bridge.setConfig(map[string]string{"knowledge.admin.knowledge.post": RouteModeNative, "knowledge.article.list": RouteModeShadow}, nil)
	unavailable := func(_ context.Context, request NativeRequest) (NativeResponse, error) {
		if request.Metadata.Scheme == "" {
			return NativeResponse{}, ErrNativeUnavailable
		}
		return NativeResponse{Body: []byte(`{"native":true}`)}, nil
	}
	router := newTestRouter(t, bridge, map[string]NativeHandler{"knowledge.admin.knowledge.post": unavailable, "knowledge.article.list": unavailable})
	router.Refresh(context.Background())

	response, err := dispatch(t, router, "knowledge.admin.knowledge.post", "POST")
	require.NoError(t, err)
	assert.Equal(t, legacyBody, string(response.ResponseBody))
	assert.Equal(t, []string{"knowledge.admin.knowledge.post"}, bridge.invocations())
	route := healthDetails(t, router).Routes["knowledge.admin.knowledge.post"]
	assert.Zero(t, route.Native, "a deferred request is not a native answer")
	assert.Zero(t, route.NativeErrors, "a deferred request is not a native error")

	_, err = dispatch(t, router, "knowledge.article.list", "GET")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return healthDetails(t, router).Routes["knowledge.article.list"].ShadowSkipped == 1 }, time.Second, 5*time.Millisecond)
	shadow := healthDetails(t, router).Routes["knowledge.article.list"]
	assert.Zero(t, shadow.ShadowErrors)
	assert.Zero(t, shadow.ShadowMismatch)
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

// A native handler gets the request binding it passes to KernelNodeOps; a
// shadow run gets none, so the kernel never acts for it or resolves its
// sealed handles. The shadow comparison masks handles on both sides.
func TestRouterBindsNativeRunsButNeverShadowRuns(t *testing.T) {
	handle := v2compat.SealedHandlePrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	bridge := &routerBridgeFake{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"data":{"key":"real-key"},"msg":"ok","ts":1}`)}}
	bridge.setConfig(map[string]string{"proxy.admin.auth_keys.post": RouteModeNative, "proxy.admin.auth_keys.get": RouteModeShadow}, nil)
	var mu sync.Mutex
	bindings := map[string][]byte{}
	record := func(_ context.Context, request NativeRequest) (NativeResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		bindings[request.RouteID] = request.Binding
		return NativeResponse{Body: []byte(`{"code":0,"data":{"key":"` + handle + `"},"msg":"ok","ts":2}`)}, nil
	}
	router := newTestRouter(t, bridge, map[string]NativeHandler{"proxy.admin.auth_keys.post": record, "proxy.admin.auth_keys.get": record})
	router.Refresh(context.Background())
	capability := make([]byte, 32)
	capability[0] = 7

	_, err := router.Dispatch(context.Background(), DispatchRequest{RouteID: "proxy.admin.auth_keys.post", Method: "POST", BridgeCapability: capability})
	require.NoError(t, err)
	mu.Lock()
	assert.Equal(t, capability, bindings["proxy.admin.auth_keys.post"], "a native run is bound to its request")
	mu.Unlock()

	_, err = router.Dispatch(context.Background(), DispatchRequest{RouteID: "proxy.admin.auth_keys.get", Method: "GET", BridgeCapability: capability})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return healthDetails(t, router).Routes["proxy.admin.auth_keys.get"].Shadow == 1 }, time.Second, 5*time.Millisecond)
	mu.Lock()
	binding, ran := bindings["proxy.admin.auth_keys.get"]
	mu.Unlock()
	assert.True(t, ran)
	assert.Nil(t, binding, "a shadow run is never bound")
	assert.Zero(t, healthDetails(t, router).Routes["proxy.admin.auth_keys.get"].ShadowMismatch, "answers that differ only in handles match")
}

func TestRouterReportsSanitizedShadowSamples(t *testing.T) {
	const legacy = `{"code":0,"data":{"email":"alice@example.com","token":"tok-legacy","last_ip":"10.2.3.4","plan":1},"msg":"操作成功","ts":1}`
	bridge := &routerBridgeFake{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(legacy)}}
	bridge.setConfig(map[string]string{"knowledge.user.knowledge.id.get": RouteModeShadow}, nil)
	now := time.Unix(1_800_000_000, 0)
	router, err := NewRouter(RouterConfig{
		PackageID: "knowledge", LeaseID: "lease-1", Bridge: bridge, ShadowConcurrency: 1, Now: func() time.Time { return now },
		Native: map[string]NativeHandler{"knowledge.user.knowledge.id.get": func(context.Context, NativeRequest) (NativeResponse, error) {
			return PanelJSON(v2compat.PanelSuccess(map[string]any{"email": "bob@example.com", "token": "tok-native", "last_ip": "10.2.9.9", "plan": 2}, now))
		}},
	})
	require.NoError(t, err)
	router.Refresh(context.Background())

	_, err = router.Dispatch(context.Background(), DispatchRequest{
		RequestID: "req-42", RouteID: "knowledge.user.knowledge.id.get", Method: "GET", BridgeCapability: make([]byte, 32),
		RequestBody: []byte(`{"password":"hunter2"}`),
		Metadata: RequestMetadata{
			Path: "/api/v2/user/knowledge/17", PathParams: map[string]string{"id": "17"},
			Query: map[string][]string{"token": {"subscribe-secret"}},
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(healthDetails(t, router).ShadowSamples) == 1 }, time.Second, 5*time.Millisecond)
	sample := healthDetails(t, router).ShadowSamples[0]
	assert.Len(t, sample.ID, 32)
	assert.Equal(t, "knowledge.user.knowledge.id.get", sample.RouteID)
	assert.Equal(t, "GET", sample.Method)
	assert.Equal(t, "/api/v2/user/knowledge/{id}?token=***", sample.Path)
	assert.Equal(t, "req-42", sample.RequestID)
	assert.Equal(t, now.Unix(), sample.ObservedAt)
	encoded, err := json.Marshal(sample.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"path":"$.data.email","kind":"changed","legacy":"a***@example.com","native":"b***@example.com"},
		{"path":"$.data.last_ip","kind":"changed","legacy":"10.2.*.*","native":"10.2.*.*"},
		{"path":"$.data.plan","kind":"changed","legacy":1,"native":2},
		{"path":"$.data.token","kind":"changed","legacy":"***","native":"***"}
	]`, string(encoded))
	health, err := router.Health(context.Background())
	require.NoError(t, err)
	for _, secret := range []string{"hunter2", "tok-legacy", "tok-native", "subscribe-secret", "alice", "10.2.3.4"} {
		assert.NotContains(t, health.DetailsJSON, secret)
	}

	// Samples age out of the details, and at most maxShadowSamples are kept.
	now = now.Add(shadowSampleMaxAge + time.Second)
	assert.Empty(t, healthDetails(t, router).ShadowSamples)
	for index := 0; index < maxShadowSamples+5; index++ {
		router.recordSample(DispatchRequest{RouteID: "knowledge.user.knowledge.id.get", Method: "GET"}, DispatchResponse{StatusCode: 200}, NativeResponse{StatusCode: 500}, now.Unix())
	}
	assert.Len(t, healthDetails(t, router).ShadowSamples, maxShadowSamples)
}
