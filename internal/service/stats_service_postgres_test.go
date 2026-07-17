package service

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/cache"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStatsServicePostgresLargeTrafficAggregates(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("POSTGRES_TEST_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}

	models := []any{
		&model.TrafficLog{},
		&model.User{},
		&model.Order{},
		&model.ServerVMess{},
		&model.ServerVLESS{},
		&model.ServerTrojan{},
		&model.ServerShadowsocks{},
	}
	require.NoError(t, db.Migrator().DropTable(models...))
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(models...)
	})

	cache.InitMemory()
	svc := &StatsService{db: db}

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	previousHour := currentHour - hourSeconds

	u1 := &model.User{Email: "pg-large-1@example.com", Token: "pg-large-token-1", UUID: "pg-large-uuid-1"}
	u2 := &model.User{Email: "pg-large-2@example.com", Token: "pg-large-token-2", UUID: "pg-large-uuid-2"}
	require.NoError(t, db.Create(u1).Error)
	require.NoError(t, db.Create(u2).Error)

	const gib = int64(1073741824)
	logs := []model.TrafficLog{
		{UserID: u1.ID, ServerID: 1, ServerType: "node", U: 3 * gib, D: 2 * gib, Rate: 1, LogAt: currentHour + 10},
		{UserID: u2.ID, ServerID: 1, ServerType: "node", U: gib, D: 0, Rate: 2, LogAt: currentHour + 20},
		{UserID: u1.ID, ServerID: 1, ServerType: "node", U: 512, D: 256, Rate: 1, LogAt: previousHour + 30},
	}
	for i := range logs {
		require.NoError(t, db.Create(&logs[i]).Error)
	}

	series, err := svc.GetHourlyTraffic(2, 0)
	require.NoError(t, err)
	require.Len(t, series, 2)
	require.Equal(t, previousHour, series[0].HourTs)
	require.Equal(t, int64(768), series[0].Traffic)
	require.Equal(t, currentHour, series[1].HourTs)
	require.Equal(t, 7*gib, series[1].Traffic)

	ranking, err := svc.GetUserTrafficRanking(24, 10, true)
	require.NoError(t, err)
	require.Len(t, ranking, 2)
	require.Equal(t, u1.ID, ranking[0].UserID)
	require.Equal(t, 5*gib+768, ranking[0].Traffic)
	require.Equal(t, u2.ID, ranking[1].UserID)
	require.Equal(t, 2*gib, ranking[1].Traffic)

	stats, err := svc.fetchDashboardFromDB()
	require.NoError(t, err)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()
	expectedTodayTraffic := 7 * gib
	// At midnight previousHour belongs to the prior calendar day and must not be
	// included in the dashboard's today-only aggregate.
	if previousHour >= todayStart {
		expectedTodayTraffic += 768
	}
	require.Equal(t, expectedTodayTraffic, stats.TodayTraffic)
}

func TestStatsServicePostgresSanitizesAndClampsTrafficAggregates(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("POSTGRES_TEST_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}

	models := []any{
		&model.TrafficLog{},
		&model.User{},
		&model.Order{},
		&model.ServerVMess{},
		&model.ServerVLESS{},
		&model.ServerTrojan{},
		&model.ServerShadowsocks{},
	}
	require.NoError(t, db.Migrator().DropTable(models...))
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(models...)
	})

	cache.InitMemory()
	svc := &StatsService{db: db}

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	nextHour := currentHour + hourSeconds
	const maxInt64 = int64(1<<63 - 1)

	dirty := &model.User{Email: "pg-dirty@example.com", Token: "pg-dirty-token", UUID: "pg-dirty-uuid"}
	huge := &model.User{Email: "pg-huge@example.com", Token: "pg-huge-token", UUID: "pg-huge-uuid"}
	require.NoError(t, db.Create(dirty).Error)
	require.NoError(t, db.Create(huge).Error)

	logs := []model.TrafficLog{
		{UserID: dirty.ID, ServerID: 1, ServerType: "node", U: -100, D: 200, Rate: 1, LogAt: currentHour + 10},
		{UserID: dirty.ID, ServerID: 1, ServerType: "node", U: 100, D: 100, Rate: -2, LogAt: currentHour + 20},
		{UserID: dirty.ID, ServerID: 1, ServerType: "node", U: 3, D: 0, Rate: 0.5, LogAt: currentHour + 30},
		{UserID: dirty.ID, ServerID: 1, ServerType: "node", U: 9999, D: 9999, Rate: 1, LogAt: nextHour + 10},
		{UserID: huge.ID, ServerID: 1, ServerType: "node", U: maxInt64 - 10, D: 100, Rate: 1, LogAt: currentHour + 40},
	}
	for i := range logs {
		require.NoError(t, db.Create(&logs[i]).Error)
	}

	dirtySeries, err := svc.GetHourlyTraffic(1, dirty.ID)
	require.NoError(t, err)
	require.Len(t, dirtySeries, 1)
	require.Equal(t, int64(401), dirtySeries[0].Traffic)

	hugeSeries, err := svc.GetHourlyTraffic(1, huge.ID)
	require.NoError(t, err)
	require.Len(t, hugeSeries, 1)
	require.Equal(t, maxInt64, hugeSeries[0].Traffic)

	ranking, err := svc.GetUserTrafficRanking(24, 10, true)
	require.NoError(t, err)
	require.Len(t, ranking, 2)
	require.Equal(t, huge.ID, ranking[0].UserID)
	require.Equal(t, maxInt64, ranking[0].Traffic)
	require.Equal(t, dirty.ID, ranking[1].UserID)
	require.Equal(t, int64(401), ranking[1].Traffic)

	stats, err := svc.fetchDashboardFromDB()
	require.NoError(t, err)
	require.Equal(t, maxInt64, stats.TodayTraffic)
}
