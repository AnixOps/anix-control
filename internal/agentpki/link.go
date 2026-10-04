package agentpki

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The forward link PKI (owner decision H28, forward-sdk.md section 6.2).
//
// Forward engines that encrypt links between nodes (gost) verify a
// certificate chain and the dialled server name; they cannot match a SPIFFE
// URI, and they must never hold the Agent's Control key. So a dedicated
// link CA, a self-signed root separate from the module, kernel and Agent CA,
// issues each node whose Agent negotiated forward.v1 a link certificate for
// a key the Agent generates for it alone:
//
//   - the subject's common name and the only DNS name are the node's
//     identity name (proxy-12, forward-7), the planner's default server_name;
//   - the only URI is the node's SPIFFE identity
//     (spiffe://anixops/<cluster>/agent/forward-7), the planner's
//     peer_identity and ingress_peers;
//   - the extended key usages are serverAuth and clientAuth;
//   - it lives as long as the Agent certificate (seven days) and is renewed
//     at two thirds of its lifetime, alongside it.
//
// The CA's key is sealed with module_runtime.ca_kek under additional data of
// its own, so a sealed module CA key can never be opened as a link CA key.
// It rotates as the module CA does, with the link certificate lifetime as
// the overlap: a next CA joins the link trust bundle at once and starts
// signing only one link certificate lifetime later, by when every node that
// renewed has fetched it; a retired CA stays in the bundle until the last
// link certificate it signed has expired.

const (
	// DefaultLinkCertificateLifetime is the lifetime of link certificates:
	// the Agent certificate's.
	DefaultLinkCertificateLifetime = DefaultCertificateLifetime
	// linkCALifetime bounds a link CA certificate; rotate well before it
	// ends.
	linkCALifetime = 5 * 365 * 24 * time.Hour
	// linkClockSkew backdates certificates for peers whose clock is behind.
	linkClockSkew = time.Minute
	// linkKEKSize is the AES-256 key size of module_runtime.ca_kek.
	linkKEKSize = 32
)

var (
	// ErrLinkDisabled means the kernel does not run the forward link CA. It
	// needs the built-in CA's module_runtime.ca_kek with pki builtin; an
	// external PKI holds no key in the kernel, so it cannot issue link
	// certificates either.
	ErrLinkDisabled = errors.New("forward link certificates need the built-in CA (module_runtime.ca_kek with module_runtime.pki: builtin)")
	// ErrNoLinkAuthority means the cluster has no current link CA yet.
	ErrNoLinkAuthority = errors.New("the forward link PKI has no current CA")
	// ErrLinkNotNegotiated means the node's Agent did not negotiate
	// forward.v1 in its last Hello.
	ErrLinkNotNegotiated = errors.New("the node's Agent has not negotiated forward.v1")
	// ErrInvalidLinkRequest reports an unacceptable link certificate
	// request: malformed, signed with an unsupported key or with the Agent's
	// own key, or asking for a name other than the node's.
	ErrInvalidLinkRequest = errors.New("invalid link certificate request")
)

// AuditActionLinkIssue is the operation log action of an issued link
// certificate.
const AuditActionLinkIssue = "forward_link_certificate_issue"

