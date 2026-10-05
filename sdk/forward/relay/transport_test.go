package relay

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

func nodeID(node string) string { return relaytest.Identity("test", node) }

func linkCreds(t testing.TB, ca *relaytest.PKI, bundle []byte, node string) *link.Credentials {
	t.Helper()
	certPEM, keyPEM, err := ca.Issue(relaytest.Cert{Node: node})
	if err != nil {
		t.Fatal(err)
	}
	c, err := link.NewCredentials(certPEM, keyPEM, bundle)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type tlsEnv struct {
	t      *testing.T
	ca     *relaytest.PKI
	server *link.Credentials
	l      *Listener
}

// newTLSEnv starts a TLS carrier listener for forward-2 that admits
// forward-1 (and the given extra peers) from loopback.
func newTLSEnv(t *testing.T, ccfg Config, peers ...string) *tlsEnv {
	t.Helper()
	ca, err := relaytest.New("a")
	if err != nil {
		t.Fatal(err)
	}
	e := &tlsEnv{t: t, ca: ca, server: linkCreds(t, ca, ca.CAPEM(), "forward-2")}
	e.l, err = Listen("127.0.0.1:0", link.ListenerConfig{
		Credentials: e.server,
		Sources:     []string{"127.0.0.1"},
		Peers:       append([]string{nodeID("forward-1")}, peers...),
	}, ccfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.l.Close() })
	return e
}

func (e *tlsEnv) dial(creds *link.Credentials, ccfg Config) (*Carrier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	return DialTLS(ctx, e.l.Addr().String(), link.DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}, ccfg)
}

// pair dials as forward-1 and returns both ends of the carrier.
func (e *tlsEnv) pair(ccfg Config) (d, a *Carrier) {
	e.t.Helper()
	d, err := e.dial(linkCreds(e.t, e.ca, e.ca.CAPEM(), "forward-1"), ccfg)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = d.Close() })
	a, err = e.l.Accept(ctxTimeout(e.t))
	if err != nil {
		e.t.Fatal(err)
	}
	return d, a
}

func TestTLSCarrierEndToEnd(t *testing.T) {
	e := newTLSEnv(t, Config{})
	d, a := e.pair(Config{})
	serveEcho(t, a)
	if d.Type() != CarrierTLS || a.Type() != CarrierTLS || CarrierTLS.String() != "tls_tcp" {
		t.Fatalf("types %v %v", d.Type(), a.Type())
	}
	if p := d.Peer(); p.Identity != nodeID("forward-2") || !p.Encrypted() {
		t.Fatalf("dialler's peer %+v", p)
	}
	if p := a.Peer(); p.Identity != nodeID("forward-1") || !p.Encrypted() {
		t.Fatalf("listener's peer %+v", p)
	}
	for _, n := range []int{0, 1, 100_000, 3 * DefaultStreamWindow} {
		data := pattern(n, byte(n))
		eqBytes(t, roundTrip(t, d, data), data)
	}
	if st := e.l.Stats(); st.Carriers != 1 || st.Accepted != 1 || st.Link.Accepted != 1 {
		t.Fatalf("stats %+v", st)
	}
	if e.l.Link() == nil || e.l.Addr() == nil {
		t.Fatal("accessors")
	}
	if CarrierType(0).String() != "raw" || CarrierPlain.String() != "plain" {
		t.Fatal("names")
	}
}

