package service

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var nodeTrafficModels = []any{&model.Node{}, &model.TrafficLog{}, &model.StatServer{}}

// nodeTrafficDatabases opens SQLite and, when ANIX_TEST_POSTGRES_DSN is set,
// a throwaway schema of that PostgreSQL, and runs body on each.
func forEachNodeTrafficDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "traffic.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		require.NoError(t, db.AutoMigrate(nodeTrafficModels...))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
		if base == "" {
			t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
		}
		config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
		admin, err := gorm.Open(postgres.Open(base), config)
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
		schema := "node_traffic_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		db, err := gorm.Open(postgres.Open(base+" search_path="+schema), config)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		require.NoError(t, db.AutoMigrate(nodeTrafficModels...))
		body(t, db)
	})
}

func TestGetNodeTrafficHourly(t *testing.T) {
	forEachNodeTrafficDatabase(t, func(t *testing.T, db *gorm.DB) {
		svc := &StatsService{db: db}
		require.NoError(t, db.Create(&[]model.Node{{ID: 7, Name: "seven", APIKey: "k7"}, {ID: 8, Name: "eight", APIKey: "k8"}}).Error)
		current := time.Now().UTC().Truncate(time.Hour).Unix()
		const gib = int64(1 << 30)
		require.NoError(t, db.Create(&[]model.TrafficLog{
			// This hour: two users and two server types of node 7; the rate multiplies each row.
			{UserID: 1, ServerID: 7, ServerType: "node", U: 100, D: 1000, Rate: 1, LogAt: current + 10},
			{UserID: 2, ServerID: 7, ServerType: "vmess", U: 50, D: 500, Rate: 2, LogAt: current + 20},
			// Two hours ago.
			{UserID: 1, ServerID: 7, ServerType: "node", U: 3 * gib, D: gib, Rate: 1, LogAt: current - 2*3600 + 5},
			// A rate of zero counts as one, a negative value as zero.
			{UserID: 3, ServerID: 7, ServerType: "node", U: 7, D: -9, Rate: 0, LogAt: current - 2*3600 + 6},
			// Another node, and an hour outside the default window.
			{UserID: 1, ServerID: 8, ServerType: "node", U: 999, D: 999, Rate: 1, LogAt: current + 30},
			{UserID: 1, ServerID: 7, ServerType: "node", U: 12345, D: 12345, Rate: 1, LogAt: current - 25*3600},
		}).Error)

		series, err := svc.GetNodeTraffic(7, NodeTrafficHour, time.Time{}, time.Time{})
		require.NoError(t, err)
		require.Equal(t, uint(7), series.NodeID)
		require.Equal(t, NodeTrafficHour, series.Granularity)
		require.Len(t, series.Points, 24, "the last 24 hours up to the end of the current one")
		require.Equal(t, current+3600, series.Until.Unix())
		require.Equal(t, current-23*3600, series.Since.Unix())
		for i, point := range series.Points {
			require.Equal(t, current-int64(23-i)*3600, point.Start.Unix(), "ascending, one per hour")
		}
		last := series.Points[23]
		require.Equal(t, int64(100+50*2), last.Up)
		require.Equal(t, int64(1000+500*2), last.Down)
		twoAgo := series.Points[21]
		require.Equal(t, 3*gib+7, twoAgo.Up)
		require.Equal(t, gib, twoAgo.Down, "a negative value counts as zero")
		var empty, total int64
		for _, point := range series.Points {
			if point.Up == 0 && point.Down == 0 {
				empty++
			}
			total += point.Up + point.Down
		}
		require.Equal(t, int64(22), empty, "hours without traffic are zeros")
		require.Equal(t, int64(100+100+1000+1000)+3*gib+7+gib, total, "node 8 and the old row are left out")

		// An explicit window, rounded out to whole hours.
		series, err = svc.GetNodeTraffic(7, NodeTrafficHour, time.Unix(current-2*3600+1800, 0), time.Unix(current-3600+1, 0))
		require.NoError(t, err)
		require.Len(t, series.Points, 2)
		require.Equal(t, current-2*3600, series.Since.Unix())
		require.Equal(t, current, series.Until.Unix())
		require.Equal(t, 3*gib+7, series.Points[0].Up)

		// The old hour is reachable with a longer window.
		series, err = svc.GetNodeTraffic(7, NodeTrafficHour, time.Unix(current-30*3600, 0), time.Time{})
		require.NoError(t, err)
		require.Equal(t, int64(12345), series.Points[5].Up)

		// A node with no traffic still answers zeros; an unknown node does not.
		series, err = svc.GetNodeTraffic(8, NodeTrafficHour, time.Unix(current-3600, 0), time.Unix(current, 0))
		require.NoError(t, err)
		require.Len(t, series.Points, 1)
		require.Equal(t, int64(0), series.Points[0].Up)
		_, err = svc.GetNodeTraffic(99, NodeTrafficHour, time.Time{}, time.Time{})
		require.ErrorIs(t, err, ErrNodeTrafficNode)
	})
}

