package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/anixops/v2board/internal/model"
	"gopkg.in/yaml.v3"
)

// V2RayFormatter V2Ray Base64 格式化器
// 符合 V2bX/Xray 协议配置规范
type V2RayFormatter struct{}

func (f *V2RayFormatter) Name() string {
	return "v2ray"
}

func (f *V2RayFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *V2RayFormatter) FileExtension() string {
	return "txt"
}

func (f *V2RayFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	var links []string

	for _, node := range nodes {
		var link string
		var err error

		switch node.Type {
		case "vmess":
			link, err = f.formatVMess(node, ctx)
		case "vless":
			link, err = f.formatVLESS(node, ctx)
		case "trojan":
			link, err = f.formatTrojan(node, ctx)
		case "shadowsocks", "ss":
			link, err = f.formatShadowsocks(node, ctx)
		case "hysteria2", "hy2":
			link, err = f.formatHysteria2(node, ctx)
		case "tuic":
			link, err = f.formatTUIC(node, ctx)
		case "anytls":
			link, err = f.formatAnyTLS(node, ctx)
		}

		if err == nil && link != "" {
			links = append(links, link)
		}
	}

	content := strings.Join(links, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	return []byte(encoded), nil
}

// formatVMess 格式化 VMess 链接
// VMess 不支持 Reality (tls=2)，只支持 None(0) 和 TLS(1)
func (f *V2RayFormatter) formatVMess(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}

	// VMess alterId (V2Ray 新版本建议设为 0)
	alterId := 0
	if v, ok := node.Settings["alter_id"]; ok {
		alterId = toInt(v)
	}

	// 加密方式
	security := "auto"
	if v, ok := node.Settings["security"].(string); ok && v != "" {
		security = v
	}

	// 传输层协议
	network := node.Transport
	if network == "" {
		network = "tcp"
	}

	vmess := map[string]interface{}{
		"v":    "2",
		"ps":   node.Name,
		"add":  node.Server,
		"port": node.Port,
		"id":   uuid,
		"aid":  alterId,
		"scy":  security,
		"net":  network,
		"type": "none",
	}

	// TLS 配置
	if node.TLS || node.TLSMode == 1 {
		vmess["tls"] = "tls"
		if node.ServerName != "" {
			vmess["sni"] = node.ServerName
		}
		if node.ALPN != "" {
			vmess["alpn"] = node.ALPN
		}
		if node.TLSFingerprint != "" {
			vmess["fp"] = node.TLSFingerprint
		}
	} else {
		vmess["tls"] = ""
	}

	// 传输层配置
	f.applyVMessTransport(vmess, node)

	data, _ := json.Marshal(vmess)
	return "vmess://" + base64.StdEncoding.EncodeToString(data), nil
}

