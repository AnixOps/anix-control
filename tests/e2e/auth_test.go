package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/router"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// AuthE2ETestSuite 认证 API E2E 测试套件
type AuthE2ETestSuite struct {
	suite.Suite
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

// SetupSuite 测试套件初始化
func (s *AuthE2ETestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	// 使用测试配置
	s.cfg = &config.Config{
		Env: "test",
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: "debug",
		},
		Database: config.DatabaseConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret-key-for-e2e-testing",
			Expire: 86400,
		},
		App: config.AppConfig{
			Name:          "V2Board E2E Test",
			Version:       "test",
			APIToken:      "test-api-token",
			SubscribePath: "s",
		},
		Admin: config.AdminConfig{
			Email:    "admin@example.com",
			Password: "admin123456",
		},
	}

	// 注册配置到全局 (必须在中间件初始化前设置)
	config.Set(s.cfg)

	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	err := database.Init(&s.cfg.Database)
	s.Require().NoError(err)
	s.db = database.Get()

	// 自动迁移
	err = database.AutoMigrate(&model.User{})
	s.Require().NoError(err)

	// 初始化管理员
	service.InitAdmin(s.cfg)

	// 创建路由
	s.router = gin.New()
	router.Setup(s.router, s.cfg)
}

// TearDownSuite 测试套件清理
func (s *AuthE2ETestSuite) TearDownSuite() {
	database.Close()
}

// SetupTest 每个测试前的清理
func (s *AuthE2ETestSuite) SetupTest() {
	// 清理用户数据，保留管理员
	s.db.Where("is_admin = 0").Delete(&model.User{})
}

// TestRegister_Success 测试注册成功
func (s *AuthE2ETestSuite) TestRegister_Success() {
	body := map[string]string{
		"email":    "newuser@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	assert.NotEmpty(s.T(), data["token"])
	assert.Equal(s.T(), "newuser@example.com", data["email"])
	assert.False(s.T(), data["is_admin"].(bool))
}

// TestRegister_InvalidEmail 测试无效邮箱注册
func (s *AuthE2ETestSuite) TestRegister_InvalidEmail() {
	testCases := []struct {
		name     string
		email    string
		password string
	}{
		{"空邮箱", "", "password123"},
		{"邮箱太短", "a@b", "password123"},
		{"缺少@符号", "invalid-email", "password123"},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			body := map[string]string{
				"email":    tc.email,
				"password": tc.password,
			}
			jsonBody, _ := json.Marshal(body)

			req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestRegister_ShortPassword 测试密码过短注册
func (s *AuthE2ETestSuite) TestRegister_ShortPassword() {
	body := map[string]string{
		"email":    "testpass@example.com",
		"password": "12345", // 少于6位
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// TestRegister_DuplicateEmail 测试重复邮箱注册
func (s *AuthE2ETestSuite) TestRegister_DuplicateEmail() {
	// 第一次注册
	body := map[string]string{
		"email":    "duplicate@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 第二次注册相同邮箱
	req2, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req2.Header.Set("Content-Type", "application/json")

	w2 := httptest.NewRecorder()
	s.router.ServeHTTP(w2, req2)
	assert.Equal(s.T(), http.StatusBadRequest, w2.Code)
}

// TestLogin_Success 测试登录成功
func (s *AuthE2ETestSuite) TestLogin_Success() {
	// 先注册用户
	registerBody := map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	require := s.Require()
	require.Equal(http.StatusOK, w.Code)

	// 登录
	loginBody := map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	}
	jsonLoginBody, _ := json.Marshal(loginBody)

	req, _ = http.NewRequest("POST", "/api/v2/login", bytes.NewReader(jsonLoginBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	assert.NotEmpty(s.T(), data["token"])
	assert.Equal(s.T(), "login@example.com", data["email"])
}

// TestLogin_WrongPassword 测试密码错误
func (s *AuthE2ETestSuite) TestLogin_WrongPassword() {
	// 先注册用户
	registerBody := map[string]string{
		"email":    "wrongpass@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 使用错误密码登录
	loginBody := map[string]string{
		"email":    "wrongpass@example.com",
		"password": "wrongpassword",
	}
	jsonLoginBody, _ := json.Marshal(loginBody)

	req, _ = http.NewRequest("POST", "/api/v2/login", bytes.NewReader(jsonLoginBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// TestLogin_UserNotFound 测试用户不存在
func (s *AuthE2ETestSuite) TestLogin_UserNotFound() {
	loginBody := map[string]string{
		"email":    "nonexistent@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(loginBody)

	req, _ := http.NewRequest("POST", "/api/v2/login", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// TestJWTMiddleware_ValidToken 测试有效 JWT Token
func (s *AuthE2ETestSuite) TestJWTMiddleware_ValidToken() {
	// 注册并获取 token
	registerBody := map[string]string{
		"email":    "jwt@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)

	req, _ := http.NewRequest("POST", "/api/v2/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].(map[string]interface{})
	token := data["token"].(string)

	// 使用 token 访问受保护的接口
	req, _ = http.NewRequest("GET", "/api/v2/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestJWTMiddleware_InvalidToken 测试无效 JWT Token
func (s *AuthE2ETestSuite) TestJWTMiddleware_InvalidToken() {
	testCases := []struct {
		name  string
		token string
	}{
		{"空 Token", ""},
		{"无效格式", "invalid-token"},
		{"Bearer 空", "Bearer "},
		{"伪造 Token", "Bearer fake.jwt.token"},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v2/user/profile", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", tc.token)
			}

			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// TestAdminLogin 测试管理员登录
func (s *AuthE2ETestSuite) TestAdminLogin() {
	// 使用默认管理员账号登录
	loginBody := map[string]string{
		"email":    "admin@example.com",
		"password": "admin123456",
	}
	jsonBody, _ := json.Marshal(loginBody)

	req, _ := http.NewRequest("POST", "/api/v2/login", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	assert.True(s.T(), data["is_admin"].(bool))
}

// TestAuthE2E 运行测试套件
func TestAuthE2E(t *testing.T) {
	suite.Run(t, new(AuthE2ETestSuite))
}