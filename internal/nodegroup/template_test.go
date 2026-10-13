package nodegroup

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplateNormalizes(t *testing.T) {
	entries, err := ParseTemplate(json.RawMessage(`[
		{"name": "VLESS Reality", "type": "VLESS", "port": 443, "tls": "reality",
		 "reality": {"server_name": "www.example.com"}, "settings": {"flow": "xtls-rprx-vision"}},
		{"type": "shadowsocks", "port_range": [20000, 20010], "settings": {"method": "2022-blake3-aes-128-gcm"}},
		{"type": "vmess", "port_range": [30000, 30002], "transport": "ws", "transport_settings": {"path": "/v"}}
	]`))
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "vless", entries[0].Type)
	assert.Equal(t, TemplateTLSReality, entries[0].TLS)
	assert.Equal(t, "tcp", entries[0].Transport)
	assert.Equal(t, "www.example.com:443", entries[0].Reality.Dest, "dest defaults to the server name on 443")
	assert.Equal(t, "chrome", entries[0].Reality.Fingerprint)
	assert.Equal(t, "shadowsocks", entries[1].Name, "the name defaults to the type")
	assert.Equal(t, TemplateTLSNone, entries[1].TLS)
	assert.JSONEq(t, `{"path":"/v"}`, string(entries[2].TransportSettings))

	empty, err := ParseTemplate(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	empty, err = ParseTemplate(json.RawMessage(`null`))
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestParseTemplateRefuses(t *testing.T) {
	for name, document := range map[string]string{
		"not a list":           `{"type":"vless"}`,
		"unknown field":        `[{"type":"vless","port":443,"tls":"none","cert":"x"}]`,
		"unsupported type":     `[{"type":"trojan","port":443}]`,
		"needs a certificate":  `[{"type":"hysteria2","port":443}]`,
		"acme":                 `[{"type":"vless","port":443,"tls":"acme"}]`,
		"self signed":          `[{"type":"vless","port":443,"tls":"self_signed"}]`,
		"reality on vmess":     `[{"type":"vmess","port":443,"tls":"reality","reality":{"server_name":"a.example"}}]`,
		"reality on ws":        `[{"type":"vless","port":443,"tls":"reality","transport":"ws","reality":{"server_name":"a.example"}}]`,
		"reality without sni":  `[{"type":"vless","port":443,"tls":"reality"}]`,
		"bad server name":      `[{"type":"vless","port":443,"tls":"reality","reality":{"server_name":"a b"}}]`,
		"bad dest":             `[{"type":"vless","port":443,"tls":"reality","reality":{"server_name":"a.example","dest":"a.example"}}]`,
		"reality without tls":  `[{"type":"vless","port":443,"reality":{"server_name":"a.example"}}]`,
		"no port":              `[{"type":"vless"}]`,
		"port and range":       `[{"type":"vless","port":443,"port_range":[1000,2000]}]`,
		"port out of range":    `[{"type":"vless","port":70000}]`,
		"reversed range":       `[{"type":"vless","port_range":[2000,1000]}]`,
		"range of one":         `[{"type":"vless","port_range":[2000]}]`,
		"duplicate port":       `[{"type":"vless","port":443},{"type":"vmess","port":443}]`,
		"duplicate name":       `[{"name":"a","type":"vless","port":443},{"name":"a","type":"vmess","port":444}]`,
		"range too small":      `[{"type":"vless","port":443},{"type":"vmess","port_range":[1000,1001]},{"type":"shadowsocks","port":8388,"settings":{"method":"aes-128-gcm"}}]`,
		"transport":            `[{"type":"vless","port":443,"transport":"kcp"}]`,
		"settings not object":  `[{"type":"vless","port":443,"settings":[1]}]`,
		"shared reality key":   `[{"type":"vless","port":443,"settings":{"private_key":"x"}}]`,
		"shared ss key":        `[{"type":"shadowsocks","port":8388,"settings":{"method":"2022-blake3-aes-128-gcm","server_key":"x"}}]`,
		"shared password":      `[{"type":"vless","port":443,"transport_settings":{"password":"x"}}]`,
		"shadowsocks method":   `[{"type":"shadowsocks","port":8388}]`,
		"wireguard bad cidr":   `[{"type":"wireguard","port":51820,"settings":{"cidr":"nonsense"}}]`,
		"control in name":      `[{"name":"a\u0001","type":"vless","port":443}]`,
		"seventeen protocols":  `[` + repeatEntries(17) + `]`,
		"two documents":        `[] []`,
		"trailing garbage":     `[{"type":"vless","port":443}] x`,
		"tls on shadowsocks":   `[{"type":"shadowsocks","port":8388,"tls":"reality","settings":{"method":"aes-128-gcm"}}]`,
		"transport on ss":      `[{"type":"shadowsocks","port":8388,"transport":"ws","settings":{"method":"aes-128-gcm"}}]`,
		"empty reality object": `[{"type":"vless","port":443,"tls":"reality","reality":{}}]`,
	} {
		_, err := ParseTemplate(json.RawMessage(document))
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
}

func repeatEntries(n int) string {
	entries := make([]string, n)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"name":"p%d","type":"vmess","port":%d}`, i, 10000+i)
	}
	return strings.Join(entries, ",")
}

func TestAssignPortsKeepsPortsDistinct(t *testing.T) {
	entries := []TemplateEntry{
		{Name: "fixed", Port: 20001},
		{Name: "a", PortRange: []int{20000, 20002}},
		{Name: "b", PortRange: []int{20000, 20002}},
	}
	for i := 0; i < 50; i++ {
		ports, err := assignPorts(entries)
		require.NoError(t, err)
		assert.Equal(t, 20001, ports[0])
		assert.ElementsMatch(t, []int{20000, 20001, 20002}, ports, "the range has exactly the free ports")
	}
	_, err := assignPorts([]TemplateEntry{{Name: "x", Port: 1}, {Name: "y", PortRange: []int{1, 1}}})
	assert.Error(t, err, "a full range is an error, never a duplicate port")
}
