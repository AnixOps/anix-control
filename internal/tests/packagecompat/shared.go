package packagecompat

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Migrating a route's Models into two new databases for every case
// dominated the run time on PostgreSQL: a schema, its tables and indexes
// created and dropped per case. The cases a test runs through the same
// *testing.T instead share one database per side and backend: migrated
// once, then emptied before each case so it starts as a new one would.
//
// Emptying drops the views, tables and other objects a case added (a
// seed's kernel API views), checks that what is left is exactly what the
// migration created (no dropped constraint or altered column survives), and
// deletes every row and restarts every id sequence. A database that does
// not match is replaced by a newly migrated one, so a case that changes the
// schema costs a migration, never a wrong result.

const (
	backendSQLite   = "sqlite"
	backendPostgres = "postgres"
)

// pool holds the shared databases of one test.
type pool struct {
	parent *testing.T
	mu     sync.Mutex
	shared map[string]*sharedDatabase
}

var (
	poolsMu sync.Mutex
	pools   = map[*testing.T]*pool{}
)

// poolFor returns t's pool, closed when t ends.
func poolFor(t *testing.T) *pool {
	poolsMu.Lock()
	defer poolsMu.Unlock()
	if p, ok := pools[t]; ok {
		return p
	}
	p := &pool{parent: t, shared: map[string]*sharedDatabase{}}
	pools[t] = p
	t.Cleanup(func() {
		p.mu.Lock()
		for _, shared := range p.shared {
			shared.close()
		}
		p.shared = nil
		p.mu.Unlock()
		poolsMu.Lock()
		delete(pools, t)
		poolsMu.Unlock()
	})
	return p
}

// sharedDatabase is one migrated database and what its migration created.
type sharedDatabase struct {
	backend  string
	cfg      *config.DatabaseConfig
	db       *gorm.DB
	close    func()
	baseline []string
	tables   []string
	// restore adds back a constraint of the migration a case dropped
	// (PostgreSQL), by its baseline line.
	restore map[string]string
	// deleteOrder lists the tables referencing ones first (PostgreSQL);
	// nil when their foreign keys form a cycle.
	deleteOrder []string
	inUse       bool
}

// opener returns the shared database for backend, emptied, with models
// migrated.
func (p *pool) opener(backend string) opener {
	return func(t *testing.T, label string, models []any) (*config.DatabaseConfig, *gorm.DB, bool) {
		t.Helper()
		key := backend + "/" + label + "/" + modelsKey(models)
		p.mu.Lock()
		defer p.mu.Unlock()
		shared := p.shared[key]
		if shared != nil && shared.inUse {
			// Taken by a case still running (a nested run): this case
			// gets a database of its own, closed with it.
			fresh, err := p.create(backend, label, models)
			skipUnsafe(t, err)
			require.NoError(t, err)
			t.Cleanup(fresh.close)
			return fresh.cfg, fresh.db, true
		}
		if shared != nil {
			reusable, err := shared.reset()
			require.NoError(t, err)
			if !reusable {
				shared.close()
				delete(p.shared, key)
				shared = nil
			}
		}
		if shared == nil {
			var err error
			shared, err = p.create(backend, label, models)
			skipUnsafe(t, err)
			require.NoError(t, err)
			p.shared[key] = shared
		}
		shared.inUse = true
		t.Cleanup(func() {
			p.mu.Lock()
			shared.inUse = false
			p.mu.Unlock()
		})
		// A handle of the case's own: state a test or native service keeps
		// by *gorm.DB (a warmed service, a cache) starts from nothing, as
		// with a new database.
		return shared.cfg, shared.db.Session(&gorm.Session{NewDB: true}), true
	}
}

func skipUnsafe(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, errUnsafePostgres) {
		t.Skip(err.Error())
	}
}

func modelsKey(models []any) string {
	names := make([]string, len(models))
	for i, model := range models {
		names[i] = fmt.Sprintf("%T", model)
	}
	return strings.Join(names, ",")
}

// create opens a new database for backend and migrates models into it.
func (p *pool) create(backend, label string, models []any) (*sharedDatabase, error) {
	shared := &sharedDatabase{backend: backend}
	switch backend {
	case backendSQLite:
		cfg, db, err := createSQLite(p.parent.TempDir(), fmt.Sprintf("%s-%d", label, nextSQLiteFile()))
		if err != nil {
			return nil, err
		}
		shared.cfg, shared.db = cfg, db
		shared.close = func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	case backendPostgres:
		cfg, db, drop, err := createPostgresSchema(label)
		if err != nil {
			return nil, err
		}
		shared.cfg, shared.db, shared.close = cfg, db, drop
	default:
		return nil, fmt.Errorf("unknown backend %q", backend)
	}
	if len(models) > 0 {
		if err := shared.db.AutoMigrate(models...); err != nil {
			shared.close()
			return nil, err
		}
	}
	objects, err := shared.objects()
	if err != nil {
		shared.close()
		return nil, err
	}
	shared.baseline = objects
	if backend == backendPostgres {
		var constraints []struct{ Line, Statement string }
		if err := shared.db.Raw(postgresConstraints).Scan(&constraints).Error; err != nil {
			shared.close()
			return nil, err
		}
		shared.restore = make(map[string]string, len(constraints))
		for _, constraint := range constraints {
			shared.restore[constraint.Line] = constraint.Statement
		}
	}
	for _, object := range objects {
		if name, ok := strings.CutPrefix(object, "table "); ok {
			shared.tables = append(shared.tables, name)
		}
	}
	if backend == backendPostgres {
		var edges []struct{ Child, Parent string }
		if err := shared.db.Raw(postgresForeignKeys).Scan(&edges).Error; err != nil {
			shared.close()
			return nil, err
		}
		shared.deleteOrder = childrenFirst(shared.tables, edges)
	}
	return shared, nil
}

