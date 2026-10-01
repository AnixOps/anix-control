package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// The credential, secret and retirement functions the KernelNodeOps
// executors and the legacy routes share (node-ops-service.md section 3.3,
// NO-5). Each takes the transaction of its caller: the legacy service
// methods run them in the transaction they ran before, with every
// nodesecrets.Sync and every agent certificate revocation (#107, A2-1) in
// it, and the executors run them in the operation's transaction. Native
// and legacy then write identical rows.
//
// No function here logs, answers or returns a secret in an error.

// ProxyNodeCredentials are a proxy node's API key and shared secret, as
// generated or typed; an empty one is not issued.
type ProxyNodeCredentials struct {
	APIKey string
	Secret string
}

// GenerateProxyNodeCredentials generates the credentials of a proxy node:
// 32 random bytes, hex encoded, each.
func GenerateProxyNodeCredentials(apiKey, secret bool) (ProxyNodeCredentials, error) {
	var credentials ProxyNodeCredentials
	var err error
	if apiKey {
		if credentials.APIKey, err = generateSecureToken(32); err != nil {
			return ProxyNodeCredentials{}, err
		}
	}
	if secret {
		if credentials.Secret, err = generateSecureToken(32); err != nil {
			return ProxyNodeCredentials{}, err
		}
	}
	return credentials, nil
}

// Apply sets the credentials on a node row about to be written: the key,
// its hash and the secret.
func (c ProxyNodeCredentials) Apply(node *model.Node) {
	if c.APIKey != "" {
		node.APIKey = c.APIKey
		node.APIKeyHash = hashString(c.APIKey)
	}
	if c.Secret != "" {
		node.Secret = c.Secret
	}
}

// IssueProxyNodeCredentialsTx writes a proxy node's credentials to its row
// and the split tables. With replaced, the node's agent certificates and
// enrollments are revoked (credentials_replaced): an agent enrolled with the
// old key must enroll again.
func IssueProxyNodeCredentialsTx(tx *gorm.DB, nodeID uint, credentials ProxyNodeCredentials, replaced bool) error {
	updates := map[string]any{}
	if credentials.APIKey != "" {
		updates["api_key"] = credentials.APIKey
		updates["api_key_hash"] = hashString(credentials.APIKey)
	}
	if credentials.Secret != "" {
		updates["secret"] = credentials.Secret
	}
	if len(updates) == 0 {
		return errors.New("no credential to issue")
	}
	result := tx.Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	if err := nodesecrets.Sync(tx, nodesecrets.TableNode, nodeID); err != nil {
		return err
	}
	if !replaced {
		return nil
	}
	return revokeNodeAgents(tx, agentcontrol.NodeKindProxy, nodeID, agentpki.RevokeReasonCredentialsReplaced)
}

// ProxyNodeHasCredentials reports whether a proxy node holds an API key and
// a shared secret, read through the credential split.
func ProxyNodeHasCredentials(db *gorm.DB, nodeID uint) (apiKey, secret bool, err error) {
	var node model.Node
	if err := db.Select("id", "api_key", "api_key_hash", "secret").Where("id = ?", nodeID).First(&node).Error; err != nil {
		return false, false, err
	}
	return !nodesecrets.Unusable(nodesecrets.NodeAPIKey(db, &node)), !nodesecrets.Unusable(nodesecrets.NodeSharedSecret(db, &node)), nil
}

// GenerateForwardNodeToken generates a forward node's API token, as the
// forward node creation does: 16 random bytes, hex encoded.
func GenerateForwardNodeToken() (string, error) {
	return NewForwardNodeService(nil).GenerateAPIToken()
}

// ForwardNodeHasToken reports whether a forward node holds a token, read
// through the credential split.
func ForwardNodeHasToken(db *gorm.DB, nodeID uint) (bool, error) {
	var node model.ForwardNode
	if err := db.Select("id", "api_token").Where("id = ?", nodeID).First(&node).Error; err != nil {
		return false, err
	}
	return !nodesecrets.Unusable(nodesecrets.ForwardNodeToken(db, &node)), nil
}

// IssueForwardNodeTokenTx writes a forward node's token to its row and the
// split tables, which pin it to the node's management endpoint
// (nodesecrets). A token that replaces another revokes the node's agent
// certificates (credentials_replaced), as the forward node update does. It
// reports whether a token was replaced.
func IssueForwardNodeTokenTx(tx *gorm.DB, nodeID uint, token string) (bool, error) {
	if token == "" {
		return false, errors.New("a token is required")
	}
	var stored model.ForwardNode
	if err := tx.Select("id", "api_token").Where("id = ?", nodeID).First(&stored).Error; err != nil {
		return false, err
	}
	current := nodesecrets.ForwardNodeToken(tx, &stored)
	replaced := !nodesecrets.Unusable(current) && current != token
	if err := tx.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Update("api_token", token).Error; err != nil {
		return false, err
	}
	if err := nodesecrets.Sync(tx, nodesecrets.TableForwardNode, nodeID); err != nil {
		return false, err
	}
	if !replaced {
		return false, nil
	}
	return true, revokeNodeAgents(tx, agentcontrol.NodeKindForward, nodeID, agentpki.RevokeReasonCredentialsReplaced)
}

