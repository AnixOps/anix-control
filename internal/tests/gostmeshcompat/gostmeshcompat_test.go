// Package gostmeshcompat proves the gost-mesh package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL.
//
// The gost API connection test runs through the whole path a deployment
// uses (NO-7): the kernel's v2 gateway seals the token the administrator
// typed into a handle, the package host's router runs the native handler,
// which submits a diagnose.forward_backend operation over its bridge
// session with the request binding, and the kernel's KernelNodeOps engine
// resolves the handle and dials. The package never sees the token. Both
// sides call the same test gost APIs: one that answers its service list to
// the right token, one that refuses, fails or answers what the client
// cannot decode, and an address nothing listens on.
package gostmeshcompat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	configtables "github.com/AnixOps/anix-control/v4/config"
	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/gost-mesh/native"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	path      = "/api/v2/admin/forward/test-connection"
	routeID   = native.TestConnectionRouteID
	packageID = "gost-mesh"
	token     = "gost-api-token"
)

// admin is the principal of the NodeX route parity cases.
var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

// gostAPI serves answer for GET /api/config/services when the request
// carries the token as its basic auth password, else 401. It records the
// passwords it was shown.
type gostAPIServer struct {
	host      string
	port      int
	mu        sync.Mutex
	passwords []string
}

func gostAPI(t *testing.T, answer func(w http.ResponseWriter, call int64)) *gostAPIServer {
	t.Helper()
	var calls atomic.Int64
	api := &gostAPIServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/config/services" {
			http.NotFound(w, r)
			return
		}
		_, password, _ := r.BasicAuth()
		api.mu.Lock()
		api.passwords = append(api.passwords, password)
		api.mu.Unlock()
		if password != token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"msg":"unauthorized"}`))
			return
		}
		answer(w, calls.Add(1))
	}))
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	api.port, err = strconv.Atoi(address.Port())
	require.NoError(t, err)
	api.host = address.Hostname()
	return api
}

func (a *gostAPIServer) shown() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.passwords...)
}

func body(text string) func(http.ResponseWriter, int64) {
	return func(w http.ResponseWriter, _ int64) { _, _ = w.Write([]byte(text)) }
}

// closedPort is a local port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func request(host string, port int, apiToken string) []byte {
	return []byte(fmt.Sprintf(`{"host":%q,"api_port":%d,"api_token":%q}`, host, port, apiToken))
}

// forEachDatabase runs body on SQLite and, with ANIX_TEST_POSTGRES_DSN set,
// on a throwaway PostgreSQL schema, each with the KernelNodeOps ledger.
func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"), config)
		require.NoError(t, err)
		closeOnCleanup(t, db)
		require.NoError(t, db.AutoMigrate(model.KernelNodeOperationModels()...))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv(packagecompat.PostgresDSNEnvironment))
		if base == "" {
			t.Skip(packagecompat.PostgresDSNEnvironment + " is not set")
		}
		admin, err := gorm.Open(postgres.Open(base), config)
		require.NoError(t, err)
		closeOnCleanup(t, admin)
		var databaseName string
		require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
		if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
			t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
		}
		suffix := make([]byte, 4)
		_, err = rand.Read(suffix)
		require.NoError(t, err)
		schema := "gostmeshcompat_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		db, err := gorm.Open(postgres.Open(base+" search_path="+schema), config)
		require.NoError(t, err)
		closeOnCleanup(t, db)
		require.NoError(t, db.AutoMigrate(model.KernelNodeOperationModels()...))
		body(t, db)
	})
}

func closeOnCleanup(t *testing.T, db *gorm.DB) {
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// kernel is the kernel of the native side: its gateway and sealer, the
// gost-mesh host's bridge session and router, and the KernelNodeOps engine
// serving the connection test; and the legacy handler on its own gin
// engine.
type kernel struct {
	t        *testing.T
	db       *gorm.DB
	store    *sealedsecrets.Store
	executor *kernelnodeops.ConnectionTest
	gateway  *gin.Engine
	legacy   *gin.Engine
	session  *packagebridge.Session
	router   *pluginhostsdk.Router
	mu       sync.Mutex
	received []pluginhostsdk.NativeRequest
	modes    map[string]string
}

type hostOperations struct{ k *kernel }

func (o hostOperations) PackageConfig(context.Context, packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	o.k.mu.Lock()
	defer o.k.mu.Unlock()
	modes := map[string]string{}
	for route, mode := range o.k.modes {
		modes[route] = mode
	}
	return packagebridge.PackageConfig{Revision: 1, ConfigHash: "h", RouteModes: modes}, nil
}

func (hostOperations) LeaseStorage(context.Context, packagebridge.HostIdentity) (packagebridge.StorageLease, error) {
	return packagebridge.StorageLease{}, packagebridge.ErrStorageUnavailable
}

// grants is the gost-mesh package's signed capability set: the diagnose
// family only.
type grants struct{}

func (grants) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host.PackageID == packageID && capability == service.CapabilityNodeOpsDiagnose {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

type routeSource struct{ k *kernel }

func (s routeSource) ResolveV2Route(_ context.Context, method, requestPath string) (compatv2.Route, error) {
	if method == http.MethodPost && requestPath == path {
		return compatv2.Route{Method: method, LegacyPath: path, PackageID: packageID, Version: "4.1.0", Generation: 5, PackageRoute: routeID, Envelope: compatv2.EnvelopeRaw}, nil
	}
	return compatv2.Route{}, compatv2.ErrRouteNotDeclared
}

// supervisor dispatches like pluginhost.Supervisor: it mints the request's
// bridge capability on the host's session and runs the host's router.
type supervisor struct{ k *kernel }

func (s supervisor) Dispatch(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	capability, err := s.k.session.Mint(request)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	defer s.k.session.Revoke(capability)
	response, err := s.k.router.Dispatch(ctx, pluginhostsdk.DispatchRequest{
		PackageID: input.PackageID, RouteID: input.RouteID, Method: input.Method, RequestID: input.RequestID,
		RequestBody: input.Body, PrincipalJSON: input.PrincipalJSON, BridgeCapability: capability,
		Metadata: pluginhostsdk.RequestMetadata{Path: input.Metadata.Path, PathParams: input.Metadata.PathParams},
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

func (s supervisor) DispatchLegacy(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	response, err := s.k.session.ServeLegacy(ctx, request, input.RouteID)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	return pluginhost.DispatchOutput{StatusCode: response.StatusCode, Body: response.Body}, nil
}

// newKernel starts the kernel with the route in mode.
func newKernel(t *testing.T, db *gorm.DB, mode string) *kernel {
	t.Helper()
	gin.SetMode(gin.TestMode)
	k := &kernel{t: t, db: db, store: sealedsecrets.NewStore(nil), executor: &kernelnodeops.ConnectionTest{}, modes: map[string]string{routeID: mode}}
	registry := kernelnodeops.NewRegistry()
	require.NoError(t, k.executor.Register(registry))
	engine := &kernelnodeops.Engine{DB: db, Executors: registry, PollInterval: 10 * time.Millisecond, Secrets: k.store}
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
	server := &kernelnodeops.Server{Engine: engine, Authorizer: grants{}}

	legacyHandler := handler.NewForwardHandler()
	admin := func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Set("is_admin", true)
	}
	k.legacy = gin.New()
	k.legacy.POST(path, func(c *gin.Context) {
		admin(c)
		legacyHandler.TestGostConnection(c)
	})

	table, err := sealedsecrets.ParseTable(configtables.NodeSecretFields)
	require.NoError(t, err)
	gateway := &compatv2.Gateway{
		Registry: compatv2.NewRegistry(routeSource{k: k}), Dispatcher: supervisor{k: k}, Metrics: compatv2.NewGatewayMetrics(),
		Sealer: sealedsecrets.NewSealer(table, nil, k.store),
	}
	k.gateway = gin.New()
	k.gateway.POST(path, func(c *gin.Context) {
		admin(c)
		gateway.Serve(c)
	})

	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: packageID, RouteID: routeID, Name: routeID, Handler: packagebridge.NewHTTPAdapter(legacyHandler.TestGostConnection),
	})
	require.NoError(t, err)
	identity := packagebridge.HostIdentity{PackageID: packageID, Version: "4.1.0", Generation: 5}
	session, child, err := packagebridge.NewSessionWithOptions(identity, allowlist, packagebridge.SessionOptions{
		HostOperations: hostOperations{k: k}, KernelNodeOps: server.For,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	k.session = session
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	native := &native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(client.Conn())}
	handlers := map[string]pluginhostsdk.NativeHandler{}
	for id, handle := range native.Handlers() {
		handlers[id] = k.recording(handle)
	}
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{PackageID: packageID, LeaseID: "lease", Bridge: client, Native: handlers})
	require.NoError(t, err)
	k.router = router
	router.Refresh(context.Background())
	return k
}

func (k *kernel) recording(handle pluginhostsdk.NativeHandler) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		k.mu.Lock()
		k.received = append(k.received, request)
		k.mu.Unlock()
		return handle(ctx, request)
	}
}

var clock = regexp.MustCompile(`"ts":\d+`)

// withoutClock replaces the envelope's clock reading, the one value the
// two sides cannot share.
func withoutClock(body []byte) string {
	return clock.ReplaceAllString(string(body), `"ts":0`)
}

func serve(engine *gin.Engine, body []byte) packagecompat.Result {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return packagecompat.Result{StatusCode: recorder.Code, Body: recorder.Body.Bytes(), Header: recorder.Header()}
}

// ledger is everything the KernelNodeOps ledger holds.
func (k *kernel) ledger() string {
	k.t.Helper()
	var rows []model.KernelNodeOperation
	require.NoError(k.t, k.db.Find(&rows).Error)
	encoded, err := json.Marshal(rows)
	require.NoError(k.t, err)
	return string(encoded)
}

type testCase struct {
	name string
	body []byte
}

func TestGostConnectionTestRouteParity(t *testing.T) {
	healthy := gostAPI(t, body(`{"data":{"count":2,"list":[{"name":"svc-a","addr":":8080","handler":{"type":"tcp"}},{"name":"svc-b","addr":":8081"}]}}`))
	empty := gostAPI(t, body(`{}`))
	failing := gostAPI(t, func(w http.ResponseWriter, _ int64) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream <down> & out"))
	})
	notJSON := gostAPI(t, body(`<html>gost</html>`))
	wrongCount := gostAPI(t, body(`{"data":{"count":"two"}}`))
	wrongNested := gostAPI(t, body(`{"data":{"count":1,"list":[{"name":"svc","handler":{"type":7}}]}}`))
	// The health check passes and the service list then fails: odd calls
	// answer, even calls fail, and each side makes two calls.
	flaky := gostAPI(t, func(w http.ResponseWriter, call int64) {
		if call%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("services unavailable"))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"count":1,"list":[]}}`))
	})
	refused := closedPort(t)
	host := healthy.host

	cases := []testCase{
		{"a healthy gost API", request(host, healthy.port, token)},
		{"an empty service list", request(host, empty.port, token)},
		{"a wrong token", request(host, healthy.port, "wrong")},
		{"no token", request(host, healthy.port, "")},
		{"an API error", request(host, failing.port, token)},
		{"an answer that is not JSON", request(host, notJSON.port, token)},
		{"a count that is not a number", request(host, wrongCount.port, token)},
		{"a nested field of the wrong type", request(host, wrongNested.port, token)},
		{"the service list fails after the health check", request(host, flaky.port, token)},
		{"nothing listens", request("127.0.0.1", refused, token)},
		{"a host that is not a host name", request("gost host", healthy.port, token)},
		{"a negative port", request(host, -1, token)},
		{"a host with a path", request(host+":"+strconv.Itoa(healthy.port)+"/x?", healthy.port, token)},
		{"no body", nil},
		{"invalid JSON", []byte(`{"host":`)},
		{"no host", []byte(`{"api_port":8080}`)},
		{"no port", []byte(`{"host":"127.0.0.1"}`)},
		{"port zero", []byte(`{"host":"127.0.0.1","api_port":0}`)},
		{"a port that is a string", []byte(`{"host":"127.0.0.1","api_port":"8080"}`)},
		{"a host that is a number", []byte(`{"host":7,"api_port":8080}`)},
		{"a body that is an array", []byte(`[1]`)},
		{"a body that is a string", []byte(`"127.0.0.1"`)},
		{"a body that is null", []byte(`null`)},
	}
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		k := newKernel(t, db, pluginhostsdk.RouteModeNative)
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				legacy := serve(k.legacy, c.body)
				native := serve(k.gateway, c.body)
				require.NoError(t, packagecompat.CompareAnswers(legacy, native))
				require.Equal(t, withoutClock(legacy.Body), withoutClock(native.Body), "byte for byte, but for the clock")
			})
		}
		require.Zero(t, k.store.Pending(), "handles die with their requests")
		require.Zero(t, k.executor.Held(), "the kernel holds no token after a test")
	})
}

