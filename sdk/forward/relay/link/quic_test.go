package link

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

func TestQUICLink(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")})
	if !l.Encrypted() || l.String() == "" {
		t.Fatal("listener kind")
	}

	c, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := acceptQUIC(t, l)
	defer func() { _ = in.Close() }()

	// each end knows exactly who is at the other
	if p := c.Peer(); p.Identity != id("forward-2") || !p.Encrypted() || !c.Encrypted() {
		t.Fatalf("dialler sees %+v", p)
	}
	if p := in.Peer(); p.Identity != id("forward-1") || !p.Encrypted() {
		t.Fatalf("listener sees %+v", p)
	}
	if ap := in.RemoteAddrPort(); !ap.Addr().IsLoopback() || ap.Port() == 0 {
		t.Fatalf("remote address %v", ap)
	}
	// QUIC version 1, TLS 1.3, the ALPN protocol, no resumption, no early data
	// on either side, and DATAGRAM frames negotiated.
	for _, qc := range []*QUICConn{c, in} {
		cs := qc.ConnectionState()
		if cs.Version != quic.Version1 || cs.TLS.Version != tls.VersionTLS13 || cs.TLS.NegotiatedProtocol != testProto ||
			cs.TLS.DidResume || cs.Used0RTT || !cs.SupportsDatagrams.Local || !cs.SupportsDatagrams.Remote {
			t.Fatalf("session %+v", cs)
		}
	}
	// a stream carries bytes both ways
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	st, err := c.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("ping over the link")); err != nil {
		t.Fatal(err)
	}
	peerSt, err := in.AcceptStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("ping over the link"))
	_ = peerSt.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(peerSt, buf); err != nil || string(buf) != "ping over the link" {
		t.Fatalf("read %q %v", buf, err)
	}
	// The counters settle just after Accept returns.
	waitFor(t, "the counters to settle", func() bool {
		st := l.Stats()
		return st.Accepted == 1 && st.Pending == 0
	})
	if st := l.Stats(); st.Retries != 0 {
		t.Fatalf("an idle listener asked for a Retry: %+v", st)
	}
}

// The dialler accepts only the node the state names, over QUIC as over TLS:
// the same certificates fail for the same reasons.
func TestQUICDiallerPinsTheListener(t *testing.T) {
	f := newFixture(t)
	client := f.creds("forward-1")
	for _, tc := range diallerPinCases(t, f) {
		t.Run(tc.name, func(t *testing.T) {
			l := listenQUIC(t, tc.server, []string{id("forward-1")})
			c, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
			if err == nil {
				_ = c.Close()
				t.Fatal("the dial succeeded")
			}
			var he *HandshakeError
			if !errors.As(err, &he) || he.Reason != tc.want || ReasonOf(err) != tc.want {
				t.Fatalf("err = %v (reason %v), want %v", err, ReasonOf(err), tc.want)
			}
		})
	}
}

// The listener admits only the nodes the state lists, over QUIC as over TLS.
func TestQUICListenerPinsTheDiallers(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	for _, tc := range listenerPinCases(t, f) {
		t.Run(tc.name, func(t *testing.T) {
			l := listenQUIC(t, server, []string{id("forward-1")})
			c, err := dialQUIC(t, l, tc.client, "forward-2", id("forward-2"))
			if err == nil {
				// The client finishes first, so the refusal arrives right
				// behind as the listener's close.
				expectRefused(t, c.Conn)
				_ = c.Close()
			}
			waitFor(t, "the refusal to be counted", func() bool { return quicFailures(l, tc.want) == 1 })
			if st := l.Stats(); st.Accepted != 0 {
				t.Fatalf("stats %+v", st)
			}
			expectNoAcceptQUIC(t, l, 50*time.Millisecond)
			waitFor(t, "the slot to come back", func() bool { return l.Stats().Pending == 0 })
		})
	}
}

func TestQUICClientWithoutACertificateIsRefused(t *testing.T) {
	f := newFixture(t)
	l := listenQUIC(t, f.creds("forward-2"), []string{id("forward-1")})
	c, err := rawQUICDial(t, l.Addr().String(), rawQUICTLS(t, f, nil), nil)
	if err == nil {
		expectRefused(t, c)
	}
	waitFor(t, "the refusal", func() bool { return quicFailures(l, ReasonCertificate) == 1 })
}

