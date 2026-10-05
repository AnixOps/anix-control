package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetup_V4NodeTrafficRequiresAdmin: the proxy node traffic series is in
// the administrator-only /api/v4 group.
func TestSetup_V4NodeTrafficRequiresAdmin(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, database.GetDB().AutoMigrate(&model.Node{}, &model.TrafficLog{}, &model.StatServer{}))
	require.NoError(t, database.GetDB().Create(&model.Node{ID: 5, Name: "traffic", APIKey: "traffic-key"}).Error)

	serve := func(path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	path := "/api/v4/kernel/nodes/5/traffic"
	assert.Equal(t, http.StatusUnauthorized, serve(path, "").Code)
	assert.Equal(t, http.StatusForbidden, serve(path, testRouterJWT(t, cfg.JWT.Secret, false)).Code)

	admin := testRouterJWT(t, cfg.JWT.Secret, true)
	answered := serve(path+"?granularity=day", admin)
	require.Equal(t, http.StatusOK, answered.Code, answered.Body.String())
	assert.Contains(t, answered.Body.String(), `"granularity":"day"`)
	assert.Equal(t, http.StatusNotFound, serve("/api/v4/kernel/nodes/6/traffic", admin).Code)
}
