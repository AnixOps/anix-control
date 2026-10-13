package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rewriteNow stamps the golden files.
var rewriteNow = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

// adoptedRootKey is the root.key a start with the 4.2-installed fixture
// writes: its ca_kek, jwt.secret and identity.kek.
func adoptedRootKey(t *testing.T) *rootkey.File {
	t.Helper()
	key, err := rootkey.ParseKEK("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	require.NoError(t, err)
	return &rootkey.File{
		Key:                key,
		AdoptedJWTSecret:   "6f1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
		AdoptedIdentityKEK: "IdiAOyOt8/iWo4XgQfl/6yBrnG+uhSkEdilcHlACMIw=",
	}
}

// The rewrite of every shipped 4.0, 4.1 and 4.2 template, of an installed
// 4.2 configuration and of an M2 configuration, against golden files.
// Regenerate them with UPDATE_GOLDEN=1 GOWORK=off go test ./internal/config -run TestRewriteGolden.
func TestRewriteGolden(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("testdata", "rewrite", "*.yaml"))
	require.NoError(t, err)
	require.Len(t, cases, 9)
	for _, input := range cases {
		name := strings.TrimSuffix(filepath.Base(input), ".yaml")
		t.Run(name, func(t *testing.T) {
			old, err := os.ReadFile(input)
			require.NoError(t, err)
			opts := RewriteOptions{
				Source: "/opt/anixops/control/config/config.yaml", Now: rewriteNow,
				ResolvePath:    func(path string) string { return "/opt/anixops/control/" + path },
				RootKeyPath:    "/var/lib/anix-control/root.key",
				RootKeyProblem: "it does not exist yet",
			}
			if name == "4.2-installed" {
				opts.RootKey, opts.RootKeyProblem = adoptedRootKey(t), ""
			}
			result, err := Rewrite(old, opts)
			require.NoError(t, err)
			assert.Empty(t, result.Refused)
			got := "# --- the new file ---\n" + string(result.Output) + "# --- the report ---" + result.Report()
			golden := filepath.Join("testdata", "rewrite", name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
				return
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err, "run UPDATE_GOLDEN=1 to create %s", golden)
			assert.Equal(t, string(want), got)

			// The new file loads without any deprecated key but the
			// database section, and twice is the same as once.
			loaded, report, err := loadBytes(result.Output)
			require.NoError(t, err)
			for _, key := range report.Deprecated {
				assert.True(t, strings.HasPrefix(key.Path, "database."), key.Path)
			}
			again, err := Rewrite(result.Output, opts)
			require.NoError(t, err)
			reloaded, _, err := loadBytes(again.Output)
			require.NoError(t, err)
			loaded.Deprecated, reloaded.Deprecated = DeprecatedConfig{}, DeprecatedConfig{}
			assert.Equal(t, loaded, reloaded)
		})
	}
}

// The installed 4.2 file keeps what nodes and users see: the domain, the
// gRPC name, port and certificate, the subscription path and the database.
func TestRewriteKeepsWhatNodesAndUsersSee(t *testing.T) {
	old, err := os.ReadFile(filepath.Join("testdata", "rewrite", "4.2-installed.yaml"))
	require.NoError(t, err)
	before, _, err := loadBytes(old)
	require.NoError(t, err)
	result, err := Rewrite(old, RewriteOptions{Now: rewriteNow, RootKey: adoptedRootKey(t), RootKeyPath: "/var/lib/anix-control/root.key"})
	require.NoError(t, err)
	after, _, err := loadBytes(result.Output)
	require.NoError(t, err)

	assert.Equal(t, "panel.example.com", after.Panel().Domain)
	assert.Equal(t, TLSModeExternal, after.Panel().Mode)
	assert.Equal(t, before.Panel().GRPCTarget, after.Panel().GRPCTarget, "nodes dial the same name and port")
	assert.Equal(t, "grpc.example.com:50052", after.Panel().GRPCTarget)
	assert.Equal(t, before.GRPC.TLSCertFile, after.Panel().GRPCCertFile, "the same certificate")
	assert.Equal(t, before.App.SubscribePath, after.App.SubscribePath, "subscription links keep working")
	assert.Equal(t, before.Database, after.Database, "the same database")
	assert.Equal(t, "https://panel.example.com", after.Panel().PublicURL)
	for _, secret := range []string{"6f1b2c3d", "0a1b2c3d", "AAECAwQF", "IdiAOyOt", "a-bootstrap-password"} {
		assert.NotContains(t, string(result.Output), secret, "the new file holds no secret")
		assert.NotContains(t, result.Report(), secret, "the report shows no secret")
	}
}

