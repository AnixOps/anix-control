package parser

import (
	"fmt"
	"net"
	"strings"

	"github.com/anixops/v2board/internal/model"
)

// WireGuardFormatter emits native WireGuard configuration blocks. The format is
// intentionally plain .conf so clients that support WireGuard import can use it
// without depending on non-standard URI schemes.
type WireGuardFormatter struct{}

func (f *WireGuardFormatter) Name() string {
	return "wireguard"
}

func (f *WireGuardFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *WireGuardFormatter) FileExtension() string {
	return "conf"
}

func (f *WireGuardFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	configs := make([]string, 0)
	for _, node := range nodes {
		if node.Type != "wireguard" || !node.IsValid() {
			continue
		}
		configs = append(configs, f.formatNode(node))
	}
	return []byte(strings.Join(configs, "\n\n")), nil
}

func (f *WireGuardFormatter) formatNode(node *model.ParsedNode) string {
	dns := node.DNS
	if len(dns) == 0 {
		dns = []string{"1.1.1.1", "8.8.8.8"}
	}
	allowedIPs := node.AllowedIPs
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	mtu := node.MTU
	if mtu <= 0 {
		mtu = 1280
	}

	lines := []string{
		fmt.Sprintf("# %s", node.Name),
		"[Interface]",
		fmt.Sprintf("PrivateKey = %s", node.PrivateKey),
		fmt.Sprintf("Address = %s", wireGuardAddress(node.PeerIP)),
		fmt.Sprintf("DNS = %s", strings.Join(dns, ", ")),
		fmt.Sprintf("MTU = %d", mtu),
		"",
		"[Peer]",
		fmt.Sprintf("PublicKey = %s", node.PublicKey),
	}
	if node.PresharedKey != "" {
		lines = append(lines, fmt.Sprintf("PresharedKey = %s", node.PresharedKey))
	}
	lines = append(lines,
		fmt.Sprintf("AllowedIPs = %s", strings.Join(allowedIPs, ", ")),
		fmt.Sprintf("Endpoint = %s", net.JoinHostPort(node.Server, fmt.Sprintf("%d", node.Port))),
		"PersistentKeepalive = 25",
	)
	return strings.Join(lines, "\n")
}

func wireGuardAddress(peerIP string) string {
	if strings.Contains(peerIP, "/") {
		return peerIP
	}
	if strings.Contains(peerIP, ":") {
		return peerIP + "/128"
	}
	return peerIP + "/32"
}
