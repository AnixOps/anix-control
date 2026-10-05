package relay

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// CarrierListener is the next hop's end of a link: it accepts carriers whose
// connections were admitted, verified and whose SETTINGS exchange finished, and
// owns them. Listener (TLS over TCP and plaintext), QUICListener and
// AutoListener (both, on one port) implement it.
type CarrierListener interface {
	// Accept returns the next carrier. It returns net.ErrClosed after Close.
	Accept(ctx context.Context) (Carrier, error)
	// Addr returns the listening address (the TCP address of an
	// AutoListener).
	Addr() net.Addr
	// SetSources replaces the admitted source addresses (the hop's new
	// ingress_sources); SetPeers replaces the identities that may dial (the
	// hop's new ingress_peers) and closes every carrier authenticated as an
	// identity that is no longer allowed (section 3.5).
	SetSources(items []string) error
	SetPeers(ids []string) (removed []string, err error)
	// Stats returns a snapshot of the counters.
	Stats() ListenerStats
	// Close stops the listener and everything it accepted (L1).
	Close() error
}

var (
	_ CarrierListener = (*Listener)(nil)
	_ CarrierListener = (*QUICListener)(nil)
)

// ListenerStats are a listener's counters.
type ListenerStats struct {
	// Link are the link layer's: connections accepted, handshakes in
	// flight, refusals by reason (and, on QUIC, address validations).
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

// registry is what every listener has in common: the carriers it owns and the
// rules that keep them from outliving it. A carrier accepted by a listener
// never outlives it (L1), a peer removed from ingress_peers loses its carriers
// at once (section 3.5), and a carrier whose peer lost its CA from the trust
// bundle is closed. The link layer admits and verifies; the registry runs the
// SETTINGS exchange of what it admitted under the MaxPending bound, registers
// the carrier and hands it to Accept.
type registry struct {
	cfg     Config
	creds   *link.Credentials // nil on a plaintext listener
	allowed func(identity string) bool
	stats   func() link.ListenerStats

	slots   chan struct{} // SETTINGS exchanges in flight
	ready   chan Carrier
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	unwatch func()

	mu         sync.Mutex
	carriers   map[Carrier]struct{}
	exchanging map[io.Closer]struct{} // admitted connections in their SETTINGS exchange
	closed     bool

	accepted       atomic.Uint64
	settingsFailed atomic.Uint64
	overloaded     atomic.Uint64
}

func (r *registry) init(cfg Config, creds *link.Credentials, allowed func(string) bool, stats func() link.ListenerStats) {
	r.cfg, r.creds, r.allowed, r.stats = cfg, creds, allowed, stats
	r.slots = make(chan struct{}, cfg.MaxPending)
	r.ready = make(chan Carrier)
	r.carriers = make(map[Carrier]struct{})
	r.exchanging = make(map[io.Closer]struct{})
	r.unwatch = func() {}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	if creds != nil {
		r.unwatch = creds.OnReload(r.recheckTrust)
	}
}

// accept returns the next carrier whose connection was admitted, verified and
// whose SETTINGS exchange finished.
func (r *registry) accept(ctx context.Context) (Carrier, error) {
	select {
	case c := <-r.ready:
		return c, nil
	case <-r.ctx.Done():
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// admit takes a slot for the SETTINGS exchange of conn, which the link layer
// admitted, and reports false (the caller closes conn) when the listener is
// closed or too many exchanges are in flight: the peer passed the link
// layer's checks but is not answering. When it returns true, the caller must
// call serve.
func (r *registry) admit(conn io.Closer) bool {
	select {
	case r.slots <- struct{}{}:
	default:
		r.overloaded.Add(1)
		return false
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		<-r.slots
		return false
	}
	r.exchanging[conn] = struct{}{}
	r.mu.Unlock()
	r.wg.Add(1)
	return true
}

// serve runs the SETTINGS exchange of one admitted connection (open makes the
// carrier over it), registers the carrier and hands it to Accept.
func (r *registry) serve(conn io.Closer, open func() (Carrier, error)) {
	defer r.wg.Done()
	defer func() { <-r.slots }()
	c, err := open()
	r.mu.Lock()
	delete(r.exchanging, conn)
	r.mu.Unlock()
	if err != nil {
		r.settingsFailed.Add(1)
		return
	}
	if !r.register(c) {
		return
	}
	select {
	case r.ready <- c:
		r.accepted.Add(1)
	case <-r.ctx.Done():
		_ = c.GoAway(GoAwayListenerClosed, 0)
		_ = c.Close()
	case <-c.Done():
	}
}

// register adds c to the listener's carriers unless the listener closed or
// c's peer was removed from ingress_peers while it was being set up; in
// those cases it closes c and reports false.
func (r *registry) register(c Carrier) bool {
	r.mu.Lock()
	switch {
	case r.closed:
		r.mu.Unlock()
		_ = c.CloseWithReason(GoAwayListenerClosed)
		return false
	case c.Peer().Encrypted() && !r.allowed(c.Peer().Identity):
		r.mu.Unlock()
		_ = c.CloseWithReason(GoAwayPeerNotAllowed)
		return false
	}
	r.carriers[c] = struct{}{}
	r.mu.Unlock()
	go func() {
		<-c.Done()
		r.mu.Lock()
		delete(r.carriers, c)
		r.mu.Unlock()
	}()
	return true
}

// closeRemoved closes every carrier authenticated as an identity that is no
// longer allowed, with GOAWAY peer_not_allowed: the revocation path of
// section 3.5. The caller has already updated the link layer's peers, so a
// carrier being set up concurrently is closed when it registers.
func (r *registry) closeRemoved() {
	r.mu.Lock()
	var doomed []Carrier
	for c := range r.carriers {
		if !r.allowed(c.Peer().Identity) {
			doomed = append(doomed, c)
		}
	}
	r.mu.Unlock()
	closeAll(doomed, GoAwayPeerNotAllowed)
}

// closeAll closes the carriers concurrently (each close may wait up to the
// grace period for a wedged writer) and returns when all have ended.
func closeAll(carriers []Carrier, reason GoAwayReason) {
	var wg sync.WaitGroup
	for _, c := range carriers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.CloseWithReason(reason)
		}()
	}
	wg.Wait()
}

// recheckTrust closes the carriers whose peer no longer verifies under the
// reloaded trust bundle.
func (r *registry) recheckTrust() {
	r.mu.Lock()
	var doomed []Carrier
	for c := range r.carriers {
		if c.Peer().Encrypted() && !r.creds.PeerStillTrusted(c.Peer()) {
			doomed = append(doomed, c)
		}
	}
	r.mu.Unlock()
	closeAll(doomed, GoAwayCredentials)
}

func (r *registry) snapshot() ListenerStats {
	r.mu.Lock()
	n := len(r.carriers)
	r.mu.Unlock()
	return ListenerStats{Link: r.stats(), Carriers: n, Accepted: r.accepted.Load(), SettingsFailed: r.settingsFailed.Load(), Overloaded: r.overloaded.Load()}
}

// shutdown stops the listener and everything it accepted (L1): handshakes in
// flight are abandoned, every carrier gets a GOAWAY of GoAwayListenerClosed and
// is closed when its streams have ended or after Config.DrainTimeout,
// whichever comes first. SETTINGS exchanges in flight are abandoned. stop makes
// the link listener stop accepting (it must leave established connections
// alone), and finish, if not nil, closes what the carriers still need until
// they have drained (the UDP socket of a QUIC listener). It returns when
// everything has ended, so it is bounded by the drain timeout.
func (r *registry) shutdown(stop, finish func() error) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	carriers := make([]Carrier, 0, len(r.carriers))
	for c := range r.carriers {
		carriers = append(carriers, c)
	}
	// Connections still in their SETTINGS exchange belong to no carrier yet:
	// abandon them, so Close does not wait for the handshake timeout.
	for c := range r.exchanging {
		_ = c.Close()
	}
	r.mu.Unlock()
	r.unwatch()
	err := stop()
	r.cancel()
	for _, c := range carriers {
		_ = c.GoAway(GoAwayListenerClosed, r.cfg.DrainTimeout)
	}
	for _, c := range carriers {
		<-c.Done()
	}
	r.wg.Wait()
	if finish != nil {
		if ferr := finish(); err == nil {
			err = ferr
		}
	}
	return err
}

