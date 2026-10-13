package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodegroup"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type nodeGroupsTestEnv struct {
	*installTestEnv
	groups *nodegroup.Service
	kek    string
	router *gin.Engine
}

func newNodeGroupsTestEnv(t *testing.T) *nodeGroupsTestEnv {
	t.Helper()
	install := newInstallTestEnv(t)
	db := install.db
	require.NoError(t, db.AutoMigrate(append(model.KernelModels(), &model.SubscriptionGroup{}, &model.NodeProtocol{})...))
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	env := &nodeGroupsTestEnv{installTestEnv: install, kek: base64.StdEncoding.EncodeToString(raw)}
	env.groups = &nodegroup.Service{DB: db, KEK: func() string { return env.kek }, PKI: install.pki}
	handler := &NodeGroupsHandler{
		groups: func() *nodegroup.Service { return env.groups },
		db:     func() *gorm.DB { return db },
		install: &AgentInstallHandler{
			pki:    func() (*agentpki.Service, error) { return install.pki, nil },
			config: func() *config.Config { return install.cfg },
			db:     func() *gorm.DB { return db },
		},
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID := uint(7)
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 32)
			userID = uint(parsed)
		}
		c.Set("user_id", userID)
		c.Set("email", "root@example.com")
	})
	router.GET("/node-groups", handler.List)
	router.POST("/node-groups", handler.Create)
	router.GET("/node-groups/:id", handler.Get)
	router.PUT("/node-groups/:id", handler.Update)
	router.DELETE("/node-groups/:id", handler.Delete)
	router.GET("/node-groups/:id/command", handler.Command)
	router.GET("/node-groups/:id/nodes", handler.Nodes)
	router.POST("/node-groups/default", handler.EnsureDefault)
	router.GET("/install-readiness", handler.install.Readiness)
	router.POST("/node-groups/:id/reset-token", handler.ResetToken)
	router.POST("/nodes/:id/approve", handler.ApproveNode)
	router.POST("/nodes/:id/reject", handler.RejectNode)
	env.router = router
	return env
}

func (e *nodeGroupsTestEnv) call(t *testing.T, user uint, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", strconv.FormatUint(uint64(user), 10))
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func decodeData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	return body.Data
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	return body.Error.Code
}

