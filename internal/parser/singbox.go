package parser

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
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
	outbounds := make([]any, 0, len(nodes)+10)
	endpoints := make([]any, 0)
	proxyNames := make([]string, 0, len(nodes))
	nodeOutbounds := make([]any, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Type == "wireguard" {
			endpoints = append(endpoints, f.buildWireGuardEndpoint(node))
			proxyNames = append(proxyNames, node.Name)
			continue
		}
		outbound := f.buildOutbound(node, ctx)
		if outbound == nil {
			continue
		}
		proxyNames = append(proxyNames, node.Name)
		nodeOutbounds = append(nodeOutbounds, outbound)
	}

	// 1. Selector (策略组)
	outbounds = append(outbounds, map[string]any{
		"type":      "selector",
		"tag":       "🚀 节点选择",
		"outbounds": append([]string{"♻️ 自动选择", "DIRECT"}, proxyNames...),
		"default":   "♻️ 自动选择",
	})

	// 2. URLTest (自动选择)
	outbounds = append(outbounds, map[string]any{
		"type":      "urltest",
		"tag":       "♻️ 自动选择",
		"outbounds": proxyNames,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  "5m",
		"tolerance": 50,
	})

	// 3. 节点 Outbounds
	outbounds = append(outbounds, nodeOutbounds...)

	// 4. 基础 Outbounds (直连、拦截、DNS)
	outbounds = append(outbounds,
		map[string]any{"type": "direct", "tag": "DIRECT"},
		map[string]any{"type": "block", "tag": "REJECT"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	)

	config := map[string]any{
		"dns": map[string]any{
			"servers": []any{
				map[string]any{
					"tag":     "google",
					"address": "https://8.8.8.8/dns-query",
				},
				map[string]any{
					"tag":     "local",
					"address": "https://223.5.5.5/dns-query",
					"detour":  "DIRECT",
				},
			},
			"rules": []any{
				map[string]any{
					"outbound": "any",
					"server":   "google",
				},
				map[string]any{
					"geosite": []string{"cn"},
					"server":  "local",
				},
			},
			"strategy": "prefer_ipv4",
		},
		"outbounds": outbounds,
		"route": map[string]any{
			"rules": []any{
				map[string]any{"protocol": "dns", "outbound": "dns-out"},
				map[string]any{"geosite": []string{"cn"}, "outbound": "DIRECT"},
				map[string]any{"geoip": []string{"cn", "private"}, "outbound": "DIRECT"},
				map[string]any{"geosite": []string{"category-ads-all"}, "outbound": "REJECT"},
			},
			"final":                 "🚀 节点选择",
			"auto_detect_interface": true,
		},
	}
	if len(endpoints) > 0 {
		// WireGuard outbound was removed in sing-box 1.13. Endpoints remain
		// selectable as outbound tags while keeping the modern schema.
		config["endpoints"] = endpoints
	}

	return json.MarshalIndent(config, "", "  ")
}

func (f *SingBoxFormatter) buildOutbound(node *model.ParsedNode, ctx *model.TemplateRenderContext) map[string]any {
	outbound := map[string]any{
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

func (f *SingBoxFormatter) buildVMess(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
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

func (f *SingBoxFormatter) buildVLESS(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
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

func (f *SingBoxFormatter) buildTrojan(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "trojan"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	outbound["password"] = password

	f.addTLS(outbound, node)
	f.addTransport(outbound, node)
}

func (f *SingBoxFormatter) buildShadowsocks(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
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

func (f *SingBoxFormatter) buildHysteria2(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "hysteria2"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	outbound["password"] = password

	tls := map[string]any{
		"enabled": true,
	}
	if node.ServerName != "" {
		tls["server_name"] = node.ServerName
	}
	tls["insecure"] = node.SkipCertVerify
	outbound["tls"] = tls

	if obfs, ok := node.Settings["obfs"].(string); ok && obfs != "" {
		obfsMap := map[string]any{"type": obfs}
		if obfsPassword, ok := node.Settings["obfs-password"].(string); ok && obfsPassword != "" {
			obfsMap["password"] = obfsPassword
		}
		outbound["obfs"] = obfsMap
	}
}

func (f *SingBoxFormatter) buildTUIC(outbound map[string]any, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	outbound["type"] = "tuic"
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	outbound["uuid"] = uuid
	outbound["password"] = node.Password

	f.addTLS(outbound, node)
}

func (f *SingBoxFormatter) buildWireGuardEndpoint(node *model.ParsedNode) map[string]any {
	allowedIPs := node.AllowedIPs
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0"}
	}
	mtu := node.MTU
	if mtu <= 0 {
		mtu = 1280
	}
	peer := map[string]any{
		"address":                       wireGuardHost(node.Server),
		"port":                          node.Port,
		"public_key":                    node.PublicKey,
		"allowed_ips":                   allowedIPs,
		"persistent_keepalive_interval": 25,
	}
	if node.PresharedKey != "" {
		peer["pre_shared_key"] = node.PresharedKey
	}
	return map[string]any{
		"type":        "wireguard",
		"tag":         node.Name,
		"system":      false,
		"mtu":         mtu,
		"address":     []string{wireGuardAddress(node.PeerIP)},
		"private_key": node.PrivateKey,
		"peers":       []any{peer},
	}
}

func (f *SingBoxFormatter) addTLS(outbound map[string]any, node *model.ParsedNode) {
	if node.TLSMode == 0 && !node.TLS {
		return
	}

	tls := map[string]any{
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
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": node.TLSFingerprint,
		}
	}

	if node.TLSMode == 2 || node.RealityPublicKey != "" {
		tls["reality"] = map[string]any{
			"enabled":    true,
			"public_key": node.RealityPublicKey,
			"short_id":   node.RealityShortID,
		}
	}

	outbound["tls"] = tls
}

func (f *SingBoxFormatter) addTransport(outbound map[string]any, node *model.ParsedNode) {
	if node.Transport == "" || node.Transport == "tcp" {
		return
	}

	transport := map[string]any{
		"type": node.Transport,
	}

	switch node.Transport {
	case "ws":
		if path, ok := node.TransportSettings["path"].(string); ok {
			transport["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			transport["headers"] = map[string]any{"Host": host}
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

func toUint32(v any) uint32 {
	const maxUint32 = uint64(1<<32 - 1)

	switch val := v.(type) {
	case int:
		if val < 0 || uint64(val) > maxUint32 {
			return 0
		}
		return uint32(val)
	case int64:
		if val < 0 || uint64(val) > maxUint32 {
			return 0
		}
		return uint32(val)
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) || val < 0 || val > float64(maxUint32) {
			return 0
		}
		return uint32(val)
	default:
		return 0
	}
}
