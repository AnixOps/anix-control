package destinationstats

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentreports"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testNow = time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC)

var gormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

func migrate(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.DestinationStat{}, &model.SystemConfig{}, &model.AgentReportBatch{}, &model.TrafficCategoryRule{}, &model.TrafficCategorySource{}, &model.OperationLog{}))
	return db
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_txlock=immediate&_pragma=journal_mode(WAL)"), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return migrate(t, db)
}

// openPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), gormConfig)
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
	schema := "destination_stats_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return migrate(t, db)
}

func forEachDialect(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { body(t, openSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openPostgres(t)) })
}

func enable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, SaveSettings(db, Settings{Enabled: true, RetentionDays: DefaultRetentionDays}))
}

func stats(t *testing.T, db *gorm.DB) []model.DestinationStat {
	t.Helper()
	var rows []model.DestinationStat
	require.NoError(t, db.Order("user_id, domain, protocol").Find(&rows).Error)
	return rows
}

func TestSettingsDefaultOffAndBoundTheRetention(t *testing.T) {
	db := openSQLite(t)
	settings, err := LoadSettings(db)
	require.NoError(t, err)
	assert.Equal(t, Settings{Enabled: false, RetentionDays: 30}, settings)

	require.ErrorIs(t, SaveSettings(db, Settings{Enabled: true, RetentionDays: 6}), ErrRetentionDays)
	require.ErrorIs(t, SaveSettings(db, Settings{Enabled: true, RetentionDays: 181}), ErrRetentionDays)
	require.NoError(t, SaveSettings(db, Settings{Enabled: true, RetentionDays: 90}))
	require.NoError(t, SaveSettings(db, Settings{Enabled: true, RetentionDays: 60}))
	settings, err = LoadSettings(db)
	require.NoError(t, err)
	assert.Equal(t, Settings{Enabled: true, RetentionDays: 60}, settings)
	assert.Equal(t, 60*24*time.Hour, settings.Retention())

	// A value an administrator mistyped in the system settings falls back
	// to the default.
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("key = ?", RetentionDaysKey).Update("value", "forever").Error)
	settings, err = LoadSettings(db)
	require.NoError(t, err)
	assert.Equal(t, 30, settings.RetentionDays)
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct{ domain, protocol, want string }{
		{"youtube.com", "tls", CategoryVideo},
		{"googlevideo.com", "quic", CategoryVideo},
		{"telegram.org", "tls", CategoryMessaging},
		{"149.154.167.0/24", "", CategoryMessaging},
		{"91.108.56.130", "", CategoryMessaging},
		{"2001:b28:f23d::/48", "", CategoryMessaging},
		{"2001:b28:f23d::/40", "", CategoryUnknown},
		{"8.8.8.0/24", "", CategoryUnknown},
		{"chatgpt.com", "tls", CategoryAI},
		{"spotify.com", "tls", CategoryMusic},
		{"steamcontent.com", "http", CategoryDownload},
		{"example.com", "bittorrent", CategoryDownload},
		{"", "bittorrent", CategoryDownload},
		{"x.com", "tls", CategorySocial},
		{"steampowered.com", "tls", CategoryGaming},
		{"example.com", "tls", CategoryWeb},
		{"google.com", "tls", CategoryWeb},
		{"", "", CategoryUnknown},
		// A rule matches its subdomains too, should an Agent report one.
		{"rr3---sn-a5m7ln7d.googlevideo.com", "tls", CategoryVideo},
		{"notyoutube.com", "tls", CategoryWeb},
	} {
		assert.Equal(t, tc.want, Classify(tc.domain, tc.protocol), "%s %s", tc.domain, tc.protocol)
	}
	for _, category := range []string{CategoryVideo, CategoryMusic, CategoryMessaging, CategorySocial, CategoryGaming, CategoryAI, CategoryDownload} {
		found := false
		for _, rule := range domainRules {
			found = found || rule == category
		}
		assert.True(t, found, "category %s has rules", category)
	}
	for domain := range domainRules {
		assert.Equal(t, strings.ToLower(domain), domain)
	}
}

