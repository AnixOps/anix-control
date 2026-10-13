package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The runner's own tables. They are created with the first step (or the
// adoption) and are not part of any step.
const (
	MigrationTable = "v5_schema_migration"
	MetaTable      = "v5_control_meta"
)

// The keys of v5_control_meta.
const (
	// MetaSchemaVersion is the highest applied step.
	MetaSchemaVersion = "schema_version"
	// MetaProgramVersion is the program version that last migrated or
	// opened the database.
	MetaProgramVersion = "program_version"
	// MetaMinProgramVersion is the oldest program version that may open
	// the database; present once an applied step declared one.
	MetaMinProgramVersion = "min_program_version"
	// MetaCreatedAt is when the runner first prepared the database (RFC
	// 3339, UTC): its creation, or its adoption.
	MetaCreatedAt = "created_at"
	// MetaOrigin is "fresh" for a database the runner created from
	// nothing and "adopted" for one an earlier Control created.
	MetaOrigin = "origin"
)

// MigrationRecord is a row of v5_schema_migration: one applied step.
type MigrationRecord struct {
	Version        int       `gorm:"column:version;primaryKey"`
	Name           string    `gorm:"column:name"`
	Checksum       string    `gorm:"column:checksum"`
	AppliedAt      time.Time `gorm:"column:applied_at"`
	ProgramVersion string    `gorm:"column:program_version"`
}

// TableName is v5_schema_migration.
func (MigrationRecord) TableName() string { return MigrationTable }

