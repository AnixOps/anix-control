package nodesecrets

import (
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file writes the tombstones of phase P3 (section 4.4). Once a table
// is finalized its legacy secret columns hold no secret:
//
//   - a key column (v2_wireguard_peer.private_key and .preshared_key) holds
//     the tombstone "!moved:<row id>": unique per row, non-empty, and never
//     a key;
//   - a JSON column (v2_node.raw_config, the five settings columns of
//     v2_node_protocol) holds its redacted document, Redact of the
//     original, with the placeholder at every secret position.
//
// Tombstones are value writes; no table is altered.

// scopeLegacyOriginal is the scope of v4_kernel_protocol_secret that keeps
// what finalize cannot derive back from the new tables, so unsplit writes
// the legacy columns back byte for byte: a JSON column's original document
// (its formatting and key order). The column is "<table>.<column>", the owner the row's id. It
// is never read as a secret position: readers, writers and verify select
// the other scopes only, and the rows go with their legacy row.
const scopeLegacyOriginal = "legacy_original"

func originalColumn(table, column string) string {
	return table + "." + column
}

// legacyColumn is one secret column of a legacy table.
type legacyColumn struct {
	name string
	// kind: tombstone (a key column) or document (a JSON column).
	kind string
}

const (
	columnTombstone = "tombstone"
	columnDocument  = "document"
)

// legacyColumns lists the secret columns of each split table.
var legacyColumns = map[string][]legacyColumn{
	TableNode: {
		{name: "raw_config", kind: columnDocument},
	},
	TableNodeProtocol: {
		{name: "settings", kind: columnDocument},
		{name: "tls_settings", kind: columnDocument},
		{name: "transport_settings", kind: columnDocument},
		{name: "reality_settings", kind: columnDocument},
		{name: "custom_config", kind: columnDocument},
	},
	TableWireGuardPeer: {
		{name: "private_key", kind: columnTombstone},
		{name: "preshared_key", kind: columnTombstone},
	},
}

// legacyRow is the secret columns of one legacy row, as text; a NULL
// document is absent from values.
type legacyRow struct {
	id     uint64
	values map[string]string
}

// loadLegacyRows reads the secret columns of the given rows of spec's
// table; lock takes their row locks (PostgreSQL).
func loadLegacyRows(tx *gorm.DB, spec tableSpec, ids []uint64, lock bool) ([]legacyRow, error) {
	columns := legacyColumns[spec.table]
	selected := make([]string, 0, len(columns)+1)
	selected = append(selected, "id")
	for _, column := range columns {
		selected = append(selected, column.name)
	}
	query := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Select(quoteColumns(tx, selected)).Where("id IN ?", ids).Order("id")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var raw []map[string]any
	if err := query.Find(&raw).Error; err != nil {
		return nil, err
	}
	rows := make([]legacyRow, 0, len(raw))
	for _, item := range raw {
		row := legacyRow{id: toUint64(item["id"]), values: map[string]string{}}
		for _, column := range columns {
			if text, ok := toText(item[column.name]); ok {
				row.values[column.name] = text
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// quoteColumns quotes column names for a raw select ("key" is a keyword).
func quoteColumns(tx *gorm.DB, names []string) []string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = tx.Statement.Quote(name)
	}
	return quoted
}

func toUint64(value any) uint64 {
	switch typed := value.(type) {
	case int64:
		return uint64(typed) // #nosec G115 -- legacy ids are positive.
	case int32:
		return uint64(typed) // #nosec G115 -- legacy ids are positive.
	case int:
		return uint64(typed) // #nosec G115 -- legacy ids are positive.
	case uint64:
		return typed
	case uint32:
		return uint64(typed)
	case uint:
		return uint64(typed)
	case []byte:
		var id uint64
		_, _ = fmt.Sscan(string(typed), &id)
		return id
	case string:
		var id uint64
		_, _ = fmt.Sscan(typed, &id)
		return id
	}
	return 0
}

func toText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	case *string:
		if typed == nil {
			return "", false
		}
		return *typed, true
	}
	return fmt.Sprint(value), true
}

// tombstoneRows rewrites the secret columns of the given legacy rows of
// spec's table to their finalized form (section 4.4), in tx. Call it after
// the new tables hold the rows' secrets (syncRows). Already-finalized
// values are left as they are, so it is idempotent. It answers how many
// legacy rows it changed.
//
// A JSON column with a secret keeps its original document in the
// _original scope, so unsplit can restore it byte for byte; an original
// that no longer matches the column (the column was rewritten since) is
// removed, so no secret outlives its document there.
func tombstoneRows(tx *gorm.DB, spec tableSpec, ids []uint64, lock bool, now time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := loadLegacyRows(tx, spec, ids, lock)
	if err != nil {
		return 0, err
	}
	originals, err := originalValues(tx, spec.table, ids)
	if err != nil {
		return 0, err
	}
	var changed int64
	for _, row := range rows {
		updates := map[string]any{}
		for _, column := range legacyColumns[spec.table] {
			value, present := row.values[column.name]
			if !present {
				continue
			}
			switch column.kind {
			case columnTombstone:
				if value == "" || IsTombstone(value) {
					continue
				}
				updates[column.name] = Tombstone(row.id)
			case columnDocument:
				key := secretOwner{owner: row.id, column: column.name}
				if hasLiteralSecret(value) {
					redacted := Redact(value)
					if err := keepOriginal(tx, spec.table, row.id, column.name, value, now); err != nil {
						return changed, err
					}
					updates[column.name] = redacted
					continue
				}
				if original, ok := originals[key]; ok && Redact(original) != value {
					if err := dropOriginals(tx, spec.table, []uint64{row.id}, column.name); err != nil {
						return changed, err
					}
				}
			}
		}
		if len(updates) == 0 {
			continue
		}
		// Table, not Model: a tombstone changes no timestamp.
		if err := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Where("id = ?", row.id).UpdateColumns(updates).Error; err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// hasLiteralSecret reports whether a JSON column holds a secret that is not
// the placeholder: one a finalized column must not keep.
func hasLiteralSecret(document string) bool {
	for _, position := range SecretPositions(document) {
		if !position.Moved {
			return true
		}
	}
	return false
}

// originalValues loads the kept originals of the given rows of table, by
// row and column.
func originalValues(tx *gorm.DB, table string, owners []uint64) (map[secretOwner]string, error) {
	out := map[secretOwner]string{}
	var rows []model.ProtocolSecret
	if err := tx.Session(&gorm.Session{NewDB: true}).Select("owner_id", "column_name", "value").
		Where("scope = ? AND owner_id IN ? AND column_name LIKE ?", scopeLegacyOriginal, owners, table+".%").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[secretOwner{owner: row.OwnerID, column: strings.TrimPrefix(row.ColumnName, table+".")}] = row.Value
	}
	return out, nil
}

// keepOriginal stores the original value of one legacy column, replacing
// an older one.
func keepOriginal(tx *gorm.DB, table string, owner uint64, column, value string, now time.Time) error {
	if err := dropOriginals(tx, table, []uint64{owner}, column); err != nil {
		return err
	}
	return tx.Create(&model.ProtocolSecret{
		Scope: scopeLegacyOriginal, OwnerID: owner, ColumnName: originalColumn(table, column), Value: value, Version: 1, UpdatedAt: now,
	}).Error
}

// dropOriginals removes the kept originals of the given rows of table; an
// empty column removes every column's.
func dropOriginals(tx *gorm.DB, table string, owners []uint64, column string) error {
	if len(owners) == 0 {
		return nil
	}
	query := tx.Session(&gorm.Session{NewDB: true}).Where("scope = ? AND owner_id IN ?", scopeLegacyOriginal, owners)
	if column != "" {
		query = query.Where("column_name = ?", originalColumn(table, column))
	} else {
		query = query.Where("column_name LIKE ?", table+".%")
	}
	return query.Delete(&model.ProtocolSecret{}).Error
}

// tablePhaseInTx reads table's phase in tx, uncached: a writer's Sync
// follows a finalize at once, whatever its process cached.
func tablePhaseInTx(tx *gorm.DB, table string) (string, error) {
	if !tx.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return "", nil
	}
	var phases []string
	err := tx.Session(&gorm.Session{NewDB: true}).Model(&model.NodeSecretSplit{}).
		Where("table_name = ?", table).Limit(1).Pluck("phase", &phases).Error
	if err != nil || len(phases) == 0 {
		return "", err
	}
	return phases[0], nil
}
