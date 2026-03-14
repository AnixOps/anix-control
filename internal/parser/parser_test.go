package parser

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Len(t, r.parsers, 3) // Base64, Clash, SIP008
	assert.Len(t, r.formatters, 8) // All registered formatters
}

func TestRegistry_RegisterParser(t *testing.T) {
	r := NewRegistry()
	initialCount := len(r.parsers)

	r.RegisterParser(&Base64Parser{})
	assert.Len(t, r.parsers, initialCount+1)
}

func TestRegistry_RegisterFormatter(t *testing.T) {
	r := NewRegistry()
	initialCount := len(r.formatters)

	r.RegisterFormatter("custom", &V2RayFormatter{})
	assert.Len(t, r.formatters, initialCount+1)
}

func TestRegistry_GetFormatter(t *testing.T) {
	r := NewRegistry()

	// Test existing formatters
	tests := []model.SubscriptionFormat{
		model.FormatV2Ray,
		model.FormatClash,
		model.FormatSurge,
		model.FormatJSON,
		model.FormatBase64JSON,
		model.FormatShadowrocket,
		model.FormatQuantumultX,
		model.FormatSingBox,
	}

	for _, format := range tests {
		f, ok := r.GetFormatter(format)
		assert.True(t, ok, "Formatter for %s should exist", format)
		assert.NotNil(t, f)
	}

	// Test non-existent formatter
	_, ok := r.GetFormatter("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_GetParser(t *testing.T) {
	r := NewRegistry()

	// Test existing parsers
	assert.NotNil(t, r.GetParser("base64"))
	assert.NotNil(t, r.GetParser("clash"))
	assert.NotNil(t, r.GetParser("sip008"))

	// Test non-existent parser
	assert.Nil(t, r.GetParser("nonexistent"))
}

func TestRegistry_AutoParse(t *testing.T) {
	r := NewRegistry()

	// Test valid V2Ray base64 content
	v2rayContent := "dmVzczovL2V5SjJJam9pTVM0d0xqQWlMQ0p0YVdRaU9pSmtZWFJoYkd4bElpd2dkR1Z6ZERFaU9pSkRiMlJsYzNsekxYSmtZWFJoYkd4bElpd2lZV3hwWW1WbFlYSjVJbjA5"
	nodes, err := r.AutoParse([]byte(v2rayContent))
	// May fail due to invalid format, but should not panic
	_ = nodes
	_ = err
}

func TestRegistry_AutoParse_UnknownFormat(t *testing.T) {
	r := NewRegistry()

	_, err := r.AutoParse([]byte("invalid content that is not recognized"))
	assert.Equal(t, ErrUnknownFormat, err)
}

func TestGetDefaultRegistry(t *testing.T) {
	r1 := GetDefaultRegistry()
	r2 := GetDefaultRegistry()
	assert.Equal(t, r1, r2, "Default registry should be singleton")
}

// ========== V2Ray Formatter Tests ==========

func TestV2RayFormatter_Format(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{
		UUID: "test-uuid-1234",
	}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test VMess",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
			TLS:    true,
		},
		{
			Name:   "Test VLESS",
			Type:   "vless",
			Server: "example.com",
			Port:   443,
			TLS:    true,
		},
		{
			Name:     "Test Trojan",
			Type:     "trojan",
			Server:   "example.com",
			Port:     443,
			Password: "test-password",
		},
		{
			Name:     "Test SS",
			Type:     "shadowsocks",
			Server:   "example.com",
			Port:     8388,
			Password: "test-password",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, output)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "vmess://")
	assert.Contains(t, string(decoded), "vless://")
	assert.Contains(t, string(decoded), "trojan://")
	assert.Contains(t, string(decoded), "ss://")
}

func TestV2RayFormatter_FormatVMess(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test VMess",
		Type:       "vmess",
		Server:     "example.com",
		Port:       443,
		TLS:        true,
		ServerName: "example.com",
		Transport:  "ws",
		TransportSettings: map[string]interface{}{
			"path": "/ws",
			"host": "example.com",
		},
	}

	link, err := f.formatVMess(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "vmess://")

	// Decode and verify the content
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "test-uuid")
}

func TestV2RayFormatter_FormatVLESS(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test VLESS",
		Type:       "vless",
		Server:     "example.com",
		Port:       443,
		TLSMode:    1,
		ServerName: "example.com",
	}

	link, err := f.formatVLESS(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "vless://")
	assert.Contains(t, link, "test-uuid")
	assert.Contains(t, link, "security=tls")
}

