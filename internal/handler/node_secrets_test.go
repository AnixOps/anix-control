package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// NodeSecretsTestSuite holds the regression tests for node secrets in the
// administrator's answers: protocol settings, raw configurations,
// registration keys and forward node tokens read as the placeholder, a
// write that sends the placeholder back keeps the stored secret, and nodes
// still receive the real values.
type NodeSecretsTestSuite struct {
	HandlerTestSuite
	node       *model.Node
	reality    *model.NodeProtocol
	wireGuard  *model.NodeProtocol
	wgPrivate  string
	wgPublic   string
	group      *model.SubscriptionGroup
	rawConfig  string
	secretText []string
}

const (
	realityPrivateKey = "reality-private-key-value"
	realityPublicKey  = "reality-public-key-value"
	realityShortID    = "6ba85179e30d4fc2"
	tlsPrivateKeyPEM  = "-----BEGIN PRIVATE KEY-----tls-key-----END PRIVATE KEY-----"
	hysteriaPassword  = "hysteria-obfs-password"
	customSecret      = "custom-config-token"
	rawConfigSecret   = "raw-config-private-key"
)

func (s *NodeSecretsTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	var err error
	s.wgPrivate, s.wgPublic, err = service.GenerateWireGuardKeypair()
	s.Require().NoError(err)

	s.rawConfig = `{"type":"vless","node_type":"vless","server_port":443,"tls_settings":{"private_key":"` + rawConfigSecret + `","server_name":"raw.example.test"}}`
	s.node = &model.Node{Name: "edge", Host: "edge.example.test", Port: 443, APIKey: "node-api-key", APIKeyHash: hashString("node-api-key"),
		Secret: "node-shared-secret", RawConfig: &s.rawConfig}
	s.Require().NoError(s.db.Create(s.node).Error)

	settings := `{"flow":"xtls-rprx-vision","obfs":"salamander","obfs-password":"` + hysteriaPassword + `"}`
	tlsSettings := `{"server_name":"www.example.test","key":"` + tlsPrivateKeyPEM + `"}`
	realitySettings := `{"dest":"www.example.test:443","private_key":"` + realityPrivateKey + `","public_key":"` + realityPublicKey + `","short_id":"` + realityShortID + `"}`
	customConfig := `{"api":{"token":"` + customSecret + `"}}`
	s.reality = &model.NodeProtocol{NodeID: s.node.ID, Name: "reality", Type: model.ProtocolVLESS, Port: 443, Enable: 1, Show: 1, TLS: 2,
		Settings: &settings, TLSSettings: &tlsSettings, RealitySettings: &realitySettings, CustomConfig: &customConfig}
	s.Require().NoError(s.db.Create(s.reality).Error)

	wgSettings := s.wireGuardSettings(s.wgPrivate, s.wgPublic)
	s.wireGuard = &model.NodeProtocol{NodeID: s.node.ID, Name: "wg", Type: model.ProtocolWireGuard, Port: 51820, Enable: 1, Show: 1, Settings: &wgSettings}
	s.Require().NoError(s.db.Create(s.wireGuard).Error)

	s.group = &model.SubscriptionGroup{Name: "group", Enable: 1}
	s.Require().NoError(s.db.Create(s.group).Error)
	s.Require().NoError(s.db.Model(s.group).Association("Protocols").Append(s.reality))

	s.secretText = []string{realityPrivateKey, tlsPrivateKeyPEM, hysteriaPassword, customSecret, rawConfigSecret, s.wgPrivate, "node-api-key", "node-shared-secret"}
}

func (s *NodeSecretsTestSuite) wireGuardSettings(privateKey, publicKey string) string {
	return fmt.Sprintf(`{"cidr":"10.77.0.0/24","server_address":"10.77.0.1/24","server_private_key":%q,"server_public_key":%q,`+
		`"relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"}}`,
		privateKey, publicKey)
}

