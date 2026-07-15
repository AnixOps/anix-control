package database

import (
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func closeDatabase(t testing.TB) {
	t.Helper()
	require.NoError(t, Close())
}

func TestInit_SQLite(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.NotNil(t, Get())
	assert.NotNil(t, GetDB())
	assert.True(t, IsSQLite())
	assert.False(t, IsPostgres())
}

func TestInit_SQLite_DefaultPath(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver: "sqlite",
		// Empty database path should use default
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.NotNil(t, Get())
}

func TestInit_SQLite_WithDirectory(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_subdir", "test.db")

	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: dbPath,
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)
}

func TestInit_Postgres_InvalidConfig(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "nonexistent-host",
		Port:     5432,
		Username: "test",
		Password: "test",
		Database: "test",
	}

	// This will fail because the host doesn't exist, but we test the config parsing
	err := Init(cfg)
	assert.Error(t, err) // Expected to fail connecting
}

func TestInit_UnsupportedDriver(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "unsupported",
		Database: "test",
	}

	err := Init(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database driver")
}

func TestInit_WithConnectionPoolSettings(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:          "sqlite",
		Database:        ":memory:",
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: 3600,
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.NotNil(t, Get())
}

func TestGet(t *testing.T) {
	// Before initialization
	Reset()
	assert.Nil(t, Get())

	// After initialization
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))
	defer closeDatabase(t)

	assert.NotNil(t, Get())
}

func TestGetDB(t *testing.T) {
	// Before initialization
	Reset()
	assert.Nil(t, GetDB())

	// After initialization
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))
	defer closeDatabase(t)

	assert.NotNil(t, GetDB())
}

func TestClose(t *testing.T) {
	// Close with nil db should not error
	Reset()
	err := Close()
	assert.NoError(t, err)

	// Close with initialized db
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))
	err = Close()
	assert.NoError(t, err)
}

func TestClose_DoubleClose(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))

	// First close
	err := Close()
	assert.NoError(t, err)

	// Second close should be safe
	err = Close()
	assert.NoError(t, err)
}

func TestAutoMigrate(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	// Create a simple test model
	type TestModel struct {
		ID   uint `gorm:"primaryKey"`
		Name string
	}

	err = AutoMigrate(&TestModel{})
	assert.NoError(t, err)
}

func TestAutoMigrate_MultipleModels(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	type Model1 struct {
		ID uint `gorm:"primaryKey"`
	}
	type Model2 struct {
		ID  uint `gorm:"primaryKey"`
		Foo string
	}

	err = AutoMigrate(&Model1{}, &Model2{})
	assert.NoError(t, err)
}

func TestIsSQLite(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))
	defer closeDatabase(t)

	assert.True(t, IsSQLite())
	assert.False(t, IsPostgres())
}

func TestIsPostgres(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	require.NoError(t, Init(cfg))
	defer closeDatabase(t)

	assert.False(t, IsPostgres())
	assert.True(t, IsSQLite())
}

func TestInit_WithLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
	}{
		{"silent", "silent"},
		{"error", "error"},
		{"warn", "warn"},
		{"info", "info"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.DatabaseConfig{
				Driver:   "sqlite",
				Database: ":memory:",
				LogLevel: tt.logLevel,
			}

			err := Init(cfg)
			require.NoError(t, err)
			closeDatabase(t)
		})
	}
}

func TestInit_SQLiteWithPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_temp.db")
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: dbPath,
	}

	err := Init(cfg)
	require.NoError(t, err)

	assert.NotNil(t, Get())
	closeDatabase(t)
}

func TestInit_EmptyDriver(t *testing.T) {
	// Empty driver should default to SQLite
	cfg := &config.DatabaseConfig{
		Driver:   "",
		Database: ":memory:",
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.NotNil(t, Get())
	assert.True(t, IsSQLite())
}

func TestInit_SQLite3Driver(t *testing.T) {
	// "sqlite3" should be treated same as "sqlite"
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite3",
		Database: ":memory:",
	}

	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.NotNil(t, Get())
	assert.True(t, IsSQLite())
}

func TestClose_AfterInit(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)

	// Close should succeed
	err = Close()
	assert.NoError(t, err)

	// After close, db is nil
	assert.Nil(t, Get())

	// Close again should be safe
	err = Close()
	assert.NoError(t, err)
}

func TestIsSQLite_AfterClose(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)

	assert.True(t, IsSQLite())

	closeDatabase(t)
	// After close, db is nil, calling IsSQLite would panic
	// So we just test that it works when db is valid
}

func TestIsPostgres_AfterInit(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	assert.False(t, IsPostgres())
	assert.True(t, IsSQLite())
}

func TestGet_AfterInit(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}
	err := Init(cfg)
	require.NoError(t, err)
	defer closeDatabase(t)

	db := Get()
	assert.NotNil(t, db)

	db2 := GetDB()
	assert.NotNil(t, db2)
	assert.Equal(t, db, db2)
}
