package link

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

// Fuzz targets: `go test` runs their seed corpus; run one longer from the sdk
// module with
//
//	go test -run '^$' -fuzz '^FuzzVerifyConnection$' -fuzztime 60s ./forward/relay/link

// The identity grammar is the Agent's: whatever agentcontrol accepts as an
// agent identity, and nothing else, is accepted here (it is what the planner
// writes into peer_identity and ingress_peers).
func FuzzParseIdentity(f *testing.F) {
	for _, s := range []string{
		"spiffe://anixops/test/agent/forward-1", "spiffe://anixops/test/agent/proxy-4294967295",
		"spiffe://anixops/test/kernel", "spiffe://anixops/test/agent/forward-01", "", "spiffe://anixops/test/agent/forward%2d1",
		"spiffe://anixops/test/agent/forward-1?x", "spiffe://anixops:1/test/agent/forward-1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := ParseIdentity(s)
		_, ref := agentcontrol.ParseAgentIdentity(s)
		if (err == nil) != (ref == nil) {
			t.Fatalf("%q: link err=%v, agentcontrol err=%v", s, err, ref)
		}
		if err == nil && got != s {
			t.Fatalf("%q changed to %q", s, got)
		}
	})
}

func FuzzParseSources(f *testing.F) {
	for _, s := range []string{"192.0.2.1", "198.51.100.0/24", "2001:db8::/32", "::ffff:1.2.3.4", "fe80::1%eth0", "1.2.3.4/33", "", "::ffff:0.0.0.0/0"} {
		f.Add(s, "10.0.0.1")
	}
	f.Fuzz(func(t *testing.T, src, probe string) {
		s, err := ParseSources([]string{src})
		if err != nil {
			return
		}
		// Every prefix kept contains its own address and is masked; an
		// admitted address is inside one of them.
		for _, p := range s.Prefixes() {
			if p != p.Masked() || !p.Addr().IsValid() || p.Addr().Zone() != "" || p.Addr().Is4In6() {
				t.Fatalf("malformed prefix %v from %q", p, src)
			}
			if !s.Contains(p.Addr()) {
				t.Fatalf("%v does not contain its own address", p)
			}
		}
		if a, err := netip.ParseAddr(probe); err == nil && s.Contains(a) {
			ok := false
			for _, p := range s.Prefixes() {
				ok = ok || p.Contains(a.Unmap().WithZone(""))
			}
			if !ok {
				t.Fatalf("%v admitted by %q but by none of its prefixes", a, src)
			}
		}
	})
}

// FuzzVerifyConnection feeds the identity checks synthetic certificates and
// asserts the security property: a connection is accepted only when the peer
// carries exactly the expected names and the right key usage.
func FuzzVerifyConnection(f *testing.F) {
	f.Add("spiffe://anixops/test/agent/forward-1", "", "forward-1", uint8(3), false, false)
	f.Add("spiffe://anixops/test/agent/forward-1", "spiffe://anixops/test/agent/forward-2", "forward-1", uint8(3), false, false)
	f.Add("spiffe://anixops/test/agent/forward-2", "", "forward-2", uint8(1), true, false)
	f.Add("spiffe://anixops/test/agent/forward-1", "", "*", uint8(0), false, true)
	f.Fuzz(func(t *testing.T, uri1, uri2, dns string, eku uint8, ip, email bool) {
		cert := &x509.Certificate{}
		for _, raw := range []string{uri1, uri2} {
			if raw == "" {
				continue
			}
			u, err := url.Parse(raw)
			if err != nil {
				return
			}
			cert.URIs = append(cert.URIs, u)
		}
		if dns != "" {
			cert.DNSNames = []string{dns}
		}
		if eku&1 != 0 {
			cert.ExtKeyUsage = append(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		}
		if eku&2 != 0 {
			cert.ExtKeyUsage = append(cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
		}
		if ip {
			cert.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
		}
		if email {
			cert.EmailAddresses = []string{"a@example.com"}
		}
		cs := tls.ConnectionState{Version: tls.VersionTLS13, NegotiatedProtocol: testProto, PeerCertificates: []*x509.Certificate{cert}}
		peers, err := newPeerSet([]string{id("forward-1"), id("forward-3")})
		if err != nil {
			t.Fatal(err)
		}
		oneIdentity := len(cert.URIs) == 1 && !ip && !email

		if err := verifyClientConnection(cs, peers, testProto); err == nil {
			if !oneIdentity || !peers.has(cert.URIs[0].String()) || !slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
				t.Fatalf("client accepted: %+v", cert)
			}
		}
		if err := verifyServerConnection(cs, "forward-1", id("forward-1"), testProto); err == nil {
			if !oneIdentity || cert.URIs[0].String() != id("forward-1") || !slices.Equal(cert.DNSNames, []string{"forward-1"}) ||
				!slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
				t.Fatalf("server accepted: %+v", cert)
			}
		}
		// the session checks hold whatever the certificate says
		for _, bad := range []tls.ConnectionState{
			{Version: tls.VersionTLS12, NegotiatedProtocol: testProto, PeerCertificates: cs.PeerCertificates},
			{Version: tls.VersionTLS13, NegotiatedProtocol: "", PeerCertificates: cs.PeerCertificates},
			{Version: tls.VersionTLS13, NegotiatedProtocol: testProto, DidResume: true, PeerCertificates: cs.PeerCertificates},
			{Version: tls.VersionTLS13, NegotiatedProtocol: testProto},
		} {
			if verifyClientConnection(bad, peers, testProto) == nil || verifyServerConnection(bad, "forward-1", id("forward-1"), testProto) == nil {
				t.Fatalf("a bad session was accepted: %+v", bad)
			}
		}
	})
}

