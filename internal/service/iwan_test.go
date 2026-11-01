package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestIWANPassword(t *testing.T) {
	require.Equal(t, "c9a1e7b05b0e4a4b", IWANPassword("C9A1E7B0-5B0E-4A4B-9A54-2F3F1C1B0D11"))
	require.Empty(t, IWANPassword("short"))
}

func TestBuildNodeProtocolConfig_IWAN(t *testing.T) {
	node := &model.Node{ID: 1, Host: "iwan.example.com", CreatedAt: time.Now()}
	settings := `{"pool":"10.70.0.0/24","mtu":1380,"allow_encrypt":false,"fixed_addresses":{"7":"10.70.0.9"}}`
	protocol := &model.NodeProtocol{ID: 3, NodeID: 1, Type: model.ProtocolIWAN, Port: 4500, Settings: &settings}

	config := BuildNodeProtocolConfig(node, protocol)

	require.Equal(t, "iwan", config["type"])
	require.Equal(t, 4500, config["server_port"])
	require.Equal(t, "10.70.0.0/24", config["pool"])
	require.Equal(t, 1380, config["mtu"])
	require.Equal(t, "1.1.1.1", config["dns_upstream"])
	require.Equal(t, false, config["allow_encrypt"])
	require.Equal(t, false, config["allow_client_to_client"])
	require.Equal(t, map[string]string{"7": "10.70.0.9"}, config["fixed_addresses"])

	empty := BuildNodeProtocolConfig(node, &model.NodeProtocol{ID: 4, NodeID: 1, Type: model.ProtocolIWAN, Port: 4501})
	require.Equal(t, "10.66.0.0/16", empty["pool"])
	require.Equal(t, 1400, empty["mtu"])
	require.Equal(t, true, empty["allow_encrypt"])
}

func TestValidateIWANProtocol(t *testing.T) {
	valid := []string{
		``,
		`{}`,
		`{"pool":"10.66.0.0/16","mtu":1400,"dns_upstream":"9.9.9.9:53","allow_encrypt":true,"allow_client_to_client":false,"fixed_addresses":{"7":"10.66.0.9","8":"10.66.3.4"}}`,
		`{"pool":"192.168.50.0/30","fixed_addresses":{}}`,
	}
	for _, settings := range valid {
		settings := settings
		require.NoError(t, ValidateNodeProtocol(&model.NodeProtocol{Type: model.ProtocolIWAN, Settings: &settings}), settings)
	}
	invalid := []string{
		`[]`,
		`{"pool":"10.66.0.1/16"}`,
		`{"pool":"10.66.0.0/31"}`,
		`{"pool":"fd00::/64"}`,
		`{"pool":16}`,
		`{"mtu":500}`,
		`{"mtu":1601}`,
		`{"mtu":1400.5}`,
		`{"dns_upstream":"dns.google"}`,
		`{"dns_upstream":"0.0.0.0"}`,
		`{"allow_encrypt":"yes"}`,
		`{"fixed_addresses":["10.66.0.9"]}`,
		`{"fixed_addresses":{"alice":"10.66.0.9"}}`,
		`{"fixed_addresses":{"7":"10.66.0.1"}}`,
		`{"fixed_addresses":{"7":"10.66.255.255"}}`,
		`{"fixed_addresses":{"7":"10.67.0.9"}}`,
		`{"fixed_addresses":{"7":"10.66.0.9","8":"10.66.0.9"}}`,
	}
	for _, settings := range invalid {
		settings := settings
		err := ValidateNodeProtocol(&model.NodeProtocol{Type: model.ProtocolIWAN, Settings: &settings})
		require.ErrorIs(t, err, ErrInvalidNodeProtocol, settings)
	}
}

func TestApplyIWANUser(t *testing.T) {
	parsed := &model.ParsedNode{
		Type:     "iwan",
		Settings: map[string]any{"mtu": float64(1300), "fixed_addresses": map[string]any{"7": "10.66.0.9", "8": "10.66.0.10"}},
	}
	applyIWANUser(parsed, &model.TemplateRenderContext{UserID: 7, UUID: "c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"})
	require.Equal(t, "7", parsed.Username)
	require.Equal(t, "c9a1e7b05b0e4a4b", parsed.Password)
	require.Equal(t, "10.66.0.9", parsed.PeerIP)
	require.Equal(t, 1300, parsed.MTU)
	require.NotContains(t, parsed.Settings, "fixed_addresses")
}

func TestPreferNativeIWANFormat(t *testing.T) {
	onlyIWAN := []*model.ParsedNode{{Type: "iwan"}, {Type: "iwan"}}
	require.Equal(t, model.FormatIWAN, preferNativeWireGuardFormat(model.FormatV2Ray, onlyIWAN))
	mixed := []*model.ParsedNode{{Type: "iwan"}, {Type: string(model.ProtocolWireGuard)}}
	require.Equal(t, model.FormatV2Ray, preferNativeWireGuardFormat(model.FormatV2Ray, mixed))
}

func TestProtocolIWANTemplate(t *testing.T) {
	for _, template := range model.GetProtocolTemplates() {
		if template.Type == model.ProtocolIWAN {
			settings := template.Settings
			require.NoError(t, ValidateNodeProtocol(&model.NodeProtocol{Type: model.ProtocolIWAN, Settings: &settings}))
			return
		}
	}
	require.Fail(t, "iwan protocol template missing")
}
