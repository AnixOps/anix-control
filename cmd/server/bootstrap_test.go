package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
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

func TestMigrateSchemaCreatesTheFullSchemaOnAnEmptyProductionDatabase(t *testing.T) {
	db := openSQLite(t)
	if err := migrateSchema(db, "production"); err != nil {
		t.Fatal(err)
	}
	for _, value := range schemaModels() {
		if !db.Migrator().HasTable(value) {
			t.Fatalf("table for %T was not created", value)
		}
	}
}

func TestMigrateSchemaNeverAltersExistingProductionTables(t *testing.T) {
	db := openSQLite(t)
	// An existing, older users table without most current columns.
	if err := db.Exec(`CREATE TABLE v2_user (id integer primary key, email text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema(db, "production"); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&model.User{}, "token") {
		t.Fatal("production must not add columns to an existing table")
	}
	if !db.Migrator().HasTable(&model.Plan{}) || !db.Migrator().HasTable(&model.Ticket{}) {
		t.Fatal("missing tables must be created")
	}

	// Development keeps its full AutoMigrate.
	if err := migrateSchema(db, "development"); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&model.User{}, "token") {
		t.Fatal("development AutoMigrate must add missing columns")
	}
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