// LinkAuthorityOptions configures a LinkAuthority.
type LinkAuthorityOptions struct {
	DB      *gorm.DB
	Cluster string
	// KEK seals the CA keys: module_runtime.ca_kek (modulepki.ParseKEK).
	KEK []byte
	// Lifetime of link certificates; DefaultLinkCertificateLifetime when
	// zero.
	Lifetime time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// LinkAuthority is the forward link CA of one cluster.
type LinkAuthority struct {
	db       *gorm.DB
	cluster  string
	aead     cipher.AEAD
	lifetime time.Duration
	now      func() time.Time
}

// NewLinkAuthority returns the forward link CA of opts.Cluster.
func NewLinkAuthority(opts LinkAuthorityOptions) (*LinkAuthority, error) {
	if opts.DB == nil {
		return nil, errors.New("the forward link PKI needs a database")
	}
	if !moduletls.ValidCluster(opts.Cluster) {
		return nil, fmt.Errorf("invalid cluster name %q", opts.Cluster)
	}
	if len(opts.KEK) != linkKEKSize {
		return nil, errors.New("the forward link CA key-encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(opts.KEK)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	lifetime := opts.Lifetime
	if lifetime == 0 {
		lifetime = DefaultLinkCertificateLifetime
	}
	if lifetime < modulepki.MinLeafLifetime || lifetime > modulepki.MaxLeafLifetime {
		return nil, fmt.Errorf("link certificate lifetime %s must be between %s and %s", lifetime, modulepki.MinLeafLifetime, modulepki.MaxLeafLifetime)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &LinkAuthority{db: opts.DB, cluster: opts.Cluster, aead: aead, lifetime: lifetime, now: now}, nil
}

// Cluster returns the cluster the link CA serves.
func (a *LinkAuthority) Cluster() string { return a.cluster }

// Lifetime returns the lifetime of link certificates, which is also the
// rotation overlap.
func (a *LinkAuthority) Lifetime() time.Duration { return a.lifetime }

// Ensure creates the cluster's first link CA when it has none. Concurrent
// kernels call it under the bootstrap lock.
func (a *LinkAuthority) Ensure(ctx context.Context) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.ForwardLinkCA{}).Where("cluster = ? AND state = ?", a.cluster, model.ForwardLinkCAStateCurrent).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		ca, err := a.newCA()
		if err != nil {
			return err
		}
		activated := a.now().UTC()
		ca.State, ca.ActivatedAt = model.ForwardLinkCAStateCurrent, &activated
		return tx.Create(ca).Error
	})
}

// Rotate creates the next link CA. It joins the trust bundle at once and
// starts signing one link certificate lifetime later (Maintain), so every
// node that renewed meanwhile trusts it before any peer presents a
// certificate it signed. Rotating while a next CA exists returns it.
func (a *LinkAuthority) Rotate(ctx context.Context) (*model.ForwardLinkCA, error) {
	var result model.ForwardLinkCA
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("cluster = ? AND state = ?", a.cluster, model.ForwardLinkCAStateNext).First(&result).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		ca, err := a.newCA()
		if err != nil {
			return err
		}
		ca.State = model.ForwardLinkCAStateNext
		if err := tx.Create(ca).Error; err != nil {
			return err
		}
		result = *ca
		return nil
	})
	return &result, err
}

// Maintain promotes a next link CA that has been in the trust bundle for
// one link certificate lifetime and retires the current one.
func (a *LinkAuthority) Maintain(ctx context.Context) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var next model.ForwardLinkCA
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cluster = ? AND state = ?", a.cluster, model.ForwardLinkCAStateNext).First(&next).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		now := a.now().UTC()
		if next.CreatedAt.Add(a.lifetime).After(now) {
			return nil
		}
		if err := tx.Model(&model.ForwardLinkCA{}).Where("cluster = ? AND state = ?", a.cluster, model.ForwardLinkCAStateCurrent).
			Updates(map[string]any{"state": model.ForwardLinkCAStateRetired, "retired_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ForwardLinkCA{}).Where("id = ?", next.ID).
			Updates(map[string]any{"state": model.ForwardLinkCAStateCurrent, "activated_at": now}).Error
	})
}

// CAs returns the cluster's link CAs, oldest first, without their keys'
// plaintext (SealedKey stays sealed and is not serialized).
func (a *LinkAuthority) CAs(ctx context.Context) ([]model.ForwardLinkCA, error) {
	var rows []model.ForwardLinkCA
	err := a.db.WithContext(ctx).Where("cluster = ?", a.cluster).Order("id").Find(&rows).Error
	return rows, err
}

// TrustBundle returns every link CA a node must trust to verify its peers'
// link certificates: the current and next CAs and retired CAs whose link
// certificates may still be valid.
func (a *LinkAuthority) TrustBundle(ctx context.Context) ([]*x509.Certificate, error) {
	return a.trustBundle(a.db.WithContext(ctx))
}

