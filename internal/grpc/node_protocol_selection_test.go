package grpc

import (
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/model"
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

func TestFillNodeConfigResponsePreservesWireGuardWSSVerificationContract(t *testing.T) {
	settings := `{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","server_private_key":"server-private","server_public_key":"server-public","tunnel_type":"wss","relay":{"backend":"gost","mode":"relay+wss","role":"entry","wss_compat":true,"wss_path":"/wireguard","wss_secure":true,"wss_server_name":"exit.example.com","wss_ca_file":"/etc/v2bx/relay-ca.pem","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.66.2/24","exit_tun_address":"172.31.66.1/24"}}`
	node := &model.Node{ID: 1, Host: "entry.example.com"}
	protocol := &model.NodeProtocol{Type: model.ProtocolWireGuard, Port: 51820, Settings: &settings}

	resp, err := fillNodeConfigResponse(node, protocol)
	require.NoError(t, err)
	require.Equal(t, "wireguard", resp.Type)
	require.Equal(t, "wss", resp.Extra["tunnel_type"])

	var relay map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Extra["relay"]), &relay))
	require.Equal(t, "entry", relay["role"])
	require.Equal(t, "/wireguard", relay["wss_path"])
	require.Equal(t, true, relay["wss_secure"])
	require.Equal(t, "exit.example.com", relay["wss_server_name"])
	require.Equal(t, "/etc/v2bx/relay-ca.pem", relay["wss_ca_file"])
	require.Empty(t, relay["wss_cert_file"])
	require.Empty(t, relay["wss_key_file"])
}
