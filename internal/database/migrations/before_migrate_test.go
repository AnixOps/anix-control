package migrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hook runs once, before the first pending step, with what the run is
// about to do; a run with nothing pending does not call it.
func TestBeforeMigrateSeesThePendingSteps(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		all := steps(t)
		var calls []migrations.Pending
		options := migrateOptions()
		options.BeforeMigrate = func(_ context.Context, pending migrations.Pending) error {
			calls = append(calls, pending)
			return nil
		}

		// A database one step behind.
		_, err := migrations.RunSteps(ctx, db, all[:len(all)-1], migrateOptions())
		require.NoError(t, err)
		_, err = migrations.Migrate(ctx, db, options)
		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Equal(t, []string{all[len(all)-1].ID()}, calls[0].Steps)
		assert.False(t, calls[0].Empty)
		assert.False(t, calls[0].Adopt)
		assert.Equal(t, len(all)-1, calls[0].SchemaVersion)

		_, err = migrations.Migrate(ctx, db, options)
		require.NoError(t, err)
		assert.Len(t, calls, 1, "nothing pending: no call")

		// An empty database: every step, Empty.
		calls = nil
		_, err = migrations.Migrate(ctx, open(t), options)
		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Len(t, calls[0].Steps, len(all))
		assert.True(t, calls[0].Empty)
		assert.Zero(t, calls[0].SchemaVersion)
	})
}

// A failing hook (the pre-migration backup) stops the run before anything
// changed.
func TestBeforeMigrateErrorStopsTheRun(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t)
	all := steps(t)
	_, err := migrations.RunSteps(ctx, db, all[:len(all)-1], migrateOptions())
	require.NoError(t, err)
	options := migrateOptions()
	options.BeforeMigrate = func(context.Context, migrations.Pending) error {
		return errors.New("the pre-migration backup failed: disk full")
	}
	_, err = migrations.Migrate(ctx, db, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "before migration "+all[len(all)-1].ID())
	assert.Contains(t, err.Error(), "disk full")
	assert.Len(t, appliedRecords(t, db), len(all)-1, "the pending step was not applied")
}

// Check answers what a start would do, changes nothing, and refuses with
// the start's own messages.
func TestCheckRefusesWithoutChangingTheDatabase(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t)
	all := steps(t)

	status, err := migrations.Check(ctx, db, "4.2.1")
	require.NoError(t, err)
	assert.Zero(t, status.SchemaVersion)
	assert.Len(t, status.Pending, len(all))

	_, err = migrations.RunSteps(ctx, db, all[:len(all)-1], migrateOptions())
	require.NoError(t, err)
	status, err = migrations.Check(ctx, db, "4.2.1")
	require.NoError(t, err)
	assert.Equal(t, len(all)-1, status.SchemaVersion)
	assert.Equal(t, []string{all[len(all)-1].ID()}, status.Pending)
	assert.Len(t, appliedRecords(t, db), len(all)-1, "Check applies nothing")

	_, err = migrations.Migrate(ctx, db, migrateOptions())
	require.NoError(t, err)
	require.NoError(t, db.Create(&migrations.MigrationRecord{
		Version: len(all) + 1, Name: "from_the_future", Checksum: "sha256:00", AppliedAt: time.Now(), ProgramVersion: "5.1.0",
	}).Error)
	_, err = migrations.Check(ctx, db, "4.2.1")
	requireRefused(t, err, "this database was migrated by a newer AnixOps Control", "Control 5.1.0")
}
