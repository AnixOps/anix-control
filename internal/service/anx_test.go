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

	config := buildNodeProtocolConfig(node, protocol)

	require.Equal(t, "anx", config["type"])
	require.Equal(t, 8443, config["server_port"])
	require.Equal(t, []string{"a.example.com", "b.example.com"}, config["server_names"])
	require.Equal(t, "a.example.com:443", config["dest"])
	require.Equal(t, []string{"h2", "http/1.1"}, config["alpn"])

	empty := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 4, NodeID: 1, Type: model.ProtocolANX, Port: 8444})
	require.Equal(t, []string{"www.apple.com"}, empty["server_names"])
	require.Equal(t, "www.apple.com:443", empty["dest"])
	// The other A2 options stay out until set, for anx-nodes that do not know them.
	require.NotContains(t, empty, "allow_no_sni")
	require.NotContains(t, empty, "shape_plain")
	require.NotContains(t, empty, "max_streams")
	require.NotContains(t, empty, "wg_listen_port")
	require.NotContains(t, empty, "bare_listen_port")
	require.NotContains(t, empty, "dest_stack")
	require.NotContains(t, empty, "dest_behavior")
	require.NotContains(t, empty, "min_wire_gen")
	require.NotContains(t, empty, "wire_gen")

	a2 := `{"allow_no_sni":true,"shape_plain":true,"max_streams":512,"wg_listen_port":51820}`
	full := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 5, NodeID: 1, Type: model.ProtocolANX, Port: 8445, Settings: &a2})
	require.Equal(t, true, full["allow_no_sni"])
	require.Equal(t, true, full["shape_plain"])
	require.Equal(t, 512, full["max_streams"])
	require.Equal(t, 51820, full["wg_listen_port"])

	bare := `{"allow_no_sni":true,"bare_listen_port":8444}`
	withBare := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 8, NodeID: 1, Type: model.ProtocolANX, Port: 8448, Settings: &bare})
	require.Equal(t, 8444, withBare["bare_listen_port"])

	site := `{"dest_stack":"boringssl","dest_behavior":{"session_tickets":"266/266","flight_records":"none","max_ccs":32}}`
	withSite := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 9, NodeID: 1, Type: model.ProtocolANX, Port: 8449, Settings: &site})
	require.Equal(t, "boringssl", withSite["dest_stack"])
	require.Equal(t, map[string]any{"session_tickets": "266/266", "flight_records": "none", "max_ccs": float64(32)}, withSite["dest_behavior"])

	// wire_gen only changes the key text clients get; min_wire_gen is the
	// node's own setting and goes out when it is 2.
	gen2 := `{"wire_gen":2}`
	published := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 6, NodeID: 1, Type: model.ProtocolANX, Port: 8446, Settings: &gen2})
	require.NotContains(t, published, "min_wire_gen")
	require.NotContains(t, published, "wire_gen")
	strict := `{"wire_gen":2,"min_wire_gen":2}`
	enforced := buildNodeProtocolConfig(node, &model.NodeProtocol{ID: 7, NodeID: 1, Type: model.ProtocolANX, Port: 8447, Settings: &strict})
	require.Equal(t, 2, enforced["min_wire_gen"])
}

func TestANXClientPublicKey(t *testing.T) {
	key := "aabb:ccdd"
	require.Equal(t, key, ANXClientPublicKey(key, nil))
	require.Equal(t, key, ANXClientPublicKey(key, map[string]any{"wire_gen": float64(1)}))
	require.Equal(t, key+"@2", ANXClientPublicKey(" "+key+" ", map[string]any{"wire_gen": float64(2)}))
	// A key the Agent reported with its generation is not touched.
	require.Equal(t, key+"@2", ANXClientPublicKey(key+"@2", map[string]any{"wire_gen": float64(2)}))
	require.Equal(t, key+"@2", ANXClientPublicKey(key+"@2", nil))
	// No key yet: nothing to publish.
	require.Empty(t, ANXClientPublicKey("", map[string]any{"wire_gen": float64(2)}))
}

