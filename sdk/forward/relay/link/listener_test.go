package link

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// A connection from outside ingress_sources is closed before a single
// handshake byte is read: the server never answers, and no signature is made.
func TestSourceAdmission(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")}, func(c *ListenerConfig) { c.Sources = []string{"10.0.0.0/8", "2001:db8::/32"} })
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Send a ClientHello-looking record: an admitted source would get a reply.
	_, _ = conn.Write([]byte{0x16, 0x03, 0x01, 0x00, 0x05, 1, 2, 3, 4, 5})
	_ = conn.SetReadDeadline(time.Now().Add(testWait))
	n, err := conn.Read(make([]byte, 100))
	if n != 0 || err == nil {
		t.Fatalf("read %d bytes, %v: a refused source got an answer", n, err)
	}
	waitFor(t, "the refusal to be counted", func() bool { return l.Stats().Failures[ReasonSourceNotAllowed] == 1 })
	if st := l.Stats(); st.Accepted != 0 || st.Pending != 0 {
		t.Fatalf("stats %+v", st)
	}

	// The next connection is admitted after SetSources, with no new listener.
	if err := l.SetSources([]string{"127.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := accept(t, l)
	defer func() { _ = in.Close() }()
	roundTrip(t, c, in)
	if err := l.SetSources(nil); err == nil {
		t.Fatal("SetSources(nil) succeeded: a listener open to nobody is a configuration error")
	}
	if err := l.SetSources([]string{"not an address"}); err == nil {
		t.Fatal("a bad source was accepted")
	}
}

func TestHandshakeDeadline(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")}, func(c *ListenerConfig) { c.HandshakeTimeout = 100 * time.Millisecond })
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Silence: the listener closes the connection when the deadline passes.
	_ = conn.SetReadDeadline(time.Now().Add(testWait))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF && !isReset(err) {
		t.Fatalf("read = %v, want the listener to close", err)
	}
	waitFor(t, "the timeout to be counted", func() bool { return l.Stats().Failures[ReasonTimeout] == 1 })
	waitFor(t, "the slot to be free", func() bool { return l.Stats().Pending == 0 })
}

func isReset(err error) bool {
	var oe *net.OpError
	return err != nil && (errorsAs(err, &oe))
}

func TestPendingHandshakeLimit(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")}, func(c *ListenerConfig) {
		c.MaxPending = 2
		c.HandshakeTimeout = 300 * time.Millisecond
	})
	var held []net.Conn
	for range 2 {
		c, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
		defer func() { _ = c.Close() }()
	}
	waitFor(t, "two handshakes pending", func() bool { return l.Stats().Pending == 2 })
	extra, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extra.Close() }()
	// Over the limit: closed at once, long before the deadline of the others.
	_ = extra.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := extra.Read(make([]byte, 1)); err == nil || isTimeoutErr(err) {
		t.Fatalf("read = %v, want the listener to close the extra connection at once", err)
	}
	waitFor(t, "the refusal", func() bool { return l.Stats().Failures[ReasonHandshakeLimit] == 1 })
	// When the silent ones time out, real nodes get in again.
	waitFor(t, "slots free", func() bool { return l.Stats().Pending == 0 })
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = accept(t, l).Close()
	_ = held
}

func isTimeoutErr(err error) bool {
	var ne net.Error
	return errorsAs(err, &ne) && ne.Timeout()
}