// TestGetNodeTrafficClampsHugeSums: a sum beyond int64 is clamped, not an
// error, as the global aggregates do.
func TestGetNodeTrafficClampsHugeSums(t *testing.T) {
	forEachNodeTrafficDatabase(t, func(t *testing.T, db *gorm.DB) {
		svc := &StatsService{db: db}
		require.NoError(t, db.Create(&model.Node{ID: 7, Name: "seven", APIKey: "k7"}).Error)
		current := time.Now().UTC().Truncate(time.Hour).Unix()
		const maxInt64 = int64(1<<63 - 1)
		require.NoError(t, db.Create(&[]model.TrafficLog{
			{UserID: 1, ServerID: 7, ServerType: "node", U: maxInt64, D: 1, Rate: 3, LogAt: current + 1},
			{UserID: 2, ServerID: 7, ServerType: "node", U: maxInt64, D: 1, Rate: 3, LogAt: current + 2},
		}).Error)
		series, err := svc.GetNodeTraffic(7, NodeTrafficHour, time.Unix(current, 0), time.Unix(current+3600, 0))
		require.NoError(t, err)
		require.Len(t, series.Points, 1)
		require.Equal(t, maxInt64, series.Points[0].Up)
		require.Equal(t, int64(6), series.Points[0].Down)
	})
}

func TestGetNodeTrafficDaily(t *testing.T) {
	forEachNodeTrafficDatabase(t, func(t *testing.T, db *gorm.DB) {
		svc := &StatsService{db: db}
		require.NoError(t, db.Create(&[]model.Node{{ID: 7, Name: "seven", APIKey: "k7"}, {ID: 8, Name: "eight", APIKey: "k8"}}).Error)
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		day := func(back int) int64 { return today.AddDate(0, 0, -back).Unix() }
		require.NoError(t, db.Create(&[]model.StatServer{
			{ServerID: 7, ServerType: "node", U: 10, D: 100, RecordType: "d", RecordAt: day(0)},
			{ServerID: 7, ServerType: "vmess", U: 1, D: 2, RecordType: "d", RecordAt: day(0)},
			{ServerID: 7, ServerType: "node", U: 20, D: 200, RecordType: "d", RecordAt: day(3)},
			// A day recorded at another zone's midnight lands in the local day containing it.
			{ServerID: 7, ServerType: "node", U: 5, D: 50, RecordType: "d", RecordAt: day(5) + 8*3600},
			// Monthly rows, other nodes and other days are left out.
			{ServerID: 7, ServerType: "node", U: 999, D: 999, RecordType: "m", RecordAt: day(0)},
			{ServerID: 8, ServerType: "node", U: 888, D: 888, RecordType: "d", RecordAt: day(0)},
			{ServerID: 7, ServerType: "node", U: 777, D: 777, RecordType: "d", RecordAt: day(40)},
		}).Error)

		series, err := svc.GetNodeTraffic(7, NodeTrafficDay, time.Time{}, time.Time{})
		require.NoError(t, err)
		require.Equal(t, NodeTrafficDay, series.Granularity)
		require.Len(t, series.Points, 30, "the last 30 days up to the end of today")
		require.Equal(t, today.AddDate(0, 0, 1).Unix(), series.Until.Unix())
		require.Equal(t, day(29), series.Since.Unix())
		for i, point := range series.Points {
			require.Equal(t, day(29-i), point.Start.Unix(), "ascending, one per local day")
		}
		require.Equal(t, int64(11), series.Points[29].Up, "both server types of today")
		require.Equal(t, int64(102), series.Points[29].Down)
		require.Equal(t, int64(20), series.Points[26].Up)
		require.Equal(t, int64(5), series.Points[24].Up)
		var total int64
		for _, point := range series.Points {
			total += point.Up + point.Down
		}
		require.Equal(t, int64(10+100+1+2+20+200+5+50), total)

		series, err = svc.GetNodeTraffic(7, NodeTrafficDay, time.Unix(day(45), 0), time.Time{})
		require.NoError(t, err)
		require.Len(t, series.Points, 46)
		require.Equal(t, int64(777), series.Points[5].Up)

		_, err = svc.GetNodeTraffic(99, NodeTrafficDay, time.Time{}, time.Time{})
		require.ErrorIs(t, err, ErrNodeTrafficNode)
	})
}

