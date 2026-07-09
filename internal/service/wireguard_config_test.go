package service

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildNodeProtocolConfig_WireGuardDefaults(t *testing.T) {
	settings := `{"cidr":"10.88.0.0/24","server_address":"10.88.0.1/24","server_private_key":"server-private","server_public_key":"server-public"}`
	host := "entry.example.com"
	node := &model.Node{
		ID:        1,
		Name:      "Entry",
		Host:      "node.example.com",
		CreatedAt: time.Now(),
	}
	protocol := &model.NodeProtocol{
		ID:        10,
		NodeID:    1,
		Name:      "WG",
		Type:      model.ProtocolWireGuard,
		Port:      51820,
		Host:      &host,
		Settings:  &settings,
		Transport: stringPtr("udp"),
	}

	config := BuildNodeProtocolConfig(node, protocol)

	require.Equal(t, "wireguard", config["type"])
	require.Equal(t, "wireguard", config["node_type"])
	require.Equal(t, 51820, config["server_port"])
	require.Equal(t, "entry.example.com", config["host"])
	require.Equal(t, "10.88.0.0/24", config["cidr"])
	require.Equal(t, "10.88.0.1/24", config["server_address"])
	require.Equal(t, "server-private", config["server_private_key"])
	require.Equal(t, "server-public", config["server_public_key"])
	require.Equal(t, 1280, config["mtu"])
	require.Equal(t, "quic", config["tunnel_type"])
	require.Equal(t, "udp", config["network"])
}

func TestProtocolWireGuardTemplate(t *testing.T) {
	templates := model.GetProtocolTemplates()
	for _, template := range templates {
		if template.Type == model.ProtocolWireGuard {
			require.Equal(t, 51820, template.DefaultPort)
			require.Contains(t, template.Settings, `"tunnel_type":"quic"`)
			return
		}
	}
	require.Fail(t, "wireguard protocol template missing")
}

func TestWireGuardPeerAllocationAndKeyGeneration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))

	svc := &SubscriptionService{db: db}
	first, err := svc.getOrCreateWireGuardPeer(10, 100, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, "10.99.0.2", first.PeerIP)

	again, err := svc.getOrCreateWireGuardPeer(10, 100, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.PrivateKey, again.PrivateKey)
	require.Equal(t, first.PeerIP, again.PeerIP)

	second, err := svc.getOrCreateWireGuardPeer(10, 101, "10.99.0.0/29")
	require.NoError(t, err)
	require.Equal(t, "10.99.0.3", second.PeerIP)
	require.NotEqual(t, first.PublicKey, second.PublicKey)

	requireBase64KeyLen(t, first.PrivateKey, 32)
	requireBase64KeyLen(t, first.PublicKey, 32)
	requireBase64KeyLen(t, first.PresharedKey, 32)
}

func requireBase64KeyLen(t *testing.T, key string, want int) {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(key)
	require.NoError(t, err)
	require.Len(t, decoded, want)
}

func stringPtr(v string) *string {
	return &v
}
