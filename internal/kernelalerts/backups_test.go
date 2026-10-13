package kernelalerts

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openBackupDB(t *testing.T, createdAgo time.Duration) *gorm.DB {
	t.Helper()
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(&model.Backup{}, &migrations.MetaEntry{}))
	require.NoError(t, db.Create(&migrations.MetaEntry{Key: migrations.MetaCreatedAt, Value: start.Add(-createdAgo).Format(time.RFC3339), UpdatedAt: start}).Error)
	return db
}

func addBackup(t *testing.T, db *gorm.DB, name, status string, ago time.Duration, verifyError string) {
	t.Helper()
	row := model.Backup{Path: name, Type: "daily", CreatedAt: start.Add(-ago), VerifyStatus: status, VerifyError: verifyError}
	if status != model.BackupVerifyPending {
		at := start.Add(-ago + time.Minute)
		row.VerifiedAt = &at
	}
	require.NoError(t, db.Create(&row).Error)
}

func scanBackups(t *testing.T, db *gorm.DB) map[string]Finding {
	t.Helper()
	scanner := &Scanner{DB: db, Settings: defaultSettings(), Now: func() time.Time { return start }}
	result, err := scanner.Scan(t.Context())
	require.NoError(t, err)
	require.Empty(t, result.Unevaluated)
	return kinds(result)
}

func TestBackupAlerts(t *testing.T) {
	stale := KindBackupStale + "/" + SubjectBackups
	failed := KindBackupVerifyFailed + "/" + SubjectBackups

	t.Run("a new panel without a backup yet", func(t *testing.T) {
		assert.Empty(t, scanBackups(t, openBackupDB(t, 10*time.Hour)))
	})
	t.Run("a verified backup of today", func(t *testing.T) {
		db := openBackupDB(t, 30*day)
		addBackup(t, db, "a.tar", model.BackupVerifyPassed, 8*time.Hour, "")
		assert.Empty(t, scanBackups(t, db))
	})
	t.Run("no verified backup for 36 hours", func(t *testing.T) {
		db := openBackupDB(t, 30*day)
		addBackup(t, db, "old.tar", model.BackupVerifyPassed, 40*time.Hour, "")
		addBackup(t, db, "new.tar", model.BackupVerifyPending, 2*time.Hour, "")
		found := scanBackups(t, db)
		require.Len(t, found, 1)
		alert := found[stale]
		assert.Equal(t, model.KernelAlertWarning, alert.Severity)
		assert.Equal(t, SubjectBackups, alert.SubjectKind)
		assert.Equal(t, "old.tar", alert.Detail["last_verified_file"])
		assert.EqualValues(t, 40, alert.Detail["age_hours"])
		assert.Contains(t, alert.Message, "younger than 36 hours")
	})
	t.Run("three days without one is critical", func(t *testing.T) {
		db := openBackupDB(t, 30*day)
		addBackup(t, db, "old.tar", model.BackupVerifyPassed, 80*time.Hour, "")
		assert.Equal(t, model.KernelAlertCritical, scanBackups(t, db)[stale].Severity)
	})
	t.Run("never a verified backup on an older panel", func(t *testing.T) {
		db := openBackupDB(t, 2*day)
		found := scanBackups(t, db)
		require.Contains(t, found, stale)
		assert.Contains(t, found[stale].Message, "has no backup that passed its verification")
	})
	t.Run("the newest verified backup failed", func(t *testing.T) {
		db := openBackupDB(t, 30*day)
		addBackup(t, db, "good.tar", model.BackupVerifyPassed, 30*time.Hour, "")
		addBackup(t, db, "bad.tar", model.BackupVerifyFailed, 6*time.Hour, "integrity: PRAGMA integrity_check: database disk image is malformed")
		found := scanBackups(t, db)
		require.Len(t, found, 1)
		alert := found[failed]
		assert.Equal(t, model.KernelAlertCritical, alert.Severity)
		assert.Equal(t, "bad.tar", alert.Detail["file"])
		assert.Contains(t, alert.Message, "failed its verification and would not restore")
		assert.Contains(t, alert.Detail["error"], "integrity_check")
	})
	t.Run("a later verified backup resolves a failure", func(t *testing.T) {
		db := openBackupDB(t, 30*day)
		addBackup(t, db, "bad.tar", model.BackupVerifyFailed, 30*time.Hour, "checksum")
		addBackup(t, db, "good.tar", model.BackupVerifyPassed, 6*time.Hour, "")
		assert.Empty(t, scanBackups(t, db))
	})
	t.Run("before the table exists", func(t *testing.T) {
		assert.Empty(t, scanBackups(t, openSQLite(t)))
	})
}