// applyVMessTransport 应用 VMess 传输层配置
func (f *V2RayFormatter) applyVMessTransport(vmess map[string]interface{}, node *model.ParsedNode) {
	if node.TransportSettings == nil {
		return
	}

	switch node.Transport {
	case "ws":
		if path, ok := node.TransportSettings["path"].(string); ok {
			vmess["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			vmess["host"] = host
		}
		if headers, ok := node.TransportSettings["headers"].(map[string]interface{}); ok {
			if h, ok := headers["Host"].(string); ok {
				vmess["host"] = h
			}
		}
	case "grpc":
		if sn, ok := node.TransportSettings["serviceName"].(string); ok {
			vmess["path"] = sn
		}
		vmess["type"] = "gun"
	case "h2":
		if path, ok := node.TransportSettings["path"].(string); ok {
			vmess["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			vmess["host"] = host
		}
	case "httpupgrade":
		if path, ok := node.TransportSettings["path"].(string); ok {
			vmess["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			vmess["host"] = host
		}
	}
}

// formatVLESS 格式化 VLESS 链接
// VLESS 支持 TLS(1) 和 Reality(2)
// 格式: vless://uuid@server:port?params#name
func (f *V2RayFormatter) formatVLESS(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}

	params := url.Values{}

	// 传输层 (type)
	transport := node.Transport
	if transport == "" {
		transport = "tcp"
	}
	params.Set("type", transport)

	// 安全模式 (security)
	if node.TLSMode == 2 || node.RealityPublicKey != "" {
		// Reality 模式
		params.Set("security", "reality")
		if node.RealityPublicKey != "" {
			params.Set("pbk", node.RealityPublicKey)
		}
		if node.RealityShortID != "" {
			params.Set("sid", node.RealityShortID)
		}
		if node.RealitySpiderX != "" {
			params.Set("spx", node.RealitySpiderX)
		}
	} else if node.TLS || node.TLSMode == 1 {
		params.Set("security", "tls")
	} else {
		params.Set("security", "none")
	}

	// SNI
	if node.ServerName != "" {
		params.Set("sni", node.ServerName)
	}

	// TLS 指纹
	if node.TLSFingerprint != "" {
		params.Set("fp", node.TLSFingerprint)
	}

	// ALPN
	if node.ALPN != "" {
		params.Set("alpn", node.ALPN)
	}

	// Flow (XTLS Vision 等)
	flow := node.Flow
	if flow == "" {
		if f, ok := node.Settings["flow"].(string); ok {
			flow = f
		}
	}
	if flow != "" {
		params.Set("flow", flow)
	}

	// Encryption (mlkem768x25519plus 等后量子加密)
	if node.Encryption != "" {
		params.Set("encryption", node.Encryption)
	}

	// 传输层配置
	f.applyVLESSTransport(params, node)

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		uuid, node.Server, node.Port, params.Encode(), url.QueryEscape(node.Name)), nil
}

// applyVLESSTransport 应用 VLESS 传输层配置
func (f *V2RayFormatter) applyVLESSTransport(params url.Values, node *model.ParsedNode) {
	if node.TransportSettings == nil {
		return
	}

	switch node.Transport {
	case "ws":
		if path, ok := node.TransportSettings["path"].(string); ok && path != "" {
			params.Set("path", path)
		}
		if host, ok := node.TransportSettings["host"].(string); ok && host != "" {
			params.Set("host", host)
		}
		if headers, ok := node.TransportSettings["headers"].(map[string]interface{}); ok {
			if h, ok := headers["Host"].(string); ok && h != "" {
				params.Set("host", h)
			}
		}
	case "grpc":
		if sn, ok := node.TransportSettings["serviceName"].(string); ok && sn != "" {
			params.Set("serviceName", sn)
		}
		if mode, ok := node.TransportSettings["mode"].(string); ok && mode != "" {
			params.Set("mode", mode)
		}
	case "h2":
		if path, ok := node.TransportSettings["path"].(string); ok && path != "" {
			params.Set("path", path)
		}
		if host, ok := node.TransportSettings["host"].(string); ok && host != "" {
			params.Set("host", host)
		}
	case "httpupgrade":
		if path, ok := node.TransportSettings["path"].(string); ok && path != "" {
			params.Set("path", path)
		}
		if host, ok := node.TransportSettings["host"].(string); ok && host != "" {
			params.Set("host", host)
		}
	case "xhttp", "splithttp":
		if path, ok := node.TransportSettings["path"].(string); ok && path != "" {
			params.Set("path", path)
		}
	}
}

// formatTrojan 格式化 Trojan 链接
// Trojan 协议强制使用 TLS
// 格式: trojan://password@server:port?params#name
func (f *V2RayFormatter) formatTrojan(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID // Trojan 使用 UUID 作为密码
	}

	params := url.Values{}

	// Trojan 强制 TLS
	params.Set("security", "tls")

	// SNI
	if node.ServerName != "" {
		params.Set("sni", node.ServerName)
	}

	// TLS 指纹
	if node.TLSFingerprint != "" {
		params.Set("fp", node.TLSFingerprint)
	}

	// ALPN
	if node.ALPN != "" {
		params.Set("alpn", node.ALPN)
	}

	// 跳过证书验证
	if node.SkipCertVerify {
		params.Set("allowInsecure", "1")
	}

	// 传输层 (Trojan 支持 tcp, ws, grpc)
	transport := node.Transport
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" {
		params.Set("type", transport)
		f.applyTrojanTransport(params, node)
	}

	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		url.QueryEscape(password), node.Server, node.Port, params.Encode(), url.QueryEscape(node.Name)), nil
}

// applyTrojanTransport 应用 Trojan 传输层配置
func (f *V2RayFormatter) applyTrojanTransport(params url.Values, node *model.ParsedNode) {
	if node.TransportSettings == nil {
		return
	}

	switch node.Transport {
	case "ws":
		if path, ok := node.TransportSettings["path"].(string); ok && path != "" {
			params.Set("path", path)
		}
		if host, ok := node.TransportSettings["host"].(string); ok && host != "" {
			params.Set("host", host)
		}
	case "grpc":
		if sn, ok := node.TransportSettings["serviceName"].(string); ok && sn != "" {
			params.Set("serviceName", sn)
		}
	}
}

