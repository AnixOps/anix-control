// Package relaytest builds the credentials the relay packages' tests need
// without the kernel: a link CA shaped like the one internal/agentpki issues
// (a self-signed ECDSA P-256 root, name-constrained to spiffe://anixops URIs,
// signing nothing but link certificates) and node certificates of the H28
// shape (the node's identity name as CN and only DNS name, its SPIFFE ID as
// only URI, serverAuth and clientAuth). Options bend the shape one way at a
// time so a test can present exactly the certificate an attacker or a stale
// node would. It is for tests and in-process harnesses only; nothing here
// belongs on a node.
package relaytest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"time"
)

// TrustDomain is the SPIFFE trust domain of every AnixOps identity.
const TrustDomain = "anixops"

// PKI is one link CA and the certificates it issues.
type PKI struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	// Now is the clock certificates are issued against; time.Now when nil.
	Now func() time.Time
}

// New creates a link CA named name. A test that needs a second, foreign CA
// calls New again.
func New(name string) (*PKI, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "AnixOps forward link CA " + name, Organization: []string{"AnixOps"}},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(5 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		PermittedURIDomains:   []string{TrustDomain},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &PKI{cert: cert, key: key}, nil
}

// CAPEM returns the CA certificate as a trust bundle of one.
func (p *PKI) CAPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.cert.Raw})
}

// Bundle returns a trust bundle of the CAs of all the given PKIs, in order.
func Bundle(pkis ...*PKI) []byte {
	var out []byte
	for _, p := range pkis {
		out = append(out, p.CAPEM()...)
	}
	return out
}

// Identity returns the SPIFFE ID of node in cluster: the value of
// peer_identity and ingress_peers.
func Identity(cluster, node string) string {
	return "spiffe://" + TrustDomain + "/" + cluster + "/agent/" + node
}

// Cert describes a node certificate. The zero value is a valid link
// certificate for Node in Cluster; the other fields bend it.
type Cert struct {
	Cluster string // default "test"
	Node    string // the identity name, such as "forward-41"; required

	// Overrides, for certificates that should not pass.
	DNSNames       []string // replaces the default [Node]
	URIs           []string // replaces the default [Identity(Cluster, Node)]
	IPAddresses    []net.IP
	EmailAddresses []string
	ExtKeyUsage    []x509.ExtKeyUsage // replaces serverAuth and clientAuth
	NoExtKeyUsage  bool               // no extended key usage at all
	NotBefore      time.Time          // default: a minute ago
	NotAfter       time.Time          // default: seven days from now
	Serial         *big.Int
}

// Issue signs a certificate for a fresh key and returns both as PEM.
func (p *PKI) Issue(c Cert) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	if c.Cluster == "" {
		c.Cluster = "test"
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	now = now.UTC()
	if c.NotBefore.IsZero() {
		c.NotBefore = now.Add(-time.Minute)
	}
	if c.NotAfter.IsZero() {
		c.NotAfter = now.Add(7 * 24 * time.Hour)
	}
	dns := c.DNSNames
	if dns == nil {
		dns = []string{c.Node}
	}
	rawURIs := c.URIs
	if rawURIs == nil {
		rawURIs = []string{Identity(c.Cluster, c.Node)}
	}
	var uris []*url.URL
	for _, raw := range rawURIs {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, nil, err
		}
		uris = append(uris, u)
	}
	eku := c.ExtKeyUsage
	if eku == nil && !c.NoExtKeyUsage {
		eku = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
	sn := c.Serial
	if sn == nil {
		sn = serial()
	}
	template := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               pkix.Name{CommonName: c.Node},
		NotBefore:             c.NotBefore,
		NotAfter:              c.NotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           eku,
		BasicConstraintsValid: true,
		DNSNames:              dns,
		URIs:                  uris,
		IPAddresses:           c.IPAddresses,
		EmailAddresses:        c.EmailAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, p.cert, &key.PublicKey, p.key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return n.Add(n, big.NewInt(1))
}
