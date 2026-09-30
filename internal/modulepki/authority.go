// Package modulepki is the kernel's built-in certificate authority for
// network modules. It issues short-lived certificates whose only identity is
// a SPIFFE URI SAN (see pkg/moduletls), enrolls modules with hashed one-time
// or revocable credentials, renews certificates over mTLS, and rotates the CA
// with an overlap during which both CAs are trusted.
//
// The CA private key is stored sealed with AES-256-GCM under a
// key-encryption key from configuration; it is unsealed only to sign.
package modulepki

import (
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
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// caLifetime bounds a CA certificate; rotate well before it ends.
	caLifetime = 5 * 365 * 24 * time.Hour
	// clockSkew backdates certificates to tolerate peers whose clock is
	// slightly behind.
	clockSkew = time.Minute
	// DefaultCertificateLifetime is the lifetime of module and kernel
	// certificates.
	DefaultCertificateLifetime = 24 * time.Hour
	// kekSize is the AES-256 key size of the key-encryption key.
	kekSize = 32
)

var (
	// ErrNoAuthority means the cluster has no current CA yet.
	ErrNoAuthority = errors.New("module PKI has no current CA")
	// ErrInvalidRequest reports a malformed or unacceptable CSR.
	ErrInvalidRequest = errors.New("invalid certificate request")
)

// Options configures an Authority.
type Options struct {
	DB      *gorm.DB
	Cluster string
	// KEK seals the CA private key; see ParseKEK.
	KEK []byte
	// Lifetime of issued certificates; DefaultCertificateLifetime when zero.
	Lifetime time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// Authority is the built-in module CA of one cluster.
type Authority struct {
	db       *gorm.DB
	cluster  string
	aead     cipher.AEAD
	lifetime time.Duration
	now      func() time.Time
}

// ParseKEK decodes a 32-byte key-encryption key given as standard base64 or
// hex.
func ParseKEK(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	for _, decode := range []func(string) ([]byte, error){base64.StdEncoding.DecodeString, hex.DecodeString} {
		if key, err := decode(raw); err == nil && len(key) == kekSize {
			return key, nil
		}
	}
	return nil, errors.New("the module CA key-encryption key must be 32 bytes, base64 or hex encoded")
}

// New returns the Authority of opts.Cluster.
func New(opts Options) (*Authority, error) {
	if opts.DB == nil {
		return nil, errors.New("module PKI needs a database")
	}
	if !moduletls.ValidCluster(opts.Cluster) {
		return nil, fmt.Errorf("invalid module cluster name %q", opts.Cluster)
	}
	if len(opts.KEK) != kekSize {
		return nil, errors.New("the module CA key-encryption key must be 32 bytes")
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
	if lifetime <= 0 {
		lifetime = DefaultCertificateLifetime
	}
	if lifetime < 10*time.Minute || lifetime > 30*24*time.Hour {
		return nil, fmt.Errorf("module certificate lifetime %s must be between 10m and 720h", lifetime)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Authority{db: opts.DB, cluster: opts.Cluster, aead: aead, lifetime: lifetime, now: now}, nil
}

// Cluster returns the cluster the authority serves.
func (a *Authority) Cluster() string { return a.cluster }

// Lifetime returns the lifetime of issued certificates.
func (a *Authority) Lifetime() time.Duration { return a.lifetime }

// Ensure creates the cluster's first CA when it has none.
func (a *Authority) Ensure(ctx context.Context) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.ServiceCA{}).Where("cluster = ? AND state = ?", a.cluster, model.ServiceCAStateCurrent).
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
		ca.State = model.ServiceCAStateCurrent
		ca.ActivatedAt = &activated
		return tx.Create(ca).Error
	})
}

// Rotate creates the next CA. It joins the trust bundle at once and starts
// signing only after one certificate lifetime (see Maintain), so every peer
// has fetched it by then. Rotating while a next CA exists returns it.
func (a *Authority) Rotate(ctx context.Context) (*model.ServiceCA, error) {
	var result model.ServiceCA
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("cluster = ? AND state = ?", a.cluster, model.ServiceCAStateNext).First(&result).Error
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
		ca.State = model.ServiceCAStateNext
		if err := tx.Create(ca).Error; err != nil {
			return err
		}
		result = *ca
		return nil
	})
	return &result, err
}

