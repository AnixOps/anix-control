package packagestoresdk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// MigrationIndexFormat is the format of migrations/index.json.
	MigrationIndexFormat = "anixops.migrations/v1"
	// PrefixPlaceholder in a migration script expands to Store.Prefix.
	PrefixPlaceholder = "__PKG_PREFIX__"
	// StateTable records applied steps in the package's own storage.
	StateTable = "schema_migrations"
)

// ErrMigrationChanged means a step that was already applied now has a
// different digest. Applied migrations are immutable; ship a new step.
var ErrMigrationChanged = errors.New("an applied package migration changed")

var migrationIDPattern = regexp.MustCompile(`^[0-9a-z][0-9a-z_]{0,63}$`)

type migrationIndex struct {
	Format     string           `json:"format"`
	PackageID  string           `json:"package_id"`
	Version    string           `json:"version"`
	Migrations []migrationEntry `json:"migrations"`
}

type migrationEntry struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

// MigrationResult reports a migration run.
type MigrationResult struct {
	// Applied lists the steps this run applied, in order.
	Applied []string
	// StateDigest is the SHA-256 of every applied step ("id:sha256\n",
	// sorted by id) after the run.
	StateDigest string
}

// RunEmbeddedMigrations applies the steps of the migration index at
// indexPath in fsys that are not applied yet. Step paths are relative to the
// root of fsys, as in a package artifact. A step's digest is checked against
// the index when the index lists one. Each step runs in its own transaction
// together with its state row; __PKG_PREFIX__ in the script expands to
// store.Prefix().
func RunEmbeddedMigrations(ctx context.Context, store *Store, fsys fs.FS, indexPath string) (MigrationResult, error) {
	if store == nil || store.DB == nil {
		return MigrationResult{}, errors.New("package storage is not open")
	}
	steps, err := loadMigrationSteps(fsys, indexPath)
	if err != nil {
		return MigrationResult{}, err
	}
	db := store.DB.WithContext(ctx)
	state := store.Table(StateTable)
	if err := db.Exec("CREATE TABLE IF NOT EXISTS " + state + " (id VARCHAR(64) PRIMARY KEY, sha256 VARCHAR(64) NOT NULL, applied_at BIGINT NOT NULL)").Error; err != nil {
		return MigrationResult{}, fmt.Errorf("create migration state: %w", err)
	}
	var result MigrationResult
	for _, step := range steps {
		applied, err := applyMigrationStep(db, store, state, step)
		if err != nil {
			return result, fmt.Errorf("migration %s: %w", step.id, err)
		}
		if applied {
			result.Applied = append(result.Applied, step.id)
		}
	}
	result.StateDigest, err = MigrationStateDigest(ctx, store)
	return result, err
}

type migrationStep struct {
	id, digest string
	script     []byte
}

func loadMigrationSteps(fsys fs.FS, indexPath string) ([]migrationStep, error) {
	raw, err := fs.ReadFile(fsys, indexPath)
	if err != nil {
		return nil, fmt.Errorf("read migration index: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var index migrationIndex
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("decode migration index: %w", err)
	}
	if index.Format != MigrationIndexFormat {
		return nil, fmt.Errorf("migration index format must be %s", MigrationIndexFormat)
	}
	seen := make(map[string]bool, len(index.Migrations))
	steps := make([]migrationStep, 0, len(index.Migrations))
	for _, entry := range index.Migrations {
		if !migrationIDPattern.MatchString(entry.ID) || seen[entry.ID] {
			return nil, fmt.Errorf("migration id %q is invalid or repeated", entry.ID)
		}
		seen[entry.ID] = true
		clean := path.Clean(entry.Path)
		if clean != entry.Path || !strings.HasPrefix(clean, "migrations/") || !fs.ValidPath(clean) {
			return nil, fmt.Errorf("migration %s: path %q must stay under migrations/", entry.ID, entry.Path)
		}
		script, err := fs.ReadFile(fsys, clean)
		if err != nil {
			return nil, fmt.Errorf("migration %s: %w", entry.ID, err)
		}
		sum := sha256.Sum256(script)
		digest := hex.EncodeToString(sum[:])
		if entry.SHA256 != "" && !strings.EqualFold(entry.SHA256, digest) {
			return nil, fmt.Errorf("migration %s: script digest does not match the index", entry.ID)
		}
		steps = append(steps, migrationStep{id: entry.ID, digest: digest, script: script})
	}
	return steps, nil
}

type migrationStateRow struct {
	ID        string `gorm:"column:id"`
	SHA256    string `gorm:"column:sha256"`
	AppliedAt int64  `gorm:"column:applied_at"`
}

func applyMigrationStep(db *gorm.DB, store *Store, state string, step migrationStep) (bool, error) {
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if store.Lease.Driver == "postgres" {
			// Serialize concurrent hosts of the same package.
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", state).Error; err != nil {
				return err
			}
		}
		var rows []migrationStateRow
		if err := tx.Table(state).Where("id = ?", step.id).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if !strings.EqualFold(rows[0].SHA256, step.digest) {
				return ErrMigrationChanged
			}
			return nil
		}
		script := strings.ReplaceAll(string(step.script), PrefixPlaceholder, store.Prefix())
		if err := tx.Exec(script).Error; err != nil {
			return err
		}
		applied = true
		return tx.Table(state).Create(&migrationStateRow{ID: step.id, SHA256: step.digest, AppliedAt: time.Now().Unix()}).Error
	})
	return applied, err
}

// MigrationStateDigest summarizes the applied steps: SHA-256 over
// "id:sha256\n" lines sorted by id. Before any step ran it is the SHA-256
// of empty input.
func MigrationStateDigest(ctx context.Context, store *Store) (string, error) {
	var rows []migrationStateRow
	if err := store.DB.WithContext(ctx).Table(store.Table(StateTable)).Find(&rows).Error; err != nil {
		return "", err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	hash := sha256.New()
	for _, row := range rows {
		_, _ = fmt.Fprintf(hash, "%s:%s\n", row.ID, strings.ToLower(row.SHA256))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
