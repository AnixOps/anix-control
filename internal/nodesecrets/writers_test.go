package nodesecrets

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// requireFinalizedColumns requires the legacy columns of the fixture to
// hold exactly what section 4.4 leaves there.
func (f *fixture) requireFinalizedColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	var nodes []model.Node
	require.NoError(t, db.Order("id").Find(&nodes).Error)
	require.Equal(t, Redact(f.rawConfig), *nodes[0].RawConfig)
	require.Contains(t, *nodes[0].RawConfig, Placeholder)
	require.Contains(t, *nodes[0].RawConfig, "fake-raw-public", "the public key stays")

	var protocols []model.NodeProtocol
	require.NoError(t, db.Order("id").Find(&protocols).Error)
	require.Equal(t, Redact(f.reality), *protocols[0].RealitySettings)
	require.Equal(t, f.plainProto, *protocols[0].Settings, "a column without a secret is untouched")
	require.Equal(t, Placeholder, *protocols[1].CustomConfig)
	var peers []model.WireGuardPeer
	require.NoError(t, db.Order("id").Find(&peers).Error)
	for _, peer := range peers {
		require.Equal(t, Tombstone(uint64(peer.ID)), peer.PrivateKey)
		require.Equal(t, Tombstone(uint64(peer.ID)), peer.PresharedKey)
		require.NotEmpty(t, peer.PublicKey)
	}
}

// TestWritersLeaveTombstones: a writer's secret reaches the new tables
// only; a row saved back as it is, or a document sent back with the
// placeholder, keeps the secret.
func TestWritersLeaveTombstones(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		write := func(table string, id uint, model any, updates map[string]any) {
			t.Helper()
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(model).Where("id = ?", id).UpdateColumns(updates).Error; err != nil {
					return err
				}
				return Sync(tx, table, id)
			}))
		}
		node := f.nodes[0]
		require.NoError(t, db.First(&node, node.ID).Error)
		// A row loaded and saved back as it is keeps every secret.
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&node).Error; err != nil {
				return err
			}
			return Sync(tx, TableNode, node.ID)
		}))
		equalJSON(t, f.rawConfig, func() string { n := node; ResolveNodeRawConfig(db, &n); return *n.RawConfig }())
		// A protocol written with a new literal secret: redacted in the
		// column, the value in the new table.
		reality := `{"short_id":"02","private_key":"fake-reality-rotated"}`
		write(TableNodeProtocol, f.protocols[0].ID, &model.NodeProtocol{}, map[string]any{"reality_settings": reality})
		var protocol model.NodeProtocol
		require.NoError(t, db.First(&protocol, f.protocols[0].ID).Error)
		require.Equal(t, Redact(reality), *protocol.RealitySettings)
		ResolveProtocol(db, &protocol)
		equalJSON(t, reality, *protocol.RealitySettings)
		// A document sent back with the placeholder keeps the secret.
		write(TableNodeProtocol, f.protocols[0].ID, &model.NodeProtocol{}, map[string]any{"reality_settings": `{"private_key":"********","short_id":"03"}`})
		require.NoError(t, db.First(&protocol, f.protocols[0].ID).Error)
		ResolveProtocol(db, &protocol)
		equalJSON(t, `{"private_key":"fake-reality-rotated","short_id":"03"}`, *protocol.RealitySettings)
	})
}
