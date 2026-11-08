package nodesecretsplit

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fleet is every kind of subject the readers authenticate or present a
// secret of, written by the kernel's own writers (and one row written past
// them). Every secret is fake.
type fleet struct {
	plain          model.Node // written past the writers
	created        model.Node // CreateNode: a Reality protocol
	rawNode        model.Node // a raw configuration with a private key
	rawPrivate     string
	reality        model.NodeProtocol
	realityPrivate string
	wireGuard      model.NodeProtocol
	user           model.User
	peer           model.WireGuardPeer
}

func base64Key(t *testing.T, encoding *base64.Encoding) string {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return encoding.EncodeToString(key)
}

func newFleet(t *testing.T, db *gorm.DB) *fleet {
	t.Helper()
	f := &fleet{}
	nodes := service.NewNodeService()

	f.plain = model.Node{Name: "plain", Host: "203.0.113.50", Port: 443}
	require.NoError(t, db.Create(&f.plain).Error)

	f.created = model.Node{Name: "created", Host: "203.0.113.51", Port: 443}
	require.NoError(t, nodes.CreateNode(&f.created))
	f.realityPrivate = base64Key(t, base64.RawURLEncoding)
	f.reality = model.NodeProtocol{NodeID: f.created.ID, Name: "reality", Type: model.ProtocolVLESS, Port: 8443, TLS: 2, Enable: 1,
		RealitySettings: ptr(`{"private_key":"` + f.realityPrivate + `","short_id":"6ba85179","dest":"www.example.test:443"}`)}
	require.NoError(t, nodes.CreateProtocol(&f.reality))

	var err error
	f.rawNode = model.Node{Name: "raw", Host: "203.0.113.52", Port: 443}
	require.NoError(t, nodes.CreateNode(&f.rawNode))
	f.rawPrivate = base64Key(t, base64.RawURLEncoding)
	raw := `{"node_type":"vless","type":"vless","server_port":443,"tls":2,"tls_settings":{"private_key":"` + f.rawPrivate + `","short_id":"01"}}`
	require.NoError(t, nodes.UpdateNode(f.rawNode.ID, map[string]any{"raw_config": raw}))

	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	// Disabled, so it needs no relay; its peers are kept all the same.
	f.wireGuard = model.NodeProtocol{NodeID: f.created.ID, Name: "wg", Type: model.ProtocolWireGuard, Port: 51820,
		Settings: ptr(`{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","server_private_key":"` + base64.StdEncoding.EncodeToString(private.Bytes()) + `"}`)}
	require.NoError(t, nodes.CreateProtocol(&f.wireGuard))
	f.user = model.User{Email: "fleet@example.test", Token: "fake-fleet-user-token", UUID: "00000000-0000-4000-8000-000000000050"}
	require.NoError(t, db.Create(&f.user).Error)
	peer, err := service.NewSubscriptionService().GetOrCreateWireGuardPeer(f.wireGuard.ID, f.user.ID, "10.66.0.0/24")
	require.NoError(t, err)
	f.peer = *peer
	return f
}

// readerResults runs every kernel reader of a moved column once and
// reports, per reader, whether it answered its real secret.
func (f *fleet) readerResults(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	results := map[string]bool{}
	// The node configuration: the Reality private key of a protocol, and
	// the private key of a raw configuration.
	configKey := func(nodeID uint) string {
		config, err := service.BuildNodeRuntimeConfig(db, nodeID, "vless")
		if err != nil {
			return ""
		}
		settings, _ := config["tls_settings"].(map[string]any)
		value, _ := settings["private_key"].(string)
		return value
	}
	results["created config"] = configKey(f.created.ID) == f.realityPrivate
	results["raw config"] = configKey(f.rawNode.ID) == f.rawPrivate

	// The WireGuard peer in a subscription and in the node's user list.
	peer, err := service.NewSubscriptionService().GetOrCreateWireGuardPeer(f.wireGuard.ID, f.user.ID, "10.66.0.0/24")
	results["wireguard subscription"] = err == nil && peer.PrivateKey == f.peer.PrivateKey && peer.PresharedKey == f.peer.PresharedKey
	var protocol model.NodeProtocol
	require.NoError(t, db.First(&protocol, f.wireGuard.ID).Error)
	extras, err := service.NewSubscriptionService().BuildWireGuardRuntimeUserExtras(&protocol, []*model.User{&f.user})
	results["wireguard users"] = err == nil && extras[f.user.ID]["wireguard_preshared_key"] == f.peer.PresharedKey
	return results
}

func requireAll(t *testing.T, results map[string]bool, want bool) {
	t.Helper()
	for reader, got := range results {
		require.Equal(t, want, got, reader)
	}
}

