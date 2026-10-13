package migrations_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// opener opens a new, empty database of one dialect; every call is a
// separate database.
type opener func(t *testing.T) *gorm.DB

// forEachDialect runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN names
// a database whose name contains "test", on PostgreSQL, where every opened
// database is a throwaway schema.
func forEachDialect(t *testing.T, body func(t *testing.T, open opener)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, openSQLite) })
	t.Run("postgres", func(t *testing.T) {
		base := postgresDSN(t)
		body(t, func(t *testing.T) *gorm.DB { return openPostgresSchema(t, base) })
	})
}

func quiet() *gorm.Config {
	return &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
}

// openSQLite opens a new SQLite file the way Control does (immediate
// transactions, WAL).
func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_txlock=immediate&_pragma=journal_mode(WAL)"), quiet())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func postgresDSN(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), quiet())
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	defer func() { _ = adminDB.Close() }()
	var name string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&name).Error)
	if !strings.Contains(strings.ToLower(name), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", name)
	}
	return base
}

// openPostgresSchema opens a connection whose search_path is a new schema,
// dropped when the test ends.
func openPostgresSchema(t *testing.T, base string) *gorm.DB {
	t.Helper()
	suffix := make([]byte, 4)
	_, err := rand.Read(suffix)
	require.NoError(t, err)
	schema := "migrations_" + hex.EncodeToString(suffix)
	admin, err := gorm.Open(postgres.Open(base), quiet())
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() {
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		_ = adminDB.Close()
	})
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), quiet())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
