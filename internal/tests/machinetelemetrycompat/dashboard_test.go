package machinetelemetrycompat

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kerneltelemetry"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/machine-telemetry/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The dashboard's parity: the legacy handler answers the snapshot the
// kernel caches; the native one reads it through the real KernelTelemetry
// server (internal/kerneltelemetry), served in process over gRPC on the
// native side's database. The cache is the kernel's memory, one per
// process: each side's seed starts it afresh, so a case's sides do not
// share it, and a seeded snapshot is answered byte for byte, cached_at
// included. A snapshot a side builds itself has that side's cached_at,
// which is masked; the cache it leaves is compared.

// telemetryHost is the identity the kernel serves the machine-telemetry
// host as.
var telemetryHost = packagebridge.HostIdentity{PackageID: "machine-telemetry", Version: "4.1.0", Generation: 1}

// dashboardOnly authorizes what the machine-telemetry package's signed
// release declares of KernelTelemetry, for its host.
type dashboardOnly struct{}

func (dashboardOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == telemetryHost && capability == service.CapabilityTelemetryDashboard {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelTelemetry serves the kernel's KernelTelemetry on db in process and
// returns a client for it, as the machine-telemetry host gets one over its
// bridge.
func kernelTelemetry(t *testing.T, db *gorm.DB) kerneltelemetryv1.KernelTelemetryClient {
	t.Helper()
	server := &kerneltelemetry.Server{DB: db, Authorizer: dashboardOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kerneltelemetryv1.RegisterKernelTelemetryServer(grpcServer, server.For(telemetryHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kerneltelemetryv1.NewKernelTelemetryClient(conn)
}

func dashboardRoute(t *testing.T) packagecompat.Route {
	return packagecompat.Route{
		Method: "GET", Pattern: "/api/v2/admin/dashboard", RouteID: native.DashboardRouteID,
		Legacy: stats((*handler.AdminHandler).GetDashboard),
		Models: []any{
			&model.User{}, &model.Plan{}, &model.Order{}, &model.ServerVMess{}, &model.ServerVLESS{}, &model.ServerTrojan{},
			&model.ServerShadowsocks{}, &model.TrafficLog{},
		},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open:      func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Telemetry: kernelTelemetry(t, db),
			}
			return service.Handlers()[native.DashboardRouteID]
		},
	}
}

// seededAt is the cached_at of a seeded snapshot: a fraction and an offset,
// both of which an answer must keep.
var seededAt = time.Date(2026, 9, 30, 23, 59, 58, 123456789, time.FixedZone("", 8*3600))

// seededSnapshot differs from what the seeded rows count in every field,
// so an answer shows whether it came from the cache.
func seededSnapshot(cachedAt time.Time) *service.DashboardStats {
	return &service.DashboardStats{
		TotalUsers: 101, ActiveUsers: 102, ExpiredUsers: 103, BannedUsers: 104, TodayNewUsers: 105,
		TotalOrders: 106, PendingOrders: 107, PaidOrders: 108, TotalRevenue: 109, MonthlyIncome: 110, TodayIncome: 111,
		TotalNodes: 112, ActiveNodes: 113, OnlineUsers: 114, TotalTrafficUsed: 115, TodayTraffic: 116,
		CachedAt: cachedAt,
	}
}

// freshCache starts the kernel's cache afresh, with three users in the
// alive set node reports keep.
func freshCache(t testing.TB) {
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)
	require.NoError(t, cache.SAdd("alive:users", "1", "2", "3"))
}

// seedDashboard writes rows that make every count of the snapshot nonzero:
// users active, expired, banned and
// new today, orders pending, paid, completed and cancelled (paid today and
// forty days ago), shown and hidden nodes of each legacy server table with
// recent and old checks, and traffic reported today and yesterday.
func seedDashboard(t testing.TB, db *gorm.DB) {
	freshCache(t)
	now := time.Now()
	old := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	past, future := now.Unix()-86400, now.Unix()+10*86400
	users := []model.User{
		{ID: 1, Email: "a@example.test", Token: "token-a", UUID: "uuid-a", U: 100, D: 200, CreatedAt: old},
		{ID: 2, Email: "b@example.test", Token: "token-b", UUID: "uuid-b", U: 1000, D: 3000, ExpiredAt: &future, CreatedAt: old},
		{ID: 3, Email: "c@example.test", Token: "token-c", UUID: "uuid-c", U: 7, ExpiredAt: &past, CreatedAt: old},
		{ID: 4, Email: "d@example.test", Token: "token-d", UUID: "uuid-d", ExpiredAt: &past, Banned: 1, CreatedAt: old},
		{ID: 5, Email: "e@example.test", Token: "token-e", UUID: "uuid-e", D: 50, CreatedAt: now},
		{ID: 6, Email: "f@example.test", Token: "token-f", UUID: "uuid-f", Banned: 1, CreatedAt: now},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&model.Plan{ID: 1, Name: "Basic", CreatedAt: old, UpdatedAt: old}).Error)
	paidNow, paidBefore := now.Unix(), now.Unix()-40*86400
	orders := []model.Order{
		{ID: 1, PlanID: 1, UserID: 1, TradeNo: "T1", TotalAmount: 1000, Status: 0},
		{ID: 2, PlanID: 1, UserID: 1, TradeNo: "T2", TotalAmount: 2000, Status: 1, PaidAt: &paidNow},
		{ID: 3, PlanID: 1, UserID: 2, TradeNo: "T3", TotalAmount: 4000, Status: 3, PaidAt: &paidBefore},
		{ID: 4, PlanID: 1, UserID: 2, TradeNo: "T4", TotalAmount: 8000, Status: 3, PaidAt: &paidNow},
		{ID: 5, PlanID: 1, UserID: 3, TradeNo: "T5", TotalAmount: 16000, Status: 2},
		{ID: 6, PlanID: 1, UserID: 5, TradeNo: "T6", TotalAmount: 32000, Status: 0},
	}
	require.NoError(t, db.Create(&orders).Error)
	recent, stale := now.Unix()-60, now.Unix()-3600
	node := func(id uint, show int, checked *int64) model.BaseServer {
		return model.BaseServer{ID: id, Name: "node", Show: show, LastCheckAt: checked, CreatedAt: old, UpdatedAt: old}
	}
	require.NoError(t, db.Create(&[]model.ServerVMess{{BaseServer: node(1, 1, &recent)}, {BaseServer: node(2, 1, &stale)}}).Error)
	require.NoError(t, db.Create(&[]model.ServerVLESS{{BaseServer: node(1, 1, &recent)}, {BaseServer: node(2, 1, nil)}}).Error)
	require.NoError(t, db.Create(&[]model.ServerTrojan{{BaseServer: node(1, 1, &stale)}}).Error)
	require.NoError(t, db.Create(&[]model.ServerShadowsocks{{BaseServer: node(1, 1, &recent)}, {BaseServer: node(2, 1, &recent)}}).Error)
	// A hidden node of each table, recently checked, counts nowhere.
	require.NoError(t, db.Create(&model.ServerVMess{BaseServer: node(9, 0, &recent)}).Error)
	require.NoError(t, db.Create(&model.ServerVLESS{BaseServer: node(9, 0, &recent)}).Error)
	require.NoError(t, db.Create(&model.ServerTrojan{BaseServer: node(9, 0, &recent)}).Error)
	require.NoError(t, db.Create(&model.ServerShadowsocks{BaseServer: node(9, 0, &recent)}).Error)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()
	logs := []model.TrafficLog{
		{ID: 1, UserID: 1, ServerID: 1, ServerType: "vmess", U: 300, D: 700, Rate: 1, LogAt: today + 1},
		{ID: 2, UserID: 2, ServerID: 1, ServerType: "vless", U: 50, D: 50, Rate: 2, LogAt: now.Unix()},
		{ID: 3, UserID: 2, ServerID: 1, ServerType: "vless", U: 9000, D: 9000, Rate: 1, LogAt: today - 3600},
	}
	for i := range logs {
		logs[i].CreatedAt = time.Unix(logs[i].LogAt, 0).UTC()
	}
	require.NoError(t, db.Create(&logs).Error)
}

// withSnapshot also caches a snapshot, as the kernel does after a build.
func withSnapshot(cachedAt time.Time, ttl time.Duration) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedDashboard(t, db)
		require.NoError(t, cache.Set(service.CacheKeyDashboardStats, seededSnapshot(cachedAt), ttl))
	}
}

