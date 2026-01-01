package parser

import (
	"encoding/json"
	"strings"

	"github.com/anixops/v2board/internal/model"
)

// SingBoxFormatter Sing-box JSON 格式化器
type SingBoxFormatter struct{}

func (f *SingBoxFormatter) Name() string {
	return "sing-box"
}

func (f *SingBoxFormatter) ContentType() string {
	return "application/json; charset=utf-8"
}

func (f *SingBoxFormatter) FileExtension() string {
	return "json"
}

func (f *SingBoxFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	outbounds := make([]interface{}, 0, len(nodes)+10)

	proxyNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Name)
	}

	// 1. Selector (策略组)
	outbounds = append(outbounds, map[string]interface{}{
		"type":      "selector",
		"tag":       "🚀 节点选择",
		"outbounds": append([]string{"♻️ 自动选择", "DIRECT"}, proxyNames...),
		"default":   "♻️ 自动选择",
	})

	// 2. URLTest (自动选择)
	outbounds = append(outbounds, map[string]interface{}{
		"type":      "urltest",
		"tag":       "♻️ 自动选择",
		"outbounds": proxyNames,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  "5m",
		"tolerance": 50,
	})

	// 3. 节点 Outbounds
	for _, node := range nodes {
		outbound := f.buildOutbound(node, ctx)
		if outbound != nil {
			outbounds = append(outbounds, outbound)
		}
	}

	// 4. 基础 Outbounds (直连、拦截、DNS)
	outbounds = append(outbounds,
		map[string]interface{}{"type": "direct", "tag": "DIRECT"},
		map[string]interface{}{"type": "block", "tag": "REJECT"},
		map[string]interface{}{"type": "dns", "tag": "dns-out"},
	)

	config := map[string]interface{}{
		"dns": map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{
					"tag":     "google",
					"address": "https://8.8.8.8/dns-query",
				},
				map[string]interface{}{
					"tag":     "local",
					"address": "https://223.5.5.5/dns-query",
					"detour":  "DIRECT",
				},
			},
			"rules": []interface{}{
				map[string]interface{}{
					"outbound": "any",
					"server":   "google",
				},
				map[string]interface{}{
					"geosite": []string{"cn"},
					"server":  "local",
				},
			},
			"strategy": "prefer_ipv4",
		},
		"outbounds": outbounds,
		"route": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{"protocol": "dns", "outbound": "dns-out"},
				map[string]interface{}{"geosite": []string{"cn"}, "outbound": "DIRECT"},
				map[string]interface{}{"geoip": []string{"cn", "private"}, "outbound": "DIRECT"},
				map[string]interface{}{"geosite": []string{"category-ads-all"}, "outbound": "REJECT"},
			},
			"final":                 "🚀 节点选择",
			"auto_detect_interface": true,
		},
	}

	return json.MarshalIndent(config, "", "  ")
}

func (f *SingBoxFormatter) buildOutbound(node *model.ParsedNode, ctx *model.TemplateRenderContext) map[string]interface{} {
	outbound := map[string]interface{}{
		"tag":         node.Name,
		"server":      node.Server,
		"server_port": node.Port,
	}

	switch node.Type {
	case "vmess":
		f.buildVMess(outbound, node, ctx)
	case "vless":
		f.buildVLESS(outbound, node, ctx)
	case "trojan":
		f.buildTrojan(outbound, node, ctx)
	case "shadowsocks", "ss":
		f.buildShadowsocks(outbound, node, ctx)
	case "hysteria2", "hy2":
		f.buildHysteria2(outbound, node, ctx)
	case "tuic":
		f.buildTUIC(outbound, node, ctx)
	default:
		return nil
	}

	return outbound
}

func (f *SingBoxFormatter) buildVMess(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "vmess"
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	outbound["uuid"] = uuid
	outbound["security"] = "auto"
	outbound["alter_id"] = 0

	if v, ok := node.Settings["alter_id"]; ok {
		outbound["alter_id"] = toUint32(v)
	}

	f.addTLS(outbound, node)
	f.addTransport(outbound, node)
}

