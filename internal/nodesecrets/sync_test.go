package nodesecrets

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fixture is a database written before P1: legacy rows only. Every value is
// fake.
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

// syncAll dual-writes every legacy row of the fixture, as the writers do.
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

// requireVerified runs Verify and requires every table to match.
func requireVerified(t *testing.T, db *gorm.DB) []TableVerify {
	t.Helper()
	results, err := Verify(context.Background(), db, VerifyOptions{})
	require.NoError(t, err, "%+v", results)
	for _, result := range results {
		require.True(t, result.Match, "%+v", result)
		require.Equal(t, result.LegacyDigest, result.NewDigest)
	}
	return results
}

type snapshot struct {
	Secrets []model.ProtocolSecret
}

func takeSnapshot(t *testing.T, db *gorm.DB) snapshot {
	t.Helper()
	var s snapshot
	require.NoError(t, db.Order("id").Find(&s.Secrets).Error)
	return s
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

		requireVerified(t, db)
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
		requireVerified(t, db)

		// A deleted row takes every row of it, retired versions included.
		require.NoError(t, db.Delete(&model.Node{}, node.ID).Error)
		sync(TableNode, node.ID)
		require.Empty(t, secrets(t, db, ScopeNodeRawConfig, node.ID))
		require.NoError(t, db.Delete(&model.WireGuardPeer{}, f.peers[0].ID).Error)
		sync(TableWireGuardPeer, f.peers[0].ID)
		require.Empty(t, secrets(t, db, ScopeWireGuardPeer, f.peers[0].ID))
		requireVerified(t, db)
	})
}

func TestBackfillThenVerifyIsIdempotent(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		seedLegacy(t, db)

		// Before the backfill the new tables are empty: every secret is
		// missing, and the report names none of them.
		results, err := Verify(ctx, db, VerifyOptions{})
		require.ErrorIs(t, err, ErrMismatch)
		missing := int64(0)
		for _, result := range results {
			require.False(t, result.Match && result.LegacySecrets > 0)
			missing += result.Missing
		}
		require.EqualValues(t, 7, missing)
		report, err := json.Marshal(results)
		require.NoError(t, err)
		require.NotContains(t, string(report), "fake-")

		backfilled, err := Backfill(ctx, db, BackfillOptions{BatchSize: 2})
		require.NoError(t, err)
		require.Len(t, backfilled, len(Tables()))
		changed := int64(0)
		for _, result := range backfilled {
			require.False(t, result.Resumed)
			changed += result.Changed
		}
		require.EqualValues(t, 7, changed)

		verified := requireVerified(t, db)
		var splits []model.NodeSecretSplit
		require.NoError(t, db.Order("table_name").Find(&splits).Error)
		require.Len(t, splits, len(Tables()))
		digests := map[string]string{}
		for _, result := range verified {
			digests[result.Table] = result.LegacyDigest
		}
		for _, split := range splits {
			require.Equal(t, PhaseDualWrite, split.Phase, "verify leaves the phase at P1")
			require.NotNil(t, split.BackfilledAt)
			require.NotNil(t, split.VerifiedAt)
			require.Equal(t, digests[split.Table], split.Digest)
			require.Zero(t, split.Mismatches)
		}
		// A second backfill changes nothing, not even a timestamp.
		before := takeSnapshot(t, db)
		again, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		for _, result := range again {
			require.Zero(t, result.Changed, result.Table)
		}
		require.Equal(t, before, takeSnapshot(t, db))
		requireVerified(t, db)
	})
}

