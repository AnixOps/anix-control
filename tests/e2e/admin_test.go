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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AdminE2ETestSuite 管理员 API E2E 测试套件
type AdminE2ETestSuite struct {
	suite.Suite
	router     *gin.Engine
	db         *gorm.DB
	cfg        *config.Config
	adminToken string
	userToken  string
	testUser   *model.User
	testPlan   *model.Plan
	testNode   *model.Node
}

// SetupSuite 测试套件初始化
func (s *AdminE2ETestSuite) SetupSuite() {
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
			Secret: "test-jwt-secret-key-for-admin-e2e-testing",
			Expire: 86400,
		},
		App: config.AppConfig{
			Name:          "V2Board Admin E2E Test",
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
	err = database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.Order{},
		&model.Coupon{},
		&model.Knowledge{},
		&model.SubscriptionGroup{},
	)
	s.Require().NoError(err)

	// 初始化管理员
	service.InitAdmin(s.cfg)

	// 创建路由
	s.router = gin.New()
	router.Setup(s.router, s.cfg)

	// 管理员登录获取 token
	s.adminToken = s.login("admin@example.com", "admin123456")

	// 创建普通用户 (使用正确的密码哈希)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("userpassword"), bcrypt.DefaultCost)
	s.Require().NoError(err)

	s.testUser = &model.User{
		Email:          "user@example.com",
		Password:       string(hashedPassword),
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Balance:        0,
		TransferEnable: 10737418240,
		Banned:         0,
		IsAdmin:        0,
	}
	s.db.Create(s.testUser)
	s.userToken = s.login("user@example.com", "userpassword")

	// 创建测试套餐
	content := "Test plan description"
	speedLimit := int64(104857600)
	deviceLimit := 5
	sort := 0
	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		Content:        &content,
		SpeedLimit:     &speedLimit,
		DeviceLimit:    &deviceLimit,
		TransferEnable: 107374182400,
		Sort:           &sort,
		Show:           1,
		Renew:          1,
	}
	s.db.Create(s.testPlan)

	// 创建测试节点
	s.testNode = &model.Node{
		Name:        "Test Node",
		Host:        "127.0.0.1",
		Port:        443,
		Status:      model.NodeStatusOnline,
		Rate:        1.0,
		TrafficRate: 1.0,
		Show:        1,
	}
	s.db.Create(s.testNode)
}

// login 辅助函数：登录并获取 token
func (s *AdminE2ETestSuite) login(email, password string) string {
	body := map[string]string{
		"email":    email,
		"password": password,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/login", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return ""
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	return data["token"].(string)
}

// TearDownSuite 测试套件清理
func (s *AdminE2ETestSuite) TearDownSuite() {
	database.Close()
}

// TestAdminAuth_RequireAdmin 测试管理员权限验证
func (s *AdminE2ETestSuite) TestAdminAuth_RequireAdmin() {
	// 普通用户访问管理员接口
	req, _ := http.NewRequest("GET", "/api/v2/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+s.userToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// 应该被拒绝
	assert.Equal(s.T(), http.StatusForbidden, w.Code)
}

// TestGetDashboard 测试获取管理仪表盘
func (s *AdminE2ETestSuite) TestGetDashboard() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	// 检查仪表盘数据
	assert.Contains(s.T(), response, "data")
}

// TestGetUsers 测试获取用户列表
func (s *AdminE2ETestSuite) TestGetUsers() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	users := data["list"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(users), 1)
}

// TestCreateUser 测试创建用户
func (s *AdminE2ETestSuite) TestCreateUser() {
	body := map[string]interface{}{
		"email":          "newuser@admin.test",
		"password":       "password123",
		"transfer_enable": 10737418240,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/admin/users", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestBanUser 测试封禁用户
func (s *AdminE2ETestSuite) TestBanUser() {
	// 封禁用户
	req, _ := http.NewRequest("POST", "/api/v2/admin/users/2/ban", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 验证用户已被封禁
	var user model.User
	s.db.First(&user, 2)
	assert.Equal(s.T(), 1, user.Banned)
}

// TestGetNodes 测试获取节点列表
func (s *AdminE2ETestSuite) TestGetNodes() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	nodes := data["list"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(nodes), 1)
}

// TestCreateNode 测试创建节点
func (s *AdminE2ETestSuite) TestCreateNode() {
	body := map[string]interface{}{
		"name":        "New Test Node",
		"host":        "192.168.1.1",
		"port":        443,
		"rate":        1.0,
		"traffic_rate": 1.0,
		"show":        1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/admin/nodes", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestGetPlans 测试获取套餐列表
func (s *AdminE2ETestSuite) TestGetPlans() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/plans", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestCreatePlan 测试创建套餐
func (s *AdminE2ETestSuite) TestCreatePlan() {
	body := map[string]interface{}{
		"name":            "New Test Plan",
		"transfer_enable": 107374182400,
		"show":            1,
		"renew":           1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v2/admin/plans", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestGetOrders 测试获取订单列表
func (s *AdminE2ETestSuite) TestGetOrders() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/orders", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// TestAdminE2E 运行测试套件
func TestAdminE2E(t *testing.T) {
	suite.Run(t, new(AdminE2ETestSuite))
}