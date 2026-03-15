package parser

import (
	"encoding/base64"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Clash Formatter Extended Tests ==========

func TestClashFormatter_Format_VLESS(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "VLESS Test",
			Type:       "vless",
			Server:     "vless.example.com",
			Port:       443,
			UUID:       "vless-uuid",
			TLS:        true,
			ServerName: "vless.example.com",
			Flow:       "xtls-rprx-vision",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "VLESS Test")
	assert.Contains(t, string(output), "vless")
}

func TestClashFormatter_Format_VLESS_Reality(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:              "VLESS Reality",
			Type:              "vless",
			Server:            "reality.example.com",
			Port:              443,
			UUID:              "reality-uuid",
			TLSMode:           2,
			RealityPublicKey:  "public-key",
			RealityShortID:    "short-id",
			ServerName:        "www.example.com",
			TLSFingerprint:    "chrome",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "VLESS Reality")
}

func TestClashFormatter_Format_Trojan(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "Trojan Test",
			Type:       "trojan",
			Server:     "trojan.example.com",
			Port:       443,
			Password:   "trojan-password",
			TLS:        true,
			ServerName: "trojan.example.com",
		},
		{
			Name:       "Trojan WS",
			Type:       "trojan",
			Server:     "trojan-ws.example.com",
			Port:       443,
			Password:   "trojan-ws-password",
			Transport:  "ws",
			TLS:        true,
			ServerName: "trojan-ws.example.com",
			TransportSettings: map[string]interface{}{
				"path": "/trojan-ws",
			},
		},
		{
			Name:         "Trojan gRPC",
			Type:         "trojan",
			Server:       "trojan-grpc.example.com",
			Port:         443,
			Password:     "trojan-grpc-password",
			Transport:    "grpc",
			TLS:          true,
			ServerName:   "trojan-grpc.example.com",
			TransportSettings: map[string]interface{}{
				"serviceName": "trojan-grpc",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "Trojan Test")
	assert.Contains(t, string(output), "Trojan WS")
	assert.Contains(t, string(output), "Trojan gRPC")
}

func TestClashFormatter_Format_Shadowsocks(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:     "SS AEAD",
			Type:     "shadowsocks",
			Server:   "ss.example.com",
			Port:     8388,
			Cipher:   "aes-256-gcm",
			Password: "ss-password",
		},
		{
			Name:       "SS 2022",
			Type:       "shadowsocks",
			Server:     "ss2022.example.com",
			Port:       8388,
			Cipher:     "2022-blake3-aes-256-gcm",
			Password:   "ss2022-password",
			ServerKey:  "server-key",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "SS AEAD")
	assert.Contains(t, string(output), "SS 2022")
}

func TestClashFormatter_Format_Hysteria2(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:     "Hysteria2 Test",
			Type:     "hysteria2",
			Server:   "hy2.example.com",
			Port:     443,
			Password: "hy2-password",
		},
		{
			Name:     "Hysteria2 Obfs",
			Type:     "hysteria2",
			Server:   "hy2-obfs.example.com",
			Port:     443,
			Password: "hy2-obfs-password",
			Settings: map[string]interface{}{
				"obfs":          "salamander",
				"obfs-password": "obfs-password",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "Hysteria2 Test")
	assert.Contains(t, string(output), "Hysteria2 Obfs")
}

func TestClashFormatter_Format_TUIC(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:     "TUIC Test",
			Type:     "tuic",
			Server:   "tuic.example.com",
			Port:     443,
			UUID:     "tuic-uuid",
			Password: "tuic-password",
			ALPN:     "h3",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "TUIC Test")
}

func TestClashFormatter_Format_AnyTLS(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "AnyTLS Test",
			Type:       "anytls",
			Server:     "anytls.example.com",
			Port:       443,
			Password:   "anytls-password",
			ServerName: "anytls.example.com",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "AnyTLS Test")
}

func TestClashFormatter_Format_SkipCertVerify(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:           "Skip Cert Verify",
			Type:           "vmess",
			Server:         "skip.example.com",
			Port:           443,
			TLS:            true,
			SkipCertVerify: true,
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "Skip Cert Verify")
}