// The old file's root key and identity KEK may be dropped only once
// root.key holds them; the JWT secret only costs a sign-in.
func TestRewriteRefusesToDropSecretsRootKeyDoesNotHold(t *testing.T) {
	old, err := os.ReadFile(filepath.Join("testdata", "rewrite", "4.2-installed.yaml"))
	require.NoError(t, err)
	result, err := Rewrite(old, RewriteOptions{Now: rewriteNow, RootKeyPath: "/var/lib/anix-control/root.key", RootKeyProblem: "it does not exist yet"})
	require.NoError(t, err)
	require.Len(t, result.Refused, 2)
	assert.Contains(t, result.Refused[0], "module_runtime.ca_kek is not the key in /var/lib/anix-control/root.key (it does not exist yet)")
	assert.Contains(t, result.Refused[1], "identity.kek is not adopted")
	assert.Contains(t, result.Report(), "jwt.secret: derived from the root key from now on: every user signs in again once")

	other, err := rootkey.Generate()
	require.NoError(t, err)
	result, err = Rewrite(old, RewriteOptions{Now: rewriteNow, RootKey: &rootkey.File{Key: other}, RootKeyPath: "/x/root.key"})
	require.NoError(t, err)
	require.Len(t, result.Refused, 2, "a root.key with another key does not hold the old one")
	for _, reason := range result.Refused {
		assert.NotContains(t, reason, "AAECAwQF")
	}
}

func TestRewriteSmallCases(t *testing.T) {
	for _, test := range []struct {
		name, old, want string
	}{
		{"empty", "", "# Every key has its default.\n"},
		{"a public_url with an address", "agent_install:\n  public_url: https://203.0.113.5:8443\n", "# Every key has its default.\n"},
		{"tls.domain alone", "tls:\n  domain: Panel.Example.com.\n", "domain: panel.example.com\n"},
		{"domain wins over public_url", "domain: a.example.com\nagent_install:\n  public_url: https://b.example.com\n", "domain: a.example.com\n"},
		{"acme drops the gRPC files", "domain: a.example.com\ngrpc:\n  tls_cert_file: /a\n  tls_key_file: /b\n", "domain: a.example.com\n"},
		{"files mode", "domain: a.example.com\ntls:\n  mode: files\n  cert_file: /a\n  key_file: /b\n", "domain: a.example.com\ntls:\n  mode: files\n  cert_file: /a\n  key_file: /b\n"},
		{"4.x https on the web port", "agent_install:\n  public_url: https://a.example.com\ntls:\n  enable: true\n  cert_file: /a\n  key_file: /b\n", "domain: a.example.com\ntls:\n  mode: files\n  cert_file: /a\n  key_file: /b\n"},
		{"grpc_target names the domain", "agent_install:\n  public_url: https://a.example.com\n  grpc_target: a.example.com:50051\n", "domain: a.example.com\n"},
		{"data_dir", "data_dir: /srv/anix\n", "data_dir: /srv/anix\n"},
		{"unknown keys", "colour: blue\nserver:\n  speed: 3\n", "# Every key has its default.\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Rewrite([]byte(test.old), RewriteOptions{Now: rewriteNow})
			require.NoError(t, err)
			body := string(result.Output)
			body = body[strings.Index(body, "not in this file.\n")+len("not in this file.\n"):]
			assert.Equal(t, test.want, body)
		})
	}
	_, err := Rewrite([]byte("- a list\n"), RewriteOptions{})
	require.Error(t, err)
	_, err = Rewrite([]byte("app:\n  subscribe_path: api/v2\n"), RewriteOptions{})
	require.ErrorContains(t, err, "subscribe_path")
}

// A closed or restricted registration and a cluster name survive the
// rewrite while they are still read; their defaults are dropped.
func TestRewriteKeepsAClosedRegistrationAndTheCluster(t *testing.T) {
	old := "auth:\n  registration:\n    enabled: false\n    require_invite: false\n    allowed_email_domains: [example.com]\n    blocked_email_domains: []\n  login_rate_limit:\n    enabled: false\nmodule_runtime:\n  cluster: prod\n  enabled: false\n"
	result, err := Rewrite([]byte(old), RewriteOptions{Now: rewriteNow})
	require.NoError(t, err)
	body := string(result.Output)
	assert.Contains(t, body, "auth:\n  registration:\n    allowed_email_domains: [example.com]\n    enabled: false\n")
	assert.Contains(t, body, "module_runtime:\n  cluster: prod\n")
	assert.NotContains(t, body, "require_invite")
	assert.NotContains(t, body, "login_rate_limit")
	loaded, _, err := loadBytes(result.Output)
	require.NoError(t, err)
	require.NotNil(t, loaded.Auth.Registration.Enabled)
	assert.False(t, *loaded.Auth.Registration.Enabled)
	assert.Equal(t, []string{"example.com"}, loaded.Auth.Registration.AllowedEmailDomains)
	assert.Equal(t, "prod", loaded.ModuleRuntime.ClusterOrDefault())
	assert.Contains(t, result.Report(), "auth.registration.enabled: still read until M3 (N35), and not the built-in value")

	result, err = Rewrite([]byte("auth:\n  registration:\n    enabled: true\nmodule_runtime:\n  cluster: default\n"), RewriteOptions{Now: rewriteNow})
	require.NoError(t, err)
	assert.Contains(t, string(result.Output), "Every key has its default.")
}
