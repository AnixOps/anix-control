package service

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/anixops/v2board/internal/model"
)

const defaultWireGuardServerAddress = "10.66.0.1/24"

var ErrInvalidNodeProtocol = errors.New("invalid node protocol")

// ValidateNodeProtocol validates the fields that affect node runtime. The
// existing protocol API accepts JSON strings for protocol-specific settings,
// so validation belongs in the service layer and covers both HTTP and gRPC
// callers that persist protocols.
func ValidateNodeProtocol(protocol *model.NodeProtocol) error {
	if protocol == nil {
		return errors.New("协议不能为空")
	}
	if strings.EqualFold(strings.TrimSpace(string(protocol.Type)), string(model.ProtocolWireGuard)) {
		if err := validateWireGuardProtocol(protocol); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidNodeProtocol, err)
		}
	}
	return nil
}

// ValidateWireGuardRuntimeConfig validates a raw node config after it has been
// decoded from JSON. Raw configs bypass NodeProtocol persistence, so they need
// the same runtime guard before UniProxy returns them to a node.
func ValidateWireGuardRuntimeConfig(config map[string]any) error {
	if config == nil {
		return nil
	}
	typeValue := strings.ToLower(strings.TrimSpace(stringSetting(config, "type", "")))
	nodeTypeValue := strings.ToLower(strings.TrimSpace(stringSetting(config, "node_type", "")))
	if typeValue != "" && typeValue != string(model.ProtocolWireGuard) {
		if nodeTypeValue == string(model.ProtocolWireGuard) {
			return errors.New("WireGuard raw_config 的 type 必须为 wireguard")
		}
		return nil
	}
	if nodeTypeValue != "" && nodeTypeValue != string(model.ProtocolWireGuard) {
		return errors.New("WireGuard raw_config 的 node_type 必须为 wireguard")
	}
	if typeValue != string(model.ProtocolWireGuard) && nodeTypeValue != string(model.ProtocolWireGuard) {
		return nil
	}

	settings := make(map[string]any)
	for _, key := range []string{
		"cidr", "server_address", "server_private_key", "server_public_key", "public_key",
		"mtu", "dns", "allowed_ips", "tunnel_type", "relay",
	} {
		if value, ok := config[key]; ok {
			settings[key] = value
		}
	}
	protocol := &model.NodeProtocol{
		Type:   model.ProtocolWireGuard,
		Port:   intSetting(config, "server_port", 0),
		Enable: 1,
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("WireGuard raw_config 无法编码: %w", err)
	}
	settingsJSON := string(encoded)
	protocol.Settings = &settingsJSON
	return ValidateNodeProtocol(protocol)
}

