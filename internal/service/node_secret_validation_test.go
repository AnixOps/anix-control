package service

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/stretchr/testify/require"
)

func protocolFinding(protocol *model.NodeProtocol, column, field, reason string) NodeSecretFinding {
	return NodeSecretFinding{
		NodeID: protocol.NodeID, ProtocolID: protocol.ID, Type: strings.ToLower(string(protocol.Type)), Enabled: protocol.Enable != 0,
		Column: column, Field: field, Reason: reason,
	}
}

func TestValidateProtocolSecrets(t *testing.T) {
	private, public, err := generateWireGuardKeypair()
	require.NoError(t, err)
	_, otherPublic, err := generateWireGuardKeypair()
	require.NoError(t, err)
	realityKey := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	key16 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))
	key32 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))

	for _, c := range []struct {
		name     string
		protocol model.NodeProtocol
		want     [][3]string // column, field, reason
	}{
		{name: "nothing secret", protocol: model.NodeProtocol{Type: model.ProtocolVMess, Settings: textPtr(`{"flow":""}`)}},
		{name: "a WireGuard entry with a valid pair", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"server_private_key":"` + private + `","server_public_key":"` + public + `"}`)}},
		{name: "a WireGuard entry with a placeholder key", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"server_private_key":"********"}`)},
			want: [][3]string{{"settings", "/server_private_key", NodeSecretReasonPlaceholder}}},
		{name: "a WireGuard entry with a malformed key", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"server_private_key":"fake-not-a-key"}`)},
			want: [][3]string{{"settings", "/server_private_key", NodeSecretReasonWireGuardKey}}},
		{name: "an enabled WireGuard entry without a key", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"cidr":"10.66.0.0/24"}`)},
			want: [][3]string{{"settings", "/server_private_key", NodeSecretReasonWireGuardKey}}},
		{name: "a disabled WireGuard entry without a key", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard,
			Settings: textPtr(`{"cidr":"10.66.0.0/24"}`)}},
		{name: "a WireGuard entry whose public key is another's", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"server_private_key":"` + private + `","public_key":"` + otherPublic + `"}`)},
			want: [][3]string{{"settings", "/public_key", NodeSecretReasonWireGuardKeyPair}}},
		{name: "a WireGuard exit holds no key", protocol: model.NodeProtocol{Type: model.ProtocolWireGuard, Enable: 1,
			Settings: textPtr(`{"relay":{"role":"exit"}}`)}},
		{name: "a Reality key", protocol: model.NodeProtocol{Type: model.ProtocolVLESS, TLS: 2,
			RealitySettings: textPtr(`{"private_key":"` + realityKey + `","short_id":"6ba8"}`)}},
		{name: "a malformed Reality key", protocol: model.NodeProtocol{Type: model.ProtocolVLESS, TLS: 2,
			RealitySettings: textPtr(`{"privateKey":"fake-short"}`)},
			want: [][3]string{{"reality_settings", "/privateKey", NodeSecretReasonRealityKey}}},
		{name: "a Reality key masked", protocol: model.NodeProtocol{Type: model.ProtocolVLESS, TLS: 2,
			RealitySettings: textPtr(`{"private_key":"********"}`)},
			want: [][3]string{{"reality_settings", "/private_key", NodeSecretReasonPlaceholder}}},
		{name: "Shadowsocks 2022 keys of the cipher's length", protocol: model.NodeProtocol{Type: model.ProtocolShadowsocks,
			Settings: textPtr(`{"cipher":"2022-blake3-aes-128-gcm","server_key":"` + key16 + `"}`)}},
		{name: "a Shadowsocks 2022 key of another length", protocol: model.NodeProtocol{Type: model.ProtocolShadowsocks,
			Settings: textPtr(`{"method":"2022-blake3-aes-128-gcm","server_key":"` + key32 + `"}`)},
			want: [][3]string{{"settings", "/server_key", NodeSecretReasonSS2022Key}}},
		{name: "a derived Shadowsocks 2022 key", protocol: model.NodeProtocol{Type: model.ProtocolShadowsocks,
			Settings: textPtr(`{"cipher":"2022-blake3-aes-256-gcm"}`)}},
		{name: "a classic Shadowsocks password", protocol: model.NodeProtocol{Type: model.ProtocolShadowsocks,
			Settings: textPtr(`{"cipher":"aes-128-gcm","server_key":"fake-anything"}`)}},
		{name: "tombstones and masks anywhere", protocol: model.NodeProtocol{Type: model.ProtocolTrojan,
			TLSSettings: textPtr(`{"key":"!moved:4"}`), TransportSettings: textPtr(`{"headers":{"token":"********"}}`), CustomConfig: textPtr(`********`)},
			want: [][3]string{
				{"tls_settings", "/key", NodeSecretReasonTombstone},
				{"transport_settings", "/headers/token", NodeSecretReasonPlaceholder},
				{"custom_config", "", NodeSecretReasonPlaceholder},
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			protocol := c.protocol
			protocol.ID, protocol.NodeID = 7, 3
			var want []NodeSecretFinding
			for _, w := range c.want {
				want = append(want, protocolFinding(&protocol, w[0], w[1], w[2]))
			}
			require.Equal(t, want, ValidateProtocolSecrets(&protocol))
		})
	}
	require.Nil(t, ValidateProtocolSecrets(nil))
}

func TestValidateRawConfigSecretsAndReport(t *testing.T) {
	raw := `{"server_port":51820,"wireguard":{"private_key":"********","preshared_key":"!moved:3","public_key":"fake-public"}}`
	node := &model.Node{ID: 3, RawConfig: &raw}
	findings := ValidateRawConfigSecrets(node)
	require.Equal(t, []NodeSecretFinding{
		{NodeID: 3, Type: "raw_config", Enabled: true, Column: "raw_config", Field: "/wireguard/preshared_key", Reason: NodeSecretReasonTombstone},
		{NodeID: 3, Type: "raw_config", Enabled: true, Column: "raw_config", Field: "/wireguard/private_key", Reason: NodeSecretReasonPlaceholder},
	}, findings)
	require.Nil(t, ValidateRawConfigSecrets(&model.Node{ID: 4}))

	// Reported: counted every time, logged once per subject, never a value.
	nodesecrets.ForgetLogged()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	before := nodesecrets.InvalidCount(nodesecrets.TableNode, NodeSecretReasonPlaceholder)
	ReportNodeSecretFindings(findings)
	ReportNodeSecretFindings(findings)
	require.EqualValues(t, 2, nodesecrets.InvalidCount(nodesecrets.TableNode, NodeSecretReasonPlaceholder)-before)
	require.Equal(t, 1, strings.Count(logs.String(), `subject="node 3 raw_config /wireguard/private_key"`), logs.String())
	require.NotContains(t, logs.String(), "fake-")
}

func textPtr(value string) *string { return &value }
