package handler

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// An empty token never authenticates an agent, even for a node whose key or
// token is empty.
func TestVerifyForwardNodeTokenRefusesEmptyTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}, &model.Node{}))
	keyless := model.Node{ID: 1, Name: "keyless", Host: "a.example.test"}
	keyed := model.Node{ID: 2, Name: "keyed", Host: "b.example.test", APIKey: "stored-elsewhere", APIKeyHash: hashString("node-key")}
	legacy := model.Node{ID: 3, Name: "legacy", Host: "c.example.test", APIKey: "legacy-key"}
	// What the credential split leaves in the legacy columns of a finalized
	// table: a tombstone, or the placeholder.
	tombstoned := model.Node{ID: 4, Name: "tombstoned", Host: "d.example.test", APIKey: "!moved:4"}
	masked := model.Node{ID: 5, Name: "masked", Host: "e.example.test", APIKey: "********"}
	require.NoError(t, db.Create(&[]model.Node{keyless, keyed, legacy, tombstoned, masked}).Error)
	require.NoError(t, db.Create(&[]model.ForwardNode{{ID: 10, Name: "tokenless"}, {ID: 11, Name: "tokened", APIToken: "fwd-token"},
		{ID: 12, Name: "masked", APIToken: "********"}}).Error)
	h := &AgentHandler{db: db}

	for _, c := range []struct {
		name  string
		id    uint
		token string
		ok    bool
	}{
		{"node without a key, empty token", 1, "", false},
		{"node with a key, empty token", 2, "", false},
		{"node with a key, its token", 2, "node-key", true},
		{"node with a key, another token", 2, "other", false},
		{"node with a plain legacy key, its token", 3, "legacy-key", true},
		{"node with a plain legacy key, empty token", 3, "", false},
		{"forward node without a token, empty token", 10, "", false},
		{"forward node with a token, empty token", 11, "", false},
		{"forward node with a token, its token", 11, "fwd-token", true},
		{"node with a tombstone, the tombstone", 4, "!moved:4", false},
		{"node with a tombstone, the placeholder", 4, "********", false},
		{"node with the placeholder, the placeholder", 5, "********", false},
		{"node with a key, a tombstone", 2, "!moved:2", false},
		{"forward node with the placeholder, the placeholder", 12, "********", false},
		{"forward node with a token, a tombstone", 11, "!moved:11", false},
	} {
		_, err := h.verifyForwardNodeToken(c.id, c.token)
		if c.ok {
			require.NoError(t, err, c.name)
		} else {
			require.ErrorIs(t, err, errAgentInvalidToken, c.name)
		}
	}
}
