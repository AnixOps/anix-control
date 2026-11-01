package nodesecrets

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// toDualRead copies the fixture into the new tables, verifies both forms
// and moves every table's readers to dual_read.
func toDualRead(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := Backfill(ctx, db, BackfillOptions{})
	require.NoError(t, err)
	requireVerified(t, db)
	_, err = SetPhase(ctx, db, PhaseOptions{Phase: PhaseDualRead, Actor: "test"})
	require.NoError(t, err)
}

func setPhase(t *testing.T, db *gorm.DB, phase string) {
	t.Helper()
	_, err := SetPhase(context.Background(), db, PhaseOptions{Phase: phase, Actor: "test"})
	require.NoError(t, err)
}

// moveLegacySecrets writes into the legacy columns what P3 leaves there
// (section 4.4), bypassing the writer: empty peer keys and the placeholder
// at every JSON secret position. Only the new tables then hold the secrets.
func (f *fixture) moveLegacySecrets(t *testing.T, db *gorm.DB) {
	t.Helper()
	raw := `{"server_port":51820,"wireguard":{"private_key":"********","public_key":"fake-raw-public"}}`
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[0].ID).UpdateColumn("raw_config", raw).Error)
	reality := `{"private_key":"********","short_id":"6ba85179","dest":"www.example.test:443"}`
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).UpdateColumn("reality_settings", reality).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[1].ID).UpdateColumn("custom_config", Placeholder).Error)
	for _, row := range f.peers {
		require.NoError(t, db.Model(&model.WireGuardPeer{}).Where("id = ?", row.ID).
			UpdateColumns(map[string]any{"private_key": "", "preshared_key": ""}).Error)
	}
}

// readings is what every reader answers for the fixture's real secrets.
type readings struct {
	Reality            string
	Custom             string
	Plain              string
	RawConfig          string
	PeerPrivate        []string
	PeerPreshared      []string
	ProtocolUntouched  bool
	RawConfigUntouched bool
}

func (f *fixture) read(t *testing.T, db *gorm.DB) readings {
	t.Helper()
	var r readings
	var nodes []model.Node
	require.NoError(t, db.Order("id").Find(&nodes).Error)

	var protocols []model.NodeProtocol
	require.NoError(t, db.Order("id").Find(&protocols).Error)
	stored := make([]model.NodeProtocol, len(protocols))
	copy(stored, protocols)
	ResolveProtocols(db, protocols)
	r.Reality = *protocols[0].RealitySettings
	r.Plain = *protocols[0].Settings
	r.Custom = *protocols[1].CustomConfig
	r.ProtocolUntouched = protocols[0].RealitySettings == stored[0].RealitySettings && protocols[0].Settings == stored[0].Settings &&
		protocols[1].CustomConfig == stored[1].CustomConfig
	single := stored[0]
	ResolveProtocol(db, &single)
	require.Equal(t, r.Reality, *single.RealitySettings)

	node := nodes[0]
	storedRaw := node.RawConfig
	ResolveNodeRawConfig(db, &node)
	r.RawConfig = *node.RawConfig
	r.RawConfigUntouched = node.RawConfig == storedRaw

	var peers []model.WireGuardPeer
	require.NoError(t, db.Order("id").Find(&peers).Error)
	pointers := []*model.WireGuardPeer{&peers[0], &peers[1]}
	ResolveWireGuardPeers(db, pointers...)
	for _, peer := range peers {
		r.PeerPrivate = append(r.PeerPrivate, peer.PrivateKey)
		r.PeerPreshared = append(r.PeerPreshared, peer.PresharedKey)
	}
	return r
}

// fallbacksTotal sums every fallback counter of the split tables.
func fallbacksTotal() uint64 {
	var total uint64
	for _, table := range Tables() {
		for _, kind := range []string{"raw_config", "settings", "tls_settings", "transport_settings", "reality_settings", "custom_config", "private_key", "preshared_key"} {
			total += FallbackCount(table, kind, "")
		}
	}
	return total
}

// equalJSON requires two JSON documents to hold the same value.
func equalJSON(t *testing.T, want, got string) {
	t.Helper()
	var a, b any
	require.NoError(t, json.Unmarshal([]byte(want), &a))
	require.NoError(t, json.Unmarshal([]byte(got), &b))
	require.Equal(t, a, b)
}

