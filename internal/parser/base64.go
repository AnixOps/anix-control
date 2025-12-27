package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/model"
)

// Base64Parser Base64 编码的 V2Ray 订阅解析器
type Base64Parser struct{}

func (p *Base64Parser) Name() string {
	return "base64"
}

func (p *Base64Parser) Detect(content []byte) bool {
	// 尝试 Base64 解码
	str := strings.TrimSpace(string(content))
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(str)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(str)
	}
	if err != nil {
		return false
	}

	// 检查是否包含协议链接
	decodedStr := string(decoded)
	return strings.Contains(decodedStr, "vmess://") ||
		strings.Contains(decodedStr, "vless://") ||
		strings.Contains(decodedStr, "trojan://") ||
		strings.Contains(decodedStr, "ss://") ||
		strings.Contains(decodedStr, "ssr://") ||
		strings.Contains(decodedStr, "hy2://") ||
		strings.Contains(decodedStr, "hysteria2://")
}

func (p *Base64Parser) Parse(content []byte) ([]*model.ParsedNode, error) {
	// Base64 解码
	str := strings.TrimSpace(string(content))
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(str)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(str)
	}
	if err != nil {
		// 可能本身就不是 Base64，直接使用原始内容
		decoded = content
	}

	// 按行分割
	lines := strings.Split(string(decoded), "\n")
	var nodes []*model.ParsedNode

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var node *model.ParsedNode
		var parseErr error

		switch {
		case strings.HasPrefix(line, "vmess://"):
			node, parseErr = p.parseVMess(line)
		case strings.HasPrefix(line, "vless://"):
			node, parseErr = p.parseVLESS(line)
		case strings.HasPrefix(line, "trojan://"):
			node, parseErr = p.parseTrojan(line)
		case strings.HasPrefix(line, "ss://"):
			node, parseErr = p.parseShadowsocks(line)
		case strings.HasPrefix(line, "hy2://"), strings.HasPrefix(line, "hysteria2://"):
			node, parseErr = p.parseHysteria2(line)
		case strings.HasPrefix(line, "tuic://"):
			node, parseErr = p.parseTUIC(line)
		}

		if parseErr == nil && node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// parseVMess 解析 vmess:// 链接
func (p *Base64Parser) parseVMess(link string) (*model.ParsedNode, error) {
	// vmess://base64(json)
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
	}

	var vmess struct {
		V    interface{} `json:"v"`
		Ps   string      `json:"ps"`
		Add  string      `json:"add"`
		Port interface{} `json:"port"`
		ID   string      `json:"id"`
		Aid  interface{} `json:"aid"`
		Scy  string      `json:"scy"`
		Net  string      `json:"net"`
		Type string      `json:"type"`
		Host string      `json:"host"`
		Path string      `json:"path"`
		TLS  string      `json:"tls"`
		Sni  string      `json:"sni"`
		Alpn string      `json:"alpn"`
		Fp   string      `json:"fp"`
	}

	if err := json.Unmarshal(decoded, &vmess); err != nil {
		return nil, err
	}

	port := toInt(vmess.Port)
	aid := toInt(vmess.Aid)

	node := &model.ParsedNode{
		Name:           vmess.Ps,
		Type:           "vmess",
		Server:         vmess.Add,
		Port:           port,
		UUID:           vmess.ID,
		TLS:            vmess.TLS == "tls",
		TLSFingerprint: vmess.Fp,
		ServerName:     vmess.Sni,
		ALPN:           vmess.Alpn,
		Transport:      vmess.Net,
		Settings: map[string]interface{}{
			"alter_id": aid,
			"security": vmess.Scy,
		},
	}

	// 传输层配置
	if vmess.Net != "" && vmess.Net != "tcp" {
		node.TransportSettings = map[string]interface{}{}
		switch vmess.Net {
		case "ws":
			node.TransportSettings["path"] = vmess.Path
			node.TransportSettings["host"] = vmess.Host
		case "grpc":
			node.TransportSettings["serviceName"] = vmess.Path
		case "h2":
			node.TransportSettings["path"] = vmess.Path
			node.TransportSettings["host"] = vmess.Host
		}
	}

	return node, nil
}

// parseVLESS 解析 vless:// 链接
func (p *Base64Parser) parseVLESS(link string) (*model.ParsedNode, error) {
	// vless://uuid@server:port?params#name
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(u.Port())
	name, _ := url.QueryUnescape(u.Fragment)

	query := u.Query()

	node := &model.ParsedNode{
		Name:           name,
		Type:           "vless",
		Server:         u.Hostname(),
		Port:           port,
		UUID:           u.User.Username(),
		TLS:            query.Get("security") == "tls" || query.Get("security") == "reality",
		TLSFingerprint: query.Get("fp"),
		ServerName:     query.Get("sni"),
		ALPN:           query.Get("alpn"),
		Transport:      query.Get("type"),
		Settings:       map[string]interface{}{},
	}

	// Flow
	if flow := query.Get("flow"); flow != "" {
		node.Settings["flow"] = flow
	}

	// Reality
	if query.Get("security") == "reality" {
		node.RealityPublicKey = query.Get("pbk")
		node.RealityShortID = query.Get("sid")
	}

	// 传输层配置
	switch node.Transport {
	case "ws":
		node.TransportSettings = map[string]interface{}{
			"path": query.Get("path"),
			"host": query.Get("host"),
		}
	case "grpc":
		node.TransportSettings = map[string]interface{}{
			"serviceName": query.Get("serviceName"),
			"mode":        query.Get("mode"),
		}
	case "h2":
		node.TransportSettings = map[string]interface{}{
			"path": query.Get("path"),
			"host": query.Get("host"),
		}
	}

	return node, nil
}

