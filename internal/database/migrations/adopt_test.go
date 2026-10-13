package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func exec(t *testing.T, db *gorm.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
}

// freshSchema is the schema the runner builds in an empty database.
func freshSchema(t *testing.T, open opener) *migrations.Schema {
	t.Helper()
	db := open(t)
	_, err := migrations.Migrate(context.Background(), db, migrateOptions())
	require.NoError(t, err)
	return inspect(t, db)
}

// olderRelease turns a database the legacy start built into one an older
// 4.x left: before the node groups (the node columns and v5_node_group),
// the admin API tokens, alerts, worker leases and diagnostic tasks, with a
// narrower v2_user and v2_server_log (production never added columns to
// existing tables), the plain unique topology name index topologies had
// before they could be deleted, and no partial index on active flux
// runtime jobs, with three active jobs for one forward.
func olderRelease(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec(t, db,
		"DROP INDEX idx_v2_node_machine_fingerprint",
		"DROP INDEX idx_v2_node_node_group_id",
		"ALTER TABLE v2_node DROP COLUMN node_group_id",
		"ALTER TABLE v2_node DROP COLUMN machine_fingerprint",
		"ALTER TABLE v2_node DROP COLUMN approval",
		"ALTER TABLE v2_node DROP COLUMN last_public_address",
		"DROP INDEX idx_v2_forward_node_machine_fingerprint",
		"DROP INDEX idx_v2_forward_node_node_group_id",
		"ALTER TABLE v2_forward_node DROP COLUMN metrics_port",
		"ALTER TABLE v2_forward_node DROP COLUMN node_group_id",
		"ALTER TABLE v2_forward_node DROP COLUMN machine_fingerprint",
		"ALTER TABLE v2_forward_node DROP COLUMN approval",
		"ALTER TABLE v2_forward_node DROP COLUMN last_public_address",
		"DROP INDEX idx_v2_user_token",
		"ALTER TABLE v2_user DROP COLUMN token",
		"DROP INDEX idx_server_log_log_at_user_id",
		"DROP INDEX idx_v2_server_log_log_at",
		"ALTER TABLE v2_server_log DROP COLUMN log_at",
		"ALTER TABLE v2_server_log DROP COLUMN rate",
		"DROP TABLE v5_node_group",
		"DROP TABLE v4_kernel_admin_api_token",
		"DROP TABLE v4_kernel_alert",
		"DROP TABLE v4_kernel_lease",
		"DROP TABLE v2_agent_diagnostic_task",
		"DROP INDEX ux_topology_live_name",
		"CREATE UNIQUE INDEX idx_v3_kernel_topology_name ON v3_kernel_topology (name)",
		"DROP INDEX idx_forward_runtime_job_active_forward",
		"INSERT INTO v2_node (name, host) VALUES ('proxy', '203.0.113.4')",
		"INSERT INTO v2_forward_node (name, type, host, port) VALUES ('relay', 'exit', '203.0.113.3', 0)",
	)
	for _, status := range []int{model.ForwardRuntimeJobStatusPending, model.ForwardRuntimeJobStatusRunning, model.ForwardRuntimeJobStatusPending} {
		forwardID := uint(42)
		require.NoError(t, db.Create(&model.ForwardRuntimeJob{
			Backend: model.ForwardRuntimeBackendNftablesAnsible, Action: model.ForwardRuntimeJobActionPause, ForwardID: &forwardID, Status: status,
		}).Error)
	}
}

