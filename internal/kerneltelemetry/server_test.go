package kerneltelemetry

import (
	"context"
	"testing"
	"time"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type grants map[string]bool

func (g grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if g[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

type fenced struct{}

func (fenced) AuthorizeCapability(context.Context, packagebridge.HostIdentity, string) error {
	return packagebridge.ErrHostFenced
}

var dashboardOnly = grants{service.CapabilityTelemetryDashboard: true}

// fixture has two users, a paid order, a shown node and a fresh kernel
// cache.
func fixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kerneltelemetryv1.KernelTelemetryServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Order{}, &model.ServerVMess{}, &model.ServerVLESS{},
		&model.ServerTrojan{}, &model.ServerShadowsocks{}, &model.TrafficLog{}))
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "a@x", Token: "token-a", UUID: "uuid-a", U: 5, D: 7},
		{ID: 2, Email: "b@x", Token: "token-b", UUID: "uuid-b", Banned: 1},
	}).Error)
	paid := time.Now().Unix()
	require.NoError(t, db.Create(&model.Order{ID: 1, UserID: 1, TradeNo: "T1", TotalAmount: 990, Status: 3, PaidAt: &paid}).Error)
	require.NoError(t, db.Create(&model.ServerVMess{BaseServer: model.BaseServer{ID: 1, Name: "vm", Show: 1}}).Error)
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)
	server := &Server{DB: db, Authorizer: authorizer}
	return db, server.For(packagebridge.HostIdentity{PackageID: "machine-telemetry", Version: "4.1.0", Generation: 1})
}

func TestDashboardNeedsItsCapability(t *testing.T) {
	for name, authorizer := range map[string]Authorizer{"no grant": grants{service.CapabilitySubscriberSummary: true}, "fenced generation": fenced{}} {
		t.Run(name, func(t *testing.T) {
			_, server := fixture(t, authorizer)
			_, err := server.GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			_, err = cache.Get(service.CacheKeyDashboardStats)
			require.ErrorIs(t, err, cache.ErrKeyNotFound, "a refused call builds nothing")
		})
	}
	_, err := (&Server{}).For(packagebridge.HostIdentity{}).GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestDashboardAnswersTheKernelsCachedSnapshot(t *testing.T) {
	db, server := fixture(t, dashboardOnly)
	ctx := context.Background()
	require.NoError(t, cache.SAdd("alive:users", "1", "2", "3"))

	first, err := server.GetDashboard(ctx, &kerneltelemetryv1.GetDashboardRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 2, first.GetTotalUsers())
	require.EqualValues(t, 1, first.GetBannedUsers())
	require.EqualValues(t, 1, first.GetPaidOrders())
	require.EqualValues(t, 990, first.GetTotalRevenue())
	require.EqualValues(t, 1, first.GetTotalNodes())
	require.EqualValues(t, 3, first.GetOnlineUsers(), "the alive set leaves the kernel as a count")
	require.EqualValues(t, 12, first.GetTotalTrafficUsed())

	// The answer is the kernel's cached snapshot, the one its legacy
	// handler answers: the same time, to the nanosecond and offset.
	entry, err := cache.Get(service.CacheKeyDashboardStats)
	require.NoError(t, err)
	cached := entry.(*service.DashboardStats)
	require.Equal(t, cached.CachedAt.Format(time.RFC3339Nano), first.GetCachedAt())
	parsed, err := time.Parse(time.RFC3339Nano, first.GetCachedAt())
	require.NoError(t, err)
	require.True(t, parsed.Equal(cached.CachedAt))

	// Within the cache's 60 seconds a change is not seen, as in legacy
	// mode; refresh rebuilds the snapshot both modes read.
	require.NoError(t, db.Create(&model.User{ID: 3, Email: "c@x", Token: "token-c", UUID: "uuid-c"}).Error)
	second, err := server.GetDashboard(ctx, &kerneltelemetryv1.GetDashboardRequest{})
	require.NoError(t, err)
	require.Equal(t, first.GetCachedAt(), second.GetCachedAt())
	require.EqualValues(t, 2, second.GetTotalUsers())
	refreshed, err := server.GetDashboard(ctx, &kerneltelemetryv1.GetDashboardRequest{Refresh: true})
	require.NoError(t, err)
	require.EqualValues(t, 3, refreshed.GetTotalUsers())
	require.NotEqual(t, first.GetCachedAt(), refreshed.GetCachedAt())
	entry, err = cache.Get(service.CacheKeyDashboardStats)
	require.NoError(t, err)
	require.Equal(t, entry.(*service.DashboardStats).CachedAt.Format(time.RFC3339Nano), refreshed.GetCachedAt(), "refresh replaces the cached snapshot")
}

// A failed build answers the kernel's error, which the v2 route shows the
// administrator, and caches nothing.
func TestDashboardAnswersTheKernelsError(t *testing.T) {
	db, server := fixture(t, dashboardOnly)
	require.NoError(t, db.Migrator().DropTable(&model.ServerTrojan{}))
	_, err := server.GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "count trojan nodes: SQL logic error: no such table: v2_server_trojan (1)", status.Convert(err).Message())
	_, err = cache.Get(service.CacheKeyDashboardStats)
	require.ErrorIs(t, err, cache.ErrKeyNotFound)
}
