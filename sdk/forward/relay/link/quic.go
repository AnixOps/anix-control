package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// QUICConn is a QUIC connection between two nodes (anixops-protocol.md
// section 5.2) whose peer has been admitted, authenticated and pinned with the
// same checks as a TLS link connection. The carrier layer runs on it; the
// embedded *quic.Conn is the connection itself.
type QUICConn struct {
	*quic.Conn
	peer Peer
}

// Peer returns the node at the other end: its verified identity and chain.
func (c *QUICConn) Peer() Peer { return c.peer }

// Encrypted is always true: QUIC has no plaintext mode.
func (c *QUICConn) Encrypted() bool { return true }

// RemoteAddrPort returns the remote address, or the zero value for an address
// that is not an IP socket address.
func (c *QUICConn) RemoteAddrPort() netip.AddrPort { return addrPort(c.RemoteAddr()) }

// Close closes the connection with no error code, at once.
func (c *QUICConn) Close() error { return c.CloseWithError(0, "") }

// quicConfig returns the quic.Config a link connection runs with: the
// caller's transport settings (idle timeout, keepalive, stream limits, flow
// control windows, datagrams) with every setting that is a security decision
// forced here, whatever base says:
//
//   - QUIC version 1 only (RFC 9000, section 5.2): nothing to negotiate down
//     to, and no version negotiation to answer;
//   - no 0-RTT: the server never accepts early data and the client never
//     sends it (anixops-protocol.md section 3.4);
//   - no token store: a client never skips address validation on a promise
//     from an earlier connection (no resumption, P3);
//   - the handshake idle timeout is half the handshake timeout: quic-go aborts
//     a handshake that has not completed in twice that, which makes the
//     handshake timeout (10 s) a hard deadline.
func quicConfig(base *quic.Config, handshakeTimeout time.Duration) *quic.Config {
	var c quic.Config
	if base != nil {
		c = *base
	}
	c.Versions = []quic.Version{quic.Version1}
	c.Allow0RTT = false
	c.TokenStore = nil
	c.GetConfigForClient = nil
	c.HandshakeIdleTimeout = max(handshakeTimeout/2, time.Millisecond)
	return &c
}

// checkQUICState is checkSession's counterpart for what only QUIC has: the
// handshake must have used QUIC version 1 and no early data. (TLS 1.3, the ALPN
// protocol and the absence of resumption are checked by VerifyConnection on
// both ends.)
func checkQUICState(cs quic.ConnectionState, protocol string) *HandshakeError {
	switch {
	case cs.Version != quic.Version1:
		return refuse(ReasonProtocol, "", "QUIC version 1 is required")
	case cs.Used0RTT:
		return refuse(ReasonProtocol, "", "0-RTT is not accepted")
	case cs.TLS.Version != tls.VersionTLS13 || cs.TLS.NegotiatedProtocol != protocol || cs.TLS.DidResume:
		return refuse(ReasonProtocol, "", "the QUIC handshake did not negotiate TLS 1.3 and ALPN %q without resumption", protocol)
	case len(cs.TLS.PeerCertificates) == 0:
		return refuse(ReasonCertificate, "", "the peer presented no certificate")
	}
	return nil
}

// QUICDialConfig configures DialQUIC. The DialConfig fields mean what they
// mean for DialTLS (Dialer contributes only its Resolver and, if it is a
// *net.UDPAddr, its LocalAddr).
type QUICDialConfig struct {
	DialConfig
	// QUIC holds the transport settings (see quicConfig for what is forced).
	// Nil means quic-go's defaults.
	QUIC *quic.Config
}

