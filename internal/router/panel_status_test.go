package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanelStatusRouteIsAdminOnly(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.MigrateKernelModelsForTest(database.GetDB()))
	// Control creates v5_acme_cache with the migration 0003_acme_cache.
	require.NoError(t, database.GetDB().AutoMigrate(&model.ACMECacheEntry{}))
	call := func(method, path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	assert.Equal(t, http.StatusUnauthorized, call(http.MethodGet, "/api/v5/panel/status", "").Code)
	user, err := utils.GenerateToken(2, "user@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, call(http.MethodGet, "/api/v5/panel/status", user).Code)
	assert.Equal(t, http.StatusForbidden, call(http.MethodPost, "/api/v5/panel/certificates/retry", user).Code)

	admin, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	// No manager in this process and nothing stored: unavailable.
	panelcert.SetDefault(nil)
	assert.Equal(t, http.StatusServiceUnavailable, call(http.MethodGet, "/api/v5/panel/status", admin).Code)

	// The running manager's status.
	manager, err := panelcert.New(panelcert.Options{Mode: "external", Domain: "panel.example.com"})
	require.NoError(t, err)
	require.NoError(t, manager.Start(context.Background()))
	panelcert.SetDefault(manager)
	t.Cleanup(func() { panelcert.SetDefault(nil) })
	response := call(http.MethodGet, "/api/v5/panel/status", admin)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var answer struct {
		Data struct {
			Certificate panelcert.Status `json:"certificate"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
	assert.Equal(t, panelcert.StateNotConfigured, answer.Data.Certificate.State)
	assert.Equal(t, "external", answer.Data.Certificate.Mode)
	assert.Equal(t, "panel.example.com", answer.Data.Certificate.Domain)

	retried := call(http.MethodPost, "/api/v5/panel/certificates/retry", admin)
	require.Equal(t, http.StatusAccepted, retried.Code, retried.Body.String())
	assert.Contains(t, retried.Body.String(), `"retrying":[]`)

	// Without a manager the request is stored for the running Control.
	panelcert.SetDefault(nil)
	require.Equal(t, http.StatusAccepted, call(http.MethodPost, "/api/v5/panel/certificates/retry", admin).Code)
	pending, err := panelcert.RetryPending(context.Background(), database.GetDB())
	require.NoError(t, err)
	assert.True(t, pending)
}