func validateWireGuardProtocol(protocol *model.NodeProtocol) error {
	settings, err := decodeWireGuardSettings(protocol.Settings)
	if err != nil {
		return err
	}
	if err := validateWireGuardCustomConfig(protocol.CustomConfig); err != nil {
		return err
	}
	role := wireGuardRelayRole(settings)
	if role != "exit" && (protocol.Port < 1 || protocol.Port > 65535) {
		return errors.New("WireGuard 端口必须在 1-65535 之间")
	}

	cidr := stringSetting(settings, "cidr", defaultWireGuardCIDR)
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || prefix == (netip.Prefix{}) {
		return fmt.Errorf("WireGuard CIDR 无效: %q", cidr)
	}
	if prefix.Bits() == 0 || (prefix.Addr().Is4() && prefix.Bits() > 30) || (prefix.Addr().Is6() && prefix.Bits() > 126) {
		return fmt.Errorf("WireGuard CIDR 可用地址不足: %q", cidr)
	}
	if role == "exit" {
		for _, key := range []string{"server_address", "server_private_key", "server_public_key", "public_key"} {
			if strings.TrimSpace(stringSetting(settings, key, "")) != "" {
				return fmt.Errorf("WireGuard exit 不允许配置入口字段 %q", key)
			}
		}
	} else {
		serverAddress := stringSetting(settings, "server_address", defaultWireGuardServerAddress)
		serverPrefix, err := netip.ParsePrefix(serverAddress)
		if err != nil || serverPrefix.Bits() != prefix.Bits() || !prefix.Contains(serverPrefix.Addr()) {
			return fmt.Errorf("WireGuard server_address 必须是 cidr 内的同掩码地址: %q", serverAddress)
		}
		if serverPrefix.Addr() == prefix.Masked().Addr() || (prefix.Addr().Is4() && isIPv4Broadcast(prefix, serverPrefix.Addr())) {
			return fmt.Errorf("WireGuard server_address 不能使用网络地址或广播地址: %q", serverAddress)
		}
	}

	mtu := intSetting(settings, "mtu", defaultWireGuardMTU)
	if mtu < 576 || mtu > 1500 {
		return fmt.Errorf("WireGuard MTU 必须在 576-1500 之间: %d", mtu)
	}
	if err := validateWireGuardStringList(settings, "dns", false, false); err != nil {
		return err
	}
	if err := validateWireGuardStringList(settings, "allowed_ips", true, true); err != nil {
		return err
	}
	if protocol.Enable != 0 {
		if !prefix.Addr().Is4() {
			return errors.New("WireGuard relay 首版仅支持 IPv4 peer CIDR")
		}
		if err := validateWireGuardIPv4List(settings, "allowed_ips"); err != nil {
			return err
		}
	}

	privateKey := stringSetting(settings, "server_private_key", "")
	publicKey := firstStringSetting(settings, "server_public_key", "public_key")
	if role != "exit" && (protocol.Enable != 0 || privateKey != "" || publicKey != "") {
		if privateKey == "" {
			return errors.New("WireGuard server_private_key 不能为空")
		}
		derivedPublic, err := validateWireGuardKey(privateKey, "server_private_key")
		if err != nil {
			return err
		}
		if publicKey != "" {
			if _, err := validateWireGuardKey(publicKey, "server_public_key"); err != nil {
				return err
			}
			if publicKey != derivedPublic {
				return errors.New("WireGuard server_public_key 与 server_private_key 不匹配")
			}
		}
	}

	tunnelType := strings.ToLower(stringSetting(settings, "tunnel_type", "quic"))
	if tunnelType != "quic" && tunnelType != "wss" {
		return fmt.Errorf("WireGuard tunnel_type 不支持: %q", tunnelType)
	}
	if err := validateWireGuardRelay(settings, protocol.Enable != 0, tunnelType); err != nil {
		return err
	}
	return nil
}

func wireGuardRelayRole(settings map[string]any) string {
	relay, ok := settings["relay"].(map[string]any)
	if !ok {
		return "entry"
	}
	role := strings.ToLower(strings.TrimSpace(stringSetting(relay, "role", "entry")))
	if role == "" {
		return "entry"
	}
	return role
}

func validateWireGuardCustomConfig(raw *string) error {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var custom map[string]any
	if err := json.Unmarshal([]byte(*raw), &custom); err != nil {
		return fmt.Errorf("WireGuard custom_config 不是有效 JSON: %w", err)
	}
	if custom == nil {
		return errors.New("WireGuard custom_config 必须是 JSON 对象")
	}
	for _, key := range []string{
		"type", "node_type", "host", "server_name", "server_port",
		"cidr", "server_address", "server_private_key", "server_public_key", "public_key",
		"mtu", "dns", "allowed_ips", "tunnel_type", "relay",
	} {
		if _, ok := custom[key]; ok {
			return fmt.Errorf("WireGuard custom_config 不允许覆盖运行时字段 %q，请使用 settings", key)
		}
	}
	return nil
}

func decodeWireGuardSettings(raw *string) (map[string]any, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(*raw), &settings); err != nil {
		return nil, fmt.Errorf("WireGuard settings 不是有效 JSON: %w", err)
	}
	if settings == nil {
		return map[string]any{}, nil
	}
	return settings, nil
}

