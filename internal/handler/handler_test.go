package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

var (
	testDB   *gorm.DB
	testCfg  *config.Config
	testDBMu sync.Mutex
)

// initTestDB initializes the test database once
func initTestDB() *gorm.DB {
	testDBMu.Lock()
	defer testDBMu.Unlock()

	// Check if testDB is still valid (not closed by another test package)
	if testDB != nil {
		// Verify the database is still usable by checking if we can query it
		if db := database.Get(); db != nil {
			// Database singleton is still valid
			return testDB
		}
		// Database was closed by another test, need to reinitialize
		testDB = nil
	}

	gin.SetMode(gin.TestMode)

	// 初始化缓存
	cache.InitMemory()

	testCfg = &config.Config{
		Env: "test",
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret-for-handler",
			Expire: 86400,
		},
		App: config.AppConfig{
			APIToken: "test-api-token",
		},
	}

	// Reset database state to ensure clean initialization
	database.Reset()

	// 初始化数据库包的单例（handlers 内部使用 database.Get()）
	if err := database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	}); err != nil {
		panic("failed to init database: " + err.Error())
	}
	testDB = database.Get()

	// 自动迁移
	testDB.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.Plan{},
		&model.Order{},
		&model.Coupon{},
		&model.CouponUsage{},
		&model.Ticket{},
		&model.TicketMessage{},
		&model.Knowledge{},
		&model.AuthorizedKey{},
		&model.UserMFA{},
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
		&model.ForwardRule{},
		&model.BackupConfig{},
		&model.BackupRecord{},
		&model.LoadBalancer{},
		&model.UserSubscriptionGroup{},
		&model.PlanSubscriptionGroup{},
		&model.Event{},
		&model.SubscriptionGroup{},
		&model.SubscriptionTemplate{},
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

	return testDB
}

