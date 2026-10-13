package migrations_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const bucketsStep = "0002_server_log_buckets"

// afterTheBaseline are the IDs of the steps after 0001_baseline, the
// buckets step first: what a database at the baseline applies.
func afterTheBaseline(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, step := range steps(t)[1:] {
		ids = append(ids, step.ID())
	}
	require.Equal(t, bucketsStep, ids[0])
	return ids
}

// serverLogRow is a v2_server_log row as the tests read it.
type serverLogRow struct {
	ID         uint
	UserID     *uint
	ServerID   *uint
	ServerType *string
	U          *int64
	D          *int64
	Rate       *float64
	LogAt      *int64
}

// insertServerLog adds one row the way an older Control wrote it: a row per
// report, at the report's second. Pass nil for a NULL.
func insertServerLog(t *testing.T, db *gorm.DB, user, server, serverType, u, d, rate, logAt any) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO v2_server_log (user_id, server_id, server_type, u, d, rate, log_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		user, server, serverType, u, d, rate, logAt).Error)
}

// seedBucketRows writes the rows the tests expect the step to merge: three
// of one bucket key, a key of another rate, another user, a report's second
// that is not on the hour, and rows with a NULL in the key.
func seedBucketRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	insertServerLog(t, db, 1, 7, "vmess", 10, 20, 1.0, 3600)   // 1: oldest of its key
	insertServerLog(t, db, 1, 7, "vmess", 1, 2, 1.0, 3600)     // 2
	insertServerLog(t, db, 1, 7, "vmess", 100, 200, 1.0, 3600) // 3
	insertServerLog(t, db, 1, 7, "vmess", 5, 6, 2.0, 3600)     // 4: another rate
	insertServerLog(t, db, 2, 7, "vmess", 7, 8, 1.0, 3600)     // 5: another user
	insertServerLog(t, db, 1, 7, "vmess", 9, 9, 1.0, 3601)     // 6: not on the hour
	insertServerLog(t, db, 1, 7, nil, 3, 3, 1.0, 3600)         // 7: NULL server type
	insertServerLog(t, db, 1, 7, nil, 4, 4, 1.0, 3600)         // 8: the same key with NULL
	insertServerLog(t, db, 3, 8, "vmess", 1, 1, 1.0, nil)      // 9: NULL log_at
	insertServerLog(t, db, 3, 8, "vmess", 1, 1, 1.0, nil)      // 10
}

// requireMergedBucketRows checks what the step leaves of seedBucketRows.
func requireMergedBucketRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	var rows []serverLogRow
	require.NoError(t, db.Table("v2_server_log").Order("id").Find(&rows).Error)
	ids := make([]uint, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	require.Equal(t, []uint{1, 4, 5, 6, 7, 8, 9, 10}, ids, "rows 2 and 3 are merged into row 1; rows with a NULL in the key stay")
	assert.Equal(t, int64(111), *rows[0].U)
	assert.Equal(t, int64(222), *rows[0].D)
	assert.Equal(t, int64(5), *rows[1].U, "another rate is another bucket")
	assert.Equal(t, int64(7), *rows[2].U)
	assert.Equal(t, int64(3601), *rows[3].LogAt, "rows are not moved to the hour")
	assert.Equal(t, int64(3), *rows[4].U)
	assert.Equal(t, int64(4), *rows[5].U)

	assert.True(t, db.Migrator().HasIndex("v2_server_log", "idx_v2_server_log_bucket"))
	// The unique index is enforced: a row of an existing key is refused.
	err := db.Exec("INSERT INTO v2_server_log (user_id, server_id, server_type, u, d, rate, log_at) VALUES (1, 7, 'vmess', 1, 1, 1, 3600)").Error
	require.Error(t, err)
}

// The step merges the rows of one bucket key before it creates the unique
// index, in the transaction of the step, and leaves rows with a NULL key.
func TestServerLogBucketsStepMergesDuplicatesBeforeCreatingTheIndex(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		require.Equal(t, bucketsStep, history[1].ID())
		require.Nil(t, history[1].Go, "the merge is set-based SQL: the statements run in order, the index last")

		db := open(t)
		_, err := migrations.RunSteps(ctx, db, history[:1], migrateOptions())
		require.NoError(t, err)
		seedBucketRows(t, db)

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.Nil(t, result.Adoption)
		require.Equal(t, afterTheBaseline(t), result.Applied)
		requireMergedBucketRows(t, db)

		records := appliedRecords(t, db)
		require.Len(t, records, len(history))
		assert.Equal(t, "server_log_buckets", records[1].Name)

		again, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		assert.Empty(t, again.Applied, "applied once")
		requireMergedBucketRows(t, db)
	})
}

