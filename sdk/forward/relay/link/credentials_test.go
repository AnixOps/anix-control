package link

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

func TestCredentialsValidation(t *testing.T) {
	ca := mustPKI(t, "a")
	other := mustPKI(t, "b")
	now := time.Now()
	issue := func(p *relaytest.PKI, c relaytest.Cert) (certPEM, keyPEM []byte) {
		t.Helper()
		certPEM, keyPEM, err := p.Issue(c)
		if err != nil {
			t.Fatal(err)
		}
		return certPEM, keyPEM
	}
	goodCert, goodKey := issue(ca, relaytest.Cert{Node: "forward-7"})
	_, otherKey := issue(ca, relaytest.Cert{Node: "forward-7"})
	nonCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: mustParse(t, goodCert).Raw})

	tests := []struct {
		name              string
		cert, key, bundle []byte
		wantErrContains   string
	}{
		{"valid", goodCert, goodKey, ca.CAPEM(), ""},
		{"valid with a bundle of next and retired CAs", goodCert, goodKey, append(ca.CAPEM(), other.CAPEM()...), ""},
		{"key of another certificate", goodCert, otherKey, ca.CAPEM(), "private key does not match"},
		{"garbage certificate", []byte("nope"), goodKey, ca.CAPEM(), "link certificate"},
		{"no bundle", goodCert, goodKey, nil, "no PEM certificate"},
		{"bundle of garbage", goodCert, goodKey, []byte("nope"), "no PEM certificate"},
		{"bundle with a key in it", goodCert, goodKey, append(ca.CAPEM(), goodKey...), "want certificates only"},
		{"bundle with a leaf in it", goodCert, goodKey, append(ca.CAPEM(), nonCA...), "not a CA"},
		{"bundle with trailing data", goodCert, goodKey, append(ca.CAPEM(), []byte("junk")...), "after its last certificate"},
		{"certificate from a CA outside the bundle", goodCert, goodKey, other.CAPEM(), "does not verify"},
	}
	cases := map[string]relaytest.Cert{
		"DNS name is not the identity's node": {Node: "forward-7", DNSNames: []string{"forward-8"}},
		"two DNS names":                       {Node: "forward-7", DNSNames: []string{"forward-7", "forward-8"}},
		"no URI":                              {Node: "forward-7", URIs: []string{}},
		"two URIs":                            {Node: "forward-7", URIs: []string{id("forward-7"), id("forward-8")}},
		"not an agent identity":               {Node: "forward-7", URIs: []string{"spiffe://anixops/test/kernel"}},
		"another trust domain":                {Node: "forward-7", URIs: []string{"spiffe://example.org/test/agent/forward-7"}},
		"missing clientAuth":                  {Node: "forward-7", ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		"missing serverAuth":                  {Node: "forward-7", ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}},
		"no key usage at all":                 {Node: "forward-7", NoExtKeyUsage: true},
		"expired":                             {Node: "forward-7", NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-time.Hour)},
		"not yet valid":                       {Node: "forward-7", NotBefore: now.Add(time.Hour), NotAfter: now.Add(2 * time.Hour)},
	}
	for name, c := range cases {
		certPEM, keyPEM := issue(ca, c)
		want := "node's certificate"
		if strings.Contains(name, "expired") || strings.Contains(name, "yet valid") || strings.Contains(name, "key usage") && !strings.Contains(name, "no key") {
			want = "does not verify"
		}
		if strings.HasPrefix(name, "missing") || strings.HasPrefix(name, "no key usage") {
			want = "serverAuth and clientAuth"
		}
		tests = append(tests, struct {
			name              string
			cert, key, bundle []byte
			wantErrContains   string
		}{name, certPEM, keyPEM, ca.CAPEM(), want})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewCredentials(tc.cert, tc.key, tc.bundle)
			if tc.wantErrContains == "" {
				if err != nil {
					t.Fatal(err)
				}
				if c.Identity() != id("forward-7") || c.Node() != "forward-7" {
					t.Fatalf("%s %s", c.Identity(), c.Node())
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted; want an error containing %q", tc.wantErrContains)
			}
			if !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErrContains)
			}
		})
	}
}

