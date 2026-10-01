package native

import (
	"context"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// ProtocolTemplate is a quick-add protocol template, as the kernel's
// model.ProtocolTemplate declares it.
type ProtocolTemplate struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DefaultPort int    `json:"default_port"`
	Settings    string `json:"settings"`
	TLS         int    `json:"tls"`
	TLSSettings string `json:"tls_settings"`
	Transport   string `json:"transport"`
	Reality     string `json:"reality_settings"`
}

// ProtocolTemplates is GET /api/v2/admin/protocol-templates.
func (s *Service) ProtocolTemplates(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	return s.panel(protocolTemplates())
}

// protocolTemplates is the kernel's model.GetProtocolTemplates; the parity
// test compares the two answers byte for byte.
func protocolTemplates() []ProtocolTemplate {
	return []ProtocolTemplate{
		{
			Type:        "vless",
			Name:        "VLESS + Reality (推荐)",
			Description: "目前最安全的协议组合，抗封锁能力极强",
			DefaultPort: 443,
			TLS:         2, // Reality
			Settings:    `{"flow":"xtls-rprx-vision"}`,
			TLSSettings: `{"fingerprint":"chrome","server_name":"www.microsoft.com"}`,
			Reality:     `{"short_id":"6ba85179e30d4fc2","dest":"www.microsoft.com:443"}`,
			Transport:   "tcp",
		},
		{
			Type:        "vmess",
			Name:        "VMess + WebSocket + TLS",
			Description: "经典的 CDN 转发配置，适合配合 Cloudflare 等使用",
			DefaultPort: 443,
			TLS:         1, // TLS
			Settings:    `{}`,
			TLSSettings: `{"allowInsecure":false}`,
			Transport:   "ws",
		},
		{
			Type:        "trojan",
			Name:        "Trojan",
			Description: "伪装成 HTTPS 流量，简单稳定",
			DefaultPort: 443,
			TLS:         1,
			Settings:    `{}`,
			Transport:   "tcp",
		},
		{
			Type:        "shadowsocks",
			Name:        "Shadowsocks (2022-Blake3)",
			Description: "高性能的加密传输协议",
			DefaultPort: 8388,
			TLS:         0,
			Settings:    `{"method":"2022-blake3-aes-128-gcm"}`,
			Transport:   "tcp",
		},
		{
			Type:        "hysteria2",
			Name:        "Hysteria2",
			Description: "基于 UDP 的高速传输协议，适合高丢包环境",
			DefaultPort: 443,
			TLS:         1,
			Settings:    `{"up_mbps":100,"down_mbps":100,"obfs":"salamander","obfs-password":"auth_password"}`,
			Transport:   "udp",
		},
		{
			Type:        "tuic",
			Name:        "TUIC v5",
			Description: "基于 QUIC 的现代传输协议",
			DefaultPort: 443,
			TLS:         1,
			Settings:    `{"congestion_control":"bbr"}`,
			Transport:   "udp",
		},
		{
			Type:        "wireguard",
			Name:        "WireGuard 双机入口",
			Description: "P0 WireGuard 用户接入，国内入口终止，默认通过 GOST relay+QUIC 到海外出口 NAT",
			DefaultPort: 51820,
			TLS:         0,
			Settings:    `{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","server_public_key":"","mtu":1280,"dns":["1.1.1.1","8.8.8.8"],"allowed_ips":["0.0.0.0/0"],"tunnel_type":"quic","relay":{"backend":"gost","mode":"relay+quic","role":"entry","wss_compat":false,"wss_path":"/ws","wss_secure":true,"wss_server_name":"","wss_ca_file":"","wss_cert_file":"","wss_key_file":"","exit_nat":true,"entry_stats":true,"server":"","server_port":0,"tun_port":8421,"entry_tun_address":"172.31.66.2/24","exit_tun_address":"172.31.66.1/24","outbound_iface":"","routing_table":0,"routing_priority":0}}`,
			Transport:   "udp",
		},
	}
}