// childrenFirst orders tables so that each comes before the tables it
// references; nil when the references form a cycle.
func childrenFirst(tables []string, edges []struct{ Child, Parent string }) []string {
	referencedBy := map[string]int{}
	references := map[string][]string{}
	for _, edge := range edges {
		if edge.Child == edge.Parent {
			continue
		}
		references[edge.Child] = append(references[edge.Child], edge.Parent)
		referencedBy[edge.Parent]++
	}
	var order, ready []string
	for _, table := range tables {
		if referencedBy[table] == 0 {
			ready = append(ready, table)
		}
	}
	for len(ready) > 0 {
		table := ready[0]
		ready = ready[1:]
		order = append(order, table)
		for _, parent := range references[table] {
			referencedBy[parent]--
			if referencedBy[parent] == 0 {
				ready = append(ready, parent)
			}
		}
	}
	if len(order) != len(tables) {
		return nil
	}
	return order
}

var (
	sqliteFilesMu sync.Mutex
	sqliteFiles   int
)

func nextSQLiteFile() int {
	sqliteFilesMu.Lock()
	defer sqliteFilesMu.Unlock()
	sqliteFiles++
	return sqliteFiles
}

// reset empties the database for the next case. It reports false when the
// schema is no longer the migrated one and the database must be replaced.
func (s *sharedDatabase) reset() (bool, error) {
	objects, err := s.objects()
	if err != nil {
		return false, err
	}
	baseline := make(map[string]bool, len(s.baseline))
	for _, object := range s.baseline {
		baseline[object] = true
	}
	// Drop what the last case added: views first, as they depend on tables.
	var extra []string
	for _, object := range objects {
		if !baseline[object] {
			extra = append(extra, object)
		}
	}
	for _, prefix := range []string{"view ", "table "} {
		for _, object := range extra {
			if name, ok := strings.CutPrefix(object, prefix); ok {
				kind := strings.ToUpper(strings.TrimSpace(prefix))
				statement := "DROP " + kind + " IF EXISTS " + quoteIdentifier(name)
				if s.backend == backendPostgres {
					statement += " CASCADE"
				}
				if err := s.db.Exec(statement).Error; err != nil {
					return false, err
				}
			}
		}
	}
	if len(extra) > 0 {
		if objects, err = s.objects(); err != nil {
			return false, err
		}
	}
	if slices.Equal(objects, s.baseline) {
		return true, s.empty()
	}
	// A case that dropped constraints of the migration (to seed rows the
	// kernel's foreign keys refuse): add them back to the empty tables.
	missing := missingConstraints(objects, s.baseline, s.restore)
	if missing == nil {
		return false, nil
	}
	if err := s.empty(); err != nil {
		return false, err
	}
	for _, statement := range missing {
		if err := s.db.Exec(statement).Error; err != nil {
			return false, err
		}
	}
	if objects, err = s.objects(); err != nil {
		return false, err
	}
	return slices.Equal(objects, s.baseline), nil
}

// missingConstraints returns the statements that add back what objects
// lacks of baseline, or nil when it differs in anything else.
func missingConstraints(objects, baseline []string, restore map[string]string) []string {
	present := make(map[string]bool, len(objects))
	for _, object := range objects {
		present[object] = true
	}
	inBaseline := make(map[string]bool, len(baseline))
	var statements []string
	for _, object := range baseline {
		inBaseline[object] = true
		if present[object] {
			continue
		}
		statement, ok := restore[object]
		if !ok {
			return nil
		}
		statements = append(statements, statement)
	}
	for _, object := range objects {
		if !inBaseline[object] {
			return nil
		}
	}
	return statements
}

