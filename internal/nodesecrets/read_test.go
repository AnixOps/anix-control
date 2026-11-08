package nodesecrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

// equalJSON requires two JSON documents to hold the same value.
func equalJSON(t *testing.T, want, got string) {
	t.Helper()
	var a, b any
	require.NoError(t, json.Unmarshal([]byte(want), &a))
	require.NoError(t, json.Unmarshal([]byte(got), &b))
	require.Equal(t, a, b)
}

// TestReadersReadTheNewTables: every reader answers the secrets from the
// new tables; a column without a secret is never rewritten, a literal
// secret written past the writers is ignored, and a missing row leaves the
// placeholder or no key. The validate-on-build counter is exported.
func TestReadersReadTheNewTables(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := seedLegacy(t, db)
		f.syncAll(t, db)
		read := f.read(t, db)
		equalJSON(t, f.reality, read.Reality)
		require.Equal(t, f.notJSON, read.Custom)
		equalJSON(t, f.rawConfig, read.RawConfig)
		require.Equal(t, f.plainProto, read.Plain, "a column without a secret is never rewritten")
		require.Equal(t, []string{"fake-peer-private-1", "fake-peer-private-2"}, read.PeerPrivate)
		require.Equal(t, []string{"fake-peer-psk-1", "fake-peer-psk-2"}, read.PeerPreshared)

		// A literal secret written past the writers is never read.
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", f.protocols[0].ID).
			UpdateColumn("reality_settings", `{"private_key":"fake-bypass","short_id":"6ba85179","dest":"www.example.test:443"}`).Error)
		equalJSON(t, f.reality, f.read(t, db).Reality)

		// Without its rows, a position keeps the placeholder and a peer has
		// no key.
		require.NoError(t, db.Where("scope IN ?", []string{ScopeNodeProtocol, ScopeWireGuardPeer}).Delete(&model.ProtocolSecret{}).Error)
		missing := f.read(t, db)
		require.Contains(t, missing.Reality, Placeholder)
		require.NotContains(t, missing.Reality, "fake-bypass")
		require.Equal(t, Placeholder, missing.Custom)
		require.Equal(t, []string{"", ""}, missing.PeerPrivate)

		var body strings.Builder
		WritePrometheus(&body)
		require.Contains(t, body.String(), "# TYPE anixops_node_secrets_invalid_total counter\n")
		require.NotContains(t, body.String(), "fallback")
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
