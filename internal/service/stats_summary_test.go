package service

import (
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The subscription summary adds the link settings to a copy of the cached
// entry. The entry is the one the user dashboard answers, so writing them
// into it showed them on the dashboard for 30 seconds after a summary read
// (and raced concurrent readers); the dashboard never shows them now, in
// either mode.
func TestUserSubscriptionSummaryLeavesTheCachedEntryAlone(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "summary.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Plan{}, &model.User{}, &model.SystemConfig{}))
	require.NoError(t, db.Create(&model.User{ID: 2, Email: "b@x", Token: "token-b", UUID: "uuid-b"}).Error)
	require.NoError(t, db.Create(&model.SystemConfig{Key: SystemConfigKeySubscribeDomains, Value: "sub.example.test"}).Error)
	cache.InitMemory()
	t.Cleanup(cache.CloseMemory)

	stats := NewStatsServiceOn(db)
	summary, err := stats.UserSubscriptionSummary(2, false, NewSystemConfigService(db), &config.Config{})
	require.NoError(t, err)
	require.Equal(t, "/s", summary.SubscribePath)
	require.Equal(t, []string{"sub.example.test"}, summary.SubscribeDomains)

	dashboard, err := stats.GetUserSubscription(2, false)
	require.NoError(t, err)
	require.Equal(t, summary.CachedAt, dashboard.CachedAt, "the dashboard reads the summary's cached entry")
	require.Empty(t, dashboard.SubscribePath)
	require.Empty(t, dashboard.SubscribeDomains)
}
