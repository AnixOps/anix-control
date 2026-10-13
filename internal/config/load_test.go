package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

func deprecatedPaths(report LoadReport) []string {
	var paths []string
	for _, key := range report.Deprecated {
		paths = append(paths, key.Path)
	}
	return paths
}

// The built-in defaults, then the file key by key, then the environment.
func TestLoadPrecedenceDefaultsFileEnvironment(t *testing.T) {
	// Defaults alone.
	loaded, report, err := LoadFile("", nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	assert.Equal(t, "", loaded.Domain)
	assert.Equal(t, "/var/lib/anix-control", loaded.DataDir)
	assert.Equal(t, 50051, loaded.GRPC.Port)
	assert.Equal(t, "s", loaded.App.SubscribePath)
	assert.Equal(t, []string{DefaultListenAddress}, loaded.Panel().PlainAddresses)

	// The file overrides one key and keeps every other default (the file
	// no longer replaces the defaults as a whole).
	path := writeConfig(t, "domain: panel.example.com\ngrpc:\n  port: 50061\n")
	loaded, report, err = LoadFile(path, nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	assert.Equal(t, "panel.example.com", loaded.Domain)
	assert.Equal(t, 50061, loaded.GRPC.Port)
	assert.Equal(t, "/var/lib/anix-control", loaded.DataDir, "a default the file does not set")
	assert.Equal(t, Defaults().Alerts, loaded.Alerts)
	assert.Equal(t, TLSModeACME, loaded.TLSModeOrDefault())

	// The environment overrides the file.
	loaded, _, err = LoadFile(path, []string{"ANIX_CONTROL_GRPC_PORT=50071", "ANIX_CONTROL_DATA_DIR=/srv/control", "ANIX_CONTROL_SUBSCRIBE_PATH=/sub/"})
	require.NoError(t, err)
	assert.Equal(t, 50071, loaded.GRPC.Port)
	assert.Equal(t, "/srv/control", loaded.DataDir)
	assert.Equal(t, "sub", loaded.App.SubscribePath)
	assert.Equal(t, "panel.example.com", loaded.Domain)
}

func TestLoadEveryOperatorKey(t *testing.T) {
	path := writeConfig(t, `domain: Panel.Example.com
tls:
  mode: external
  cert_file: /etc/anix-control/tls/fullchain.pem
  key_file: /etc/anix-control/tls/privkey.pem
listen: 127.0.0.1:9080
grpc:
  port: 50052
  public_name: grpc.example.com
subscribe_path: link
data_dir: /srv/anix-control/
`)
	loaded, report, err := LoadFile(path, nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	panel := loaded.Panel()
	assert.Equal(t, "panel.example.com", panel.Domain)
	assert.Equal(t, TLSModeExternal, panel.Mode)
	assert.Equal(t, []string{"127.0.0.1:9080"}, panel.PlainAddresses)
	assert.Equal(t, "grpc.example.com:50052", panel.GRPCTarget)
	assert.Equal(t, GRPCCertFiles, panel.GRPCCertSource)
	assert.Equal(t, "/etc/anix-control/tls/fullchain.pem", panel.GRPCCertFile)
	assert.Equal(t, "link", loaded.App.SubscribePath)
	assert.Equal(t, "/srv/anix-control", loaded.DataDir)
}

func TestLoadRefusesUnknownKeysNamingTheClosest(t *testing.T) {
	for text, want := range map[string]string{
		"domian: panel.example.com\n":                              "config.yaml:1: unknown configuration key domian; did you mean domain?",
		"domain: a.example.com\ntls:\n  mod: acme\n":               "config.yaml:3: unknown configuration key tls.mod; did you mean tls.mode?",
		"grpc:\n  prot: 1\n":                                       "unknown configuration key grpc.prot; did you mean grpc.port?",
		"datadir: /x\n":                                            "did you mean data_dir?",
		"databse:\n  host: x\n":                                    "unknown configuration key databse; did you mean database",
		"forward_runtime:\n  nftables_ansible:\n    inventry: x\n": "did you mean forward_runtime.nftables_ansible.inventory?",
	} {
		_, _, err := LoadFile(writeConfig(t, text), nil)
		require.Error(t, err, text)
		assert.Contains(t, err.Error(), want, text)
	}
}

func TestLoadRefusesMalformedValues(t *testing.T) {
	for text, want := range map[string]string{
		"grpc:\n  port: many\n":                          "invalid grpc.port",
		"grpc: 50051\n":                                  "grpc is a section of keys, not a value",
		"domain: a\ndomain: b\n":                         "domain is set twice",
		"- domain\n":                                     "must be a mapping of keys",
		"data_dir: relative/dir\n":                       "invalid data_dir",
		"domain: https://panel.example.com\n":            "invalid domain",
		"base: &b {port: 1}\ngrpc:\n  <<: *b\n":          "unknown configuration key base",
		"grpc:\n  <<: {port: 1}\n":                       "merge keys",
		"tls:\n  mode: sometimes\n":                      "invalid tls.mode",
		"server:\n  trusted_proxies: [nope]\n":           "server.trusted_proxies",
		"subscribe_path: api/v2\n":                       "invalid subscribe_path",
		"subscribe_path: a\napp:\n  subscribe_path: b\n": "keep subscribe_path",
	} {
		_, _, err := LoadFile(writeConfig(t, text), nil)
		require.Error(t, err, text)
		assert.Contains(t, err.Error(), want, text)
	}
}

func TestLoadAcceptsEmptyAndNullValues(t *testing.T) {
	for _, text := range []string{"", "\n# only a comment\n", "~\n", "domain:\ngrpc:\n"} {
		loaded, report, err := LoadFile(writeConfig(t, text), nil)
		require.NoError(t, err, text)
		assert.Empty(t, report.Deprecated)
		assert.Equal(t, 50051, loaded.GRPC.Port)
		assert.Equal(t, "/var/lib/anix-control", loaded.DataDir, "null keeps the default")
	}
}

// The renamed key and its replacement agree or the load stops.
func TestLoadRenamedSubscribePath(t *testing.T) {
	loaded, report, err := LoadFile(writeConfig(t, "app:\n  subscribe_path: old\n"), nil)
	require.NoError(t, err)
	assert.Equal(t, "old", loaded.App.SubscribePath)
	assert.Equal(t, []string{"app.subscribe_path"}, deprecatedPaths(report))
	assert.Contains(t, report.Deprecated[0].Warning(), "Use subscribe_path.")

	loaded, _, err = LoadFile(writeConfig(t, "subscribe_path: same\napp:\n  subscribe_path: same\n"), nil)
	require.NoError(t, err)
	assert.Equal(t, "same", loaded.App.SubscribePath)

	loaded, _, err = LoadFile("", []string{"ANIX_CONTROL_APP_SUBSCRIBE_PATH=old"})
	require.NoError(t, err)
	assert.Equal(t, "old", loaded.App.SubscribePath)
	_, _, err = LoadFile("", []string{"ANIX_CONTROL_APP_SUBSCRIBE_PATH=old", "ANIX_CONTROL_SUBSCRIBE_PATH=new"})
	require.ErrorContains(t, err, "keep subscribe_path")
}

// A deprecated key warns once, whichever sources set it, and the warning
// never carries its value.
func TestDeprecatedKeysWarnOnceAndNeverShowTheirValue(t *testing.T) {
	path := writeConfig(t, "jwt:\n  secret: file-secret-value-1234567890\n  expire: 60\nenv: development\n")
	loaded, report, err := LoadFile(path, []string{"ANIX_CONTROL_JWT_SECRET=env-secret-value-1234567890", "ANIX_CONTROL_ENV=staging"})
	require.NoError(t, err)
	assert.Equal(t, []string{"env", "jwt.expire", "jwt.secret"}, deprecatedPaths(report))
	warnings := report.Warnings()
	require.Len(t, warnings, 3)
	for _, warning := range warnings {
		assert.NotContains(t, warning, "secret-value")
		assert.NotContains(t, warning, "staging")
	}
	assert.Contains(t, warnings[0], "env (set in the configuration file and ANIX_CONTROL_ENV) is no longer read")
	assert.Contains(t, warnings[2], "jwt.secret (set in the configuration file and ANIX_CONTROL_JWT_SECRET) is deprecated and still read until M5")
	assert.Equal(t, "env-secret-value-1234567890", loaded.JWT.Secret, "still read: the environment wins")
	assert.Equal(t, 86400, loaded.JWT.Expire, "ignored: the built-in value stands")
	assert.Equal(t, "production", loaded.Env)
	assert.Equal(t, deprecatedPaths(report), func() []string {
		var paths []string
		for _, key := range loaded.Deprecated.Keys() {
			paths = append(paths, key.Path)
		}
		return paths
	}())
}

// deprecatedFixture is a configuration that sets every deprecated key: the
// keys with a field to their built-in value, the others to a placeholder.
func deprecatedFixture(t *testing.T) (string, []string) {
	t.Helper()
	defaults := Defaults()
	document := map[string]any{}
	var envs []string
	set := func(path string, value any) {
		node := document
		parts := strings.Split(path, ".")
		for _, part := range parts[:len(parts)-1] {
			next, ok := node[part].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[part] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = value
	}
	for _, key := range Keys() {
		if !key.Deprecated() {
			continue
		}
		field, ok := configField(defaults, key.fieldPath())
		switch {
		case key.Subtree:
			set(key.Path, map[string]any{"inventory": "config/deploy/ansible/inventory.ini", "become": false})
			continue
		case !ok:
			set(key.Path, "community")
			envs = append(envs, key.EnvName()+"=community")
			continue
		}
		switch field.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
			if field.IsNil() {
				// Unset: null keeps the built-in value, and a variable
				// cannot say "unset".
				set(key.Path, nil)
				continue
			}
		}
		set(key.Path, field.Interface())
		if envCapable(key) {
			envs = append(envs, key.EnvName()+"="+formatEnvValue(field))
		}
	}
	data, err := yaml.Marshal(document)
	require.NoError(t, err)
	return writeConfig(t, string(data)), envs
}

// Every deprecated key in a file, or in the environment, warns, and Control
// starts with the effective settings it has without them.
func TestEveryDeprecatedKeyWarnsAndChangesNothing(t *testing.T) {
	path, envs := deprecatedFixture(t)
	baseline, _, err := LoadFile("", nil)
	require.NoError(t, err)

	var deprecated []string
	for _, key := range Keys() {
		if key.Deprecated() {
			deprecated = append(deprecated, key.Path)
		}
	}

	fromFile, report, err := LoadFile(path, nil)
	require.NoError(t, err)
	assert.Equal(t, deprecated, deprecatedPaths(report), "every deprecated key warns once")
	fromFile.Deprecated = DeprecatedConfig{}
	baseline.Deprecated = DeprecatedConfig{}
	assert.Equal(t, baseline, fromFile)

	fromEnv, report, err := LoadFile("", envs)
	require.NoError(t, err)
	assert.Len(t, report.Deprecated, len(envs))
	assert.Empty(t, report.UnknownEnv)
	fromEnv.Deprecated = DeprecatedConfig{}
	// A variable cannot tell an empty list from an unset one.
	fromEnv.Server.CORS.AllowedOrigins = baseline.Server.CORS.AllowedOrigins
	assert.Equal(t, baseline, fromEnv)
}

// An ignored key never reaches its field, whatever its value.
func TestIgnoredKeysKeepTheBuiltInValue(t *testing.T) {
	defaults := Defaults()
	for _, key := range Keys() {
		if key.Status != KeyIgnored || key.Subtree {
			continue
		}
		field, ok := configField(defaults, key.fieldPath())
		value := "an-ignored-value"
		if ok {
			switch field.Kind() {
			case reflect.Int, reflect.Int64:
				value = "424242"
			case reflect.Bool, reflect.Pointer:
				value = fmt.Sprint(field.Kind() != reflect.Bool || !field.Bool())
			case reflect.Slice:
				value = "[ignored]"
			}
		}
		// One key, nested with two spaces per level.
		text := ""
		parts := strings.Split(key.Path, ".")
		for i, part := range parts {
			text += strings.Repeat("  ", i) + part + ":"
			if i == len(parts)-1 {
				text += " " + value
			}
			text += "\n"
		}
		loaded, report, err := LoadFile(writeConfig(t, text), nil)
		require.NoError(t, err, key.Path)
		assert.Equal(t, []string{key.Path}, deprecatedPaths(report), key.Path)
		if ok {
			got, _ := configField(loaded, key.fieldPath())
			assert.Equal(t, field.Interface(), got.Interface(), "%s must keep its built-in value", key.Path)
		}
	}
}

// The database section keeps its 4.x meaning while PostgreSQL is supported.
func TestDatabaseSectionKeepsItsMeaning(t *testing.T) {
	loaded, _, err := LoadFile("", nil)
	require.NoError(t, err)
	assert.Equal(t, "postgres", loaded.Database.Driver, "the container default")

	loaded, report, err := LoadFile(writeConfig(t, "domain: panel.example.com\n"), nil)
	require.NoError(t, err)
	assert.Empty(t, report.Deprecated)
	assert.Equal(t, DatabaseConfig{}, loaded.Database, "a file without database keys is SQLite at its default path, as in 4.x")

	loaded, report, err = LoadFile(writeConfig(t, "database:\n  driver: sqlite\n  database: data/control.db\n"), []string{"ANIX_CONTROL_DATABASE_LOG_LEVEL=silent"})
	require.NoError(t, err)
	assert.Equal(t, DatabaseConfig{Driver: "sqlite", Database: "data/control.db", LogLevel: "silent"}, loaded.Database)
	assert.Equal(t, []string{"database.database", "database.driver", "database.log_level"}, deprecatedPaths(report))
	assert.Contains(t, report.Deprecated[1].Warning(), "still read until M2 (P4)")

	loaded, _, err = LoadFile("", []string{"ANIX_CONTROL_DATABASE_HOST=db.internal", "ANIX_CONTROL_DATABASE_PASSWORD=pw"})
	require.NoError(t, err)
	assert.Equal(t, "postgres", loaded.Database.Driver)
	assert.Equal(t, "db.internal", loaded.Database.Host)
	assert.Equal(t, "anix_control", loaded.Database.Username)
}

// The legacy listener keys still place the plain listener of external mode
// while listen is unset; listen wins.
func TestLegacyListenerKeys(t *testing.T) {
	loaded, report, err := LoadFile(writeConfig(t, "server:\n  host: 0.0.0.0\n  port: 9080\nfrontend:\n  port: 9000\n"), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"frontend.port", "server.host", "server.port"}, deprecatedPaths(report))
	assert.Equal(t, []string{"0.0.0.0:9080", "0.0.0.0:9000"}, loaded.Panel().PlainAddresses)

	loaded, _, err = LoadFile("", []string{"ANIX_CONTROL_SERVER_HOST=0.0.0.0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"0.0.0.0:8080"}, loaded.Panel().PlainAddresses, "the container default of 4.x")

	loaded, _, err = LoadFile("", []string{"ANIX_CONTROL_SERVER_PORT=9080", "ANIX_CONTROL_LISTEN=0.0.0.0:8080"})
	require.NoError(t, err)
	assert.Equal(t, []string{"0.0.0.0:8080"}, loaded.Panel().PlainAddresses)
}

func TestLoadEnvironment(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "jwt")
	require.NoError(t, os.WriteFile(secretFile, []byte("file-secret\n"), 0o600))

	loaded, report, err := LoadFile("", []string{
		"ANIX_CONTROL_DOMAIN=panel.example.com",
		"ANIX_CONTROL_JWT_SECRET_FILE=" + secretFile,
		"ANIX_CONTROL_SERVER_TRUSTED_PROXIES=10.0.0.0/8, 192.168.0.0/16 ,",
		"ANIX_CONTROL_AUTH_REGISTRATION_ENABLED=false",
		"ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TOKEN=nodex-token",
		"ANIX_CONTROL_DATABSE_PASSWORD=typo",
		"ANIX_CONTROL_CONFIG=/etc/anix-control/config.yaml",
		"ANIX_CONTROL_HOST_SOCKET=/proc/self/fd/3/host.sock",
		"ANIX_CONTROL_ALERTS_CHECK_INTERVAL_FILE=/nonexistent/is/never/read",
		"UNRELATED=1",
	})
	require.NoError(t, err)
	assert.Equal(t, "panel.example.com", loaded.Domain)
	assert.Equal(t, "file-secret", loaded.JWT.Secret)
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, loaded.Server.TrustedProxies)
	require.NotNil(t, loaded.Auth.Registration.Enabled)
	assert.False(t, *loaded.Auth.Registration.Enabled)
	assert.Equal(t, "nodex-token", loaded.ForwardRuntime.NodeX.Token)
	assert.Equal(t, []UnknownVariable{{Name: "ANIX_CONTROL_DATABSE_PASSWORD", Closest: "ANIX_CONTROL_DATABASE_PASSWORD"}}, report.UnknownEnv)
	assert.Contains(t, report.Warnings()[len(report.Warnings())-1], "did you mean ANIX_CONTROL_DATABASE_PASSWORD?")
	var sources []string
	for _, key := range report.Deprecated {
		sources = append(sources, key.Source)
	}
	assert.Contains(t, sources, "ANIX_CONTROL_JWT_SECRET_FILE")
	assert.Contains(t, sources, "ANIX_CONTROL_ALERTS_CHECK_INTERVAL_FILE")

	for env, want := range map[string]string{
		"ANIX_CONTROL_GRPC_PORT=abc":                                            "ANIX_CONTROL_GRPC_PORT",
		"ANIX_CONTROL_GRPC_ENABLED=maybe":                                       "ANIX_CONTROL_GRPC_ENABLED",
		"ANIX_CONTROL_DATABASE_SSLMODE=often":                                   "database.sslmode",
		"ANIX_CONTROL_DATABASE_TIMEZONE=Mars/X":                                 "database.timezone",
		"ANIX_CONTROL_LOG_FORMAT=xml":                                           "log.format",
		"ANIX_CONTROL_JWT_SECRET_FILE=" + filepath.Join(t.TempDir(), "missing"): "ANIX_CONTROL_JWT_SECRET_FILE",
	} {
		_, _, err := LoadFile("", []string{env})
		require.ErrorContains(t, err, want, env)
	}
	_, _, err = LoadFile("", []string{"ANIX_CONTROL_JWT_SECRET=x", "ANIX_CONTROL_JWT_SECRET_FILE=" + secretFile})
	require.ErrorContains(t, err, "use one")
}

func TestLoadLogsEachWarningOnce(t *testing.T) {
	var lines []string
	restore := captureLog(t, &lines)
	defer restore()
	_, err := load(writeConfig(t, "env: production\nserver:\n  mode: release\n"), []string{"ANIX_CONTROL_SERVER_MODE=debug"})
	require.NoError(t, err)
	assert.Len(t, lines, 2)
}
