package migrations_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func requireRefused(t *testing.T, err error, contains ...string) {
	t.Helper()
	var refused *migrations.RefusedError
	require.True(t, errors.As(err, &refused), "want a refusal, got %v", err)
	for _, text := range contains {
		assert.Contains(t, refused.Error(), text)
	}
}

func appliedRecords(t *testing.T, db *gorm.DB) []migrations.MigrationRecord {
	t.Helper()
	var records []migrations.MigrationRecord
	require.NoError(t, db.Order("version").Find(&records).Error)
	return records
}

func TestMigrateCreatesTheSchemaAndRecordsIt(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		steps, err := migrations.Steps()
		require.NoError(t, err)
		now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
		var logged []string
		options := migrations.Options{
			ProgramVersion: "4.2.1",
			Now:            func() time.Time { return now },
			Logf:           func(format string, args ...any) { logged = append(logged, format) },
		}

		result, err := migrations.Migrate(ctx, db, options)
		require.NoError(t, err)
		assert.True(t, result.Fresh)
		assert.Equal(t, len(steps), result.SchemaVersion)
		assert.NotEmpty(t, logged)

		records := appliedRecords(t, db)
		require.Len(t, records, len(steps))
		dialect := migrations.DialectSQLite
		if db.Name() == "postgres" {
			dialect = migrations.DialectPostgres
		}
		assert.Equal(t, 1, records[0].Version)
		assert.Equal(t, "baseline", records[0].Name)
		assert.Equal(t, steps[0].Checksum(dialect), records[0].Checksum)
		assert.Equal(t, "4.2.1", records[0].ProgramVersion)
		assert.True(t, records[0].AppliedAt.Equal(now), records[0].AppliedAt)

		meta, err := migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, strconv.Itoa(len(steps)), meta[migrations.MetaSchemaVersion])
		assert.Equal(t, "4.2.1", meta[migrations.MetaProgramVersion])
		assert.Equal(t, "2026-10-13T08:00:00Z", meta[migrations.MetaCreatedAt])
		assert.Equal(t, "fresh", meta[migrations.MetaOrigin])
		assert.NotContains(t, meta, migrations.MetaMinProgramVersion, "no step declares one")

		// The next start applies nothing and records its program version;
		// the creation time stays.
		now = now.Add(time.Hour)
		options.ProgramVersion = "4.2.2"
		again, err := migrations.Migrate(ctx, db, options)
		require.NoError(t, err)
		assert.False(t, again.Fresh)
		assert.Nil(t, again.Adoption)
		assert.Empty(t, again.Applied)
		assert.Len(t, appliedRecords(t, db), len(steps))
		meta, err = migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, "4.2.2", meta[migrations.MetaProgramVersion])
		assert.Equal(t, "2026-10-13T08:00:00Z", meta[migrations.MetaCreatedAt])
	})
}

// An older program refuses a database a newer one migrated: the database
// has a step the older program does not know.
func TestMigrateRefusesADatabaseANewerProgramMigrated(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		_, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NoError(t, db.Create(&migrations.MigrationRecord{
			Version: 2, Name: "from_the_future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "5.1.0",
		}).Error)

		_, err = migrations.Migrate(ctx, db, migrateOptions())
		requireRefused(t, err, "newer AnixOps Control", "0002_from_the_future", "Control 5.1.0", "this program (Control 4.2.1)",
			"knows migrations up to 0001", "restore the backup")
	})
}

// A database that declares a minimum program version refuses older
// programs, even ones that know every applied step.
func TestMigrateRefusesAProgramOlderThanTheMinimum(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		_, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NoError(t, db.Create(&migrations.MetaEntry{Key: migrations.MetaMinProgramVersion, Value: "4.3.0", UpdatedAt: time.Now()}).Error)

		_, err = migrations.Migrate(ctx, db, migrateOptions())
		requireRefused(t, err, "needs AnixOps Control 4.3.0 or newer", "this program is Control 4.2.1", "Install that Control version")

		newer := migrateOptions()
		newer.ProgramVersion = "4.3.0"
		_, err = migrations.Migrate(ctx, db, newer)
		require.NoError(t, err)
	})
}

