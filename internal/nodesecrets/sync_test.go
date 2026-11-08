package nodesecrets

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fixture is legacy rows written past the writers, secrets in the legacy
// columns. Every value is fake.
type fixture struct {
	nodes      []model.Node
	protocols  []model.NodeProtocol
	peers      []model.WireGuardPeer
	rawConfig  string
	reality    string
	notJSON    string
	plainProto string
}

func seedLegacy(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	f := &fixture{
		rawConfig:  `{"server_port":51820,"wireguard":{"private_key":"fake-raw-private","public_key":"fake-raw-public"}}`,
		reality:    `{"private_key":"fake-reality-private","short_id":"6ba85179","dest":"www.example.test:443"}`,
		notJSON:    "fake-custom-config",
		plainProto: `{"flow":"xtls-rprx-vision"}`,
	}
	f.nodes = []model.Node{
		{Name: "n1", Host: "203.0.113.1", RawConfig: &f.rawConfig},
		{Name: "n2", Host: "203.0.113.2"},
		{Name: "n3", Host: "203.0.113.3"},
	}
	for i := range f.nodes {
		require.NoError(t, db.Create(&f.nodes[i]).Error)
	}
	f.protocols = []model.NodeProtocol{
		{NodeID: f.nodes[0].ID, Name: "reality", Type: model.ProtocolVLESS, Port: 443, TLS: 2, Settings: &f.plainProto, RealitySettings: &f.reality},
		{NodeID: f.nodes[0].ID, Name: "custom", Type: model.ProtocolVMess, Port: 8443, CustomConfig: &f.notJSON},
		{NodeID: f.nodes[1].ID, Name: "plain", Type: model.ProtocolVMess, Port: 443},
	}
	for i := range f.protocols {
		require.NoError(t, db.Create(&f.protocols[i]).Error)
	}
	f.peers = []model.WireGuardPeer{
		{NodeProtocolID: f.protocols[0].ID, UserID: 1, PeerIP: "10.66.0.2", PrivateKey: "fake-peer-private-1", PublicKey: "fake-peer-public-1", PresharedKey: "fake-peer-psk-1"},
		{NodeProtocolID: f.protocols[0].ID, UserID: 2, PeerIP: "10.66.0.3", PrivateKey: "fake-peer-private-2", PublicKey: "fake-peer-public-2", PresharedKey: "fake-peer-psk-2"},
	}
	for i := range f.peers {
		require.NoError(t, db.Create(&f.peers[i]).Error)
	}
	return f
}

// syncAll syncs every legacy row of the fixture, as the writers do.
func (f *fixture) syncAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for table, ids := range f.ids() {
			if err := Sync(tx, table, ids...); err != nil {
				return err
			}
		}
		return nil
	}))
}

func (f *fixture) ids() map[string][]uint {
	ids := map[string][]uint{}
	for _, row := range f.nodes {
		ids[TableNode] = append(ids[TableNode], row.ID)
	}
	for _, row := range f.protocols {
		ids[TableNodeProtocol] = append(ids[TableNodeProtocol], row.ID)
	}
	for _, row := range f.peers {
		ids[TableWireGuardPeer] = append(ids[TableWireGuardPeer], row.ID)
	}
	return ids
}

func secrets(t *testing.T, db *gorm.DB, scope string, ownerID uint) map[string]string {
	t.Helper()
	var rows []model.ProtocolSecret
	require.NoError(t, db.Where("scope = ? AND owner_id = ?", scope, ownerID).Find(&rows).Error)
	out := map[string]string{}
	for _, row := range rows {
		out[row.ColumnName+" "+row.JSONPointer] = row.Value
	}
	return out
}

func TestSyncDerivesEveryTable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)

		require.Empty(t, secrets(t, db, ScopeNodeRawConfig, f.nodes[1].ID), "a node without a raw configuration")
		require.Equal(t, map[string]string{"raw_config /wireguard/private_key": `"fake-raw-private"`}, secrets(t, db, ScopeNodeRawConfig, f.nodes[0].ID))
		require.Equal(t, map[string]string{"reality_settings /private_key": `"fake-reality-private"`}, secrets(t, db, ScopeNodeProtocol, f.protocols[0].ID))
		require.Equal(t, map[string]string{"custom_config ": "fake-custom-config"}, secrets(t, db, ScopeNodeProtocol, f.protocols[1].ID))
		require.Empty(t, secrets(t, db, ScopeNodeProtocol, f.protocols[2].ID))
		require.Equal(t, map[string]string{"private_key ": "fake-peer-private-1", "preshared_key ": "fake-peer-psk-1"}, secrets(t, db, ScopeWireGuardPeer, f.peers[0].ID))
		f.requireFinalizedColumns(t, db)

		// A second sync changes nothing.
		var before []model.ProtocolSecret
		require.NoError(t, db.Order("id").Find(&before).Error)
		f.syncAll(t, db)
		var after []model.ProtocolSecret
		require.NoError(t, db.Order("id").Find(&after).Error)
		require.Equal(t, before, after)
		f.requireFinalizedColumns(t, db)
	})
}

