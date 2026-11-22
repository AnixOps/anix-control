package migrations

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// tablePrefixVersion is the number of 0040_table_prefix, which renames the
// tables Control kept from V2Board (v2_*) to the v5_ prefix.
const tablePrefixVersion = 40

const (
	formerTablePrefix = "v2_"
	tablePrefix       = "v5_"
)

// renameTablePrefixObjects is the Go part of 0040_table_prefix. Its SQL
// renamed the tables and their idx_ indexes; on PostgreSQL this renames
// what the database named after the tables when it created them: primary
// key and foreign key constraints and id sequences. It reads the names
// from the catalogs, so a database whose names drifted from a fresh one is
// renamed all the same. SQLite has no such names to rename.
func renameTablePrefixObjects(ctx context.Context, tx *gorm.DB, dialect string) error {
	if dialect != DialectPostgres {
		return nil
	}
	return renameDerivedNames(tx.WithContext(ctx), formerTablePrefix, tablePrefix)
}

// renameDerivedNames renames, on the tables whose name starts with to, the
// constraints named from... or fk_from... and the sequences they own named
// from..., to the same name with to.
func renameDerivedNames(tx *gorm.DB, from, to string) error {
	var constraints []struct{ Table, Name string }
	if err := tx.Raw(`SELECT t.relname AS "table", c.conname AS name FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = current_schema() AND left(t.relname, length(?)) = ?
		AND (left(c.conname, length(?)) = ? OR left(c.conname, length(?)) = ?)
		ORDER BY 1, 2`, to, to, from, from, "fk_"+from, "fk_"+from).Scan(&constraints).Error; err != nil {
		return err
	}
	for _, constraint := range constraints {
		renamed := strings.Replace(constraint.Name, from, to, 1)
		if err := tx.Exec(fmt.Sprintf(`ALTER TABLE %q RENAME CONSTRAINT %q TO %q`, constraint.Table, constraint.Name, renamed)).Error; err != nil {
			return err
		}
	}
	var sequences []string
	if err := tx.Raw(`SELECT DISTINCT s.relname FROM pg_class s
		JOIN pg_namespace n ON n.oid = s.relnamespace
		JOIN pg_depend d ON d.objid = s.oid AND d.classid = 'pg_class'::regclass AND d.refclassid = 'pg_class'::regclass
		JOIN pg_class t ON t.oid = d.refobjid
		WHERE s.relkind = 'S' AND d.deptype IN ('a', 'i') AND n.nspname = current_schema()
		AND left(t.relname, length(?)) = ? AND left(s.relname, length(?)) = ?
		ORDER BY 1`, to, to, from, from).Scan(&sequences).Error; err != nil {
		return err
	}
	for _, sequence := range sequences {
		if err := tx.Exec(fmt.Sprintf(`ALTER SEQUENCE %q RENAME TO %q`, sequence, to+strings.TrimPrefix(sequence, from))).Error; err != nil {
			return err
		}
	}
	return nil
}

// TableIn answers the name table has in db. Control reads a few tables
// before it migrates (the start's read-only gates); in a database
// 0040_table_prefix has not renamed yet, a v5_ table it renames still has
// its v2_ name.
func TableIn(db *gorm.DB, table string) string {
	if !strings.HasPrefix(table, tablePrefix) || db.Migrator().HasTable(table) {
		return table
	}
	if former := formerTablePrefix + strings.TrimPrefix(table, tablePrefix); db.Migrator().HasTable(former) {
		return former
	}
	return table
}