func (a *LinkAuthority) trustBundle(db *gorm.DB) ([]*x509.Certificate, error) {
	var rows []model.ForwardLinkCA
	if err := db.Where("cluster = ?", a.cluster).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := a.now().UTC()
	var bundle []*x509.Certificate
	for _, row := range rows {
		if row.State == model.ForwardLinkCAStateRetired && (row.RetiredAt == nil || row.RetiredAt.Add(a.lifetime+linkClockSkew).Before(now)) {
			continue
		}
		certificate, err := parseLinkCertificatePEM(row.CertificatePEM)
		if err != nil {
			return nil, fmt.Errorf("forward link CA %s: %w", row.KeyID, err)
		}
		bundle = append(bundle, certificate)
	}
	if len(bundle) == 0 {
		return nil, ErrNoLinkAuthority
	}
	return bundle, nil
}

// LinkIssued is a signed link certificate.
type LinkIssued struct {
	CertificateDER []byte
	// TrustBundleDER is the link trust bundle at issuance.
	TrustBundleDER [][]byte
	Identity       agentcontrol.AgentIdentity
	// DNSName is the certificate's only DNS name: the node's identity name.
	DNSName     string
	Serial      string
	NotAfter    time.Time
	RenewAfter  time.Time
	IssuerKeyID string
}

// issue signs, inside tx, node's link certificate for the public key of
// csrDER with the current link CA and records it. agentKey is the public key
// of the Agent certificate that asked; the link key must differ from it.
func (a *LinkAuthority) issue(tx *gorm.DB, node agentcontrol.AgentNode, agentSerial string, agentKey crypto.PublicKey, csrDER []byte) (LinkIssued, error) {
	identity, err := agentcontrol.NewAgentIdentity(a.cluster, node)
	if err != nil {
		return LinkIssued{}, err
	}
	dnsName := node.String()
	publicKey, err := checkLinkCSR(csrDER, dnsName, identity.String(), agentKey)
	if err != nil {
		return LinkIssued{}, err
	}
	var row model.ForwardLinkCA
	if err := tx.Where("cluster = ? AND state = ?", a.cluster, model.ForwardLinkCAStateCurrent).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return LinkIssued{}, ErrNoLinkAuthority
		}
		return LinkIssued{}, err
	}
	caCertificate, err := parseLinkCertificatePEM(row.CertificatePEM)
	if err != nil {
		return LinkIssued{}, err
	}
	caKey, err := a.unseal(row)
	if err != nil {
		return LinkIssued{}, err
	}
	serial, err := randomSerial()
	if err != nil {
		return LinkIssued{}, err
	}
	now := a.now().UTC()
	notAfter := now.Add(a.lifetime)
	if notAfter.After(caCertificate.NotAfter) {
		notAfter = caCertificate.NotAfter
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: dnsName},
		NotBefore:             now.Add(-linkClockSkew),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{dnsName},
		URIs:                  []*url.URL{identity.URL()},
	}
	if _, isRSA := publicKey.(*rsa.PublicKey); isRSA {
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCertificate, publicKey, caKey)
	if err != nil {
		return LinkIssued{}, err
	}
	bundle, err := a.trustBundle(tx)
	if err != nil {
		return LinkIssued{}, err
	}
	bundleDER := make([][]byte, 0, len(bundle))
	for _, certificate := range bundle {
		bundleDER = append(bundleDER, certificate.Raw)
	}
	issued := LinkIssued{
		CertificateDER: der, TrustBundleDER: bundleDER, Identity: identity, DNSName: dnsName,
		Serial: modulepki.SerialString(serial), NotAfter: notAfter, RenewAfter: now.Add(notAfter.Sub(now) * 2 / 3),
		IssuerKeyID: row.KeyID,
	}
	if err := tx.Create(&model.ForwardLinkCertificate{
		Serial: issued.Serial, NodeKind: node.Kind, NodeID: uint(node.ID), Cluster: a.cluster, AgentSerial: agentSerial,
		IssuerKeyID: row.KeyID, DNSName: dnsName, SPIFFEID: identity.String(), NotAfter: notAfter, CreatedAt: now,
	}).Error; err != nil {
		return LinkIssued{}, err
	}
	return issued, nil
}

