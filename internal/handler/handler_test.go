package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// HandlerTestSuite Handler 测试套件基类
type HandlerTestSuite struct {
	suite.Suite
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

func (s *HandlerTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	s.cfg = &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret-for-handler",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken: "test-api-token",
		},
	}

	// 初始化数据库
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	s.db = database.Get()

	// 自动迁移
	s.db.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.Plan{},
		&model.Order{},
		&model.Coupon{},
		&model.Ticket{},
		&model.Knowledge{},
	)
}

func (s *HandlerTestSuite) TearDownSuite() {
	database.Close()
}

func (s *HandlerTestSuite) SetupTest() {
	// 清理数据
	s.db.Exec("DELETE FROM v2_user")
	s.db.Exec("DELETE FROM v2_node")
	s.db.Exec("DELETE FROM v2_node_protocol")
	s.db.Exec("DELETE FROM v2_plan")
	s.db.Exec("DELETE FROM v2_order")
}

// AuthHandlerTestSuite 认证 Handler 测试套件
type AuthHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *AuthHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *AuthHandlerTestSuite) TestRegisterHandler() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "handler@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_InvalidEmail() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "invalid",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_ShortPassword() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "shortpass@example.com",
		"password": "12345",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AuthHandlerTestSuite) TestLoginHandler() {
	// 先注册用户
	regHandler := NewAuthHandler(s.cfg)
	s.router.POST("/register", regHandler.Register)

	body := map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 登录
	s.router.POST("/login", regHandler.Login)
	loginBody := map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	}
	jsonLoginBody, _ := json.Marshal(loginBody)

	req, _ = http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AuthHandlerTestSuite) TestLoginHandler_WrongPassword() {
	// 先注册用户
	regHandler := NewAuthHandler(s.cfg)
	s.router.POST("/register", regHandler.Register)

	body := map[string]string{
		"email":    "wrongpass@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 使用错误密码登录
	s.router.POST("/login", regHandler.Login)
	loginBody := map[string]string{
		"email":    "wrongpass@example.com",
		"password": "wrongpassword",
	}
	jsonLoginBody, _ := json.Marshal(loginBody)

	req, _ = http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func TestAuthHandler(t *testing.T) {
	suite.Run(t, new(AuthHandlerTestSuite))
}

// UniProxyHandlerTestSuite UniProxy Handler 测试套件
type UniProxyHandlerTestSuite struct {
	HandlerTestSuite
	testNode *model.Node
	testUser *model.User
}

func (s *UniProxyHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试节点
	s.testNode = &model.Node{
		Name:        "UniProxy Test Node",
		Host:        "127.0.0.1",
		Port:        443,
		Status:      model.NodeStatusOnline,
		Rate:        1.0,
		TrafficRate: 1.0,
		Show:        1,
	}
	s.db.Create(s.testNode)

	// 创建节点协议
	transport := "tcp"
	protocol := &model.NodeProtocol{
		NodeID:    s.testNode.ID,
		Name:      "Test VLESS",
		Type:      model.ProtocolVLESS,
		Port:      443,
		Enable:    1,
		Show:      1,
		Transport: &transport,
	}
	s.db.Create(protocol)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "uniproxy@example.com",
		Password:       "hash",
		Token:          "test-token-uniproxy",
		UUID:           "test-uuid-uniproxy",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *UniProxyHandlerTestSuite) TestGenerateETag() {
	data := []byte(`{"test": "data"}`)
	etag := generateETag(data)

	assert.NotEmpty(s.T(), etag)
	assert.Len(s.T(), etag, 32) // MD5 是 32 个字符
}

func (s *UniProxyHandlerTestSuite) TestGenerateETag_Consistent() {
	data := []byte(`{"test": "data"}`)
	etag1 := generateETag(data)
	etag2 := generateETag(data)

	assert.Equal(s.T(), etag1, etag2)
}

func (s *UniProxyHandlerTestSuite) TestGenerateETag_Different() {
	etag1 := generateETag([]byte(`{"test": "data1"}`))
	etag2 := generateETag([]byte(`{"test": "data2"}`))

	assert.NotEqual(s.T(), etag1, etag2)
}

func (s *UniProxyHandlerTestSuite) TestGetConfig_MissingNodeID() {
	handler := NewUniProxyHandler()
	s.router.GET("/config", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *UniProxyHandlerTestSuite) TestGetConfig_InvalidNodeID() {
	handler := NewUniProxyHandler()
	s.router.GET("/config", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/config?node_id=invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *UniProxyHandlerTestSuite) TestPushTraffic_MissingNodeID() {
	handler := NewUniProxyHandler()
	s.router.POST("/push", handler.PushTraffic)

	req, _ := http.NewRequest("POST", "/push", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *UniProxyHandlerTestSuite) TestPushTraffic_InvalidBody() {
	handler := NewUniProxyHandler()
	s.router.POST("/push", handler.PushTraffic)

	req, _ := http.NewRequest("POST", "/push?node_id=1", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *UniProxyHandlerTestSuite) TestPushAlive_MissingNodeID() {
	handler := NewUniProxyHandler()
	s.router.POST("/alive", handler.PushAlive)

	req, _ := http.NewRequest("POST", "/alive", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestUniProxyHandler(t *testing.T) {
	suite.Run(t, new(UniProxyHandlerTestSuite))
}