// emptyDashboard has the tables, no rows and an empty alive set.
func emptyDashboard(t testing.TB, _ *gorm.DB) {
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)
}

// brokenDashboard has no v2_server_trojan, so building fails.
func brokenDashboard(t testing.TB, db *gorm.DB) {
	seedDashboard(t, db)
	require.NoError(t, db.Migrator().DropTable(&model.ServerTrojan{}))
}

// dashboardCache is the snapshot the kernel holds after the request: its
// counts, and whether its cached_at is the seeded one or the time of a
// build ("<now>").
func dashboardCache(t testing.TB, _ *gorm.DB) any {
	entry, err := cache.Get(service.CacheKeyDashboardStats)
	if errors.Is(err, cache.ErrKeyNotFound) {
		return "none"
	}
	require.NoError(t, err)
	snapshot := *entry.(*service.DashboardStats)
	cachedAt := "<now>"
	switch {
	case snapshot.CachedAt.Equal(seededAt):
		cachedAt = "<seeded>"
	case time.Since(snapshot.CachedAt) > time.Minute:
		cachedAt = snapshot.CachedAt.String()
	}
	snapshot.CachedAt = time.Time{}
	return map[string]any{"snapshot": snapshot, "cached_at": cachedAt}
}

// rebuilt is dashboardCache for a case whose side builds a snapshot from
// the seeded rows: the build must succeed on every backend and count them,
// or two failing sides would agree.
func rebuilt(t testing.TB, db *gorm.DB) any {
	state := dashboardCache(t, db)
	cached, ok := state.(map[string]any)
	require.True(t, ok, "the side built and cached a snapshot")
	snapshot := cached["snapshot"].(service.DashboardStats)
	require.Equal(t, "<now>", cached["cached_at"])
	require.EqualValues(t, 6, snapshot.TotalUsers)
	require.EqualValues(t, 7, snapshot.TotalNodes)
	require.EqualValues(t, 3, snapshot.OnlineUsers)
	require.EqualValues(t, 1200, snapshot.TodayTraffic)
	return state
}

