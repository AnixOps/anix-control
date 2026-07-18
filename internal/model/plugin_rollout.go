package model

import "time"

const (
	PackageMigrationStateRunning   = "running"
	PackageMigrationStateCompleted = "completed"
	PackageMigrationStateFailed    = "failed"

	PackageValidationStateValidated = "validated"

	PackageRouteGenerationStateValidated  = "validated"
	PackageRouteGenerationStateRolledBack = "rolled_back"
)

// PackageMigrationRun is kernel-owned bookkeeping for one opaque package
// migration. The kernel records package-supplied checkpoints but never
// interprets them or accesses package-owned schema.
type PackageMigrationRun struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	PackageID            string     `gorm:"size:120;not null;uniqueIndex:ux_v4_package_migration_generation" json:"package_id"`
	PackageVersion       string     `gorm:"size:64;not null" json:"package_version"`
	Generation           uint64     `gorm:"not null;uniqueIndex:ux_v4_package_migration_generation" json:"generation"`
	MigrationID          string     `gorm:"size:160;not null" json:"migration_id"`
	MigrationChecksum    string     `gorm:"size:128;not null" json:"migration_checksum"`
	BeforeSchemaVersion  string     `gorm:"size:128;not null" json:"before_schema_version"`
	AfterSchemaVersion   string     `gorm:"size:128;not null" json:"after_schema_version"`
	OpaqueCheckpoint     string     `gorm:"type:text;not null;default:''" json:"opaque_checkpoint"`
	ValidationDigest     string     `gorm:"type:text;not null;default:''" json:"validation_digest"`
	State                string     `gorm:"size:32;not null;index" json:"state"`
	Complete             bool       `gorm:"not null;default:false" json:"complete"`
	FailureCode          string     `gorm:"size:160" json:"failure_code"`
	HealthLeaseID        string     `gorm:"size:160" json:"health_lease_id"`
	HealthGeneration     uint64     `gorm:"not null;default:0" json:"health_generation"`
	HealthVerifiedAt     *time.Time `json:"health_verified_at"`
	BackupReference      string     `gorm:"type:text" json:"backup_reference"`
	BackupReferenceID    *uint      `gorm:"index" json:"backup_reference_id"`
	PreviousGenerationID *uint      `gorm:"index" json:"previous_generation_id"`
	CompletedAt          *time.Time `json:"completed_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (PackageMigrationRun) TableName() string { return "v4_kernel_package_migration_run" }

// PackageValidationResult stores the opaque validation digest supplied by a
// package host after a completed migration. It contains no domain mapping.
type PackageValidationResult struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	MigrationRunID    uint      `gorm:"not null;uniqueIndex:ux_v4_package_validation_run" json:"migration_run_id"`
	PackageID         string    `gorm:"size:120;not null;index" json:"package_id"`
	PackageVersion    string    `gorm:"size:64;not null" json:"package_version"`
	Generation        uint64    `gorm:"not null;index" json:"generation"`
	ValidationDigest  string    `gorm:"type:text;not null" json:"validation_digest"`
	State             string    `gorm:"size:32;not null;index" json:"state"`
	RouteGenerationID *uint     `gorm:"uniqueIndex" json:"route_generation_id"`
	CreatedAt         time.Time `json:"created_at"`
}

func (PackageValidationResult) TableName() string { return "v4_kernel_package_validation_result" }

// PackageRouteGeneration is an immutable, cohort-specific route generation.
// Successors link to their predecessor so rollback can restore a previously
// verified route without deleting any rollout evidence.
type PackageRouteGeneration struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	PackageID          string     `gorm:"size:120;not null;uniqueIndex:ux_v4_package_route_generation;index" json:"package_id"`
	Version            string     `gorm:"size:64;not null" json:"version"`
	Generation         uint64     `gorm:"not null;uniqueIndex:ux_v4_package_route_generation" json:"generation"`
	CohortPercent      uint8      `gorm:"not null" json:"cohort_percent"`
	State              string     `gorm:"size:32;not null;index" json:"state"`
	PreviousID         *uint      `gorm:"index" json:"previous_id"`
	MigrationRunID     *uint      `gorm:"index" json:"migration_run_id"`
	ValidationResultID *uint      `gorm:"index" json:"validation_result_id"`
	BackupReferenceID  *uint      `gorm:"index" json:"backup_reference_id"`
	ActivatedAt        *time.Time `json:"activated_at"`
	RolledBackAt       *time.Time `json:"rolled_back_at"`
	RollbackReason     string     `gorm:"type:text" json:"rollback_reason"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (PackageRouteGeneration) TableName() string { return "v4_kernel_package_route_generation" }

// PackageBackupReference records an opaque package backup location. The
// kernel can retain and audit the reference but never reads package data from
// it.
type PackageBackupReference struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	MigrationRunID       uint       `gorm:"not null;uniqueIndex" json:"migration_run_id"`
	PackageID            string     `gorm:"size:120;not null;index" json:"package_id"`
	PackageVersion       string     `gorm:"size:64;not null" json:"package_version"`
	Generation           uint64     `gorm:"not null;index" json:"generation"`
	Reference            string     `gorm:"type:text;not null" json:"reference"`
	Checksum             string     `gorm:"size:128" json:"checksum"`
	PreviousGenerationID *uint      `gorm:"index" json:"previous_generation_id"`
	RestoredAt           *time.Time `json:"restored_at"`
	RestoreReason        string     `gorm:"type:text" json:"restore_reason"`
	CreatedAt            time.Time  `json:"created_at"`
}

func (PackageBackupReference) TableName() string { return "v4_kernel_package_backup_reference" }
