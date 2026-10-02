package shadowsamples

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/shadowsample"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(model.KernelModels()...))
	return db
}

// openPostgres opens a throwaway schema of the ANIX_TEST_POSTGRES_DSN
// database with the kernel tables, or skips.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
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
	schema := "shadowsamples_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.ShadowMismatchSample{}))
	return db
}

var testRoutes = map[string]RouteInfo{
	"knowledge.user.knowledge.id.get": {PackageID: "knowledge", Path: "/api/v2/user/knowledge/:id"},
	"knowledge.article.list":          {PackageID: "knowledge", Path: "/api/v2/admin/knowledge/fetch"},
	"ticket.user.fetch":               {PackageID: "ticket", Path: "/api/v2/user/ticket/fetch"},
}

func newCollector(db *gorm.DB, now *time.Time) *Collector {
	return &Collector{
		DB:    db,
		Route: func(id string) (RouteInfo, bool) { route, ok := testRoutes[id]; return route, ok },
		Now:   func() time.Time { return *now },
	}
}

func sampleID(index int) string {
	return fmt.Sprintf("%032x", index+1)
}

func report(t *testing.T, packageID string, samples ...any) Report {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"config": map[string]any{"status": "ok"}, "shadow_samples": samples})
	require.NoError(t, err)
	return Report{PackageID: packageID, Version: "1.0.0", DetailsJSON: string(encoded), CheckedAt: time.Now()}
}

func TestIngestSanitizesAgainAndStoresOnce(t *testing.T) {
	testIngestSanitizesAgainAndStoresOnce(t, openSQLite(t))
}

func TestPostgresIngestSanitizesAgainAndStoresOnce(t *testing.T) {
	testIngestSanitizesAgainAndStoresOnce(t, openPostgres(t))
}