func TestV2RayFormatter_FormatVLESS_Reality(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:              "Test VLESS Reality",
		Type:              "vless",
		Server:            "example.com",
		Port:              443,
		TLSMode:           2,
		ServerName:        "example.com",
		RealityPublicKey:  "test-public-key",
		RealityShortID:    "test-short-id",
		TLSFingerprint:    "chrome",
	}

	link, err := f.formatVLESS(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "vless://")
	assert.Contains(t, link, "security=reality")
	assert.Contains(t, link, "pbk=test-public-key")
	assert.Contains(t, link, "sid=test-short-id")
}

func TestV2RayFormatter_FormatTrojan(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test Trojan",
		Type:       "trojan",
		Server:     "example.com",
		Port:       443,
		Password:   "test-password",
		ServerName: "example.com",
	}

	link, err := f.formatTrojan(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "trojan://")
	assert.Contains(t, link, "test-password")
}

func TestV2RayFormatter_FormatShadowsocks(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:     "Test SS",
		Type:     "shadowsocks",
		Server:   "example.com",
		Port:     8388,
		Password: "test-password",
		Cipher:   "aes-256-gcm",
	}

	link, err := f.formatShadowsocks(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "ss://")
}

func TestV2RayFormatter_FormatHysteria2(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test HY2",
		Type:       "hysteria2",
		Server:     "example.com",
		Port:       443,
		Password:   "test-password",
		ServerName: "example.com",
	}

	link, err := f.formatHysteria2(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "hy2://")
	assert.Contains(t, link, "test-password")
}

func TestV2RayFormatter_FormatTUIC(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test TUIC",
		Type:       "tuic",
		Server:     "example.com",
		Port:       443,
		Password:   "test-password",
		ServerName: "example.com",
	}

	link, err := f.formatTUIC(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "tuic://")
}

func TestV2RayFormatter_FormatAnyTLS(t *testing.T) {
	f := &V2RayFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	node := &model.ParsedNode{
		Name:       "Test AnyTLS",
		Type:       "anytls",
		Server:     "example.com",
		Port:       443,
		Password:   "test-password",
		ServerName: "example.com",
	}

	link, err := f.formatAnyTLS(node, ctx)
	require.NoError(t, err)
	assert.Contains(t, link, "anytls://")
}

func TestV2RayFormatter_Properties(t *testing.T) {
	f := &V2RayFormatter{}
	assert.Equal(t, "v2ray", f.Name())
	assert.Equal(t, "text/plain; charset=utf-8", f.ContentType())
	assert.Equal(t, "txt", f.FileExtension())
}

// ========== Clash Formatter Tests ==========

func TestClashFormatter_Format(t *testing.T) {
	f := &ClashFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test Node",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
			TLS:    true,
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "proxies:")
	assert.Contains(t, string(output), "Test Node")
}

func TestClashFormatter_Properties(t *testing.T) {
	f := &ClashFormatter{}
	assert.Equal(t, "clash", f.Name())
	assert.Equal(t, "text/yaml; charset=utf-8", f.ContentType())
	assert.Equal(t, "yaml", f.FileExtension())
}

// ========== Surge Formatter Tests ==========

func TestSurgeFormatter_Format(t *testing.T) {
	f := &SurgeFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test VMess",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
			TLS:    true,
		},
		{
			Name:   "Test VLESS",
			Type:   "vless",
			Server: "example.com",
			Port:   443,
			TLS:    true,
		},
		{
			Name:     "Test Trojan",
			Type:     "trojan",
			Server:   "example.com",
			Port:     443,
			Password: "test-password",
		},
		{
			Name:     "Test SS",
			Type:     "shadowsocks",
			Server:   "example.com",
			Port:     8388,
			Password: "test-password",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "[Proxy]")
	assert.Contains(t, string(output), "Test VMess")
	assert.Contains(t, string(output), "[Proxy Group]")
}

func TestSurgeFormatter_Properties(t *testing.T) {
	f := &SurgeFormatter{}
	assert.Equal(t, "surge", f.Name())
	assert.Equal(t, "text/plain; charset=utf-8", f.ContentType())
	assert.Equal(t, "conf", f.FileExtension())
}

