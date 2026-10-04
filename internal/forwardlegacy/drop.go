package forwardlegacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// BackupMaxAge is how old a database backup the drop accepts may be.
const BackupMaxAge = 24 * time.Hour

// DefaultLockTimeout bounds how long the drop waits on PostgreSQL for a
// table another session still reads.
const DefaultLockTimeout = 10 * time.Second

// Backup is the database backup a drop accepted.
type Backup struct {
	// Source is "backup_record" (Control's own backup service) or
	// "operator" (--backup-taken).
	Source     string    `json:"source"`
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	ModifiedAt time.Time `json:"modified_at"`
	RecordID   uint      `json:"record_id,omitempty"`
}

// Preconditions are what the drop needs, and what is missing.
type Preconditions struct {
	Archive *model.ForwardLegacyArchive `json:"archive"`
	Nodes   []NodeStatus                `json:"nodes"`
	Backup  *Backup                     `json:"backup"`
	// Present and Missing split DropTables by whether they exist now.
	Present []string `json:"present"`
	Missing []string `json:"missing"`
	// Blockers lists every unmet precondition; the drop runs only when it
	// is empty.
	Blockers []string `json:"blockers"`
}

// PreconditionOptions configure the checks.
type PreconditionOptions struct {
	// BackupPath is the operator's database backup (--backup-taken); empty
	// looks for a backup Control's backup service made.
	BackupPath string
	// SkipBackup leaves the backup check out (the status command, which
	// only reports).
	SkipBackup bool
	Now        time.Time
}

// CheckPreconditions answers every precondition of the drop except the
// confirmation phrase.
func CheckPreconditions(ctx context.Context, db *gorm.DB, options PreconditionOptions) (*Preconditions, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	db = db.WithContext(ctx)
	result := &Preconditions{Present: []string{}, Missing: []string{}, Blockers: []string{}}
	present := presentTables(db, DropTables)
	for _, table := range DropTables {
		if present[table] {
			result.Present = append(result.Present, table)
		} else {
			result.Missing = append(result.Missing, table)
		}
	}
	if drop, err := LatestDrop(db); err != nil {
		return nil, err
	} else if drop != nil {
		result.Blockers = append(result.Blockers, fmt.Sprintf("the flux tables were already dropped at %s", drop.CreatedAt.UTC().Format(time.RFC3339)))
		return result, nil
	}

	// The archive: recorded, readable, unchanged, and current.
	archiveRecord, err := LatestArchive(db)
	if err != nil {
		return nil, err
	}
	result.Archive = archiveRecord
	if archiveRecord == nil {
		result.Blockers = append(result.Blockers, "no archive: run `anix-control forward legacy archive` first")
	} else if _, err := VerifyArchive(archiveRecord); err != nil {
		result.Blockers = append(result.Blockers, err.Error()+": write a new archive")
	} else if stale, err := staleTables(db, archiveRecord); err != nil {
		return nil, err
	} else if len(stale) > 0 {
		result.Blockers = append(result.Blockers, "the flux data changed since the archive ("+strings.Join(stale, ", ")+"): write a new archive")
	}

	// Every forward node clean or abandoned.
	nodes, err := Status(ctx, db)
	if err != nil {
		return nil, err
	}
	result.Nodes = nodes
	var waiting []string
	for _, node := range nodes {
		if !node.Ready() {
			waiting = append(waiting, fmt.Sprintf("%s (%s) %s", node.Ref, node.Name, node.State))
		}
	}
	if len(waiting) > 0 {
		result.Blockers = append(result.Blockers, fmt.Sprintf("%d forward node(s) neither clean nor abandoned: %s", len(waiting), strings.Join(waiting, ", ")))
	}

	// No Control running on the database.
	if holder, until, running, err := ControlRunning(ctx, db, now); err != nil {
		return nil, err
	} else if running {
		result.Blockers = append(result.Blockers, fmt.Sprintf("Control is running on this database (lease %s held by %s until %s): stop every Control process, wait for the lease to expire, then drop",
			SingletonLease, holder, until.UTC().Format(time.RFC3339)))
	}

	if !options.SkipBackup {
		backup, err := findBackup(db, options.BackupPath, now)
		if err != nil {
			result.Blockers = append(result.Blockers, err.Error())
		}
		result.Backup = backup
	}
	return result, nil
}

// staleTables answers the drop tables whose row count differs from the
// archive's: the flux routes still write until F5d removes them.
func staleTables(db *gorm.DB, record *model.ForwardLegacyArchive) ([]string, error) {
	archived := map[string]int64{}
	if err := json.Unmarshal([]byte(record.RowCounts), &archived); err != nil {
		return nil, fmt.Errorf("archive record %d: row counts: %w", record.ID, err)
	}
	var stale []string
	for _, table := range DropTables {
		current := int64(-1)
		if db.Migrator().HasTable(table) {
			if err := db.Table(table).Count(&current).Error; err != nil {
				return nil, fmt.Errorf("count %s: %w", table, err)
			}
		}
		was, ok := archived[table]
		if !ok {
			was = -1
		}
		// A table gone since the archive (another release removed it) is
		// not new data.
		if current == -1 {
			continue
		}
		if current != was {
			stale = append(stale, fmt.Sprintf("%s: %d rows, archived %d", table, current, was))
		}
	}
	return stale, nil
}