func testIngestSanitizesAgainAndStoresOnce(t *testing.T, db *gorm.DB) {
	now := time.Unix(1_800_000_000, 0).UTC()
	collector := newCollector(db, &now)
	// An older or modified host reports unmasked values: the kernel masks
	// them anyway.
	unsafe := map[string]any{
		"id": sampleID(0), "route_id": "knowledge.user.knowledge.id.get", "method": "get",
		"path":          "/api/v2/user/knowledge/alice@example.com?token=plain-subscribe-token",
		"legacy_status": 200, "native_status": 200, "request_id": "req-1", "observed_at_unix": now.Add(-time.Minute).Unix(),
		"diff": []map[string]any{
			{"path": "$.data.token", "kind": "changed", "legacy": "plain-token-1", "native": "plain-token-2"},
			{"path": "$.data.email", "kind": "changed", "legacy": "alice@example.com", "native": "bob@example.com"},
			{"path": "$.data.ip", "kind": "changed", "legacy": "203.0.113.7", "native": "2001:db8::7"},
			{"path": "$.data.note", "kind": "native_only", "native": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLXZhbHVl"},
		},
	}
	foreignRoute := map[string]any{"id": sampleID(1), "route_id": "ticket.user.fetch", "method": "GET", "observed_at_unix": now.Unix(), "diff": []any{}}
	unknownRoute := map[string]any{"id": sampleID(2), "route_id": "knowledge.secret.route", "method": "GET", "observed_at_unix": now.Unix(), "diff": []any{}}
	badID := map[string]any{"id": "../../etc", "route_id": "knowledge.article.list", "method": "GET", "observed_at_unix": now.Unix(), "diff": []any{}}
	expired := map[string]any{"id": sampleID(3), "route_id": "knowledge.article.list", "method": "GET", "observed_at_unix": now.Add(-Retention - time.Hour).Unix(), "diff": []any{}}
	future := map[string]any{"id": sampleID(4), "route_id": "knowledge.article.list", "method": "GET", "observed_at_unix": now.Add(time.Hour).Unix(), "diff": []any{}}

	stored, err := collector.Ingest(context.Background(), report(t, "knowledge", unsafe, foreignRoute, unknownRoute, badID, expired, future, "not an object"))
	require.NoError(t, err)
	require.Equal(t, 2, stored)

	// The same report again stores nothing.
	stored, err = collector.Ingest(context.Background(), report(t, "knowledge", unsafe, future))
	require.NoError(t, err)
	require.Zero(t, stored)

	samples, err := List(db, Filter{PackageID: "knowledge"})
	require.NoError(t, err)
	require.Len(t, samples, 2)
	byRoute := map[string]Sample{}
	for _, sample := range samples {
		byRoute[sample.RouteID] = sample
	}
	require.Equal(t, now, byRoute["knowledge.article.list"].ObservedAt.UTC(), "a future time is clamped")
	sample := byRoute["knowledge.user.knowledge.id.get"]
	require.Equal(t, "GET", sample.Method)
	require.Equal(t, "/api/v2/user/knowledge/:id?token=***", sample.Path, "the path template replaces the reported path")
	require.Equal(t, "req-1", sample.RequestID)
	require.Equal(t, "1.0.0", sample.PackageVersion)
	encoded, err := json.Marshal(sample.Diff)
	require.NoError(t, err)
	require.JSONEq(t, `[
		{"path":"$.data.token","kind":"changed","legacy":"***","native":"***"},
		{"path":"$.data.email","kind":"changed","legacy":"a***@example.com","native":"b***@example.com"},
		{"path":"$.data.ip","kind":"changed","legacy":"203.0.*.*","native":"2001:db8:*:*:*:*:*:*"},
		{"path":"$.data.note","kind":"native_only","native":"***"}
	]`, string(encoded))
	var raw []model.ShadowMismatchSample
	require.NoError(t, db.Find(&raw).Error)
	for _, row := range raw {
		for _, secret := range []string{"plain-token", "plain-subscribe-token", "alice", "bob@", "203.0.113.7", "eyJ"} {
			require.NotContains(t, row.DiffJSON+row.Path+row.RequestID, secret)
		}
	}

	// Another package cannot read or write knowledge samples.
	other, err := List(db, Filter{PackageID: "ticket"})
	require.NoError(t, err)
	require.Empty(t, other)
}

func TestRetentionCapsAndExpires(t *testing.T) {
	testRetentionCapsAndExpires(t, openSQLite(t))
}

func TestPostgresRetentionCapsAndExpires(t *testing.T) {
	testRetentionCapsAndExpires(t, openPostgres(t))
}

func testRetentionCapsAndExpires(t *testing.T, db *gorm.DB) {
	now := time.Unix(1_800_000_000, 0).UTC()
	collector := newCollector(db, &now)
	// 130 samples in reports of MaxPerReport, one second apart.
	index := 0
	for batch := 0; index < 130; batch++ {
		var samples []any
		for count := 0; count < MaxPerReport && index < 130; count++ {
			samples = append(samples, shadowsample.Sample{
				ID: sampleID(index), RouteID: "knowledge.article.list", Method: "GET", LegacyStatus: 200, NativeStatus: 500,
				Diff: []shadowsample.DiffEntry{}, ObservedAt: now.Add(time.Duration(index-200) * time.Second).Unix(),
			})
			index++
		}
		_, err := collector.Ingest(context.Background(), report(t, "knowledge", samples...))
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&model.ShadowMismatchSample{}).Where("route_id = ?", "knowledge.article.list").Count(&count).Error)
	require.EqualValues(t, MaxPerRoute, count, "only the newest samples of a route are kept")
	var oldest model.ShadowMismatchSample
	require.NoError(t, db.Order("observed_at ASC").First(&oldest).Error)
	require.Equal(t, sampleID(30), oldest.SampleID, "the oldest samples were dropped")

	// Another route is capped on its own.
	_, err := collector.Ingest(context.Background(), report(t, "knowledge", shadowsample.Sample{
		ID: sampleID(500), RouteID: "knowledge.user.knowledge.id.get", Method: "GET", Diff: []shadowsample.DiffEntry{}, ObservedAt: now.Unix(),
	}))
	require.NoError(t, err)

	summaries, err := Summaries(db, "knowledge")
	require.NoError(t, err)
	require.EqualValues(t, MaxPerRoute, summaries["knowledge"]["knowledge.article.list"].Stored)
	require.Equal(t, now.Add(-71*time.Second), summaries["knowledge"]["knowledge.article.list"].LastObservedAt)
	require.EqualValues(t, 1, summaries["knowledge"]["knowledge.user.knowledge.id.get"].Stored)

	limited, err := List(db, Filter{PackageID: "knowledge", RouteID: "knowledge.article.list", Limit: 5})
	require.NoError(t, err)
	require.Len(t, limited, 5)
	require.Equal(t, sampleID(129), limited[0].SampleID, "newest first")
	capped, err := List(db, Filter{Limit: 1000})
	require.NoError(t, err)
	require.Len(t, capped, MaxPerRoute)

	// Prune drops what is older than seven days, and trims crowded routes
	// (rows written by an older kernel, for example).
	for extra := 0; extra < 5; extra++ {
		require.NoError(t, db.Create(&model.ShadowMismatchSample{
			PackageID: "knowledge", SampleID: sampleID(600 + extra), RouteID: "knowledge.article.list", DiffJSON: "[]",
			ObservedAt: now.Add(-time.Duration(300+extra) * time.Second), CreatedAt: now,
		}).Error)
	}
	pruned, err := Prune(db, now.Add(Retention).Add(-100*time.Second))
	require.NoError(t, err)
	require.EqualValues(t, 5+70, pruned, "rows older than seven days")
	require.NoError(t, db.Model(&model.ShadowMismatchSample{}).Where("route_id = ?", "knowledge.article.list").Count(&count).Error)
	require.EqualValues(t, 30, count)
	_, err = Prune(db, now.Add(Retention+time.Hour))
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ShadowMismatchSample{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPruneTrimsCrowdedRoutes(t *testing.T) {
	db := openSQLite(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	for index := 0; index < MaxPerRoute+7; index++ {
		require.NoError(t, db.Create(&model.ShadowMismatchSample{
			PackageID: "knowledge", SampleID: sampleID(index), RouteID: "knowledge.article.list", DiffJSON: "[]",
			ObservedAt: now.Add(-time.Duration(index) * time.Second), CreatedAt: now,
		}).Error)
	}
	_, err := Prune(db, now)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&model.ShadowMismatchSample{}).Count(&count).Error)
	require.EqualValues(t, MaxPerRoute, count)
}

func TestCollectSkipsReportsAlreadyRead(t *testing.T) {
	db := openSQLite(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	collector := newCollector(db, &now)
	first := report(t, "knowledge", shadowsample.Sample{ID: sampleID(1), RouteID: "knowledge.article.list", Method: "GET", ObservedAt: now.Unix()})
	stored, err := collector.Collect(context.Background(), []Report{first, {PackageID: "ticket"}})
	require.NoError(t, err)
	require.Equal(t, 1, stored)
	// The same poll result is not parsed again; a new poll is.
	require.NoError(t, db.Where("1 = 1").Delete(&model.ShadowMismatchSample{}).Error)
	stored, err = collector.Collect(context.Background(), []Report{first})
	require.NoError(t, err)
	require.Zero(t, stored)
	first.CheckedAt = first.CheckedAt.Add(time.Second)
	stored, err = collector.Collect(context.Background(), []Report{first})
	require.NoError(t, err)
	require.Equal(t, 1, stored)

	// Reports without samples, or unreadable, store nothing.
	stored, err = collector.Collect(context.Background(), []Report{
		{PackageID: "knowledge", DetailsJSON: `{"routes":{}}`}, {PackageID: "knowledge", DetailsJSON: `not json`},
		{PackageID: "knowledge", DetailsJSON: `{"shadow_samples":` + strings.Repeat(" ", maxDetailsBytes) + `[]}`},
	})
	require.NoError(t, err)
	require.Zero(t, stored)
}
