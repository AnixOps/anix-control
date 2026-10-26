package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStripServerPrivateKeysKeepsClientCredentials(t *testing.T) {
	settings := map[string]any{
		"server_private_key": "a",
		"private_key":        "b",
		"privateKey":         "c",
		"server_key":         "keep-1",
		"obfs-password":      "keep-2",
		"password":           "keep-3",
		"server_public_key":  "keep-4",
		"peer_public_key":    "keep-5",
		"cidr":               "keep-6",
	}
	stripServerPrivateKeys(settings)

	for _, gone := range []string{"server_private_key", "private_key", "privateKey"} {
		require.NotContains(t, settings, gone)
	}
	for _, kept := range []string{"server_key", "obfs-password", "password", "server_public_key", "peer_public_key", "cidr"} {
		require.Contains(t, settings, kept)
	}
}

// A subscriber's JSON must never contain the node's WireGuard server private key.
// The formatter marshals the same ParsedNode, so marshalling it here covers that output.
func TestWireGuardSubscriptionNeverCarriesServerPrivateKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))
	svc := &SubscriptionService{db: db}

	serverPrivate, serverPublic, err := generateWireGuardKeypair()
	require.NoError(t, err)
	settings := fmt.Sprintf(`{"cidr":"10.77.0.0/24","server_private_key":%q}`, serverPrivate)
	node := &model.Node{ID: 7, Name: "Entry", Host: "entry.example.com", CreatedAt: time.Now()}
	protocol := &model.NodeProtocol{ID: 70, NodeID: 7, Name: "WG", Type: model.ProtocolWireGuard, Port: 51820, Settings: &settings}
	ctx := &model.TemplateRenderContext{UserID: 701, UUID: "uuid-701"}

	parsed := svc.nodeProtocolToParsedNode(node, protocol, ctx)
	require.NotNil(t, parsed)
	require.Equal(t, serverPublic, parsed.PublicKey, "the client still gets the server public key")
	require.NotEmpty(t, parsed.PrivateKey, "the subscriber keeps its own peer key")
	require.NotEqual(t, serverPrivate, parsed.PrivateKey)

	body, err := json.Marshal(parsed)
	require.NoError(t, err)
	require.NotContains(t, string(body), serverPrivate)
	require.NotContains(t, string(body), "server_private_key")
}

// A VLESS Reality protocol reaches every client format with its flow, its SNI
// and a uTLS fingerprint; a client without the flow is rejected by an inbound
// that requires xtls-rprx-vision.
func TestVLESSRealityProtocolCarriesFlowSNIAndFingerprint(t *testing.T) {
	svc := &SubscriptionService{}
	settings := `{"flow":"xtls-rprx-vision"}`
	reality := `{"dest":"www.example.com:443","private_key":"server-secret","public_key":"pbk-1","server_name":"www.example.com","short_id":"ab12"}`
	node := &model.Node{ID: 57, Name: "DE", Host: "de.example.com", CreatedAt: time.Now()}
	protocol := &model.NodeProtocol{ID: 41, NodeID: 57, Name: "Reality", Type: model.ProtocolVLESS, Port: 443, TLS: 2,
		Settings: &settings, RealitySettings: &reality}
	ctx := &model.TemplateRenderContext{UserID: 1, UUID: "uuid-1"}

	parsed := svc.nodeProtocolToParsedNode(node, protocol, ctx)
	require.Equal(t, "xtls-rprx-vision", parsed.Flow)
	require.Equal(t, "www.example.com", parsed.ServerName, "the SNI falls back to the Reality server name")
	require.Equal(t, "chrome", parsed.TLSFingerprint)
	require.Equal(t, "pbk-1", parsed.RealityPublicKey)

	tlsSettings := `{"server_name":"cdn.example.com","fingerprint":"firefox"}`
	protocol.TLSSettings = &tlsSettings
	parsed = svc.nodeProtocolToParsedNode(node, protocol, ctx)
	require.Equal(t, "cdn.example.com", parsed.ServerName)
	require.Equal(t, "firefox", parsed.TLSFingerprint)
}
