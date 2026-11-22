//go:build unix

package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The kernel's bridge snapshot carries the original request address beside
// the request metadata.
func TestBridgeSnapshotCarriesTheRequestAddress(t *testing.T) {
	metadata := RequestMetadata{Path: "/api/v2/forward-agent/install.sh", ClientIP: "198.51.100.7", Host: "panel.example.test:8443", TLS: true}

	bridgeJSON, err := marshalBridgeRequestMetadata(metadata)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"/api/v2/forward-agent/install.sh","client_ip":"198.51.100.7","host":"panel.example.test:8443","tls":true}`, string(bridgeJSON))
}
