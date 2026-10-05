package relaytest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestIssuedCertificatesHaveTheLinkShape(t *testing.T) {
	ca, err := New("a")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue(Cert{Node: "forward-41", Cluster: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "forward-41" || len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "forward-41" ||
		len(leaf.URIs) != 1 || leaf.URIs[0].String() != Identity("c1", "forward-41") || leaf.URIs[0].String() != "spiffe://anixops/c1/agent/forward-41" {
		t.Fatalf("names: %+v", leaf)
	}
	if len(leaf.ExtKeyUsage) != 2 || leaf.NotAfter.Sub(leaf.NotBefore).Hours() < 24*7 {
		t.Fatalf("usage %v, lifetime %v", leaf.ExtKeyUsage, leaf.NotAfter.Sub(leaf.NotBefore))
	}
	block, _ := pem.Decode(ca.CAPEM())
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !root.IsCA || !root.MaxPathLenZero || len(root.PermittedURIDomains) != 1 || root.PermittedURIDomains[0] != TrustDomain {
		t.Fatalf("CA: %+v", root)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatal(err)
	}

	// The name constraint is real: the CA cannot vouch for another domain.
	other, _, err := ca.Issue(Cert{Node: "x", URIs: []string{"spiffe://example.org/c1/agent/x"}})
	if err != nil {
		t.Fatal(err)
	}
	block, _ = pem.Decode(other)
	bad, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err == nil {
		t.Fatal("a certificate outside the trust domain verified")
	}
	if got := Bundle(ca, ca); len(got) != 2*len(ca.CAPEM()) {
		t.Fatal("Bundle")
	}
}
