// Package nodesecrets is the one writer of the node credential split
// (docs/architecture/node-ops-service.md, section 4).
//
// The split moves node credentials and protocol secrets out of the legacy
// tables (v2_node, v2_authorized_key, v2_forward_node,
// v2_forward_clean_agent, v2_node_protocol, v2_wireguard_peer) into the
// protected kernel tables v4_kernel_node_credential and
// v4_kernel_protocol_secret. Existing tables are never altered.
//
// Phase P1 (dual_write): every kernel writer of a moved column calls Sync in
// the transaction of its legacy write, and Sync derives the new rows from the
// legacy rows it just wrote, so the two forms cannot drift. Backfill copies
// the rows written before P1 and Verify compares the digests of both forms.
//
// Phase P2 (dual_read): every kernel reader of a moved column reads through
// this package (read.go), which applies each table's phase. SetPhase moves a
// table's readers to dual_read once a recent verification matched, and back
// to dual_write. Writers dual-write in both phases.
//
// Secret values are never logged, printed or put in an error.
package nodesecrets

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The legacy tables the split moves secrets out of.
const (
	TableNode          = "v2_node"
	TableAuthorizedKey = "v2_authorized_key"
	TableForwardNode   = "v2_forward_node"
	TableCleanAgent    = "v2_forward_clean_agent"
	TableNodeProtocol  = "v2_node_protocol"
	TableWireGuardPeer = "v2_wireguard_peer"
)

// Tables lists the split tables in backfill order.
func Tables() []string {
	return []string{TableNode, TableAuthorizedKey, TableForwardNode, TableCleanAgent, TableNodeProtocol, TableWireGuardPeer}
}

// The phases of a table (section 4.3).
const (
	PhaseLegacy    = "legacy"
	PhaseDualWrite = "dual_write"
	PhaseDualRead  = "dual_read"
	PhaseFinalized = "finalized"
)

// Subject kinds of v4_kernel_node_credential.
const (
	SubjectProxy           = "proxy"
	SubjectForward         = "forward"
	SubjectCleanAgent      = "clean_agent"
	SubjectRegistrationKey = "registration_key"
	SubjectAgentEnrollment = "agent_enrollment"
)

// Credential kinds.
const (
	KindNodeAPIKey       = "node_api_key"       // #nosec G101 -- this names a credential kind, not a hardcoded credential.
	KindNodeSharedSecret = "node_shared_secret" // #nosec G101 -- this names a credential kind, not a hardcoded credential.
	KindRegistrationKey  = "registration_key"
	KindForwardNodeToken = "forward_node_token"
	KindCleanAgentToken  = "clean_agent_token"
)

// Credential statuses.
const (
	StatusActive  = "active"
	StatusRetired = "retired"
	StatusRevoked = "revoked"
)

// Sources: who wrote a credential's current version.
const (
	SourceBackfill  = "backfill"
	SourceDualWrite = "dual_write"
	SourceIssued    = "issued"
)

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

// Tombstone is the value of a legacy unique credential column once its
// secret has moved (v2_node.api_key, v2_authorized_key.key,
// v2_forward_clean_agent.token): unique per row, non-empty, and never a
// credential.
func Tombstone(id uint64) string {
	return tombstonePrefix + strconv.FormatUint(id, 10)
}

// IsTombstone reports whether a legacy credential value is a tombstone.
// The writer never copies one, and never removes the new row it stands
// for.
func IsTombstone(value string) bool {
	return strings.HasPrefix(value, tombstonePrefix)
}

// EnsureSchema creates the split tables and gives every split table its
// state row, in phase dual_write: the release that ships the writer starts
// P1. It never changes the phase of an existing row, and never alters a
// legacy table.
func EnsureSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, table := range Tables() {
		row := model.NodeSecretSplit{Table: table, Phase: PhaseDualWrite, UpdatedAt: now}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// installed reports whether the split tables exist. Control creates them at
// startup, before any writer runs; a database without them (a tool's or a
// test's that never created the kernel schema) has nothing to dual-write.
func installed(tx *gorm.DB) bool {
	return tx.Migrator().HasTable(&model.NodeCredential{}) && tx.Migrator().HasTable(&model.ProtocolSecret{})
}

// hashSecret is the SHA-256 (hex) of a secret, as the legacy key hash
// columns store it.
func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
