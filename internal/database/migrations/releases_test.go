package migrations_test

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The schemas of SQLite databases released Control versions left, in
// testdata/releases: the tag built from source, run once on an empty
// SQLite file (`anix-control migrate`, or the server for 4.0.0, which has
// no migrate command), and sqlite_master's statements dumped in creation
// order (gzip -n). v4.0.0-production is a 4.0.0 production start, which
// skipped AutoMigrate and stopped at the forward runtime settings: a
// partial schema with its anchors. The complete starts of 4.0.0 to 4.2.0
// created v2_node_group, which the baseline no longer has.
var releasedSchemas = map[string][]string{
	"v4.0.0-development": {"table v2_node_group is not in the baseline"},
	"v4.0.0-production":  nil,
	"v4.1.0-production":  {"table v2_node_group is not in the baseline"},
	"v4.2.0-production":  {"table v2_node_group is not in the baseline"},
}

func replayRelease(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "releases", name+".sqlite.sql.gz"))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	list, err := migrations.Statements(string(body))
	require.NoError(t, err)
	for _, statement := range list {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
}

// Every released database is adopted, and then has the fresh schema: the
// tables, columns and indexes it lacked are added, and the obsolete
// v2_node_group stays (kept, reported). The kernel API views the start
// creates are compared too.
func TestAdoptionOfReleasedDatabases(t *testing.T) {
	ctx := context.Background()
	fresh := openSQLite(t)
	_, err := migrations.Migrate(ctx, fresh, migrateOptions())
	require.NoError(t, err)
	require.NoError(t, packagestore.EnsureKernelAPIViews(fresh))
	want := inspect(t, fresh)

	for name, differences := range releasedSchemas {
		t.Run(name, func(t *testing.T) {
			db := openSQLite(t)
			replayRelease(t, db, name)

			result, err := migrations.Migrate(ctx, db, migrateOptions())
			require.NoError(t, err)
			require.NotNil(t, result.Adoption)
			assert.Equal(t, differences, result.Adoption.Differences)
			assert.NotEmpty(t, result.Adoption.AddedColumns, "every release lacks the node group columns")
			require.NoError(t, packagestore.EnsureKernelAPIViews(db))

			var unexpected []string
			for _, line := range migrations.Diff(want, inspect(t, db)) {
				if !strings.Contains(line, "v2_node_group:") && !strings.Contains(line, "v2_node_group.") && line != "+ table v2_node_group" {
					unexpected = append(unexpected, line)
				}
			}
			assert.Empty(t, unexpected, "fresh (-) and adopted %s (+)", name)
		})
	}
}
