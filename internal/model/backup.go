package model

import "time"

// Backup verification states (Backup.VerifyStatus).
const (
	// BackupVerifyPending: the bundle was not verified yet.
	BackupVerifyPending = "pending"
	// BackupVerifyPassed: the latest verification restored the bundle into
	// a scratch directory and every check passed.
	BackupVerifyPassed = "passed"
	// BackupVerifyFailed: the latest verification found a problem
	// (VerifyError says which).
	BackupVerifyFailed = "failed"
)

// Backup is a row of v5_backup: a backup bundle in <data_dir>/backups and
// the result of its latest verification (internal/backup,
// docs/reference/backups.md). The bundle files are the truth: a bundle
// written while the table did not exist yet (a pre-migration backup) is
// added when the backup worker next reads the directory, and the row of a
// bundle that is gone is removed.
type Backup struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Path is the bundle's file name in <data_dir>/backups
	// (anix-control-20261013T031500Z-daily.tar).
	Path string `gorm:"size:255;not null;uniqueIndex:idx_v5_backup_path" json:"path"`
	// Type is daily, pre-migration, manual or verify.
	Type string `gorm:"size:32;not null;index:idx_v5_backup_type" json:"type"`
	// Size is the bundle's size in bytes and SHA256 its digest (hex).
	Size   int64  `gorm:"not null;default:0" json:"size"`
	SHA256 string `gorm:"column:sha256;size:64;not null;default:''" json:"sha256"`
	// CreatedAt is the manifest's created_at.
	CreatedAt time.Time `gorm:"not null;index:idx_v5_backup_created_at" json:"created_at"`
	// VerifiedAt is when the latest verification ended; nil while pending.
	VerifiedAt   *time.Time `json:"verified_at"`
	VerifyStatus string     `gorm:"size:16;not null;default:'pending'" json:"verify_status"`
	// VerifyError says what the latest verification found; empty unless
	// it failed. It never holds key material.
	VerifyError string `gorm:"type:text;not null;default:''" json:"verify_error"`
	// ProgramVersion is the Control version that wrote the bundle and
	// SchemaVersion the newest migration of its database.
	ProgramVersion string `gorm:"size:64;not null;default:''" json:"program_version"`
	SchemaVersion  int    `gorm:"not null;default:0" json:"schema_version"`
}

// TableName is v5_backup.
func (Backup) TableName() string { return "v5_backup" }