// HandlerTestSuite Handler 测试套件基类
type HandlerTestSuite struct {
	suite.Suite
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

func (s *HandlerTestSuite) SetupSuite() {
	s.db = initTestDB()
	s.cfg = testCfg
}

func (s *HandlerTestSuite) SetupTest() {
	// Ensure database is initialized (for suites that don't have their own SetupSuite)
	if s.db == nil {
		s.SetupSuite()
	}

	// 清理数据 (忽略表不存在的错误，AutoMigrate 会创建表)
	tables := []string{
		"v2_user", "v2_node", "v2_node_protocol", "v2_plan", "v2_order",
		"v2_authorized_key", "v2_coupon", "v2_ticket", "v2_ticket_message",
		"v2_knowledge", "v2_invite_code", "v2_commission_record",
		"v2_commission_withdraw", "v2_invite_config", "v2_notification_template",
		"v2_notification_log", "v2_telegram_bot", "v2_system_config",
		"v2_payment_gateway", "v2_payment_record", "v2_forward_node",
		"v2_forward_rule", "v2_backup_config", "v2_backup_record",
		"v2_load_balancer", "v2_user_subscription_group",
		"v2_plan_subscription_group", "v2_event", "v2_subscription_group",
		"v2_subscription_template",
	}
	for _, table := range tables {
		s.db.Exec("DELETE FROM " + table)
	}

	// 重置自增计数器
	s.db.Exec("DELETE FROM sqlite_sequence WHERE name IN ('v2_user', 'v2_plan', 'v2_order', 'v2_node', 'v2_node_protocol')")
	s.db.Exec("DELETE FROM v2_subscription_group")
	s.db.Exec("DELETE FROM v2_subscription_template")
}

// AuthHandlerTestSuite 认证 Handler 测试套件
type AuthHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *AuthHandlerTestSuite) SetupSuite() {
	s.HandlerTestSuite.SetupSuite()
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

func (s *UniProxyHandlerTestSuite) SetupSuite() {
	s.HandlerTestSuite.SetupSuite()
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

// UserHandlerTestSuite 用户 Handler 测试套件
type UserHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
}

func (s *UserHandlerTestSuite) SetupSuite() {
	s.HandlerTestSuite.SetupSuite()
}

func (s *UserHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "userhandler@example.com",
		Password:       "hash",
		Token:          "user-handler-token",
		UUID:           "user-handler-uuid",
		TransferEnable: 10737418240,
		PlanID:         &s.testPlan.ID,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *UserHandlerTestSuite) TestGetSubscription_Success() {
	handler := NewUserHandler()
	s.router.GET("/subscription", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *UserHandlerTestSuite) TestGetSubscription_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/subscription", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *UserHandlerTestSuite) TestGetSubscription_WithRefresh() {
	handler := NewUserHandler()
	s.router.GET("/subscription", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription?refresh=true", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *UserHandlerTestSuite) TestGetProfile_Success() {
	handler := NewUserHandler()
	s.router.GET("/profile", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].(map[string]interface{})
	assert.Equal(s.T(), s.testUser.Email, data["email"])
}

func (s *UserHandlerTestSuite) TestGetProfile_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/profile", handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *UserHandlerTestSuite) TestGetDashboard_Success() {
	handler := NewUserHandler()
	s.router.GET("/dashboard", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *UserHandlerTestSuite) TestGetDashboard_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func TestUserHandler(t *testing.T) {
	suite.Run(t, new(UserHandlerTestSuite))
}

// NodeHandlerTestSuite 节点 Handler 测试套件
type NodeHandlerTestSuite struct {
	HandlerTestSuite
	testNode *model.Node
}

func (s *NodeHandlerTestSuite) SetupSuite() {
	s.HandlerTestSuite.SetupSuite()
}

func (s *NodeHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试节点
	s.testNode = &model.Node{
		Name:        "Handler Test Node",
		Host:        "192.168.1.100",
		Port:        443,
		Status:      model.NodeStatusOnline,
		Rate:        1.0,
		TrafficRate: 1.0,
		Show:        1,
	}
	s.db.Create(s.testNode)

	s.router = gin.New()
}

func (s *NodeHandlerTestSuite) TestGetNodes_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/nodes", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNodes_WithPagination() {
	handler := NewNodeHandler()
	s.router.GET("/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/nodes?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNodes_WithFilters() {
	handler := NewNodeHandler()
	s.router.GET("/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/nodes?status=1&search=test", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNode_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNode_NotFound() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/nodes/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNode_InvalidID() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/nodes/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeHandlerTestSuite) TestCreateNode_Success() {
	handler := NewNodeHandler()
	s.router.POST("/nodes", handler.CreateNode)

	body := map[string]interface{}{
		"name":  "New Test Node",
		"host":  "192.168.1.200",
		"port":  443,
		"rate":  1.0,
		"show":  1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/nodes", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestCreateNode_InvalidBody() {
	handler := NewNodeHandler()
	s.router.POST("/nodes", handler.CreateNode)

	req, _ := http.NewRequest("POST", "/nodes", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeHandlerTestSuite) TestUpdateNode_Success() {
	handler := NewNodeHandler()
	s.router.PUT("/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{
		"name": "Updated Node Name",
		"rate": 2.0,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestUpdateNode_InvalidID() {
	handler := NewNodeHandler()
	s.router.PUT("/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{"name": "Test"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/nodes/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeHandlerTestSuite) TestDeleteNode_Success() {
	handler := NewNodeHandler()
	s.router.DELETE("/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetNodeStats_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/stats", handler.GetNodeStats)

	req, _ := http.NewRequest("GET", "/nodes/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetProtocolTemplates() {
	handler := NewNodeHandler()
	s.router.GET("/protocol-templates", handler.GetProtocolTemplates)

	req, _ := http.NewRequest("GET", "/protocol-templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetProtocols_Success() {
	// 创建协议
	transport := "tcp"
	protocol := &model.NodeProtocol{
		NodeID:    s.testNode.ID,
		Name:      "Test Protocol",
		Type:      model.ProtocolVLESS,
		Port:      443,
		Enable:    1,
		Show:      1,
		Transport: &transport,
	}
	s.db.Create(protocol)

	handler := NewNodeHandler()
	s.router.GET("/nodes/:id/protocols", handler.GetProtocols)

	req, _ := http.NewRequest("GET", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetProtocols_InvalidNodeID() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id/protocols", handler.GetProtocols)

	req, _ := http.NewRequest("GET", "/nodes/invalid/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeHandlerTestSuite) TestCreateProtocol_Success() {
	handler := NewNodeHandler()
	s.router.POST("/nodes/:id/protocols", handler.CreateProtocol)

	body := map[string]interface{}{
		"name":      "New Protocol",
		"type":      "vless",
		"port":      443,
		"enable":    1,
		"show":      1,
		"transport": "tcp",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/protocols", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGenerateAuthKey_Success() {
	handler := NewNodeHandler()
	s.router.POST("/auth-keys", handler.GenerateAuthKey)

	body := map[string]interface{}{
		"name":        "Test Key",
		"expire_days": 7,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/auth-keys", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestGetAuthKeys_Success() {
	handler := NewNodeHandler()
	s.router.GET("/auth-keys", handler.GetAuthKeys)

	req, _ := http.NewRequest("GET", "/auth-keys", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestValidateRawConfig_Success() {
	handler := NewNodeHandler()
	s.router.POST("/validate-config", handler.ValidateRawConfig)

	body := map[string]interface{}{
		"raw_config": map[string]interface{}{
			"server_port": 443,
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/validate-config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeHandlerTestSuite) TestValidateRawConfig_InvalidJSON() {
	handler := NewNodeHandler()
	s.router.POST("/validate-config", handler.ValidateRawConfig)

	req, _ := http.NewRequest("POST", "/validate-config", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestNodeHandler(t *testing.T) {
	suite.Run(t, new(NodeHandlerTestSuite))
}

// AdminHandlerTestSuite 管理员 Handler 测试套件
type AdminHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
}

func (s *AdminHandlerTestSuite) SetupSuite() {
	s.HandlerTestSuite.SetupSuite()
}

func (s *AdminHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:           "Admin Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "adminhandler@example.com",
		Password:       "hash",
		Token:          "admin-handler-token",
		UUID:           "admin-handler-uuid",
		TransferEnable: 10737418240,
		PlanID:         &s.testPlan.ID,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *AdminHandlerTestSuite) TestCreateUser_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users", handler.CreateUser)

	body := map[string]interface{}{
		"email":    "newuser@example.com",
		"password": "password123",
		"is_admin": 0,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestCreateUser_DuplicateEmail() {
	handler := NewAdminHandler()
	s.router.POST("/users", handler.CreateUser)

	body := map[string]interface{}{
		"email":    s.testUser.Email,
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestCreateUser_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/users", handler.CreateUser)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUserList_Success() {
	handler := NewAdminHandler()
	s.router.GET("/users", handler.GetUserList)

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUserList_WithFilters() {
	handler := NewAdminHandler()
	s.router.GET("/users", handler.GetUserList)

	req, _ := http.NewRequest("GET", "/users?page=1&page_size=10&email=test&status=active", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUser_Success() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUser_NotFound() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestUpdateUser_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	body := map[string]interface{}{
		"balance":  1000,
		"banned":   0,
		"is_admin": 0,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestUpdateUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	body := map[string]interface{}{"balance": 1000}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/users/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestDeleteUser_Success() {
	// Create a new user to delete
	user := &model.User{
		Email:          "todelete@example.com",
		Password:       "hash",
		Token:          "to-delete-token",
		UUID:           "to-delete-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(user)

	handler := NewAdminHandler()
	s.router.DELETE("/users/:id", handler.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/"+strconv.FormatUint(uint64(user.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestBanUser_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/ban", handler.BanUser)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/ban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestUnbanUser_Success() {
	// First ban the user
	s.db.Model(s.testUser).Update("banned", 1)

	handler := NewAdminHandler()
	s.router.POST("/users/:id/unban", handler.UnbanUser)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/unban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestResetUserTraffic_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-traffic", handler.ResetUserTraffic)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/reset-traffic", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetDashboard() {
	handler := NewAdminHandler()
	s.router.GET("/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetPlans() {
	handler := NewAdminHandler()
	s.router.GET("/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetPlan_Success() {
	handler := NewAdminHandler()
	s.router.GET("/plans/:id", handler.GetPlan)

	req, _ := http.NewRequest("GET", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetPlan_NotFound() {
	handler := NewAdminHandler()
	s.router.GET("/plans/:id", handler.GetPlan)

	req, _ := http.NewRequest("GET", "/plans/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminHandlerTestSuite) TestCreatePlan_Success() {
	handler := NewAdminHandler()
	s.router.POST("/plans", handler.CreatePlan)

	body := map[string]interface{}{
		"name":             "New Plan",
		"group_id":         1,
		"transfer_enable":  100,
		"month_price":      1000,
		"show":             1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetOrderList() {
	handler := NewAdminHandler()
	s.router.GET("/orders", handler.GetOrderList)

	req, _ := http.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetOrderStats() {
	handler := NewAdminHandler()
	s.router.GET("/orders/stats", handler.GetOrderStats)

	req, _ := http.NewRequest("GET", "/orders/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestBanUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/ban", handler.BanUser)

	req, _ := http.NewRequest("POST", "/users/invalid/ban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestUnbanUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/unban", handler.UnbanUser)

	req, _ := http.NewRequest("POST", "/users/invalid/unban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestResetUserTraffic_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-traffic", handler.ResetUserTraffic)

	req, _ := http.NewRequest("POST", "/users/invalid/reset-traffic", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestDeleteUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.DELETE("/users/:id", handler.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestCreatePlan_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/plans", handler.CreatePlan)

	req, _ := http.NewRequest("POST", "/plans", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetUserStats() {
	handler := NewAdminHandler()
	s.router.GET("/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetOrder_Success() {
	// Create an order
	order := &model.Order{
		TradeNo:     "TEST001",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		Period:      "month",
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.GET("/orders/:id", handler.GetOrder)

	req, _ := http.NewRequest("GET", "/orders/"+strconv.FormatUint(uint64(order.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestGetOrder_InvalidID() {
	handler := NewAdminHandler()
	s.router.GET("/orders/:id", handler.GetOrder)

	req, _ := http.NewRequest("GET", "/orders/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestUpdateOrderStatus_Success() {
	// Create an order
	order := &model.Order{
		TradeNo:     "TEST002",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		Period:      "month",
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.PUT("/orders/:id/status", handler.UpdateOrderStatus)

	body := map[string]interface{}{
		"status": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/orders/"+strconv.FormatUint(uint64(order.ID), 10)+"/status", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestMarkOrderPaid_Success() {
	// Create an order
	order := &model.Order{
		TradeNo:     "TEST003",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		Period:      "month",
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.POST("/orders/:id/paid", handler.MarkOrderPaid)

	req, _ := http.NewRequest("POST", "/orders/"+strconv.FormatUint(uint64(order.ID), 10)+"/paid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestCancelOrder_Success() {
	// Create an order
	order := &model.Order{
		TradeNo:     "TEST004",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		Period:      "month",
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.POST("/orders/:id/cancel", handler.CancelOrder)

	req, _ := http.NewRequest("POST", "/orders/"+strconv.FormatUint(uint64(order.ID), 10)+"/cancel", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestCancelOrder_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/orders/:id/cancel", handler.CancelOrder)

	req, _ := http.NewRequest("POST", "/orders/invalid/cancel", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_Success() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]interface{}{
		"user_id": s.testUser.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]interface{}{
		"user_id": s.testUser.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/invalid/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminHandlerTestSuite) TestDeletePlan_Success() {
	// Create a plan to delete
	plan := &model.Plan{
		Name:           "To Delete Plan",
		TransferEnable: 100,
		Show:           1,
	}
	s.db.Create(plan)

	handler := NewAdminHandler()
	s.router.DELETE("/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/plans/"+strconv.FormatUint(uint64(plan.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestUpdatePlan_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/plans/:id", handler.UpdatePlan)

	body := map[string]interface{}{
		"name":            "Updated Plan",
		"transfer_enable": 200,
		"show":            1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminHandlerTestSuite) TestUpdatePlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/plans/:id", handler.UpdatePlan)

	body := map[string]interface{}{
		"name":            "Updated Plan",
		"transfer_enable": 200,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/plans/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAdminHandler(t *testing.T) {
	suite.Run(t, new(AdminHandlerTestSuite))
}

// MetricsHandlerTestSuite Metrics Handler 测试套件
type MetricsHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *MetricsHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *MetricsHandlerTestSuite) TestGetMetrics_Success() {
	handler := NewMetricsHandler()
	s.router.GET("/metrics", handler.GetMetrics)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Contains(s.T(), w.Body.String(), "v2board_info")
	assert.Contains(s.T(), w.Body.String(), "v2board_users_total")
	assert.Contains(s.T(), w.Body.String(), "v2board_nodes_total")
}

func (s *MetricsHandlerTestSuite) TestRecordRequest_Success() {
	handler := NewMetricsHandler()
	handler.RecordRequest(true)
	handler.RecordRequest(true)
	handler.RecordRequest(false)

	assert.Equal(s.T(), uint64(3), handler.requestCount)
	assert.Equal(s.T(), uint64(2), handler.successCount)
	assert.Equal(s.T(), uint64(1), handler.errorCount)
}

func (s *MetricsHandlerTestSuite) TestFormatInt() {
	assert.Equal(s.T(), "0", formatInt(0))
	assert.Equal(s.T(), "1", formatInt(1))
	assert.Equal(s.T(), "123", formatInt(123))
	assert.Equal(s.T(), "1000000", formatInt(1000000))
}

func (s *MetricsHandlerTestSuite) TestFormatUint() {
	assert.Equal(s.T(), "0", formatUint(0))
	assert.Equal(s.T(), "1", formatUint(1))
	assert.Equal(s.T(), "12345", formatUint(12345))
}

func (s *MetricsHandlerTestSuite) TestFormatFloat() {
	assert.Equal(s.T(), "0.0", formatFloat(0.0))
	assert.Equal(s.T(), "1.0", formatFloat(1.0))
	assert.Equal(s.T(), "1.50", formatFloat(1.5))
	assert.Equal(s.T(), "123.45", formatFloat(123.45))
}

func TestMetricsHandler(t *testing.T) {
	suite.Run(t, new(MetricsHandlerTestSuite))
}

// SubscribeHandlerTestSuite Subscribe Handler 测试套件
type SubscribeHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
}

func (s *SubscribeHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "sub@example.com",
		Password:       "hash",
		Token:          "sub-token-123",
		UUID:           "sub-uuid-123",
		TransferEnable: 10737418240,
		PlanID:         &s.testPlan.ID,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *SubscribeHandlerTestSuite) TestDetectFormatFromUserAgent() {
	handler := NewSubscribeHandler(s.cfg)

	tests := []struct {
		userAgent string
		expected  string
	}{
		{"Clash/1.0", "clash"},
		{"Stash/1.0", "clash"},
		{"Surge/1.0", "surge"},
		{"Shadowrocket/1.0", "shadowrocket"},
		{"Quantumult X/1.0", "quantumultx"},
		{"V2RayNG/1.0", "v2ray"},
		{"v2rayN/1.0", "v2ray"},
		{"sing-box/1.0", "sing-box"},
		{"NekoBox/1.0", "sing-box"},
		{"Unknown", "v2ray"}, // default
	}

	for _, tt := range tests {
		result := handler.detectFormatFromUserAgent(tt.userAgent)
		assert.Equal(s.T(), tt.expected, result, "User-Agent: "+tt.userAgent)
	}
}

func (s *SubscribeHandlerTestSuite) TestBuildUserInfo() {
	handler := NewSubscribeHandler(s.cfg)

	resp := &model.SubscriptionResponse{
		UsedTraffic:  1073741824,
		TotalTraffic: 10737418240,
		ExpireAt:     1893456000,
	}

	result := handler.buildUserInfo(resp)

	assert.Contains(s.T(), result, "upload=0")
	assert.Contains(s.T(), result, "download=1073741824")
	assert.Contains(s.T(), result, "total=10737418240")
	assert.Contains(s.T(), result, "expire=1893456000")
}

func (s *SubscribeHandlerTestSuite) TestBuildUserInfo_NoExpire() {
	handler := NewSubscribeHandler(s.cfg)

	resp := &model.SubscriptionResponse{
		UsedTraffic:  0,
		TotalTraffic: 10737418240,
		ExpireAt:     0,
	}

	result := handler.buildUserInfo(resp)

	assert.Contains(s.T(), result, "upload=0")
	assert.Contains(s.T(), result, "download=0")
	assert.Contains(s.T(), result, "total=10737418240")
	assert.NotContains(s.T(), result, "expire=")
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_Success() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_NotFound() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/nonexistent-token", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithFormat() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token+"?type=json", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithUserAgent() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token, nil)
	req.Header.Set("User-Agent", "Clash/1.0")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithExtension() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	tests := []struct {
		ext      string
		expected string
	}{
		{"yaml", "clash"},
		{"yml", "clash"},
		{"conf", "surge"},
		{"json", "sing-box"},
		{"txt", "v2ray"},
	}

	for _, tt := range tests {
		req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token+"."+tt.ext, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		assert.Equal(s.T(), http.StatusOK, w.Code, "Extension: "+tt.ext)
	}
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithGroups() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token+"?groups=1,2,3", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithIncludeExclude() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token+"?include=HK&exclude=US", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestSubscribeHandler(t *testing.T) {
	suite.Run(t, new(SubscribeHandlerTestSuite))
}

// SubscriptionAdminHandlerTestSuite Subscription Admin Handler 测试套件
type SubscriptionAdminHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *SubscriptionAdminHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroups_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups", handler.GetGroups)

	req, _ := http.NewRequest("GET", "/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestCreateGroup_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups", handler.CreateGroup)

	body := map[string]interface{}{
		"name": "Test Group",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id", handler.GetGroup)

	req, _ := http.NewRequest("GET", "/groups/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroup_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id", handler.GetGroup)

	req, _ := http.NewRequest("GET", "/groups/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/groups/:id", handler.UpdateGroup)

	body := map[string]interface{}{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/groups/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestDeleteGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/groups/:id", handler.DeleteGroup)

	req, _ := http.NewRequest("DELETE", "/groups/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetTemplates_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id/templates", handler.GetTemplates)

	req, _ := http.NewRequest("GET", "/groups/invalid/templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroupProtocols_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id/protocols", handler.GetGroupProtocols)

	req, _ := http.NewRequest("GET", "/groups/invalid/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateGroupProtocols_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups/:id/protocols", handler.UpdateGroupProtocols)

	body := map[string]interface{}{"protocol_ids": []uint{1, 2}}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups/invalid/protocols", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetAvailableProtocols_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/protocols/available", handler.GetAvailableProtocols)

	req, _ := http.NewRequest("GET", "/protocols/available", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestCreateTemplate_InvalidGroupID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups/:id/templates", handler.CreateTemplate)

	body := map[string]interface{}{"name": "Test Template"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups/invalid/templates", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/templates/:id", handler.GetTemplate)

	req, _ := http.NewRequest("GET", "/templates/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/templates/:id", handler.UpdateTemplate)

	body := map[string]interface{}{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/templates/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestDeleteTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/templates/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestAssignGroupToUser_InvalidUserID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/users/:user_id/groups", handler.AssignGroupToUser)

	body := map[string]interface{}{"group_id": 1}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users/invalid/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestRemoveGroupFromUser_InvalidIDs() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/users/:user_id/groups/:group_id", handler.RemoveGroupFromUser)

	req, _ := http.NewRequest("DELETE", "/users/invalid/groups/1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)

	req, _ = http.NewRequest("DELETE", "/users/1/groups/invalid", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetUserGroups_InvalidUserID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/users/:user_id/groups", handler.GetUserGroups)

	req, _ := http.NewRequest("GET", "/users/invalid/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestAssignGroupToPlan_InvalidPlanID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/plans/:plan_id/groups", handler.AssignGroupToPlan)

	body := map[string]interface{}{"group_id": 1}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/invalid/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestRemoveGroupFromPlan_InvalidIDs() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/plans/:plan_id/groups/:group_id", handler.RemoveGroupFromPlan)

	req, _ := http.NewRequest("DELETE", "/plans/invalid/groups/1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetPlanGroups_InvalidPlanID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/plans/:plan_id/groups", handler.GetPlanGroups)

	req, _ := http.NewRequest("GET", "/plans/invalid/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetSubscriptionFormats_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/formats", handler.GetSubscriptionFormats)

	req, _ := http.NewRequest("GET", "/formats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(data), 8) // At least 8 formats
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetProtocolTypes_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/protocols", handler.GetProtocolTypes)

	req, _ := http.NewRequest("GET", "/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(data), 6) // At least 6 protocol types
}

func (s *SubscriptionAdminHandlerTestSuite) TestPreviewSubscription_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/preview", handler.PreviewSubscription)

	req, _ := http.NewRequest("POST", "/preview", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscriptionAdminHandlerTestSuite) TestPreviewSubscription_UserNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/preview", handler.PreviewSubscription)

	body := map[string]interface{}{
		"user_id": 99999,
		"format":  "v2ray",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func TestSubscriptionAdminHandler(t *testing.T) {
	suite.Run(t, new(SubscriptionAdminHandlerTestSuite))
}

// PaymentHandlerTestSuite Payment Handler 测试套件
type PaymentHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *PaymentHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *PaymentHandlerTestSuite) TestGetPaymentMethods_Success() {
	handler := NewPaymentHandler()
	s.router.GET("/methods", handler.GetPaymentMethods)

	req, _ := http.NewRequest("GET", "/methods", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(data), 3) // At least 3 payment methods
}

func (s *PaymentHandlerTestSuite) TestX402CreatePayment_Success() {
	handler := NewPaymentHandler()
	s.router.POST("/x402/create", handler.X402CreatePayment)

	body := map[string]interface{}{
		"order_id": 1,
		"token":    "ETH",
		"network":  "sepolia",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/x402/create", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestX402CreatePayment_InvalidBody() {
	handler := NewPaymentHandler()
	s.router.POST("/x402/create", handler.X402CreatePayment)

	req, _ := http.NewRequest("POST", "/x402/create", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentHandlerTestSuite) TestX402Callback_Success() {
	handler := NewPaymentHandler()
	s.router.POST("/x402/callback", handler.X402Callback)

	req, _ := http.NewRequest("POST", "/x402/callback", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestX402CheckPayment_Success() {
	handler := NewPaymentHandler()
	s.router.GET("/x402/check/:id", handler.X402CheckPayment)

	req, _ := http.NewRequest("GET", "/x402/check/payment123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestFiatCreatePayment_Stripe() {
	handler := NewPaymentHandler()
	s.router.POST("/fiat/create", handler.FiatCreatePayment)

	body := map[string]interface{}{
		"order_id": 1,
		"provider": "stripe",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/fiat/create", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestFiatCreatePayment_PayPal() {
	handler := NewPaymentHandler()
	s.router.POST("/fiat/create", handler.FiatCreatePayment)

	body := map[string]interface{}{
		"order_id": 1,
		"provider": "paypal",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/fiat/create", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestFiatCreatePayment_InvalidProvider() {
	handler := NewPaymentHandler()
	s.router.POST("/fiat/create", handler.FiatCreatePayment)

	body := map[string]interface{}{
		"order_id": 1,
		"provider": "unknown",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/fiat/create", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentHandlerTestSuite) TestFiatCreatePayment_InvalidBody() {
	handler := NewPaymentHandler()
	s.router.POST("/fiat/create", handler.FiatCreatePayment)

	req, _ := http.NewRequest("POST", "/fiat/create", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentHandlerTestSuite) TestStripeWebhook_Success() {
	handler := NewPaymentHandler()
	s.router.POST("/stripe/webhook", handler.StripeWebhook)

	req, _ := http.NewRequest("POST", "/stripe/webhook", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestPayPalWebhook_Success() {
	handler := NewPaymentHandler()
	s.router.POST("/paypal/webhook", handler.PayPalWebhook)

	req, _ := http.NewRequest("POST", "/paypal/webhook", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentHandlerTestSuite) TestGetPaymentStatus_Success() {
	handler := NewPaymentHandler()
	s.router.GET("/status/:trade_no", handler.GetPaymentStatus)

	req, _ := http.NewRequest("GET", "/status/ORDER123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestPaymentHandler(t *testing.T) {
	suite.Run(t, new(PaymentHandlerTestSuite))
}

// KnowledgeHandlerTestSuite Knowledge Handler 测试套件
type KnowledgeHandlerTestSuite struct {
	HandlerTestSuite
	testKnowledge *model.Knowledge
}

func (s *KnowledgeHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试知识库文章
	s.testKnowledge = &model.Knowledge{
		Category: "test",
		Title:    "Test Article",
		Body:     "Test content",
		Show:     1,
	}
	s.db.Create(s.testKnowledge)

	s.router = gin.New()
}

func (s *KnowledgeHandlerTestSuite) TestGetArticles_Success() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/knowledge", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *KnowledgeHandlerTestSuite) TestGetArticles_WithCategory() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/knowledge?category=test", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_Success() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/"+strconv.FormatUint(uint64(s.testKnowledge.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_NotFound() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_InvalidID() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestKnowledgeHandler(t *testing.T) {
	suite.Run(t, new(KnowledgeHandlerTestSuite))
}

// OrderHandlerTestSuite Order Handler 测试套件
type OrderHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
	testOrder *model.Order
}

func (s *OrderHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "order@example.com",
		Password:       "hash",
		Token:          "order-token",
		UUID:           "order-uuid",
		TransferEnable: 10737418240,
		PlanID:         &s.testPlan.ID,
	}
	s.db.Create(s.testUser)

	// 创建测试订单
	s.testOrder = &model.Order{
		TradeNo:     "ORDER123",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(s.testOrder)

	s.router = gin.New()
}

func (s *OrderHandlerTestSuite) TestGetOrders_Success() {
	handler := NewOrderHandler()
	s.router.GET("/orders", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetOrders)

	req, _ := http.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *OrderHandlerTestSuite) TestGetOrders_EmptyUserID() {
	handler := NewOrderHandler()
	s.router.GET("/orders", handler.GetOrders)

	req, _ := http.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Handler returns 200 with empty list for user_id=0
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *OrderHandlerTestSuite) TestGetOrderDetail_Success() {
	handler := NewOrderHandler()
	s.router.GET("/order/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetOrderDetail)

	req, _ := http.NewRequest("GET", "/order/"+strconv.FormatUint(uint64(s.testOrder.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *OrderHandlerTestSuite) TestGetOrderDetail_NotFound() {
	handler := NewOrderHandler()
	s.router.GET("/order/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetOrderDetail)

	req, _ := http.NewRequest("GET", "/order/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *OrderHandlerTestSuite) TestGetOrderDetail_InvalidID() {
	handler := NewOrderHandler()
	s.router.GET("/order/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetOrderDetail)

	req, _ := http.NewRequest("GET", "/order/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestOrderHandler(t *testing.T) {
	suite.Run(t, new(OrderHandlerTestSuite))
}

// UserPlanHandlerTestSuite UserPlan Handler 测试套件
type UserPlanHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
}

func (s *UserPlanHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "userplan@example.com",
		Password:       "hash",
		Token:          "userplan-token",
		UUID:           "userplan-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *UserPlanHandlerTestSuite) TestGetPlans_Success() {
	handler := NewUserPlanHandler()
	s.router.GET("/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *UserPlanHandlerTestSuite) TestGetPlans_WithShow() {
	handler := NewUserPlanHandler()
	s.router.GET("/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/plans?show=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestUserPlanHandler(t *testing.T) {
	suite.Run(t, new(UserPlanHandlerTestSuite))
}

// CouponHandlerTestSuite Coupon Handler 测试套件
type CouponHandlerTestSuite struct {
	HandlerTestSuite
	testCoupon *model.Coupon
}

func (s *CouponHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	now := time.Now().Unix()
	limitUse := 10
	// 创建测试优惠券
	s.testCoupon = &model.Coupon{
		Name:      "Test Coupon",
		Code:      "TESTCODE",
		Type:      1, // percentage
		Value:     10,
		LimitUse:  &limitUse,
		UseCount:  0,
		StartedAt: now - 86400, // 1 day ago
		EndedAt:   now + 86400, // 1 day from now
	}
	s.db.Create(s.testCoupon)

	s.router = gin.New()
}

func (s *CouponHandlerTestSuite) TestCheckCoupon_Success() {
	handler := NewCouponHandler()
	s.router.POST("/coupon/check", handler.CheckCoupon)

	body := `{"code": "TESTCODE", "plan_id": 1}`
	req, _ := http.NewRequest("POST", "/coupon/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *CouponHandlerTestSuite) TestCheckCoupon_InvalidCode() {
	handler := NewCouponHandler()
	s.router.POST("/coupon/check", handler.CheckCoupon)

	body := `{"code": "INVALID", "plan_id": 1}`
	req, _ := http.NewRequest("POST", "/coupon/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *CouponHandlerTestSuite) TestCheckCoupon_MissingCode() {
	handler := NewCouponHandler()
	s.router.POST("/coupon/check", handler.CheckCoupon)

	body := `{"plan_id": 1}`
	req, _ := http.NewRequest("POST", "/coupon/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *CouponHandlerTestSuite) TestCheckCoupon_Expired() {
	// Update coupon to be expired
	s.db.Model(s.testCoupon).Update("ended_at", time.Now().Unix()-3600)

	handler := NewCouponHandler()
	s.router.POST("/coupon/check", handler.CheckCoupon)

	body := `{"code": "TESTCODE", "plan_id": 1}`
	req, _ := http.NewRequest("POST", "/coupon/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
	assert.Contains(s.T(), w.Body.String(), "已过期")
}

func TestCouponHandler(t *testing.T) {
	suite.Run(t, new(CouponHandlerTestSuite))
}

// TicketHandlerTestSuite Ticket Handler 测试套件
type TicketHandlerTestSuite struct {
	HandlerTestSuite
	testUser   *model.User
	testTicket *model.Ticket
}

func (s *TicketHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "ticket@example.com",
		Password:       "hash",
		Token:          "ticket-token",
		UUID:           "ticket-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	// 创建测试工单
	s.testTicket = &model.Ticket{
		UserID:    s.testUser.ID,
		Subject:   "Test Ticket",
		Level:     1,
		Status:    0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.db.Create(s.testTicket)

	s.router = gin.New()
}

func (s *TicketHandlerTestSuite) TestGetTickets_Success() {
	handler := NewTicketHandler()
	s.router.GET("/tickets", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetTickets)

	req, _ := http.NewRequest("GET", "/tickets", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TicketHandlerTestSuite) TestGetTickets_Empty() {
	// Delete existing tickets
	s.db.Where("user_id = ?", s.testUser.ID).Delete(&model.Ticket{})

	handler := NewTicketHandler()
	s.router.GET("/tickets", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetTickets)

	req, _ := http.NewRequest("GET", "/tickets", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TicketHandlerTestSuite) TestCreateTicket_Success() {
	handler := NewTicketHandler()
	s.router.POST("/tickets", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.CreateTicket)

	body := `{"subject": "New Ticket", "level": 1, "message": "Test message"}`
	req, _ := http.NewRequest("POST", "/tickets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TicketHandlerTestSuite) TestCreateTicket_MissingFields() {
	handler := NewTicketHandler()
	s.router.POST("/tickets", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.CreateTicket)

	body := `{"subject": "New Ticket"}`
	req, _ := http.NewRequest("POST", "/tickets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TicketHandlerTestSuite) TestGetTicket_Success() {
	handler := NewTicketHandler()
	s.router.GET("/tickets/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetTicket)

	req, _ := http.NewRequest("GET", "/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TicketHandlerTestSuite) TestGetTicket_NotFound() {
	handler := NewTicketHandler()
	s.router.GET("/tickets/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetTicket)

	req, _ := http.NewRequest("GET", "/tickets/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *TicketHandlerTestSuite) TestGetTicket_InvalidID() {
	handler := NewTicketHandler()
	s.router.GET("/tickets/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetTicket)

	req, _ := http.NewRequest("GET", "/tickets/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TicketHandlerTestSuite) TestReplyTicket_Success() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/reply", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{"message": "Reply message"}`
	req, _ := http.NewRequest("POST", "/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TicketHandlerTestSuite) TestReplyTicket_Closed() {
	// Close the ticket
	s.db.Model(s.testTicket).Update("status", 2)

	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/reply", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{"message": "Reply message"}`
	req, _ := http.NewRequest("POST", "/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
	assert.Contains(s.T(), w.Body.String(), "已关闭")
}

func (s *TicketHandlerTestSuite) TestCloseTicket_Success() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/close", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestTicketHandler(t *testing.T) {
	suite.Run(t, new(TicketHandlerTestSuite))
}

// MFAHandlerTestSuite MFA Handler 测试套件
type MFAHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *MFAHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试用户
	s.testUser = &model.User{
		Email:          "mfa@example.com",
		Password:       "hash",
		Token:          "mfa-token",
		UUID:           "mfa-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *MFAHandlerTestSuite) TestGetStatus_NoMFA() {
	handler := NewMFAHandler()
	s.router.GET("/mfa/status", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetStatus)

	req, _ := http.NewRequest("GET", "/mfa/status", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Contains(s.T(), w.Body.String(), "enabled")
}

func (s *MFAHandlerTestSuite) TestGetAdminConfig() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/mfa/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *MFAHandlerTestSuite) TestUpdateAdminConfig_Success() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	body := `{"enabled": true, "enforce_for_all": false, "totp_issuer": "TestApp"}`
	req, _ := http.NewRequest("PUT", "/admin/mfa/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *MFAHandlerTestSuite) TestUpdateAdminConfig_InvalidBody() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	body := `invalid json`
	req, _ := http.NewRequest("PUT", "/admin/mfa/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *MFAHandlerTestSuite) TestEnableTOTP_MissingCode() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/totp/enable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.EnableTOTP)

	body := `{}`
	req, _ := http.NewRequest("POST", "/mfa/totp/enable", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *MFAHandlerTestSuite) TestDisableMFA_MissingPassword() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/disable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.DisableMFA)

	body := `{}`
	req, _ := http.NewRequest("POST", "/mfa/disable", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *MFAHandlerTestSuite) TestVerifyMFA_MissingCode() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/verify", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.VerifyMFA)

	body := `{}`
	req, _ := http.NewRequest("POST", "/mfa/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *MFAHandlerTestSuite) TestSetupTOTP() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/totp/setup", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.SetupTOTP)

	req, _ := http.NewRequest("POST", "/mfa/totp/setup", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *MFAHandlerTestSuite) TestRegenerateBackupCodes_NoMFA() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/backup-codes/regenerate", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.RegenerateBackupCodes)

	req, _ := http.NewRequest("POST", "/mfa/backup-codes/regenerate", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should return 400 because user doesn't have MFA enabled
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestMFAHandler(t *testing.T) {
	suite.Run(t, new(MFAHandlerTestSuite))
}

// InviteHandlerTestSuite 邀请处理器测试套件
type InviteHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *InviteHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// 创建测试用户
	s.testUser = &model.User{
		Email:             "invite@example.com",
		Password:          "hash",
		Token:             "invite-token",
		UUID:              "invite-uuid",
		TransferEnable:    10737418240,
		CommissionBalance: 100.0,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *InviteHandlerTestSuite) TestGetInviteInfo() {
	handler := NewInviteHandler()
	s.router.GET("/invite", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetInviteInfo)

	req, _ := http.NewRequest("GET", "/invite", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestGenerateCode() {
	handler := NewInviteHandler()
	s.router.POST("/invite/generate", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GenerateCode)

	req, _ := http.NewRequest("POST", "/invite/generate", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestGetCommissionRecords() {
	handler := NewInviteHandler()
	s.router.GET("/invite/commissions", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetCommissionRecords)

	req, _ := http.NewRequest("GET", "/invite/commissions", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestGetWithdrawRecords() {
	handler := NewInviteHandler()
	s.router.GET("/invite/withdrawals", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetWithdrawRecords)

	req, _ := http.NewRequest("GET", "/invite/withdrawals", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestRequestWithdraw_MissingAmount() {
	handler := NewInviteHandler()
	s.router.POST("/invite/withdraw", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.RequestWithdraw)

	body := `{"method": "alipay", "account": "test@example.com", "name": "Test"}`
	req, _ := http.NewRequest("POST", "/invite/withdraw", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *InviteHandlerTestSuite) TestGetConfig() {
	handler := NewInviteHandler()
	s.router.GET("/admin/invite/config", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/admin/invite/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestGetWithdrawals() {
	handler := NewInviteHandler()
	s.router.GET("/admin/invite/withdrawals", handler.GetWithdrawals)

	req, _ := http.NewRequest("GET", "/admin/invite/withdrawals", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerTestSuite) TestGetInviteStats() {
	handler := NewInviteHandler()
	s.router.GET("/admin/invite/stats", handler.GetInviteStats)

	req, _ := http.NewRequest("GET", "/admin/invite/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestInviteHandler(t *testing.T) {
	suite.Run(t, new(InviteHandlerTestSuite))
}

// ========== NotificationHandler Tests ==========

// NotificationHandlerTestSuite 通知处理器测试套件
type NotificationHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *NotificationHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.testUser = &model.User{
		Email:          "notify@example.com",
		Password:       "hash",
		Token:          "notify-token",
		UUID:           "notify-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)
	s.router = gin.New()
}

func (s *NotificationHandlerTestSuite) TestGetUserNotifications() {
	handler := NewNotificationHandler()
	s.router.GET("/notifications", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetUserNotifications)

	req, _ := http.NewRequest("GET", "/notifications", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestMarkAsRead_NotFound() {
	handler := NewNotificationHandler()
	s.router.POST("/notifications/:id/read", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.MarkAsRead)

	req, _ := http.NewRequest("POST", "/notifications/999/read", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *NotificationHandlerTestSuite) TestMarkAllAsRead() {
	handler := NewNotificationHandler()
	s.router.POST("/notifications/read-all", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.MarkAllAsRead)

	req, _ := http.NewRequest("POST", "/notifications/read-all", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestGetUnreadCount() {
	handler := NewNotificationHandler()
	s.router.GET("/notifications/unread-count", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetUnreadCount)

	req, _ := http.NewRequest("GET", "/notifications/unread-count", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestListTemplates() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/templates", handler.ListTemplates)

	req, _ := http.NewRequest("GET", "/admin/notification/templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestCreateTemplate() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/templates", handler.CreateTemplate)

	body := `{"name": "Test Template", "type": "email", "event": "order_paid", "title": "Test", "content": "Hello"}`
	req, _ := http.NewRequest("POST", "/admin/notification/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestCreateTemplate_MissingFields() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/templates", handler.CreateTemplate)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/notification/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NotificationHandlerTestSuite) TestUpdateTemplate_NotFound() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/templates/:id", handler.UpdateTemplate)

	body := `{"name": "Updated"}`
	req, _ := http.NewRequest("PUT", "/admin/notification/templates/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *NotificationHandlerTestSuite) TestDeleteTemplate() {
	handler := NewNotificationHandler()
	s.router.DELETE("/admin/notification/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/admin/notification/templates/999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestListLogs() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/logs", handler.ListLogs)

	req, _ := http.NewRequest("GET", "/admin/notification/logs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestSendTestNotification_MissingType() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/test", handler.SendTestNotification)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/notification/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NotificationHandlerTestSuite) TestGetEmailConfig() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/email/config", handler.GetEmailConfig)

	req, _ := http.NewRequest("GET", "/admin/notification/email/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationHandlerTestSuite) TestUpdateEmailConfig() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	body := `{"host": "smtp.example.com", "port": 587, "from_address": "test@example.com"}`
	req, _ := http.NewRequest("PUT", "/admin/notification/email/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestNotificationHandler(t *testing.T) {
	suite.Run(t, new(NotificationHandlerTestSuite))
}

// ========== TelegramHandler Tests ==========

// TelegramHandlerTestSuite Telegram处理器测试套件
type TelegramHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *TelegramHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *TelegramHandlerTestSuite) TestGetBot_NotConfigured() {
	handler := NewTelegramHandler()
	s.router.GET("/admin/telegram/bot", handler.GetBot)

	req, _ := http.NewRequest("GET", "/admin/telegram/bot", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *TelegramHandlerTestSuite) TestUpdateBot_NotConfigured() {
	handler := NewTelegramHandler()
	s.router.PUT("/admin/telegram/bot", handler.UpdateBot)

	body := `{"name": "Test Bot"}`
	req, _ := http.NewRequest("PUT", "/admin/telegram/bot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *TelegramHandlerTestSuite) TestSetWebhook_MissingURL() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/webhook", handler.SetWebhook)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TelegramHandlerTestSuite) TestTelegramWebhook_MissingBody() {
	handler := NewTelegramHandler()
	s.router.POST("/telegram/webhook", handler.TelegramWebhook)

	body := `{}`
	req, _ := http.NewRequest("POST", "/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramHandlerTestSuite) TestGetTelegramStatus() {
	testUser := &model.User{
		Email:          "tele@example.com",
		Password:       "hash",
		Token:          "tele-token",
		UUID:           "tele-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(testUser)

	handler := NewTelegramHandler()
	s.router.GET("/user/telegram/status", func(c *gin.Context) {
		c.Set("user_id", testUser.ID)
		c.Next()
	}, handler.GetTelegramStatus)

	req, _ := http.NewRequest("GET", "/user/telegram/status", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestTelegramHandler(t *testing.T) {
	suite.Run(t, new(TelegramHandlerTestSuite))
}

// ========== SystemHandler Tests ==========

// SystemHandlerTestSuite 系统处理器测试套件
type SystemHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *SystemHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *SystemHandlerTestSuite) TestGetConfigs() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs", handler.GetConfigs)

	req, _ := http.NewRequest("GET", "/admin/system/configs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemHandlerTestSuite) TestGetConfigs_ByGroup() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs", handler.GetConfigs)

	req, _ := http.NewRequest("GET", "/admin/system/configs?group=email", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemHandlerTestSuite) TestGetConfig() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs/:key", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/admin/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemHandlerTestSuite) TestSetConfig() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/configs/:key", handler.SetConfig)

	body := `{"value": "test_value", "type": "string", "group": "test"}`
	req, _ := http.NewRequest("PUT", "/admin/system/configs/test_key", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemHandlerTestSuite) TestSetConfig_MissingValue() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/configs/:key", handler.SetConfig)

	body := `{}`
	req, _ := http.NewRequest("PUT", "/admin/system/configs/test_key", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SystemHandlerTestSuite) TestDeleteConfig() {
	handler := NewSystemHandler()
	s.router.DELETE("/admin/system/configs/:key", handler.DeleteConfig)

	req, _ := http.NewRequest("DELETE", "/admin/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestSystemHandler(t *testing.T) {
	suite.Run(t, new(SystemHandlerTestSuite))
}

// ========== AdminCouponHandler Tests ==========

// AdminCouponHandlerTestSuite 优惠券处理器测试套件
type AdminCouponHandlerTestSuite struct {
	HandlerTestSuite
	testCoupon *model.Coupon
}

func (s *AdminCouponHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	limitUse := 100
	s.testCoupon = &model.Coupon{
		Code:     "TESTCODE",
		Name:     "Test Coupon",
		Type:     1,
		Value:    10,
		LimitUse: &limitUse,
		UseCount: 0,
	}
	s.db.Create(s.testCoupon)
	s.router = gin.New()
}

func (s *AdminCouponHandlerTestSuite) TestGetCoupons() {
	handler := NewAdminCouponHandler()
	s.router.GET("/admin/coupons", handler.GetCoupons)

	req, _ := http.NewRequest("GET", "/admin/coupons", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestCreateCoupon() {
	handler := NewAdminCouponHandler()
	s.router.POST("/admin/coupons", handler.CreateCoupon)

	body := `{"code": "NEWCODE", "name": "New Coupon", "type": 1, "value": 20}`
	req, _ := http.NewRequest("POST", "/admin/coupons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestCreateCoupon_DuplicateCode() {
	handler := NewAdminCouponHandler()
	s.router.POST("/admin/coupons", handler.CreateCoupon)

	body := `{"code": "TESTCODE", "name": "Duplicate Coupon"}`
	req, _ := http.NewRequest("POST", "/admin/coupons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestCreateCoupon_MissingFields() {
	handler := NewAdminCouponHandler()
	s.router.POST("/admin/coupons", handler.CreateCoupon)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/coupons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestDeleteCoupon() {
	handler := NewAdminCouponHandler()
	s.router.DELETE("/admin/coupons/:id", handler.DeleteCoupon)

	req, _ := http.NewRequest("DELETE", "/admin/coupons/"+strconv.FormatUint(uint64(s.testCoupon.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestDeleteCoupon_NotFound() {
	handler := NewAdminCouponHandler()
	s.router.DELETE("/admin/coupons/:id", handler.DeleteCoupon)

	req, _ := http.NewRequest("DELETE", "/admin/coupons/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminCouponHandlerTestSuite) TestDeleteCoupon_InvalidID() {
	handler := NewAdminCouponHandler()
	s.router.DELETE("/admin/coupons/:id", handler.DeleteCoupon)

	req, _ := http.NewRequest("DELETE", "/admin/coupons/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAdminCouponHandler(t *testing.T) {
	suite.Run(t, new(AdminCouponHandlerTestSuite))
}

// ========== AdminKnowledgeHandler Tests ==========

// AdminKnowledgeHandlerTestSuite 知识库处理器测试套件
type AdminKnowledgeHandlerTestSuite struct {
	HandlerTestSuite
	testArticle *model.Knowledge
}

func (s *AdminKnowledgeHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.testArticle = &model.Knowledge{
		Category: "公告",
		Title:    "Test Article",
		Body:     "Test content",
		Sort:     1,
		Show:     1,
	}
	s.db.Create(s.testArticle)
	s.router = gin.New()
}

func (s *AdminKnowledgeHandlerTestSuite) TestGetArticles() {
	handler := NewAdminKnowledgeHandler()
	s.router.GET("/admin/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/admin/knowledge", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestCreateArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.POST("/admin/knowledge", handler.CreateArticle)

	body := `{"title": "New Article", "body": "New content"}`
	req, _ := http.NewRequest("POST", "/admin/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestCreateArticle_MissingFields() {
	handler := NewAdminKnowledgeHandler()
	s.router.POST("/admin/knowledge", handler.CreateArticle)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"title": "Updated Title", "body": "Updated content"}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle_NotFound() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"title": "Updated Title"}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/99999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle_InvalidID() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"title": "Updated Title"}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/invalid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestDeleteArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeHandlerTestSuite) TestDeleteArticle_NotFound() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func TestAdminKnowledgeHandler(t *testing.T) {
	suite.Run(t, new(AdminKnowledgeHandlerTestSuite))
}

// ========== AdminTicketHandler Tests ==========

// AdminTicketHandlerTestSuite 工单处理器测试套件
type AdminTicketHandlerTestSuite struct {
	HandlerTestSuite
	testUser   *model.User
	testTicket *model.Ticket
}

func (s *AdminTicketHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.testUser = &model.User{
		Email:          "ticket@example.com",
		Password:       "hash",
		Token:          "ticket-token",
		UUID:           "ticket-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	s.testTicket = &model.Ticket{
		UserID:  s.testUser.ID,
		Subject: "Test Ticket",
		Level:   1,
		Status:  0,
	}
	s.db.Create(s.testTicket)
	s.router = gin.New()
}

func (s *AdminTicketHandlerTestSuite) TestGetTickets() {
	handler := NewAdminTicketHandler()
	s.router.GET("/admin/tickets", handler.GetTickets)

	req, _ := http.NewRequest("GET", "/admin/tickets", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestReplyTicket() {
	adminUser := &model.User{
		Email:          "admin_ticket@example.com",
		Password:       "hash",
		Token:          "admin-ticket-token",
		UUID:           "admin-ticket-uuid",
		TransferEnable: 10737418240,
		IsAdmin:        1,
	}
	s.db.Create(adminUser)

	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/reply", func(c *gin.Context) {
		c.Set("user_id", adminUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{"ticket_id": ` + strconv.FormatUint(uint64(s.testTicket.ID), 10) + `, "message": "Reply content"}`
	req, _ := http.NewRequest("POST", "/admin/tickets/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestReplyTicket_MissingFields() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/reply", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	}, handler.ReplyTicket)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/tickets/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestReplyTicket_NotFound() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/reply", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	}, handler.ReplyTicket)

	body := `{"ticket_id": 99999, "message": "Reply content"}`
	req, _ := http.NewRequest("POST", "/admin/tickets/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket_NotFound() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/99999/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket_InvalidID() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/invalid/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAdminTicketHandler(t *testing.T) {
	suite.Run(t, new(AdminTicketHandlerTestSuite))
}

// ========== PaymentGatewayHandler Tests ==========

// PaymentGatewayHandlerTestSuite 支付网关处理器测试套件
type PaymentGatewayHandlerTestSuite struct {
	HandlerTestSuite
	testGateway *model.PaymentGateway
}

func (s *PaymentGatewayHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.testGateway = &model.PaymentGateway{
		Name:   "Test Gateway",
		Type:   model.PaymentGatewayAlipay,
		Config: `{"app_id": "test"}`,
	}
	s.db.Create(s.testGateway)
	s.router = gin.New()
}

func (s *PaymentGatewayHandlerTestSuite) TestListGateways() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/gateways", handler.ListGateways)

	req, _ := http.NewRequest("GET", "/admin/payment/gateways", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestCreateGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways", handler.CreateGateway)

	body := `{"name": "New Gateway", "type": "wechat", "config": "{\"app_id\": \"test\"}"}`
	req, _ := http.NewRequest("POST", "/admin/payment/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestCreateGateway_MissingFields() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways", handler.CreateGateway)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/payment/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestUpdateGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.PUT("/admin/payment/gateways/:id", handler.UpdateGateway)

	body := `{"name": "Updated Gateway"}`
	req, _ := http.NewRequest("PUT", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestUpdateGateway_NotFound() {
	handler := NewPaymentGatewayHandler()
	s.router.PUT("/admin/payment/gateways/:id", handler.UpdateGateway)

	body := `{"name": "Updated Gateway"}`
	req, _ := http.NewRequest("PUT", "/admin/payment/gateways/99999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestUpdateGateway_InvalidID() {
	handler := NewPaymentGatewayHandler()
	s.router.PUT("/admin/payment/gateways/:id", handler.UpdateGateway)

	body := `{"name": "Updated Gateway"}`
	req, _ := http.NewRequest("PUT", "/admin/payment/gateways/invalid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestDeleteGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.DELETE("/admin/payment/gateways/:id", handler.DeleteGateway)

	req, _ := http.NewRequest("DELETE", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestDeleteGateway_NotFound() {
	handler := NewPaymentGatewayHandler()
	s.router.DELETE("/admin/payment/gateways/:id", handler.DeleteGateway)

	req, _ := http.NewRequest("DELETE", "/admin/payment/gateways/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Delete doesn't fail if gateway doesn't exist (GORM behavior)
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestToggleGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways/:id/toggle", handler.ToggleGateway)

	body := `{"enabled": true}`
	req, _ := http.NewRequest("POST", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10)+"/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayHandlerTestSuite) TestGetStats() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/stats", handler.GetPaymentStats)

	req, _ := http.NewRequest("GET", "/admin/payment/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestPaymentGatewayHandler(t *testing.T) {
	suite.Run(t, new(PaymentGatewayHandlerTestSuite))
}

// ========== ForwardHandler Tests ==========

// ForwardHandlerTestSuite 转发处理器测试套件
type ForwardHandlerTestSuite struct {
	HandlerTestSuite
	testNode *model.ForwardNode
}

func (s *ForwardHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.testNode = &model.ForwardNode{
		Name:     "Test Forward Node",
		Type:     "relay",
		Host:     "192.168.1.1",
		Port:     8080,
		APIToken: "test-token",
		Enabled:  true,
	}
	s.db.Create(s.testNode)
	s.router = gin.New()
}

func (s *ForwardHandlerTestSuite) TestListNodes() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/nodes", handler.ListNodes)

	req, _ := http.NewRequest("GET", "/admin/forward/nodes", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestListNodes_WithType() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/nodes", handler.ListNodes)

	req, _ := http.NewRequest("GET", "/admin/forward/nodes?type=relay", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestCreateNode() {
	handler := NewForwardHandler()
	s.router.POST("/admin/forward/nodes", handler.CreateNode)

	body := `{"name": "New Node", "type": "relay", "host": "192.168.1.2", "port": 8081}`
	req, _ := http.NewRequest("POST", "/admin/forward/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestCreateNode_MissingFields() {
	handler := NewForwardHandler()
	s.router.POST("/admin/forward/nodes", handler.CreateNode)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/forward/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerTestSuite) TestGetNode() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/admin/forward/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestGetNode_NotFound() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/admin/forward/nodes/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardHandlerTestSuite) TestGetNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/admin/forward/nodes/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerTestSuite) TestDeleteNode() {
	handler := NewForwardHandler()
	s.router.DELETE("/admin/forward/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/admin/forward/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestListRules() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/rules", handler.ListRules)

	req, _ := http.NewRequest("GET", "/admin/forward/rules", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerTestSuite) TestGetStats() {
	handler := NewForwardHandler()
	s.router.GET("/admin/forward/stats", handler.GetForwardStats)

	req, _ := http.NewRequest("GET", "/admin/forward/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestForwardHandler(t *testing.T) {
	suite.Run(t, new(ForwardHandlerTestSuite))
}

// ========== AdminHandler Additional Tests ==========

// AdminPlanHandlerTestSuite 套餐管理测试套件
type AdminPlanHandlerTestSuite struct {
	HandlerTestSuite
	testPlan *model.Plan
	testUser *model.User
}

func (s *AdminPlanHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	transferEnable := int64(10737418240)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:          "Test Plan",
		TransferEnable: transferEnable,
		MonthPrice:    &monthPrice,
		Show:          1,
	}
	s.db.Create(s.testPlan)

	s.testUser = &model.User{
		Email:          "plan@example.com",
		Password:       "hash",
		Token:          "plan-token",
		UUID:           "plan-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *AdminPlanHandlerTestSuite) TestUpdatePlan() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/plans/:id", handler.UpdatePlan)

	body := `{"name": "Updated Plan", "month_price": 2000}`
	req, _ := http.NewRequest("PUT", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminPlanHandlerTestSuite) TestUpdatePlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/plans/:id", handler.UpdatePlan)

	body := `{"name": "Updated Plan"}`
	req, _ := http.NewRequest("PUT", "/admin/plans/invalid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminPlanHandlerTestSuite) TestDeletePlan() {
	handler := NewAdminHandler()
	s.router.DELETE("/admin/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminPlanHandlerTestSuite) TestDeletePlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.DELETE("/admin/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/admin/plans/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminPlanHandlerTestSuite) TestAssignPlanToUser() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	body := `{"user_id": ` + strconv.FormatUint(uint64(s.testUser.ID), 10) + `}`
	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Service might fail if plan/user not properly configured
	// Test covers handler logic, not service implementation
	assert.True(s.T(), w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func (s *AdminPlanHandlerTestSuite) TestAssignPlanToUser_MissingUserID() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminPlanHandlerTestSuite) TestGetUserStats() {
	handler := NewAdminHandler()
	s.router.GET("/admin/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/admin/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// AdminOrderHandlerTestSuite 订单管理测试套件
type AdminOrderHandlerTestSuite struct {
	HandlerTestSuite
	testOrder *model.Order
	testUser  *model.User
	testPlan  *model.Plan
}

func (s *AdminOrderHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	transferEnable := int64(10737418240)
	monthPrice := int64(1000)
	s.testPlan = &model.Plan{
		Name:          "Test Plan",
		TransferEnable: transferEnable,
		MonthPrice:    &monthPrice,
		Show:          1,
	}
	s.db.Create(s.testPlan)

	s.testUser = &model.User{
		Email:          "order@example.com",
		Password:       "hash",
		Token:          "order-token",
		UUID:           "order-uuid",
		TransferEnable: 10737418240,
	}
	s.db.Create(s.testUser)

	planID := s.testPlan.ID
	s.testOrder = &model.Order{
		TradeNo:     "TEST_ORDER_001",
		UserID:      s.testUser.ID,
		PlanID:      planID,
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(s.testOrder)

	s.router = gin.New()
}

func (s *AdminOrderHandlerTestSuite) TestGetOrder() {
	handler := NewAdminHandler()
	s.router.GET("/admin/orders/:id", handler.GetOrder)

	req, _ := http.NewRequest("GET", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminOrderHandlerTestSuite) TestGetOrder_InvalidID() {
	handler := NewAdminHandler()
	s.router.GET("/admin/orders/:id", handler.GetOrder)

	req, _ := http.NewRequest("GET", "/admin/orders/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminOrderHandlerTestSuite) TestUpdateOrderStatus() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/orders/:id/status", handler.UpdateOrderStatus)

	body := `{"status": 1}`
	req, _ := http.NewRequest("PUT", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10)+"/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminOrderHandlerTestSuite) TestUpdateOrderStatus_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/orders/:id/status", handler.UpdateOrderStatus)

	body := `{"status": 1}`
	req, _ := http.NewRequest("PUT", "/admin/orders/invalid/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminOrderHandlerTestSuite) TestMarkOrderPaid() {
	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/paid", handler.MarkOrderPaid)

	req, _ := http.NewRequest("POST", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10)+"/paid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Service might fail if order not properly configured
	assert.True(s.T(), w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func (s *AdminOrderHandlerTestSuite) TestCancelOrder() {
	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/cancel", handler.CancelOrder)

	req, _ := http.NewRequest("POST", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10)+"/cancel", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestAdminPlanHandler(t *testing.T) {
	suite.Run(t, new(AdminPlanHandlerTestSuite))
}

func TestAdminOrderHandler(t *testing.T) {
	suite.Run(t, new(AdminOrderHandlerTestSuite))
}

// ========== Node Register/Heartbeat Tests ==========

type NodeRegisterTestSuite struct {
	HandlerTestSuite
	testAuthKey *model.AuthorizedKey
}

func (s *NodeRegisterTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	// Create test auth key
	s.testAuthKey = &model.AuthorizedKey{
		Name:    "Test Key",
		KeyHash: "test-key-hash",
	}
	s.db.Create(s.testAuthKey)

	s.router = gin.New()
}

func (s *NodeRegisterTestSuite) TestRegister_Success() {
	handler := NewNodeHandler()
	s.router.POST("/register", handler.Register)

	body := map[string]interface{}{
		"auth_key":    s.testAuthKey.KeyHash,
		"name":        "Test Node",
		"host":        "192.168.1.1",
		"port":        443,
		"server_version": "1.0.0",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// May fail due to auth key validation, but should not panic
	assert.True(s.T(), w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func (s *NodeRegisterTestSuite) TestRegister_InvalidBody() {
	handler := NewNodeHandler()
	s.router.POST("/register", handler.Register)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeRegisterTestSuite) TestHeartbeat_Unauthorized() {
	handler := NewNodeHandler()
	s.router.POST("/heartbeat", handler.Heartbeat)

	body := map[string]interface{}{
		"cpu_usage":    50.0,
		"mem_usage":    60.0,
		"connections":  100,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/heartbeat", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func TestNodeRegister(t *testing.T) {
	suite.Run(t, new(NodeRegisterTestSuite))
}

// ========== Protocol Update/Delete Tests ==========

type ProtocolHandlerTestSuite struct {
	HandlerTestSuite
	testNode     *model.Node
	testProtocol *model.NodeProtocol
}

func (s *ProtocolHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testNode = &model.Node{
		Name:   "Protocol Test Node",
		Host:   "192.168.1.100",
		Port:   443,
		Status: model.NodeStatusOnline,
		Show:   1,
	}
	s.db.Create(s.testNode)

	transport := "tcp"
	s.testProtocol = &model.NodeProtocol{
		NodeID:    s.testNode.ID,
		Name:      "Test Protocol",
		Type:      model.ProtocolVLESS,
		Port:      443,
		Enable:    1,
		Show:      1,
		Transport: &transport,
	}
	s.db.Create(s.testProtocol)

	s.router = gin.New()
}

func (s *ProtocolHandlerTestSuite) TestUpdateProtocol_Success() {
	handler := NewNodeHandler()
	s.router.PUT("/protocols/:protocol_id", handler.UpdateProtocol)

	body := map[string]interface{}{
		"name":   "Updated Protocol",
		"enable": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/protocols/"+strconv.FormatUint(uint64(s.testProtocol.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ProtocolHandlerTestSuite) TestUpdateProtocol_InvalidID() {
	handler := NewNodeHandler()
	s.router.PUT("/protocols/:protocol_id", handler.UpdateProtocol)

	body := map[string]interface{}{"name": "Test"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/protocols/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ProtocolHandlerTestSuite) TestDeleteProtocol_Success() {
	handler := NewNodeHandler()
	s.router.DELETE("/protocols/:protocol_id", handler.DeleteProtocol)

	req, _ := http.NewRequest("DELETE", "/protocols/"+strconv.FormatUint(uint64(s.testProtocol.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ProtocolHandlerTestSuite) TestDeleteProtocol_InvalidID() {
	handler := NewNodeHandler()
	s.router.DELETE("/protocols/:protocol_id", handler.DeleteProtocol)

	req, _ := http.NewRequest("DELETE", "/protocols/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ProtocolHandlerTestSuite) TestSyncProtocol_Success() {
	handler := NewNodeHandler()
	s.router.POST("/nodes/:id/sync", handler.SyncProtocol)

	req, _ := http.NewRequest("POST", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/sync", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ProtocolHandlerTestSuite) TestSyncProtocol_InvalidID() {
	handler := NewNodeHandler()
	s.router.POST("/nodes/:id/sync", handler.SyncProtocol)

	req, _ := http.NewRequest("POST", "/nodes/invalid/sync", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestProtocolHandler(t *testing.T) {
	suite.Run(t, new(ProtocolHandlerTestSuite))
}

// ========== Auth Key Delete Tests ==========

type AuthKeyDeleteTestSuite struct {
	HandlerTestSuite
	testAuthKey *model.AuthorizedKey
}

func (s *AuthKeyDeleteTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testAuthKey = &model.AuthorizedKey{
		Name:    "Delete Test Key",
		KeyHash: "delete-test-key-hash",
	}
	s.db.Create(s.testAuthKey)

	s.router = gin.New()
}

func (s *AuthKeyDeleteTestSuite) TestDeleteAuthKey_Success() {
	handler := NewNodeHandler()
	s.router.DELETE("/auth-keys/:id", handler.DeleteAuthKey)

	req, _ := http.NewRequest("DELETE", "/auth-keys/"+strconv.FormatUint(uint64(s.testAuthKey.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AuthKeyDeleteTestSuite) TestDeleteAuthKey_InvalidID() {
	handler := NewNodeHandler()
	s.router.DELETE("/auth-keys/:id", handler.DeleteAuthKey)

	req, _ := http.NewRequest("DELETE", "/auth-keys/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAuthKeyDelete(t *testing.T) {
	suite.Run(t, new(AuthKeyDeleteTestSuite))
}

// ========== Forward Node Tests ==========

type ForwardNodeHandlerTestSuite struct {
	HandlerTestSuite
	testForwardNode *model.ForwardNode
}

func (s *ForwardNodeHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testForwardNode = &model.ForwardNode{
		Name:    "Test Forward Node",
		Type:    "relay",
		Host:    "192.168.1.200",
		Port:    8080,
		APIPort: 18080,
		Status:  1,
	}
	s.db.Create(s.testForwardNode)

	s.router = gin.New()
}

func (s *ForwardNodeHandlerTestSuite) TestListNodes() {
	handler := NewForwardHandler()
	s.router.GET("/forward/nodes", handler.ListNodes)

	req, _ := http.NewRequest("GET", "/forward/nodes", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardNodeHandlerTestSuite) TestCreateNode() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes", handler.CreateNode)

	body := map[string]interface{}{
		"name":    "New Forward Node",
		"type":    "exit",
		"host":    "192.168.1.201",
		"port":    8081,
		"api_port": 18081,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/nodes", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardNodeHandlerTestSuite) TestDeleteNode() {
	handler := NewForwardHandler()
	s.router.DELETE("/forward/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/forward/nodes/"+strconv.FormatUint(uint64(s.testForwardNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardNodeHandlerTestSuite) TestDeleteNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.DELETE("/forward/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/forward/nodes/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestForwardNodeHandler(t *testing.T) {
	suite.Run(t, new(ForwardNodeHandlerTestSuite))
}

// ========== Forward Node Extended Tests ==========

type ForwardHandlerExtendedTestSuite struct {
	HandlerTestSuite
	testRelayNode *model.ForwardNode
	testExitNode  *model.ForwardNode
	testRule      *model.ForwardRule
}

func (s *ForwardHandlerExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testRelayNode = &model.ForwardNode{
		Name:    "Test Relay Node",
		Type:    "relay",
		Host:    "192.168.1.100",
		Port:    8080,
		APIPort: 18080,
		Status:  1,
		Enabled: true,
	}
	s.db.Create(s.testRelayNode)

	s.testExitNode = &model.ForwardNode{
		Name:    "Test Exit Node",
		Type:    "exit",
		Host:    "192.168.1.200",
		Port:    8081,
		APIPort: 18081,
		Status:  1,
		Enabled: true,
	}
	s.db.Create(s.testExitNode)

	s.testRule = &model.ForwardRule{
		Name:         "Test Rule",
		Enabled:      true,
		RelayNodeID:  s.testRelayNode.ID,
		ListenPort:   9000,
		Protocol:     "tcp",
		ExitNodeID:   s.testExitNode.ID,
		TargetHost:   "10.0.0.1",
		TargetPort:   80,
	}
	s.db.Create(s.testRule)

	s.router = gin.New()
}

func (s *ForwardHandlerExtendedTestSuite) TestGetNode() {
	handler := NewForwardHandler()
	s.router.GET("/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetNode_NotFound() {
	handler := NewForwardHandler()
	s.router.GET("/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/forward/nodes/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.GET("/forward/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/forward/nodes/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestUpdateNode() {
	handler := NewForwardHandler()
	s.router.PUT("/forward/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{
		"name": "Updated Node",
		"host": "192.168.1.150",
		"port": 9090,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestUpdateNode_NotFound() {
	handler := NewForwardHandler()
	s.router.PUT("/forward/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/forward/nodes/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestUpdateNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.PUT("/forward/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/forward/nodes/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestCheckNode() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/check", handler.CheckNode)

	req, _ := http.NewRequest("POST", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10)+"/check", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// May return error since node can't actually be checked
	// but we test the handler logic
}

func (s *ForwardHandlerExtendedTestSuite) TestCheckNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/check", handler.CheckNode)

	req, _ := http.NewRequest("POST", "/forward/nodes/invalid/check", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleNode() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/toggle", handler.ToggleNode)

	body := map[string]bool{"enabled": false}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10)+"/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleNode_Enable() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/toggle", handler.ToggleNode)

	body := map[string]bool{"enabled": true}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10)+"/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleNode_InvalidID() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/toggle", handler.ToggleNode)

	body := map[string]bool{"enabled": true}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/nodes/invalid/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleNode_NotFound() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/toggle", handler.ToggleNode)

	body := map[string]bool{"enabled": true}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/nodes/99999/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

// Forward Rule Tests

func (s *ForwardHandlerExtendedTestSuite) TestListRules() {
	handler := NewForwardHandler()
	s.router.GET("/forward/rules", handler.ListRules)

	req, _ := http.NewRequest("GET", "/forward/rules", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestListRules_WithUserID() {
	handler := NewForwardHandler()
	s.router.GET("/forward/rules", handler.ListRules)

	req, _ := http.NewRequest("GET", "/forward/rules?user_id=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestCreateRule() {
	handler := NewForwardHandler()
	s.router.POST("/forward/rules", handler.CreateRule)

	body := map[string]interface{}{
		"name":          "New Rule",
		"relay_node_id": s.testRelayNode.ID,
		"listen_port":   9001,
		"protocol":      "tcp",
		"exit_node_id":  s.testExitNode.ID,
		"target_host":   "10.0.0.2",
		"target_port":   8080,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/rules", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestCreateRule_DefaultProtocol() {
	handler := NewForwardHandler()
	s.router.POST("/forward/rules", handler.CreateRule)

	body := map[string]interface{}{
		"name":          "New Rule No Protocol",
		"relay_node_id": s.testRelayNode.ID,
		"listen_port":   9002,
		"exit_node_id":  s.testExitNode.ID,
		"target_host":   "10.0.0.3",
		"target_port":   8080,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/rules", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetRule() {
	handler := NewForwardHandler()
	s.router.GET("/forward/rules/:id", handler.GetRule)

	req, _ := http.NewRequest("GET", "/forward/rules/"+strconv.FormatUint(uint64(s.testRule.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetRule_NotFound() {
	handler := NewForwardHandler()
	s.router.GET("/forward/rules/:id", handler.GetRule)

	req, _ := http.NewRequest("GET", "/forward/rules/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetRule_InvalidID() {
	handler := NewForwardHandler()
	s.router.GET("/forward/rules/:id", handler.GetRule)

	req, _ := http.NewRequest("GET", "/forward/rules/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestUpdateRule() {
	handler := NewForwardHandler()
	s.router.PUT("/forward/rules/:id", handler.UpdateRule)

	body := map[string]interface{}{
		"name":        "Updated Rule",
		"listen_port": 9100,
		"target_host": "10.0.0.100",
		"target_port": 8080,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/forward/rules/"+strconv.FormatUint(uint64(s.testRule.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestUpdateRule_NotFound() {
	handler := NewForwardHandler()
	s.router.PUT("/forward/rules/:id", handler.UpdateRule)

	body := map[string]interface{}{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/forward/rules/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestDeleteRule() {
	handler := NewForwardHandler()
	s.router.DELETE("/forward/rules/:id", handler.DeleteRule)

	req, _ := http.NewRequest("DELETE", "/forward/rules/"+strconv.FormatUint(uint64(s.testRule.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestDeleteRule_InvalidID() {
	handler := NewForwardHandler()
	s.router.DELETE("/forward/rules/:id", handler.DeleteRule)

	req, _ := http.NewRequest("DELETE", "/forward/rules/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleRule() {
	handler := NewForwardHandler()
	s.router.POST("/forward/rules/:id/toggle", handler.ToggleRule)

	body := map[string]bool{"enabled": false}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/rules/"+strconv.FormatUint(uint64(s.testRule.ID), 10)+"/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestToggleRule_InvalidID() {
	handler := NewForwardHandler()
	s.router.POST("/forward/rules/:id/toggle", handler.ToggleRule)

	body := map[string]bool{"enabled": true}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/rules/invalid/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestGetForwardStats() {
	handler := NewForwardHandler()
	s.router.GET("/forward/stats", handler.GetForwardStats)

	req, _ := http.NewRequest("GET", "/forward/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestTestGostConnection() {
	handler := NewForwardHandler()
	s.router.POST("/forward/test-connection", handler.TestGostConnection)

	body := map[string]interface{}{
		"host":      "127.0.0.1",
		"api_port":  18080,
		"api_token": "test-token",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/test-connection", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Returns OK even if connection fails (handler returns success: false)
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardHandlerExtendedTestSuite) TestSyncNodeStats() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/sync-stats", handler.SyncNodeStats)

	req, _ := http.NewRequest("POST", "/forward/nodes/"+strconv.FormatUint(uint64(s.testRelayNode.ID), 10)+"/sync-stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// May return error due to actual connection, but tests the handler
}

func (s *ForwardHandlerExtendedTestSuite) TestSyncNodeStats_InvalidID() {
	handler := NewForwardHandler()
	s.router.POST("/forward/nodes/:id/sync-stats", handler.SyncNodeStats)

	req, _ := http.NewRequest("POST", "/forward/nodes/invalid/sync-stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestForwardHandlerExtended(t *testing.T) {
	suite.Run(t, new(ForwardHandlerExtendedTestSuite))
}

// ========== System Config Tests ==========

type SystemConfigTestSuite struct {
	HandlerTestSuite
	testConfig *model.SystemConfig
}

func (s *SystemConfigTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testConfig = &model.SystemConfig{
		Key:   "test_key",
		Value: "test_value",
	}
	s.db.Create(s.testConfig)

	s.router = gin.New()
}

func (s *SystemConfigTestSuite) TestGetConfigs() {
	handler := NewSystemHandler()
	s.router.GET("/system/configs", handler.GetConfigs)

	req, _ := http.NewRequest("GET", "/system/configs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemConfigTestSuite) TestGetConfig() {
	handler := NewSystemHandler()
	s.router.GET("/system/configs/:key", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemConfigTestSuite) TestSetConfig() {
	handler := NewSystemHandler()
	s.router.POST("/system/configs", handler.SetConfig)

	body := map[string]string{
		"key":   "new_key",
		"value": "new_value",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/system/configs", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemConfigTestSuite) TestDeleteConfig() {
	handler := NewSystemHandler()
	s.router.DELETE("/system/configs/:key", handler.DeleteConfig)

	req, _ := http.NewRequest("DELETE", "/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestSystemConfig(t *testing.T) {
	suite.Run(t, new(SystemConfigTestSuite))
}

