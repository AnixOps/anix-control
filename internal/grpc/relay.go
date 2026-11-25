package grpc

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/AnixOps/anix-control/v5/internal/proxyproto"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
)

// Region relays (grpc.relay_sources, docs/design/region-relay) forward a
// node's TCP connection to this listener byte for byte, behind a PROXY v2
// header that names the node's address. Unlike the edge, a relay terminates
// nothing: the TLS handshake, the client certificate and every byte of the
// stream are the node's own, so a relay can neither read nor forge a node's
// identity. All it vouches for is the address, which is why the header is
// read only from the configured relays: from any other peer it is never
// parsed, and the peer's own address stands.
//
// The listener wraps the connection before the TLS handshake, so the
// address gRPC records for the peer (peer.Addr, and with it remoteHost in
// agent_auth.go) is the node's: enrollment records it, and the per-address
// rules keep applying to nodes, not to the relay.

// relayListener wraps lis so that connections from sources open with a
// PROXY v2 header.
func relayListener(lis net.Listener, sources []string) (net.Listener, error) {
	policy, err := requestorigin.NewPolicy(sources)
	if err != nil {
		return nil, fmt.Errorf("gRPC relay sources: %w", err)
	}
	return &proxyproto.Listener{
		Listener: lis,
		Trusted:  policy.TrustedIP,
		OnRefused: func(remote net.Addr, err error) {
			slog.Warn("closed a relay connection without a valid PROXY v2 header", "component", "grpc", "relay", remote.String(), "error", err)
		},
	}, nil
}
