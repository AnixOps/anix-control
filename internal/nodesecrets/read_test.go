package nodesecrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
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
// (section 4.4), bypassing the writer: tombstones in the unique credential
// columns, empty tokens, keys and hashes, and the placeholder at every JSON
// secret position. Only the new tables then hold the secrets.
func (f *fixture) moveLegacySecrets(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, row := range f.nodes[:2] {
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", row.ID).
			UpdateColumns(map[string]any{"api_key": Tombstone(uint64(row.ID)), "api_key_hash": "", "secret": ""}).Error)
	}
	raw := `{"server_port":51820,"wireguard":{"private_key":"********","public_key":"fake-raw-public"}}`
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[0].ID).UpdateColumn("raw_config", raw).Error)
	for _, row := range f.keys {
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("id = ?", row.ID).
			UpdateColumns(map[string]any{"key": Tombstone(uint64(row.ID)), "key_hash": ""}).Error)
	}
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", f.forwards[0].ID).UpdateColumn("api_token", "").Error)
	for _, row := range f.agents {
		require.NoError(t, db.Model(&model.ForwardCleanAgent{}).Where("id = ?", row.ID).UpdateColumn("token", Tombstone(uint64(row.ID))).Error)
	}
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
	NodeKeyMatches     []bool
	NodeByKey          []uint
	NodeKeys           []string
	NodeSecrets        []string
	RegistrationKeys   []uint
	ForwardMatches     bool
	ForwardToken       string
	CleanByToken       []uint
	CleanWithToken     []uint
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
	for i, key := range []string{"fake-node-key-1", "fake-node-key-2"} {
		r.NodeKeyMatches = append(r.NodeKeyMatches, NodeAPIKeyMatches(db, &nodes[i], key))
		if node, err := NodeByAPIKey(db, key, true); err == nil {
			r.NodeByKey = append(r.NodeByKey, node.ID)
		} else {
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			r.NodeByKey = append(r.NodeByKey, 0)
		}
		r.NodeKeys = append(r.NodeKeys, NodeAPIKey(db, &nodes[i]))
		r.NodeSecrets = append(r.NodeSecrets, NodeSharedSecret(db, &nodes[i]))
	}
	for _, key := range []string{"fake-registration-1", "fake-registration-2"} {
		var id uint
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if authKey, err := LockRegistrationKey(tx, key); err == nil {
				id = authKey.ID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			return nil
		}))
		r.RegistrationKeys = append(r.RegistrationKeys, id)
	}
	var forward model.ForwardNode
	require.NoError(t, db.First(&forward, f.forwards[0].ID).Error)
	r.ForwardMatches = ForwardNodeTokenMatches(db, &forward, "fake-forward-token-1")
	r.ForwardToken = ForwardNodeToken(db, &forward)
	for i, token := range []string{"fake-clean-token-1", "fake-clean-token-2"} {
		var byToken, withToken uint
		if agent, err := CleanAgentByToken(db, token); err == nil {
			byToken = agent.ID
		}
		if agent, err := CleanAgentWithToken(db, f.agents[i].ID, token); err == nil {
			withToken = agent.ID
		}
		r.CleanByToken = append(r.CleanByToken, byToken)
		r.CleanWithToken = append(r.CleanWithToken, withToken)
	}

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
		for _, kind := range []string{KindNodeAPIKey, KindNodeSharedSecret, KindRegistrationKey, KindForwardNodeToken, KindCleanAgentToken,
			"raw_config", "settings", "tls_settings", "transport_settings", "reality_settings", "custom_config", "private_key", "preshared_key"} {
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
		require.Equal(t, []bool{true, true}, legacy.NodeKeyMatches)
		require.Equal(t, []uint{f.nodes[0].ID, f.nodes[1].ID}, legacy.NodeByKey, "node 2 has no hash: matched by its key")
		require.Equal(t, []string{"fake-node-key-1", "fake-node-key-2"}, legacy.NodeKeys)
		require.Equal(t, []string{"fake-node-secret-1", ""}, legacy.NodeSecrets)
		require.Equal(t, []uint{f.keys[0].ID, f.keys[1].ID}, legacy.RegistrationKeys)
		require.True(t, legacy.ForwardMatches)
		require.Equal(t, "fake-forward-token-1", legacy.ForwardToken)
		require.Equal(t, []uint{f.agents[0].ID, f.agents[1].ID}, legacy.CleanByToken, "a revoked agent is found; the caller refuses it")
		require.Equal(t, []uint{f.agents[0].ID, f.agents[1].ID}, legacy.CleanWithToken)
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
		require.Equal(t, legacy.NodeKeyMatches, moved.NodeKeyMatches)
		require.Equal(t, legacy.NodeByKey, moved.NodeByKey)
		require.Equal(t, legacy.NodeKeys, moved.NodeKeys)
		require.Equal(t, legacy.NodeSecrets, moved.NodeSecrets)
		require.Equal(t, legacy.RegistrationKeys, moved.RegistrationKeys)
		require.True(t, moved.ForwardMatches)
		require.Equal(t, legacy.ForwardToken, moved.ForwardToken)
		require.Equal(t, legacy.CleanByToken, moved.CleanByToken)
		require.Equal(t, legacy.CleanWithToken, moved.CleanWithToken)
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
		require.Equal(t, []bool{false, false}, back.NodeKeyMatches)
		require.Equal(t, []uint{0, 0}, back.NodeByKey)
		require.Equal(t, []uint{0, 0}, back.RegistrationKeys)
		require.False(t, back.ForwardMatches)
		require.Empty(t, back.ForwardToken)
		require.Equal(t, []uint{0, 0}, back.CleanByToken)
		require.Equal(t, []uint{0, 0}, back.CleanWithToken)
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

		// Missing rows: every credential of node 1, the forward token, the
		// registration keys, clean agent 1, the Reality key and peer 1.
		require.NoError(t, db.Where("subject_kind = ? AND subject_id = ?", SubjectProxy, f.nodes[0].ID).Delete(&model.NodeCredential{}).Error)
		require.NoError(t, db.Where("subject_kind IN ?", []string{SubjectForward, SubjectRegistrationKey}).Delete(&model.NodeCredential{}).Error)
		require.NoError(t, db.Where("subject_kind = ? AND subject_id = ?", SubjectCleanAgent, f.agents[0].ID).Delete(&model.NodeCredential{}).Error)
		require.NoError(t, db.Where("scope = ? AND column_name = ?", ScopeNodeProtocol, "reality_settings").Delete(&model.ProtocolSecret{}).Error)
		require.NoError(t, db.Where("scope = ? AND owner_id = ?", ScopeWireGuardPeer, f.peers[0].ID).Delete(&model.ProtocolSecret{}).Error)
		// Mismatching rows: node 2's key, clean agent 2's token, the
		// custom configuration and the raw configuration.
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", SubjectProxy, f.nodes[1].ID).
			Updates(map[string]any{"key_hash": hashSecret("fake-other"), "value": "fake-other"}).Error)
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ?", SubjectCleanAgent, f.agents[1].ID).
			Updates(map[string]any{"key_hash": hashSecret("fake-other"), "value": "fake-other"}).Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ? AND column_name = ?", ScopeNodeProtocol, "custom_config").Update("value", "fake-other").Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope = ?", ScopeNodeRawConfig).Update("value", `"fake-other"`).Error)

		counts := map[[3]string]uint64{}
		keys := [][3]string{
			{TableNode, KindNodeAPIKey, FallbackMissing}, {TableNode, KindNodeAPIKey, FallbackMismatch},
			{TableNode, KindNodeSharedSecret, FallbackMissing}, {TableNode, "raw_config", FallbackMismatch},
			{TableAuthorizedKey, KindRegistrationKey, FallbackMissing},
			{TableForwardNode, KindForwardNodeToken, FallbackMissing},
			{TableCleanAgent, KindCleanAgentToken, FallbackMissing}, {TableCleanAgent, KindCleanAgentToken, FallbackMismatch},
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
		// Node 1's key, read by id, by key and for display, and node 2's
		// looked up by key (its new hash differs, so no row is found),
		// twice: eight fallbacks of a missing row.
		require.EqualValues(t, 8, FallbackCount(TableNode, KindNodeAPIKey, FallbackMissing)-counts[keys[0]])

		// One line per subject, never a value.
		output := logs.String()
		require.NotContains(t, output, "fake-")
		require.Equal(t, 1, strings.Count(output, "table=v2_forward_node kind=forward_node_token subject="+uintString(f.forwards[0].ID)+" reason=missing"), output)
		require.Contains(t, output, "table=v2_node_protocol kind=reality_settings subject=\""+uintString(f.protocols[0].ID)+" /private_key\" reason=missing")

		// A wrong key is not a fallback: nothing matched either form.
		before := FallbackCount(TableNode, KindNodeAPIKey, "")
		node := model.Node{ID: f.nodes[0].ID, APIKey: "fake-node-key-1", APIKeyHash: hashSecret("fake-node-key-1")}
		require.False(t, NodeAPIKeyMatches(db, &node, "fake-wrong"))
		_, err := NodeByAPIKey(db, "fake-wrong", true)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		require.Equal(t, before, FallbackCount(TableNode, KindNodeAPIKey, ""))

		// The counters are exported.
		var body strings.Builder
		WritePrometheus(&body)
		require.Contains(t, body.String(), "# TYPE anixops_node_secrets_fallback_total counter\n")
		require.Contains(t, body.String(), `anixops_node_secrets_fallback_total{table="v2_forward_node",kind="forward_node_token",reason="missing"} `)
		require.Contains(t, body.String(), "# TYPE anixops_node_secrets_invalid_total counter\n")
	})
}

