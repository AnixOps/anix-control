package nodesecrets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// DefaultBatchSize is how many legacy rows one backfill transaction copies.
const DefaultBatchSize = 500

// BackfillOptions selects the tables and the batch size of a backfill.
type BackfillOptions struct {
	// Tables limits the backfill to these split tables; empty is all.
	Tables []string
	// BatchSize is the number of legacy rows per transaction.
	BatchSize int
	// Restart starts a new pass even where an earlier one was interrupted.
	Restart bool
}

// TableBackfill is the outcome of one table's backfill. It holds counts
// only, never a secret.
type TableBackfill struct {
	Table string `json:"table"`
	// Rows counts the legacy rows of the pass, including those an
	// interrupted pass copied before (Resumed).
	Rows int64 `json:"rows"`
	// Changed counts the new-table rows this run inserted, changed or
	// deleted; a repeated backfill changes none.
	Changed int64 `json:"changed"`
	// Pruned counts the new-table rows removed because their legacy row no
	// longer exists.
	Pruned  int64 `json:"pruned"`
	Resumed bool  `json:"resumed"`
}

// Backfill copies the secrets of the legacy rows into the new tables, table
// by table, in batches by id. Each batch is one transaction that locks its
// legacy rows, so it serializes with the writers of those rows, and records
// its last id in v4_kernel_node_secret_split: an interrupted pass resumes
// after it. A completed pass is followed by a full new one. Rows already
// equal are left as they are, so backfill is idempotent.
func Backfill(ctx context.Context, db *gorm.DB, options BackfillOptions) ([]TableBackfill, error) {
	tables, err := selectTables(options.Tables)
	if err != nil {
		return nil, err
	}
	if !installed(db) {
		return nil, errors.New("nodesecrets: the split tables do not exist; start Control or run its migrate command first")
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	results := make([]TableBackfill, 0, len(tables))
	for _, table := range tables {
		result, err := backfillTable(ctx, db, specs[table], batchSize, options.Restart)
		results = append(results, result)
		if err != nil {
			return results, fmt.Errorf("backfill %s: %w", table, err)
		}
	}
	return results, nil
}

func backfillTable(ctx context.Context, db *gorm.DB, spec tableSpec, batchSize int, restart bool) (TableBackfill, error) {
	result := TableBackfill{Table: spec.table}
	split, err := splitRow(db.WithContext(ctx), spec.table)
	if err != nil {
		return result, err
	}
	cursor := uint64(0)
	if !restart && split.BackfilledAt == nil && split.BackfillCursor > 0 {
		cursor = split.BackfillCursor
		result.Rows = split.BackfilledRows
		result.Resumed = true
	} else if err := db.WithContext(ctx).Model(&model.NodeSecretSplit{}).Where("table_name = ?", spec.table).Updates(map[string]any{
		"backfill_cursor": 0, "backfilled_rows": 0, "backfilled_at": nil, "updated_at": time.Now().UTC(),
	}).Error; err != nil {
		return result, err
	}

	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		done := false
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var ids []uint64
			if err := tx.Table(spec.table).Where("id > ?", cursor).Order("id").Limit(batchSize).Pluck("id", &ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				done = true
				return nil
			}
			now := time.Now().UTC()
			changes, err := syncRows(tx, spec, ids, SourceBackfill, true, now)
			if err != nil {
				return err
			}
			last := ids[len(ids)-1]
			if err := tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", spec.table).Updates(map[string]any{
				"backfill_cursor": last, "backfilled_rows": result.Rows + int64(len(ids)), "updated_at": now,
			}).Error; err != nil {
				return err
			}
			cursor = last
			result.Rows += int64(len(ids))
			result.Changed += changes
			return nil
		})
		if err != nil {
			return result, err
		}
		if done {
			break
		}
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pruned, err := pruneOrphans(tx, spec)
		if err != nil {
			return err
		}
		result.Pruned = pruned
		result.Changed += pruned
		now := time.Now().UTC()
		return tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", spec.table).Updates(map[string]any{
			"backfilled_rows": result.Rows, "backfilled_at": now, "updated_at": now,
		}).Error
	})
	return result, err
}

// Prune removes the new-table rows of the given split tables whose legacy
// row no longer exists, in the caller's transaction: for a tool that
// deletes legacy rows without the writers (a re-import). It does nothing
// when the split tables do not exist.
func Prune(tx *gorm.DB, tables ...string) (int64, error) {
	if !installed(tx) {
		return 0, nil
	}
	var pruned int64
	for _, table := range tables {
		spec, err := lookupSpec(table)
		if err != nil {
			return pruned, err
		}
		n, err := pruneOrphans(tx, spec)
		pruned += n
		if err != nil {
			return pruned, err
		}
	}
	return pruned, nil
}

// pruneOrphans removes the new-table rows of spec's table whose legacy row
// no longer exists: rows a writer before P1 never removed, or rows copied
// into a database whose legacy rows were then replaced.
func pruneOrphans(tx *gorm.DB, spec tableSpec) (int64, error) {
	var pruned int64
	if spec.subjectKind != "" {
		legacy := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Select("1").
			Where(spec.table + ".id = v4_kernel_node_credential.subject_id")
		result := tx.Where("subject_kind = ? AND kind IN ?", spec.subjectKind, spec.kinds).
			Where("NOT EXISTS (?)", legacy).Delete(&model.NodeCredential{})
		if result.Error != nil {
			return pruned, result.Error
		}
		pruned += result.RowsAffected
	}
	if spec.scope != "" {
		legacy := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Select("1").
			Where(spec.table + ".id = v4_kernel_protocol_secret.owner_id")
		result := tx.Where("scope = ?", spec.scope).Where("NOT EXISTS (?)", legacy).Delete(&model.ProtocolSecret{})
		if result.Error != nil {
			return pruned, result.Error
		}
		pruned += result.RowsAffected
	}
	return pruned, nil
}

// Status answers the state of every split table.
func Status(ctx context.Context, db *gorm.DB) ([]model.NodeSecretSplit, error) {
	if !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return nil, errors.New("nodesecrets: the split tables do not exist; start Control or run its migrate command first")
	}
	var rows []model.NodeSecretSplit
	if err := db.WithContext(ctx).Order("table_name").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// splitRow loads a table's state row, creating it in phase dual_write when
// it is missing.
func splitRow(db *gorm.DB, table string) (model.NodeSecretSplit, error) {
	var row model.NodeSecretSplit
	err := db.Where("table_name = ?", table).Limit(1).Find(&row).Error
	if err != nil {
		return row, err
	}
	if row.Table != "" {
		return row, nil
	}
	if err := EnsureSchema(db); err != nil {
		return row, err
	}
	err = db.Where("table_name = ?", table).First(&row).Error
	return row, err
}

func selectTables(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return Tables(), nil
	}
	tables := make([]string, 0, len(requested))
	for _, table := range requested {
		if _, err := lookupSpec(table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}
