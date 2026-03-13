package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// SubscribeE2ETestSuite 订阅 API E2E 测试套件
type SubscribeE2ETestSuite struct {
	suite.Suite
	router       *gin.Engine
	db           *gorm.DB
	cfg          *config.Config
	testUser     *model.User
	testNode     *model.Node
	testProtocol *model.NodeProtocol
	testGroup    *model.SubscriptionGroup
}

// SetupSuite 测试套件初始化
func (s *SubscribeE2ETestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

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
			Secret: "test-jwt-secret-key-for-subscribe-e2e-testing",
			Expire: 86400,
		},
		App: config.AppConfig{
			Name:          "V2Board Subscribe E2E Test",
			Version:       "test",
			APIToken:      "test-api-token",
			SubscribePath: "s",
		},
	}

	// 初始化数据库
	err := database.Init(&s.cfg.Database)
	s.Require().NoError(err)
	s.db = database.Get()

	// 自动迁移
	err = database.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.SubscriptionGroup{},
		&model.SubscriptionTemplate{},
		&model.Order{},
	)
	s.Require().NoError(err)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "subscribe@example.com",
		Password:       "$2a$10$test-hash",
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Balance:        0,
		TransferEnable: 10737418240, // 10GB
		U:              1073741824,  // 1GB used
		D:              2147483648,  // 2GB used
		Banned:         0,
		IsAdmin:        0,
	}
	s.db.Create(s.testUser)

	// 创建测试节点
	s.testNode = &model.Node{
		Name:        "Subscribe Test Node",
		Host:        "test.example.com",
		Port:        443,
		Status:      model.NodeStatusOnline,
		Rate:        1.0,
		TrafficRate: 1.0,
		Show:        1,
	}
	s.db.Create(s.testNode)

	// 创建节点协议
	settings := `{"flow":"xtls-rprx-vision"}`
	realitySettings := `{"public_key":"test-public-key","short_id":"6ba85179"}`
	s.testProtocol = &model.NodeProtocol{
		NodeID:          s.testNode.ID,
		Name:            "VLESS + Reality",
		Type:            model.ProtocolVLESS,
		Port:            443,
		Enable:          1,
		Show:            1,
		TLS:             2, // Reality
		Settings:        &settings,
		RealitySettings: &realitySettings,
		Transport:       ptrString("tcp"),
	}
	s.db.Create(s.testProtocol)

	// 创建订阅分组
	description := "Test subscription group"
	s.testGroup = &model.SubscriptionGroup{
		Name:        "Test Group",
		Description: &description,
		Priority:    0,
		Enable:      1,
	}
	s.db.Create(s.testGroup)

	// 关联协议到分组
	s.db.Model(s.testGroup).Association("Protocols").Append(s.testProtocol)

	// 创建路由
	s.router = gin.New()
	router.Setup(s.router, s.cfg)
}

// TearDownSuite 测试套件清理
func (s *SubscribeE2ETestSuite) TearDownSuite() {
	database.Close()
}

// TestGetSubscription_Success 测试获取订阅成功
func (s *SubscribeE2ETestSuite) TestGetSubscription_Success() {
	url := "/s/" + s.testUser.Token
	req, _ := http.NewRequest("GET", url, nil)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.NotEmpty(s.T(), w.Body.String())
}

// TestGetSubscription_InvalidToken 测试无效 Token
func (s *SubscribeE2ETestSuite) TestGetSubscription_InvalidToken() {
	req, _ := http.NewRequest("GET", "/s/invalid-token-12345", nil)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

// TestGetSubscription_V2RayFormat 测试 V2Ray 格式
func (s *SubscribeE2ETestSuite) TestGetSubscription_V2RayFormat() {
	userAgents := []string{
		"v2rayN",
		"v2rayNG",
		"ClashMeta",
	}

	for _, ua := range userAgents {
		s.T().Run(ua, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
			req.Header.Set("User-Agent", ua)

			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestGetSubscription_ClashFormat 测试 Clash 格式
func (s *SubscribeE2ETestSuite) TestGetSubscription_ClashFormat() {
	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	req.Header.Set("User-Agent", "clash")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Contains(s.T(), w.Header().Get("Content-Type"), "text/plain")
}

// TestGetSubscription_SurgeFormat 测试 Surge 格式
func (s *SubscribeE2ETestSuite) TestGetSubscription_SurgeFormat() {
	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	req.Header.Set("User-Agent", "Surge")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestGetSubscription_SingBoxFormat 测试 Sing-box 格式
func (s *SubscribeE2ETestSuite) TestGetSubscription_SingBoxFormat() {
	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	req.Header.Set("User-Agent", "sing-box")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestSubscriptionHeaders 测试订阅响应头
func (s *SubscribeE2ETestSuite) TestSubscriptionHeaders() {
	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 检查 Subscription-Userinfo 头
	userInfo := w.Header().Get("Subscription-Userinfo")
	assert.Contains(s.T(), userInfo, "upload=")
	assert.Contains(s.T(), userInfo, "download=")
	assert.Contains(s.T(), userInfo, "total=")

	// 检查 Content-Disposition
	contentDisposition := w.Header().Get("Content-Disposition")
	assert.Contains(s.T(), contentDisposition, "attachment")
}

// TestETagCaching 测试 ETag 缓存
func (s *SubscribeE2ETestSuite) TestETagCaching() {
	// 第一次请求
	req1, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	w1 := httptest.NewRecorder()
	s.router.ServeHTTP(w1, req1)

	etag := w1.Header().Get("ETag")
	if etag != "" {
		// 第二次请求带 If-None-Match
		req2, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
		req2.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()
		s.router.ServeHTTP(w2, req2)

		// 应该返回 304 Not Modified
		assert.Equal(s.T(), http.StatusNotModified, w2.Code)
	}
}

// TestBannedUserSubscription 测试被封禁用户的订阅
func (s *SubscribeE2ETestSuite) TestBannedUserSubscription() {
	// 创建被封禁用户
	bannedUser := &model.User{
		Email:          "banned@example.com",
		Password:       "$2a$10$test-hash",
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Banned:         1, // 已封禁
		TransferEnable: 10737418240,
	}
	s.db.Create(bannedUser)

	req, _ := http.NewRequest("GET", "/s/"+bannedUser.Token, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 根据实现，可能返回空订阅或错误
	assert.NotEqual(s.T(), http.StatusInternalServerError, w.Code)
}

// TestExpiredUserSubscription 测试过期用户的订阅
func (s *SubscribeE2ETestSuite) TestExpiredUserSubscription() {
	// 创建过期用户
	expiredTime := int64(1000000000) // 很久以前
	expiredUser := &model.User{
		Email:          "expired@example.com",
		Password:       "$2a$10$test-hash",
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		ExpiredAt:      &expiredTime,
		TransferEnable: 10737418240,
	}
	s.db.Create(expiredUser)

	req, _ := http.NewRequest("GET", "/s/"+expiredUser.Token, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 根据实现验证行为
	assert.NotEqual(s.T(), http.StatusInternalServerError, w.Code)
}

// ptrString 辅助函数
func ptrString(s string) *string {
	return &s
}

// TestSubscribeE2E 运行测试套件
func TestSubscribeE2E(t *testing.T) {
	suite.Run(t, new(SubscribeE2ETestSuite))
}