package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

func TestEncryptedLink(t *testing.T) {
	f := newFixture(t)
	server, client := f.creds("forward-2"), f.creds("forward-1")
	l := listen(t, server, []string{id("forward-1")})
	if !l.Encrypted() || l.String() == "" {
		t.Fatal("listener kind")
	}

	c, err := dial(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	in := accept(t, l)
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
	cs := c.Conn.(*tls.Conn).ConnectionState()
	if cs.Version != tls.VersionTLS13 || cs.NegotiatedProtocol != testProto || cs.DidResume {
		t.Fatalf("session %+v", cs)
	}
	roundTrip(t, c, in)
	// The counters settle just after Accept returns.
	waitFor(t, "the counters to settle", func() bool {
		st := l.Stats()
		return st.Accepted == 1 && st.Pending == 0
	})
	if client.Identity() != id("forward-1") || client.Node() != "forward-1" || client.Generation() != 1 || client.NotAfter().Before(time.Now()) {
		t.Fatalf("credentials: %s %s %d %v", client.Identity(), client.Node(), client.Generation(), client.NotAfter())
	}
}

// The dialler accepts only the node the state names: the same CA's other
// nodes, wrong names, extra names and foreign CAs all fail, each for its own
// reason.
func TestDiallerPinsTheListener(t *testing.T) {
	f := newFixture(t)
	client := f.creds("forward-1")
	for _, tc := range diallerPinCases(t, f) {
		t.Run(tc.name, func(t *testing.T) {
			l := listen(t, tc.server, []string{id("forward-1")})
			c, err := dial(t, l, client, "forward-2", id("forward-2"))
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

// diallerCase is a listener's credentials and the reason a dialler pinning
// forward-2 refuses them for.
type diallerCase struct {
	name   string
	server *Credentials
	want   Reason
}

// diallerPinCases are the listeners a dialler of forward-2 must refuse, one
// rule broken each. Both link types run them.
func diallerPinCases(t *testing.T, f *fixture) []diallerCase {
	foreign := mustPKI(t, "foreign")
	now := time.Now()
	return []diallerCase{
		{"another node of the same CA", f.creds("forward-3"), ReasonIdentityMismatch},
		{"right identity, other DNS name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", DNSNames: []string{"forward-9"}}), ReasonIdentityMismatch},
		{"right identity and name plus another name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", DNSNames: []string{"forward-2", "forward-9"}}), ReasonIdentityMismatch},
		{"wildcard name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", DNSNames: []string{"*.example"}}), ReasonIdentityMismatch},
		{"no URI name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", URIs: []string{}}), ReasonIdentityMismatch},
		{"two URI names", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", URIs: []string{id("forward-2"), id("forward-3")}}), ReasonIdentityMismatch},
		{"an extra IP name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}), ReasonIdentityMismatch},
		{"another cluster's identity", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", URIs: []string{relaytest.Identity("other", "forward-2")}}), ReasonIdentityMismatch},
		{"another trust domain: the CA's name constraint refuses it", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", URIs: []string{"spiffe://example.org/test/agent/forward-2"}}), ReasonCertificate},
		{"a CA the dialler does not trust", unchecked(t, foreign, foreign.CAPEM(), relaytest.Cert{Node: "forward-2"}), ReasonUnknownCA},
		{"an expired certificate", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-time.Hour)}), ReasonCertificate},
		{"not yet valid", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", NotBefore: now.Add(time.Hour), NotAfter: now.Add(2 * time.Hour)}), ReasonCertificate},
		{"clientAuth only", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}), ReasonCertificate},
		{"no key usage stated", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-2", NoExtKeyUsage: true}), ReasonCertificate},
	}
}

// The listener admits only the nodes the state lists, however genuine their
// certificates.
func TestListenerPinsTheDiallers(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	for _, tc := range listenerPinCases(t, f) {
		t.Run(tc.name, func(t *testing.T) {
			l := listen(t, server, []string{id("forward-1")})
			c, err := dial(t, l, tc.client, "forward-2", id("forward-2"))
			if err == nil {
				// TLS 1.3: the client finishes first, so the refusal shows
				// on its first read.
				_ = c.SetReadDeadline(time.Now().Add(testWait))
				if _, rerr := c.Read(make([]byte, 1)); rerr == nil {
					t.Fatal("read succeeded on a refused connection")
				}
				_ = c.Close()
			}
			waitFor(t, "the refusal to be counted", func() bool { return l.Stats().Failures[tc.want] == 1 })
			if st := l.Stats(); st.Accepted != 0 {
				t.Fatalf("stats %+v", st)
			}
			expectNoAccept(t, l, 50*time.Millisecond)
		})
	}
}

// listenerCase is a dialler's credentials and the reason a listener of
// forward-2 that lists only forward-1 refuses them for.
type listenerCase struct {
	name   string
	client *Credentials
	want   Reason
}

// listenerPinCases are the diallers a listener of forward-2 listing only
// forward-1 must refuse, one rule broken each. Both link types run them.
func listenerPinCases(t *testing.T, f *fixture) []listenerCase {
	foreign := mustPKI(t, "foreign")
	now := time.Now()
	return []listenerCase{
		{"a genuine node that is not listed", f.creds("forward-3"), ReasonPeerNotAllowed},
		{"another trust domain: the CA's name constraint refuses it", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", URIs: []string{"spiffe://example.org/test/agent/forward-1"}}), ReasonCertificate},
		{"a CA the listener does not trust", unchecked(t, foreign, relaytest.Bundle(foreign, f.ca), relaytest.Cert{Node: "forward-1"}), ReasonUnknownCA},
		{"an expired certificate", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-time.Hour)}), ReasonCertificate},
		{"serverAuth only", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}), ReasonCertificate},
		{"no key usage stated", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", NoExtKeyUsage: true}), ReasonCertificate},
		{"two URI names", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", URIs: []string{id("forward-1"), id("forward-3")}}), ReasonCertificate},
		{"an extra email name", unchecked(t, f.ca, f.ca.CAPEM(), relaytest.Cert{Node: "forward-1", EmailAddresses: []string{"a@example.com"}}), ReasonCertificate},
	}
}