func TestNodeGroupsAPI(t *testing.T) {
	env := newNodeGroupsTestEnv(t)

	created := env.call(t, 7, http.MethodPost, "/node-groups", `{
		"name": "HK", "kind": "proxy", "require_approval": true, "max_nodes": 5, "rate": 1.5, "tags": ["hk"],
		"protocol_template": [{"name": "VLESS Reality", "type": "vless", "port": 443, "tls": "reality",
			"reality": {"server_name": "www.example.com"}, "settings": {"flow": "xtls-rprx-vision"}}]
	}`)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	assert.NotContains(t, created.Body.String(), "anixjoin_", "only the command shows the token")
	group := decodeData[nodegroup.GroupView](t, created)
	assert.Equal(t, "HK", group.Name)
	assert.True(t, group.RequireApproval)
	require.Len(t, group.ProtocolTemplate, 1)
	assert.Equal(t, "www.example.com:443", group.ProtocolTemplate[0].Reality.Dest)
	id := strconv.FormatUint(uint64(group.ID), 10)

	for body, code := range map[string]string{
		`{"name": "HK", "kind": "forward"}`: "name_taken",
		`{"name": "x", "kind": "relay"}`:    "invalid_request",
		`{"name": "x", "kind": "proxy", "protocol_template": [{"type": "vless", "port": 443, "tls": "acme"}]}`: "invalid_request",
		`{"name": 1}`: "invalid_request",
	} {
		response := env.call(t, 7, http.MethodPost, "/node-groups", body)
		assert.Equal(t, code, errorCode(t, response), body)
	}

	listed := decodeData[struct {
		Groups []nodegroup.GroupView `json:"groups"`
	}](t, env.call(t, 7, http.MethodGet, "/node-groups", ""))
	require.Len(t, listed.Groups, 1)
	assert.Equal(t, group.ID, listed.Groups[0].ID)
	assert.Equal(t, http.StatusOK, env.call(t, 8, http.MethodGet, "/node-groups/"+id, "").Code, "any administrator reads groups")
	assert.Equal(t, http.StatusNotFound, env.call(t, 7, http.MethodGet, "/node-groups/4242", "").Code)
	assert.Equal(t, http.StatusBadRequest, env.call(t, 7, http.MethodGet, "/node-groups/x", "").Code)

	updated := env.call(t, 7, http.MethodPut, "/node-groups/"+id, `{"name": "HK 1", "require_approval": true}`)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Equal(t, "HK 1", decodeData[nodegroup.GroupView](t, updated).Name)
	assert.Equal(t, "invalid_request", errorCode(t, env.call(t, 7, http.MethodPut, "/node-groups/"+id, `{"name": "HK 1", "kind": "forward"}`)))

	// The command: super administrators only, audited, never cached.
	for _, user := range []uint{8, 9, 10} {
		response := env.call(t, user, http.MethodGet, "/node-groups/"+id+"/command", "")
		assert.Equal(t, http.StatusForbidden, response.Code, "user %d", user)
		assert.Equal(t, "super_admin_required", errorCode(t, response))
	}
	commandResponse := env.call(t, 7, http.MethodGet, "/node-groups/"+id+"/command", "")
	require.Equal(t, http.StatusOK, commandResponse.Code, commandResponse.Body.String())
	assert.Equal(t, "no-store", commandResponse.Header().Get("Cache-Control"))
	command := decodeData[nodeGroupCommandResponse](t, commandResponse)
	assert.True(t, nodegroup.ValidToken(command.Token))
	assert.Equal(t, "v4.2.0", command.AgentVersion)
	assert.Equal(t, "sudo anix-agent uninstall", command.UninstallCommand)
	require.Len(t, command.Commands, 3)
	assert.Equal(t, agentinstall.MirrorControl, command.Commands[0].Mirror)
	assert.Equal(t, "curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- -u https://ctl.example.com -t "+command.Token, command.Commands[0].Command)
	assert.True(t, strings.HasSuffix(command.Commands[2].Command, " -t "+command.Token+" --mirror github"))
	assert.Equal(t, "https://ctl.example.com/install.sh", command.Script.URL)
	var reveals []model.OperationLog
	require.NoError(t, env.db.Where("action = ?", nodegroup.AuditActionTokenReveal).Find(&reveals).Error)
	require.Len(t, reveals, 1, "a refused read reveals nothing")
	assert.Equal(t, "root@example.com", reveals[0].Username)
	assert.NotContains(t, reveals[0].Content, command.Token)

	// An unconfigured Control shows no command (and reveals nothing).
	env.cfg.AgentInstall.PublicURL = ""
	unconfigured := env.call(t, 7, http.MethodGet, "/node-groups/"+id+"/command", "")
	assert.Equal(t, "agent_install_unconfigured", errorCode(t, unconfigured))
	env.cfg.AgentInstall.PublicURL = "https://ctl.example.com"

	// Reset: super administrators only; the old token stops joining.
	assert.Equal(t, http.StatusForbidden, env.call(t, 8, http.MethodPost, "/node-groups/"+id+"/reset-token", "").Code)
	reset := env.call(t, 7, http.MethodPost, "/node-groups/"+id+"/reset-token", "")
	require.Equal(t, http.StatusOK, reset.Code, reset.Body.String())
	fresh := decodeData[nodeGroupCommandResponse](t, env.call(t, 7, http.MethodGet, "/node-groups/"+id+"/command", ""))
	assert.NotEqual(t, command.Token, fresh.Token)
	_, err := env.groups.Join(t.Context(), nodegroup.JoinRequest{Token: command.Token, CSRDER: testCSR(t), PublicAddresses: []string{"203.0.113.1"}})
	assert.ErrorIs(t, err, nodegroup.ErrJoinRejected)

	// Approve and reject pending nodes.
	pending, err := env.groups.Join(t.Context(), nodegroup.JoinRequest{Token: fresh.Token, CSRDER: testCSR(t), Hostname: "a", PublicAddresses: []string{"203.0.113.1"}})
	require.NoError(t, err)
	other, err := env.groups.Join(t.Context(), nodegroup.JoinRequest{Token: fresh.Token, CSRDER: testCSR(t), Hostname: "b", PublicAddresses: []string{"203.0.113.2"}})
	require.NoError(t, err)
	approved := env.call(t, 8, http.MethodPost, "/nodes/"+pending.Node.String()+"/approve", "")
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	assert.Equal(t, nodegroup.NodeDecision{Node: pending.Node.String(), Name: "a", Approval: model.NodeApprovalApproved}, decodeData[nodegroup.NodeDecision](t, approved))
	assert.Equal(t, "node_not_pending", errorCode(t, env.call(t, 7, http.MethodPost, "/nodes/"+pending.Node.String()+"/approve", "")))
	assert.Equal(t, "invalid_request", errorCode(t, env.call(t, 7, http.MethodPost, "/nodes/module-1/approve", "")))
	assert.Equal(t, "node_not_found", errorCode(t, env.call(t, 7, http.MethodPost, "/nodes/proxy-4242/reject", "")))
	rejected := env.call(t, 7, http.MethodPost, "/nodes/"+other.Node.String()+"/reject", "")
	require.Equal(t, http.StatusOK, rejected.Code, rejected.Body.String())
	assert.Equal(t, model.NodeApprovalRejected, decodeData[nodegroup.NodeDecision](t, rejected).Approval)

	// A group with live nodes is not deleted.
	assert.Equal(t, "node_group_in_use", errorCode(t, env.call(t, 7, http.MethodDelete, "/node-groups/"+id, "")))
	empty := decodeData[nodegroup.GroupView](t, env.call(t, 7, http.MethodPost, "/node-groups", `{"name": "edge", "kind": "forward"}`))
	deleted := env.call(t, 7, http.MethodDelete, "/node-groups/"+strconv.FormatUint(uint64(empty.ID), 10), "")
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	assert.Equal(t, http.StatusNotFound, env.call(t, 7, http.MethodGet, "/node-groups/"+strconv.FormatUint(uint64(empty.ID), 10), "").Code)

	// Without the key-encryption key no group is created.
	env.kek = ""
	assert.Equal(t, "ca_kek_missing", errorCode(t, env.call(t, 7, http.MethodPost, "/node-groups", `{"name": "y", "kind": "proxy"}`)))
}