// The administrator's token reaches the gost API from the kernel only: the
// package host receives a sealed handle, the kernel presents the typed
// token once, and neither the token nor the handle is in the ledger.
func TestTheTokenNeverReachesThePackage(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		api := gostAPI(t, body(`{"data":{"count":3,"list":[]}}`))
		k := newKernel(t, db, pluginhostsdk.RouteModeNative)
		response := serve(k.gateway, request(api.host, api.port, "typed-token-7"))
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Contains(t, string(response.Body), `"data":{"message":"API error: 401 Unauthorized - {\"msg\":\"unauthorized\"}","success":false}`)
		require.Equal(t, []string{"typed-token-7"}, api.shown(), "the kernel dialled with the typed token")

		k.mu.Lock()
		received := append([]pluginhostsdk.NativeRequest(nil), k.received...)
		k.mu.Unlock()
		require.Len(t, received, 1)
		require.NotContains(t, string(received[0].Body), "typed-token-7", "the token never reaches the package")
		require.True(t, sealedsecrets.ContainsHandle(string(received[0].Body)), "the package sees a handle")
		require.NotEmpty(t, received[0].Binding)

		ledger := k.ledger()
		require.NotContains(t, ledger, "typed-token-7")
		require.False(t, sealedsecrets.ContainsHandle(ledger), "the ledger never holds a handle")
		require.Contains(t, ledger, `"kind":"diagnose.forward_backend"`)
		require.Contains(t, ledger, `"state":"succeeded"`)
		require.Zero(t, k.executor.Held())
	})
}

