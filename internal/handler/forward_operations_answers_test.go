package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// ForwardOperationsAnswersTestSuite pins the bytes the legacy forward
// operation routes answer, and what they send NodeX: panel forward changes,
// a tunnel update, the backend sync, the tunnel permission removal and the
// legacy rules. The KernelNodeOps forward executors (NO-7) share these
// routes' implementation; the answers stay what they were before the routes
// moved onto it. Only the values that change from one call to the next are
// normalized (normalizeOperationAnswer).
type ForwardOperationsAnswersTestSuite struct {
	HandlerTestSuite
	nodex *fakeNodeX
}

func TestForwardOperationsAnswers(t *testing.T) {
	suite.Run(t, new(ForwardOperationsAnswersTestSuite))
}

var operationVolatile = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`"ts":\d+`), `"ts":0`},
	{regexp.MustCompile(`"createdTime":\d+`), `"createdTime":0`},
	{regexp.MustCompile(`"updatedTime":\d+`), `"updatedTime":0`},
	{regexp.MustCompile(`"lastRuntimeSyncTime":\d+`), `"lastRuntimeSyncTime":0`},
	{regexp.MustCompile(`"created_at":"[^"]*"`), `"created_at":"-"`},
	{regexp.MustCompile(`"updated_at":"[^"]*"`), `"updated_at":"-"`},
	{regexp.MustCompile(`"last_check":"[^"]*"`), `"last_check":"-"`},
}

// normalizeOperationAnswer replaces the clock readings of an answer.
func normalizeOperationAnswer(body string) string {
	for _, volatile := range operationVolatile {
		body = volatile.pattern.ReplaceAllString(body, volatile.with)
	}
	return body
}

// fakeNodeX is the NodeX control plane: it records every execute request
// and answers success, except for a forward or rule named "failing", which
// it refuses.
type fakeNodeX struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
	auth   []string
}