func TestSetPeersIsTheRevocationPath(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1"), id("forward-3")})
	c1, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	in1 := accept(t, l)
	defer func() { _ = in1.Close() }()
	if !l.PeerAllowed(id("forward-1")) || !l.PeerAllowed(id("forward-3")) || l.PeerAllowed(id("forward-4")) {
		t.Fatal("PeerAllowed")
	}

	removed, err := l.SetPeers([]string{id("forward-3"), id("forward-4")})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != id("forward-1") {
		t.Fatalf("removed = %v", removed)
	}
	if l.PeerAllowed(id("forward-1")) || !l.PeerAllowed(id("forward-4")) {
		t.Fatal("PeerAllowed after SetPeers")
	}
	// The removed node's connection is the carrier layer's to close (it has
	// Peer().Identity and the removed list); a new handshake from it fails.
	if in1.Peer().Identity != removed[0] {
		t.Fatal("the established connection does not name the removed identity")
	}
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err == nil {
		_ = c.SetReadDeadline(time.Now().Add(testWait))
		if _, rerr := c.Read(make([]byte, 1)); rerr == nil {
			t.Fatal("a removed peer completed a handshake")
		}
		_ = c.Close()
	}
	waitFor(t, "peer_not_allowed", func() bool { return l.Stats().Failures[ReasonPeerNotAllowed] == 1 })
	// A newly listed peer is admitted at once.
	c4, err := dial(t, l, f.creds("forward-4"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c4.Close() }()
	in4 := accept(t, l)
	defer func() { _ = in4.Close() }()
	roundTrip(t, c4, in4)

	for _, bad := range [][]string{nil, {"spiffe://anixops/test/agent/forward-0"}, {"forward-1"}} {
		if _, err := l.SetPeers(bad); err == nil {
			t.Fatalf("SetPeers(%v) succeeded", bad)
		}
	}
	if !l.PeerAllowed(id("forward-4")) {
		t.Fatal("a failed SetPeers changed the peers")
	}
}

func TestListenerCloseAbandonsEverything(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")})
	silent, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	waitFor(t, "handshake pending", func() bool { return l.Stats().Pending == 1 })
	// A finished handshake nobody accepts is abandoned too.
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	acceptErr := make(chan error, 1)
	go func() {
		<-time.After(10 * time.Millisecond)
		_ = l.Close()
	}()
	go func() {
		// take the finished one first, then block
		in, err := l.Accept()
		if err == nil {
			_ = in.Close()
		}
		_, err = l.Accept()
		acceptErr <- err
	}()
	select {
	case err := <-acceptErr:
		if err != net.ErrClosed {
			t.Fatalf("Accept after Close = %v", err)
		}
	case <-time.After(testWait):
		t.Fatal("Accept did not return after Close")
	}
	_ = silent.SetReadDeadline(time.Now().Add(testWait))
	if _, err := silent.Read(make([]byte, 1)); err == nil {
		t.Fatal("the pending handshake's connection stayed open")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	// the listening socket itself is closed (not merely refusing: a port
	// number can be reused by another process the moment it is free)
	if _, err := l.inner.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("the listening socket is still open: %v", err)
	}
}

func TestPlainLink(t *testing.T) {
	if _, err := ListenPlain("127.0.0.1:0", PlainListenerConfig{Sources: []string{"127.0.0.1"}}); err == nil {
		t.Fatal("a plain listener without TrustedLink")
	}
	if _, err := ListenPlain("127.0.0.1:0", PlainListenerConfig{TrustedLink: true}); err == nil {
		t.Fatal("a plain listener without sources: they are its only authentication")
	}
	if _, err := DialPlain(t.Context(), "127.0.0.1:1", PlainDialConfig{}); err == nil {
		t.Fatal("a plain dial without TrustedLink")
	}
	l, err := ListenPlain("127.0.0.1:0", PlainListenerConfig{TrustedLink: true, Sources: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if l.Encrypted() {
		t.Fatal("a plain listener is not encrypted")
	}
	if _, err := l.SetPeers([]string{id("forward-1")}); err == nil {
		t.Fatal("SetPeers on a plain listener")
	}
	c, err := DialPlain(t.Context(), l.Addr().String(), PlainDialConfig{TrustedLink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := accept(t, l)
	defer func() { _ = in.Close() }()
	if c.Encrypted() || in.Encrypted() || in.Peer().Identity != "" || len(in.Peer().Chain) != 0 {
		t.Fatalf("a plain link has no identity: %+v", in.Peer())
	}
	if _, ok := c.Conn.(*tls.Conn); ok {
		t.Fatal("a plain link runs TLS")
	}
	roundTrip(t, c, in)

	// Admission is its only check.
	if err := l.SetSources([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	raw, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	_ = raw.SetReadDeadline(time.Now().Add(testWait))
	if _, err := raw.Read(make([]byte, 1)); err == nil {
		t.Fatal("a refused source stayed connected")
	}
	waitFor(t, "refusal counted", func() bool { return l.Stats().Failures[ReasonSourceNotAllowed] == 1 })
}

func TestConstructorsRejectBadConfig(t *testing.T) {
	f := newFixture(t)
	creds := f.creds("forward-2")
	good := ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: []string{id("forward-1")}}
	for name, mod := range map[string]func(*ListenerConfig){
		"no credentials": func(c *ListenerConfig) { c.Credentials = nil },
		"no protocol":    func(c *ListenerConfig) { c.Protocol = "" },
		"no sources":     func(c *ListenerConfig) { c.Sources = nil },
		"no peers":       func(c *ListenerConfig) { c.Peers = nil },
		"bad peer":       func(c *ListenerConfig) { c.Peers = []string{"spiffe://anixops/test/kernel"} },
		"negative limit": func(c *ListenerConfig) { c.MaxPending = -1 },
	} {
		cfg := good
		mod(&cfg)
		if l, err := Listen("127.0.0.1:0", cfg); err == nil {
			_ = l.Close()
			t.Errorf("%s: accepted", name)
		}
	}
	good2 := DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto}
	for name, mod := range map[string]func(*DialConfig){
		"no credentials":   func(c *DialConfig) { c.Credentials = nil },
		"no protocol":      func(c *DialConfig) { c.Protocol = "" },
		"no server name":   func(c *DialConfig) { c.ServerName = "" },
		"an IP as a name":  func(c *DialConfig) { c.ServerName = "127.0.0.1" },
		"bad server name":  func(c *DialConfig) { c.ServerName = "forward 2" },
		"no peer identity": func(c *DialConfig) { c.PeerIdentity = "" },
		"bad identity":     func(c *DialConfig) { c.PeerIdentity = "spiffe://anixops/test/agent/forward-01" },
	} {
		cfg := good2
		mod(&cfg)
		if _, err := DialTLS(t.Context(), "127.0.0.1:1", cfg); err == nil {
			t.Errorf("dial %s: accepted", name)
		}
	}
	// a dial to a closed port is a failure with a reason, not a panic
	if _, err := DialTLS(t.Context(), "127.0.0.1:1", good2); err == nil {
		t.Error("dial to a closed port succeeded")
	} else if ReasonOf(err) == ReasonSourceNotAllowed {
		t.Error(err)
	}
}

func TestConcurrentUse(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1"), id("forward-3")})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // drain
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, c); _ = c.Close() }()
		}
	}()
	for i := range 4 {
		node := []string{"forward-1", "forward-3"}[i%2]
		creds := f.creds(node)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if c, err := dial(t, l, creds, "forward-2", id("forward-2")); err == nil {
					_, _ = c.Write([]byte("x"))
					_ = c.Close()
				}
			}
		}()
	}
	for i := range 20 {
		certPEM, keyPEM, err := f.ca.Issue(certFor("forward-2"))
		if err != nil {
			t.Fatal(err)
		}
		if err := l.creds.Reload(certPEM, keyPEM, f.ca.CAPEM()); err != nil {
			t.Fatal(err)
		}
		_, _ = l.SetPeers([]string{id("forward-1"), id([]string{"forward-3", "forward-4"}[i%2])})
		_ = l.SetSources([]string{"127.0.0.1", "10.0.0.0/8"})
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	_ = l.Close()
	wg.Wait()
}
