package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ListenerConfig configures an encrypted link listener, the next hop's end
// of a link.
type ListenerConfig struct {
	// Credentials are this node's link credentials. Required.
	Credentials *Credentials
	// Protocol is the ALPN protocol both ends must negotiate. Required.
	Protocol string
	// Sources are the hop's ingress_sources: the addresses admitted, checked
	// before any handshake bytes are read. At least one is required.
	Sources []string
	// Peers are the hop's ingress_peers: the SPIFFE IDs of the nodes that may
	// dial, canonical. At least one is required.
	Peers []string
	// HandshakeTimeout bounds each TLS handshake; the default is
	// DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration
	// MaxPending is the number of handshakes in flight at once; one more is
	// closed at once. The default is DefaultMaxPending.
	MaxPending int
}

// PlainListenerConfig configures a plaintext link listener.
type PlainListenerConfig struct {
	// TrustedLink must be true; see PlainDialConfig.TrustedLink.
	TrustedLink bool
	// Sources are the hop's ingress_sources. At least one is required: on a
	// plaintext link they are the only authentication there is.
	Sources []string
}

// Listener accepts link connections: it checks each connection's source
// address before reading a byte of it (anixops-protocol.md section 2.5),
// bounds the handshakes in flight, runs the handshake under a deadline, and
// returns only connections whose peer passed every check of its kind. The
// sources, the peers and the credentials change in place (SetSources,
// SetPeers, Credentials.Reload) without re-creating the listener.
type Listener struct {
	policy  // credentials, protocol, ingress sources and peers
	inner   net.Listener
	timeout time.Duration

	pending chan struct{} // a slot per handshake in flight
	ready   chan *Conn

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	inflight map[net.Conn]struct{}
	closed   bool

	accepted atomic.Uint64
	failures [NumReasons]atomic.Uint64
}

// Listen listens on address (TCP) for encrypted link connections.
func Listen(address string, cfg ListenerConfig) (*Listener, error) {
	l, err := newListener(cfg)
	if err != nil {
		return nil, err
	}
	inner, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	l.start(inner)
	return l, nil
}

// NewListener is Listen over an existing listener, which the Listener owns.
func NewListener(inner net.Listener, cfg ListenerConfig) (*Listener, error) {
	l, err := newListener(cfg)
	if err != nil {
		return nil, err
	}
	l.start(inner)
	return l, nil
}

func newListener(cfg ListenerConfig) (*Listener, error) {
	switch {
	case cfg.Credentials == nil:
		return nil, errors.New("link: an encrypted listener needs credentials")
	case cfg.Protocol == "":
		return nil, errors.New("link: an encrypted listener needs an ALPN protocol")
	case cfg.HandshakeTimeout < 0 || cfg.MaxPending < 0:
		return nil, errors.New("link: negative listener limits")
	}
	l := &Listener{timeout: cfg.HandshakeTimeout}
	if err := l.init(cfg.Credentials, cfg.Protocol, cfg.Sources, cfg.Peers); err != nil {
		return nil, err
	}
	if l.timeout == 0 {
		l.timeout = DefaultHandshakeTimeout
	}
	maxPending := cfg.MaxPending
	if maxPending == 0 {
		maxPending = DefaultMaxPending
	}
	l.pending = make(chan struct{}, maxPending)
	return l, nil
}

// ListenPlain listens on address (TCP) for plaintext link connections.
func ListenPlain(address string, cfg PlainListenerConfig) (*Listener, error) {
	l, err := newPlainListener(cfg)
	if err != nil {
		return nil, err
	}
	inner, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	l.start(inner)
	return l, nil
}

// NewPlainListener is ListenPlain over an existing listener, which the
// Listener owns.
func NewPlainListener(inner net.Listener, cfg PlainListenerConfig) (*Listener, error) {
	l, err := newPlainListener(cfg)
	if err != nil {
		return nil, err
	}
	l.start(inner)
	return l, nil
}

func newPlainListener(cfg PlainListenerConfig) (*Listener, error) {
	if !cfg.TrustedLink {
		return nil, errors.New("link: a plaintext link is for trusted links only (PlainListenerConfig.TrustedLink)")
	}
	l := &Listener{}
	if err := l.init(nil, "", cfg.Sources, nil); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Listener) start(inner net.Listener) {
	l.inner = inner
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.ready = make(chan *Conn)
	l.inflight = make(map[net.Conn]struct{})
	l.wg.Add(1)
	go l.acceptLoop()
}

// Addr returns the listening address.
func (l *Listener) Addr() net.Addr { return l.inner.Addr() }

// Encrypted reports whether the listener runs TLS (it is not a plaintext
// listener).
func (l *Listener) Encrypted() bool { return l.creds != nil }

// Accept returns the next admitted connection, after its source was checked
// and, on an encrypted listener, its handshake and identity checks passed. It
// returns net.ErrClosed after Close.
func (l *Listener) Accept() (*Conn, error) {
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	}
}

// Close stops accepting, abandons the handshakes in flight, closes the
// listening socket and waits for the listener's goroutines. Connections
// already returned by Accept belong to the caller.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	for c := range l.inflight {
		_ = c.Close()
	}
	l.mu.Unlock()
	l.cancel()
	err := l.inner.Close()
	l.wg.Wait()
	return err
}