func newFakeNodeX(t *testing.T) *fakeNodeX {
	t.Helper()
	n := &fakeNodeX{}
	n.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.bodies = append(n.bodies, string(body))
		n.auth = append(n.auth, r.Header.Get("Authorization"))
		n.mu.Unlock()
		if r.URL.Path != "/api/v2/internal/forward/runtime/execute" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(string(body), `"name":"failing"`) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"nodex refused the change"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"nodex applied","result":"applied"}}`))
	}))
	t.Cleanup(n.server.Close)
	return n
}

// received answers the request bodies NodeX got, decoded and re-encoded
// with sorted keys, and forgets them.
func (n *fakeNodeX) received(t *testing.T) []string {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, 0, len(n.bodies))
	for _, body := range n.bodies {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			t.Fatalf("NodeX received a body that is not JSON: %s", body)
		}
		encoded, err := json.Marshal(decoded)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(encoded))
	}
	n.bodies = nil
	return out
}

func (s *ForwardOperationsAnswersTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.nodex = newFakeNodeX(s.T())
	configs := service.NewSystemConfigService(s.db)
	for key, value := range map[string]string{
		"forward.runtime_backend":        model.ForwardRuntimeBackendGost,
		"forward.runtime.nodex.base_url": s.nodex.server.URL,
		"forward.runtime.nodex.token":    "nodex-shared-token",
	} {
		s.Require().NoError(configs.Set(key, value, "string", "forward", "forward operations answers"))
	}
}

func (s *ForwardOperationsAnswersTestSuite) serve(method, pattern, path, body string, actor uint, admin bool, handler gin.HandlerFunc) string {
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
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	return normalizeOperationAnswer(w.Body.String())
}

func (s *ForwardOperationsAnswersTestSuite) node(node model.ForwardNode) *model.ForwardNode {
	s.Require().NoError(service.NewForwardNodeService(s.db).Create(&node))
	return &node
}

func (s *ForwardOperationsAnswersTestSuite) user(email string) *model.User {
	user := &model.User{Email: email, Token: email + "-token", UUID: email + "-uuid", TransferEnable: 1 << 40}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *ForwardOperationsAnswersTestSuite) tunnel(name string, in *model.ForwardNode, out *model.ForwardNode) *model.ForwardTunnel {
	tunnel := &model.ForwardTunnel{Name: name, InNodeID: in.ID, InIP: in.Host, Type: 1, Flow: 2, TrafficRatio: 1, Protocol: "tcp",
		TCPListenAddr: "0.0.0.0", UDPListenAddr: "0.0.0.0", Status: model.ForwardTunnelStatusActive}
	if out != nil {
		tunnel.OutNodeID = &out.ID
		tunnel.OutIP = out.Host
		tunnel.Type = 2
	}
	s.Require().NoError(s.db.Create(tunnel).Error)
	return tunnel
}

func (s *ForwardOperationsAnswersTestSuite) job(forwardID uint) model.ForwardRuntimeJob {
	var jobs []model.ForwardRuntimeJob
	s.Require().NoError(s.db.Where("forward_id = ?", forwardID).Order("id DESC").Limit(1).Find(&jobs).Error)
	s.Require().Len(jobs, 1, "the change recorded a runtime job")
	return jobs[0]
}

// rule loads the rule an answer names, without its nodes.
func (s *ForwardOperationsAnswersTestSuite) rule(answer string) model.ForwardRule {
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal([]byte(answer), &created))
	var rule model.ForwardRule
	s.Require().NoError(s.db.First(&rule, created.Data.ID).Error)
	return rule
}

func (s *ForwardOperationsAnswersTestSuite) forward(id uint) model.Forward {
	var record model.Forward
	s.Require().NoError(s.db.First(&record, id).Error)
	return record
}

func panelForwardItem(id, tunnelID uint, name, host string, port int, remote, strategy string, status, runtimeStatus int, runtimeMessage string, user *model.User) string {
	return fmt.Sprintf(`{"code":0,"data":{"id":%d,"name":%q,"tunnelId":%d,"tunnelName":"tunnel","inIp":%q,"inPort":%d,"remoteAddr":%q,"interfaceName":"","strategy":%q,"status":%d,"inFlow":0,"outFlow":0,"runtimeBackend":"gost","runtimeStatus":%d,"runtimeMessage":%q,"lastRuntimeSyncTime":0,"createdTime":0,"updatedTime":0,"userName":%q,"userId":%d,"inx":0},"msg":"操作成功","ts":0}`,
		id, name, tunnelID, host, port, remote, strategy, status, runtimeStatus, runtimeMessage, user.Email, user.ID)
}

// nodeXPanelForward is the NodeX execute body a panel forward change sends:
// the forward, its tunnel, the ingress node with its token, and the action.
func nodeXPanelForward(action string, forward model.Forward, tunnel *model.ForwardTunnel, node *model.ForwardNode, status int) string {
	return fmt.Sprintf(`{"action":%q,"backend":"gost","panelForward":{"forward":{"id":%d,"inPort":%d,"interfaceName":"","name":%q,"remoteAddr":%q,"status":%d,"strategy":%q,"userId":%d},`+
		`"ingressNode":{"apiPort":%d,"apiToken":%q,"host":%q,"id":%d,"name":%q,"port":%d},`+
		`"tunnel":{"id":%d,"inNodeId":%d,"interfaceName":"","name":%q,"protocol":%q,"tcpListenAddr":"0.0.0.0","udpListenAddr":"0.0.0.0"}},"resourceType":"panel_forward"}`,
		action, forward.ID, forward.InPort, forward.Name, forward.RemoteAddr, status, forward.Strategy, forward.UserID,
		node.APIPort, node.APIToken, node.Host, node.ID, node.Name, node.Port,
		tunnel.ID, tunnel.InNodeID, tunnel.Name, tunnel.Protocol)
}

func (s *ForwardOperationsAnswersTestSuite) TestPanelForwardChanges() {
	relay := s.node(model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-token", Enabled: true})
	user := s.user("ops@example.test")
	tunnel := s.tunnel("tunnel", relay, nil)
	handler := NewForwardHandler()
	post := func(path, body string) string {
		return s.serve("POST", path, path, body, user.ID, true, map[string]gin.HandlerFunc{
			"/admin/forward/create": handler.CreatePanelForward, "/admin/forward/update": handler.UpdatePanelForward,
			"/admin/forward/pause": handler.PausePanelForward, "/admin/forward/resume": handler.ResumePanelForward,
			"/admin/forward/delete": handler.DeletePanelForward, "/admin/forward/force-delete": handler.ForceDeletePanelForward,
		}[path])
	}

	// Create: NodeX applies it synchronously and the answer carries the
	// runtime result.
	answer := post("/admin/forward/create", fmt.Sprintf(`{"name":"web","tunnelId":%d,"inPort":30000,"remoteAddr":"203.0.113.1:80","strategy":"fifo"}`, tunnel.ID))
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal([]byte(answer), &created))
	id := created.Data.ID
	s.Equal(panelForwardItem(id, tunnel.ID, "web", relay.Host, 30000, "203.0.113.1:80", "fifo", model.ForwardStatusActive, model.ForwardRuntimeJobStatusSuccess, "nodex applied", user), answer)
	s.Equal([]string{nodeXPanelForward("create", s.forward(id), tunnel, relay, model.ForwardStatusActive)}, s.nodex.received(s.T()))
	job := s.job(id)
	s.Equal(model.ForwardRuntimeJobStatusSuccess, job.Status)
	s.Equal("create", job.Action)
	s.Equal("applied", job.Result)

	// Update.
	// A single target keeps the fifo strategy whatever was asked.
	answer = post("/admin/forward/update", fmt.Sprintf(`{"id":%d,"name":"web2","tunnelId":%d,"inPort":30001,"remoteAddr":"203.0.113.2:80","strategy":"round"}`, id, tunnel.ID))
	s.Equal(panelForwardItem(id, tunnel.ID, "web2", relay.Host, 30001, "203.0.113.2:80", "fifo", model.ForwardStatusActive, model.ForwardRuntimeJobStatusSuccess, "nodex applied", user), answer)
	s.Equal([]string{nodeXPanelForward("update", s.forward(id), tunnel, relay, model.ForwardStatusActive)}, s.nodex.received(s.T()))

	// Pause, resume, pause again (idempotent), then delete.
	s.Equal(`{"code":0,"data":true,"msg":"操作成功","ts":0}`, post("/admin/forward/pause", fmt.Sprintf(`{"id":%d}`, id)))
	s.Equal([]string{nodeXPanelForward("pause", s.forward(id), tunnel, relay, model.ForwardStatusActive)}, s.nodex.received(s.T()))
	s.Equal(model.ForwardStatusPaused, s.forward(id).Status)
	s.Equal(`{"code":0,"data":true,"msg":"操作成功","ts":0}`, post("/admin/forward/resume", fmt.Sprintf(`{"id":%d}`, id)))
	s.Equal([]string{nodeXPanelForward("resume", s.forward(id), tunnel, relay, model.ForwardStatusActive)}, s.nodex.received(s.T()))
	s.Equal(`{"code":0,"data":true,"msg":"操作成功","ts":0}`, post("/admin/forward/resume", fmt.Sprintf(`{"id":%d}`, id)))
	s.Empty(s.nodex.received(s.T()), "resuming an active forward sends nothing")
	s.Equal(`{"code":-1,"data":null,"msg":"转发服务正在运行，请先暂停或使用强制删除","ts":0}`, post("/admin/forward/delete", fmt.Sprintf(`{"id":%d}`, id)))
	s.Empty(s.nodex.received(s.T()))
	post("/admin/forward/pause", fmt.Sprintf(`{"id":%d}`, id))
	s.nodex.received(s.T())
	s.Equal(`{"code":0,"data":true,"msg":"操作成功","ts":0}`, post("/admin/forward/delete", fmt.Sprintf(`{"id":%d}`, id)))
	s.Equal([]string{nodeXPanelForward("delete", model.Forward{ID: id, UserID: user.ID, Name: "web2", InPort: 30001, RemoteAddr: "203.0.113.2:80", Strategy: "fifo"}, tunnel, relay, model.ForwardStatusPaused)}, s.nodex.received(s.T()))
	s.Equal(`{"code":-1,"data":null,"msg":"转发不存在","ts":0}`, post("/admin/forward/delete", fmt.Sprintf(`{"id":%d}`, id)))

	// A change NodeX refuses: the forward is created in error and the
	// answer carries NodeX's message; force-delete removes it anyway.
	answer = post("/admin/forward/create", fmt.Sprintf(`{"name":"failing","tunnelId":%d,"inPort":30002,"remoteAddr":"203.0.113.3:80"}`, tunnel.ID))
	s.Require().NoError(json.Unmarshal([]byte(answer), &created))
	failing := created.Data.ID
	s.Equal(panelForwardItem(failing, tunnel.ID, "failing", relay.Host, 30002, "203.0.113.3:80", "fifo", model.ForwardStatusError, model.ForwardRuntimeJobStatusFailed, "nodex refused the change", user), answer)
	s.nodex.received(s.T())
	job = s.job(failing)
	s.Equal(model.ForwardRuntimeJobStatusFailed, job.Status)
	s.Equal("nodex refused the change", job.Error)
	s.Equal(`{"code":0,"data":true,"msg":"操作成功","ts":0}`, post("/admin/forward/force-delete", fmt.Sprintf(`{"id":%d}`, failing)))
	s.Equal([]string{nodeXPanelForward("delete", model.Forward{ID: failing, UserID: user.ID, Name: "failing", InPort: 30002, RemoteAddr: "203.0.113.3:80", Strategy: "fifo"}, tunnel, relay, model.ForwardStatusError)}, s.nodex.received(s.T()))

	// A user may not point a forward at a private target (#90).
	s.Equal(`{"code":-1,"data":null,"msg":"用户不存在","ts":0}`,
		s.serve("POST", "/forward/create", "/forward/create", fmt.Sprintf(`{"name":"private","tunnelId":%d,"inPort":30003,"remoteAddr":"10.0.0.1:80"}`, tunnel.ID), 999, false, handler.CreatePanelForward))
	s.Empty(s.nodex.received(s.T()))
}

func (s *ForwardOperationsAnswersTestSuite) TestTunnelUpdateAndBackendSync() {
	relay := s.node(model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-token", Enabled: true})
	exit := s.node(model.ForwardNode{Name: "exit", Type: model.ForwardNodeTypeExit, Host: "198.51.100.11", Port: 443, APIPort: 18080, APIToken: "exit-token", Enabled: true})
	user := s.user("ops@example.test")
	tunnel := s.tunnel("tunnel", relay, exit)
	first := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "a", TunnelID: tunnel.ID, InPort: 30000, RemoteAddr: "203.0.113.1:80", Strategy: "fifo", Status: model.ForwardStatusActive, RuntimeBackend: "gost"}
	second := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "failing", TunnelID: tunnel.ID, InPort: 30001, RemoteAddr: "203.0.113.2:80", Strategy: "fifo", Status: model.ForwardStatusActive, RuntimeBackend: "gost"}
	paused := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "paused", TunnelID: tunnel.ID, InPort: 30002, RemoteAddr: "203.0.113.3:80", Strategy: "fifo", Status: model.ForwardStatusActive, RuntimeBackend: "gost"}
	s.Require().NoError(s.db.Create(&[]*model.Forward{first, second, paused}).Error)
	// The status column defaults to active on insert; pause it afterwards.
	s.Require().NoError(s.db.Model(paused).Update("status", model.ForwardStatusPaused).Error)
	paused.Status = model.ForwardStatusPaused
	handler := NewForwardHandler()

	// A tunnel update that changes what its nodes run re-applies every
	// active forward; one NodeX refuses is marked in error and named.
	answer := s.serve("POST", "/admin/tunnel/update", "/admin/tunnel/update",
		fmt.Sprintf(`{"id":%d,"name":"tunnel","flow":2,"protocol":"udp","tcpListenAddr":"0.0.0.0","udpListenAddr":"0.0.0.0"}`, tunnel.ID), user.ID, true, handler.UpdatePanelTunnel)
	s.Equal(`{"code":-1,"data":null,"msg":"隧道更新成功，但部分转发同步失败: nodex refused the change","ts":0}`, answer)
	updated := *tunnel
	updated.Protocol = "udp"
	s.Equal([]string{
		nodeXPanelForward("update", *first, &updated, relay, model.ForwardStatusActive),
		nodeXPanelForward("update", *second, &updated, relay, model.ForwardStatusActive),
	}, s.nodex.received(s.T()))
	s.Equal(model.ForwardStatusActive, s.forward(first.ID).Status)
	s.Equal(model.ForwardStatusError, s.forward(second.ID).Status)
	s.Equal(model.ForwardStatusPaused, s.forward(paused.ID).Status)
	s.Equal(model.ForwardRuntimeJobStatusSuccess, s.forward(first.ID).RuntimeStatus)
	s.Equal("nodex refused the change", s.forward(second.ID).RuntimeMessage)

	// A tunnel update that changes nothing a node runs sends nothing.
	answer = s.serve("POST", "/admin/tunnel/update", "/admin/tunnel/update",
		fmt.Sprintf(`{"id":%d,"name":"renamed","flow":2,"protocol":"udp","tcpListenAddr":"0.0.0.0","udpListenAddr":"0.0.0.0"}`, tunnel.ID), user.ID, true, handler.UpdatePanelTunnel)
	s.Equal(fmt.Sprintf(`{"code":0,"data":{"id":%d,"name":"renamed","inNodeId":%d,"outNodeId":%d,"type":2,"flow":2,"trafficRatio":1,"interfaceName":"","protocol":"udp","tcpListenAddr":"0.0.0.0","udpListenAddr":"0.0.0.0","inIp":"198.51.100.10","outIp":"198.51.100.11","status":1,"createdTime":0,"updatedTime":0},"msg":"操作成功","ts":0}`, tunnel.ID, relay.ID, exit.ID), answer)
	s.Empty(s.nodex.received(s.T()))
	updated.Name = "renamed"

	// The backend sync re-applies the active forwards not on the target
	// backend, with the sync action, and counts them.
	s.Require().NoError(s.db.Model(&model.Forward{}).Where("id IN ?", []uint{first.ID, second.ID, paused.ID}).Update("runtime_backend", model.ForwardRuntimeBackendCleanAgent).Error)
	s.Require().NoError(s.db.Model(&model.Forward{}).Where("id = ?", second.ID).Update("status", model.ForwardStatusActive).Error)
	answer = s.serve("POST", "/admin/forward/sync-backend", "/admin/forward/sync-backend", `{"backend":"gost"}`, user.ID, true, handler.SyncForwardsToBackend)
	s.Equal(`{"code":0,"data":{"failed":1,"synced":1},"msg":"操作成功","ts":0}`, answer)
	s.Equal([]string{
		nodeXPanelForward("sync", *first, &updated, relay, model.ForwardStatusActive),
		nodeXPanelForward("sync", *second, &updated, relay, model.ForwardStatusActive),
	}, s.nodex.received(s.T()))
	s.Equal(model.ForwardRuntimeBackendGost, s.forward(first.ID).RuntimeBackend)
	s.Equal(model.ForwardRuntimeBackendCleanAgent, s.forward(second.ID).RuntimeBackend, "a forward NodeX refused stays on its backend")
	s.Equal(model.ForwardRuntimeBackendCleanAgent, s.forward(paused.ID).RuntimeBackend, "a paused forward is not synced")
	s.Equal(`{"code":-1,"data":null,"msg":"不支持的运行时后端","ts":0}`,
		s.serve("POST", "/admin/forward/sync-backend", "/admin/forward/sync-backend", `{"backend":"docker"}`, user.ID, true, handler.SyncForwardsToBackend))

	// Removing a user's tunnel permission removes the user's forwards from
	// the node first.
	permission := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnel.ID, Flow: 100, Num: 10, Status: model.ForwardUserTunnelStatusActive}
	s.Require().NoError(s.db.Create(permission).Error)
	s.Require().NoError(s.db.Model(&model.Forward{}).Where("id = ?", second.ID).Updates(map[string]any{"name": "b", "runtime_backend": model.ForwardRuntimeBackendGost}).Error)
	s.Require().NoError(s.db.Model(&model.Forward{}).Where("id = ?", paused.ID).Update("runtime_backend", model.ForwardRuntimeBackendGost).Error)
	answer = s.serve("POST", "/admin/tunnel/user/remove", "/admin/tunnel/user/remove", fmt.Sprintf(`{"id":%d}`, permission.ID), user.ID, true, handler.RemovePanelUserTunnel)
	s.Equal(`{"code":0,"data":"用户隧道权限删除成功","msg":"操作成功","ts":0}`, answer)
	renamed := *second
	renamed.Name = "b"
	s.Equal([]string{
		nodeXPanelForward("delete", *first, &updated, relay, model.ForwardStatusActive),
		nodeXPanelForward("delete", renamed, &updated, relay, model.ForwardStatusActive),
		nodeXPanelForward("delete", *paused, &updated, relay, model.ForwardStatusPaused),
	}, s.nodex.received(s.T()))
	var remaining int64
	s.Require().NoError(s.db.Model(&model.Forward{}).Where("tunnel_id = ?", tunnel.ID).Count(&remaining).Error)
	s.Zero(remaining)
}

// nodeXLegacyRule is the NodeX execute body a legacy rule change sends: the
// rule and both nodes with their tokens.
func nodeXLegacyRule(action string, rule model.ForwardRule, relay, exit *model.ForwardNode) string {
	node := func(n *model.ForwardNode) string {
		return fmt.Sprintf(`{"apiPort":%d,"apiToken":%q,"host":%q,"id":%d,"name":%q,"port":%d}`, n.APIPort, n.APIToken, n.Host, n.ID, n.Name, n.Port)
	}
	return fmt.Sprintf(`{"action":%q,"backend":"gost","legacyRule":{"exitNode":%s,"relayNode":%s,"rule":{"enabled":%t,"id":%d,"listenPort":%d,"name":%q,"protocol":%q,"targetHost":%q,"targetPort":%d}},"resourceType":"legacy_rule"}`,
		action, node(exit), node(relay), rule.Enabled, rule.ID, rule.ListenPort, rule.Name, rule.Protocol, rule.TargetHost, rule.TargetPort)
}

func ruleAnswer(rule model.ForwardRule, relay, exit *model.ForwardNode) string {
	node := func(n *model.ForwardNode) string {
		return fmt.Sprintf(`{"id":%d,"name":%q,"type":%q,"host":%q,"port":%d,"api_port":%d,"api_token":"********","metrics_port":0,"region":"","isp":"","datacenter":"","bandwidth":0,"status":0,"last_check":"-","latency":0,"load":0,"uptime":0,"tags":"","weight":1,"max_conn":0,"enabled":true,"total_upload":0,"total_download":0,"current_conn":0,"created_at":"-","updated_at":"-"}`,
			n.ID, n.Name, n.Type, n.Host, n.Port, n.APIPort)
	}
	nodes := ""
	if relay != nil {
		nodes = fmt.Sprintf(`,"relay_node":%s`, node(relay))
	}
	exitPart := ""
	if exit != nil {
		exitPart = fmt.Sprintf(`,"exit_node":%s`, node(exit))
	}
	return fmt.Sprintf(`{"code":0,"data":{"id":%d,"name":%q,"enabled":%t,"relay_node_id":%d%s,"listen_port":%d,"protocol":%q,"exit_node_id":%d%s,"target_host":%q,"target_port":%d,"user_id":null,"user_group_id":null,"allowed_ips":"","speed_limit":null,"traffic_limit":null,"expire_time":null,"upload":0,"download":0,"connections":0,"total_conns":0,"remark":"","created_at":"-","updated_at":"-"},"msg":"操作成功","ts":0}`,
		rule.ID, rule.Name, rule.Enabled, rule.RelayNodeID, nodes, rule.ListenPort, rule.Protocol, rule.ExitNodeID, exitPart, rule.TargetHost, rule.TargetPort)
}

func (s *ForwardOperationsAnswersTestSuite) TestLegacyRules() {
	relay := s.node(model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-token", Enabled: true})
	exit := s.node(model.ForwardNode{Name: "exit", Type: model.ForwardNodeTypeExit, Host: "198.51.100.11", Port: 443, APIPort: 18081, APIToken: "exit-token", Enabled: true})
	handler := NewForwardHandler()
	id := func(value uint) string { return strconv.FormatUint(uint64(value), 10) }

	// Create: the row is written, the rule is pushed through NodeX, and the
	// answer shows the rule without the nodes.
	answer := s.serve("POST", "/admin/forward/rules", "/admin/forward/rules",
		fmt.Sprintf(`{"name":"rule","relay_node_id":%d,"listen_port":2000,"exit_node_id":%d,"target_host":"203.0.113.9","target_port":80}`, relay.ID, exit.ID), 1, true, handler.CreateRule)
	rule := s.rule(answer)
	s.Equal(ruleAnswer(rule, nil, nil), answer)
	s.Equal("tcp", rule.Protocol)
	s.Equal([]string{nodeXLegacyRule("create", rule, relay, exit)}, s.nodex.received(s.T()))

	// Update answers the rule with its nodes, tokens masked.
	answer = s.serve("PUT", "/admin/forward/rules/:id", "/admin/forward/rules/"+id(rule.ID), `{"name":"rule2","target_port":81,"protocol":"udp"}`, 1, true, handler.UpdateRule)
	rule.Name, rule.TargetPort, rule.Protocol = "rule2", 81, "udp"
	s.Equal(ruleAnswer(rule, relay, exit), answer)
	s.Equal([]string{nodeXLegacyRule("update", rule, relay, exit)}, s.nodex.received(s.T()))

	// Toggle syncs the rule's enabled state.
	s.Equal(`{"code":0,"data":"updated","msg":"操作成功","ts":0}`,
		s.serve("POST", "/admin/forward/rules/:id/toggle", "/admin/forward/rules/"+id(rule.ID)+"/toggle", `{"enabled":false}`, 1, true, handler.ToggleRule))
	rule.Enabled = false
	s.Equal([]string{nodeXLegacyRule("sync", rule, relay, exit)}, s.nodex.received(s.T()))

	// A refusal by NodeX does not undo the row: the legacy routes log it.
	answer = s.serve("PUT", "/admin/forward/rules/:id", "/admin/forward/rules/"+id(rule.ID), `{"name":"failing"}`, 1, true, handler.UpdateRule)
	rule.Name = "failing"
	s.Equal(ruleAnswer(rule, relay, exit), answer)
	s.Equal([]string{nodeXLegacyRule("update", rule, relay, exit)}, s.nodex.received(s.T()))

	// Delete removes it from NodeX, then the row.
	s.Equal(`{"code":0,"data":"deleted","msg":"操作成功","ts":0}`,
		s.serve("DELETE", "/admin/forward/rules/:id", "/admin/forward/rules/"+id(rule.ID), "", 1, true, handler.DeleteRule))
	s.Equal([]string{nodeXLegacyRule("delete", rule, relay, exit)}, s.nodex.received(s.T()))
	s.Equal(`{"code":-1,"data":null,"msg":"record not found","ts":0}`,
		s.serve("DELETE", "/admin/forward/rules/:id", "/admin/forward/rules/"+id(rule.ID), "", 1, true, handler.DeleteRule))

	// A user may not create a legacy rule (#90); an administrator may,
	// through the user route.
	s.Equal(`{"code":-1,"data":null,"msg":"`+service.ErrForwardRuleAdminOnly.Error()+`","ts":0}`,
		s.serve("POST", "/user/forward/rules", "/user/forward/rules", fmt.Sprintf(`{"name":"mine","relay_node_id":%d,"exit_node_id":%d,"target_host":"203.0.113.9","target_port":80}`, relay.ID, exit.ID), 2, false, handler.CreateUserRule))
	s.Empty(s.nodex.received(s.T()))
	answer = s.serve("POST", "/user/forward/rules", "/user/forward/rules", fmt.Sprintf(`{"name":"admins","relay_node_id":%d,"exit_node_id":%d,"protocol":"tcp","target_host":"203.0.113.9","target_port":80}`, relay.ID, exit.ID), 1, true, handler.CreateUserRule)
	owned := s.rule(answer)
	s.Equal(10000, owned.ListenPort)
	s.Equal(strings.Replace(ruleAnswer(owned, nil, nil), `"user_id":null`, `"user_id":1`, 1), answer)
	s.Equal([]string{nodeXLegacyRule("create", owned, relay, exit)}, s.nodex.received(s.T()))
}
