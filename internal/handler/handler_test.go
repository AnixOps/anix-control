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

	"github.com/AnixOps/anix-control/v5/internal/cache"
	compatv2 "github.com/AnixOps/anix-control/v5/internal/compat/v2"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	if err := testDB.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.NodeLog{},
		&model.Plan{},
		&model.Order{},
		&model.Coupon{},
		&model.CouponUsage{},
		&model.Ticket{},
		&model.TicketMessage{},
		&model.Knowledge{},
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
		&model.ForwardLatencyBucket{},
		&model.OperationLog{},
		&model.UserSubscriptionGroup{},
		&model.SubscriberRequest{},
		&model.SubscriberChange{},
		&model.PlanSubscriptionGroup{},
		&model.Event{},
		&model.SubscriptionGroup{},
		&model.SubscriptionTemplate{},
		&model.ServerVMess{},
		&model.ServerVLESS{},
		&model.ServerTrojan{},
		&model.ServerShadowsocks{},
		&model.ServerTUIC{},
		&model.ServerAnyTLS{},
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatServer{},
		&model.StatUser{},
	); err != nil {
		panic("failed to migrate handler test database: " + err.Error())
	}
	if err := service.MigrateKernelModelsForTest(testDB); err != nil {
		panic("failed to migrate kernel handler test database: " + err.Error())
	}

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

	testutil.CleanupDB(s.db)
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