// A database a legacy start built (adopted as the baseline) with duplicate
// rows and rows with NULL keys gets the step after the adoption.
func TestAdoptedDatabaseWithDuplicateServerLogRowsGetsTheBucketsStep(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		legacyPrepare(t, db, true)
		require.False(t, db.Migrator().HasIndex("v2_server_log", "idx_v2_server_log_bucket"), "the legacy start of the baseline had no bucket index")
		seedBucketRows(t, db)

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NotNil(t, result.Adoption, "the database is adopted first")
		require.Empty(t, result.Adoption.Differences)
		require.Equal(t, afterTheBaseline(t), result.Applied, "and the later step is applied after the adoption")
		requireMergedBucketRows(t, db)

		meta, err := migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, "adopted", meta[migrations.MetaOrigin])
		assert.Equal(t, strconv.Itoa(len(steps(t))), meta[migrations.MetaSchemaVersion])
		records := appliedRecords(t, db)
		require.Len(t, records, len(steps(t)))
		assert.Equal(t, "baseline", records[0].Name)
		assert.Equal(t, "server_log_buckets", records[1].Name)
	})
}

// A key column an older release left out is added by the adoption, and its
// default can make rows share a key: the step then merges them.
func TestAdoptedDatabaseOfAnOlderReleaseGetsTheKeyColumnBeforeTheBucketsStep(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		legacyPrepare(t, db, true)
		// An older release's table has no rate column.
		exec(t, db, "ALTER TABLE v2_server_log DROP COLUMN rate")
		for i := 0; i < 3; i++ {
			exec(t, db, "INSERT INTO v2_server_log (user_id, server_id, server_type, u, d, log_at) VALUES (1, 7, 'vmess', 10, 20, 7200)")
		}
		exec(t, db, "INSERT INTO v2_server_log (user_id, server_id, server_type, u, d, log_at) VALUES (1, 7, 'vmess', 1, 1, 10800)")

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NotNil(t, result.Adoption)
		assert.Equal(t, []string{"v2_server_log.rate"}, result.Adoption.AddedColumns)
		require.Equal(t, afterTheBaseline(t), result.Applied)

		// The added column's default (1) gave the three rows one key.
		var rows []serverLogRow
		require.NoError(t, db.Table("v2_server_log").Order("id").Find(&rows).Error)
		require.Len(t, rows, 2)
		assert.Equal(t, int64(30), *rows[0].U)
		assert.Equal(t, int64(60), *rows[0].D)
		assert.InDelta(t, 1.0, *rows[0].Rate, 0)
		assert.Equal(t, int64(1), *rows[1].U)
		assert.True(t, db.Migrator().HasIndex("v2_server_log", "idx_v2_server_log_bucket"))
	})
}

// Every database a released program left, adopted, with duplicate and NULL
// key rows in v2_server_log where the release has the table.
func TestReleasedDatabasesWithDuplicateServerLogRowsGetTheBucketsStep(t *testing.T) {
	for name := range releasedSchemas {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := openSQLite(t)
			replayRelease(t, db, name)
			hasTable := db.Migrator().HasTable("v2_server_log")
			if hasTable {
				seedBucketRows(t, db)
			}

			result, err := migrations.Migrate(ctx, db, migrateOptions())
			require.NoError(t, err)
			require.NotNil(t, result.Adoption)
			require.Equal(t, afterTheBaseline(t), result.Applied)
			if hasTable {
				requireMergedBucketRows(t, db)
			} else {
				assert.True(t, db.Migrator().HasIndex("v2_server_log", "idx_v2_server_log_bucket"),
					"a partial schema gets the table from the adoption and the index from the step")
			}
		})
	}
}

// The traffic writers' upsert needs exactly the index the step creates.
func TestServerLogBucketIndexIsTheUpsertTarget(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		db := open(t)
		_, err := migrations.Migrate(context.Background(), db, migrateOptions())
		require.NoError(t, err)
		upsert := func(u int64) error {
			return db.Exec(fmt.Sprintf("INSERT INTO v2_server_log (user_id, server_id, server_type, u, d, rate, log_at) VALUES (1, 7, 'node', %d, 0, 1, 7200) "+
				"ON CONFLICT (server_id, log_at, user_id, server_type, rate) DO UPDATE SET u = v2_server_log.u + excluded.u", u)).Error
		}
		require.NoError(t, upsert(5))
		require.NoError(t, upsert(6))
		var u int64
		require.NoError(t, db.Raw("SELECT u FROM v2_server_log WHERE log_at = 7200").Scan(&u).Error)
		assert.Equal(t, int64(11), u)
	})
}