func (f *SingBoxFormatter) buildVLESS(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "vless"
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	outbound["uuid"] = uuid

	if node.Flow != "" {
		outbound["flow"] = node.Flow
	}

	f.addTLS(outbound, node)
	f.addTransport(outbound, node)
}

func (f *SingBoxFormatter) buildTrojan(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "trojan"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	outbound["password"] = password

	f.addTLS(outbound, node)
	f.addTransport(outbound, node)
}

func (f *SingBoxFormatter) buildShadowsocks(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "shadowsocks"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	cipher := node.Cipher
	if cipher == "" {
		cipher = "aes-256-gcm"
	}
	outbound["method"] = cipher
	outbound["password"] = password

	// SS2022
	if strings.HasPrefix(cipher, "2022-blake3-") {
		serverKey := node.ServerKey
		if serverKey == "" {
			if sk, ok := node.Settings["server_key"].(string); ok {
				serverKey = sk
			}
		}
		if serverKey != "" {
			userKey := generateSS2022UserKey(password, cipher)
			outbound["password"] = serverKey + ":" + userKey
		}
	}
}

func (f *SingBoxFormatter) buildHysteria2(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "hysteria2"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	outbound["password"] = password

	tls := map[string]interface{}{
		"enabled": true,
	}
	if node.ServerName != "" {
		tls["server_name"] = node.ServerName
	}
	tls["insecure"] = node.SkipCertVerify
	outbound["tls"] = tls

	if obfs, ok := node.Settings["obfs"].(string); ok && obfs != "" {
		obfsMap := map[string]interface{}{"type": obfs}
		if obfsPassword, ok := node.Settings["obfs-password"].(string); ok && obfsPassword != "" {
			obfsMap["password"] = obfsPassword
		}
		outbound["obfs"] = obfsMap
	}
}

func (f *SingBoxFormatter) buildTUIC(outbound map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "tuic"
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	outbound["uuid"] = uuid
	outbound["password"] = node.Password

	f.addTLS(outbound, node)
}

func (f *SingBoxFormatter) addTLS(outbound map[string]interface{}, node *model.ParsedNode) {
	if node.TLSMode == 0 && !node.TLS {
		return
	}

	tls := map[string]interface{}{
		"enabled": true,
	}

	if node.ServerName != "" {
		tls["server_name"] = node.ServerName
	}
	if node.SkipCertVerify {
		tls["insecure"] = true
	}
	if node.ALPN != "" {
		tls["alpn"] = strings.Split(node.ALPN, ",")
	}
	if node.TLSFingerprint != "" {
		tls["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": node.TLSFingerprint,
		}
	}

	if node.TLSMode == 2 || node.RealityPublicKey != "" {
		tls["reality"] = map[string]interface{}{
			"enabled":    true,
			"public_key": node.RealityPublicKey,
			"short_id":   node.RealityShortID,
		}
	}

	outbound["tls"] = tls
}

func (f *SingBoxFormatter) addTransport(outbound map[string]interface{}, node *model.ParsedNode) {
	if node.Transport == "" || node.Transport == "tcp" {
		return
	}

	transport := map[string]interface{}{
		"type": node.Transport,
	}

	switch node.Transport {
	case "ws":
		if path, ok := node.TransportSettings["path"].(string); ok {
			transport["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			transport["headers"] = map[string]interface{}{"Host": host}
		}
	case "grpc":
		if sn, ok := node.TransportSettings["serviceName"].(string); ok {
			transport["service_name"] = sn
		}
	case "h2":
		if path, ok := node.TransportSettings["path"].(string); ok {
			transport["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			transport["host"] = []string{host}
		}
	}

	outbound["transport"] = transport
}

func toUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case int:
		return uint32(val)
	case int64:
		return uint32(val)
	case float64:
		return uint32(val)
	default:
		return 0
	}
}