// Maintain promotes a next CA that has been trusted for one certificate
// lifetime and retires the previous one. Retired CAs stay in the trust bundle
// until the certificates they signed have expired.
func (a *Authority) Maintain(ctx context.Context) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var next model.ServiceCA
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cluster = ? AND state = ?", a.cluster, model.ServiceCAStateNext).First(&next).Error
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
		if err := tx.Model(&model.ServiceCA{}).Where("cluster = ? AND state = ?", a.cluster, model.ServiceCAStateCurrent).
			Updates(map[string]any{"state": model.ServiceCAStateRetired, "retired_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ServiceCA{}).Where("id = ?", next.ID).
			Updates(map[string]any{"state": model.ServiceCAStateCurrent, "activated_at": now}).Error
	})
}

// TrustBundle returns every CA a peer must trust: the current and next CAs
// and retired CAs whose certificates may still be valid.
func (a *Authority) TrustBundle(ctx context.Context) ([]*x509.Certificate, error) {
	var rows []model.ServiceCA
	if err := a.db.WithContext(ctx).Where("cluster = ?", a.cluster).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := a.now().UTC()
	var bundle []*x509.Certificate
	for _, row := range rows {
		if row.State == model.ServiceCAStateRetired && (row.RetiredAt == nil || row.RetiredAt.Add(a.lifetime+clockSkew).Before(now)) {
			continue
		}
		certificate, err := parseCertificatePEM(row.CertificatePEM)
		if err != nil {
			return nil, fmt.Errorf("module CA %s: %w", row.KeyID, err)
		}
		bundle = append(bundle, certificate)
	}
	if len(bundle) == 0 {
		return nil, ErrNoAuthority
	}
	return bundle, nil
}

// TrustBundlePEM returns TrustBundle PEM-encoded, the file a module needs
// to verify the kernel when it enrolls (ANIX_MODULE_TRUST_BUNDLE_FILE).
func (a *Authority) TrustBundlePEM(ctx context.Context) ([]byte, error) {
	bundle, err := a.TrustBundle(ctx)
	if err != nil {
		return nil, err
	}
	var encoded []byte
	for _, certificate := range bundle {
		encoded = append(encoded, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})...)
	}
	return encoded, nil
}

// Roots returns TrustBundle as a certificate pool.
func (a *Authority) Roots(ctx context.Context) (*x509.CertPool, error) {
	bundle, err := a.TrustBundle(ctx)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	for _, certificate := range bundle {
		pool.AddCert(certificate)
	}
	return pool, nil
}

// Issued is a signed leaf certificate.
type Issued struct {
	CertificateDER []byte
	TrustBundleDER [][]byte
	Identity       moduletls.Identity
	Serial         string
	NotAfter       time.Time
	RenewAfter     time.Time
}

// signer is the current CA with its unsealed key.
type signer struct {
	row         model.ServiceCA
	certificate *x509.Certificate
	key         crypto.Signer
}

func (a *Authority) currentSigner(tx *gorm.DB) (*signer, error) {
	var row model.ServiceCA
	if err := tx.Where("cluster = ? AND state = ?", a.cluster, model.ServiceCAStateCurrent).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoAuthority
		}
		return nil, err
	}
	certificate, err := parseCertificatePEM(row.CertificatePEM)
	if err != nil {
		return nil, err
	}
	key, err := a.unseal(row)
	if err != nil {
		return nil, err
	}
	return &signer{row: row, certificate: certificate, key: key}, nil
}

