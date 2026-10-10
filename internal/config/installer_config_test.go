package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInstallerFreshConfigScript runs the systemd installer's fresh-config step
// (scripts/install.sh, through scripts/tests/test_install_fresh_config.sh) and
// returns the configuration it writes for a default install and the directory
// that holds the files it wrote for each awkward-credential case
// (<name>.yaml, <name>.email, <name>.password: the values the installer was
// given, byte for byte).
func runInstallerFreshConfigScript(t *testing.T) (generated, casesDir string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the systemd installer and its test need a Linux userland")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	script := filepath.Join("..", "..", "scripts", "tests", "test_install_fresh_config.sh")
	dir := t.TempDir()
	generated = filepath.Join(dir, "config.yaml")
	casesDir = filepath.Join(dir, "cases")

	command := exec.Command(bash, script)
	command.Env = append(os.Environ(),
		"ANIX_INSTALL_FRESH_CONFIG_OUT="+generated,
		"ANIX_INSTALL_FRESH_CONFIG_CASES_DIR="+casesDir,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "installer fresh-config test failed:\n%s", output)
	return generated, casesDir
}

// TestInstallerFreshConfigIsProductionAndPassesServerValidation loads the file
// the installer writes with the real loader. The installer downloads
// config/config.yaml.example, which says `env: "development"` for local use; a
// host install must be production, with a generated JWT secret that
// ValidateForServer accepts.
func TestInstallerFreshConfigIsProductionAndPassesServerValidation(t *testing.T) {
	generated, _ := runInstallerFreshConfigScript(t)

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

// TestInstallerDefaultAdminEmailPassesTheLoginCheck: without --admin-email the
// installer picks the bootstrap administrator's address, and the installer's
// own login check posts it to /api/v2/login, which binds model.LoginRequest
// (`required,email`). The earlier default, admin@localhost, has no dot in its
// domain and is refused, so a fresh install left a running service whose
// bootstrap login check failed.
func TestInstallerDefaultAdminEmailPassesTheLoginCheck(t *testing.T) {
	generated, _ := runInstallerFreshConfigScript(t)
	loaded, err := load(generated, []string{})
	require.NoError(t, err)

	login := func(email string) error {
		return binding.Validator.ValidateStruct(&model.LoginRequest{Email: email, Password: "x"})
	}
	require.Error(t, login("admin@localhost"),
		"the validator must still refuse a bare host name, or this test no longer proves anything")
	require.NotEmpty(t, loaded.Admin.Email)
	assert.NoError(t, login(loaded.Admin.Email),
		"the default administrator email %q must pass the server's login validation", loaded.Admin.Email)
}

// TestInstallerWritesCredentialsThatLoadVerbatim loads, with the real loader,
// every config the installer wrote for an operator-supplied email and password
// with characters awk, the shell or YAML treat specially. The values must come
// back byte for byte; awk -v used to unescape the already escaped value a
// second time, so a password with a backslash ("ab\qcd") did not load at all.
func TestInstallerWritesCredentialsThatLoadVerbatim(t *testing.T) {
	_, casesDir := runInstallerFreshConfigScript(t)

	files, err := filepath.Glob(filepath.Join(casesDir, "*.yaml"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(files), 10, "the script must write its awkward-credential cases")

	var passwords, emails strings.Builder
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		t.Run(name, func(t *testing.T) {
			email, err := os.ReadFile(strings.TrimSuffix(file, ".yaml") + ".email")
			require.NoError(t, err)
			password, err := os.ReadFile(strings.TrimSuffix(file, ".yaml") + ".password")
			require.NoError(t, err)
			passwords.Write(password)
			emails.Write(email)

			loaded, err := load(file, []string{})
			require.NoError(t, err, "the installer wrote a config the loader cannot read")
			assert.Equal(t, string(email), loaded.Admin.Email)
			assert.Equal(t, string(password), loaded.Admin.Password)
			assert.Equal(t, "production", loaded.Env)
			assert.NotEmpty(t, loaded.JWT.Secret)

			// The install's own login check posts the same values as JSON.
			var login struct{ Email, Password string }
			require.NoError(t, json.Unmarshal(installerLoginPayload(email, password), &login),
				"the installer's login check would post invalid JSON for these values")
			assert.Equal(t, string(email), login.Email)
			assert.Equal(t, string(password), login.Password)
		})
	}

	// The cases must cover what the installer is expected to carry.
	for name, character := range map[string]string{
		"backslash": `\`, "double quote": `"`, "single quote": `'`, "dollar": `$`,
		"ampersand": `&`, "hash": `#`, "space": ` `, "non-ASCII": "密",
		"no-break space": "\u00a0", "zero width no-break space": "\ufeff", "replacement character": "\ufffd",
		"character beside the line separator": "\u2027", "character after the paragraph separator": "\u202a",
		"astral plane": "\U00010000", "last code point": "\U0010ffff",
	} {
		assert.Contains(t, passwords.String(), character, "no password case contains a %s", name)
	}
	assert.Contains(t, emails.String(), `\`, "no email case contains a backslash")
}

// installerLoginPayload is the body verify_identity_login builds in
// scripts/install.sh: json_escape escapes only the backslash and the double
// quote.
func installerLoginPayload(email, password []byte) []byte {
	escape := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return []byte(`{"email":"` + escape.Replace(string(email)) + `","password":"` + escape.Replace(string(password)) + `"}`)
}

// TestInstallerRefusesCredentialsTheConfigCannotCarry: every value the
// installer refuses (scripts/tests/test_install_fresh_config.sh, section 7)
// must be one that the loader or the install's own login check really cannot
// carry. A refusal that protects nothing is a rule an operator has to work
// around; and the documentation promises that everything else is written
// exactly as given (TestInstallerWritesCredentialsThatLoadVerbatim).
//
// The config is written the way the installer writes an accepted value: a
// double-quoted scalar with the backslash and the double quote escaped.
func TestInstallerRefusesCredentialsTheConfigCannotCarry(t *testing.T) {
	_, casesDir := runInstallerFreshConfigScript(t)

	files, err := filepath.Glob(filepath.Join(casesDir, "refused", "*.password"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(files), 20, "the script must write the values it refuses")

	escape := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".password")
		t.Run(name, func(t *testing.T) {
			password, err := os.ReadFile(file)
			require.NoError(t, err)
			email, err := os.ReadFile(strings.TrimSuffix(file, ".password") + ".email")
			require.NoError(t, err)

			config := filepath.Join(t.TempDir(), "config.yaml")
			yaml := "env: \"production\"\nadmin:\n  email: \"" + escape.Replace(string(email)) +
				"\"\n  password: \"" + escape.Replace(string(password)) + "\"\n"
			require.NoError(t, os.WriteFile(config, []byte(yaml), 0o600))

			loaded, loadErr := load(config, []string{})
			carriedByConfig := loadErr == nil &&
				loaded.Admin.Email == string(email) && loaded.Admin.Password == string(password)

			var login struct{ Email, Password string }
			jsonErr := json.Unmarshal(installerLoginPayload(email, password), &login)
			carriedByLogin := jsonErr == nil &&
				login.Email == string(email) && login.Password == string(password)

			assert.False(t, carriedByConfig && carriedByLogin,
				"the installer refuses %q / %q, but config.yaml and the login check carry them unchanged", email, password)
		})
	}
}
