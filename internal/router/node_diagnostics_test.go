package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetup_V4NodeDiagnosticsRequireAdmin: the node diagnostics and the
// Agent upgrade plan are in the administrator-only /api/v4 group, and the
// diagnostics refuse what the whitelist does not name.
func TestSetup_V4NodeDiagnosticsRequireAdmin(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, database.GetDB().AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.AgentDiagnosticTask{}, &model.OperationLog{},
		&model.AgentTransport{}, &model.AgentUpgradeCampaign{}))
	require.NoError(t, database.GetDB().Create(&model.Node{ID: 5, Name: "diag"}).Error)

	serve := func(method, path, body, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	admin := testRouterJWT(t, cfg.JWT.Secret, true)
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v4/kernel/nodes/proxy-5/diagnostics", ""},
		{http.MethodPost, "/api/v4/kernel/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`},
		{http.MethodGet, "/api/v4/kernel/agents/upgrade-plan", ""},
	} {
		assert.Equal(t, http.StatusUnauthorized, serve(route.method, route.path, route.body, "").Code, route.path)
		assert.Equal(t, http.StatusForbidden, serve(route.method, route.path, route.body, testRouterJWT(t, cfg.JWT.Secret, false)).Code, route.path)
	}

	listed := serve(http.MethodGet, "/api/v4/kernel/nodes/proxy-5/diagnostics", "", admin)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), `"connected":false`)
	assert.Equal(t, http.StatusNotFound, serve(http.MethodGet, "/api/v4/kernel/nodes/proxy-6/diagnostics", "", admin).Code)

	offline := serve(http.MethodPost, "/api/v4/kernel/nodes/proxy-5/diagnostics", `{"action":"service_status","params":{"service":"anix-agent"}}`, admin)
	require.Equal(t, http.StatusConflict, offline.Code, offline.Body.String())
	assert.Contains(t, offline.Body.String(), `"node_offline"`)
	refused := serve(http.MethodPost, "/api/v4/kernel/nodes/proxy-5/diagnostics", `{"action":"bash"}`, admin)
	require.Equal(t, http.StatusBadRequest, refused.Code, refused.Body.String())
	assert.Contains(t, refused.Body.String(), `"action_not_allowed"`)

	var audits int64
	require.NoError(t, database.GetDB().Model(&model.OperationLog{}).Where("module = ?", "node_diagnostic").Count(&audits).Error)
	assert.EqualValues(t, 2, audits, "the kernel prefix is not audited by the middleware: the handler wrote its rows")

	plan := serve(http.MethodGet, "/api/v4/kernel/agents/upgrade-plan", "", admin)
	require.Equal(t, http.StatusOK, plan.Code, plan.Body.String())
	assert.Contains(t, plan.Body.String(), `"eligible_nodes":0`)
}
