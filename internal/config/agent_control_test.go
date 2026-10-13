package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentControlMTLSValidation(t *testing.T) {
	valid := func() *Config {
		return &Config{
			GRPC:          GRPCConfig{Enable: true, TLSCertFile: "control.crt", TLSKeyFile: "control.key"},
			ModuleRuntime: ModuleRuntimeConfig{Enabled: true, CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="},
		}
	}
	require.Equal(t, AgentMTLSRequired, AgentControlConfig{}.MTLSOrDefault(), "4.2 defaults to required (H5)")
	require.Equal(t, AgentMTLSPreferred, AgentControlConfig{MTLS: "preferred"}.MTLSOrDefault(), "preferred stays selectable")
	require.False(t, AgentControlConfig{MTLS: "  "}.MTLSExplicit())
	require.True(t, AgentControlConfig{MTLS: "required"}.MTLSExplicit())
	require.Equal(t, AgentMTLSRequired, AgentControlConfig{MTLS: " Required "}.MTLSOrDefault())
	require.Equal(t, AgentMTLSOff, AgentControlConfig{MTLS: "OFF"}.MTLSOrDefault())
	require.Equal(t, AgentMTLSRequired, Defaults().AgentControl.MTLSOrDefault(), "the container defaults leave the mode to the code")
	require.False(t, Defaults().AgentControl.MTLSExplicit())
	require.False(t, Defaults().ModuleRuntime.BuiltinCA(), "no CA without a key")
	require.True(t, ModuleRuntimeConfig{CAKEK: "key"}.BuiltinCA())
	require.True(t, ModuleRuntimeConfig{Enabled: true}.BuiltinCA())
	require.False(t, ModuleRuntimeConfig{PKI: ModulePKIExternal, CAKEK: "key"}.BuiltinCA())

	for _, mode := range append([]string{""}, AgentMTLSModes...) {
		cfg := valid()
		cfg.AgentControl.MTLS = mode
		require.NoError(t, cfg.ValidateForServer(), mode)
	}
	cfg := valid()
	cfg.AgentControl.MTLS = "strict"
	require.ErrorContains(t, cfg.ValidateForServer(), "agent_control.mtls must be")

	// off, optional, preferred and the default required need nothing: a
	// kernel without gRPC TLS or the CA keeps starting (the default required
	// then refuses legacy agents and warns that none can enroll).
	for _, mode := range []string{"", AgentMTLSOff, AgentMTLSOptional, AgentMTLSPreferred} {
		bare := &Config{GRPC: GRPCConfig{Enable: true}, AgentControl: AgentControlConfig{MTLS: mode}}
		require.NoError(t, bare.ValidateForServer(), mode)
		bare = &Config{AgentControl: AgentControlConfig{MTLS: mode}}
		require.NoError(t, bare.ValidateForServer(), mode)
	}
	bare := &Config{}
	require.Equal(t, AgentMTLSRequired, bare.AgentControl.MTLSOrDefault())
	require.ErrorContains(t, bare.RequiredPrerequisitesError(), "grpc.enabled", "the startup warning names what is missing")
	require.NoError(t, valid().RequiredPrerequisitesError())
	// An explicit required needs the gRPC listener, its TLS and the built-in
	// CA, or refuses to start.
	mode := AgentMTLSRequired
	cfg = valid()
	cfg.AgentControl.MTLS = mode
	cfg.GRPC.TLSKeyFile = ""
	require.ErrorContains(t, cfg.ValidateForServer(), "grpc.tls_cert_file")
	cfg = valid()
	cfg.AgentControl.MTLS = mode
	cfg.ModuleRuntime = ModuleRuntimeConfig{}
	require.ErrorContains(t, cfg.ValidateForServer(), "root key")
	cfg = valid()
	cfg.AgentControl.MTLS = mode
	cfg.ModuleRuntime.PKI = ModulePKIExternal
	cfg.ModuleRuntime.TrustBundleFile, cfg.ModuleRuntime.CertFile, cfg.ModuleRuntime.KeyFile = "a", "b", "c"
	require.ErrorContains(t, cfg.ValidateForServer(), `module_runtime.pki "external"`)
	// The CA alone is enough: the module runtime and its listener stay off.
	cfg = valid()
	cfg.AgentControl.MTLS = mode
	cfg.ModuleRuntime.Enabled = false
	require.NoError(t, cfg.ValidateForServer())
	// Without the gRPC listener an enrolled agent could not connect at all.
	cfg = &Config{AgentControl: AgentControlConfig{MTLS: mode}}
	require.ErrorContains(t, cfg.ValidateForServer(), "grpc.enabled")
}

func TestAgentControlLegacySunset(t *testing.T) {
	sunset, err := AgentControlConfig{}.LegacySunsetTime()
	require.NoError(t, err)
	require.True(t, sunset.IsZero())
	sunset, err = AgentControlConfig{LegacySunset: "2027-03-31"}.LegacySunsetTime()
	require.NoError(t, err)
	require.Equal(t, time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC), sunset)
	sunset, err = AgentControlConfig{LegacySunset: "2027-03-31T12:00:00+02:00"}.LegacySunsetTime()
	require.NoError(t, err)
	require.Equal(t, time.Date(2027, 3, 31, 10, 0, 0, 0, time.UTC), sunset)

	cfg := &Config{AgentControl: AgentControlConfig{LegacySunset: "next spring"}}
	require.ErrorContains(t, cfg.ValidateForServer(), "agent_control.legacy_sunset")

	// Both keys are built in now: a variable that sets one is ignored.
	loaded, report, err := LoadFile("", []string{EnvPrefix + "AGENT_CONTROL_MTLS=preferred", EnvPrefix + "AGENT_CONTROL_LEGACY_SUNSET=2027-03-31"})
	require.NoError(t, err)
	require.Len(t, report.Deprecated, 2)
	require.Equal(t, AgentMTLSRequired, loaded.AgentControl.MTLSOrDefault())
	require.False(t, loaded.AgentControl.MTLSExplicit())
	require.Empty(t, loaded.AgentControl.LegacySunset)
}

// TestShippedConfigLeavesTheAgentModeToTheDefault: the template follows the
// default (required) and starts without gRPC TLS or the CA (a warning, not
// a refusal).
func TestShippedConfigLeavesTheAgentModeToTheDefault(t *testing.T) {
	loaded, _, err := LoadFile(filepath.Join("..", "..", "config", "config.yaml.example"), nil)
	require.NoError(t, err)
	require.False(t, loaded.AgentControl.MTLSExplicit())
	require.Equal(t, AgentMTLSRequired, loaded.AgentControl.MTLSOrDefault())
	require.NoError(t, loaded.validateAgentControl())
}
