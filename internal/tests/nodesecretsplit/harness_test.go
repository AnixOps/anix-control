// Package nodesecretsplit tests phases P1 and P2 of the node credential
// split (docs/architecture/node-ops-service.md, section 4) through the
// kernel's own writers and readers: every writer of a moved column
// dual-writes in its transaction, backfill and verify agree, every reader
// follows its table's phase and falls back with a metric, and an older
// binary still authenticates every node from the unchanged legacy columns.
// It runs on SQLite and, with ANIX_TEST_POSTGRES_DSN set, on PostgreSQL.
// Every secret is fake.
package nodesecretsplit

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const postgresDSNEnvironment = "ANIX_TEST_POSTGRES_DSN"

// backend is a database the kernel's global connection points at.
type backend struct {
	name string
	db   *gorm.DB
}

// forEachBackend runs body with the kernel's database on SQLite and, when
// ANIX_TEST_POSTGRES_DSN is set, on a throwaway PostgreSQL schema.
func forEachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "control.db")
		body(t, initKernelDatabase(t, "sqlite", &config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"}))
	})
	t.Run("postgres", func(t *testing.T) {
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
		schema := "nodesecretsplit_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		body(t, initKernelDatabase(t, "postgres", &config.DatabaseConfig{Driver: "postgres", DSN: base + " search_path=" + schema, LogLevel: "silent"}))
	})
}

// initKernelDatabase opens the kernel's database and creates what Control
// creates at startup for these tables: the legacy schema, the kernel schema
// and the split's state.
func initKernelDatabase(t *testing.T, name string, cfg *config.DatabaseConfig) backend {
	t.Helper()
	database.Reset()
	require.NoError(t, database.Init(cfg))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
	})
	db := database.Get()
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Plan{}, &model.SubscriptionGroup{},
		&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.ForwardNode{}, &model.ForwardCleanAgent{}, &model.ForwardRuntimeJob{}, &model.ForwardRule{},
		// Phase changes are audited in the operation log.
		&model.OperationLog{},
	))
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	return backend{name: name, db: db}
}
