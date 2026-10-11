package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
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
