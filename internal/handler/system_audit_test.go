package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemAuditTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()

	db := initTestDB()
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	require.NoError(t, db.Exec("DELETE FROM v2_operation_log").Error)

	router := gin.New()
	handler := NewSystemHandler()
	router.GET("/admin/system/audit-logs", handler.GetAuditLogs)

	return db, router
}

func decodeAuditResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func extractAuditList(t *testing.T, body map[string]any) []any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	list, ok := data["list"].([]any)
	require.True(t, ok)
	return list
}

func TestGetAuditLogsSupportsFilterAndPagination(t *testing.T) {
	db, router := setupSystemAuditTest(t)

	now := time.Now()
	seed := []model.OperationLog{
		{Username: "admin", Action: "update", Module: "system", TargetType: "system_config", Content: `{"key":"a"}`, Status: 1, CreatedAt: now.Add(-2 * time.Minute)},
		{Username: "admin", Action: "update", Module: "system", TargetType: "backup_config", Content: `{"key":"b"}`, Status: 1, CreatedAt: now.Add(-1 * time.Minute)},
		{Username: "admin", Action: "delete", Module: "system", TargetType: "backup_record", Content: `{"key":"c"}`, Status: 1, CreatedAt: now},
		{Username: "admin", Action: "update", Module: "forward", TargetType: "rule", Content: `{"key":"d"}`, Status: 1, CreatedAt: now},
	}
	require.NoError(t, db.Create(&seed).Error)

	req := httptest.NewRequest(http.MethodGet, "/admin/system/audit-logs?action=update&target_type=backup_config&page=1&page_size=1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := decodeAuditResponse(t, recorder)
	list := extractAuditList(t, body)

	assert.Equal(t, float64(1), body["total"])
	assert.Equal(t, float64(1), body["page"])
	assert.Equal(t, float64(1), body["page_size"])
	require.Len(t, list, 1)

	row, ok := list[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "backup_config", row["target_type"])
	assert.Equal(t, "update", row["action"])
	assert.Equal(t, "system", row["module"])
}

func TestGetAuditLogsRedactsSensitiveContent(t *testing.T) {
	db, router := setupSystemAuditTest(t)

	seed := []model.OperationLog{
		{
			Username:   "admin",
			Action:     "update",
			Module:     "system",
			TargetType: "system_config",
			Content:    `{"token":"plain-secret-token","note":"ok"}`,
			Status:     1,
		},
		{
			Username:   "admin",
			Action:     "update",
			Module:     "system",
			TargetType: "backup_config",
			Content:    `api_key=super-secret-key`,
			Status:     1,
		},
	}
	require.NoError(t, db.Create(&seed).Error)

	req := httptest.NewRequest(http.MethodGet, "/admin/system/audit-logs?page=1&page_size=20", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := decodeAuditResponse(t, recorder)
	list := extractAuditList(t, body)
	require.Len(t, list, 2)

	for _, item := range list {
		row, ok := item.(map[string]any)
		require.True(t, ok)
		content, ok := row["content"].(string)
		require.True(t, ok)
		assert.NotContains(t, content, "plain-secret-token")
		assert.NotContains(t, content, "super-secret-key")
	}
}
