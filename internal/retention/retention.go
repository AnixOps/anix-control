// Package retention deletes the rows of the append-only log tables that
// outlived their retention, in small batches that give the write lock back
// between them, and on SQLite returns the freed pages to the file.
//
// One Control process runs it: it is a singleton worker
// (cmd/server/singleton_workers.go), so a rolling update or a second replica
// never deletes twice at once. The retention periods and the batch size are
// the design's numbers (docs/design/single-binary section 3.4); a Policy
// field overrides one for a test or a caller.
package retention

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

const (
	// DefaultServerLogRetention is how long v2_server_log (the hourly
	// traffic buckets) keeps a row, by its hour.
	DefaultServerLogRetention = 400 * 24 * time.Hour
	// DefaultNodeLogRetention is how long v2_node_log keeps a row.
	DefaultNodeLogRetention = 30 * 24 * time.Hour
	// DefaultAuditLogRetention is how long v2_audit_log keeps a row.
	DefaultAuditLogRetention = 400 * 24 * time.Hour

	// DefaultInterval is the time between two runs.
	DefaultInterval = time.Hour
	// DefaultBatchSize is the number of rows of one DELETE.
	DefaultBatchSize = 5000
	// DefaultBatchPause is the time between two batches: the write lock is
	// free for the writers that queued behind a batch.
	DefaultBatchPause = 100 * time.Millisecond
	// MaxBatchesPerTable bounds one run per table: a backlog of ten million
	// rows is cleared over the following runs instead of one that never
	// ends.
	MaxBatchesPerTable = 2000
	// VacuumPages is how many free pages one run returns to the file
	// (about 20 MB with 4 KiB pages), so a run never holds the writer for
	// a long compaction.
	VacuumPages = 5000
)

// Policy is the retention of each table. Zero means the default, a negative
// value keeps the table's rows for ever.
type Policy struct {
	ServerLog time.Duration
	NodeLog   time.Duration
	AuditLog  time.Duration
}

// Resolved returns p with the defaults filled in.
func (p Policy) Resolved() Policy {
	if p.ServerLog == 0 {
		p.ServerLog = DefaultServerLogRetention
	}
	if p.NodeLog == 0 {
		p.NodeLog = DefaultNodeLogRetention
	}
	if p.AuditLog == 0 {
		p.AuditLog = DefaultAuditLogRetention
	}
	return p
}

// target is a table the worker prunes: the column that dates a row, and
// whether it holds Unix seconds (v2_server_log's log_at) or a timestamp.
type target struct {
	table     string
	column    string
	unix      bool
	retention func(Policy) time.Duration
}

var targets = []target{
	{model.TrafficLog{}.TableName(), "log_at", true, func(p Policy) time.Duration { return p.ServerLog }},
	{model.NodeLog{}.TableName(), "created_at", false, func(p Policy) time.Duration { return p.NodeLog }},
	{model.AuditLog{}.TableName(), "created_at", false, func(p Policy) time.Duration { return p.AuditLog }},
}

// Result is what one run deleted, per table.
type Result struct {
	Deleted map[string]int64
}

// Worker deletes expired log rows every Interval.
type Worker struct {
	DB     *gorm.DB
	Policy Policy
	// Interval, BatchSize and BatchPause default to the constants above.
	Interval   time.Duration
	BatchSize  int
	BatchPause time.Duration
	// MaxBatches bounds the batches of one table in one run; it defaults to
	// MaxBatchesPerTable.
	MaxBatches int
	// Now defaults to time.Now.
	Now func() time.Time
}

// NewWorker returns a worker over db with the default policy.
func NewWorker(db *gorm.DB) *Worker {
	return &Worker{DB: db}
}

func (w *Worker) interval() time.Duration {
	if w.Interval > 0 {
		return w.Interval
	}
	return DefaultInterval
}

func (w *Worker) batchSize() int {
	if w.BatchSize > 0 {
		return w.BatchSize
	}
	return DefaultBatchSize
}

func (w *Worker) batchPause() time.Duration {
	if w.BatchPause > 0 {
		return w.BatchPause
	}
	return DefaultBatchPause
}

func (w *Worker) maxBatches() int {
	if w.MaxBatches > 0 {
		return w.MaxBatches
	}
	return MaxBatchesPerTable
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Run prunes now and then every Interval until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	for {
		result, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("Retention run failed: %v", err)
		}
		for table, deleted := range result.Deleted {
			if deleted > 0 {
				log.Printf("Retention: deleted %d rows of %s", deleted, table)
			}
		}
		timer := time.NewTimer(w.interval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// RunOnce prunes every table once, then compacts and optimizes a SQLite
// database. It stops at the first error and returns what it deleted so far.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	result := Result{Deleted: map[string]int64{}}
	if w == nil || w.DB == nil {
		return result, nil
	}
	policy := w.Policy.Resolved()
	db := w.DB.WithContext(ctx)
	now := w.now()
	var total int64
	for _, t := range targets {
		retention := t.retention(policy)
		if retention < 0 || !db.Migrator().HasTable(t.table) {
			continue
		}
		deleted, err := w.prune(ctx, db, t, now.Add(-retention))
		result.Deleted[t.table] = deleted
		total += deleted
		if err != nil {
			return result, fmt.Errorf("prune %s: %w", t.table, err)
		}
	}
	if db.Name() == "sqlite" {
		if err := maintainSQLite(db, total > 0); err != nil {
			return result, fmt.Errorf("sqlite maintenance: %w", err)
		}
	}
	return result, nil
}

// prune deletes the rows of t older than cutoff, a batch at a time.
func (w *Worker) prune(ctx context.Context, db *gorm.DB, t target, cutoff time.Time) (int64, error) {
	var bound any = cutoff
	if t.unix {
		bound = cutoff.Unix()
	}
	// The oldest rows first: the date column's index serves the subquery.
	statement := "DELETE FROM " + t.table + " WHERE id IN (SELECT id FROM " + t.table +
		" WHERE " + t.column + " < ? ORDER BY " + t.column + " LIMIT ?)"
	var total int64
	for batch := 0; batch < w.maxBatches(); batch++ {
		result := db.Exec(statement, bound, w.batchSize())
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < int64(w.batchSize()) {
			return total, nil
		}
		timer := time.NewTimer(w.batchPause())
		select {
		case <-ctx.Done():
			timer.Stop()
			return total, ctx.Err()
		case <-timer.C:
		}
	}
	return total, nil
}

// incrementalVacuum frees up to pages free pages. The pragma answers one row
// per page it freed and frees the next page when its row is fetched, so the
// rows are read to the end; an Exec would stop after the first.
func incrementalVacuum(db *gorm.DB, pages int) error {
	rows, err := db.Raw(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", pages)).Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
	}
	return rows.Err()
}

// maintainSQLite returns freed pages to the file (only a database created
// with incremental auto-vacuum has any to return) and lets SQLite refresh
// the statistics its planner needs. Both are cheap and bounded. The
// statements run one after the other on the writer: a connection pinned
// with db.Connection would be a second one for the router, and the writer
// pool has one.
func maintainSQLite(db *gorm.DB, deleted bool) error {
	if deleted {
		var mode int
		if err := db.Raw("PRAGMA auto_vacuum").Scan(&mode).Error; err != nil {
			return err
		}
		if mode == 2 { // INCREMENTAL
			if err := incrementalVacuum(db, VacuumPages); err != nil {
				return err
			}
		}
	}
	// Approximate statistics keep an ANALYZE of a big table short.
	if err := db.Exec("PRAGMA analysis_limit = 1000").Error; err != nil {
		return err
	}
	return db.Exec("PRAGMA optimize").Error
}
