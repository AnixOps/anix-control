package database

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestSQLLoggerOmitsBoundValues: at log level info every statement is
// logged, failed ones too, but never the values bound to it.
func TestSQLLoggerOmitsBoundValues(t *testing.T) {
	var out bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: newSQLLogger(log.New(&out, "", 0), false, logger.Info)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	const secret = "node-api-key-that-must-not-be-logged"
	require.NoError(t, db.Exec("CREATE TABLE nodes (api_key_hash TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO nodes (api_key_hash) VALUES (?)", secret).Error)
	var count int64
	require.NoError(t, db.Table("nodes").Where("api_key_hash = ?", secret).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.Error(t, db.Exec("INSERT INTO missing_table (api_key_hash) VALUES (?)", secret).Error)

	logged := out.String()
	assert.Contains(t, logged, "INSERT INTO nodes (api_key_hash) VALUES (?)")
	assert.Contains(t, logged, "api_key_hash = ?")
	assert.Contains(t, logged, "missing_table")
	assert.NotContains(t, logged, secret)
}

// TestInitUsesParameterizedSQLLogger: the database Init opens logs
// statements without their values whatever database.log_level says.
func TestInitUsesParameterizedSQLLogger(t *testing.T) {
	for _, level := range []string{"info", "warn", "error", ""} {
		require.NoError(t, Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:", LogLevel: level}))
		filter, ok := Get().Logger.(gorm.ParamsFilter)
		require.True(t, ok, "log level %q", level)
		_, params := filter.ParamsFilter(context.Background(), "SELECT ?", "secret")
		assert.Empty(t, params, "log level %q", level)
		closeDatabase(t)
	}
}

// An empty database.log_level (a configuration file without a database
// section) is error, the built-in default, as an unknown value is: never
// info, which logs every statement.
func TestSQLLogLevelDefaultsToError(t *testing.T) {
	for value, want := range map[string]logger.LogLevel{
		"": logger.Error, "  ": logger.Error, "error": logger.Error, "verbose": logger.Error,
		"silent": logger.Silent, "warn": logger.Warn, "info": logger.Info, "INFO": logger.Info, " Warn ": logger.Warn,
	} {
		assert.Equal(t, want, sqlLogLevel(value), "log level %q", value)
	}
	assert.Equal(t, "error", config.Defaults().Database.LogLevel, "the Go default follows defaults.yaml")

	// Through Init: an empty level logs a failed statement, not a good one.
	var out bytes.Buffer
	previous := sqlLogOutput
	SetSQLLogOutput(&out)
	t.Cleanup(func() { SetSQLLogOutput(previous) })
	require.NoError(t, Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { closeDatabase(t) })
	require.NoError(t, Get().Exec("CREATE TABLE quiet (id INTEGER)").Error)
	assert.NotContains(t, out.String(), "CREATE TABLE quiet")
	require.Error(t, Get().Exec("INSERT INTO missing_table (id) VALUES (1)").Error)
	assert.Contains(t, out.String(), "missing_table")
}
