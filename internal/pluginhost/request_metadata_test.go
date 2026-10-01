//go:build unix

package pluginhost

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

// Package hosts built with the v4.0.0 SDK decode request metadata with
// DisallowUnknownFields, so the original request address is never a key of
// the metadata JSON: the kernel's bridge snapshot carries it, and hosts get
// it as DispatchRequest fields of their own (request_scheme, request_host).
func TestRequestAddressReachesOnlyTheBridgeSnapshot(t *testing.T) {
	metadata := RequestMetadata{Path: "/api/v2/forward-agent/install.sh", ClientIP: "198.51.100.7", Host: "panel.example.test:8443", TLS: true}

	hostJSON, err := marshalRequestMetadata(metadata)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"/api/v2/forward-agent/install.sh","client_ip":"198.51.100.7"}`, string(hostJSON))
	decoder := json.NewDecoder(bytes.NewReader(hostJSON))
	decoder.DisallowUnknownFields()
	var sdkMetadata pluginhostsdk.RequestMetadata
	require.NoError(t, decoder.Decode(&sdkMetadata))

	bridgeJSON, err := marshalBridgeRequestMetadata(metadata)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"/api/v2/forward-agent/install.sh","client_ip":"198.51.100.7","host":"panel.example.test:8443","tls":true}`, string(bridgeJSON))
}
