package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PackageMigrationCheckpoint is an opaque host response persisted before the
// caller can act on that response. Its fields are intentionally not parsed by
// the kernel.
type PackageMigrationCheckpoint struct {
	OpaqueCheckpoint string
	ValidationDigest string
	Complete         bool
	FailureCode      string
}

// BeginPackageMigration creates one generation-fenced migration run. A retry
// with the same immutable input returns the original run and its latest
// checkpoint so an interrupted host call can resume safely.
func BeginPackageMigration(db *gorm.DB, input model.PackageMigrationRun) (*model.PackageMigrationRun, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	input.PackageID = strings.TrimSpace(input.PackageID)
	input.PackageVersion = strings.TrimSpace(input.PackageVersion)
	input.MigrationID = strings.TrimSpace(input.MigrationID)
	input.MigrationChecksum = strings.TrimSpace(input.MigrationChecksum)
	input.BeforeSchemaVersion = strings.TrimSpace(input.BeforeSchemaVersion)
	input.AfterSchemaVersion = strings.TrimSpace(input.AfterSchemaVersion)
	input.BackupReference = strings.TrimSpace(input.BackupReference)
	input.PreviousGenerationID = nil
	if input.PackageID == "" || input.PackageVersion == "" || input.Generation == 0 || input.MigrationID == "" ||
		input.MigrationChecksum == "" || input.BeforeSchemaVersion == "" || input.AfterSchemaVersion == "" {
		return nil, ErrPackageMigrationImmutable
	}

	var result model.PackageMigrationRun
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockPackageRollout(tx, input.PackageID); err != nil {
			return err
		}
		var existing model.PackageMigrationRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "package_id = ? AND generation = ?", input.PackageID, input.Generation).Error
		if err == nil {
			if !sameMigrationInput(existing, input) {
				return ErrPackageMigrationImmutable
			}
			result = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var previous model.PackageRouteGeneration
		previousErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("package_id = ? AND rolled_back_at IS NULL", input.PackageID).
			Order("generation DESC, id DESC").First(&previous).Error
		if previousErr == nil {
			verified, verifyErr := isVerifiedGeneration(tx, previous)
			if verifyErr != nil {
				return verifyErr
			}
			if input.Generation <= previous.Generation || !verified {
				return ErrValidationPrecondition
			}
			input.PreviousGenerationID = &previous.ID
		} else if !errors.Is(previousErr, gorm.ErrRecordNotFound) {
			return previousErr
		}
		if input.PreviousGenerationID != nil && input.BackupReference == "" {
			return ErrValidationPrecondition
		}

		input.ID = 0
		input.OpaqueCheckpoint = ""
		input.ValidationDigest = ""
		input.State = model.PackageMigrationStateRunning
		input.Complete = false
		input.FailureCode = ""
		input.HealthLeaseID = ""
		input.HealthGeneration = 0
		input.HealthVerifiedAt = nil
		input.CompletedAt = nil
		input.BackupReferenceID = nil
		create := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&input)
		if create.Error != nil {
			return create.Error
		}
		if create.RowsAffected == 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "package_id = ? AND generation = ?", input.PackageID, input.Generation).Error; err != nil {
				return err
			}
			if !sameMigrationInput(existing, input) {
				return ErrPackageMigrationImmutable
			}
			result = existing
			return nil
		}

		if input.BackupReference != "" {
			backup := model.PackageBackupReference{
				MigrationRunID: input.ID, PackageID: input.PackageID, PackageVersion: input.PackageVersion,
				Generation: input.Generation, Reference: input.BackupReference, Checksum: input.MigrationChecksum,
				PreviousGenerationID: input.PreviousGenerationID,
			}
			if err := tx.Create(&backup).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.PackageMigrationRun{}).Where("id = ?", input.ID).Update("backup_reference_id", backup.ID).Error; err != nil {
				return err
			}
			input.BackupReferenceID = &backup.ID
		}
		result = input
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RecordMigrationCheckpoint durably records an opaque host reply. It cannot
// activate a route; the health proof is recorded only by the service-owned
// migration coordinator after the supervisor returns successfully.
func RecordMigrationCheckpoint(db *gorm.DB, migrationRunID uint, checkpoint PackageMigrationCheckpoint) (*model.PackageMigrationRun, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if migrationRunID == 0 {
		return nil, ErrPackageMigrationImmutable
	}
	checkpoint.FailureCode = strings.TrimSpace(checkpoint.FailureCode)
	var result model.PackageMigrationRun
	err := db.Transaction(func(tx *gorm.DB) error {
		var run model.PackageMigrationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, migrationRunID).Error; err != nil {
			return err
		}
		if sameMigrationCheckpoint(run, checkpoint) {
			result = run
			return nil
		}
		if run.Complete || run.State == model.PackageMigrationStateFailed {
			return ErrPackageMigrationImmutable
		}

		state := model.PackageMigrationStateRunning
		var completedAt *time.Time
		if checkpoint.FailureCode != "" {
			state = model.PackageMigrationStateFailed
			now := time.Now().UTC()
			completedAt = &now
		} else if checkpoint.Complete {
			state = model.PackageMigrationStateCompleted
			now := time.Now().UTC()
			completedAt = &now
		}
		updates := map[string]any{
			"opaque_checkpoint": checkpoint.OpaqueCheckpoint,
			"validation_digest": checkpoint.ValidationDigest,
			"complete":          checkpoint.Complete,
			"failure_code":      checkpoint.FailureCode,
			"state":             state,
			"completed_at":      completedAt,
		}
		if err := tx.Model(&model.PackageMigrationRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&result, run.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// MigratePackageHost is the only public orchestration path that can attach a
// health fence to a migration run. Requiring the concrete Supervisor prevents
// callers from substituting an arbitrary MigrationManager that fabricates a
// successful health result.
func MigratePackageHost(ctx context.Context, db *gorm.DB, manager *pluginhost.Supervisor, migrationRunID uint, input pluginhost.MigrationInput) (pluginhost.MigrationOutput, error) {
	if manager == nil {
		return pluginhost.MigrationOutput{}, ErrValidationPrecondition
	}
	return migratePackageHost(ctx, db, manager, migrationRunID, input)
}

type packageMigrationInvoker interface {
	Migrate(context.Context, pluginhost.MigrationInput, pluginhost.MigrationCheckpointRecorder) (pluginhost.MigrationOutput, error)
}

func migratePackageHost(ctx context.Context, db *gorm.DB, manager packageMigrationInvoker, migrationRunID uint, input pluginhost.MigrationInput) (pluginhost.MigrationOutput, error) {
	if db == nil {
		return pluginhost.MigrationOutput{}, errors.New("database is not initialized")
	}
	if manager == nil || migrationRunID == 0 {
		return pluginhost.MigrationOutput{}, ErrValidationPrecondition
	}
	var run model.PackageMigrationRun
	if err := db.First(&run, migrationRunID).Error; err != nil {
		return pluginhost.MigrationOutput{}, err
	}
	if run.PackageID != input.PackageID || run.PackageVersion != input.Version || run.MigrationID != input.MigrationID || run.Generation != input.Generation {
		return pluginhost.MigrationOutput{}, ErrValidationPrecondition
	}
	output, err := manager.Migrate(ctx, input, func(ctx context.Context, reply pluginhost.MigrationOutput) error {
		_, checkpointErr := RecordMigrationCheckpoint(db, migrationRunID, PackageMigrationCheckpoint{
			OpaqueCheckpoint: reply.Checkpoint, ValidationDigest: reply.ValidationDigest,
			Complete: reply.Complete, FailureCode: reply.FailureCode,
		})
		return checkpointErr
	})
	if err != nil {
		return output, err
	}
	if !output.Complete {
		return output, nil
	}
	if _, err := recordMigrationHealthFence(db, migrationRunID, input.Generation, output.HealthLeaseID, output.HealthGeneration); err != nil {
		return output, err
	}
	return output, nil
}

func recordMigrationHealthFence(db *gorm.DB, migrationRunID uint, generation uint64, leaseID string, healthGeneration uint64) (*model.PackageMigrationRun, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if migrationRunID == 0 || generation == 0 || strings.TrimSpace(leaseID) == "" || generation != healthGeneration {
		return nil, ErrValidationPrecondition
	}
	var result model.PackageMigrationRun
	err := db.Transaction(func(tx *gorm.DB) error {
		var run model.PackageMigrationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, migrationRunID).Error; err != nil {
			return err
		}
		if !run.Complete || run.State != model.PackageMigrationStateCompleted || run.FailureCode != "" || run.Generation != generation {
			return ErrValidationPrecondition
		}
		if run.HealthVerifiedAt != nil || run.HealthLeaseID != "" || run.HealthGeneration != 0 {
			if run.HealthLeaseID == leaseID && run.HealthGeneration == healthGeneration {
				result = run
				return nil
			}
			return ErrPackageMigrationImmutable
		}
		now := time.Now().UTC()
		if err := tx.Model(&model.PackageMigrationRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"health_lease_id": leaseID, "health_generation": healthGeneration, "health_verified_at": &now,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&result, run.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RecordPackageValidation accepts only a completed migration checkpoint and
// creates the initial one-percent route generation. The digest remains opaque
// to the kernel.
func RecordPackageValidation(db *gorm.DB, input model.PackageValidationResult) (*model.PackageValidationResult, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if input.MigrationRunID == 0 {
		return nil, ErrValidationPrecondition
	}
	var result model.PackageValidationResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var migrationIdentity model.PackageMigrationRun
		if err := tx.Select("id", "package_id").First(&migrationIdentity, input.MigrationRunID).Error; err != nil {
			return err
		}
		if err := lockPackageRollout(tx, migrationIdentity.PackageID); err != nil {
			return err
		}
		var run model.PackageMigrationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, input.MigrationRunID).Error; err != nil {
			return err
		}
		if !run.Complete || run.State != model.PackageMigrationStateCompleted || run.FailureCode != "" {
			return ErrValidationPrecondition
		}
		if run.HealthVerifiedAt == nil || run.HealthLeaseID == "" || run.HealthGeneration != run.Generation {
			return ErrValidationPrecondition
		}
		if strings.TrimSpace(run.ValidationDigest) == "" {
			return ErrValidationPrecondition
		}
		if strings.TrimSpace(input.ValidationDigest) == "" {
			input.ValidationDigest = run.ValidationDigest
		} else if input.ValidationDigest != run.ValidationDigest {
			return ErrValidationPrecondition
		}
		if (input.PackageID != "" && input.PackageID != run.PackageID) ||
			(input.PackageVersion != "" && input.PackageVersion != run.PackageVersion) ||
			(input.Generation != 0 && input.Generation != run.Generation) {
			return ErrValidationPrecondition
		}

		var existing model.PackageValidationResult
		existingErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "migration_run_id = ?", run.ID).Error
		if existingErr == nil {
			if existing.PackageID != run.PackageID || existing.PackageVersion != run.PackageVersion || existing.Generation != run.Generation ||
				existing.ValidationDigest != input.ValidationDigest || existing.State != model.PackageValidationStateValidated || existing.RouteGenerationID == nil {
				return ErrPackageMigrationImmutable
			}
			var route model.PackageRouteGeneration
			if err := tx.First(&route, *existing.RouteGenerationID).Error; err != nil {
				return err
			}
			if route.PackageID != run.PackageID || route.Version != run.PackageVersion || route.Generation != run.Generation ||
				route.MigrationRunID == nil || *route.MigrationRunID != run.ID || route.ValidationResultID == nil || *route.ValidationResultID != existing.ID {
				return ErrPackageMigrationImmutable
			}
			result = existing
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}

		var latest model.PackageRouteGeneration
		latestErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("package_id = ? AND rolled_back_at IS NULL", run.PackageID).
			Order("generation DESC, id DESC").First(&latest).Error
		if run.PreviousGenerationID == nil {
			if latestErr == nil {
				return ErrValidationPrecondition
			}
			if !errors.Is(latestErr, gorm.ErrRecordNotFound) {
				return latestErr
			}
		} else {
			if latestErr != nil {
				if errors.Is(latestErr, gorm.ErrRecordNotFound) {
					return ErrValidationPrecondition
				}
				return latestErr
			}
			if latest.ID != *run.PreviousGenerationID {
				return ErrValidationPrecondition
			}
			verified, verifyErr := isVerifiedGeneration(tx, latest)
			if verifyErr != nil {
				return verifyErr
			}
			if !verified {
				return ErrValidationPrecondition
			}
		}

		input.ID = 0
		input.PackageID = run.PackageID
		input.PackageVersion = run.PackageVersion
		input.Generation = run.Generation
		input.State = model.PackageValidationStateValidated
		input.RouteGenerationID = nil
		if err := tx.Create(&input).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		route := model.PackageRouteGeneration{
			PackageID: run.PackageID, Version: run.PackageVersion, Generation: run.Generation,
			CohortPercent: 1, State: model.PackageRouteGenerationStateValidated,
			PreviousID: run.PreviousGenerationID, MigrationRunID: &run.ID,
			ValidationResultID: &input.ID, BackupReferenceID: run.BackupReferenceID, ActivatedAt: &now,
		}
		if err := tx.Create(&route).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PackageValidationResult{}).Where("id = ?", input.ID).Update("route_generation_id", route.ID).Error; err != nil {
			return err
		}
		input.RouteGenerationID = &route.ID
		result = input
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func sameMigrationInput(existing, input model.PackageMigrationRun) bool {
	return existing.PackageID == input.PackageID && existing.PackageVersion == input.PackageVersion &&
		existing.Generation == input.Generation && existing.MigrationID == input.MigrationID &&
		existing.MigrationChecksum == input.MigrationChecksum && existing.BeforeSchemaVersion == input.BeforeSchemaVersion &&
		existing.AfterSchemaVersion == input.AfterSchemaVersion && existing.BackupReference == input.BackupReference
}

// lockPackageRollout creates a permanent package-scoped mutex row if needed,
// then locks it for the transaction. The row lock works across PostgreSQL
// sessions; SQLite serializes its write transaction through the same GORM path.
func lockPackageRollout(tx *gorm.DB, packageID string) error {
	if strings.TrimSpace(packageID) == "" {
		return ErrValidationPrecondition
	}
	lock := model.PackageRolloutLock{PackageID: packageID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, "package_id = ?", packageID).Error
}

func sameMigrationCheckpoint(run model.PackageMigrationRun, checkpoint PackageMigrationCheckpoint) bool {
	return run.OpaqueCheckpoint == checkpoint.OpaqueCheckpoint && run.ValidationDigest == checkpoint.ValidationDigest &&
		run.Complete == checkpoint.Complete && run.FailureCode == checkpoint.FailureCode
}

func isVerifiedGeneration(tx *gorm.DB, generation model.PackageRouteGeneration) (bool, error) {
	if generation.State != model.PackageRouteGenerationStateValidated || generation.RolledBackAt != nil || generation.ValidationResultID == nil {
		return false, nil
	}
	var validation model.PackageValidationResult
	if err := tx.First(&validation, *generation.ValidationResultID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if validation.PackageID != generation.PackageID || validation.PackageVersion != generation.Version ||
		validation.State != model.PackageValidationStateValidated || strings.TrimSpace(validation.ValidationDigest) == "" ||
		validation.RouteGenerationID == nil {
		return false, nil
	}
	var origin model.PackageRouteGeneration
	if err := tx.First(&origin, *validation.RouteGenerationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if origin.PackageID != generation.PackageID || origin.Version != generation.Version || origin.RolledBackAt != nil ||
		origin.Generation != validation.Generation || origin.ValidationResultID == nil || *origin.ValidationResultID != validation.ID {
		return false, nil
	}
	return routeDescendsFrom(tx, generation, origin.ID)
}

func routeDescendsFrom(tx *gorm.DB, generation model.PackageRouteGeneration, originID uint) (bool, error) {
	seen := map[uint]struct{}{}
	for {
		if generation.ID == originID {
			return true, nil
		}
		if generation.ID == 0 || generation.PreviousID == nil {
			return false, nil
		}
		if _, exists := seen[generation.ID]; exists {
			return false, nil
		}
		seen[generation.ID] = struct{}{}
		var previous model.PackageRouteGeneration
		if err := tx.First(&previous, *generation.PreviousID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		generation = previous
	}
}
