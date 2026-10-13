package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLog collects what Load logs while fn runs.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buffer bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&buffer)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	fn()
	return buffer.String()
}

// A file from an earlier release that still sets the removed keys loads;
// each key is reported once with its reason and changes nothing.
func TestLoadIgnoresRemovedKeysWithAWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`env: development
database:
  driver: sqlite
plugins:
  official_public_key: "ZmFrZS1wdWJsaWMta2V5"
  control_execution_enabled: false
  control_poll_interval: "3s"
  dispatch_enabled: true
  dispatch_poll_interval: "not a duration"
  topology_execution_enabled: [unexpected, shape]
  topology_poll_interval:
package_routes:
  default_mode: legacy
`), 0o600))

	var loaded *Config
	var err error
	output := captureLog(t, func() { loaded, err = load(path, nil) })
	require.NoError(t, err)

	for _, key := range removedKeys {
		assert.Contains(t, output, "Ignoring the removed configuration key "+key.path+" (set in the configuration file): "+key.reason+"\n")
	}
	assert.Equal(t, len(removedKeys), strings.Count(output, "Ignoring the removed configuration key"))
	// The keys that remain apply as before.
	assert.Equal(t, "3s", loaded.Plugins.ControlPollInterval)
	assert.Equal(t, "ZmFrZS1wdWJsaWMta2V5", loaded.Plugins.OfficialPublicKey)
}

// The ANIX_CONTROL_* variables of the removed keys are reported as removed,
// not as unknown, and the file variants too.
func TestLoadIgnoresRemovedEnvironmentVariablesWithAWarning(t *testing.T) {
	var err error
	output := captureLog(t, func() {
		_, err = load("", []string{
			"ANIX_CONTROL_JWT_SECRET=container-secret",
			"ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED=false",
			"ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE=legacy",
			"ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED_FILE=/nonexistent",
			"ANIX_CONTROL_NOT_A_KEY=1",
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(output, "Ignoring the removed configuration key"))
	assert.Contains(t, output, "Ignoring the removed configuration key plugins.control_execution_enabled (set in ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED): the built-in packages always run")
	assert.Contains(t, output, "Ignoring the removed configuration key plugins.dispatch_enabled (set in ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED_FILE)")
	assert.Contains(t, output, "Ignoring the removed configuration key package_routes.default_mode (set in ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE)")
	assert.Contains(t, output, "Ignoring unknown configuration variable ANIX_CONTROL_NOT_A_KEY")
	assert.Equal(t, 1, strings.Count(output, "Ignoring unknown configuration variable"))
}

// The built-in defaults set no removed key, and no removed key is a key of
// the current configuration.
func TestRemovedKeysAreNotConfigurationKeys(t *testing.T) {
	found, _ := findRemovedKeys(defaultsYAML, nil)
	assert.Empty(t, found)
	current := map[string]bool{}
	for _, variable := range EnvVars(Defaults()) {
		current[variable.Path] = true
	}
	for _, key := range removedKeys {
		assert.NotEmpty(t, key.reason, key.path)
		assert.False(t, current[key.path], key.path)
	}
	for path := range current {
		assert.False(t, strings.HasPrefix(path, "package_routes."), path)
	}
}
