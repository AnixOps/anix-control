package parser

import (
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/require"
)

func TestLoonFormatterWireGuardIPv6Endpoint(t *testing.T) {
	f := &LoonFormatter{}
	node := wireGuardTestNode()
	node.Name = "WireGuard CN Entry"
	node.Server = "[2001:db8::10]"
	node.DNS = []string{"1.1.1.1", "2606:4700:4700::1111"}

	output, err := f.Format([]*model.ParsedNode{node}, nil)
	require.NoError(t, err)
	content := string(output)
	require.Contains(t, content, "WireGuard CN Entry = wireguard,interface-ip=10.66.0.2")
	require.Contains(t, content, `private-key="client-private"`)
	require.Contains(t, content, `public-key="server-public"`)
	require.Contains(t, content, `preshared-key="psk"`)
	require.Contains(t, content, `allowed-ips="0.0.0.0/0"`)
	require.Contains(t, content, "endpoint=[2001:db8::10]:51820")
	require.Contains(t, content, "dns=1.1.1.1")
	require.Contains(t, content, "dnsV6=2606:4700:4700::1111")
	require.Contains(t, content, "keeyalive=25")
}

func TestLoonFormatterUsesOnlyNativeSupportedNodeSyntax(t *testing.T) {
	f := &LoonFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "user-uuid"}
	nodes := []*model.ParsedNode{
		{
			Name:              "Reality",
			Type:              "vless",
			Server:            "reality.example.com",
			Port:              443,
			TLSMode:           2,
			Flow:              "xtls-rprx-vision",
			RealityPublicKey:  "reality-public-key",
			RealityShortID:    "0123456789abcdef",
			ServerName:        "www.example.com",
			SkipCertVerify:    true,
			Transport:         "tcp",
			TransportSettings: map[string]any{},
		},
		{Name: "Unsupported TUIC", Type: "tuic", Server: "tuic.example.com", Port: 443, UUID: "tuic-uuid"},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	content := string(output)
	require.Contains(t, content, `Reality = VLESS,reality.example.com,443,"user-uuid"`)
	require.Contains(t, content, "flow=xtls-rprx-vision")
	require.Contains(t, content, `public-key="reality-public-key"`)
	require.Contains(t, content, "short-id=0123456789abcdef")
	require.Contains(t, content, "over-tls=true")
	require.NotContains(t, content, "Unsupported TUIC")
	require.NotContains(t, content, "vless://")
}

func TestLoonFormatterDoesNotReuseOtherClientFormats(t *testing.T) {
	f := &LoonFormatter{}
	output, err := f.Format([]*model.ParsedNode{{
		Name:      "Trojan Node",
		Type:      "trojan",
		Server:    "trojan.example.com",
		Port:      443,
		Password:  `p,ass"word`,
		Transport: "ws",
		TransportSettings: map[string]any{
			"path": "/ws",
			"host": "cdn.example.com",
		},
		ServerName: "tls.example.com",
	}}, nil)
	require.NoError(t, err)
	content := string(output)
	require.Contains(t, content, `Trojan Node = trojan,trojan.example.com,443,"p,ass\"word"`)
	require.Contains(t, content, `transport=ws,path="/ws",host="cdn.example.com"`)
	require.NotContains(t, content, "trojan://")
	require.NotContains(t, content, "proxies:")
}
