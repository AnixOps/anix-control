package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestANXUserKey(t *testing.T) {
	key := ANXUserKey("C9A1E7B0-5B0E-4A4B-9A54-2F3F1C1B0D11")
	require.Len(t, key, 64)
	require.Equal(t, key, ANXUserKey(" c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11 "))
	require.NotEqual(t, key, ANXUserKey("c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d12"))
	require.Empty(t, ANXUserKey(" "))
}

func TestBuildNodeProtocolConfig_ANX(t *testing.T) {
	node := &model.Node{ID: 1, Host: "anx.example.com", CreatedAt: time.Now()}
	settings := `{"server_names":["a.example.com","b.example.com"],"dest":"a.example.com:443"}`
	protocol := &model.NodeProtocol{ID: 3, NodeID: 1, Type: model.ProtocolANX, Port: 8443, Settings: &settings}

	config := BuildNodeProtocolConfig(node, protocol)

	require.Equal(t, "anx", config["type"])
	require.Equal(t, 8443, config["server_port"])
	require.Equal(t, []string{"a.example.com", "b.example.com"}, config["server_names"])
	require.Equal(t, "a.example.com:443", config["dest"])
	require.Equal(t, []string{"h2", "http/1.1"}, config["alpn"])
	require.Equal(t, `["a.example.com","b.example.com"]`, StringifyConfigMap(config)["server_names"])

	empty := BuildNodeProtocolConfig(node, &model.NodeProtocol{ID: 4, NodeID: 1, Type: model.ProtocolANX, Port: 8444})
	require.Equal(t, []string{"www.apple.com"}, empty["server_names"])
	require.Equal(t, "www.apple.com:443", empty["dest"])
}

func TestApplyANXUser(t *testing.T) {
	parsed := &model.ParsedNode{Type: "anx", Settings: map[string]any{"server_names": []any{"x.example.com"}, "alpn": []any{"h2"}}}
	applyANXUser(parsed, &model.TemplateRenderContext{UUID: "c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"})
	require.Equal(t, ANXUserKey("c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"), parsed.Password)
	require.Equal(t, "x.example.com", parsed.ServerName)
	require.Equal(t, "h2", parsed.ALPN)
}

func TestValidateANXProtocol(t *testing.T) {
	valid := []string{``, `{}`, `{"server_names":["www.apple.com"],"dest":"www.apple.com:443","alpn":["h2"]}`}
	for _, settings := range valid {
		s := settings
		require.NoError(t, validateANXProtocol(&model.NodeProtocol{Type: model.ProtocolANX, Settings: &s}), settings)
	}
	invalid := []string{`[]`, `{"server_names":[]}`, `{"server_names":["a b"]}`, `{"server_names":"x"}`,
		`{"dest":"www.apple.com"}`, `{"dest":"www.apple.com:0"}`, `{"dest":5}`, `{"alpn":[1]}`}
	for _, settings := range invalid {
		s := settings
		require.Error(t, validateANXProtocol(&model.NodeProtocol{Type: model.ProtocolANX, Settings: &s}), settings)
	}
}

// The same vector as anix-agent core/anixcore TestAnxUserKeyMatchesControl.
func TestANXUserKeyVector(t *testing.T) {
	require.Equal(t, "a05385f0ca7b3e2223b65d0af4dbc51cbd50a1330d27f509ed9de142b771f20a", ANXUserKey("C9A1E7B0-5B0E-4A4B-9A54-2F3F1C1B0D11"))
}
