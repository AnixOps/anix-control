package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runConfig(t *testing.T, arguments ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, handled := runConfigCommand(append([]string{"config"}, arguments...), &stdout, &stderr)
	require.True(t, handled)
	return code, stdout.String(), stderr.String()
}

func TestConfigCommandIsOnlyConfig(t *testing.T) {
	_, handled := runConfigCommand([]string{"doctor"}, &bytes.Buffer{}, &bytes.Buffer{})
	assert.False(t, handled)
	_, handled = runConfigCommand(nil, &bytes.Buffer{}, &bytes.Buffer{})
	assert.False(t, handled)
	code, _, stderr := runConfig(t)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "anix-control config rewrite --in <old> --out <new>")
	code, _, _ = runConfig(t, "lint")
	assert.Equal(t, 2, code)
}

func TestConfigCheck(t *testing.T) {
	dataDir := t.TempDir()
	write := func(text string) string {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
		return path
	}
	check := func(path string, env ...string) (int, string) {
		var stdout bytes.Buffer
		code := runConfigCheck([]string{"--file", path}, append([]string{"ANIX_CONTROL_DATA_DIR=" + dataDir}, env...), &stdout, &bytes.Buffer{})
		return code, stdout.String()
	}

	code, out := check(filepath.Join("..", "..", "config", "config.yaml.example"))
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "OK    tls.mode acme: Control obtains the certificate for panel.example.com")
	assert.Contains(t, out, "root key: the first start generates "+filepath.Join(dataDir, "root.key"))
	assert.NotContains(t, out, "WARN")

	old, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "testdata", "rewrite", "4.2-installed.yaml"))
	require.NoError(t, err)
	code, out = check(write(string(old)))
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "WARN  module_runtime.ca_kek (set in the configuration file) is deprecated and still read until M5")
	assert.Contains(t, out, "WARN  env (set in the configuration file) is no longer read and is ignored")
	assert.Contains(t, out, "root key: the next start adopts module_runtime.ca_kek")
	for _, secret := range []string{"AAECAwQF", "6f1b2c3d", "a-bootstrap-password"} {
		assert.NotContains(t, out, secret)
	}

	code, out = check(write("domian: panel.example.com\n"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "unknown configuration key domian; did you mean domain?")

	code, out = check(write("domain: panel.example.com\n"), "ANIX_CONTROL_DOMIAN=x")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "ignoring the unknown variable ANIX_CONTROL_DOMIAN (did you mean ANIX_CONTROL_DOMAIN?")

	// A root.key that does not hold the configured legacy key is refused,
	// as the start would refuse it.
	_, err = rootkey.Resolve(rootkey.Options{DataDir: dataDir, Write: true})
	require.NoError(t, err)
	code, out = check(write("module_runtime:\n  ca_kek: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "hold different keys")
	code, out = check(write("domain: panel.example.com\n"))
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "OK    root key "+filepath.Join(dataDir, "root.key"))
}

func TestConfigRewrite(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	old, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "testdata", "rewrite", "4.2-installed.yaml"))
	require.NoError(t, err)
	dir := t.TempDir()
	in := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(in, old, 0o640))
	require.NoError(t, os.Chmod(in, 0o640))
	dataDir := filepath.Join(dir, "data")
	rewrite := func(arguments ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := runConfigRewrite(arguments, now, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	// root.key does not hold the old file's secrets yet: nothing is
	// written.
	code, _, stderr := rewrite("--in", in, "--out", in, "--data-dir", dataDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "module_runtime.ca_kek is not the key in "+filepath.Join(dataDir, "root.key")+" (it does not exist yet)")
	assert.Contains(t, stderr, "nothing was written")
	unchanged, err := os.ReadFile(in)
	require.NoError(t, err)
	assert.Equal(t, old, unchanged)

	// The first start adopts them.
	_, err = rootkey.Resolve(rootkey.Options{DataDir: dataDir, Write: true,
		LegacyCAKEK:       "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
		LegacyJWTSecret:   "6f1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
		LegacyIdentityKEK: "IdiAOyOt8/iWo4XgQfl/6yBrnG+uhSkEdilcHlACMIw="})
	require.NoError(t, err)

	// Another --out that exists is refused.
	other := filepath.Join(dir, "other.yaml")
	require.NoError(t, os.WriteFile(other, []byte("domain: x.example.com\n"), 0o600))
	code, _, stderr = rewrite("--in", in, "--out", other, "--data-dir", dataDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "exists")

	// In place: the old file stays as a .bak copy with its mode.
	code, stdout, stderr := rewrite("--in", in, "--out", in, "--data-dir", dataDir)
	require.Equal(t, 0, code, stderr)
	backup := in + ".bak-20261013T093000Z"
	assert.Contains(t, stdout, "Wrote "+in)
	assert.Contains(t, stdout, "The old file is "+backup)
	assert.Contains(t, stdout, "domain: panel.example.com (from agent_install.public_url)")
	assert.Contains(t, stdout, "module_runtime.ca_kek: the root key, kept in "+filepath.Join(dataDir, "root.key"))
	assert.Contains(t, stdout, "server.trusted_proxies: still read until M2 (P11)")
	kept, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.Equal(t, old, kept)
	for _, path := range []string{in, backup} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), path)
	}
	written, err := os.ReadFile(in)
	require.NoError(t, err)
	assert.Contains(t, string(written), "domain: panel.example.com\n")
	for _, secret := range []string{"AAECAwQF", "6f1b2c3d", "IdiAOyOt", "a-bootstrap-password"} {
		assert.NotContains(t, string(written), secret)
		assert.NotContains(t, stdout, secret)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, []string{"config.yaml", "config.yaml.bak-20261013T093000Z", "data", "other.yaml"}, names, "no temporary file is left")

	// The rewritten file passes check.
	var out bytes.Buffer
	require.Equal(t, 0, runConfigCheck([]string{"--file", in}, []string{"ANIX_CONTROL_DATA_DIR=" + dataDir}, &out, &bytes.Buffer{}), out.String())
	assert.NotContains(t, strings.ReplaceAll(out.String(), "WARN  database.", ""), "WARN", "only the database section still warns")

	// --force drops a key root.key does not hold, and says so.
	forced := filepath.Join(dir, "forced.yaml")
	code, stdout, stderr = rewrite("--in", backup, "--out", forced, "--data-dir", filepath.Join(dir, "empty"), "--force")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "refused without --force: module_runtime.ca_kek")

	code, _, _ = rewrite("--in", in)
	assert.Equal(t, 2, code, "--out is required")
	code, _, stderr = rewrite("--in", filepath.Join(dir, "missing.yaml"), "--out", filepath.Join(dir, "n.yaml"))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no such file")
}
