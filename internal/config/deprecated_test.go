package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The keys the plugin center reduction (P7) removed are entries of the
// deprecated list like every other key: ignored with one warning each.
var p7RemovedKeys = []string{
	"plugins.control_execution_enabled",
	"plugins.dispatch_enabled",
	"plugins.dispatch_poll_interval",
	"plugins.topology_execution_enabled",
	"plugins.topology_poll_interval",
	"package_routes.default_mode",
}

// A file from an earlier release that still sets the removed keys loads,
// whatever their values' shape; each key is reported once with its reason
// and changes nothing.
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

	var lines []string
	restore := captureLog(t, &lines)
	loaded, err := load(path, nil)
	restore()
	require.NoError(t, err)
	output := strings.Join(lines, "\n") + "\n"

	for _, path := range p7RemovedKeys {
		key, ok := LookupKey(path)
		require.True(t, ok, path)
		assert.Equal(t, KeyIgnored, key.Status, path)
		assert.Contains(t, output, "Configuration: "+path+" (set in the configuration file) is no longer read and is ignored: "+key.Reason+".\n")
		assert.Equal(t, 1, strings.Count(output, "Configuration: "+path+" "), path)
	}
	// A key that is still read applies as before; a built-in value stands.
	assert.Equal(t, "ZmFrZS1wdWJsaWMta2V5", loaded.Plugins.OfficialPublicKey)
	assert.Equal(t, Defaults().Plugins.ControlPollInterval, loaded.Plugins.ControlPollInterval)
}

// The ANIX_CONTROL_* variables of the removed keys are reported as
// deprecated, not as unknown, and the file variants too (a file variant of
// an ignored key is never read).
func TestLoadIgnoresRemovedEnvironmentVariablesWithAWarning(t *testing.T) {
	var lines []string
	restore := captureLog(t, &lines)
	_, err := load("", []string{
		"ANIX_CONTROL_JWT_SECRET=container-secret-0123456789abcdef",
		"ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED=false",
		"ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE=legacy",
		"ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED_FILE=/nonexistent",
		"ANIX_CONTROL_NOT_A_KEY=1",
	})
	restore()
	require.NoError(t, err)
	output := strings.Join(lines, "\n")
	assert.Equal(t, 3, strings.Count(output, "is no longer read and is ignored"))
	assert.Contains(t, output, "Configuration: plugins.control_execution_enabled (set in ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED) is no longer read and is ignored: the built-in packages always run")
	assert.Contains(t, output, "Configuration: plugins.dispatch_enabled (set in ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED_FILE)")
	assert.Contains(t, output, "Configuration: package_routes.default_mode (set in ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE)")
	assert.Contains(t, output, "Configuration: jwt.secret (set in ANIX_CONTROL_JWT_SECRET) is deprecated and still read")
	assert.Contains(t, output, "ignoring the unknown variable ANIX_CONTROL_NOT_A_KEY")
	assert.Equal(t, 1, strings.Count(output, "ignoring the unknown variable"))
}

// The built-in defaults set no deprecated key, and no removed key is an
// operator key or a field of the configuration.
func TestRemovedKeysAreNotConfigurationKeys(t *testing.T) {
	_, report, err := LoadFile("", nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	current := map[string]bool{}
	for _, variable := range EnvVars(Defaults()) {
		current[variable.Path] = true
	}
	for _, path := range p7RemovedKeys {
		key, _ := LookupKey(path)
		assert.NotEmpty(t, key.Reason, path)
		assert.False(t, current[path], path)
		_, isField := configFieldTypes[path]
		assert.False(t, isField, "%s has no field any more", path)
	}
	for path := range configFieldTypes {
		assert.False(t, strings.HasPrefix(path, "package_routes."), path)
	}
}

// admin.* (the first administrator before the claim link, N10) is ignored:
// a file or an environment that still sets it loads, with one warning per
// key that never shows the password, and nothing reads it.
func TestAdminKeysAreIgnoredWithAWarning(t *testing.T) {
	const password = "operator-chosen-passphrase-7"
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("domain: panel.example.com\nadmin:\n  email: \"owner@example.com\"\n  password: \""+password+"\"\n"), 0o600))
	var lines []string
	restore := captureLog(t, &lines)
	_, report, err := LoadFile(path, []string{"ANIX_CONTROL_ADMIN_PASSWORD=" + password, "ANIX_CONTROL_ADMIN_EMAIL_FILE=/nonexistent"})
	restore()
	require.NoError(t, err)
	var paths []string
	for _, key := range report.Deprecated {
		paths = append(paths, key.Path)
		assert.Equal(t, KeyIgnored, key.Status, key.Path)
	}
	assert.Equal(t, []string{"admin.email", "admin.password"}, paths)
	output := strings.Join(report.Warnings(), "\n") + strings.Join(lines, "\n")
	assert.NotContains(t, output, password)
	assert.Contains(t, output, "Configuration: admin.password (set in the configuration file and ANIX_CONTROL_ADMIN_PASSWORD) is no longer read and is ignored")
	assert.Contains(t, output, "anix-control admin claim-link")
	_, field := configFieldTypes["admin.password"]
	assert.False(t, field, "no configuration field holds it")
}