// In shadow mode the native run has no binding, so it cannot submit and
// the answer comes from the legacy handler, which dials with the token.
func TestAShadowRunNeverDialsFromThePackage(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		api := gostAPI(t, body(`{"data":{"count":3,"list":[]}}`))
		k := newKernel(t, db, pluginhostsdk.RouteModeShadow)
		response := serve(k.gateway, request(api.host, api.port, token))
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Contains(t, string(response.Body), `"data":{"message":"Connection successful","service_count":3,"success":true}`)
		deadline := time.Now().Add(5 * time.Second)
		for len(k.ledger()) <= 2 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		require.Equal(t, "[]", k.ledger(), "a shadow run submits nothing")
		require.Equal(t, []string{token, token}, api.shown(), "the legacy handler dialled twice, the package never")
	})
}

// A request without the request binding, or without KernelNodeOps, is not
// served natively: the handler answers ErrNativeUnavailable and the router
// falls back to the legacy handler.
func TestWithoutABindingTheHandlerIsUnavailable(t *testing.T) {
	_, err := (&native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(nil)}).TestConnection(context.Background(), pluginhostsdk.NativeRequest{Body: request("127.0.0.1", 1, "")})
	require.ErrorIs(t, err, pluginhostsdk.ErrNativeUnavailable)
	_, err = (&native.Service{}).TestConnection(context.Background(), pluginhostsdk.NativeRequest{Body: request("127.0.0.1", 1, ""), Binding: []byte("b")})
	require.ErrorIs(t, err, pluginhostsdk.ErrNativeUnavailable)
	_, err = (&native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(nil)}).TestConnection(context.Background(),
		pluginhostsdk.NativeRequest{Body: request("127.0.0.1", 1, service.NodeSecretPlaceholder), Binding: []byte("b")})
	require.ErrorIs(t, err, pluginhostsdk.ErrNativeUnavailable, "a value the gateway did not seal is never sent")
}
