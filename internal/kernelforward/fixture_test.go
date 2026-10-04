package kernelforward

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs (TestPostgres*); each gets a
// throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var start = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

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

// openPostgres creates a throwaway schema in the test database and returns
// a migrated connection whose search_path is that schema.
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
	schema := "kernel_forward_" + hex.EncodeToString(suffix)
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

// clock is a settable time.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// fixture is a service on db with forward nodes 11 (entry, 192.0.2.11) and
// 12 (exit, 192.0.2.12) whose Agents negotiated forward.v1 with the
// nftables engine, a disabled forward node 13 and a proxy node 21.
type fixture struct {
	t       *testing.T
	db      *gorm.DB
	service *Service
	clock   *clock
	ctx     context.Context
}

var (
	entry = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 11}
	exit  = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 12}
	proxy = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 21}
	// proxyMissing has no node row.
	proxyMissing = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 99}
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

// openBare is a SQLite database without the forwarding tables.
func openBare(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func newFixture(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	f := &fixture{t: t, db: db, clock: &clock{now: start}, ctx: context.Background()}
	f.service = &Service{DB: db, Cluster: func() string { return "test" }, Now: f.clock.Now, Probes: noDials()}
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 11, Name: "entry", Type: "relay", Host: "192.0.2.11", Port: 7000, APIPort: 7001, Enabled: true},
		{ID: 12, Name: "exit", Type: "exit", Host: "192.0.2.12", Port: 7000, Enabled: true},
		{ID: 13, Name: "off", Type: "exit", Host: "192.0.2.13", Port: 7000, Enabled: true},
	}).Error)
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 13).Update("enabled", false).Error)
	require.NoError(t, db.Create(&model.Node{ID: 21, Name: "proxy", Host: "proxy.example.com", Port: 443, APIKey: "proxy-key", Status: model.NodeStatusOnline}).Error)
	require.NoError(t, db.Create(&model.NodeProtocol{NodeID: 21, Name: "vless", Type: "vless", Port: 30443}).Error)
	f.hello(entry, nftCaps())
	f.hello(exit, nftCaps())
	return f
}

func nftCaps(engines ...forwardv1.Engine) *forwardv1.NodeCapabilities {
	if len(engines) == 0 {
		engines = []forwardv1.Engine{forwardv1.Engine_ENGINE_NFTABLES}
	}
	caps := &forwardv1.NodeCapabilities{KernelVersion: "6.12", Cgroup: "v2", Ipv6: true, AgentVersion: "4.2.0"}
	for _, engine := range engines {
		caps.Engines = append(caps.Engines, &forwardv1.EngineCapabilities{
			Engine: engine, Version: "test", Available: true, Ipv6: true, Udp: true,
			Strategies: []forwardv1.BalanceStrategy{
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN, forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
				forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
			},
			LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
			BandwidthLimit: true, Quota: true, MaxConns: true,
		})
	}
	return caps
}

func (f *fixture) hello(node agentcontrol.AgentNode, caps *forwardv1.NodeCapabilities) PlanOutcome {
	f.t.Helper()
	outcome, _, err := f.service.RecordHello(f.ctx, node, caps, "4.2.0")
	require.NoError(f.t, err)
	return outcome
}

// twoHop is an nftables route from forward-11 to forward-12 and on to
// 198.51.100.10:443.
func twoHop(port uint32) *forwardv1.Route {
	return &forwardv1.Route{
		Owner: "admin", Name: "hk-jp",
		Listen: &forwardv1.Listen{Port: port, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-11"}},
			{Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{"forward-12"},
				Ingress: &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW}},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 443}},
	}
}

func (f *fixture) create(requestID string, route *forwardv1.Route) *forwardv1.Route {
	f.t.Helper()
	created, err := f.service.CreateRoute(f.ctx, requestID, route)
	require.NoError(f.t, err)
	return created
}

func (f *fixture) state(node agentcontrol.AgentNode) *forwardv1.NodeForwardState {
	f.t.Helper()
	state, found, err := f.service.State(f.ctx, node.String())
	require.NoError(f.t, err)
	require.True(f.t, found, node.String())
	return state
}

func (f *fixture) allocations() []model.KernelForwardAllocation {
	f.t.Helper()
	var rows []model.KernelForwardAllocation
	require.NoError(f.t, f.db.Order("route_id, hop_index, node_ref").Find(&rows).Error)
	return rows
}

// recordedListener collects the nodes every committed plan changed.
type recordedListener struct {
	mu    sync.Mutex
	nodes []string
}

func listen(t *testing.T) *recordedListener {
	recorder := &recordedListener{}
	cancel := OnStateChange(func(nodes []agentcontrol.AgentNode) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		for _, node := range nodes {
			recorder.nodes = append(recorder.nodes, node.String())
		}
	})
	t.Cleanup(cancel)
	return recorder
}

func (r *recordedListener) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	nodes := r.nodes
	r.nodes = nil
	return nodes
}

// noDials are diagnosis probes that never reach the network: every dial
// is refused and every name fails to resolve.
func noDials() service.DiagnosisProbes {
	return service.DiagnosisProbes{
		Dial: func(context.Context, string, string, time.Duration) (net.Conn, error) {
			return nil, errors.New("connection refused (test)")
		},
		Lookup: func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("no such host (test)") },
	}
}
