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

// TestInstallerFreshConfigIsProductionAndPassesServerValidation runs the
// systemd installer's fresh-config step (scripts/install.sh, through
// scripts/tests/test_install_fresh_config.sh) and loads the file it writes with
// the real loader. The installer downloads config/config.yaml.example, which
// says `env: "development"` for local use; a host install must be production,
// with a generated JWT secret that ValidateForServer accepts.
func TestInstallerFreshConfigIsProductionAndPassesServerValidation(t *testing.T) {
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

	// An empty environment: the host's ANIX_CONTROL_* variables must not
	// decide the result.
	loaded, err := load(generated, []string{})
	require.NoError(t, err)

	assert.Equal(t, "production", loaded.Env, "a fresh systemd install must not run in development mode")
	assert.Equal(t, "release", loaded.Server.Mode)
	require.NoError(t, loaded.ValidateForServer(), "the generated config must pass the production server checks")
	assert.NotEmpty(t, loaded.JWT.Secret)
	assert.False(t, placeholderJWTSecrets[loaded.JWT.Secret], "the JWT secret must not be a template value")
	assert.Empty(t, loaded.SecretStrengthWarnings(), "the generated secrets must not be weak")
	assert.NotEmpty(t, loaded.Admin.Password, "a fresh install generates the bootstrap admin password")
}