func TestBackfillResumesAndPrunes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		// An interrupted pass stopped after the first node.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Updates(map[string]any{
			"backfill_cursor": f.nodes[0].ID, "backfilled_rows": 1, "backfilled_at": nil,
		}).Error)
		// A secret of a node that no longer exists.
		require.NoError(t, db.Create(&model.ProtocolSecret{
			Scope: ScopeNodeRawConfig, OwnerID: 999, ColumnName: "raw_config", JSONPointer: "/wireguard/private_key", Version: 1,
			Value: `"fake-orphan"`, UpdatedAt: time.Now(),
		}).Error)

		results, err := Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}, BatchSize: 1})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.True(t, results[0].Resumed)
		require.EqualValues(t, 3, results[0].Rows)
		require.EqualValues(t, 1, results[0].Pruned)
		// The resumed pass skipped the first node: verify says so.
		verified, err := Verify(ctx, db, VerifyOptions{Tables: []string{TableNode}})
		require.ErrorIs(t, err, ErrMismatch)
		require.EqualValues(t, 1, verified[0].Missing, "the first node's raw configuration")
		require.Zero(t, verified[0].Extra)

		// The pass completed, so the next one starts over and copies it.
		results, err = Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}})
		require.NoError(t, err)
		require.False(t, results[0].Resumed)
		require.EqualValues(t, 1, results[0].Changed)
		verified, err = Verify(ctx, db, VerifyOptions{Tables: []string{TableNode}})
		require.NoError(t, err)
		require.True(t, verified[0].Match)

		// Restart ignores an interrupted pass.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Updates(map[string]any{
			"backfill_cursor": f.nodes[2].ID, "backfilled_at": nil,
		}).Error)
		results, err = Backfill(ctx, db, BackfillOptions{Tables: []string{TableNode}, Restart: true})
		require.NoError(t, err)
		require.False(t, results[0].Resumed)
		require.EqualValues(t, 3, results[0].Rows)
	})
}

func TestVerifyDetectsMismatch(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		f := seedLegacy(t, db)
		_, err := Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		var before model.NodeSecretSplit
		require.NoError(t, db.First(&before, "table_name = ?", TableNode).Error)

		// A legacy write that skipped the writer, and a new row changed
		// behind its back.
		drifted := `{"server_port":51820,"wireguard":{"private_key":"fake-raw-private-x","public_key":"fake-raw-public"}}`
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[0].ID).Update("raw_config", drifted).Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ? AND owner_id = ?", ScopeWireGuardPeer, f.peers[1].ID).
			Where("column_name = ?", "private_key").Update("value", "fake-peer-private-x").Error)

		results, err := Verify(ctx, db, VerifyOptions{})
		require.ErrorIs(t, err, ErrMismatch)
		byTable := map[string]TableVerify{}
		for _, result := range results {
			byTable[result.Table] = result
		}
		node := byTable[TableNode]
		require.False(t, node.Match)
		require.NotEqual(t, node.LegacyDigest, node.NewDigest)
		require.EqualValues(t, 1, node.Different)
		require.Equal(t, []Mismatch{{Problem: "different", Secret: "node_raw_config " + uintString(f.nodes[0].ID) + " raw_config /wireguard/private_key"}}, node.Samples)
		require.EqualValues(t, 1, byTable[TableWireGuardPeer].Different)
		require.Equal(t, "wireguard_peer "+uintString(f.peers[1].ID)+" private_key ", byTable[TableWireGuardPeer].Samples[0].Secret)
		require.True(t, byTable[TableNodeProtocol].Match)
		report, err := json.Marshal(results)
		require.NoError(t, err)
		require.NotContains(t, string(report), "fake-")

		var after model.NodeSecretSplit
		require.NoError(t, db.First(&after, "table_name = ?", TableNode).Error)
		require.EqualValues(t, 1, after.Mismatches)
		require.NotNil(t, after.CheckedAt)
		require.Equal(t, before.Digest, after.Digest, "a mismatch keeps the last verified digest")
		require.Equal(t, before.VerifiedAt.Unix(), after.VerifiedAt.Unix())

		// A backfill repairs both kinds of drift.
		_, err = Backfill(ctx, db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
	})
}

func TestSplitTablesAndPhase(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		rows, err := Status(context.Background(), db)
		require.NoError(t, err)
		require.Len(t, rows, len(Tables()))
		for _, row := range rows {
			require.Equal(t, PhaseDualWrite, row.Phase)
		}
		// EnsureStateRows never changes an existing phase.
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", TableNode).Update("phase", PhaseLegacy).Error)
		require.NoError(t, EnsureStateRows(db))
		var split model.NodeSecretSplit
		require.NoError(t, db.First(&split, "table_name = ?", TableNode).Error)
		require.Equal(t, PhaseLegacy, split.Phase)

		_, err = Backfill(context.Background(), db, BackfillOptions{Tables: []string{"v2_user"}})
		require.Error(t, err)
		require.Error(t, Sync(db, "v2_user", 1))
	})
}

func TestSyncWithoutSplitTables(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(legacyModels...))
	f := seedLegacy(t, db)
	require.NoError(t, Sync(db, TableNode, f.nodes[0].ID), "nothing to dual-write")
	_, err := Backfill(context.Background(), db, BackfillOptions{})
	require.Error(t, err)
	_, err = Verify(context.Background(), db, VerifyOptions{})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMismatch))
}

func uintString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
