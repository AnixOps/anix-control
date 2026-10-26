package service

import (
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

func TestValidateProtocolTransport(t *testing.T) {
	str := func(v string) *string { return &v }
	cases := []struct {
		name      string
		protocol  model.NodeProtocol
		wantError bool
	}{
		{"vless ws", model.NodeProtocol{Type: model.ProtocolVLESS, Transport: str("ws"), TLS: 1}, false},
		{"vless reality grpc", model.NodeProtocol{Type: model.ProtocolVLESS, Transport: str("grpc"), TLS: 2}, false},
		{"vless reality ws", model.NodeProtocol{Type: model.ProtocolVLESS, Transport: str("ws"), TLS: 2}, true},
		{"trojan reality", model.NodeProtocol{Type: model.ProtocolTrojan, Transport: str("tcp"), TLS: 2}, false},
		{"vmess reality", model.NodeProtocol{Type: model.ProtocolVMess, Transport: str("tcp"), TLS: 2}, true},
		{"shadowsocks tcp", model.NodeProtocol{Type: model.ProtocolShadowsocks, Transport: str("tcp")}, false},
		{"shadowsocks udp label", model.NodeProtocol{Type: model.ProtocolShadowsocks, Transport: str("udp")}, false},
		{"shadowsocks ws", model.NodeProtocol{Type: model.ProtocolShadowsocks, Transport: str("ws")}, true},
		{"shadowsocks grpc", model.NodeProtocol{Type: model.ProtocolShadowsocks, Transport: str("grpc")}, true},
		{"hysteria2 legacy tcp", model.NodeProtocol{Type: model.ProtocolHysteria2, Transport: str("tcp"), TLS: 1}, false},
		{"hysteria2 h2", model.NodeProtocol{Type: model.ProtocolHysteria2, Transport: str("h2"), TLS: 1}, true},
		{"hysteria2 reality", model.NodeProtocol{Type: model.ProtocolHysteria2, Transport: str("udp"), TLS: 2}, true},
		{"tuic no transport", model.NodeProtocol{Type: model.ProtocolTUIC, TLS: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			protocol := tc.protocol
			err := ValidateNodeProtocol(&protocol)
			if tc.wantError {
				if !errors.Is(err, ErrInvalidNodeProtocol) {
					t.Fatalf("want ErrInvalidNodeProtocol, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
