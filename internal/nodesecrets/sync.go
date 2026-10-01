package nodesecrets

import (
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// syncBatch bounds the ids of one statement.
const syncBatch = 500

// Sync is the dual-write of a kernel writer. Call it with the transaction of
// the legacy write, after that write, with the ids of the legacy rows it
// created, changed or deleted: the new tables are brought in line with
// those rows as they now stand, so a failure of either rolls back both.
//
//   - A legacy row that holds a secret gets its current row in the new
//     table; a changed value retires that row (its value cleared) and adds
//     the next version.
//   - A deleted row, or a secret that is gone, takes its rows away.
//   - A tombstone or the placeholder in a legacy column leaves the new row
//     as it is: the secret has moved there.
//
// Sync does nothing when the split tables do not exist.
func Sync(tx *gorm.DB, table string, ids ...uint) error {
	if len(ids) == 0 {
		return nil
	}
	spec, err := lookupSpec(table)
	if err != nil {
		return err
	}
	if !installed(tx) {
		return nil
	}
	subjects := make([]uint64, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			subjects = append(subjects, uint64(id))
		}
	}
	_, err = syncRows(tx, spec, subjects, SourceDualWrite, false, time.Now().UTC())
	return err
}

// syncRows derives the legacy rows ids of spec's table and applies them to
// the new tables. It answers the number of new-table rows it inserted,
// changed or deleted.
func syncRows(tx *gorm.DB, spec tableSpec, ids []uint64, source string, lock bool, now time.Time) (int64, error) {
	var changes int64
	for start := 0; start < len(ids); start += syncBatch {
		end := min(start+syncBatch, len(ids))
		batch := ids[start:end]
		derived, err := spec.derive(tx, batch, lock)
		if err != nil {
			return changes, err
		}
		if spec.subjectKind != "" {
			n, err := applyCredentials(tx, spec, batch, derived, source, now)
			changes += n
			if err != nil {
				return changes, err
			}
		}
		if spec.scope != "" {
			n, err := applySecrets(tx, spec.scope, batch, derived, now)
			changes += n
			if err != nil {
				return changes, err
			}
		}
	}
	return changes, nil
}

func applyCredentials(tx *gorm.DB, spec tableSpec, ids []uint64, derived *derivation, source string, now time.Time) (int64, error) {
	var existing []model.NodeCredential
	if err := tx.Where("subject_kind = ? AND subject_id IN ? AND kind IN ?", spec.subjectKind, ids, spec.kinds).
		Order("subject_id, kind, version").Find(&existing).Error; err != nil {
		return 0, err
	}
	rows := make(map[credentialKey][]model.NodeCredential)
	for _, row := range existing {
		key := credentialKey{SubjectID: row.SubjectID, Kind: row.Kind}
		rows[key] = append(rows[key], row)
	}
	desired := make(map[credentialKey]credentialEntry, len(derived.credentials))
	for _, entry := range derived.credentials {
		desired[credentialKey{SubjectID: entry.SubjectID, Kind: entry.Kind}] = entry
	}

	var changes int64
	var gone []uint64
	for key, current := range rows {
		if _, ok := desired[key]; ok || derived.keptCredentials[key] {
			continue
		}
		for _, row := range current {
			gone = append(gone, row.ID)
		}
	}
	if len(gone) > 0 {
		result := tx.Where("id IN ?", gone).Delete(&model.NodeCredential{})
		if result.Error != nil {
			return changes, result.Error
		}
		changes += result.RowsAffected
	}

	for key, want := range desired {
		n, err := applyCredential(tx, spec.subjectKind, want, rows[key], source, now)
		changes += n
		if err != nil {
			return changes, err
		}
	}
	return changes, nil
}

