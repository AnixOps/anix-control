package grpc

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestSelectNodeProtocolForRequestRequiresExactRequestedType(t *testing.T) {
	protocols := []model.NodeProtocol{
		{Type: model.ProtocolVLESS, Enable: 1},
		{Type: model.ProtocolWireGuard, Enable: 1},
	}

	require.Equal(t, model.ProtocolWireGuard, selectNodeProtocolForRequest(protocols, "wireguard").Type)
	require.Nil(t, selectNodeProtocolForRequest(protocols, "trojan"))
	require.Error(t, requireRequestedProtocol("trojan"))
	require.NoError(t, requireRequestedProtocol(""))
}

// A WireGuard node terminates its users and egresses itself: no relay (it
// ran on gost) reaches the Agent.
func TestFillNodeConfigResponseSendsWireGuardWithoutARelay(t *testing.T) {
	settings := `{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","server_private_key":"server-private","server_public_key":"server-public"}`
	node := &model.Node{ID: 1, Host: "entry.example.com"}
	protocol := &model.NodeProtocol{Type: model.ProtocolWireGuard, Port: 51820, Settings: &settings}

	resp, err := fillNodeConfigResponse(node, protocol)
	require.NoError(t, err)
	require.Equal(t, "wireguard", resp.Type)
	require.NotContains(t, resp.Extra, "relay")
	require.NotContains(t, resp.Extra, "tunnel_type")
	require.Equal(t, "10.66.0.0/24", resp.Extra["cidr"])
}
