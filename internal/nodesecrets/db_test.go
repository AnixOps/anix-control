package nodesecrets

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSNEnvironment enables the PostgreSQL runs.
const postgresDSNEnvironment = "ANIX_TEST_POSTGRES_DSN"

// legacyModels are the tables the split reads.
var legacyModels = []any{
	&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
	&model.ForwardNode{}, &model.ForwardCleanAgent{},
}

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on a throwaway PostgreSQL schema, each with the legacy tables and the
// split tables.
func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db := openSQLite(t)
		prepareDatabase(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openPostgres(t)
		prepareDatabase(t, db)
		body(t, db)
	})
}

func prepareDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(legacyModels...))
	require.NoError(t, EnsureSchema(db))
}

// testConfig leaves out the foreign keys of the legacy models (a peer's
// user, a protocol's node): the split reads the rows alone.
func testConfig() *gorm.Config {
	return &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "split.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)"), testConfig())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSNEnvironment))
	if base == "" {
		t.Skip(postgresDSNEnvironment + " is not set")
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
	schema := "nodesecrets_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), testConfig())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