// The ALPN protocol is required of whoever connects, and a session is never
// resumed and early data is never accepted.
func TestQUICSessionRequirements(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	clientCreds := f.creds("forward-1")
	cert := clientCreds.state.Load().cert

	tests := []struct {
		name string
		alpn []string
		want Reason
	}{
		{"another ALPN protocol", []string{"h3"}, ReasonProtocol},
		{"the production ALPN version", []string{"anixops/1"}, ReasonProtocol},
		{"no ALPN", nil, ReasonProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := listenQUIC(t, server, []string{id("forward-1")})
			conf := rawQUICTLS(t, f, &cert)
			conf.NextProtos = tc.alpn
			c, err := rawQUICDial(t, l.Addr().String(), conf, nil)
			if err == nil {
				expectRefused(t, c)
			}
			waitFor(t, "the refusal", func() bool { return quicFailures(l, tc.want) == 1 })
			if l.Stats().Accepted != 0 {
				t.Fatal("the connection was accepted")
			}
		})
	}

	t.Run("a listener of another ALPN protocol is refused by the dialler", func(t *testing.T) {
		srvCert := server.state.Load().cert
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rogue, err := quic.Listen(pc, &tls.Config{Certificates: []tls.Certificate{srvCert}, NextProtos: []string{"h3"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rogue.Close(); _ = pc.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), testWait)
		defer cancel()
		_, err = DialQUIC(ctx, pc.LocalAddr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: clientCreds, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}})
		if err == nil || ReasonOf(err) != ReasonProtocol {
			t.Fatalf("err = %v (reason %v), want a protocol refusal", err, ReasonOf(err))
		}
	})

	t.Run("only QUIC version 1", func(t *testing.T) {
		l := listenQUIC(t, server, []string{id("forward-1")})
		qcfg := quicTestConfig()
		qcfg.Versions = []quic.Version{quic.Version2}
		qcfg.HandshakeIdleTimeout = 200 * time.Millisecond
		_, err := rawQUICDial(t, l.Addr().String(), rawQUICTLS(t, f, &cert), qcfg)
		if err == nil {
			t.Fatal("a QUIC version 2 handshake succeeded")
		}
		if st := l.Stats(); st.Accepted != 0 || st.Pending != 0 {
			t.Fatalf("stats %+v: the listener spent state on a version it does not speak", st)
		}
	})

	t.Run("no resumption and no early data", func(t *testing.T) {
		l := listenQUIC(t, server, []string{id("forward-1")})
		conf := rawQUICTLS(t, f, &cert)
		conf.ClientSessionCache = tls.NewLRUClientSessionCache(8)
		for i := range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), testWait)
			c, err := quic.DialAddrEarly(ctx, l.Addr().String(), conf, quicTestConfig())
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			in := acceptQUIC(t, l)
			// let any session ticket the listener might send arrive
			st, err := c.OpenStreamSync(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, _ = st.Write([]byte("x"))
			ps, err := in.AcceptStream(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_ = ps.SetReadDeadline(time.Now().Add(testWait))
			if _, err := ps.Read(make([]byte, 1)); err != nil {
				t.Fatal(err)
			}
			if cs := c.ConnectionState(); cs.TLS.DidResume || cs.Used0RTT {
				t.Fatalf("handshake %d resumed a session or used 0-RTT: %+v", i, cs)
			}
			_ = c.CloseWithError(0, "")
			_ = in.Close()
		}
	})

	t.Run("a dialler never sends early data or resumes", func(t *testing.T) {
		// A listener that does everything a server may to invite resumption
		// and early data: the dialler (no ticket cache, no early dial) never
		// takes it.
		srvCert := server.state.Load().cert
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rogue, err := quic.ListenEarly(pc, &tls.Config{
			Certificates: []tls.Certificate{srvCert}, NextProtos: []string{testProto},
			ClientAuth: tls.RequireAnyClientCert, ClientCAs: nil,
		}, &quic.Config{Allow0RTT: true})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rogue.Close(); _ = pc.Close() }()
		go func() {
			for {
				c, err := rogue.Accept(context.Background())
				if err != nil {
					return
				}
				go func() { <-c.Context().Done() }()
			}
		}()
		for range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), testWait)
			c, err := DialQUIC(ctx, pc.LocalAddr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: clientCreds, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}})
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if cs := c.ConnectionState(); cs.TLS.DidResume || cs.Used0RTT {
				t.Fatalf("dialler resumed or used 0-RTT: %+v", cs)
			}
			_ = c.Close()
		}
	})
}

