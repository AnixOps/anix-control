package service

import (
	"crypto/ecdh"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func realityKeys(t *testing.T, protocol *model.NodeProtocol) (string, string) {
	t.Helper()
	require.NotNil(t, protocol.RealitySettings)
	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(*protocol.RealitySettings), &settings))
	private, _ := settings["private_key"].(string)
	public, _ := settings["public_key"].(string)
	return private, public
}

func assertRealityKeyPair(t *testing.T, private, public string) {
	t.Helper()
	decoded, ok := decodeBase64Key(private)
	require.True(t, ok, "private key %q", private)
	key, err := ecdh.X25519().NewPrivateKey(decoded)
	require.NoError(t, err)
	publicDecoded, ok := decodeBase64Key(public)
	require.True(t, ok, "public key %q", public)
	assert.Equal(t, key.PublicKey().Bytes(), publicDecoded)
}

func TestEnsureRealityKeyPair(t *testing.T) {
	settings := `{"dest":"gateway.icloud.com:443","server_name":"gateway.icloud.com","short_id":"6ba85179e30d4fc2"}`
	protocol := &model.NodeProtocol{TLS: 2, RealitySettings: &settings}
	changed, err := ensureRealityKeyPair(protocol)
	require.NoError(t, err)
	assert.True(t, changed)
	private, public := realityKeys(t, protocol)
	assertRealityKeyPair(t, private, public)
	assert.Contains(t, *protocol.RealitySettings, `"short_id":"6ba85179e30d4fc2"`)

	// A complete pair is kept.
	changed, err = ensureRealityKeyPair(protocol)
	require.NoError(t, err)
	assert.False(t, changed)

	// A private key without its public key gets it.
	onlyPrivate := `{"private_key":"` + private + `"}`
	protocol = &model.NodeProtocol{TLS: 2, RealitySettings: &onlyPrivate}
	changed, err = ensureRealityKeyPair(protocol)
	require.NoError(t, err)
	assert.True(t, changed)
	gotPrivate, gotPublic := realityKeys(t, protocol)
	assert.Equal(t, private, gotPrivate)
	assert.Equal(t, public, gotPublic)

	// Not Reality, or a private key that does not decode: left alone.
	for _, protocol := range []*model.NodeProtocol{
		{TLS: 1, RealitySettings: &settings},
		{TLS: 2, RealitySettings: ptrString(`{"private_key":"not a key!"}`)},
		{TLS: 2, RealitySettings: ptrString(`[]`)},
	} {
		before := *protocol.RealitySettings
		changed, err := ensureRealityKeyPair(protocol)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, before, *protocol.RealitySettings)
	}
}

func (s *NodeServiceTestSuite) TestRealityProtocolGetsAKeyPair() {
	t := s.T()
	node := &model.Node{Name: "Reality Keys", Host: "reality-keys.example.test", Port: 443, Rate: 1.0, Show: 1}
	require.NoError(t, s.svc.CreateNode(node))
	stored := func(id uint) *model.NodeProtocol {
		var protocol model.NodeProtocol
		require.NoError(t, s.svc.db.First(&protocol, id).Error)
		nodesecrets.ResolveProtocol(s.svc.db, &protocol)
		return &protocol
	}

	// Created without a private key: one is generated.
	settings := `{"dest":"gateway.icloud.com:443","server_name":"gateway.icloud.com"}`
	protocol := &model.NodeProtocol{NodeID: node.ID, Name: "Reality", Type: model.ProtocolVLESS, Port: 8443, Enable: 1, Show: 1, TLS: 2, RealitySettings: &settings}
	require.NoError(t, s.svc.CreateProtocol(protocol))
	private, public := realityKeys(t, stored(protocol.ID))
	assertRealityKeyPair(t, private, public)

	// Updated to Reality without a private key: one is generated.
	plain := &model.NodeProtocol{NodeID: node.ID, Name: "Plain", Type: model.ProtocolVLESS, Port: 9443, Enable: 1, Show: 1}
	require.NoError(t, s.svc.CreateProtocol(plain))
	require.NoError(t, s.svc.UpdateProtocol(plain.ID, map[string]any{"tls": 2, "reality_settings": map[string]any{"dest": "www.apple.com:443", "server_name": "www.apple.com"}}))
	updatedPrivate, updatedPublic := realityKeys(t, stored(plain.ID))
	assertRealityKeyPair(t, updatedPrivate, updatedPublic)
	assert.NotEqual(t, private, updatedPrivate)

	// An update that does not touch the key keeps it.
	require.NoError(t, s.svc.UpdateProtocol(protocol.ID, map[string]any{"port": 443}))
	keptPrivate, keptPublic := realityKeys(t, stored(protocol.ID))
	assert.Equal(t, private, keptPrivate)
	assert.Equal(t, public, keptPublic)
}