// ========== V2Ray Formatter Extended Tests ==========

func TestV2RayFormatter_Format_Trojan_Transport(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "Trojan WS",
			Type:       "trojan",
			Server:     "trojan-ws.example.com",
			Port:       443,
			Password:   "trojan-ws-password",
			Transport:  "ws",
			TLS:        true,
			ServerName: "trojan-ws.example.com",
			TransportSettings: map[string]interface{}{
				"path": "/trojan-ws",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	// Output is base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "trojan-ws.example.com")
}

// ========== V2Ray Formatter Extended Tests - Transport ==========

func TestV2RayFormatter_FormatVMess_WS(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "VMess WS",
			Type:       "vmess",
			Server:     "vmess-ws.example.com",
			Port:       443,
			UUID:       "vmess-uuid",
			Transport:  "ws",
			TLS:        true,
			ServerName: "vmess-ws.example.com",
			TransportSettings: map[string]interface{}{
				"path": "/vmess-ws",
				"host": "vmess-ws.example.com",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	// V2RayFormatter output is base64 encoded links
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	// The decoded content is the vmess:// link
	assert.Contains(t, string(decoded), "vmess://")
}

func TestV2RayFormatter_FormatVMess_gRPC(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "VMess gRPC",
			Type:       "vmess",
			Server:     "vmess-grpc.example.com",
			Port:       443,
			UUID:       "vmess-uuid",
			Transport:  "grpc",
			TLS:        true,
			ServerName: "vmess-grpc.example.com",
			TransportSettings: map[string]interface{}{
				"serviceName": "vmess-grpc",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	// V2RayFormatter output is base64 encoded links
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	// The decoded content is the vmess:// link itself, which is base64 JSON
	assert.Contains(t, string(decoded), "vmess://")
}

func TestV2RayFormatter_FormatVLESS_WS(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "VLESS WS",
			Type:       "vless",
			Server:     "vless-ws.example.com",
			Port:       443,
			UUID:       "vless-uuid",
			Transport:  "ws",
			TLS:        true,
			ServerName: "vless-ws.example.com",
			TransportSettings: map[string]interface{}{
				"path": "/vless-ws",
				"host": "vless-ws.example.com",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "vless-ws.example.com")
}

func TestV2RayFormatter_FormatVLESS_gRPC(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "VLESS gRPC",
			Type:       "vless",
			Server:     "vless-grpc.example.com",
			Port:       443,
			UUID:       "vless-uuid",
			Transport:  "grpc",
			TLS:        true,
			ServerName: "vless-grpc.example.com",
			TransportSettings: map[string]interface{}{
				"serviceName": "vless-grpc",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "vless-grpc.example.com")
}

func TestV2RayFormatter_FormatTrojan_gRPC(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "Trojan gRPC",
			Type:       "trojan",
			Server:     "trojan-grpc.example.com",
			Port:       443,
			Password:   "trojan-password",
			Transport:  "grpc",
			TLS:        true,
			ServerName: "trojan-grpc.example.com",
			TransportSettings: map[string]interface{}{
				"serviceName": "trojan-grpc",
			},
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "trojan-grpc.example.com")
}

func TestV2RayFormatter_Format_Hysteria2(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:     "Hysteria2",
			Type:     "hysteria2",
			Server:   "hy2.example.com",
			Port:     443,
			Password: "hy2-password",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "hy2.example.com")
}

func TestV2RayFormatter_Format_TUIC(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:     "TUIC",
			Type:     "tuic",
			Server:   "tuic.example.com",
			Port:     443,
			UUID:     "tuic-uuid",
			Password: "tuic-password",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "tuic.example.com")
}

func TestV2RayFormatter_Format_AnyTLS(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:       "AnyTLS",
			Type:       "anytls",
			Server:     "anytls.example.com",
			Port:       443,
			Password:   "anytls-password",
			ServerName: "anytls.example.com",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "anytls.example.com")
}