// formatShadowsocks 格式化 Shadowsocks 链接
// 支持传统 SS 和 SS2022
// 格式: ss://base64(method:password)@server:port#name
// SS2022格式: ss://method:base64(server_key):base64(user_key)@server:port#name
func (f *V2RayFormatter) formatShadowsocks(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	// 获取加密方式
	cipher := node.Cipher
	if cipher == "" {
		if c, ok := node.Settings["cipher"].(string); ok && c != "" {
			cipher = c
		} else {
			cipher = "aes-256-gcm"
		}
	}

	// 检查是否是 SS2022
	isSS2022 := strings.HasPrefix(cipher, "2022-blake3-")

	if isSS2022 {
		// SS2022 格式
		// 需要 server_key，用户密钥从 UUID 生成
		serverKey := node.ServerKey
		if serverKey == "" {
			if sk, ok := node.Settings["server_key"].(string); ok {
				serverKey = sk
			}
		}

		if serverKey != "" {
			// ss2022://method:server_key:user_key@server:port#name
			userKey := generateSS2022UserKey(password, cipher)
			userInfo := fmt.Sprintf("%s:%s:%s", cipher, serverKey, userKey)
			return fmt.Sprintf("ss://%s@%s:%d#%s",
				base64.RawURLEncoding.EncodeToString([]byte(userInfo)),
				node.Server, node.Port, url.QueryEscape(node.Name)), nil
		}
	}

	// 传统 SS 格式: ss://base64(method:password)@server:port#name
	userInfo := base64.RawURLEncoding.EncodeToString([]byte(cipher + ":" + password))
	return fmt.Sprintf("ss://%s@%s:%d#%s",
		userInfo, node.Server, node.Port, url.QueryEscape(node.Name)), nil
}

// generateSS2022UserKey 生成 SS2022 用户密钥
func generateSS2022UserKey(uuid string, cipher string) string {
	// SS2022 密钥长度要求
	keyLen := 32 // 默认 256 位
	if strings.Contains(cipher, "128") {
		keyLen = 16
	}

	// 使用 UUID 前 N 个字符
	source := strings.ReplaceAll(uuid, "-", "")
	if len(source) > keyLen {
		source = source[:keyLen]
	}

	return base64.StdEncoding.EncodeToString([]byte(source))
}

// formatHysteria2 格式化 Hysteria2 链接
// 格式: hy2://password@server:port?params#name
func (f *V2RayFormatter) formatHysteria2(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	params := url.Values{}

	// SNI
	if node.ServerName != "" {
		params.Set("sni", node.ServerName)
	}

	// 跳过证书验证
	if node.SkipCertVerify {
		params.Set("insecure", "1")
	}

	// Obfs 混淆
	if obfs, ok := node.Settings["obfs"].(string); ok && obfs != "" {
		params.Set("obfs", obfs)
		if obfsPassword, ok := node.Settings["obfs-password"].(string); ok && obfsPassword != "" {
			params.Set("obfs-password", obfsPassword)
		}
	}

	// 上下行速度限制
	if up, ok := node.Settings["up"].(string); ok && up != "" {
		params.Set("up", up)
	}
	if down, ok := node.Settings["down"].(string); ok && down != "" {
		params.Set("down", down)
	}

	return fmt.Sprintf("hy2://%s@%s:%d?%s#%s",
		url.QueryEscape(password), node.Server, node.Port, params.Encode(), url.QueryEscape(node.Name)), nil
}

// formatTUIC 格式化 TUIC 链接
// 格式: tuic://uuid:password@server:port?params#name
func (f *V2RayFormatter) formatTUIC(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	password := node.Password
	if password == "" {
		password = uuid
	}

	params := url.Values{}

	// SNI
	if node.ServerName != "" {
		params.Set("sni", node.ServerName)
	}

	// ALPN
	if node.ALPN != "" {
		params.Set("alpn", node.ALPN)
	}

	// 跳过证书验证
	if node.SkipCertVerify {
		params.Set("insecure", "1")
	}

	// 拥塞控制
	if cc, ok := node.Settings["congestion_control"].(string); ok && cc != "" {
		params.Set("congestion_control", cc)
	}

	// UDP Relay Mode
	if udp, ok := node.Settings["udp_relay_mode"].(string); ok && udp != "" {
		params.Set("udp_relay_mode", udp)
	}

	return fmt.Sprintf("tuic://%s:%s@%s:%d?%s#%s",
		uuid, url.QueryEscape(password), node.Server, node.Port, params.Encode(), url.QueryEscape(node.Name)), nil
}

