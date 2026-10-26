package retention

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// openFile opens a SQLite file the way the kernel does: WAL, incremental
// auto-vacuum (before WAL, as it must be on a new file).
func openFile(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retention.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_txlock=immediate&_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.TrafficLog{}, &model.NodeLog{}, &model.AuditLog{}))
	return db, path
}

func count(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table(table).Count(&n).Error)
	return n
}

func seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	day := 24 * time.Hour
	var traffic []model.TrafficLog
	for i := 0; i < 250; i++ { // 250 hourly rows: 149 older than the retention, one at it
		at := testNow.Add(-DefaultServerLogRetention).Add(time.Duration(100-i) * time.Hour)
		traffic = append(traffic, model.TrafficLog{UserID: uint(i + 1), ServerID: 1, ServerType: "node", U: 1, D: 1, Rate: 1, LogAt: at.Unix()})
	}
	require.NoError(t, db.CreateInBatches(&traffic, 100).Error)
	var nodeLogs []model.NodeLog
	for i := 0; i < 40; i++ {
		nodeLogs = append(nodeLogs, model.NodeLog{NodeID: 1, Level: "info", Message: "m", CreatedAt: testNow.Add(-time.Duration(i) * day)})
	}
	require.NoError(t, db.CreateInBatches(&nodeLogs, 100).Error)
	var audits []model.AuditLog
	for _, age := range []time.Duration{1 * day, 399 * day, 401 * day, 900 * day} {
		audits = append(audits, model.AuditLog{Method: "POST", Path: "/x", CreatedAt: testNow.Add(-age)})
	}
	require.NoError(t, db.Create(&audits).Error)
}

func TestDefaultsAreTheDesignsRetention(t *testing.T) {
	policy := Policy{}.Resolved()
	require.Equal(t, 400*24*time.Hour, policy.ServerLog)
	require.Equal(t, 30*24*time.Hour, policy.NodeLog)
	require.Equal(t, 400*24*time.Hour, policy.AuditLog)
	require.Equal(t, time.Hour, DefaultInterval)
	require.Equal(t, 5000, DefaultBatchSize)

	custom := Policy{ServerLog: 48 * time.Hour, NodeLog: -1}.Resolved()
	require.Equal(t, 48*time.Hour, custom.ServerLog)
	require.Equal(t, time.Duration(-1), custom.NodeLog, "negative keeps for ever")
	require.Equal(t, DefaultAuditLogRetention, custom.AuditLog)
}

func TestRunOnceDeletesExpiredRowsInBatches(t *testing.T) {
	db, _ := openFile(t)
	seed(t, db)
	var deletes int
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test:count_exec", func(tx *gorm.DB) {
		if len(tx.Statement.SQL.String()) > 6 && tx.Statement.SQL.String()[:6] == "DELETE" {
			deletes++
		}
	}))

	worker := &Worker{DB: db, BatchSize: 40, BatchPause: time.Millisecond, Now: func() time.Time { return testNow }}
	result, err := worker.RunOnce(context.Background())
	require.NoError(t, err)

	// 149 traffic rows are older than 400 days: 4 batches of 40, the last
	// one short (29), which ends the table.
	require.Equal(t, int64(149), result.Deleted["v2_server_log"])
	require.Equal(t, int64(101), count(t, db, "v2_server_log"))
	// node log: rows aged 0..39 days, 30 days kept: the row at the cutoff
	// stays, the 9 older ones (31..39 days) go.
	require.Equal(t, int64(9), result.Deleted["v2_node_log"])
	require.Equal(t, int64(31), count(t, db, "v2_node_log"))
	require.Equal(t, int64(2), result.Deleted["v2_audit_log"])
	require.Equal(t, int64(2), count(t, db, "v2_audit_log"))
	require.Equal(t, 4+1+1, deletes, "149 rows in batches of 40, 9 and 2 rows in one each")

	again, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, again.Deleted["v2_server_log"]+again.Deleted["v2_node_log"]+again.Deleted["v2_audit_log"])
}

