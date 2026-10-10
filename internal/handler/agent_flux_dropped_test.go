package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fluxDroppedAgentDB is a database after `forward legacy drop`: the flux
// tables were created and are gone, the node inventory and the diagnostic
// task table stay. Its GORM logger writes to the returned buffer, so a test
// sees the failed statements the handlers would log.
func fluxDroppedAgentDB(t *testing.T) (*gorm.DB, *strings.Builder) {
	t.Helper()
	statements := &strings.Builder{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.New(log.New(statements, "", 0), logger.Config{LogLevel: logger.Error, IgnoreRecordNotFoundError: true}),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&model.ForwardNode{}, &model.AgentDiagnosticTask{},
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{},
		&model.SpeedLimit{}, &model.ForwardRuntimeJob{}, &model.ForwardTrafficCursor{}, &model.ForwardAgentBridgeTask{},
		&model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{}, &model.ForwardStats{},
	))
	for _, table := range forwardlegacy.DropTables {
		require.NoError(t, db.Migrator().DropTable(table), table)
	}
	statements.Reset()
	return db, statements
}

// The legacy HTTP agent keeps polling and reporting after the drop. Every
// task poll and every result report used to read the bridge task table:
// the poll logged a warning each time, and the report answered 500 whatever
// the task, so a proxy node's diagnostics stopped with the tables.
func TestLegacyAgentEndpointsAfterTheFluxTablesWereDropped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, statements := fluxDroppedAgentDB(t)
	node := model.ForwardNode{Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "192.0.2.10", Port: 22, APIToken: "agent-token-after-drop", Enabled: true}
	require.NoError(t, db.Create(&node).Error)
	// What NewAgentHandler builds around database.Get(), on this database.
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

	t.Run("task poll answers no bridge tasks and logs nothing", func(t *testing.T) {
		statements.Reset()
		recorder := serve(http.MethodGet, "/api/v2/agent/tasks", nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer struct {
			Tasks []any `json:"tasks"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
		assert.Empty(t, answer.Tasks)
		assert.NotContains(t, logged.String(), "[WARN]")
		assert.NotContains(t, statements.String(), "no such table")
	})

	t.Run("a result report is not a bridge task", func(t *testing.T) {
		statements.Reset()
		body, err := json.Marshal(AgentTaskResult{TaskID: "diagnostic-task-1", NodeID: node.ID, Success: true, Output: "ok", Timestamp: time.Now()})
		require.NoError(t, err)
		recorder := serve(http.MethodPost, "/api/v2/agent/result", body)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
		assert.NotContains(t, answer, "bridged")
		assert.NotContains(t, statements.String(), "no such table")
	})

	t.Run("a forward node's rules are empty", func(t *testing.T) {
		statements.Reset()
		recorder := serve(http.MethodGet, "/api/v2/forward/agent/rules", nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.NotContains(t, statements.String(), "no such table")
	})
}