// A database an older 4.x left is completed to the baseline the way the
// start before the runner did, and then has exactly the fresh schema.
func TestAdoptionCompletesADatabaseAnOlderReleaseLeft(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		want := freshSchema(t, open)
		db := open(t)
		legacyPrepare(t, db, true)
		olderRelease(t, db)

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		adoption := result.Adoption
		require.NotNil(t, adoption)
		assert.ElementsMatch(t, []string{"v2_agent_diagnostic_task", "v4_kernel_admin_api_token", "v4_kernel_alert", "v4_kernel_lease", "v5_node_group"}, adoption.CreatedTables)
		assert.ElementsMatch(t, []string{
			"v2_node.node_group_id", "v2_node.machine_fingerprint", "v2_node.approval", "v2_node.last_public_address",
			"v2_forward_node.metrics_port", "v2_forward_node.node_group_id", "v2_forward_node.machine_fingerprint",
			"v2_forward_node.approval", "v2_forward_node.last_public_address",
			"v2_user.token", "v2_server_log.rate", "v2_server_log.log_at",
		}, adoption.AddedColumns)
		assert.Equal(t, []string{model.LegacyTopologyNameIndex}, adoption.DroppedIndexes)
		assert.Contains(t, adoption.CreatedIndexes, "idx_forward_runtime_job_active_forward")
		assert.Contains(t, adoption.CreatedIndexes, model.TopologyLiveNameIndex)
		assert.Contains(t, adoption.CreatedIndexes, model.NodeGroupLiveNameIndex)
		assert.EqualValues(t, 2, adoption.SupersededJobs)
		assert.Empty(t, adoption.Differences)
		requireSameSchema(t, want, inspect(t, db), "fresh (-) and adopted older release (+)")

		// Existing nodes are approved (the column default), one active
		// job per forward is left, and the others failed.
		var proxy, relay string
		require.NoError(t, db.Raw("SELECT approval FROM v2_node WHERE name = 'proxy'").Scan(&proxy).Error)
		require.NoError(t, db.Raw("SELECT approval FROM v2_forward_node WHERE name = 'relay'").Scan(&relay).Error)
		assert.Equal(t, model.NodeApprovalApproved, proxy)
		assert.Equal(t, model.NodeApprovalApproved, relay)
		var active []model.ForwardRuntimeJob
		require.NoError(t, db.Where("forward_id = 42 AND status IN ?", []int{0, 1}).Find(&active).Error)
		require.Len(t, active, 1)
		assert.Equal(t, model.ForwardRuntimeJobStatusRunning, active[0].Status)

		meta, err := migrations.Meta(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, "adopted", meta[migrations.MetaOrigin])
		assert.Equal(t, "1", meta[migrations.MetaSchemaVersion])

		again, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		assert.Nil(t, again.Adoption, "adoption happens once")
	})
}

// After `forward legacy drop` the flux tables stay dropped: adoption does
// not create them again.
func TestAdoptionKeepsTheFluxTablesDropped(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		legacyPrepare(t, db, true)
		require.NoError(t, db.Create(&model.ForwardLegacyDrop{ArchivePath: "/var/lib/archive.json", Actor: "test", CreatedAt: time.Now()}).Error)
		for _, table := range forwardlegacy.DropTables {
			require.NoError(t, db.Migrator().DropTable(table))
		}
		// An older release's start also lacked a column of a kept table.
		exec(t, db, "DROP INDEX idx_v2_forward_node_node_group_id", "ALTER TABLE v2_forward_node DROP COLUMN node_group_id")

		result, err := migrations.Migrate(ctx, db, migrateOptions())
		require.NoError(t, err)
		require.NotNil(t, result.Adoption)
		assert.ElementsMatch(t, forwardlegacy.DropTables, result.Adoption.SkippedTables)
		assert.Empty(t, result.Adoption.CreatedTables)
		assert.Equal(t, []string{"v2_forward_node.node_group_id"}, result.Adoption.AddedColumns)
		assert.Empty(t, result.Adoption.Differences)
		for _, table := range forwardlegacy.DropTables {
			assert.False(t, db.Migrator().HasTable(table), table)
		}
		assert.True(t, db.Migrator().HasTable("v2_forward_node"))
	})
}

