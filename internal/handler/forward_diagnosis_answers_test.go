package handler

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// ForwardDiagnosisAnswersTestSuite pins the bytes the legacy diagnosis
// routes answer: the forward node and Ansible machine checks, the node
// statistics, and the forward and tunnel diagnoses. The KernelNodeOps
// diagnose executors (NO-8) share these routes' implementation; the answers
// stay what they were before the routes moved onto it. Only the values that
// change from one call to the next are normalized (normalizeDiagnosisAnswer).
type ForwardDiagnosisAnswersTestSuite struct {
	HandlerTestSuite
}

func TestForwardDiagnosisAnswers(t *testing.T) {
	suite.Run(t, new(ForwardDiagnosisAnswersTestSuite))
}

var diagnosisVolatile = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`"ts":\d+`), `"ts":0`},
	{regexp.MustCompile(`"timestamp":\d+`), `"timestamp":0`},
	{regexp.MustCompile(`"check_time":"[^"]*"`), `"check_time":"-"`},
	{regexp.MustCompile(`"latency":\d+`), `"latency":0`},
	// averageTime is omitted when a probe took under a millisecond.
	{regexp.MustCompile(`,"averageTime":[0-9.e+]+`), ``},
}

// normalizeDiagnosisAnswer replaces the clock readings of an answer: the
// envelope's ts, a report's timestamp, a check's time and latencies.
func normalizeDiagnosisAnswer(body string) string {
	for _, volatile := range diagnosisVolatile {
		body = volatile.pattern.ReplaceAllString(body, volatile.with)
	}
	return body
}

func (s *ForwardDiagnosisAnswersTestSuite) serve(method, pattern, path, body string, actor uint, admin bool, handler gin.HandlerFunc) string {
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
	return normalizeDiagnosisAnswer(w.Body.String())
}

// listening returns a loopback address that accepts connections.
func (s *ForwardDiagnosisAnswersTestSuite) listening() (string, int) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	s.T().Cleanup(func() { _ = listener.Close() })
	return "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
}

// closed returns a loopback port nothing listens on: a dial is refused.
func (s *ForwardDiagnosisAnswersTestSuite) closed() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	port := listener.Addr().(*net.TCPAddr).Port
	s.Require().NoError(listener.Close())
	return port
}

func (s *ForwardDiagnosisAnswersTestSuite) node(node model.ForwardNode) *model.ForwardNode {
	s.Require().NoError(s.db.Create(&node).Error)
	return &node
}

func refusedMessage(port int) string {
	return fmt.Sprintf("dial tcp 127.0.0.1:%d: connect: connection refused", port)
}

func (s *ForwardDiagnosisAnswersTestSuite) TestNodeChecks() {
	host, port := s.listening()
	down := s.closed()
	online := s.node(model.ForwardNode{Name: "online", Type: model.ForwardNodeTypeRelay, Host: host, Port: port, APIPort: 18080, APIToken: "check-token", Enabled: true})
	offline := s.node(model.ForwardNode{Name: "offline", Type: model.ForwardNodeTypeRelay, Host: host, Port: down, APIPort: 18080, Enabled: true, Status: model.ForwardNodeStatusOnline})
	machine := s.node(model.ForwardNode{Name: "machine", Type: model.ForwardNodeTypeRelay, Host: host, Port: port, Tags: `["ansible-machine"]`, Enabled: true})
	handler := NewForwardHandler()
	check := func(pattern, prefix string, id uint, h gin.HandlerFunc) string {
		return s.serve("POST", pattern, prefix+strconv.FormatUint(uint64(id), 10)+"/check", "", 1, true, h)
	}

	s.Equal(fmt.Sprintf(`{"code":0,"data":{"node_id":%d,"status":1,"latency":0,"check_time":"-"},"msg":"操作成功","ts":0}`, online.ID),
		check("/admin/forward/nodes/:id/check", "/admin/forward/nodes/", online.ID, handler.CheckNode))
	s.Equal(fmt.Sprintf(`{"code":0,"data":{"node_id":%d,"status":0,"latency":0,"check_time":"-","error":%q},"msg":"操作成功","ts":0}`, offline.ID, refusedMessage(down)),
		check("/admin/forward/nodes/:id/check", "/admin/forward/nodes/", offline.ID, handler.CheckNode))
	s.Equal(fmt.Sprintf(`{"code":0,"data":{"node_id":%d,"status":1,"latency":0,"check_time":"-"},"msg":"操作成功","ts":0}`, machine.ID),
		check("/admin/forward/ansible-machines/:id/check", "/admin/forward/ansible-machines/", machine.ID, handler.CheckAnsibleMachine))
	s.Equal(`{"code":-1,"data":null,"msg":"node not found","ts":0}`,
		check("/admin/forward/nodes/:id/check", "/admin/forward/nodes/", 999, handler.CheckNode))

	// The check records what it found; uptime is 100 only when the node
	// was online before the check.
	load := func(id uint) model.ForwardNode {
		var stored model.ForwardNode
		s.Require().NoError(s.db.First(&stored, id).Error)
		return stored
	}
	stored := load(online.ID)
	s.Equal(model.ForwardNodeStatusOnline, stored.Status)
	s.False(stored.LastCheck.IsZero())
	s.Zero(stored.Uptime)
	s.Equal("check-token", stored.APIToken)
	stored = load(offline.ID)
	s.Equal(model.ForwardNodeStatusOffline, stored.Status)
	s.False(stored.LastCheck.IsZero())
	check("/admin/forward/nodes/:id/check", "/admin/forward/nodes/", online.ID, handler.CheckNode)
	s.Equal(float64(100), load(online.ID).Uptime)
}

