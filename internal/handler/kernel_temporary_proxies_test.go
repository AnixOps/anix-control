package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTemporaryProxyAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.NodeTemporaryProxy{}, &model.NodeProtocol{}))
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	handler := &TemporaryProxyHandler{db: func() *gorm.DB { return env.db }, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/nodes/:id/temporary-proxies", handler.List)
	router.POST("/nodes/:id/temporary-proxies", handler.Open)
	router.DELETE("/nodes/:id/temporary-proxies/:proxy_id", handler.Close)

	response := serveJSON(router, http.MethodGet, "/nodes/proxy-5/temporary-proxies", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.JSONEq(t, `[]`, string(dataOf(t, response.Body.Bytes())))

	for path, status := range map[string]int{
		"/nodes/forward-5/temporary-proxies": http.StatusBadRequest,
		"/nodes/proxy-404/temporary-proxies": http.StatusNotFound,
	} {
		response = serveJSON(router, http.MethodPost, path, `{"listen":"10.0.0.1","port":1080}`)
		assert.Equal(t, status, response.Code, path)
	}
	for _, bad := range []string{`{"listen":"0.0.0.0","port":1080}`, `{"listen":"203.0.113.1","port":1080}`,
		`{"listen":"10.0.0.1","port":0}`, `{"listen":"10.0.0.1","port":1080,"minutes":1441}`,
		`{"listen":"10.0.0.1","port":1080,"minutes":-1}`, `{"listen":10}`} {
		response = serveJSON(router, http.MethodPost, "/nodes/proxy-5/temporary-proxies", bad)
		assert.Equal(t, http.StatusBadRequest, response.Code, bad)
		assert.Equal(t, "invalid_request", errorCode(t, response), bad)
	}

	response = serveJSON(router, http.MethodPost, "/nodes/proxy-5/temporary-proxies", `{"listen":"10.126.126.1","port":1080,"minutes":30}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var opened struct {
		ID        uint      `json:"id"`
		Listen    string    `json:"listen"`
		Port      int       `json:"port"`
		Username  string    `json:"username"`
		Password  string    `json:"password"`
		ExpiresAt time.Time `json:"expires_at"`
		CreatedBy *uint     `json:"created_by"`
	}
	require.NoError(t, json.Unmarshal(dataOf(t, response.Body.Bytes()), &opened))
	assert.Equal(t, "10.126.126.1", opened.Listen)
	assert.Equal(t, 1080, opened.Port)
	assert.Len(t, opened.Password, 24)
	assert.Equal(t, now.Add(30*time.Minute), opened.ExpiresAt.UTC())
	require.NotNil(t, opened.CreatedBy)
	assert.Equal(t, uint(7), *opened.CreatedBy)

	response = serveJSON(router, http.MethodPost, "/nodes/proxy-5/temporary-proxies", `{"listen":"10.126.126.1","port":1080}`)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "port_taken", errorCode(t, response))

	// The list never carries the password.
	response = serveJSON(router, http.MethodGet, "/nodes/proxy-5/temporary-proxies", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), opened.Username)
	assert.NotContains(t, response.Body.String(), opened.Password)
	assert.NotContains(t, response.Body.String(), "password")

	id := strconv.FormatUint(uint64(opened.ID), 10)
	response = serveJSON(router, http.MethodDelete, "/nodes/proxy-5/temporary-proxies/x", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)
	response = serveJSON(router, http.MethodDelete, "/nodes/proxy-5/temporary-proxies/0", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)
	response = serveJSON(router, http.MethodDelete, "/nodes/proxy-5/temporary-proxies/"+id, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"closed_at":"2026-11-25T08:00:00Z"`)
	assert.NotContains(t, response.Body.String(), opened.Password)
	response = serveJSON(router, http.MethodDelete, "/nodes/proxy-5/temporary-proxies/"+id, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "temporary_proxy_not_found", errorCode(t, response))

	var audits []model.OperationLog
	require.NoError(t, env.db.Where("module = ?", "temporary_proxy").Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "temporary_proxy_open", audits[0].Action)
	assert.Equal(t, "temporary_proxy_close", audits[1].Action)
	content := `{"node":"proxy-5","proxy_id":` + id + `,"listen":"10.126.126.1","port":1080,"expires_at":"2026-11-25T08:30:00Z"}`
	assert.JSONEq(t, content, audits[0].Content)
	assert.JSONEq(t, content, audits[1].Content)
	for _, audit := range audits {
		assert.NotContains(t, audit.Content, opened.Password)
		require.NotNil(t, audit.UserID)
		assert.Equal(t, uint(7), *audit.UserID)
	}
}

// dataOf answers the "data" member of a kernel answer.
func dataOf(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Data
}