func TestMigrateRefusesAChangedMigration(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		_, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NoError(t, db.Model(&migrations.MigrationRecord{}).Where("version = ?", 1).Update("checksum", "sha256:edited").Error)

		_, err = migrations.Migrate(ctx, db, migrateOptions())
		requireRefused(t, err, "schema migration 0001", "sha256:edited", "a released migration was changed")

		require.NoError(t, db.Model(&migrations.MigrationRecord{}).Where("version = ?", 1).Updates(map[string]any{
			"checksum": steps(t)[0].Checksum(dialectName(db)), "name": "renamed",
		}).Error)
		_, err = migrations.Migrate(ctx, db, migrateOptions())
		requireRefused(t, err, "recorded as renamed")
	})
}

func steps(t *testing.T) []migrations.Step {
	t.Helper()
	list, err := migrations.Steps()
	require.NoError(t, err)
	return list
}

func dialectName(db *gorm.DB) string {
	if db.Name() == "postgres" {
		return migrations.DialectPostgres
	}
	return migrations.DialectSQLite
}

func TestStepChecksumIgnoresLineEndingsAndMarksGoSteps(t *testing.T) {
	unix := migrations.Step{Version: 1, Name: "x", SQL: map[string]string{"sqlite": "CREATE TABLE a (id integer);\n"}}
	windows := migrations.Step{Version: 1, Name: "x", SQL: map[string]string{"sqlite": "CREATE TABLE a (id integer);\r\n"}}
	assert.Equal(t, unix.Checksum("sqlite"), windows.Checksum("sqlite"))
	withGo := unix
	withGo.Go = func(context.Context, *gorm.DB, string) error { return nil }
	assert.Equal(t, unix.Checksum("sqlite")+"+go", withGo.Checksum("sqlite"))
}

func TestStatementsSplitsAtLineEndSemicolons(t *testing.T) {
	list, err := migrations.Statements("-- a comment\n\nCREATE TABLE a (\n  id integer, -- inline\n  b text DEFAULT ';'\n);\nCREATE INDEX i ON a (id);\n")
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "CREATE TABLE a (\n  id integer, -- inline\n  b text DEFAULT ';'\n)", list[0])
	assert.Equal(t, "CREATE INDEX i ON a (id)", list[1])
	_, err = migrations.Statements("CREATE TABLE a (id integer)")
	require.Error(t, err)
}

func TestLoadStepsValidatesTheFiles(t *testing.T) {
	file := func(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }
	for name, files := range map[string]fstest.MapFS{
		"gap":             {"m/0001_a.sqlite.sql": file(""), "m/0001_a.postgres.sql": file(""), "m/0003_c.sqlite.sql": file(""), "m/0003_c.postgres.sql": file("")},
		"missing dialect": {"m/0001_a.sqlite.sql": file("")},
		"bad name":        {"m/1_a.sqlite.sql": file("")},
		"unknown dialect": {"m/0001_a.mysql.sql": file("")},
		"two names":       {"m/0001_a.sqlite.sql": file(""), "m/0001_b.postgres.sql": file("")},
	} {
		_, err := migrations.LoadSteps(files, "m", nil, nil)
		assert.Error(t, err, name)
	}
	_, err := migrations.LoadSteps(fstest.MapFS{"m/0001_a.sqlite.sql": file(""), "m/0001_a.postgres.sql": file("")}, "m",
		map[int]func(context.Context, *gorm.DB, string) error{2: nil}, nil)
	assert.Error(t, err, "a Go step without SQL files")
}

