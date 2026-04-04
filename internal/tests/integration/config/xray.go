package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// XrayGenerator Xray 配置生成器
type XrayGenerator struct {
	supportedProtocols map[Protocol]bool
}

// NewXrayGenerator 创建 Xray 配置生成器
func NewXrayGenerator() *XrayGenerator {
	return &XrayGenerator{
		supportedProtocols: map[Protocol]bool{
			ProtocolVMess:       true,
			ProtocolVLESS:       true,
			ProtocolTrojan:      true,
			ProtocolShadowsocks: true,
		},
	}
}

// Name 返回生成器名称
func (g *XrayGenerator) Name() string {
	return "xray"
}

// SupportedProtocols 返回支持的协议列表
func (g *XrayGenerator) SupportedProtocols() []Protocol {
	protocols := make([]Protocol, 0, len(g.supportedProtocols))
	for p := range g.supportedProtocols {
		protocols = append(protocols, p)
	}
	return protocols
}

// IsProtocolSupported 检查协议是否支持
func (g *XrayGenerator) IsProtocolSupported(p Protocol) bool {
	return g.supportedProtocols[p]
}

// Generate 生成 Xray 客户端配置
func (g *XrayGenerator) Generate(cfg *ClientConfig) ([]byte, error) {
	if !g.IsProtocolSupported(cfg.Server.Protocol) {
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Server.Protocol)
	}

	xrayConfig := &XrayConfig{
		Log: &XrayLog{
			LogLevel: "warning",
		},
		Inbounds: []XrayInbound{
			{
				Port:    10808,
				Listen:  "127.0.0.1",
				Tag:     "socks-in",
				Protocol: "socks",
				Settings: map[string]interface{}{
					"udp": true,
				},
			},
			{
				Port:    10809,
				Listen:  "127.0.0.1",
				Tag:     "http-in",
				Protocol: "http",
			},
		},
		Outbounds: []XrayOutbound{
			g.generateOutbound(cfg),
			{
				Tag:      "direct",
				Protocol: "freedom",
			},
			{
				Tag:      "block",
				Protocol: "blackhole",
			},
		},
		Routing: &XrayRouting{
			Rules: []XrayRoutingRule{
				{
					Type:        "field",
					IP:          []string{"geoip:private"},
					OutboundTag: "direct",
				},
				{
					Type:        "field",
					Domain:      []string{"geosite:category-ads-all"},
					OutboundTag: "block",
				},
			},
		},
	}

	return ToJSON(xrayConfig)
}

// GenerateFromScenario 从测试场景生成配置
func (g *XrayGenerator) GenerateFromScenario(scenario TestScenario, server ServerConfig, user UserConfig) ([]byte, error) {
	cfg := &ClientConfig{
		Name:   scenario.Name,
		Server: server,
		User:   user,
	}
	return g.Generate(cfg)
}

// generateOutbound 生成出站配置
func (g *XrayGenerator) generateOutbound(cfg *ClientConfig) XrayOutbound {
	outbound := XrayOutbound{
		Tag:      "proxy",
		Protocol: string(cfg.Server.Protocol),
		Settings: g.generateOutboundSettings(cfg),
		StreamSettings: g.generateStreamSettings(cfg),
	}

	return outbound
}

// generateOutboundSettings 生成出站设置
func (g *XrayGenerator) generateOutboundSettings(cfg *ClientConfig) map[string]interface{} {
	switch cfg.Server.Protocol {
	case ProtocolVMess:
		return map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": cfg.Server.Host,
					"port":    cfg.Server.Port,
					"users": []map[string]interface{}{
						{
							"id":       cfg.User.UUID,
							"alterId":  cfg.User.AlterID,
							"security": "auto",
						},
					},
				},
			},
		}

	case ProtocolVLESS:
		user := map[string]interface{}{
			"id":    cfg.User.UUID,
			"email": cfg.User.Email,
		}
		if cfg.Server.Flow != "" {
			user["flow"] = cfg.Server.Flow
		}
		return map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": cfg.Server.Host,
					"port":    cfg.Server.Port,
					"users":   []map[string]interface{}{user},
				},
			},
		}

	case ProtocolTrojan:
		return map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address":  cfg.Server.Host,
					"port":     cfg.Server.Port,
					"password": cfg.Server.Password,
				},
			},
		}

	case ProtocolShadowsocks:
		return map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address":  cfg.Server.Host,
					"port":     cfg.Server.Port,
					"method":   cfg.Server.Method,
					"password": cfg.Server.Password,
				},
			},
		}
	}

	return nil
}

// generateStreamSettings 生成传输层设置
func (g *XrayGenerator) generateStreamSettings(cfg *ClientConfig) *XrayStreamSettings {
	settings := &XrayStreamSettings{
		Network: string(cfg.Server.Transport),
	}

	// TLS 配置
	if cfg.Server.TLSType != TLSNone {
		settings.Security = string(cfg.Server.TLSType)

		switch cfg.Server.TLSType {
		case TLS:
			settings.TLSSettings = &XrayTLSSettings{
				ServerName:    cfg.Server.SNI,
				AllowInsecure: cfg.Server.Insecure,
			}

		case TLSReality:
			settings.RealitySettings = &XrayRealitySettings{
				ServerName: cfg.Server.SNI,
				PublicKey:  cfg.Server.PublicKey,
				ShortId:    cfg.Server.ShortID,
				SpiderX:    cfg.Server.SpiderX,
				Fingerprint: "chrome",
			}
		}
	}

	// 传输层配置
	switch cfg.Server.Transport {
	case TransportWS:
		settings.WSSettings = &XrayWSSettings{
			Path:    cfg.Server.TransportWS.Path,
			Headers: cfg.Server.TransportWS.Headers,
		}

	case TransportGRPC:
		settings.GRPCSettings = &XrayGRPCSettings{
			ServiceName: cfg.Server.TransportGRPC.ServiceName,
		}
	}

	return settings
}

