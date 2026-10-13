package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAlertSettingsDefaults(t *testing.T) {
	for name, cfg := range map[string]AlertsConfig{"zero": {}, "container defaults": Defaults().Alerts} {
		settings := cfg.Settings()
		require.True(t, settings.Enabled, name)
		require.Equal(t, 15*time.Minute, settings.CheckInterval, name)
		require.Equal(t, 14, settings.LeafExpiryDays, name)
		require.Equal(t, 60, settings.CAExpiryDays, name)
		require.Equal(t, 24*time.Hour, settings.RenotifyInterval, name)
		require.Equal(t, 72*time.Hour, settings.PhaseStuckAfter, name)
	}
}

func TestAlertSettingsFromConfigAndEnvironment(t *testing.T) {
	off := false
	settings := AlertsConfig{
		Enabled: &off, CheckInterval: "5m", LeafExpiryDays: 7, CAExpiryDays: 90, RenotifyInterval: "12h", PhaseStuckAfter: "0",
	}.Settings()
	require.False(t, settings.Enabled)
	require.Equal(t, 5*time.Minute, settings.CheckInterval)
	require.Equal(t, 7, settings.LeafExpiryDays)
	require.Equal(t, 90, settings.CAExpiryDays)
	require.Equal(t, 12*time.Hour, settings.RenotifyInterval)
	require.Zero(t, settings.PhaseStuckAfter, `"0" turns the phase alerts off`)

	// The alert settings are built-in values: the keys are ignored.
	loaded, report, err := LoadFile("", []string{
		EnvPrefix + "ALERTS_ENABLED=false", EnvPrefix + "ALERTS_LEAF_EXPIRY_DAYS=3", EnvPrefix + "ALERTS_PHASE_STUCK_AFTER=48h",
	})
	require.NoError(t, err)
	require.Len(t, report.Deprecated, 3)
	require.Equal(t, Defaults().Alerts.Settings(), loaded.Alerts.Settings())
	require.True(t, loaded.Alerts.Settings().Enabled)
}

func TestAlertsConfigValidation(t *testing.T) {
	require.NoError(t, AlertsConfig{}.validate())
	require.NoError(t, Defaults().Alerts.validate())
	require.NoError(t, AlertsConfig{PhaseStuckAfter: "0"}.validate())
	for name, cfg := range map[string]AlertsConfig{
		"unparsable interval": {CheckInterval: "often"},
		"interval too short":  {CheckInterval: "10s"},
		"renotify too short":  {RenotifyInterval: "30m"},
		"phase too short":     {PhaseStuckAfter: "10m"},
		"negative leaf days":  {LeafExpiryDays: -1},
		"negative ca days":    {CAExpiryDays: -5},
	} {
		require.Error(t, cfg.validate(), name)
	}
}

// A bad alerts block in an old file cannot stop the start: it is ignored.
func TestLoadIgnoresAnAlertsBlock(t *testing.T) {
	resetConfig()
	defer resetConfig()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("alerts:\n  check_interval: \"1s\"\n"), 0o600))
	loaded, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, Defaults().Alerts.Settings(), loaded.Alerts.Settings())
}
