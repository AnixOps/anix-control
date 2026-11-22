package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPackageRolloutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, MigrateKernelModelsForTest(db))
	return db
}

func seedGeneration(t *testing.T, db *gorm.DB, packageID string, generation uint64, cohort uint8, state string) model.PackageRouteGeneration {
	t.Helper()
	run := model.PackageMigrationRun{
		PackageID: packageID, PackageVersion: "4.0.0", Generation: generation,
		MigrationID: fmt.Sprintf("seed-%d", generation), MigrationChecksum: "seed-checksum",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", State: "completed", Complete: true,
	}
	require.NoError(t, db.Create(&run).Error)
	validation := model.PackageValidationResult{
		MigrationRunID: run.ID, PackageID: packageID, PackageVersion: "4.0.0", Generation: generation,
		ValidationDigest: "opaque-validation-" + packageID, State: "validated",
	}
	require.NoError(t, db.Create(&validation).Error)
	route := model.PackageRouteGeneration{
		PackageID: packageID, Version: "4.0.0", Generation: generation,
		CohortPercent: cohort, State: state, MigrationRunID: &run.ID, ValidationResultID: &validation.ID,
	}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Model(&model.PackageValidationResult{}).Where("id = ?", validation.ID).Update("route_generation_id", route.ID).Error)
	validation.RouteGenerationID = &route.ID
	return route
}

type testMigrationManager struct {
	output pluginhost.MigrationOutput
}

func (m testMigrationManager) Migrate(ctx context.Context, _ pluginhost.MigrationInput, recorder pluginhost.MigrationCheckpointRecorder) (pluginhost.MigrationOutput, error) {
	reply := m.output
	reply.HealthLeaseID = ""
	reply.HealthGeneration = 0
	if err := recorder(ctx, reply); err != nil {
		return pluginhost.MigrationOutput{}, err
	}
	return m.output, nil
}

func TestBeginPackageMigrationResumesCheckpointIdempotently(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	input := model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", BackupReference: "backup://order/v3",
	}

	started, err := BeginPackageMigration(db, input)
	require.NoError(t, err)
	checkpointed, err := RecordMigrationCheckpoint(db, started.ID, PackageMigrationCheckpoint{OpaqueCheckpoint: "opaque-step-1"})
	require.NoError(t, err)
	require.Equal(t, "opaque-step-1", checkpointed.OpaqueCheckpoint)

	retried, err := BeginPackageMigration(db, input)
	require.NoError(t, err)
	require.Equal(t, started.ID, retried.ID)
	require.Equal(t, "opaque-step-1", retried.OpaqueCheckpoint)
}

func TestBeginPackageMigrationDerivesPreviousGenerationFromKernelState(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	untrustedPrevious := uint(999)
	run, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", PreviousGenerationID: &untrustedPrevious,
	})
	require.NoError(t, err)
	require.Nil(t, run.PreviousGenerationID)
}

func TestRecordPackageValidationRequiresCompletedMigration(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4",
	})
	require.NoError(t, err)

	_, err = RecordPackageValidation(db, model.PackageValidationResult{
		MigrationRunID: started.ID, ValidationDigest: "opaque-validation",
	})
	require.ErrorIs(t, err, ErrValidationPrecondition)
}

func TestRecordPackageValidationRequiresPersistedHostDigest(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4",
	})
	require.NoError(t, err)
	_, err = migratePackageHost(context.Background(), db, testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-7", HealthGeneration: 7,
	}}, started.ID, pluginhost.MigrationInput{PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 7})
	require.NoError(t, err)

	_, err = RecordPackageValidation(db, model.PackageValidationResult{
		MigrationRunID: started.ID, ValidationDigest: "caller-substituted-validation",
	})
	require.ErrorIs(t, err, ErrValidationPrecondition)

	validated, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	require.Equal(t, "host-validation", validated.ValidationDigest)
}

func TestRecordPackageValidationRequiresHealthLeaseFence(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4",
	})
	require.NoError(t, err)
	checkpoint := PackageMigrationCheckpoint{OpaqueCheckpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true}
	_, err = RecordMigrationCheckpoint(db, started.ID, checkpoint)
	require.NoError(t, err)

	_, err = RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.ErrorIs(t, err, ErrValidationPrecondition)

	var count int64
	require.NoError(t, db.Model(&model.PackageRouteGeneration{}).Where("package_id = ?", "order").Count(&count).Error)
	require.Zero(t, count)
}

func TestMigratePackageHostRecordsHealthFenceAfterManagerSuccess(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4",
	})
	require.NoError(t, err)
	manager := testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-7", HealthGeneration: 7,
	}}
	_, err = migratePackageHost(context.Background(), db, manager, started.ID, pluginhost.MigrationInput{
		PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 7,
	})
	require.NoError(t, err)

	validated, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	require.NotNil(t, validated.RouteGenerationID)
}

func TestRecordPackageValidationReturnsExistingRootValidationBeforeLineageCheck(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 7,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4",
	})
	require.NoError(t, err)
	_, err = migratePackageHost(context.Background(), db, testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-7", HealthGeneration: 7,
	}}, started.ID, pluginhost.MigrationInput{PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 7})
	require.NoError(t, err)

	first, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	retried, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	require.Equal(t, first.ID, retried.ID)
	require.Equal(t, first.RouteGenerationID, retried.RouteGenerationID)
}

func TestRecordPackageValidationReturnsExistingSuccessorValidationBeforeLineageCheck(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	seedGeneration(t, db, "order", 4, 100, model.PackageRouteGenerationStateValidated)
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 6,
		MigrationID: "order-v6", MigrationChecksum: "checksum-v6",
		BeforeSchemaVersion: "4", AfterSchemaVersion: "6", BackupReference: "backup://order/v4",
	})
	require.NoError(t, err)
	_, err = migratePackageHost(context.Background(), db, testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-6", HealthGeneration: 6,
	}}, started.ID, pluginhost.MigrationInput{PackageID: "order", Version: "4.0.0", MigrationID: "order-v6", Generation: 6})
	require.NoError(t, err)

	first, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	retried, err := RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.NoError(t, err)
	require.Equal(t, first.ID, retried.ID)
	require.Equal(t, first.RouteGenerationID, retried.RouteGenerationID)
}

func TestRecordPackageValidationRejectsStalePredecessor(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	seedGeneration(t, db, "order", 4, 1, "validated")
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 6,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", BackupReference: "backup://order/v3",
	})
	require.NoError(t, err)
	// A newer generation appears after the migration started.
	seedGeneration(t, db, "order", 5, 5, "validated")

	_, err = migratePackageHost(context.Background(), db, testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-6", HealthGeneration: 6,
	}}, started.ID, pluginhost.MigrationInput{PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 6})
	require.NoError(t, err)
	_, err = RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.ErrorIs(t, err, ErrValidationPrecondition)
}