func (s *ForwardDiagnosisAnswersTestSuite) TestNodeStatistics() {
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/metrics", r.URL.Path)
		s.Empty(r.Header.Get("Authorization"), "the metrics endpoint gets no credential")
		_, _ = w.Write([]byte("# HELP gost_service_transfer_input_bytes_total in\n" +
			`gost_service_transfer_input_bytes_total{client="198.51.100.1",service="svc-b"} 100` + "\n" +
			`gost_service_transfer_input_bytes_total{client="198.51.100.2",service="svc-b"} 23` + "\n" +
			`gost_service_transfer_output_bytes_total{client="198.51.100.1",service="svc-b"} 7` + "\n" +
			`gost_service_transfer_input_bytes_total{client="198.51.100.1",service="svc-a"} 5` + "\n" +
			`gost_service_transfer_output_bytes_total{client="198.51.100.1",service="svc-a"} 6` + "\n"))
	}))
	s.T().Cleanup(metrics.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "metrics are down", http.StatusServiceUnavailable)
	}))
	s.T().Cleanup(failing.Close)
	metricsPort := metrics.Listener.Addr().(*net.TCPAddr).Port
	failingPort := failing.Listener.Addr().(*net.TCPAddr).Port

	gost := s.node(model.ForwardNode{Name: "gost", Type: model.ForwardNodeTypeRelay, Host: "127.0.0.1", Port: 443, APIPort: 18080, APIToken: "stats-token", MetricsPort: metricsPort, Enabled: true})
	down := s.node(model.ForwardNode{Name: "down", Type: model.ForwardNodeTypeRelay, Host: "127.0.0.1", Port: 443, APIPort: 18080, MetricsPort: failingPort, Enabled: true})
	unmetered := s.node(model.ForwardNode{Name: "unmetered", Type: model.ForwardNodeTypeRelay, Host: "127.0.0.1", Port: 443, APIPort: 18080, Enabled: true})
	noAPI := s.node(model.ForwardNode{Name: "no-api", Type: model.ForwardNodeTypeExit, Host: "127.0.0.1", Port: 443, MetricsPort: metricsPort, Enabled: true})
	machine := s.node(model.ForwardNode{Name: "machine", Type: model.ForwardNodeTypeRelay, Host: "127.0.0.1", Port: 22, Tags: `["ansible-machine"]`,
		Enabled: true, CurrentConn: 3, TotalUpload: 1024, TotalDownload: 2048})
	handler := NewForwardHandler()
	stats := func(id uint) string {
		return s.serve("POST", "/admin/forward/nodes/:id/sync-stats", "/admin/forward/nodes/"+strconv.FormatUint(uint64(id), 10)+"/sync-stats", "", 1, true, handler.SyncNodeStats)
	}

	s.Equal(`{"code":0,"data":{"message":"Stats synced","stats":{"svc-a":{"InBytes":5,"OutBytes":6},"svc-b":{"InBytes":123,"OutBytes":7}}},"msg":"操作成功","ts":0}`, stats(gost.ID))
	s.Equal(`{"code":-1,"data":null,"msg":"metrics API error: 503 Service Unavailable - metrics are down\n","ts":0}`, stats(down.ID))
	s.Equal(fmt.Sprintf(`{"code":-1,"data":null,"msg":"metrics endpoint not configured for node %d","ts":0}`, unmetered.ID), stats(unmetered.ID))
	s.Equal(`{"code":-1,"data":null,"msg":"node API port not configured","ts":0}`, stats(noAPI.ID))
	s.Equal(`{"code":-1,"data":null,"msg":"node not found","ts":0}`, stats(999))

	s.Equal(`{"code":0,"data":{"message":"Ansible machines do not expose gost management API stats; keeping panel-side counters","stats":{"current_conn":3,"total_download":2048,"total_upload":1024}},"msg":"操作成功","ts":0}`,
		s.serve("POST", "/admin/forward/ansible-machines/:id/sync-stats", "/admin/forward/ansible-machines/"+strconv.FormatUint(uint64(machine.ID), 10)+"/sync-stats", "", 1, true, handler.SyncAnsibleMachineStats))
}