func TestHourOf(t *testing.T) {
	hour := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC).Unix()
	assert.Equal(t, hour, HourOf(time.Time{}, testNow), "no window end: now")
	assert.Equal(t, hour, HourOf(testNow.Add(-time.Minute), testNow))
	assert.Equal(t, hour-3600, HourOf(time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC), testNow), "a window ending on the hour belongs to the hour before")
	assert.Equal(t, hour, HourOf(testNow.Add(time.Hour), testNow), "a window from the future: now")
	assert.Equal(t, hour-48*3600, HourOf(testNow.Add(-48*time.Hour), testNow), "a resent old batch keeps its hour")
}

func TestRecordSumsABatchOnceAndAddsToTheHour(t *testing.T) {
	forEachDialect(t, func(t *testing.T, db *gorm.DB) {
		enable(t, db)
		entries := []Entry{
			{UserID: 7, Domain: "YouTube.com.", Protocol: "TLS", Upload: 10, Download: 1000, Connections: 2},
			{UserID: 7, Domain: "youtube.com", Protocol: "tls", Upload: 5, Download: 500, Connections: 1},
			{UserID: 7, Domain: "149.154.167.0/24", Upload: 1, Download: 2, Connections: 1},
			{UserID: 8, Domain: "", Upload: 3, Download: 4},
		}
		applied, err := Record(db, "proxy", 3, "node:proxy-3:boot:destinations-1", testNow, entries, testNow)
		require.NoError(t, err)
		require.True(t, applied)
		applied, err = Record(db, "proxy", 3, "node:proxy-3:boot:destinations-1", testNow, entries, testNow)
		require.NoError(t, err)
		assert.False(t, applied, "a resent batch counts once")

		applied, err = Record(db, "proxy", 3, "node:proxy-3:boot:destinations-2", testNow, entries[:1], testNow)
		require.NoError(t, err)
		require.True(t, applied)

		hour := HourOf(testNow, testNow)
		rows := stats(t, db)
		require.Len(t, rows, 3)
		assert.Equal(t, model.DestinationStat{ID: rows[0].ID, HourAt: hour, NodeID: 3, UserID: 7, Domain: "149.154.167.0/24", Upload: 1, Download: 2, Connections: 1}, rows[0])
		assert.Equal(t, model.DestinationStat{ID: rows[1].ID, HourAt: hour, NodeID: 3, UserID: 7, Domain: "youtube.com", Protocol: "tls", Upload: 25, Download: 2500, Connections: 5}, rows[1])
		assert.Equal(t, model.DestinationStat{ID: rows[2].ID, HourAt: hour, NodeID: 3, UserID: 8, Upload: 3, Download: 4}, rows[2])
	})
}

func TestRecordDropsBatchesWhileOff(t *testing.T) {
	db := openSQLite(t)
	applied, err := Record(db, "proxy", 3, "node:proxy-3:boot:destinations-1", testNow, []Entry{{UserID: 7, Domain: "example.com", Upload: 1}}, testNow)
	require.NoError(t, err)
	assert.True(t, applied, "claimed, so the Agent drops it")
	assert.Empty(t, stats(t, db))
	var claims int64
	require.NoError(t, db.Model(&model.AgentReportBatch{}).Where("kind = ?", agentreports.KindDestinations).Count(&claims).Error)
	assert.Equal(t, int64(1), claims)
}

func TestRecordRefusesInvalidBatches(t *testing.T) {
	db := openSQLite(t)
	enable(t, db)
	record := func(batchID string, entries ...Entry) error {
		_, err := Record(db, "proxy", 3, batchID, testNow, entries, testNow)
		return err
	}
	require.ErrorIs(t, record(""), agentreports.ErrInvalidBatchID)
	require.ErrorIs(t, record("b1", Entry{Domain: "example.com"}), ErrInvalid)
	require.ErrorIs(t, record("b2", Entry{UserID: 1, Domain: strings.Repeat("a", 256)}), ErrInvalid)
	require.ErrorIs(t, record("b3", Entry{UserID: 1, Protocol: strings.Repeat("p", 33)}), ErrInvalid)
	require.ErrorIs(t, record("b4", Entry{UserID: 1, Upload: 1 << 63}), ErrOverflow)
	require.ErrorIs(t, record("b5", Entry{UserID: 1, Upload: 1 << 62}, Entry{UserID: 1, Upload: 1 << 62}), ErrOverflow)
	require.ErrorIs(t, record("b6", make([]Entry, MaxEntries+1)...), ErrInvalid)
	assert.Empty(t, stats(t, db))
	var claims int64
	require.NoError(t, db.Model(&model.AgentReportBatch{}).Count(&claims).Error)
	assert.Zero(t, claims, "a refused batch is not claimed")
	assert.False(t, errors.Is(ErrInvalid, ErrOverflow))
}

