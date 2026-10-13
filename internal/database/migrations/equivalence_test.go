package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func migrateOptions() migrations.Options {
	return migrations.Options{ProgramVersion: "4.2.1"}
}

func inspect(t *testing.T, db *gorm.DB) *migrations.Schema {
	t.Helper()
	schema, err := migrations.Inspect(context.Background(), db)
	require.NoError(t, err)
	return schema
}

// withoutRunnerTables is schema without v5_schema_migration and
// v5_control_meta, which only the runner creates.
func withoutRunnerTables(schema *migrations.Schema) *migrations.Schema {
	copied := *schema
	copied.Tables = map[string]*migrations.Table{}
	for name, table := range schema.Tables {
		if name != migrations.MigrationTable && name != migrations.MetaTable {
			copied.Tables[name] = table
		}
	}
	return &copied
}

// requireSameSchema fails with the lines that differ: "-" lines are
// missing from got, "+" lines are not in want.
func requireSameSchema(t *testing.T, want, got *migrations.Schema, what string) {
	t.Helper()
	diff := migrations.Diff(want, got)
	if len(diff) > 0 {
		t.Fatalf("%s: the schemas differ (- only in the first, + only in the second):\n%s", what, strings.Join(diff, "\n"))
	}
}

// The CI guard of the migrations and the models. The runner's fresh schema
// is the schema the legacy start builds from today's models (tables,
// columns, types, nullability, defaults, indexes, unique and foreign key
// constraints, and the kernel API views the start adds): a model changed
// without a migration fails here. And the runner adopts a database the
// legacy start built at the baseline (legacyPrepare: today's models less
// what later migrations added) and applies the later migrations: the
// upgraded schema is the fresh one.
func TestFreshSchemaEqualsTheLegacySchema(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		var all []string
		for _, step := range history {
			all = append(all, step.ID())
		}

		fresh := open(t)
		result, err := migrations.Migrate(ctx, fresh, migrateOptions())
		require.NoError(t, err)
		require.True(t, result.Fresh)
		require.Nil(t, result.Adoption)
		require.Equal(t, all, result.Applied)
		require.NoError(t, packagestore.EnsureKernelAPIViews(fresh))
		freshSchema := inspect(t, fresh)

		models := open(t)
		legacyModelsPrepare(t, models, true)
		require.NoError(t, packagestore.EnsureKernelAPIViews(models))
		modelsSchema := inspect(t, models)
		require.NotEmpty(t, modelsSchema.Views)
		requireSameSchema(t, modelsSchema, withoutRunnerTables(freshSchema), "legacy start from today's models (-) and runner (+) on an empty database")

		legacy := open(t)
		legacyPrepare(t, legacy, true)
		upgraded, err := migrations.Migrate(ctx, legacy, migrateOptions())
		require.NoError(t, err)
		require.False(t, upgraded.Fresh)
		require.NotNil(t, upgraded.Adoption, "a database the legacy start built is adopted")
		require.Empty(t, upgraded.Adoption.CreatedTables)
		require.Empty(t, upgraded.Adoption.AddedColumns)
		require.Empty(t, upgraded.Adoption.CreatedIndexes)
		require.Empty(t, upgraded.Adoption.Differences)
		require.Equal(t, all[1:], upgraded.Applied, "the migrations after the baseline are applied after the adoption")
		require.NoError(t, packagestore.EnsureKernelAPIViews(legacy))
		requireSameSchema(t, freshSchema, inspect(t, legacy), "fresh (-) and upgraded (+)")

		// The development start ran the full AutoMigrate: same schema.
		development := open(t)
		legacyPrepare(t, development, false)
		_, err = migrations.Migrate(ctx, development, migrateOptions())
		require.NoError(t, err)
		require.NoError(t, packagestore.EnsureKernelAPIViews(development))
		requireSameSchema(t, freshSchema, inspect(t, development), "fresh (-) and upgraded development database (+)")
	})
}

// Every database a released program left upgrades to the fresh schema: one
// at each step, and the legacy start's own schema (the recorded baseline
// statements without the runner's tables), which is adopted.
func TestUpgradedSchemaEqualsFresh(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		want := freshSchema(t, open)

		legacy := open(t)
		list, err := migrations.Statements(history[0].SQL[dialectName(legacy)])
		require.NoError(t, err)
		for _, statement := range list {
			require.NoError(t, legacy.Exec(statement).Error)
		}
		result, err := migrations.Migrate(ctx, legacy, migrateOptions())
		require.NoError(t, err)
		require.NotNil(t, result.Adoption)
		require.Empty(t, result.Adoption.Differences)
		require.Empty(t, result.Adoption.CreatedTables)
		require.Empty(t, result.Adoption.AddedColumns)
		require.Empty(t, result.Adoption.CreatedIndexes)
		requireSameSchema(t, want, inspect(t, legacy), "fresh (-) and the adopted recorded legacy schema (+)")

		for applied := 1; applied < len(history); applied++ {
			db := open(t)
			_, err := migrations.RunSteps(ctx, db, history[:applied], migrateOptions())
			require.NoError(t, err)
			_, err = migrations.Migrate(ctx, db, migrateOptions())
			require.NoError(t, err)
			requireSameSchema(t, want, inspect(t, db), "fresh (-) and upgraded from "+history[applied-1].ID()+" (+)")
		}
	})
}