// IssueRegistrationKeyTx creates a node registration key and returns its
// row and the key, which is shown once. expireAt ends the key; nil never.
func IssueRegistrationKeyTx(tx *gorm.DB, name string, expireAt *int64) (*model.AuthorizedKey, string, error) {
	key, err := generateSecureToken(32)
	if err != nil {
		return nil, "", err
	}
	authKey := &model.AuthorizedKey{Name: name, Key: key, KeyHash: hashString(key), Used: 0, ExpireAt: expireAt}
	if err := tx.Create(authKey).Error; err != nil {
		return nil, "", err
	}
	if err := nodesecrets.Sync(tx, nodesecrets.TableAuthorizedKey, authKey.ID); err != nil {
		return nil, "", err
	}
	return authKey, key, nil
}

// RevokeRegistrationKeyTx deletes a registration key with its split rows.
// It reports whether a key was deleted; a key already gone is no error.
func RevokeRegistrationKeyTx(tx *gorm.DB, id uint) (bool, error) {
	result := tx.Delete(&model.AuthorizedKey{}, id)
	if result.Error != nil {
		return false, result.Error
	}
	if err := nodesecrets.Sync(tx, nodesecrets.TableAuthorizedKey, id); err != nil {
		return false, err
	}
	return result.RowsAffected > 0, nil
}

// RetireCounts is what a retirement removed.
type RetireCounts struct {
	CredentialsRevoked       int64
	SecretsDeleted           int64
	WireGuardPeersDeleted    int64
	ProtocolsDeleted         int64
	SubscriptionLinksDeleted int64
}

func (c *RetireCounts) add(other RetireCounts) {
	c.CredentialsRevoked += other.CredentialsRevoked
	c.SecretsDeleted += other.SecretsDeleted
	c.WireGuardPeersDeleted += other.WireGuardPeersDeleted
	c.ProtocolsDeleted += other.ProtocolsDeleted
	c.SubscriptionLinksDeleted += other.SubscriptionLinksDeleted
}

// RetireProtocolTx removes what goes with node protocols being deleted:
// their users' WireGuard peers, their subscription group links and their
// secrets in the split table. The protocol rows themselves are the
// caller's: the legacy deletion removes them in the same transaction, a
// package after its RetireProtocol succeeded.
func RetireProtocolTx(tx *gorm.DB, protocolIDs ...uint) (RetireCounts, error) {
	var counts RetireCounts
	if len(protocolIDs) == 0 {
		return counts, nil
	}
	peers, err := deleteWireGuardPeersCounted(tx, "node_protocol_id IN ?", protocolIDs)
	if err != nil {
		return counts, err
	}
	counts.WireGuardPeersDeleted = peers
	links, err := deleteProtocolGroupLinksCounted(tx, protocolIDs...)
	if err != nil {
		return counts, err
	}
	counts.SubscriptionLinksDeleted = links
	retired, err := nodesecrets.Retire(tx, nodesecrets.TableNodeProtocol, protocolIDs...)
	if err != nil {
		return counts, err
	}
	counts.SecretsDeleted = retired.Secrets
	return counts, nil
}

// RetireProxyNodeTx removes what the kernel holds for a proxy node being
// deleted: its protocols, with their peers, links and secrets
// (RetireProtocolTx), its credentials and raw configuration secrets in the
// split tables, and its agent certificates and enrollments (node_deleted).
// The node row is the caller's.
func RetireProxyNodeTx(tx *gorm.DB, nodeID uint) (RetireCounts, error) {
	var counts RetireCounts
	var protocolIDs []uint
	if err := tx.Model(&model.NodeProtocol{}).Where("node_id = ?", nodeID).Pluck("id", &protocolIDs).Error; err != nil {
		return counts, err
	}
	if len(protocolIDs) > 0 {
		protocols, err := RetireProtocolTx(tx, protocolIDs...)
		counts.add(protocols)
		if err != nil {
			return counts, err
		}
		result := tx.Where("node_id = ?", nodeID).Delete(&model.NodeProtocol{})
		if result.Error != nil {
			return counts, result.Error
		}
		counts.ProtocolsDeleted = result.RowsAffected
		if err := nodesecrets.Sync(tx, nodesecrets.TableNodeProtocol, protocolIDs...); err != nil {
			return counts, err
		}
	}
	retired, err := nodesecrets.Retire(tx, nodesecrets.TableNode, nodeID)
	if err != nil {
		return counts, err
	}
	counts.CredentialsRevoked += retired.Credentials
	counts.SecretsDeleted += retired.Secrets
	return counts, revokeNodeAgents(tx, agentcontrol.NodeKindProxy, nodeID, agentpki.RevokeReasonNodeDeleted)
}