// formatAnyTLS 格式化 AnyTLS 链接
// AnyTLS 是一种新协议，基于 TLS
// 格式: anytls://password@server:port?params#name
func (f *V2RayFormatter) formatAnyTLS(node *model.ParsedNode, ctx *model.TemplateRenderContext) (string, error) {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	params := url.Values{}

	// SNI
	if node.ServerName != "" {
		params.Set("sni", node.ServerName)
	}

	// TLS 指纹
	if node.TLSFingerprint != "" {
		params.Set("fp", node.TLSFingerprint)
	}

	// 跳过证书验证
	if node.SkipCertVerify {
		params.Set("insecure", "1")
	}

	return fmt.Sprintf("anytls://%s@%s:%d?%s#%s",
		url.QueryEscape(password), node.Server, node.Port, params.Encode(), url.QueryEscape(node.Name)), nil
}

func boolToTLS(b bool) string {
	if b {
		return "tls"
	}
	return ""
}

// ClashFormatter Clash YAML 格式化器
// 支持 Clash Meta (mihomo) 扩展功能
type ClashFormatter struct{}

func (f *ClashFormatter) Name() string {
	return "clash"
}

func (f *ClashFormatter) ContentType() string {
	return "text/yaml; charset=utf-8"
}

func (f *ClashFormatter) FileExtension() string {
	return "yaml"
}

func (f *ClashFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	proxies := make([]map[string]interface{}, 0, len(nodes))
	proxyNames := make([]string, 0, len(nodes))

	for _, node := range nodes {
		proxy := f.buildProxy(node, ctx)
		if proxy != nil {
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, node.Name)
		}
	}

	// 如果没有节点，添加一个 DIRECT
	if len(proxyNames) == 0 {
		proxyNames = []string{"DIRECT"}
	}

	// 构建 Clash Meta 配置
	clash := map[string]interface{}{
		"mixed-port":          7890,
		"allow-lan":           false,
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": "127.0.0.1:9090",
		"proxies":             proxies,
		"proxy-groups": []map[string]interface{}{
			{
				"name":    "🚀 节点选择",
				"type":    "select",
				"proxies": append([]string{"♻️ 自动选择", "DIRECT"}, proxyNames...),
			},
			{
				"name":     "♻️ 自动选择",
				"type":     "url-test",
				"proxies":  proxyNames,
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 300,
				"lazy":     true,
			},
		},
		"rules": []string{
			"DOMAIN-SUFFIX,google.com,🚀 节点选择",
			"DOMAIN-KEYWORD,google,🚀 节点选择",
			"DOMAIN-SUFFIX,ad.com,REJECT",
			"GEOIP,CN,DIRECT",
			"MATCH,🚀 节点选择",
		},
	}

	return yaml.Marshal(clash)
}

func (f *ClashFormatter) buildProxy(node *model.ParsedNode, ctx *model.TemplateRenderContext) map[string]interface{} {
	proxy := map[string]interface{}{
		"name":   node.Name,
		"type":   node.Type,
		"server": node.Server,
		"port":   node.Port,
	}

	switch node.Type {
	case "vmess":
		f.buildVMess(proxy, node, ctx)
	case "vless":
		f.buildVLESS(proxy, node, ctx)
	case "trojan":
		f.buildTrojan(proxy, node, ctx)
	case "shadowsocks", "ss":
		f.buildShadowsocks(proxy, node, ctx)
	case "hysteria2", "hy2":
		f.buildHysteria2(proxy, node, ctx)
	case "tuic":
		f.buildTUIC(proxy, node, ctx)
	case "anytls":
		f.buildAnyTLS(proxy, node, ctx)
	default:
		return nil
	}

	return proxy
}

func (f *ClashFormatter) buildVMess(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	proxy["uuid"] = uuid
	proxy["alterId"] = 0
	if v, ok := node.Settings["alter_id"]; ok {
		proxy["alterId"] = toInt(v)
	}
	proxy["cipher"] = "auto"
	if v, ok := node.Settings["security"].(string); ok && v != "" {
		proxy["cipher"] = v
	}

	// 传输层
	if node.Transport != "" && node.Transport != "tcp" {
		proxy["network"] = node.Transport
	}

	// TLS (VMess 不支持 Reality)
	if node.TLS || node.TLSMode == 1 {
		proxy["tls"] = true
		if node.ServerName != "" {
			proxy["servername"] = node.ServerName
		}
		if node.TLSFingerprint != "" {
			proxy["client-fingerprint"] = node.TLSFingerprint
		}
		if node.ALPN != "" {
			proxy["alpn"] = strings.Split(node.ALPN, ",")
		}
	}
	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}

	f.addTransportOpts(proxy, node)
}

