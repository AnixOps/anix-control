package service

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
)

// BuildNodeProtocolConfig 从 Node + NodeProtocol 构建下发给节点的完整协议配置。
//
// 这是节点拉配置的**唯一真源**: HTTP UniProxy handler 和 gRPC 两个配置构建函数
// 都调它, 避免"改了一处忘了另一处"(SS2022 server_key 之前就是只在订阅端修了,
// gRPC 节点拿不到)。输出字段名与老 UniProxy JSON 保持一致:
//
//	node_type, type, host, server_port, server_name, tls, network,
//	cipher, server_key, flow, tls_settings, network_settings,
//	send_through, routes, base_config, 以及 custom_config 覆盖的任意字段
//
// SS2022 (2022-blake3-*) 的 server_key 若 Settings 未显式给出, 按老 XBoard 算法
// 从节点 created_at 派生 (子节点用父节点 created_at), 与订阅端 DeriveSS2022ServerKey
// 和节点端 V2bX 三方一致。
func BuildNodeProtocolConfig(node *model.Node, protocol *model.NodeProtocol) map[string]any {
	config := make(map[string]any)

	nodeType := NormalizeNodeType(string(protocol.Type))
	if nodeType == "" {
		nodeType = "vless"
	}

	config["node_type"] = nodeType
	config["type"] = nodeType
	config["server_port"] = protocol.Port

	if protocol.Host != nil && *protocol.Host != "" {
		config["host"] = *protocol.Host
		config["server_name"] = *protocol.Host
	} else {
		config["host"] = node.Host
		config["server_name"] = node.Host
	}

	var protocolConfig map[string]any
	if protocol.Settings != nil && *protocol.Settings != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &protocolConfig); err != nil {
			log.Printf("invalid protocol settings JSON for node %d: %v", node.ID, err)
		}
	}

	config["tls"] = protocol.TLS
	if protocol.TLSSettings != nil && *protocol.TLSSettings != "" {
		var tlsSettings map[string]any
		if err := json.Unmarshal([]byte(*protocol.TLSSettings), &tlsSettings); err != nil {
			log.Printf("invalid TLS settings JSON for node %d: %v", node.ID, err)
		} else {
			config["tls_settings"] = tlsSettings
		}
	}

	if protocol.Transport != nil && *protocol.Transport != "" {
		config["network"] = *protocol.Transport
	} else {
		config["network"] = "tcp"
	}
	if protocol.TransportSettings != nil && *protocol.TransportSettings != "" {
		var transportSettings map[string]any
		if err := json.Unmarshal([]byte(*protocol.TransportSettings), &transportSettings); err != nil {
			log.Printf("invalid transport settings JSON for node %d: %v", node.ID, err)
		} else {
			config["network_settings"] = transportSettings
		}
	}

	switch nodeType {
	case "vless":
		config["flow"] = configValue(protocolConfig, "flow", "")
	case "wireguard":
		config["cidr"] = configValue(protocolConfig, "cidr", "10.66.0.0/24")
		config["server_address"] = configValue(protocolConfig, "server_address", "10.66.0.1/24")
		config["server_private_key"] = configValue(protocolConfig, "server_private_key", "")
		config["server_public_key"] = configValue(protocolConfig, "server_public_key", "")
		config["mtu"] = configValue(protocolConfig, "mtu", 1280)
		config["dns"] = configValue(protocolConfig, "dns", []string{"1.1.1.1", "8.8.8.8"})
		config["allowed_ips"] = configValue(protocolConfig, "allowed_ips", []string{"0.0.0.0/0", "::/0"})
		config["tunnel_type"] = configValue(protocolConfig, "tunnel_type", "quic")
		config["relay"] = configValue(protocolConfig, "relay", map[string]any{
			"backend":           "gost",
			"mode":              "relay+quic",
			"role":              "entry",
			"wss_compat":        false,
			"exit_nat":          true,
			"entry_stats":       true,
			"server":            "",
			"server_port":       0,
			"tun_port":          8421,
			"entry_tun_address": "172.31.66.2/24",
			"exit_tun_address":  "172.31.66.1/24",
			"outbound_iface":    "",
			"routing_table":     0,
			"routing_priority":  0,
		})
	case "shadowsocks":
		cipherStr := "aes-256-gcm"
		if c, ok := protocolConfig["cipher"].(string); ok && c != "" {
			cipherStr = c
			config["cipher"] = c
		} else if m, ok := protocolConfig["method"].(string); ok && m != "" {
			cipherStr = m
			config["cipher"] = m
		} else {
			config["cipher"] = cipherStr
		}
		if serverKey, ok := protocolConfig["server_key"]; ok {
			config["server_key"] = serverKey
		} else if strings.HasPrefix(cipherStr, "2022-blake3-") {
			// SS2022: 老库不存 server_key, 从节点 created_at 派生。子节点用父节点
			// created_at, 与 XBoard Server::generateServerPassword 一致。
			createdAt := node.CreatedAt
			if node.ParentID != nil {
				var parentCreatedAt = lookupParentCreatedAt(*node.ParentID)
				if !parentCreatedAt.IsZero() {
					createdAt = parentCreatedAt
				}
			}
			config["server_key"] = DeriveSS2022ServerKey(createdAt, cipherStr)
		}
	}

	// Reality (TLS=2): 把 reality settings 合进 tls_settings, 客户端从 tls_settings
	// 里读 private_key/short_id/dest/server_port/server_name。
	if protocol.TLS == 2 && protocol.RealitySettings != nil && *protocol.RealitySettings != "" {
		var realitySettings map[string]any
		if err := json.Unmarshal([]byte(*protocol.RealitySettings), &realitySettings); err == nil {
			tlsSettings, _ := config["tls_settings"].(map[string]any)
			if tlsSettings == nil {
				tlsSettings = make(map[string]any)
				config["tls_settings"] = tlsSettings
			}
			for k, v := range realitySettings {
				tlsSettings[k] = v
			}
		}
	}

	// 自定义配置全量覆盖 (优先级最高)
	if protocol.CustomConfig != nil && *protocol.CustomConfig != "" {
		var customConfig map[string]any
		if err := json.Unmarshal([]byte(*protocol.CustomConfig), &customConfig); err == nil {
			for k, v := range customConfig {
				config[k] = v
			}
		}
	}

	config["send_through"] = "0.0.0.0"
	config["routes"] = []any{}
	config["base_config"] = map[string]any{
		"push_interval": 60,
		"pull_interval": 60,
	}

	return config
}

