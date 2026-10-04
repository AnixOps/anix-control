// Package forwardv4 runs the forward package's v4 API (packages/forward/
// v4api) against the kernel's real ForwardControl (internal/kernelforward),
// served in process over gRPC, on SQLite and PostgreSQL (TestPostgres*,
// ANIX_TEST_POSTGRES_DSN).
package forwardv4

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/edition"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/packages/forward/v4api"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var forwardHost = packagebridge.HostIdentity{PackageID: "forward", Version: "4.0.0", Generation: 1}

// forwardOnly authorizes kernel.forward.v1 for the forward host, as its
// signed manifest declares it.
type forwardOnly struct{}

func (forwardOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == forwardHost && capability == service.CapabilityForward {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSN))
	if base == "" {
		t.Skip(postgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "forward_v4_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.ForwardNode{}, &model.Node{}, &model.NodeProtocol{})...))
}

func nftCaps() *forwardv1.NodeCapabilities {
	return &forwardv1.NodeCapabilities{KernelVersion: "6.12", Cgroup: "v2", AgentVersion: "4.2.0", Engines: []*forwardv1.EngineCapabilities{{
		Engine: forwardv1.Engine_ENGINE_NFTABLES, Version: "test", Available: true, Udp: true,
		Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: true, Quota: true, MaxConns: true,
	}}}
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	kernel *kernelforward.Service
	api    *v4api.Service
	now    time.Time
}

// newEnv serves ForwardControl on db to the forward host over gRPC, with
// forward nodes 11 and 12 whose Agents negotiated forward.v1.
func newEnv(t *testing.T, db *gorm.DB) *env {
	t.Helper()
	e := &env{t: t, db: db, now: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	e.kernel = &kernelforward.Service{DB: db, Cluster: func() string { return "test" }, Now: func() time.Time { return e.now }}
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 11, Name: "hk-entry", Type: "relay", Host: "192.0.2.11", Port: 7000, Enabled: true},
		{ID: 12, Name: "jp-exit", Type: "exit", Host: "192.0.2.12", Port: 7000, Enabled: true},
	}).Error)
	for _, id := range []uint32{11, 12} {
		_, _, err := e.kernel.RecordHello(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: id}, nftCaps(), "4.2.0")
		require.NoError(t, err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	forwardv1.RegisterForwardControlServer(server, (&kernelforward.Server{Service: e.kernel, Authorizer: forwardOnly{}}).For(forwardHost))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	e.api = &v4api.Service{Forward: forwardv1.NewForwardControlClient(conn)}
	return e
}

type answer struct {
	status int
	body   map[string]any
	raw    string
}

func (a answer) data() map[string]any {
	data, _ := a.body["data"].(map[string]any)
	return data
}

func (a answer) errorCode() string {
	failure, _ := a.body["error"].(map[string]any)
	code, _ := failure["code"].(string)
	return code
}

func (a answer) violationCodes() []string {
	failure, _ := a.body["error"].(map[string]any)
	list, _ := failure["violations"].([]any)
	var out []string
	for _, item := range list {
		out = append(out, item.(map[string]any)["code"].(string))
	}
	return out
}

func (e *env) call(method, path, body, key string) answer {
	e.t.Helper()
	request := v4api.Request{Method: method, Path: v4api.PublicPrefix + path, Body: []byte(body), IdempotencyKey: key, ActorID: 1, Query: map[string][]string{}}
	if i := strings.Index(path, "?"); i >= 0 {
		request.Path = v4api.PublicPrefix + path[:i]
		for _, pair := range strings.Split(path[i+1:], "&") {
			k, v, _ := strings.Cut(pair, "=")
			request.Query[k] = append(request.Query[k], v)
		}
	}
	response := e.api.Serve(context.Background(), request)
	out := answer{status: response.StatusCode, raw: string(response.Body)}
	require.NoError(e.t, json.Unmarshal(response.Body, &out.body), out.raw)
	return out
}