// empty deletes every row of the migrated tables and restarts their ids.
func (s *sharedDatabase) empty() error {
	if len(s.tables) == 0 {
		return nil
	}
	quoted := make([]string, len(s.tables))
	for i, table := range s.tables {
		quoted[i] = quoteIdentifier(table)
	}
	if s.backend == backendPostgres {
		if s.deleteOrder == nil {
			return s.db.Exec("TRUNCATE " + strings.Join(quoted, ", ") + " RESTART IDENTITY CASCADE").Error
		}
		// DELETE, children first, is much cheaper than TRUNCATE on tables
		// this small (TRUNCATE gives every table and index a new file).
		return s.db.Transaction(func(tx *gorm.DB) error {
			for _, table := range s.deleteOrder {
				if err := tx.Exec("DELETE FROM " + quoteIdentifier(table)).Error; err != nil {
					return err
				}
			}
			return tx.Exec(postgresRestartSequences).Error
		})
	}
	return s.db.Connection(func(tx *gorm.DB) error {
		var foreignKeys int
		if err := tx.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
			return err
		}
		if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		for _, table := range quoted {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable("sqlite_sequence") {
			if err := tx.Exec("DELETE FROM sqlite_sequence").Error; err != nil {
				return err
			}
		}
		return tx.Exec(fmt.Sprintf("PRAGMA foreign_keys = %d", foreignKeys)).Error
	})
}

// objects lists the schema's objects as "<kind> <name>" lines (with each
// table's columns, constraints and indexes), sorted, so two lists are
// equal exactly when the schemas are.
func (s *sharedDatabase) objects() ([]string, error) {
	var rows []string
	var err error
	if s.backend == backendPostgres {
		err = s.db.Raw(postgresObjects).Scan(&rows).Error
	} else {
		err = s.db.Raw(sqliteObjects).Scan(&rows).Error
	}
	if err != nil {
		return nil, err
	}
	slices.Sort(rows)
	return rows, nil
}

// The tables and views are listed as "table <name>" and "view <name>" (the
// lines reset drops by); every other line describes a detail to compare.
const sqliteObjects = `SELECT type || ' ' || name FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
UNION ALL SELECT 'sql ' || type || ' ' || name || ' ' || COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`

const postgresObjects = `SELECT CASE c.relkind WHEN 'r' THEN 'table ' WHEN 'p' THEN 'table ' WHEN 'v' THEN 'view ' ELSE 'relation ' || c.relkind::text || ' ' END || c.relname
  FROM pg_class c WHERE c.relnamespace = current_schema()::regnamespace
UNION ALL SELECT 'column ' || c.relname || '.' || a.attname || ' ' || format_type(a.atttypid, a.atttypmod) || ' ' || a.attnotnull || ' ' || COALESCE(pg_get_expr(d.adbin, d.adrelid), '')
  FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
  WHERE c.relnamespace = current_schema()::regnamespace AND c.relkind IN ('r', 'p') AND a.attnum > 0 AND NOT a.attisdropped
UNION ALL SELECT 'constraint ' || c.relname || '.' || k.conname || ' ' || pg_get_constraintdef(k.oid)
  FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid WHERE k.connamespace = current_schema()::regnamespace
UNION ALL SELECT 'index ' || pg_get_indexdef(i.indexrelid)
  FROM pg_index i JOIN pg_class c ON c.oid = i.indrelid WHERE c.relnamespace = current_schema()::regnamespace
UNION ALL SELECT 'function ' || p.oid::regprocedure::text FROM pg_proc p WHERE p.pronamespace = current_schema()::regnamespace
UNION ALL SELECT 'trigger ' || c.relname || '.' || g.tgname
  FROM pg_trigger g JOIN pg_class c ON c.oid = g.tgrelid WHERE c.relnamespace = current_schema()::regnamespace AND NOT g.tgisinternal
UNION ALL SELECT 'type ' || t.typname FROM pg_type t
  WHERE t.typnamespace = current_schema()::regnamespace AND t.typtype IN ('e', 'd', 'c') AND NOT EXISTS (SELECT 1 FROM pg_class c WHERE c.reltype = t.oid)`

// postgresConstraints pairs each constraint line of postgresObjects with
// the statement that creates it; foreign keys last, after the keys they
// reference.
const postgresConstraints = `SELECT 'constraint ' || c.relname || '.' || k.conname || ' ' || pg_get_constraintdef(k.oid) AS line,
    'ALTER TABLE ' || quote_ident(c.relname) || ' ADD CONSTRAINT ' || quote_ident(k.conname) || ' ' || pg_get_constraintdef(k.oid) AS statement
  FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid
  WHERE k.connamespace = current_schema()::regnamespace AND k.contype = 'f'`

// postgresForeignKeys lists which table references which.
const postgresForeignKeys = `SELECT c.relname AS child, r.relname AS parent
  FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid JOIN pg_class r ON r.oid = k.confrelid
  WHERE k.contype = 'f' AND k.connamespace = current_schema()::regnamespace`

// postgresRestartSequences sets every sequence of the schema back to its
// start, as a new one is.
const postgresRestartSequences = `SELECT setval(format('%I', sequencename)::regclass, start_value, false)
  FROM pg_sequences WHERE schemaname = current_schema()`

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