func TestDashboardRouteParity(t *testing.T) {
	path := "/api/v2/admin/dashboard"
	built := []string{"data.cached_at"}
	utc := time.Date(2026, 10, 1, 0, 0, 1, 0, time.UTC)
	cases := []packagecompat.Case{
		// No snapshot cached: each side builds one from its rows, answers
		// it and caches it.
		{Name: "built from the rows", Path: path, Seed: seedDashboard, Mask: built, Snapshot: rebuilt},
		{Name: "built from no rows", Path: path, Seed: emptyDashboard, Mask: built},
		{Name: "built on refresh", Path: path + "?refresh=true", Seed: seedDashboard, Mask: built, Snapshot: rebuilt},
		// A cached snapshot is answered as cached, cached_at included,
		// unless refresh=true (gin's c.Query: the first value, exactly
		// "true") rebuilds it.
		{Name: "cached", Path: path, Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		{Name: "cached in UTC on a whole second", Path: path, Seed: withSnapshot(utc, service.StatsCacheTTL)},
		{Name: "cached, refresh=true", Path: path + "?refresh=true", Seed: withSnapshot(seededAt, service.StatsCacheTTL), Mask: built, Snapshot: rebuilt},
		{Name: "cached, refresh=1", Path: path + "?refresh=1", Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		{Name: "cached, refresh=TRUE", Path: path + "?refresh=TRUE", Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		{Name: "cached, refresh empty", Path: path + "?refresh=", Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		{Name: "cached, refresh=false", Path: path + "?refresh=false", Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		{Name: "cached, refresh=true twice", Path: path + "?refresh=true&refresh=false", Seed: withSnapshot(seededAt, service.StatsCacheTTL), Mask: built, Snapshot: rebuilt},
		{Name: "cached, refresh=false first", Path: path + "?refresh=false&refresh=true", Seed: withSnapshot(seededAt, service.StatsCacheTTL)},
		// An expired snapshot is rebuilt.
		{Name: "expired", Path: path, Seed: withSnapshot(seededAt, time.Nanosecond), Mask: built, Snapshot: rebuilt},
		// A failed build answers the kernel's error and caches nothing.
		{Name: "build fails", Path: path, Seed: brokenDashboard},
		{Name: "build fails, a snapshot cached", Path: path, Seed: func(t testing.TB, db *gorm.DB) {
			brokenDashboard(t, db)
			require.NoError(t, cache.Set(service.CacheKeyDashboardStats, seededSnapshot(seededAt), service.StatsCacheTTL))
		}},
	}
	route := dashboardRoute(t)
	for _, c := range cases {
		c.Principal = admin
		if c.Snapshot == nil {
			c.Snapshot = dashboardCache
		}
		packagecompat.RunWrite(t, route, c)
	}
}

// The rows seeded for the dashboard make every count nonzero, so the
// parity cases compare each field; the alive set is counted, its members
// are not answered.
func TestSeededDashboardCountsEveryField(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dashboard.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(dashboardRoute(t).Models...))
	seedDashboard(t, db)
	service := &native.Service{Telemetry: kernelTelemetry(t, db)}
	response, err := service.Dashboard(context.Background(), pluginhostsdk.NativeRequest{})
	require.NoError(t, err)
	var answer struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &answer))
	require.Len(t, answer.Data, 17)
	for field, value := range answer.Data {
		require.NotZero(t, value, field)
	}
	counts := map[string]float64{
		"total_users": 6, "active_users": 3, "expired_users": 2, "banned_users": 2, "today_new_users": 2,
		"total_orders": 6, "pending_orders": 2, "paid_orders": 3, "total_revenue": 14000, "monthly_income": 10000, "today_income": 10000,
		"total_nodes": 7, "active_nodes": 4, "online_users": 3, "total_traffic_used": 4357, "today_traffic": 1200,
	}
	for field, count := range counts {
		require.Equal(t, count, answer.Data[field], field)
	}
}