// MetaEntry is a row of v5_control_meta.
type MetaEntry struct {
	Key       string    `gorm:"column:key;primaryKey"`
	Value     string    `gorm:"column:value"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName is v5_control_meta.
func (MetaEntry) TableName() string { return MetaTable }

var runnerTables = map[string][]string{
	DialectSQLite: {
		"CREATE TABLE IF NOT EXISTS `v5_schema_migration` (`version` integer PRIMARY KEY,`name` text NOT NULL,`checksum` text NOT NULL,`applied_at` datetime NOT NULL,`program_version` text NOT NULL DEFAULT '')",
		"CREATE TABLE IF NOT EXISTS `v5_control_meta` (`key` text PRIMARY KEY,`value` text NOT NULL,`updated_at` datetime NOT NULL)",
	},
	DialectPostgres: {
		`CREATE TABLE IF NOT EXISTS "v5_schema_migration" ("version" integer PRIMARY KEY,"name" text NOT NULL,"checksum" text NOT NULL,"applied_at" timestamptz NOT NULL,"program_version" text NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS "v5_control_meta" ("key" text PRIMARY KEY,"value" text NOT NULL,"updated_at" timestamptz NOT NULL)`,
	},
}

// Options configure a run.
type Options struct {
	// ProgramVersion is the running program's version (branding version,
	// "4.2.1"). It is recorded with every step and checked against
	// min_program_version.
	ProgramVersion string
	// Logf receives one line per decision; nil discards them.
	Logf func(format string, args ...any)
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Result is what a run did.
type Result struct {
	// Dialect is the database's dialect.
	Dialect string
	// Fresh is true when the run created the schema in an empty database.
	Fresh bool
	// Adoption is set when the run adopted a database an earlier Control
	// created.
	Adoption *Adoption
	// Applied are the IDs of the steps this run applied, in order.
	Applied []string
	// SchemaVersion is the highest applied step after the run.
	SchemaVersion int
}

// RefusedError is a database this program must not open or migrate. Its
// message says why and what to do.
type RefusedError struct {
	Reason string
}

func (e *RefusedError) Error() string { return e.Reason }

// remedy closes every refusal about versions.
const remedy = "Install that Control version or a newer one, or restore the backup taken before the upgrade (docs/UPGRADE.md, \"Database migrations\")."

// Migrate brings db to the newest schema this program knows: it creates
// the schema in an empty database, adopts a database an earlier Control
// created, applies the pending steps and records the program version. It
// refuses a database a newer program migrated, one whose applied steps
// differ from this program's, and an unrecognized partial schema.
func Migrate(ctx context.Context, db *gorm.DB, options Options) (*Result, error) {
	steps, err := Steps()
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return run(ctx, db, steps, options)
}

func run(ctx context.Context, db *gorm.DB, steps []Step, options Options) (*Result, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	dialect, err := dialectOf(db)
	if err != nil {
		return nil, err
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Logf == nil {
		options.Logf = func(string, ...any) {}
	}
	db = db.WithContext(ctx)
	result := &Result{Dialect: dialect}

	applied, err := appliedSteps(db)
	if err != nil {
		return nil, err
	}
	if err := checkApplied(db, dialect, steps, applied, options.ProgramVersion); err != nil {
		return nil, err
	}

	for _, step := range steps {
		if _, done := applied[step.Version]; done {
			continue
		}
		var adoption *Adoption
		var fresh bool
		err := db.Transaction(func(tx *gorm.DB) error {
			// Another process may have applied the step since the
			// check above: SQLite's immediate transactions and the
			// PostgreSQL bootstrap lock serialize the runs, so the
			// answer read here holds until the commit.
			current, err := appliedSteps(tx)
			if err != nil {
				return err
			}
			if _, done := current[step.Version]; done {
				return errAlreadyApplied
			}
			if len(current) == 0 {
				empty, err := isEmpty(tx)
				if err != nil {
					return err
				}
				if !empty {
					if step.Version != 1 {
						return fmt.Errorf("adoption needs the baseline, not %s", step.ID())
					}
					adoption, err = adopt(ctx, tx, step, options)
					if err != nil {
						return err
					}
				} else {
					fresh = true
				}
				for _, statement := range runnerTables[dialect] {
					if err := tx.Exec(statement).Error; err != nil {
						return fmt.Errorf("create %s and %s: %w", MigrationTable, MetaTable, err)
					}
				}
				origin := "fresh"
				if adoption != nil {
					origin = "adopted"
				}
				if err := setMeta(tx, options.Now(), map[string]string{
					MetaCreatedAt: options.Now().UTC().Format(time.RFC3339),
					MetaOrigin:    origin,
				}); err != nil {
					return err
				}
			}
			if adoption == nil {
				if err := apply(ctx, tx, step, dialect); err != nil {
					return err
				}
			}
			record := MigrationRecord{
				Version:        step.Version,
				Name:           step.Name,
				Checksum:       step.Checksum(dialect),
				AppliedAt:      options.Now().UTC(),
				ProgramVersion: options.ProgramVersion,
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", step.ID(), err)
			}
			return setMeta(tx, options.Now(), map[string]string{MetaSchemaVersion: strconv.Itoa(step.Version)})
		})
		if errors.Is(err, errAlreadyApplied) {
			continue
		}
		if err != nil {
			var refused *RefusedError
			if errors.As(err, &refused) {
				return nil, err
			}
			return nil, fmt.Errorf("migration %s: %w", step.ID(), err)
		}
		if fresh {
			result.Fresh = true
		}
		if adoption != nil {
			result.Adoption = adoption
			options.Logf("Database schema: adopted the existing database as %s; %s", step.ID(), adoption.Summary())
			for _, difference := range adoption.Differences {
				options.Logf("Database schema: kept a difference from the baseline: %s", difference)
			}
		} else {
			result.Applied = append(result.Applied, step.ID())
			options.Logf("Database schema: applied migration %s.", step.ID())
		}
	}
	if len(steps) > 0 {
		result.SchemaVersion = steps[len(steps)-1].Version
	}
	if err := recordProgram(db, steps, options); err != nil {
		return nil, err
	}
	if len(result.Applied) == 0 && result.Adoption == nil {
		options.Logf("Database schema: version %d, up to date.", result.SchemaVersion)
	}
	return result, nil
}

var errAlreadyApplied = errors.New("migration already applied")

// apply runs a step's statements and Go function in tx.
func apply(ctx context.Context, tx *gorm.DB, step Step, dialect string) error {
	list, err := statements(step.SQL[dialect])
	if err != nil {
		return err
	}
	for _, statement := range list {
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("%s: %w", abbreviate(statement), err)
		}
	}
	if step.Go != nil {
		if err := step.Go(ctx, tx, dialect); err != nil {
			return err
		}
	}
	return nil
}

func abbreviate(statement string) string {
	statement = strings.Join(strings.Fields(statement), " ")
	if len(statement) > 120 {
		return statement[:117] + "..."
	}
	return statement
}

// tableExists reports whether table is in the schema Control uses. Unlike
// the GORM migrator's HasTable it answers a failed query with the error, not
// with false: a lost connection must not look like a database without
// migrations.
func tableExists(db *gorm.DB, table string) (bool, error) {
	query := "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?"
	if db.Name() == "postgres" {
		query = "SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = ?"
	}
	var count int64
	if err := db.Raw(query, table).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("look for %s: %w", table, err)
	}
	return count > 0, nil
}

// appliedSteps reads v5_schema_migration; a database without it has none.
func appliedSteps(db *gorm.DB) (map[int]MigrationRecord, error) {
	applied := map[int]MigrationRecord{}
	exists, err := tableExists(db, MigrationTable)
	if err != nil || !exists {
		return applied, err
	}
	var records []MigrationRecord
	if err := db.Order("version").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read %s: %w", MigrationTable, err)
	}
	for _, record := range records {
		applied[record.Version] = record
	}
	return applied, nil
}

// checkApplied refuses a database whose applied steps this program does
// not know or knows differently, or that needs a newer program.
func checkApplied(db *gorm.DB, dialect string, steps []Step, applied map[int]MigrationRecord, programVersion string) error {
	known := map[int]Step{}
	for _, step := range steps {
		known[step.Version] = step
	}
	versions := make([]int, 0, len(applied))
	for version := range applied {
		versions = append(versions, version)
	}
	sort.Ints(versions)
	newest := 0
	if len(steps) > 0 {
		newest = steps[len(steps)-1].Version
	}
	for _, version := range versions {
		record := applied[version]
		step, ok := known[version]
		if !ok {
			by := record.ProgramVersion
			if by == "" {
				by = "a newer version"
			}
			return &RefusedError{Reason: fmt.Sprintf(
				"this database was migrated by a newer AnixOps Control: it has schema migration %04d_%s, applied by Control %s, and this program (Control %s) knows migrations up to %04d. %s",
				record.Version, record.Name, by, displayVersion(programVersion), newest, remedy)}
		}
		if record.Name != step.Name || record.Checksum != step.Checksum(dialect) {
			return &RefusedError{Reason: fmt.Sprintf(
				"schema migration %04d is recorded as %s with checksum %s, but this program has %s with checksum %s: a released migration was changed. Migrations are never edited once released; run the release that applied it, or restore a backup, and report this as a bug.",
				version, record.Name, record.Checksum, step.ID(), step.Checksum(dialect))}
		}
	}
	for _, step := range steps {
		if _, ok := applied[step.Version]; ok {
			continue
		}
		for _, version := range versions {
			if version > step.Version {
				return &RefusedError{Reason: fmt.Sprintf(
					"schema migration %s is not applied, but the later migration %04d is: migrations must be added after the newest released one, never between. Restore a backup and report this as a bug.",
					step.ID(), version)}
			}
		}
	}
	minimum, err := metaValue(db, MetaMinProgramVersion)
	if err != nil {
		return err
	}
	if minimum != "" && olderThan(programVersion, minimum) {
		return &RefusedError{Reason: fmt.Sprintf(
			"this database needs AnixOps Control %s or newer (min_program_version in %s), and this program is Control %s. %s",
			minimum, MetaTable, displayVersion(programVersion), remedy)}
	}
	return nil
}

// recordProgram writes the program version that opened the database and
// raises min_program_version to the highest any applied step declares.
func recordProgram(db *gorm.DB, steps []Step, options Options) error {
	values := map[string]string{}
	if options.ProgramVersion != "" {
		values[MetaProgramVersion] = options.ProgramVersion
	}
	minimum, err := metaValue(db, MetaMinProgramVersion)
	if err != nil {
		return err
	}
	raised := minimum
	for _, step := range steps {
		if step.MinProgramVersion != "" && (raised == "" || olderThan(raised, step.MinProgramVersion)) {
			raised = step.MinProgramVersion
		}
	}
	if raised != minimum {
		values[MetaMinProgramVersion] = raised
	}
	if len(values) == 0 {
		return nil
	}
	dialect, err := dialectOf(db)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// The tables exist once a step was recorded; IF NOT EXISTS only
		// matters for a database whose meta table was dropped by hand.
		for _, statement := range runnerTables[dialect] {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return setMeta(tx, options.Now(), values)
	})
}

func setMeta(tx *gorm.DB, now time.Time, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := MetaEntry{Key: key, Value: values[key], UpdatedAt: now.UTC()}
		if key == MetaCreatedAt || key == MetaOrigin {
			// Set once, when the runner first prepares the database.
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
				return fmt.Errorf("write %s: %w", MetaTable, err)
			}
			continue
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&entry).Error; err != nil {
			return fmt.Errorf("write %s: %w", MetaTable, err)
		}
	}
	return nil
}

// metaValue reads one key of v5_control_meta, "" when it or the table is
// missing.
func metaValue(db *gorm.DB, key string) (string, error) {
	exists, err := tableExists(db, MetaTable)
	if err != nil || !exists {
		return "", err
	}
	var entries []MetaEntry
	if err := db.Where("key = ?", key).Limit(1).Find(&entries).Error; err != nil {
		return "", fmt.Errorf("read %s: %w", MetaTable, err)
	}
	if len(entries) == 0 {
		return "", nil
	}
	return entries[0].Value, nil
}

// Meta reads v5_control_meta.
func Meta(ctx context.Context, db *gorm.DB) (map[string]string, error) {
	values := map[string]string{}
	exists, err := tableExists(db.WithContext(ctx), MetaTable)
	if err != nil || !exists {
		return values, err
	}
	var entries []MetaEntry
	if err := db.WithContext(ctx).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("read %s: %w", MetaTable, err)
	}
	for _, entry := range entries {
		values[entry.Key] = entry.Value
	}
	return values, nil
}

// olderThan reports whether version is older than minimum. Versions that
// are not semantic versions (a development build's) are never older.
func olderThan(version, minimum string) bool {
	v, m := "v"+strings.TrimPrefix(version, "v"), "v"+strings.TrimPrefix(minimum, "v")
	if !semver.IsValid(v) || !semver.IsValid(m) {
		return false
	}
	return semver.Compare(v, m) < 0
}

func displayVersion(version string) string {
	if version == "" {
		return "of an unknown version"
	}
	return version
}

func dialectOf(db *gorm.DB) (string, error) {
	switch db.Name() {
	case "sqlite", "sqlite3":
		return DialectSQLite, nil
	case "postgres":
		return DialectPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database dialect %q", db.Name())
	}
}

// isEmpty reports whether the database has no table and no view.
func isEmpty(db *gorm.DB) (bool, error) {
	schema, err := Inspect(db.Statement.Context, db)
	if err != nil {
		return false, err
	}
	return len(schema.Tables) == 0 && len(schema.Views) == 0, nil
}
