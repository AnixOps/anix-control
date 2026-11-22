package service

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The plugin telemetry and observed-state upserts keep the newest
// observation on both databases. On PostgreSQL a bare observed_at in the
// conflict condition is ambiguous with excluded.observed_at (SQLSTATE
// 42702), which dropped every machine-telemetry sample there (found by the
// cross-repository Agent suite, internal/tests/agente2e).
func TestPluginStateUpsertsKeepTheNewestObservation(t *testing.T) {
	for name, open := range map[string]func(*testing.T) *gorm.DB{
		"sqlite":   func(t *testing.T) *gorm.DB { return newKernelTestDB(t) },
		"postgres": openServicePostgres,
	} {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			require.NoError(t, db.AutoMigrate(&model.PluginTelemetryState{}, &model.NodePluginObservedState{}))
			service := &NodeService{db: db}
			newer := time.Unix(1_800_000_000, 0).UTC()

			require.NoError(t, service.persistPluginTelemetryState(7, "machine-telemetry", map[string]float64{"cpu_usage_percent": 20}, newer))
			require.NoError(t, service.persistPluginTelemetryState(7, "machine-telemetry", map[string]float64{"cpu_usage_percent": 30}, newer.Add(time.Minute)))
			require.NoError(t, service.persistPluginTelemetryState(7, "machine-telemetry", map[string]float64{"cpu_usage_percent": 90, "sample_age_seconds": 120}, newer.Add(time.Minute)))
			var telemetry model.PluginTelemetryState
			require.NoError(t, db.First(&telemetry, "node_id = ? AND plugin_id = ?", 7, "machine-telemetry").Error)
			require.True(t, telemetry.ObservedAt.Equal(newer.Add(time.Minute)), "observed at %s", telemetry.ObservedAt)
			require.Contains(t, telemetry.MetricsJSON, `"cpu_usage_percent":30`)

			snapshot := NodePluginObservedSnapshot{PluginID: "machine-telemetry", Version: "4.0.0", DesiredRevision: 2, ObservedRevision: 2, ConfigHash: "hash", Health: "healthy", ObservedAt: newer}
			require.NoError(t, service.persistNodePluginObservedState(7, snapshot, newer))
			later := snapshot
			later.ObservedAt, later.ObservedRevision, later.DesiredRevision = newer.Add(time.Minute), 3, 3
			require.NoError(t, service.persistNodePluginObservedState(7, later, newer.Add(time.Minute)))
			require.NoError(t, service.persistNodePluginObservedState(7, snapshot, newer.Add(2*time.Minute)))
			var observed model.NodePluginObservedState
			require.NoError(t, db.First(&observed, "node_id = ? AND plugin_id = ?", 7, "machine-telemetry").Error)
			require.Equal(t, int64(3), observed.ObservedRevision, "an older observation does not replace a newer one")
		})
	}
}

// openServicePostgres opens a fresh schema of the test PostgreSQL database,
// or skips without one.
func openServicePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	}
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN and POSTGRES_TEST_DSN are not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "service_test_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