func (f *ClashFormatter) buildVLESS(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	proxy["uuid"] = uuid

	// Flow
	flow := node.Flow
	if flow == "" {
		if f, ok := node.Settings["flow"].(string); ok {
			flow = f
		}
	}
	if flow != "" {
		proxy["flow"] = flow
	}

	// 传输层
	if node.Transport != "" && node.Transport != "tcp" {
		proxy["network"] = node.Transport
	}

	// TLS / Reality
	if node.TLSMode == 2 || node.RealityPublicKey != "" {
		proxy["tls"] = true
		realityOpts := map[string]interface{}{
			"public-key": node.RealityPublicKey,
		}
		if node.RealityShortID != "" {
			realityOpts["short-id"] = node.RealityShortID
		}
		proxy["reality-opts"] = realityOpts

		if node.ServerName != "" {
			proxy["servername"] = node.ServerName
		}
		if node.TLSFingerprint != "" {
			proxy["client-fingerprint"] = node.TLSFingerprint
		}
	} else if node.TLS || node.TLSMode == 1 {
		proxy["tls"] = true
		if node.ServerName != "" {
			proxy["servername"] = node.ServerName
		}
		if node.TLSFingerprint != "" {
			proxy["client-fingerprint"] = node.TLSFingerprint
		}
		if node.ALPN != "" {
			proxy["alpn"] = strings.Split(node.ALPN, ",")
		}
	}

	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}

	f.addTransportOpts(proxy, node)
}

func (f *ClashFormatter) buildTrojan(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	proxy["password"] = password

	// Trojan 强制 TLS
	proxy["tls"] = true
	if node.ServerName != "" {
		proxy["sni"] = node.ServerName
	}
	if node.TLSFingerprint != "" {
		proxy["client-fingerprint"] = node.TLSFingerprint
	}
	if node.ALPN != "" {
		proxy["alpn"] = strings.Split(node.ALPN, ",")
	}
	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}

	// 传输层
	if node.Transport != "" && node.Transport != "tcp" {
		proxy["network"] = node.Transport
	}

	f.addTransportOpts(proxy, node)
}

func (f *ClashFormatter) buildShadowsocks(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	proxy["type"] = "ss"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	proxy["password"] = password

	cipher := node.Cipher
	if cipher == "" {
		if c, ok := node.Settings["cipher"].(string); ok && c != "" {
			cipher = c
		} else {
			cipher = "aes-256-gcm"
		}
	}
	proxy["cipher"] = cipher

	// SS2022 需要配置 server-key
	if strings.HasPrefix(cipher, "2022-blake3-") {
		serverKey := node.ServerKey
		if serverKey == "" {
			if sk, ok := node.Settings["server_key"].(string); ok {
				serverKey = sk
			}
		}
		// Clash Meta 使用 password 字段传递 server:user 密钥对
		if serverKey != "" {
			userKey := generateSS2022UserKey(password, cipher)
			proxy["password"] = serverKey + ":" + userKey
		}
	}

	proxy["udp"] = true
}

func (f *ClashFormatter) buildHysteria2(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	proxy["type"] = "hysteria2"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	proxy["password"] = password

	if node.ServerName != "" {
		proxy["sni"] = node.ServerName
	}
	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}

	// Obfs
	if obfs, ok := node.Settings["obfs"].(string); ok && obfs != "" {
		proxy["obfs"] = obfs
		if obfsPassword, ok := node.Settings["obfs-password"].(string); ok && obfsPassword != "" {
			proxy["obfs-password"] = obfsPassword
		}
	}

	// 速度限制
	if up, ok := node.Settings["up"].(string); ok && up != "" {
		proxy["up"] = up
	}
	if down, ok := node.Settings["down"].(string); ok && down != "" {
		proxy["down"] = down
	}
}

func (f *ClashFormatter) buildTUIC(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	proxy["uuid"] = uuid
	proxy["password"] = node.Password

	if node.ServerName != "" {
		proxy["sni"] = node.ServerName
	}
	if node.ALPN != "" {
		proxy["alpn"] = strings.Split(node.ALPN, ",")
	}
	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}

	// 拥塞控制
	if cc, ok := node.Settings["congestion_control"].(string); ok && cc != "" {
		proxy["congestion-controller"] = cc
	}

	// UDP Relay Mode
	if udp, ok := node.Settings["udp_relay_mode"].(string); ok && udp != "" {
		proxy["udp-relay-mode"] = udp
	}
}