func (s *NodeHandlerTestSuite) TestCreateNode_Success() {
	handler := NewNodeHandler()
	s.router.POST("/nodes", handler.CreateNode)

	body := map[string]any{
		"name": "New Test Node",
		"host": "192.168.1.200",
		"port": 443,
		"rate": 1.0,
		"show": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/nodes", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.NotZero(s.T(), data["node_id"])
	assert.NotContains(s.T(), data, "api_key")
	assert.NotContains(s.T(), resp, "error")
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

	body := map[string]any{
		"name": "Updated Node Name",
		"rate": 2.0,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "更新成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestUpdateNode_InvalidID() {
	handler := NewNodeHandler()
	s.router.PUT("/nodes/:id", handler.UpdateNode)

	body := map[string]any{"name": "Test"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/nodes/invalid", bytes.NewReader(jsonBody))
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

func (s *AdminHandlerTestSuite) TestGetSystemInfo_UsesPanelEnvelope() {
	handler := NewAdminHandler()
	s.router.GET("/system/info", handler.GetSystemInfo)

	req, _ := http.NewRequest("GET", "/system/info", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), BuildVersion, data["version"])
	assert.Contains(s.T(), data, "build_code")
	assert.Contains(s.T(), data, "build_time")
	assert.Contains(s.T(), data, "commit")
	assert.NotContains(s.T(), resp, "error")
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

func (s *MetricsHandlerTestSuite) TestGetMetrics_IncludesV2GatewaySeries() {
	compatv2.DefaultGatewayMetrics().Observe("knowledge", "knowledge.article.list", http.StatusOK, "", 3*time.Millisecond)
	handler := NewMetricsHandler()
	s.router.GET("/metrics", handler.GetMetrics)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(s.T(), body, "# TYPE anixops_v2_gateway_requests_total counter")
	assert.Contains(s.T(), body, `anixops_v2_gateway_requests_total{package="knowledge",route="knowledge.article.list",code_class="2xx"}`)
	assert.Contains(s.T(), body, "# TYPE anixops_v2_gateway_errors_total counter")
	assert.Contains(s.T(), body, `anixops_v2_gateway_request_duration_seconds_bucket{package="knowledge",le="0.005"}`)
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
	assert.Equal(s.T(), "-42", formatInt(-42))
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
		{"Stash/1.0", "stash"},
		{"Egern/1.0", "egern"},
		{"Surge/1.0", "surge"},
		{"Loon/1.0", "loon"},
		{"Loon/997 CFNetwork/1496.0.7 Darwin/23.5.0", "loon"},
		{"Shadowrocket/1.0", "shadowrocket"},
		{"Quantumult X/1.0", "quantumultx"},
		{"V2RayNG/1.0", "v2ray"},
		{"v2rayN/1.0", "v2ray"},
		{"sing-box/1.0", "sing-box"},
		{"NekoBox/1.0", "sing-box"},
		{"AnixOpsNet/1.0", "anixops"},
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

func (s *SubscribeHandlerTestSuite) TestGetSubscription_WithAutoFormat() {
	handler := NewSubscribeHandler(s.cfg)
	s.router.GET("/s/:token", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/s/"+s.testUser.Token+"?type=auto", nil)
	req.Header.Set("User-Agent", "Clash/1.0")
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

func (s *SubscriptionAdminHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
	if msgContains != "" {
		msg, ok := resp["msg"].(string)
		assert.True(s.T(), ok)
		assert.Contains(s.T(), strings.ToLower(msg), strings.ToLower(msgContains))
	}
}

func (s *SubscriptionAdminHandlerTestSuite) TestPreviewSubscription_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/preview", handler.PreviewSubscription)

	req, _ := http.NewRequest("POST", "/preview", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscriptionAdminHandlerTestSuite) TestPreviewSubscription_UserNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/preview", handler.PreviewSubscription)

	body := map[string]any{
		"user_id": 99999,
		"format":  "v2ray",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
}

func TestSubscriptionAdminHandler(t *testing.T) {
	suite.Run(t, new(SubscriptionAdminHandlerTestSuite))
}

// ========== NotificationHandler Tests ==========

// ========== TelegramHandler Tests ==========

// TelegramHandlerTestSuite Telegram处理器测试套件
type TelegramHandlerTestSuite struct {
	HandlerTestSuite
}

func (s *TelegramHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *TelegramHandlerTestSuite) TestTelegramWebhook_RejectsMissingSecret() {
	handler := NewTelegramHandler()
	s.router.POST("/telegram/webhook", handler.TelegramWebhook)
	require.NoError(s.T(), s.db.Create(&model.TelegramBot{Name: "Bot", Token: "123:abc", Enabled: true}).Error)

	// A forged update: the sender claims to be user 42 and asks for /sub.
	body := `{"update_id":1,"message":{"message_id":1,"from":{"id":42},"chat":{"id":42},"text":"/sub"}}`
	req, _ := http.NewRequest("POST", "/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusForbidden, w.Code)
	var chats int64
	s.db.Model(&model.TelegramChat{}).Count(&chats)
	assert.Zero(s.T(), chats, "a rejected update must not be stored")
}

func (s *TelegramHandlerTestSuite) TestTelegramWebhook_MissingBody() {
	handler := NewTelegramHandler()
	s.router.POST("/telegram/webhook", handler.TelegramWebhook)
	require.NoError(s.T(), s.db.Create(&model.TelegramBot{Name: "Bot", Token: "123:abc", Enabled: true}).Error)

	body := `{}`
	req, _ := http.NewRequest("POST", "/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", service.TelegramWebhookSecret("123:abc"))
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "list")
	assert.Contains(s.T(), data, "total")
	assert.NotContains(s.T(), resp, "list")
	assert.NotContains(s.T(), resp, "error")
}

func (s *SystemHandlerTestSuite) TestGetConfigs_ByGroup() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs", handler.GetConfigs)

	req, _ := http.NewRequest("GET", "/admin/system/configs?group=email", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "list")
	assert.Contains(s.T(), data, "total")
	assert.NotContains(s.T(), resp, "list")
}

func (s *SystemHandlerTestSuite) TestGetConfigs_HidesInternalKeys() {
	configs := service.NewSystemConfigService(database.GetDB())
	require.NoError(s.T(), configs.Set("scheduler.forward_flow_reset.last_day", "2026-10-31", "string", "", ""))
	require.NoError(s.T(), configs.Set("app.subscribe_domains", `["a.example.com"]`, "json", "app", ""))
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs", handler.GetConfigs)

	req, _ := http.NewRequest("GET", "/admin/system/configs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	data := decodePanelTestResponse(s.T(), w)["data"].(map[string]any)
	var keys []string
	for _, item := range data["list"].([]any) {
		keys = append(keys, item.(map[string]any)["key"].(string))
	}
	assert.Contains(s.T(), keys, "app.subscribe_domains")
	assert.NotContains(s.T(), keys, "scheduler.forward_flow_reset.last_day")
}

func (s *SystemHandlerTestSuite) TestSettingsCatalog() {
	handler := NewSettingsCatalogHandler()
	s.router.GET("/api/v5/settings/catalog", handler.List)

	req, _ := http.NewRequest("GET", "/api/v5/settings/catalog", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &body))
	byKey := map[string]map[string]any{}
	for _, item := range body.Data.Items {
		byKey[item["key"].(string)] = item
	}
	assert.Equal(s.T(), float64(30), byKey["traffic.destinations.retention_days"]["value"])
	assert.Equal(s.T(), false, byKey["traffic.destinations.enabled"]["value"])
	assert.Equal(s.T(), "/admin/notifications/email", byKey["notification.email.config"]["link"])
}

func (s *SystemHandlerTestSuite) TestGetSubscriptionSettings_UsesPanelEnvelope() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/subscription-settings", handler.GetSubscriptionSettings)

	req, _ := http.NewRequest("GET", "/admin/system/subscription-settings", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "subscribe_path")
	assert.Contains(s.T(), data, "subscribe_domains")
}

func (s *SystemHandlerTestSuite) TestGetConfig() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/configs/:key", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/admin/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "test_key", data["key"])
	assert.Equal(s.T(), "", data["value"])
	assert.Contains(s.T(), data, "display_value")
	assert.Contains(s.T(), data, "sensitive")
	assert.Contains(s.T(), data, "has_value")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "test_key", data["key"])
	assert.Equal(s.T(), "test_value", data["value"])
	assert.Equal(s.T(), "string", data["type"])
	assert.Equal(s.T(), "test", data["group"])
	assert.Equal(s.T(), "config updated", data["message"])
}

func (s *SystemHandlerTestSuite) TestSetConfig_MissingValue() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/configs/:key", handler.SetConfig)

	body := `{}`
	req, _ := http.NewRequest("PUT", "/admin/system/configs/test_key", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SystemHandlerTestSuite) TestSetConfig_InvalidBody() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/configs/:key", handler.SetConfig)

	req, _ := http.NewRequest("PUT", "/admin/system/configs/test_key", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SystemHandlerTestSuite) TestDeleteConfig() {
	handler := NewSystemHandler()
	s.router.DELETE("/admin/system/configs/:key", handler.DeleteConfig)

	req, _ := http.NewRequest("DELETE", "/admin/system/configs/test_key", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "config deleted", data["message"])
}

func TestSystemHandler(t *testing.T) {
	suite.Run(t, new(SystemHandlerTestSuite))
}

// ========== AdminCouponHandler Tests ==========

// ========== AdminKnowledgeHandler Tests ==========

// ========== AdminTicketHandler Tests ==========

// ========== PaymentGatewayHandler Tests ==========

// ========== ForwardHandler Tests ==========

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
		Name:           "Test Plan",
		TransferEnable: transferEnable,
		MonthPrice:     &monthPrice,
		Show:           1,
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

func TestAdminPlanHandler(t *testing.T) {
	suite.Run(t, new(AdminPlanHandlerTestSuite))
}

// ========== Protocol Update/Delete Tests ==========

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

func TestForwardNodeHandler(t *testing.T) {
	suite.Run(t, new(ForwardNodeHandlerTestSuite))
}

// ========== Forward Node Extended Tests ==========

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
