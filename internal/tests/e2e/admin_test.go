package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/AnixOps/anix-control/v4/internal/service"
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
	router              *gin.Engine
	db                  *gorm.DB
	cfg                 *config.Config
	adminToken          string
	userToken           string
	testUser            *model.User
	testPlan            *model.Plan
	testNode            *model.Node
	restoreIdentityHost func()
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
		&model.CouponUsage{},
		&model.Ticket{},
		&model.TicketMessage{},
		&model.Knowledge{},
		&model.AuthorizedKey{},
		&model.UserMFA{},
		&model.MFALoginAttempt{},
		&model.InviteCode{},
		&model.CommissionRecord{},
		&model.CommissionWithdraw{},
		&model.InviteConfig{},
		&model.NotificationTemplate{},
		&model.NotificationLog{},
		&model.TelegramBot{},
		&model.TelegramUser{},
		&model.TelegramChat{},
		&model.SystemConfig{},
		&model.PaymentGateway{},
		&model.PaymentRecord{},
		&model.ForwardNode{},
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.ForwardRule{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{},
		&model.ForwardStats{},
		&model.BackupConfig{},
		&model.BackupRecord{},
		&model.OperationLog{},
		&model.AuditLog{},
		&model.Event{},
		&model.LoadBalancer{},
		&model.UserSubscriptionGroup{},
		&model.PlanSubscriptionGroup{},
		&model.SubscriptionGroup{},
		&model.SubscriptionTemplate{},
		&model.SpeedLimit{},
		&model.ServerVMess{},
		&model.ServerVLESS{},
		&model.ServerTrojan{},
		&model.ServerShadowsocks{},
		&model.ServerHysteria{},
		&model.ServerTUIC{},
		&model.ServerAnyTLS{},
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatServer{},
		&model.StatUser{},
	)
	s.Require().NoError(err)

	// 初始化管理员
	service.InitAdmin(s.cfg)
	s.restoreIdentityHost = installIdentityPlatformE2EPackage(s.T(), s.cfg)

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

	var response map[string]any
	requireJSONUnmarshal(s.T(), w.Body.Bytes(), &response)

	data := response["data"].(map[string]any)
	return data["token"].(string)
}

// TearDownSuite 测试套件清理
func (s *AdminE2ETestSuite) TearDownSuite() {
	if s.restoreIdentityHost != nil {
		s.restoreIdentityHost()
	}
	requireDatabaseClosed(s.T())
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

	var response map[string]any
	requireJSONUnmarshal(s.T(), w.Body.Bytes(), &response)

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

	var response map[string]any
	requireJSONUnmarshal(s.T(), w.Body.Bytes(), &response)

	data := response["data"].(map[string]any)
	users := data["list"].([]any)
	assert.GreaterOrEqual(s.T(), len(users), 1)
}

// TestCreateUser 测试创建用户
func (s *AdminE2ETestSuite) TestCreateUser() {
	body := map[string]any{
		"email":           "newuser@admin.test",
		"password":        "password123",
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
}

// TestGetNodes 测试获取节点列表
func (s *AdminE2ETestSuite) TestGetNodes() {
	req, _ := http.NewRequest("GET", "/api/v2/admin/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]any
	requireJSONUnmarshal(s.T(), w.Body.Bytes(), &response)

	data := response["data"].(map[string]any)
	nodes := data["list"].([]any)
	assert.GreaterOrEqual(s.T(), len(nodes), 1)
}

// TestCreateNode 测试创建节点
func (s *AdminE2ETestSuite) TestCreateNode() {
	body := map[string]any{
		"name":         "New Test Node",
		"host":         "192.168.1.1",
		"port":         443,
		"rate":         1.0,
		"traffic_rate": 1.0,
		"show":         1,
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
	body := map[string]any{
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

// TestTrafficHourlyEndpoints verifies the real admin HTTP routes that feed
// /admin/traffic-hourly, including include_zero_users and legacy rate=0 logs.
func (s *AdminE2ETestSuite) TestTrafficHourlyEndpoints() {
	trafficUser := &model.User{
		Email:          "traffic-http@example.com",
		Password:       "unused",
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Balance:        0,
		TransferEnable: 10737418240,
		Banned:         0,
		IsAdmin:        0,
	}
	s.Require().NoError(s.db.Create(trafficUser).Error)

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	s.Require().NoError(s.db.Create(&model.TrafficLog{
		UserID:     trafficUser.ID,
		ServerID:   s.testNode.ID,
		ServerType: "node",
		U:          1024,
		D:          2048,
		Rate:       0,
		LogAt:      currentHour + 10,
	}).Error)

	hourlyReq, _ := http.NewRequest("GET", "/api/v2/admin/traffic/hourly?hours=2&user_id="+strconv.FormatUint(uint64(trafficUser.ID), 10), nil)
	hourlyReq.Header.Set("Authorization", "Bearer "+s.adminToken)
	hourlyRecorder := httptest.NewRecorder()
	s.router.ServeHTTP(hourlyRecorder, hourlyReq)
	assert.Equal(s.T(), http.StatusOK, hourlyRecorder.Code, hourlyRecorder.Body.String())

	hourlyData := requirePanelDataMap(s.T(), hourlyRecorder.Body.Bytes())
	hourlyRows := hourlyData["list"].([]any)
	var currentHourTraffic float64
	for _, item := range hourlyRows {
		row := item.(map[string]any)
		if int64(row["hour_ts"].(float64)) == currentHour {
			currentHourTraffic = row["traffic"].(float64)
		}
	}
	assert.Equal(s.T(), float64(3072), currentHourTraffic)

	rankingReq, _ := http.NewRequest("GET", "/api/v2/admin/traffic/user-ranking?hours=168&limit=500&include_zero_users=true", nil)
	rankingReq.Header.Set("Authorization", "Bearer "+s.adminToken)
	rankingRecorder := httptest.NewRecorder()
	s.router.ServeHTTP(rankingRecorder, rankingReq)
	assert.Equal(s.T(), http.StatusOK, rankingRecorder.Code, rankingRecorder.Body.String())

	rankingData := requirePanelDataMap(s.T(), rankingRecorder.Body.Bytes())
	rankingRows := rankingData["list"].([]any)
	var found bool
	for _, item := range rankingRows {
		row := item.(map[string]any)
		if uint(row["user_id"].(float64)) == trafficUser.ID {
			found = true
			assert.Equal(s.T(), "traffic-http@example.com", row["email"])
			assert.Equal(s.T(), float64(3072), row["traffic"])
		}
	}
	assert.True(s.T(), found)

	oversizedRankingReq, _ := http.NewRequest("GET", "/api/v2/admin/traffic/user-ranking?hours=999999&limit=999999&include_zero_users=true", nil)
	oversizedRankingReq.Header.Set("Authorization", "Bearer "+s.adminToken)
	oversizedRankingRecorder := httptest.NewRecorder()
	s.router.ServeHTTP(oversizedRankingRecorder, oversizedRankingReq)
	assert.Equal(s.T(), http.StatusOK, oversizedRankingRecorder.Code, oversizedRankingRecorder.Body.String())

	oversizedRankingData := requirePanelDataMap(s.T(), oversizedRankingRecorder.Body.Bytes())
	oversizedRankingRows := oversizedRankingData["list"].([]any)
	assert.LessOrEqual(s.T(), len(oversizedRankingRows), 1000)
}

// TestAdminE2E 运行测试套件
func TestAdminE2E(t *testing.T) {
	suite.Run(t, new(AdminE2ETestSuite))
}