func (f *ClashFormatter) buildAnyTLS(proxy map[string]interface{}, node *model.ParsedNode, ctx *model.TemplateRenderContext) {
	proxy["type"] = "anytls"
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}
	proxy["password"] = password

	if node.ServerName != "" {
		proxy["sni"] = node.ServerName
	}
	if node.TLSFingerprint != "" {
		proxy["client-fingerprint"] = node.TLSFingerprint
	}
	if node.SkipCertVerify {
		proxy["skip-cert-verify"] = true
	}
}

func (f *ClashFormatter) addTransportOpts(proxy map[string]interface{}, node *model.ParsedNode) {
	if node.TransportSettings == nil {
		return
	}

	switch node.Transport {
	case "ws":
		wsOpts := map[string]interface{}{}
		if path, ok := node.TransportSettings["path"].(string); ok {
			wsOpts["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			wsOpts["headers"] = map[string]interface{}{
				"Host": host,
			}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}

	case "grpc":
		grpcOpts := map[string]interface{}{}
		if sn, ok := node.TransportSettings["serviceName"].(string); ok {
			grpcOpts["grpc-service-name"] = sn
		}
		if len(grpcOpts) > 0 {
			proxy["grpc-opts"] = grpcOpts
		}

	case "h2":
		h2Opts := map[string]interface{}{}
		if path, ok := node.TransportSettings["path"].(string); ok {
			h2Opts["path"] = path
		}
		if host, ok := node.TransportSettings["host"].(string); ok {
			h2Opts["host"] = []string{host}
		}
		if len(h2Opts) > 0 {
			proxy["h2-opts"] = h2Opts
		}
	}
}

// SurgeFormatter Surge 格式化器
type SurgeFormatter struct{}

func (f *SurgeFormatter) Name() string {
	return "surge"
}

func (f *SurgeFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *SurgeFormatter) FileExtension() string {
	return "conf"
}

func (f *SurgeFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	var lines []string
	lines = append(lines, "[Proxy]")

	for _, node := range nodes {
		line := f.formatNode(node, ctx)
		if line != "" {
			lines = append(lines, line)
		}
	}

	// 添加一个默认的节点组
	lines = append(lines, "[Proxy Group]")
	var proxyNames []string
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Name)
	}
	if len(proxyNames) > 0 {
		lines = append(lines, "Proxy = select, "+strings.Join(proxyNames, ", "))
	} else {
		lines = append(lines, "Proxy = direct")
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func (f *SurgeFormatter) formatNode(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	var line string
	switch node.Type {
	case "vmess":
		line = f.formatVMess(node, ctx)
	case "vless":
		line = f.formatVLESS(node, ctx)
	case "trojan":
		line = f.formatTrojan(node, ctx)
	case "shadowsocks", "ss":
		line = f.formatShadowsocks(node, ctx)
	default:
		return ""
	}
	if line != "" {
		return fmt.Sprintf("%s = %s", node.Name, line)
	}
	return ""
}

func (f *SurgeFormatter) formatVMess(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}
	// vmess, server, port, username=uuid, ws=true, ws-path=/path, ws-headers=Host:host.com, tls=true, sni=host.com
	parts := []string{
		"vmess",
		node.Server,
		fmt.Sprintf("%d", node.Port),
		fmt.Sprintf("username=%s", uuid),
	}

	// VMess AEAD is enabled by default in Surge
	parts = append(parts, "vmess-aead=true")

	if node.TLS || node.TLSMode == 1 {
		parts = append(parts, "tls=true")
		if node.ServerName != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", node.ServerName))
		}
		if node.SkipCertVerify {
			parts = append(parts, "skip-cert-verify=true")
		}
	}

	switch node.Transport {
	case "ws":
		parts = append(parts, "ws=true")
		if node.TransportSettings != nil {
			if path, ok := node.TransportSettings["path"].(string); ok {
				parts = append(parts, fmt.Sprintf("ws-path=%s", path))
			}
			if host, ok := node.TransportSettings["host"].(string); ok {
				parts = append(parts, fmt.Sprintf("ws-headers=Host:%s", host))
			}
		}
	case "h2":
		parts = append(parts, "http/2=true")
		if node.TransportSettings != nil {
			if path, ok := node.TransportSettings["path"].(string); ok {
				parts = append(parts, fmt.Sprintf("h2-path=%s", path))
			}
			if host, ok := node.TransportSettings["host"].(string); ok {
				parts = append(parts, fmt.Sprintf("h2-host=%s", host))
			}
		}
	}

	return strings.Join(parts, ", ")
}