// parseTrojan 解析 trojan:// 链接
func (p *Base64Parser) parseTrojan(link string) (*model.ParsedNode, error) {
	// trojan://password@server:port?params#name
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(u.Port())
	name, _ := url.QueryUnescape(u.Fragment)
	password := u.User.Username()

	query := u.Query()

	node := &model.ParsedNode{
		Name:           name,
		Type:           "trojan",
		Server:         u.Hostname(),
		Port:           port,
		Password:       password,
		TLS:            true,
		TLSFingerprint: query.Get("fp"),
		ServerName:     query.Get("sni"),
		ALPN:           query.Get("alpn"),
		Transport:      query.Get("type"),
		Settings:       map[string]interface{}{},
	}

	if query.Get("allowInsecure") == "1" {
		node.SkipCertVerify = true
	}

	// 传输层配置
	switch node.Transport {
	case "ws":
		node.TransportSettings = map[string]interface{}{
			"path": query.Get("path"),
			"host": query.Get("host"),
		}
	case "grpc":
		node.TransportSettings = map[string]interface{}{
			"serviceName": query.Get("serviceName"),
		}
	}

	return node, nil
}

// parseShadowsocks 解析 ss:// 链接
func (p *Base64Parser) parseShadowsocks(link string) (*model.ParsedNode, error) {
	// ss://base64(method:password)@server:port#name
	// 或 ss://base64(method:password@server:port)#name
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	name, _ := url.QueryUnescape(u.Fragment)

	var server string
	var port int
	var method, password string

	if u.User != nil && u.Host != "" {
		// 格式: ss://base64(method:password)@server:port#name
		server = u.Hostname()
		port, _ = strconv.Atoi(u.Port())

		userInfo := u.User.String()
		decoded, err := base64.RawURLEncoding.DecodeString(userInfo)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(userInfo)
		}
		if err != nil {
			decoded = []byte(userInfo)
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			method = parts[0]
			password = parts[1]
		}
	} else {
		// 格式: ss://base64(method:password@server:port)#name
		encoded := strings.TrimPrefix(link, "ss://")
		if idx := strings.Index(encoded, "#"); idx != -1 {
			encoded = encoded[:idx]
		}

		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(encoded)
		}
		if err != nil {
			return nil, err
		}

		// 解析 method:password@server:port
		decodedStr := string(decoded)
		atIdx := strings.LastIndex(decodedStr, "@")
		if atIdx == -1 {
			return nil, fmt.Errorf("invalid ss link format")
		}

		userPart := decodedStr[:atIdx]
		hostPart := decodedStr[atIdx+1:]

		parts := strings.SplitN(userPart, ":", 2)
		if len(parts) == 2 {
			method = parts[0]
			password = parts[1]
		}

		hostParts := strings.Split(hostPart, ":")
		if len(hostParts) >= 2 {
			server = hostParts[0]
			port, _ = strconv.Atoi(hostParts[1])
		}
	}

	node := &model.ParsedNode{
		Name:     name,
		Type:     "shadowsocks",
		Server:   server,
		Port:     port,
		Password: password,
		Settings: map[string]interface{}{
			"cipher": method,
		},
	}

	return node, nil
}

// parseHysteria2 解析 hy2:// 或 hysteria2:// 链接
func (p *Base64Parser) parseHysteria2(link string) (*model.ParsedNode, error) {
	// hy2://password@server:port?params#name
	link = strings.Replace(link, "hysteria2://", "hy2://", 1)
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(u.Port())
	name, _ := url.QueryUnescape(u.Fragment)
	password := u.User.Username()

	query := u.Query()

	node := &model.ParsedNode{
		Name:       name,
		Type:       "hysteria2",
		Server:     u.Hostname(),
		Port:       port,
		Password:   password,
		TLS:        true,
		ServerName: query.Get("sni"),
		Settings:   map[string]interface{}{},
	}

	if query.Get("insecure") == "1" {
		node.SkipCertVerify = true
	}

	if obfs := query.Get("obfs"); obfs != "" {
		node.Settings["obfs"] = obfs
		node.Settings["obfs-password"] = query.Get("obfs-password")
	}

	return node, nil
}

// parseTUIC 解析 tuic:// 链接
func (p *Base64Parser) parseTUIC(link string) (*model.ParsedNode, error) {
	// tuic://uuid:password@server:port?params#name
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(u.Port())
	name, _ := url.QueryUnescape(u.Fragment)

	password, _ := u.User.Password()
	uuid := u.User.Username()

	query := u.Query()

	node := &model.ParsedNode{
		Name:       name,
		Type:       "tuic",
		Server:     u.Hostname(),
		Port:       port,
		UUID:       uuid,
		Password:   password,
		TLS:        true,
		ServerName: query.Get("sni"),
		ALPN:       query.Get("alpn"),
		Settings: map[string]interface{}{
			"congestion_control": query.Get("congestion_control"),
			"udp_relay_mode":     query.Get("udp_relay_mode"),
		},
	}

	return node, nil
}

// toInt 将 interface{} 转换为 int
func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	case json.Number:
		i, _ := val.Int64()
		return int(i)
	}
	return 0
}