func TestTLSCarrierRefusals(t *testing.T) {
	e := newTLSEnv(t, Config{})
	t.Run("a genuine node that is not listed", func(t *testing.T) {
		_, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-3"), Config{})
		if err == nil {
			t.Fatal("dial succeeded")
		}
		var he *link.HandshakeError
		if !errors.As(err, &he) || he.Reason != link.ReasonRemoteRejected {
			t.Fatalf("err = %v, want remote_rejected", err)
		}
		waitFor(t, "peer_not_allowed counted", func() bool {
			return e.l.Stats().Link.Failures[link.ReasonPeerNotAllowed] == 1
		})
	})
	t.Run("the wrong listener", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testWait)
		defer cancel()
		_, err := DialTLS(ctx, e.l.Addr().String(), link.DialConfig{Credentials: linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-9")}, Config{})
		if link.ReasonOf(err) != link.ReasonIdentityMismatch {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("another ALPN protocol", func(t *testing.T) {
		_, err := DialTLS(t.Context(), e.l.Addr().String(), link.DialConfig{Credentials: linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2"), Protocol: "anixops/1"}, Config{})
		if err == nil {
			t.Fatal("dialled with another protocol")
		}
		if _, err := Listen("127.0.0.1:0", link.ListenerConfig{Credentials: e.server, Protocol: "h2", Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, Config{}); err == nil {
			t.Fatal("listened with another protocol")
		}
	})
	t.Run("a bad carrier config", func(t *testing.T) {
		if _, err := Listen("127.0.0.1:0", link.ListenerConfig{Credentials: e.server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, Config{MaxFrame: 1}); err == nil {
			t.Fatal("bad config accepted")
		}
	})
	expectNoCarrier(t, e.l)
}

func expectNoCarrier(t *testing.T, l *Listener) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if c, err := l.Accept(ctx); err == nil {
		_ = c.Close()
		t.Fatal("a refused carrier was accepted")
	}
}

// L1: a carrier belongs to its listener. Closing the listener sends GOAWAY on
// every carrier and closes them when their streams end or after the drain.
func TestListenerCloseDrainsItsCarriers(t *testing.T) {
	t.Run("streams finish within the drain", func(t *testing.T) {
		e := newTLSEnv(t, Config{DrainTimeout: 3 * time.Second})
		d, a := e.pair(Config{})
		serveEcho(t, a)
		s, _ := d.Open(testParams())
		_, _ = s.Write([]byte("hello"))
		buf := make([]byte, 5)
		if _, err := io.ReadFull(s, buf); err != nil {
			t.Fatal(err)
		}
		closed := make(chan struct{})
		go func() { _ = e.l.Close(); close(closed) }()
		select {
		case <-d.Draining():
		case <-time.After(testWait):
			t.Fatal("no GOAWAY")
		}
		if r, ok := d.PeerGoAway(); !ok || r != GoAwayListenerClosed {
			t.Fatalf("PeerGoAway = %v %v", r, ok)
		}
		if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
			t.Fatalf("Open = %v", err)
		}
		select {
		case <-closed:
			t.Fatal("Close returned with a stream open")
		case <-time.After(100 * time.Millisecond):
		}
		_, _ = s.Write([]byte("more!"))
		if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "more!" {
			t.Fatalf("stream after GOAWAY: %q %v", buf, err)
		}
		_ = s.CloseWrite()
		_, _ = io.Copy(io.Discard, s)
		_ = s.Close()
		select {
		case <-closed:
		case <-time.After(testWait):
			t.Fatal("Close did not return after the streams ended")
		}
		waitDone(t, d)
		waitDone(t, a)
		if _, err := e.l.Accept(context.Background()); err != net.ErrClosed {
			t.Fatalf("Accept after Close = %v", err)
		}
		if _, err := net.DialTimeout("tcp", e.l.Addr().String(), 200*time.Millisecond); err == nil {
			t.Fatal("the port still listens")
		}
	})
	t.Run("a stream that never ends is cut at the drain deadline", func(t *testing.T) {
		e := newTLSEnv(t, Config{DrainTimeout: 200 * time.Millisecond})
		d, a := e.pair(Config{})
		serveEcho(t, a)
		s, _ := d.Open(testParams())
		_, _ = s.Write([]byte("x"))
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := e.l.Close(); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took < 150*time.Millisecond || took > testWait {
			t.Fatalf("Close took %v", took)
		}
		waitDone(t, d)
		waitDone(t, a)
		if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrCarrierClosed) {
			t.Fatalf("stream = %v", err)
		}
	})
	t.Run("a carrier that is accepted but never taken is closed too", func(t *testing.T) {
		e := newTLSEnv(t, Config{DrainTimeout: 200 * time.Millisecond})
		d, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), Config{})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "carrier registered", func() bool { return e.l.Stats().Carriers == 1 })
		_ = e.l.Close()
		waitDone(t, d)
	})
}

// Section 3.5: removing an identity from ingress_peers closes the carriers
// authenticated as it, with a GOAWAY the dialler can read, and its next
// handshake fails.
func TestSetPeersClosesTheRemovedPeersCarriers(t *testing.T) {
	e := newTLSEnv(t, Config{}, nodeID("forward-3"))
	d1, a1 := e.pair(Config{})
	d3, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-3"), Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d3.Close() }()
	a3, err := e.l.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	serveEcho(t, a1)
	serveEcho(t, a3)
	s, _ := d1.Open(testParams())
	_, _ = s.Write([]byte("x"))
	_ = s.AwaitResult(ctxTimeout(t))

	removed, err := e.l.SetPeers([]string{nodeID("forward-3")})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != nodeID("forward-1") {
		t.Fatalf("removed %v", removed)
	}
	waitDone(t, d1)
	waitDone(t, a1)
	if r, ok := d1.PeerGoAway(); !ok || r != GoAwayPeerNotAllowed {
		t.Fatalf("the dialler saw GOAWAY %v %v, want peer_not_allowed", r, ok)
	}
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("stream = %v", err)
	}
	// forward-3 is untouched
	eqBytes(t, roundTrip(t, d3, []byte("still here")), []byte("still here"))
	// and forward-1 cannot come back
	if _, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), Config{}); err == nil {
		t.Fatal("a removed peer connected again")
	}
	waitFor(t, "carrier forgotten", func() bool { return e.l.Stats().Carriers == 1 })
	if _, err := e.l.SetPeers(nil); err == nil {
		t.Fatal("SetPeers(nil) succeeded")
	}
}