func (f *SurgeFormatter) formatVLESS(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}

	// vless, server, port, username=uuid, tls=true, sni=host.com
	parts := []string{
		"vless",
		node.Server,
		fmt.Sprintf("%d", node.Port),
		fmt.Sprintf("username=%s", uuid),
	}

	if node.TLSMode == 2 || node.RealityPublicKey != "" {
		parts = append(parts, "tls=true") // Surge uses 'tls' for Reality
		if node.ServerName != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", node.ServerName))
		}
		if node.RealityPublicKey != "" {
			// Surge combines reality-key and short-id into a single 'experimental-reality-key' field
			// For simplicity, we only use the public key here.
			// A more advanced implementation might require combining them if the format standardizes.
			parts = append(parts, fmt.Sprintf("reality-public-key=%s", node.RealityPublicKey))
		}
	} else if node.TLS || node.TLSMode == 1 {
		parts = append(parts, "tls=true")
		if node.ServerName != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", node.ServerName))
		}
	}

	if node.SkipCertVerify {
		parts = append(parts, "skip-cert-verify=true")
	}

	switch node.Transport {
	case "ws":
		parts = append(parts, "ws=true")
		if node.TransportSettings != nil {
			if path, ok := node.TransportSettings["path"].(string); ok {
				parts = append(parts, fmt.Sprintf("ws-path=%s", path))
			}
			if host, ok := node.TransportSettings["host"].(string); ok {
				parts = append(parts, fmt.Sprintf("ws-headers=Host:%s", host))
			}
		}
	case "h2":
		parts = append(parts, "http/2=true")
		if node.TransportSettings != nil {
			if path, ok := node.TransportSettings["path"].(string); ok {
				parts = append(parts, fmt.Sprintf("h2-path=%s", path))
			}
			if host, ok := node.TransportSettings["host"].(string); ok {
				parts = append(parts, fmt.Sprintf("h2-host=%s", host))
			}
		}
	}

	return strings.Join(parts, ", ")
}


func (f *SurgeFormatter) formatTrojan(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	// trojan, server, port, password=password, sni=host.com
	parts := []string{
		"trojan",
		node.Server,
		fmt.Sprintf("%d", node.Port),
		fmt.Sprintf("password=%s", password),
	}

	if node.ServerName != "" {
		parts = append(parts, fmt.Sprintf("sni=%s", node.ServerName))
	}

	if node.SkipCertVerify {
		parts = append(parts, "skip-cert-verify=true")
	}

	return strings.Join(parts, ", ")
}

func (f *SurgeFormatter) formatShadowsocks(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	cipher := "aes-256-gcm"
	if c, ok := node.Settings["cipher"].(string); ok && c != "" {
		cipher = c
	}

	// ss, server, port, encrypt-method=cipher, password=password
	parts := []string{
		"ss",
		node.Server,
		fmt.Sprintf("%d", node.Port),
		fmt.Sprintf("encrypt-method=%s", cipher),
		fmt.Sprintf("password=%s", password),
	}

	return strings.Join(parts, ", ")
}

// JSONFormatter 原始 JSON 格式化器
type JSONFormatter struct{}

func (f *JSONFormatter) Name() string {
	return "json"
}

func (f *JSONFormatter) ContentType() string {
	return "application/json; charset=utf-8"
}

func (f *JSONFormatter) FileExtension() string {
	return "json"
}

func (f *JSONFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	// 注入 UUID
	for _, node := range nodes {
		if node.UUID == "" && ctx != nil {
			node.UUID = ctx.UUID
		}
		if node.Password == "" && ctx != nil {
			node.Password = ctx.UUID
		}
	}

	return json.MarshalIndent(nodes, "", "  ")
}

// Base64JSONFormatter Base64 编码的 JSON 格式化器 (你的自定义格式)
type Base64JSONFormatter struct{}

func (f *Base64JSONFormatter) Name() string {
	return "base64json"
}

func (f *Base64JSONFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *Base64JSONFormatter) FileExtension() string {
	return "txt"
}