// Listener accepts TLS over TCP carriers, or plaintext ones on a trusted
// link: the next hop's end of a link. It wraps a link.Listener (which checks
// sources before the handshake, bounds and deadlines the handshakes, and pins
// the diallers' identities), runs each admitted connection's SETTINGS
// exchange, and owns the carriers it accepted: they never outlive it (L1), a
// peer removed from ingress_peers loses its carriers at once (section 3.5), and
// a carrier whose peer lost its CA from the trust bundle is closed.
type Listener struct {
	registry
	link  *link.Listener
	ctype CarrierType
}

// Listen listens on address (TCP) for TLS carriers. lcfg is the link
// layer's configuration (credentials, ingress sources and peers, limits); its
// Protocol is always ALPN. ccfg configures every accepted carrier.
func Listen(address string, lcfg link.ListenerConfig, ccfg Config) (*Listener, error) {
	lcfg, cfg, err := listenerConfigs(lcfg, ccfg)
	if err != nil {
		return nil, err
	}
	ll, err := link.Listen(address, lcfg)
	if err != nil {
		return nil, err
	}
	return newListener(ll, lcfg.Credentials, cfg, CarrierTLS), nil
}

// listenerConfigs applies the ALPN rule and the carrier defaults shared by
// every encrypted listener.
func listenerConfigs(lcfg link.ListenerConfig, ccfg Config) (link.ListenerConfig, Config, error) {
	if lcfg.Protocol == "" {
		lcfg.Protocol = ALPN
	}
	if lcfg.Protocol != ALPN {
		return lcfg, ccfg, alpnError(lcfg.Protocol)
	}
	cfg, err := ccfg.withDefaults(RoleAcceptor)
	return lcfg, cfg, err
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
	l := &Listener{link: ll, ctype: ctype}
	l.init(cfg, creds, ll.PeerAllowed, ll.Stats)
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
func (l *Listener) Accept(ctx context.Context) (Carrier, error) { return l.accept(ctx) }

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
	l.closeRemoved()
	return removed, nil
}

// Stats returns a snapshot of the counters.
func (l *Listener) Stats() ListenerStats { return l.snapshot() }

// Close stops the listener and everything it accepted (L1): handshakes in
// flight are abandoned, every carrier gets a GOAWAY of GoAwayListenerClosed
// and is closed when its streams have ended or after Config.DrainTimeout,
// whichever comes first. SETTINGS exchanges in flight are abandoned. It
// returns when everything has ended, so it is bounded by the drain timeout.
// A carrier never outlives the listener that accepted it.
func (l *Listener) Close() error { return l.shutdown(l.link.Close, nil) }

func (l *Listener) acceptLoop() {
	defer l.wg.Done()
	for {
		conn, err := l.link.Accept()
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
			c, err := NewConnCarrier(conn, RoleAcceptor, l.cfg)
			if err != nil {
				return nil, err
			}
			c.attach(conn.Peer(), l.ctype)
			return c, nil
		})
	}
}

// shutdownStarted reports whether Close has begun.
func (r *registry) shutdownStarted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}
