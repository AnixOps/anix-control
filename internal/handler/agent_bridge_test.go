package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// AgentBridgeHandlerTestSuite covers the durable clean_agent bridge integration in
// AgentGetTasks (node_id filtered dispatch) and AgentReportResult (job write-back).
type AgentBridgeHandlerTestSuite struct {
	suite.Suite
	router *gin.Engine
	db     *gorm.DB
	nodeA  uint
	nodeB  uint
}

func (s *AgentBridgeHandlerTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	s.db = database.Get()
	s.db.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
		&model.ForwardAgentBridgeTask{},
		&model.AgentDiagnosticTask{},
	)
}

func (s *AgentBridgeHandlerTestSuite) TearDownSuite() {
	database.Close()
}

func (s *AgentBridgeHandlerTestSuite) SetupTest() {
	s.db.Exec("DELETE FROM v2_forward_agent_bridge_task")
	s.db.Exec("DELETE FROM v2_forward_runtime_job")
	s.db.Exec("DELETE FROM v2_forward")
	s.db.Exec("DELETE FROM v2_forward_node")

	nodeA := &model.ForwardNode{Name: "relay-a", Type: model.ForwardNodeTypeRelay, Host: "10.0.0.1", Port: 22, Enabled: true}
	nodeB := &model.ForwardNode{Name: "relay-b", Type: model.ForwardNodeTypeRelay, Host: "10.0.0.2", Port: 22, Enabled: true}
	s.db.Create(nodeA)
	s.db.Create(nodeB)
	s.nodeA = nodeA.ID
	s.nodeB = nodeB.ID

	s.router = gin.New()
}

// seedBridgeJob creates a running clean_agent job + a pending bridge mapping targeting nodeID.
func (s *AgentBridgeHandlerTestSuite) seedBridgeJob(nodeID uint, action string) (*model.Forward, *model.ForwardRuntimeJob, *model.ForwardAgentBridgeTask) {
	forward := &model.Forward{
		Name:           "bridge-forward",
		InPort:         30001,
		RemoteAddr:     "127.0.0.1:9000",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendCleanAgent,
	}
	s.db.Create(forward)

	fid := forward.ID
	nid := nodeID
	now := time.Now()
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       action,
		ResourceType: "panel_forward",
		ForwardID:    &fid,
		NodeID:       &nid,
		Status:       model.ForwardRuntimeJobStatusRunning,
		StartedAt:    &now,
		Payload:      `{"resourceType":"panel_forward","backend":"clean_agent","action":"` + action + `"}`,
	}
	s.db.Create(job)

	mapping := &model.ForwardAgentBridgeTask{
		TaskID:       "forward-runtime-job-" + strconv.FormatUint(uint64(job.ID), 10),
		RuntimeJobID: job.ID,
		NodeID:       nodeID,
		ForwardID:    &fid,
		Action:       action,
		Type:         "forward",
		Params:       `{"source_job_id":` + strconv.FormatUint(uint64(job.ID), 10) + `}`,
		Status:       model.ForwardAgentBridgeTaskStatusPending,
	}
	s.db.Create(mapping)
	return forward, job, mapping
}