func (s *NodeSecretsTestSuite) serve(method, pattern, path, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	router := gin.New()
	router.Handle(method, pattern, handler)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func (s *NodeSecretsTestSuite) ok(w *httptest.ResponseRecorder) map[string]any {
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	resp := decodePanelTestResponse(s.T(), w)
	s.Require().Equal(float64(0), resp["code"], "%v", resp)
	return resp
}

// requireMasked checks that an answer shows none of the stored secrets but
// still shows the public halves.
func (s *NodeSecretsTestSuite) requireMasked(name string, w *httptest.ResponseRecorder) {
	s.Require().Equal(http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
	body := w.Body.String()
	for _, secret := range s.secretText {
		s.NotContains(body, secret, name)
	}
	s.Contains(body, service.NodeSecretPlaceholder, name)
}

func (s *NodeSecretsTestSuite) storedProtocol(id uint) model.NodeProtocol {
	var protocol model.NodeProtocol
	s.Require().NoError(s.db.First(&protocol, id).Error)
	return protocol
}

func settingsOf(text *string) map[string]any {
	var value map[string]any
	if text != nil {
		_ = json.Unmarshal([]byte(*text), &value)
	}
	return value
}

// Every administrator answer that shows node protocols masks their secret
// settings, and the nodes' raw configuration secrets, but keeps public keys
// and Reality's short_id.
func (s *NodeSecretsTestSuite) TestAnswersMaskProtocolSecrets() {
	handler := NewNodeHandler()
	subscriptions := NewSubscriptionAdminHandler()
	nodePath := fmt.Sprintf("/nodes/%d", s.node.ID)
	answers := map[string]*httptest.ResponseRecorder{
		"node list":           s.serve("GET", "/nodes", "/nodes", "", handler.GetNodes),
		"node detail":         s.serve("GET", "/nodes/:id", nodePath, "", handler.GetNode),
		"protocol list":       s.serve("GET", "/nodes/:id/protocols", nodePath+"/protocols", "", handler.GetProtocols),
		"group protocols":     s.serve("GET", "/groups/:id/protocols", fmt.Sprintf("/groups/%d/protocols", s.group.ID), "", subscriptions.GetGroupProtocols),
		"available protocols": s.serve("GET", "/protocols/available", "/protocols/available", "", subscriptions.GetAvailableProtocols),
	}
	for name, w := range answers {
		s.requireMasked(name, w)
		s.Contains(w.Body.String(), realityPublicKey, name)
		s.Contains(w.Body.String(), realityShortID, name)
	}
	s.Contains(answers["protocol list"].Body.String(), s.wgPublic)

	resp := s.ok(answers["protocol list"])
	protocols := resp["data"].([]any)
	s.Require().Len(protocols, 2)
	reality := protocols[0].(map[string]any)
	realitySettings := settingsOf(stringPointer(reality["reality_settings"]))
	s.Equal(service.NodeSecretPlaceholder, realitySettings["private_key"])
	s.Equal(realityPublicKey, realitySettings["public_key"])
	s.Equal(service.NodeSecretPlaceholder, settingsOf(stringPointer(reality["settings"]))["obfs-password"])
	s.Equal("salamander", settingsOf(stringPointer(reality["settings"]))["obfs"])
	s.Equal(service.NodeSecretPlaceholder, settingsOf(stringPointer(reality["tls_settings"]))["key"])

	raw := s.ok(s.serve("GET", "/nodes/:id/raw-config", nodePath+"/raw-config", "", handler.GetNodeRawConfig))
	rawConfig := raw["data"].(map[string]any)["raw_config"].(map[string]any)
	tls := rawConfig["tls_settings"].(map[string]any)
	s.Equal(service.NodeSecretPlaceholder, tls["private_key"])
	s.Equal("raw.example.test", tls["server_name"])

	// The answer that creates a protocol masks it too.
	body := `{"name":"new","type":"vless","port":8443,"reality_settings":"{\"private_key\":\"fresh-private\",\"short_id\":\"aa\"}"}`
	created := s.serve("POST", "/nodes/:id/protocols", nodePath+"/protocols", body, handler.CreateProtocol)
	s.Require().Equal(http.StatusOK, created.Code, created.Body.String())
	s.NotContains(created.Body.String(), "fresh-private")
	data := s.ok(created)["data"].(map[string]any)
	stored := s.storedProtocol(uint(data["id"].(float64)))
	s.Equal("fresh-private", settingsOf(stored.RealitySettings)["private_key"], "the protocol stores the real key")

	// The stored rows are unchanged.
	s.Equal(realityPrivateKey, settingsOf(s.storedProtocol(s.reality.ID).RealitySettings)["private_key"])
}

func stringPointer(value any) *string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

// The protocol editor sends back what it was shown: the placeholder keeps
// the stored secret, in a JSON string or a JSON object and in any spelling
// of the column, and a new value replaces it.
func (s *NodeSecretsTestSuite) TestProtocolUpdateKeepsMaskedSecrets() {
	handler := NewNodeHandler()
	path := fmt.Sprintf("/nodes/%d/protocols/%d", s.node.ID, s.reality.ID)
	shown := s.ok(s.serve("GET", "/nodes/:id/protocols", fmt.Sprintf("/nodes/%d/protocols", s.node.ID), "", handler.GetProtocols))
	reality := shown["data"].([]any)[0].(map[string]any)

	update := map[string]any{"name": "renamed"}
	for _, column := range []string{"settings", "tls_settings", "reality_settings", "custom_config"} {
		update[column] = reality[column]
	}
	encoded, err := json.Marshal(update)
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path, string(encoded), handler.UpdateProtocol))
	stored := s.storedProtocol(s.reality.ID)
	s.Equal("renamed", stored.Name)
	s.Equal(realityPrivateKey, settingsOf(stored.RealitySettings)["private_key"])
	s.Equal(realityPublicKey, settingsOf(stored.RealitySettings)["public_key"])
	s.Equal(hysteriaPassword, settingsOf(stored.Settings)["obfs-password"])
	s.Equal(tlsPrivateKeyPEM, settingsOf(stored.TLSSettings)["key"])
	s.Equal(customSecret, settingsOf(stored.CustomConfig)["api"].(map[string]any)["token"])

	// An object and another spelling of the column keep it too.
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path,
		`{"RealitySettings":{"private_key":"********","public_key":"rotated-public","short_id":"cd"}}`, handler.UpdateProtocol))
	stored = s.storedProtocol(s.reality.ID)
	s.Equal(realityPrivateKey, settingsOf(stored.RealitySettings)["private_key"])
	s.Equal("rotated-public", settingsOf(stored.RealitySettings)["public_key"])

	// A new value replaces the stored one.
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path,
		`{"reality_settings":"{\"private_key\":\"rotated-private\",\"public_key\":\"rotated-public\"}"}`, handler.UpdateProtocol))
	s.Equal("rotated-private", settingsOf(s.storedProtocol(s.reality.ID).RealitySettings)["private_key"])
}