func TestNodeTrafficBucketBounds(t *testing.T) {
	now := time.Date(2026, 10, 5, 13, 40, 0, 0, time.UTC)
	hourly := func(since, until time.Time) (time.Time, time.Time, error) {
		return NodeTrafficBucketBounds(NodeTrafficHour, since, until, now)
	}
	since, until, err := hourly(time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 4, 14, 0, 0, 0, time.UTC), since.UTC())
	require.Equal(t, time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC), until.UTC())

	since, until, err = hourly(time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC), time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), since.UTC(), "since rounds down")
	require.Equal(t, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), until.UTC(), "an aligned until stays")
	_, until, err = hourly(time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC), time.Date(2026, 10, 5, 12, 0, 1, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC), until.UTC(), "until rounds up")

	// The bound is on buckets: 720 hours pass, 721 do not.
	end := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	_, _, err = hourly(end.Add(-720*time.Hour), end)
	require.NoError(t, err)
	_, _, err = hourly(end.Add(-721*time.Hour), end)
	require.ErrorIs(t, err, ErrNodeTrafficWindow)
	require.Contains(t, err.Error(), "720")
	_, _, err = hourly(time.UnixMilli(1), time.Time{})
	require.ErrorIs(t, err, ErrNodeTrafficWindow, "an old since is refused before it is walked")
	_, _, err = hourly(end, end)
	require.ErrorIs(t, err, ErrNodeTrafficWindow, "an empty window")
	_, _, err = hourly(end.Add(time.Hour), end)
	require.ErrorIs(t, err, ErrNodeTrafficWindow, "a reversed window")
	_, _, err = NodeTrafficBucketBounds("month", time.Time{}, time.Time{}, now)
	require.ErrorIs(t, err, ErrNodeTrafficWindow)

	_, _, err = NodeTrafficBucketBounds(NodeTrafficDay, time.Time{}, time.Time{}, now)
	require.NoError(t, err)
}

// TestNodeTrafficDaysFollowTheLocalCalendar: a day is a local calendar day,
// 23 or 25 hours long where the clocks change, and the bound counts days.
func TestNodeTrafficDaysFollowTheLocalCalendar(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata: " + err.Error())
	}
	previous := time.Local
	time.Local = berlin
	t.Cleanup(func() { time.Local = previous })

	// 2026-10-25 is 25 hours long in Berlin.
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, berlin)
	since, until, err := NodeTrafficBucketBounds(NodeTrafficDay, time.Date(2026, 10, 24, 12, 0, 0, 0, berlin), time.Time{}, now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 24, 0, 0, 0, 0, berlin), since)
	require.Equal(t, time.Date(2026, 10, 27, 0, 0, 0, 0, berlin), until)
	require.Equal(t, 73*time.Hour, until.Sub(since), "the 25-hour day is one bucket")

	since, _, err = NodeTrafficBucketBounds(NodeTrafficDay, time.Time{}, time.Time{}, now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 9, 27, 0, 0, 0, 0, berlin), since, "30 calendar days up to the end of the 26th")
}
