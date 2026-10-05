package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

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
)

// String returns the carrier's name in the contract and the metrics labels.
func (t CarrierType) String() string {
	switch t {
	case CarrierTLS:
		return "tls_tcp"
	case CarrierPlain:
		return "plain"
	}
	return "raw"
}

// attach records the link-layer facts of a carrier before it is shared.
func (c *Carrier) attach(p link.Peer, t CarrierType) {
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
func DialTLS(ctx context.Context, address string, cfg link.DialConfig, ccfg Config) (*Carrier, error) {
	if cfg.Protocol == "" {
		cfg.Protocol = ALPN
	}
	if cfg.Protocol != ALPN {
		return nil, fmt.Errorf("relay: the ALPN protocol of a carrier is %q, not %q", ALPN, cfg.Protocol)
	}
	conn, err := link.DialTLS(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	c, err := NewCarrier(conn, RoleDialer, ccfg)
	if err != nil {
		return nil, handshakeError(err)
	}
	c.attach(conn.Peer(), CarrierTLS)
	return c, nil
}

// DialPlain opens a carrier to the next hop over a plaintext trusted link
// (link.DialPlain: cfg.TrustedLink must be set) and runs the SETTINGS
// exchange. Nothing is authenticated on the dialling side.
func DialPlain(ctx context.Context, address string, cfg link.PlainDialConfig, ccfg Config) (*Carrier, error) {
	conn, err := link.DialPlain(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	c, err := NewCarrier(conn, RoleDialer, ccfg)
	if err != nil {
		return nil, handshakeError(err)
	}
	c.attach(conn.Peer(), CarrierPlain)
	return c, nil
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
func WatchCredentials(creds *link.Credentials, c *Carrier) (stop func()) {
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

// Listener accepts carriers: the next hop's end of a link. It wraps a
// link.Listener (which checks sources before the handshake, bounds and
// deadlines the handshakes, and pins the diallers' identities), runs each
// admitted connection's SETTINGS exchange, and owns the carriers it
// accepted: they never outlive it (L1), a peer removed from ingress_peers
// loses its carriers at once (section 3.5), and a carrier whose peer lost
// its CA from the trust bundle is closed.
type Listener struct {
	link  *link.Listener
	creds *link.Credentials // nil on a plaintext listener
	cfg   Config
	ctype CarrierType

	slots   chan struct{} // SETTINGS exchanges in flight
	ready   chan *Carrier
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	unwatch func()

	mu       sync.Mutex
	carriers map[*Carrier]struct{}
	closed   bool

	accepted       atomic.Uint64
	settingsFailed atomic.Uint64
	overloaded     atomic.Uint64
}

// Listen listens on address (TCP) for TLS carriers. lcfg is the link
// layer's configuration (credentials, ingress sources and peers, limits); its
// Protocol is always ALPN. ccfg configures every accepted carrier.
func Listen(address string, lcfg link.ListenerConfig, ccfg Config) (*Listener, error) {
	if lcfg.Protocol == "" {
		lcfg.Protocol = ALPN
	}
	if lcfg.Protocol != ALPN {
		return nil, fmt.Errorf("relay: the ALPN protocol of a carrier is %q, not %q", ALPN, lcfg.Protocol)
	}
	cfg, err := ccfg.withDefaults(RoleAcceptor)
	if err != nil {
		return nil, err
	}
	ll, err := link.Listen(address, lcfg)
	if err != nil {
		return nil, err
	}
	return newListener(ll, lcfg.Credentials, cfg, CarrierTLS), nil
}

// ListenPlain listens on address (TCP) for plaintext carriers of a trusted
// link; lcfg.TrustedLink must be set and at least one source given.
func ListenPlain(address string, lcfg link.PlainListenerConfig, ccfg Config) (*Listener, error) {
	cfg, err := ccfg.withDefaults(RoleAcceptor)
	if err != nil {
		return nil, err
	}
	ll, err := link.ListenPlain(address, lcfg)
	if err != nil {
		return nil, err
	}
	return newListener(ll, nil, cfg, CarrierPlain), nil
}

func newListener(ll *link.Listener, creds *link.Credentials, cfg Config, ctype CarrierType) *Listener {
	l := &Listener{
		link:     ll,
		creds:    creds,
		cfg:      cfg,
		ctype:    ctype,
		slots:    make(chan struct{}, cfg.MaxPending),
		ready:    make(chan *Carrier),
		carriers: make(map[*Carrier]struct{}),
		unwatch:  func() {},
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	if creds != nil {
		l.unwatch = creds.OnReload(l.recheckTrust)
	}
	l.wg.Add(1)
	go l.acceptLoop()
	return l
}

// Addr returns the listening address.
func (l *Listener) Addr() net.Addr { return l.link.Addr() }

// Link returns the underlying link listener, for its statistics.
func (l *Listener) Link() *link.Listener { return l.link }

// Accept returns the next carrier whose connection was admitted, verified and
// whose SETTINGS exchange finished. It returns net.ErrClosed after Close.
func (l *Listener) Accept(ctx context.Context) (*Carrier, error) {
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SetSources replaces the admitted source addresses (the hop's new
// ingress_sources). It applies to connections accepted from now on.
func (l *Listener) SetSources(items []string) error { return l.link.SetSources(items) }

// SetPeers replaces the identities that may dial (the hop's new
// ingress_peers) and closes every carrier authenticated as an identity that
// is no longer allowed, with a GOAWAY of GoAwayPeerNotAllowed: the
// revocation path of anixops-protocol.md section 3.5. It returns the
// identities removed. A handshake by one of them fails from the moment this
// returns; a carrier that was being set up concurrently is closed when it
// registers. It is an error on a plaintext listener.
func (l *Listener) SetPeers(ids []string) (removed []string, err error) {
	removed, err = l.link.SetPeers(ids)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	var doomed []*Carrier
	for c := range l.carriers {
		if !l.link.PeerAllowed(c.Peer().Identity) {
			doomed = append(doomed, c)
		}
	}
	l.mu.Unlock()
	for _, c := range doomed {
		_ = c.CloseWithReason(GoAwayPeerNotAllowed)
	}
	return removed, nil
}

// recheckTrust closes the carriers whose peer no longer verifies under the
// reloaded trust bundle.
func (l *Listener) recheckTrust() {
	l.mu.Lock()
	var doomed []*Carrier
	for c := range l.carriers {
		if c.Peer().Encrypted() && !l.creds.PeerStillTrusted(c.Peer()) {
			doomed = append(doomed, c)
		}
	}
	l.mu.Unlock()
	for _, c := range doomed {
		_ = c.CloseWithReason(GoAwayCredentials)
	}
}

// ListenerStats are a listener's counters.
type ListenerStats struct {
	// Link are the link layer's: connections accepted, handshakes in
	// flight, refusals by reason.
	Link link.ListenerStats
	// Carriers is the number of carriers alive.
	Carriers int
	// Accepted counts carriers handed out by Accept.
	Accepted uint64
	// SettingsFailed counts admitted connections whose SETTINGS exchange
	// failed.
	SettingsFailed uint64
	// Overloaded counts admitted connections closed because too many SETTINGS
	// exchanges were in flight.
	Overloaded uint64
}

// Stats returns a snapshot of the counters.
func (l *Listener) Stats() ListenerStats {
	l.mu.Lock()
	n := len(l.carriers)
	l.mu.Unlock()
	return ListenerStats{Link: l.link.Stats(), Carriers: n, Accepted: l.accepted.Load(), SettingsFailed: l.settingsFailed.Load(), Overloaded: l.overloaded.Load()}
}

// Close stops the listener and everything it accepted (L1): handshakes in
// flight are abandoned, every carrier gets a GOAWAY of GoAwayListenerClosed
// and is closed when its streams have ended or after Config.DrainTimeout,
// whichever comes first. It returns when they have all ended. A carrier never
// outlives the listener that accepted it.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	carriers := make([]*Carrier, 0, len(l.carriers))
	for c := range l.carriers {
		carriers = append(carriers, c)
	}
	l.mu.Unlock()
	l.unwatch()
	err := l.link.Close()
	l.cancel()
	for _, c := range carriers {
		_ = c.GoAway(GoAwayListenerClosed, l.cfg.DrainTimeout)
	}
	for _, c := range carriers {
		<-c.Done()
	}
	l.wg.Wait()
	return err
}

func (l *Listener) acceptLoop() {
	defer l.wg.Done()
	for {
		conn, err := l.link.Accept()
		if err != nil {
			return
		}
		select {
		case l.slots <- struct{}{}:
		default:
			// Too many SETTINGS exchanges in flight: the peer passed the
			// link layer's checks but is not answering.
			l.overloaded.Add(1)
			_ = conn.Close()
			continue
		}
		l.wg.Add(1)
		go l.serve(conn)
	}
}

// serve runs the SETTINGS exchange of one admitted connection, registers the
// carrier and hands it to Accept.
func (l *Listener) serve(conn *link.Conn) {
	defer l.wg.Done()
	defer func() { <-l.slots }()
	c, err := NewCarrier(conn, RoleAcceptor, l.cfg)
	if err != nil {
		l.settingsFailed.Add(1)
		return
	}
	c.attach(conn.Peer(), l.ctype)
	if !l.register(c) {
		return
	}
	select {
	case l.ready <- c:
		l.accepted.Add(1)
	case <-l.ctx.Done():
		_ = c.GoAway(GoAwayListenerClosed, 0)
		_ = c.Close()
	case <-c.Done():
	}
}

// register adds c to the listener's carriers unless the listener closed or
// c's peer was removed from ingress_peers while it was being set up; in
// those cases it closes c and reports false.
func (l *Listener) register(c *Carrier) bool {
	l.mu.Lock()
	switch {
	case l.closed:
		l.mu.Unlock()
		_ = c.CloseWithReason(GoAwayListenerClosed)
		return false
	case c.Peer().Encrypted() && !l.link.PeerAllowed(c.Peer().Identity):
		l.mu.Unlock()
		_ = c.CloseWithReason(GoAwayPeerNotAllowed)
		return false
	}
	l.carriers[c] = struct{}{}
	l.mu.Unlock()
	go func() {
		<-c.Done()
		l.mu.Lock()
		delete(l.carriers, c)
		l.mu.Unlock()
	}()
	return true
}
