// Package agentpki issues the mTLS client certificates of AnixOps Agents
// (node-ops-service.md, section 5.3).
//
// A certificate names one node, spiffe://anixops/<cluster>/agent/proxy-<id>
// or .../agent/forward-<id>, and is signed by the module PKI's CA
// (internal/modulepki) with its current/next rotation. Agents get their
// first certificate with a bootstrap credential: the node credential they
// already have (a proxy node's API key or a forward node's token), or a
// one-time anixagt_ enrollment credential bound to a node. They renew with
// the current certificate at two thirds of its lifetime.
//
// Every certificate is recorded in v4_kernel_agent_certificate and every
// way in in v4_kernel_agent_enrollment. Revoking, replacing or disabling a
// node's credentials, or deleting the node, revokes its certificates
// (RevokeNode), and the agent listener refuses revoked serials through a
// cache of at most RevocationCacheTTL.
package agentpki

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"gorm.io/gorm"
)

const (
	// DefaultCertificateLifetime is the lifetime of agent certificates
	// (decision D9): nodes can be offline longer than modules.
	DefaultCertificateLifetime = 7 * 24 * time.Hour
	// MaxEnrollmentCredentialTTL bounds a one-time enrollment credential.
	MaxEnrollmentCredentialTTL = 7 * 24 * time.Hour
	// DefaultEnrollmentCredentialTTL is used when a request names none.
	DefaultEnrollmentCredentialTTL = 24 * time.Hour
	// RevocationCacheTTL is the longest the listener trusts a cached
	// revocation answer.
	RevocationCacheTTL = 30 * time.Second
	// rootsRefresh is how often the trust bundle is reloaded.
	rootsRefresh = time.Minute
)

var (
	// ErrDisabled means the kernel does not run the built-in module CA
	// (module_runtime.enabled with pki: builtin), which signs agent
	// certificates.
	ErrDisabled = errors.New("agent enrollment needs the built-in module PKI (module_runtime.enabled with pki: builtin)")
	// ErrEnrollmentRejected is the single answer to any unusable bootstrap
	// credential, so credentials cannot be probed.
	ErrEnrollmentRejected = errors.New("agent enrollment rejected")
	// ErrCertificateRevoked means the certificate, its enrollment or its
	// node's credentials were revoked, or the node is gone or disabled.
	ErrCertificateRevoked = errors.New("agent certificate revoked")
	// ErrInvalidCertificate reports a client certificate that does not
	// authenticate an agent of this cluster: unparsable, not chaining to the
	// trust bundle, expired, or carrying another identity.
	ErrInvalidCertificate = errors.New("invalid agent client certificate")
	// ErrInvalidNode reports an enrollment credential request for a node
	// that does not exist or is disabled.
	ErrInvalidNode = errors.New("agent node not found or disabled")
)

