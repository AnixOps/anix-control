package nodesecrets

import (
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// secretEntry is a protocol secret as a legacy column holds it.
type secretEntry struct {
	OwnerID uint64
	Column  string
	Pointer string
	Value   string
}

type secretKey struct {
	OwnerID uint64
	Column  string
	Pointer string
}

// derivation is what the legacy rows of a set of ids hold. kept lists the
// positions whose legacy value is a tombstone or the placeholder: their
// secret lives in the new table only, and the writer leaves it there.
type derivation struct {
	secrets     []secretEntry
	keptSecrets map[secretKey]bool
}

func newDerivation() *derivation {
	return &derivation{keptSecrets: map[secretKey]bool{}}
}

func (d *derivation) addPositions(ownerID uint64, column string, document *string) {
	if document == nil {
		return
	}
	for _, position := range SecretPositions(*document) {
		if position.Moved {
			d.keptSecrets[secretKey{OwnerID: ownerID, Column: column, Pointer: position.Pointer}] = true
			continue
		}
		d.secrets = append(d.secrets, secretEntry{OwnerID: ownerID, Column: column, Pointer: position.Pointer, Value: position.Value})
	}
}

// addPeerKey adds a WireGuard peer's key; a tombstone keeps the new row.
func (d *derivation) addPeerKey(ownerID uint64, column, value string) {
	switch {
	case IsTombstone(value):
		d.keptSecrets[secretKey{OwnerID: ownerID, Column: column}] = true
	case value != "":
		d.secrets = append(d.secrets, secretEntry{OwnerID: ownerID, Column: column, Value: value})
	}
}

// tableSpec says which rows of the new tables a legacy table owns, and how
// its rows are derived.
type tableSpec struct {
	table string
	// scope selects the table's protocol secrets.
	scope  string
	derive func(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error)
}

var specs = map[string]tableSpec{
	TableNode:          {table: TableNode, scope: ScopeNodeRawConfig, derive: deriveNodes},
	TableNodeProtocol:  {table: TableNodeProtocol, scope: ScopeNodeProtocol, derive: deriveNodeProtocols},
	TableWireGuardPeer: {table: TableWireGuardPeer, scope: ScopeWireGuardPeer, derive: deriveWireGuardPeers},
}

func lookupSpec(table string) (tableSpec, error) {
	spec, ok := specs[table]
	if !ok {
		return tableSpec{}, fmt.Errorf("nodesecrets: %q is not a split table", table)
	}
	return spec, nil
}

// legacyRows reads the given legacy rows; lock takes their row locks
// (PostgreSQL), so a backfill batch and a concurrent writer of the same row
// run one after the other.
func legacyRows(tx *gorm.DB, value any, columns []string, ids []uint64, lock bool, rows any) error {
	query := tx.Model(value).Select(columns).Where("id IN ?", ids).Order("id")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return query.Find(rows).Error
}

func deriveNodes(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.Node
	if err := legacyRows(tx, &model.Node{}, []string{"id", "raw_config"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		d.addPositions(uint64(row.ID), "raw_config", row.RawConfig)
	}
	return d, nil
}

// nodeProtocolColumns are the JSON columns of v2_node_protocol that can hold
// secrets.
var nodeProtocolColumns = []string{"settings", "tls_settings", "transport_settings", "reality_settings", "custom_config"}

func deriveNodeProtocols(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.NodeProtocol
	if err := legacyRows(tx, &model.NodeProtocol{}, append([]string{"id"}, nodeProtocolColumns...), ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		d.addPositions(id, "settings", row.Settings)
		d.addPositions(id, "tls_settings", row.TLSSettings)
		d.addPositions(id, "transport_settings", row.TransportSettings)
		d.addPositions(id, "reality_settings", row.RealitySettings)
		d.addPositions(id, "custom_config", row.CustomConfig)
	}
	return d, nil
}

func deriveWireGuardPeers(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.WireGuardPeer
	if err := legacyRows(tx, &model.WireGuardPeer{}, []string{"id", "private_key", "preshared_key"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		d.addPeerKey(id, "private_key", row.PrivateKey)
		d.addPeerKey(id, "preshared_key", row.PresharedKey)
	}
	return d, nil
}
