package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The administrator routes the built-in packages made obsolete
// (single-binary design, section 4.3) answer 404, not 405, for a signed-in
// administrator and without a session alike, and the routes the remaining
// status view, the Agent and the packages need stay.
func TestSetup_RemovedPluginCenterRoutesAnswerNotFound(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, service.MigrateKernelModelsForTest(db))
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "admin@example.com", IsAdmin: 1, Password: "x", Token: "t1", UUID: "u1"}).Error)
	session, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	do := func(method, path, authorization string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}

	for _, probe := range []struct{ method, path string }{
		{http.MethodPost, "/api/v3/plugin-releases"},
		{http.MethodPost, "/api/v3/plugin-releases/1/artifact"},
		{http.MethodPut, "/api/v3/plugin-installations"},
	} {
		for name, authorization := range map[string]string{"administrator": "Bearer " + session, "anonymous": ""} {
			response := do(probe.method, probe.path, authorization)
			assert.Equal(t, http.StatusNotFound, response.Code, name+" "+probe.method+" "+probe.path+" "+response.Body.String())
		}
	}

	// What stays: the status view, the package configuration, restart, the
	// extension catalog and the operations list.
	for _, probe := range []struct{ method, path string }{
		{http.MethodGet, "/api/v3/plugins"},
		{http.MethodGet, "/api/v3/plugin-releases"},
		{http.MethodGet, "/api/v3/plugin-installations"},
		{http.MethodGet, "/api/v3/plugin-installations/1/config"},
		{http.MethodPut, "/api/v3/plugin-installations/1/config"},
		{http.MethodPost, "/api/v3/plugin-installations/1/actions"},
		{http.MethodGet, "/api/v3/extensions"},
		{http.MethodGet, "/api/v3/operations"},
	} {
		response := do(probe.method, probe.path, "Bearer "+session)
		// A handler answered: gin's own 404 for an unregistered route is
		// plain text; a handler's 404 (installation 1 does not exist) is a
		// kernel error document.
		assert.NotEqual(t, http.StatusMethodNotAllowed, response.Code, probe.method+" "+probe.path)
		if response.Code == http.StatusNotFound {
			assert.Contains(t, response.Body.String(), `"error"`, probe.method+" "+probe.path+" is not registered")
		}
	}
}