// A WireGuard protocol saved back with its masked private key keeps the key
// and passes the check of the key pair; a new pair replaces it.
func (s *NodeSecretsTestSuite) TestWireGuardUpdateKeepsTheMaskedPrivateKey() {
	handler := NewNodeHandler()
	path := fmt.Sprintf("/nodes/%d/protocols/%d", s.node.ID, s.wireGuard.ID)
	masked := service.RedactNodeSecretsJSON(*s.wireGuard.Settings)
	s.Require().NotContains(masked, s.wgPrivate)
	body, err := json.Marshal(map[string]any{"type": "wireguard", "port": 51820, "enable": 1, "settings": masked})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path, string(body), handler.UpdateProtocol))
	settings := settingsOf(s.storedProtocol(s.wireGuard.ID).Settings)
	s.Equal(s.wgPrivate, settings["server_private_key"])
	s.Equal(s.wgPublic, settings["server_public_key"])

	private, public, err := service.GenerateWireGuardKeypair()
	s.Require().NoError(err)
	body, err = json.Marshal(map[string]any{"settings": s.wireGuardSettings(private, public)})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path, string(body), handler.UpdateProtocol))
	s.Equal(private, settingsOf(s.storedProtocol(s.wireGuard.ID).Settings)["server_private_key"])
}

