package subscriptioncompat

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The summary's parity: the legacy handler answers the user's summary the
// kernel caches for 30 seconds, with the subscription link settings; the
// native one reads it through the real KernelSubscriber server, served in
// process over gRPC on the native side's database. The cache is the
// kernel's memory, one per process: each side's seed starts it afresh, so a
// case's sides do not share it, and a seeded summary is answered byte for
// byte, cached_at included. A summary a side builds itself has that side's
// cached_at, which is masked; the cache entry it leaves is compared.

func summaryRoute(t *testing.T) packagecompat.Route {
	return packagecompat.Route{
		Method: "GET", Pattern: "/api/v2/user/subscription", RouteID: native.SummaryRouteID,
		// The legacy handler and its StatsService are built per request: the
		// service keeps the database it was first built with.
		Legacy: func(c *gin.Context) {
			service.ResetStatsServiceForTest()
			handler.NewUserHandler().GetSubscription(c)
		},
		Models: []any{&model.Plan{}, &model.User{}, &model.SystemConfig{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open:       func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Subscriber: kernelSubscriber(t, db),
			}
			return service.Handlers()[native.SummaryRouteID]
		},
	}
}

// summaryLater is an expiry ten and a half days ahead, far from a day
// boundary, so both sides count the same days remaining.
var summaryLater = expiryBase + 10*86400 + 43200

// cachedAt is the cached_at of a seeded summary: a fraction and an offset,
// both of which an answer must keep.
var cachedAt = time.Date(2026, 9, 30, 23, 59, 58, 987654321, time.FixedZone("", -5*3600))

// seedSummary starts the kernel's cache afresh and writes two plans, the
// subscribers the cases read and the subscription link domains:
//   - 2 has plan 2, traffic used a third of its transfer, and an expiry;
//   - 3 has no plan, no transfer and no expiry;
//   - 4 has plan 1 and expired an hour ago;
//   - 5 used more than its transfer.
func seedSummary(t testing.TB, db *gorm.DB) {
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)
	require.NoError(t, db.Create(&[]model.Plan{
		{ID: 1, Name: "Basic", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "Pro", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	user := func(id uint) model.User {
		return model.User{
			ID: id, Email: fmt.Sprintf("user%d@example.test", id), Token: fmt.Sprintf("token-%d", id), UUID: fmt.Sprintf("uuid-%d", id),
			CreatedAt: seeded, UpdatedAt: seeded,
		}
	}
	users := []model.User{user(1), user(2), user(3), user(4), user(5)}
	users[1].PlanID, users[1].TransferEnable, users[1].U, users[1].D, users[1].ExpiredAt = ptr(uint(2)), 3000, 400, 600, &summaryLater
	users[2].U = 5
	users[3].PlanID, users[3].TransferEnable, users[3].U, users[3].D, users[3].ExpiredAt = ptr(uint(1)), 1<<30, 7, 9, ptr(expiryBase-3600)
	users[4].TransferEnable, users[4].U, users[4].D = 3, 10, 0
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&model.SystemConfig{
		Key: service.SystemConfigKeySubscribeDomains, Value: "sub.example.test, https://cdn.example.test/path", Type: "string",
		CreatedAt: seeded, UpdatedAt: seeded,
	}).Error)
}

// withCachedSummary also caches a summary for user, as the kernel does
// after a build. It differs from user 2's rows in every field.
func withCachedSummary(user uint, ttl time.Duration) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedSummary(t, db)
		require.NoError(t, cache.Set(fmt.Sprintf("%s%d", service.CacheKeyUserSubscription, user), &service.UserSubscription{
			UserID: user, Email: "cached@example.test", PlanID: ptr(uint(7)), PlanName: "Cached", TransferEnable: 50,
			UsedTraffic: 20, UploadTraffic: 5, DownloadTraffic: 15, ExpiredAt: 1, IsExpired: true, DaysRemaining: 0,
			UsagePercent: 40, CachedAt: cachedAt,
		}, ttl))
	}
}

// summaryCache is user's entry in the kernel's cache after the request:
// the summary, and whether its cached_at is the seeded one or the time of a
// build ("<now>"). The entry never holds the link settings.
func summaryCache(user uint) func(testing.TB, *gorm.DB) any {
	return func(t testing.TB, _ *gorm.DB) any {
		entry, err := cache.Get(fmt.Sprintf("%s%d", service.CacheKeyUserSubscription, user))
		if errors.Is(err, cache.ErrKeyNotFound) {
			return "none"
		}
		require.NoError(t, err)
		summary := *entry.(*service.UserSubscription)
		require.Empty(t, summary.SubscribePath, "the cached entry holds no link settings")
		require.Empty(t, summary.SubscribeDomains, "the cached entry holds no link settings")
		at := "<now>"
		switch {
		case summary.CachedAt.Equal(cachedAt):
			at = "<seeded>"
		case time.Since(summary.CachedAt) > time.Minute:
			at = summary.CachedAt.String()
		}
		summary.CachedAt = time.Time{}
		return map[string]any{"summary": summary, "cached_at": at}
	}
}