func TestSyncRotatesChangesAndRemoves(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		node := f.nodes[0]
		sync := func(table string, id uint) {
			t.Helper()
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return Sync(tx, table, id) }))
		}

		// A changed secret position counts a version; a removed one goes.
		changed := `{"private_key":"fake-reality-private-b","short_id":"6ba85179"}`
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).Update("reality_settings", changed).Error)
		sync(TableNodeProtocol, f.protocols[0].ID)
		var position model.ProtocolSecret
		require.NoError(t, db.Where("scope = ? AND owner_id = ? AND json_pointer = ?", ScopeNodeProtocol, f.protocols[0].ID, "/private_key").First(&position).Error)
		require.Equal(t, `"fake-reality-private-b"`, position.Value)
		require.Equal(t, 2, position.Version)
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).Update("reality_settings", `{"short_id":"6ba85179"}`).Error)
		sync(TableNodeProtocol, f.protocols[0].ID)
		require.Empty(t, secrets(t, db, ScopeNodeProtocol, f.protocols[0].ID))

		// A deleted row takes every row of it, retired versions included.
		require.NoError(t, db.Delete(&model.Node{}, node.ID).Error)
		sync(TableNode, node.ID)
		require.Empty(t, secrets(t, db, ScopeNodeRawConfig, node.ID))
		require.NoError(t, db.Delete(&model.WireGuardPeer{}, f.peers[0].ID).Error)
		sync(TableWireGuardPeer, f.peers[0].ID)
		require.Empty(t, secrets(t, db, ScopeWireGuardPeer, f.peers[0].ID))
	})
}

func TestPruneRemovesOrphans(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		require.NoError(t, db.Create(&model.ProtocolSecret{
			Scope: ScopeNodeRawConfig, OwnerID: 999, ColumnName: "raw_config", JSONPointer: "/wireguard/private_key", Version: 1,
			Value: `"fake-orphan"`, UpdatedAt: time.Now(),
		}).Error)
		pruned, err := Prune(db, TableNode, TableNodeProtocol, TableWireGuardPeer)
		require.NoError(t, err)
		require.EqualValues(t, 1, pruned)
		require.Empty(t, secrets(t, db, ScopeNodeRawConfig, 999))
		require.NotEmpty(t, secrets(t, db, ScopeNodeRawConfig, f.nodes[0].ID))
		_, err = Prune(db, "v2_user")
		require.Error(t, err)
	})
}

// TestStateRowsAreFinalized: EnsureStateRows records every table as
// finalized, and marks an unfinalized row so.
func TestStateRowsAreFinalized(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		requireAllFinalized := func() {
			t.Helper()
			rows, err := Status(context.Background(), db)
			require.NoError(t, err)
			require.Len(t, rows, len(Tables()))
			for _, row := range rows {
				require.Equal(t, PhaseFinalized, row.Phase)
				require.NotNil(t, row.FinalizedAt)
			}
		}
		requireAllFinalized()
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).
			Updates(map[string]any{"phase": "dual_read", "finalized_at": nil}).Error)
		require.NoError(t, EnsureStateRows(db))
		requireAllFinalized()
		require.Error(t, Sync(db, "v2_user", 1))
	})
}

// TestRequireFinalized: a split table that holds rows and is not finalized
// refuses the start; an empty one, or a database without the tables,
// passes.
func TestRequireFinalized(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, RequireFinalized(db), "finalized, and empty")
		f := seedLegacy(t, db)
		require.NoError(t, RequireFinalized(db), "finalized")

		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNodeProtocol).
			Update("phase", "dual_read").Error)
		err := RequireFinalized(db)
		require.ErrorContains(t, err, "table v2_node_protocol is in phase dual_read")
		require.ErrorContains(t, err, "Upgrading To 5.0.3")

		// An interrupted finalize.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNodeProtocol).
			Updates(map[string]any{"phase": PhaseFinalized, "finalized_at": nil}).Error)
		require.ErrorContains(t, RequireFinalized(db), "table v2_node_protocol is in phase finalized")

		// The same table without rows passes.
		require.NoError(t, db.Where("id IN ?", []uint{f.protocols[0].ID, f.protocols[1].ID, f.protocols[2].ID}).Delete(&model.NodeProtocol{}).Error)
		require.NoError(t, RequireFinalized(db))

		// No state rows, legacy rows: a database from before the split.
		require.NoError(t, db.Where("1 = 1").Delete(&model.NodeSecretSplit{}).Error)
		require.ErrorContains(t, RequireFinalized(db), "table v2_node is in phase none")
	})
	t.Run("fresh", func(t *testing.T) {
		require.NoError(t, RequireFinalized(openSQLite(t)))
	})
}

func TestSyncWithoutSplitTables(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(legacyModels...))
	f := seedLegacy(t, db)
	require.NoError(t, Sync(db, TableNode, f.nodes[0].ID), "nothing to move")
	var node model.Node
	require.NoError(t, db.First(&node, f.nodes[0].ID).Error)
	require.Equal(t, f.rawConfig, *node.RawConfig)
}
