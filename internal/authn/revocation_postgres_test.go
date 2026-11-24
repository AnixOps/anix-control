package authn

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Concurrent revocations of one user keep the highest bounds, whatever
// order they commit in, on the production database (CT-010). Before Write
// locked the row, two writers read the same old row and the later commit
// could put an older bound back, or the first two inserts collided.
func TestConcurrentRevocationsKeepTheHighestBounds(t *testing.T) {
	db := openRevocationPostgres(t)
	base := time.Now().Truncate(time.Second)
	const writers = 16
	var wait sync.WaitGroup
	errs := make(chan error, writers)
	for i := 1; i <= writers; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			// Odd writers move the version, even ones not_before.
			r := Revocation{UserID: 7, NotBefore: base.Add(time.Duration(i) * time.Second)}
			if i%2 == 1 {
				r = Revocation{UserID: 7, TokenVersion: uint64(i)}
			}
			errs <- Write(db, r)
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var row model.IdentityRevocation
	require.NoError(t, db.Take(&row, "user_id = ?", 7).Error)
	require.Equal(t, uint64(writers-1), row.TokenVersion)
	require.True(t, row.NotBefore.Equal(base.Add(writers*time.Second)), "not_before %s", row.NotBefore)
	require.NotNil(t, row.IdentityNotBefore)
	require.True(t, row.IdentityNotBefore.Equal(base.Add(writers*time.Second)))
}

// openRevocationPostgres connects to ANIX_TEST_POSTGRES_DSN in a throwaway
// schema with the revocation tables.
func openRevocationPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	schema := "authn_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.IdentityRevocation{}, &model.IdentitySessionRevocation{}))
	return db
}
