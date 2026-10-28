package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// bootstrapDatabases runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN
// names a database whose name contains "test", on a throwaway PostgreSQL
// schema. Either way the process database (database.Get) is the one under
// test, as it is for `run`, and cfg describes it.
func bootstrapDatabases(t *testing.T, body func(t *testing.T, cfg *config.Config)) {
	t.Helper()
	use := func(t *testing.T, cfg *config.Config) {
		t.Helper()
		cfg.Database.LogLevel = "silent"
		_ = database.Close()
		require.NoError(t, database.Init(&cfg.Database))
		t.Cleanup(func() { _ = database.Close() })
	}
	t.Run("sqlite", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.Database = config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "control.db")}
		use(t, cfg)
		body(t, cfg)
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
		if base == "" {
			t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
		}
		quiet := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
		admin, err := gorm.Open(postgres.Open(base), quiet)
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
		schema := "bootstrap_dropped_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		cfg := config.Defaults()
		cfg.Database = config.DatabaseConfig{Driver: "postgres", DSN: base + " search_path=" + schema}
		use(t, cfg)
		body(t, cfg)
	})
}
