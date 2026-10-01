package subscriber

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgresDB opens a throwaway schema of the ANIX_TEST_POSTGRES_DSN
// database with the subscriber tables, or skips.
func openPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	schema := "subscriber_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })

	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscriptionGroup{}, &model.SubscriberRequest{}, &model.SubscriberChange{}))
	return db
}

// A change log id is taken when its row is inserted, but rows become
// visible when their transactions commit. If a later id could commit first,
// a consumer would read it, move its cursor past the earlier id, and never
// see that row. Writers of the change log commit in id order instead: a
// second writer waits for the first.
func TestPostgresChangeLogCommitsInCursorOrder(t *testing.T) {
	db := openPostgresDB(t)
	now := time.Now()

	first := db.Begin()
	require.NoError(t, first.Error)
	// A failed test must not leave the transaction open: dropping the
	// schema would wait for it.
	t.Cleanup(func() { first.Rollback() })
	require.NoError(t, RecordChangesTx(first, []uint{1}, false, now))

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- db.Transaction(func(tx *gorm.DB) error {
			return RecordChangesTx(tx, []uint{2}, false, now)
		})
	}()

	// While the first writer is open, a consumer sees nothing it could skip
	// past: the second writer has not committed ahead of it.
	var cursor uint64
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		changes, resync, err := ChangesAfter(db, cursor, 100)
		require.NoError(t, err)
		require.False(t, resync)
		require.Empty(t, changes, "a change committed ahead of an earlier, open one")
		select {
		case err := <-secondDone:
			t.Fatalf("the second writer committed while the first was open (err %v)", err)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}

	require.NoError(t, first.Commit().Error)
	require.NoError(t, <-secondDone)

	var seen []uint
	for {
		changes, _, err := ChangesAfter(db, cursor, 1)
		require.NoError(t, err)
		if len(changes) == 0 {
			break
		}
		seen = append(seen, changes[0].UserID)
		cursor = changes[0].ID
	}
	require.Equal(t, []uint{1, 2}, seen)
}
