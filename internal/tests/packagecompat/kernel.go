package packagecompat

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	configtables "github.com/AnixOps/anix-control/v4/config"
	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// KernelRoute is a route whose native side runs the whole path a
// deployment runs: the kernel's v2 gateway (which seals the node secrets of
// the routes in config/node-secret-fields.json into handles and expands the
// handles an answer shows), a real package bridge session serving
// KernelNodeOps, the SDK router with the route in native mode, and the
// kernel's KernelNodeOps engine with its executors on the native database
// (docs/architecture/node-ops-service.md). The package host never sees a
// node secret, as in a deployment.
type KernelRoute struct {
	Route
	// PackageID is the route's package.
	PackageID string
	// Capabilities are the KernelNodeOps capabilities the package's signed
	// release declares (kernel.nodeops.<family>.v1).
	Capabilities []string
	// NativeService builds the package's native handler on the native
	// database, with the host's KernelNodeOps client.
	NativeService func(db *gorm.DB, nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler
	// Agents are the agent sessions the kernel's executors and session RPCs
	// read; nil reads none.
	Agents kernelnodeops.SourcesFunc
}

// RunKernelRead is RunRead for a KernelRoute. Unless the case expects the
// route to fall back (Case.Fallback), the native handler must answer
// itself: an answer the host relayed to the kernel's legacy handler would
// match by construction.
func RunKernelRead(t *testing.T, route KernelRoute, c Case) {
	t.Helper()
	forEachBackend(t, c.Name, func(t *testing.T, open opener) {
		legacy, native := runKernel(t, open, route, c)
		requireSame(t, c, legacy, native)
	})
}

// RunKernelWrite is RunWrite for a KernelRoute.
func RunKernelWrite(t *testing.T, route KernelRoute, c Case) {
	t.Helper()
	require.NotNil(t, c.Snapshot, "RunKernelWrite needs Case.Snapshot")
	forEachBackend(t, c.Name, func(t *testing.T, open opener) {
		legacy, native := runKernel(t, open, route, c)
		requireSame(t, c, legacy, native)
		legacyState, err := json.Marshal(legacy.State)
		require.NoError(t, err)
		nativeState, err := json.Marshal(native.State)
		require.NoError(t, err)
		require.JSONEq(t, string(legacyState), string(nativeState), "database state after the request differs")
	})
}

func runKernel(t *testing.T, open opener, route KernelRoute, c Case) (Result, Result) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	legacyConfig, _, migrated := open(t, "legacy", route.Models)
	database.Reset()
	require.NoError(t, database.Init(legacyConfig))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
	})
	legacyDB := database.GetDB()
	prepare(t, legacyDB, route.Route, c, migrated)
	legacy := serve(t, route.Method, route.Pattern, c, func(ctx *gin.Context) {
		ctx.Set("user_id", c.Principal.ActorID)
		ctx.Set("is_admin", c.Principal.Admin)
		route.Legacy(ctx)
	})
	if c.Snapshot != nil {
		legacy.State = c.Snapshot(t, legacyDB)
	}

	// The kernel's handlers and services read the global database: on the
	// native side it is the native database.
	nativeConfig, nativeDB, migrated := open(t, "native", route.Models)
	_ = database.Close()
	database.Reset()
	require.NoError(t, database.Init(nativeConfig))
	prepare(t, nativeDB, route.Route, c, migrated)
	kernel := StartKernel(t, KernelOptions{
		DB: nativeDB, PackageID: route.PackageID, Capabilities: route.Capabilities, Agents: route.Agents,
		Routes: []KernelHostRoute{{Method: route.Method, Pattern: route.Pattern, RouteID: route.RouteID, Legacy: route.Legacy,
			Native: func(nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
				return route.NativeService(nativeDB, nodeOps)
			}}},
	})
	native := kernel.Serve(t, route.Method, route.Pattern, c)
	if c.Fallback {
		require.Positive(t, kernel.LegacyCalls(route.RouteID), "the host was to answer from the legacy handler")
	} else {
		require.Zero(t, kernel.LegacyCalls(route.RouteID), "the native handler answered from the legacy handler: %s", native.Body)
	}
	if c.Snapshot != nil {
		native.State = c.Snapshot(t, nativeDB)
	}
	return legacy, native
}

