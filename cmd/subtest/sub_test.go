package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVLESSLink_None(t *testing.T) {
	node := TestNode{
		Name:     "test-node",
		Type:     "vless",
		Server:   "127.0.0.1",
		Port:     443,
		UUID:     "test-uuid",
		Network:  "tcp",
	}
	link := generateVLESSLink(node, "test-uuid")

	assert.True(t, strings.HasPrefix(link, "vless://"))
	assert.Contains(t, link, "security=none")
	assert.Contains(t, link, "type=tcp")
	assert.Contains(t, link, "headerType=none")
	assert.Contains(t, link, "test-node")
}

func TestGenerateVLESSLink_Reality(t *testing.T) {
	node := TestNode{
		Name:        "reality-node",
		Type:        "vless",
		Server:      "us.example.com",
		Port:        443,
		UUID:        "{{UUID}}",
		Network:     "tcp",
		TLS:         true,
		ServerName:  "www.microsoft.com",
		Fingerprint: "chrome",
		Flow:        "xtls-rprx-vision",
		RealityOpts: &RealityOpts{
			PublicKey: "pub-key-123",
			ShortID:   "abcd1234",
		},
	}
	link := generateVLESSLink(node, "real-uuid")

	assert.Contains(t, link, "security=reality")
	assert.Contains(t, link, "pbk=pub-key-123")
	assert.Contains(t, link, "sid=abcd1234")
	assert.Contains(t, link, "sni=www.microsoft.com")
	assert.Contains(t, link, "fp=chrome")
	assert.Contains(t, link, "flow=xtls-rprx-vision")
	assert.NotContains(t, link, "{{UUID}}")
}

func TestGenerateVLESSLink_TLS_WS(t *testing.T) {
	node := TestNode{
		Name:       "ws-node",
		Type:       "vless",
		Server:     "jp.example.com",
		Port:       443,
		UUID:       "uuid",
		Network:    "ws",
		TLS:        true,
		ServerName: "jp.example.com",
		WSOpts: &WSOpts{
			Path: "/ws",
			Headers: map[string]string{
				"Host": "jp.example.com",
			},
		},
	}
	link := generateVLESSLink(node, "uuid")

	assert.Contains(t, link, "security=tls")
	assert.Contains(t, link, "type=ws")
	assert.Contains(t, link, "path=/ws")
	assert.Contains(t, link, "host=jp.example.com")
}

func TestGenerateVLESSLink_GRPC(t *testing.T) {
	node := TestNode{
		Name:     "grpc-node",
		Type:     "vless",
		Server:   "grpc.example.com",
		Port:     443,
		UUID:     "uuid",
		Network:  "grpc",
		GRPCOpts: &GRPCOpts{ServiceName: "my-service"},
	}
	link := generateVLESSLink(node, "uuid")

	assert.Contains(t, link, "type=grpc")
	assert.Contains(t, link, "serviceName=my-service")
}

func TestGenerateVMessLink_NoTLS(t *testing.T) {
	node := TestNode{
		Name:    "vmess-node",
		Type:    "vmess",
		Server:  "127.0.0.1",
		Port:    8080,
		UUID:    "uuid",
		Network: "tcp",
	}
	link := generateVMessLink(node, "uuid")

	assert.True(t, strings.HasPrefix(link, "vmess://"))

	encoded := strings.TrimPrefix(link, "vmess://")
	data, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))

	assert.Equal(t, "vmess-node", cfg["ps"])
	assert.Equal(t, "127.0.0.1", cfg["add"])
	assert.Equal(t, "uuid", cfg["id"])
	assert.Equal(t, "", cfg["tls"])
}

func TestGenerateVMessLink_WithTLS(t *testing.T) {
	node := TestNode{
		Name:       "vmess-tls",
		Type:       "vmess",
		Server:     "tls.example.com",
		Port:       443,
		UUID:       "uuid",
		Network:    "ws",
		TLS:        true,
		ServerName: "tls.example.com",
		WSOpts: &WSOpts{
			Path: "/path",
			Headers: map[string]string{"Host": "tls.example.com"},
		},
	}
	link := generateVMessLink(node, "uuid")

	encoded := strings.TrimPrefix(link, "vmess://")
	data, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))

	assert.Equal(t, "tls", cfg["tls"])
	assert.Equal(t, "tls.example.com", cfg["sni"])
	assert.Equal(t, "/path", cfg["path"])
	assert.Equal(t, "tls.example.com", cfg["host"])
}

