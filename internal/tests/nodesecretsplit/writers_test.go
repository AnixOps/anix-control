package nodesecretsplit

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func ptr[T any](value T) *T { return &value }

// requireVerified requires both forms of every split table to match.
func requireVerified(t *testing.T, db *gorm.DB) {
	t.Helper()
	results, err := nodesecrets.Verify(context.Background(), db, nodesecrets.VerifyOptions{})
	require.NoError(t, err, "%+v", results)
	for _, result := range results {
		require.True(t, result.Match, "%+v", result)
	}
}

// current is the current version of a subject's credential.
func current(t *testing.T, db *gorm.DB, subjectKind string, subjectID uint, kind string) model.NodeCredential {
	t.Helper()
	var rows []model.NodeCredential
	require.NoError(t, db.Where("subject_kind = ? AND subject_id = ? AND kind = ? AND status <> ?",
		subjectKind, subjectID, kind, nodesecrets.StatusRetired).Find(&rows).Error)
	require.Len(t, rows, 1, "%s %d %s", subjectKind, subjectID, kind)
	return rows[0]
}

// credentials counts every row of a subject, retired versions included.
func credentials(t *testing.T, db *gorm.DB, subjectKind string, subjectID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", subjectKind, subjectID).Count(&count).Error)
	return count
}

// positions answers an owner's protocol secrets by column and pointer.
func positions(t *testing.T, db *gorm.DB, scope string, ownerID uint) map[string]string {
	t.Helper()
	var rows []model.ProtocolSecret
	require.NoError(t, db.Where("scope = ? AND owner_id = ?", scope, ownerID).Find(&rows).Error)
	out := map[string]string{}
	for _, row := range rows {
		out[row.ColumnName+" "+row.JSONPointer] = row.Value
	}
	return out
}

func secretVersion(t *testing.T, db *gorm.DB, scope string, ownerID uint, column, pointer string) int {
	t.Helper()
	var row model.ProtocolSecret
	require.NoError(t, db.Where("scope = ? AND owner_id = ? AND column_name = ? AND json_pointer = ?", scope, ownerID, column, pointer).First(&row).Error)
	return row.Version
}

