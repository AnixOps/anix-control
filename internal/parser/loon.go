package parser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// LoonFormatter emits Loon's native node-subscription syntax. It is deliberately
// independent from the V2Ray, Shadowrocket, Clash, and Sing-box formatters so a
// Loon compatibility change cannot alter another client's subscription.
type LoonFormatter struct {
	lookupIP func(context.Context, string, string) ([]net.IP, error)
}

func (f *LoonFormatter) Name() string { return "loon" }

func (f *LoonFormatter) ContentType() string { return "text/plain; charset=utf-8" }

func (f *LoonFormatter) FileExtension() string { return "txt" }

func (f *LoonFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if line := f.formatNode(node, ctx); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []byte{}, nil
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func (f *LoonFormatter) formatNode(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	switch strings.ToLower(node.Type) {
	case "vmess":
		return f.formatVMess(node, ctx)
	case "vless":
		return f.formatVLESS(node, ctx)
	case "trojan":
		return f.formatTrojan(node, ctx)
	case "shadowsocks":
		return f.formatShadowsocks(node, ctx)
	case "wireguard":
		return f.formatWireGuard(node)
	case "hysteria2":
		return f.formatHysteria2(node, ctx)
	case "anytls":
		return f.formatAnyTLS(node, ctx)
	default:
		return ""
	}
}

func (f *LoonFormatter) formatVMess(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	uuid := loonCredential(node.UUID, ctx)
	transport, ok := loonTransport(node)
	if !loonEndpointValid(node) || uuid == "" || !ok {
		return ""
	}
	parts := []string{"vmess", node.Server, fmt.Sprint(node.Port), "aes-128-gcm", loonQuote(uuid)}
	parts = append(parts, transport...)
	parts = append(parts, "alterId=0")
	parts = append(parts, loonTLSOptions(node, true)...)
	parts = append(parts, "udp=true")
	return loonLine(node.Name, parts)
}

func (f *LoonFormatter) formatVLESS(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	uuid := loonCredential(node.UUID, ctx)
	transport, ok := loonTransport(node)
	if !loonEndpointValid(node) || uuid == "" || !ok {
		return ""
	}
	parts := []string{"VLESS", node.Server, fmt.Sprint(node.Port), loonQuote(uuid)}
	parts = append(parts, transport...)
	if node.Flow != "" {
		parts = append(parts, "flow="+node.Flow)
	}
	if node.TLSMode == 2 {
		if node.RealityPublicKey == "" {
			return ""
		}
		parts = append(parts, "public-key="+loonQuote(node.RealityPublicKey))
		if node.RealityShortID != "" {
			parts = append(parts, "short-id="+node.RealityShortID)
		}
	}
	parts = append(parts, loonTLSOptions(node, true)...)
	parts = append(parts, "udp=true")
	return loonLine(node.Name, parts)
}

func (f *LoonFormatter) formatTrojan(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := loonCredential(node.Password, ctx)
	transport, ok := loonTransport(node)
	if !loonEndpointValid(node) || password == "" || !ok {
		return ""
	}
	parts := []string{"trojan", node.Server, fmt.Sprint(node.Port), loonQuote(password)}
	parts = append(parts, transport...)
	parts = append(parts, loonTLSOptions(node, false)...)
	if node.ALPN != "" {
		parts = append(parts, "alpn="+node.ALPN)
	}
	parts = append(parts, "udp=true")
	return loonLine(node.Name, parts)
}

func (f *LoonFormatter) formatShadowsocks(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := loonCredential(node.Password, ctx)
	if !loonEndpointValid(node) || password == "" {
		return ""
	}
	cipher := node.Cipher
	if cipher == "" {
		cipher = loonStringSetting(node.Settings, "cipher")
	}
	if cipher == "" {
		cipher = "aes-256-gcm"
	}
	if strings.HasPrefix(cipher, "2022-blake3-") {
		serverKey := node.ServerKey
		if serverKey == "" {
			serverKey = loonStringSetting(node.Settings, "server_key")
		}
		if serverKey == "" {
			return ""
		}
		password = serverKey + ":" + generateSS2022UserKey(password, cipher)
	}
	parts := []string{"Shadowsocks", node.Server, fmt.Sprint(node.Port), cipher, loonQuote(password), "fast-open=false", "udp=true"}
	return loonLine(node.Name, parts)
}

func (f *LoonFormatter) formatWireGuard(node *model.ParsedNode) string {
	if !loonEndpointValid(node) || node.PrivateKey == "" || node.PublicKey == "" || node.PeerIP == "" {
		return ""
	}
	peerIP := strings.Split(node.PeerIP, "/")[0]
	interfaceKey := "interface-ip"
	if ip := net.ParseIP(peerIP); ip != nil && ip.To4() == nil {
		interfaceKey = "interface-ipV6"
	}
	parts := []string{
		"wireguard",
		interfaceKey + "=" + peerIP,
		"private-key=" + loonQuote(node.PrivateKey),
	}
	if node.MTU > 0 {
		parts = append(parts, fmt.Sprintf("mtu=%d", node.MTU))
	}
	for _, dns := range node.DNS {
		ip := net.ParseIP(strings.TrimSpace(dns))
		if ip == nil {
			continue
		}
		key := "dns"
		if ip.To4() == nil {
			key = "dnsV6"
		}
		parts = append(parts, key+"="+ip.String())
	}
	peer := []string{"public-key=" + loonQuote(node.PublicKey)}
	if node.PresharedKey != "" {
		peer = append(peer, "preshared-key="+loonQuote(node.PresharedKey))
	}
	allowedIPs := node.AllowedIPs
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	peer = append(peer,
		"allowed-ips="+loonQuote(strings.Join(allowedIPs, ",")),
		"endpoint="+f.wireGuardEndpoint(node.Server, node.Port),
	)
	parts = append(parts, "keeyalive=25", "peers=[{"+strings.Join(peer, ",")+"}]", "udp=true")
	return loonLine(node.Name, parts)
}

// wireGuardEndpoint avoids a Loon bootstrap failure for IPv6-only endpoints.
// Some Loon DNS configurations issue only an A query while opening the UDP
// transport, so an IPv6-only hostname never reaches the WireGuard handshake.
// Keep hostnames that have IPv4 and fall back to the original hostname on any
// resolver failure; only a confirmed IPv6-only result is rendered as a
// bracketed IPv6 literal.
func (f *LoonFormatter) wireGuardEndpoint(server string, port int) string {
	host := wireGuardHost(server)
	if net.ParseIP(host) != nil {
		return net.JoinHostPort(host, fmt.Sprint(port))
	}

	lookupIP := f.lookupIP
	if lookupIP == nil {
		lookupIP = net.DefaultResolver.LookupIP
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ipv4, err := lookupIP(ctx, "ip4", host)
	if err == nil && len(ipv4) > 0 {
		return net.JoinHostPort(host, fmt.Sprint(port))
	}
	if err != nil && !loonDNSNotFound(err) {
		return net.JoinHostPort(host, fmt.Sprint(port))
	}

	ipv6, err := lookupIP(ctx, "ip6", host)
	if err != nil || len(ipv6) == 0 {
		return net.JoinHostPort(host, fmt.Sprint(port))
	}
	sort.Slice(ipv6, func(i, j int) bool {
		return ipv6[i].String() < ipv6[j].String()
	})
	return net.JoinHostPort(ipv6[0].String(), fmt.Sprint(port))
}

func loonDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func (f *LoonFormatter) formatHysteria2(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := loonCredential(node.Password, ctx)
	if !loonEndpointValid(node) || password == "" {
		return ""
	}
	parts := []string{"Hysteria2", node.Server, fmt.Sprint(node.Port), loonQuote(password)}
	parts = append(parts, loonTLSOptions(node, false)...)
	if obfsPassword := loonStringSetting(node.Settings, "obfs-password"); obfsPassword != "" {
		parts = append(parts, "salamander-password="+loonQuote(obfsPassword))
	}
	parts = append(parts, "udp=true", "fast-open=true")
	return loonLine(node.Name, parts)
}

func (f *LoonFormatter) formatAnyTLS(node *model.ParsedNode, ctx *model.TemplateRenderContext) string {
	password := loonCredential(node.Password, ctx)
	if !loonEndpointValid(node) || password == "" {
		return ""
	}
	parts := []string{"AnyTLS", node.Server, fmt.Sprint(node.Port), loonQuote(password)}
	parts = append(parts, loonTLSOptions(node, false)...)
	parts = append(parts, "udp=true", "block-quic=false")
	return loonLine(node.Name, parts)
}

func loonTransport(node *model.ParsedNode) ([]string, bool) {
	transport := strings.ToLower(strings.TrimSpace(node.Transport))
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" && transport != "ws" && transport != "http" {
		return nil, false
	}
	parts := []string{"transport=" + transport}
	if transport == "ws" || transport == "http" {
		if path := loonStringSetting(node.TransportSettings, "path"); path != "" {
			parts = append(parts, "path="+loonQuote(path))
		}
		if host := loonTransportHost(node.TransportSettings); host != "" {
			parts = append(parts, "host="+loonQuote(host))
		}
	}
	return parts, true
}

func loonTransportHost(settings map[string]any) string {
	if host := loonStringSetting(settings, "host"); host != "" {
		return host
	}
	if headers, ok := settings["headers"].(map[string]any); ok {
		return loonStringSetting(headers, "Host")
	}
	return ""
}

func loonTLSOptions(node *model.ParsedNode, includeOverTLS bool) []string {
	tlsEnabled := node.TLS || node.TLSMode > 0
	parts := make([]string, 0, 3)
	if includeOverTLS {
		parts = append(parts, fmt.Sprintf("over-tls=%t", tlsEnabled))
	}
	if node.ServerName != "" {
		parts = append(parts, "sni="+loonQuote(node.ServerName))
	}
	parts = append(parts, fmt.Sprintf("skip-cert-verify=%t", node.SkipCertVerify))
	return parts
}

func loonEndpointValid(node *model.ParsedNode) bool {
	return node != nil && strings.TrimSpace(node.Server) != "" && node.Port > 0
}

func loonCredential(value string, ctx *model.TemplateRenderContext) string {
	if value != "" {
		return value
	}
	if ctx != nil {
		return ctx.UUID
	}
	return ""
}

func loonStringSetting(settings map[string]any, key string) string {
	if value, ok := settings[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func loonLine(name string, parts []string) string {
	cleanName := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(name))
	if cleanName == "" {
		cleanName = "Loon Node"
	}
	return cleanName + " = " + strings.Join(parts, ",")
}

func loonQuote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "", "\n", "").Replace(value)
	return `"` + escaped + `"`
}