// TestEveryReaderReadsTheNewTables: every reader authenticates every
// subject and answers every secret from the new tables, while the legacy
// columns hold none; without the new rows no reader falls back to the
// legacy columns.
func TestEveryReaderReadsTheNewTables(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		f := newFleet(t, db)
		requireMoved(t, db)
		requireAll(t, f.readerResults(t, db), true)

		// The writers keep working.
		require.NoError(t, service.NewNodeService().UpdateNode(f.rawNode.ID, map[string]any{"name": "raw-renamed"}))
		requireAll(t, f.readerResults(t, db), true)
		requireMoved(t, db)

		require.NoError(t, db.Where("owner_id IN ? AND scope = ?", []uint{f.reality.ID, f.wireGuard.ID}, nodesecrets.ScopeNodeProtocol).Delete(&model.ProtocolSecret{}).Error)
		require.NoError(t, db.Where("scope IN ?", []string{nodesecrets.ScopeWireGuardPeer, nodesecrets.ScopeNodeRawConfig}).Delete(&model.ProtocolSecret{}).Error)
		requireAll(t, f.readerResults(t, db), false)
	})
}

// TestValidateOnBuildReportsWithoutExcluding: a protocol whose secret fails
// validation is reported when a node's configuration is built from it,
// with the metric and a log line naming the node, protocol and field, and
// is still in the configuration (D7: report-only). The scan reports the
// same rows and excludes nothing.
func TestValidateOnBuildReportsWithoutExcluding(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		nodes := service.NewNodeService()
		node := &model.Node{Name: "invalid", Host: "203.0.113.60", Port: 443}
		require.NoError(t, nodes.CreateNode(node))
		// Written around the kernel's write checks, as an import or a
		// package could: a masked Reality key and a malformed WireGuard key.
		masked := model.NodeProtocol{NodeID: node.ID, Name: "masked", Type: model.ProtocolVLESS, Port: 8443, TLS: 2, Enable: 1,
			RealitySettings: ptr(`{"private_key":"********","short_id":"01"}`)}
		malformed := model.NodeProtocol{NodeID: node.ID, Name: "malformed", Type: model.ProtocolWireGuard, Port: 51820, Enable: 1,
			Settings: ptr(`{"cidr":"10.66.0.0/24","server_private_key":"fake-not-a-key"}`)}
		require.NoError(t, db.Create(&masked).Error)
		require.NoError(t, db.Create(&malformed).Error)
		require.NoError(t, nodesecrets.SeedForTest(db, nodesecrets.TableNodeProtocol))

		nodesecrets.ForgetLogged()
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })
		placeholders := nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonPlaceholder)
		wireGuardKeys := nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonWireGuardKey)

		var stored model.Node
		require.NoError(t, db.First(&stored, node.ID).Error)
		config := service.BuildNodeProtocolConfig(db, &stored, &masked)
		require.Equal(t, "********", config["tls_settings"].(map[string]any)["private_key"], "reported, not excluded")
		config = service.BuildNodeProtocolConfig(db, &stored, &malformed)
		require.Equal(t, "fake-not-a-key", config["server_private_key"], "reported, not excluded")
		require.EqualValues(t, 1, nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonPlaceholder)-placeholders)
		require.EqualValues(t, 1, nodesecrets.InvalidCount(nodesecrets.TableNodeProtocol, service.NodeSecretReasonWireGuardKey)-wireGuardKeys)
		output := logs.String()
		require.Contains(t, output, fmt.Sprintf(`subject="node %d protocol %d reality_settings /private_key" reason=placeholder`, node.ID, masked.ID))
		require.Contains(t, output, fmt.Sprintf(`subject="node %d protocol %d settings /server_private_key" reason=wireguard_key`, node.ID, malformed.ID))
		require.NotContains(t, output, "fake-not-a-key")

		report, err := service.ScanNodeSecrets(context.Background(), db)
		require.NoError(t, err)
		require.Zero(t, report.Excluded)
		require.EqualValues(t, 3, report.Protocols, "the default protocol and the two written above")
		require.Equal(t, []service.NodeSecretFinding{
			{NodeID: node.ID, ProtocolID: masked.ID, Type: "vless", Enabled: true, Column: "reality_settings", Field: "/private_key", Reason: service.NodeSecretReasonPlaceholder},
			{NodeID: node.ID, ProtocolID: malformed.ID, Type: "wireguard", Enabled: true, Column: "settings", Field: "/server_private_key", Reason: service.NodeSecretReasonWireGuardKey},
		}, report.Findings)
		var count int64
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("node_id = ?", node.ID).Count(&count).Error)
		require.EqualValues(t, 3, count, "validation changes nothing")
	})
}
