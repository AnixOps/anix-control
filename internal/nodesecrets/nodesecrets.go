// Package nodesecrets is the one writer of the node credential split
// (docs/architecture/node-ops-service.md, section 4).
//
// The split moves protocol secrets out of the legacy tables (v2_node's raw
// configuration, v2_node_protocol, v2_wireguard_peer) into the protected
// kernel table v4_kernel_protocol_secret. Existing tables are never altered.
//
// Every table is finalized (phase P3; the earlier phases and the commands
// that moved a table through them were removed in 5.0.3). Every kernel
// writer of a moved column calls Sync in the transaction of its legacy
// write: Sync moves the secrets to the new table and leaves tombstones and
// redacted documents in the legacy columns. Every kernel reader reads
// through this package (read.go), from the new table only. Control refuses
// to start on a database whose split is not finalized (RequireFinalized).
//
// Secret values are never logged, printed or put in an error.
package nodesecrets

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The legacy tables the split moves secrets out of.
const (
	TableNode          = "v2_node"
	TableNodeProtocol  = "v2_node_protocol"
	TableWireGuardPeer = "v2_wireguard_peer"
)

// Tables lists the split tables.
func Tables() []string {
	return []string{TableNode, TableNodeProtocol, TableWireGuardPeer}
}

// PhaseFinalized is the phase of every split table (section 4.3).
const PhaseFinalized = "finalized"

// Scopes of v4_kernel_protocol_secret.
const (
	ScopeNodeProtocol  = "node_protocol"
	ScopeNodeRawConfig = "node_raw_config"
	ScopeWireGuardPeer = "wireguard_peer"
)

// tombstonePrefix starts the value a finalized unique column keeps
// (section 4.4). Generated keys and tokens are hex or base64 and never
// contain "!".
const tombstonePrefix = "!moved:"

// Tombstone is the value of a legacy key column once its secret has moved
// (v2_wireguard_peer.private_key and .preshared_key): unique per row,
// non-empty, and never a key.
func Tombstone(id uint64) string {
	return tombstonePrefix + strconv.FormatUint(id, 10)
}

// IsTombstone reports whether a legacy credential value is a tombstone.
// The writer never copies one, and never removes the new row it stands
// for.
func IsTombstone(value string) bool {
	return strings.HasPrefix(value, tombstonePrefix)
}

// RequireFinalized refuses a database whose node secret split is not
// finalized: a split table whose state row is not finalized with
// finalized_at recorded, and whose legacy table holds rows. A fresh
// database passes. It only reads, so Control runs it before the schema
// migrations and a refused start leaves the database as it was.
func RequireFinalized(db *gorm.DB) error {
	db = newSession(db)
	states := map[string]model.NodeSecretSplit{}
	if db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		var rows []model.NodeSecretSplit
		if err := db.Find(&rows).Error; err != nil {
			return fmt.Errorf("read the node secret split state: %w", err)
		}
		for _, row := range rows {
			states[row.Table] = row
		}
	}
	for _, table := range Tables() {
		state := states[table]
		if state.Phase == PhaseFinalized && state.FinalizedAt != nil || !db.Migrator().HasTable(table) {
			continue
		}
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if count == 0 {
			continue
		}
		phase := state.Phase
		if phase == "" {
			phase = "none"
		}
		return fmt.Errorf("node secret split: table %s is in phase %s (finalize not completed). This release reads finalized tables only. "+
			"On 5.0.2, finish the split (backfill, verify, phase all dual_read, finalize -confirm), "+
			"then start this release (docs/UPGRADE.md, \"Upgrading To 5.0.3\")", table, phase)
	}
	return nil
}

// EnsureStateRows records every split table as finalized: a missing state
// row is created so, and an unfinalized one (its legacy table is empty,
// RequireFinalized saw to that) is marked so. Control runs it at every
// start, after the schema migrations created the split tables.
func EnsureStateRows(db *gorm.DB) error {
	now := time.Now().UTC()
	for _, table := range Tables() {
		row := model.NodeSecretSplit{Table: table, Phase: PhaseFinalized, FinalizedAt: &now, FinalizedBy: "bootstrap", UpdatedAt: now}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return db.Model(&model.NodeSecretSplit{}).Where("phase <> ? OR finalized_at IS NULL", PhaseFinalized).Updates(map[string]any{
		"phase": PhaseFinalized, "finalized_at": now, "finalized_by": "bootstrap", "updated_at": now,
	}).Error
}

// MigrateForTest creates the split tables with GORM's AutoMigrate and their
// state rows, on a test's or a tool's own database; Control's are created
// by internal/database/migrations.
func MigrateForTest(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.ProtocolSecret{}, &model.NodeSecretSplit{}); err != nil {
		return err
	}
	return EnsureStateRows(db)
}

// SeedForTest moves the secrets of the rows a test wrote straight into the
// legacy tables (every split table when none is named) to the new tables,
// as the kernel's writers would have: MigrateForTest, then Sync of every
// row; a table the database lacks is skipped. The legacy columns then hold
// tombstones and redacted documents.
func SeedForTest(db *gorm.DB, tables ...string) error {
	if err := MigrateForTest(db); err != nil {
		return err
	}
	if len(tables) == 0 {
		tables = Tables()
	}
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		var ids []uint
		if err := db.Table(table).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if err := Sync(db, table, ids...); err != nil {
			return err
		}
	}
	return nil
}

// installed reports whether the split tables exist. Control creates them at
// startup, before any writer runs; a database without them (a tool's or a
// test's that never created the kernel schema) has nothing to move.
func installed(tx *gorm.DB) bool {
	return tx.Migrator().HasTable(&model.ProtocolSecret{})
}
