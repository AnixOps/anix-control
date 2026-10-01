package kernelsubscriber

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// summaryFixture is the fixture with the plan and system configuration
// tables, a plan for user 1, link settings, and a fresh kernel cache.
func summaryFixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kernelsubscriberv1.KernelSubscriberServer) {
	t.Helper()
	db, _ := fixture(t, grants{})
	require.NoError(t, db.AutoMigrate(&model.Plan{}, &model.SystemConfig{}))
	require.NoError(t, db.Create(&model.Plan{ID: 6, Name: "Pro"}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).UpdateColumns(map[string]any{"plan_id": 6, "u": 10, "d": 30}).Error)
	require.NoError(t, db.Create(&model.SystemConfig{Key: service.SystemConfigKeySubscribeDomains, Value: `["sub.example.test"]`}).Error)
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)
	server := &Server{DB: db, Authorizer: authorizer, Config: func() *config.Config {
		return &config.Config{App: config.AppConfig{SubscribePath: "/feed/"}}
	}}
	return db, server.For(packagebridge.HostIdentity{PackageID: "subscription", Version: "4.1.0", Generation: 1})
}

func TestSubscriptionSummaryNeedsItsCapability(t *testing.T) {
	ctx := context.Background()
	for name, authorizer := range map[string]Authorizer{"other families": allFamilies, "fenced generation": fenced{}} {
		t.Run(name, func(t *testing.T) {
			_, server := summaryFixture(t, authorizer)
			_, err := server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 1})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			_, err = cache.Get(fmt.Sprintf("%s%d", service.CacheKeyUserSubscription, 1))
			require.ErrorIs(t, err, cache.ErrKeyNotFound, "a refused call builds nothing")
		})
	}
}

func TestSubscriptionSummaryAnswersTheKernelsCachedEntry(t *testing.T) {
	db, server := summaryFixture(t, grants{service.CapabilitySubscriberSummary: true})
	ctx := context.Background()

	first, err := server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, first.GetUserId())
	require.Equal(t, "a@x", first.GetEmail())
	require.EqualValues(t, 6, first.GetPlanId())
	require.Equal(t, "Pro", first.GetPlanName())
	require.EqualValues(t, 40, first.GetUsedTraffic())
	require.InDelta(t, 40.0, first.GetUsagePercent(), 1e-9)
	require.Equal(t, "/feed", first.GetSubscribePath())
	require.Equal(t, []string{"sub.example.test"}, first.GetSubscribeDomains())

	// The answer is the kernel's cache entry, the one its legacy handlers
	// answer: the same time, to the nanosecond and offset.
	entry, err := cache.Get(fmt.Sprintf("%s%d", service.CacheKeyUserSubscription, 1))
	require.NoError(t, err)
	cached := entry.(*service.UserSubscription)
	require.Equal(t, cached.CachedAt.Format(time.RFC3339Nano), first.GetCachedAt())
	require.Empty(t, cached.SubscribePath, "the link settings are not cached")
	require.Empty(t, cached.SubscribeDomains)

	// Within the cache's 30 seconds a change is not seen, as in legacy mode;
	// the link settings are read on every call.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).UpdateColumn("u", 60).Error)
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("1 = 1").UpdateColumn("value", "other.example.test").Error)
	second, err := server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 1})
	require.NoError(t, err)
	require.Equal(t, first.GetCachedAt(), second.GetCachedAt())
	require.EqualValues(t, 40, second.GetUsedTraffic())
	require.Equal(t, []string{"other.example.test"}, second.GetSubscribeDomains())

	refreshed, err := server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 1, Refresh: true})
	require.NoError(t, err)
	require.EqualValues(t, 90, refreshed.GetUsedTraffic())
	require.NotEqual(t, first.GetCachedAt(), refreshed.GetCachedAt())
	entry, err = cache.Get(fmt.Sprintf("%s%d", service.CacheKeyUserSubscription, 1))
	require.NoError(t, err)
	require.Equal(t, entry.(*service.UserSubscription).CachedAt.Format(time.RFC3339Nano), refreshed.GetCachedAt(), "refresh replaces the cached entry")
}

func TestSubscriptionSummaryOfASubscriberWithoutAPlan(t *testing.T) {
	_, server := summaryFixture(t, grants{service.CapabilitySubscriberSummary: true})
	summary, err := server.GetSubscriptionSummary(context.Background(), &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 2})
	require.NoError(t, err)
	require.Nil(t, summary.PlanId)
	require.Equal(t, "无套餐", summary.GetPlanName())
	require.EqualValues(t, -1, summary.GetDaysRemaining(), "never expires")
}

func TestSubscriptionSummaryRefusesUnknownAndInvalidSubscribers(t *testing.T) {
	_, server := summaryFixture(t, grants{service.CapabilitySubscriberSummary: true})
	ctx := context.Background()
	_, err := server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: 99})
	require.Equal(t, codes.NotFound, status.Code(err))
	for _, id := range []uint64{0, 1 << 32} {
		_, err = server.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{UserId: id})
		require.Equal(t, codes.InvalidArgument, status.Code(err), "user id %d", id)
	}
}

// The summary carries no credential: no field of the answer names a token,
// a uuid or a key.
func TestSubscriptionSummaryCarriesNoCredential(t *testing.T) {
	fields := (&kernelsubscriberv1.GetSubscriptionSummaryResponse{}).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		name := string(fields.Get(i).Name())
		for _, credential := range []string{"token", "uuid", "key", "secret", "password"} {
			require.NotContains(t, strings.ToLower(name), credential)
		}
	}
}
