package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingMigrationInvoker struct {
	inputs []pluginhost.MigrationInput
	reply  func(pluginhost.MigrationInput) pluginhost.MigrationOutput
}

func (r *recordingMigrationInvoker) Migrate(ctx context.Context, input pluginhost.MigrationInput, recorder pluginhost.MigrationCheckpointRecorder) (pluginhost.MigrationOutput, error) {
	r.inputs = append(r.inputs, input)
	output := r.reply(input)
	checkpoint := output
	checkpoint.HealthLeaseID, checkpoint.HealthGeneration = "", 0
	if err := recorder(ctx, checkpoint); err != nil {
		return pluginhost.MigrationOutput{}, err
	}
	if output.FailureCode != "" {
		return output, pluginhost.ErrHostIncompatible
	}
	return output, nil
}

func storageRef(version, indexDigest string, steps ...pluginhost.MigrationStep) pluginhost.ArtifactRef {
	return pluginhost.ArtifactRef{
		PackageID: "knowledge", Version: version, Storage: true,
		MigrationIndexSHA256: indexDigest, Migrations: steps,
	}
}

func completingInvoker(digest string) *recordingMigrationInvoker {
	return &recordingMigrationInvoker{reply: func(input pluginhost.MigrationInput) pluginhost.MigrationOutput {
		return pluginhost.MigrationOutput{
			Checkpoint: input.MigrationID, ValidationDigest: digest, Complete: true,
			HealthLeaseID: "lease", HealthGeneration: input.Generation,
		}
	}}
}

func TestPackageHostMigratorRecordsOneRunPerGeneration(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	steps := []pluginhost.MigrationStep{
		{ID: "001_notes", Path: "migrations/001_notes.sql", SHA256: strings.Repeat("a", 64)},
		{ID: "002_seed", Path: "migrations/002_seed.sql", SHA256: strings.Repeat("b", 64)},
	}
	ref := storageRef("4.1.0", strings.Repeat("c", 64), steps...)
	invoker := completingInvoker(PackageMigrationStepsDigest(steps))
	now := time.Unix(1_790_000_000, 0)
	migrator := PackageHostMigrator{DB: db, invoker: invoker, Now: func() time.Time { return now }}
	ctx := context.Background()

	require.NoError(t, migrator.MigrateStartedHost(ctx, ref, 3))
	require.Len(t, invoker.inputs, 1)
	assert.Equal(t, pluginhost.MigrationInput{
		PackageID: "knowledge", Version: "4.1.0", MigrationID: "index." + strings.Repeat("c", 32), Generation: 3,
	}, invoker.inputs[0])
	var run model.PackageMigrationRun
	require.NoError(t, db.First(&run, "package_id = ? AND generation = ?", "knowledge", 3).Error)
	assert.Equal(t, "none", run.BeforeSchemaVersion)
	assert.Equal(t, "4.1.0", run.AfterSchemaVersion)
	assert.Empty(t, run.BackupReference)
	var validations int64
	require.NoError(t, db.Model(&model.PackageValidationResult{}).Where("migration_run_id = ?", run.ID).Count(&validations).Error)
	assert.EqualValues(t, 1, validations)

	// A Control restart re-runs plugin.enable for the same generation.
	require.NoError(t, migrator.MigrateStartedHost(ctx, ref, 3))
	assert.Len(t, invoker.inputs, 1, "a validated generation is not migrated again")

	// The next generation upgrades from the validated one.
	next := storageRef("4.2.0", strings.Repeat("d", 64), append(steps, pluginhost.MigrationStep{ID: "003_more", SHA256: strings.Repeat("e", 64)})...)
	invoker.reply = completingInvoker(PackageMigrationStepsDigest(next.Migrations)).reply
	require.NoError(t, migrator.MigrateStartedHost(ctx, next, 4))
	var upgrade model.PackageMigrationRun
	require.NoError(t, db.First(&upgrade, "package_id = ? AND generation = ?", "knowledge", 4).Error)
	assert.Equal(t, "4.1.0", upgrade.BeforeSchemaVersion)
	assert.Equal(t, "operator-managed:knowledge:4:1790000000", upgrade.BackupReference)
	require.NotNil(t, upgrade.PreviousGenerationID)
}

func TestPackageHostMigratorRejectsADifferentStepSet(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	steps := []pluginhost.MigrationStep{{ID: "001_notes", SHA256: strings.Repeat("a", 64)}}
	ref := storageRef("4.1.0", strings.Repeat("c", 64), steps...)
	tampered := []pluginhost.MigrationStep{{ID: "001_notes", SHA256: strings.Repeat("f", 64)}}
	migrator := PackageHostMigrator{DB: db, invoker: completingInvoker(PackageMigrationStepsDigest(tampered))}

	err := migrator.MigrateStartedHost(context.Background(), ref, 3)
	require.ErrorIs(t, err, ErrValidationPrecondition)
	require.ErrorContains(t, err, "differ from the verified index")
	var validations int64
	require.NoError(t, db.Model(&model.PackageValidationResult{}).Count(&validations).Error)
	assert.Zero(t, validations)
}

func TestPackageHostMigratorReportsAFailedRun(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	ref := storageRef("4.1.0", strings.Repeat("c", 64), pluginhost.MigrationStep{ID: "001_notes", SHA256: strings.Repeat("a", 64)})
	invoker := &recordingMigrationInvoker{reply: func(pluginhost.MigrationInput) pluginhost.MigrationOutput {
		return pluginhost.MigrationOutput{FailureCode: "script_failed"}
	}}
	migrator := PackageHostMigrator{DB: db, invoker: invoker}

	require.Error(t, migrator.MigrateStartedHost(context.Background(), ref, 3))
	err := migrator.MigrateStartedHost(context.Background(), ref, 3)
	require.ErrorContains(t, err, "script_failed")
	assert.Len(t, invoker.inputs, 1, "a failed run is not retried within its generation")
}

func TestPackageHostMigratorIgnoresPackagesWithoutStorageMigrations(t *testing.T) {
	db := newPackageRolloutTestDB(t)
	invoker := completingInvoker("unused")
	migrator := PackageHostMigrator{DB: db, invoker: invoker}
	steps := []pluginhost.MigrationStep{{ID: "001_identity_platform", SHA256: strings.Repeat("a", 64)}}

	withoutStorage := storageRef("4.0.0", strings.Repeat("c", 64), steps...)
	withoutStorage.Storage = false
	require.NoError(t, migrator.MigrateStartedHost(context.Background(), withoutStorage, 1))
	require.NoError(t, migrator.MigrateStartedHost(context.Background(), storageRef("4.0.0", strings.Repeat("c", 64)), 1))
	assert.Empty(t, invoker.inputs)

	require.ErrorIs(t, PackageHostMigrator{DB: db}.MigrateStartedHost(context.Background(), storageRef("4.0.0", "short", steps...), 1), ErrValidationPrecondition)
}