// RetireForwardNodeTx removes what the kernel holds for a forward node
// being deleted: its token in the split table and its agent certificates
// and enrollments (node_deleted). The node row is the caller's.
func RetireForwardNodeTx(tx *gorm.DB, nodeID uint) (RetireCounts, error) {
	var counts RetireCounts
	retired, err := nodesecrets.Retire(tx, nodesecrets.TableForwardNode, nodeID)
	if err != nil {
		return counts, err
	}
	counts.CredentialsRevoked = retired.Credentials
	return counts, revokeNodeAgents(tx, agentcontrol.NodeKindForward, nodeID, agentpki.RevokeReasonNodeDeleted)
}

// deleteWireGuardPeersCounted is deleteWireGuardPeers reporting how many
// peers it removed.
func deleteWireGuardPeersCounted(tx *gorm.DB, query string, args ...any) (int64, error) {
	if !tx.Migrator().HasTable(&model.WireGuardPeer{}) {
		return 0, nil
	}
	var count int64
	if err := tx.Model(&model.WireGuardPeer{}).Where(query, args...).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, deleteWireGuardPeers(tx, query, args...)
}

// deleteProtocolGroupLinksCounted is deleteProtocolGroupLinks reporting how
// many links it removed.
func deleteProtocolGroupLinksCounted(tx *gorm.DB, protocolIDs ...uint) (int64, error) {
	if !tx.Migrator().HasTable("v2_subscription_group_node_protocols") {
		return 0, nil
	}
	result := tx.Exec("DELETE FROM v2_subscription_group_node_protocols WHERE node_protocol_id IN ?", protocolIDs)
	return result.RowsAffected, result.Error
}

// SecretDocumentOutcome is what PutNodeProtocolSecretDocumentTx and
// PutNodeRawConfigDocumentTx stored.
type SecretDocumentOutcome struct {
	// Full is the document as stored in the legacy column: every secret
	// in clear.
	Full string
	// Redacted is the document with the placeholder at every secret
	// position: what administrators' answers show, and what a package
	// stores in its own row.
	Redacted string
	// Kept counts the placeholders the incoming document sent, which keep
	// their stored values; Cleared the stored secrets the incoming
	// document no longer holds.
	Kept    int
	Cleared int
}

// nodeProtocolSecretColumnValue returns a protocol's secret column.
func nodeProtocolSecretColumnValue(protocol *model.NodeProtocol, column string) *string {
	switch column {
	case "settings":
		return protocol.Settings
	case "tls_settings":
		return protocol.TLSSettings
	case "transport_settings":
		return protocol.TransportSettings
	case "reality_settings":
		return protocol.RealitySettings
	case "custom_config":
		return protocol.CustomConfig
	}
	return nil
}

// isNodeProtocolSecretColumn reports whether column is one of the five
// protocol columns that hold secrets.
func isNodeProtocolSecretColumn(column string) bool {
	for _, known := range nodeProtocolSecretColumns {
		if known == column {
			return true
		}
	}
	return false
}

// PutNodeProtocolSecretDocumentTx stores one secret column of a node
// protocol: the incoming document, with the stored value at every position
// it sends as the placeholder (KeepNodeSecretsJSON), is written to the
// column and the split table (putNodeProtocolColumnsTx), as the protocol
// update does for the columns it carries.
func PutNodeProtocolSecretDocumentTx(tx *gorm.DB, protocolID uint, column, document string) (SecretDocumentOutcome, error) {
	if !isNodeProtocolSecretColumn(column) {
		return SecretDocumentOutcome{}, fmt.Errorf("%s is not a protocol secret column", column)
	}
	var current model.NodeProtocol
	if err := tx.Where("id = ?", protocolID).First(&current).Error; err != nil {
		return SecretDocumentOutcome{}, err
	}
	nodesecrets.ResolveProtocol(tx, &current)
	stored := ""
	if value := nodeProtocolSecretColumnValue(&current, column); value != nil {
		stored = *value
	}
	outcome := secretDocumentOutcome(document, stored)
	if err := putNodeProtocolColumnsTx(tx, protocolID, map[string]any{column: outcome.Full}); err != nil {
		return SecretDocumentOutcome{}, err
	}
	return outcome, nil
}

