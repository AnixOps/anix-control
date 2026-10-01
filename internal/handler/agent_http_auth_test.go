package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// AgentHTTPAuthTestSuite holds the regression tests for the agent HTTP
// routes (heartbeat, tasks, result, monitor), which had no authentication:
// anyone could mark any node online, take and complete its queued tasks
// (forward runtime jobs included) and report its monitoring data.
type AgentHTTPAuthTestSuite struct {
	suite.Suite
	db *gorm.DB
	// relayA and relayB are forward nodes, edge a proxy node.
	relayA, relayB *model.ForwardNode
	edge           *model.Node
}

const (
	relayAToken = "relay-a-token"
	relayBToken = "relay-b-token"
	edgeKey     = "edge-api-key"
)

func (s *AgentHTTPAuthTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	s.Require().NoError(database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	s.db = database.Get()
	s.Require().NoError(s.db.AutoMigrate(
		&model.Node{}, &model.ForwardNode{}, &model.ForwardTunnel{}, &model.Forward{},
		&model.ForwardRuntimeJob{}, &model.ForwardAgentBridgeTask{}, &model.AgentDiagnosticTask{},
	))
}

func (s *AgentHTTPAuthTestSuite) TearDownSuite() {
	s.Require().NoError(database.Close())
}

func (s *AgentHTTPAuthTestSuite) SetupTest() {
	for _, table := range []string{"v2_node", "v2_forward_node", "v2_forward", "v2_forward_runtime_job", "v2_forward_agent_bridge_task", "v2_agent_diagnostic_task"} {
		s.Require().NoError(s.db.Exec("DELETE FROM " + table).Error)
	}
	s.relayA = &model.ForwardNode{Name: "relay-a", Type: model.ForwardNodeTypeRelay, Host: "10.0.0.1", Port: 22, APIToken: relayAToken, Enabled: true}
	s.relayB = &model.ForwardNode{Name: "relay-b", Type: model.ForwardNodeTypeRelay, Host: "10.0.0.2", Port: 22, APIToken: relayBToken, Enabled: true}
	s.Require().NoError(s.db.Create(s.relayA).Error)
	s.Require().NoError(s.db.Create(s.relayB).Error)
	// The proxy node's id is not a forward node's, so it authenticates
	// against v2_node.
	s.edge = &model.Node{ID: s.relayB.ID + 100, Name: "edge", Host: "edge.example.test", APIKey: edgeKey, APIKeyHash: hashString(edgeKey), Status: model.NodeStatusOffline}
	s.Require().NoError(s.db.Create(s.edge).Error)
}

// router serves the four routes as the kernel registers them, behind
// RequireAgentNode.
func (s *AgentHTTPAuthTestSuite) router(handler *AgentHandler) *gin.Engine {
	router := gin.New()
	router.POST("/api/v2/agent/heartbeat", handler.RequireAgentNode, handler.AgentHeartbeat)
	router.GET("/api/v2/agent/tasks", handler.RequireAgentNode, handler.AgentGetTasks)
	router.POST("/api/v2/agent/result", handler.RequireAgentNode, handler.AgentReportResult)
	router.POST("/api/v2/agent/monitor", handler.RequireAgentNode, handler.AgentMonitor)
	return router
}

// direct serves the legacy handlers without the middleware, as a bridge
// call without the kernel's trusted identity would reach them.
func (s *AgentHTTPAuthTestSuite) direct(handler *AgentHandler) *gin.Engine {
	router := gin.New()
	router.POST("/api/v2/agent/heartbeat", handler.AgentHeartbeat)
	router.GET("/api/v2/agent/tasks", handler.AgentGetTasks)
	router.POST("/api/v2/agent/result", handler.AgentReportResult)
	router.POST("/api/v2/agent/monitor", handler.AgentMonitor)
	return router
}

func (s *AgentHTTPAuthTestSuite) send(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func as(nodeID uint, token string) map[string]string {
	return map[string]string{"X-Node-ID": strconv.FormatUint(uint64(nodeID), 10), "X-API-Key": token}
}

// seedBridgeJob queues a forward runtime job for nodeID through the
// clean_agent bridge.
func (s *AgentHTTPAuthTestSuite) seedBridgeJob(nodeID uint) (*model.ForwardRuntimeJob, *model.ForwardAgentBridgeTask) {
	forward := &model.Forward{Name: "bridge-forward", InPort: 30001, RemoteAddr: "127.0.0.1:9000", Status: model.ForwardStatusActive, RuntimeBackend: model.ForwardRuntimeBackendCleanAgent}
	s.Require().NoError(s.db.Create(forward).Error)
	now := time.Now()
	job := &model.ForwardRuntimeJob{Backend: model.ForwardRuntimeBackendCleanAgent, Action: model.ForwardRuntimeJobActionCreate, ResourceType: "panel_forward",
		ForwardID: &forward.ID, NodeID: &nodeID, Status: model.ForwardRuntimeJobStatusRunning, StartedAt: &now}
	s.Require().NoError(s.db.Create(job).Error)
	mapping := &model.ForwardAgentBridgeTask{TaskID: fmt.Sprintf("forward-runtime-job-%d", job.ID), RuntimeJobID: job.ID, NodeID: nodeID, ForwardID: &forward.ID,
		Action: model.ForwardRuntimeJobActionCreate, Type: "forward", Params: "{}", Status: model.ForwardAgentBridgeTaskStatusPending}
	s.Require().NoError(s.db.Create(mapping).Error)
	return job, mapping
}

func (s *AgentHTTPAuthTestSuite) seedDiagnosticTask(taskID string, nodeID uint, status string) {
	s.Require().NoError(s.db.Create(&model.AgentDiagnosticTask{TaskID: taskID, NodeID: nodeID, Action: "service_status", Params: `{"service":"gost"}`, Status: status}).Error)
}

func (s *AgentHTTPAuthTestSuite) forwardNodeStatus(id uint) int {
	var node model.ForwardNode
	s.Require().NoError(s.db.First(&node, id).Error)
	return node.Status
}

func (s *AgentHTTPAuthTestSuite) requireUntouched(handler *AgentHandler, job *model.ForwardRuntimeJob, mapping *model.ForwardAgentBridgeTask) {
	s.Equal(model.ForwardNodeStatusOffline, s.forwardNodeStatus(s.relayA.ID), "a node was marked online")
	var reloadedMapping model.ForwardAgentBridgeTask
	s.Require().NoError(s.db.First(&reloadedMapping, mapping.ID).Error)
	s.Equal(model.ForwardAgentBridgeTaskStatusPending, reloadedMapping.Status, "a queued forward task was taken")
	var reloadedJob model.ForwardRuntimeJob
	s.Require().NoError(s.db.First(&reloadedJob, job.ID).Error)
	s.Equal(model.ForwardRuntimeJobStatusRunning, reloadedJob.Status, "a forward runtime job was completed")
	var diagnostic model.AgentDiagnosticTask
	s.Require().NoError(s.db.Where("task_id = ?", "task-a").First(&diagnostic).Error)
	s.Equal(model.AgentDiagnosticTaskStatusPending, diagnostic.Status, "a diagnostic task was taken or completed")
	var created int64
	s.Require().NoError(s.db.Model(&model.AgentDiagnosticTask{}).Where("task_id = ?", "task-forged").Count(&created).Error)
	s.Zero(created, "a forged diagnostic result was recorded")
	_, stored := handler.monitorData.Load(s.relayA.ID)
	s.False(stored, "monitoring data was stored")
}

// Without the node's credentials, or with another node's or a wrong token,
// no route changes anything, behind the kernel's middleware or not.
func (s *AgentHTTPAuthTestSuite) TestRoutesRefuseRequestsWithoutTheNodesCredentials() {
	job, mapping := s.seedBridgeJob(s.relayA.ID)
	s.seedDiagnosticTask("task-a", s.relayA.ID, model.AgentDiagnosticTaskStatusPending)
	nodeA := strconv.FormatUint(uint64(s.relayA.ID), 10)
	requests := []struct{ method, path, body string }{
		{"POST", "/api/v2/agent/heartbeat", `{"node_id":` + nodeA + `}`},
		{"GET", "/api/v2/agent/tasks?node_id=" + nodeA, ""},
		{"POST", "/api/v2/agent/result", `{"task_id":"` + mapping.TaskID + `","node_id":` + nodeA + `,"success":true,"output":"forged"}`},
		{"POST", "/api/v2/agent/result", `{"task_id":"task-a","node_id":` + nodeA + `,"success":true,"output":"forged"}`},
		{"POST", "/api/v2/agent/result", `{"task_id":"task-forged","node_id":` + nodeA + `,"success":true,"output":"forged"}`},
		{"POST", "/api/v2/agent/monitor", `{"node_id":` + nodeA + `,"system":{"cpu":99}}`},
	}
	credentials := []map[string]string{
		nil,
		{"X-Node-ID": nodeA},
		as(s.relayA.ID, "wrong-token"),
		as(s.relayA.ID, relayBToken),
		as(s.relayA.ID, ""),
	}
	handler := NewAgentHandler()
	for name, router := range map[string]*gin.Engine{"middleware": s.router(handler), "handler": s.direct(handler)} {
		for _, headers := range credentials {
			for _, request := range requests {
				w := s.send(router, request.method, request.path, request.body, headers)
				s.Equal(http.StatusUnauthorized, w.Code, "%s %s %s %v: %s", name, request.method, request.path, headers, w.Body.String())
			}
		}
	}
	s.requireUntouched(handler, job, mapping)
}

// An authenticated node acts only as itself: it cannot report for another
// node, take its tasks or complete them.
func (s *AgentHTTPAuthTestSuite) TestANodeActsOnlyAsItself() {
	job, mapping := s.seedBridgeJob(s.relayA.ID)
	s.seedDiagnosticTask("task-a", s.relayA.ID, model.AgentDiagnosticTaskStatusPending)
	nodeA := strconv.FormatUint(uint64(s.relayA.ID), 10)
	handler := NewAgentHandler()
	router := s.router(handler)
	asB := as(s.relayB.ID, relayBToken)

	w := s.send(router, "POST", "/api/v2/agent/heartbeat", `{"node_id":`+nodeA+`}`, asB)
	s.Equal(http.StatusForbidden, w.Code, w.Body.String())
	w = s.send(router, "POST", "/api/v2/agent/monitor", `{"node_id":`+nodeA+`,"system":{"cpu":99}}`, asB)
	s.Equal(http.StatusForbidden, w.Code, w.Body.String())
	w = s.send(router, "POST", "/api/v2/agent/result", `{"task_id":"task-a","node_id":`+nodeA+`,"success":true}`, asB)
	s.Equal(http.StatusForbidden, w.Code, w.Body.String())
	// Without a node_id the result is node B's: node A's tasks are not found.
	w = s.send(router, "POST", "/api/v2/agent/result", `{"task_id":"`+mapping.TaskID+`","success":true,"output":"forged"}`, asB)
	s.Equal(http.StatusNotFound, w.Code, w.Body.String())
	w = s.send(router, "POST", "/api/v2/agent/result", `{"task_id":"task-a","success":true,"output":"forged"}`, asB)
	s.Equal(http.StatusNotFound, w.Code, w.Body.String())
	// The node_id query of the tasks poll names the node that authenticates.
	w = s.send(router, "GET", "/api/v2/agent/tasks?node_id="+nodeA, "", asB)
	s.Equal(http.StatusUnauthorized, w.Code, w.Body.String())
	w = s.send(router, "GET", "/api/v2/agent/tasks", "", asB)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	s.JSONEq(`{"tasks":[]}`, w.Body.String())
	s.Equal(model.ForwardNodeStatusOffline, s.forwardNodeStatus(s.relayA.ID))

	// Node A itself does all of it.
	asA := as(s.relayA.ID, relayAToken)
	w = s.send(router, "GET", "/api/v2/agent/tasks", "", asA)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	s.Contains(w.Body.String(), mapping.TaskID)
	s.Contains(w.Body.String(), `"task-a"`)
	w = s.send(router, "POST", "/api/v2/agent/result", `{"task_id":"`+mapping.TaskID+`","success":true,"output":"applied"}`, asA)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	var reloadedJob model.ForwardRuntimeJob
	s.Require().NoError(s.db.First(&reloadedJob, job.ID).Error)
	s.Equal(model.ForwardRuntimeJobStatusSuccess, reloadedJob.Status)
	w = s.send(router, "POST", "/api/v2/agent/result", `{"task_id":"task-a","success":true,"output":"active"}`, asA)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	var diagnostic model.AgentDiagnosticTask
	s.Require().NoError(s.db.Where("task_id = ?", "task-a").First(&diagnostic).Error)
	s.Equal(model.AgentDiagnosticTaskStatusCompleted, diagnostic.Status)
	s.Equal(s.relayA.ID, diagnostic.NodeID)
	w = s.send(router, "POST", "/api/v2/agent/monitor", `{"node_id":`+nodeA+`,"system":{"cpu":12}}`, asA)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	_, stored := handler.monitorData.Load(s.relayA.ID)
	s.True(stored)
	w = s.send(router, "POST", "/api/v2/agent/heartbeat", `{"node_id":`+nodeA+`}`, asA)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	s.Equal(model.ForwardNodeStatusOnline, s.forwardNodeStatus(s.relayA.ID))
}

// A proxy node's heartbeat marks the proxy node online. Without a live
// connection the handler assumed a forward node and wrote v2_forward_node.
func (s *AgentHTTPAuthTestSuite) TestAProxyNodesHeartbeatMarksTheProxyNodeOnline() {
	router := s.router(NewAgentHandler())
	edgeID := strconv.FormatUint(uint64(s.edge.ID), 10)
	w := s.send(router, "POST", "/api/v2/agent/heartbeat", `{"node_id":`+edgeID+`}`, as(s.edge.ID, edgeKey))
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	var edge model.Node
	s.Require().NoError(s.db.First(&edge, s.edge.ID).Error)
	s.Equal(model.NodeStatusOnline, edge.Status)
}

// A disabled proxy node's heartbeat records its liveness but leaves it
// disabled.
func (s *AgentHTTPAuthTestSuite) TestAProxyNodesHeartbeatKeepsADisabledNodeDisabled() {
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.edge.ID).Update("status", model.NodeStatusDisabled).Error)
	router := s.router(NewAgentHandler())
	edgeID := strconv.FormatUint(uint64(s.edge.ID), 10)
	w := s.send(router, "POST", "/api/v2/agent/heartbeat", `{"node_id":`+edgeID+`}`, as(s.edge.ID, edgeKey))
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	var edge model.Node
	s.Require().NoError(s.db.First(&edge, s.edge.ID).Error)
	s.Equal(model.NodeStatusDisabled, edge.Status)
	s.NotNil(edge.LastCheckAt)
}

// Query credentials authenticate as for the agent WebSocket.
func (s *AgentHTTPAuthTestSuite) TestQueryCredentialsAuthenticate() {
	router := s.router(NewAgentHandler())
	nodeA := strconv.FormatUint(uint64(s.relayA.ID), 10)
	w := s.send(router, "GET", "/api/v2/agent/tasks?node_id="+nodeA+"&api_key="+relayAToken, "", nil)
	s.Equal(http.StatusOK, w.Code, w.Body.String())
	w = s.send(router, "GET", "/api/v2/agent/tasks?node_id="+nodeA+"&token="+relayBToken, "", nil)
	s.Equal(http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestAgentHTTPAuth(t *testing.T) {
	suite.Run(t, new(AgentHTTPAuthTestSuite))
}