// TestEveryWriterDualWrites drives each kernel writer of a moved column and
// requires the new tables to follow it, in the same transaction.
func TestEveryWriterDualWrites(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		nodes := service.NewNodeService()

		// Node creation: the generated key and secret, and the default
		// protocol (which holds no secret).
		node := &model.Node{Name: "split-node", Host: "203.0.113.10", Port: 443}
		require.NoError(t, nodes.CreateNode(node))
		key := current(t, db, nodesecrets.SubjectProxy, node.ID, nodesecrets.KindNodeAPIKey)
		require.Equal(t, node.APIKey, key.Value)
		require.Equal(t, node.APIKeyHash, key.KeyHash)
		require.Equal(t, nodesecrets.SourceDualWrite, key.Source)
		require.Equal(t, node.Secret, current(t, db, nodesecrets.SubjectProxy, node.ID, nodesecrets.KindNodeSharedSecret).Value)
		requireVerified(t, db)

		// Node update: the raw configuration's secret; the placeholder
		// keeps it; a refused credential column changes nothing.
		require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"raw_config": `{"server_port":51820,"wireguard":{"private_key":"fake-raw-private"}}`}))
		require.Equal(t, map[string]string{"raw_config /wireguard/private_key": `"fake-raw-private"`}, positions(t, db, nodesecrets.ScopeNodeRawConfig, node.ID))
		require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"raw_config": `{"server_port":51821,"wireguard":{"private_key":"********"}}`, "name": "renamed", "api_key": "fake-refused"}))
		require.Equal(t, 1, secretVersion(t, db, nodesecrets.ScopeNodeRawConfig, node.ID, "raw_config", "/wireguard/private_key"))
		require.Equal(t, 1, current(t, db, nodesecrets.SubjectProxy, node.ID, nodesecrets.KindNodeAPIKey).Version)
		require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"raw_config": `{"server_port":51821,"wireguard":{"private_key":"fake-raw-private-b"}}`}))
		require.Equal(t, 2, secretVersion(t, db, nodesecrets.ScopeNodeRawConfig, node.ID, "raw_config", "/wireguard/private_key"))
		requireVerified(t, db)

		// Protocol creation and update; the placeholder keeps the secret.
		reality := `{"private_key":"fake-reality-private","short_id":"6ba85179","dest":"www.example.test:443"}`
		protocol := &model.NodeProtocol{NodeID: node.ID, Name: "reality", Type: model.ProtocolVLESS, Port: 8443, TLS: 2,
			Settings: ptr(`{"flow":"xtls-rprx-vision"}`), RealitySettings: &reality, Transport: ptr("tcp")}
		require.NoError(t, nodes.CreateProtocol(protocol))
		require.Equal(t, map[string]string{"reality_settings /private_key": `"fake-reality-private"`}, positions(t, db, nodesecrets.ScopeNodeProtocol, protocol.ID))
		require.NoError(t, nodes.UpdateProtocol(protocol.ID, map[string]any{"reality_settings": map[string]any{"private_key": "fake-reality-private-b", "short_id": "6ba85179"}}))
		require.Equal(t, `"fake-reality-private-b"`, positions(t, db, nodesecrets.ScopeNodeProtocol, protocol.ID)["reality_settings /private_key"])
		require.NoError(t, nodes.UpdateProtocol(protocol.ID, map[string]any{"reality_settings": `{"private_key":"********","short_id":"6ba85179"}`, "custom_config": "fake-custom"}))
		require.Equal(t, map[string]string{"reality_settings /private_key": `"fake-reality-private-b"`, "custom_config ": "fake-custom"}, positions(t, db, nodesecrets.ScopeNodeProtocol, protocol.ID))
		require.Equal(t, 2, secretVersion(t, db, nodesecrets.ScopeNodeProtocol, protocol.ID, "reality_settings", "/private_key"))
		requireVerified(t, db)

		// Registration keys: creation, use (registration), deletion.
		authKey, rawKey, err := nodes.GenerateAuthKey("split-key", 30)
		require.NoError(t, err)
		registration := current(t, db, nodesecrets.SubjectRegistrationKey, authKey.ID, nodesecrets.KindRegistrationKey)
		require.Equal(t, rawKey, registration.Value)
		require.NotNil(t, registration.ExpiresAt)
		registered, err := nodes.RegisterNode(&model.NodeRegisterRequest{AuthKey: rawKey, Name: "registered", Port: 443}, "192.0.2.10")
		require.NoError(t, err)
		require.Equal(t, registered.APIKey, current(t, db, nodesecrets.SubjectProxy, registered.NodeID, nodesecrets.KindNodeAPIKey).Value)
		require.Equal(t, registered.Secret, current(t, db, nodesecrets.SubjectProxy, registered.NodeID, nodesecrets.KindNodeSharedSecret).Value)
		requireVerified(t, db)
		require.NoError(t, nodes.DeleteAuthKey(authKey.ID))
		require.Zero(t, credentials(t, db, nodesecrets.SubjectRegistrationKey, authKey.ID))

		// The registration key from the environment replaces its
		// predecessor.
		t.Setenv("NODE_DEFAULT_AUTH_KEY", "fake-env-key-1")
		service.InitDefaultAuthKeyFromEnv()
		var envKey model.AuthorizedKey
		require.NoError(t, db.Where("name = ?", "Default (from env)").First(&envKey).Error)
		require.Equal(t, "fake-env-key-1", current(t, db, nodesecrets.SubjectRegistrationKey, envKey.ID, nodesecrets.KindRegistrationKey).Value)
		t.Setenv("NODE_DEFAULT_AUTH_KEY", "fake-env-key-2")
		service.InitDefaultAuthKeyFromEnv()
		require.Zero(t, credentials(t, db, nodesecrets.SubjectRegistrationKey, envKey.ID))
		var replacement model.AuthorizedKey
		require.NoError(t, db.Where("name = ?", "Default (from env)").First(&replacement).Error)
		require.Equal(t, "fake-env-key-2", current(t, db, nodesecrets.SubjectRegistrationKey, replacement.ID, nodesecrets.KindRegistrationKey).Value)
		requireVerified(t, db)

		// Forward nodes: creation, a new token and address, an Ansible
		// machine's empty token, deletion.
		forwards := service.NewForwardNodeService(db)
		forward := &model.ForwardNode{Name: "relay", Type: "relay", Host: "198.51.100.7", Port: 443, APIPort: 9000, APIToken: "fake-forward-token-1", Enabled: true}
		require.NoError(t, forwards.Create(forward))
		token := current(t, db, nodesecrets.SubjectForward, forward.ID, nodesecrets.KindForwardNodeToken)
		require.Equal(t, "fake-forward-token-1", token.Value)
		require.Equal(t, "198.51.100.7:9000", token.Endpoint)
		forward.APIToken = "fake-forward-token-2"
		forward.Host = "198.51.100.8"
		require.NoError(t, forwards.Update(forward))
		token = current(t, db, nodesecrets.SubjectForward, forward.ID, nodesecrets.KindForwardNodeToken)
		require.Equal(t, 2, token.Version)
		require.Equal(t, "198.51.100.8:9000", token.Endpoint)
		requireVerified(t, db)

		// Clean agents: a token for the forward node, then its revocation.
		issued, err := service.CreateCleanAgentTokenTx(db, service.ForwardCleanAgentCreateInput{Name: "agent", NodeID: &forward.ID})
		require.NoError(t, err)
		agentToken := current(t, db, nodesecrets.SubjectCleanAgent, issued.Agent.ID, nodesecrets.KindCleanAgentToken)
		require.Equal(t, issued.Token, agentToken.Value)
		require.Equal(t, nodesecrets.StatusActive, agentToken.Status)
		require.NoError(t, service.RevokeCleanAgentTx(db, issued.Agent.ID))
		agentToken = current(t, db, nodesecrets.SubjectCleanAgent, issued.Agent.ID, nodesecrets.KindCleanAgentToken)
		require.Equal(t, nodesecrets.StatusRevoked, agentToken.Status)
		require.NotNil(t, agentToken.RevokedAt)
		requireVerified(t, db)

		ansible := &model.ForwardNode{Name: "machine", Type: "exit", Host: "198.51.100.9", Port: 22, APIToken: "fake-forward-token-3", Enabled: true}
		require.NoError(t, forwards.Create(ansible))
		ansible.APIToken = ""
		require.NoError(t, forwards.Update(ansible))
		require.Zero(t, credentials(t, db, nodesecrets.SubjectForward, ansible.ID))
		require.NoError(t, forwards.Delete(forward.ID))
		require.Zero(t, credentials(t, db, nodesecrets.SubjectForward, forward.ID))
		requireVerified(t, db)

		// WireGuard peers: the subscription renderer creates them, a
		// rotation replaces their keys, deleting a user takes theirs.
		users := []model.User{
			{Email: "u1@example.test", Token: "fake-user-token-1", UUID: "00000000-0000-4000-8000-000000000001"},
			{Email: "u2@example.test", Token: "fake-user-token-2", UUID: "00000000-0000-4000-8000-000000000002"},
		}
		require.NoError(t, db.Create(&users).Error)
		subscriptions := service.NewSubscriptionService()
		var peerIDs []uint
		for _, user := range users {
			peer, err := subscriptions.GetOrCreateWireGuardPeer(protocol.ID, user.ID, "10.66.0.0/24")
			require.NoError(t, err)
			require.Equal(t, map[string]string{"private_key ": peer.PrivateKey, "preshared_key ": peer.PresharedKey},
				positions(t, db, nodesecrets.ScopeWireGuardPeer, peer.ID))
			peerIDs = append(peerIDs, peer.ID)
		}
		before := positions(t, db, nodesecrets.ScopeWireGuardPeer, peerIDs[0])
		rotated, err := service.RotateWireGuardPeerKeys(db, protocol.ID)
		require.NoError(t, err)
		require.Equal(t, 2, rotated.RotatedPeerCount)
		require.NotEqual(t, before, positions(t, db, nodesecrets.ScopeWireGuardPeer, peerIDs[0]))
		require.Equal(t, 2, secretVersion(t, db, nodesecrets.ScopeWireGuardPeer, peerIDs[0], "private_key", ""))
		requireVerified(t, db)
		require.NoError(t, service.NewUserService().Delete(users[0].ID))
		require.Empty(t, positions(t, db, nodesecrets.ScopeWireGuardPeer, peerIDs[0]))
		requireVerified(t, db)

		// Protocol deletion takes its secrets and its peers'; node deletion
		// takes the node's credentials, raw configuration and protocols.
		require.NoError(t, nodes.DeleteProtocol(protocol.ID))
		require.Empty(t, positions(t, db, nodesecrets.ScopeNodeProtocol, protocol.ID))
		require.Empty(t, positions(t, db, nodesecrets.ScopeWireGuardPeer, peerIDs[1]))
		second := &model.NodeProtocol{NodeID: node.ID, Name: "second", Type: model.ProtocolTrojan, Port: 9443, Settings: ptr(`{"password":"fake-trojan"}`)}
		require.NoError(t, nodes.CreateProtocol(second))
		require.NotEmpty(t, positions(t, db, nodesecrets.ScopeNodeProtocol, second.ID))
		require.NoError(t, nodes.DeleteNode(node.ID))
		require.Zero(t, credentials(t, db, nodesecrets.SubjectProxy, node.ID))
		require.Empty(t, positions(t, db, nodesecrets.ScopeNodeRawConfig, node.ID))
		require.Empty(t, positions(t, db, nodesecrets.ScopeNodeProtocol, second.ID))
		requireVerified(t, db)
	})
}

