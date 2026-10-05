package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

const (
	testProto = "anixops/0"
	testWait  = 5 * time.Second
)

func id(node string) string { return relaytest.Identity("test", node) }

type fixture struct {
	t  testing.TB
	ca *relaytest.PKI
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	ca, err := relaytest.New("test")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, ca: ca}
}

func mustPKI(t testing.TB, name string) *relaytest.PKI {
	t.Helper()
	p, err := relaytest.New(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// creds returns valid link credentials for node, issued by the fixture's CA
// with a trust bundle of that CA.
func (f *fixture) creds(node string) *Credentials {
	f.t.Helper()
	return credsFrom(f.t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: node})
}

func credsFrom(t testing.TB, ca *relaytest.PKI, bundle []byte, c relaytest.Cert) *Credentials {
	t.Helper()
	certPEM, keyPEM, err := ca.Issue(c)
	if err != nil {
		t.Fatal(err)
	}
	cr, err := NewCredentials(certPEM, keyPEM, bundle)
	if err != nil {
		t.Fatal(err)
	}
	return cr
}

// unchecked builds credentials without the shape checks NewCredentials makes,
// so a test can present the certificate a faulty or hostile node would.
func unchecked(t testing.TB, ca *relaytest.PKI, bundle []byte, c relaytest.Cert) *Credentials {
	t.Helper()
	certPEM, keyPEM, err := ca.Issue(c)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
		t.Fatal(err)
	}
	roots, err := parseBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	cr := &Credentials{watchers: map[int]func(){}}
	cr.state.Store(&credState{cert: cert, roots: roots, generation: 1, notAfter: cert.Leaf.NotAfter})
	return cr
}

// listen starts an encrypted listener on loopback admitting 127.0.0.1.
func listen(t testing.TB, creds *Credentials, peers []string, mod ...func(*ListenerConfig)) *Listener {
	t.Helper()
	cfg := ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: peers}
	for _, m := range mod {
		m(&cfg)
	}
	l, err := Listen("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func dial(t testing.TB, l *Listener, creds *Credentials, name, peer string) (*Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	return DialTLS(ctx, l.Addr().String(), DialConfig{Credentials: creds, ServerName: name, PeerIdentity: peer, Protocol: testProto, HandshakeTimeout: testWait})
}

// accept returns the next connection the listener hands out, or fails.
func accept(t testing.TB, l *Listener) *Conn {
	t.Helper()
	type res struct {
		c   *Conn
		err error
	}
	ch := make(chan res, 1)
	go func() { c, err := l.Accept(); ch <- res{c, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.c
	case <-time.After(testWait):
		t.Fatal("Accept timed out")
		return nil
	}
}

// expectNoAccept fails if the listener hands out a connection within d.
func expectNoAccept(t testing.TB, l *Listener, d time.Duration) {
	t.Helper()
	got := make(chan *Conn, 1)
	go func() {
		if c, err := l.Accept(); err == nil {
			got <- c
		}
	}()
	select {
	case c := <-got:
		_ = c.Close()
		t.Fatal("Accept returned a connection that should have been refused")
	case <-time.After(d):
	}
}

func waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// echoOnce checks that bytes flow both ways over a pair of connections.
func roundTrip(t testing.TB, a, b net.Conn) {
	t.Helper()
	_ = a.SetDeadline(time.Now().Add(testWait))
	_ = b.SetDeadline(time.Now().Add(testWait))
	msg := []byte("ping over the link")
	go func() { _, _ = a.Write(msg) }()
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(b, buf); err != nil || string(buf) != string(msg) {
		t.Fatalf("a to b: %q %v", buf, err)
	}
	go func() { _, _ = b.Write(msg) }()
	if _, err := io.ReadFull(a, buf); err != nil || string(buf) != string(msg) {
		t.Fatalf("b to a: %q %v", buf, err)
	}
}

func errorsAs[T any](err error, target *T) bool { return errors.As(err, target) }

func certFor(node string) relaytest.Cert { return relaytest.Cert{Node: node} }

func tcpAddrOf(a netip.AddrPort) net.Addr {
	return &net.TCPAddr{IP: a.Addr().AsSlice(), Port: int(a.Port())}
}