func (s *ForwardDiagnosisAnswersTestSuite) TestForwardDiagnosis() {
	host, port := s.listening()
	down := s.closed()
	relay := s.node(model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, Enabled: true})
	user := &model.User{Email: "diagnosis@example.test", Token: "diagnosis-token", UUID: "diagnosis-uuid", TransferEnable: 1 << 40}
	s.Require().NoError(s.db.Create(user).Error)
	tunnel := &model.ForwardTunnel{Name: "diag-tunnel", InNodeID: relay.ID, InIP: relay.Host, Type: 1, Flow: 2, TrafficRatio: 1, Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(s.db.Create(tunnel).Error)
	forward := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "diag-forward", TunnelID: tunnel.ID, InPort: 30000,
		RemoteAddr: fmt.Sprintf("%s:%d, ,%s:%d,no-port,127.0.0.2:%d", host, port, host, down, down), Status: model.ForwardStatusPaused}
	s.Require().NoError(s.db.Create(forward).Error)
	empty := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "empty-forward", TunnelID: tunnel.ID, InPort: 30001, RemoteAddr: " , ", Status: model.ForwardStatusPaused}
	s.Require().NoError(s.db.Create(empty).Error)
	handler := NewForwardHandler()
	diagnose := func(id uint, actor uint, admin bool) string {
		return s.serve("POST", "/forward/diagnose", "/forward/diagnose", fmt.Sprintf(`{"forwardId":%d}`, id), actor, admin, handler.DiagnosePanelForward)
	}
	outcome := func(success bool, target string, port int, message string) string {
		text := fmt.Sprintf(`{"success":%t,"description":"转发-\u003e目标","nodeName":"diag-tunnel","nodeId":"%d","targetIp":%q`, success, tunnel.ID, target)
		if port != 0 {
			text += fmt.Sprintf(`,"targetPort":%d`, port)
		}
		if message != "" {
			text += fmt.Sprintf(`,"message":%q`, message)
		}
		return text + "}"
	}
	report := func(outcomes ...string) string {
		text := `{"code":0,"data":{"forwardName":"diag-forward","timestamp":0,"results":[`
		for i, item := range outcomes {
			if i > 0 {
				text += ","
			}
			text += item
		}
		return text + `]},"msg":"操作成功","ts":0}`
	}

	s.Equal(report(
		outcome(true, host, port, ""),
		outcome(false, host, down, refusedMessage(down)),
		outcome(false, "no-port", 0, "无法解析目标地址"),
		outcome(false, "127.0.0.2", down, fmt.Sprintf("dial tcp 127.0.0.2:%d: connect: connection refused", down)),
	), diagnose(forward.ID, 1, true), "an administrator's diagnosis dials every target")
	s.Equal(report(
		outcome(false, host, port, "不能诊断内网或本机地址"),
		outcome(false, host, down, "不能诊断内网或本机地址"),
		outcome(false, "no-port", 0, "无法解析目标地址"),
		outcome(false, "127.0.0.2", down, "不能诊断内网或本机地址"),
	), diagnose(forward.ID, user.ID, false), "a user's diagnosis dials public targets only")
	s.Equal(`{"code":0,"data":{"forwardName":"empty-forward","timestamp":0,"results":[{"success":false,"description":"转发-\u003e目标","nodeName":"diag-tunnel","nodeId":"`+
		strconv.FormatUint(uint64(tunnel.ID), 10)+`","targetIp":"-","message":"没有可诊断的目标地址"}]},"msg":"操作成功","ts":0}`, diagnose(empty.ID, 1, true))
	s.Equal(`{"code":-1,"data":null,"msg":"转发不存在","ts":0}`, diagnose(forward.ID, user.ID+1, false), "another user's forward")
	s.Equal(`{"code":-1,"data":null,"msg":"转发不存在","ts":0}`, diagnose(999, 1, true))
}