// Options configures a Service.
type Options struct {
	DB        *gorm.DB
	Authority *modulepki.Authority
	// Lifetime of issued certificates; DefaultCertificateLifetime when zero.
	Lifetime time.Duration
	// RevocationCacheTTL defaults to RevocationCacheTTL and may not exceed it.
	RevocationCacheTTL time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// Service is the agent PKI of one cluster.
type Service struct {
	db          *gorm.DB
	authority   *modulepki.Authority
	cluster     string
	lifetime    time.Duration
	now         func() time.Time
	revocations *revocationCache

	rootsMu       sync.Mutex
	roots         *x509.CertPool
	rootsLoadedAt time.Time
}

// New returns the agent PKI signing with opts.Authority.
func New(opts Options) (*Service, error) {
	if opts.DB == nil || opts.Authority == nil {
		return nil, errors.New("agent PKI needs a database and the module CA")
	}
	lifetime := opts.Lifetime
	if lifetime == 0 {
		lifetime = DefaultCertificateLifetime
	}
	if lifetime < modulepki.MinLeafLifetime || lifetime > modulepki.MaxLeafLifetime {
		return nil, fmt.Errorf("agent certificate lifetime %s must be between %s and %s", lifetime, modulepki.MinLeafLifetime, modulepki.MaxLeafLifetime)
	}
	cacheTTL := opts.RevocationCacheTTL
	if cacheTTL <= 0 || cacheTTL > RevocationCacheTTL {
		cacheTTL = RevocationCacheTTL
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		db: opts.DB, authority: opts.Authority, cluster: opts.Authority.Cluster(), lifetime: lifetime, now: now,
		revocations: newRevocationCache(cacheTTL, now),
	}, nil
}

// FromConfig returns the agent PKI of the built-in module CA, or ErrDisabled.
func FromConfig(cfg *config.Config, db *gorm.DB) (*Service, error) {
	if cfg == nil {
		return nil, ErrDisabled
	}
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if errors.Is(err, modulepki.ErrBuiltinPKIDisabled) {
		return nil, ErrDisabled
	}
	if err != nil {
		return nil, err
	}
	return New(Options{DB: db, Authority: authority})
}

// Cluster returns the cluster named in every agent SPIFFE ID.
func (s *Service) Cluster() string { return s.cluster }

// Lifetime returns the lifetime of issued certificates.
func (s *Service) Lifetime() time.Duration { return s.lifetime }

// TrustBundle returns every CA that may have signed a valid agent
// certificate.
func (s *Service) TrustBundle(ctx context.Context) ([]*x509.Certificate, error) {
	return s.authority.TrustBundleRetaining(ctx, s.lifetime)
}

// Roots returns TrustBundle as a pool, reloaded at most every minute; a
// failed reload keeps the last good pool.
func (s *Service) Roots(ctx context.Context) (*x509.CertPool, error) {
	s.rootsMu.Lock()
	defer s.rootsMu.Unlock()
	if s.roots != nil && s.now().Sub(s.rootsLoadedAt) < rootsRefresh {
		return s.roots, nil
	}
	bundle, err := s.TrustBundle(ctx)
	if err != nil {
		if s.roots != nil {
			return s.roots, nil
		}
		return nil, err
	}
	pool := x509.NewCertPool()
	for _, certificate := range bundle {
		pool.AddCert(certificate)
	}
	s.roots, s.rootsLoadedAt = pool, s.now()
	return pool, nil
}

// VerifyPeer verifies a client certificate chain presented on the agent
// listener: it must chain to the trust bundle, be valid now, carry the
// agent identity of a node of this cluster, and not be revoked. Module and
// kernel certificates of the same CA are refused.
func (s *Service) VerifyPeer(ctx context.Context, rawCerts [][]byte) (agentcontrol.AgentIdentity, *x509.Certificate, error) {
	if len(rawCerts) == 0 {
		return agentcontrol.AgentIdentity{}, nil, fmt.Errorf("%w: no certificate", ErrInvalidCertificate)
	}
	certificates := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		certificate, err := x509.ParseCertificate(raw)
		if err != nil {
			return agentcontrol.AgentIdentity{}, nil, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
		}
		certificates = append(certificates, certificate)
	}
	roots, err := s.Roots(ctx)
	if err != nil {
		return agentcontrol.AgentIdentity{}, nil, err
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		intermediates.AddCert(certificate)
	}
	leaf := certificates[0]
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, CurrentTime: s.now(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return agentcontrol.AgentIdentity{}, nil, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
	}
	identity, err := agentcontrol.AgentIdentityFromCertificate(leaf)
	if err != nil {
		return agentcontrol.AgentIdentity{}, nil, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
	}
	if identity.Cluster != s.cluster {
		return agentcontrol.AgentIdentity{}, nil, fmt.Errorf("%w: %s belongs to another cluster", ErrInvalidCertificate, identity)
	}
	revoked, err := s.IsRevoked(ctx, modulepki.SerialString(leaf.SerialNumber), identity.Node)
	if err != nil {
		return agentcontrol.AgentIdentity{}, nil, err
	}
	if revoked {
		return agentcontrol.AgentIdentity{}, nil, ErrCertificateRevoked
	}
	return identity, leaf, nil
}

// hashSecret is the stored form of a credential, the hex SHA-256 the node
// API key hash also uses.
func hashSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}