// Xray 配置结构体

type XrayConfig struct {
	Log       *XrayLog        `json:"log"`
	Inbounds  []XrayInbound   `json:"inbounds"`
	Outbounds []XrayOutbound  `json:"outbounds"`
	Routing   *XrayRouting    `json:"routing"`
}

type XrayLog struct {
	LogLevel string `json:"logLevel"`
}

type XrayInbound struct {
	Port     int                    `json:"port"`
	Listen   string                 `json:"listen"`
	Tag      string                 `json:"tag"`
	Protocol string                 `json:"protocol"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type XrayOutbound struct {
	Tag            string                 `json:"tag"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	StreamSettings *XrayStreamSettings   `json:"streamSettings,omitempty"`
}

type XrayStreamSettings struct {
	Network        string                `json:"network"`
	Security       string                `json:"security,omitempty"`
	TLSSettings    *XrayTLSSettings      `json:"tlsSettings,omitempty"`
	RealitySettings *XrayRealitySettings `json:"realitySettings,omitempty"`
	WSSettings     *XrayWSSettings       `json:"wsSettings,omitempty"`
	GRPCSettings   *XrayGRPCSettings     `json:"grpcSettings,omitempty"`
}

type XrayTLSSettings struct {
	ServerName    string `json:"serverName,omitempty"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`
}

type XrayRealitySettings struct {
	ServerName  string `json:"serverName,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	ShortId     string `json:"shortId,omitempty"`
	SpiderX     string `json:"spiderX,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type XrayWSSettings struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type XrayGRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
}

type XrayRouting struct {
	Rules []XrayRoutingRule `json:"rules"`
}

type XrayRoutingRule struct {
	Type        string   `json:"type"`
	IP          []string `json:"ip,omitempty"`
	Domain      []string `json:"domain,omitempty"`
	OutboundTag string   `json:"outboundTag"`
}

// GenerateV2RayLink 生成 V2Ray 订阅链接
func (g *XrayGenerator) GenerateV2RayLink(cfg *ClientConfig) (string, error) {
	switch cfg.Server.Protocol {
	case ProtocolVMess:
		return g.generateVMessLink(cfg)
	case ProtocolVLESS:
		return g.generateVLESSLink(cfg)
	case ProtocolTrojan:
		return g.generateTrojanLink(cfg)
	case ProtocolShadowsocks:
		return g.generateSSLink(cfg)
	}
	return "", fmt.Errorf("unsupported protocol: %s", cfg.Server.Protocol)
}

func (g *XrayGenerator) generateVMessLink(cfg *ClientConfig) (string, error) {
	vmess := map[string]interface{}{
		"v":    "2",
		"ps":   cfg.Name,
		"add":  cfg.Server.Host,
		"port": cfg.Server.Port,
		"id":   cfg.User.UUID,
		"aid":  cfg.User.AlterID,
		"scy":  "auto",
		"net":  cfg.Server.Transport,
		"type": "none",
		"host": cfg.Server.SNI,
		"path": "",
		"tls":  "",
	}

	if cfg.Server.TLSType == TLS {
		vmess["tls"] = "tls"
	}
	if cfg.Server.TLSType == TLSReality {
		vmess["tls"] = "reality"
	}

	if cfg.Server.TransportWS != nil {
		vmess["path"] = cfg.Server.TransportWS.Path
	}

	data, _ := json.Marshal(vmess)
	return "vmess://" + base64.StdEncoding.EncodeToString(data), nil
}

func (g *XrayGenerator) generateVLESSLink(cfg *ClientConfig) (string, error) {
	link := fmt.Sprintf("vless://%s@%s:%d",
		cfg.User.UUID,
		cfg.Server.Host,
		cfg.Server.Port,
	)

	params := fmt.Sprintf("?type=%s", cfg.Server.Transport)

	if cfg.Server.TLSType == TLS {
		params += "&security=tls&sni=" + cfg.Server.SNI
	}
	if cfg.Server.TLSType == TLSReality {
		params += "&security=reality&sni=" + cfg.Server.SNI
		params += "&pbk=" + cfg.Server.PublicKey
		params += "&sid=" + cfg.Server.ShortID
	}
	if cfg.Server.Flow != "" {
		params += "&flow=" + cfg.Server.Flow
	}

	link += params + "#" + cfg.Name
	return link, nil
}

func (g *XrayGenerator) generateTrojanLink(cfg *ClientConfig) (string, error) {
	link := fmt.Sprintf("trojan://%s@%s:%d",
		cfg.Server.Password,
		cfg.Server.Host,
		cfg.Server.Port,
	)

	if cfg.Server.TLSType == TLS {
		link += "?sni=" + cfg.Server.SNI
	}

	link += "#" + cfg.Name
	return link, nil
}

func (g *XrayGenerator) generateSSLink(cfg *ClientConfig) (string, error) {
	userInfo := base64.URLEncoding.EncodeToString(
		[]byte(cfg.Server.Method + ":" + cfg.Server.Password),
	)
	link := fmt.Sprintf("ss://%s@%s:%d#%s",
		userInfo,
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Name,
	)
	return link, nil
}