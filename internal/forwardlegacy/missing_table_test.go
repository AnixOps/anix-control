package forwardlegacy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MissingTable is the answer of a statement on a dropped flux table. It
// must say yes for the table the database names and for nothing else: every
// other failure of a statement is an error the caller keeps.
func TestMissingTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.ForwardRule{}, &model.ForwardNode{}))
	require.NoError(t, db.Migrator().DropTable(&model.ForwardRule{}))

	t.Run("SQLite: a statement on a table that is gone", func(t *testing.T) {
		var rules []model.ForwardRule
		err := db.Find(&rules).Error
		require.Error(t, err)
		assert.True(t, MissingTable(err, "v2_forward_rule"), err.Error())
		assert.True(t, MissingTable(fmt.Errorf("load rules: %w", err), "V2_FORWARD_RULE"), "wrapped, and the case of the name is not significant")
		assert.False(t, MissingTable(err, "v2_forward"), "a table whose name is a prefix is another table")
		assert.False(t, MissingTable(err, "v2_forward_rule_extra"))
		assert.False(t, MissingTable(err, "v2_forward_node"))
		assert.False(t, MissingTable(err, ""))
	})

	t.Run("SQLite: a statement on a table that is there fails for another reason", func(t *testing.T) {
		var nodes []model.ForwardNode
		require.NoError(t, db.Find(&nodes).Error)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := db.WithContext(ctx).Find(&nodes).Error
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, MissingTable(err, "v2_forward_node"))

		err = db.Table("v2_forward_node").Select("no_such_column").Find(&nodes).Error
		require.Error(t, err)
		assert.False(t, MissingTable(err, "v2_forward_node"), "a missing column is not a missing table: %v", err)
	})

	t.Run("SQLite: another table of the statement is gone", func(t *testing.T) {
		var rules []model.ForwardRule
		err := db.Find(&rules).Error
		require.Error(t, err)
		assert.False(t, MissingTable(err, "v2_forward_node"), "the error names v2_forward_rule, not the node table")
	})

	t.Run("PostgreSQL: undefined table", func(t *testing.T) {
		undefined := &pgconn.PgError{Severity: "ERROR", Code: "42P01", Message: `relation "v2_forward_rule" does not exist`}
		assert.True(t, MissingTable(undefined, "v2_forward_rule"))
		assert.True(t, MissingTable(fmt.Errorf("find: %w", undefined), "v2_forward_rule"))
		assert.False(t, MissingTable(undefined, "v2_forward"))
		assert.False(t, MissingTable(undefined, "v2_forward_node"))
		// Qualified with the schema the database searched.
		qualified := &pgconn.PgError{Severity: "ERROR", Code: "42P01", Message: `relation "public.v2_forward_rule" does not exist`}
		assert.True(t, MissingTable(qualified, "v2_forward_rule"))
	})

	t.Run("PostgreSQL: other failures", func(t *testing.T) {
		for name, failure := range map[string]error{
			"undefined column":  &pgconn.PgError{Severity: "ERROR", Code: "42703", Message: `column "v2_forward_rule" does not exist`},
			"statement timeout": &pgconn.PgError{Severity: "ERROR", Code: "57014", Message: "canceling statement due to statement timeout"},
			"aborted tx":        &pgconn.PgError{Severity: "ERROR", Code: "25P02", Message: "current transaction is aborted, commands ignored until end of transaction block"},
			"connection reset":  errors.New("write tcp 127.0.0.1:5432: connection reset by peer"),
			"deadline":          context.DeadlineExceeded,
			"record not found":  gorm.ErrRecordNotFound,
			"nothing":           nil,
		} {
			assert.False(t, MissingTable(failure, "v2_forward_rule"), name)
		}
	})

	t.Run("a driver error that carries only its SQLSTATE", func(t *testing.T) {
		undefined := stateError{state: "42P01", message: `relation "v2_forward_rule" does not exist`}
		assert.True(t, MissingTable(undefined, "v2_forward_rule"))
		assert.True(t, MissingTable(fmt.Errorf("find: %w", undefined), "v2_forward_rule"))
		assert.False(t, MissingTable(undefined, "v2_forward"))
		assert.False(t, MissingTable(stateError{state: "42703", message: `relation "v2_forward_rule" does not exist`}, "v2_forward_rule"))
	})

	t.Run("an error that is only text", func(t *testing.T) {
		// A driver error that lost its type on the way keeps its SQLSTATE in
		// the message.
		text := errors.New(`ERROR: relation "v2_forward_rule" does not exist (SQLSTATE 42P01)`)
		assert.True(t, MissingTable(text, "v2_forward_rule"))
		assert.False(t, MissingTable(text, "v2_forward"))
	})
}

// stateError is a driver error whose text does not repeat its SQLSTATE.
type stateError struct{ state, message string }

func (e stateError) Error() string    { return e.message }
func (e stateError) SQLState() string { return e.state }
