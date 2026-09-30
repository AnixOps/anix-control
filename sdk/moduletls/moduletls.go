// Package moduletls holds the mTLS identity rules shared by the Control kernel
// and network modules.
//
// Every certificate carries exactly one URI SAN:
//
//	spiffe://anixops/<cluster>/kernel             the kernel
//	spiffe://anixops/<cluster>/module/<package>   a module
//
// Peers are authenticated by that identity, never by host names, so a module
// can be reached at any address (pod IP, service name).
package moduletls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// TrustDomain is the SPIFFE trust domain of every AnixOps identity.
const TrustDomain = "anixops"

// Kinds of identities.
const (
	KindKernel = "kernel"
	KindModule = "module"
)

var (
	// ErrInvalidIdentity reports a certificate or URI that is not an AnixOps
	// SPIFFE identity.
	ErrInvalidIdentity = errors.New("invalid AnixOps module identity")
	// ErrUnexpectedPeer reports a verified peer whose identity is not the one
	// the connection expects.
	ErrUnexpectedPeer = errors.New("unexpected mTLS peer identity")

	clusterPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	packagePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// Identity is a parsed AnixOps SPIFFE identity.
type Identity struct {
	Cluster   string
	Kind      string
	PackageID string
}

// String returns the SPIFFE ID.
func (id Identity) String() string {
	if id.Kind == KindKernel {
		return "spiffe://" + TrustDomain + "/" + id.Cluster + "/" + KindKernel
	}
	return "spiffe://" + TrustDomain + "/" + id.Cluster + "/" + KindModule + "/" + id.PackageID
}

// URL returns the SPIFFE ID as a URI SAN.
func (id Identity) URL() *url.URL {
	parsed, _ := url.Parse(id.String())
	return parsed
}

// ValidCluster reports whether name can be a cluster name.
func ValidCluster(name string) bool { return clusterPattern.MatchString(name) }

// ValidPackageID reports whether id can name a module.
func ValidPackageID(id string) bool { return packagePattern.MatchString(id) }

// Kernel returns the kernel identity of cluster.
func Kernel(cluster string) (Identity, error) {
	if !ValidCluster(cluster) {
		return Identity{}, fmt.Errorf("%w: cluster %q", ErrInvalidIdentity, cluster)
	}
	return Identity{Cluster: cluster, Kind: KindKernel}, nil
}

// Module returns the identity of a module in cluster.
func Module(cluster, packageID string) (Identity, error) {
	if !ValidCluster(cluster) {
		return Identity{}, fmt.Errorf("%w: cluster %q", ErrInvalidIdentity, cluster)
	}
	if !ValidPackageID(packageID) {
		return Identity{}, fmt.Errorf("%w: package %q", ErrInvalidIdentity, packageID)
	}
	return Identity{Cluster: cluster, Kind: KindModule, PackageID: packageID}, nil
}

// Parse parses a SPIFFE ID.
func Parse(raw string) (Identity, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "spiffe" || parsed.Host != TrustDomain || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return Identity{}, fmt.Errorf("%w: %q", ErrInvalidIdentity, raw)
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	switch {
	case len(segments) == 2 && segments[1] == KindKernel:
		return Kernel(segments[0])
	case len(segments) == 3 && segments[1] == KindModule:
		return Module(segments[0], segments[2])
	default:
		return Identity{}, fmt.Errorf("%w: %q", ErrInvalidIdentity, raw)
	}
}

// FromCertificate returns the identity of a certificate with exactly one
// AnixOps URI SAN and no other SAN kinds.
func FromCertificate(certificate *x509.Certificate) (Identity, error) {
	if certificate == nil || len(certificate.URIs) != 1 || len(certificate.DNSNames) != 0 ||
		len(certificate.IPAddresses) != 0 || len(certificate.EmailAddresses) != 0 {
		return Identity{}, fmt.Errorf("%w: certificate must carry exactly one URI SAN", ErrInvalidIdentity)
	}
	return Parse(certificate.URIs[0].String())
}

// Accept decides whether a verified peer identity may connect.
type Accept func(Identity) error

// AcceptExactly accepts only want.
func AcceptExactly(want Identity) Accept {
	return func(got Identity) error {
		if got != want {
			return fmt.Errorf("%w: got %s, want %s", ErrUnexpectedPeer, got, want)
		}
		return nil
	}
}

// AcceptModules accepts any module of cluster.
func AcceptModules(cluster string) Accept {
	return func(got Identity) error {
		if got.Kind != KindModule || got.Cluster != cluster {
			return fmt.Errorf("%w: got %s, want a module of cluster %s", ErrUnexpectedPeer, got, cluster)
		}
		return nil
	}
}

// VerifyChain verifies a peer chain against roots for both client and
// server use and returns the leaf identity.
func VerifyChain(rawCerts [][]byte, roots *x509.CertPool) (Identity, *x509.Certificate, error) {
	return VerifyChainAt(rawCerts, roots, time.Time{})
}

// VerifyChainAt is VerifyChain at a given time; the zero time means now.
func VerifyChainAt(rawCerts [][]byte, roots *x509.CertPool, at time.Time) (Identity, *x509.Certificate, error) {
	if len(rawCerts) == 0 {
		return Identity{}, nil, fmt.Errorf("%w: no certificate", ErrInvalidIdentity)
	}
	if roots == nil {
		return Identity{}, nil, errors.New("mTLS trust bundle is not loaded")
	}
	certificates := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		certificate, err := x509.ParseCertificate(raw)
		if err != nil {
			return Identity{}, nil, fmt.Errorf("%w: %v", ErrInvalidIdentity, err)
		}
		certificates = append(certificates, certificate)
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		intermediates.AddCert(certificate)
	}
	leaf := certificates[0]
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, CurrentTime: at,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return Identity{}, nil, err
	}
	identity, err := FromCertificate(leaf)
	if err != nil {
		return Identity{}, nil, err
	}
	return identity, leaf, nil
}

