package packagecompat

import (
	"os"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A shared database starts every case as a new one would: no rows, ids
// from 1, and nothing a previous case added or changed in the schema.
func TestSharedDatabasesStartEachCaseEmpty(t *testing.T) {
	backends := []string{backendSQLite}
	if strings.TrimSpace(os.Getenv(PostgresDSNEnvironment)) != "" {
		backends = append(backends, backendPostgres)
	}
	models := []any{&model.User{}, &model.Order{}}
	for _, backend := range backends {
		open := poolFor(t).opener(backend)
		var first *gorm.DB
		step := func(name string, body func(t *testing.T, db *gorm.DB)) {
			t.Run(backend+"/"+name, func(t *testing.T) {
				_, db, migrated := open(t, "shared", models)
				require.True(t, migrated)
				body(t, db)
			})
		}

		step("a case writes rows and adds a table and a view", func(t *testing.T, db *gorm.DB) {
			first = db
			require.NoError(t, db.Create(&model.User{Email: "a@example.test", Token: "token-a", UUID: "uuid-a"}).Error)
			require.NoError(t, db.Create(&model.User{Email: "b@example.test", Token: "token-b", UUID: "uuid-b"}).Error)
			require.NoError(t, db.Exec("CREATE TABLE extra_rows (id INTEGER)").Error)
			require.NoError(t, db.Exec("CREATE VIEW extra_users AS SELECT id FROM v2_user").Error)
		})
		step("the next case finds none of it", func(t *testing.T, db *gorm.DB) {
			require.NotSame(t, first, db, "each case gets its own handle")
			var users int64
			require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
			require.Zero(t, users)
			require.False(t, db.Migrator().HasTable("extra_rows"))
			require.False(t, db.Migrator().HasTable("extra_users"))
			user := model.User{Email: "c@example.test", Token: "token-c", UUID: "uuid-c"}
			require.NoError(t, db.Create(&user).Error)
			require.EqualValues(t, 1, user.ID, "ids start from 1 again")
		})
		step("a case changes the migrated schema", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Exec("ALTER TABLE v2_order ADD COLUMN extra_column INTEGER").Error)
			if backend == backendPostgres {
				require.NoError(t, db.Exec("ALTER TABLE v2_order DROP CONSTRAINT IF EXISTS fk_v2_order_user").Error)
			}
		})
		step("the next case gets the migrated schema", func(t *testing.T, db *gorm.DB) {
			require.False(t, db.Migrator().HasColumn(&model.Order{}, "extra_column"))
			if backend == backendPostgres {
				require.True(t, db.Migrator().HasConstraint(&model.Order{}, "fk_v2_order_user"))
			}
		})
		step("a case drops a foreign key only", func(t *testing.T, db *gorm.DB) {
			if backend == backendPostgres {
				require.NoError(t, db.Exec("ALTER TABLE v2_order DROP CONSTRAINT fk_v2_order_user").Error)
			}
		})
		step("the next case has it back", func(t *testing.T, db *gorm.DB) {
			if backend == backendPostgres {
				require.True(t, db.Migrator().HasConstraint(&model.Order{}, "fk_v2_order_user"))
			}
		})
	}
}

func TestChildrenFirstOrdersByReference(t *testing.T) {
	edges := []struct{ Child, Parent string }{{"order", "user"}, {"order", "plan"}, {"plan", "group"}, {"user", "user"}}
	order := childrenFirst([]string{"group", "plan", "user", "order"}, edges)
	position := map[string]int{}
	for i, table := range order {
		position[table] = i
	}
	require.Len(t, order, 4)
	require.Less(t, position["order"], position["user"])
	require.Less(t, position["order"], position["plan"])
	require.Less(t, position["plan"], position["group"])
	require.Nil(t, childrenFirst([]string{"a", "b"}, []struct{ Child, Parent string }{{"a", "b"}, {"b", "a"}}), "a cycle")
}
