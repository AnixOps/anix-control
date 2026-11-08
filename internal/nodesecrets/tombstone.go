package nodesecrets

import (
	"fmt"

	"gorm.io/gorm"
)

// This file writes the tombstones of section 4.4. A legacy secret column
// holds no secret:
//
//   - a key column (v2_wireguard_peer.private_key and .preshared_key) holds
//     the tombstone "!moved:<row id>": unique per row, non-empty, and never
//     a key;
//   - a JSON column (v2_node.raw_config, the five settings columns of
//     v2_node_protocol) holds its redacted document, Redact of the
//     original, with the placeholder at every secret position.
//
// Tombstones are value writes; no table is altered.

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
// table.
func loadLegacyRows(tx *gorm.DB, spec tableSpec, ids []uint64) ([]legacyRow, error) {
	columns := legacyColumns[spec.table]
	selected := make([]string, 0, len(columns)+1)
	selected = append(selected, "id")
	for _, column := range columns {
		selected = append(selected, column.name)
	}
	var raw []map[string]any
	if err := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Select(quoteColumns(tx, selected)).Where("id IN ?", ids).Order("id").Find(&raw).Error; err != nil {
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
// values are left as they are, so it is idempotent.
func tombstoneRows(tx *gorm.DB, spec tableSpec, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := loadLegacyRows(tx, spec, ids)
	if err != nil {
		return err
	}
	for _, row := range rows {
		updates := map[string]any{}
		for _, column := range legacyColumns[spec.table] {
			value, present := row.values[column.name]
			if !present {
				continue
			}
			switch column.kind {
			case columnTombstone:
				if value != "" && !IsTombstone(value) {
					updates[column.name] = Tombstone(row.id)
				}
			case columnDocument:
				if hasLiteralSecret(value) {
					updates[column.name] = Redact(value)
				}
			}
		}
		if len(updates) == 0 {
			continue
		}
		// Table, not Model: a tombstone changes no timestamp.
		if err := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Where("id = ?", row.id).UpdateColumns(updates).Error; err != nil {
			return err
		}
	}
	return nil
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
