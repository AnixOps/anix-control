package relay

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// DialQUIC opens a carrier to the next hop over QUIC: link.DialQUIC (which
// verifies the listener against the link trust bundle and pins its
// peer_identity and server_name, never skipping a check, over QUIC version 1
// with no 0-RTT) and then the SETTINGS exchange on the control streams, which
// returns only when the listener has accepted this node as well. A refusal at
// either step is a *link.HandshakeError: link.ReasonOf(err) is its bounded
// reason, such as remote_rejected when the listener turned this node away. The
// ALPN protocol is always ALPN; cfg.Protocol, if set, must be that.
func DialQUIC(ctx context.Context, address string, cfg link.DialConfig, qcfg QUICConfig) (*QUICCarrier, error) {
	if cfg.Protocol == "" {
		cfg.Protocol = ALPN
	}
	if cfg.Protocol != ALPN {
		return nil, alpnError(cfg.Protocol)
	}
	qcfg, err := qcfg.withDefaults(RoleDialer)
	if err != nil {
		return nil, err
	}
	tr, err := qcfg.Transport(RoleDialer)
	if err != nil {
		return nil, err
	}
	conn, err := link.DialQUIC(ctx, address, link.QUICDialConfig{DialConfig: cfg, QUIC: tr})
	if err != nil {
		return nil, err
	}
	c, err := NewQUICCarrierContext(ctx, conn, RoleDialer, qcfg)
	if err != nil {
		return nil, handshakeError(err)
	}
	return c, nil
}

// QUICListener accepts QUIC carriers: the next hop's end of a QUIC link. It
// wraps a link.QUICListener (which checks sources before anything else is spent
// on a connection, bounds the handshakes and asks unvalidated addresses for
// Retry when its queue is over half full, and pins the diallers' identities),
// runs each admitted connection's SETTINGS exchange, and owns the carriers it
// accepted exactly as Listener does: they never outlive it (L1), a peer removed
// from ingress_peers loses its carriers at once (section 3.5), and a carrier
// whose peer lost its CA from the trust bundle is closed.
type QUICListener struct {
	registry
	link *link.QUICListener
	qcfg QUICConfig
}

// ListenQUIC listens on address (UDP) for QUIC carriers. lcfg is the link
// layer's configuration (credentials, ingress sources and peers, handshake
// limits); its Protocol is always ALPN. qcfg configures every accepted carrier
// and the QUIC transport; its StatelessResetKey makes the listener answer the
// packets of connections it forgot (after a restart) with a stateless reset, so
// peers redial at once (L3).
//
// QUIC needs UDP socket buffers of about 7 MiB: see the package documentation
// for the sysctl settings.
func ListenQUIC(address string, lcfg link.ListenerConfig, qcfg QUICConfig) (*QUICListener, error) {
	lcfg, qcfg, tr, err := quicListenerConfigs(lcfg, qcfg)
	if err != nil {
		return nil, err
	}
	ll, err := link.ListenQUIC(address, link.QUICListenerConfig{ListenerConfig: lcfg, QUIC: tr, StatelessResetKey: qcfg.StatelessResetKey})
	if err != nil {
		return nil, err
	}
	return newQUICListener(ll, lcfg.Credentials, qcfg), nil
}

// NewQUICListener is ListenQUIC over an existing packet connection, which the
// listener owns.
func NewQUICListener(pc net.PacketConn, lcfg link.ListenerConfig, qcfg QUICConfig) (*QUICListener, error) {
	lcfg, qcfg, tr, err := quicListenerConfigs(lcfg, qcfg)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	ll, err := link.NewQUICListener(pc, link.QUICListenerConfig{ListenerConfig: lcfg, QUIC: tr, StatelessResetKey: qcfg.StatelessResetKey})
	if err != nil {
		return nil, err
	}
	return newQUICListener(ll, lcfg.Credentials, qcfg), nil
}

func quicListenerConfigs(lcfg link.ListenerConfig, qcfg QUICConfig) (link.ListenerConfig, QUICConfig, *quic.Config, error) {
	if lcfg.Protocol == "" {
		lcfg.Protocol = ALPN
	}
	if lcfg.Protocol != ALPN {
		return lcfg, qcfg, nil, alpnError(lcfg.Protocol)
	}
	qcfg, err := qcfg.withDefaults(RoleAcceptor)
	if err != nil {
		return lcfg, qcfg, nil, err
	}
	tr, err := qcfg.Transport(RoleAcceptor)
	return lcfg, qcfg, tr, err
}

func newQUICListener(ll *link.QUICListener, creds *link.Credentials, qcfg QUICConfig) *QUICListener {
	l := &QUICListener{link: ll, qcfg: qcfg}
	l.init(qcfg.Config, creds, ll.PeerAllowed, ll.Stats)
	l.wg.Add(1)
	go l.acceptLoop()
	return l
}

// Addr returns the listening (UDP) address.
func (l *QUICListener) Addr() net.Addr { return l.link.Addr() }

// Link returns the underlying link listener, for its statistics.
func (l *QUICListener) Link() *link.QUICListener { return l.link }

// Accept returns the next carrier whose connection was admitted, verified and
// whose SETTINGS exchange finished. It returns net.ErrClosed after Close.
func (l *QUICListener) Accept(ctx context.Context) (Carrier, error) { return l.accept(ctx) }

// SetSources replaces the admitted source addresses (the hop's new
// ingress_sources). It applies to connection attempts from now on.
func (l *QUICListener) SetSources(items []string) error { return l.link.SetSources(items) }

// SetPeers replaces the identities that may dial and closes every carrier
// authenticated as an identity that is no longer allowed, with a GOAWAY of
// GoAwayPeerNotAllowed (as an application error code, on QUIC): the revocation
// path of anixops-protocol.md section 3.5. It returns the identities removed. A
// handshake by one of them fails from the moment this returns; a carrier that
// was being set up concurrently is closed when it registers.
func (l *QUICListener) SetPeers(ids []string) (removed []string, err error) {
	removed, err = l.link.SetPeers(ids)
	if err != nil {
		return nil, err
	}
	l.closeRemoved()
	return removed, nil
}

// Stats returns a snapshot of the counters.
func (l *QUICListener) Stats() ListenerStats { return l.snapshot() }

// Close stops the listener and everything it accepted (L1): handshakes in
// flight are abandoned, every carrier gets a GOAWAY of GoAwayListenerClosed and
// is closed when its streams have ended or after Config.DrainTimeout, whichever
// comes first, and then the UDP socket is closed. It returns when everything has
// ended, so it is bounded by the drain timeout.
func (l *QUICListener) Close() error { return l.shutdown(l.link.StopAccepting, l.link.Close) }

func (l *QUICListener) acceptLoop() {
	defer l.wg.Done()
	for {
		conn, err := l.link.Accept(l.ctx)
		if err != nil {
			return
		}
		if !l.admit(conn) {
			_ = conn.Close()
			if l.shutdownStarted() {
				return
			}
			continue
		}
		go l.serve(conn, func() (Carrier, error) {
			c, err := NewQUICCarrier(conn, RoleAcceptor, l.qcfg)
			if err != nil {
				return nil, err
			}
			return c, nil
		})
	}
}