// A database that is not one a Control start prepared is refused with the
// list of what it lacks, and left as it was.
func TestAdoptionRefusesAnUnrecognizedPartialSchema(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		partial := open(t)
		// The shape of the SQLite to PostgreSQL dry-run fixture.
		exec(t, partial,
			"CREATE TABLE v2_user (id INTEGER PRIMARY KEY, email TEXT, token TEXT)",
			"CREATE TABLE v2_plan (id INTEGER PRIMARY KEY, name TEXT)",
		)
		_, err := migrations.Migrate(ctx, partial, migrateOptions())
		requireRefused(t, err, "not a database an AnixOps Control start prepared", "2 tables",
			"lacks the tables every Control 4.x start creates: v2_node, v2_wireguard_peer, v3_kernel_access_group",
			"v3_kernel_topology_vertex", "of the other baseline tables it lacks", "v2_order", "v4_kernel_lease",
			"Nothing was changed")
		assert.False(t, partial.Migrator().HasTable(migrations.MigrationTable))
		assert.False(t, partial.Migrator().HasTable("v2_order"))
		assert.False(t, partial.Migrator().HasColumn("v2_user", "uuid"))

		unrelated := open(t)
		exec(t, unrelated, "CREATE TABLE wp_posts (id INTEGER PRIMARY KEY)")
		_, err = migrations.Migrate(ctx, unrelated, migrateOptions())
		requireRefused(t, err, "1 tables", "v2_user")
		assert.False(t, unrelated.Migrator().HasTable("v2_user"))

		// A Control database with its anchors that lost other tables is
		// completed instead (the start before the runner did that too).
		damaged := open(t)
		legacyPrepare(t, damaged, true)
		exec(t, damaged, "DROP TABLE v2_ticket_message", "DROP TABLE v2_ticket")
		result, err := migrations.Migrate(ctx, damaged, migrateOptions())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"v2_ticket", "v2_ticket_message"}, result.Adoption.CreatedTables)
	})
}

// When completing fails (here: duplicate codes where the baseline has a
// unique index), adoption refuses and changes nothing.
func TestAdoptionThatCannotCompleteChangesNothing(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		db := open(t)
		legacyPrepare(t, db, true)
		exec(t, db,
			"DROP INDEX idx_v2_invite_code_code",
			"DROP TABLE v2_ticket_message",
			"INSERT INTO v2_invite_code (user_id, code, status) VALUES (1, 'same', 0)",
			"INSERT INTO v2_invite_code (user_id, code, status) VALUES (2, 'same', 0)",
		)
		_, err := migrations.Migrate(ctx, db, migrateOptions())
		requireRefused(t, err, "adopting the existing database failed at", "idx_v2_invite_code_code", "Nothing was changed")
		assert.False(t, db.Migrator().HasTable(migrations.MigrationTable))
		assert.False(t, db.Migrator().HasTable("v2_ticket_message"), "the tables created before the failure are rolled back")
	})
}

// A database whose columns differ from the baseline in ways only a
// rebuild could change (a type, a default) is adopted; the differences are
// reported and kept.
func TestAdoptionKeepsAndReportsDifferences(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t)
	legacyPrepare(t, db, true)
	exec(t, db,
		"DROP TABLE v2_knowledge",
		"CREATE TABLE v2_knowledge (id integer PRIMARY KEY AUTOINCREMENT, category varchar(64), title text NOT NULL DEFAULT '', extra text)",
		"CREATE TABLE v2_node_group_legacy (id integer PRIMARY KEY)",
	)
	result, err := migrations.Migrate(ctx, db, migrateOptions())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"v2_knowledge.body", "v2_knowledge.sort", "v2_knowledge.show", "v2_knowledge.created_at", "v2_knowledge.updated_at"},
		result.Adoption.AddedColumns)
	assert.ElementsMatch(t, []string{
		`column v2_knowledge.category is "category varchar(64)", the baseline has "category text"`,
		`column v2_knowledge.title is "title text NOT NULL DEFAULT ''", the baseline has "title text"`,
		"column v2_knowledge.extra is not in the baseline",
		"table v2_node_group_legacy is not in the baseline",
	}, result.Adoption.Differences)
}

// A column the database cannot add to an existing table (NOT NULL without
// a default, on a table with rows) refuses adoption with the statement and the reason.
func TestAdoptionRefusesAColumnItCannotAdd(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t)
	legacyPrepare(t, db, true)
	exec(t, db,
		"DROP TABLE v4_kernel_alert",
		"CREATE TABLE v4_kernel_alert (id integer PRIMARY KEY AUTOINCREMENT, alert_key text NOT NULL)",
		"INSERT INTO v4_kernel_alert (alert_key) VALUES ('node_offline:4')",
	)
	_, err := migrations.Migrate(ctx, db, migrateOptions())
	requireRefused(t, err, "adopting the existing database failed at", "ALTER TABLE `v4_kernel_alert` ADD COLUMN `kind` text NOT NULL",
		"Cannot add a NOT NULL column", "Nothing was changed")
	assert.False(t, db.Migrator().HasTable(migrations.MigrationTable))
}