func TestClientWithoutACertificateIsRefused(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")})
	roots := x509.NewCertPool()
	roots.AddCert(mustParse(t, f.ca.CAPEM()))
	conn, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{RootCAs: roots, ServerName: "forward-2", NextProtos: []string{testProto}, MinVersion: tls.VersionTLS13})
	if err == nil {
		_ = conn.SetReadDeadline(time.Now().Add(testWait))
		_, err = conn.Read(make([]byte, 1))
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("a client without a certificate got through")
	}
	waitFor(t, "the refusal", func() bool { return l.Stats().Failures[ReasonCertificate] == 1 })
}

// TLS 1.3 and the ALPN protocol are required of whoever connects, and a
// session is never resumed.
func TestSessionRequirements(t *testing.T) {
	f := newFixture(t)
	server := f.creds("forward-2")
	clientCreds := f.creds("forward-1")
	roots := x509.NewCertPool()
	roots.AddCert(mustParse(t, f.ca.CAPEM()))
	cert := clientCreds.state.Load().cert
	base := func() *tls.Config {
		return &tls.Config{RootCAs: roots, ServerName: "forward-2", Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, NextProtos: []string{testProto}}
	}
	tests := []struct {
		name string
		edit func(c *tls.Config)
		want Reason
	}{
		{"TLS 1.2 only", func(c *tls.Config) { c.MinVersion, c.MaxVersion = tls.VersionTLS12, tls.VersionTLS12 }, ReasonProtocol},
		{"no ALPN", func(c *tls.Config) { c.NextProtos = nil }, ReasonProtocol},
		{"another ALPN protocol", func(c *tls.Config) { c.NextProtos = []string{"h2"} }, ReasonProtocol},
		{"the production ALPN version", func(c *tls.Config) { c.NextProtos = []string{"anixops/1"} }, ReasonProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := listen(t, server, []string{id("forward-1")})
			cfg := base()
			tc.edit(cfg)
			conn, err := tls.Dial("tcp", l.Addr().String(), cfg)
			if err == nil {
				_ = conn.SetReadDeadline(time.Now().Add(testWait))
				_, err = conn.Read(make([]byte, 1))
				_ = conn.Close()
			}
			if err == nil {
				t.Fatal("the connection was accepted")
			}
			waitFor(t, "the refusal", func() bool { return l.Stats().Failures[tc.want] == 1 })
		})
	}

	t.Run("a TLS 1.2 listener is refused by the dialler", func(t *testing.T) {
		raw, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = raw.Close() }()
		srvCert := server.state.Load().cert
		go func() {
			for {
				c, err := raw.Accept()
				if err != nil {
					return
				}
				go func() {
					defer func() { _ = c.Close() }()
					tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{srvCert}, MaxVersion: tls.VersionTLS12, NextProtos: []string{testProto}})
					_ = tc.Handshake()
				}()
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), testWait)
		defer cancel()
		_, err = DialTLS(ctx, raw.Addr().String(), DialConfig{Credentials: clientCreds, ServerName: "forward-2", PeerIdentity: id("forward-2"), Protocol: testProto})
		if err == nil || ReasonOf(err) != ReasonProtocol {
			t.Fatalf("err = %v (reason %v), want a protocol refusal", err, ReasonOf(err))
		}
	})

	t.Run("no resumption", func(t *testing.T) {
		l := listen(t, server, []string{id("forward-1")})
		cfg := base()
		cfg.ClientSessionCache = tls.NewLRUClientSessionCache(8)
		for i := range 3 {
			conn, err := tls.Dial("tcp", l.Addr().String(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			in := accept(t, l)
			// let any NewSessionTicket the server might send arrive
			_ = in.SetWriteDeadline(time.Now().Add(testWait))
			_, _ = in.Write([]byte("x"))
			buf := make([]byte, 1)
			_ = conn.SetReadDeadline(time.Now().Add(testWait))
			if _, err := conn.Read(buf); err != nil {
				t.Fatal(err)
			}
			if conn.ConnectionState().DidResume {
				t.Fatalf("handshake %d resumed a session", i)
			}
			_ = conn.Close()
			_ = in.Close()
		}
	})
}

func mustParse(t testing.TB, caPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(caPEM)
	if block == nil {
		t.Fatal("no PEM block")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNetConnIsTheTCPConnectionBelowTLS(t *testing.T) {
	f := newFixture(t)
	l := listen(t, f.creds("forward-2"), []string{id("forward-1")})
	c, err := dial(t, l, f.creds("forward-1"), "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, ok := c.NetConn().(*net.TCPConn); !ok {
		t.Fatalf("NetConn = %T, want the TCP connection", c.NetConn())
	}
	plain, err := ListenPlain("127.0.0.1:0", PlainListenerConfig{TrustedLink: true, Sources: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = plain.Close() }()
	pc, err := DialPlain(t.Context(), plain.Addr().String(), PlainDialConfig{TrustedLink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	if _, ok := pc.NetConn().(*net.TCPConn); !ok {
		t.Fatalf("plain NetConn = %T", pc.NetConn())
	}
}