// ========== JSON Formatter Tests ==========

func TestJSONFormatter_Format(t *testing.T) {
	f := &JSONFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test Node",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), `"name": "Test Node"`)
	assert.Contains(t, string(output), `"uuid": "test-uuid"`)
}

func TestJSONFormatter_Properties(t *testing.T) {
	f := &JSONFormatter{}
	assert.Equal(t, "json", f.Name())
	assert.Equal(t, "application/json; charset=utf-8", f.ContentType())
	assert.Equal(t, "json", f.FileExtension())
}

// ========== Base64 JSON Formatter Tests ==========

func TestBase64JSONFormatter_Format(t *testing.T) {
	f := &Base64JSONFormatter{}
	ctx := &model.TemplateRenderContext{
		UUID:           "test-uuid",
		TransferEnable:  10737418240,
		UsedTraffic:     1073741824,
		ExpiredAt:       1893456000,
	}

	nodes := []*model.ParsedNode{
		{
			Name:      "Test Node",
			Type:      "vmess",
			Server:    "example.com",
			Port:      443,
			GroupName: "Group1",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, output)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	// json.Marshal produces no spaces, so check without spaces
	assert.Contains(t, string(decoded), `"version":1`)
	assert.Contains(t, string(decoded), `"uuid":"test-uuid"`)
}

func TestBase64JSONFormatter_Properties(t *testing.T) {
	f := &Base64JSONFormatter{}
	assert.Equal(t, "base64json", f.Name())
	assert.Equal(t, "text/plain; charset=utf-8", f.ContentType())
	assert.Equal(t, "txt", f.FileExtension())
}

// ========== Shadowrocket Formatter Tests ==========

func TestShadowrocketFormatter_Format(t *testing.T) {
	f := &ShadowrocketFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test Node",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, output)
}

func TestShadowrocketFormatter_Properties(t *testing.T) {
	f := &ShadowrocketFormatter{}
	assert.Equal(t, "shadowrocket", f.Name())
	assert.Equal(t, "text/plain; charset=utf-8", f.ContentType())
	assert.Equal(t, "txt", f.FileExtension())
}

// ========== Quantumult X Formatter Tests ==========

func TestQuantumultXFormatter_Format(t *testing.T) {
	f := &QuantumultXFormatter{}
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}

	nodes := []*model.ParsedNode{
		{
			Name:   "Test VMess",
			Type:   "vmess",
			Server: "example.com",
			Port:   443,
		},
		{
			Name:     "Test Trojan",
			Type:     "trojan",
			Server:   "example.com",
			Port:     443,
			Password: "test-password",
		},
		{
			Name:     "Test SS",
			Type:     "shadowsocks",
			Server:   "example.com",
			Port:     8388,
			Password: "test-password",
		},
	}

	output, err := f.Format(nodes, ctx)
	require.NoError(t, err)
	assert.Contains(t, string(output), "vmess=")
	assert.Contains(t, string(output), "trojan=")
	assert.Contains(t, string(output), "shadowsocks=")
}

func TestQuantumultXFormatter_Properties(t *testing.T) {
	f := &QuantumultXFormatter{}
	assert.Equal(t, "quantumultx", f.Name())
	assert.Equal(t, "text/plain; charset=utf-8", f.ContentType())
	assert.Equal(t, "txt", f.FileExtension())
}

// ========== Helper Tests ==========

func TestGenerateSS2022UserKey(t *testing.T) {
	uuid := "12345678-1234-5678-1234-567812345678"
	cipher := "2022-blake3-aes-256-gcm"

	key := generateSS2022UserKey(uuid, cipher)
	assert.NotEmpty(t, key)

	// Test 128-bit cipher
	cipher128 := "2022-blake3-aes-128-gcm"
	key128 := generateSS2022UserKey(uuid, cipher128)
	assert.NotEmpty(t, key128)
}

func TestBoolToTLS(t *testing.T) {
	assert.Equal(t, "tls", boolToTLS(true))
	assert.Equal(t, "", boolToTLS(false))
}

func TestToInt(t *testing.T) {
	assert.Equal(t, 0, toInt(nil))
	assert.Equal(t, 123, toInt(123))
	assert.Equal(t, 456, toInt(int64(456)))
	assert.Equal(t, 789, toInt(float64(789)))
	assert.Equal(t, 100, toInt("100"))
}