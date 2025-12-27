package parser

import (
	"strings"

	"github.com/anixops/v2board/internal/model"
	"gopkg.in/yaml.v3"
)

// ClashParser Clash YAML 订阅解析器
type ClashParser struct{}

func (p *ClashParser) Name() string {
	return "clash"
}

func (p *ClashParser) Detect(content []byte) bool {
	// 检查是否包含 Clash 配置关键字
	str := string(content)
	return strings.Contains(str, "proxies:") ||
		strings.Contains(str, "Proxy:") ||
		(strings.Contains(str, "proxy-groups:") && strings.Contains(str, "rules:"))
}

func (p *ClashParser) Parse(content []byte) ([]*model.ParsedNode, error) {
	var clash struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
		Proxy   []map[string]interface{} `yaml:"Proxy"` // 兼容旧版
	}

	if err := yaml.Unmarshal(content, &clash); err != nil {
		return nil, err
	}

	proxies := clash.Proxies
	if len(proxies) == 0 {
		proxies = clash.Proxy
	}

	var nodes []*model.ParsedNode

	for _, proxy := range proxies {
		node := p.parseProxy(proxy)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

func (p *ClashParser) parseProxy(proxy map[string]interface{}) *model.ParsedNode {
	node := &model.ParsedNode{
		Name:     getString(proxy, "name"),
		Type:     strings.ToLower(getString(proxy, "type")),
		Server:   getString(proxy, "server"),
		Port:     getInt(proxy, "port"),
		Settings: make(map[string]interface{}),
	}

	// 根据类型解析
	switch node.Type {
	case "vmess":
		node.UUID = getString(proxy, "uuid")
		node.TLS = getBool(proxy, "tls")
		node.ServerName = getString(proxy, "servername")
		node.SkipCertVerify = getBool(proxy, "skip-cert-verify")
		node.Transport = getString(proxy, "network")

		node.Settings["alter_id"] = getInt(proxy, "alterId")
		node.Settings["security"] = getString(proxy, "cipher")

		if wsOpts, ok := proxy["ws-opts"].(map[string]interface{}); ok {
			node.TransportSettings = map[string]interface{}{
				"path": getString(wsOpts, "path"),
			}
			if headers, ok := wsOpts["headers"].(map[string]interface{}); ok {
				node.TransportSettings["host"] = getString(headers, "Host")
			}
		}

		if grpcOpts, ok := proxy["grpc-opts"].(map[string]interface{}); ok {
			node.TransportSettings = map[string]interface{}{
				"serviceName": getString(grpcOpts, "grpc-service-name"),
			}
		}

	case "vless":
		node.UUID = getString(proxy, "uuid")
		node.TLS = getBool(proxy, "tls")
		node.ServerName = getString(proxy, "servername")
		node.SkipCertVerify = getBool(proxy, "skip-cert-verify")
		node.Transport = getString(proxy, "network")

		if flow := getString(proxy, "flow"); flow != "" {
			node.Settings["flow"] = flow
		}

		// Reality
		if realityOpts, ok := proxy["reality-opts"].(map[string]interface{}); ok {
			node.RealityPublicKey = getString(realityOpts, "public-key")
			node.RealityShortID = getString(realityOpts, "short-id")
		}

	case "trojan":
		node.Password = getString(proxy, "password")
		node.TLS = true
		node.ServerName = getString(proxy, "sni")
		node.SkipCertVerify = getBool(proxy, "skip-cert-verify")
		node.Transport = getString(proxy, "network")

		if wsOpts, ok := proxy["ws-opts"].(map[string]interface{}); ok {
			node.TransportSettings = map[string]interface{}{
				"path": getString(wsOpts, "path"),
			}
			if headers, ok := wsOpts["headers"].(map[string]interface{}); ok {
				node.TransportSettings["host"] = getString(headers, "Host")
			}
		}

	case "ss", "shadowsocks":
		node.Type = "shadowsocks"
		node.Password = getString(proxy, "password")
		node.Settings["cipher"] = getString(proxy, "cipher")

		if obfs := getString(proxy, "plugin"); obfs != "" {
			node.Settings["plugin"] = obfs
			if pluginOpts, ok := proxy["plugin-opts"].(map[string]interface{}); ok {
				node.Settings["plugin-opts"] = pluginOpts
			}
		}

	case "hysteria2", "hy2":
		node.Type = "hysteria2"
		node.Password = getString(proxy, "password")
		node.TLS = true
		node.ServerName = getString(proxy, "sni")
		node.SkipCertVerify = getBool(proxy, "skip-cert-verify")

		if obfs := getString(proxy, "obfs"); obfs != "" {
			node.Settings["obfs"] = obfs
			node.Settings["obfs-password"] = getString(proxy, "obfs-password")
		}

	case "tuic":
		node.UUID = getString(proxy, "uuid")
		node.Password = getString(proxy, "password")
		node.TLS = true
		node.ServerName = getString(proxy, "sni")

		if cc := getString(proxy, "congestion-controller"); cc != "" {
			node.Settings["congestion_control"] = cc
		}

	default:
		return nil
	}

	return node
}

// SIP008Parser SIP008 JSON 格式解析器
type SIP008Parser struct{}

func (p *SIP008Parser) Name() string {
	return "sip008"
}

func (p *SIP008Parser) Detect(content []byte) bool {
	str := string(content)
	return strings.Contains(str, `"version"`) && strings.Contains(str, `"servers"`)
}

func (p *SIP008Parser) Parse(content []byte) ([]*model.ParsedNode, error) {
	var sip008 struct {
		Version int `json:"version"`
		Servers []struct {
			ID         string `json:"id"`
			Remarks    string `json:"remarks"`
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			Password   string `json:"password"`
			Method     string `json:"method"`
			Plugin     string `json:"plugin"`
			PluginOpts string `json:"plugin_opts"`
		} `json:"servers"`
	}

	if err := yaml.Unmarshal(content, &sip008); err != nil {
		return nil, err
	}

	var nodes []*model.ParsedNode

	for _, server := range sip008.Servers {
		node := &model.ParsedNode{
			ID:       server.ID,
			Name:     server.Remarks,
			Type:     "shadowsocks",
			Server:   server.Server,
			Port:     server.ServerPort,
			Password: server.Password,
			Settings: map[string]interface{}{
				"cipher": server.Method,
			},
		}

		if server.Plugin != "" {
			node.Settings["plugin"] = server.Plugin
			node.Settings["plugin_opts"] = server.PluginOpts
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// 辅助函数
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
