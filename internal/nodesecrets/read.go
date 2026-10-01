package nodesecrets

import (
	"crypto/subtle"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file is the read side of phase P2 (dual_read, section 4.3). Every
// kernel reader of a moved column reads through it:
//
//   - In phase dual_write (and legacy) a reader reads the legacy column
//     exactly as before the split, and never the new tables.
//   - In phase dual_read (and finalized) it reads the new tables. A missing
//     or mismatching row falls back to the legacy column, which still holds
//     every secret until P3, counts anixops_node_secrets_fallback_total and
//     logs once per subject, without the value.
//
// No reader accepts a tombstone or the placeholder as a credential, in any
// phase (Unusable).

// PhaseCacheTTL bounds how long a process keeps the phases it read. A phase
// change made by the node-secrets command, in another process, reaches every
// Control process within it.
const PhaseCacheTTL = 5 * time.Second

type phaseEntry struct {
	phases    map[string]string
	installed bool
	loaded    time.Time
}

var (
	phaseMu    sync.Mutex
	phaseCache = map[any]phaseEntry{}
)

// phaseCacheKey is the connection pool a handle reads: every session and
// transaction of one database shares it.
func phaseCacheKey(db *gorm.DB) any {
	if db == nil || db.Config == nil {
		return nil
	}
	return db.ConnPool
}

// ReadPhase answers the phase whose read rule table's readers apply. A
// database without the split tables, a table without a state row, an
// unknown phase and a failed read all answer dual_write: the legacy
// columns. The phases are cached for PhaseCacheTTL.
func ReadPhase(db *gorm.DB, table string) string {
	switch phase := cachedPhases(db).phases[table]; phase {
	case PhaseDualRead, PhaseFinalized, PhaseLegacy:
		return phase
	default:
		return PhaseDualWrite
	}
}

// SplitInstalled reports whether db holds the split tables (EnsureSchema
// ran): a database without them (a tool's, or a test's that never created
// the kernel schema) has no credential rows and no endpoint pins. It is
// cached with the phases.
func SplitInstalled(db *gorm.DB) bool {
	return cachedPhases(db).installed
}

// cachedPhases answers the phases of db, loaded at most every
// PhaseCacheTTL.
func cachedPhases(db *gorm.DB) phaseEntry {
	key := phaseCacheKey(db)
	if key == nil {
		return phaseEntry{}
	}
	phaseMu.Lock()
	entry, ok := phaseCache[key]
	phaseMu.Unlock()
	if !ok || time.Since(entry.loaded) >= PhaseCacheTTL {
		phases, installed := loadPhases(db)
		entry = phaseEntry{phases: phases, installed: installed, loaded: time.Now()}
		phaseMu.Lock()
		phaseCache[key] = entry
		phaseMu.Unlock()
	}
	return entry
}

// loadPhases reads the split state table; it reports whether the split
// tables exist.
func loadPhases(db *gorm.DB) (map[string]string, bool) {
	session := db.Session(&gorm.Session{NewDB: true})
	if !session.Migrator().HasTable(&model.NodeSecretSplit{}) || !installed(session) {
		return nil, false
	}
	var rows []model.NodeSecretSplit
	if err := session.Select("table_name", "phase").Find(&rows).Error; err != nil {
		return nil, true
	}
	phases := make(map[string]string, len(rows))
	for _, row := range rows {
		phases[row.Table] = row.Phase
	}
	return phases, true
}

// invalidatePhases drops the cached phases of db, so this process reads a
// change it just made at once.
func invalidatePhases(db *gorm.DB) {
	if key := phaseCacheKey(db); key != nil {
		phaseMu.Lock()
		delete(phaseCache, key)
		phaseMu.Unlock()
	}
}

// readsNew reports whether table's readers read the new tables.
func readsNew(db *gorm.DB, table string) bool {
	switch ReadPhase(db, table) {
	case PhaseDualRead, PhaseFinalized:
		return true
	default:
		return false
	}
}

// Unusable reports whether a credential value, presented or stored, can
// never authenticate: empty, a tombstone, or the placeholder. P3 writes
// tombstones and the placeholder into the legacy columns (section 4.4), so
// a legacy comparison must never accept them.
func Unusable(value string) bool {
	trimmed := strings.TrimSpace(value)
	return value == "" || IsTombstone(trimmed) || trimmed == Placeholder
}

// secretEqual compares a stored secret with a presented one in constant
// time; an unusable stored or presented value matches nothing.
func secretEqual(stored, presented string) bool {
	if Unusable(stored) || Unusable(presented) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(presented)) == 1
}

