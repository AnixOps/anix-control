package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"gorm.io/gorm"
)

// noPreviousSchemaVersion is the before-schema version of a package's first
// migration run.
const noPreviousSchemaVersion = "none"

// PackageIndexMigrationID names the ledger run that applies a whole migration
// index, keyed by the index digest.
func PackageIndexMigrationID(indexSHA256 string) string {
	indexSHA256 = strings.ToLower(indexSHA256)
	if len(indexSHA256) > 32 {
		indexSHA256 = indexSHA256[:32]
	}
	return pluginhostsdk.IndexMigrationPrefix + indexSHA256
}

// PackageMigrationStepsDigest is the validation digest a host must report
// after applying steps; see packagestoresdk.StepsDigest.
func PackageMigrationStepsDigest(steps []pluginhost.MigrationStep) string {
	identities := make([]packagestoresdk.MigrationStep, 0, len(steps))
	for _, step := range steps {
		identities = append(identities, packagestoresdk.MigrationStep{ID: step.ID, SHA256: step.SHA256})
	}
	return packagestoresdk.StepsDigest(identities)
}

// PackageHostMigrator runs the migration index of a started storage package
// through the migration ledger: one run per package lifecycle generation.
type PackageHostMigrator struct {
	DB    *gorm.DB
	Hosts *pluginhost.Supervisor
	// Now defaults to time.Now.
	Now func() time.Time
	// invoker replaces Hosts in tests; production always migrates through
	// the concrete Supervisor, which alone attaches the health fence.
	invoker packageMigrationInvoker
}

// MigrateStartedHost applies ref's migration index in the host that was just
// started for generation, verifies that the host applied exactly the steps of
// the verified index, and records the validation. Releases that do not
// declare kernel.storage.v1, or declare no steps, are left alone. A repeated
// call for the same generation, for example after a Control restart, resumes
// or confirms the recorded run instead of starting another one.
func (m PackageHostMigrator) MigrateStartedHost(ctx context.Context, ref pluginhost.ArtifactRef, generation uint64) error {
	if !ref.Storage || len(ref.Migrations) == 0 {
		return nil
	}
	if m.DB == nil || (m.Hosts == nil && m.invoker == nil) || generation == 0 || !validSHA256Hex(ref.MigrationIndexSHA256) {
		return ErrValidationPrecondition
	}
	db := m.DB.WithContext(ctx)
	migrationID := PackageIndexMigrationID(ref.MigrationIndexSHA256)
	run, err := m.migrationRun(db, ref, generation, migrationID)
	if err != nil {
		return err
	}
	if run.State == model.PackageMigrationStateFailed {
		return fmt.Errorf("%w: package migration run %d failed with %q", ErrValidationPrecondition, run.ID, run.FailureCode)
	}
	if !run.Complete || run.HealthVerifiedAt == nil {
		input := pluginhost.MigrationInput{
			PackageID: ref.PackageID, Version: ref.Version, MigrationID: migrationID,
			Checkpoint: run.OpaqueCheckpoint, Generation: generation,
		}
		var output pluginhost.MigrationOutput
		if m.invoker != nil {
			output, err = migratePackageHost(ctx, db, m.invoker, run.ID, input)
		} else {
			output, err = MigratePackageHost(ctx, db, m.Hosts, run.ID, input)
		}
		if err != nil {
			return err
		}
		if !output.Complete {
			return fmt.Errorf("%w: package host did not complete its migration index", ErrValidationPrecondition)
		}
		if err := db.First(run, run.ID).Error; err != nil {
			return err
		}
	}
	expected := PackageMigrationStepsDigest(ref.Migrations)
	if !strings.EqualFold(run.ValidationDigest, expected) {
		return fmt.Errorf("%w: package host applied migrations that differ from the verified index", ErrValidationPrecondition)
	}
	_, err = RecordPackageValidation(db, model.PackageValidationResult{MigrationRunID: run.ID, ValidationDigest: run.ValidationDigest})
	return err
}

// migrationRun returns the ledger run of generation, beginning it if needed.
func (m PackageHostMigrator) migrationRun(db *gorm.DB, ref pluginhost.ArtifactRef, generation uint64, migrationID string) (*model.PackageMigrationRun, error) {
	var existing model.PackageMigrationRun
	err := db.First(&existing, "package_id = ? AND generation = ?", ref.PackageID, generation).Error
	if err == nil {
		if existing.PackageVersion != ref.Version || existing.MigrationID != migrationID ||
			!strings.EqualFold(existing.MigrationChecksum, ref.MigrationIndexSHA256) {
			return nil, ErrPackageMigrationImmutable
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	input := model.PackageMigrationRun{
		PackageID: ref.PackageID, PackageVersion: ref.Version, Generation: generation,
		MigrationID: migrationID, MigrationChecksum: strings.ToLower(ref.MigrationIndexSHA256),
		BeforeSchemaVersion: noPreviousSchemaVersion, AfterSchemaVersion: ref.Version,
	}
	var previous model.PackageRouteGeneration
	err = db.Where("package_id = ? AND rolled_back_at IS NULL", ref.PackageID).Order("generation DESC, id DESC").First(&previous).Error
	switch {
	case err == nil:
		input.BeforeSchemaVersion = previous.Version
		// Operators back up the database before an upgrade (docs/UPGRADE.md);
		// the kernel records that the backup is theirs.
		input.BackupReference = fmt.Sprintf("operator-managed:%s:%d:%d", ref.PackageID, generation, m.now().Unix())
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	return BeginPackageMigration(db, input)
}

func (m PackageHostMigrator) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
