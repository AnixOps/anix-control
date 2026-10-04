package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The plugin telemetry and observed-state upserts keep the newest
// observation on both databases. On PostgreSQL a bare observed_at in the
// conflict condition is ambiguous with excluded.observed_at (SQLSTATE
// 42702), which dropped every machine-telemetry sample there (found by the
// cross-repository Agent suite, internal/tests/agente2e).
func TestPluginStateUpsertsKeepTheNewestObservation(t *testing.T) {
	for name, open := range map[string]func(*testing.T) *gorm.DB{
		"sqlite":   func(t *testing.T) *gorm.DB { return newKernelTestDB(t) },
		"postgres": openRouteModePostgres,
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

			snapshot := NodePluginObservedSnapshot{PluginID: "nftables-forward", Version: "4.0.0", DesiredRevision: 2, ObservedRevision: 2, ConfigHash: "hash", Health: "healthy", ObservedAt: newer}
			require.NoError(t, service.persistNodePluginObservedState(7, snapshot, newer))
			later := snapshot
			later.ObservedAt, later.ObservedRevision, later.DesiredRevision = newer.Add(time.Minute), 3, 3
			require.NoError(t, service.persistNodePluginObservedState(7, later, newer.Add(time.Minute)))
			require.NoError(t, service.persistNodePluginObservedState(7, snapshot, newer.Add(2*time.Minute)))
			var observed model.NodePluginObservedState
			require.NoError(t, db.First(&observed, "node_id = ? AND plugin_id = ?", 7, "nftables-forward").Error)
			require.Equal(t, int64(3), observed.ObservedRevision, "an older observation does not replace a newer one")
		})
	}
}
