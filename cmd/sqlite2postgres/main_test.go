package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSQLiteTablesAndCounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "source.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_user (id integer primary key, email text)`).Error; err != nil {
		t.Fatalf("create v2_user: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_plan (id integer primary key, name text)`).Error; err != nil {
		t.Fatalf("create v2_plan: %v", err)
	}
	if err := db.Exec(`INSERT INTO v2_user (id, email) VALUES (1, 'a@example.com'), (2, 'b@example.com')`).Error; err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if err := db.Exec(`INSERT INTO v2_plan (id, name) VALUES (1, 'basic')`).Error; err != nil {
		t.Fatalf("insert plans: %v", err)
	}

	tables, err := sqliteTables(db)
	if err != nil {
		t.Fatalf("list sqlite tables: %v", err)
	}
	if want := []string{"v2_plan", "v2_user"}; !reflect.DeepEqual(tables, want) {
		t.Fatalf("tables = %#v, want %#v", tables, want)
	}

	counts, err := tableCounts(db, tables)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if counts["v2_user"] != 2 || counts["v2_plan"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestCopyTableCopiesRows(t *testing.T) {
	src, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "source.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open source sqlite: %v", err)
	}
	dst, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "target.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open target sqlite: %v", err)
	}

	for _, db := range []*gorm.DB{src, dst} {
		if err := db.Exec(`CREATE TABLE v2_user (id integer primary key, email text, banned integer)`).Error; err != nil {
			t.Fatalf("create target/source table: %v", err)
		}
	}
	if err := src.Exec(`INSERT INTO v2_user (id, email, banned) VALUES (1, 'a@example.com', 1), (2, 'b@example.com', 0)`).Error; err != nil {
		t.Fatalf("insert source rows: %v", err)
	}

	if err := copyTable(src, dst, "v2_user", map[string]bool{"banned": true}); err != nil {
		t.Fatalf("copy table: %v", err)
	}

	var rows []struct {
		ID     int
		Email  string
		Banned bool
	}
	if err := dst.Table("v2_user").Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read copied rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("copied rows = %#v, want 2 rows", rows)
	}
	if rows[0].Email != "a@example.com" || !rows[0].Banned || rows[1].Email != "b@example.com" || rows[1].Banned {
		t.Fatalf("copied rows = %#v", rows)
	}
}

func TestOpenPostgresRejectsNonPostgresConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("database:\n  driver: sqlite\n  database: config/data/v2board.db\n")
	if err := os.WriteFile(configPath, content, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	db, err := openPostgres(configPath, "")
	if err == nil {
		if db != nil {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				_ = sqlDB.Close()
			}
		}
		t.Fatal("expected sqlite target config to be rejected")
	}
	if !strings.Contains(err.Error(), `expected postgres`) {
		t.Fatalf("error = %q, want expected postgres", err.Error())
	}
}

func TestNormalizeValueForPostgresBoolColumns(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{name: "nil", in: nil, want: nil},
		{name: "int one", in: int64(1), want: true},
		{name: "int zero", in: int64(0), want: false},
		{name: "string true", in: "true", want: true},
		{name: "string yes", in: "yes", want: true},
		{name: "string false", in: "false", want: false},
		{name: "bytes one", in: []byte("1"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeValue(tt.in, true); got != tt.want {
				t.Fatalf("normalizeValue(%#v, true) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