// hashEqual compares a stored SHA-256 with the hash of a presented secret.
func hashEqual(stored, presentedHash string) bool {
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(presentedHash)) == 1
}

func newSession(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true})
}

func subjectName(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// currentCredential loads the current version of one subject's credential:
// the newest row that is not retired. It answers nil when there is none.
func currentCredential(db *gorm.DB, subjectKind string, subjectID uint64, kind string) (*model.NodeCredential, error) {
	var rows []model.NodeCredential
	err := newSession(db).Where("subject_kind = ? AND subject_id = ? AND kind = ? AND status <> ?", subjectKind, subjectID, kind, StatusRetired).
		Order("version DESC").Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// credentialSubject answers the subject whose current credential of kind
// has keyHash and one of statuses: the lowest id, as the legacy lookups
// take the first matching row.
func credentialSubject(db *gorm.DB, subjectKind, kind, keyHash string, statuses ...string) (uint64, bool, error) {
	var ids []uint64
	err := newSession(db).Model(&model.NodeCredential{}).
		Where("kind = ? AND key_hash = ? AND subject_kind = ? AND status IN ?", kind, keyHash, subjectKind, statuses).
		Order("subject_id").Limit(1).Pluck("subject_id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, false, err
	}
	return ids[0], true, nil
}

// fallbackReason names why a credential read fell back.
func fallbackReason(err error, row *model.NodeCredential) string {
	switch {
	case err != nil:
		return FallbackError
	case row == nil:
		return FallbackMissing
	default:
		return FallbackMismatch
	}
}

// credentialValue is the dual-read rule for a credential a reader presents
// or shows: the new table's value, or the legacy one, counted, when the new
// row is missing or differs. Where the legacy column no longer holds the
// secret (empty, a tombstone or the placeholder), the new value is the only
// one.
func credentialValue(db *gorm.DB, table, subjectKind, kind string, subjectID uint64, legacy string) string {
	if !readsNew(db, table) {
		return legacy
	}
	row, err := currentCredential(db, subjectKind, subjectID, kind)
	switch {
	case err == nil && row != nil && row.Value != "" && (row.Value == legacy || Unusable(legacy)):
		return row.Value
	case legacy == "":
		// No secret in either form.
		return legacy
	}
	if err == nil && row != nil && row.Value == "" {
		row = nil
	}
	recordFallback(table, kind, fallbackReason(err, row), subjectName(subjectID))
	return legacy
}

// --- Proxy nodes (v2_node) ---

// legacyNodeKeyMatches is the legacy check of a node's API key: its stored
// hash, or, for an old row without a hash, the key itself.
func legacyNodeKeyMatches(node *model.Node, apiKey string) bool {
	if hashEqual(node.APIKeyHash, hashSecret(apiKey)) {
		return true
	}
	return node.APIKeyHash == "" && secretEqual(node.APIKey, apiKey)
}

// NodeAPIKeyMatches reports whether apiKey is the API key of node, a row of
// v2_node the caller loaded by id: UniProxy's node authentication and the
// agent channels. An unusable key never matches.
func NodeAPIKeyMatches(db *gorm.DB, node *model.Node, apiKey string) bool {
	if node == nil || Unusable(apiKey) {
		return false
	}
	if !readsNew(db, TableNode) {
		return legacyNodeKeyMatches(node, apiKey)
	}
	row, err := currentCredential(db, SubjectProxy, uint64(node.ID), KindNodeAPIKey)
	if err == nil && row != nil && row.Status == StatusActive && hashEqual(row.KeyHash, hashSecret(apiKey)) {
		return true
	}
	if !legacyNodeKeyMatches(node, apiKey) {
		return false
	}
	recordFallback(TableNode, KindNodeAPIKey, fallbackReason(err, row), subjectName(uint64(node.ID)))
	return true
}

// NodeByAPIKey answers the proxy node whose API key apiKey is: the gRPC node
// listener, the Agent Control stream and the node API look nodes up by key.
// The legacy lookup matches the stored hash; with plain it also matches the
// key of an old row by the key itself, as the node API always did. It
// answers gorm.ErrRecordNotFound when no node has the key, and for an
// unusable key.
func NodeByAPIKey(db *gorm.DB, apiKey string, plain bool) (*model.Node, error) {
	if Unusable(apiKey) {
		return nil, gorm.ErrRecordNotFound
	}
	keyHash := hashSecret(apiKey)
	newRead := readsNew(db, TableNode)
	var readErr error
	if newRead {
		id, found, err := credentialSubject(db, SubjectProxy, KindNodeAPIKey, keyHash, StatusActive)
		readErr = err
		if found {
			var node model.Node
			if err := newSession(db).First(&node, id).Error; err == nil {
				return &node, nil
			}
		}
	}
	node, err := legacyNodeByAPIKey(db, apiKey, keyHash, plain)
	if err != nil {
		return nil, err
	}
	if newRead {
		reason := FallbackMissing
		if readErr != nil {
			reason = FallbackError
		}
		recordFallback(TableNode, KindNodeAPIKey, reason, subjectName(uint64(node.ID)))
	}
	return node, nil
}

func legacyNodeByAPIKey(db *gorm.DB, apiKey, keyHash string, plain bool) (*model.Node, error) {
	var node model.Node
	err := newSession(db).Where("api_key_hash = ?", keyHash).First(&node).Error
	if err != nil && plain {
		node = model.Node{}
		err = newSession(db).Where("api_key = ?", apiKey).First(&node).Error
	}
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// NodeAPIKey answers the API key of node, where the kernel shows it (the
// administrators' credentials route).
func NodeAPIKey(db *gorm.DB, node *model.Node) string {
	if node == nil {
		return ""
	}
	return credentialValue(db, TableNode, SubjectProxy, KindNodeAPIKey, uint64(node.ID), node.APIKey)
}

// NodeSharedSecret answers the shared signing secret of node: the request
// signature check and the administrators' credentials route.
func NodeSharedSecret(db *gorm.DB, node *model.Node) string {
	if node == nil {
		return ""
	}
	return credentialValue(db, TableNode, SubjectProxy, KindNodeSharedSecret, uint64(node.ID), node.Secret)
}

// --- Registration keys (v2_authorized_key) ---

// LockRegistrationKey loads the registration key key is, with its row lock,
// in tx: node registration. It answers gorm.ErrRecordNotFound when no key
// matches, and for an unusable key. Expiry stays the caller's check, on the
// row.
func LockRegistrationKey(tx *gorm.DB, key string) (*model.AuthorizedKey, error) {
	if Unusable(key) {
		return nil, gorm.ErrRecordNotFound
	}
	keyHash := hashSecret(key)
	newRead := readsNew(tx, TableAuthorizedKey)
	var readErr error
	if newRead {
		id, found, err := credentialSubject(tx, SubjectRegistrationKey, KindRegistrationKey, keyHash, StatusActive)
		readErr = err
		if found {
			var authKey model.AuthorizedKey
			if err := newSession(tx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&authKey, id).Error; err == nil {
				return &authKey, nil
			}
		}
	}
	var authKey model.AuthorizedKey
	if err := newSession(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("key_hash = ?", keyHash).First(&authKey).Error; err != nil {
		return nil, err
	}
	if newRead {
		reason := FallbackMissing
		if readErr != nil {
			reason = FallbackError
		}
		recordFallback(TableAuthorizedKey, KindRegistrationKey, reason, subjectName(uint64(authKey.ID)))
	}
	return &authKey, nil
}

// --- Forward nodes (v2_forward_node) ---

// ForwardNodeTokenMatches reports whether token is the API token of the
// forward node node: its agent's WebSocket and REST calls. An unusable
// token never matches.
func ForwardNodeTokenMatches(db *gorm.DB, node *model.ForwardNode, token string) bool {
	if node == nil || Unusable(token) {
		return false
	}
	if !readsNew(db, TableForwardNode) {
		return secretEqual(node.APIToken, token)
	}
	row, err := currentCredential(db, SubjectForward, uint64(node.ID), KindForwardNodeToken)
	if err == nil && row != nil && row.Status == StatusActive && hashEqual(row.KeyHash, hashSecret(token)) {
		return true
	}
	if !secretEqual(node.APIToken, token) {
		return false
	}
	recordFallback(TableForwardNode, KindForwardNodeToken, fallbackReason(err, row), subjectName(uint64(node.ID)))
	return true
}

// ForwardNodeToken answers the API token of the forward node node, where
// the kernel presents it: the gost API and the NodeX payloads.
func ForwardNodeToken(db *gorm.DB, node *model.ForwardNode) string {
	if node == nil {
		return ""
	}
	return credentialValue(db, TableForwardNode, SubjectForward, KindForwardNodeToken, uint64(node.ID), node.APIToken)
}

// --- Clean agents (v2_forward_clean_agent) ---

// CleanAgentByToken loads the clean agent whose token token is: clean agent
// registration. token is trimmed by the caller. A revoked agent is loaded;
// revocation stays the caller's check, on the row. It answers
// gorm.ErrRecordNotFound when no agent has the token, and for an unusable
// token.
func CleanAgentByToken(db *gorm.DB, token string) (*model.ForwardCleanAgent, error) {
	if Unusable(token) {
		return nil, gorm.ErrRecordNotFound
	}
	newRead := readsNew(db, TableCleanAgent)
	var readErr error
	if newRead {
		id, found, err := credentialSubject(db, SubjectCleanAgent, KindCleanAgentToken, hashSecret(token), StatusActive, StatusRevoked)
		readErr = err
		if found {
			var agent model.ForwardCleanAgent
			if err := newSession(db).First(&agent, id).Error; err == nil {
				return &agent, nil
			}
		}
	}
	var agent model.ForwardCleanAgent
	if err := newSession(db).Where("token = ?", token).First(&agent).Error; err != nil {
		return nil, err
	}
	if newRead {
		reason := FallbackMissing
		if readErr != nil {
			reason = FallbackError
		}
		recordFallback(TableCleanAgent, KindCleanAgentToken, reason, subjectName(uint64(agent.ID)))
	}
	return &agent, nil
}

// CleanAgentWithToken loads clean agent agentID when token is its token:
// the heartbeat and report calls. It answers gorm.ErrRecordNotFound
// otherwise, and for an unusable token.
func CleanAgentWithToken(db *gorm.DB, agentID uint, token string) (*model.ForwardCleanAgent, error) {
	if agentID == 0 || Unusable(token) {
		return nil, gorm.ErrRecordNotFound
	}
	if readsNew(db, TableCleanAgent) {
		row, err := currentCredential(db, SubjectCleanAgent, uint64(agentID), KindCleanAgentToken)
		if err == nil && row != nil && hashEqual(row.KeyHash, hashSecret(token)) {
			var agent model.ForwardCleanAgent
			if err := newSession(db).First(&agent, agentID).Error; err == nil {
				return &agent, nil
			}
		}
		agent, legacyErr := legacyCleanAgentWithToken(db, agentID, token)
		if legacyErr != nil {
			return nil, legacyErr
		}
		recordFallback(TableCleanAgent, KindCleanAgentToken, fallbackReason(err, row), subjectName(uint64(agentID)))
		return agent, nil
	}
	return legacyCleanAgentWithToken(db, agentID, token)
}

func legacyCleanAgentWithToken(db *gorm.DB, agentID uint, token string) (*model.ForwardCleanAgent, error) {
	var agent model.ForwardCleanAgent
	if err := newSession(db).Where("id = ? AND token = ?", agentID, token).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// --- Protocol secrets (v2_node_protocol, v2_node.raw_config, v2_wireguard_peer) ---

// ResolveProtocols gives the secret positions of the protocols' JSON
// columns their values under the dual-read rule, in place, where the
// kernel builds a node's or a user's configuration from them. In phase
// dual_write it changes nothing. A column is rewritten only where a
// position takes a value its legacy document does not hold (the
// placeholder in it); otherwise it stays byte for byte as stored.
func ResolveProtocols(db *gorm.DB, protocols []model.NodeProtocol) {
	if len(protocols) == 0 || !readsNew(db, TableNodeProtocol) {
		return
	}
	pointers := make([]*model.NodeProtocol, len(protocols))
	for i := range protocols {
		pointers[i] = &protocols[i]
	}
	resolveProtocols(db, pointers)
}

// ResolveProtocol is ResolveProtocols for one protocol.
func ResolveProtocol(db *gorm.DB, protocol *model.NodeProtocol) {
	if protocol == nil || !readsNew(db, TableNodeProtocol) {
		return
	}
	resolveProtocols(db, []*model.NodeProtocol{protocol})
}

func resolveProtocols(db *gorm.DB, protocols []*model.NodeProtocol) {
	ids := make([]uint64, 0, len(protocols))
	for _, protocol := range protocols {
		ids = append(ids, uint64(protocol.ID))
	}
	rows, err := secretRows(db, ScopeNodeProtocol, ids)
	for _, protocol := range protocols {
		owner := uint64(protocol.ID)
		for _, column := range []struct {
			name  string
			value **string
		}{
			{"settings", &protocol.Settings},
			{"tls_settings", &protocol.TLSSettings},
			{"transport_settings", &protocol.TransportSettings},
			{"reality_settings", &protocol.RealitySettings},
			{"custom_config", &protocol.CustomConfig},
		} {
			if *column.value == nil {
				continue
			}
			resolved := resolveDocument(TableNodeProtocol, column.name, owner, **column.value, rows[secretOwner{owner, column.name}], err)
			if resolved != **column.value {
				*column.value = &resolved
			}
		}
	}
}

// ResolveNodeRawConfig gives the secret positions of node's raw
// configuration their values under the dual-read rule, in place.
func ResolveNodeRawConfig(db *gorm.DB, node *model.Node) {
	if node == nil || node.RawConfig == nil || !readsNew(db, TableNode) {
		return
	}
	owner := uint64(node.ID)
	rows, err := secretRows(db, ScopeNodeRawConfig, []uint64{owner})
	resolved := resolveDocument(TableNode, "raw_config", owner, *node.RawConfig, rows[secretOwner{owner, "raw_config"}], err)
	if resolved != *node.RawConfig {
		node.RawConfig = &resolved
	}
}

// ResolveWireGuardPeers gives the peers' private and preshared keys their
// values under the dual-read rule, in place: a user's subscription and the
// node's runtime user list.
func ResolveWireGuardPeers(db *gorm.DB, peers ...*model.WireGuardPeer) {
	if len(peers) == 0 || !readsNew(db, TableWireGuardPeer) {
		return
	}
	ids := make([]uint64, 0, len(peers))
	for _, peer := range peers {
		if peer != nil {
			ids = append(ids, uint64(peer.ID))
		}
	}
	rows, err := secretRows(db, ScopeWireGuardPeer, ids)
	for _, peer := range peers {
		if peer == nil {
			continue
		}
		owner := uint64(peer.ID)
		peer.PrivateKey = peerKey(owner, "private_key", peer.PrivateKey, rows[secretOwner{owner, "private_key"}], err)
		peer.PresharedKey = peerKey(owner, "preshared_key", peer.PresharedKey, rows[secretOwner{owner, "preshared_key"}], err)
	}
}

func peerKey(owner uint64, column, legacy string, rows map[string]string, err error) string {
	value, found := rows[""]
	switch {
	case err == nil && found && value != "" && (value == legacy || Unusable(legacy)):
		return value
	case legacy == "":
		return legacy
	}
	reason := FallbackMissing
	switch {
	case err != nil:
		reason = FallbackError
	case found:
		reason = FallbackMismatch
	}
	recordFallback(TableWireGuardPeer, column, reason, subjectName(owner))
	return legacy
}

type secretOwner struct {
	owner  uint64
	column string
}

// secretRows loads the new-table secrets of the owners of one scope, by
// owner and column, then pointer.
func secretRows(db *gorm.DB, scope string, owners []uint64) (map[secretOwner]map[string]string, error) {
	out := make(map[secretOwner]map[string]string)
	for start := 0; start < len(owners); start += syncBatch {
		end := min(start+syncBatch, len(owners))
		var rows []model.ProtocolSecret
		if err := newSession(db).Select("owner_id", "column_name", "json_pointer", "value").
			Where("scope = ? AND owner_id IN ?", scope, owners[start:end]).Find(&rows).Error; err != nil {
			return out, err
		}
		for _, row := range rows {
			key := secretOwner{row.OwnerID, row.ColumnName}
			if out[key] == nil {
				out[key] = make(map[string]string)
			}
			out[key][row.JSONPointer] = row.Value
		}
	}
	return out, nil
}

// resolveDocument applies the dual-read rule to each secret position of one
// legacy JSON column: the new table's value where the legacy document holds
// the placeholder, the legacy value (counted) where the new row is missing
// or differs, and the document unchanged where every position agrees.
func resolveDocument(table, column string, owner uint64, document string, rows map[string]string, readErr error) string {
	positions := SecretPositions(document)
	if len(positions) == 0 {
		return document
	}
	replacements := map[string]string{}
	for _, position := range positions {
		value, found := rows[position.Pointer]
		if readErr == nil && found && value == position.Value {
			continue
		}
		if readErr == nil && found && position.Moved {
			replacements[position.Pointer] = value
			continue
		}
		reason := FallbackMissing
		switch {
		case readErr != nil:
			reason = FallbackError
		case found:
			reason = FallbackMismatch
		}
		recordFallback(table, column, reason, subjectName(owner)+" "+position.Pointer)
	}
	if len(replacements) == 0 {
		return document
	}
	resolved, err := replacePositions(document, replacements)
	if err != nil {
		recordFallback(table, column, FallbackError, subjectName(owner))
		return document
	}
	return resolved
}

// ReplacePositions returns document with each pointer set to its
// JSON-encoded value (an empty pointer stands for the whole text). A
// document that is not JSON, a value that is not, or a pointer whose parent
// is not in the document is an error.
func ReplacePositions(document string, replacements map[string]string) (string, error) {
	if len(replacements) == 0 {
		return document, nil
	}
	return replacePositions(document, replacements)
}

// replacePositions sets each pointer of document to its JSON-encoded value.
// An empty pointer stands for the whole column, whose value is the text
// itself.
func replacePositions(document string, replacements map[string]string) (string, error) {
	if value, whole := replacements[""]; whole {
		return value, nil
	}
	root, ok := decodeJSON(document)
	if !ok {
		return "", errors.New("nodesecrets: the document is not JSON")
	}
	for pointer, encoded := range replacements {
		value, ok := decodeJSON(encoded)
		if !ok {
			return "", errors.New("nodesecrets: a stored secret is not JSON")
		}
		if !setPointer(root, pointer, value) {
			return "", errors.New("nodesecrets: a secret position is not in the document")
		}
	}
	encoded := encodeJSON(root)
	if encoded == "" {
		return "", errors.New("nodesecrets: the document cannot be encoded")
	}
	return encoded, nil
}

// setPointer sets the value at an RFC 6901 pointer whose parent exists.
func setPointer(root any, pointer string, value any) bool {
	tokens := strings.Split(pointer, "/")[1:]
	parent := root
	for i, token := range tokens {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		last := i == len(tokens)-1
		switch typed := parent.(type) {
		case map[string]any:
			if last {
				typed[token] = value
				return true
			}
			parent = typed[token]
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return false
			}
			if last {
				typed[index] = value
				return true
			}
			parent = typed[index]
		default:
			return false
		}
	}
	return false
}
