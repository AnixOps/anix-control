package sealedhandles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	configtables "github.com/AnixOps/anix-control/v4/config"
	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const proxyNodeID, forwardNodeID = 3, 7

// extractionRoute is a row of config/package-extraction.json.
type extractionRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Package string `json:"package_id"`
	RouteID string `json:"route_id"`
}

func extraction(t *testing.T) map[string]extractionRoute {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "package-extraction.json"))
	require.NoError(t, err)
	var document struct {
		Routes []extractionRoute `json:"routes"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	rows := map[string]extractionRoute{}
	for _, row := range document.Routes {
		rows[row.RouteID] = row
	}
	return rows
}

// host is one package host: its kernel bridge session, its SDK client and
// its router.
type host struct {
	identity packagebridge.HostIdentity
	session  *packagebridge.Session
	client   *packagebridgesdk.Client
	router   *pluginhostsdk.Router
	modes    map[string]string
}

// hostOperations serves each host its route modes.
type hostOperations struct{ h *harness }

func (o hostOperations) PackageConfig(_ context.Context, identity packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	o.h.mu.Lock()
	defer o.h.mu.Unlock()
	modes := map[string]string{}
	for route, mode := range o.h.hosts[identity.PackageID].modes {
		modes[route] = mode
	}
	return packagebridge.PackageConfig{Revision: 1, ConfigHash: "h", RouteModes: modes}, nil
}

func (hostOperations) LeaseStorage(context.Context, packagebridge.HostIdentity) (packagebridge.StorageLease, error) {
	return packagebridge.StorageLease{}, packagebridge.ErrStorageUnavailable
}

type grants struct{}

func (grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if capability == service.CapabilityNodeOpsCredentials {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// harness is the kernel (gateway, sealer, bridge sessions, KernelNodeOps
// with a fake credential executor) and the package hosts.
type harness struct {
	t       *testing.T
	db      *gorm.DB
	store   *sealedsecrets.Store
	server  *kernelnodeops.Server
	gin     *gin.Engine
	serve   gin.HandlerFunc
	metrics *compatv2.GatewayMetrics

	mu         sync.Mutex
	hosts      map[string]*host
	routes     []compatv2.Route
	native     map[string]pluginhostsdk.NativeHandler
	dispatched []pluginhost.DispatchInput
	viaLegacy  []pluginhost.DispatchInput
	legacy     map[string][]string
	received   map[string][]pluginhostsdk.NativeRequest
	resolved   map[string]string
	lastMinted []byte
	requests   int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(model.KernelNodeOperationModels(), &model.Node{}, &model.ForwardNode{})...))
	require.NoError(t, db.Create(&model.Node{ID: proxyNodeID, Name: "edge", APIKey: "stored-key"}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: forwardNodeID, Name: "relay", Host: "relay.example", Port: 443}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: forwardNodeID + 1, Name: "other", Host: "other.example", Port: 443}).Error)

	h := &harness{
		t: t, db: db, store: sealedsecrets.NewStore(nil), hosts: map[string]*host{}, native: map[string]pluginhostsdk.NativeHandler{},
		legacy: map[string][]string{}, received: map[string][]pluginhostsdk.NativeRequest{}, resolved: map[string]string{},
		metrics: compatv2.NewGatewayMetrics(),
	}
	registry := kernelnodeops.NewRegistry()
	require.NoError(t, registry.Register(kernelnodeops.KindCredentialIssue, credentialExecutor{h: h}))
	engine := &kernelnodeops.Engine{DB: db, Executors: registry, PollInterval: 10 * time.Millisecond, Secrets: h.store}
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
	h.server = &kernelnodeops.Server{Engine: engine, Authorizer: grants{}}

	table, err := sealedsecrets.ParseTable(configtables.NodeSecretFields)
	require.NoError(t, err)
	gateway := &compatv2.Gateway{
		Registry: compatv2.NewRegistry(routeSource{h: h}), Dispatcher: supervisor{h: h}, Metrics: h.metrics,
		Sealer: sealedsecrets.NewSealer(table, nil, h.store),
	}
	h.gin = gin.New()
	h.serve = gateway.Serve
	return h
}

// route serves a v2 route through a package host: its legacy handler (in
// the kernel, through the bridge) records the body it read, and native, when
// set, is the package's implementation.
func (h *harness) route(routeID string, native pluginhostsdk.NativeHandler) {
	h.t.Helper()
	row, ok := extraction(h.t)[routeID]
	if !ok {
		// A route the tests add: GET /api/v2/admin/nodes/:id of proxy-node.
		row = extractionRoute{Method: http.MethodGet, Path: "/api/v2/admin/nodes/:id", Package: "proxy-node", RouteID: routeID}
	}
	// The kernel registers each route's own pattern, so the gateway sends
	// its path parameters.
	h.gin.Handle(row.Method, row.Path, h.serve)
	h.mu.Lock()
	h.routes = append(h.routes, compatv2.Route{
		Method: row.Method, LegacyPath: row.Path, PackageID: row.Package, Version: "4.1.0", Generation: 5,
		PackageRoute: routeID, Envelope: compatv2.EnvelopeRaw,
	})
	if native != nil {
		h.native[routeID] = native
	}
	current := h.hosts[row.Package]
	h.mu.Unlock()
	if current != nil {
		h.t.Fatalf("declare every route of %s before its host starts", row.Package)
	}
}

// start starts the package hosts with their routes in the given modes.
func (h *harness) start(modes map[string]string) {
	h.t.Helper()
	byPackage := map[string][]compatv2.Route{}
	for _, route := range h.routes {
		byPackage[route.PackageID] = append(byPackage[route.PackageID], route)
	}
	for packageID, routes := range byPackage {
		var operations []packagebridge.Operation
		native := map[string]pluginhostsdk.NativeHandler{}
		hostModes := map[string]string{}
		for _, route := range routes {
			routeID := route.PackageRoute
			operations = append(operations, packagebridge.Operation{PackageID: packageID, RouteID: routeID, Name: routeID, Handler: packagebridge.NewHTTPAdapter(func(c *gin.Context) {
				body, _ := io.ReadAll(c.Request.Body)
				h.mu.Lock()
				h.legacy[routeID] = append(h.legacy[routeID], string(body))
				h.mu.Unlock()
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"legacy": true}, "msg": "ok"})
			})})
			if handler := h.native[routeID]; handler != nil {
				native[routeID] = h.recording(routeID, handler)
			}
			if mode := modes[routeID]; mode != "" {
				hostModes[routeID] = mode
			}
		}
		allowlist, err := packagebridge.NewAllowlistWithFallback(nil, operations...)
		require.NoError(h.t, err)
		identity := packagebridge.HostIdentity{PackageID: packageID, Version: "4.1.0", Generation: 5}
		session, child, err := packagebridge.NewSessionWithOptions(identity, allowlist, packagebridge.SessionOptions{
			HostOperations: hostOperations{h: h}, KernelNodeOps: h.server.For,
		})
		require.NoError(h.t, err)
		h.t.Cleanup(func() { _ = session.Close() })
		client, err := packagebridgesdk.DialFile(context.Background(), child)
		require.NoError(h.t, err)
		h.t.Cleanup(func() { _ = client.Close() })
		router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{PackageID: packageID, LeaseID: "lease", Bridge: client, Native: native})
		require.NoError(h.t, err)
		h.mu.Lock()
		h.hosts[packageID] = &host{identity: identity, session: session, client: client, router: router, modes: hostModes}
		h.mu.Unlock()
		router.Refresh(context.Background())
	}
}

// recording records what a native handler is sent.
func (h *harness) recording(routeID string, handler pluginhostsdk.NativeHandler) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		h.mu.Lock()
		h.received[routeID] = append(h.received[routeID], request)
		h.mu.Unlock()
		return handler(ctx, request)
	}
}

// nodeOps is the KernelNodeOps client of a package host.
func (h *harness) nodeOps(packageID string) kernelnodeopsv1.KernelNodeOpsClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	return kernelnodeopsv1.NewKernelNodeOpsClient(h.hosts[packageID].client.Conn())
}

// requestID is a fresh KernelNodeOps request id.
func (h *harness) requestID(prefix string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests++
	return prefix + ":" + strconv.Itoa(h.requests)
}

// do sends one v2 request through the gateway.
func (h *harness) do(method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.gin.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

// supervisor dispatches like pluginhost.Supervisor: it mints the request's
// bridge capability on the host's session from what the capability retains,
// and runs the host's router.
type supervisor struct{ h *harness }

func (s supervisor) host(packageID string) *host {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	return s.h.hosts[packageID]
}

func (s supervisor) Dispatch(ctx context.Context, input pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	s.h.mu.Lock()
	s.h.dispatched = append(s.h.dispatched, input)
	s.h.mu.Unlock()
	target := s.host(input.PackageID)
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	capability, err := target.session.Mint(request)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	defer target.session.Revoke(capability)
	s.h.mu.Lock()
	s.h.lastMinted = append([]byte(nil), capability...)
	s.h.mu.Unlock()
	response, err := target.router.Dispatch(ctx, pluginhostsdk.DispatchRequest{
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
	s.h.mu.Lock()
	s.h.viaLegacy = append(s.h.viaLegacy, input)
	s.h.mu.Unlock()
	request, err := pluginhost.BridgeRequest(input)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	response, err := s.host(input.PackageID).session.ServeLegacy(ctx, request, input.RouteID)
	if err != nil {
		return pluginhost.DispatchOutput{}, err
	}
	return pluginhost.DispatchOutput{StatusCode: response.StatusCode, Body: response.Body}, nil
}

// routeSource resolves the harness's routes as the verified source does.
type routeSource struct{ h *harness }

func (s routeSource) ResolveV2Route(_ context.Context, method, path string) (compatv2.Route, error) {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	for _, route := range s.h.routes {
		if route.Method == method && patternMatches(route.LegacyPath, path) {
			return route, nil
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

// credentialExecutor stands for NO-5's credential.issue: its Preparer
// resolves the token an administrator typed (sealed from /api_token) for
// the operation's subject, and its executor reveals the credentials it
// generates.
type credentialExecutor struct{ h *harness }

func (e credentialExecutor) Prepare(_ context.Context, submission *kernelnodeops.Submission) error {
	op := submission.Operation.GetIssueCredential()
	if op.GetValue() == nil {
		return nil
	}
	secrets, err := submission.Unseal(sealedsecrets.Use{
		Handle: op.GetValue().GetHandle(), Target: kernelnodeops.NodeTarget(op.GetSubject()), Field: "/api_token",
	})
	if err != nil {
		return err
	}
	e.h.mu.Lock()
	e.h.resolved[fmt.Sprintf("forward-%d", op.GetSubject().GetId())] = secrets[0].Value
	e.h.mu.Unlock()
	return nil
}

func (e credentialExecutor) Execute(_ context.Context, run *kernelnodeops.Run) kernelnodeops.Outcome {
	op := run.Operation.GetIssueCredential()
	result := &kernelnodeopsv1.CredentialResult{Subject: op.GetSubject(), Kind: op.GetKind(), Version: 1}
	if op.GetValue() == nil {
		for _, name := range []string{"api_key", "secret"} {
			handle, err := run.Reveal(name, fmt.Sprintf("generated-%s-%d", name, op.GetSubject().GetId()))
			if err != nil {
				return kernelnodeops.Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the credential cannot be shown", false)
			}
			result.Reveal = append(result.Reveal, handle)
		}
	}
	return kernelnodeops.Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}

// panel is a native panel answer.
func panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, time.Unix(1, 0)))
}

// refusal answers a KernelNodeOps refusal as the package would, by its code.
func refusal(err error) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(status.Code(err).String(), time.Unix(1, 0)))
}

// ledgerText is everything the KernelNodeOps ledger holds.
func (h *harness) ledgerText() string {
	h.t.Helper()
	var rows []model.KernelNodeOperation
	require.NoError(h.t, h.db.Find(&rows).Error)
	encoded, err := json.Marshal(rows)
	require.NoError(h.t, err)
	var events []model.KernelNodeOperationEvent
	require.NoError(h.t, h.db.Find(&events).Error)
	eventsEncoded, err := json.Marshal(events)
	require.NoError(h.t, err)
	return string(encoded) + string(eventsEncoded)
}
