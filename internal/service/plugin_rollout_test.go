package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPackageRolloutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
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

func seedRollbackPair(t *testing.T, db *gorm.DB, packageID string) (model.PackageRouteGeneration, model.PackageRouteGeneration) {
	t.Helper()
	previous := seedGeneration(t, db, packageID, 3, 100, "validated")
	current := seedGeneration(t, db, packageID, 4, 1, "validated")
	current.PreviousID = &previous.ID
	require.NoError(t, db.Save(&current).Error)
	return current, previous
}

func attachRouteBackup(t *testing.T, db *gorm.DB, route *model.PackageRouteGeneration) {
	t.Helper()
	require.NotNil(t, route.MigrationRunID)
	backup := model.PackageBackupReference{
		MigrationRunID: *route.MigrationRunID, PackageID: route.PackageID, PackageVersion: route.Version,
		Generation: route.Generation, Reference: fmt.Sprintf("backup://%s/%d", route.PackageID, route.Generation),
	}
	require.NoError(t, db.Create(&backup).Error)
	route.BackupReferenceID = &backup.ID
	require.NoError(t, db.Model(&model.PackageRouteGeneration{}).Where("id = ?", route.ID).Update("backup_reference_id", backup.ID).Error)
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

func TestAdvancePackageCohortRequiresVerifiedPriorGeneration(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	generation := seedGeneration(t, db, "order", 4, 5, "validated")

	advanced, err := AdvancePackageCohort(db, generation.ID, 25)
	require.NoError(t, err)
	require.Equal(t, uint64(5), advanced.Generation)
	require.Equal(t, uint8(25), advanced.CohortPercent)
	require.Equal(t, generation.ID, *advanced.PreviousID)

	_, err = AdvancePackageCohort(db, generation.ID, 100)
	require.ErrorIs(t, err, ErrCohortTransition)
}

func TestAdvancePackageCohortIsIdempotentAfterInterruption(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	generation := seedGeneration(t, db, "order", 4, 5, "validated")

	first, err := AdvancePackageCohort(db, generation.ID, 25)
	require.NoError(t, err)
	retried, err := AdvancePackageCohort(db, generation.ID, 25)
	require.NoError(t, err)
	require.Equal(t, first.ID, retried.ID)

	var count int64
	require.NoError(t, db.Model(&model.PackageRouteGeneration{}).Where("package_id = ?", "order").Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestAdvancePackageCohortAllowsCanonicalSequence(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	current := seedGeneration(t, db, "order", 4, 1, "validated")

	for _, target := range []uint8{5, 25, 100} {
		advanced, err := AdvancePackageCohort(db, current.ID, target)
		require.NoError(t, err)
		current = *advanced
		require.Equal(t, target, current.CohortPercent)
	}

	_, err := AdvancePackageCohort(db, current.ID, 100)
	require.ErrorIs(t, err, ErrCohortTransition)
}

func TestRollbackRestoresPreviousVerifiedGeneration(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	current, previous := seedRollbackPair(t, db, "payment")
	attachRouteBackup(t, db, &current)

	restored, err := RollbackPackageGeneration(db, current.ID, "host_crash")
	require.NoError(t, err)
	require.Equal(t, previous.ID, restored.ID)

	var rolledBack model.PackageRouteGeneration
	require.NoError(t, db.First(&rolledBack, current.ID).Error)
	require.Equal(t, "host_crash", rolledBack.RollbackReason)
	require.NotNil(t, rolledBack.RolledBackAt)
}

func TestRollbackRejectsMissingBackupAudit(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	current, _ := seedRollbackPair(t, db, "payment")

	_, err := RollbackPackageGeneration(db, current.ID, "host_crash")
	require.ErrorIs(t, err, ErrRollbackPrecondition)

	var unchanged model.PackageRouteGeneration
	require.NoError(t, db.First(&unchanged, current.ID).Error)
	require.Nil(t, unchanged.RolledBackAt)
}

func TestRollbackRetryRejectsRetiredPredecessor(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	first := seedGeneration(t, db, "payment", 1, 1, "validated")
	second := seedGeneration(t, db, "payment", 2, 5, "validated")
	second.PreviousID = &first.ID
	require.NoError(t, db.Save(&second).Error)
	third := seedGeneration(t, db, "payment", 3, 25, "validated")
	third.PreviousID = &second.ID
	require.NoError(t, db.Save(&third).Error)
	attachRouteBackup(t, db, &second)
	attachRouteBackup(t, db, &third)

	_, err := RollbackPackageGeneration(db, third.ID, "host_crash")
	require.NoError(t, err)
	_, err = RollbackPackageGeneration(db, second.ID, "host_crash")
	require.NoError(t, err)

	_, err = RollbackPackageGeneration(db, third.ID, "host_crash")
	require.ErrorIs(t, err, ErrRollbackPrecondition)
}

func TestRollbackLeavesGenerationUnchangedWhenBackupAuditIsMissing(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	current, _ := seedRollbackPair(t, db, "payment")
	missingBackupID := uint(999)
	require.NoError(t, db.Model(&model.PackageRouteGeneration{}).Where("id = ?", current.ID).Update("backup_reference_id", missingBackupID).Error)

	_, err := RollbackPackageGeneration(db, current.ID, "host_crash")
	require.ErrorIs(t, err, ErrRollbackPrecondition)

	var unchanged model.PackageRouteGeneration
	require.NoError(t, db.First(&unchanged, current.ID).Error)
	require.Nil(t, unchanged.RolledBackAt)
	require.Empty(t, unchanged.RollbackReason)
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

func TestRecordPackageValidationRejectsStalePredecessor(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	predecessor := seedGeneration(t, db, "order", 4, 1, "validated")
	started, err := BeginPackageMigration(db, model.PackageMigrationRun{
		PackageID: "order", PackageVersion: "4.0.0", Generation: 6,
		MigrationID: "order-v4", MigrationChecksum: "checksum-v4",
		BeforeSchemaVersion: "3", AfterSchemaVersion: "4", BackupReference: "backup://order/v3",
	})
	require.NoError(t, err)
	_, err = AdvancePackageCohort(db, predecessor.ID, 5)
	require.NoError(t, err)

	_, err = migratePackageHost(context.Background(), db, testMigrationManager{output: pluginhost.MigrationOutput{
		Checkpoint: "opaque-complete", ValidationDigest: "host-validation", Complete: true,
		HealthLeaseID: "lease-6", HealthGeneration: 6,
	}}, started.ID, pluginhost.MigrationInput{PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 6})
	require.NoError(t, err)
	_, err = RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: started.ID})
	require.ErrorIs(t, err, ErrValidationPrecondition)
}
