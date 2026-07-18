package integration

import (
	"os"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPackageMigrationCohortRollbackSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(packageRolloutModels()...))
	exercisePackageMigrationRollback(t, db)
}

func TestPostgresPackageMigrationRollback(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	models := packageRolloutModels()
	require.NoError(t, db.Migrator().DropTable(models...))
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() { _ = db.Migrator().DropTable(models...) })
	exercisePackageMigrationRollback(t, db)
}

func packageRolloutModels() []any {
	return []any{
		&model.PackageMigrationRun{},
		&model.PackageValidationResult{},
		&model.PackageRouteGeneration{},
		&model.PackageBackupReference{},
	}
}

func exercisePackageMigrationRollback(t *testing.T, db *gorm.DB) {
	t.Helper()
	run, err := service.BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "payment", PackageVersion: "4.0.0", Generation: 8,
		MigrationID: "payment-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", BackupReference: "backup://payment/v3",
	})
	require.NoError(t, err)
	_, err = service.RecordMigrationCheckpoint(db, run.ID, service.PackageMigrationCheckpoint{
		OpaqueCheckpoint: "opaque-complete", ValidationDigest: "opaque-validation", Complete: true,
	})
	require.NoError(t, err)
	validation, err := service.RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: run.ID})
	require.NoError(t, err)
	require.NotNil(t, validation.RouteGenerationID)

	var initial model.PackageRouteGeneration
	require.NoError(t, db.First(&initial, *validation.RouteGenerationID).Error)
	advanced, err := service.AdvancePackageCohort(db, initial.ID, 5)
	require.NoError(t, err)
	require.NotNil(t, advanced.BackupReferenceID)
	restored, err := service.RollbackPackageGeneration(db, advanced.ID, "integration_host_crash")
	require.NoError(t, err)
	require.Equal(t, initial.ID, restored.ID)

	var backup model.PackageBackupReference
	require.NoError(t, db.First(&backup, *advanced.BackupReferenceID).Error)
	require.NotNil(t, backup.RestoredAt)
	require.Equal(t, "integration_host_crash", backup.RestoreReason)
}
