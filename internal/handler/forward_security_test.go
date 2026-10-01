package handler

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// POST /api/v2/user/forward/rules let any user create a legacy rule on any
// relay and exit node, to any target. Only an administrator creates one now;
// a user still lists their own rules.
func (s *ForwardSecurityTestSuite) TestOnlyAdministratorsCreateLegacyRules() {
	create := NewForwardHandler().CreateUserRule
	body := fmt.Sprintf(`{"name":"mine","relay_node_id":%d,"exit_node_id":%d,"protocol":"tcp","target_host":"10.0.0.1","target_port":22}`, s.relay.ID, s.exit.ID)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"a rule":        s.serve("POST", "/user/forward/rules", "/user/forward/rules", body, nil, s.user.ID, false, create),
		"an empty body": s.serve("POST", "/user/forward/rules", "/user/forward/rules", "", nil, s.user.ID, false, create),
	} {
		s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
		resp := decodePanelTestResponse(s.T(), w)
		s.Equal(float64(-1), resp["code"], name)
		s.Equal("only administrators can create or change legacy forward rules; forward through your tunnels instead", resp["msg"], name)
	}
	var count int64
	s.Require().NoError(s.db.Model(&model.ForwardRule{}).Count(&count).Error)
	s.Zero(count, "a user's rule was stored")

	// An administrator creates one, and the user lists theirs.
	w := s.serve("POST", "/user/forward/rules", "/user/forward/rules", body, nil, 999, true, create)
	s.Require().Equal(float64(0), decodePanelTestResponse(s.T(), w)["code"], w.Body.String())
	s.createRule(&s.user.ID, 20001)
	w = s.serve("GET", "/user/forward/rules", "/user/forward/rules", "", nil, s.user.ID, false, NewForwardHandler().GetUserRules)
	resp := decodePanelTestResponse(s.T(), w)
	s.Require().Equal(float64(0), resp["code"], w.Body.String())
	s.Len(resp["data"].([]any), 1)
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

// listen accepts connections on a loopback port and counts them.
func (s *ForwardSecurityTestSuite) listen() (string, func() int) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	accepted := make(chan struct{}, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			_ = conn.Close()
		}
	}()
	s.T().Cleanup(func() { _ = listener.Close() })
	return listener.Addr().String(), func() int {
		time.Sleep(50 * time.Millisecond)
		return len(accepted)
	}
}

func (s *ForwardSecurityTestSuite) forwardTo(owner *model.User, remoteAddr string) *model.Forward {
	tunnel := &model.ForwardTunnel{Name: "tunnel-" + remoteAddr, InNodeID: s.relay.ID, OutNodeID: &s.relay.ID, InIP: s.relay.Host, Type: 1, Flow: 2, TrafficRatio: 1, Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(s.db.Create(tunnel).Error)
	forward := &model.Forward{UserID: owner.ID, UserName: owner.Email, Name: "fwd", TunnelID: tunnel.ID, InPort: 30000, RemoteAddr: remoteAddr, Status: model.ForwardStatusPaused}
	s.Require().NoError(s.db.Create(forward).Error)
	return forward
}

func (s *ForwardSecurityTestSuite) diagnose(forward *model.Forward, actor uint, admin bool) []any {
	w := s.serve("POST", "/forward/diagnose", "/forward/diagnose", fmt.Sprintf(`{"forwardId":%d}`, forward.ID), nil, actor, admin, NewForwardHandler().DiagnosePanelForward)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	resp := decodePanelTestResponse(s.T(), w)
	s.Require().Equal(float64(0), resp["code"], w.Body.String())
	return resp["data"].(map[string]any)["results"].([]any)
}

// A user's forward diagnosis connected from Control to the forward's
// target, which the user chooses: a loopback or private target probed
// Control's own network and showed which ports were open.
func (s *ForwardSecurityTestSuite) TestUserDiagnosisDoesNotProbeControlsNetwork() {
	address, connections := s.listen()
	_, port, err := net.SplitHostPort(address)
	s.Require().NoError(err)
	forward := s.forwardTo(s.user, strings.Join([]string{
		address, "localhost:" + port, "[::ffff:127.0.0.1]:" + port, "10.0.0.1:" + port, "169.254.169.254:80",
	}, ","))

	results := s.diagnose(forward, s.user.ID, false)
	s.Require().Len(results, 5)
	for _, raw := range results {
		result := raw.(map[string]any)
		s.Equal(false, result["success"], "%v", result)
		s.Equal("不能诊断内网或本机地址", result["message"], "%v", result)
	}
	s.Zero(connections(), "the user's diagnosis connected to a loopback service")

	// An administrator's diagnosis still connects to the target.
	only := s.forwardTo(s.user, address)
	results = s.diagnose(only, 1, true)
	s.Require().Len(results, 1)
	s.Equal(true, results[0].(map[string]any)["success"], "%v", results[0])
	s.Equal(1, connections())
}

func TestForwardSecurity(t *testing.T) {
	suite.Run(t, new(ForwardSecurityTestSuite))
}
