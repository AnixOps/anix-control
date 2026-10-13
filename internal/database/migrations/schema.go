package migrations

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// Schema is what a database holds in the schema Control uses (SQLite: the
// main database; PostgreSQL: current_schema()), read from the catalogs.
// It is the fingerprint adoption checks an existing database against, and
// what the tests compare a fresh schema with an upgraded one by.
type Schema struct {
	Dialect string
	Tables  map[string]*Table
	// Views maps a view name to its normalized definition.
	Views map[string]string
}

// Table is one table of a Schema.
type Table struct {
	Name    string
	Columns map[string]Column
	// Indexes are the indexes created by CREATE INDEX, by name. Indexes
	// that back a constraint (a primary key, a UNIQUE column) are
	// Constraints instead.
	Indexes map[string]Index
	// Constraints are the normalized primary key, unique, foreign key and
	// check constraints.
	Constraints []string
}

// Column is one column of a Table.
type Column struct {
	Name    string
	Type    string
	NotNull bool
	// Default is the column default as the catalog shows it, "" when it
	// has none.
	Default string
}

// Index is a named index.
type Index struct {
	Name  string
	Table string
	// Definition is the normalized CREATE INDEX statement: unique or not,
	// the columns and the predicate of a partial index.
	Definition string
}

// Inspect reads the schema of db.
func Inspect(ctx context.Context, db *gorm.DB) (*Schema, error) {
	dialect, err := dialectOf(db)
	if err != nil {
		return nil, err
	}
	db = db.WithContext(ctx)
	schema := &Schema{Dialect: dialect, Tables: map[string]*Table{}, Views: map[string]string{}}
	if dialect == DialectPostgres {
		err = inspectPostgres(db, schema)
	} else {
		err = inspectSQLite(db, schema)
	}
	if err != nil {
		return nil, fmt.Errorf("read the database schema: %w", err)
	}
	return schema, nil
}

func newTable(name string) *Table {
	return &Table{Name: name, Columns: map[string]Column{}, Indexes: map[string]Index{}}
}

