package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// MihomoGenerator Mihomo (Clash.Meta) 配置生成器
type MihomoGenerator struct {
	supportedProtocols map[Protocol]bool
}

// NewMihomoGenerator 创建 Mihomo 配置生成器
func NewMihomoGenerator() *MihomoGenerator {
	return &MihomoGenerator{
		supportedProtocols: map[Protocol]bool{
			ProtocolVMess:       true,
			ProtocolVLESS:       true,
			ProtocolTrojan:      true,
			ProtocolShadowsocks: true,
			ProtocolHysteria2:   true,
			ProtocolTUIC:        true,
		},
	}
}

// Name 返回生成器名称
func (g *MihomoGenerator) Name() string {
	return "mihomo"
}

// SupportedProtocols 返回支持的协议列表
func (g *MihomoGenerator) SupportedProtocols() []Protocol {
	protocols := make([]Protocol, 0, len(g.supportedProtocols))
	for p := range g.supportedProtocols {
		protocols = append(protocols, p)
	}
	return protocols
}

// IsProtocolSupported 检查协议是否支持
func (g *MihomoGenerator) IsProtocolSupported(p Protocol) bool {
	return g.supportedProtocols[p]
}

// Generate 生成 Mihomo 客户端配置
func (g *MihomoGenerator) Generate(cfg *ClientConfig) ([]byte, error) {
	if !g.IsProtocolSupported(cfg.Server.Protocol) {
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Server.Protocol)
	}

	mihomoConfig := &MihomoConfig{
		Port:               7890,
		SocksPort:          7891,
		MixedPort:          7892,
		AllowLan:           false,
		BindAddress:        "127.0.0.1",
		Mode:               "rule",
		LogLevel:           "warning",
		IPv6:               false,
		ExternalController: "127.0.0.1:9090",
		Proxies:            []map[string]interface{}{g.generateProxy(cfg)},
		ProxyGroups:        g.generateProxyGroups(),
		Rules:              g.generateRules(),
		DNS:                g.generateDNS(),
	}

	return yaml.Marshal(mihomoConfig)
}

// GenerateFromScenario 从测试场景生成配置
func (g *MihomoGenerator) GenerateFromScenario(scenario TestScenario, server ServerConfig, user UserConfig) ([]byte, error) {
	cfg := &ClientConfig{
		Name:   scenario.Name,
		Server: server,
		User:   user,
	}
	return g.Generate(cfg)
}

// generateProxy 生成代理配置
func (g *MihomoGenerator) generateProxy(cfg *ClientConfig) map[string]interface{} {
	proxy := map[string]interface{}{
		"name":     cfg.Name,
		"type":     string(cfg.Server.Protocol),
		"server":   cfg.Server.Host,
		"port":     cfg.Server.Port,
	}

	switch cfg.Server.Protocol {
	case ProtocolVMess:
		proxy["uuid"] = cfg.User.UUID
		proxy["alterId"] = cfg.User.AlterID
		proxy["cipher"] = "auto"

	case ProtocolVLESS:
		proxy["uuid"] = cfg.User.UUID
		if cfg.Server.Flow != "" {
			proxy["flow"] = cfg.Server.Flow
		}

	case ProtocolTrojan:
		proxy["password"] = cfg.Server.Password

	case ProtocolShadowsocks:
		proxy["cipher"] = cfg.Server.Method
		proxy["password"] = cfg.Server.Password

	case ProtocolHysteria2:
		proxy["password"] = cfg.Server.Password
		proxy["obfs"] = "salamander"
		proxy["obfs-password"] = ""

	case ProtocolTUIC:
		proxy["uuid"] = cfg.User.UUID
		proxy["password"] = cfg.Server.Password
		proxy["congestion-controller"] = "bbr"
	}

	// 传输层配置
	switch cfg.Server.Transport {
	case TransportWS:
		proxy["network"] = "ws"
		if cfg.Server.TransportWS != nil {
			proxy["ws-opts"] = map[string]interface{}{
				"path":    cfg.Server.TransportWS.Path,
				"headers": cfg.Server.TransportWS.Headers,
			}
		}

	case TransportGRPC:
		proxy["network"] = "grpc"
		if cfg.Server.TransportGRPC != nil {
			proxy["grpc-opts"] = map[string]interface{}{
				"grpc-service-name": cfg.Server.TransportGRPC.ServiceName,
			}
		}
	}

	// TLS 配置
	switch cfg.Server.TLSType {
	case TLS:
		proxy["tls"] = true
		if cfg.Server.SNI != "" {
			proxy["servername"] = cfg.Server.SNI
		}
		if cfg.Server.Insecure {
			proxy["skip-cert-verify"] = true
		}

	case TLSReality:
		proxy["tls"] = true
		proxy["reality-opts"] = map[string]interface{}{
			"public-key": cfg.Server.PublicKey,
			"short-id":   cfg.Server.ShortID,
		}
		if cfg.Server.SNI != "" {
			proxy["servername"] = cfg.Server.SNI
		}
	}

	return proxy
}

