package modulepki

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(base), config)
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "modulepki_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), config)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
	return db
}

// A renewal that is in flight when its enrollment is revoked must not leave a
// live certificate behind. PostgreSQL reads each statement on its own
// snapshot, so the renewal's early reads say "not revoked" while the
// revocation commits; the new certificate then has to be caught by the
// revocation or refused by the renewal.
func TestRenewInFlightDuringRevocationLeavesNoLiveCertificate(t *testing.T) {
	db := openPostgres(t)
	clock := &testClock{now: time.Unix(1_790_000_000, 0)}
	authority := newTestAuthority(t, db, clock)
	ctx := context.Background()
	credential, enrollment, err := authority.CreateEnrollment(ctx, EnrollmentRequest{PackageID: "identity-platform", TTL: 30 * 24 * time.Hour, Reusable: true})
	require.NoError(t, err)
	_, csr := newCSR(t)
	first, err := authority.Enroll(ctx, credential, "identity-platform", "prod", csr)
	require.NoError(t, err)
	peer := verifyIssued(t, authority, first)

	// Hold the renewal just before it records its certificate.
	var armed atomic.Bool
	paused := make(chan struct{})
	release := make(chan struct{})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:pause_certificate", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (model.ModuleCertificate{}).TableName() && armed.CompareAndSwap(true, false) {
			close(paused)
			<-release
		}
	}))
	armed.Store(true)

	renewed := make(chan error, 1)
	go func() {
		_, err := authority.Renew(ctx, peer, csr)
		renewed <- err
	}()
	select {
	case <-paused:
	case <-time.After(10 * time.Second):
		t.Fatal("the renewal never reached its insert")
	}

	revoked := make(chan error, 1)
	go func() { revoked <- authority.RevokeEnrollment(ctx, enrollment.ID) }()
	// Without a lock the revocation finishes while the renewal is held; with
	// one it waits for the renewal. Either way it must be done once released.
	select {
	case err := <-revoked:
		require.NoError(t, err)
		revoked = nil
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-func() chan error {
		if revoked == nil {
			done := make(chan error, 1)
			done <- nil
			return done
		}
		return revoked
	}())
	<-renewed

	var live int64
	require.NoError(t, db.Model(&model.ModuleCertificate{}).Where("enrollment_id = ? AND revoked_at IS NULL", enrollment.ID).Count(&live).Error)
	require.Zero(t, live, "a revoked enrollment has no live certificate, whatever interleaving the renewal had")
}
