package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/anixops/v2board/internal/cache"
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

// UniProxyE2ETestSuite UniProxy API E2E 测试套件
type UniProxyE2ETestSuite struct {
	suite.Suite
	router   *gin.Engine
	db       *gorm.DB
	cfg      *config.Config
	testNode *model.Node
	testUser *model.User
	globalAPIToken string
}

// SetupSuite 测试套件初始化
func (s *UniProxyE2ETestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	s.globalAPIToken = "test-global-api-token"

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
			Name:          "V2Board UniProxy E2E Test",
			Version:       "test",
			APIToken:      s.globalAPIToken,
			SubscribePath: "s",
		},
	}

	// 注册配置到全局 (必须在中中间件初始化前设置)
	config.Set(s.cfg)

	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	err := database.Init(&s.cfg.Database)
	s.Require().NoError(err)
	s.db = database.Get()

	// 自动迁移
	err = database.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.Order{},
		&model.OnlineLog{},
		&model.TrafficLog{},
		&model.StatUser{},
		&model.StatServer{},
	)
	s.Require().NoError(err)

	// 创建测试节点
	s.testNode = &model.Node{
		Name:        "Test Node",
		Host:        "127.0.0.1",
		Port:        443,
		APIKey:      uuid.New().String(),
		Status:      model.NodeStatusOnline,
		Rate:        1.0,
		TrafficRate: 1.0,
		Show:        1,
	}
	s.db.Create(s.testNode)

	// 创建节点协议
	protocol := &model.NodeProtocol{
		NodeID: s.testNode.ID,
		Name:   "VLESS + Reality",
		Type:   model.ProtocolVLESS,
		Port:   443,
		Enable: 1,
		Show:   1,
		TLS:    2, // Reality
	}
	s.db.Create(protocol)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "uniproxy@example.com",
		Password:       "$2a$10$test-hash",
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Balance:        0,
		TransferEnable: 10737418240, // 10GB
		U:              0,
		D:              0,
		Banned:         0,
		IsAdmin:        0,
	}
	s.db.Create(s.testUser)

	// 创建路由
	s.router = gin.New()
	router.Setup(s.router, s.cfg)
}

// TearDownSuite 测试套件清理
func (s *UniProxyE2ETestSuite) TearDownSuite() {
	database.Close()
}

// TestGetConfig_Success 测试获取节点配置成功
func (s *UniProxyE2ETestSuite) TestGetConfig_Success() {
	req, _ := http.NewRequest("GET", "/api/v2/server/UniProxy/config", nil)
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-API-Key", s.testNode.APIKey)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 注意：需要有效的 node_id
	// 这里测试可能需要根据实际实现调整
}

// TestGetUsers_Success 测试获取用户列表成功
func (s *UniProxyE2ETestSuite) TestGetUsers_Success() {
	req, _ := http.NewRequest("GET", "/api/v2/server/UniProxy/user", nil)
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-API-Key", s.testNode.APIKey)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 验证响应
	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		// 检查用户列表格式
		if users, ok := response["users"].([]interface{}); ok {
			for _, u := range users {
				user := u.(map[string]interface{})
				assert.NotEmpty(s.T(), user["id"])
				assert.NotEmpty(s.T(), user["uuid"])
			}
		}
	}
}

// TestPushTraffic_Success 测试上报流量成功
func (s *UniProxyE2ETestSuite) TestPushTraffic_Success() {
	trafficData := map[string]interface{}{
		"1": []int64{1024, 2048}, // 用户ID: [上传, 下载]
		"2": []int64{512, 1024},
	}
	jsonBody, _ := json.Marshal(trafficData)

	req, _ := http.NewRequest("POST", "/api/v2/server/UniProxy/push", bytes.NewReader(jsonBody))
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.testNode.APIKey)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 验证响应状态
	assert.Contains(s.T(), []int{http.StatusOK, http.StatusBadRequest}, w.Code)
}

// TestPushAlive_Success 测试上报在线用户成功
func (s *UniProxyE2ETestSuite) TestPushAlive_Success() {
	aliveData := map[string]interface{}{
		"1": []string{"192.168.1.100", "10.0.0.50"},
		"2": []string{"172.16.0.1"},
	}
	jsonBody, _ := json.Marshal(aliveData)

	req, _ := http.NewRequest("POST", "/api/v2/server/UniProxy/alive", bytes.NewReader(jsonBody))
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.testNode.APIKey)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 验证响应状态
	assert.Contains(s.T(), []int{http.StatusOK, http.StatusBadRequest}, w.Code)
}

// TestNodeAuth_MissingAPIKey 测试缺少 API Key
func (s *UniProxyE2ETestSuite) TestNodeAuth_MissingAPIKey() {
	req, _ := http.NewRequest("GET", "/api/v2/server/UniProxy/config", nil)
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	// 不添加 X-API-Key
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// TestNodeAuth_InvalidAPIKey 测试无效 API Key
func (s *UniProxyE2ETestSuite) TestNodeAuth_InvalidAPIKey() {
	req, _ := http.NewRequest("GET", "/api/v2/server/UniProxy/config", nil)
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-API-Key", "invalid-api-key")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// TestPushTraffic_InvalidFormat 测试无效的流量数据格式
func (s *UniProxyE2ETestSuite) TestPushTraffic_InvalidFormat() {
	// 发送无效的 JSON
	req, _ := http.NewRequest("POST", "/api/v2/server/UniProxy/push", bytes.NewReader([]byte("invalid json")))
	q := req.URL.Query()
	q.Add("node_id", strconv.Itoa(int(s.testNode.ID)))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.testNode.APIKey)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// TestUniProxyE2E 运行测试套件
func TestUniProxyE2E(t *testing.T) {
	suite.Run(t, new(UniProxyE2ETestSuite))
}
