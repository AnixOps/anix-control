package nodesecrets

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Retired counts what Retire removed.
type Retired struct {
	Secrets int64
}

// Retire removes the new-table rows of legacy rows that are about to be
// deleted by their owner: every secret of the table's scope, for the given
// ids. It is the kernel's
// part of a retirement (node-ops-service.md section 3.3, RetireNode and
// RetireProtocol): the package deletes the legacy row afterwards, and Sync
// on a deleted row removes nothing more. Deleting the legacy row first and
// calling Sync ends in the same state. Retire does nothing when the split
// tables do not exist.
func Retire(tx *gorm.DB, table string, ids ...uint) (Retired, error) {
	var retired Retired
	if len(ids) == 0 {
		return retired, nil
	}
	spec, err := lookupSpec(table)
	if err != nil {
		return retired, err
	}
	if !installed(tx) {
		return retired, nil
	}
	subjects := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			subjects = append(subjects, uint64(id))
		}
	}
	if len(subjects) == 0 {
		return retired, nil
	}
	result := tx.Where("scope = ? AND owner_id IN ?", spec.scope, subjects).Delete(&model.ProtocolSecret{})
	if result.Error != nil {
		return retired, result.Error
	}
	retired.Secrets = result.RowsAffected
	return retired, nil
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
		legacy := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Select("1").
			Where(spec.table + ".id = v4_kernel_protocol_secret.owner_id")
		result := tx.Where("scope = ?", spec.scope).Where("NOT EXISTS (?)", legacy).Delete(&model.ProtocolSecret{})
		pruned += result.RowsAffected
		if result.Error != nil {
			return pruned, result.Error
		}
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
