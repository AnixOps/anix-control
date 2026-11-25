package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grpc.relay_sources (docs/design/region-relay): region relays forward
// nodes' connections byte for byte behind a PROXY v2 header.
func TestRelaySourcesLayout(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: panel.example.com\ngrpc:\n  relay_sources: [203.0.113.10, 203.0.113.11]\n"), nil)
	require.NoError(t, err)
	panel := loaded.Panel()
	assert.Equal(t, []string{"203.0.113.10", "203.0.113.11"}, panel.GRPCRelaySources)
	assert.False(t, panel.GRPCEdge, "relays terminate nothing: the listener stays TLS")
	assert.NotEqual(t, GRPCCertEdge, panel.GRPCCertSource)
}

func TestRelaySourcesFromEnvironment(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: panel.example.com\n"), []string{"ANIX_CONTROL_GRPC_RELAY_SOURCES=203.0.113.10,198.51.100.0/24"})
	require.NoError(t, err)
	assert.Equal(t, []string{"203.0.113.10", "198.51.100.0/24"}, loaded.GRPC.RelaySources)
	assert.Equal(t, []string{"203.0.113.10", "198.51.100.0/24"}, loaded.Panel().GRPCRelaySources)
}

func TestRelaySourcesAreOptIn(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: panel.example.com\n"), nil)
	require.NoError(t, err)
	assert.Empty(t, loaded.Panel().GRPCRelaySources)
}

func TestRelaySourcesValidation(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"not an address": {
			text: "domain: panel.example.com\ngrpc:\n  relay_sources: [relay.example.com]\n",
			want: "grpc.relay_sources",
		},
		"too wide v4": {
			text: "domain: panel.example.com\ngrpc:\n  relay_sources: [0.0.0.0/0]\n",
			want: "wider than /24",
		},
		"too wide v6": {
			text: "domain: panel.example.com\ngrpc:\n  relay_sources: [\"2001:db8::/32\"]\n",
			want: "wider than /64",
		},
		// net.ParseCIDR reads ::ffff:0.0.0.0/96 as all of IPv4 (re-audit N2).
		"ipv4-mapped range": {
			text: "domain: panel.example.com\ngrpc:\n  relay_sources: [\"::ffff:0.0.0.0/96\"]\n",
			want: "IPv4-mapped",
		},
		"ipv4-mapped narrow range": {
			text: "domain: panel.example.com\ngrpc:\n  relay_sources: [\"::ffff:203.0.113.10/128\"]\n",
			want: "IPv4-mapped",
		},
		"excludes edge proxies": {
			text: "domain: panel.example.com\ntls:\n  mode: external\ngrpc:\n  edge_proxies: [10.0.0.2]\n  relay_sources: [203.0.113.10]\n",
			want: "grpc.relay_sources excludes grpc.edge_proxies",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := LoadFile(writeConfig(t, tc.text), nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