// KernelHostRoute is one route of a KernelOptions host.
type KernelHostRoute struct {
	Method, Pattern, RouteID string
	// Legacy is the kernel's handler, which the host relays to.
	Legacy gin.HandlerFunc
	// Native builds the package's handler with the host's KernelNodeOps
	// client.
	Native func(nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler
}

// KernelOptions configure StartKernel.
type KernelOptions struct {
	// DB is the kernel database the engine records in.
	DB        *gorm.DB
	PackageID string
	// Capabilities are the KernelNodeOps capabilities the package holds.
	Capabilities []string
	// Agents are the agent sessions the executors and session RPCs read.
	Agents kernelnodeops.SourcesFunc
	// Routes are the package's routes, each in native mode.
	Routes []KernelHostRoute
}

// Kernel is the native side of a KernelRoute: the gateway, one package
// host and the KernelNodeOps engine.
type Kernel struct {
	gateway *compatv2.Gateway
	store   *sealedsecrets.Store
	session *packagebridge.Session
	router  *pluginhostsdk.Router
	routes  []KernelHostRoute

	mu       sync.Mutex
	legacy   map[string]int
	received map[string][]pluginhostsdk.NativeRequest
}

// StartKernel starts the kernel's side of a package: a KernelNodeOps engine
// on opts.DB with the executors a kernel registers (agent ones on
// opts.Agents), the gateway with the kernel's node secret fields, and the
// package host on a real bridge session, every route native.
func StartKernel(t testing.TB, opts KernelOptions) *Kernel {
	t.Helper()
	k := &Kernel{store: sealedsecrets.NewStore(nil), routes: opts.Routes, legacy: map[string]int{}, received: map[string][]pluginhostsdk.NativeRequest{}}
	registry := kernelnodeops.NewRegistry()
	for _, register := range []func(*kernelnodeops.Registry) error{
		(&kernelnodeops.Credentials{}).Register, (&kernelnodeops.Retirements{}).Register, (&kernelnodeops.SecretDocuments{}).Register,
		(&kernelnodeops.Diagnosis{}).Register, (&kernelnodeops.ConnectionTest{}).Register,
	} {
		require.NoError(t, register(registry))
	}
	agents := opts.Agents
	if agents == nil {
		agents = func() kernelnodeops.AgentSources { return kernelnodeops.AgentSources{} }
	}
	require.NoError(t, kernelnodeops.RegisterNodeOperationExecutors(registry, agents))
	engine := &kernelnodeops.Engine{DB: opts.DB, Executors: registry, PollInterval: 10 * time.Millisecond, Secrets: k.store}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		engine.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	sources := agents()
	server := &kernelnodeops.Server{Engine: engine, Authorizer: declaredGrants{packageID: opts.PackageID, capabilities: opts.Capabilities}, Agents: &sources}

	table, err := sealedsecrets.ParseTable(configtables.NodeSecretFields)
	require.NoError(t, err)
	k.gateway = &compatv2.Gateway{
		Registry: compatv2.NewRegistry(kernelRoutes{k: k, packageID: opts.PackageID}), Dispatcher: kernelDispatcher{k: k},
		Metrics: compatv2.NewGatewayMetrics(), Sealer: sealedsecrets.NewSealer(table, nil, k.store),
	}

	operations := make([]packagebridge.Operation, 0, len(opts.Routes))
	modes := map[string]string{}
	for _, route := range opts.Routes {
		routeID, legacy := route.RouteID, route.Legacy
		operations = append(operations, packagebridge.Operation{PackageID: opts.PackageID, RouteID: routeID, Name: routeID,
			Handler: packagebridge.NewHTTPAdapter(func(c *gin.Context) {
				k.mu.Lock()
				k.legacy[routeID]++
				k.mu.Unlock()
				legacy(c)
			})})
		modes[routeID] = pluginhostsdk.RouteModeNative
	}
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, operations...)
	require.NoError(t, err)
	identity := packagebridge.HostIdentity{PackageID: opts.PackageID, Version: "4.2.0", Generation: 5}
	session, child, err := packagebridge.NewSessionWithOptions(identity, allowlist, packagebridge.SessionOptions{
		HostOperations: kernelHostOperations{modes: modes}, KernelNodeOps: server.For,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	k.session = session
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	nodeOps := kernelnodeopsv1.NewKernelNodeOpsClient(client.Conn())
	native := map[string]pluginhostsdk.NativeHandler{}
	for _, route := range opts.Routes {
		routeID, handler := route.RouteID, route.Native(nodeOps)
		require.NotNil(t, handler, "%s has no native handler", routeID)
		native[routeID] = func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
			k.mu.Lock()
			k.received[routeID] = append(k.received[routeID], request)
			k.mu.Unlock()
			return handler(ctx, request)
		}
	}
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{PackageID: opts.PackageID, LeaseID: "lease", Bridge: client, Native: native})
	require.NoError(t, err)
	router.Refresh(context.Background())
	k.router = router
	return k
}

