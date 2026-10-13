package kernelalerts

import (
	"context"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
)

// BackupStaleAfter: backup_stale fires when no bundle that passed its
// verification is younger than this. The daily backup leaves 24 hours
// between two bundles; 36 hours means one was missed.
const BackupStaleAfter = 36 * time.Hour

// backupStaleCritical: backup_stale turns critical after three missed days.
const backupStaleCritical = 72 * time.Hour

// backups reads v5_backup (internal/backup) and finds a newest verified
// bundle that failed, and a panel without a verified bundle for
// BackupStaleAfter. Backups need SQLite: on PostgreSQL there is nothing to
// check, and a database without the table (before its migration) has none.
func (s *Scanner) backups(ctx context.Context, now time.Time) ([]Finding, error) {
	db := s.DB.WithContext(ctx)
	if db.Name() == "postgres" || !db.Migrator().HasTable(&model.Backup{}) {
		return nil, nil
	}
	var verified []model.Backup
	if err := db.Where("verify_status IN ?", []string{model.BackupVerifyPassed, model.BackupVerifyFailed}).
		Order("created_at DESC").Order("id DESC").Limit(1).Find(&verified).Error; err != nil {
		return nil, err
	}
	var findings []Finding
	if len(verified) == 1 && verified[0].VerifyStatus == model.BackupVerifyFailed {
		row := verified[0]
		detail := map[string]any{"file": row.Path, "type": row.Type, "created_at": stamp(row.CreatedAt), "error": truncate(row.VerifyError, 400)}
		if row.VerifiedAt != nil {
			detail["verified_at"] = stamp(*row.VerifiedAt)
		}
		findings = append(findings, Finding{
			Kind: KindBackupVerifyFailed, Severity: model.KernelAlertCritical, SubjectKind: SubjectBackups, Subject: SubjectBackups,
			Message: truncate(fmt.Sprintf("The backup %s (%s, taken %s) failed its verification and would not restore: %s. Run anix-control backup list and backup create on the server, and read the Control log.",
				plain(row.Path), row.Type, stamp(row.CreatedAt), plain(row.VerifyError)), 500),
			Detail: detail,
		})
	}

	var passed []model.Backup
	if err := db.Where("verify_status = ?", model.BackupVerifyPassed).Order("created_at DESC").Limit(1).Find(&passed).Error; err != nil {
		return nil, err
	}
	detail := map[string]any{"stale_after_hours": int64(BackupStaleAfter / time.Hour)}
	var since time.Time
	if len(passed) == 1 {
		since = passed[0].CreatedAt
		detail["last_verified_file"] = passed[0].Path
		detail["last_verified_created_at"] = stamp(passed[0].CreatedAt)
	} else {
		// Never a verified bundle: count from when the database was
		// created, so a new panel does not alert before its first night.
		created, err := databaseCreatedAt(ctx, s)
		if err != nil || created.IsZero() {
			return findings, err
		}
		since = created
	}
	age := now.Sub(since)
	if age <= BackupStaleAfter {
		return findings, nil
	}
	severity := model.KernelAlertWarning
	if age > backupStaleCritical {
		severity = model.KernelAlertCritical
	}
	detail["age_hours"] = int64(age / time.Hour)
	message := fmt.Sprintf("No backup of this panel that passed its verification is younger than %d hours; the newest is from %s. Check the backup worker in the Control log, or run anix-control backup create on the server.",
		int64(BackupStaleAfter/time.Hour), stamp(since))
	if len(passed) == 0 {
		message = fmt.Sprintf("This panel has no backup that passed its verification, %d hours after its database was created. Check the backup worker in the Control log, or run anix-control backup create on the server.", int64(age/time.Hour))
	}
	findings = append(findings, Finding{
		Kind: KindBackupStale, Severity: severity, SubjectKind: SubjectBackups, Subject: SubjectBackups,
		Message: truncate(message, 500), Detail: detail,
	})
	return findings, nil
}

// databaseCreatedAt is v5_control_meta's created_at; zero when the
// database does not record it.
func databaseCreatedAt(ctx context.Context, s *Scanner) (time.Time, error) {
	meta, err := migrations.Meta(ctx, s.DB)
	if err != nil {
		return time.Time{}, err
	}
	raw := meta[migrations.MetaCreatedAt]
	if raw == "" {
		return time.Time{}, nil
	}
	created, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, nil
	}
	return created, nil
}
