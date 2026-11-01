package nodesecrets

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The readers below have no caller in Control any more: the node API key
// middleware, the registration and the default registration key that
// used them were removed. The tests keep exercising the split through them.

// legacyNodeKeyMatches is the legacy check of a node's API key: its stored
// hash, or, for an old row without a hash, the key itself.
func legacyNodeKeyMatches(node *model.Node, apiKey string) bool {
	if hashEqual(node.APIKeyHash, hashSecret(apiKey)) {
		return true
	}
	return node.APIKeyHash == "" && secretEqual(node.APIKey, apiKey)
}

// NodeAPIKeyMatches reports whether apiKey is the API key of node, a row of
// v2_node the caller loaded by id: the node API key authentication. An unusable key never matches.
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
	if readsOnlyNew(db, TableNode) || !legacyNodeKeyMatches(node, apiKey) {
		return false
	}
	recordFallback(TableNode, KindNodeAPIKey, fallbackReason(err, row), subjectName(uint64(node.ID)))
	return true
}

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
		if readsOnlyNew(tx, TableAuthorizedKey) {
			return nil, notFound(readErr)
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

// RegistrationKeyExists reports whether key is a registration key, active
// or not, expired or not: the default key Control creates from its
// environment exists once. An unusable key exists nowhere.
func RegistrationKeyExists(db *gorm.DB, key string) (bool, error) {
	if Unusable(key) {
		return false, nil
	}
	keyHash := hashSecret(key)
	if readsNew(db, TableAuthorizedKey) {
		_, found, err := credentialSubject(db, SubjectRegistrationKey, KindRegistrationKey, keyHash, StatusActive)
		if err != nil || found || readsOnlyNew(db, TableAuthorizedKey) {
			return found, err
		}
	}
	var count int64
	if err := newSession(db).Model(&model.AuthorizedKey{}).Where("key_hash = ?", keyHash).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