// DialQUIC opens a QUIC connection to address (a UDP host and port) and runs
// the TLS 1.3 handshake of DialTLS inside it: the listener's certificate must
// chain to the link trust bundle, carry ServerName as its only DNS name and
// PeerIdentity as its only URI name, and the ALPN protocol must be negotiated.
// Each connection has a UDP socket of its own, closed with it. A failure is a
// *HandshakeError with a bounded Reason.
//
// As with DialTLS, the dialler's handshake completes before the listener has
// checked the dialler's certificate (in QUIC too: the client finishes first), so
// a nil error does not yet mean the listener accepted this node; a listener
// that refuses it closes the connection with a TLS alert shortly after. The
// carrier layer's SETTINGS exchange turns that into a dial error.
func DialQUIC(ctx context.Context, address string, cfg QUICDialConfig) (*QUICConn, error) {
	switch {
	case cfg.Credentials == nil:
		return nil, errors.New("link: DialQUIC needs credentials")
	case cfg.Protocol == "":
		return nil, errors.New("link: DialQUIC needs an ALPN protocol")
	case !serverNamePattern.MatchString(cfg.ServerName) || netIsIP(cfg.ServerName):
		return nil, fmt.Errorf("link: server name %q is not a DNS name", cfg.ServerName)
	}
	if _, err := ParseIdentity(cfg.PeerIdentity); err != nil {
		return nil, err
	}
	timeout := cfg.HandshakeTimeout
	if timeout == 0 {
		timeout = DefaultHandshakeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raddr, err := resolveUDP(ctx, address, cfg.Dialer)
	if err != nil {
		return nil, classify(err, "")
	}
	network, laddr := "udp4", &net.UDPAddr{}
	if raddr.Addr().Is6() {
		network = "udp6"
	}
	if cfg.Dialer != nil {
		if u, ok := cfg.Dialer.LocalAddr.(*net.UDPAddr); ok {
			laddr = u
		}
	}
	udp, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, classify(err, "")
	}
	tr := &quic.Transport{Conn: udp}
	tlsConf := cfg.Credentials.clientTLS(cfg.ServerName, cfg.PeerIdentity, cfg.Protocol)
	// crypto/tls's refusals reach the caller as QUIC's TLS alert and text:
	// keep the typed ones.
	var rec handshakeRecord
	verify := tlsConf.VerifyConnection
	tlsConf.VerifyConnection = func(cs tls.ConnectionState) error {
		err := verify(cs)
		if err != nil {
			rec.set(err)
		}
		return err
	}
	closeAll := func() {
		_ = tr.Close()
		_ = udp.Close()
	}
	conn, err := tr.Dial(ctx, net.UDPAddrFromAddrPort(raddr), tlsConf, quicConfig(cfg.QUIC, timeout))
	if err != nil {
		closeAll()
		return nil, classifyQUIC(err, rec.get())
	}
	cs := conn.ConnectionState()
	if he := checkQUICState(cs, cfg.Protocol); he != nil {
		_ = conn.CloseWithError(0, "")
		closeAll()
		return nil, he
	}
	go func() {
		<-conn.Context().Done()
		closeAll()
	}()
	return &QUICConn{Conn: conn, peer: peerFor(cs.TLS, x509.ExtKeyUsageServerAuth)}, nil
}

// resolveUDP resolves a host:port with the dialler's resolver.
func resolveUDP(ctx context.Context, address string, d *net.Dialer) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(address); err == nil {
		return netip.AddrPortFrom(ap.Addr().Unmap().WithZone(""), ap.Port()), nil
	}
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return netip.AddrPort{}, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("link: port %q is not a number", portStr)
	}
	r := net.DefaultResolver
	if d != nil && d.Resolver != nil {
		r = d.Resolver
	}
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.AddrPort{}, err
	}
	if len(addrs) == 0 {
		return netip.AddrPort{}, fmt.Errorf("link: %q has no address", host)
	}
	return netip.AddrPortFrom(addrs[0].Unmap().WithZone(""), uint16(port)), nil
}

// QUICListenerConfig configures an encrypted QUIC link listener, the next
// hop's end of a QUIC link. The ListenerConfig fields mean what they mean for
// the TCP listener (MaxPending bounds the QUIC handshakes in flight).
type QUICListenerConfig struct {
	ListenerConfig
	// QUIC holds the transport settings (see quicConfig for what is forced).
	// Nil means quic-go's defaults.
	QUIC *quic.Config
	// StatelessResetKey makes the listener answer packets of connections it
	// no longer knows (after a restart, for instance) with a stateless reset
	// (RFC 9000 section 10.3), so peers redial at once instead of waiting for
	// their idle timeout (anixops-protocol.md section 4.6, L3). The caller
	// keeps the key in its state directory so it survives the restart; this
	// library never generates or stores one. Nil disables stateless resets.
	StatelessResetKey *[32]byte
}

// handshakeKey is the context key under which a QUIC listener files the
// record of a connection attempt.
type handshakeKey struct{}

// handshakeRecord keeps the first typed error a verification callback
// returned, because quic-go reports a failed handshake to the application as
// a TLS alert and its text, not as the error crypto/tls returned.
type handshakeRecord struct {
	mu  sync.Mutex
	err error
}

