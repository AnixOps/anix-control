package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentControlMTLSValidation(t *testing.T) {
	valid := func() *Config {
		return &Config{
			GRPC:          GRPCConfig{Enable: true, TLSCertFile: "control.crt", TLSKeyFile: "control.key"},
			ModuleRuntime: ModuleRuntimeConfig{Enabled: true, CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="},
		}
	}
	require.Equal(t, AgentMTLSOptional, AgentControlConfig{}.MTLSOrDefault())
	require.Equal(t, AgentMTLSRequired, AgentControlConfig{MTLS: " Required "}.MTLSOrDefault())
	require.Equal(t, AgentMTLSOptional, Defaults().AgentControl.MTLSOrDefault())

	for _, mode := range []string{"", AgentMTLSOptional, AgentMTLSPreferred, AgentMTLSRequired} {
		cfg := valid()
		cfg.AgentControl.MTLS = mode
		require.NoError(t, cfg.ValidateForServer(), mode)
	}
	cfg := valid()
	cfg.AgentControl.MTLS = "strict"
	require.ErrorContains(t, cfg.ValidateForServer(), "agent_control.mtls must be")

	// Optional needs nothing; the other modes need TLS and the module CA.
	optional := &Config{GRPC: GRPCConfig{Enable: true}}
	require.NoError(t, optional.ValidateForServer())
	for _, mode := range []string{AgentMTLSPreferred, AgentMTLSRequired} {
		cfg := valid()
		cfg.AgentControl.MTLS = mode
		cfg.GRPC.TLSKeyFile = ""
		require.ErrorContains(t, cfg.ValidateForServer(), "grpc.tls_cert_file", mode)
		cfg = valid()
		cfg.AgentControl.MTLS = mode
		cfg.ModuleRuntime = ModuleRuntimeConfig{}
		require.ErrorContains(t, cfg.ValidateForServer(), "built-in module PKI", mode)
		cfg = valid()
		cfg.AgentControl.MTLS = mode
		cfg.ModuleRuntime.PKI = ModulePKIExternal
		cfg.ModuleRuntime.TrustBundleFile, cfg.ModuleRuntime.CertFile, cfg.ModuleRuntime.KeyFile = "a", "b", "c"
		require.ErrorContains(t, cfg.ValidateForServer(), "built-in module PKI", mode)
		// Without the gRPC listener the mode has nothing to apply to.
		cfg = &Config{AgentControl: AgentControlConfig{MTLS: mode}}
		require.NoError(t, cfg.ValidateForServer(), mode)
	}
}
