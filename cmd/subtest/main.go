package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TestNode 测试节点配置
type TestNode struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Server   string `json:"server" yaml:"server"`
	Port     int    `json:"port" yaml:"port"`
	UUID     string `json:"uuid" yaml:"uuid"`
	Flow     string `json:"flow,omitempty" yaml:"flow,omitempty"`
	Security string `json:"security,omitempty" yaml:"security,omitempty"`

	// TLS
	TLS            bool   `json:"tls,omitempty" yaml:"tls,omitempty"`
	ServerName     string `json:"servername,omitempty" yaml:"servername,omitempty"`
	SkipCertVerify bool   `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	Fingerprint    string `json:"client-fingerprint,omitempty" yaml:"client-fingerprint,omitempty"`

	// Reality
	RealityOpts *RealityOpts `json:"reality-opts,omitempty" yaml:"reality-opts,omitempty"`

	// Transport
	Network  string    `json:"network,omitempty" yaml:"network,omitempty"`
	WSOpts   *WSOpts   `json:"ws-opts,omitempty" yaml:"ws-opts,omitempty"`
	GRPCOpts *GRPCOpts `json:"grpc-opts,omitempty" yaml:"grpc-opts,omitempty"`
}

type RealityOpts struct {
	PublicKey string `json:"public-key" yaml:"public-key"`
	ShortID   string `json:"short-id" yaml:"short-id"`
}

type WSOpts struct {
	Path    string            `json:"path,omitempty" yaml:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

type GRPCOpts struct {
	ServiceName string `json:"grpc-service-name,omitempty" yaml:"grpc-service-name,omitempty"`
}