// checkLinkCSR verifies a link certificate request and returns its public
// key. Control builds the certificate itself and copies nothing from the
// request but the key; still, a request that asks for another name than the
// node's DNS name and SPIFFE ID, or for an IP address or e-mail name, is
// refused rather than silently narrowed, and so is one signed with the
// Agent's own key.
func checkLinkCSR(der []byte, dnsName, spiffeID string, agentKey crypto.PublicKey) (crypto.PublicKey, error) {
	request, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLinkRequest, err)
	}
	if err := request.CheckSignature(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLinkRequest, err)
	}
	switch key := request.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if key.Curve != elliptic.P256() && key.Curve != elliptic.P384() {
			return nil, fmt.Errorf("%w: ECDSA keys must use P-256 or P-384", ErrInvalidLinkRequest)
		}
	case ed25519.PublicKey:
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 {
			return nil, fmt.Errorf("%w: RSA keys must have at least 2048 bits", ErrInvalidLinkRequest)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported key type", ErrInvalidLinkRequest)
	}
	if len(request.IPAddresses) > 0 || len(request.EmailAddresses) > 0 {
		return nil, fmt.Errorf("%w: only the DNS name %s and the URI %s may be requested", ErrInvalidLinkRequest, dnsName, spiffeID)
	}
	for _, name := range request.DNSNames {
		if name != dnsName {
			return nil, fmt.Errorf("%w: DNS name %q is not the node's (%s)", ErrInvalidLinkRequest, name, dnsName)
		}
	}
	for _, uri := range request.URIs {
		if uri.String() != spiffeID {
			return nil, fmt.Errorf("%w: URI %q is not the node's identity (%s)", ErrInvalidLinkRequest, uri.String(), spiffeID)
		}
	}
	if agentKey != nil {
		linkDER, err := x509.MarshalPKIXPublicKey(request.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidLinkRequest, err)
		}
		if agentDER, err := x509.MarshalPKIXPublicKey(agentKey); err == nil && bytes.Equal(linkDER, agentDER) {
			return nil, fmt.Errorf("%w: the link key must not be the Agent certificate's key", ErrInvalidLinkRequest)
		}
	}
	return request.PublicKey, nil
}

func (a *LinkAuthority) newCA() (*model.ForwardLinkCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(publicDER)
	keyID := hex.EncodeToString(digest[:16])
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := a.now().UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "AnixOps forward link CA " + a.cluster + " " + keyID[:8], Organization: []string{"AnixOps"}},
		NotBefore:             now.Add(-linkClockSkew),
		NotAfter:              now.Add(linkCALifetime),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		SubjectKeyId:          digest[:20],
		// Only AnixOps SPIFFE identities can chain to this CA.
		PermittedURIDomains: []string{moduletls.TrustDomain},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	sealed, err := a.seal(key, keyID)
	if err != nil {
		return nil, err
	}
	return &model.ForwardLinkCA{
		Cluster: a.cluster, KeyID: keyID,
		CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		SealedKey:      sealed, NotBefore: template.NotBefore, NotAfter: template.NotAfter, CreatedAt: now,
	}, nil
}

// additionalData binds a sealed key to the link PKI, the cluster and the
// CA: the module CA seals under "anixops-module-ca:", so neither key opens
// as the other.
func (a *LinkAuthority) additionalData(keyID string) []byte {
	return []byte("anixops-forward-link-ca:" + a.cluster + ":" + keyID)
}

func (a *LinkAuthority) seal(key *ecdsa.PrivateKey, keyID string) (string, error) {
	plain, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(a.aead.Seal(nonce, nonce, plain, a.additionalData(keyID))), nil
}

func (a *LinkAuthority) unseal(row model.ForwardLinkCA) (crypto.Signer, error) {
	sealed, err := base64.StdEncoding.DecodeString(row.SealedKey)
	if err != nil || len(sealed) < a.aead.NonceSize() {
		return nil, fmt.Errorf("forward link CA %s key is corrupt", row.KeyID)
	}
	nonce, ciphertext := sealed[:a.aead.NonceSize()], sealed[a.aead.NonceSize():]
	plain, err := a.aead.Open(nil, nonce, ciphertext, a.additionalData(row.KeyID))
	if err != nil {
		return nil, fmt.Errorf("forward link CA %s key cannot be unsealed: wrong key-encryption key", row.KeyID)
	}
	key, err := x509.ParsePKCS8PrivateKey(plain)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("forward link CA %s key cannot sign", row.KeyID)
	}
	return signer, nil
}

