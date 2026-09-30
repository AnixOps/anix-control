package modulepki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kernel.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newTestAuthority(t *testing.T, db *gorm.DB, clock *testClock) *Authority {
	t.Helper()
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := New(Options{DB: db, Cluster: "prod", KEK: kek, Now: clock.Now})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	return authority
}

func newCSR(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	// A CSR asking for another identity: the kernel must ignore it.
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "evil"}, DNSNames: []string{"kernel.evil.example"},
	}, key)
	require.NoError(t, err)
	return key, der
}

func verifyIssued(t *testing.T, authority *Authority, issued Issued) *x509.Certificate {
	t.Helper()
	identity, leaf, err := moduletls.VerifyChainAt([][]byte{issued.CertificateDER}, mustRoots(t, authority), authority.now())
	require.NoError(t, err)
	require.Equal(t, issued.Identity, identity)
	return leaf
}

func mustRoots(t *testing.T, authority *Authority) *x509.CertPool {
	t.Helper()
	roots, err := authority.Roots(context.Background())
	require.NoError(t, err)
	return roots
}

func TestParseKEK(t *testing.T) {
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index)
	}
	for _, raw := range []string{"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"} {
		parsed, err := ParseKEK(raw)
		require.NoError(t, err)
		require.Equal(t, key, parsed)
	}
	_, err := ParseKEK("too-short")
	require.Error(t, err)
}

func TestEnsureCreatesOneSealedCA(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	require.NoError(t, authority.Ensure(context.Background()))

	var rows []model.ServiceCA
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, model.ServiceCAStateCurrent, rows[0].State)
	require.NotContains(t, rows[0].SealedKey, "PRIVATE")
	ca, err := parseCertificatePEM(rows[0].CertificatePEM)
	require.NoError(t, err)
	require.True(t, ca.IsCA)
	require.Equal(t, []string{moduletls.TrustDomain}, ca.PermittedURIDomains)

	// Another key-encryption key cannot unseal it.
	other := make([]byte, 32)
	wrong, err := New(Options{DB: db, Cluster: "prod", KEK: other, Now: clock.Now})
	require.NoError(t, err)
	_, _, err = wrong.sign(context.Background(), db, Identity(t, "identity-platform"), mustPublicKey(t))
	require.ErrorContains(t, err, "wrong key-encryption key")
}

func Identity(t *testing.T, packageID string) moduletls.Identity {
	t.Helper()
	identity, err := moduletls.Module("prod", packageID)
	require.NoError(t, err)
	return identity
}

func mustPublicKey(t *testing.T) *ecdsa.PublicKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &key.PublicKey
}

func TestOneTimeEnrollmentIssuesOnlyThePackageIdentity(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, enrollment, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Hour, CreatedBy: 1})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(credential, "anixenr_"))
	require.NotContains(t, enrollment.CredentialHash, credential)

	_, csr := newCSR(t)
	issued, err := authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.NoError(t, err)
	leaf := verifyIssued(t, authority, issued)
	require.Empty(t, leaf.DNSNames, "requested SANs are ignored")
	require.Equal(t, "spiffe://anixops/prod/module/identity-platform", leaf.URIs[0].String())
	require.Equal(t, clock.Now().Add(DefaultCertificateLifetime).Unix(), leaf.NotAfter.Unix())
	require.Equal(t, clock.Now().Add(16*time.Hour).Unix(), issued.RenewAfter.Unix())
	var record model.ModuleCertificate
	require.NoError(t, db.First(&record, "serial = ?", issued.Serial).Error)
	require.Equal(t, enrollment.ID, record.EnrollmentID)

	_, err = authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.ErrorIs(t, err, ErrEnrollmentRejected, "a one-time credential works once")
}

func TestEnrollmentRejectsWithoutConsuming(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, _, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Hour})
	require.NoError(t, err)
	_, csr := newCSR(t)

	for name, attempt := range map[string]func() error{
		"other package": func() error { _, err := authority.Enroll(ctx, credential, "ticket", "prod", csr); return err },
		"other cluster": func() error {
			_, err := authority.Enroll(ctx, credential, "identity-platform", "staging", csr)
			return err
		},
		"unknown": func() error {
			_, err := authority.Enroll(ctx, "anixenr_unknown", "identity-platform", "prod", csr)
			return err
		},
		"no prefix": func() error { _, err := authority.Enroll(ctx, "guess", "identity-platform", "prod", csr); return err },
	} {
		require.ErrorIs(t, attempt(), ErrEnrollmentRejected, name)
	}
	_, err = authority.Enroll(ctx, credential, "identity-platform", "prod", []byte("not a csr"))
	require.ErrorIs(t, err, ErrInvalidRequest)

	_, err = authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.NoError(t, err, "rejected attempts did not consume the credential")

	expiring, _, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Minute})
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	_, err = authority.Enroll(ctx, expiring, "identity-platform", "prod", csr)
	require.ErrorIs(t, err, ErrEnrollmentRejected, "expired")

	_, _, err = authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: 8 * 24 * time.Hour})
	require.Error(t, err, "one-time credentials live at most a week")
	_, _, err = authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "Bad_ID", TTL: time.Hour})
	require.Error(t, err)
}

