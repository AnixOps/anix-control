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
	require.Contains(t, content, "AllowedIPs = 0.0.0.0/0, ::/0")
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

func TestSingBoxFormatter_BuildWireGuardOutbound(t *testing.T) {
	f := &SingBoxFormatter{}
	output, err := f.Format([]*model.ParsedNode{wireGuardTestNode()}, nil)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(output, &config))
	outbounds := config["outbounds"].([]any)
	var wireguard map[string]any
	for _, item := range outbounds {
		outbound := item.(map[string]any)
		if outbound["type"] == "wireguard" {
			wireguard = outbound
			break
		}
	}
	require.NotNil(t, wireguard)
	require.Equal(t, "wg.example.com", wireguard["server"])
	require.Equal(t, float64(51820), wireguard["server_port"])
	require.Equal(t, "client-private", wireguard["private_key"])
	require.Equal(t, "server-public", wireguard["peer_public_key"])
	require.Equal(t, "psk", wireguard["pre_shared_key"])
	require.Equal(t, []any{"10.66.0.2/32"}, wireguard["local_address"])
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
		AllowedIPs:   []string{"0.0.0.0/0", "::/0"},
		DNS:          []string{"1.1.1.1", "8.8.8.8"},
		MTU:          1280,
		Settings:     map[string]any{},
	}
}