// Source supplies the local certificate and the current trust bundle; both
// may change at runtime (renewal, CA rotation).
type Source struct {
	Certificate func() (*tls.Certificate, error)
	Roots       func() *x509.CertPool
}

// verifier checks the peer on every handshake, including resumed sessions,
// which skip VerifyPeerCertificate.
func (s Source) verifier(accept Accept, optional bool) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if optional && len(state.PeerCertificates) == 0 {
			return nil
		}
		raw := make([][]byte, 0, len(state.PeerCertificates))
		for _, certificate := range state.PeerCertificates {
			raw = append(raw, certificate.Raw)
		}
		identity, _, err := VerifyChain(raw, s.Roots())
		if err != nil {
			return err
		}
		if accept != nil {
			return accept(identity)
		}
		return nil
	}
}

// ServerConfig returns a TLS 1.3 server configuration that verifies client
// certificates by SPIFFE identity. With optionalClientCert, clients without a
// certificate may connect (module enrollment); handlers must then check the
// peer themselves.
func (s Source) ServerConfig(accept Accept, optionalClientCert bool) *tls.Config {
	clientAuth := tls.RequireAnyClientCert
	if optionalClientCert {
		clientAuth = tls.RequestClientCert
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: clientAuth,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.Certificate()
		},
		// Chain and identity are verified here instead of by host name.
		VerifyConnection: s.verifier(accept, optionalClientCert),
	}
}

// ClientConfig returns a TLS 1.3 client configuration that authenticates the
// server by SPIFFE identity rather than host name. A nil Certificate
// function makes an anonymous client (module enrollment).
func (s Source) ClientConfig(accept Accept) *tls.Config {
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		// Host-name verification is replaced by VerifyConnection, which
		// verifies the chain against the trust bundle and the SPIFFE ID.
		InsecureSkipVerify: true, // #nosec G402 -- chain and identity are verified in VerifyConnection.
		VerifyConnection:   s.verifier(accept, false),
	}
	if s.Certificate != nil {
		config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return s.Certificate()
		}
	}
	return config
}