const twoHop = `{"name":"hk-jp","listen":{"port":%PORT%,"protocol":"L4_PROTOCOL_TCP"},
 "hops":[{"role":"HOP_ROLE_ENTRY","engine":"ENGINE_NFTABLES","node_refs":["forward-11"]},
         {"role":"HOP_ROLE_EXIT","engine":"ENGINE_NFTABLES","node_refs":["forward-12"],"ingress":{"security":"%SEC%"}}],
 "targets":[{"host":"198.51.100.10","port":443}]}`

func route(port, security string) string {
	return strings.NewReplacer("%PORT%", port, "%SEC%", security).Replace(twoHop)
}

func TestForwardV4API(t *testing.T)         { runForwardV4API(t, openSQLite(t)) }
func TestPostgresForwardV4API(t *testing.T) { runForwardV4API(t, openPostgres(t)) }

func runForwardV4API(t *testing.T, db *gorm.DB) {
	e := newEnv(t, db)

	nodes := e.call("GET", "/nodes", "", "")
	require.Equal(t, http.StatusOK, nodes.status, nodes.raw)
	require.Len(t, nodes.data()["nodes"], 2)

	// Create, with the Idempotency-Key as the request id: a retry answers
	// the same route.
	created := e.call("POST", "/routes", route("31000", "LINK_SECURITY_RAW"), "create-1")
	require.Equal(t, http.StatusCreated, created.status, created.raw)
	stored := created.data()["route"].(map[string]any)
	id := stored["id"].(string)
	assert.Equal(t, "admin", stored["owner"])
	assert.Equal(t, "1", stored["revision"])
	retried := e.call("POST", "/routes", route("31000", "LINK_SECURITY_RAW"), "create-1")
	require.Equal(t, http.StatusCreated, retried.status, retried.raw)
	assert.Equal(t, id, retried.data()["route"].(map[string]any)["id"])
	conflict := e.call("POST", "/routes", route("31001", "LINK_SECURITY_RAW"), "create-1")
	assert.Equal(t, http.StatusConflict, conflict.status)
	assert.Equal(t, "idempotency_conflict", conflict.errorCode())

	// Validation errors carry sdk/forward/validate's codes.
	invalid := e.call("POST", "/routes", route("31002", "LINK_SECURITY_TLS"), "")
	assert.Equal(t, http.StatusBadRequest, invalid.status, invalid.raw)
	assert.Equal(t, "invalid_route", invalid.errorCode())
	assert.Contains(t, invalid.violationCodes(), "link_unsupported")
	taken := e.call("POST", "/routes", route("31000", "LINK_SECURITY_RAW"), "")
	assert.Equal(t, http.StatusConflict, taken.status, taken.raw)
	assert.Equal(t, "refused", taken.errorCode())
	assert.Contains(t, taken.violationCodes(), "port_in_use")
	preview := e.call("POST", "/routes/preview", `{"route":`+route("31005", "LINK_SECURITY_RAW")+`}`, "")
	require.Equal(t, http.StatusOK, preview.status, preview.raw)
	assert.Len(t, preview.data()["states"], 2)

	// Pause and resume move the revision and the nodes' state.
	paused := e.call("POST", "/routes/"+id+"/pause", "", "pause-1")
	require.Equal(t, http.StatusOK, paused.status, paused.raw)
	assert.Equal(t, true, paused.data()["route"].(map[string]any)["paused"])
	state, _, err := e.kernel.State(context.Background(), "forward-11")
	require.NoError(t, err)
	require.Len(t, state.GetHops(), 1)
	assert.True(t, state.GetHops()[0].GetPaused())
	resumed := e.call("POST", "/routes/"+id+"/resume", "", "")
	require.Equal(t, http.StatusOK, resumed.status, resumed.raw)
	assert.Equal(t, "3", resumed.data()["route"].(map[string]any)["revision"])
	stale := e.call("PUT", "/routes/"+id, `{"name":"renamed","revision":1,"listen":{"port":31000,"protocol":"L4_PROTOCOL_TCP"}}`, "")
	assert.Equal(t, http.StatusConflict, stale.status)
	assert.Equal(t, "revision_conflict", stale.errorCode())

	// Node settings and the node view.
	settings := e.call("PUT", "/nodes/forward-12/settings", `{"port_range":{"first":40000,"last":40999},"labels":{"link":"iepl"}}`, "")
	require.Equal(t, http.StatusOK, settings.status, settings.raw)
	assert.Empty(t, settings.data()["violations"])
	view := e.call("GET", "/nodes/forward-12", "", "")
	require.Equal(t, http.StatusOK, view.status, view.raw)
	node := view.data()["node"].(map[string]any)
	assert.EqualValues(t, 40000, node["info"].(map[string]any)["port_range"].(map[string]any)["first"])
	assert.NotNil(t, view.data()["state"])
	badSettings := e.call("PUT", "/nodes/forward-12/settings", `{"addresses":["exit.example.com"]}`, "")
	assert.Equal(t, http.StatusBadRequest, badSettings.status)
	assert.Equal(t, "invalid_request", badSettings.errorCode())

	inUse := e.call("POST", "/nodes/forward-12/toggle", `{"enabled":false}`, "")
	assert.Equal(t, http.StatusConflict, inUse.status, inUse.raw)
	assert.Equal(t, []string{"node_in_use"}, inUse.violationCodes())

	// The ledger: stats, the trend and the observability views.
	entryState, _, err := e.kernel.State(context.Background(), "forward-11")
	require.NoError(t, err)
	_, err = e.kernel.RecordReport(context.Background(), agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 11}, &forwardv1.NodeForwardReport{
		NodeRef: "forward-11", Generation: entryState.GetGeneration(), StateHash: entryState.GetStateHash(), Applied: true,
		Counters:         []*forwardv1.Counters{{RouteId: id, NodeRef: "forward-11", UpBytes: 1000, DownBytes: 4000, TotalConns: 3, CounterEpoch: "e1"}},
		Health:           []*forwardv1.UpstreamHealth{{RouteId: id, HopIndex: 0, Address: "192.0.2.12", Port: 40000, State: forwardv1.HealthState_HEALTH_STATE_HEALTHY, RttUs: 900}},
		ObservedAtUnixMs: e.now.UnixMilli(),
	}, e.now, e.now)
	require.NoError(t, err)
	stats := e.call("GET", "/routes/"+id+"/stats", "", "")
	require.Equal(t, http.StatusOK, stats.status, stats.raw)
	assert.Len(t, stats.data()["counters"], 1)
	assert.Len(t, stats.data()["series"], 1)
	all := e.call("GET", "/stats?node_ref=forward-11", "", "")
	require.Equal(t, http.StatusOK, all.status, all.raw)
	totals := all.data()["nodes"].([]any)
	require.Len(t, totals, 1)
	assert.EqualValues(t, 4000, totals[0].(map[string]any)["down_bytes"])
	trend := e.call("GET", "/observability/trend", "", "")
	require.Equal(t, http.StatusOK, trend.status, trend.raw)
	require.Len(t, trend.data()["points"], 1)
	targets := e.call("GET", "/observability/targets", "", "")
	require.Equal(t, http.StatusOK, targets.status, targets.raw)
	require.Len(t, targets.data()["targets"], 1)
	assert.Equal(t, "198.51.100.10:443", targets.data()["targets"].([]any)[0].(map[string]any)["key"])
	topology := e.call("GET", "/observability/topology", "", "")
	require.Equal(t, http.StatusOK, topology.status, topology.raw)
	assert.Len(t, topology.data()["edges"], 2)
	assert.Len(t, topology.data()["nodes"], 3)
	health := e.call("GET", "/routes/"+id+"/health", "", "")
	require.Equal(t, http.StatusOK, health.status, health.raw)
	assert.Len(t, health.data()["health"], 1)

	// The registry: an Ansible machine, then deletes.
	machine := e.call("POST", "/ansible-machines", `{"node":{"name":"sg","host":"192.0.2.31","port":22}}`, "m-1")
	require.Equal(t, http.StatusCreated, machine.status, machine.raw)
	ref := machine.data()["node"].(map[string]any)["node_ref"].(string)
	machineID := strings.TrimPrefix(ref, "forward-")
	got := e.call("GET", "/ansible-machines/"+machineID, "", "")
	require.Equal(t, http.StatusOK, got.status, got.raw)
	assert.Equal(t, http.StatusNotFound, e.call("GET", "/ansible-machines/11", "", "").status, "an Agent node is not an Ansible machine")
	machines := e.call("GET", "/ansible-machines", "", "")
	require.Len(t, machines.data()["nodes"], 1)
	assert.Equal(t, http.StatusOK, e.call("DELETE", "/ansible-machines/"+machineID, "", "").status)

	refused := e.call("DELETE", "/nodes/forward-12", "", "")
	assert.Equal(t, http.StatusConflict, refused.status, refused.raw)
	deleted := e.call("DELETE", "/routes/"+id, "", "")
	require.Equal(t, http.StatusOK, deleted.status, deleted.raw)
	assert.Equal(t, http.StatusNotFound, e.call("GET", "/routes/"+id, "", "").status)
	assert.Equal(t, http.StatusOK, e.call("DELETE", "/nodes/forward-12", "", "").status)
	assert.Equal(t, http.StatusNotFound, e.call("GET", "/nodes/forward-12", "", "").status)

	// No answer carries a node credential.
	var token string
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 11).Pluck("api_token", &token).Error)
	for _, raw := range []string{nodes.raw, view.raw, machine.raw, e.call("GET", "/nodes", "", "").raw} {
		assert.NotContains(t, raw, "api_token")
		if token != "" {
			assert.NotContains(t, raw, token)
		}
	}
}