// findBackup answers the database backup the drop accepts: the operator's
// file, or else a successful database or full backup of Control's backup
// service (SQLite only). Either must be at most BackupMaxAge old.
func findBackup(db *gorm.DB, operatorPath string, now time.Time) (*Backup, error) {
	if strings.TrimSpace(operatorPath) != "" {
		backup, err := describeBackup(operatorPath, now)
		if err != nil {
			return nil, fmt.Errorf("--backup-taken %s: %w", operatorPath, err)
		}
		backup.Source = "operator"
		return backup, nil
	}
	if db.Migrator().HasTable(&model.BackupRecord{}) {
		var records []model.BackupRecord
		if err := db.Where("status = ? AND type IN ?", 1, []string{"database", "full"}).
			Where("completed_at IS NOT NULL AND completed_at >= ?", now.Add(-BackupMaxAge)).
			Order("completed_at DESC").Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			backup, err := describeBackup(record.Path, now)
			if err != nil {
				continue
			}
			backup.Source, backup.RecordID = "backup_record", record.ID
			return backup, nil
		}
	}
	return nil, errors.New("no database backup of the last 24 hours: take one (on SQLite Control's backup of type database or full; on PostgreSQL pg_dump) and pass its file with --backup-taken <path>")
}

// describeBackup checks a backup file: a non-empty regular file modified
// within BackupMaxAge.
func describeBackup(path string, now time.Time) (*Backup, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("not readable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() == 0 {
		return nil, errors.New("empty")
	}
	if now.Sub(info.ModTime()) > BackupMaxAge {
		return nil, fmt.Errorf("older than %s (modified %s): take a new backup", BackupMaxAge, info.ModTime().UTC().Format(time.RFC3339))
	}
	file, err := os.Open(absolute) // #nosec G304 -- the administrator named the backup.
	if err != nil {
		return nil, fmt.Errorf("not readable: %w", err)
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return nil, fmt.Errorf("not readable: %w", err)
	}
	return &Backup{Path: absolute, SizeBytes: info.Size(), SHA256: hex.EncodeToString(digest.Sum(nil)), ModifiedAt: info.ModTime().UTC()}, nil
}

// DropOptions configure the drop.
type DropOptions struct {
	// Confirm must be ConfirmPhrase exactly.
	Confirm    string
	BackupPath string
	Actor      string
	// LockTimeout bounds the wait for a table lock on PostgreSQL;
	// DefaultLockTimeout when zero.
	LockTimeout time.Duration
	Now         time.Time
}

// DropResult is a completed drop.
type DropResult struct {
	Record         model.ForwardLegacyDrop `json:"record"`
	Dropped        []string                `json:"dropped"`
	AlreadyMissing []string                `json:"already_missing"`
	Backup         *Backup                 `json:"backup"`
}

// RefusedError lists every reason the drop refused.
type RefusedError struct{ Reasons []string }

func (e *RefusedError) Error() string {
	return "drop refused:\n  - " + strings.Join(e.Reasons, "\n  - ")
}

// Drop drops the flux tables, all in one transaction, and records it. It
// refuses unless the phrase is exact, the archive is readable and current,
// every forward node is clean or abandoned, no Control runs on the
// database and a database backup was taken. Tables already gone are
// reported, not an error.
func Drop(ctx context.Context, db *gorm.DB, options DropOptions) (*DropResult, error) {
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	var reasons []string
	if options.Confirm != ConfirmPhrase {
		reasons = append(reasons, fmt.Sprintf("the confirmation phrase is wrong: pass --confirm %q exactly", ConfirmPhrase))
	}
	pre, err := CheckPreconditions(ctx, db, PreconditionOptions{BackupPath: options.BackupPath, Now: now})
	if err != nil {
		return nil, err
	}
	reasons = append(reasons, pre.Blockers...)
	if len(reasons) > 0 {
		return nil, &RefusedError{Reasons: reasons}
	}
	lockTimeout := options.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = DefaultLockTimeout
	}
	backupJSON, _ := json.Marshal(pre.Backup)
	droppedJSON, _ := json.Marshal(pre.Present)
	missingJSON, _ := json.Marshal(pre.Missing)
	record := model.ForwardLegacyDrop{
		ArchiveID: pre.Archive.ID, ArchivePath: pre.Archive.Path, ArchiveSHA256: pre.Archive.SHA256,
		Backup: string(backupJSON), Dropped: string(droppedJSON), AlreadyMissing: string(missingJSON),
		Actor: options.Actor, CreatedAt: now.UTC(),
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			if err := tx.Exec(fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", lockTimeout.Milliseconds())).Error; err != nil {
				return err
			}
		}
		for _, table := range pre.Present {
			if err := tx.Exec("DROP TABLE " + quoteIdent(table)).Error; err != nil {
				return fmt.Errorf("drop %s (nothing was dropped): %w", table, err)
			}
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return &DropResult{Record: record, Dropped: pre.Present, AlreadyMissing: pre.Missing, Backup: pre.Backup}, nil
}

// quoteIdent quotes a table name; DropTables are constants.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
