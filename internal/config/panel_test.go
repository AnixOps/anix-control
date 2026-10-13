package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func loadPanelYAML(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	return load(path, nil)
}

func TestPanelSettingsResolveTheModes(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, p PanelSettings)
	}{
		{
			name: "domain alone is acme with the same certificate on gRPC",
			yaml: "domain: Panel.Example.com.\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, TLSModeACME, p.Mode)
				require.False(t, p.ModeExplicit)
				require.Equal(t, "panel.example.com", p.Domain)
				require.Equal(t, ":443", p.HTTPSAddress)
				require.Equal(t, ":80", p.HTTPAddress)
				require.Empty(t, p.PlainAddresses)
				require.True(t, p.GRPCEnabled, "domain turns the gRPC listener on")
				require.Equal(t, "0.0.0.0:50051", p.GRPCAddress)
				require.Equal(t, "panel.example.com", p.GRPCName)
				require.Equal(t, GRPCCertPanel, p.GRPCCertSource)
				require.Equal(t, []string{"panel.example.com"}, p.ACMENames)
				require.Equal(t, "https://panel.example.com", p.PublicURL)
				require.Equal(t, "panel.example.com:50051", p.GRPCTarget)
				require.True(t, p.ServesWebTLS())
				require.True(t, p.GRPCTLS())
			},
		},
		{
			name: "the CDN mode: external web, a DNS-only gRPC name over ACME",
			yaml: "domain: panel.example.com\ntls:\n  mode: external\nlisten: 127.0.0.1:9000\ngrpc:\n  public_name: grpc.example.com\n  port: 50443\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, TLSModeExternal, p.Mode)
				require.True(t, p.ModeExplicit)
				require.Equal(t, []string{"127.0.0.1:9000"}, p.PlainAddresses)
				require.Empty(t, p.HTTPSAddress)
				require.Equal(t, "grpc.example.com", p.GRPCName)
				require.Equal(t, GRPCCertACME, p.GRPCCertSource)
				require.Equal(t, []string{"grpc.example.com"}, p.ACMENames)
				require.Equal(t, "https://panel.example.com", p.PublicURL)
				require.Equal(t, "grpc.example.com:50443", p.GRPCTarget)
				require.False(t, p.ServesWebTLS())
				require.Equal(t, ":80", p.ChallengeHTTPAddress, "HTTP-01 for the DNS-only name when port 80 is free")
				require.Equal(t, ":443", p.ChallengeTLSAddress)
			},
		},
		{
			name: "a plain listener on port 80 answers HTTP-01 itself",
			yaml: "domain: panel.example.com\ntls:\n  mode: external\nlisten: 0.0.0.0:80\ngrpc:\n  public_name: grpc.example.com\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Empty(t, p.ChallengeHTTPAddress)
				require.Equal(t, ":443", p.ChallengeTLSAddress)
			},
		},
		{
			name: "acme with another gRPC name holds two certificates",
			yaml: "domain: panel.example.com\ngrpc:\n  public_name: grpc.example.com\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, GRPCCertACME, p.GRPCCertSource)
				require.Equal(t, []string{"panel.example.com", "grpc.example.com"}, p.ACMENames)
			},
		},
		{
			name: "files mode shares its certificate with gRPC",
			yaml: "domain: panel.example.com\ntls:\n  mode: files\n  cert_file: /etc/tls/panel.crt\n  key_file: /etc/tls/panel.key\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, TLSModeFiles, p.Mode)
				require.Equal(t, "/etc/tls/panel.crt", p.WebCertFile)
				require.Equal(t, GRPCCertPanel, p.GRPCCertSource)
				require.False(t, p.NeedsACME())
			},
		},
		{
			name: "external mode serves tls.cert_file on gRPC instead of ACME",
			yaml: "domain: panel.example.com\ntls:\n  mode: external\n  cert_file: /etc/tls/grpc.crt\n  key_file: /etc/tls/grpc.key\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, GRPCCertFiles, p.GRPCCertSource)
				require.Equal(t, "/etc/tls/grpc.crt", p.GRPCCertFile)
				require.False(t, p.NeedsACME())
				require.Equal(t, []string{"127.0.0.1:8080"}, p.PlainAddresses, "the built-in default of server.host:server.port")
			},
		},
		{
			name: "grpc.tls_cert_file wins in every mode",
			yaml: "domain: panel.example.com\ngrpc:\n  tls_cert_file: /g.crt\n  tls_key_file: /g.key\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, GRPCCertFiles, p.GRPCCertSource)
				require.Equal(t, "/g.crt", p.GRPCCertFile)
				require.Equal(t, []string{"panel.example.com"}, p.ACMENames)
			},
		},
		{
			name: "legacy layout: no domain, both old ports on one handler, plaintext gRPC",
			yaml: "server:\n  host: 0.0.0.0\n  port: 8080\nfrontend:\n  enable: true\n  port: 3000\ngrpc:\n  enabled: true\n  host: 127.0.0.1\n  port: 50052\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, TLSModeExternal, p.Mode)
				require.False(t, p.ModeExplicit)
				require.Equal(t, []string{"0.0.0.0:8080", "0.0.0.0:3000"}, p.PlainAddresses)
				require.True(t, p.GRPCEnabled)
				require.Equal(t, "127.0.0.1:50052", p.GRPCAddress)
				require.Equal(t, GRPCCertNone, p.GRPCCertSource)
				require.False(t, p.GRPCTLS())
				require.Empty(t, p.PublicURL)
				require.Empty(t, p.GRPCTarget)
			},
		},
		{
			name: "tls.domain is not read: an existing config stays external",
			yaml: "tls:\n  domain: panel.example.com\n  enable: false\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, TLSModeExternal, p.Mode)
				require.Empty(t, p.Domain)
				require.False(t, p.GRPCEnabled)
			},
		},
		{
			name: "agent_install overrides the derived addresses",
			yaml: "domain: panel.example.com\nagent_install:\n  public_url: https://nodes.example.com/\n  grpc_target: grpc.example.net:443\n",
			check: func(t *testing.T, p PanelSettings) {
				require.Equal(t, "https://nodes.example.com", p.PublicURL)
				require.Equal(t, "grpc.example.net:443", p.GRPCTarget)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadPanelYAML(t, tc.yaml)
			require.NoError(t, err)
			tc.check(t, cfg.Panel())
		})
	}
}