// TestReadersFollowThePhase drives every reader in dual_write and in
// dual_read. In dual_read they read the new tables: with the legacy columns
// holding only what P3 leaves there, they still find every secret. In
// dual_write they read the legacy columns only and never the new tables.
func TestReadersFollowThePhase(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		// Before anything is copied, in dual_write: the legacy answers.
		before := fallbacksTotal()
		legacy := f.read(t, db)
		require.Equal(t, f.reality, legacy.Reality)
		require.Equal(t, f.notJSON, legacy.Custom)
		require.Equal(t, f.rawConfig, legacy.RawConfig)
		require.Equal(t, []string{"fake-peer-private-1", "fake-peer-private-2"}, legacy.PeerPrivate)
		require.Equal(t, []string{"fake-peer-psk-1", "fake-peer-psk-2"}, legacy.PeerPreshared)
		require.True(t, legacy.ProtocolUntouched)
		require.True(t, legacy.RawConfigUntouched)
		require.Equal(t, before, fallbacksTotal(), "dual_write never reads the new tables")

		// In dual_read with both forms equal: the same answers, byte for
		// byte, and no fallback.
		toDualRead(t, db)
		require.Equal(t, legacy, f.read(t, db))
		require.Equal(t, before, fallbacksTotal())

		// Only the new tables hold the secrets now: dual_read still reads
		// every one of them.
		f.moveLegacySecrets(t, db)
		moved := f.read(t, db)
		require.Equal(t, before, fallbacksTotal(), "every secret came from the new tables")
		equalJSON(t, f.reality, moved.Reality)
		require.Equal(t, f.notJSON, moved.Custom)
		equalJSON(t, f.rawConfig, moved.RawConfig)
		require.Equal(t, f.plainProto, moved.Plain, "a column without a secret is never rewritten")
		require.Equal(t, legacy.PeerPrivate, moved.PeerPrivate)
		require.Equal(t, legacy.PeerPreshared, moved.PeerPreshared)

		// Rolled back to dual_write, the readers read only the legacy
		// columns, which no longer hold the secrets here.
		setPhase(t, db, PhaseDualWrite)
		back := f.read(t, db)
		require.Equal(t, before, fallbacksTotal())
		require.Contains(t, back.Reality, Placeholder)
		require.Equal(t, Placeholder, back.Custom)
		require.Equal(t, []string{"", ""}, back.PeerPrivate)
		require.True(t, back.ProtocolUntouched)
	})
}

// TestDualReadFallsBackAndCounts: a missing or mismatching new row falls
// back to the legacy column, counts the metric and logs once per subject,
// without the value.
func TestDualReadFallsBackAndCounts(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		toDualRead(t, db)
		want := f.read(t, db)

		ForgetLogged()
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })

		// Missing rows: the Reality key and peer 1.
		require.NoError(t, db.Where("scope = ? AND column_name = ?", ScopeNodeProtocol, "reality_settings").Delete(&model.ProtocolSecret{}).Error)
		require.NoError(t, db.Where("scope = ? AND owner_id = ?", ScopeWireGuardPeer, f.peers[0].ID).Delete(&model.ProtocolSecret{}).Error)
		// Mismatching rows: the custom configuration and the raw
		// configuration.
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ? AND column_name = ?", ScopeNodeProtocol, "custom_config").Update("value", "fake-other").Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ?", ScopeNodeRawConfig).Update("value", `"fake-other"`).Error)

		counts := map[[3]string]uint64{}
		keys := [][3]string{
			{TableNode, "raw_config", FallbackMismatch},
			{TableNodeProtocol, "reality_settings", FallbackMissing}, {TableNodeProtocol, "custom_config", FallbackMismatch},
			{TableWireGuardPeer, "private_key", FallbackMissing}, {TableWireGuardPeer, "preshared_key", FallbackMissing},
		}
		for _, key := range keys {
			counts[key] = FallbackCount(key[0], key[1], key[2])
		}

		// Every reader still answers from the legacy columns.
		require.Equal(t, want, f.read(t, db))
		require.Equal(t, want, f.read(t, db))

		// Each reader counted each of its fallbacks, on both reads.
		for _, key := range keys {
			require.Greater(t, FallbackCount(key[0], key[1], key[2]), counts[key], "%v", key)
		}
		// One line per subject, never a value.
		output := logs.String()
		require.NotContains(t, output, "fake-")
		require.Equal(t, 1, strings.Count(output, "table=v2_node_protocol kind=reality_settings subject=\""+uintString(f.protocols[0].ID)+" /private_key\" reason=missing"), output)

		// The counters are exported.
		var body strings.Builder
		WritePrometheus(&body)
		require.Contains(t, body.String(), "# TYPE anixops_node_secrets_fallback_total counter\n")
		require.Contains(t, body.String(), `anixops_node_secrets_fallback_total{table="v2_node_protocol",kind="reality_settings",reason="missing"} `)
		require.Contains(t, body.String(), "# TYPE anixops_node_secrets_invalid_total counter\n")
	})
}

// TestUnusable: the tombstone, the placeholder and an empty value are never
// a usable secret.
func TestUnusable(t *testing.T) {
	require.True(t, Unusable(""))
	require.True(t, Unusable("!moved:9"))
	require.True(t, Unusable(Placeholder))
	require.False(t, Unusable("fake-key"))
}

func TestReplacePositions(t *testing.T) {
	document := `{"a":{"private_key":"********","n":1.50},"list":[{"password":"********"}],"x~/y":{"token":"********"}}`
	resolved, err := replacePositions(document, map[string]string{
		"/a/private_key": `"fake-a"`, "/list/0/password": `"fake-b"`, "/x~0~1y/token": `["fake-c"]`,
	})
	require.NoError(t, err)
	equalJSON(t, `{"a":{"private_key":"fake-a","n":1.50},"list":[{"password":"fake-b"}],"x~/y":{"token":["fake-c"]}}`, resolved)
	require.Contains(t, resolved, "1.50", "numbers keep their text")

	whole, err := replacePositions(Placeholder, map[string]string{"": "fake-text"})
	require.NoError(t, err)
	require.Equal(t, "fake-text", whole)

	for _, bad := range []map[string]string{{"/missing/key": `"x"`}, {"/list/7/password": `"x"`}, {"/a/private_key": `not json`}} {
		_, err := replacePositions(document, bad)
		require.Error(t, err)
	}
	_, err = replacePositions("not json", map[string]string{"/a": `"x"`})
	require.Error(t, err)
}