// withConfig runs the cases with cfg as the kernel's configuration, which
// the link settings read (app.subscribe_path).
func withConfig(t *testing.T, cfg *config.Config) {
	previous := config.Get()
	config.Set(cfg)
	t.Cleanup(func() { config.Set(previous) })
}

func summaryCases(t *testing.T, cases []packagecompat.Case) {
	route := summaryRoute(t)
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seedSummary
		}
		if c.Principal.ActorID == 0 && c.Name != "user id 0" {
			c.Principal = pluginhostsdk.Principal{ActorID: 2}
		}
		if c.Snapshot == nil {
			c.Snapshot = summaryCache(c.Principal.ActorID)
		}
		packagecompat.RunWrite(t, route, c)
	}
}

func TestUserSubscriptionSummaryParity(t *testing.T) {
	withConfig(t, &config.Config{App: config.AppConfig{SubscribePath: "feed/"}})
	path := "/api/v2/user/subscription"
	built := []string{"data.cached_at"}
	user := func(id uint) pluginhostsdk.Principal { return pluginhostsdk.Principal{ActorID: id} }
	summaryCases(t, []packagecompat.Case{
		// Nothing cached: each side builds the caller's summary, answers it
		// with the link settings and caches it without them.
		{Name: "plan, traffic and expiry", Path: path, Mask: built},
		{Name: "no plan, no transfer, never expires", Path: path, Principal: user(3), Mask: built},
		{Name: "expired", Path: path, Principal: user(4), Mask: built},
		{Name: "beyond its transfer", Path: path, Principal: user(5), Mask: built},
		{Name: "built on refresh", Path: path + "?refresh=true", Mask: built},
		{Name: "another user's summary cached", Path: path, Mask: built, Seed: withCachedSummary(3, service.SubscriptionCacheTTL)},
		{Name: "no link domains", Path: path, Mask: built, Seed: func(t testing.TB, db *gorm.DB) {
			seedSummary(t, db)
			require.NoError(t, db.Where("1 = 1").Delete(&model.SystemConfig{}).Error)
		}},
		{Name: "link domains unreadable", Path: path, Mask: built, Seed: func(t testing.TB, db *gorm.DB) {
			seedSummary(t, db)
			require.NoError(t, db.Migrator().DropTable(&model.SystemConfig{}))
		}},
		// A cached summary is answered as cached, cached_at included, with
		// the current link settings, unless refresh=true (gin's c.Query:
		// the first value, exactly "true") rebuilds it.
		{Name: "cached", Path: path, Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "cached, refresh=true", Path: path + "?refresh=true", Mask: built, Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "cached, refresh=1", Path: path + "?refresh=1", Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "cached, refresh=True", Path: path + "?refresh=True", Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "cached, refresh=false first", Path: path + "?refresh=false&refresh=true", Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "cached, refresh=true first", Path: path + "?refresh=true&refresh=false", Mask: built, Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
		{Name: "expired entry", Path: path, Mask: built, Seed: withCachedSummary(2, time.Nanosecond)},
		// An unknown caller and a failed build answer the kernel's
		// messages and cache nothing.
		{Name: "unknown user", Path: path, Principal: user(99)},
		{Name: "user id 0", Path: path},
		{Name: "user id beyond 32 bits", Path: path, Principal: user(1 << 32)},
		{Name: "build fails", Path: path, Seed: func(t testing.TB, db *gorm.DB) {
			seedSummary(t, db)
			require.NoError(t, db.Migrator().DropTable(&model.Plan{}))
		}},
	})
}

// Without a configured subscription path the link settings answer the
// default, "/s", on both sides.
func TestUserSubscriptionSummaryDefaultPathParity(t *testing.T) {
	withConfig(t, &config.Config{})
	summaryCases(t, []packagecompat.Case{
		{Name: "built", Path: "/api/v2/user/subscription", Mask: []string{"data.cached_at"}},
		{Name: "cached", Path: "/api/v2/user/subscription", Seed: withCachedSummary(2, service.SubscriptionCacheTTL)},
	})
}