func (r *handshakeRecord) set(err error) {
	r.mu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.mu.Unlock()
}

func (r *handshakeRecord) get() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// attempt is one connection attempt that passed admission: it holds a
// handshake slot until the connection is handed to Accept's caller or the
// handshake fails.
type attempt struct {
	handshakeRecord
	l        *QUICListener
	released atomic.Bool
	dequeued atomic.Bool // the handshake completed and the accept loop has the connection
	handed   atomic.Bool // the connection was delivered to Accept
}

// release gives the slot back, once.
func (a *attempt) release() {
	if a.released.CompareAndSwap(false, true) {
		a.l.pendingN.Add(-1)
	}
}

// QUICListener accepts QUIC link connections. Like the TCP Listener it checks
// the source address before spending anything on a connection (an Initial from
// an address outside ingress_sources is refused with one small CONNECTION_REFUSED
// packet: no TLS state, no signature), bounds the handshakes in flight, runs
// each handshake under a hard deadline, and returns only connections whose
// peer passed every check of its kind. When more than half of the handshake
// slots are taken it asks unvalidated addresses to prove themselves with QUIC's
// Retry before it spends a slot on them (anixops-protocol.md section 2.5).
// Sources, peers and credentials change in place, as for the TCP listener.
type QUICListener struct {
	policy
	pc         net.PacketConn
	tr         *quic.Transport
	inner      *quic.Listener
	timeout    time.Duration
	maxPending int64

	pendingN atomic.Int64
	ready    chan *QUICConn

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	stop   sync.Once
	stopE  error
	once   sync.Once
	closeE error

	accepted atomic.Uint64
	retries  atomic.Uint64
	failures [NumReasons]atomic.Uint64
}

// ListenQUIC listens on address (UDP) for QUIC link connections.
func ListenQUIC(address string, cfg QUICListenerConfig) (*QUICListener, error) {
	l, err := newQUICListener(cfg)
	if err != nil {
		return nil, err
	}
	laddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}
	if err := l.start(pc, cfg); err != nil {
		_ = pc.Close()
		return nil, err
	}
	return l, nil
}

// NewQUICListener is ListenQUIC over an existing packet connection, which the
// listener owns (it closes it). A *net.UDPConn enables ECN, packet info and
// batched reads in quic-go; other connections work without them.
func NewQUICListener(pc net.PacketConn, cfg QUICListenerConfig) (*QUICListener, error) {
	l, err := newQUICListener(cfg)
	if err != nil {
		return nil, err
	}
	if err := l.start(pc, cfg); err != nil {
		_ = pc.Close()
		return nil, err
	}
	return l, nil
}

func newQUICListener(cfg QUICListenerConfig) (*QUICListener, error) {
	switch {
	case cfg.Credentials == nil:
		return nil, errors.New("link: an encrypted listener needs credentials")
	case cfg.Protocol == "":
		return nil, errors.New("link: an encrypted listener needs an ALPN protocol")
	case cfg.HandshakeTimeout < 0 || cfg.MaxPending < 0:
		return nil, errors.New("link: negative listener limits")
	}
	l := &QUICListener{timeout: cfg.HandshakeTimeout, maxPending: int64(cfg.MaxPending)}
	if err := l.init(cfg.Credentials, cfg.Protocol, cfg.Sources, cfg.Peers); err != nil {
		return nil, err
	}
	if l.timeout == 0 {
		l.timeout = DefaultHandshakeTimeout
	}
	if l.maxPending == 0 {
		l.maxPending = DefaultMaxPending
	}
	return l, nil
}

func (l *QUICListener) start(pc net.PacketConn, cfg QUICListenerConfig) error {
	l.pc = pc
	l.tr = &quic.Transport{
		Conn:                             pc,
		DisableVersionNegotiationPackets: true,
		VerifySourceAddress:              l.verifySource,
		ConnContext:                      l.connContext,
	}
	if cfg.StatelessResetKey != nil {
		key := quic.StatelessResetKey(*cfg.StatelessResetKey)
		l.tr.StatelessResetKey = &key
	}
	inner, err := l.tr.Listen(l.serverTLS(), quicConfig(cfg.QUIC, l.timeout))
	if err != nil {
		return err
	}
	l.inner = inner
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.ready = make(chan *QUICConn)
	l.wg.Add(1)
	go l.acceptLoop()
	return nil
}

