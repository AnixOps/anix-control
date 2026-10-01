package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// NodeCredentialAnswersTestSuite pins the bytes the legacy credential and
// secret routes answer: node creation and deletion, raw configurations and
// their validation, protocols, registration keys and clean agents. The
// KernelNodeOps credential, secret and retirement executors (NO-5) share
// these routes' implementation; the answers stay what they were before the
// routes moved onto it. Only the values that change from one call to the
// next are normalized (normalizeCredentialAnswer): the envelope's ts, the
// generated keys and tokens, and the timestamps of created rows.
type NodeCredentialAnswersTestSuite struct {
	HandlerTestSuite
}

func TestNodeCredentialAnswers(t *testing.T) {
	suite.Run(t, new(NodeCredentialAnswersTestSuite))
}

var credentialVolatile = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`"ts":\d+`), `"ts":0`},
	{regexp.MustCompile(`"(api_key|secret|key)":"[0-9a-f]{64}"`), `"$1":"<hex64>"`},
	{regexp.MustCompile(`"token":"v2fa_[A-Za-z0-9_-]{43}"`), `"token":"<clean-agent-token>"`},
	{regexp.MustCompile(`"(created_at|updated_at)":"[^"]*"`), `"$1":"-"`},
	{regexp.MustCompile(`"expire_at":\d+`), `"expire_at":1`},
}

func normalizeCredentialAnswer(body string) string {
	for _, volatile := range credentialVolatile {
		body = volatile.pattern.ReplaceAllString(body, volatile.with)
	}
	return body
}