// v5_destination_stat keeps the administrator's retention, 30 days by
// default, and a policy's retention overrides it.
func TestRunOnceKeepsTheDestinationStatsRetentionSetting(t *testing.T) {
	db, _ := openFile(t)
	require.NoError(t, db.AutoMigrate(&model.DestinationStat{}, &model.SystemConfig{}))
	var rows []model.DestinationStat
	for days := 0; days < 60; days++ {
		rows = append(rows, model.DestinationStat{HourAt: testNow.Add(-time.Duration(days) * 24 * time.Hour).Unix(), NodeID: 1, UserID: 1, Domain: "example.com"})
	}
	require.NoError(t, db.Create(&rows).Error)
	worker := &Worker{DB: db, BatchPause: time.Millisecond, Now: func() time.Time { return testNow }}

	result, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(29), result.Deleted["v5_destination_stat"], "30 days kept: the rows of days 31 to 59 go")
	require.Equal(t, int64(31), count(t, db, "v5_destination_stat"))

	require.NoError(t, destinationstats.SaveSettings(db, destinationstats.Settings{Enabled: true, RetentionDays: 7}))
	result, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(23), result.Deleted["v5_destination_stat"])
	require.Equal(t, int64(8), count(t, db, "v5_destination_stat"))

	worker.Policy.DestinationStats = 24 * time.Hour
	result, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(6), result.Deleted["v5_destination_stat"], "a policy's retention overrides the setting")
}

func TestRunOnceKeepsATableWithANegativeRetentionAndHonoursMaxBatches(t *testing.T) {
	db, _ := openFile(t)
	seed(t, db)
	worker := &Worker{
		DB: db, Policy: Policy{NodeLog: -1}, BatchSize: 40, MaxBatches: 2, BatchPause: time.Millisecond,
		Now: func() time.Time { return testNow },
	}
	result, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(80), result.Deleted["v2_server_log"], "two batches of 40, the rest is for the next run")
	require.Equal(t, int64(170), count(t, db, "v2_server_log"))
	require.Equal(t, int64(40), count(t, db, "v2_node_log"), "a negative retention deletes nothing")
}

func TestRunOnceStopsWhenTheContextEnds(t *testing.T) {
	db, _ := openFile(t)
	seed(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{DB: db, BatchSize: 10, BatchPause: time.Hour, Now: func() time.Time { return testNow }}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	result, err := worker.RunOnce(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(started), 5*time.Second, "the pause between batches does not outlive the context")
	require.Equal(t, int64(10), result.Deleted["v2_server_log"], "one batch ran")
}

// After a delete the freed pages go back to the file, which a database with
// incremental auto-vacuum can do and the worker asks for.
func TestRunOnceReturnsFreedPagesToTheFile(t *testing.T) {
	db, path := openFile(t)
	var logs []model.NodeLog
	filler := string(make([]byte, 1024))
	for i := 0; i < 2000; i++ {
		logs = append(logs, model.NodeLog{NodeID: 1, Level: "info", Message: filler, CreatedAt: testNow.Add(-90 * 24 * time.Hour)})
	}
	require.NoError(t, db.CreateInBatches(&logs, 200).Error)
	require.NoError(t, db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)
	before, err := os.Stat(path)
	require.NoError(t, err)

	worker := &Worker{DB: db, BatchSize: 500, BatchPause: time.Millisecond, Now: func() time.Time { return testNow }}
	result, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2000), result.Deleted["v2_node_log"])
	require.NoError(t, db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)

	var free int64
	require.NoError(t, db.Raw("PRAGMA freelist_count").Scan(&free).Error)
	require.Zero(t, free, "the free pages were returned")
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Less(t, after.Size(), before.Size()/2)
}

func TestRunOnceOnADatabaseWithoutTheTablesIsANoOp(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	result, err := (&Worker{DB: db}).RunOnce(context.Background())
	require.NoError(t, err)
	require.Empty(t, result.Deleted)
	_, err = (*Worker)(nil).RunOnce(context.Background())
	require.NoError(t, err)
}

func TestRunRunsAgainAfterTheInterval(t *testing.T) {
	db, _ := openFile(t)
	seed(t, db)
	worker := &Worker{DB: db, Interval: 20 * time.Millisecond, BatchSize: 1000, Now: func() time.Time { return testNow }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()
	require.Eventually(t, func() bool { return count(t, db, "v2_server_log") == 101 }, 5*time.Second, 10*time.Millisecond)
	// A row that expires later is taken by a later run.
	require.NoError(t, db.Create(&model.AuditLog{Method: "GET", Path: "/old", CreatedAt: testNow.Add(-1000 * 24 * time.Hour)}).Error)
	require.Eventually(t, func() bool { return count(t, db, "v2_audit_log") == 2 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop with its context")
	}
}