// verifySource decides, on the first Initial of a connection attempt, whether
// the address must prove itself with Retry: only an address the listener would
// admit anyway, and only while the handshake slots are over half taken, so an
// unlisted address never costs a Retry and an idle listener adds no round
// trip. It runs on quic-go's packet goroutine and does nothing slow.
func (l *QUICListener) verifySource(a net.Addr) bool {
	if !l.sources.Load().Contains(addrPort(a).Addr()) || l.pendingN.Load() <= l.maxPending/2 {
		return false
	}
	l.retries.Add(1)
	return true
}

// connContext runs for every connection attempt that is about to get state:
// admission first (an address outside ingress_sources is refused before
// anything else), then a handshake slot (the next one past MaxPending is
// refused at once). The returned context carries the attempt and ends with
// the connection or the failed handshake.
func (l *QUICListener) connContext(ctx context.Context, ci *quic.ClientInfo) (context.Context, error) {
	if !l.sources.Load().Contains(addrPort(ci.RemoteAddr).Addr()) {
		l.fail(ReasonSourceNotAllowed)
		return nil, errSourceNotAllowed
	}
	if l.pendingN.Add(1) > l.maxPending {
		l.pendingN.Add(-1)
		l.fail(ReasonHandshakeLimit)
		return nil, errHandshakeLimit
	}
	a := &attempt{l: l}
	ctx = context.WithValue(ctx, handshakeKey{}, &a.handshakeRecord)
	ctx = context.WithValue(ctx, attemptKey{}, a)
	context.AfterFunc(ctx, func() { a.abandon(ctx) })
	return ctx, nil
}

type attemptKey struct{}

var (
	errSourceNotAllowed = errors.New("link: source address not allowed")
	errHandshakeLimit   = errors.New("link: too many handshakes in flight")
)

// abandon runs when the attempt's context ends: a handshake that never
// completed is a failure (counted under the reason it failed for, and its slot
// freed); the end of a connection that was delivered is not.
func (a *attempt) abandon(ctx context.Context) {
	a.release()
	if a.dequeued.Load() {
		return
	}
	a.l.fail(classifyQUIC(context.Cause(ctx), a.get()).Reason)
}

func (l *QUICListener) fail(reason Reason) { l.failures[reason].Add(1) }

// Addr returns the listening address.
func (l *QUICListener) Addr() net.Addr { return l.pc.LocalAddr() }

// Encrypted is always true.
func (l *QUICListener) Encrypted() bool { return true }

// Accept returns the next connection whose source was admitted and whose
// handshake and identity checks passed. It returns net.ErrClosed after Close.
func (l *QUICListener) Accept(ctx context.Context) (*QUICConn, error) {
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *QUICListener) acceptLoop() {
	defer l.wg.Done()
	for {
		qc, err := l.inner.Accept(l.ctx)
		if err != nil {
			return
		}
		a, _ := qc.Context().Value(attemptKey{}).(*attempt)
		if a == nil { // cannot happen: every connection passed connContext
			_ = qc.CloseWithError(0, "")
			continue
		}
		a.dequeued.Store(true)
		cs := qc.ConnectionState()
		if he := checkQUICState(cs, l.protocol); he != nil {
			l.fail(he.Reason)
			_ = qc.CloseWithError(0, "")
			a.release()
			continue
		}
		c := &QUICConn{Conn: qc, peer: peerFor(cs.TLS, x509.ExtKeyUsageClientAuth)}
		// The slot stays taken until the application has the connection, so
		// finished handshakes nobody accepts cannot pile up beyond the limit.
		select {
		case l.ready <- c:
			a.handed.Store(true)
			l.accepted.Add(1)
			a.release()
		case <-l.ctx.Done():
			_ = qc.CloseWithError(0, "")
			a.release()
			return
		}
	}
}

// SetSources replaces the admitted source addresses (the hop's new
// ingress_sources). It applies to the next connection attempt; connections
// already established are the carrier layer's to keep or close.
func (l *QUICListener) SetSources(items []string) error { return l.setSources(items) }

// SetPeers replaces the identities that may dial and returns the identities
// that were allowed before and no longer are, with the same meaning as
// Listener.SetPeers: handshakes by a removed identity fail from now on, and the
// caller closes the connections already authenticated as one of them.
func (l *QUICListener) SetPeers(ids []string) (removed []string, err error) {
	return l.setPeers(ids)
}