// pinnedCredentialAnswers are the legacy answers, normalized, by case.
var pinnedCredentialAnswers = map[string]string{
	"create clean agent":                            `200 {"code":0,"data":{"agent":{"id":1,"nodeId":10,"name":"relay-agent","version":"","hostname":"","os":"","arch":"","kernel":"","publicIp":"","privateIp":"","capabilities":"","status":0,"lastSeen":null,"lastError":"","revokedAt":null,"created_at":"-","updated_at":"-"},"token":"<clean-agent-token>"},"msg":"操作成功","ts":0}`,
	"create clean agent without a name":             `200 {"code":0,"data":{"agent":{"id":2,"nodeId":10,"name":"v2forward-agent","version":"","hostname":"","os":"","arch":"","kernel":"","publicIp":"","privateIp":"","capabilities":"","status":0,"lastSeen":null,"lastError":"","revokedAt":null,"created_at":"-","updated_at":"-"},"token":"<clean-agent-token>"},"msg":"操作成功","ts":0}`,
	"create clean agent without a node":             `200 {"code":-1,"data":null,"msg":"nodeId is required: a clean agent token is issued for one forward node","ts":0}`,
	"create clean agent for a missing node":         `200 {"code":-1,"data":null,"msg":"forward node not found","ts":0}`,
	"create clean agent, bad body":                  `200 {"code":-1,"data":null,"msg":"invalid request body","ts":0}`,
	"revoke clean agent":                            `200 {"code":0,"data":true,"msg":"操作成功","ts":0}`,
	"revoke clean agent again":                      `200 {"code":0,"data":true,"msg":"操作成功","ts":0}`,
	"revoke clean agent that does not exist":        `200 {"code":-1,"data":null,"msg":"agent not found","ts":0}`,
	"revoke clean agent, bad id":                    `200 {"code":-1,"data":null,"msg":"invalid agent id","ts":0}`,
	"create node":                                   `200 {"code":0,"data":{"api_key":"<hex64>","node_id":1,"secret":"<hex64>"},"msg":"操作成功","ts":0}`,
	"create node, bad body":                         `400 {"error":"unexpected EOF","message":"参数错误"}`,
	"put raw config":                                `200 {"code":0,"data":{"message":"配置更新成功"},"msg":"操作成功","ts":0}`,
	"put raw config with the placeholder":           `200 {"code":0,"data":{"message":"配置更新成功"},"msg":"操作成功","ts":0}`,
	"put raw config that is not an object":          `400 {"message":"原始配置必须是 JSON 对象"}`,
	"put raw config with an invalid WireGuard key":  `400 {"error":"invalid node protocol: WireGuard server_private_key 必须是标准 Base64 的 32 字节密钥","message":"WireGuard 配置无效"}`,
	"put raw config, bad id":                        `400 {"message":"无效的节点ID"}`,
	"update node with the placeholder":              `200 {"code":0,"data":{"message":"更新成功"},"msg":"操作成功","ts":0}`,
	"validate config":                               `200 {"code":0,"data":{"message":"配置有效","size":34,"valid":true,"warnings":[]},"msg":"操作成功","ts":0}`,
	"validate config without server_port":           `200 {"code":0,"data":{"message":"配置有效","size":16,"valid":true,"warnings":["缺少 server_port 字段"]},"msg":"操作成功","ts":0}`,
	"validate config that is not an object":         `400 {"message":"配置必须是 JSON 对象","valid":false}`,
	"validate config with an invalid WireGuard key": `400 {"error":"invalid node protocol: WireGuard server_private_key 必须是标准 Base64 的 32 字节密钥","message":"WireGuard 配置无效","valid":false}`,
	"validate config without raw_config":            `400 {"error":"Key: 'RawConfig' Error:Field validation for 'RawConfig' failed on the 'required' tag","message":"参数错误"}`,
	"delete node":                                   `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete node again":                             `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete node, bad id":                           `400 {"message":"无效的节点ID"}`,
	"create protocol":                               `200 {"code":0,"data":{"id":1,"node_id":1,"name":"reality","type":"vless","port":443,"enable":1,"show":1,"sort":0,"group_id":null,"host":null,"tls":2,"alpn":null,"settings":null,"tls_settings":null,"transport":null,"transport_settings":null,"reality_settings":"{\"dest\":\"www.example.test:443\",\"private_key\":\"********\",\"public_key\":\"reality-public\",\"short_id\":\"6ba85179\"}","custom_config":null,"created_at":"-","updated_at":"-"},"msg":"操作成功","ts":0}`,
	"create protocol with an invalid WireGuard key": `400 {"error":"invalid node protocol: WireGuard server_address 必须是 cidr 内的同掩码地址: \"10.66.0.1/24\"","message":"创建失败"}`,
	"create protocol, bad id":                       `400 {"message":"无效的节点ID"}`,
	"update protocol with the placeholder":          `200 {"code":0,"data":{"message":"更新成功"},"msg":"操作成功","ts":0}`,
	"update protocol with a new secret":             `200 {"code":0,"data":{"message":"更新成功"},"msg":"操作成功","ts":0}`,
	"update protocol with an invalid WireGuard key": `400 {"error":"invalid node protocol: WireGuard server_address 必须是 cidr 内的同掩码地址: \"10.66.0.1/24\"","message":"更新失败"}`,
	"update protocol that does not exist":           `500 {"error":"record not found","message":"更新失败"}`,
	"delete protocol":                               `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete protocol again":                         `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete protocol, bad id":                       `400 {"message":"无效的协议ID"}`,
	"generate auth key":                             `200 {"code":0,"data":{"expire_at":1,"id":1,"key":"<hex64>","name":"bootstrap"},"msg":"操作成功","ts":0}`,
	"generate auth key without expiry":              `200 {"code":0,"data":{"expire_at":null,"id":2,"key":"<hex64>","name":"forever"},"msg":"操作成功","ts":0}`,
	"generate auth key, bad body":                   `400 {"message":"参数错误"}`,
	"internal generate auth key":                    `200 {"data":{"expire_at":1,"id":3,"key":"<hex64>","name":"ansible"},"message":"生成成功"}`,
	"internal generate auth key without a name":     `400 {"message":"参数错误"}`,
	"delete auth key":                               `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete auth key again":                         `200 {"code":0,"data":{"message":"删除成功"},"msg":"操作成功","ts":0}`,
	"delete auth key, bad id":                       `400 {"message":"无效的ID"}`,
}

func (s *NodeCredentialAnswersTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.Require().NoError(s.db.AutoMigrate(&model.ForwardCleanAgent{}, &model.WireGuardPeer{}))
	s.Require().NoError(nodesecrets.EnsureSchema(s.db))
}

func (s *NodeCredentialAnswersTestSuite) serve(method, pattern, path, body string, handler gin.HandlerFunc) (int, string) {
	router := gin.New()
	router.Handle(method, pattern, func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Set("is_admin", true)
		handler(c)
	})
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, normalizeCredentialAnswer(w.Body.String())
}

// pin compares an answer with its pinned bytes. With PIN_PRINT=1 the
// answers are printed instead, to record them.
func (s *NodeCredentialAnswersTestSuite) pin(name string, code int, body string) {
	s.T().Helper()
	if os.Getenv("PIN_PRINT") == "1" {
		fmt.Printf("PIN %s %d %s\n", name, code, body)
		return
	}
	want, ok := pinnedCredentialAnswers[name]
	s.Require().True(ok, "no pinned answer for %s: %d %s", name, code, body)
	s.Equal(want, fmt.Sprintf("%d %s", code, body), name)
}

const (
	pinRawConfigSecret = "pinned-raw-config-private-key"
	pinRealitySecret   = "pinned-reality-private-key"
)

func (s *NodeCredentialAnswersTestSuite) wireGuardRawConfig(privateKey string) string {
	return `{"type":"wireguard","node_type":"wireguard","server_port":51820,"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24","server_private_key":"` + privateKey + `"}`
}

func (s *NodeCredentialAnswersTestSuite) TestNodesAndRawConfigs() {
	h := NewNodeHandler()
	code, body := s.serve(http.MethodPost, "/admin/nodes", "/admin/nodes", `{"name":"edge","host":"edge.example.test","port":443}`, h.CreateNode)
	s.pin("create node", code, body)
	var node model.Node
	s.Require().NoError(s.db.First(&node, 1).Error)
	s.Len(node.APIKey, 64)
	s.Equal(hashString(node.APIKey), node.APIKeyHash)
	s.Len(node.Secret, 64)
	s.Equal(model.NodeStatusPending, node.Status)
	s.Equal(int64(1), s.count(&model.NodeProtocol{}, "node_id = ?", node.ID), "the default protocol")
	s.Equal(int64(2), s.count(&model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectProxy, node.ID), "dual-written")

	code, body = s.serve(http.MethodPost, "/admin/nodes", "/admin/nodes", `{"name":`, h.CreateNode)
	s.pin("create node, bad body", code, body)

	// A raw configuration with a secret, then saved back with the
	// placeholder: the stored secret is kept.
	raw := `{"type":"vless","server_port":443,"tls_settings":{"private_key":"` + pinRawConfigSecret + `","server_name":"edge.example.test"}}`
	code, body = s.serve(http.MethodPut, "/admin/nodes/:id/raw-config", "/admin/nodes/1/raw-config", `{"raw_config":`+raw+`}`, h.UpdateNodeRawConfig)
	s.pin("put raw config", code, body)
	s.Require().NoError(s.db.First(&node, 1).Error)
	s.Require().NotNil(node.RawConfig)
	s.Contains(*node.RawConfig, pinRawConfigSecret)
	s.Equal(int64(1), s.count(&model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeRawConfig, node.ID))

	masked := strings.ReplaceAll(raw, pinRawConfigSecret, service.NodeSecretPlaceholder)
	code, body = s.serve(http.MethodPut, "/admin/nodes/:id/raw-config", "/admin/nodes/1/raw-config", `{"raw_config":"`+strings.ReplaceAll(masked, `"`, `\"`)+`"}`, h.UpdateNodeRawConfig)
	s.pin("put raw config with the placeholder", code, body)
	s.Require().NoError(s.db.First(&node, 1).Error)
	s.Contains(*node.RawConfig, pinRawConfigSecret, "the placeholder keeps the stored secret")

	code, body = s.serve(http.MethodPut, "/admin/nodes/:id/raw-config", "/admin/nodes/1/raw-config", `{"raw_config":"not json"}`, h.UpdateNodeRawConfig)
	s.pin("put raw config that is not an object", code, body)
	code, body = s.serve(http.MethodPut, "/admin/nodes/:id/raw-config", "/admin/nodes/1/raw-config", `{"raw_config":`+s.wireGuardRawConfig("not-a-key")+`}`, h.UpdateNodeRawConfig)
	s.pin("put raw config with an invalid WireGuard key", code, body)
	code, body = s.serve(http.MethodPut, "/admin/nodes/:id/raw-config", "/admin/nodes/x/raw-config", `{}`, h.UpdateNodeRawConfig)
	s.pin("put raw config, bad id", code, body)

	// The node update route carries the raw configuration too.
	code, body = s.serve(http.MethodPut, "/admin/nodes/:id", "/admin/nodes/1", `{"name":"edge-2","raw_config":"`+strings.ReplaceAll(masked, `"`, `\"`)+`"}`, h.UpdateNode)
	s.pin("update node with the placeholder", code, body)
	s.Require().NoError(s.db.First(&node, 1).Error)
	s.Equal("edge-2", node.Name)
	s.Contains(*node.RawConfig, pinRawConfigSecret)

	// Validation never stores anything.
	code, body = s.serve(http.MethodPost, "/admin/nodes/validate-config", "/admin/nodes/validate-config", `{"raw_config":{"type":"vless","server_port":443}}`, h.ValidateRawConfig)
	s.pin("validate config", code, body)
	code, body = s.serve(http.MethodPost, "/admin/nodes/validate-config", "/admin/nodes/validate-config", `{"raw_config":{"type":"vless"}}`, h.ValidateRawConfig)
	s.pin("validate config without server_port", code, body)
	code, body = s.serve(http.MethodPost, "/admin/nodes/validate-config", "/admin/nodes/validate-config", `{"raw_config":"[1]"}`, h.ValidateRawConfig)
	s.pin("validate config that is not an object", code, body)
	code, body = s.serve(http.MethodPost, "/admin/nodes/validate-config", "/admin/nodes/validate-config", `{"raw_config":`+s.wireGuardRawConfig("not-a-key")+`}`, h.ValidateRawConfig)
	s.pin("validate config with an invalid WireGuard key", code, body)
	code, body = s.serve(http.MethodPost, "/admin/nodes/validate-config", "/admin/nodes/validate-config", `{}`, h.ValidateRawConfig)
	s.pin("validate config without raw_config", code, body)

	// Deleting the node takes its protocols, their links and peers with it.
	group := &model.SubscriptionGroup{Name: "group", Enable: 1}
	s.Require().NoError(s.db.Create(group).Error)
	var protocol model.NodeProtocol
	s.Require().NoError(s.db.Where("node_id = ?", node.ID).First(&protocol).Error)
	s.Require().NoError(s.db.Model(group).Association("Protocols").Append(&protocol))
	s.Require().NoError(s.db.Create(&model.WireGuardPeer{NodeProtocolID: protocol.ID, UserID: 1, PeerIP: "10.9.0.2", PrivateKey: "peer-private", PublicKey: "peer-public"}).Error)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/:id", "/admin/nodes/1", "", h.DeleteNode)
	s.pin("delete node", code, body)
	s.Equal(int64(0), s.count(&model.Node{}, "id = ?", 1))
	s.Equal(int64(0), s.count(&model.NodeProtocol{}, "node_id = ?", 1))
	s.Equal(int64(0), s.count(&model.WireGuardPeer{}, "node_protocol_id = ?", protocol.ID))
	s.Equal(int64(0), s.count(&model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectProxy, 1))
	s.Equal(int64(0), s.count(&model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeRawConfig, 1))
	var links int64
	s.Require().NoError(s.db.Table("v2_subscription_group_node_protocols").Where("node_protocol_id = ?", protocol.ID).Count(&links).Error)
	s.Equal(int64(0), links)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/:id", "/admin/nodes/1", "", h.DeleteNode)
	s.pin("delete node again", code, body)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/:id", "/admin/nodes/x", "", h.DeleteNode)
	s.pin("delete node, bad id", code, body)
}

func (s *NodeCredentialAnswersTestSuite) TestProtocols() {
	h := NewNodeHandler()
	node := &model.Node{Name: "edge", Host: "edge.example.test", Port: 443, APIKey: "node-api-key", APIKeyHash: hashString("node-api-key"), Secret: "node-shared-secret"}
	s.Require().NoError(s.db.Create(node).Error)

	reality := `{"dest":"www.example.test:443","private_key":"` + pinRealitySecret + `","public_key":"reality-public","short_id":"6ba85179"}`
	code, body := s.serve(http.MethodPost, "/admin/nodes/:id/protocols", "/admin/nodes/1/protocols",
		`{"name":"reality","type":"vless","port":443,"enable":1,"show":1,"tls":2,"reality_settings":"`+strings.ReplaceAll(reality, `"`, `\"`)+`"}`, h.CreateProtocol)
	s.pin("create protocol", code, body)
	var protocol model.NodeProtocol
	s.Require().NoError(s.db.Where("node_id = ?", node.ID).First(&protocol).Error)
	s.Require().NotNil(protocol.RealitySettings)
	s.Contains(*protocol.RealitySettings, pinRealitySecret)
	s.Equal(int64(1), s.count(&model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeProtocol, protocol.ID))

	code, body = s.serve(http.MethodPost, "/admin/nodes/:id/protocols", "/admin/nodes/1/protocols",
		`{"name":"wg","type":"wireguard","port":51820,"enable":1,"settings":"{\"cidr\":\"10.9.0.0/24\",\"server_private_key\":\"not-a-key\"}"}`, h.CreateProtocol)
	s.pin("create protocol with an invalid WireGuard key", code, body)
	code, body = s.serve(http.MethodPost, "/admin/nodes/:id/protocols", "/admin/nodes/x/protocols", `{}`, h.CreateProtocol)
	s.pin("create protocol, bad id", code, body)

	masked := strings.ReplaceAll(reality, pinRealitySecret, service.NodeSecretPlaceholder)
	code, body = s.serve(http.MethodPut, "/admin/nodes/protocols/:protocol_id", fmt.Sprintf("/admin/nodes/protocols/%d", protocol.ID),
		`{"name":"reality-2","reality_settings":"`+strings.ReplaceAll(masked, `"`, `\"`)+`"}`, h.UpdateProtocol)
	s.pin("update protocol with the placeholder", code, body)
	s.Require().NoError(s.db.First(&protocol, protocol.ID).Error)
	s.Equal("reality-2", protocol.Name)
	s.Contains(*protocol.RealitySettings, pinRealitySecret, "the placeholder keeps the stored secret")
	code, body = s.serve(http.MethodPut, "/admin/nodes/protocols/:protocol_id", fmt.Sprintf("/admin/nodes/protocols/%d", protocol.ID),
		`{"reality_settings":"{\"dest\":\"other.example.test:443\",\"private_key\":\"new-reality-private\"}"}`, h.UpdateProtocol)
	s.pin("update protocol with a new secret", code, body)
	s.Require().NoError(s.db.First(&protocol, protocol.ID).Error)
	s.Contains(*protocol.RealitySettings, "new-reality-private")
	var secret model.ProtocolSecret
	s.Require().NoError(s.db.Where("scope = ? AND owner_id = ?", nodesecrets.ScopeNodeProtocol, protocol.ID).First(&secret).Error)
	s.Equal(`"new-reality-private"`, secret.Value, "the JSON encoding of the value")
	code, body = s.serve(http.MethodPut, "/admin/nodes/protocols/:protocol_id", fmt.Sprintf("/admin/nodes/protocols/%d", protocol.ID),
		`{"type":"wireguard","settings":"{\"cidr\":\"10.9.0.0/24\",\"server_private_key\":\"not-a-key\"}"}`, h.UpdateProtocol)
	s.pin("update protocol with an invalid WireGuard key", code, body)
	code, body = s.serve(http.MethodPut, "/admin/nodes/protocols/:protocol_id", "/admin/nodes/protocols/999", `{"name":"gone"}`, h.UpdateProtocol)
	s.pin("update protocol that does not exist", code, body)

	group := &model.SubscriptionGroup{Name: "group", Enable: 1}
	s.Require().NoError(s.db.Create(group).Error)
	s.Require().NoError(s.db.Model(group).Association("Protocols").Append(&protocol))
	s.Require().NoError(s.db.Create(&model.WireGuardPeer{NodeProtocolID: protocol.ID, UserID: 1, PeerIP: "10.9.0.2", PrivateKey: "peer-private", PublicKey: "peer-public"}).Error)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/protocols/:protocol_id", fmt.Sprintf("/admin/nodes/protocols/%d", protocol.ID), "", h.DeleteProtocol)
	s.pin("delete protocol", code, body)
	s.Equal(int64(0), s.count(&model.NodeProtocol{}, "id = ?", protocol.ID))
	s.Equal(int64(0), s.count(&model.WireGuardPeer{}, "node_protocol_id = ?", protocol.ID))
	s.Equal(int64(0), s.count(&model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeProtocol, protocol.ID))
	var links int64
	s.Require().NoError(s.db.Table("v2_subscription_group_node_protocols").Where("node_protocol_id = ?", protocol.ID).Count(&links).Error)
	s.Equal(int64(0), links)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/protocols/:protocol_id", fmt.Sprintf("/admin/nodes/protocols/%d", protocol.ID), "", h.DeleteProtocol)
	s.pin("delete protocol again", code, body)
	code, body = s.serve(http.MethodDelete, "/admin/nodes/protocols/:protocol_id", "/admin/nodes/protocols/x", "", h.DeleteProtocol)
	s.pin("delete protocol, bad id", code, body)
}

func (s *NodeCredentialAnswersTestSuite) TestRegistrationKeys() {
	h := NewNodeHandler()
	code, body := s.serve(http.MethodPost, "/admin/auth-keys", "/admin/auth-keys", `{"name":"bootstrap","expire_days":7}`, h.GenerateAuthKey)
	s.pin("generate auth key", code, body)
	var key model.AuthorizedKey
	s.Require().NoError(s.db.First(&key, 1).Error)
	s.Len(key.Key, 64)
	s.Equal(hashString(key.Key), key.KeyHash)
	s.Require().NotNil(key.ExpireAt)
	s.Equal(int64(1), s.count(&model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectRegistrationKey, key.ID))

	code, body = s.serve(http.MethodPost, "/admin/auth-keys", "/admin/auth-keys", `{"name":"forever"}`, h.GenerateAuthKey)
	s.pin("generate auth key without expiry", code, body)
	code, body = s.serve(http.MethodPost, "/admin/auth-keys", "/admin/auth-keys", `{"expire_days":-1}`, h.GenerateAuthKey)
	s.pin("generate auth key, bad body", code, body)

	code, body = s.serve(http.MethodPost, "/internal/auth-keys", "/internal/auth-keys", `{"name":"ansible","node_name":"relay-1","expire_days":1}`, h.InternalGenerateAuthKey)
	s.pin("internal generate auth key", code, body)
	code, body = s.serve(http.MethodPost, "/internal/auth-keys", "/internal/auth-keys", `{}`, h.InternalGenerateAuthKey)
	s.pin("internal generate auth key without a name", code, body)
	s.Equal(int64(3), s.count(&model.AuthorizedKey{}, "1 = 1"), "the internal route needs a name too")

	code, body = s.serve(http.MethodDelete, "/admin/auth-keys/:id", "/admin/auth-keys/1", "", h.DeleteAuthKey)
	s.pin("delete auth key", code, body)
	s.Equal(int64(0), s.count(&model.AuthorizedKey{}, "id = ?", 1))
	s.Equal(int64(0), s.count(&model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectRegistrationKey, 1))
	code, body = s.serve(http.MethodDelete, "/admin/auth-keys/:id", "/admin/auth-keys/1", "", h.DeleteAuthKey)
	s.pin("delete auth key again", code, body)
	code, body = s.serve(http.MethodDelete, "/admin/auth-keys/:id", "/admin/auth-keys/x", "", h.DeleteAuthKey)
	s.pin("delete auth key, bad id", code, body)
}

func (s *NodeCredentialAnswersTestSuite) TestCleanAgents() {
	h := NewForwardCleanAgentHandler()
	s.Require().NoError(s.db.Create(&model.ForwardNode{ID: 10, Name: "relay", Host: "198.51.100.10", Port: 443}).Error)
	code, body := s.serve(http.MethodPost, "/admin/forward/agents", "/admin/forward/agents", `{"name":"relay-agent","nodeId":10}`, h.CreateAgentToken)
	s.pin("create clean agent", code, body)
	var agent model.ForwardCleanAgent
	s.Require().NoError(s.db.First(&agent, 1).Error)
	s.True(strings.HasPrefix(agent.Token, "v2fa_"))
	s.Equal(int64(1), s.count(&model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectCleanAgent, agent.ID))

	code, body = s.serve(http.MethodPost, "/admin/forward/agents", "/admin/forward/agents", `{"nodeId":10}`, h.CreateAgentToken)
	s.pin("create clean agent without a name", code, body)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents", "/admin/forward/agents", `{"name":"x"}`, h.CreateAgentToken)
	s.pin("create clean agent without a node", code, body)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents", "/admin/forward/agents", `{"name":"x","nodeId":99}`, h.CreateAgentToken)
	s.pin("create clean agent for a missing node", code, body)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents", "/admin/forward/agents", `[`, h.CreateAgentToken)
	s.pin("create clean agent, bad body", code, body)

	code, body = s.serve(http.MethodPost, "/admin/forward/agents/:id/revoke", "/admin/forward/agents/1/revoke", "", h.RevokeAgent)
	s.pin("revoke clean agent", code, body)
	s.Require().NoError(s.db.First(&agent, 1).Error)
	s.Equal(model.ForwardCleanAgentStatusRevoked, agent.Status)
	s.NotNil(agent.RevokedAt)
	var credential model.NodeCredential
	s.Require().NoError(s.db.Where("subject_kind = ? AND subject_id = ?", nodesecrets.SubjectCleanAgent, agent.ID).First(&credential).Error)
	s.Equal(nodesecrets.StatusRevoked, credential.Status)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents/:id/revoke", "/admin/forward/agents/1/revoke", "", h.RevokeAgent)
	s.pin("revoke clean agent again", code, body)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents/:id/revoke", "/admin/forward/agents/99/revoke", "", h.RevokeAgent)
	s.pin("revoke clean agent that does not exist", code, body)
	code, body = s.serve(http.MethodPost, "/admin/forward/agents/:id/revoke", "/admin/forward/agents/0/revoke", "", h.RevokeAgent)
	s.pin("revoke clean agent, bad id", code, body)
}

func (s *NodeCredentialAnswersTestSuite) count(value any, query string, args ...any) int64 {
	s.T().Helper()
	var n int64
	s.Require().NoError(s.db.Model(value).Where(query, args...).Count(&n).Error)
	return n
}