// A listener checks the source before it spends anything on a connection.
func TestQUICSourceAdmission(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")}, func(c *QUICListenerConfig) { c.Sources = []string{"10.0.0.0/8"} })
	_, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err == nil || ReasonOf(err) != ReasonRemoteRejected {
		t.Fatalf("err = %v (reason %v): a refused source is turned away", err, ReasonOf(err))
	}
	waitFor(t, "the refusal", func() bool { return quicFailures(l, ReasonSourceNotAllowed) >= 1 })
	if st := l.Stats(); st.Pending != 0 || st.Accepted != 0 {
		t.Fatalf("stats %+v: no slot is spent on a refused source", st)
	}
	for r, n := range l.Stats().Failures {
		if Reason(r) != ReasonSourceNotAllowed && n != 0 {
			t.Fatalf("a refused source was counted as %v, which means a handshake ran", Reason(r))
		}
	}

	// SetSources applies to the next attempt.
	if err := l.SetSources([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	c, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = acceptQUIC(t, l)
	if err := l.SetSources(nil); err == nil {
		t.Fatal("SetSources(nil) succeeded")
	}
}

// Handshakes in flight are bounded: the next attempt is refused at once, and
// the slots come back when a handshake ends.
func TestQUICHandshakeLimit(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")}, func(c *QUICListenerConfig) { c.MaxPending = 2 })
	addr := l.Addr().String()
	r1 := stalledClient(t, f, addr, client)
	r2 := stalledClient(t, f, addr, client)
	waitFor(t, "two handshakes in flight", func() bool { return l.Stats().Pending == 2 })

	// The third is turned away (after proving its address, since over half of
	// the slots are taken).
	_, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err == nil || ReasonOf(err) != ReasonRemoteRejected {
		t.Fatalf("err = %v (reason %v)", err, ReasonOf(err))
	}
	waitFor(t, "the refusal", func() bool { return quicFailures(l, ReasonHandshakeLimit) >= 1 })
	if st := l.Stats(); st.Retries == 0 || st.Pending != 2 {
		t.Fatalf("stats %+v", st)
	}

	// Releasing the stalled handshakes lets them complete, and the slots
	// return when the application takes the connections.
	r1()
	r2()
	for i := range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), testWait)
		c, err := l.Accept(ctx)
		cancel()
		if err != nil {
			t.Fatalf("accept %d: %v (stats %+v)", i, err, l.Stats())
		}
		_ = c
	}
	waitFor(t, "the slots to come back", func() bool { return l.Stats().Pending == 0 })
	c, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = acceptQUIC(t, l)
}

// Over half of the slots taken, a new address must prove itself with Retry
// before it gets a slot; with the queue short, nobody pays the round trip.
func TestQUICRetryWhenTheQueueFills(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")}, func(c *QUICListenerConfig) { c.MaxPending = 4 })
	addr := l.Addr().String()

	// an idle listener: no Retry
	c, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	_ = acceptQUIC(t, l)
	_ = c.Close()
	if r := l.Stats().Retries; r != 0 {
		t.Fatalf("%d Retry packets from an idle listener", r)
	}

	// three stalled handshakes of four slots: the next attempt is validated,
	// and succeeds.
	for range 3 {
		stalledClient(t, f, addr, client)
	}
	waitFor(t, "three handshakes in flight", func() bool { return l.Stats().Pending == 3 })
	c, err = dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = acceptQUIC(t, l)
	if r := l.Stats().Retries; r == 0 {
		t.Fatal("no Retry was asked for although the queue was over half full")
	}
}

