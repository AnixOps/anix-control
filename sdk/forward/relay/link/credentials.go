package link

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Credentials are one node's link credentials: its link certificate and key
// (H28, internal/agentpki/link.go) and the link trust bundle (the current,
// next and retired link CAs, link-ca.crt). They are loaded from the three
// files the Agent writes (or from memory) and replaced as a whole by Reload
// without re-creating a listener, a dialler or a connection: new handshakes
// use the new certificate and bundle, established connections run on
// (anixops-protocol.md section 3.5). A Reload that fails leaves the previous
// credentials in place.
//
// The relay never sees the Agent's own certificate or key, only these.
type Credentials struct {
	mu       sync.Mutex // serialises Reload and watchers
	state    atomic.Pointer[credState]
	watchers map[int]func()
	nextID   int
}

type credState struct {
	cert       tls.Certificate // chain and key; Leaf is set
	roots      *x509.CertPool
	identity   string
	node       string
	notAfter   time.Time
	generation uint64
}

// NewCredentials parses a link certificate chain, its key and a trust bundle
// (PEM). The certificate must be a link certificate: valid now, exactly one
// URI name (an agent identity), exactly one DNS name that is that identity's
// node name, no other names, serverAuth and clientAuth, signed by a CA of
// the bundle for both uses. Anything else is an error, so a node never runs
// with credentials its peers would refuse.
func NewCredentials(certPEM, keyPEM, caPEM []byte) (*Credentials, error) {
	st, err := parseCredentials(certPEM, keyPEM, caPEM, time.Now())
	if err != nil {
		return nil, err
	}
	c := &Credentials{watchers: make(map[int]func())}
	st.generation = 1
	c.state.Store(st)
	return c, nil
}

// LoadCredentials reads the three files of NewCredentials.
func LoadCredentials(certFile, keyFile, caFile string) (*Credentials, error) {
	certPEM, keyPEM, caPEM, err := readFiles(certFile, keyFile, caFile)
	if err != nil {
		return nil, err
	}
	return NewCredentials(certPEM, keyPEM, caPEM)
}

func readFiles(certFile, keyFile, caFile string) (certPEM, keyPEM, caPEM []byte, err error) {
	for _, f := range []struct {
		what, path string
		into       *[]byte
	}{{"certificate", certFile, &certPEM}, {"key", keyFile, &keyPEM}, {"trust bundle", caFile, &caPEM}} {
		if *f.into, err = os.ReadFile(f.path); err != nil { // #nosec G304 -- the Agent-written link files, named by the caller's configuration
			return nil, nil, nil, fmt.Errorf("link: reading the link %s: %w", f.what, err)
		}
	}
	return certPEM, keyPEM, caPEM, nil
}

// Reload replaces the credentials. On error nothing changes. Watchers
// (OnReload) are called after a successful reload.
func (c *Credentials) Reload(certPEM, keyPEM, caPEM []byte) error {
	st, err := parseCredentials(certPEM, keyPEM, caPEM, time.Now())
	if err != nil {
		return err
	}
	c.mu.Lock()
	st.generation = c.state.Load().generation + 1
	c.state.Store(st)
	watchers := slices.Collect(func(yield func(func()) bool) {
		for _, f := range c.watchers {
			if !yield(f) {
				return
			}
		}
	})
	c.mu.Unlock()
	for _, f := range watchers {
		f()
	}
	return nil
}

// ReloadFiles is Reload with the three files of LoadCredentials.
func (c *Credentials) ReloadFiles(certFile, keyFile, caFile string) error {
	certPEM, keyPEM, caPEM, err := readFiles(certFile, keyFile, caFile)
	if err != nil {
		return err
	}
	return c.Reload(certPEM, keyPEM, caPEM)
}

// OnReload registers f to be called after every successful Reload, on the
// goroutine that reloaded. The carrier layer uses it to re-check established
// connections against the new trust bundle (PeerStillTrusted). The returned
// function unregisters f.
func (c *Credentials) OnReload(f func()) (cancel func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	c.watchers[id] = f
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.watchers, id)
	}
}

// Identity returns this node's SPIFFE ID, the value its peers pin.
func (c *Credentials) Identity() string { return c.state.Load().identity }

// Node returns this node's identity name, the DNS name of its certificate.
func (c *Credentials) Node() string { return c.state.Load().node }

// NotAfter returns when the current certificate expires.
func (c *Credentials) NotAfter() time.Time { return c.state.Load().notAfter }

// Generation counts the credentials loaded: 1 for the first, one more per
// successful Reload.
func (c *Credentials) Generation() uint64 { return c.state.Load().generation }

// PeerStillTrusted reports whether the chain a peer presented when its
// connection was established still verifies under the current trust bundle,
// for the key usage it was verified for, now. A connection whose peer fails
// this lost its CA from the bundle (or the certificate expired) and should be
// closed at once (anixops-protocol.md section 3.5).
func (c *Credentials) PeerStillTrusted(p Peer) bool {
	if len(p.Chain) == 0 {
		return false
	}
	return verifyChain(p.Chain, c.state.Load().roots, p.usage, time.Now()) == nil
}

func verifyChain(chain []*x509.Certificate, roots *x509.CertPool, usage x509.ExtKeyUsage, now time.Time) error {
	inter := x509.NewCertPool()
	for _, ic := range chain[1:] {
		inter.AddCert(ic)
	}
	_, err := chain[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{usage}})
	return err
}

func parseCredentials(certPEM, keyPEM, caPEM []byte, now time.Time) (*credState, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("link: the link certificate and key: %w", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("link: the link certificate: %w", err)
	}
	cert.Leaf = leaf
	roots, err := parseBundle(caPEM)
	if err != nil {
		return nil, err
	}
	identity, node, err := certIdentity(leaf)
	if err != nil {
		return nil, fmt.Errorf("link: this node's certificate: %w", err)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != node {
		return nil, fmt.Errorf("link: this node's certificate names %q, want exactly the DNS name %q of its identity", leaf.DNSNames, node)
	}
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if !slices.Contains(leaf.ExtKeyUsage, usage) {
			return nil, errors.New("link: this node's certificate needs both serverAuth and clientAuth")
		}
		if err := verifyChain(append([]*x509.Certificate{leaf}, parseExtra(cert)...), roots, usage, now); err != nil {
			return nil, fmt.Errorf("link: this node's certificate does not verify under the trust bundle: %w", err)
		}
	}
	return &credState{cert: cert, roots: roots, identity: identity, node: node, notAfter: leaf.NotAfter}, nil
}

// parseExtra returns the intermediates after the leaf in a key pair's chain.
func parseExtra(cert tls.Certificate) []*x509.Certificate {
	var out []*x509.Certificate
	for _, der := range cert.Certificate[1:] {
		if c, err := x509.ParseCertificate(der); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// parseBundle reads a PEM trust bundle: at least one certificate, every one a
// CA, nothing else in it.
func parseBundle(caPEM []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	n := 0
	rest := caPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("link: the trust bundle holds a %q block, want certificates only", block.Type)
		}
		ca, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("link: the trust bundle: %w", err)
		}
		if !ca.IsCA {
			return nil, errors.New("link: the trust bundle holds a certificate that is not a CA")
		}
		pool.AddCert(ca)
		n++
	}
	if n == 0 {
		return nil, errors.New("link: the trust bundle holds no PEM certificate")
	}
	for _, b := range rest {
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			return nil, errors.New("link: the trust bundle has data after its last certificate")
		}
	}
	return pool, nil
}