// sign issues a leaf for identity and publicKey with the current CA.
func (a *Authority) sign(ctx context.Context, tx *gorm.DB, identity moduletls.Identity, publicKey crypto.PublicKey) (Issued, *signer, error) {
	current, err := a.currentSigner(tx)
	if err != nil {
		return Issued{}, nil, err
	}
	serialBytes := make([]byte, 16)
	if _, err := rand.Read(serialBytes); err != nil {
		return Issued{}, nil, err
	}
	serialBytes[0] &= 0x7f
	serial := new(big.Int).SetBytes(serialBytes)
	now := a.now().UTC()
	notAfter := now.Add(a.lifetime)
	if notAfter.After(current.certificate.NotAfter) {
		notAfter = current.certificate.NotAfter
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: identity.String()},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{identity.URL()},
	}
	if _, isRSA := publicKey.(*rsa.PublicKey); isRSA {
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	der, err := x509.CreateCertificate(rand.Reader, template, current.certificate, publicKey, current.key)
	if err != nil {
		return Issued{}, nil, err
	}
	bundle, err := a.TrustBundle(ctx)
	if err != nil {
		return Issued{}, nil, err
	}
	bundleDER := make([][]byte, 0, len(bundle))
	for _, certificate := range bundle {
		bundleDER = append(bundleDER, certificate.Raw)
	}
	lifetime := notAfter.Sub(now)
	return Issued{
		CertificateDER: der, TrustBundleDER: bundleDER, Identity: identity,
		Serial: SerialString(serial), NotAfter: notAfter, RenewAfter: now.Add(lifetime * 2 / 3),
	}, current, nil
}

// SerialString is the recorded form of a certificate serial.
func SerialString(serial *big.Int) string {
	return hex.EncodeToString(serial.Bytes())
}

// checkCSR verifies a PKCS#10 request and returns its public key. Subjects
// and SANs of the request are ignored: the kernel assigns the identity.
func checkCSR(der []byte) (crypto.PublicKey, error) {
	request, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if err := request.CheckSignature(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	switch key := request.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if key.Curve != elliptic.P256() && key.Curve != elliptic.P384() {
			return nil, fmt.Errorf("%w: ECDSA keys must use P-256 or P-384", ErrInvalidRequest)
		}
	case ed25519.PublicKey:
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 {
			return nil, fmt.Errorf("%w: RSA keys must have at least 2048 bits", ErrInvalidRequest)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported key type", ErrInvalidRequest)
	}
	return request.PublicKey, nil
}

func (a *Authority) newCA() (*model.ServiceCA, error) {
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
	serialBytes := make([]byte, 16)
	if _, err := rand.Read(serialBytes); err != nil {
		return nil, err
	}
	serialBytes[0] &= 0x7f
	now := a.now().UTC()
	template := &x509.Certificate{
		SerialNumber:          new(big.Int).SetBytes(serialBytes),
		Subject:               pkix.Name{CommonName: "AnixOps module CA " + a.cluster + " " + keyID[:8], Organization: []string{"AnixOps"}},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(caLifetime),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
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
	return &model.ServiceCA{
		Cluster: a.cluster, KeyID: keyID,
		CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		SealedKey:      sealed, NotBefore: template.NotBefore, NotAfter: template.NotAfter, CreatedAt: now,
	}, nil
}

func (a *Authority) additionalData(keyID string) []byte {
	return []byte("anixops-module-ca:" + a.cluster + ":" + keyID)
}

func (a *Authority) seal(key *ecdsa.PrivateKey, keyID string) (string, error) {
	plain, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := a.aead.Seal(nonce, nonce, plain, a.additionalData(keyID))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (a *Authority) unseal(row model.ServiceCA) (crypto.Signer, error) {
	sealed, err := base64.StdEncoding.DecodeString(row.SealedKey)
	if err != nil || len(sealed) < a.aead.NonceSize() {
		return nil, fmt.Errorf("module CA %s key is corrupt", row.KeyID)
	}
	nonce, ciphertext := sealed[:a.aead.NonceSize()], sealed[a.aead.NonceSize():]
	plain, err := a.aead.Open(nil, nonce, ciphertext, a.additionalData(row.KeyID))
	if err != nil {
		return nil, fmt.Errorf("module CA %s key cannot be unsealed: wrong key-encryption key", row.KeyID)
	}
	key, err := x509.ParsePKCS8PrivateKey(plain)
	if err != nil {
		return nil, err
	}
	signerKey, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("module CA %s key cannot sign", row.KeyID)
	}
	return signerKey, nil
}

func parseCertificatePEM(value string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
