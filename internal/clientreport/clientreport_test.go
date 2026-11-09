package clientreport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var now = time.Date(2026, 11, 9, 12, 30, 0, 0, time.UTC)

func databases(t *testing.T) map[string]*gorm.DB {
	t.Helper()
	out := map[string]*gorm.DB{}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "reports.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	out["sqlite"] = db
	if dsn := os.Getenv("ANIX_TEST_POSTGRES_DSN"); dsn != "" {
		pg, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, pg.Migrator().DropTable(&model.ClientFailureReport{}))
		out["postgres"] = pg
	}
	for _, db := range out {
		require.NoError(t, db.AutoMigrate(&model.ClientFailureReport{}))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return out
}

func report() Report {
	skew := int64(3)
	return Report{Client: "anixops-net/0.3.0", Platform: "iOS", Network: "WIFI", WindowSecs: 600, Nodes: []NodeReport{
		{Node: "hk-1", Server: "hk.example.com", Proto: "ANX", OK: 10, Reset: 2, AfterHandshake: 1},
		{Node: "jp-1", Server: "jp.example.com", Proto: "vless", Timeout: 5, Stopped: true, ClockSkewSecs: &skew},
	}}
}

func TestValidate(t *testing.T) {
	r := report()
	require.NoError(t, r.Validate())
	require.Equal(t, "wifi", r.Network)
	require.Equal(t, "ios", r.Platform)
	require.Equal(t, "anx", r.Nodes[0].Proto)

	for name, mutate := range map[string]func(*Report){
		"short window": func(r *Report) { r.WindowSecs = 10 },
		"long window":  func(r *Report) { r.WindowSecs = MaxWindowSecs + 1 },
		"no nodes":     func(r *Report) { r.Nodes = nil },
		"many nodes":   func(r *Report) { r.Nodes = make([]NodeReport, MaxNodes+1) },
		"no node name": func(r *Report) { r.Nodes[0].Node = " " },
		"big count":    func(r *Report) { r.Nodes[0].OK = MaxCount + 1 },
		"negative":     func(r *Report) { r.Nodes[1].Timeout = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			r := report()
			mutate(&r)
			require.ErrorIs(t, r.Validate(), ErrInvalid)
		})
	}
	r = report()
	r.Network = "satellite"
	require.NoError(t, r.Validate())
	require.Equal(t, "other", r.Network)
}

func TestRecordAddsUpAndSummarizes(t *testing.T) {
	for name, db := range databases(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			r := report()
			require.NoError(t, r.Validate())
			require.NoError(t, Record(ctx, db, 1, r, now))
			require.NoError(t, Record(ctx, db, 1, r, now.Add(10*time.Minute)))
			require.NoError(t, Record(ctx, db, 2, r, now))
			// Next hour: a new row.
			require.NoError(t, Record(ctx, db, 1, r, now.Add(time.Hour)))

			var rows []model.ClientFailureReport
			require.NoError(t, db.Where("user_id = ? AND node = ?", 1, "hk-1").Order("hour").Find(&rows).Error)
			require.Len(t, rows, 2)
			require.Equal(t, int64(20), rows[0].OK)
			require.Equal(t, int64(4), rows[0].Reset)

			summary, err := Summary(ctx, db, 24, now.Add(time.Hour))
			require.NoError(t, err)
			require.Len(t, summary, 2)
			require.Equal(t, "jp-1", summary[0].Node) // all failures
			require.Equal(t, 2, summary[0].Users)
			require.Equal(t, 2, summary[0].StoppedUsers)
			require.InDelta(t, 1.0, summary[0].FailureRatio, 1e-9)
			require.Equal(t, int64(40), summary[1].OK)
			require.Equal(t, int64(52), summary[1].Networks["wifi"])

			_, err = Summary(ctx, db, 0, now)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter()
	require.True(t, l.Allow("1", now))
	require.False(t, l.Allow("1", now.Add(RateWindow-time.Second)))
	require.True(t, l.Allow("2", now))
	require.True(t, l.Allow("1", now.Add(RateWindow)))
}
