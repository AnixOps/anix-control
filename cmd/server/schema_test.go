package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExplicitSchemaMigrationClosesProductionPreflight(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := verifyApplicationSchema(db); err == nil {
		t.Fatal("empty database unexpectedly passed schema preflight")
	}
	if err := migrateApplicationSchema(db); err != nil {
		t.Fatalf("migrateApplicationSchema() error = %v", err)
	}
	if err := verifyApplicationSchema(db); err != nil {
		t.Fatalf("verifyApplicationSchema() after migration error = %v", err)
	}
}

func TestExplicitSchemaMigrationPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("schema_preflight_%d", time.Now().UnixNano())
	if err := admin.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error })
	if err := admin.Exec(`SET search_path TO "` + schemaName + `"`).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateApplicationSchema(admin); err != nil {
		t.Fatalf("migrateApplicationSchema() on PostgreSQL error = %v", err)
	}
	if err := verifyApplicationSchema(admin); err != nil {
		t.Fatalf("verifyApplicationSchema() on PostgreSQL error = %v", err)
	}
}