func TestGenerateTrojanLink(t *testing.T) {
	node := TestNode{
		Name:       "trojan-node",
		Type:       "trojan",
		Server:     "trojan.example.com",
		Port:       443,
		UUID:       "trojan-uuid",
		ServerName: "trojan.example.com",
	}
	link := generateTrojanLink(node, "trojan-uuid")

	assert.True(t, strings.HasPrefix(link, "trojan://"))
	assert.Contains(t, link, "trojan-uuid@trojan.example.com:443")
	assert.Contains(t, link, "sni=trojan.example.com")
	assert.Contains(t, link, "type=tcp")
}

func TestGenerateHysteria2Link(t *testing.T) {
	node := TestNode{
		Name:           "hy2-node",
		Type:           "hysteria2",
		Server:         "hk.example.com",
		Port:           443,
		UUID:           "hy-uuid",
		TLS:            true,
		ServerName:     "hk.example.com",
		SkipCertVerify: true,
	}
	link := generateHysteria2Link(node, "hy-uuid")

	assert.True(t, strings.HasPrefix(link, "hy2://"))
	assert.Contains(t, link, "sni=hk.example.com")
	assert.Contains(t, link, "insecure=1")
}

func TestGenerateV2RayLink_UnknownType(t *testing.T) {
	node := TestNode{
		Name:   "unknown",
		Type:   "unknown-protocol",
		Server: "127.0.0.1",
		Port:   1234,
		UUID:   "uuid",
	}
	link := generateV2RayLink(node, "uuid")
	assert.Equal(t, "", link)
}

func TestParseVLESSLink(t *testing.T) {
	link := "vless://test-uuid@127.0.0.1:443?security=none&type=tcp#test-node"
	// Just verify the function doesn't panic and prints expected fields
	// parseVLESSLink prints to stdout, so we just call it
	parseVLESSLink(link)
}

func TestParseVMessLink(t *testing.T) {
	cfg := map[string]any{
		"v":  "2", "ps": "test", "add": "127.0.0.1",
		"port": 8080, "id": "uuid", "aid": 0,
	}
	jsonData, _ := json.Marshal(cfg)
	encoded := base64.StdEncoding.EncodeToString(jsonData)
	link := "vmess://" + encoded

	parseVMessLink(link)
}

func TestParseTrojanLink(t *testing.T) {
	link := "trojan://uuid@host:443?sni=host#name"
	parseTrojanLink(link)
}

func TestParseBase64_Standard(t *testing.T) {
	original := "hello world"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))
	parseBase64(encoded)
}

func TestParseBase64_Invalid(t *testing.T) {
	parseBase64("!!!not-base64!!!")
}

func TestGenerateClashSubscription(t *testing.T) {
	config := TestConfig{
		Groups: map[string][]TestNode{
			"default": {
				{
					Name:     "node-a",
					Type:     "vless",
					Server:   "127.0.0.1",
					Port:     443,
					UUID:     "{{UUID}}",
					Network:  "tcp",
					TLS:      true,
					ServerName: "example.com",
				},
			},
		},
	}
	generateClashSubscription(config, "test-uuid")
}

func TestGenerateJSONSubscription(t *testing.T) {
	config := TestConfig{
		Groups: map[string][]TestNode{
			"default": {
				{
					Name:   "test",
					Type:   "vless",
					Server: "127.0.0.1",
					Port:   443,
					UUID:   "{{UUID}}",
				},
			},
		},
	}
	generateJSONSubscription(config, "test-uuid")
}

func TestGenerateBase64JSONSubscription(t *testing.T) {
	config := TestConfig{
		Groups: map[string][]TestNode{
			"default": {
				{
					Name:   "test",
					Type:   "vless",
					Server: "127.0.0.1",
					Port:   443,
					UUID:   "{{UUID}}",
				},
			},
		},
	}
	generateBase64JSONSubscription(config, "test-uuid")
}

func TestParseLink_VLESS(t *testing.T) {
	parseLink("vless://uuid@host:443?security=none&type=tcp#name")
}

func TestParseLink_VMess(t *testing.T) {
	cfg := map[string]any{"v": "2", "ps": "t", "add": "h", "port": 1, "id": "u", "aid": 0}
	data, _ := json.Marshal(cfg)
	encoded := base64.StdEncoding.EncodeToString(data)
	parseLink("vmess://" + encoded)
}

func TestParseLink_Unknown(t *testing.T) {
	parseLink("unknown://whatever")
}

func TestGenerateSampleConfig(t *testing.T) {
	generateSampleConfig()
	// Clean up
	_ = os.Remove("config/examples/test_nodes.yaml")
}