func inspectSQLite(db *gorm.DB, schema *Schema) error {
	type object struct {
		Type    string
		Name    string
		TblName string `gorm:"column:tbl_name"`
		SQL     *string
	}
	var objects []object
	if err := db.Raw(`SELECT type, name, tbl_name, sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&objects).Error; err != nil {
		return err
	}
	indexSQL := map[string]string{}
	for _, item := range objects {
		switch item.Type {
		case "table":
			schema.Tables[item.Name] = newTable(item.Name)
		case "view":
			if item.SQL != nil {
				schema.Views[item.Name] = normalizeSQL(*item.SQL)
			}
		case "index":
			if item.SQL != nil {
				indexSQL[item.Name] = *item.SQL
			}
		}
	}
	for name, table := range schema.Tables {
		quoted := quoteSQLite(name)
		var columns []struct {
			Name      string
			Type      string
			NotNull   int     `gorm:"column:notnull"`
			DfltValue *string `gorm:"column:dflt_value"`
			PK        int     `gorm:"column:pk"`
		}
		if err := db.Raw("PRAGMA table_info(" + quoted + ")").Scan(&columns).Error; err != nil {
			return fmt.Errorf("columns of %s: %w", name, err)
		}
		if len(columns) == 0 {
			return fmt.Errorf("read no columns of table %s", name)
		}
		var primaryKey []string
		for _, column := range columns {
			value := Column{Name: column.Name, Type: strings.ToLower(column.Type), NotNull: column.NotNull != 0}
			if column.DfltValue != nil {
				value.Default = *column.DfltValue
			}
			table.Columns[column.Name] = value
			if column.PK > 0 {
				primaryKey = append(primaryKey, column.Name)
			}
		}
		if len(primaryKey) > 0 {
			table.Constraints = append(table.Constraints, "PRIMARY KEY ("+strings.Join(primaryKey, ", ")+")")
		}
		var indexes []struct {
			Name   string
			Unique int
			Origin string
		}
		if err := db.Raw("PRAGMA index_list(" + quoted + ")").Scan(&indexes).Error; err != nil {
			return fmt.Errorf("indexes of %s: %w", name, err)
		}
		for _, index := range indexes {
			if index.Origin == "c" {
				definition, ok := indexSQL[index.Name]
				if !ok {
					return fmt.Errorf("index %s has no definition", index.Name)
				}
				table.Indexes[index.Name] = Index{Name: index.Name, Table: name, Definition: normalizeIndexSQL(definition)}
				continue
			}
			if index.Origin != "u" {
				continue // the primary key, listed above
			}
			var parts []struct{ Name string }
			if err := db.Raw("PRAGMA index_info(" + quoteSQLite(index.Name) + ")").Scan(&parts).Error; err != nil {
				return fmt.Errorf("columns of index %s: %w", index.Name, err)
			}
			names := make([]string, 0, len(parts))
			for _, part := range parts {
				names = append(names, part.Name)
			}
			table.Constraints = append(table.Constraints, "UNIQUE ("+strings.Join(names, ", ")+")")
		}
		var keys []struct {
			ID       int
			Seq      int
			Table    string
			From     string
			To       *string
			OnUpdate string `gorm:"column:on_update"`
			OnDelete string `gorm:"column:on_delete"`
		}
		if err := db.Raw("PRAGMA foreign_key_list(" + quoted + ")").Scan(&keys).Error; err != nil {
			return fmt.Errorf("foreign keys of %s: %w", name, err)
		}
		type foreignKey struct {
			from, to       []string
			parent, update string
			remove         string
		}
		byID := map[int]*foreignKey{}
		var ids []int
		for _, key := range keys {
			current, ok := byID[key.ID]
			if !ok {
				current = &foreignKey{parent: key.Table, update: key.OnUpdate, remove: key.OnDelete}
				byID[key.ID] = current
				ids = append(ids, key.ID)
			}
			current.from = append(current.from, key.From)
			if key.To != nil {
				current.to = append(current.to, *key.To)
			}
		}
		for _, id := range ids {
			key := byID[id]
			table.Constraints = append(table.Constraints, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s) ON UPDATE %s ON DELETE %s",
				strings.Join(key.from, ", "), key.parent, strings.Join(key.to, ", "), key.update, key.remove))
		}
		sort.Strings(table.Constraints)
	}
	return nil
}

func inspectPostgres(db *gorm.DB, schema *Schema) error {
	var tables []struct {
		ID   int64
		Name string
	}
	if err := db.Raw(`SELECT c.oid::bigint AS id, c.relname AS name FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') ORDER BY c.relname`).Scan(&tables).Error; err != nil {
		return err
	}
	var current string
	if err := db.Raw(`SELECT current_schema()`).Scan(&current).Error; err != nil {
		return err
	}
	unqualify := func(definition string) string {
		for _, prefix := range []string{`"` + current + `".`, current + "."} {
			definition = strings.ReplaceAll(definition, prefix, "")
		}
		return definition
	}
	for _, item := range tables {
		table := newTable(item.Name)
		schema.Tables[item.Name] = table
		var columns []struct {
			Name    string
			Type    string
			NotNull bool
			Default *string
		}
		if err := db.Raw(`SELECT a.attname AS name, format_type(a.atttypid, a.atttypmod) AS type, a.attnotnull AS not_null,
				pg_get_expr(d.adbin, d.adrelid) AS "default"
			FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
			WHERE a.attrelid = ?::oid AND a.attnum > 0 AND NOT a.attisdropped`, item.ID).Scan(&columns).Error; err != nil {
			return fmt.Errorf("columns of %s: %w", item.Name, err)
		}
		if len(columns) == 0 {
			return fmt.Errorf("read no columns of table %s", item.Name)
		}
		for _, column := range columns {
			value := Column{Name: column.Name, Type: column.Type, NotNull: column.NotNull}
			if column.Default != nil {
				value.Default = unqualify(*column.Default)
			}
			table.Columns[column.Name] = value
		}
		var constraints []struct {
			Name       string
			Definition string
		}
		if err := db.Raw(`SELECT conname AS name, pg_get_constraintdef(oid) AS definition FROM pg_constraint
			WHERE conrelid = ?::oid ORDER BY conname`, item.ID).Scan(&constraints).Error; err != nil {
			return fmt.Errorf("constraints of %s: %w", item.Name, err)
		}
		for _, constraint := range constraints {
			table.Constraints = append(table.Constraints, constraint.Name+" "+unqualify(constraint.Definition))
		}
		sort.Strings(table.Constraints)
		var indexes []struct {
			Name       string
			Definition string
		}
		if err := db.Raw(`SELECT c.relname AS name, pg_get_indexdef(i.indexrelid) AS definition
			FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			WHERE i.indrelid = ?::oid AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conindid = i.indexrelid)`, item.ID).Scan(&indexes).Error; err != nil {
			return fmt.Errorf("indexes of %s: %w", item.Name, err)
		}
		for _, index := range indexes {
			table.Indexes[index.Name] = Index{Name: index.Name, Table: item.Name, Definition: normalizeIndexSQL(unqualify(index.Definition))}
		}
	}
	var views []struct {
		Name       string
		Definition string
		Options    *string
	}
	if err := db.Raw(`SELECT c.relname AS name, pg_get_viewdef(c.oid) AS definition, array_to_string(c.reloptions, ',') AS options
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'v'`).Scan(&views).Error; err != nil {
		return err
	}
	for _, view := range views {
		definition := normalizeSQL(unqualify(view.Definition))
		if view.Options != nil && *view.Options != "" {
			definition = "WITH (" + *view.Options + ") " + definition
		}
		schema.Views[view.Name] = definition
	}
	return nil
}