func randomSerial() (*big.Int, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	raw[0] &= 0x7f
	return new(big.Int).SetBytes(raw), nil
}

func parseLinkCertificatePEM(value string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

// Link returns the forward link CA, nil when the agent PKI runs without it.
func (s *Service) Link() *LinkAuthority { return s.link }

// IssueLinkCertificate issues the node of the verified Agent certificate
// peer (VerifyPeer) a link certificate for the key of csrDER. The Agent
// certificate's record must be unrevoked, the node enabled, and its Agent
// must have negotiated forward.v1 in its last Hello.
func (s *Service) IssueLinkCertificate(ctx context.Context, peer *x509.Certificate, csrDER []byte, remoteAddr string) (LinkIssued, error) {
	if s.link == nil {
		return LinkIssued{}, ErrLinkDisabled
	}
	identity, err := agentcontrol.AgentIdentityFromCertificate(peer)
	if err != nil {
		return LinkIssued{}, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
	}
	if identity.Cluster != s.cluster {
		return LinkIssued{}, ErrCertificateWrongCluster
	}
	agentSerial := modulepki.SerialString(peer.SerialNumber)
	var issued LinkIssued
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLiveAgentCertificate(tx, s.cluster, agentSerial, identity.Node); err != nil {
			return err
		}
		if err := requireEnabledNode(tx, identity.Node); err != nil {
			if errors.Is(err, ErrInvalidNode) {
				return ErrCertificateRevoked
			}
			return err
		}
		if err := requireForwardNegotiated(tx, identity.Node); err != nil {
			return err
		}
		signed, err := s.link.issue(tx, identity.Node, agentSerial, peer.PublicKey, csrDER)
		if err != nil {
			return err
		}
		issued = signed
		return writeAudit(tx, auditEntry{
			Actor: "agent:" + identity.Node.String(), Action: AuditActionLinkIssue, Node: identity.Node, IP: remoteAddr,
			Content: map[string]any{
				"node": identity.Node.String(), "serial": signed.Serial, "agent_serial": agentSerial,
				"issuer_key_id": signed.IssuerKeyID, "not_after": signed.NotAfter.Format(time.RFC3339),
			},
		})
	})
	return issued, err
}

// LinkTrustBundle returns the link trust bundle: ErrLinkDisabled without
// the link CA.
func (s *Service) LinkTrustBundle(ctx context.Context) ([]*x509.Certificate, error) {
	if s.link == nil {
		return nil, ErrLinkDisabled
	}
	return s.link.TrustBundle(ctx)
}

// requireLiveAgentCertificate fails with ErrCertificateRevoked unless the
// Agent certificate serial is recorded for node, unrevoked, with an
// unrevoked enrollment.
func requireLiveAgentCertificate(tx *gorm.DB, cluster, serial string, node agentcontrol.AgentNode) error {
	var record model.AgentCertificate
	if err := tx.First(&record, "serial = ?", serial).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCertificateRevoked
		}
		return err
	}
	if record.RevokedAt != nil || record.NodeKind != node.Kind || record.NodeID != uint(node.ID) || record.Cluster != cluster {
		return ErrCertificateRevoked
	}
	var enrollment model.AgentEnrollment
	if err := tx.First(&enrollment, "id = ?", record.EnrollmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCertificateRevoked
		}
		return err
	}
	if enrollment.RevokedAt != nil {
		return ErrCertificateRevoked
	}
	return nil
}

// requireForwardNegotiated fails with ErrLinkNotNegotiated unless node's
// forwarding inventory row says its Agent negotiated forward.v1 in its last
// Hello (kernelforward.RecordHello sets and clears the flag).
func requireForwardNegotiated(tx *gorm.DB, node agentcontrol.AgentNode) error {
	var rows []model.KernelForwardNode
	if err := tx.Select("node_ref", "negotiated").Where("node_ref = ?", node.String()).Limit(1).Find(&rows).Error; err != nil {
		if !tx.Migrator().HasTable(&model.KernelForwardNode{}) {
			return ErrLinkNotNegotiated
		}
		return err
	}
	if len(rows) == 0 || !rows[0].Negotiated {
		return ErrLinkNotNegotiated
	}
	return nil
}