func (f *Base64JSONFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	// 按分组组织节点
	grouped := make(map[string][]*model.ParsedNode)
	for _, node := range nodes {
		groupName := node.GroupName
		if groupName == "" {
			groupName = "default"
		}
		grouped[groupName] = append(grouped[groupName], node)
	}

	// 注入用户信息
	for _, nodeList := range grouped {
		for _, node := range nodeList {
			if node.UUID == "" && ctx != nil {
				node.UUID = ctx.UUID
			}
			if node.Password == "" && ctx != nil {
				node.Password = ctx.UUID
			}
		}
	}

	// 构建响应
	response := map[string]interface{}{
		"version": 1,
		"groups":  grouped,
		"user": map[string]interface{}{
			"uuid":            ctx.UUID,
			"expired_at":      ctx.ExpiredAt,
			"speed_limit":     ctx.SpeedLimit,
			"device_limit":    ctx.DeviceLimit,
			"transfer_enable": ctx.TransferEnable,
			"used_traffic":    ctx.UsedTraffic,
		},
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	// Base64 编码
	encoded := base64.StdEncoding.EncodeToString(jsonData)
	return []byte(encoded), nil
}

// ShadowrocketFormatter Shadowrocket 格式化器
type ShadowrocketFormatter struct {
	v2ray *V2RayFormatter
}

func (f *ShadowrocketFormatter) Name() string {
	return "shadowrocket"
}

func (f *ShadowrocketFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *ShadowrocketFormatter) FileExtension() string {
	return "txt"
}

func (f *ShadowrocketFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	// Shadowrocket 兼容 V2Ray 格式
	if f.v2ray == nil {
		f.v2ray = &V2RayFormatter{}
	}
	return f.v2ray.Format(nodes, ctx)
}

// QuantumultXFormatter Quantumult X 格式化器
type QuantumultXFormatter struct{}

func (f *QuantumultXFormatter) Name() string {
	return "quantumultx"
}

func (f *QuantumultXFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *QuantumultXFormatter) FileExtension() string {
	return "txt"
}

func (f *QuantumultXFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	var lines []string

	for _, node := range nodes {
		line := f.formatNode(node, ctx)
		if line != "" {
			lines = append(lines, line)
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func (f *QuantumultXFormatter) formatNode(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	switch node.Type {
	case "vmess":
		return f.formatVMess(node, ctx)
	case "trojan":
		return f.formatTrojan(node, ctx)
	case "shadowsocks", "ss":
		return f.formatShadowsocks(node, ctx)
	default:
		return ""
	}
}

func (f *QuantumultXFormatter) formatVMess(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	uuid := node.UUID
	if uuid == "" && ctx != nil {
		uuid = ctx.UUID
	}

	// vmess=server:port, method=chacha20-poly1305, password=uuid, obfs=ws, obfs-host=host, obfs-uri=path, tag=name
	parts := []string{
		fmt.Sprintf("vmess=%s:%d", node.Server, node.Port),
		fmt.Sprintf("method=%s", "chacha20-poly1305"),
		fmt.Sprintf("password=%s", uuid),
	}

	if node.Transport == "ws" {
		parts = append(parts, "obfs=ws")
		if node.TransportSettings != nil {
			if host, ok := node.TransportSettings["host"].(string); ok {
				parts = append(parts, fmt.Sprintf("obfs-host=%s", host))
			}
			if path, ok := node.TransportSettings["path"].(string); ok {
				parts = append(parts, fmt.Sprintf("obfs-uri=%s", path))
			}
		}
	}

	if node.TLS {
		parts = append(parts, "tls-verification=false")
	}

	parts = append(parts, fmt.Sprintf("tag=%s", node.Name))

	return strings.Join(parts, ", ")
}

func (f *QuantumultXFormatter) formatTrojan(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	// trojan=server:port, password=password, over-tls=true, tls-host=host, tag=name
	parts := []string{
		fmt.Sprintf("trojan=%s:%d", node.Server, node.Port),
		fmt.Sprintf("password=%s", password),
		"over-tls=true",
	}

	if node.ServerName != "" {
		parts = append(parts, fmt.Sprintf("tls-host=%s", node.ServerName))
	}

	parts = append(parts, "tls-verification=false")
	parts = append(parts, fmt.Sprintf("tag=%s", node.Name))

	return strings.Join(parts, ", ")
}

func (f *QuantumultXFormatter) formatShadowsocks(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := node.Password
	if password == "" && ctx != nil {
		password = ctx.UUID
	}

	cipher := "aes-256-gcm"
	if c, ok := node.Settings["cipher"].(string); ok && c != "" {
		cipher = c
	}

	// shadowsocks=server:port, method=cipher, password=password, tag=name
	parts := []string{
		fmt.Sprintf("shadowsocks=%s:%d", node.Server, node.Port),
		fmt.Sprintf("method=%s", cipher),
		fmt.Sprintf("password=%s", password),
		fmt.Sprintf("tag=%s", node.Name),
	}

	return strings.Join(parts, ", ")
}