// templateSteps is a two-step history: the template for a new migration
// (docs/reference/database-migrations.md). 0002 adds a column with SQL and
// fills it with Go in the same transaction.
func templateSteps(t *testing.T, goStep func(context.Context, *gorm.DB, string) error) []migrations.Step {
	t.Helper()
	file := func(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }
	files := fstest.MapFS{
		"sql/0001_baseline.sqlite.sql":            file("CREATE TABLE `v2_widget` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL);\n"),
		"sql/0001_baseline.postgres.sql":          file(`CREATE TABLE "v2_widget" ("id" bigserial,"name" text NOT NULL,PRIMARY KEY ("id"));` + "\n"),
		"sql/0002_widget_display_name.sqlite.sql": file("-- The display name, filled from name.\nALTER TABLE `v2_widget` ADD COLUMN `display_name` text NOT NULL DEFAULT '';\n"),
		"sql/0002_widget_display_name.postgres.sql": file("-- The display name, filled from name.\n" +
			`ALTER TABLE "v2_widget" ADD COLUMN "display_name" text NOT NULL DEFAULT '';` + "\n"),
	}
	list, err := migrations.LoadSteps(files, "sql", map[int]func(context.Context, *gorm.DB, string) error{2: goStep}, map[int]string{2: "4.3.0"})
	require.NoError(t, err)
	return list
}

func TestTemplateMigrationAppliesSQLAndGoInOneTransaction(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		fill := func(_ context.Context, tx *gorm.DB, _ string) error {
			return tx.Exec("UPDATE v2_widget SET display_name = upper(name)").Error
		}
		history := templateSteps(t, fill)

		// A database at 0001 with data, as the previous release left it.
		result, err := migrations.RunSteps(ctx, db, history[:1], migrateOptions())
		require.NoError(t, err)
		require.Equal(t, []string{"0001_baseline"}, result.Applied)
		require.NoError(t, db.Exec("INSERT INTO v2_widget (name) VALUES ('alpha')").Error)

		// The release with 0002 applies it, SQL then Go.
		upgrade := migrations.Options{ProgramVersion: "4.3.0"}
		result, err = migrations.RunSteps(ctx, db, history, upgrade)
		require.NoError(t, err)
		require.Equal(t, []string{"0002_widget_display_name"}, result.Applied)
		var display string
		require.NoError(t, db.Raw("SELECT display_name FROM v2_widget").Scan(&display).Error)
		assert.Equal(t, "ALPHA", display)
		records := appliedRecords(t, db)
		require.Len(t, records, 2)
		assert.Equal(t, "4.3.0", records[1].ProgramVersion)
		assert.Contains(t, records[1].Checksum, "+go")
		meta, err := migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, "2", meta[migrations.MetaSchemaVersion])
		assert.Equal(t, "4.3.0", meta[migrations.MetaMinProgramVersion], "0002 declared it")

		// The previous release now refuses the database.
		_, err = migrations.RunSteps(ctx, db, history[:1], migrateOptions())
		requireRefused(t, err, "0002_widget_display_name", "knows migrations up to 0001")
	})
}

func TestAFailingStepLeavesNoTrace(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		failing := templateSteps(t, func(context.Context, *gorm.DB, string) error { return errors.New("the data does not convert") })
		_, err := migrations.RunSteps(ctx, db, failing[:1], migrateOptions())
		require.NoError(t, err)

		_, err = migrations.RunSteps(ctx, db, failing, migrateOptions())
		require.ErrorContains(t, err, "migration 0002_widget_display_name: the data does not convert")
		assert.False(t, db.Migrator().HasColumn("v2_widget", "display_name"), "the step's SQL is rolled back with it")
		assert.Len(t, appliedRecords(t, db), 1)

		// Every step is its own transaction: on an empty database 0001
		// stays applied and 0002 leaves nothing.
		empty := open(t)
		_, err = migrations.RunSteps(ctx, empty, failing, migrateOptions())
		require.Error(t, err)
		assert.Len(t, appliedRecords(t, empty), 1)
		assert.False(t, empty.Migrator().HasColumn("v2_widget", "display_name"))
	})
}

// A step added below the newest applied one would never run on databases
// that already passed it.
func TestMigrateRefusesAStepAddedOutOfOrder(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t)
	history := templateSteps(t, nil)
	_, err := migrations.RunSteps(ctx, db, history, migrateOptions())
	require.NoError(t, err)
	require.NoError(t, db.Where("version = ?", 1).Delete(&migrations.MigrationRecord{}).Error)
	_, err = migrations.RunSteps(ctx, db, history, migrateOptions())
	requireRefused(t, err, "0001_baseline is not applied, but the later migration 0002 is")
}
