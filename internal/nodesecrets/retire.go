package nodesecrets

import (
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Retired counts what Retire removed.
type Retired struct {
	Credentials int64
	Secrets     int64
}

// Retire removes the new-table rows of legacy rows that are about to be
// deleted by their owner: every credential version of the table's subject
// kind and every secret of its scope, for the given ids. It is the kernel's
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
	if spec.subjectKind != "" {
		result := tx.Where("subject_kind = ? AND subject_id IN ?", spec.subjectKind, subjects).Delete(&model.NodeCredential{})
		if result.Error != nil {
			return retired, result.Error
		}
		retired.Credentials = result.RowsAffected
	}
	if spec.scope != "" {
		result := tx.Where("scope = ? AND owner_id IN ?", spec.scope, subjects).Delete(&model.ProtocolSecret{})
		if result.Error != nil {
			return retired, result.Error
		}
		retired.Secrets = result.RowsAffected
	}
	if err := dropOriginals(tx, spec.table, subjects, ""); err != nil {
		return retired, err
	}
	return retired, nil
}
