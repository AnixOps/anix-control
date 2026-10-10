package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
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

// Control starts after `forward legacy drop` (F5c): the flux tables are
// gone, and neither `migrate`, the server nor `forward legacy status` may
// touch them. Before the fix every one of them stopped on "no such table:
// v2_forward" (PostgreSQL: relation "v2_forward" does not exist).
func TestBootstrapDatabaseAfterForwardLegacyDrop(t *testing.T) {
	bootstrapDatabases(t, func(t *testing.T, cfg *config.Config) {
		ctx := context.Background()
		savedDir, savedConfig := forwardLegacyDataDir, config.Get()
		forwardLegacyDataDir = t.TempDir()
		t.Cleanup(func() { forwardLegacyDataDir = savedDir; config.Set(savedConfig) })

		// A v4.1 database: the first start creates the flux tables.
		require.NoError(t, bootstrapDatabase(ctx, cfg, "development"))
		db := database.Get()
		for _, table := range forwardlegacy.DropTables {
			require.True(t, db.Migrator().HasTable(table), table)
		}
		// Old data, with the retired iptables backend: before the drop the
		// start still normalizes it.
		tunnel := model.ForwardTunnel{Name: "t1", InNodeID: 1, Status: model.ForwardTunnelStatusActive}
		require.NoError(t, db.Create(&tunnel).Error)
		forward := model.Forward{UserID: 1, Name: "f1", TunnelID: tunnel.ID, InPort: 20001, RemoteAddr: "198.51.100.7:443",
			Status: model.ForwardStatusActive, RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible}
		require.NoError(t, db.Create(&forward).Error)
		require.NoError(t, bootstrapDatabase(ctx, cfg, "development"))
		var backend string
		require.NoError(t, db.Model(&model.Forward{}).Where("id = ?", forward.ID).Select("runtime_backend").Scan(&backend).Error)
		assert.Equal(t, model.ForwardRuntimeBackendNftablesAnsible, backend)

		// The upgrade the documented way: archive, then the drop.
		run := func(arguments ...string) (string, error) {
			var output bytes.Buffer
			err := runAdminCommand(ctx, cfg, db, append([]string{"forward", "legacy"}, arguments...), &output)
			return output.String(), err
		}
		_, err := run("archive", "-o", t.TempDir()+"/")
		require.NoError(t, err)
		backup := filepath.Join(t.TempDir(), "anix.dump")
		require.NoError(t, os.WriteFile(backup, []byte("dump"), 0o600))
		_, err = run("drop", "--confirm", forwardlegacy.ConfirmPhrase, "--backup-taken", backup)
		require.NoError(t, err)
		for _, table := range forwardlegacy.DropTables {
			require.False(t, db.Migrator().HasTable(table), table)
		}

		// The old runtime's settings (gost without a NodeX address) no
		// longer gate the start of a Control whose flux tables are gone.
		settings := *cfg
		settings.ForwardRuntime.Backend = model.ForwardRuntimeBackendGost
		settings.ForwardRuntime.NodeX.BaseURL, settings.ForwardRuntime.NodeX.Token = "", ""
		config.Set(&settings)

		// `migrate` and the server start in either environment, again and
		// again, and leave the tables dropped.
		for _, env := range []string{"development", "production", "development"} {
			require.NoError(t, bootstrapDatabase(ctx, cfg, env), env)
		}
		for _, table := range forwardlegacy.DropTables {
			assert.False(t, db.Migrator().HasTable(table), table)
		}
		assert.True(t, db.Migrator().HasTable("v2_forward_node"))

		// `forward legacy status` reports the dropped state.
		output, err := run("status")
		require.NoError(t, err)
		assert.Contains(t, output, "The flux tables were dropped at")
		output, err = run("status", "--json")
		require.NoError(t, err)
		var status struct {
			Drop          *model.ForwardLegacyDrop `json:"drop"`
			Preconditions struct {
				Blockers []string `json:"blockers"`
				Present  []string `json:"present"`
			} `json:"preconditions"`
		}
		require.NoError(t, json.Unmarshal([]byte(output), &status))
		require.NotNil(t, status.Drop)
		assert.NotEmpty(t, status.Drop.ArchivePath)
		assert.Empty(t, status.Preconditions.Present)
		require.Len(t, status.Preconditions.Blockers, 1)
		assert.Contains(t, status.Preconditions.Blockers[0], "already dropped")

		// A second drop refuses instead of failing on the missing tables.
		_, err = run("drop", "--confirm", forwardlegacy.ConfirmPhrase, "--backup-taken", backup)
		var refused *forwardlegacy.RefusedError
		require.ErrorAs(t, err, &refused)
		assert.Contains(t, refused.Error(), "already dropped")
	})
}