func (s *AgentBridgeHandlerTestSuite) getTasks(nodeID uint) []any {
	req, _ := http.NewRequest("GET", "/api/v2/agent/tasks?node_id="+strconv.FormatUint(uint64(nodeID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	tasks, _ := resp["tasks"].([]any)
	return tasks
}

func (s *AgentBridgeHandlerTestSuite) TestTargetNodePullsBridgeTaskAndMarksDispatched() {
	_, _, mapping := s.seedBridgeJob(s.nodeA, model.ForwardRuntimeJobActionCreate)
	handler := NewAgentHandler()
	s.router.GET("/api/v2/agent/tasks", handler.AgentGetTasks)

	tasks := s.getTasks(s.nodeA)
	assert.Equal(s.T(), 1, len(tasks))
	task := tasks[0].(map[string]any)
	assert.Equal(s.T(), mapping.TaskID, task["id"])
	assert.Equal(s.T(), "forward", task["type"])
	assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, task["action"])

	var reloaded model.ForwardAgentBridgeTask
	assert.NoError(s.T(), s.db.First(&reloaded, mapping.ID).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusDispatched, reloaded.Status)
	assert.True(s.T(), reloaded.Dispatched)

	// Second poll must not re-dispatch the same task.
	again := s.getTasks(s.nodeA)
	assert.Equal(s.T(), 0, len(again))
}

func (s *AgentBridgeHandlerTestSuite) TestNonTargetNodeCannotPullBridgeTask() {
	_, _, mapping := s.seedBridgeJob(s.nodeA, model.ForwardRuntimeJobActionCreate)
	handler := NewAgentHandler()
	s.router.GET("/api/v2/agent/tasks", handler.AgentGetTasks)

	// nodeB must not receive nodeA's task.
	tasks := s.getTasks(s.nodeB)
	assert.Equal(s.T(), 0, len(tasks))

	var reloaded model.ForwardAgentBridgeTask
	assert.NoError(s.T(), s.db.First(&reloaded, mapping.ID).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusPending, reloaded.Status)
}

func (s *AgentBridgeHandlerTestSuite) TestReportResultWritesBackBridgeJob() {
	forward, job, mapping := s.seedBridgeJob(s.nodeA, model.ForwardRuntimeJobActionCreate)

	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	body := AgentTaskResult{
		TaskID:    mapping.TaskID,
		NodeID:    s.nodeA,
		Success:   true,
		Output:    "applied",
		Timestamp: time.Now(),
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(s.T(), true, resp["bridged"])

	var reloadedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), s.db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedJob.Status)

	var reloadedForward model.Forward
	assert.NoError(s.T(), s.db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedForward.RuntimeStatus)

	var reloadedMapping model.ForwardAgentBridgeTask
	assert.NoError(s.T(), s.db.First(&reloadedMapping, mapping.ID).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusCompleted, reloadedMapping.Status)
}

func (s *AgentBridgeHandlerTestSuite) TestDuplicateReportIsIdempotent() {
	_, job, mapping := s.seedBridgeJob(s.nodeA, model.ForwardRuntimeJobActionCreate)

	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	post := func(success bool, output, errMsg string) map[string]any {
		body := AgentTaskResult{TaskID: mapping.TaskID, NodeID: s.nodeA, Success: success, Output: output, Error: errMsg, Timestamp: time.Now()}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		assert.Equal(s.T(), http.StatusOK, w.Code)
		var resp map[string]any
		json.Unmarshal(w.Body.Bytes(), &resp)
		return resp
	}

	first := post(true, "applied", "")
	assert.Equal(s.T(), true, first["bridged"])
	assert.Equal(s.T(), false, first["duplicate"])

	// A late failure report must not regress the terminal success state.
	second := post(false, "", "late failure")
	assert.Equal(s.T(), true, second["duplicate"])

	var reloadedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), s.db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedJob.Status)
	assert.Equal(s.T(), "applied", reloadedJob.Result)
	assert.Empty(s.T(), reloadedJob.Error)
}

func (s *AgentBridgeHandlerTestSuite) TestReportResultMissingTaskIDRejected() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	// task_id is required (binding) — malformed report is explicitly rejected, not swallowed.
	req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader([]byte(`{"success":true}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AgentBridgeHandlerTestSuite) TestNonBridgeResultUsesMemoryPath() {
	handler := NewAgentHandler()
	s.router.POST("/api/v2/agent/result", handler.AgentReportResult)

	// An admin/manual task_id that is not a bridge mapping must fall through to the memory path.
	body := AgentTaskResult{TaskID: "task-999", NodeID: s.nodeA, Success: true, Output: "ok", Timestamp: time.Now()}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/v2/agent/result", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	_, bridged := resp["bridged"]
	assert.False(s.T(), bridged)
}

func TestAgentBridgeHandler(t *testing.T) {
	suite.Run(t, new(AgentBridgeHandlerTestSuite))
}
