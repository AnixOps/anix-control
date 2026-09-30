package modulepki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// enrollmentPrefix marks enrollment credentials so they are recognizable in
// secret scanners and logs.
const enrollmentPrefix = "anixenr_"

// Enrollment limits.
const (
	MaxOneTimeEnrollmentTTL  = 7 * 24 * time.Hour
	MaxReusableEnrollmentTTL = 366 * 24 * time.Hour
)

var (
	// ErrEnrollmentRejected is the single answer to any unusable enrollment
	// credential, so it cannot be probed.
	ErrEnrollmentRejected = errors.New("module enrollment rejected")
	// ErrCertificateRevoked means the certificate or its enrollment was
	// revoked.
	ErrCertificateRevoked = errors.New("module certificate revoked")
)

// EnrollmentRequest describes a new enrollment credential.
type EnrollmentRequest struct {
	PackageID string
	TTL       time.Duration
	// Reusable credentials (for Kubernetes pods that may restart) can enroll
	// until they expire or are revoked; others enroll once.
	Reusable  bool
	CreatedBy uint
}

// CreateEnrollment stores a new credential and returns it. The credential is
// shown only here; the kernel keeps its SHA-256.
func (a *Authority) CreateEnrollment(ctx context.Context, request EnrollmentRequest) (string, model.ModuleEnrollment, error) {
	if !moduletls.ValidPackageID(request.PackageID) {
		return "", model.ModuleEnrollment{}, fmt.Errorf("invalid module package id %q", request.PackageID)
	}
	limit := MaxOneTimeEnrollmentTTL
	if request.Reusable {
		limit = MaxReusableEnrollmentTTL
	}
	if request.TTL <= 0 || request.TTL > limit {
		return "", model.ModuleEnrollment{}, fmt.Errorf("enrollment lifetime must be between 1s and %s", limit)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", model.ModuleEnrollment{}, err
	}
	credential := enrollmentPrefix + base64.RawURLEncoding.EncodeToString(raw)
	now := a.now().UTC()
	row := model.ModuleEnrollment{
		ID: uuid.NewString(), PackageID: request.PackageID, Cluster: a.cluster,
		CredentialHash: hashCredential(credential), Reusable: request.Reusable,
		ExpiresAt: now.Add(request.TTL), CreatedBy: request.CreatedBy, CreatedAt: now,
	}
	if err := a.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", model.ModuleEnrollment{}, err
	}
	return credential, row, nil
}

// ListEnrollments returns the cluster's enrollments, newest first.
func (a *Authority) ListEnrollments(ctx context.Context) ([]model.ModuleEnrollment, error) {
	var rows []model.ModuleEnrollment
	err := a.db.WithContext(ctx).Where("cluster = ?", a.cluster).Order("created_at DESC").Find(&rows).Error
	return rows, err
}

// RevokeEnrollment stops a credential and every certificate issued through
// it: they can no longer renew, and IsRevoked reports their serials.
func (a *Authority) RevokeEnrollment(ctx context.Context, id string) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := a.now().UTC()
		result := tx.Model(&model.ModuleEnrollment{}).Where("id = ? AND cluster = ? AND revoked_at IS NULL", id, a.cluster).
			Update("revoked_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&model.ModuleEnrollment{}).Where("id = ? AND cluster = ?", id, a.cluster).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		}
		return tx.Model(&model.ModuleCertificate{}).Where("enrollment_id = ? AND revoked_at IS NULL", id).
			Update("revoked_at", now).Error
	})
}

// Enroll exchanges a credential and a CSR for the package's first
// certificate. A one-time credential is consumed atomically; a failed
// issuance leaves it unused.
func (a *Authority) Enroll(ctx context.Context, credential, packageID, cluster string, csrDER []byte) (Issued, error) {
	if cluster != a.cluster || !moduletls.ValidPackageID(packageID) || !strings.HasPrefix(credential, enrollmentPrefix) {
		return Issued{}, ErrEnrollmentRejected
	}
	publicKey, err := checkCSR(csrDER)
	if err != nil {
		return Issued{}, err
	}
	identity, err := moduletls.Module(a.cluster, packageID)
	if err != nil {
		return Issued{}, ErrEnrollmentRejected
	}
	var issued Issued
	err = a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		hash := hashCredential(credential)
		var row model.ModuleEnrollment
		if err := tx.Where("credential_hash = ?", hash).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnrollmentRejected
			}
			return err
		}
		if row.PackageID != packageID || row.Cluster != a.cluster {
			return ErrEnrollmentRejected
		}
		now := a.now().UTC()
		update := tx.Model(&model.ModuleEnrollment{}).
			Where("id = ? AND revoked_at IS NULL AND expires_at > ?", row.ID, now)
		if !row.Reusable {
			update = update.Where("used_at IS NULL")
		}
		result := update.Updates(map[string]any{"used_at": now, "use_count": gorm.Expr("use_count + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEnrollmentRejected
		}
		signed, current, err := a.sign(ctx, tx, identity, publicKey)
		if err != nil {
			return err
		}
		issued = signed
		return a.recordCertificate(tx, signed, row.ID, current.row.KeyID)
	})
	return issued, err
}

