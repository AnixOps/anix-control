package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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
	database.GetDB().AutoMigrate(&model.User{})

	cfg := &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken:       "test-api-token",
			SubscribePath:  "s",
			TrafficLogEnable: true,
		},
	}

	r := gin.New()
	Setup(r, cfg)

	return r, cfg
}

func teardownTestRouter() {
	database.Close()
}

func TestSetup_HealthEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestSetup_MetricsEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSetup_CORS(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("OPTIONS", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestSetup_RegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("POST", "/api/v2/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_LoginEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("POST", "/api/v2/login", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_NodeRegisterEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	req, _ := http.NewRequest("POST", "/api/v2/node/register", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 400 due to missing body, not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestSetup_SubscribeEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

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

func TestSetup_AdminEndpoints_RequireAuth(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

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
	defer teardownTestRouter()

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
	defer teardownTestRouter()

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
	defer teardownTestRouter()

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
	defer teardownTestRouter()

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
	defer teardownTestRouter()

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
	defer database.Close()

	// 迁移用户表用于订阅测试
	database.GetDB().AutoMigrate(&model.User{}, &model.Plan{})

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
	defer teardownTestRouter()

	endpoints := []struct {
		method string
		path   string
	}{
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
	defer teardownTestRouter()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v2/admin/payment/gateways"},
		{"GET", "/api/v2/admin/payment/stats"},
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

func TestSetup_AdminSystemEndpoints(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

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