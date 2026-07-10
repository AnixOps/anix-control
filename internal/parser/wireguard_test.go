package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/require"
)

func TestWireGuardFormatter_Format(t *testing.T) {
	f := &WireGuardFormatter{}
	node := wireGuardTestNode()

	output, err := f.Format([]*model.ParsedNode{node}, nil)
	require.NoError(t, err)

	content := string(output)
	require.Contains(t, content, "[Interface]")
	require.Contains(t, content, "PrivateKey = client-private")
	require.Contains(t, content, "Address = 10.66.0.2/32")
	require.Contains(t, content, "DNS = 1.1.1.1, 8.8.8.8")
	require.Contains(t, content, "[Peer]")
	require.Contains(t, content, "PublicKey = server-public")
	require.Contains(t, content, "PresharedKey = psk")
	require.Contains(t, content, "AllowedIPs = 0.0.0.0/0")
	require.Contains(t, content, "Endpoint = wg.example.com:51820")
}

func TestWireGuardFormatter_SkipsIncompleteNode(t *testing.T) {
	f := &WireGuardFormatter{}
	node := wireGuardTestNode()
	node.PublicKey = ""

	output, err := f.Format([]*model.ParsedNode{node}, nil)
	require.NoError(t, err)
	require.Equal(t, "", strings.TrimSpace(string(output)))
}

func TestWireGuardFormatterEmitsOneImportableProfile(t *testing.T) {
	f := &WireGuardFormatter{}
	nodes := []*model.ParsedNode{
		{Type: "wireguard", Name: "first", PrivateKey: "client-private-1", PublicKey: "server-public-1", PeerIP: "10.66.0.2", Server: "first.example.com", Port: 51820},
		{Type: "wireguard", Name: "second", PrivateKey: "client-private-2", PublicKey: "server-public-2", PeerIP: "10.66.0.3", Server: "second.example.com", Port: 51820},
	}
	content, err := f.Format(nodes, nil)
	require.NoError(t, err)
	require.Contains(t, string(content), "Endpoint = first.example.com:51820")
	require.NotContains(t, string(content), "second.example.com")
	require.Equal(t, 1, strings.Count(string(content), "[Interface]"))
}

func TestSingBoxFormatter_BuildWireGuardEndpoint(t *testing.T) {
	f := &SingBoxFormatter{}
	output, err := f.Format([]*model.ParsedNode{wireGuardTestNode()}, nil)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(output, &config))
	endpoints := config["endpoints"].([]any)
	var wireguard map[string]any
	for _, item := range endpoints {
		endpoint := item.(map[string]any)
		if endpoint["type"] == "wireguard" {
			wireguard = endpoint
			break
		}
	}
	require.NotNil(t, wireguard)
	require.Equal(t, "client-private", wireguard["private_key"])
	require.Equal(t, false, wireguard["system"])
	require.Equal(t, []any{"10.66.0.2/32"}, wireguard["address"])
	peers := wireguard["peers"].([]any)
	require.Len(t, peers, 1)
	peer := peers[0].(map[string]any)
	require.Equal(t, "wg.example.com", peer["address"])
	require.Equal(t, float64(51820), peer["port"])
	require.Equal(t, "server-public", peer["public_key"])
	require.Equal(t, "psk", peer["pre_shared_key"])
	require.Equal(t, []any{"0.0.0.0/0"}, peer["allowed_ips"])
	require.Equal(t, float64(25), peer["persistent_keepalive_interval"])
}

func wireGuardTestNode() *model.ParsedNode {
	return &model.ParsedNode{
		ID:           "wg-1",
		Name:         "WireGuard HK",
		Type:         "wireguard",
		Server:       "wg.example.com",
		Port:         51820,
		PrivateKey:   "client-private",
		PublicKey:    "server-public",
		PresharedKey: "psk",
		PeerIP:       "10.66.0.2",
		AllowedIPs:   []string{"0.0.0.0/0"},
		DNS:          []string{"1.1.1.1", "8.8.8.8"},
		MTU:          1280,
		Settings:     map[string]any{},
	}
}