// applyCredential brings one subject's credential of one kind to want.
// versions are its rows by ascending version.
func applyCredential(tx *gorm.DB, subjectKind string, want credentialEntry, versions []model.NodeCredential, source string, now time.Time) (int64, error) {
	var current *model.NodeCredential
	maxVersion := 0
	for i := range versions {
		row := &versions[i]
		maxVersion = max(maxVersion, row.Version)
		if row.Status != StatusRetired {
			current = row
		}
	}
	if current != nil && current.Value == want.Value && current.KeyHash == want.KeyHash {
		updates := map[string]any{}
		if current.Status != want.Status {
			updates["status"] = want.Status
		}
		if current.Endpoint != want.Endpoint {
			updates["endpoint"] = want.Endpoint
		}
		if !sameSecond(current.ExpiresAt, want.ExpiresAt) {
			updates["expires_at"] = want.ExpiresAt
		}
		if !sameSecond(current.RevokedAt, want.RevokedAt) {
			updates["revoked_at"] = want.RevokedAt
		}
		if len(updates) == 0 {
			return 0, nil
		}
		updates["source"] = source
		return 1, tx.Model(&model.NodeCredential{}).Where("id = ?", current.ID).Updates(updates).Error
	}

	var changes int64
	row := model.NodeCredential{
		SubjectKind: subjectKind, SubjectID: want.SubjectID, Kind: want.Kind, Version: maxVersion + 1,
		KeyHash: want.KeyHash, Value: want.Value, Endpoint: want.Endpoint, Status: want.Status,
		ExpiresAt: want.ExpiresAt, RevokedAt: want.RevokedAt, Source: source, CreatedAt: now,
	}
	if current != nil {
		// The replaced version keeps no secret: its value and hash are
		// cleared.
		if err := tx.Model(&model.NodeCredential{}).Where("id = ?", current.ID).Updates(map[string]any{
			"status": StatusRetired, "value": "", "key_hash": "", "rotated_at": now,
		}).Error; err != nil {
			return changes, err
		}
		changes++
		row.RotatedAt = &now
	}
	if err := tx.Create(&row).Error; err != nil {
		return changes, err
	}
	return changes + 1, nil
}

func applySecrets(tx *gorm.DB, scope string, ids []uint64, derived *derivation, now time.Time) (int64, error) {
	var existing []model.ProtocolSecret
	if err := tx.Where("scope = ? AND owner_id IN ?", scope, ids).Find(&existing).Error; err != nil {
		return 0, err
	}
	rows := make(map[secretKey]model.ProtocolSecret, len(existing))
	for _, row := range existing {
		rows[secretKey{OwnerID: row.OwnerID, Column: row.ColumnName, Pointer: row.JSONPointer}] = row
	}
	desired := make(map[secretKey]string, len(derived.secrets))
	for _, entry := range derived.secrets {
		desired[secretKey{OwnerID: entry.OwnerID, Column: entry.Column, Pointer: entry.Pointer}] = entry.Value
	}

	var changes int64
	var gone []uint64
	for key, row := range rows {
		if _, ok := desired[key]; ok || derived.keptSecrets[key] {
			continue
		}
		gone = append(gone, row.ID)
	}
	if len(gone) > 0 {
		result := tx.Where("id IN ?", gone).Delete(&model.ProtocolSecret{})
		if result.Error != nil {
			return changes, result.Error
		}
		changes += result.RowsAffected
	}

	for key, value := range desired {
		row, ok := rows[key]
		switch {
		case !ok:
			if err := tx.Create(&model.ProtocolSecret{
				Scope: scope, OwnerID: key.OwnerID, ColumnName: key.Column, JSONPointer: key.Pointer,
				Value: value, Version: 1, UpdatedAt: now,
			}).Error; err != nil {
				return changes, err
			}
		case row.Value != value:
			if err := tx.Model(&model.ProtocolSecret{}).Where("id = ?", row.ID).Updates(map[string]any{
				"value": value, "version": row.Version + 1, "updated_at": now,
			}).Error; err != nil {
				return changes, err
			}
		default:
			continue
		}
		changes++
	}
	return changes, nil
}

// sameSecond compares two optional times to the second, the precision the
// legacy columns and every database keep.
func sameSecond(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Unix() == b.Unix()
}
