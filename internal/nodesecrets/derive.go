package nodesecrets

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// credentialEntry is a credential as the legacy columns of one row hold it.
type credentialEntry struct {
	SubjectID uint64
	Kind      string
	KeyHash   string
	Value     string
	Endpoint  string
	Status    string
	ExpiresAt *time.Time
	RevokedAt *time.Time
}

// secretEntry is a protocol secret as a legacy column holds it.
type secretEntry struct {
	OwnerID uint64
	Column  string
	Pointer string
	Value   string
}

type credentialKey struct {
	SubjectID uint64
	Kind      string
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
	credentials     []credentialEntry
	secrets         []secretEntry
	keptCredentials map[credentialKey]bool
	keptSecrets     map[secretKey]bool
}

func newDerivation() *derivation {
	return &derivation{keptCredentials: map[credentialKey]bool{}, keptSecrets: map[secretKey]bool{}}
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

// tableSpec says which rows of the new tables a legacy table owns, and how
// its rows are derived.
type tableSpec struct {
	table string
	// subjectKind and kinds select the table's credentials; scope its
	// protocol secrets. A table has either or both.
	subjectKind string
	kinds       []string
	scope       string
	derive      func(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error)
}

var specs = map[string]tableSpec{
	TableNode: {
		table: TableNode, subjectKind: SubjectProxy, kinds: []string{KindNodeAPIKey, KindNodeSharedSecret},
		scope: ScopeNodeRawConfig, derive: deriveNodes,
	},
	TableAuthorizedKey: {
		table: TableAuthorizedKey, subjectKind: SubjectRegistrationKey, kinds: []string{KindRegistrationKey},
		derive: deriveAuthorizedKeys,
	},
	TableForwardNode: {
		table: TableForwardNode, subjectKind: SubjectForward, kinds: []string{KindForwardNodeToken},
		derive: deriveForwardNodes,
	},
	TableCleanAgent: {
		table: TableCleanAgent, subjectKind: SubjectCleanAgent, kinds: []string{KindCleanAgentToken},
		derive: deriveCleanAgents,
	},
	TableNodeProtocol: {
		table: TableNodeProtocol, scope: ScopeNodeProtocol, derive: deriveNodeProtocols,
	},
	TableWireGuardPeer: {
		table: TableWireGuardPeer, scope: ScopeWireGuardPeer, derive: deriveWireGuardPeers,
	},
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
	if err := legacyRows(tx, &model.Node{}, []string{"id", "api_key", "api_key_hash", "secret", "raw_config"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		switch {
		case IsTombstone(row.APIKey):
			d.keptCredentials[credentialKey{SubjectID: id, Kind: KindNodeAPIKey}] = true
		case row.APIKey != "" || row.APIKeyHash != "":
			// The legacy readers match the stored hash, or the key itself
			// when no hash is stored.
			keyHash := row.APIKeyHash
			if keyHash == "" {
				keyHash = hashSecret(row.APIKey)
			}
			d.credentials = append(d.credentials, credentialEntry{
				SubjectID: id, Kind: KindNodeAPIKey, KeyHash: keyHash, Value: row.APIKey, Status: StatusActive,
			})
		}
		if row.Secret != "" {
			d.credentials = append(d.credentials, credentialEntry{
				SubjectID: id, Kind: KindNodeSharedSecret, KeyHash: hashSecret(row.Secret), Value: row.Secret, Status: StatusActive,
			})
		}
		d.addPositions(id, "raw_config", row.RawConfig)
	}
	return d, nil
}

func deriveAuthorizedKeys(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.AuthorizedKey
	if err := legacyRows(tx, &model.AuthorizedKey{}, []string{"id", "key", "key_hash", "expire_at"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		switch {
		case IsTombstone(row.Key):
			d.keptCredentials[credentialKey{SubjectID: id, Kind: KindRegistrationKey}] = true
		case row.Key != "" || row.KeyHash != "":
			keyHash := row.KeyHash
			if keyHash == "" {
				keyHash = hashSecret(row.Key)
			}
			entry := credentialEntry{SubjectID: id, Kind: KindRegistrationKey, KeyHash: keyHash, Value: row.Key, Status: StatusActive}
			if row.ExpireAt != nil {
				expires := time.Unix(*row.ExpireAt, 0).UTC()
				entry.ExpiresAt = &expires
			}
			d.credentials = append(d.credentials, entry)
		}
	}
	return d, nil
}

func deriveForwardNodes(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.ForwardNode
	if err := legacyRows(tx, &model.ForwardNode{}, []string{"id", "host", "api_port", "api_token"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		switch {
		case IsTombstone(row.APIToken):
			d.keptCredentials[credentialKey{SubjectID: id, Kind: KindForwardNodeToken}] = true
		case row.APIToken != "":
			d.credentials = append(d.credentials, credentialEntry{
				SubjectID: id, Kind: KindForwardNodeToken, KeyHash: hashSecret(row.APIToken), Value: row.APIToken,
				Endpoint: forwardNodeEndpoint(row.Host, row.APIPort), Status: StatusActive,
			})
		}
	}
	return d, nil
}

// forwardNodeEndpoint is the address a forward node's token is presented
// to: the node's management API. A node without an API port is unpinned.
func forwardNodeEndpoint(host string, apiPort int) string {
	if host == "" || apiPort <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(apiPort))
}

func deriveCleanAgents(tx *gorm.DB, ids []uint64, lock bool) (*derivation, error) {
	var rows []model.ForwardCleanAgent
	if err := legacyRows(tx, &model.ForwardCleanAgent{}, []string{"id", "token", "status", "revoked_at"}, ids, lock, &rows); err != nil {
		return nil, err
	}
	d := newDerivation()
	for _, row := range rows {
		id := uint64(row.ID)
		switch {
		case IsTombstone(row.Token):
			d.keptCredentials[credentialKey{SubjectID: id, Kind: KindCleanAgentToken}] = true
		case row.Token != "":
			entry := credentialEntry{
				SubjectID: id, Kind: KindCleanAgentToken, KeyHash: hashSecret(row.Token), Value: row.Token, Status: StatusActive,
			}
			// The legacy check: a revoked status or a revocation time.
			if row.Status == model.ForwardCleanAgentStatusRevoked || row.RevokedAt != nil {
				entry.Status = StatusRevoked
				entry.RevokedAt = row.RevokedAt
			}
			d.credentials = append(d.credentials, entry)
		}
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
		if row.PrivateKey != "" {
			d.secrets = append(d.secrets, secretEntry{OwnerID: id, Column: "private_key", Value: row.PrivateKey})
		}
		if row.PresharedKey != "" {
			d.secrets = append(d.secrets, secretEntry{OwnerID: id, Column: "preshared_key", Value: row.PresharedKey})
		}
	}
	return d, nil
}