func TestPanelSettingsAreValidated(t *testing.T) {
	tests := []struct {
		yaml string
		want string
	}{
		{"tls:\n  mode: auto\n", "invalid tls.mode"},
		{"domain: https://panel.example.com\n", "invalid domain"},
		{"domain: 203.0.113.5\n", "invalid domain"},
		{"domain: panel\n", "invalid domain"},
		{"domain: panel.example.com:443\n", "invalid domain"},
		{"grpc:\n  public_name: grpc_node.example.com\n", "invalid grpc.public_name"},
		{"tls:\n  mode: acme\n", "tls.mode acme needs domain"},
		{"tls:\n  mode: files\n", "tls.mode files needs domain"},
		{"domain: panel.example.com\ntls:\n  mode: files\n", "tls.mode files needs tls.cert_file"},
		{"tls:\n  cert_file: /a.crt\n", "tls.cert_file and tls.key_file go together"},
		{"grpc:\n  tls_key_file: /a.key\n", "grpc.tls_cert_file and grpc.tls_key_file go together"},
		{"listen: 8080\n", "invalid listen"},
		{"listen: panel.example.com:8080\n", "invalid listen"},
		{"tls:\n  enable: true\n  cert_file: /a.crt\n  key_file: /a.key\n", "tls.enable is no longer read"},
	}
	for _, tc := range tests {
		_, err := loadPanelYAML(t, tc.yaml)
		require.Error(t, err, tc.yaml)
		require.Contains(t, err.Error(), tc.want, tc.yaml)
	}
}

func TestTLSEnableWithAModeOrWithoutFilesIsOnlyIgnored(t *testing.T) {
	cfg, err := loadPanelYAML(t, "domain: panel.example.com\ntls:\n  mode: files\n  enable: true\n  cert_file: /a.crt\n  key_file: /a.key\n")
	require.NoError(t, err)
	require.Equal(t, TLSModeFiles, cfg.Panel().Mode)
	cfg, err = loadPanelYAML(t, "tls:\n  enable: true\n")
	require.NoError(t, err)
	require.Equal(t, TLSModeExternal, cfg.Panel().Mode)
}

func TestPanelACMENeedsTheRootKey(t *testing.T) {
	cfg, err := loadPanelYAML(t, "domain: panel.example.com\n")
	require.NoError(t, err)
	err = cfg.ValidateForServer()
	require.Error(t, err)
	require.Contains(t, err.Error(), "module_runtime.ca_kek")

	cfg.ModuleRuntime.CAKEK = strings.Repeat("A", 43) + "="
	require.NoError(t, cfg.ValidateForServer())

	// Files and external without a gRPC ACME name need no key.
	cfg, err = loadPanelYAML(t, "domain: panel.example.com\ntls:\n  mode: files\n  cert_file: /a\n  key_file: /b\n")
	require.NoError(t, err)
	require.NoError(t, cfg.ValidateForServer())
}

func TestRequiredPrerequisitesFollowThePanelCertificate(t *testing.T) {
	cfg, err := loadPanelYAML(t, "domain: panel.example.com\nmodule_runtime:\n  ca_kek: "+strings.Repeat("A", 43)+"=\n")
	require.NoError(t, err)
	// domain turns gRPC on and gives it the panel's certificate.
	require.NoError(t, cfg.RequiredPrerequisitesError())

	cfg, err = loadPanelYAML(t, "grpc:\n  enabled: true\nmodule_runtime:\n  ca_kek: "+strings.Repeat("A", 43)+"=\n")
	require.NoError(t, err)
	err = cfg.RequiredPrerequisitesError()
	require.Error(t, err)
	require.Contains(t, err.Error(), "needs TLS on the gRPC listener")
}