// failSplitWrites makes every insert into a split table fail, as a full
// disk or a lost connection would.
func failSplitWrites(t *testing.T, b backend) {
	t.Helper()
	for _, table := range []string{"v4_kernel_node_credential", "v4_kernel_protocol_secret"} {
		switch b.name {
		case "sqlite":
			require.NoError(t, b.db.Exec(`CREATE TRIGGER fail_`+table+` BEFORE INSERT ON `+table+` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`).Error)
		default:
			require.NoError(t, b.db.Exec(`CREATE OR REPLACE FUNCTION nodesecretsplit_fail() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql`).Error)
			require.NoError(t, b.db.Exec(`CREATE TRIGGER fail_`+table+` BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION nodesecretsplit_fail()`).Error)
		}
	}
}

// TestDualWriteIsTransactional: when the split write fails, the legacy
// write is rolled back with it.
func TestDualWriteIsTransactional(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		nodes := service.NewNodeService()
		node := &model.Node{Name: "before", Host: "203.0.113.20", Port: 443}
		require.NoError(t, nodes.CreateNode(node))
		protocol := &model.NodeProtocol{NodeID: node.ID, Name: "plain", Type: model.ProtocolVMess, Port: 443}
		require.NoError(t, nodes.CreateProtocol(protocol))
		authKey, rawKey, err := nodes.GenerateAuthKey("before", 0)
		require.NoError(t, err)
		failSplitWrites(t, b)

		count := func(value any, query string, args ...any) int64 {
			t.Helper()
			var n int64
			require.NoError(t, db.Model(value).Where(query, args...).Count(&n).Error)
			return n
		}
		require.Error(t, nodes.CreateNode(&model.Node{Name: "failed", Host: "203.0.113.21", Port: 443}))
		require.Zero(t, count(&model.Node{}, "name = ?", "failed"))
		_, err = nodes.RegisterNode(&model.NodeRegisterRequest{AuthKey: rawKey, Name: "registered"}, "192.0.2.11")
		require.Error(t, err)
		require.Zero(t, count(&model.Node{}, "name = ?", "registered"))
		require.Zero(t, count(&model.AuthorizedKey{}, "id = ? AND used > 0", authKey.ID), "the key is not counted as used")
		_, _, err = nodes.GenerateAuthKey("failed", 0)
		require.Error(t, err)
		require.Zero(t, count(&model.AuthorizedKey{}, "name = ?", "failed"))
		require.Error(t, service.NewForwardNodeService(db).Create(&model.ForwardNode{Name: "failed", Type: "relay", Host: "198.51.100.20", Port: 443, APIToken: "fake-failed-token"}))
		require.Zero(t, count(&model.ForwardNode{}, "name = ?", "failed"))
		require.Error(t, nodes.CreateProtocol(&model.NodeProtocol{NodeID: node.ID, Name: "failed", Type: model.ProtocolTrojan, Port: 9443, Settings: ptr(`{"password":"fake-failed"}`)}))
		require.Zero(t, count(&model.NodeProtocol{}, "name = ?", "failed"))
		require.Error(t, nodes.UpdateProtocol(protocol.ID, map[string]any{"settings": `{"password":"fake-failed"}`}))
		var stored model.NodeProtocol
		require.NoError(t, db.First(&stored, protocol.ID).Error)
		require.Nil(t, stored.Settings, "the protocol keeps its settings")
		require.Error(t, nodes.UpdateNode(node.ID, map[string]any{"raw_config": `{"wireguard":{"private_key":"fake-failed"}}`}))
		require.NoError(t, db.First(node, node.ID).Error)
		require.Nil(t, node.RawConfig, "the node keeps its raw configuration")
		requireVerified(t, db)
	})
}