func validateWireGuardRelay(settings map[string]any, enabled bool, tunnelType string) error {
	backend := "gost"
	role := "entry"
	if relay, ok := settings["relay"].(map[string]any); ok {
		backend = strings.ToLower(stringSetting(relay, "backend", backend))
		role = strings.ToLower(stringSetting(relay, "role", role))
		if wssCompat, ok := relay["wss_compat"].(bool); ok && wssCompat && tunnelType != "wss" {
			return errors.New("WireGuard relay.wss_compat=true 时 tunnel_type 必须为 wss")
		}
		if mode := strings.ToLower(stringSetting(relay, "mode", "")); mode != "" {
			if mode != "relay+quic" && mode != "relay+wss" {
				return fmt.Errorf("WireGuard relay.mode 不支持: %q", mode)
			}
			if (tunnelType == "quic" && mode != "relay+quic") || (tunnelType == "wss" && mode != "relay+wss") {
				return fmt.Errorf("WireGuard relay.mode 与 tunnel_type 不匹配: %q", mode)
			}
		}
		if tunPort := intSetting(relay, "tun_port", 8421); tunPort < 1 || tunPort > 65535 {
			return fmt.Errorf("WireGuard relay.tun_port 必须在 1-65535 之间: %d", tunPort)
		}
		for _, key := range []string{"entry_tun_address", "exit_tun_address", "tun_address"} {
			if value := stringSetting(relay, key, ""); value != "" {
				ip, _, err := net.ParseCIDR(value)
				if err != nil {
					return fmt.Errorf("WireGuard relay.%s 不是有效 CIDR: %q", key, value)
				}
				if enabled && ip.To4() == nil {
					return fmt.Errorf("WireGuard relay.%s 首版仅支持 IPv4 CIDR: %q", key, value)
				}
			}
		}
		if table := intSetting(relay, "routing_table", 0); table < 0 || table > 4294967295 {
			return fmt.Errorf("WireGuard relay.routing_table 无效: %d", table)
		}
		if priority := intSetting(relay, "routing_priority", 0); priority < 0 || priority >= 32766 {
			return fmt.Errorf("WireGuard relay.routing_priority 必须为 0 或 1-32765: %d", priority)
		}
		serverPort := intSetting(relay, "server_port", 0)
		if serverPort < 0 || serverPort > 65535 || (enabled && serverPort < 1) {
			return fmt.Errorf("WireGuard relay.server_port 必须在 1-65535 之间: %d", serverPort)
		}
		switch role {
		case "entry":
			if enabled && strings.TrimSpace(stringSetting(relay, "server", "")) == "" {
				return errors.New("启用的 WireGuard entry relay 必须配置 relay.server 和 relay.server_port")
			}
		case "exit":
			if enabled && stringSetting(relay, "entry_tun_address", "") == "" {
				return errors.New("启用的 WireGuard exit relay 必须配置 relay.entry_tun_address")
			}
		default:
			return fmt.Errorf("WireGuard relay.role 不支持: %q", role)
		}
		if tunnelType == "wss" {
			if err := validateWireGuardWSSRelaySettings(relay, role, enabled); err != nil {
				return err
			}
		}
	} else if enabled && backend == "gost" {
		return errors.New("启用的 WireGuard relay 必须是对象配置")
	}
	if backend != "gost" {
		return fmt.Errorf("WireGuard relay.backend 不支持: %q", backend)
	}
	return nil
}

func validateWireGuardWSSRelaySettings(relay map[string]any, role string, enabled bool) error {
	path := strings.TrimSpace(stringSetting(relay, "wss_path", "/ws"))
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n") {
		return fmt.Errorf("WireGuard relay.wss_path 无效: %q", path)
	}
	if role == "entry" {
		if strings.TrimSpace(stringSetting(relay, "wss_cert_file", "")) != "" || strings.TrimSpace(stringSetting(relay, "wss_key_file", "")) != "" {
			return errors.New("WireGuard WSS entry 不允许配置出口证书或私钥路径")
		}
		secure, ok := relay["wss_secure"].(bool)
		if !ok || !secure {
			return errors.New("WireGuard WSS entry 必须设置 relay.wss_secure=true")
		}
		serverName := strings.TrimSpace(stringSetting(relay, "wss_server_name", ""))
		if serverName == "" || strings.ContainsAny(serverName, " \t\r\n/\\") {
			return errors.New("WireGuard WSS entry 必须配置有效的 relay.wss_server_name")
		}
		return validateWireGuardRelayFile(stringSetting(relay, "wss_ca_file", ""), "wss_ca_file", false)
	}
	if role == "exit" && enabled {
		if strings.TrimSpace(stringSetting(relay, "wss_server_name", "")) != "" || strings.TrimSpace(stringSetting(relay, "wss_ca_file", "")) != "" {
			return errors.New("WireGuard WSS exit 不允许配置入口证书校验字段")
		}
		if secure, ok := relay["wss_secure"].(bool); ok && secure {
			return errors.New("WireGuard WSS exit 不允许配置入口证书校验开关")
		}
		if err := validateWireGuardRelayFile(stringSetting(relay, "wss_cert_file", ""), "wss_cert_file", true); err != nil {
			return err
		}
		return validateWireGuardRelayFile(stringSetting(relay, "wss_key_file", ""), "wss_key_file", true)
	}
	return nil
}