func TestApplyANXUser(t *testing.T) {
	parsed := &model.ParsedNode{Type: "anx", Settings: map[string]any{"server_names": []any{"x.example.com"}, "alpn": []any{"h2"}}}
	applyANXUser(parsed, &model.TemplateRenderContext{UUID: "c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"})
	require.Equal(t, ANXUserKey("c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"), parsed.Password)
	require.Equal(t, "x.example.com", parsed.ServerName)
	require.Equal(t, "h2", parsed.ALPN)

	noSNI := &model.ParsedNode{Type: "anx", Settings: map[string]any{"server_names": []any{"x.example.com"}, "allow_no_sni": true}}
	applyANXUser(noSNI, &model.TemplateRenderContext{UUID: "c9a1e7b0-5b0e-4a4b-9a54-2f3f1c1b0d11"})
	require.Empty(t, noSNI.ServerName)
}

func TestValidateANXProtocol(t *testing.T) {
	valid := []string{``, `{}`, `{"server_names":["www.apple.com"],"dest":"www.apple.com:443","alpn":["h2"]}`,
		`{"alpn":["h2","http/1.1"]}`, `{"allow_no_sni":true,"shape_plain":false,"max_streams":1024}`, `{"max_streams":65535}`, `{"wg_listen_port":51820}`,
		`{"allow_no_sni":true,"bare_listen_port":8444}`, `{"allow_no_sni":true,"wg_listen_port":51820,"bare_listen_port":8444}`,
		`{"dest_stack":"rustls"}`, `{"dest_behavior":{}}`, `{"dest_behavior":{"session_tickets":"none","flight_records":"none"}}`,
		`{"dest_behavior":{"session_tickets":"266/266","flight_records":"122/1803/79/52","flight_gap_ms":5,"read_ahead":true,"ccs_overflow":"alert:10","max_ccs":"32"}}`,
		`{"wire_gen":1}`, `{"wire_gen":2}`, `{"wire_gen":2,"min_wire_gen":1}`, `{"wire_gen":2,"min_wire_gen":2}`}
	for _, settings := range valid {
		s := settings
		require.NoError(t, validateANXProtocol(&model.NodeProtocol{Type: model.ProtocolANX, Settings: &s}), settings)
	}
	invalid := []string{`[]`, `{"server_names":[]}`, `{"server_names":["a b"]}`, `{"server_names":"x"}`,
		`{"dest":"www.apple.com"}`, `{"dest":"www.apple.com:0"}`, `{"dest":5}`, `{"alpn":[1]}`,
		`{"alpn":["h2,http/1.1"]}`, `{"alpn":["http 1.1"]}`, `{"server_names":["a/b"]}`,
		`{"allow_no_sni":"yes"}`, `{"shape_plain":1}`, `{"max_streams":0}`, `{"max_streams":65536}`, `{"max_streams":1.5}`, `{"max_streams":"256"}`,
		`{"wg_listen_port":0}`, `{"wg_listen_port":65536}`, `{"wg_listen_port":"51820"}`,
		`{"bare_listen_port":8444}`, `{"allow_no_sni":false,"bare_listen_port":8444}`, `{"allow_no_sni":true,"bare_listen_port":0}`,
		`{"allow_no_sni":true,"bare_listen_port":"8444"}`, `{"allow_no_sni":true,"wg_listen_port":8444,"bare_listen_port":8444}`,
		`{"dest_stack":"schannel"}`, `{"dest_stack":1}`, `{"dest_behavior":"session_tickets=266"}`, `{"dest_behavior":{"nope":1}}`,
		`{"dest_behavior":{"session_tickets":"34"}}`, `{"dest_behavior":{"session_tickets":"100/100/100/100/100/100/100/100/100"}}`, `{"dest_behavior":{"flight_records":"17"}}`,
		`{"dest_behavior":{"flight_records":266}}`, `{"dest_behavior":{"max_ccs":1.5}}`, `{"dest_behavior":{"timeout":""}}`, `{"dest_behavior":{"max_ccs":[1]}}`,
		`{"wire_gen":3}`, `{"wire_gen":"2"}`, `{"min_wire_gen":0}`, `{"min_wire_gen":2}`, `{"wire_gen":1,"min_wire_gen":2}`}
	for _, settings := range invalid {
		s := settings
		require.Error(t, validateANXProtocol(&model.NodeProtocol{Type: model.ProtocolANX, Settings: &s}), settings)
	}
}

// The same vector as anix-agent core/anixcore TestAnxUserKeyMatchesControl.
func TestANXUserKeyVector(t *testing.T) {
	require.Equal(t, "a05385f0ca7b3e2223b65d0af4dbc51cbd50a1330d27f509ed9de142b771f20a", ANXUserKey("C9A1E7B0-5B0E-4A4B-9A54-2F3F1C1B0D11"))
}
