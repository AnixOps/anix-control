package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelAlertsRouteIsAdminOnlyAndReadOnly(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))
	now := time.Now().UTC()
	expires := now.Add(3 * time.Hour)
	require.NoError(t, database.GetDB().Create(&model.KernelAlert{
		Key: "agent_certificate_expiring/proxy-4", Kind: "agent_certificate_expiring", Severity: model.KernelAlertCritical,
		SubjectKind: "node", Subject: "proxy-4", Message: "The Agent certificate of node proxy-4 ends soon.",
		Detail: `{"node":"proxy-4"}`, ExpiresAt: &expires, FirstSeenAt: now, LastSeenAt: now,
	}).Error)

	get := func(path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	assert.Equal(t, http.StatusUnauthorized, get("/api/v4/kernel/alerts", "").Code)

	user, err := utils.GenerateToken(2, "user@example.com", false, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, get("/api/v4/kernel/alerts", user).Code, "a non-administrator is refused")

	admin, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	listed := get("/api/v4/kernel/alerts", admin)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Equal(t, "no-store", listed.Header().Get("Cache-Control"))
	var answer struct {
		Data struct {
			Alerts  []map[string]any `json:"alerts"`
			Summary struct {
				Active   int `json:"active"`
				Critical int `json:"critical"`
				Warning  int `json:"warning"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &answer))
	require.Len(t, answer.Data.Alerts, 1)
	assert.Equal(t, "proxy-4", answer.Data.Alerts[0]["subject"])
	assert.Equal(t, "active", answer.Data.Alerts[0]["status"])
	assert.Equal(t, map[string]any{"node": "proxy-4"}, answer.Data.Alerts[0]["detail"])
	assert.Equal(t, 1, answer.Data.Summary.Critical)

	assert.Len(t, decodeAlerts(t, get("/api/v4/kernel/alerts?severity=warning", admin)), 0)
	assert.Len(t, decodeAlerts(t, get("/api/v4/kernel/alerts?status=resolved", admin)), 0)
	assert.Len(t, decodeAlerts(t, get("/api/v4/kernel/alerts?status=all&kind=agent_certificate_expiring&limit=1", admin)), 1)
	for _, bad := range []string{"?status=nope", "?severity=info", "?limit=0", "?limit=501", "?limit=x"} {
		assert.Equal(t, http.StatusBadRequest, get("/api/v4/kernel/alerts"+bad, admin).Code, bad)
	}

	// The alert list has no write method: alerts come from the monitor only.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		request := httptest.NewRequest(method, "/api/v4/kernel/alerts", nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		assert.NotEqual(t, http.StatusOK, recorder.Code, method)
		assert.NotEqual(t, http.StatusCreated, recorder.Code, method)
	}
}

func decodeAlerts(t *testing.T, recorder *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var answer struct {
		Data struct {
			Alerts []map[string]any `json:"alerts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	return answer.Data.Alerts
}
