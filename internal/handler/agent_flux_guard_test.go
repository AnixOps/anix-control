package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// catalogProbe counts the catalog lookups a database runs
// (Migrator().HasTable reads sqlite_master) and, with fail set, breaks each
// one the way a lost connection or a statement timeout would. HasTable
// cannot tell that from a table that is not there: it answers false.
type catalogProbe struct {
	queries atomic.Int32
	fail    atomic.Bool
}

func newCatalogProbe(t *testing.T, db *gorm.DB) *catalogProbe {
	t.Helper()
	probe := &catalogProbe{}
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:catalog_probe", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "sqlite_master") {
			return
		}
		probe.queries.Add(1)
		if probe.fail.Load() {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT count(*) FROM catalog_lookup_failed")
			tx.Statement.Vars = nil
		}
	}))
	return probe
}

// refuseStatements makes the queries on table fail with err while the
// returned switch is on: a statement the database refuses for any reason
// but a missing table.
func refuseStatements(t *testing.T, db *gorm.DB, table string, err error) *atomic.Bool {
	t.Helper()
	var on atomic.Bool
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:refuse_"+table, func(tx *gorm.DB) {
		if on.Load() && tx.Statement.Table == table {
			_ = tx.AddError(err)
		}
	}))
	return &on
}

// The legacy HTTP agent's guards for a dropped flux table must not change
// what a database that still has the tables answers. Before the guards, a
// failed statement was a 500 (the agent retries) or a logged warning; the
// guard that asked HasTable first took a failed catalog lookup for a
// dropped table, so a bridged job's result was sent on as a diagnostic
// task, which created a row for the unknown id and was acknowledged with
// 200: the agent never retried and the job was failed by the timeout.
func TestLegacyAgentEndpointsKeepFailuresOfPresentTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := fluxAgentDB(t)
	node := model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "192.0.2.11", Port: 22, APIToken: "agent-token-present", Enabled: true}
	require.NoError(t, db.Create(&node).Error)
	probe := newCatalogProbe(t, db)
	refused := errors.New("database is locked")
	refuse := refuseStatements(t, db, "v2_forward_agent_bridge_task", refused)
	agents := &AgentHandler{db: db, diagnosticSvc: service.NewAgentDiagnosticTaskService(db)}

	router := gin.New()
	router.GET("/api/v2/agent/tasks", agents.AgentGetTasks)
	router.POST("/api/v2/agent/result", agents.AgentReportResult)
	router.GET("/api/v2/forward/agent/rules", agents.AgentGetForwardRules)
	serve := func(method, path string, body []byte) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Node-ID", strconv.FormatUint(uint64(node.ID), 10))
		req.Header.Set("X-API-Key", node.APIToken)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}
	var logged strings.Builder
	previous := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(previous) })

	ports := 30000
	seedBridgeTask := func(t *testing.T) *model.ForwardAgentBridgeTask {
		t.Helper()
		ports++
		forward := &model.Forward{Name: "bridge-forward", InPort: ports, RemoteAddr: "127.0.0.1:9000", Status: model.ForwardStatusActive, RuntimeBackend: model.ForwardRuntimeBackendCleanAgent}
		require.NoError(t, db.Create(forward).Error)
		now := time.Now()
		job := &model.ForwardRuntimeJob{Backend: model.ForwardRuntimeBackendCleanAgent, Action: model.ForwardRuntimeJobActionCreate, ResourceType: "panel_forward",
			ForwardID: &forward.ID, NodeID: &node.ID, Status: model.ForwardRuntimeJobStatusRunning, StartedAt: &now}
		require.NoError(t, db.Create(job).Error)
		mapping := &model.ForwardAgentBridgeTask{TaskID: "forward-runtime-job-" + strconv.FormatUint(uint64(job.ID), 10), RuntimeJobID: job.ID, NodeID: node.ID,
			ForwardID: &forward.ID, Action: model.ForwardRuntimeJobActionCreate, Type: "forward", Params: "{}", Status: model.ForwardAgentBridgeTaskStatusPending}
		require.NoError(t, db.Create(mapping).Error)
		return mapping
	}
	report := func(t *testing.T, taskID string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(AgentTaskResult{TaskID: taskID, NodeID: node.ID, Success: true, Output: "applied", Timestamp: time.Now()})
		require.NoError(t, err)
		return serve(http.MethodPost, "/api/v2/agent/result", body)
	}
	diagnosticRows := func(t *testing.T, taskID string) int64 {
		t.Helper()
		var count int64
		require.NoError(t, db.Model(&model.AgentDiagnosticTask{}).Where("task_id = ?", taskID).Count(&count).Error)
		return count
	}
	taskIDs := func(t *testing.T, recorder *httptest.ResponseRecorder) []string {
		t.Helper()
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer struct {
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
		ids := []string{}
		for _, task := range answer.Tasks {
			ids = append(ids, task.ID)
		}
		return ids
	}

	t.Run("a bridged result is written back although the catalog lookup fails", func(t *testing.T) {
		mapping := seedBridgeTask(t)
		probe.fail.Store(true)
		t.Cleanup(func() { probe.fail.Store(false) })

		recorder := report(t, mapping.TaskID)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
		assert.Equal(t, true, answer["bridged"], recorder.Body.String())
		assert.Zero(t, diagnosticRows(t, mapping.TaskID), "the result is not an unknown diagnostic task")

		var reloaded model.ForwardAgentBridgeTask
		require.NoError(t, db.First(&reloaded, mapping.ID).Error)
		assert.Equal(t, model.ForwardAgentBridgeTaskStatusCompleted, reloaded.Status)
	})

	t.Run("a result whose bridge lookup fails is answered 500 and nothing is created", func(t *testing.T) {
		mapping := seedBridgeTask(t)
		refuse.Store(true)
		t.Cleanup(func() { refuse.Store(false) })

		recorder := report(t, mapping.TaskID)
		refuse.Store(false)
		assert.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
		assert.Contains(t, recorder.Body.String(), refused.Error())
		assert.Zero(t, diagnosticRows(t, mapping.TaskID), "the agent retries; no row stands in for the job")

		var reloaded model.ForwardAgentBridgeTask
		require.NoError(t, db.First(&reloaded, mapping.ID).Error)
		assert.Equal(t, model.ForwardAgentBridgeTaskStatusPending, reloaded.Status)
		// The retry, once the database answers, is the bridged result.
		assert.Equal(t, http.StatusOK, report(t, mapping.TaskID).Code)
	})

	t.Run("a poll delivers the bridge task although the catalog lookup fails", func(t *testing.T) {
		mapping := seedBridgeTask(t)
		probe.fail.Store(true)
		t.Cleanup(func() { probe.fail.Store(false) })
		logged.Reset()

		assert.Equal(t, []string{mapping.TaskID}, taskIDs(t, serve(http.MethodGet, "/api/v2/agent/tasks", nil)))
		assert.NotContains(t, logged.String(), "[WARN]")
	})

	t.Run("a poll whose bridge lookup fails answers no tasks and logs why", func(t *testing.T) {
		mapping := seedBridgeTask(t)
		refuse.Store(true)
		t.Cleanup(func() { refuse.Store(false) })
		logged.Reset()

		assert.Empty(t, taskIDs(t, serve(http.MethodGet, "/api/v2/agent/tasks", nil)))
		assert.Contains(t, logged.String(), "[WARN] agent tasks: load bridge tasks for node "+strconv.FormatUint(uint64(node.ID), 10)+" failed")
		assert.Contains(t, logged.String(), refused.Error())

		refuse.Store(false)
		assert.Equal(t, []string{mapping.TaskID}, taskIDs(t, serve(http.MethodGet, "/api/v2/agent/tasks", nil)), "the task is still pending")
	})

	t.Run("a forward node gets its rules although the catalog lookup fails", func(t *testing.T) {
		rule := model.ForwardRule{Name: "r1", Enabled: true, RelayNodeID: node.ID, ExitNodeID: node.ID, ListenPort: 20443, Protocol: "tcp", TargetHost: "198.51.100.9", TargetPort: 443}
		require.NoError(t, db.Create(&rule).Error)
		probe.fail.Store(true)
		t.Cleanup(func() { probe.fail.Store(false) })

		recorder := serve(http.MethodGet, "/api/v2/forward/agent/rules", nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer struct {
			Data []ForwardRuleForAgent `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
		require.Len(t, answer.Data, 1)
		assert.Equal(t, rule.ID, answer.Data[0].ID)
		assert.Equal(t, 20443, answer.Data[0].ListenPort)
	})

	t.Run("no path runs a catalog lookup of its own", func(t *testing.T) {
		mapping := seedBridgeTask(t)
		probe.queries.Store(0)

		require.Equal(t, http.StatusOK, serve(http.MethodGet, "/api/v2/forward/agent/rules", nil).Code)
		assert.Equal(t, []string{mapping.TaskID}, taskIDs(t, serve(http.MethodGet, "/api/v2/agent/tasks", nil)))
		recorder := report(t, mapping.TaskID)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Zero(t, probe.queries.Load(), "a poll, a report and a rules request each run only their own statements")
	})
}