// SetSources replaces the admitted source addresses (the hop's new
// ingress_sources). It applies to the next connection accepted; connections
// already established are the carrier layer's to keep or close.
func (l *Listener) SetSources(items []string) error { return l.setSources(items) }

// SetPeers replaces the identities that may dial (the hop's new
// ingress_peers) and returns the identities that were allowed before and no
// longer are, sorted. From the moment it returns, a handshake by a removed
// identity fails (ReasonPeerNotAllowed); the caller closes the carriers
// already authenticated as one of them, which is the revocation path of
// anixops-protocol.md section 3.5 (no CRL is consulted). It is an error on a
// plaintext listener, which has no peers.
func (l *Listener) SetPeers(ids []string) (removed []string, err error) {
	return l.setPeers(ids)
}

// PeerAllowed reports whether id is one of the identities that may dial now.
func (l *Listener) PeerAllowed(id string) bool { return l.peerAllowed(id) }

// ListenerStats are the listener's counters.
type ListenerStats struct {
	// Accepted counts connections returned or ready to be returned by Accept.
	Accepted uint64
	// Pending is the number of handshakes in flight.
	Pending int
	// Failures counts the connections refused, by Reason.
	Failures [NumReasons]uint64
	// Retries counts the QUIC address validations (Retry packets) a
	// QUICListener asked for because its handshake queue was over half full;
	// zero on the TCP listeners.
	Retries uint64
}

// Stats returns a snapshot of the counters.
func (l *Listener) Stats() ListenerStats {
	s := ListenerStats{Accepted: l.accepted.Load(), Pending: len(l.pending)}
	for i := range s.Failures {
		s.Failures[i] = l.failures[i].Load()
	}
	return s
}

func (l *Listener) fail(reason Reason) { l.failures[reason].Add(1) }

func (l *Listener) acceptLoop() {
	defer l.wg.Done()
	var backoff time.Duration
	for {
		raw, err := l.inner.Accept()
		if err != nil {
			if l.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			// A temporary failure such as running out of file descriptors:
			// wait and retry rather than end the listener.
			if backoff == 0 {
				backoff = 5 * time.Millisecond
			} else {
				backoff = min(2*backoff, time.Second)
			}
			select {
			case <-time.After(backoff):
				continue
			case <-l.ctx.Done():
				return
			}
		}
		backoff = 0
		// Admission comes first: an address outside ingress_sources costs
		// nothing but the close, in particular no signature.
		if !l.sources.Load().Contains(addrPort(raw.RemoteAddr()).Addr()) {
			l.fail(ReasonSourceNotAllowed)
			_ = raw.Close()
			continue
		}
		_ = tuneConn(raw)
		if l.creds == nil {
			l.wg.Add(1)
			go l.deliver(&Conn{Conn: raw}, nil)
			continue
		}
		select {
		case l.pending <- struct{}{}:
		default:
			l.fail(ReasonHandshakeLimit)
			_ = raw.Close()
			continue
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			<-l.pending
			_ = raw.Close()
			return
		}
		l.inflight[raw] = struct{}{}
		l.mu.Unlock()
		l.wg.Add(1)
		go l.handshake(raw)
	}
}

// handshake runs the TLS handshake of one admitted connection and hands the
// verified connection to Accept.
func (l *Listener) handshake(raw net.Conn) {
	slot := func() { <-l.pending }
	done := func() {
		l.mu.Lock()
		delete(l.inflight, raw)
		l.mu.Unlock()
	}
	c, err := l.handshakeConn(raw)
	done()
	if err != nil {
		slot()
		l.fail(classify(err, "").Reason)
		_ = raw.Close()
		l.wg.Done()
		return
	}
	l.deliver(c, slot)
}

// handshakeConn runs the handshake on a connection that was admitted. It is
// separate so tests can drive it over any net.Conn.
func (l *Listener) handshakeConn(raw net.Conn) (*Conn, error) {
	ctx, cancel := context.WithTimeout(l.ctx, l.timeout)
	defer cancel()
	tc := tls.Server(raw, l.serverTLS())
	if err := tc.HandshakeContext(ctx); err != nil {
		return nil, classify(err, "")
	}
	return &Conn{Conn: tc, peer: peerFor(tc.ConnectionState(), x509.ExtKeyUsageClientAuth)}, nil
}

// deliver hands a connection to Accept, holding the handshake slot (if any)
// until it is taken so that connections nobody accepts cannot pile up beyond
// the pending limit. The caller has added one to the wait group.
func (l *Listener) deliver(c *Conn, slot func()) {
	defer l.wg.Done()
	if slot != nil {
		defer slot()
	}
	select {
	case l.ready <- c:
		l.accepted.Add(1)
	case <-l.ctx.Done():
		_ = c.Close()
	}
}

// String describes the listener for logs.
func (l *Listener) String() string {
	kind := "plain"
	if l.creds != nil {
		kind = "tls"
	}
	return fmt.Sprintf("link %s listener on %s", kind, l.inner.Addr())
}
