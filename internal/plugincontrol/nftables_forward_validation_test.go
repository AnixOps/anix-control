package plugincontrol

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNftablesForwardConfigurationAdmission(t *testing.T) {
	executor := NewNftablesForwardExecutor(nil)
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{name: "safe observation defaults", config: `{"apply":false,"rollback_on_exit":true,"family":"inet","table":"anixops_forward","chain":"prerouting","priority":-100,"rules":[]}`},
		{name: "valid IPv4 apply", config: `{"apply":true,"rollback_on_exit":true,"family":"ip","rules":[{"id":"tcp-443","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":8443}]}`},
		{name: "apply requires rules", config: `{"apply":true,"rollback_on_exit":true,"rules":[]}`, wantErr: "at least one forwarding rule"},
		{name: "rollback is mandatory", config: `{"apply":false,"rollback_on_exit":false,"rules":[]}`, wantErr: "rollback_on_exit"},
		{name: "duplicate rule", config: `{"apply":true,"rules":[{"id":"same","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":443},{"id":"same","protocol":"udp","listen_address":"198.51.100.11","listen_port":443,"target_address":"203.0.113.11","target_port":443}]}`, wantErr: "duplicated"},
		{name: "unspecified listener", config: `{"apply":true,"rules":[{"id":"wide","protocol":"tcp","listen_address":"0.0.0.0","listen_port":443,"target_address":"203.0.113.10","target_port":443}]}`, wantErr: "unicast"},
		{name: "mixed family", config: `{"apply":true,"rules":[{"id":"mixed","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"2001:db8::10","target_port":443}]}`, wantErr: "same IP family"},
		{name: "table family mismatch", config: `{"apply":true,"family":"ip6","rules":[{"id":"v4","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":443}]}`, wantErr: "does not match ip6"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := executor.ValidateConfiguration(context.Background(), json.RawMessage(test.config))
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestNftablesForwardLegacyExecutorRemainsVersionBound(t *testing.T) {
	legacy := NewNftablesForwardExecutorVersion(nil, NftablesForwardLegacyVersion)
	require.Equal(t, NftablesForwardLegacyVersion, legacy.Version())
	require.NoError(t, legacy.ValidateConfiguration(context.Background(), json.RawMessage(`{"chain_priority":0,"rollback_snapshot":true}`)))

	registry, err := NewRegistry(NewNftablesForwardExecutor(nil), legacy)
	require.NoError(t, err)
	_, currentOK := registry.Lookup(NftablesForwardPluginID, NftablesForwardVersion)
	_, legacyOK := registry.Lookup(NftablesForwardPluginID, NftablesForwardLegacyVersion)
	require.True(t, currentOK)
	require.True(t, legacyOK)
}
