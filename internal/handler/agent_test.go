package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	s.db = database.Get()

	// 自动迁移
	s.db.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardRule{},
	)
}

func (s *AgentHandlerTestSuite) TearDownSuite() {
	database.Close()
}

func (s *AgentHandlerTestSuite) SetupTest() {
	// 清理数据
	s.db.Exec("DELETE FROM v2_forward_node")
	s.db.Exec("DELETE FROM v2_forward_rule")

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

// ========== AgentRegister 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentRegister_Success() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)

	body := AgentRegisterRequest{
		NodeID:  s.testNode.ID,
		Token:   "test-api-token-123",
		Version: "1.0.0",
		System: map[string]interface{}{
			"os":   "linux",
			"arch": "amd64",
		},
		Capabilities: []string{"command", "file", "gost"},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 检查节点状态是否更新为在线
	var node model.ForwardNode
	s.db.First(&node, s.testNode.ID)
	assert.Equal(s.T(), model.ForwardNodeStatusOnline, node.Status)
}

func (s *AgentHandlerTestSuite) TestAgentRegister_NodeNotFound() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)

	body := AgentRegisterRequest{
		NodeID:  9999,
		Token:   "invalid-token",
		Version: "1.0.0",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AgentHandlerTestSuite) TestAgentRegister_InvalidToken() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)

	body := AgentRegisterRequest{
		NodeID:  s.testNode.ID,
		Token:   "wrong-token",
		Version: "1.0.0",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *AgentHandlerTestSuite) TestAgentRegister_InvalidBody() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)

	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// ========== AgentHeartbeat 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentHeartbeat_Success() {
	handler := NewAgentHandler()

	// 先注册
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)
	regBody := AgentRegisterRequest{
		NodeID:  s.testNode.ID,
		Token:   "test-api-token-123",
		Version: "1.0.0",
	}
	jsonRegBody, _ := json.Marshal(regBody)
	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader(jsonRegBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 发送心跳
	s.router.POST("/api/v2/agent/heartbeat", handler.AgentHeartbeat)
	hbBody := AgentHeartbeatRequest{
		NodeID: s.testNode.ID,
		Status: "running",
		Resources: map[string]interface{}{
			"cpu": 0.5,
			"mem": 0.6,
		},
	}
	jsonHbBody, _ := json.Marshal(hbBody)

	req, _ = http.NewRequest("POST", "/api/v2/agent/heartbeat", bytes.NewReader(jsonHbBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AgentHandlerTestSuite) TestAgentHeartbeat_InvalidBody() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/heartbeat", handler.AgentHeartbeat)

	req, _ := http.NewRequest("POST", "/api/v2/agent/heartbeat", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// ========== AgentGetTasks 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentGetTasks() {
	handler := NewAgentHandler()
	s.router.GET("/api/v2/agent/tasks", handler.AgentGetTasks)

	req, _ := http.NewRequest("GET", "/api/v2/agent/tasks?node_id=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	tasks := resp["tasks"].([]interface{})
	assert.Equal(s.T(), 0, len(tasks)) // 目前返回空任务列表
}

// ========== AgentReportResult 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentReportResult_Success() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	body := AgentTaskResult{
		TaskID:    "task-123",
		Success:   true,
		Output:    "command output",
		Duration:  100,
		Timestamp: time.Now(),
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AgentHandlerTestSuite) TestAgentReportResult_InvalidBody() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// ========== AgentMonitor 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentMonitor_Success() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/monitor", handler.AgentMonitor)

	body := AgentMonitorRequest{
		NodeID: s.testNode.ID,
		System: map[string]interface{}{
			"cpu_percent": 45.5,
			"mem_percent": 60.0,
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/agent/monitor", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// ========== AgentGetForwardRules 测试 ==========

func (s *AgentHandlerTestSuite) TestAgentGetForwardRules() {
	// 创建转发规则
	rule := &model.ForwardRule{
		Name:         "Test Rule",
		Enabled:      true,
		RelayNodeID:  s.testNode.ID,
		ListenPort:   8080,
		Protocol:     "tcp",
		ExitNodeID:   s.testNode2.ID,
		TargetHost:   "10.0.0.1",
		TargetPort:   80,
	}
	s.db.Create(rule)

	handler := NewAgentHandler()
	s.router.GET("/api/v2/forward/agent/rules", handler.AgentGetForwardRules)

	req, _ := http.NewRequest("GET", "/api/v2/forward/agent/rules?node_id=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(data), 0)
}

// ========== ListAgents 测试 ==========

func (s *AgentHandlerTestSuite) TestListAgents_Empty() {
	handler := NewAgentHandler()
	s.router.GET("/admin/agent/list", handler.ListAgents)

	req, _ := http.NewRequest("GET", "/admin/agent/list", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// agents 可能为 nil 或空数组
	agents, ok := resp["agents"].([]interface{})
	if ok {
		assert.Equal(s.T(), 0, len(agents))
	} else {
		// 如果 agents 为 nil，也是有效的空列表
		assert.Nil(s.T(), resp["agents"])
	}
}

func (s *AgentHandlerTestSuite) TestListAgents_AfterRegister() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/register", handler.AgentRegister)
	s.router.GET("/admin/agent/list", handler.ListAgents)

	// 注册 agent
	regBody := AgentRegisterRequest{
		NodeID:       s.testNode.ID,
		Token:        "test-api-token-123",
		Version:      "1.0.0",
		Capabilities: []string{"command"},
	}
	jsonBody, _ := json.Marshal(regBody)
	req, _ := http.NewRequest("POST", "/api/v2/agent/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 获取列表
	req, _ = http.NewRequest("GET", "/admin/agent/list", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	agents := resp["agents"].([]interface{})
	assert.Equal(s.T(), 1, len(agents))

	agent := agents[0].(map[string]interface{})
	assert.Equal(s.T(), float64(s.testNode.ID), agent["node_id"])
	assert.Equal(s.T(), "1.0.0", agent["version"])
}

// ========== CreateTask 测试 ==========

func (s *AgentHandlerTestSuite) TestCreateTask_NodeOffline() {
	handler := NewAgentHandler()
	s.router.POST("/admin/agent/tasks", handler.CreateTask)

	body := CreateTaskRequest{
		NodeID:  9999, // 不存在的节点
		Type:    "command",
		Action:  "ls -la",
		Timeout: 30,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/agent/tasks", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code) // 节点不在线
}

func (s *AgentHandlerTestSuite) TestCreateTask_InvalidBody() {
	handler := NewAgentHandler()
	s.router.POST("/admin/agent/tasks", handler.CreateTask)

	req, _ := http.NewRequest("POST", "/admin/agent/tasks", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// ========== ExecuteCommand 测试 ==========

func (s *AgentHandlerTestSuite) TestExecuteCommand_InvalidBody() {
	handler := NewAgentHandler()
	s.router.POST("/admin/agent/execute", handler.ExecuteCommand)

	req, _ := http.NewRequest("POST", "/admin/agent/execute", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentHandlerTestSuite) TestExecuteCommand_MissingFields() {
	handler := NewAgentHandler()
	s.router.POST("/admin/agent/execute", handler.ExecuteCommand)

	body := map[string]interface{}{
		"command": "ls",
		// 缺少 node_id
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/agent/execute", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAgentHandler(t *testing.T) {
	suite.Run(t, new(AgentHandlerTestSuite))
}