// PeerAllowed reports whether id is one of the identities that may dial now.
func (l *QUICListener) PeerAllowed(id string) bool { return l.peerAllowed(id) }

// Stats returns a snapshot of the counters.
func (l *QUICListener) Stats() ListenerStats {
	s := ListenerStats{Accepted: l.accepted.Load(), Pending: int(l.pendingN.Load()), Retries: l.retries.Load()}
	for i := range s.Failures {
		s.Failures[i] = l.failures[i].Load()
	}
	return s
}

// StopAccepting stops accepting and abandons the handshakes in flight. The
// connections already returned by Accept, and the UDP socket they run on, stay
// up, so the carrier layer can drain its carriers before Close.
func (l *QUICListener) StopAccepting() error {
	l.stop.Do(func() {
		l.cancel()
		l.stopE = l.inner.Close()
		l.wg.Wait()
	})
	return l.stopE
}

// Close stops accepting, abandons the handshakes in flight, closes every
// connection the listener's transport still has (the carrier layer drains its
// carriers first) and the UDP socket, and waits for the listener's goroutines.
func (l *QUICListener) Close() error {
	_ = l.StopAccepting()
	l.once.Do(func() {
		l.closeE = l.tr.Close()
		_ = l.pc.Close()
	})
	return l.closeE
}

// String describes the listener for logs.
func (l *QUICListener) String() string {
	return fmt.Sprintf("link quic listener on %s", l.pc.LocalAddr())
}

// classifyQUIC turns whatever quic-go or crypto/tls returned for a failed QUIC
// handshake into a HandshakeError. recorded is the typed error a verification
// callback returned, when one ran.
func classifyQUIC(err, recorded error) *HandshakeError {
	for _, e := range []error{recorded, err} {
		var he *HandshakeError
		if e != nil && errors.As(e, &he) {
			return he
		}
	}
	var (
		te  *quic.TransportError
		hto *quic.HandshakeTimeoutError
		ito *quic.IdleTimeoutError
		vne *quic.VersionNegotiationError
	)
	switch {
	case errors.As(err, &hto), errors.As(err, &ito), errors.Is(err, context.DeadlineExceeded):
		return &HandshakeError{Reason: ReasonTimeout, Err: err}
	case errors.As(err, &vne):
		return &HandshakeError{Reason: ReasonProtocol, Err: err}
	case errors.As(err, &te):
		return classifyTransport(te, err)
	}
	return classify(err, "")
}

func classifyTransport(te *quic.TransportError, err error) *HandshakeError {
	switch {
	case te.ErrorCode.IsCryptoError() && te.Remote:
		// The peer refused with a TLS alert.
		switch alert := tls.AlertError(te.ErrorCode & 0xff); alert { // #nosec G115 -- masked to 8 bits
		case 70, 120: // protocol_version, no_application_protocol
			return &HandshakeError{Reason: ReasonProtocol, Err: err}
		}
		return &HandshakeError{Reason: ReasonRemoteRejected, Err: err}
	case te.ErrorCode.IsCryptoError():
		// This end refused and sent the alert. crypto/tls's typed errors
		// survive in the chain when quic-go kept the local error.
		he := classify(err, "")
		if he.Reason == ReasonOther {
			he.Reason = classifyText(err.Error())
		}
		return he
	case te.Remote && te.ErrorCode == quic.ConnectionRefused:
		return &HandshakeError{Reason: ReasonRemoteRejected, Err: err}
	}
	return classify(err, "")
}

// classifyText recognises crypto/tls's verification failures from their text,
// for the cases where only the text of a local refusal reached the caller.
func classifyText(msg string) Reason {
	switch {
	case strings.Contains(msg, "certificate signed by unknown authority"):
		return ReasonUnknownCA
	case strings.Contains(msg, "certificate is valid for"):
		return ReasonIdentityMismatch
	case strings.Contains(msg, "expired or is not yet valid"),
		strings.Contains(msg, "didn't provide a certificate"),
		strings.Contains(msg, "x509:"):
		return ReasonCertificate
	case strings.Contains(msg, "no application protocol"), strings.Contains(msg, "protocol version"):
		return ReasonProtocol
	}
	return ReasonOther
}
