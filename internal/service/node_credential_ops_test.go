package service

import (
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func credentialOpsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.WireGuardPeer{}, &model.ForwardNode{},
		&model.AgentCertificate{}, &model.AgentEnrollment{}))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	return db
}

func countRows(t *testing.T, db *gorm.DB, value any, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(value).Where(query, args...).Count(&n).Error)
	return n
}

// The retirement cascade runs in its caller's transaction: when the caller
// fails after it, every step is undone.
func TestRetireProxyNodeTxRollsBackWithItsCaller(t *testing.T) {
	db := credentialOpsDB(t)
	raw := `{"tls_settings":{"private_key":"raw-private"}}`
	reality := `{"private_key":"reality-private"}`
	require.NoError(t, db.Create(&model.Node{ID: 1, Name: "edge", APIKey: "key-1", APIKeyHash: hashString("key-1"), Secret: "secret-1", RawConfig: &raw}).Error)
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 5, NodeID: 1, Name: "reality", Type: model.ProtocolVLESS, Port: 443, RealitySettings: &reality}).Error)
	group := &model.SubscriptionGroup{Name: "group", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, 5).Error)
	require.NoError(t, db.Create(&model.WireGuardPeer{NodeProtocolID: 5, UserID: 1, PeerIP: "10.9.0.2", PrivateKey: "peer-private", PublicKey: "pub"}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := nodesecrets.Sync(tx, nodesecrets.TableNode, 1); err != nil {
			return err
		}
		if err := nodesecrets.Sync(tx, nodesecrets.TableNodeProtocol, 5); err != nil {
			return err
		}
		return nodesecrets.Sync(tx, nodesecrets.TableWireGuardPeer, 1)
	}))
	before := func() []int64 {
		var links int64
		require.NoError(t, db.Table("v2_subscription_group_node_protocols").Count(&links).Error)
		return []int64{
			countRows(t, db, &model.NodeProtocol{}, "node_id = ?", 1), countRows(t, db, &model.WireGuardPeer{}, "1 = 1"), links,
			countRows(t, db, &model.ProtocolSecret{}, "1 = 1"), countRows(t, db, &model.NodeCredential{}, "1 = 1"),
		}
	}
	require.Equal(t, []int64{1, 1, 1, 3, 2}, before())

	boom := errors.New("the caller fails after the cascade")
	err := db.Transaction(func(tx *gorm.DB) error {
		counts, err := RetireProxyNodeTx(tx, 1)
		require.NoError(t, err)
		require.Equal(t, RetireCounts{CredentialsRevoked: 2, SecretsDeleted: 2, WireGuardPeersDeleted: 1, ProtocolsDeleted: 1, SubscriptionLinksDeleted: 1}, counts)
		return boom
	})
	require.ErrorIs(t, err, boom)
	require.Equal(t, []int64{1, 1, 1, 3, 2}, before(), "everything is back")

	require.NoError(t, NewNodeServiceWithDB(db).DeleteNode(1))
	require.Equal(t, []int64{0, 0, 0, 0, 0}, before())
	require.Zero(t, countRows(t, db, &model.Node{}, "id = ?", 1))
}

// NewNodeServiceWithDB is a NodeService on db, for tests.
func NewNodeServiceWithDB(db *gorm.DB) *NodeService { return &NodeService{db: db} }

// A secret document keeps the stored values of its placeholders, clears
// what it leaves out, and answers the redacted document the routes answer.
func TestSecretDocumentOutcome(t *testing.T) {
	stored := `{"dest":"a","private_key":"old-private","public_key":"pub","nested":{"password":"old-password"}}`
	outcome := secretDocumentOutcome(`{"dest":"b","private_key":"********","public_key":"pub-2","nested":{"password":""}}`, stored)
	require.Equal(t, 1, outcome.Kept)
	require.Equal(t, 1, outcome.Cleared)
	require.Contains(t, outcome.Full, `"private_key":"old-private"`)
	require.Contains(t, outcome.Full, `"dest":"b"`)
	require.NotContains(t, outcome.Full, "old-password")
	require.Equal(t, RedactNodeSecretsJSON(outcome.Full), outcome.Redacted)
	require.NotContains(t, outcome.Redacted, "old-private")

	unchanged := secretDocumentOutcome(`{"dest":"c","private_key":"new-private"}`, stored)
	require.Equal(t, `{"dest":"c","private_key":"new-private"}`, unchanged.Full, "a document without placeholders is stored as sent")
	require.Zero(t, unchanged.Kept)
	require.Equal(t, 1, unchanged.Cleared, "the nested password is gone; the replaced key is not cleared")
}

// The raw configuration validation refuses what the route refuses.
func TestValidateRawNodeConfig(t *testing.T) {
	validated, err := ValidateRawNodeConfig(map[string]any{"type": "vless"})
	require.NoError(t, err)
	require.Equal(t, []string{"缺少 server_port 字段"}, validated.Warnings)
	require.JSONEq(t, `{"type":"vless"}`, string(validated.Normalized))
	_, err = ValidateRawNodeConfig("[1]")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidNodeProtocol, "not an object, as the route answers")
	_, err = ValidateRawNodeConfig("null")
	require.ErrorIs(t, err, ErrRawNodeConfigNotObject)
	_, err = ValidateRawNodeConfig(`{"type":"wireguard","cidr":"10.9.0.0/24","server_address":"10.9.0.1/24","server_private_key":"not-a-key"}`)
	require.ErrorIs(t, err, ErrInvalidNodeProtocol)
}
