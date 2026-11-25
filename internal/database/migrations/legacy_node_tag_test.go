package migrations_test

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 0053_drop_legacy_node_tag clears the tag "legacy-node" and leaves every
// other tag alone.
func TestDropLegacyNodeTagStep(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		at := stepIndex(t, history, "0053_drop_legacy_node_tag")
		db := open(t)
		_, err := migrations.RunSteps(ctx, db, history[:at], migrateOptions())
		require.NoError(t, err)
		exec(t, db, `INSERT INTO v5_node (id, name, tags) VALUES
			(1, 'a', 'legacy-node'), (2, 'b', '["legacy-node"]'), (3, 'c', '["hk","legacy-node"]'), (4, 'd', '["hk"]'), (5, 'e', NULL)`)

		_, err = migrations.RunSteps(ctx, db, history[:at+1], migrateOptions())
		require.NoError(t, err)

		var tags []string
		require.NoError(t, db.Raw(`SELECT COALESCE(tags, '') FROM v5_node ORDER BY id`).Scan(&tags).Error)
		require.Len(t, tags, 5)
		assert.Empty(t, tags[0])
		assert.Empty(t, tags[1])
		assert.Equal(t, `["hk","legacy-node"]`, tags[2], "a node with other tags keeps them all")
		assert.Equal(t, `["hk"]`, tags[3])
		assert.Empty(t, tags[4])
	})
}
