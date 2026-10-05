package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// CarrierType is how a carrier's connection was made (anixops-protocol.md
// section 5).
type CarrierType uint8

const (
	// CarrierTLS is TLS 1.3 over TCP with mutual authentication and identity
	// pinning (TLS_TCP, section 5.1).
	CarrierTLS CarrierType = 1
	// CarrierPlain is plaintext TCP on a trusted link (PLAIN, section 5.3).
	CarrierPlain CarrierType = 2
	// CarrierQUIC is QUIC version 1 with TLS 1.3, mutual authentication and
	// identity pinning (QUIC, section 5.2).
	CarrierQUIC CarrierType = 3
)

// String returns the carrier's name in the contract and the metrics labels.
func (t CarrierType) String() string {
	switch t {
	case CarrierTLS:
		return "tls_tcp"
	case CarrierPlain:
		return "plain"
	case CarrierQUIC:
		return "quic"
	}
	return "raw"
}

// attach records the link-layer facts of a carrier before it is shared.
func (c *ConnCarrier) attach(p link.Peer, t CarrierType) {
	c.linkPeer, c.ctype = p, t
}

// DialTLS opens a carrier to the next hop over TLS 1.3: link.DialTLS (which
// verifies the listener against the link trust bundle and pins its
// peer_identity and server_name, never skipping a check) and then the
// carrier's SETTINGS exchange, which returns only when the listener has
// accepted this node as well (in TLS 1.3 the dialler's handshake finishes
// before the listener has verified it). A refusal at either step is a
// *link.HandshakeError: link.ReasonOf(err) is its bounded reason, such as
// remote_rejected when the listener turned this node away. The ALPN protocol
// is always ALPN; cfg.Protocol, if set, must be that.
func DialTLS(ctx context.Context, address string, cfg link.DialConfig, ccfg Config) (*ConnCarrier, error) {
	if cfg.Protocol == "" {
		cfg.Protocol = ALPN
	}
	if cfg.Protocol != ALPN {
		return nil, alpnError(cfg.Protocol)
	}
	conn, err := link.DialTLS(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	c, err := newDiallerCarrier(ctx, conn, ccfg)
	if err != nil {
		return nil, handshakeError(err)
	}
	c.attach(conn.Peer(), CarrierTLS)
	return c, nil
}

// DialPlain opens a carrier to the next hop over a plaintext trusted link
// (link.DialPlain: cfg.TrustedLink must be set) and runs the SETTINGS
// exchange. Nothing is authenticated on the dialling side.
func DialPlain(ctx context.Context, address string, cfg link.PlainDialConfig, ccfg Config) (*ConnCarrier, error) {
	conn, err := link.DialPlain(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	c, err := newDiallerCarrier(ctx, conn, ccfg)
	if err != nil {
		return nil, handshakeError(err)
	}
	c.attach(conn.Peer(), CarrierPlain)
	return c, nil
}

// newDiallerCarrier runs the SETTINGS exchange of a dialled connection, giving
// up (and closing the connection) when ctx ends before it has finished, so a
// dial bounded by a deadline is bounded in the exchange too.
func newDiallerCarrier(ctx context.Context, conn *link.Conn, ccfg Config) (*ConnCarrier, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	c, err := NewConnCarrier(conn, RoleDialer, ccfg)
	if err != nil {
		stop()
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", ctx.Err(), err)
		}
		return nil, err
	}
	if !stop() { // ctx ended just now and the connection is closing
		_ = c.Close()
		return nil, ctx.Err()
	}
	return c, nil
}

// alpnError is the refusal of a configuration that names another ALPN
// protocol: a carrier always speaks ALPN.
func alpnError(p string) error {
	return fmt.Errorf("relay: the ALPN protocol of a carrier is %q, not %q", ALPN, p)
}

// handshakeError gives a failed SETTINGS exchange a link reason.
func handshakeError(err error) error {
	var he *link.HandshakeError
	if errors.As(err, &he) {
		return err
	}
	reason := link.ReasonOf(err)
	var pe *ProtocolError
	if errors.As(err, &pe) {
		reason = link.ReasonProtocol
	}
	return &link.HandshakeError{Reason: reason, Err: err}
}

// WatchCredentials closes c, with GoAwayCredentials, as soon as a reload of
// creds leaves its peer untrusted: the peer's CA was dropped from the link
// trust bundle, or its certificate expired (anixops-protocol.md section
// 3.5). A listener does this for the carriers it accepted; a dialler's pool
// calls it for the carriers it opened. It does nothing for a plaintext
// carrier, and it stops watching when c ends or stop is called.
func WatchCredentials(creds *link.Credentials, c Carrier) (stop func()) {
	if creds == nil || !c.Peer().Encrypted() {
		return func() {}
	}
	cancel := creds.OnReload(func() {
		if !creds.PeerStillTrusted(c.Peer()) {
			_ = c.CloseWithReason(GoAwayCredentials)
		}
	})
	go func() {
		<-c.Done()
		cancel()
	}()
	return cancel
}
