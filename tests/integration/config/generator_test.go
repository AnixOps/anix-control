package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXrayGenerator(t *testing.T) {
	g := NewXrayGenerator()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "xray", g.Name())
	})

	t.Run("SupportedProtocols", func(t *testing.T) {
		protocols := g.SupportedProtocols()
		assert.Contains(t, protocols, ProtocolVMess)
		assert.Contains(t, protocols, ProtocolVLESS)
		assert.Contains(t, protocols, ProtocolTrojan)
		assert.Contains(t, protocols, ProtocolShadowsocks)
		assert.NotContains(t, protocols, ProtocolHysteria2)
	})

	t.Run("IsProtocolSupported", func(t *testing.T) {
		assert.True(t, g.IsProtocolSupported(ProtocolVMess))
		assert.True(t, g.IsProtocolSupported(ProtocolVLESS))
		assert.False(t, g.IsProtocolSupported(ProtocolHysteria2))
	})

	t.Run("Generate_VMess", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-vmess",
			Server: ServerConfig{
				Host:     "example.com",
				Port:     443,
				Protocol: ProtocolVMess,
				TLSType:  TLS,
				SNI:      "example.com",
			},
			User: UserConfig{
				UUID:    "test-uuid-1234",
				AlterID: 0,
			},
		}

		data, err := g.Generate(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"protocol": "vmess"`)
		assert.Contains(t, string(data), `"address": "example.com"`)
		assert.Contains(t, string(data), `"id": "test-uuid-1234"`)
	})

	t.Run("Generate_VLESS_Reality", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-vless-reality",
			Server: ServerConfig{
				Host:      "example.com",
				Port:      443,
				Protocol:  ProtocolVLESS,
				TLSType:   TLSReality,
				SNI:       "www.google.com",
				PublicKey: "test-public-key",
				ShortID:   "test-short-id",
				Flow:      "xtls-rprx-vision",
			},
			User: UserConfig{
				UUID: "test-uuid-5678",
			},
		}

		data, err := g.Generate(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"protocol": "vless"`)
		assert.Contains(t, string(data), `"flow": "xtls-rprx-vision"`)
		assert.Contains(t, string(data), `"realitySettings"`)
		assert.Contains(t, string(data), `"publicKey": "test-public-key"`)
	})

	t.Run("Generate_UnsupportedProtocol", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-hysteria2",
			Server: ServerConfig{
				Host:     "example.com",
				Port:     443,
				Protocol: ProtocolHysteria2,
			},
		}

		_, err := g.Generate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported protocol")
	})
}

func TestMihomoGenerator(t *testing.T) {
	g := NewMihomoGenerator()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "mihomo", g.Name())
	})

	t.Run("SupportedProtocols", func(t *testing.T) {
		protocols := g.SupportedProtocols()
		assert.Contains(t, protocols, ProtocolVMess)
		assert.Contains(t, protocols, ProtocolVLESS)
		assert.Contains(t, protocols, ProtocolHysteria2)
		assert.Contains(t, protocols, ProtocolTUIC)
	})

	t.Run("Generate_VMess", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-vmess",
			Server: ServerConfig{
				Host:     "example.com",
				Port:     443,
				Protocol: ProtocolVMess,
				TLSType:  TLS,
				SNI:      "example.com",
			},
			User: UserConfig{
				UUID: "test-uuid-1234",
			},
		}

		data, err := g.Generate(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: vmess")
		assert.Contains(t, string(data), "server: example.com")
		assert.Contains(t, string(data), "uuid: test-uuid-1234")
	})

	t.Run("Generate_Hysteria2", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-hysteria2",
			Server: ServerConfig{
				Host:     "example.com",
				Port:     443,
				Protocol: ProtocolHysteria2,
				TLSType:  TLS,
				Password: "test-password",
			},
		}

		data, err := g.Generate(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: hysteria2")
		assert.Contains(t, string(data), "password: test-password")
	})

	t.Run("Generate_VLESS_Reality", func(t *testing.T) {
		cfg := &ClientConfig{
			Name: "test-vless-reality",
			Server: ServerConfig{
				Host:      "example.com",
				Port:      443,
				Protocol:  ProtocolVLESS,
				TLSType:   TLSReality,
				SNI:       "www.google.com",
				PublicKey: "test-public-key",
				ShortID:   "test-short-id",
			},
			User: UserConfig{
				UUID: "test-uuid-5678",
			},
		}

		data, err := g.Generate(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: vless")
		assert.Contains(t, string(data), "reality-opts")
		assert.Contains(t, string(data), "public-key: test-public-key")
	})
}

func TestGeneratorRegistry(t *testing.T) {
	registry := NewGeneratorRegistry()

	t.Run("RegisterAndGet", func(t *testing.T) {
		registry.Register(NewXrayGenerator())
		registry.Register(NewMihomoGenerator())

		g, err := registry.Get("xray")
		require.NoError(t, err)
		assert.Equal(t, "xray", g.Name())

		g, err = registry.Get("mihomo")
		require.NoError(t, err)
		assert.Equal(t, "mihomo", g.Name())
	})

	t.Run("GetNotFound", func(t *testing.T) {
		_, err := registry.Get("notexist")
		assert.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		names := registry.List()
		assert.Contains(t, names, "xray")
		assert.Contains(t, names, "mihomo")
	})
}

func TestDefaultTestScenarios(t *testing.T) {
	scenarios := DefaultTestScenarios

	assert.NotEmpty(t, scenarios)

	// 验证每个场景都有必要的配置
	for _, s := range scenarios {
		assert.NotEmpty(t, s.Name)
		assert.NotEmpty(t, s.Protocol)
		assert.NotEmpty(t, s.TestURLs)
	}
}

func TestGenerateConfig(t *testing.T) {
	cfg := &ClientConfig{
		Name: "test",
		Server: ServerConfig{
			Host:     "example.com",
			Port:     443,
			Protocol: ProtocolVMess,
		},
		User: UserConfig{
			UUID: "test-uuid",
		},
	}

	t.Run("Xray", func(t *testing.T) {
		data, err := GenerateConfig("xray", cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"protocol": "vmess"`)
	})

	t.Run("Mihomo", func(t *testing.T) {
		data, err := GenerateConfig("mihomo", cfg)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: vmess")
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := GenerateConfig("notexist", cfg)
		assert.Error(t, err)
	})
}