// Serve sends c through the gateway, as the principal of c.
func (k *Kernel) Serve(t *testing.T, method, pattern string, c Case) Result {
	t.Helper()
	return serve(t, method, pattern, c, func(ctx *gin.Context) {
		ctx.Set("user_id", c.Principal.ActorID)
		ctx.Set("is_admin", c.Principal.Admin)
		k.gateway.Serve(ctx)
	})
}

// LegacyCalls counts the requests of a route the host relayed to the
// kernel's legacy handler.
func (k *Kernel) LegacyCalls(routeID string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.legacy[routeID]
}

// Received returns what the package's native handler of a route was sent.
func (k *Kernel) Received(routeID string) []pluginhostsdk.NativeRequest {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]pluginhostsdk.NativeRequest(nil), k.received[routeID]...)
}

// PendingHandles counts the sealed handles still held: none once every
// request ended.
func (k *Kernel) PendingHandles() int { return k.store.Pending() }

// declaredGrants authorizes what a package's signed release declares.
type declaredGrants struct {
	packageID    string
	capabilities []string
}

func (g declaredGrants) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host.PackageID != g.packageID {
		return service.ErrCapabilityNotAuthorized
	}
	for _, granted := range g.capabilities {
		if granted == capability {
			return nil
		}
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelHostOperations serves the host its route modes; storage is the
// native service's own connection.
type kernelHostOperations struct{ modes map[string]string }

func (o kernelHostOperations) PackageConfig(context.Context, packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	return packagebridge.PackageConfig{Revision: 1, ConfigHash: "h", RouteModes: o.modes}, nil
}

func (kernelHostOperations) LeaseStorage(context.Context, packagebridge.HostIdentity) (packagebridge.StorageLease, error) {
	return packagebridge.StorageLease{}, packagebridge.ErrStorageUnavailable
}

// kernelRoutes resolves the host's routes as the verified source does.
type kernelRoutes struct {
	k         *Kernel
	packageID string
}

func (s kernelRoutes) ResolveV2Route(_ context.Context, method, path string) (compatv2.Route, error) {
	for _, route := range s.k.routes {
		if route.Method == method && patternMatches(route.Pattern, path) {
			return compatv2.Route{Method: method, LegacyPath: route.Pattern, PackageID: s.packageID, Version: "4.2.0", Generation: 5,
				PackageRoute: route.RouteID, Envelope: compatv2.EnvelopeRaw}, nil
		}
	}
	return compatv2.Route{}, compatv2.ErrRouteNotDeclared
}

func patternMatches(pattern, path string) bool {
	patternParts, pathParts := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for index, part := range patternParts {
		if !strings.HasPrefix(part, ":") && part != pathParts[index] {
			return false
		}
	}
	return true
}

// kernelDispatcher dispatches like pluginhost.Supervisor: it mints the
// request's bridge capability on the host's session and runs the host's
// router.
type kernelDispatcher struct{ k *Kernel }

func (d kernelDispatcher) Dispatch(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	capability, err := d.k.session.Mint(request)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	defer d.k.session.Revoke(capability)
	scheme := "http"
	if input.Metadata.TLS {
		scheme = "https"
	}
	response, err := d.k.router.Dispatch(ctx, pluginhostsdk.DispatchRequest{
		PackageID: input.PackageID, RouteID: input.RouteID, Method: input.Method, RequestID: input.RequestID,
		RequestBody: input.Body, PrincipalJSON: input.PrincipalJSON, BridgeCapability: capability,
		Metadata: pluginhostsdk.RequestMetadata{
			Path: input.Metadata.Path, Query: input.Metadata.Query, Headers: input.Metadata.Headers, PathParams: input.Metadata.PathParams,
			ClientIP: input.Metadata.ClientIP, UserAgent: input.Metadata.UserAgent, Scheme: scheme, Host: input.Metadata.Host,
		},
	})
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	headers := make([]pluginhost.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = pluginhost.Header{Name: header.Name, Value: header.Value}
	}
	return pluginhost.DispatchOutput{StatusCode: response.StatusCode, Body: response.ResponseBody, Headers: headers}, nil
}

func (d kernelDispatcher) DispatchLegacy(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	d.k.mu.Lock()
	d.k.legacy[input.RouteID]++
	d.k.mu.Unlock()
	response, err := d.k.session.ServeLegacy(ctx, request, input.RouteID)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	return pluginhost.DispatchOutput{StatusCode: response.StatusCode, Body: response.Body}, nil
}

// Do sends one request through the gateway, without a Case: method, path,
// body and request headers, as the administrator (actor 1).
func (k *Kernel) Do(t testing.TB, method, path string, body []byte, headers map[string]string) Result {
	t.Helper()
	engine := gin.New()
	for _, route := range k.routes {
		engine.Handle(route.Method, route.Pattern, func(ctx *gin.Context) {
			ctx.Set("user_id", uint(1))
			ctx.Set("is_admin", true)
			k.gateway.Serve(ctx)
		})
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewReader(body))
	request.RemoteAddr = clientIP + ":1234"
	request.TLS = (*tls.ConnectionState)(nil)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	answer, _ := io.ReadAll(recorder.Result().Body)
	return Result{StatusCode: recorder.Code, Body: answer, Header: recorder.Header()}
}

// ForEachKernelDatabase runs body on a fresh kernel database with models
// migrated, on SQLite and, with ANIX_TEST_POSTGRES_DSN set, on a throwaway
// PostgreSQL schema; the database is also the kernel's global one, which
// the legacy handlers and the kernel's services read. A test whose legacy
// and native sides must share live state (agent sessions the kernel holds
// in memory) runs both on it.
func ForEachKernelDatabase(t *testing.T, models []any, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	run := func(t *testing.T, cfg *config.DatabaseConfig, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(models...))
		database.Reset()
		require.NoError(t, database.Init(cfg))
		t.Cleanup(func() {
			_ = database.Close()
			database.Reset()
		})
		body(t, db)
	}
	t.Run("sqlite", func(t *testing.T) {
		cfg, db, err := createSQLite(t.TempDir(), "kernel")
		require.NoError(t, err)
		closeOnCleanup(t, db)
		run(t, cfg, db)
	})
	if strings.TrimSpace(os.Getenv(PostgresDSNEnvironment)) == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		cfg, db, drop, err := createPostgresSchema("kernel")
		if errors.Is(err, errUnsafePostgres) {
			t.Skip(err.Error())
		}
		require.NoError(t, err)
		t.Cleanup(drop)
		run(t, cfg, db)
	})
}

// ServeLegacy sends c to the kernel's legacy handler, as the principal of
// c, for a test that runs both sides on one database
// (ForEachKernelDatabase).
func ServeLegacy(t *testing.T, method, pattern string, c Case, legacy gin.HandlerFunc) Result {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return serve(t, method, pattern, c, func(ctx *gin.Context) {
		ctx.Set("user_id", c.Principal.ActorID)
		ctx.Set("is_admin", c.Principal.Admin)
		legacy(ctx)
	})
}

// RequireSame requires two answers to be the same, as RunRead does: the
// headers c lists sent by both, the status and the normalized body equal
// once c's masked paths are masked.
func RequireSame(t *testing.T, c Case, legacy, native Result) {
	t.Helper()
	requireSame(t, c, legacy, native)
}