// TestConfig 测试配置文件格式
type TestConfig struct {
	Groups map[string][]TestNode `json:"groups" yaml:"groups"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "gen":
		generateSampleConfig()
	case "sub":
		if len(os.Args) < 3 {
			fmt.Println("用法: subtest sub <config.yaml> [format]")
			fmt.Println("格式: v2ray (默认), clash, json, base64json")
			return
		}
		format := "v2ray"
		if len(os.Args) >= 4 {
			format = os.Args[3]
		}
		generateSubscription(os.Args[2], format)
	case "parse":
		if len(os.Args) < 3 {
			fmt.Println("用法: subtest parse <base64_content>")
			return
		}
		parseBase64(os.Args[2])
	case "link":
		if len(os.Args) < 3 {
			fmt.Println("用法: subtest link <v2ray_link>")
			return
		}
		parseLink(os.Args[2])
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("订阅测试工具")
	fmt.Println("")
	fmt.Println("用法:")
	fmt.Println("  subtest gen                      - 生成示例配置文件")
	fmt.Println("  subtest sub <config.yaml> [fmt]  - 从配置生成订阅")
	fmt.Println("  subtest parse <base64>           - 解析 Base64 订阅")
	fmt.Println("  subtest link <v2ray_link>        - 解析单个链接")
	fmt.Println("")
	fmt.Println("格式 (fmt):")
	fmt.Println("  v2ray     - Base64 编码的链接列表 (默认)")
	fmt.Println("  clash     - Clash YAML 配置")
	fmt.Println("  json      - 原始 JSON")
	fmt.Println("  base64json - Base64 编码的分组 JSON")
}

func generateSampleConfig() {
	config := TestConfig{
		Groups: map[string][]TestNode{
			"default": {
				{
					Name:     "本地测试-TCP",
					Type:     "vless",
					Server:   "127.0.0.1",
					Port:     19999,
					UUID:     "4d7ccb2c-7c21-4003-bf45-47408593e4e4",
					Security: "none",
					Network:  "tcp",
				},
				{
					Name:        "美国节点-Reality",
					Type:        "vless",
					Server:      "us.example.com",
					Port:        443,
					UUID:        "{{UUID}}",
					Flow:        "xtls-rprx-vision",
					Network:     "tcp",
					TLS:         true,
					ServerName:  "www.microsoft.com",
					Fingerprint: "chrome",
					RealityOpts: &RealityOpts{
						PublicKey: "your-public-key-here",
						ShortID:   "abcd1234",
					},
				},
				{
					Name:       "日本节点-WS",
					Type:       "vless",
					Server:     "jp.example.com",
					Port:       443,
					UUID:       "{{UUID}}",
					Network:    "ws",
					TLS:        true,
					ServerName: "jp.example.com",
					WSOpts: &WSOpts{
						Path: "/ws",
						Headers: map[string]string{
							"Host": "jp.example.com",
						},
					},
				},
			},
			"vip": {
				{
					Name:       "VIP香港-Hysteria2",
					Type:       "hysteria2",
					Server:     "hk.example.com",
					Port:       443,
					UUID:       "{{UUID}}",
					TLS:        true,
					ServerName: "hk.example.com",
				},
			},
		},
	}

	data, _ := yaml.Marshal(config)
	filename := "test_nodes.yaml"
	os.WriteFile(filename, data, 0644)
	fmt.Printf("已生成示例配置: %s\n", filename)
	fmt.Println("")
	fmt.Println(string(data))
}

func generateSubscription(configFile, format string) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Printf("读取配置失败: %v\n", err)
		return
	}

	var config TestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		fmt.Printf("解析配置失败: %v\n", err)
		return
	}

	// 模拟用户 UUID
	userUUID := "test-user-uuid-1234-5678-abcdef"

	switch format {
	case "v2ray":
		generateV2RaySubscription(config, userUUID)
	case "clash":
		generateClashSubscription(config, userUUID)
	case "json":
		generateJSONSubscription(config, userUUID)
	case "base64json":
		generateBase64JSONSubscription(config, userUUID)
	default:
		fmt.Printf("未知格式: %s\n", format)
	}
}

func generateV2RaySubscription(config TestConfig, userUUID string) {
	var links []string

	for _, nodes := range config.Groups {
		for _, node := range nodes {
			uuid := node.UUID
			if uuid == "{{UUID}}" {
				uuid = userUUID
			}

			link := generateV2RayLink(node, uuid)
			if link != "" {
				links = append(links, link)
			}
		}
	}

	content := strings.Join(links, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	fmt.Println("=== V2Ray 订阅 (Base64) ===")
	fmt.Println(encoded)
	fmt.Println("")
	fmt.Println("=== 解码后的链接 ===")
	fmt.Println(content)
}

func generateV2RayLink(node TestNode, uuid string) string {
	switch node.Type {
	case "vless":
		return generateVLESSLink(node, uuid)
	case "vmess":
		return generateVMessLink(node, uuid)
	case "trojan":
		return generateTrojanLink(node, uuid)
	case "hysteria2", "hy2":
		return generateHysteria2Link(node, uuid)
	default:
		return ""
	}
}

func generateVLESSLink(node TestNode, uuid string) string {
	// vless://uuid@server:port?params#name
	params := []string{}

	// encryption
	params = append(params, "encryption=none")

	// security
	if node.RealityOpts != nil {
		params = append(params, "security=reality")
		params = append(params, "pbk="+node.RealityOpts.PublicKey)
		params = append(params, "sid="+node.RealityOpts.ShortID)
		if node.ServerName != "" {
			params = append(params, "sni="+node.ServerName)
		}
		if node.Fingerprint != "" {
			params = append(params, "fp="+node.Fingerprint)
		}
	} else if node.TLS {
		params = append(params, "security=tls")
		if node.ServerName != "" {
			params = append(params, "sni="+node.ServerName)
		}
	} else {
		params = append(params, "security=none")
	}

	// flow
	if node.Flow != "" {
		params = append(params, "flow="+node.Flow)
	}

	// transport
	network := node.Network
	if network == "" {
		network = "tcp"
	}
	params = append(params, "type="+network)

	if network == "ws" && node.WSOpts != nil {
		if node.WSOpts.Path != "" {
			params = append(params, "path="+node.WSOpts.Path)
		}
		if host, ok := node.WSOpts.Headers["Host"]; ok {
			params = append(params, "host="+host)
		}
	}

	if network == "grpc" && node.GRPCOpts != nil {
		if node.GRPCOpts.ServiceName != "" {
			params = append(params, "serviceName="+node.GRPCOpts.ServiceName)
		}
	}

	if network == "tcp" {
		params = append(params, "headerType=none")
	}

	link := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		uuid,
		node.Server,
		node.Port,
		strings.Join(params, "&"),
		node.Name,
	)

	return link
}

func generateVMessLink(node TestNode, uuid string) string {
	vmessConfig := map[string]interface{}{
		"v":    "2",
		"ps":   node.Name,
		"add":  node.Server,
		"port": node.Port,
		"id":   uuid,
		"aid":  0,
		"scy":  "auto",
		"net":  node.Network,
		"type": "none",
		"tls":  "",
	}

	if node.TLS {
		vmessConfig["tls"] = "tls"
		if node.ServerName != "" {
			vmessConfig["sni"] = node.ServerName
		}
	}

	if node.Network == "ws" && node.WSOpts != nil {
		vmessConfig["path"] = node.WSOpts.Path
		if host, ok := node.WSOpts.Headers["Host"]; ok {
			vmessConfig["host"] = host
		}
	}

	jsonData, _ := json.Marshal(vmessConfig)
	encoded := base64.StdEncoding.EncodeToString(jsonData)
	return "vmess://" + encoded
}

func generateTrojanLink(node TestNode, uuid string) string {
	params := []string{}

	if node.ServerName != "" {
		params = append(params, "sni="+node.ServerName)
	}

	network := node.Network
	if network == "" {
		network = "tcp"
	}
	params = append(params, "type="+network)

	paramStr := ""
	if len(params) > 0 {
		paramStr = "?" + strings.Join(params, "&")
	}

	return fmt.Sprintf("trojan://%s@%s:%d%s#%s",
		uuid,
		node.Server,
		node.Port,
		paramStr,
		node.Name,
	)
}

func generateHysteria2Link(node TestNode, uuid string) string {
	params := []string{}

	if node.ServerName != "" {
		params = append(params, "sni="+node.ServerName)
	}

	if node.SkipCertVerify {
		params = append(params, "insecure=1")
	}

	paramStr := ""
	if len(params) > 0 {
		paramStr = "?" + strings.Join(params, "&")
	}

	return fmt.Sprintf("hy2://%s@%s:%d%s#%s",
		uuid,
		node.Server,
		node.Port,
		paramStr,
		node.Name,
	)
}

func generateClashSubscription(config TestConfig, userUUID string) {
	clashConfig := map[string]interface{}{
		"mixed-port":          7890,
		"allow-lan":           false,
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": ":9090",
	}

	var proxies []map[string]interface{}
	var proxyNames []string

	for _, nodes := range config.Groups {
		for _, node := range nodes {
			uuid := node.UUID
			if uuid == "{{UUID}}" {
				uuid = userUUID
			}

			proxy := map[string]interface{}{
				"name":   node.Name,
				"type":   node.Type,
				"server": node.Server,
				"port":   node.Port,
				"uuid":   uuid,
			}

			if node.Network != "" {
				proxy["network"] = node.Network
			}

			if node.TLS {
				proxy["tls"] = true
				if node.ServerName != "" {
					proxy["servername"] = node.ServerName
				}
				if node.Fingerprint != "" {
					proxy["client-fingerprint"] = node.Fingerprint
				}
			}

			if node.Flow != "" {
				proxy["flow"] = node.Flow
			}

			if node.RealityOpts != nil {
				proxy["reality-opts"] = map[string]string{
					"public-key": node.RealityOpts.PublicKey,
					"short-id":   node.RealityOpts.ShortID,
				}
			}

			if node.WSOpts != nil {
				proxy["ws-opts"] = node.WSOpts
			}

			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, node.Name)
		}
	}

	clashConfig["proxies"] = proxies
	clashConfig["proxy-groups"] = []map[string]interface{}{
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
		},
	}

	clashConfig["rules"] = []string{
		"GEOIP,CN,DIRECT",
		"MATCH,🚀 节点选择",
	}

	yamlData, _ := yaml.Marshal(clashConfig)
	fmt.Println("=== Clash 订阅 (YAML) ===")
	fmt.Println(string(yamlData))
}

func generateJSONSubscription(config TestConfig, userUUID string) {
	result := map[string]interface{}{
		"version": 1,
		"groups":  config.Groups,
		"user": map[string]interface{}{
			"uuid":            userUUID,
			"expired_at":      1735689600,
			"speed_limit":     0,
			"device_limit":    3,
			"transfer_enable": 107374182400,
			"used_traffic":    0,
		},
	}

	// 替换 UUID
	for groupName, nodes := range config.Groups {
		for i, node := range nodes {
			if node.UUID == "{{UUID}}" {
				config.Groups[groupName][i].UUID = userUUID
			}
		}
	}
	result["groups"] = config.Groups

	jsonData, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println("=== JSON 订阅 ===")
	fmt.Println(string(jsonData))
}

func generateBase64JSONSubscription(config TestConfig, userUUID string) {
	// 替换 UUID
	for groupName, nodes := range config.Groups {
		for i, node := range nodes {
			if node.UUID == "{{UUID}}" {
				config.Groups[groupName][i].UUID = userUUID
			}
		}
	}

	result := map[string]interface{}{
		"version": 1,
		"groups":  config.Groups,
		"user": map[string]interface{}{
			"uuid":            userUUID,
			"expired_at":      1735689600,
			"speed_limit":     0,
			"device_limit":    3,
			"transfer_enable": 107374182400,
			"used_traffic":    0,
		},
	}

	jsonData, _ := json.Marshal(result)
	encoded := base64.StdEncoding.EncodeToString(jsonData)

	fmt.Println("=== Base64 JSON 订阅 ===")
	fmt.Println(encoded)
	fmt.Println("")
	fmt.Println("=== 解码后的 JSON ===")
	prettyJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(prettyJSON))
}

func parseBase64(content string) {
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(content)
		if err != nil {
			fmt.Printf("Base64 解码失败: %v\n", err)
			return
		}
	}

	fmt.Println("=== 解码结果 ===")
	fmt.Println(string(decoded))
}

func parseLink(link string) {
	fmt.Println("=== 链接解析 ===")
	fmt.Printf("原始链接: %s\n\n", link)

	if strings.HasPrefix(link, "vless://") {
		parseVLESSLink(link)
	} else if strings.HasPrefix(link, "vmess://") {
		parseVMessLink(link)
	} else if strings.HasPrefix(link, "trojan://") {
		parseTrojanLink(link)
	} else if strings.HasPrefix(link, "ss://") {
		fmt.Println("Shadowsocks 链接")
	} else if strings.HasPrefix(link, "hy2://") || strings.HasPrefix(link, "hysteria2://") {
		fmt.Println("Hysteria2 链接")
	} else {
		fmt.Println("未知链接格式")
	}
}

func parseVLESSLink(link string) {
	// vless://uuid@server:port?params#name
	link = strings.TrimPrefix(link, "vless://")

	// 分离名称
	parts := strings.SplitN(link, "#", 2)
	name := ""
	if len(parts) == 2 {
		name = parts[1]
	}
	link = parts[0]

	// 分离参数
	parts = strings.SplitN(link, "?", 2)
	params := ""
	if len(parts) == 2 {
		params = parts[1]
	}
	link = parts[0]

	// 分离 uuid@server:port
	parts = strings.SplitN(link, "@", 2)
	uuid := parts[0]
	serverPort := ""
	if len(parts) == 2 {
		serverPort = parts[1]
	}

	fmt.Println("协议: VLESS")
	fmt.Printf("UUID: %s\n", uuid)
	fmt.Printf("服务器: %s\n", serverPort)
	fmt.Printf("名称: %s\n", name)
	fmt.Println("\n参数:")

	for _, param := range strings.Split(params, "&") {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			fmt.Printf("  %s = %s\n", kv[0], kv[1])
		}
	}
}

func parseVMessLink(link string) {
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fmt.Printf("VMess Base64 解码失败: %v\n", err)
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(decoded, &config); err != nil {
		fmt.Printf("VMess JSON 解析失败: %v\n", err)
		return
	}

	fmt.Println("协议: VMess")
	prettyJSON, _ := json.MarshalIndent(config, "", "  ")
	fmt.Println(string(prettyJSON))
}

func parseTrojanLink(link string) {
	fmt.Println("协议: Trojan")
	// 简化解析
	link = strings.TrimPrefix(link, "trojan://")
	fmt.Printf("内容: %s\n", link)
}
