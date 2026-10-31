package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// AgentHandlerTestSuite Agent Handler 测试套件
type AgentHandlerTestSuite struct {
	suite.Suite
	router    *gin.Engine
	db        *gorm.DB
	testNode  *model.ForwardNode
	testNode2 *model.ForwardNode
}

func (s *AgentHandlerTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	s.Require().NoError(database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}))
	s.db = database.Get()

	// 自动迁移
	s.Require().NoError(s.db.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardRule{},
		&model.ForwardRuntimeJob{},
		&model.ForwardAgentBridgeTask{},
		&model.AgentDiagnosticTask{},
	))
}

func (s *AgentHandlerTestSuite) TearDownSuite() {
	s.Require().NoError(database.Close())
}

func (s *AgentHandlerTestSuite) SetupTest() {
	// 清理数据
	s.db.Exec("DELETE FROM v2_forward_node")
	s.db.Exec("DELETE FROM v2_forward_rule")
	s.db.Exec("DELETE FROM v2_agent_diagnostic_task")
	s.db.Exec("DELETE FROM v2_forward_agent_bridge_task")

	// 创建测试节点
	s.testNode = &model.ForwardNode{
		Name:     "Test Relay Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "192.168.1.100",
		Port:     443,
		APIPort:  18080,
		APIToken: "test-api-token-123",
		Status:   model.ForwardNodeStatusOffline,
		Enabled:  true,
	}
	s.db.Create(s.testNode)

	s.testNode2 = &model.ForwardNode{
		Name:     "Test Exit Node",
		Type:     model.ForwardNodeTypeExit,
		Host:     "192.168.1.200",
		Port:     443,
		APIPort:  18080,
		APIToken: "test-api-token-456",
		Status:   model.ForwardNodeStatusOffline,
		Enabled:  true,
	}
	s.db.Create(s.testNode2)

	s.router = gin.New()
}

func (s *AgentHandlerTestSuite) post(path string, handler gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	s.router.POST(path, handler)
	payload, ok := body.([]byte)
	if !ok {
		payload, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest("POST", path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func (s *AgentHandlerTestSuite) get(path string, route string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	s.router.GET(route, handler)
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// The monitor snapshots came from the retired REST channel: the route
// answers 404 for every node, and 400 without a valid node_id.
func (s *AgentHandlerTestSuite) TestGetMonitor_Retired() {
	handler := NewAgentHandler()
	w := s.get("/admin/agent/monitor?node_id=1", "/admin/agent/monitor", handler.GetMonitor)
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
	w = httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/agent/monitor", nil)
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentHandlerTestSuite) TestGetTaskResult_Success() {
	s.Require().NoError(s.db.Create(&model.AgentDiagnosticTask{
		TaskID: "task-xyz-1", NodeID: s.testNode.ID, Action: "service_status", Status: "completed", Success: true, Output: "ok", DurationMS: 66,
	}).Error)
	handler := NewAgentHandler()
	w := s.get("/admin/agent/tasks/task-xyz-1", "/admin/agent/tasks/:task_id", handler.GetTaskResult)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "task-xyz-1", data["task_id"])
	assert.Equal(s.T(), float64(s.testNode.ID), data["node_id"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AgentHandlerTestSuite) TestGetTaskResult_NotFound() {
	handler := NewAgentHandler()
	w := s.get("/admin/agent/tasks/not-exists", "/admin/agent/tasks/:task_id", handler.GetTaskResult)
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AgentHandlerTestSuite) TestListAgents_Empty() {
	handler := NewAgentHandler()
	w := s.get("/admin/agent/list", "/admin/agent/list", handler.ListAgents)
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.Empty(s.T(), resp["data"].(map[string]any)["agents"])
}

func (s *AgentHandlerTestSuite) TestListDiagnosticTasks() {
	handler := NewAgentHandler()
	w := s.get("/admin/agent/tasks?limit=50", "/admin/agent/tasks", handler.ListDiagnosticTasks)
	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Empty(s.T(), decodePanelTestResponse(s.T(), w)["data"].([]any))

	s.Require().NoError(s.db.Create(&model.AgentDiagnosticTask{TaskID: "task-1", NodeID: s.testNode.ID, Action: "service_status", Status: "completed"}).Error)
	s.router = gin.New()
	w = s.get("/admin/agent/tasks", "/admin/agent/tasks", handler.ListDiagnosticTasks)
	assert.Len(s.T(), decodePanelTestResponse(s.T(), w)["data"].([]any), 1)
}

// A node without an Agent Control stream session is offline: the task is
// not sent and none is recorded.
func (s *AgentHandlerTestSuite) TestCreateTask_NodeOffline() {
	handler := NewAgentHandler()
	w := s.post("/admin/agent/tasks", handler.CreateTask, CreateTaskRequest{NodeID: 9999, Type: "diagnostic", Action: "service_status", Timeout: 30})
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
	var tasks int64
	s.Require().NoError(s.db.Model(&model.AgentDiagnosticTask{}).Count(&tasks).Error)
	assert.Zero(s.T(), tasks)
}

// Only diagnostic tasks are sent to agents, as for ExecuteCommand.
func (s *AgentHandlerTestSuite) TestCreateTask_RefusesOtherTaskTypes() {
	handler := NewAgentHandler()
	s.router.POST("/admin/agent/tasks", handler.CreateTask)
	for _, taskType := range []string{"forward", "shell", "Diagnostic", ""} {
		body, _ := json.Marshal(map[string]any{
			"node_id": s.testNode.ID, "type": taskType, "action": "service_restart",
			"params": map[string]any{"service": "anix-agent"},
		})
		req, _ := http.NewRequest("POST", "/admin/agent/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		assert.Equal(s.T(), http.StatusBadRequest, w.Code, "%q: %s", taskType, w.Body.String())
	}
	var tasks int64
	s.Require().NoError(s.db.Model(&model.AgentDiagnosticTask{}).Count(&tasks).Error)
	assert.Zero(s.T(), tasks, "a task of another type was created")
}

func (s *AgentHandlerTestSuite) TestCreateTask_InvalidBody() {
	w := s.post("/admin/agent/tasks", NewAgentHandler().CreateTask, []byte("invalid"))
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentHandlerTestSuite) TestExecuteCommand_InvalidBody() {
	w := s.post("/admin/agent/execute", NewAgentHandler().ExecuteCommand, []byte("invalid"))
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentHandlerTestSuite) TestExecuteCommand_NodeOffline() {
	w := s.post("/admin/agent/execute", NewAgentHandler().ExecuteCommand,
		ExecuteCommandRequest{NodeID: s.testNode.ID, Action: "service_status", Params: map[string]any{"service": "anix-agent"}, Timeout: 30})
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentHandlerTestSuite) TestExecuteCommand_MissingFields() {
	w := s.post("/admin/agent/execute", NewAgentHandler().ExecuteCommand, map[string]any{"command": "ls"})
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAgentHandler(t *testing.T) {
	suite.Run(t, new(AgentHandlerTestSuite))
}
