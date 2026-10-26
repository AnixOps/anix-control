package service

import (
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// datagramProtocols carry no stream transport: Shadowsocks listens on TCP
// and UDP itself, Hysteria2 and TUIC are QUIC on UDP, WireGuard and iWAN are
// UDP. Their transport column is only a label, so tcp, udp and empty are all
// accepted (older editors stored tcp for Hysteria2); a stream transport such
// as ws or grpc is a mistake the kernels would ignore.
var datagramProtocols = map[model.ProtocolType]bool{
	model.ProtocolShadowsocks: true,
	model.ProtocolHysteria2:   true,
	model.ProtocolTUIC:        true,
	model.ProtocolWireGuard:   true,
	model.ProtocolIWAN:        true,
}

// realityProtocols are the types Reality (tls 2) runs with, and
// realityTransports the transports it runs on.
var (
	realityProtocols  = map[model.ProtocolType]bool{model.ProtocolVLESS: true, model.ProtocolTrojan: true}
	realityTransports = map[string]bool{"": true, "tcp": true, "grpc": true, "h2": true}
)

// validateProtocolTransport refuses the transport and TLS combinations no
// kernel runs (web: protocolCapabilities.js). It is lenient with values
// that are only labels, so stored protocols keep saving.
func validateProtocolTransport(protocol *model.NodeProtocol) error {
	protocolType := model.ProtocolType(strings.ToLower(strings.TrimSpace(string(protocol.Type))))
	transport := ""
	if protocol.Transport != nil {
		transport = strings.ToLower(strings.TrimSpace(*protocol.Transport))
	}
	if datagramProtocols[protocolType] && transport != "" && transport != "tcp" && transport != "udp" {
		return fmt.Errorf("%s 不支持 %s 传输", protocolType, transport)
	}
	if protocol.TLS == 2 {
		if !realityProtocols[protocolType] {
			return fmt.Errorf("%s 不支持 Reality", protocolType)
		}
		if !realityTransports[transport] {
			return fmt.Errorf("Reality 不支持 %s 传输，只能用 tcp、grpc 或 h2", transport)
		}
	}
	return nil
}
