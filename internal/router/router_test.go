package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(t *testing.T) (*gin.Engine, *config.Config) {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)

	// 迁移必要的表
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}))

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken:         "test-api-token",
			SubscribePath:    "s",
			TrafficLogEnable: true,
		},
	}
	config.Set(cfg)

	r := gin.New()
	Setup(r, cfg)

	return r, cfg
}

func teardownTestRouter(t *testing.T) {
	t.Helper()
	require.NoError(t, database.Close())
}

func TestSetup_HealthEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestSetup_V3KernelAccessGroupsRequireAdminAndPersistScope(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))

	payload, err := json.Marshal(gin.H{"scope_id": "forward", "name": "canary", "enabled": true})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v3/access-groups", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	unauthenticated := httptest.NewRecorder()
	r.ServeHTTP(unauthenticated, request)
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, "/api/v3/access-groups", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	created := httptest.NewRecorder()
	r.ServeHTTP(created, request)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v3/access-groups?scope_id=forward", nil)
	listRequest.Header.Set("Authorization", "Bearer "+token)
	listed := httptest.NewRecorder()
	r.ServeHTTP(listed, listRequest)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), "canary")
}

func TestSetup_V3KernelOperationIsIdempotent(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))
	manifestJSON, err := service.CanonicalPluginManifest(service.PluginManifest{
		ID: "subscription", Name: "Subscription", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&model.PluginRelease{
		PluginID: "subscription", Version: "1.0.0", APIVersion: "v1", ManifestJSON: string(manifestJSON),
		ArtifactSHA256: strings.Repeat("a", 64), Signature: "test",
	}).Error)
	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	payload, err := json.Marshal(gin.H{
		"operation_id": "73c38025-6e13-4af8-a3bc-f3204bbf7cee", "idempotency_key": "router-operation-1",
		"plugin_id": "subscription", "target_version": "1.0.0", "kind": "plugin.health",
		"revision": 1, "config": gin.H{}, "deadline_at": time.Now().Add(time.Minute).UTC(),
	})
	require.NoError(t, err)
	for attempt, expectedStatus := range []int{http.StatusAccepted, http.StatusOK} {
		request := httptest.NewRequest(http.MethodPost, "/api/v3/operations", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		require.Equalf(t, expectedStatus, recorder.Code, "attempt %d: %s", attempt, recorder.Body.String())
	}
}

func TestSetup_V3ExtensionsRequiresAdminAndReturnsEmptyCatalog(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, service.EnsureKernelSchema(database.GetDB()))

	request := httptest.NewRequest(http.MethodGet, "/api/v3/extensions", nil)
	unauthenticated := httptest.NewRecorder()
	r.ServeHTTP(unauthenticated, request)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := utils.GenerateToken(1, "admin@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodGet, "/api/v3/extensions", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"data":[]}`, recorder.Body.String())
}

func TestSetup_MetricsEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetup_CORS(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// OPTIONS with Origin header — should echo back the origin (default: allow all)
	req, _ := http.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID")
}

func TestSetup_SecurityHeadersAndRequestID(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "0", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, "none", w.Header().Get("X-Permitted-Cross-Domain-Policies"))
	assert.Contains(t, w.Header().Get("Permissions-Policy"), "camera=()")
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", w.Header().Get("Strict-Transport-Security"))
}

func TestSetup_RequestIDHonorsInboundHeader(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "external-trace-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, "external-trace-id", w.Header().Get("X-Request-ID"))
}

func TestSetup_RegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_LoginEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/login", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_NodeRegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/node/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_SubscribeEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// 创建测试用户
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid-1234",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	req, _ := http.NewRequest("GET", "/s/test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return some response, not 404 (route is registered)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_LegacySubscribeEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	user := &model.User{
		Email:          "legacy-subscribe@example.com",
		Token:          "legacy-test-token",
		UUID:           "legacy-test-uuid",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	req, _ := http.NewRequest("GET", "/api/v1/client/subscribe?token=legacy-test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_AdminEndpoints_RequireAuth(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test admin endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/dashboard"},
		{"GET", "/api/v2/admin/users"},
		{"GET", "/api/v2/admin/nodes"},
		{"GET", "/api/v2/admin/orders"},
		{"GET", "/api/v2/admin/plans"},
		{"GET", "/api/v2/admin/agent/monitor"},
		{"GET", "/api/v2/admin/agent/tasks/task-1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_UserEndpoints_RequireAuth(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test user endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/user/profile"},
		{"GET", "/api/v2/user/dashboard"},
		{"GET", "/api/v2/user/subscription"},
		{"GET", "/api/v2/user/plan"},
		{"GET", "/api/v2/user/order"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_UniProxyEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Test UniProxy endpoints without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/server/UniProxy/config"},
		{"GET", "/api/v2/server/UniProxy/user"},
		{"POST", "/api/v2/server/UniProxy/push"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_PaymentEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Public payment endpoints
	t.Run("payment methods", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v2/payment/methods", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestSetup_AgentEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	// Agent public endpoints
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v2/agent/register"},
		{"POST", "/api/v2/agent/heartbeat"},
		{"GET", "/api/v2/agent/tasks"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should not return 404
			assert.NotEqual(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestSetup_TelegramWebhook(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v2/telegram/webhook", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should not return 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_CustomSubscribePath(t *testing.T) {
	cache.InitMemory()
	err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, database.Close())
	}()

	// 迁移用户表用于订阅测试
	require.NoError(t, database.GetDB().AutoMigrate(&model.User{}, &model.Plan{}))

	// 创建测试用户
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid-1234",
		TransferEnable: 1073741824,
	}
	database.GetDB().Create(user)

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken:      "test-api-token",
			SubscribePath: "custom-sub",
		},
	}

	r := gin.New()
	Setup(r, cfg)

	req, _ := http.NewRequest("GET", "/custom-sub/test-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should not return 404 - route is registered
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_AdminForwardEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/forward/ansible-machines"},
		{"GET", "/api/v2/admin/forward/nodes"},
		{"GET", "/api/v2/admin/forward/rules"},
		{"GET", "/api/v2/admin/forward/stats"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminPaymentGatewayEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/payment/gateways"},
		{"GET", "/api/v2/admin/payment/stats"},
		{"GET", "/api/v2/admin/payment/records"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminTelegramEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/telegram/users"},
		{"PUT", "/api/v2/admin/telegram/users/1/notify"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			body := strings.NewReader("{}")
			if ep.method == "GET" {
				body = strings.NewReader("")
			}
			req, _ := http.NewRequest(ep.method, ep.path, body)
			if ep.method == "PUT" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSetup_AdminSystemEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/system/configs"},
		{"GET", "/api/v2/admin/system/backup/config"},
		{"GET", "/api/v2/admin/system/backups"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Should return 401 Unauthorized (needs auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}