var (
	whitespace      = regexp.MustCompile(`\s+`)
	spaceBeforeOpen = regexp.MustCompile(`\s*\(\s*`)
	spaceBeforeEnd  = regexp.MustCompile(`\s*\)`)
	spaceAfterComma = regexp.MustCompile(`\s*,\s*`)
)

// normalizeSQL folds identifier quotes and whitespace, so the same
// statement written by two tools compares equal.
func normalizeSQL(statement string) string {
	statement = strings.NewReplacer("`", "", `"`, "").Replace(statement)
	statement = whitespace.ReplaceAllString(strings.TrimSpace(statement), " ")
	statement = spaceBeforeOpen.ReplaceAllString(statement, " (")
	statement = spaceBeforeEnd.ReplaceAllString(statement, ")")
	statement = spaceAfterComma.ReplaceAllString(statement, ", ")
	return strings.TrimSuffix(statement, ";")
}

// normalizeIndexSQL is normalizeSQL for an index definition, without the
// IF NOT EXISTS of the statement that created it and PostgreSQL's default
// access method.
func normalizeIndexSQL(statement string) string {
	statement = normalizeSQL(statement)
	statement = strings.Replace(statement, " IF NOT EXISTS ", " ", 1)
	return strings.Replace(statement, " USING btree ", " ", 1)
}

// Lines renders the schema as sorted lines, one per table, column, index,
// constraint and view: the form the tests diff.
func (s *Schema) Lines() []string {
	var lines []string
	for name, table := range s.Tables {
		lines = append(lines, "table "+name)
		for _, column := range table.Columns {
			lines = append(lines, "column "+name+"."+column.describe())
		}
		for _, index := range table.Indexes {
			lines = append(lines, "index "+name+"."+index.Name+": "+index.Definition)
		}
		for _, constraint := range table.Constraints {
			lines = append(lines, "constraint "+name+": "+constraint)
		}
	}
	for name, definition := range s.Views {
		lines = append(lines, "view "+name+": "+definition)
	}
	sort.Strings(lines)
	return lines
}

func (c Column) describe() string {
	text := c.Name + " " + c.Type
	if c.NotNull {
		text += " NOT NULL"
	}
	if c.Default != "" {
		text += " DEFAULT " + c.Default
	}
	return text
}

// Diff answers the lines of want missing from got ("-") and the lines of
// got that want does not have ("+"), sorted: an empty answer means the two
// schemas are the same.
func Diff(want, got *Schema) []string {
	wanted := map[string]bool{}
	for _, line := range want.Lines() {
		wanted[line] = true
	}
	have := map[string]bool{}
	for _, line := range got.Lines() {
		have[line] = true
	}
	var diff []string
	for line := range wanted {
		if !have[line] {
			diff = append(diff, "- "+line)
		}
	}
	for line := range have {
		if !wanted[line] {
			diff = append(diff, "+ "+line)
		}
	}
	sort.Slice(diff, func(i, j int) bool {
		if diff[i][2:] == diff[j][2:] {
			return diff[i] < diff[j]
		}
		return diff[i][2:] < diff[j][2:]
	})
	return diff
}

func quoteSQLite(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
