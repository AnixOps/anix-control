package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// ForwardSecurityTestSuite holds the regression tests for forward routes
// that showed forward node credentials to users, served a node's rules
// without authentication, or probed Control's own network for a user.
type ForwardSecurityTestSuite struct {
	HandlerTestSuite
	relay *model.ForwardNode
	exit  *model.ForwardNode
	user  *model.User
	other *model.User
}

func (s *ForwardSecurityTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.relay = &model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-secret-token", Enabled: true}
	s.exit = &model.ForwardNode{Name: "exit", Type: model.ForwardNodeTypeExit, Host: "198.51.100.20", Port: 443, APIPort: 18081, APIToken: "exit-secret-token", Enabled: true}
	s.Require().NoError(s.db.Create(s.relay).Error)
	s.Require().NoError(s.db.Create(s.exit).Error)
	s.user = &model.User{Email: "user@example.test", Token: "user-sub-token", UUID: "user-uuid", TransferEnable: 1 << 40}
	s.other = &model.User{Email: "other@example.test", Token: "other-sub-token", UUID: "other-uuid", TransferEnable: 1 << 40}
	s.Require().NoError(s.db.Create(s.user).Error)
	s.Require().NoError(s.db.Create(s.other).Error)
}

func (s *ForwardSecurityTestSuite) serve(method, pattern, path, body string, headers map[string]string, actor uint, admin bool, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	router := gin.New()
	router.Handle(method, pattern, func(c *gin.Context) {
		c.Set("user_id", actor)
		c.Set("is_admin", admin)
		handler(c)
	})
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func (s *ForwardSecurityTestSuite) createRule(owner *uint, listenPort int) {
	s.Require().NoError(s.db.Create(&model.ForwardRule{
		Name: fmt.Sprintf("rule-%d", listenPort), Enabled: true, RelayNodeID: s.relay.ID, ExitNodeID: s.exit.ID,
		ListenPort: listenPort, Protocol: "tcp", TargetHost: "203.0.113.5", TargetPort: 8443, UserID: owner,
	}).Error)
}

// A user's rules showed their relay and exit nodes with the nodes' API
// tokens, which authenticate the nodes' agents.
func (s *ForwardSecurityTestSuite) TestUserRulesHideNodeTokens() {
	s.createRule(&s.user.ID, 20001)
	w := s.serve("GET", "/user/forward/rules", "/user/forward/rules", "", nil, s.user.ID, false, NewForwardHandler().GetUserRules)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	s.NotContains(w.Body.String(), "relay-secret-token")
	s.NotContains(w.Body.String(), "exit-secret-token")
	resp := decodePanelTestResponse(s.T(), w)
	rules := resp["data"].([]any)
	s.Require().Len(rules, 1)
	rule := rules[0].(map[string]any)
	for _, key := range []string{"relay_node", "exit_node"} {
		node := rule[key].(map[string]any)
		s.Equal("", node["api_token"], key)
		s.NotEmpty(node["host"], key)
	}

	// The nodes keep their tokens.
	var relay model.ForwardNode
	s.Require().NoError(s.db.First(&relay, s.relay.ID).Error)
	s.Equal("relay-secret-token", relay.APIToken)
}

// GET /api/v2/forward/agent/rules is a public route and answered anyone with
// the rules of any node: every user's listen ports and targets.
func (s *ForwardSecurityTestSuite) TestAgentRulesNeedTheForwardNodeToken() {
	s.createRule(&s.user.ID, 20001)
	s.createRule(&s.other.ID, 20002)
	handler := NewAgentHandler().AgentGetForwardRules
	path := fmt.Sprintf("/api/v2/forward/agent/rules?node_id=%d", s.relay.ID)
	get := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		return s.serve("GET", "/api/v2/forward/agent/rules", path, "", headers, 0, false, handler)
	}

	for name, w := range map[string]*httptest.ResponseRecorder{
		"no token":                 get(path, nil),
		"wrong token":              get(path, map[string]string{"X-API-Key": "wrong"}),
		"another node's token":     get(path, map[string]string{"X-API-Key": "exit-secret-token"}),
		"unknown node":             get("/api/v2/forward/agent/rules?node_id=999&token=relay-secret-token", nil),
		"no node":                  get("/api/v2/forward/agent/rules?token=relay-secret-token", nil),
		"a node id that is a word": get("/api/v2/forward/agent/rules?node_id=x&token=relay-secret-token", nil),
	} {
		s.Equal(http.StatusUnauthorized, w.Code, "%s: %s", name, w.Body.String())
		s.NotContains(w.Body.String(), "203.0.113.5", name)
	}

	for name, w := range map[string]*httptest.ResponseRecorder{
		"header token": get(path, map[string]string{"X-API-Key": "relay-secret-token"}),
		"query token":  get(path+"&token=relay-secret-token", nil),
		"api_key":      get(path+"&api_key=relay-secret-token", nil),
		"node header": get("/api/v2/forward/agent/rules", map[string]string{
			"X-Node-ID": fmt.Sprint(s.relay.ID), "X-API-Key": "relay-secret-token",
		}),
	} {
		s.Equal(http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
		s.Equal(2, strings.Count(w.Body.String(), "203.0.113.5"), name)
	}

	// A proxy node's credentials are not a forward node's.
	proxy := &model.Node{ID: 50, Name: "proxy", Host: "198.51.100.30", APIKey: "proxy-key", APIKeyHash: hashString("proxy-key")}
	s.Require().NoError(s.db.Create(proxy).Error)
	w := get("/api/v2/forward/agent/rules?node_id=50", map[string]string{"X-API-Key": "proxy-key"})
	s.Equal(http.StatusForbidden, w.Code, w.Body.String())
	s.NotContains(w.Body.String(), "203.0.113.5")
}

// A clean agent's token registered under any node id and then claimed that
// node's jobs. The token stays bound to its node: another node is 403.
func (s *ForwardSecurityTestSuite) TestCleanAgentCannotRegisterUnderAnotherNode() {
	s.Require().NoError(s.db.AutoMigrate(&model.ForwardCleanAgent{}))
	s.Require().NoError(s.db.Exec("DELETE FROM v2_forward_clean_agent").Error)
	agent := &model.ForwardCleanAgent{Name: "relay-agent", NodeID: &s.relay.ID, Token: "v2fa_relay-agent-token"}
	s.Require().NoError(s.db.Create(agent).Error)
	register := NewForwardCleanAgentHandler().Register
	post := func(body string) *httptest.ResponseRecorder {
		return s.serve("POST", "/forward-agent/register", "/forward-agent/register", body, map[string]string{"X-Agent-Token": agent.Token}, 0, false, register)
	}

	w := post(fmt.Sprintf(`{"nodeId":%d,"hostname":"elsewhere"}`, s.exit.ID))
	s.Equal(http.StatusForbidden, w.Code, w.Body.String())
	s.JSONEq(`{"code":-1,"msg":"agent is bound to another node","data":null}`, w.Body.String())
	var stored model.ForwardCleanAgent
	s.Require().NoError(s.db.First(&stored, agent.ID).Error)
	s.Equal(s.relay.ID, *stored.NodeID)
	s.Empty(stored.Hostname)

	w = post(fmt.Sprintf(`{"nodeId":%d}`, s.relay.ID))
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	s.Contains(w.Body.String(), fmt.Sprintf(`"nodeId":%d`, s.relay.ID))
}

func TestForwardSecurity(t *testing.T) {
	suite.Run(t, new(ForwardSecurityTestSuite))
}
