package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DELETE /api/v3/topologies/:id sits in the administrator-only /api/v3 group
// and is written to the audit log like every other kernel write.
func TestKernelTopologyDeleteRouteIsAdminOnlyAndAudited(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	// Every connection to :memory: is its own database: keep one, so the
	// audit goroutine writes where the test reads.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, service.MigrateKernelModelsForTest(db))
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))
	admin, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	user, err := utils.GenerateToken(2, "user@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}

	created := do(http.MethodPost, "/api/v3/topologies", admin, `{"name":"router-mesh","service_scope":"forward"}`)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var topology struct {
		Data model.Topology `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &topology))
	path := "/api/v3/topologies/" + strconv.FormatUint(uint64(topology.Data.ID), 10)

	assert.Equal(t, http.StatusUnauthorized, do(http.MethodDelete, path, "", "").Code)
	assert.Equal(t, http.StatusForbidden, do(http.MethodDelete, path, user, "").Code, "a non-administrator is refused")

	// The audit middleware skips gin's test mode.
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	deleted := do(http.MethodDelete, path, admin, "")
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	assert.Contains(t, deleted.Body.String(), `"deleted":true`)
	require.Eventually(t, func() bool {
		var count int64
		if err := db.Model(&model.AuditLog{}).
			Where("method = ? AND path = ? AND module = ? AND action = ? AND status_code = ?", http.MethodDelete, path, "topologies", "delete", http.StatusOK).
			Count(&count).Error; err != nil {
			return false
		}
		return count == 1
	}, 5*time.Second, 20*time.Millisecond, "the delete is audited")
	gin.SetMode(previousMode)

	listed := do(http.MethodGet, "/api/v3/topologies", admin, "")
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.NotContains(t, listed.Body.String(), "router-mesh")
	assert.Equal(t, http.StatusNotFound, do(http.MethodDelete, path, admin, "").Code)
	assert.Equal(t, http.StatusNotFound, do(http.MethodGet, path+"/revisions", admin, "").Code)
	recreated := do(http.MethodPost, "/api/v3/topologies", admin, `{"name":"router-mesh","service_scope":"forward"}`)
	assert.Equal(t, http.StatusCreated, recreated.Code, recreated.Body.String())
}
