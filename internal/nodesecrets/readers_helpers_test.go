package nodesecrets

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The readers below have no caller in Control any more: the node API key
// middleware and the registration that used them were removed. The tests keep exercising the split through them.

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