// clientHello returns the first flight of a real TLS 1.3 client, a seed that
// gets the fuzzer deep into crypto/tls's parser.
func clientHello(t testing.TB) []byte {
	t.Helper()
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	go func() {
		_ = tls.Client(c1, &tls.Config{ServerName: "forward-2", NextProtos: []string{testProto}, InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}).Handshake() // #nosec G402 -- only the first flight is captured
	}()
	buf := make([]byte, 4096)
	_ = c2.SetReadDeadline(time.Now().Add(testWait))
	n, err := c2.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

// FuzzHandshake feeds arbitrary bytes to a listener's handshake: it must
// never succeed, never hang beyond its deadline and never panic.
func FuzzHandshake(f *testing.F) {
	hello := clientHello(f)
	f.Add(hello)
	f.Add(hello[:len(hello)/2])
	f.Add([]byte{})
	f.Add([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	f.Add(append(bytes.Clone(hello), hello...))
	mutated := bytes.Clone(hello)
	mutated[len(mutated)/2] ^= 0xff
	f.Add(mutated)

	ca, err := relaytest.New("fuzz")
	if err != nil {
		f.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue(relaytest.Cert{Node: "forward-2"})
	if err != nil {
		f.Fatal(err)
	}
	creds, err := NewCredentials(certPEM, keyPEM, ca.CAPEM())
	if err != nil {
		f.Fatal(err)
	}
	l, err := newListener(ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: []string{id("forward-1")}, HandshakeTimeout: 200 * time.Millisecond})
	if err != nil {
		f.Fatal(err)
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	f.Cleanup(l.cancel)

	f.Fuzz(func(t *testing.T, data []byte) {
		server, client := net.Pipe()
		defer func() { _ = server.Close() }()
		go func() {
			_ = client.SetWriteDeadline(time.Now().Add(testWait))
			_, _ = client.Write(data)
			// keep the connection open: the deadline must end the handshake
			buf := make([]byte, 4096)
			for {
				if _, err := client.Read(buf); err != nil {
					return
				}
			}
		}()
		defer func() { _ = client.Close() }()
		start := time.Now()
		c, err := l.handshakeConn(server)
		if err == nil {
			_ = c.Close()
			t.Fatal("a handshake on fuzzed bytes succeeded")
		}
		if time.Since(start) > testWait {
			t.Fatal("the handshake outlived its deadline")
		}
	})
}

func FuzzCredentialsPEM(f *testing.F) {
	ca, err := relaytest.New("fuzz")
	if err != nil {
		f.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue(relaytest.Cert{Node: "forward-2"})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(certPEM, keyPEM, ca.CAPEM())
	f.Add([]byte{}, []byte{}, []byte{})
	f.Add(certPEM, certPEM, ca.CAPEM())
	f.Add(certPEM, keyPEM, append(ca.CAPEM(), []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n")...))
	f.Fuzz(func(t *testing.T, c, k, b []byte) {
		creds, err := NewCredentials(c, k, b)
		if err != nil {
			return
		}
		// whatever loads is a link certificate: one identity, its node name
		if _, perr := ParseIdentity(creds.Identity()); perr != nil || creds.Node() == "" {
			t.Fatalf("loaded credentials with identity %q node %q", creds.Identity(), creds.Node())
		}
	})
}