func validateWireGuardRelayFile(value, key string, required bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return fmt.Errorf("WireGuard WSS 必须配置 relay.%s", key)
		}
		return nil
	}
	if len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("WireGuard relay.%s 无效", key)
	}
	return nil
}

func validateWireGuardIPv4List(settings map[string]any, key string) error {
	raw, exists := settings[key]
	if !exists {
		return nil
	}
	values, err := validationStringSliceSetting(raw)
	if err != nil {
		return fmt.Errorf("WireGuard %s 无效: %w", key, err)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if ip := net.ParseIP(value); ip != nil {
			if ip.To4() == nil {
				return fmt.Errorf("WireGuard %s 首版仅支持 IPv4 地址: %q", key, value)
			}
			continue
		}
		ip, _, parseErr := net.ParseCIDR(value)
		if parseErr != nil || ip.To4() == nil {
			return fmt.Errorf("WireGuard %s 首版仅支持 IPv4 CIDR: %q", key, value)
		}
	}
	return nil
}

func validateWireGuardStringList(settings map[string]any, key string, allowCIDR, allowIP bool) error {
	raw, exists := settings[key]
	if !exists {
		return nil
	}
	values, err := validationStringSliceSetting(raw)
	if err != nil {
		return fmt.Errorf("WireGuard %s 无效: %w", key, err)
	}
	if len(values) == 0 {
		return fmt.Errorf("WireGuard %s 不能为空", key)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if allowCIDR {
			if _, _, err := net.ParseCIDR(value); err != nil && (!allowIP || net.ParseIP(value) == nil) {
				return fmt.Errorf("WireGuard %s 包含无效地址: %q", key, value)
			}
		} else if value == "" {
			return fmt.Errorf("WireGuard %s 不能包含空值", key)
		}
	}
	return nil
}

func validateWireGuardKey(value, field string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("WireGuard %s 必须是标准 Base64 的 32 字节密钥", field)
	}
	if field == "server_public_key" {
		return strings.TrimSpace(value), nil
	}
	private, err := ecdh.X25519().NewPrivateKey(decoded)
	if err != nil {
		return "", fmt.Errorf("WireGuard %s 无效: %w", field, err)
	}
	return base64.StdEncoding.EncodeToString(private.PublicKey().Bytes()), nil
}

func wireGuardPublicKeyFromPrivate(value string) string {
	public, err := validateWireGuardKey(value, "server_private_key")
	if err != nil {
		return ""
	}
	return public
}

func isIPv4Broadcast(prefix netip.Prefix, addr netip.Addr) bool {
	if !prefix.Addr().Is4() {
		return false
	}
	bits := prefix.Bits()
	if bits >= 31 {
		return false
	}
	mask := uint32(^uint32(0) >> uint(bits))
	base := binaryIPv4(prefix.Masked().Addr())
	return binaryIPv4(addr) == base|mask
}

func binaryIPv4(addr netip.Addr) uint32 {
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func validationStringSliceSetting(value any) ([]string, error) {
	switch value := value.(type) {
	case string:
		return compactWireGuardStringList(strings.Split(value, ",")), nil
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			item, ok := item.(string)
			if !ok {
				return nil, errors.New("必须是字符串列表")
			}
			out = append(out, item)
		}
		return compactWireGuardStringList(out), nil
	case []string:
		return compactWireGuardStringList(value), nil
	default:
		return nil, errors.New("必须是字符串或字符串列表")
	}
}

func compactWireGuardStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