// NormalizeNodeType 归一化节点类型别名 (v2ray/vmess-aead → vmess)。
func NormalizeNodeType(nodeType string) string {
	t := strings.ToLower(strings.TrimSpace(nodeType))
	switch t {
	case "v2ray", "vmess-aead", "vmessaead":
		return "vmess"
	default:
		return t
	}
}

// lookupParentCreatedAt 查父节点 created_at (给子节点 SS2022 server_key 派生用)。
func lookupParentCreatedAt(parentID uint) time.Time {
	var createdAt time.Time
	database.Get().Model(&model.Node{}).Where("id = ?", parentID).
		Select("created_at").Scan(&createdAt)
	return createdAt
}

func configValue(config map[string]any, key string, defaultValue any) any {
	if config == nil {
		return defaultValue
	}
	if val, ok := config[key]; ok {
		return val
	}
	return defaultValue
}

// StringifyConfigMap 把 map[string]any 转成 map[string]string (proto 的
// map<string,string> 字段需要)。数字按整数/浮点合理格式化, 避免 443 变 "443.000000"。
func StringifyConfigMap(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = stringifyValue(v)
	}
	return out
}

func stringifyValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		// JSON 数字统一解成 float64; 整数值不带小数点。
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		// map/slice 等复杂值序列化成 JSON 字符串。
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