func (s *ForwardDiagnosisAnswersTestSuite) TestTunnelDiagnosis() {
	host, port := s.listening()
	down := s.closed()
	entry := s.node(model.ForwardNode{Name: "entry", Type: model.ForwardNodeTypeRelay, Host: host, Port: port, Enabled: true})
	exit := s.node(model.ForwardNode{Name: "exit", Type: model.ForwardNodeTypeExit, Host: " " + host + " ", Port: down, Enabled: true})
	unaddressed := s.node(model.ForwardNode{Name: "unaddressed", Type: model.ForwardNodeTypeRelay, Host: " ", Port: 0, Enabled: true})
	twoHop := &model.ForwardTunnel{Name: "two-hop", InNodeID: entry.ID, OutNodeID: &exit.ID, InIP: host, Type: 2, Flow: 2, TrafficRatio: 1, Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(s.db.Create(twoHop).Error)
	bare := &model.ForwardTunnel{Name: "bare", InNodeID: unaddressed.ID, InIP: "-", Type: 1, Flow: 2, TrafficRatio: 1, Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(s.db.Create(bare).Error)
	none := &model.ForwardTunnel{Name: "none", InNodeID: 0, InIP: "-", Type: 1, Flow: 2, TrafficRatio: 1, Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(s.db.Create(none).Error)
	handler := NewForwardHandler()
	diagnose := func(id uint) string {
		return s.serve("POST", "/admin/tunnel/diagnose", "/admin/tunnel/diagnose", fmt.Sprintf(`{"tunnelId":%d}`, id), 1, true, handler.DiagnosePanelTunnel)
	}
	id := func(value uint) string { return strconv.FormatUint(uint64(value), 10) }

	s.Equal(`{"code":0,"data":{"tunnelId":`+id(twoHop.ID)+`,"tunnelName":"two-hop","tunnelType":"隧道转发","timestamp":0,"results":[`+
		`{"success":true,"description":"管理端-\u003e入口节点","nodeName":"entry","nodeId":"`+id(entry.ID)+`","targetIp":"127.0.0.1","targetPort":`+strconv.Itoa(port)+`},`+
		`{"success":false,"description":"管理端-\u003e出口节点","nodeName":"exit","nodeId":"`+id(exit.ID)+`","targetIp":"127.0.0.1","targetPort":`+strconv.Itoa(down)+`,"message":"`+refusedMessage(down)+`"}]},"msg":"操作成功","ts":0}`,
		diagnose(twoHop.ID))
	s.Equal(`{"code":0,"data":{"tunnelId":`+id(bare.ID)+`,"tunnelName":"bare","tunnelType":"端口转发","timestamp":0,"results":[`+
		`{"success":false,"description":"管理端-\u003e入口节点","nodeName":"unaddressed","nodeId":"`+id(unaddressed.ID)+`","targetIp":"","message":"节点地址未配置"}]},"msg":"操作成功","ts":0}`,
		diagnose(bare.ID))
	s.Equal(`{"code":0,"data":{"tunnelId":`+id(none.ID)+`,"tunnelName":"none","tunnelType":"端口转发","timestamp":0,"results":[`+
		`{"success":false,"description":"隧道诊断","nodeName":"none","nodeId":"`+id(none.ID)+`","targetIp":"-","message":"没有可诊断的节点"}]},"msg":"操作成功","ts":0}`,
		diagnose(none.ID))
	s.Equal(`{"code":-1,"data":null,"msg":"隧道不存在","ts":0}`, diagnose(999))
}