// A host the kernel does not authorize for kernel.forward.v1 gets nothing:
// the API answers 503.
func TestUnauthorizedHostIsRefused(t *testing.T) {
	db := openSQLite(t)
	kernel := &kernelforward.Service{DB: db}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	other := packagebridge.HostIdentity{PackageID: "gost-mesh", Version: "4.0.0", Generation: 1}
	forwardv1.RegisterForwardControlServer(server, (&kernelforward.Server{Service: kernel, Authorizer: forwardOnly{}}).For(other))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	api := &v4api.Service{Forward: forwardv1.NewForwardControlClient(conn)}
	response := api.Serve(context.Background(), v4api.Request{Method: "GET", Path: v4api.PublicPrefix + "/routes"})
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

// The manifest declares the control route the kernel serves at
// /api/v4/forward/*, and the capability the API needs; every endpoint is
// outside the prefixes config/editions.json reserves for the commercial
// edition, unless it is commercial itself.
func TestManifestAndEditions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "forward", "manifest.template.json"))
	require.NoError(t, err)
	var manifest struct {
		ControlRoutes []string `json:"control_routes"`
		Capabilities  []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	assert.Equal(t, []string{v4api.Route}, manifest.ControlRoutes)
	assert.Equal(t, service.PluginControlBridgeRouteID("forward", v4api.Route), v4api.RouteID,
		"the host recognizes the route id the kernel dispatches the API with")
	assert.Contains(t, manifest.Capabilities, service.CapabilityForward)

	community := edition.New("community")
	commercial := edition.New("commercial")
	for _, endpoint := range v4api.Endpoints() {
		path := v4api.PublicPrefix + strings.ReplaceAll(strings.ReplaceAll(endpoint.Pattern, "{id}", "1"), "{ref}", "forward-1")
		switch endpoint.Edition {
		case v4api.EditionAll:
			assert.False(t, community.HidesPath(path), "%s is in both editions", path)
		case v4api.EditionCommercial:
			assert.True(t, community.HidesPath(path), "%s is commercial: it must sit under a commercial prefix", path)
			assert.False(t, commercial.HidesPath(path))
		default:
			t.Errorf("%s has edition %q", path, endpoint.Edition)
		}
	}
}
