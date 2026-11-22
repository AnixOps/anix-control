package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 0040_table_prefix renames the v2_ tables with their data, their id
// sequences and the node secret split's records of them; until it runs,
// TableIn answers the v2_ name.
func TestTablePrefixStepRenamesTheTablesWithTheirData(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		at := stepIndex(t, history, "0040_table_prefix")
		db := open(t)
		_, err := migrations.RunSteps(ctx, db, history[:at], migrateOptions())
		require.NoError(t, err)

		now := time.Now().UTC()
		exec(t, db, "INSERT INTO v2_knowledge (id, title) VALUES (5, 'kept'), (6, 'deleted')", "DELETE FROM v2_knowledge WHERE id = 6")
		require.NoError(t, db.Exec("INSERT INTO v4_kernel_node_secret_split (table_name, phase, finalized_at, updated_at) VALUES ('v2_node', 'finalized', ?, ?), ('v2_wireguard_peer', 'finalized', ?, ?)",
			now, now, now, now).Error)
		if db.Name() == "postgres" {
			exec(t, db, "SELECT setval('v2_knowledge_id_seq', 6)")
		}
		assert.Equal(t, "v2_knowledge", migrations.TableIn(db, "v5_knowledge"))

		_, err = migrations.RunSteps(ctx, db, history[:at+1], migrateOptions())
		require.NoError(t, err)

		assert.Equal(t, "v5_knowledge", migrations.TableIn(db, "v5_knowledge"))
		assert.Equal(t, "v5_line", migrations.TableIn(db, "v5_line"), "a table the step did not rename")
		var titles []string
		require.NoError(t, db.Raw("SELECT title FROM v5_knowledge ORDER BY id").Scan(&titles).Error)
		assert.Equal(t, []string{"kept"}, titles)
		exec(t, db, "INSERT INTO v5_knowledge (title) VALUES ('new')")
		var id int
		require.NoError(t, db.Raw("SELECT id FROM v5_knowledge WHERE title = 'new'").Scan(&id).Error)
		assert.Equal(t, 7, id, "the id sequence continues")

		var split []string
		require.NoError(t, db.Raw("SELECT table_name FROM v4_kernel_node_secret_split ORDER BY table_name").Scan(&split).Error)
		assert.Equal(t, []string{"v5_node", "v5_wireguard_peer"}, split)

		var left []string
		if db.Name() == "postgres" {
			require.NoError(t, db.Raw(`SELECT relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = current_schema() AND (relname LIKE 'v2\_%' OR relname LIKE 'idx\_v2\_%')
				UNION ALL SELECT conname FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
				WHERE n.nspname = current_schema() AND (conname LIKE 'v2\_%' OR conname LIKE 'fk\_v2\_%')`).Scan(&left).Error)
		} else {
			require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE name LIKE 'v2\_%' ESCAPE '\' OR name LIKE 'idx\_v2\_%' ESCAPE '\'`).Scan(&left).Error)
		}
		assert.Empty(t, left, "nothing keeps the v2_ prefix")
	})
}