// TestTombstonesAndThePlaceholderNeverAuthenticate: P3 writes tombstones
// and the placeholder into the legacy columns. No reader accepts one as a
// credential, in any phase, whether the legacy column or the new table
// holds it, and whatever reader falls back to the plain key.
func TestTombstonesAndThePlaceholderNeverAuthenticate(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		_, err := Backfill(context.Background(), db, BackfillOptions{})
		require.NoError(t, err)
		requireVerified(t, db)
		f.moveLegacySecrets(t, db)
		// A forward node and a node whose legacy column holds the
		// placeholder, and new rows that hold the hash of a tombstone.
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", f.forwards[1].ID).UpdateColumn("api_token", Placeholder).Error)
		require.NoError(t, db.Model(&model.Node{}).Where("id = ?", f.nodes[2].ID).UpdateColumns(map[string]any{"api_key": Placeholder, "api_key_hash": ""}).Error)
		require.NoError(t, db.Model(&model.NodeCredential{}).Where("subject_kind = ? AND subject_id = ? AND kind = ?", SubjectProxy, f.nodes[1].ID, KindNodeAPIKey).
			Update("key_hash", hashSecret(Tombstone(uint64(f.nodes[1].ID)))).Error)

		for _, phase := range []string{PhaseDualWrite, PhaseDualRead} {
			setPhase(t, db, phase)
			var nodes []model.Node
			require.NoError(t, db.Order("id").Find(&nodes).Error)
			for _, node := range nodes {
				for _, presented := range []string{node.APIKey, Tombstone(uint64(node.ID)), Placeholder, " " + Placeholder} {
					require.False(t, NodeAPIKeyMatches(db, &node, presented), "%s node %d %q", phase, node.ID, presented)
					_, err := NodeByAPIKey(db, presented, true)
					require.ErrorIs(t, err, gorm.ErrRecordNotFound, "%s node %d %q", phase, node.ID, presented)
				}
			}
			for _, key := range f.keys {
				for _, presented := range []string{Tombstone(uint64(key.ID)), Placeholder} {
					err := db.Transaction(func(tx *gorm.DB) error {
						_, err := LockRegistrationKey(tx, presented)
						return err
					})
					require.ErrorIs(t, err, gorm.ErrRecordNotFound, "%s key %d", phase, key.ID)
				}
			}
			var forwards []model.ForwardNode
			require.NoError(t, db.Order("id").Find(&forwards).Error)
			for _, forward := range forwards {
				for _, presented := range []string{forward.APIToken, Placeholder, Tombstone(uint64(forward.ID))} {
					require.False(t, ForwardNodeTokenMatches(db, &forward, presented), "%s forward %d %q", phase, forward.ID, presented)
				}
			}
			for _, agent := range f.agents {
				for _, presented := range []string{Tombstone(uint64(agent.ID)), Placeholder} {
					_, err := CleanAgentByToken(db, presented)
					require.ErrorIs(t, err, gorm.ErrRecordNotFound, "%s agent %d", phase, agent.ID)
					_, err = CleanAgentWithToken(db, agent.ID, presented)
					require.ErrorIs(t, err, gorm.ErrRecordNotFound, "%s agent %d", phase, agent.ID)
				}
			}
		}
		require.True(t, Unusable(""))
		require.True(t, Unusable("!moved:9"))
		require.True(t, Unusable(Placeholder))
		require.False(t, Unusable("fake-key"))
	})
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
