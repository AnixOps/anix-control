package handler

import (
	"net/http"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExitPolicyAPI(t *testing.T) {
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.NodeExitPolicy{}))
	handler := &ExitPolicyHandler{db: func() *gorm.DB { return env.db }}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/nodes/:id/exit-policy", handler.Get)
	router.PUT("/nodes/:id/exit-policy", handler.Set)

	response := serveJSON(router, http.MethodGet, "/nodes/proxy-5/exit-policy", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"block_mainland":false`)
	for _, bad := range []string{`{}`, `{"block_mainland":"yes"}`} {
		response = serveJSON(router, http.MethodPut, "/nodes/proxy-5/exit-policy", bad)
		require.Equal(t, http.StatusBadRequest, response.Code, bad)
	}
	response = serveJSON(router, http.MethodPut, "/nodes/forward-5/exit-policy", `{"block_mainland":true}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	response = serveJSON(router, http.MethodPut, "/nodes/proxy-404/exit-policy", `{"block_mainland":true}`)
	require.Equal(t, http.StatusNotFound, response.Code)

	response = serveJSON(router, http.MethodPut, "/nodes/proxy-5/exit-policy", `{"block_mainland":true}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"block_mainland":true`)
	assert.Contains(t, response.Body.String(), `"mainland_blocked_total":0`)
	var audits []model.OperationLog
	require.NoError(t, env.db.Where("action = ?", "exit_policy_set").Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.JSONEq(t, `{"node":"proxy-5","block_mainland":true}`, audits[0].Content)
}
