package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grpc.edge_proxies (docs/design/edge-grpc): a reverse proxy on another
// machine terminates the nodes' TLS and Control serves plain gRPC to it.
func TestEdgeProxiesLayout(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: new-x.anixops.com\ntls:\n  mode: external\ngrpc:\n  edge_proxies: [10.0.0.2]\n"), nil)
	require.NoError(t, err)
	panel := loaded.Panel()
	assert.True(t, panel.GRPCEdge)
	assert.True(t, panel.GRPCEnabled)
	assert.Equal(t, GRPCCertEdge, panel.GRPCCertSource, "the proxy holds the gRPC certificate")
	assert.True(t, panel.GRPCTLS(), "nodes use TLS, to the proxy")
	assert.False(t, panel.NeedsACME(), "Control obtains no certificate on the edge")
	assert.Equal(t, []string{"10.0.0.2"}, panel.GRPCEdgeProxies)
}

func TestEdgeProxiesFromEnvironment(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: new-x.anixops.com\ntls:\n  mode: external\n"), []string{"ANIX_CONTROL_GRPC_EDGE_PROXIES=10.0.0.2,10.0.0.0/24"})
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.2", "10.0.0.0/24"}, loaded.GRPC.EdgeProxies)
	assert.True(t, loaded.Panel().GRPCEdge)
}

func TestEdgeProxiesAreOptIn(t *testing.T) {
	loaded, _, err := LoadFile(writeConfig(t, "domain: panel.example.com\n"), nil)
	require.NoError(t, err)
	panel := loaded.Panel()
	assert.False(t, panel.GRPCEdge)
	assert.NotEqual(t, GRPCCertEdge, panel.GRPCCertSource)
}

func TestEdgeProxiesValidation(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"not an address": {
			text: "domain: new-x.anixops.com\ntls:\n  mode: external\ngrpc:\n  edge_proxies: [not-an-ip]\n",
			want: "grpc.edge_proxies",
		},
		"needs tls.mode external": {
			text: "domain: new-x.anixops.com\ngrpc:\n  edge_proxies: [10.0.0.2]\n",
			want: "needs tls.mode external",
		},
		"excludes grpc certificate files": {
			text: "domain: new-x.anixops.com\ntls:\n  mode: external\ngrpc:\n  edge_proxies: [10.0.0.2]\n  tls_cert_file: /etc/a.pem\n  tls_key_file: /etc/a.key\n",
			want: "excludes grpc.tls_cert_file",
		},
		"excludes tls.cert_file": {
			text: "domain: new-x.anixops.com\ntls:\n  mode: external\n  cert_file: /etc/a.pem\n  key_file: /etc/a.key\ngrpc:\n  edge_proxies: [10.0.0.2]\n",
			want: "excludes grpc.tls_cert_file and tls.cert_file",
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
