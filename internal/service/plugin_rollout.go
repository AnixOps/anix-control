package service

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
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

		input.ID = 0
		input.OpaqueCheckpoint = ""
		input.ValidationDigest = ""
		input.State = model.PackageMigrationStateRunning
		input.Complete = false
		input.FailureCode = ""
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

// RecordMigrationCheckpoint durably records a host reply. A complete reply is
// only eligible for validation after its checkpoint and opaque digest are in
// this kernel-owned row.
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
		var run model.PackageMigrationRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, input.MigrationRunID).Error; err != nil {
			return err
		}
		if !run.Complete || run.State != model.PackageMigrationStateCompleted || run.FailureCode != "" {
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
			if existing.ValidationDigest != input.ValidationDigest || existing.State != model.PackageValidationStateValidated {
				return ErrPackageMigrationImmutable
			}
			result = existing
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
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

// AdvancePackageCohort creates the next immutable route generation only for
// the canonical 1, 5, 25, 100 rollout sequence. Retrying the exact request
// returns the previously-created successor.
func AdvancePackageCohort(db *gorm.DB, generationID uint, target uint8) (*model.PackageRouteGeneration, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if generationID == 0 || !isCohortPercent(target) {
		return nil, ErrCohortTransition
	}
	var result model.PackageRouteGeneration
	err := db.Transaction(func(tx *gorm.DB) error {
		var current model.PackageRouteGeneration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, generationID).Error; err != nil {
			return err
		}
		verified, verifyErr := isVerifiedGeneration(tx, current)
		if verifyErr != nil {
			return verifyErr
		}
		if !verified || current.RolledBackAt != nil || nextCohort(current.CohortPercent) != target || current.Generation == math.MaxUint64 {
			return ErrCohortTransition
		}

		var successor model.PackageRouteGeneration
		successorErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&successor, "previous_id = ?", current.ID).Error
		if successorErr == nil {
			if successor.CohortPercent == target && successor.State == model.PackageRouteGenerationStateValidated && successor.RolledBackAt == nil {
				result = successor
				return nil
			}
			return ErrCohortTransition
		}
		if !errors.Is(successorErr, gorm.ErrRecordNotFound) {
			return successorErr
		}

		var latest model.PackageRouteGeneration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("package_id = ? AND rolled_back_at IS NULL", current.PackageID).
			Order("generation DESC, id DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.ID != current.ID {
			return ErrCohortTransition
		}

		now := time.Now().UTC()
		successor = model.PackageRouteGeneration{
			PackageID: current.PackageID, Version: current.Version, Generation: current.Generation + 1,
			CohortPercent: target, State: model.PackageRouteGenerationStateValidated,
			PreviousID: &current.ID, MigrationRunID: current.MigrationRunID,
			ValidationResultID: current.ValidationResultID, BackupReferenceID: current.BackupReferenceID, ActivatedAt: &now,
		}
		create := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&successor)
		if create.Error != nil {
			return create.Error
		}
		if create.RowsAffected == 0 {
			if err := tx.First(&successor, "package_id = ? AND generation = ?", current.PackageID, current.Generation+1).Error; err != nil {
				return err
			}
			if successor.PreviousID == nil || *successor.PreviousID != current.ID || successor.CohortPercent != target || successor.RolledBackAt != nil {
				return ErrCohortTransition
			}
		}
		result = successor
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RollbackPackageGeneration retires the current generation and returns its
// verified predecessor. Both the reason and any retained backup reference are
// recorded transactionally; no package-owned table is touched.
func RollbackPackageGeneration(db *gorm.DB, generationID uint, reason string) (*model.PackageRouteGeneration, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	reason = strings.TrimSpace(reason)
	if generationID == 0 || reason == "" {
		return nil, ErrRollbackPrecondition
	}
	var result model.PackageRouteGeneration
	err := db.Transaction(func(tx *gorm.DB) error {
		var current model.PackageRouteGeneration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, generationID).Error; err != nil {
			return err
		}
		if current.PreviousID == nil {
			return ErrRollbackPrecondition
		}
		var previous model.PackageRouteGeneration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, *current.PreviousID).Error; err != nil {
			return err
		}
		verified, verifyErr := isVerifiedGeneration(tx, previous)
		if verifyErr != nil {
			return verifyErr
		}
		if !verified {
			return ErrRollbackPrecondition
		}
		if current.RolledBackAt != nil {
			if current.RollbackReason != reason {
				return ErrRollbackPrecondition
			}
			result = previous
			return nil
		}

		var latest model.PackageRouteGeneration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("package_id = ? AND rolled_back_at IS NULL", current.PackageID).
			Order("generation DESC, id DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.ID != current.ID {
			return ErrRollbackPrecondition
		}

		now := time.Now().UTC()
		if err := tx.Model(&model.PackageRouteGeneration{}).Where("id = ?", current.ID).Updates(map[string]any{
			"state": model.PackageRouteGenerationStateRolledBack, "rolled_back_at": &now, "rollback_reason": reason,
		}).Error; err != nil {
			return err
		}
		if current.BackupReferenceID != nil {
			backupUpdate := tx.Model(&model.PackageBackupReference{}).Where("id = ?", *current.BackupReferenceID).Updates(map[string]any{
				"restored_at": &now, "restore_reason": reason,
			})
			if backupUpdate.Error != nil {
				return backupUpdate.Error
			}
			if backupUpdate.RowsAffected != 1 {
				return ErrRollbackPrecondition
			}
		}
		result = previous
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

func sameMigrationCheckpoint(run model.PackageMigrationRun, checkpoint PackageMigrationCheckpoint) bool {
	return run.OpaqueCheckpoint == checkpoint.OpaqueCheckpoint && run.ValidationDigest == checkpoint.ValidationDigest &&
		run.Complete == checkpoint.Complete && run.FailureCode == checkpoint.FailureCode
}

func isVerifiedGeneration(tx *gorm.DB, generation model.PackageRouteGeneration) (bool, error) {
	if generation.State != model.PackageRouteGenerationStateValidated || generation.ValidationResultID == nil {
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
	if origin.PackageID != generation.PackageID || origin.Version != generation.Version ||
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

func isCohortPercent(value uint8) bool {
	return value == 1 || value == 5 || value == 25 || value == 100
}

func nextCohort(value uint8) uint8 {
	switch value {
	case 1:
		return 5
	case 5:
		return 25
	case 25:
		return 100
	default:
		return 0
	}
}
