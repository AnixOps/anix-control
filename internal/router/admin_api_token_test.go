package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestSetup_AdminAPITokens proves the wiring: tokens authenticate the four
// administrator groups, are refused everywhere else, and are managed from a
// session only. The token rules themselves are tested in
// internal/adminapitoken and internal/middleware.
func TestSetup_AdminAPITokens(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}, &model.UserMFA{}))
	const password = "router test password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		ID: 1, Email: "admin@example.com", IsAdmin: 1, Password: string(hash), Token: "t1", UUID: "u1",
	}).Error)
	session, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	do := func(method, path, authorization, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	create := func(scope string) string {
		body, err := json.Marshal(map[string]any{"name": "router", "scope": scope, "password": password})
		require.NoError(t, err)
		response := do(http.MethodPost, "/api/v4/kernel/api-tokens", "Bearer "+session, string(body))
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		var answer struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
		require.True(t, adminapitoken.WellFormed(answer.Data.Token))
		return "Bearer " + answer.Data.Token
	}
	adminToken, readToken := create("admin"), create("read")

	// Reads on every administrator group; none is refused as unauthenticated.
	for _, path := range []string{
		"/api/v3/access-groups?scope_id=forward",
		"/api/v4/kernel/route-modes/revisions",
		"/api/v2/admin/forward/runtime/status",
		"/api/v2/admin/agent/list",
	} {
		for name, token := range map[string]string{"admin": adminToken, "read": readToken} {
			response := do(http.MethodGet, path, token, "")
			assert.NotEqual(t, http.StatusUnauthorized, response.Code, name+" "+path+" "+response.Body.String())
			assert.NotEqual(t, http.StatusForbidden, response.Code, name+" "+path+" "+response.Body.String())
		}
		assert.Equal(t, http.StatusUnauthorized, do(http.MethodGet, path, "Bearer "+adminapitoken.Prefix+strings.Repeat("Z", 43), "").Code, path)
	}

	// Writes: the admin scope reaches the handler, the read scope does not.
	group := `{"scope_id":"forward","name":"by-token","enabled":true}`
	assert.Equal(t, http.StatusCreated, do(http.MethodPost, "/api/v3/access-groups", adminToken, group).Code)
	for _, path := range []string{"/api/v3/access-groups", "/api/v4/kernel/route-modes", "/api/v2/admin/agent/tasks", "/api/v2/admin/users"} {
		response := do(http.MethodPost, path, readToken, group)
		assert.Equal(t, http.StatusForbidden, response.Code, path)
		assert.Contains(t, response.Body.String(), "api_token_forbidden", path)
	}

	// Tokens are managed from a session only.
	for method, path := range map[string]string{
		http.MethodGet: "/api/v4/kernel/api-tokens", http.MethodPost: "/api/v4/kernel/api-tokens", http.MethodDelete: "/api/v4/kernel/api-tokens/x",
	} {
		assert.Equal(t, http.StatusForbidden, do(method, path, adminToken, "{}").Code, "admin token "+method)
		assert.Equal(t, http.StatusForbidden, do(method, path, readToken, "{}").Code, "read token "+method)
	}
	listed := do(http.MethodGet, "/api/v4/kernel/api-tokens", "Bearer "+session, "")
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.NotContains(t, listed.Body.String(), strings.TrimPrefix(adminToken, "Bearer "))

	// Not on user routes, nor on the compatibility group that shares their
	// authentication.
	for _, probe := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/user/profile"}, {http.MethodGet, "/api/v2/user/dashboard"}, {http.MethodPost, "/api/v2/user/reset"},
	} {
		response := do(probe.method, probe.path, adminToken, "{}")
		assert.Equal(t, http.StatusUnauthorized, response.Code, probe.path)
		assert.NotContains(t, response.Body.String(), "api_token", probe.path)
	}

	// The body of the create request never contains the token in an audit entry.
	var entries []model.OperationLog
	require.NoError(t, db.Where("module = ?", adminapitoken.AuditModule).Find(&entries).Error)
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		assert.NotContains(t, entry.Content, strings.TrimPrefix(adminToken, "Bearer "))
		assert.False(t, bytes.Contains([]byte(entry.Content), []byte(password)))
	}
}
