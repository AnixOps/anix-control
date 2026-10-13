package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInstallerFreshConfigScript runs the systemd installer's fresh-config step
// (scripts/install.sh, through scripts/tests/test_install_fresh_config.sh) and
// returns the configuration it writes for a default install.
func runInstallerFreshConfigScript(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the systemd installer and its test need a Linux userland")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	script := filepath.Join("..", "..", "scripts", "tests", "test_install_fresh_config.sh")
	generated := filepath.Join(t.TempDir(), "config.yaml")

	command := exec.Command(bash, script)
	command.Env = append(os.Environ(), "ANIX_INSTALL_FRESH_CONFIG_OUT="+generated)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "installer fresh-config test failed:\n%s", output)
	return generated
}

// TestInstallerFreshConfigPassesServerValidation loads the file the installer
// writes with the real loader. The installer downloads
// config/config.yaml.example (the domain line, which it keeps a comment)
// and adds nothing: the result loads with no unknown and no deprecated key,
// runs as production in release mode, holds no secret and no administrator
// (the first one claims the panel with the installer's link), and passes
// the server's checks once the root key derives the JWT secret.
func TestInstallerFreshConfigPassesServerValidation(t *testing.T) {
	generated := runInstallerFreshConfigScript(t)

	// An empty environment: the host's ANIX_CONTROL_* variables must not
	// decide the result.
	loaded, report, err := LoadFile(generated, []string{})
	require.NoError(t, err)

	assert.Equal(t, "production", loaded.Env, "a fresh systemd install never runs in development mode")
	assert.Equal(t, "release", loaded.Server.Mode)
	assert.Empty(t, loaded.Domain, "the installer does not ask for the domain yet")
	assert.Equal(t, TLSModeExternal, loaded.TLSModeOrDefault())
	assert.Equal(t, []string{DefaultListenAddress}, loaded.Panel().PlainAddresses, "the install's health check reaches 127.0.0.1:8080")
	assert.Empty(t, loaded.JWT.Secret, "the root key derives the JWT secret")
	assert.Empty(t, loaded.App.APIToken)
	assert.Empty(t, loaded.ModuleRuntime.CAKEK, "the CA key is a file the unit passes, not a config key")
	assert.Empty(t, report.Deprecated, "the installer writes no deprecated key")
	require.NoError(t, loaded.ValidateForServer(), "the generated config must pass the server checks")
	assert.Empty(t, loaded.SecretStrengthWarnings(), "the generated secrets must not be weak")
}
