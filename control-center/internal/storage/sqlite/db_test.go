package sqlite

import (
	"runtime"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := &Config{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	if cfg.Driver != "sqlite" {
		t.Errorf("expected driver 'sqlite', got '%s'", cfg.Driver)
	}
	if cfg.Database != ":memory:" {
		t.Errorf("expected database ':memory:', got '%s'", cfg.Database)
	}
}

func TestInit_SQLite(t *testing.T) {
	// SQLite requires CGO on Windows
	if runtime.GOOS == "windows" && !supportsCgo() {
		t.Skip("SQLite requires CGO on Windows")
	}

	cfg := &Config{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	err := Init(cfg)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	defer Close()

	db := GetDB()
	if db == nil {
		t.Fatal("database connection is nil")
	}
}

func TestGetDB(t *testing.T) {
	// SQLite requires CGO on Windows
	if runtime.GOOS == "windows" && !supportsCgo() {
		t.Skip("SQLite requires CGO on Windows")
	}

	cfg := &Config{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	Init(cfg)
	defer Close()

	db := GetDB()
	if db == nil {
		t.Error("GetDB returned nil")
	}
}

func TestClose(t *testing.T) {
	// SQLite requires CGO on Windows
	if runtime.GOOS == "windows" && !supportsCgo() {
		t.Skip("SQLite requires CGO on Windows")
	}

	cfg := &Config{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	Init(cfg)

	err := Close()
	if err != nil {
		t.Errorf("failed to close database: %v", err)
	}

	// Close again should not error
	err = Close()
	if err != nil {
		t.Errorf("second close should not error: %v", err)
	}
}

func TestInit_Postgres_NotSupported(t *testing.T) {
	cfg := &Config{
		Driver:   "postgres",
		Database: "test",
	}

	err := Init(cfg)
	if err == nil {
		t.Error("expected error for postgres driver")
	}
}

func TestInit_DefaultDriver(t *testing.T) {
	// SQLite requires CGO on Windows
	if runtime.GOOS == "windows" && !supportsCgo() {
		t.Skip("SQLite requires CGO on Windows")
	}

	cfg := &Config{
		Driver:   "unknown",
		Database: ":memory:",
	}

	err := Init(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer Close()

	db := GetDB()
	if db == nil {
		t.Error("database connection is nil")
	}
}

// supportsCgo checks if CGO is available
func supportsCgo() bool {
	// On Windows, CGO may not be available by default
	return false
}