// Renew issues a new certificate for the holder of a valid module
// certificate. peer must already be verified against the trust bundle, as
// the module listener does during the TLS handshake.
func (a *Authority) Renew(ctx context.Context, peer *x509.Certificate, csrDER []byte) (Issued, error) {
	identity, err := moduletls.FromCertificate(peer)
	if err != nil {
		return Issued{}, err
	}
	if identity.Kind != moduletls.KindModule || identity.Cluster != a.cluster {
		return Issued{}, fmt.Errorf("%w: %s cannot renew here", moduletls.ErrUnexpectedPeer, identity)
	}
	publicKey, err := checkCSR(csrDER)
	if err != nil {
		return Issued{}, err
	}
	var issued Issued
	err = a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record model.ModuleCertificate
		if err := tx.First(&record, "serial = ?", SerialString(peer.SerialNumber)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCertificateRevoked
			}
			return err
		}
		if record.RevokedAt != nil || record.PackageID != identity.PackageID || record.Cluster != a.cluster {
			return ErrCertificateRevoked
		}
		var enrollment model.ModuleEnrollment
		if err := tx.First(&enrollment, "id = ?", record.EnrollmentID).Error; err != nil {
			return ErrCertificateRevoked
		}
		if enrollment.RevokedAt != nil {
			return ErrCertificateRevoked
		}
		signed, current, err := a.sign(ctx, tx, identity, publicKey)
		if err != nil {
			return err
		}
		issued = signed
		return a.recordCertificate(tx, signed, record.EnrollmentID, current.row.KeyID)
	})
	return issued, err
}

// IsRevoked reports whether a certificate serial was revoked. Unknown
// serials (for example the kernel's own certificate) are not revoked.
func (a *Authority) IsRevoked(ctx context.Context, serial string) (bool, error) {
	var count int64
	err := a.db.WithContext(ctx).Model(&model.ModuleCertificate{}).
		Where("serial = ? AND revoked_at IS NOT NULL", serial).Count(&count).Error
	return count > 0, err
}

// PruneCertificates deletes records of certificates that expired before
// cutoff.
func (a *Authority) PruneCertificates(ctx context.Context, cutoff time.Time) error {
	return a.db.WithContext(ctx).Where("not_after < ?", cutoff).Delete(&model.ModuleCertificate{}).Error
}

func (a *Authority) recordCertificate(tx *gorm.DB, issued Issued, enrollmentID, issuerKeyID string) error {
	return tx.Create(&model.ModuleCertificate{
		Serial: issued.Serial, PackageID: issued.Identity.PackageID, Cluster: a.cluster,
		EnrollmentID: enrollmentID, IssuerKeyID: issuerKeyID, NotAfter: issued.NotAfter, CreatedAt: a.now().UTC(),
	}).Error
}

func hashCredential(credential string) string {
	digest := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(digest[:])
}

// kernelCertificateCache keeps the kernel's own certificate and replaces it
// after two thirds of its lifetime.
type kernelCertificateCache struct {
	mu          sync.Mutex
	certificate *tls.Certificate
	renewAfter  time.Time
}

// KernelTLS returns a moduletls.Source for the kernel: a certificate with the
// kernel's SPIFFE identity, issued in memory by the current CA (its key never
// leaves the process), and the trust bundle, refreshed every refresh.
func (a *Authority) KernelTLS(ctx context.Context, refresh time.Duration) (moduletls.Source, error) {
	identity, err := moduletls.Kernel(a.cluster)
	if err != nil {
		return moduletls.Source{}, err
	}
	if refresh <= 0 {
		refresh = time.Minute
	}
	cache := &kernelCertificateCache{}
	roots := &rootsCache{load: func() (*x509.CertPool, error) { return a.Roots(context.WithoutCancel(ctx)) }, refresh: refresh, now: a.now}
	if _, err := roots.get(); err != nil {
		return moduletls.Source{}, err
	}
	certificate := func() (*tls.Certificate, error) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.certificate != nil && a.now().Before(cache.renewAfter) {
			return cache.certificate, nil
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		var issued Issued
		err = a.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
			var signErr error
			issued, _, signErr = a.sign(ctx, tx, identity, &key.PublicKey)
			return signErr
		})
		if err != nil {
			return nil, err
		}
		leaf, err := x509.ParseCertificate(issued.CertificateDER)
		if err != nil {
			return nil, err
		}
		cache.certificate = &tls.Certificate{Certificate: [][]byte{issued.CertificateDER}, PrivateKey: key, Leaf: leaf}
		cache.renewAfter = issued.RenewAfter
		return cache.certificate, nil
	}
	if _, err := certificate(); err != nil {
		return moduletls.Source{}, err
	}
	return moduletls.Source{Certificate: certificate, Roots: roots.pool}, nil
}

// rootsCache reloads the trust bundle at most every refresh and keeps the
// last good pool when a reload fails.
type rootsCache struct {
	mu       sync.Mutex
	load     func() (*x509.CertPool, error)
	refresh  time.Duration
	now      func() time.Time
	current  *x509.CertPool
	loadedAt time.Time
}

func (c *rootsCache) get() (*x509.CertPool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.now().Sub(c.loadedAt) < c.refresh {
		return c.current, nil
	}
	pool, err := c.load()
	if err != nil {
		if c.current != nil {
			return c.current, nil
		}
		return nil, err
	}
	c.current, c.loadedAt = pool, c.now()
	return pool, nil
}

func (c *rootsCache) pool() *x509.CertPool {
	pool, _ := c.get()
	return pool
}
