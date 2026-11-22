package modulepki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v5/internal/model"
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
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}))
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

func signLeaf(t *testing.T, authority *Authority, csr []byte) SignedLeaf {
	t.Helper()
	leaf, err := authority.SignLeaf(context.Background(), authority.db, kernelURI(t), DefaultCertificateLifetime, csr)
	require.NoError(t, err)
	return leaf
}

func verifyAt(certificateDER []byte, roots *x509.CertPool, at time.Time) (*x509.Certificate, error) {
	leaf, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		return nil, err
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: at, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	return leaf, err
}

func verifyLeaf(t *testing.T, authority *Authority, signed SignedLeaf) *x509.Certificate {
	t.Helper()
	leaf, err := verifyAt(signed.CertificateDER, mustRoots(t, authority), authority.now())
	require.NoError(t, err)
	require.Equal(t, []*url.URL{kernelURI(t)}, leaf.URIs)
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
	_, csr := newCSR(t)
	_, err = wrong.SignLeaf(context.Background(), db, kernelURI(t), DefaultCertificateLifetime, csr)
	require.ErrorContains(t, err, "wrong key-encryption key")
}

func kernelURI(t *testing.T) *url.URL {
	t.Helper()
	identity, err := moduletls.Kernel("prod")
	require.NoError(t, err)
	return identity.URL()
}

func TestSignLeafIssuesOnlyTheRequestedIdentity(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	_, csr := newCSR(t)
	leaf := verifyLeaf(t, authority, signLeaf(t, authority, csr))
	require.Empty(t, leaf.DNSNames, "the CSR's names are ignored")
	require.Equal(t, []*url.URL{kernelURI(t)}, leaf.URIs)

	_, err := authority.SignLeaf(context.Background(), db, &url.URL{Scheme: "https", Host: "evil.example"}, DefaultCertificateLifetime, csr)
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = authority.SignLeaf(context.Background(), db, kernelURI(t), time.Minute, csr)
	require.Error(t, err, "lifetimes below the minimum are refused")
}

func TestRotationOverlapsTrust(t *testing.T) {
	db := openTestDB(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	_, csr := newCSR(t)
	enroll := func() SignedLeaf { return signLeaf(t, authority, csr) }
	old := enroll()
	oldIssuer := verifyLeaf(t, authority, old).AuthorityKeyId

	next, err := authority.Rotate(ctx)
	require.NoError(t, err)
	again, err := authority.Rotate(ctx)
	require.NoError(t, err)
	require.Equal(t, next.ID, again.ID, "one next CA at a time")
	bundle, err := authority.TrustBundle(ctx)
	require.NoError(t, err)
	require.Len(t, bundle, 2, "the next CA is trusted before it signs")
	require.Equal(t, oldIssuer, verifyLeaf(t, authority, enroll()).AuthorityKeyId, "the current CA keeps signing")

	require.NoError(t, authority.Maintain(ctx))
	require.Equal(t, oldIssuer, verifyLeaf(t, authority, enroll()).AuthorityKeyId, "promotion waits one lifetime")

	clock.Advance(DefaultCertificateLifetime + time.Second)
	lastOld := enroll()
	require.Equal(t, oldIssuer, verifyLeaf(t, authority, lastOld).AuthorityKeyId)
	require.NoError(t, authority.Maintain(ctx))
	verifyLeaf(t, authority, lastOld)
	newer := enroll()
	require.NotEqual(t, oldIssuer, verifyLeaf(t, authority, newer).AuthorityKeyId)
	_, err = verifyAt(old.CertificateDER, mustRoots(t, authority), clock.Now())
	require.Error(t, err, "the old leaf has expired by now")

	clock.Advance(time.Hour)
	verifyLeaf(t, authority, lastOld)
	clock.Advance(DefaultCertificateLifetime + 2*time.Minute)
	bundle, err = authority.TrustBundle(ctx)
	require.NoError(t, err)
	require.Len(t, bundle, 1, "the retired CA leaves the bundle once its certificates expired")
}
