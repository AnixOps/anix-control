package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// NodeSecretsTestSuite holds the regression tests for node secrets in the
// administrator's answers: protocol settings, raw configurations and
// forward node tokens read as the placeholder, a
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
	s.node = &model.Node{Name: "edge", Host: "edge.example.test", Port: 443,
		RawConfig: &s.rawConfig}
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

	s.Require().NoError(nodesecrets.SeedForTest(s.db))

	s.secretText = []string{realityPrivateKey, tlsPrivateKeyPEM, hysteriaPassword, customSecret, rawConfigSecret, s.wgPrivate, "node-api-key", "node-shared-secret"}
}

func (s *NodeSecretsTestSuite) wireGuardSettings(privateKey, publicKey string) string {
	return fmt.Sprintf(`{"cidr":"10.77.0.0/24","server_address":"10.77.0.1/24","server_private_key":%q,"server_public_key":%q}`,
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

func settingsOf(text *string) map[string]any {
	var value map[string]any
	if text != nil {
		_ = json.Unmarshal([]byte(*text), &value)
	}
	return value
}

// A raw configuration saved back with its placeholders through the node
// update keeps its secrets; a new value replaces them.
func (s *NodeSecretsTestSuite) TestRawConfigKeepsMaskedSecrets() {
	handler := NewNodeHandler()
	nodePath := fmt.Sprintf("/nodes/%d", s.node.ID)
	var node model.Node
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	nodesecrets.ResolveNodeRawConfig(s.db, &node)

	masked := service.RedactNodeSecretsJSON(*node.RawConfig)
	body, err := json.Marshal(map[string]any{"name": "renamed", "Raw_Config": masked})
	s.Require().NoError(err)
	s.ok(s.serve("PUT", "/nodes/:id", nodePath, string(body), handler.UpdateNode))
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	nodesecrets.ResolveNodeRawConfig(s.db, &node)
	s.Equal("renamed", node.Name)
	s.Equal(rawConfigSecret, settingsOf(node.RawConfig)["tls_settings"].(map[string]any)["private_key"])

	s.ok(s.serve("PUT", "/nodes/:id", nodePath, `{"raw_config":"{\"tls_settings\":{\"private_key\":\"rotated-raw-key\"}}"}`, handler.UpdateNode))
	s.Require().NoError(s.db.First(&node, s.node.ID).Error)
	nodesecrets.ResolveNodeRawConfig(s.db, &node)
	s.Equal("rotated-raw-key", settingsOf(node.RawConfig)["tls_settings"].(map[string]any)["private_key"])
}

// Nodes get their runtime configuration built from the stored rows: it
// carries the real secrets.
func (s *NodeSecretsTestSuite) TestNodesReceiveRealSecrets() {
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.node.ID).Update("raw_config", nil).Error)

	runtime := func(nodeType string) string {
		config, err := service.BuildNodeRuntimeConfig(s.db, s.node.ID, nodeType)
		s.Require().NoError(err)
		encoded, err := json.Marshal(config)
		s.Require().NoError(err)
		return string(encoded)
	}
	vless := runtime("vless")
	s.Contains(vless, realityPrivateKey)
	s.Contains(vless, tlsPrivateKeyPEM)
	s.Contains(vless, customSecret)
	s.NotContains(vless, service.NodeSecretPlaceholder)
	s.Contains(runtime("wireguard"), s.wgPrivate)
}

// Proxy node credentials stay out of the node answers; the per-node reveal
// route answers them, and the audit log records each read of it.
func TestNodeSecrets(t *testing.T) {
	suite.Run(t, new(NodeSecretsTestSuite))
}
