package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// Every start prepares the schema with the ordered migrations, in every
// environment: an empty database gets the schema and the migration
// records, the next start changes nothing, and a database an earlier
// Control prepared (without the migration tables) is adopted.
func TestBootstrapDatabaseMigratesTheSchema(t *testing.T) {
	bootstrapDatabases(t, func(t *testing.T, cfg *config.Config) {
		ctx := context.Background()
		savedDir := forwardLegacyDataDir
		forwardLegacyDataDir = t.TempDir()
		t.Cleanup(func() { forwardLegacyDataDir = savedDir })

		for _, env := range []string{"production", "development"} {
			cfg.Env = env
			require.NoError(t, bootstrapDatabase(ctx, cfg), env)
		}
		db := database.Get()
		meta, err := migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, version, meta[migrations.MetaProgramVersion])
		assert.Equal(t, "fresh", meta[migrations.MetaOrigin])
		known, err := migrations.Steps()
		require.NoError(t, err)
		assert.Equal(t, strconv.Itoa(len(known)), meta[migrations.MetaSchemaVersion], "every migration is applied")
		for _, value := range []any{&model.User{}, &model.Plan{}, &model.Ticket{}, &model.NodeGroup{}, &model.AgentDiagnosticTask{}} {
			assert.True(t, db.Migrator().HasTable(value), "%T", value)
		}
		var scopes, splitRows int64
		require.NoError(t, db.Model(&model.ServiceScope{}).Count(&scopes).Error)
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Count(&splitRows).Error)
		assert.EqualValues(t, 4, scopes, "the kernel defaults are seeded")
		assert.EqualValues(t, len(nodesecrets.Tables()), splitRows)
		var views int64
		if db.Name() == "postgres" {
			require.NoError(t, db.Raw("SELECT count(*) FROM pg_views WHERE schemaname = current_schema() AND viewname = 'kapi_user_directory_v1'").Scan(&views).Error)
		} else {
			require.NoError(t, db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'view' AND name = 'kapi_user_directory_v1'").Scan(&views).Error)
		}
		assert.EqualValues(t, 1, views, "the kernel API views are created after the migrations")

		// A database from before the migrations: the same tables, no
		// migration records, and not the tables later migrations add
		// (0003_acme_cache).
		require.NoError(t, db.Migrator().DropTable(migrations.MigrationTable, migrations.MetaTable, &model.ACMECacheEntry{}))
		require.NoError(t, bootstrapDatabase(ctx, cfg))
		meta, err = migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, "adopted", meta[migrations.MetaOrigin])
	})
}

// An older Control refuses to start on a database a newer one migrated,
// and says what to do.
func TestBootstrapDatabaseRefusesADatabaseANewerControlMigrated(t *testing.T) {
	bootstrapDatabases(t, func(t *testing.T, cfg *config.Config) {
		ctx := context.Background()
		savedDir := forwardLegacyDataDir
		forwardLegacyDataDir = t.TempDir()
		t.Cleanup(func() { forwardLegacyDataDir = savedDir })
		require.NoError(t, bootstrapDatabase(ctx, cfg))
		require.NoError(t, database.Get().Create(&migrations.MigrationRecord{
			Version: 900, Name: "future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "9.0.0",
		}).Error)

		err := bootstrapDatabase(ctx, cfg)
		var refused *migrations.RefusedError
		require.ErrorAs(t, err, &refused)
		assert.Contains(t, err.Error(), "migrated by a newer AnixOps Control")
		assert.Contains(t, err.Error(), "Control 9.0.0")
		assert.Contains(t, err.Error(), "restore the backup")
	})
}

func TestTakeMigrateCommandStripsOnlyALeadingMigrate(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })

	os.Args = []string{"anix-control", "migrate", "-config", "/app/config.yaml"}
	if !takeMigrateCommand() {
		t.Fatal("leading migrate must be recognized")
	}
	if strings.Join(os.Args, " ") != "anix-control -config /app/config.yaml" {
		t.Fatalf("args after strip = %q", os.Args)
	}

	os.Args = []string{"anix-control", "-config", "migrate"}
	if takeMigrateCommand() {
		t.Fatal("migrate is only a command in the first position")
	}
}

func TestRemoveStaleArtifactCopiesKeepsOtherEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".anix-package-123", ".anix-package-abc", "keep-dir"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".anix-package-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	removeStaleArtifactCopies(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != ".anix-package-file,keep-dir" {
		t.Fatalf("remaining entries = %v", names)
	}
}

// The bootstrap lock serializes Control processes on PostgreSQL. Run with
// POSTGRES_TEST_DSN (a database whose name contains "test").
func TestPostgresBootstrapLockIsExclusiveAcrossSessions(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	open := func() *gorm.DB {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	first, second := open(), open()

	release, err := acquireBootstrapLock(context.Background(), first, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		releaseSecond, err := acquireBootstrapLock(context.Background(), second, "postgresql")
		if err != nil {
			t.Error(err)
			close(acquired)
			return
		}
		acquired <- releaseSecond
	}()
	select {
	case <-acquired:
		t.Fatal("a second process acquired the bootstrap lock while it was held")
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case releaseSecond := <-acquired:
		if releaseSecond != nil {
			releaseSecond()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bootstrap lock was not handed over after release")
	}
}

func TestBootstrapLockIsANoOpOnSQLite(t *testing.T) {
	release, err := acquireBootstrapLock(context.Background(), openSQLite(t), "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	release()
}