// A new protocol has nothing stored: a placeholder in it stands for no
// secret, and a WireGuard protocol needs its real private key.
func (s *NodeSecretsTestSuite) TestCreateProtocolStoresNoPlaceholder() {
	handler := NewNodeHandler()
	path := fmt.Sprintf("/nodes/%d/protocols", s.node.ID)
	data := s.ok(s.serve("POST", "/nodes/:id/protocols", path,
		`{"name":"copy","type":"vless","port":8443,"reality_settings":"{\"private_key\":\"********\",\"short_id\":\"ab\"}"}`, handler.CreateProtocol))["data"].(map[string]any)
	stored := s.storedProtocol(uint(data["id"].(float64)))
	s.NotContains(*stored.RealitySettings, service.NodeSecretPlaceholder)
	s.Equal("", settingsOf(stored.RealitySettings)["private_key"])

	settings := service.RedactNodeSecretsJSON(*s.wireGuard.Settings)
	body, err := json.Marshal(map[string]any{"name": "wg-copy", "type": "wireguard", "port": 51821, "enable": 1, "settings": settings})
	s.Require().NoError(err)
	w := s.serve("POST", "/nodes/:id/protocols", path, string(body), handler.CreateProtocol)
	s.Equal(http.StatusBadRequest, w.Code, w.Body.String())
}

// A raw configuration saved back with its placeholders keeps its secrets,
// through the raw configuration route and the node update; a new value
// replaces them.
func (s *NodeSecretsTestSuite) TestRawConfigKeepsMaskedSecrets() {
	handler := NewNodeHandler()
	nodePath := fmt.Sprintf("/nodes/%d", s.node.ID)
	shown := s.ok(s.serve("GET", "/nodes/:id/raw-config", nodePath+"/raw-config", "", handler.GetNodeRawConfig))
	rawConfig := shown["data"].(map[string]any)["raw_config"].(map[string]any)
	rawConfig["server_port"] = 8443
	body, err := json.Marshal(map[string]any{"raw_config": rawConfig})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id/raw-config", nodePath+"/raw-config", string(body), handler.UpdateNodeRawConfig))

	var node model.Node
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	stored := settingsOf(node.RawConfig)
	s.EqualValues(8443, stored["server_port"])
	s.Equal(rawConfigSecret, stored["tls_settings"].(map[string]any)["private_key"])

	masked := service.RedactNodeSecretsJSON(*node.RawConfig)
	body, err = json.Marshal(map[string]any{"name": "renamed", "Raw_Config": masked})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id", nodePath, string(body), handler.UpdateNode))
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	s.Equal("renamed", node.Name)
	s.Equal(rawConfigSecret, settingsOf(node.RawConfig)["tls_settings"].(map[string]any)["private_key"])

	s.ok(s.serve("PUT", "/nodes/:id/raw-config", nodePath+"/raw-config",
		`{"raw_config":{"tls_settings":{"private_key":"rotated-raw-key"}}}`, handler.UpdateNodeRawConfig))
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	s.Equal("rotated-raw-key", settingsOf(node.RawConfig)["tls_settings"].(map[string]any)["private_key"])
}

// Nodes read their configuration through UniProxy, which builds it from the
// stored rows: it still carries the real secrets after an administrator
// saved the masked form back.
func (s *NodeSecretsTestSuite) TestNodesStillReceiveRealSecrets() {
	handler := NewNodeHandler()
	path := fmt.Sprintf("/nodes/%d/protocols/%d", s.node.ID, s.reality.ID)
	body, err := json.Marshal(map[string]any{"reality_settings": service.RedactNodeSecretsJSON(*s.reality.RealitySettings)})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id/protocols/:protocol_id", path, string(body), handler.UpdateProtocol))
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.node.ID).Update("raw_config", nil).Error)

	uniProxy := NewUniProxyHandler()
	w := s.serve("GET", "/config", fmt.Sprintf("/config?node_id=%d&node_type=vless", s.node.ID), "", uniProxy.GetConfig)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	s.Contains(w.Body.String(), realityPrivateKey)
	s.Contains(w.Body.String(), tlsPrivateKeyPEM)
	s.Contains(w.Body.String(), customSecret)
	s.NotContains(w.Body.String(), service.NodeSecretPlaceholder)

	w = s.serve("GET", "/config", fmt.Sprintf("/config?node_id=%d&node_type=wireguard", s.node.ID), "", uniProxy.GetConfig)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	s.Contains(w.Body.String(), s.wgPrivate)
}