// generateProxyGroups 生成代理组
func (g *MihomoGenerator) generateProxyGroups() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":    "PROXY",
			"type":    "select",
			"proxies": []string{"AUTO", "DIRECT"},
		},
		{
			"name":     "AUTO",
			"type":     "url-test",
			"proxies":  []string{}, // 将在运行时填充
			"url":      "http://www.gstatic.com/generate_204",
			"interval": 300,
		},
	}
}

// generateRules 生成规则
func (g *MihomoGenerator) generateRules() []string {
	return []string{
		"GEOSITE,category-ads-all,REJECT",
		"GEOIP,LAN,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT,no-resolve",
		"MATCH,PROXY",
	}
}

// generateDNS 生成 DNS 配置
func (g *MihomoGenerator) generateDNS() map[string]interface{} {
	return map[string]interface{}{
		"enable":           true,
		"ipv6":             false,
		"default-nameserver": []string{
			"223.5.5.5",
			"119.29.29.29",
		},
		"enhanced-mode": "fake-ip",
		"fake-ip-range": "198.18.0.1/16",
		"nameserver": []string{
			"https://doh.pub/dns-query",
			"https://dns.alidns.com/dns-query",
		},
	}
}

// Mihomo 配置结构体

type MihomoConfig struct {
	Port               int                            `yaml:"port"`
	SocksPort          int                            `yaml:"socks-port"`
	MixedPort          int                            `yaml:"mixed-port"`
	AllowLan           bool                           `yaml:"allow-lan"`
	BindAddress        string                         `yaml:"bind-address"`
	Mode               string                         `yaml:"mode"`
	LogLevel           string                         `yaml:"log-level"`
	IPv6               bool                           `yaml:"ipv6"`
	ExternalController string                         `yaml:"external-controller"`
	Proxies            []map[string]interface{}       `yaml:"proxies"`
	ProxyGroups        []map[string]interface{}       `yaml:"proxy-groups"`
	Rules              []string                       `yaml:"rules"`
	DNS                map[string]interface{}         `yaml:"dns"`
}

// GenerateClashProxies 生成 Clash 代理配置片段
func (g *MihomoGenerator) GenerateClashProxies(configs []*ClientConfig) ([]map[string]interface{}, error) {
	proxies := make([]map[string]interface{}, 0, len(configs))
	for _, cfg := range configs {
		if g.IsProtocolSupported(cfg.Server.Protocol) {
			proxies = append(proxies, g.generateProxy(cfg))
		}
	}
	return proxies, nil
}

// GenerateClashSubscription 生成 Clash 订阅格式
func (g *MihomoGenerator) GenerateClashSubscription(configs []*ClientConfig) ([]byte, error) {
	proxies, err := g.GenerateClashProxies(configs)
	if err != nil {
		return nil, err
	}

	subscription := &MihomoConfig{
		Port:               7890,
		SocksPort:          7891,
		MixedPort:          7892,
		AllowLan:           false,
		Mode:               "rule",
		LogLevel:           "info",
		ExternalController: "127.0.0.1:9090",
		Proxies:            proxies,
		ProxyGroups:        g.generateProxyGroupsForSubscription(len(proxies)),
		Rules:              g.generateRules(),
		DNS:                g.generateDNS(),
	}

	return yaml.Marshal(subscription)
}

func (g *MihomoGenerator) generateProxyGroupsForSubscription(proxyCount int) []map[string]interface{} {
	proxyNames := make([]string, proxyCount)
	for i := 0; i < proxyCount; i++ {
		proxyNames[i] = fmt.Sprintf("node-%d", i+1)
	}

	return []map[string]interface{}{
		{
			"name":    "PROXY",
			"type":    "select",
			"proxies": append([]string{"AUTO", "DIRECT"}, proxyNames...),
		},
		{
			"name":     "AUTO",
			"type":     "url-test",
			"proxies":  proxyNames,
			"url":      "http://www.gstatic.com/generate_204",
			"interval": 300,
		},
	}
}