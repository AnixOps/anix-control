package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ss2022KeysDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.UserSS2022Key{}, &model.SubscriberChange{}, &model.AgentTransport{}))
	return db
}

// A user's key is made once, at the first call, and the change log hears
// of it so users.v1 sends it to the nodes.
func TestEnsureSS2022UserKey(t *testing.T) {
	db := ss2022KeysDB(t)
	key, err := EnsureSS2022UserKey(db, 7)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(key)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	again, err := EnsureSS2022UserKey(db, 7)
	require.NoError(t, err)
	require.Equal(t, key, again)
	require.EqualValues(t, 1, countRows(t, db, &model.SubscriberChange{}, "user_id = ?", 7))

	keys, err := SS2022UserKeys(db, []uint{7, 8})
	require.NoError(t, err)
	require.Equal(t, map[uint]string{7: key}, keys)
}

// Only proxy nodes whose Agent said it serves stored keys, over the mTLS
// stream, get them in subscriptions.
func TestAgentUserKeyNodes(t *testing.T) {
	db := ss2022KeysDB(t)
	row := func(id uint, transport string, revision uint32) model.AgentTransport {
		return model.AgentTransport{NodeKind: agentcontrol.NodeKindProxy, NodeID: id, Transport: transport, ProtocolRevision: revision}
	}
	require.NoError(t, db.Create([]model.AgentTransport{
		row(1, model.AgentTransportMTLSStream, agentcontrol.ProtocolRevisionUserKeys),
		row(2, model.AgentTransportMTLSStream, agentcontrol.ProtocolRevisionBaseline),
		row(3, "legacy_websocket", agentcontrol.ProtocolRevisionUserKeys),
	}).Error)
	nodes, err := agentUserKeyNodes(db, []uint{1, 2, 3, 4})
	require.NoError(t, err)
	require.Equal(t, map[uint]bool{1: true}, nodes)
}

// A new root Shadowsocks 2022 protocol gets a random server key of its
// cipher's length; a child node, another cipher or a given key is left be.
func TestEnsureSS2022ServerKey(t *testing.T) {
	serverKey := func(settings string, node *model.Node) string {
		protocol := &model.NodeProtocol{Type: model.ProtocolShadowsocks, Settings: &settings}
		require.NoError(t, ensureSS2022ServerKey(protocol, node))
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(*protocol.Settings), &decoded))
		key, _ := decoded["server_key"].(string)
		return key
	}
	root, parent := &model.Node{}, uint(1)
	for cipher, size := range map[string]int{"2022-blake3-aes-128-gcm": 16, "2022-blake3-aes-256-gcm": 32} {
		raw, err := base64.StdEncoding.DecodeString(serverKey(`{"cipher":"`+cipher+`"}`, root))
		require.NoError(t, err)
		require.Len(t, raw, size)
	}
	require.NotEmpty(t, serverKey(`{"method":"2022-blake3-chacha20-poly1305"}`, root))
	require.Empty(t, serverKey(`{"cipher":"2022-blake3-aes-256-gcm"}`, &model.Node{ParentID: &parent}))
	require.Empty(t, serverKey(`{"cipher":"aes-256-gcm"}`, root))
	require.Equal(t, "given", serverKey(`{"cipher":"2022-blake3-aes-256-gcm","server_key":"given"}`, root))
}