// GET /admin/auth-keys showed every registration key; a key is now shown
// once, when it is generated, and still registers a node.
func (s *NodeSecretsTestSuite) TestRegistrationKeysAreShownOnce() {
	handler := NewNodeHandler()
	generated := s.ok(s.serve("POST", "/auth-keys", "/auth-keys", `{"name":"tokyo"}`, handler.GenerateAuthKey))
	key := generated["data"].(map[string]any)["key"].(string)
	s.Require().NotEmpty(key)
	s.NotEqual(service.NodeSecretPlaceholder, key)

	w := s.serve("GET", "/auth-keys", "/auth-keys", "", handler.GetAuthKeys)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	s.NotContains(w.Body.String(), key)
	keys := s.ok(w)["data"].([]any)
	s.Require().Len(keys, 1)
	s.Equal(service.NodeSecretPlaceholder, keys[0].(map[string]any)["key"])
	s.Equal("tokyo", keys[0].(map[string]any)["name"])

	registered := s.serve("POST", "/node/register", "/node/register", `{"auth_key":"`+key+`","name":"auto"}`, handler.Register)
	s.Equal(http.StatusOK, registered.Code, registered.Body.String())
	s.Contains(registered.Body.String(), `"api_key"`)
}

// Proxy node credentials stay out of the node answers; the per-node reveal
// route answers them, and the audit log records each read of it.
func (s *NodeSecretsTestSuite) TestNodeCredentialsOnlyThroughTheReveal() {
	handler := NewNodeHandler()
	nodePath := fmt.Sprintf("/nodes/%d", s.node.ID)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"node list":   s.serve("GET", "/nodes", "/nodes", "", handler.GetNodes),
		"node detail": s.serve("GET", "/nodes/:id", nodePath, "", handler.GetNode),
	} {
		s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
		s.NotContains(w.Body.String(), "node-api-key", name)
		s.NotContains(w.Body.String(), "node-shared-secret", name)
	}
	revealed := s.ok(s.serve("GET", "/nodes/:id/credentials", nodePath+"/credentials", "", handler.GetNodeCredentials))
	s.Equal("node-api-key", revealed["data"].(map[string]any)["api_key"])
	s.Equal("node-shared-secret", revealed["data"].(map[string]any)["secret"])
}

func TestNodeSecrets(t *testing.T) {
	suite.Run(t, new(NodeSecretsTestSuite))
}

// ForwardNodeTokenTestSuite holds the regression tests for forward node API
// tokens in the administrator's answers.
type ForwardNodeTokenTestSuite struct {
	HandlerTestSuite
	relay *model.ForwardNode
	exit  *model.ForwardNode
	rule  *model.ForwardRule
}

func (s *ForwardNodeTokenTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.relay = &model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-secret-token", Enabled: true}
	s.exit = &model.ForwardNode{Name: "exit", Type: model.ForwardNodeTypeExit, Host: "198.51.100.20", Port: 443, APIPort: 18081, APIToken: "exit-secret-token", Enabled: true}
	s.Require().NoError(s.db.Create(s.relay).Error)
	s.Require().NoError(s.db.Create(s.exit).Error)
	s.rule = &model.ForwardRule{Name: "rule", Enabled: true, RelayNodeID: s.relay.ID, ExitNodeID: s.exit.ID,
		ListenPort: 20001, Protocol: "tcp", TargetHost: "203.0.113.5", TargetPort: 8443}
	s.Require().NoError(s.db.Create(s.rule).Error)
}

func TestForwardNodeTokens(t *testing.T) {
	suite.Run(t, new(ForwardNodeTokenTestSuite))
}