// PutNodeRawConfigDocumentTx stores a proxy node's raw configuration, as
// PutNodeProtocolSecretDocumentTx stores a protocol column.
func PutNodeRawConfigDocumentTx(tx *gorm.DB, nodeID uint, document string) (SecretDocumentOutcome, error) {
	var current model.Node
	if err := tx.Select("id", "raw_config").Where("id = ?", nodeID).First(&current).Error; err != nil {
		return SecretDocumentOutcome{}, err
	}
	nodesecrets.ResolveNodeRawConfig(tx, &current)
	stored := ""
	if current.RawConfig != nil {
		stored = *current.RawConfig
	}
	outcome := secretDocumentOutcome(document, stored)
	full := outcome.Full
	if err := updateNodeColumnsTx(tx, nodeID, map[string]any{"raw_config": &full}); err != nil {
		return SecretDocumentOutcome{}, err
	}
	return outcome, nil
}

// secretDocumentOutcome gives an incoming document the stored values of its
// placeholders and counts what it keeps and clears.
func secretDocumentOutcome(document, stored string) SecretDocumentOutcome {
	outcome := SecretDocumentOutcome{Full: KeepNodeSecretsJSON(document, stored)}
	outcome.Redacted = RedactNodeSecretsJSON(outcome.Full)
	kept := map[string]bool{}
	for _, position := range nodesecrets.SecretPositions(document) {
		if position.Moved {
			outcome.Kept++
			kept[position.Pointer] = true
		}
	}
	remaining := map[string]bool{}
	for _, position := range nodesecrets.SecretPositions(outcome.Full) {
		remaining[position.Pointer] = true
	}
	for _, position := range nodesecrets.SecretPositions(stored) {
		if !remaining[position.Pointer] {
			outcome.Cleared++
		}
	}
	return outcome
}

// putNodeProtocolColumnsTx writes columns of a node protocol and the
// protocol's split rows, in the caller's transaction.
func putNodeProtocolColumnsTx(tx *gorm.DB, protocolID uint, updates map[string]any) error {
	if len(updates) > 0 {
		if err := tx.Model(&model.NodeProtocol{}).Where("id = ?", protocolID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nodesecrets.Sync(tx, nodesecrets.TableNodeProtocol, protocolID)
}

// updateNodeColumnsTx writes columns of a proxy node and the node's split
// rows, in the caller's transaction.
func updateNodeColumnsTx(tx *gorm.DB, nodeID uint, updates map[string]any) error {
	if len(updates) > 0 {
		if err := tx.Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nodesecrets.Sync(tx, nodesecrets.TableNode, nodeID)
}

// ErrRawNodeConfigNotObject refuses a raw configuration that is not a JSON
// object.
var ErrRawNodeConfigNotObject = errors.New("raw config must be an object")

// RawNodeConfigValidation is a validated raw configuration.
type RawNodeConfigValidation struct {
	Config map[string]any
	// Normalized is the configuration re-encoded, as the routes store it.
	Normalized []byte
	// Warnings are what the validation route answers besides validity.
	Warnings []string
}

// DecodeRawNodeConfig decodes a raw configuration an administrator sent
// inline or as a JSON string into an object; anything else is
// ErrRawNodeConfigNotObject.
func DecodeRawNodeConfig(raw any) (map[string]any, []byte, error) {
	var (
		jsonBytes []byte
		err       error
	)
	if rawString, ok := raw.(string); ok {
		jsonBytes = []byte(rawString)
	} else {
		jsonBytes, err = json.Marshal(raw)
		if err != nil {
			return nil, nil, err
		}
	}
	var config map[string]any
	if err := json.Unmarshal(jsonBytes, &config); err != nil || config == nil {
		if err == nil {
			err = ErrRawNodeConfigNotObject
		}
		return nil, nil, err
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return nil, nil, err
	}
	return config, normalized, nil
}

// ValidateRawNodeConfig runs the raw configuration validators of the node
// routes: the configuration must be a JSON object (ErrRawNodeConfigNotObject
// or the decoding error), and a WireGuard configuration must be valid
// (ErrInvalidNodeProtocol). A configuration without server_port is valid
// with a warning.
func ValidateRawNodeConfig(raw any) (*RawNodeConfigValidation, error) {
	config, normalized, err := DecodeRawNodeConfig(raw)
	if err != nil {
		return nil, err
	}
	if err := ValidateWireGuardRuntimeConfig(config); err != nil {
		return nil, err
	}
	warnings := []string{}
	if _, ok := config["server_port"]; !ok {
		warnings = append(warnings, "缺少 server_port 字段")
	}
	return &RawNodeConfigValidation{Config: config, Normalized: normalized, Warnings: warnings}, nil
}

// registrationKeyExpiry is the expiry of a key issued for expireDays days;
// nil for never.
func registrationKeyExpiry(expireDays int, now time.Time) *int64 {
	if expireDays <= 0 {
		return nil
	}
	expireAt := now.Add(time.Duration(expireDays) * 24 * time.Hour).Unix()
	return &expireAt
}