func testCSR(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	require.NoError(t, err)
	return der
}

// The join dialog of an empty panel asks for the default group, then waits
// on the group's nodes.
func TestNodeGroupsDefaultAndNodes(t *testing.T) {
	env := newNodeGroupsTestEnv(t)

	// Without the key-encryption key nothing is created.
	env.kek = ""
	response := env.call(t, 8, http.MethodPost, "/node-groups/default", "")
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "ca_kek_missing", errorCode(t, response))
	var subscriptions int64
	require.NoError(t, env.db.Model(&model.SubscriptionGroup{}).Count(&subscriptions).Error)
	assert.Zero(t, subscriptions)
	env.kek = base64.StdEncoding.EncodeToString(make([]byte, 32))

	// Any administrator may ask: the first call creates the group and the
	// default subscription group, the second answers the same group.
	created := env.call(t, 8, http.MethodPost, "/node-groups/default", "")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	first := decodeData[struct {
		Group   nodegroup.GroupView `json:"group"`
		Created bool                `json:"created"`
	}](t, created)
	assert.True(t, first.Created)
	assert.Equal(t, nodegroup.DefaultGroupName, first.Group.Name)
	assert.Equal(t, model.NodeGroupKindProxy, first.Group.Kind)
	require.Len(t, first.Group.ProtocolTemplate, 1)
	assert.Equal(t, "reality", first.Group.ProtocolTemplate[0].TLS)
	require.Len(t, first.Group.SubscriptionGroupIDs, 1)
	assert.NotContains(t, created.Body.String(), "anixjoin_")

	again := env.call(t, 8, http.MethodPost, "/node-groups/default", "")
	require.Equal(t, http.StatusOK, again.Code, again.Body.String())
	second := decodeData[struct {
		Group   nodegroup.GroupView `json:"group"`
		Created bool                `json:"created"`
	}](t, again)
	assert.False(t, second.Created)
	assert.Equal(t, first.Group.ID, second.Group.ID)

	// The group's nodes: none yet, then the machines that joined, with the
	// pending one marked. Never a credential.
	id := strconv.FormatUint(uint64(first.Group.ID), 10)
	waiting := env.call(t, 8, http.MethodGet, "/node-groups/"+id+"/nodes", "")
	require.Equal(t, http.StatusOK, waiting.Code, waiting.Body.String())
	assert.JSONEq(t, `{"data":{"nodes":[]}}`, waiting.Body.String())
	assert.Equal(t, "no-store", waiting.Header().Get("Cache-Control"))

	command := decodeData[nodeGroupCommandResponse](t, env.call(t, 7, http.MethodGet, "/node-groups/"+id+"/command", ""))
	joined, err := env.groups.Join(t.Context(), nodegroup.JoinRequest{
		Token: command.Token, CSRDER: testCSR(t), RequestedName: "hk-01", PublicAddresses: []string{"203.0.113.5"}, AgentVersion: "v4.2.0",
	})
	require.NoError(t, err)
	listed := env.call(t, 8, http.MethodGet, "/node-groups/"+id+"/nodes", "")
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	nodes := decodeData[struct {
		Nodes []nodegroup.GroupNode `json:"nodes"`
	}](t, listed).Nodes
	require.Len(t, nodes, 1)
	assert.Equal(t, joined.Node.String(), nodes[0].Node)
	assert.Equal(t, "hk-01", nodes[0].Name)
	assert.Equal(t, "203.0.113.5", nodes[0].Host)
	assert.Equal(t, model.NodeApprovalApproved, nodes[0].Approval)
	assert.False(t, nodes[0].Online)
	assert.Equal(t, "v4.2.0", nodes[0].AgentVersion)
	for _, secret := range []string{"anixjoin_", "api_key", "secret", command.Token} {
		assert.NotContains(t, listed.Body.String(), secret)
	}

	assert.Equal(t, http.StatusNotFound, env.call(t, 8, http.MethodGet, "/node-groups/4242/nodes", "").Code)
	assert.Equal(t, http.StatusBadRequest, env.call(t, 8, http.MethodGet, "/node-groups/x/nodes", "").Code)
}