// Section 3.5: a carrier whose peer chain no longer verifies under the new
// trust bundle is closed at once, on the listener (its bundle changed) and on
// the dialler (WatchCredentials).
func TestTrustBundleChangeClosesCarriers(t *testing.T) {
	e := newTLSEnv(t, Config{})
	clientCreds := linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1")
	ca2, err := relaytest.New("b")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("listener", func(t *testing.T) {
		d, err := e.dial(clientCreds, Config{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = d.Close() }()
		a, err := e.l.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		// the listener moves to the next CA and drops the old one
		certPEM, keyPEM, _ := ca2.Issue(relaytest.Cert{Node: "forward-2"})
		if err := e.server.Reload(certPEM, keyPEM, ca2.CAPEM()); err != nil {
			t.Fatal(err)
		}
		waitDone(t, a)
		waitDone(t, d)
		if r, ok := d.PeerGoAway(); !ok || r != GoAwayCredentials {
			t.Fatalf("GOAWAY %v %v", r, ok)
		}
	})

	t.Run("dialler", func(t *testing.T) {
		// back to a listener on the first CA
		e2 := newTLSEnv(t, Config{})
		creds := linkCreds(t, e2.ca, relaytest.Bundle(e2.ca, ca2), "forward-1")
		d, err := e2.dial(creds, Config{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = d.Close() }()
		a, err := e2.l.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		stop := WatchCredentials(creds, d)
		defer stop()
		// the dialler drops the CA its listener's certificate chains to
		certPEM, keyPEM, _ := e2.ca.Issue(relaytest.Cert{Node: "forward-1"})
		if err := creds.Reload(certPEM, keyPEM, relaytest.Bundle(e2.ca)); err != nil {
			t.Fatal(err) // trusts e2.ca still: nothing happens
		}
		time.Sleep(50 * time.Millisecond)
		if d.Err() != nil || a.Err() != nil {
			t.Fatalf("a reload that keeps the CA closed the carrier: %v / %v", d.Err(), a.Err())
		}
		certPEM, keyPEM, _ = ca2.Issue(relaytest.Cert{Node: "forward-1"})
		if err := creds.Reload(certPEM, keyPEM, ca2.CAPEM()); err != nil {
			t.Fatal(err)
		}
		waitDone(t, d)
		waitDone(t, a)
		// a plaintext carrier has nothing to watch
		WatchCredentials(creds, &Carrier{})()
		WatchCredentials(nil, d)()
	})
}

func TestPlainCarrier(t *testing.T) {
	if _, err := ListenPlain("127.0.0.1:0", link.PlainListenerConfig{Sources: []string{"127.0.0.1"}}, Config{}); err == nil {
		t.Fatal("a plain listener without TrustedLink")
	}
	l, err := ListenPlain("127.0.0.1:0", link.PlainListenerConfig{TrustedLink: true, Sources: []string{"127.0.0.1"}}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if _, err := l.SetPeers([]string{nodeID("forward-1")}); err == nil {
		t.Fatal("SetPeers on a plain listener")
	}
	if _, err := DialPlain(t.Context(), l.Addr().String(), link.PlainDialConfig{}, Config{}); err == nil {
		t.Fatal("a plain dial without TrustedLink")
	}
	d, err := DialPlain(t.Context(), l.Addr().String(), link.PlainDialConfig{TrustedLink: true}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	a, err := l.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	serveEcho(t, a)
	if d.Type() != CarrierPlain || a.Type() != CarrierPlain || a.Peer().Encrypted() || a.Peer().Identity != "" {
		t.Fatalf("a plain carrier has no identity: %v %v %+v", d.Type(), a.Type(), a.Peer())
	}
	data := pattern(200_000, 3)
	eqBytes(t, roundTrip(t, d, data), data)

	// source admission is the only check
	if err := l.SetSources([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	if c, err := DialPlain(t.Context(), l.Addr().String(), link.PlainDialConfig{TrustedLink: true, Timeout: time.Second}, Config{HandshakeTimeout: time.Second}); err == nil {
		_ = c.Close()
		t.Fatal("a refused source got a carrier")
	}
	waitFor(t, "refusal counted", func() bool { return l.Stats().Link.Failures[link.ReasonSourceNotAllowed] == 1 })

	// L1 holds for plain carriers too
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	waitDone(t, d)
	waitDone(t, a)
}

// Too many admitted connections that never finish the SETTINGS exchange are
// closed at once instead of piling up.
func TestSettingsExchangesAreBounded(t *testing.T) {
	l, err := ListenPlain("127.0.0.1:0", link.PlainListenerConfig{TrustedLink: true, Sources: []string{"127.0.0.1"}}, Config{MaxPending: 2, HandshakeTimeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	var silent []net.Conn
	for range 2 {
		c, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		silent = append(silent, c)
	}
	waitFor(t, "two exchanges in flight", func() bool { return len(l.slots) == 2 })
	extra, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extra.Close() }()
	_ = extra.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := extra.Read(make([]byte, 1)); err == nil || isTimeout(err) {
		t.Fatalf("read = %v, want the extra connection closed at once", err)
	}
	waitFor(t, "overload counted", func() bool { return l.Stats().Overloaded == 1 })
	waitFor(t, "the silent ones to time out", func() bool { return l.Stats().SettingsFailed == 2 })
	d, err := DialPlain(t.Context(), l.Addr().String(), link.PlainDialConfig{TrustedLink: true}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if _, err := l.Accept(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	_ = silent
}

// A TLS Close tries to send close_notify, which blocks for seconds when the
// peer is dead and the socket is full: closing a carrier must not wait for it.
func TestCloseDoesNotWaitForADeadPeer(t *testing.T) {
	ca, err := relaytest.New("a")
	if err != nil {
		t.Fatal(err)
	}
	ll, err := link.Listen("127.0.0.1:0", link.ListenerConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-2"), Protocol: ALPN, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ll.Close() }()
	// The "peer" completes the SETTINGS exchange by hand, announcing huge
	// windows, and then never reads again.
	go func() {
		conn, err := ll.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		huge := Settings{MaxStreams: 8, MaxFrame: MaxMaxFrame, StreamWindow: MaxStreamWindow, CarrierWindow: MaxCarrierWindow}
		_ = WriteFrame(conn, Frame{Type: TypeSettings, Payload: huge.Marshal()})
		_, _ = ReadFrame(conn, MaxMaxFrame)
		time.Sleep(30 * time.Second)
	}()
	d, err := DialTLS(t.Context(), ll.Addr().String(), link.DialConfig{Credentials: linkCreds(t, ca, ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}, Config{SendBuffer: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.Open(testParams())
	if err != nil {
		t.Fatal(err)
	}
	// Fill the socket: the writer ends up blocked in a TLS write.
	go func() {
		buf := make([]byte, 64*1024)
		for {
			if _, err := s.Write(buf); err != nil {
				return
			}
		}
	}()
	time.Sleep(500 * time.Millisecond)
	start := time.Now()
	_ = d.Close()
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Close took %v with a dead peer", took)
	}
}

// stuckClose models a TLS connection whose Close blocks sending close_notify
// into a full socket: it returns only when the connection under it closes.
type stuckClose struct {
	net.Conn
	inner *notifyClose
}

func (s *stuckClose) NetConn() net.Conn { return s.inner }

func (s *stuckClose) Close() error {
	select {
	case <-s.inner.closed:
	case <-time.After(10 * time.Second):
	}
	return s.inner.Close()
}

type notifyClose struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (n *notifyClose) Close() error {
	n.once.Do(func() { close(n.closed) })
	return n.Conn.Close()
}

func TestAbortClosesBelowTheTLSLayer(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = b.Close() }()
	tcp := &notifyClose{Conn: a, closed: make(chan struct{})}
	wrapped := &stuckClose{Conn: tcp, inner: tcp}
	type res struct {
		c   *Carrier
		err error
	}
	ch := make(chan res, 1)
	go func() { c, err := NewCarrier(b, RoleAcceptor, Config{}); ch <- res{c, err} }()
	d, err := NewCarrier(wrapped, RoleDialer, Config{})
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer func() { _ = r.c.Close() }()
	start := time.Now()
	_ = d.Close()
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("Close took %v: it waited for the stuck TLS Close", took)
	}
}