func TestSummarize(t *testing.T) {
	forEachDialect(t, func(t *testing.T, db *gorm.DB) {
		hour := HourOf(testNow, testNow)
		rows := []model.DestinationStat{
			{HourAt: hour, NodeID: 1, UserID: 7, Domain: "youtube.com", Protocol: "quic", Upload: 10, Download: 900, Connections: 3},
			{HourAt: hour - 3600, NodeID: 2, UserID: 7, Domain: "youtube.com", Protocol: "tls", Upload: 10, Download: 90, Connections: 1},
			{HourAt: hour, NodeID: 1, UserID: 7, Domain: "example.com", Protocol: "bittorrent", Upload: 400, Download: 600, Connections: 9},
			{HourAt: hour, NodeID: 1, UserID: 7, Domain: "example.com", Protocol: "tls", Upload: 1, Download: 1, Connections: 1},
			{HourAt: hour, NodeID: 1, UserID: 8, Domain: "telegram.org", Protocol: "tls", Upload: 50, Download: 50, Connections: 2},
			{HourAt: hour, NodeID: 2, UserID: 8, Domain: "", Upload: 5, Download: 5},
			// Outside the range.
			{HourAt: hour - 48*3600, NodeID: 1, UserID: 7, Domain: "netflix.com", Protocol: "tls", Upload: 1, Download: 1 << 30},
		}
		require.NoError(t, db.Create(&rows).Error)

		query := LastHours(24, testNow)
		query.UserID = 7
		summary, err := Summarize(db, query)
		require.NoError(t, err)
		assert.Equal(t, time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC).UnixMilli(), summary.SinceUnixMs)
		assert.Equal(t, time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC).UnixMilli(), summary.UntilUnixMs)
		assert.Equal(t, Totals{Upload: 421, Download: 1591, Connections: 14}, summary.Totals)
		assert.Equal(t, []CategoryTotal{
			{Category: CategoryVideo, Totals: Totals{Upload: 20, Download: 990, Connections: 4}},
			{Category: CategoryDownload, Totals: Totals{Upload: 400, Download: 600, Connections: 9}},
			{Category: CategoryWeb, Totals: Totals{Upload: 1, Download: 1, Connections: 1}},
		}, summary.Categories)
		assert.Equal(t, []DomainTotal{
			{Domain: "youtube.com", Category: CategoryVideo, Totals: Totals{Upload: 20, Download: 990, Connections: 4}},
			{Domain: "example.com", Category: CategoryDownload, Totals: Totals{Upload: 401, Download: 601, Connections: 10}},
		}, summary.Domains)
		assert.Equal(t, 2, summary.DomainCount)

		all, err := Summarize(db, Query{Since: query.Since, Until: query.Until, NodeID: 2, Limit: 1})
		require.NoError(t, err)
		assert.Equal(t, Totals{Upload: 15, Download: 95, Connections: 1}, all.Totals)
		assert.Equal(t, 2, all.DomainCount)
		require.Len(t, all.Domains, 1)
		assert.Equal(t, "youtube.com", all.Domains[0].Domain)
		assert.Equal(t, []CategoryTotal{
			{Category: CategoryVideo, Totals: Totals{Upload: 10, Download: 90, Connections: 1}},
			{Category: CategoryUnknown, Totals: Totals{Upload: 5, Download: 5}},
		}, all.Categories)

		empty, err := Summarize(db, Query{Since: query.Since, Until: query.Until, UserID: 99})
		require.NoError(t, err)
		assert.Empty(t, empty.Categories)
		assert.NotNil(t, empty.Categories)
		assert.NotNil(t, empty.Domains)
	})
}

func TestLastHoursBounds(t *testing.T) {
	assert.Equal(t, 24*time.Hour, LastHours(0, testNow).Until.Sub(LastHours(0, testNow).Since))
	assert.Equal(t, time.Duration(MaxHours)*time.Hour, LastHours(100000, testNow).Until.Sub(LastHours(100000, testNow).Since))
}