func TestCredentialsFromFilesAndReload(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	write := func(certPEM, keyPEM, caPEM []byte) (certFile, keyFile, caFile string) {
		certFile, keyFile, caFile = filepath.Join(dir, "link.crt"), filepath.Join(dir, "link.key"), filepath.Join(dir, "link-ca.crt")
		for p, b := range map[string][]byte{certFile: certPEM, keyFile: keyPEM, caFile: caPEM} {
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return certFile, keyFile, caFile
	}
	certPEM, keyPEM, _ := f.ca.Issue(relaytest.Cert{Node: "forward-1"})
	cf, kf, caf := write(certPEM, keyPEM, f.ca.CAPEM())
	c, err := LoadCredentials(cf, kf, caf)
	if err != nil {
		t.Fatal(err)
	}
	first := c.state.Load().cert.Leaf.SerialNumber

	calls := 0
	cancel := c.OnReload(func() { calls++ })

	// A renewal: new key, same identity.
	certPEM2, keyPEM2, _ := f.ca.Issue(relaytest.Cert{Node: "forward-1"})
	write(certPEM2, keyPEM2, f.ca.CAPEM())
	if err := c.ReloadFiles(cf, kf, caf); err != nil {
		t.Fatal(err)
	}
	if c.Generation() != 2 || calls != 1 || c.state.Load().cert.Leaf.SerialNumber.Cmp(first) == 0 {
		t.Fatalf("generation %d, %d callbacks", c.Generation(), calls)
	}

	// A reload that fails changes nothing and calls nobody.
	write(certPEM2, keyPEM, f.ca.CAPEM()) // key of another certificate
	if err := c.ReloadFiles(cf, kf, caf); err == nil {
		t.Fatal("reloaded a mismatched pair")
	}
	if err := c.ReloadFiles(filepath.Join(dir, "missing"), kf, caf); err == nil {
		t.Fatal("reloaded a missing file")
	}
	if c.Generation() != 2 || calls != 1 {
		t.Fatalf("a failed reload changed generation %d / callbacks %d", c.Generation(), calls)
	}
	if _, err := LoadCredentials(filepath.Join(dir, "missing"), kf, caf); err == nil {
		t.Fatal("loaded a missing file")
	}

	cancel()
	if err := c.Reload(certPEM2, keyPEM2, f.ca.CAPEM()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || c.Generation() != 3 {
		t.Fatalf("a cancelled watcher was called: %d", calls)
	}
}

// New handshakes use the new certificate and bundle; established connections
// run on. A peer whose CA left the bundle is no longer trusted.
func TestReloadAppliesToNewHandshakesOnly(t *testing.T) {
	f := newFixture(t)
	ca2 := mustPKI(t, "next")
	server := f.creds("forward-2")
	client := credsFrom(t, f.ca, relaytest.Bundle(f.ca, ca2), relaytest.Cert{Node: "forward-1"})
	l := listen(t, server, []string{id("forward-1")})

	c1, err := dial(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	in1 := accept(t, l)
	defer func() { _ = in1.Close() }()
	serial1 := c1.Peer().Chain[0].SerialNumber
	if !client.PeerStillTrusted(c1.Peer()) || !server.PeerStillTrusted(in1.Peer()) {
		t.Fatal("peers not trusted under the bundle that verified them")
	}
	if client.PeerStillTrusted(Peer{}) {
		t.Fatal("an empty peer is trusted")
	}

	// The server renews with a new key: the next dial sees the new certificate.
	certPEM, keyPEM, _ := f.ca.Issue(relaytest.Cert{Node: "forward-2"})
	if err := server.Reload(certPEM, keyPEM, f.ca.CAPEM()); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c1, in1) // the old connection is untouched
	c2, err := dial(t, l, client, "forward-2", id("forward-2"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	in2 := accept(t, l)
	defer func() { _ = in2.Close() }()
	if c2.Peer().Chain[0].SerialNumber.Cmp(serial1) == 0 {
		t.Fatal("the new handshake presented the old certificate")
	}

	// The server's CA is replaced by a new one and the old CA is dropped from
	// its bundle: its established peer (whose chain led to the old CA) is no
	// longer trusted, and a client that does not know the new CA is refused.
	certPEM, keyPEM, _ = ca2.Issue(relaytest.Cert{Node: "forward-2"})
	if err := server.Reload(certPEM, keyPEM, ca2.CAPEM()); err != nil {
		t.Fatal(err)
	}
	if server.PeerStillTrusted(in1.Peer()) {
		t.Fatal("a peer whose CA was dropped is still trusted")
	}
	if !client.PeerStillTrusted(c1.Peer()) {
		// the client's bundle still holds the old CA, so its view of the
		// server's old certificate holds
		t.Fatal("the client's own bundle was not changed")
	}
	// the old client certificate no longer verifies at the server
	c3, err := dial(t, l, client, "forward-2", id("forward-2"))
	if err == nil {
		_ = c3.SetReadDeadline(time.Now().Add(testWait))
		_, err = c3.Read(make([]byte, 1))
		_ = c3.Close()
	}
	if err == nil {
		t.Fatal("a client of a dropped CA completed a handshake")
	}
	waitFor(t, "unknown_ca", func() bool { return l.Stats().Failures[ReasonUnknownCA] == 1 })
}

func TestParseIdentity(t *testing.T) {
	good := []string{
		"spiffe://anixops/test/agent/forward-1",
		"spiffe://anixops/c1/agent/proxy-4294967295",
		"spiffe://anixops/a-b-9/agent/forward-12",
	}
	bad := []string{
		"", "forward-1", "spiffe://anixops/test/kernel", "spiffe://anixops/test/module/forward",
		"spiffe://anixops/test/agent/forward-0", "spiffe://anixops/test/agent/forward-01",
		"spiffe://anixops/test/agent/forward-4294967296", "spiffe://anixops/test/agent/forward-",
		"spiffe://anixops/test/agent/other-1", "spiffe://anixops/test/agent/forward-1/",
		"spiffe://anixops/test/agent/forward-1/x", "spiffe://anixops/test/agent/forward-1?x=1",
		"spiffe://anixops/test/agent/forward-1#f", "spiffe://anixops:80/test/agent/forward-1",
		"spiffe://user@anixops/test/agent/forward-1", "spiffe://example.org/test/agent/forward-1",
		"SPIFFE://anixops/test/agent/forward-1", "spiffe://ANIXOPS/test/agent/forward-1",
		"spiffe://anixops/Test/agent/forward-1", "spiffe://anixops/-x/agent/forward-1",
		"spiffe://anixops/test/agent/forward%2d1", "spiffe://anixops//test/agent/forward-1",
		" spiffe://anixops/test/agent/forward-1", "spiffe://anixops/test/agent/forward-1\n",
		"spiffe:anixops/test/agent/forward-1", "https://anixops/test/agent/forward-1",
		"spiffe://anixops/" + strings.Repeat("a", 64) + "/agent/forward-1",
	}
	for _, s := range good {
		if got, err := ParseIdentity(s); err != nil || got != s {
			t.Errorf("%q: %q %v", s, got, err)
		}
	}
	for _, s := range bad {
		if _, err := ParseIdentity(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}