// A handshake that does not finish is abandoned at the deadline and its slot
// comes back.
func TestQUICHandshakeDeadline(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")}, func(c *QUICListenerConfig) { c.HandshakeTimeout = 400 * time.Millisecond })
	stalledClient(t, f, l.Addr().String(), client)
	waitFor(t, "the handshake in flight", func() bool { return l.Stats().Pending == 1 })
	waitFor(t, "the deadline", func() bool { return quicFailures(l, ReasonTimeout) == 1 && l.Stats().Pending == 0 })
}

// Close abandons the handshakes in flight and does not wait for their
// deadline.
func TestQUICCloseAbandonsPendingHandshakes(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listenQUIC(t, server, []string{id("forward-1")}, func(c *QUICListenerConfig) { c.HandshakeTimeout = time.Hour })
	stalledClient(t, f, l.Addr().String(), client)
	waitFor(t, "the handshake in flight", func() bool { return l.Stats().Pending == 1 })
	done := make(chan struct{})
	go func() { _ = l.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(testWait):
		t.Fatal("Close waited for the handshake deadline")
	}
	if _, err := l.Accept(context.Background()); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close = %v", err)
	}
}

// StopAccepting leaves the connections already handed out running, which is
// what lets the carrier layer drain them before Close.
func TestQUICStopAcceptingKeepsConnections(t *testing.T) {
	f := newFixture(t)
	l := listenQUIC(t, f.creds("forward-2"), []string{id("forward-1")})
	c, err := dialQUIC(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := acceptQUIC(t, l)
	if err := l.StopAccepting(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Accept(context.Background()); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after StopAccepting = %v", err)
	}
	st, err := c.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.Write([]byte("still here"))
	ps, err := in.AcceptStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("still here"))
	_ = ps.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(ps, buf); err != nil {
		t.Fatalf("a connection died with StopAccepting: %v", err)
	}
	// A new connection attempt is not answered any more.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if c2, err := DialQUIC(ctx, l.Addr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: f.creds("forward-1"), ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}}); err == nil {
		_ = c2.Close()
		t.Fatal("a connection was made to a listener that stopped accepting")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

// SetPeers is the revocation path: a removed identity fails at once, a newly
// listed one passes at once, a failed call changes nothing.
func TestQUICSetPeers(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	l := listenQUIC(t, server, []string{id("forward-1")})
	c1, err := dialQUIC(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	_ = acceptQUIC(t, l)

	removed, err := l.SetPeers([]string{id("forward-3")})
	if err != nil || len(removed) != 1 || removed[0] != id("forward-1") {
		t.Fatalf("removed %v, %v", removed, err)
	}
	if l.PeerAllowed(id("forward-1")) || !l.PeerAllowed(id("forward-3")) {
		t.Fatal("PeerAllowed")
	}
	c2, err := dialQUIC(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err == nil {
		expectRefused(t, c2.Conn)
		_ = c2.Close()
	}
	waitFor(t, "the removed identity's refusal", func() bool { return quicFailures(l, ReasonPeerNotAllowed) == 1 })
	c3, err := dialQUIC(t, l, f.creds("forward-3"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c3.Close() }()
	_ = acceptQUIC(t, l)

	if _, err := l.SetPeers(nil); err == nil {
		t.Fatal("SetPeers(nil) succeeded")
	}
	if _, err := l.SetPeers([]string{"forward-4"}); err == nil {
		t.Fatal("SetPeers took a name that is not an identity")
	}
	if !l.PeerAllowed(id("forward-3")) {
		t.Fatal("a failed SetPeers changed the peers")
	}
}

// New handshakes use the reloaded credentials; connections already
// established run on.
func TestQUICReloadCredentials(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	l := listenQUIC(t, server, []string{id("forward-1")})
	client := f.creds("forward-1")
	c1, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	_ = acceptQUIC(t, l)
	serial1 := c1.Peer().Chain[0].SerialNumber

	certPEM, keyPEM, err := f.ca.Issue(relaytest.Cert{Node: "forward-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Reload(certPEM, keyPEM, f.ca.CAPEM()); err != nil {
		t.Fatal(err)
	}
	c2, err := dialQUIC(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	_ = acceptQUIC(t, l)
	if c2.Peer().Chain[0].SerialNumber.Cmp(serial1) == 0 {
		t.Fatal("a new handshake presented the old certificate")
	}
	// the old connection still carries data
	st, err := c1.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}

	// A dropped CA makes the dialler's chain untrusted: refused at the
	// listener, and PeerStillTrusted says so about the old connection.
	other := mustPKI(t, "other")
	oCert, oKey, err := other.Issue(relaytest.Cert{Node: "forward-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Reload(oCert, oKey, other.CAPEM()); err != nil {
		t.Fatal(err)
	}
	inPeer := c2.Peer()
	if server.PeerStillTrusted(inPeer) {
		t.Fatal("the dropped CA's certificate is still trusted")
	}
	// (c2.Peer is the listener's; a connection's own peer on the listener is
	// the dialler's, whose CA the listener just dropped.)
	if _, err := dialQUIC(t, l, client, "forward-2", id("forward-2")); err == nil {
		t.Fatal("the dial succeeded although the listener's CA is not trusted by the dialler")
	}
}

// A restarted listener with the same stateless reset key ends its peers'
// old connections at once, and the peers see why.
func TestQUICStatelessReset(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	var key [32]byte
	copy(key[:], "a stateless reset key of test..")

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := QUICListenerConfig{
		ListenerConfig:    ListenerConfig{Credentials: server, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: []string{id("forward-1")}},
		QUIC:              quicTestConfig(),
		StatelessResetKey: &key,
	}
	l1, err := NewQUICListener(pc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	addr := l1.Addr().String()
	c, err := dialQUIC(t, l1, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = acceptQUIC(t, l1)

	// "Restart": the listener goes away without a word and a new one takes
	// its port with the same key.
	if err := l1.Close(); err != nil {
		t.Fatal(err)
	}
	var l2 *QUICListener
	waitFor(t, "the port to be free", func() bool {
		var err error
		l2, err = ListenQUIC(addr, cfg)
		return err == nil
	})
	defer func() { _ = l2.Close() }()
	// A stateless reset answers a packet the new listener cannot place, and
	// only one longer than the reset itself (quic-go sends none for packets of
	// 42 bytes or less, such as the dialler's keep-alive PINGs, which end in
	// the idle timeout instead). The dialler's own larger packets right after
	// the restart (a path MTU probe, retransmitted data) may fall into the
	// moment before the new listener holds the port, so, like a peer that is
	// in use, the dialler keeps sending application data until it is reset.
	var sendWG sync.WaitGroup
	sendDone := make(chan struct{})
	sendWG.Add(1)
	go func() {
		defer sendWG.Done()
		for {
			select {
			case <-c.Context().Done():
				return
			case <-sendDone:
				return
			case <-time.After(50 * time.Millisecond):
				_ = c.SendDatagram(make([]byte, 100))
			}
		}
	}()
	defer func() { close(sendDone); sendWG.Wait() }()
	select {
	case <-c.Context().Done():
	case <-time.After(testWait):
		t.Fatal("the old connection did not end: no stateless reset")
	}
	var sr *quic.StatelessResetError
	var app *quic.ApplicationError
	cause := context.Cause(c.Context())
	if !errors.As(cause, &sr) && !errors.As(cause, &app) {
		t.Fatalf("the connection ended with %T %v, want a stateless reset or the listener's close", cause, cause)
	}
}

// Everything that changes under a listener changes while connections are
// being made: the race detector's job.
func TestQUICConcurrentUse(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	l := listenQUIC(t, server, []string{id("forward-1"), id("forward-3")})
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			node := []string{"forward-1", "forward-3", "forward-4"}[i]
			creds := f.creds(node)
			for {
				select {
				case <-stop:
					return
				default:
				}
				ctx, cancel := context.WithTimeout(context.Background(), testWait)
				c, err := DialQUIC(ctx, l.Addr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}, QUIC: quicTestConfig()})
				cancel()
				if err == nil {
					_ = c.Close()
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			if c, err := l.Accept(ctx); err == nil {
				_ = c.Close()
			}
			cancel()
		}
	}()
	for i := range 20 {
		certPEM, keyPEM, _ := f.ca.Issue(relaytest.Cert{Node: "forward-2"})
		_ = server.Reload(certPEM, keyPEM, f.ca.CAPEM())
		_, _ = l.SetPeers([]string{id("forward-1"), id("forward-3")}[:1+i%2])
		_ = l.SetSources([]string{"127.0.0.1"})
		_ = l.Stats()
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
}

func TestQUICConstructorsRefuseBadConfigs(t *testing.T) {
	f := newFixture(t)
	creds := f.creds("forward-2")
	ok := ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: []string{id("forward-1")}}
	bad := []struct {
		name string
		mod  func(*ListenerConfig)
	}{
		{"no credentials", func(c *ListenerConfig) { c.Credentials = nil }},
		{"no protocol", func(c *ListenerConfig) { c.Protocol = "" }},
		{"no sources", func(c *ListenerConfig) { c.Sources = nil }},
		{"no peers", func(c *ListenerConfig) { c.Peers = nil }},
		{"a peer that is not an identity", func(c *ListenerConfig) { c.Peers = []string{"forward-1"} }},
		{"negative limits", func(c *ListenerConfig) { c.MaxPending = -1 }},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ok
			tc.mod(&cfg)
			if l, err := ListenQUIC("127.0.0.1:0", QUICListenerConfig{ListenerConfig: cfg}); err == nil {
				_ = l.Close()
				t.Fatal("the listener started")
			}
		})
	}
	if _, err := ListenQUIC("not an address", QUICListenerConfig{ListenerConfig: ok}); err == nil {
		t.Fatal("a bad address was accepted")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	client := f.creds("forward-1")
	dialBad := []struct {
		name string
		cfg  DialConfig
	}{
		{"no credentials", DialConfig{ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}},
		{"no protocol", DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: id("forward-2")}},
		{"an IP as server name", DialConfig{Credentials: client, ServerName: "127.0.0.1", PeerIdentity: id("forward-2"), Protocol: testProto}},
		{"a bad identity", DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: "forward-2", Protocol: testProto}},
	}
	for _, tc := range dialBad {
		t.Run("dial "+tc.name, func(t *testing.T) {
			if c, err := DialQUIC(ctx, "127.0.0.1:9", QUICDialConfig{DialConfig: tc.cfg}); err == nil {
				_ = c.Close()
				t.Fatal("the dial ran")
			}
		})
	}
	// an address that does not resolve, a port that is not a number
	for _, a := range []string{"127.0.0.1", "127.0.0.1:http-nonumber", "no-such-host.invalid:443"} {
		if c, err := DialQUIC(ctx, a, QUICDialConfig{DialConfig: DialConfig{Credentials: client, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto, HandshakeTimeout: time.Second}}); err == nil {
			_ = c.Close()
			t.Fatalf("a dial to %q succeeded", a)
		}
	}
}

// The handshake deadline of a dial: nobody answers, the dial gives up.
func TestQUICDialTimesOut(t *testing.T) {
	f := newFixture(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0") // a socket nothing reads
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	_, err = DialQUIC(ctx, pc.LocalAddr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: f.creds("forward-1"), ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto, HandshakeTimeout: 300 * time.Millisecond}})
	if err == nil || ReasonOf(err) != ReasonTimeout {
		t.Fatalf("err = %v (reason %v), want a timeout", err, ReasonOf(err))
	}
}

// An IPv6 literal works as a dial target (when the host has IPv6 loopback).
func TestQUICOverIPv6(t *testing.T) {
	pc, err := net.ListenPacket("udp", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	_ = pc.Close()
	f := newFixture(t)
	l, err := ListenQUIC("[::1]:0", QUICListenerConfig{
		ListenerConfig: ListenerConfig{Credentials: f.creds("forward-2"), Protocol: testProto, Sources: []string{"::1"}, Peers: []string{id("forward-1")}},
		QUIC:           quicTestConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	c, err := DialQUIC(ctx, l.Addr().String(), QUICDialConfig{DialConfig: DialConfig{Credentials: f.creds("forward-1"), ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}, QUIC: quicTestConfig()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := acceptQUIC(t, l)
	if !in.RemoteAddrPort().Addr().Is6() {
		t.Fatalf("remote %v", in.RemoteAddrPort())
	}
	if udpAddrOf(l).Port == 0 {
		t.Fatal("no port")
	}
}