// The readiness answer lists every check with its fix; a Control that
// cannot hand out a working command says why instead.
func TestInstallReadinessAnswer(t *testing.T) {
	env := newNodeGroupsTestEnv(t)
	type answer struct {
		Ready  bool `json:"ready"`
		Checks []struct {
			ID      string `json:"id"`
			OK      bool   `json:"ok"`
			Level   string `json:"level"`
			Message string `json:"message"`
			Fix     string `json:"fix"`
		} `json:"checks"`
	}
	read := func() answer {
		response := env.call(t, 8, http.MethodGet, "/install-readiness", "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		return decodeData[answer](t, response)
	}

	// The test Control has a public https URL and an agent version, and no
	// gRPC listener or CA key.
	result := read()
	assert.False(t, result.Ready)
	byID := map[string]struct {
		ok  bool
		fix string
	}{}
	for _, check := range result.Checks {
		byID[check.ID] = struct {
			ok  bool
			fix string
		}{check.OK, check.Fix}
	}
	assert.True(t, byID["public_url"].ok)
	assert.False(t, byID["grpc"].ok)
	assert.Contains(t, byID["grpc"].fix, "grpc.enabled")
	assert.False(t, byID["ca"].ok)
	assert.Contains(t, byID["ca"].fix, "module_runtime.ca_kek")
	assert.True(t, byID["mtls_mode"].ok)
	assert.True(t, byID["agent_release"].ok)
	assert.True(t, byID["script_signature"].ok)

	// Configured, it is ready; a key that is not 32 bytes is a problem.
	env.cfg.GRPC.Enable = true
	env.cfg.GRPC.TLSCertFile, env.cfg.GRPC.TLSKeyFile = "/etc/anix/grpc.crt", "/etc/anix/grpc.key"
	env.cfg.ModuleRuntime.CAKEK = "short"
	result = read()
	assert.False(t, result.Ready)
	for _, check := range result.Checks {
		if check.ID == "ca" {
			assert.False(t, check.OK)
			assert.Contains(t, check.Message, "cannot be used")
		}
	}
	env.cfg.ModuleRuntime.CAKEK = env.kek
	result = read()
	assert.True(t, result.Ready, "%+v", result.Checks)
	for _, check := range result.Checks {
		assert.True(t, check.OK, check.ID+": "+check.Message)
		assert.NotContains(t, check.Message+check.Fix, env.kek, "the key is never echoed")
	}

	// Without a public URL the request's own address stands in, and a
	// plain-http one is refused.
	env.cfg.AgentInstall.PublicURL = ""
	result = read()
	assert.False(t, result.Ready)
	assert.False(t, result.Checks[0].OK)
	assert.Contains(t, result.Checks[0].Message, "not https")
}
