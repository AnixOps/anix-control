package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBuildNodeProtocolConfig_Snell(t *testing.T) {
	node := &model.Node{ID: 1, Host: "snell.example.com", CreatedAt: time.Now()}
	settings := `{"psk":"shared-psk"}`
	config := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 3, NodeID: 1, Type: model.ProtocolSnell, Port: 6160, Settings: &settings})

	require.Equal(t, "snell", config["type"])
	require.Equal(t, 6160, config["server_port"])
	require.Equal(t, "shared-psk", config["psk"])
	require.Equal(t, 4, config["version"])
	// obfs and mode stay out until set.
	require.NotContains(t, config, "obfs")
	require.NotContains(t, config, "mode")

	settings = `{"psk":"shared-psk","version":6,"mode":"unshaped"}`
	v6 := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 4, NodeID: 1, Type: model.ProtocolSnell, Port: 6161, Settings: &settings})
	require.Equal(t, 6, v6["version"])
	require.Equal(t, "unshaped", v6["mode"])
}

func TestValidateNodeProtocol_Snell(t *testing.T) {
	str := func(v string) *string { return &v }
	protocol := func(settings string) *model.NodeProtocol {
		return &model.NodeProtocol{Type: model.ProtocolSnell, Port: 6160, Transport: str("tcp"), Settings: &settings}
	}
	for _, settings := range []string{
		`{"psk":"shared-psk"}`, `{"psk":"shared-psk","version":1}`, `{"psk":"shared-psk","version":4,"obfs":"http"}`,
		`{"psk":"shared-psk","version":5}`, `{"psk":"shared-psk","version":6}`, `{"psk":"shared-psk","version":6,"mode":"unsafe-raw"}`,
	} {
		require.NoError(t, ValidateNodeProtocol(protocol(settings)), settings)
	}
	for _, settings := range []string{
		``, `{}`, `[]`, `{"psk":"has space"}`, `{"psk":"a,b"}`, `{"psk":"shared-psk","version":0}`, `{"psk":"shared-psk","version":7}`,
		`{"psk":"shared-psk","obfs":"tls"}`, `{"psk":"shared-psk","version":6,"obfs":"http"}`,
		`{"psk":"shared-psk","mode":"default"}`, `{"psk":"shared-psk","version":6,"mode":"fast"}`,
	} {
		require.ErrorIs(t, ValidateNodeProtocol(protocol(settings)), ErrInvalidNodeProtocol, settings)
	}

	// Snell brings its own encryption and runs on TCP only.
	withTLS := protocol(`{"psk":"shared-psk"}`)
	withTLS.TLS = 1
	require.ErrorIs(t, ValidateNodeProtocol(withTLS), ErrInvalidNodeProtocol)
	overWS := protocol(`{"psk":"shared-psk"}`)
	overWS.Transport = str("ws")
	require.ErrorIs(t, ValidateNodeProtocol(overWS), ErrInvalidNodeProtocol)
}

func TestNodeProtocolToParsedNode_Snell(t *testing.T) {
	settings := `{"psk":"shared-psk","version":4}`
	parsed := (&SubscriptionService{}).nodeProtocolToParsedNode(
		&model.Node{ID: 1, Name: "node", Host: "snell.example.com"},
		&model.NodeProtocol{ID: 3, NodeID: 1, Name: "snell", Type: model.ProtocolSnell, Port: 6160, Settings: &settings},
		&model.TemplateRenderContext{UUID: "user-uuid"})

	// The subscriber's key is their UUID; the psk is the node's.
	require.Equal(t, "user-uuid", parsed.Password)
	require.Equal(t, "shared-psk", parsed.Settings["psk"])
	require.False(t, parsed.TLS)
}
