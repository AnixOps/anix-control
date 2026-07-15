package parser

import (
	"fmt"
	"net"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/model"
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
	for _, node := range nodes {
		if node.Type != "wireguard" || !node.IsValid() {
			continue
		}
		// A native WireGuard profile can contain only one Interface section.
		// Returning the first valid profile keeps the .conf importable when a
		// subscription group also contains multiple WireGuard nodes; formats
		// such as Sing-box remain responsible for multi-node aggregation.
		return []byte(f.formatNode(node)), nil
	}
	return []byte{}, nil
}

func (f *WireGuardFormatter) formatNode(node *model.ParsedNode) string {
	dns := node.DNS
	if len(dns) == 0 {
		dns = []string{"1.1.1.1", "8.8.8.8"}
	}
	allowedIPs := node.AllowedIPs
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0"}
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
		fmt.Sprintf("Endpoint = %s", wireGuardEndpoint(node.Server, node.Port)),
		"PersistentKeepalive = 25",
	)
	return strings.Join(lines, "\n")
}

func wireGuardEndpoint(server string, port int) string {
	host := wireGuardHost(server)
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func wireGuardHost(server string) string {
	host := strings.TrimSpace(server)
	if len(host) >= 2 && strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return host
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