func TestOneTimeEnrollmentRaceHasOneWinner(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, _, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: time.Hour})
	require.NoError(t, err)

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, csr := newCSR(t)
			if _, err := authority.Enroll(ctx, credential, "identity-platform", "prod", csr); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	var count int64
	require.NoError(t, db.Model(&model.ModuleCertificate{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestReusableEnrollmentRenewalAndRevocation(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, enrollment, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: 30 * 24 * time.Hour, Reusable: true})
	require.NoError(t, err)

	_, csr := newCSR(t)
	first, err := authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.NoError(t, err)
	_, err = authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.NoError(t, err, "a restarted pod enrolls again")

	peer := verifyIssued(t, authority, first)
	renewed, err := authority.Renew(ctx, peer, csr)
	require.NoError(t, err)
	verifyIssued(t, authority, renewed)
	require.NotEqual(t, first.Serial, renewed.Serial)

	kernelSource, err := authority.KernelTLS(ctx, time.Minute)
	require.NoError(t, err)
	kernelCertificate, err := kernelSource.Certificate()
	require.NoError(t, err)
	_, err = authority.Renew(ctx, kernelCertificate.Leaf, csr)
	require.ErrorIs(t, err, moduletls.ErrUnexpectedPeer, "the kernel identity cannot renew as a module")

	require.NoError(t, authority.RevokeEnrollment(ctx, enrollment.ID))
	_, err = authority.Renew(ctx, peer, csr)
	require.ErrorIs(t, err, ErrCertificateRevoked)
	revoked, err := authority.IsRevoked(ctx, renewed.Serial)
	require.NoError(t, err)
	require.True(t, revoked, "certificates issued through a revoked enrollment are revoked")
	_, err = authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.ErrorIs(t, err, ErrEnrollmentRejected)
	require.ErrorIs(t, authority.RevokeEnrollment(ctx, "missing"), gorm.ErrRecordNotFound)
	require.NoError(t, authority.RevokeEnrollment(ctx, enrollment.ID), "revoking twice is harmless")
}

func TestRotationOverlapsTrust(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, _, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: 365 * 24 * time.Hour, Reusable: true})
	require.NoError(t, err)
	_, csr := newCSR(t)
	enroll := func() Issued {
		issued, err := authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
		require.NoError(t, err)
		return issued
	}
	old := enroll()
	oldIssuer := verifyIssued(t, authority, old).AuthorityKeyId

	next, err := authority.Rotate(ctx)
	require.NoError(t, err)
	again, err := authority.Rotate(ctx)
	require.NoError(t, err)
	require.Equal(t, next.ID, again.ID, "one next CA at a time")
	bundle, err := authority.TrustBundle(ctx)
	require.NoError(t, err)
	require.Len(t, bundle, 2, "the next CA is trusted before it signs")
	require.Equal(t, oldIssuer, verifyIssued(t, authority, enroll()).AuthorityKeyId, "the current CA keeps signing")

	require.NoError(t, authority.Maintain(ctx))
	require.Equal(t, oldIssuer, verifyIssued(t, authority, enroll()).AuthorityKeyId, "promotion waits one lifetime")

	clock.Advance(DefaultCertificateLifetime + time.Second)
	lastOld := enroll()
	require.Equal(t, oldIssuer, verifyIssued(t, authority, lastOld).AuthorityKeyId)
	require.NoError(t, authority.Maintain(ctx))
	verifyIssued(t, authority, lastOld)
	newer := enroll()
	require.NotEqual(t, oldIssuer, verifyIssued(t, authority, newer).AuthorityKeyId)
	_, _, err = moduletls.VerifyChainAt([][]byte{old.CertificateDER}, mustRoots(t, authority), clock.Now())
	require.Error(t, err, "the old leaf has expired by now")

	clock.Advance(time.Hour)
	verifyIssued(t, authority, lastOld)
	clock.Advance(DefaultCertificateLifetime + 2*time.Minute)
	bundle, err = authority.TrustBundle(ctx)
	require.NoError(t, err)
	require.Len(t, bundle, 1, "the retired CA leaves the bundle once its certificates expired")
}
