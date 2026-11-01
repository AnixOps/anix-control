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
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
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
		&model.ServerHysteria{},
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

func assertPanelTestError(t *testing.T, w *httptest.ResponseRecorder, msgContains string) {
	t.Helper()
	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodePanelTestResponse(t, w)
	assert.Equal(t, float64(-1), resp["code"])
	assert.NotEmpty(t, resp["msg"])
	assert.NotZero(t, resp["ts"])
	assert.Nil(t, resp["data"])
	assert.NotContains(t, resp, "message")
	assert.NotContains(t, resp, "error")
	if msgContains != "" {
		msg, ok := resp["msg"].(string)
		assert.True(t, ok)
		assert.Contains(t, strings.ToLower(msg), strings.ToLower(msgContains))
	}
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
	service.ResetLoginRateLimiterForTest()
	s.router = gin.New()
}

func (s *AuthHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AuthHandlerTestSuite) createLoginUser(email, password string, isAdmin int) *model.User {
	s.T().Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	s.Require().NoError(err)
	user := &model.User{
		Email:          email,
		Password:       string(hashedPassword),
		Token:          "token-" + strings.ReplaceAll(email, "@", "-"),
		UUID:           "uuid-" + strings.ReplaceAll(email, "@", "-"),
		TransferEnable: 1073741824,
		IsAdmin:        isAdmin,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *AuthHandlerTestSuite) createRestrictedPluginGrant(pluginID string) {
	s.T().Helper()
	group := model.AccessGroup{ScopeID: pluginID, Name: pluginID + "-restricted", Enabled: true}
	s.Require().NoError(s.db.Create(&group).Error)
	s.Require().NoError(s.db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: pluginID,
		Permissions: `["` + pluginID + `.view"]`,
	}).Error)
}

func (s *AuthHandlerTestSuite) setMFAAdminConfig(cfg map[string]any) {
	s.T().Helper()

	err := service.NewSystemConfigService(s.db).SetJSON(
		mfaAdminConfigKey,
		cfg,
		"security",
		"mfa admin config",
	)
	s.Require().NoError(err)
}

func (s *AuthHandlerTestSuite) createMFAEnabledLoginUser(email, password string) (*model.User, string) {
	s.T().Helper()

	user := s.createLoginUser(email, password, 0)

	mfaService := service.NewMFAService(s.db, nil)
	setup, err := mfaService.SetupTOTP(user.ID, user.Email)
	s.Require().NoError(err)
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	s.Require().NoError(err)
	s.Require().NoError(mfaService.EnableTOTP(user.ID, code))

	return user, setup.Secret
}

func (s *AuthHandlerTestSuite) TestRegisterHandler() {
	s.createRestrictedPluginGrant("register-security")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["token"])
	assert.Equal(s.T(), "handler@example.com", data["email"])
	assert.Equal(s.T(), false, data["is_admin"])
	assert.Equal(s.T(), float64(1), data["user_id"])
	assert.Equal(s.T(), service.PluginPermissionModeAuthoritative, data["permission_mode"])
	assert.Empty(s.T(), data["permissions"])
	assert.Empty(s.T(), data["restricted_plugins"])
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_Disabled() {
	disabled := false
	cfg := *s.cfg
	cfg.Auth.Registration.Enabled = &disabled
	handler := NewAuthHandler(&cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "closed-handler@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "registration is disabled")
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_InvalidBody() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/register", handler.Register)

	req, _ := http.NewRequest("POST", "/register", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_RequiresInviteCode() {
	cfg := *s.cfg
	cfg.Auth.Registration.RequireInvite = true
	handler := NewAuthHandler(&cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "invite-required@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invite code is required")
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_WithInviteCode() {
	cfg := *s.cfg
	cfg.Auth.Registration.RequireInvite = true
	handler := NewAuthHandler(&cfg)
	s.router.POST("/register", handler.Register)

	invite := &model.InviteCode{
		Code:   "HANDLERINV",
		Status: 0,
	}
	assert.NoError(s.T(), s.db.Create(invite).Error)

	body := map[string]string{
		"email":       "handler-invited@example.com",
		"password":    "password123",
		"invite_code": "HANDLERINV",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var used model.InviteCode
	assert.NoError(s.T(), s.db.First(&used, invite.ID).Error)
	assert.Equal(s.T(), 1, used.Status)
	assert.NotNil(s.T(), used.UsedBy)
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

	s.assertPanelError(w, "参数错误")
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

	s.assertPanelError(w, "参数错误")
}

func (s *AuthHandlerTestSuite) TestRegisterHandler_DuplicateEmail() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/register", handler.Register)

	body := map[string]string{
		"email":    "duplicate@example.com",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	req, _ = http.NewRequest("POST", "/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "该邮箱已被注册")
}

func (s *AuthHandlerTestSuite) TestLoginHandler() {
	s.createRestrictedPluginGrant("login-security")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["token"])
	assert.Equal(s.T(), "login@example.com", data["email"])
	assert.Equal(s.T(), false, data["is_admin"])
	assert.Equal(s.T(), float64(1), data["user_id"])
	assert.Equal(s.T(), service.PluginPermissionModeAuthoritative, data["permission_mode"])
	assert.Empty(s.T(), data["permissions"])
	assert.Empty(s.T(), data["restricted_plugins"])
}

func (s *AuthHandlerTestSuite) TestLoginHandler_InvalidBody() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)

	req, _ := http.NewRequest("POST", "/login", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
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

	s.assertPanelError(w, "用户不存在或密码错误")
}

func (s *AuthHandlerTestSuite) TestLoginHandler_MFARequiresCodeBeforeToken() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	user, _ := s.createMFAEnabledLoginUser("mfa-required@example.com", "password123")

	loginBody := map[string]string{
		"email":    "mfa-required@example.com",
		"password": "password123",
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), true, data["mfa_required"])
	assert.Empty(s.T(), data["token"])
	assert.Equal(s.T(), "mfa-required@example.com", data["email"])
	assert.Equal(s.T(), float64(user.ID), data["user_id"])
	assert.ElementsMatch(s.T(), []any{model.MFAMethodTOTP, model.MFAMethodBackup}, data["methods"].([]any))
	assert.ElementsMatch(s.T(), data["methods"], data["mfa_methods"])

	var attempts int64
	s.Require().NoError(s.db.Model(&model.MFALoginAttempt{}).Where("user_id = ?", user.ID).Count(&attempts).Error)
	assert.Equal(s.T(), int64(0), attempts)
}

func (s *AuthHandlerTestSuite) TestLoginHandler_GlobalMFAEnforceForAllRequiresEnrollmentBeforeToken() {
	s.setMFAAdminConfig(map[string]any{
		"enabled":  true,
		"required": true,
		"methods":  map[string]bool{"totp": true, "sms": false, "email": false},
	})

	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	user := s.createLoginUser("mfa-enroll-all@example.com", "password123", 0)

	loginBody := map[string]string{
		"email":    "mfa-enroll-all@example.com",
		"password": "password123",
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), true, data["mfa_enrollment_required"])
	assert.Equal(s.T(), true, data["mfa_setup_required"])
	assert.Empty(s.T(), data["token"])
	assert.Empty(s.T(), data["mfa_required"])
	assert.Equal(s.T(), "mfa-enroll-all@example.com", data["email"])
	assert.Equal(s.T(), float64(user.ID), data["user_id"])
	assert.ElementsMatch(s.T(), []any{model.MFAMethodTOTP}, data["methods"].([]any))
	assert.ElementsMatch(s.T(), data["methods"], data["mfa_methods"])

	var attempts int64
	s.Require().NoError(s.db.Model(&model.MFALoginAttempt{}).Where("user_id = ?", user.ID).Count(&attempts).Error)
	assert.Equal(s.T(), int64(0), attempts)
}

func (s *AuthHandlerTestSuite) TestLoginHandler_GlobalMFAEnforceForAdminRequiresAdminEnrollmentBeforeToken() {
	s.setMFAAdminConfig(map[string]any{
		"enabled":           true,
		"required":          false,
		"enforce_for_admin": true,
	})

	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	admin := s.createLoginUser("mfa-enroll-admin@example.com", "password123", 1)

	loginBody := map[string]string{
		"email":    "mfa-enroll-admin@example.com",
		"password": "password123",
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), true, data["mfa_enrollment_required"])
	assert.Empty(s.T(), data["token"])
	assert.Equal(s.T(), "mfa-enroll-admin@example.com", data["email"])
	assert.Equal(s.T(), float64(admin.ID), data["user_id"])
}

func (s *AuthHandlerTestSuite) TestLoginHandler_GlobalMFAEnforceForAdminAllowsRegularUserWithoutMFA() {
	s.setMFAAdminConfig(map[string]any{
		"enabled":           true,
		"required":          false,
		"enforce_for_admin": true,
	})

	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	user := s.createLoginUser("mfa-admin-policy-user@example.com", "password123", 0)

	loginBody := map[string]string{
		"email":    "mfa-admin-policy-user@example.com",
		"password": "password123",
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["token"])
	assert.Empty(s.T(), data["mfa_enrollment_required"])
	assert.Equal(s.T(), "mfa-admin-policy-user@example.com", data["email"])
	assert.Equal(s.T(), float64(user.ID), data["user_id"])
}

func (s *AuthHandlerTestSuite) TestLoginHandler_MFASuccessIssuesTokenAndRecordsAttempt() {
	s.createRestrictedPluginGrant("mfa-security")
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	user, secret := s.createMFAEnabledLoginUser("mfa-success@example.com", "password123")
	code, err := totp.GenerateCode(secret, time.Now())
	s.Require().NoError(err)

	loginBody := map[string]string{
		"email":    "mfa-success@example.com",
		"password": "password123",
		"mfa_code": code,
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mfa-test")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["token"])
	assert.Equal(s.T(), "mfa-success@example.com", data["email"])
	assert.Equal(s.T(), float64(user.ID), data["user_id"])
	assert.Equal(s.T(), service.PluginPermissionModeAuthoritative, data["permission_mode"])
	assert.Empty(s.T(), data["permissions"])
	assert.Empty(s.T(), data["restricted_plugins"])
	assert.Empty(s.T(), data["mfa_required"])

	var attempt model.MFALoginAttempt
	s.Require().NoError(s.db.Where("user_id = ?", user.ID).First(&attempt).Error)
	assert.True(s.T(), attempt.Success)
	assert.Equal(s.T(), "auto", attempt.Method)
	assert.Equal(s.T(), "mfa-test", attempt.UserAgent)

	var mfa model.UserMFA
	s.Require().NoError(s.db.Where("user_id = ?", user.ID).First(&mfa).Error)
	assert.Equal(s.T(), model.MFAMethodTOTP, mfa.LastMethod)
}

func (s *AuthHandlerTestSuite) TestLoginHandler_MFAInvalidCodeRecordsAttemptWithoutToken() {
	handler := NewAuthHandler(s.cfg)
	s.router.POST("/login", handler.Login)
	user, _ := s.createMFAEnabledLoginUser("mfa-invalid@example.com", "password123")

	loginBody := map[string]string{
		"email":    "mfa-invalid@example.com",
		"password": "password123",
		"mfa_code": "000000",
	}
	jsonLoginBody, err := json.Marshal(loginBody)
	s.Require().NoError(err)

	req, err := http.NewRequest("POST", "/login", bytes.NewReader(jsonLoginBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid mfa code")

	var attempt model.MFALoginAttempt
	s.Require().NoError(s.db.Where("user_id = ?", user.ID).First(&attempt).Error)
	assert.False(s.T(), attempt.Success)
	assert.Equal(s.T(), "auto", attempt.Method)
}

func TestAuthHandler(t *testing.T) {
	suite.Run(t, new(AuthHandlerTestSuite))
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

func (s *UserHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), s.testUser.Email, data["email"])
	assert.Equal(s.T(), float64(s.testUser.ID), data["user_id"])
	assert.Equal(s.T(), "Test Plan", data["plan_name"])
	assert.Equal(s.T(), "/s", data["subscribe_path"])
}

func (s *UserHandlerTestSuite) TestGetSubscription_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/subscription", handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "未登录")
}

func (s *UserHandlerTestSuite) TestGetSubscription_InvalidUserID() {
	handler := NewUserHandler()
	s.router.GET("/subscription", func(c *gin.Context) {
		c.Set("user_id", "invalid")
		c.Next()
	}, handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户ID无效")
}

func (s *UserHandlerTestSuite) TestGetSubscription_UserNotFound() {
	handler := NewUserHandler()
	s.router.GET("/subscription", func(c *gin.Context) {
		c.Set("user_id", uint(99999))
		c.Next()
	}, handler.GetSubscription)

	req, _ := http.NewRequest("GET", "/subscription", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), s.testUser.Email, data["email"])
	assert.Equal(s.T(), float64(s.testUser.ID), data["user_id"])
}

func (s *UserHandlerTestSuite) TestGetProfile_Success() {
	handler := NewUserHandler()
	group := model.AccessGroup{ScopeID: "profile-security", Name: "restricted", Enabled: true}
	s.Require().NoError(s.db.Create(&group).Error)
	s.Require().NoError(s.db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: "profile-security",
		Permissions: `["profile-security.view"]`,
	}).Error)
	s.router.GET("/profile", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testUser.ID), data["id"])
	assert.Equal(s.T(), s.testUser.Email, data["email"])
	assert.Equal(s.T(), s.testUser.UUID, data["uuid"])
	assert.Equal(s.T(), s.testUser.Token, data["token"])
	assert.Equal(s.T(), false, data["is_admin"])
	assert.Equal(s.T(), service.PluginPermissionModeAuthoritative, data["permission_mode"])
	assert.Empty(s.T(), data["permissions"])
	assert.Empty(s.T(), data["restricted_plugins"])
}

func (s *UserHandlerTestSuite) TestGetProfile_PluginPermissionModeAndGrants() {
	handler := NewUserHandler()
	s.router.GET("/profile", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.GetProfile)
	s.Require().NoError(s.db.Model(s.testUser).Update("is_admin", 1).Error)

	requestProfile := func() map[string]any {
		req, _ := http.NewRequest(http.MethodGet, "/profile", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		s.Require().Equal(http.StatusOK, w.Code)
		resp := decodePanelTestResponse(s.T(), w)
		s.Require().Equal(float64(0), resp["code"])
		return resp["data"].(map[string]any)
	}

	data := requestProfile()
	assert.Equal(s.T(), service.PluginPermissionModeLegacy, data["permission_mode"])
	assert.Nil(s.T(), data["permissions"])
	assert.Empty(s.T(), data["restricted_plugins"])

	group := model.AccessGroup{ScopeID: "profile-ui", Name: "profile-viewers", Enabled: true}
	s.Require().NoError(s.db.Create(&group).Error)
	s.Require().NoError(s.db.Create(&model.ResourceGrant{
		GroupID: group.ID, ResourceType: service.PluginAPIGrantResourceType, ResourceID: "profile-ui",
		Permissions: `["profile-ui.write","profile-ui.view","profile-ui.view"]`,
	}).Error)

	data = requestProfile()
	assert.Equal(s.T(), service.PluginPermissionModeMixed, data["permission_mode"])
	assert.Empty(s.T(), data["permissions"], "an unassigned admin must fail closed after the first plugin_api grant")
	assert.Equal(s.T(), []any{"profile-ui"}, data["restricted_plugins"])

	s.Require().NoError(s.db.Create(&model.AccessGroupUser{GroupID: group.ID, UserID: s.testUser.ID}).Error)
	data = requestProfile()
	assert.Equal(s.T(), service.PluginPermissionModeMixed, data["permission_mode"])
	assert.Equal(s.T(), []any{"profile-ui.view", "profile-ui.write"}, data["permissions"])
	assert.Equal(s.T(), []any{"profile-ui"}, data["restricted_plugins"])
}

func (s *UserHandlerTestSuite) TestGetProfile_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/profile", handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "未登录")
}

func (s *UserHandlerTestSuite) TestGetProfile_InvalidUserID() {
	handler := NewUserHandler()
	s.router.GET("/profile", func(c *gin.Context) {
		c.Set("user_id", "invalid")
		c.Next()
	}, handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户ID无效")
}

func (s *UserHandlerTestSuite) TestGetProfile_UserNotFound() {
	handler := NewUserHandler()
	s.router.GET("/profile", func(c *gin.Context) {
		c.Set("user_id", uint(99999))
		c.Next()
	}, handler.GetProfile)

	req, _ := http.NewRequest("GET", "/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	subscription := data["subscription"].(map[string]any)
	assert.Equal(s.T(), s.testUser.Email, subscription["email"])
	assert.Equal(s.T(), float64(s.testUser.ID), subscription["user_id"])
	assert.Equal(s.T(), "Test Plan", subscription["plan_name"])
}

func (s *UserHandlerTestSuite) TestGetDashboard_Unauthorized() {
	handler := NewUserHandler()
	s.router.GET("/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "未登录")
}

func (s *UserHandlerTestSuite) TestGetDashboard_InvalidUserID() {
	handler := NewUserHandler()
	s.router.GET("/dashboard", func(c *gin.Context) {
		c.Set("user_id", "invalid")
		c.Next()
	}, handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户ID无效")
}

func (s *UserHandlerTestSuite) TestGetDashboard_UserNotFound() {
	handler := NewUserHandler()
	s.router.GET("/dashboard", func(c *gin.Context) {
		c.Set("user_id", uint(99999))
		c.Next()
	}, handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
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
		APIKey:      "handler-test-api-key",
		Secret:      "handler-test-secret",
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "list")
	assert.Contains(s.T(), data, "total")
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNodes_WithPagination() {
	handler := NewNodeHandler()
	s.router.GET("/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/nodes?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Len(s.T(), data["list"].([]any), 1)
	assert.Equal(s.T(), float64(1), data["total"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNodes_WithFilters() {
	handler := NewNodeHandler()
	s.router.GET("/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/nodes?status=1&search=test", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotNil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNode_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testNode.ID), data["id"])
	assert.Equal(s.T(), s.testNode.Name, data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNodeCredentials_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/:id/credentials", handler.GetNodeCredentials)

	req, _ := http.NewRequest("GET", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/credentials", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testNode.ID), data["node_id"])
	assert.Equal(s.T(), s.testNode.Name, data["name"])
	assert.NotEmpty(s.T(), data["api_key"])
	assert.NotEmpty(s.T(), data["secret"])
	assert.NotContains(s.T(), resp, "error")
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
	assert.NotEmpty(s.T(), data["api_key"])
	assert.NotEmpty(s.T(), data["secret"])
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

func (s *NodeHandlerTestSuite) TestDeleteNode_Success() {
	handler := NewNodeHandler()
	s.router.DELETE("/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNodeStats_Success() {
	handler := NewNodeHandler()
	s.router.GET("/nodes/stats", handler.GetNodeStats)

	req, _ := http.NewRequest("GET", "/nodes/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "total")
	assert.Contains(s.T(), data, "online")
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetNodeLogs_Success() {
	loggedAt := time.Now().UTC()
	log := &model.NodeLog{
		NodeID:     s.testNode.ID,
		Level:      service.NodeLogLevelInfo,
		Source:     "grpc",
		Message:    "node started",
		TraceID:    "trace-1",
		FieldsJSON: `{"pid":123}`,
		LoggedAt:   &loggedAt,
	}
	assert.NoError(s.T(), s.db.Create(log).Error)

	handler := NewNodeHandler()
	s.router.GET("/nodes/:id/logs", handler.GetNodeLogs)

	req, _ := http.NewRequest("GET", "/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(1), data["total"])
	assert.Equal(s.T(), float64(1), data["page"])
	list := data["list"].([]any)
	s.Require().Len(list, 1)
	item := list[0].(map[string]any)
	assert.Equal(s.T(), "node started", item["message"])
	assert.Equal(s.T(), "grpc", item["source"])
	assert.Equal(s.T(), map[string]any{"pid": float64(123)}, item["fields"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestGetProtocolTemplates() {
	handler := NewNodeHandler()
	s.router.GET("/protocol-templates", handler.GetProtocolTemplates)

	req, _ := http.NewRequest("GET", "/protocol-templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.NotEmpty(s.T(), data)
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
	assert.NotContains(s.T(), resp, "error")
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

	body := map[string]any{
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "vless", data["type"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NodeHandlerTestSuite) TestValidateRawConfig_Success() {
	handler := NewNodeHandler()
	s.router.POST("/validate-config", handler.ValidateRawConfig)

	body := map[string]any{
		"raw_config": map[string]any{
			"server_port": 443,
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/validate-config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), true, data["valid"])
	assert.Equal(s.T(), "配置有效", data["message"])
	assert.NotContains(s.T(), resp, "error")
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

	speedLimit := int64(80)
	deviceLimit := 5
	transferEnable := int64(1073741824)
	body := map[string]any{
		"email":           "newuser@example.com",
		"password":        "password123",
		"is_admin":        0,
		"transfer_enable": transferEnable,
		"speed_limit":     speedLimit,
		"device_limit":    deviceLimit,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "newuser@example.com", data["email"])
	assert.NotZero(s.T(), data["id"])
	assert.NotContains(s.T(), resp, "error")

	var created model.User
	assert.NoError(s.T(), s.db.Where("email = ?", "newuser@example.com").First(&created).Error)
	assert.Equal(s.T(), transferEnable, created.TransferEnable)
	if assert.NotNil(s.T(), created.SpeedLimit) {
		assert.Equal(s.T(), speedLimit, *created.SpeedLimit)
	}
	if assert.NotNil(s.T(), created.DeviceLimit) {
		assert.Equal(s.T(), deviceLimit, *created.DeviceLimit)
	}
}

func (s *AdminHandlerTestSuite) TestCreateUser_DuplicateEmail() {
	handler := NewAdminHandler()
	s.router.POST("/users", handler.CreateUser)

	body := map[string]any{
		"email":    s.testUser.Email,
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "该邮箱已被注册")
}

func (s *AdminHandlerTestSuite) TestCreateUser_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/users", handler.CreateUser)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "参数错误")
}

func (s *AdminHandlerTestSuite) TestGetUserList_Success() {
	handler := NewAdminHandler()
	s.router.GET("/users", handler.GetUserList)

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "list")
	assert.Contains(s.T(), data, "total")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetUserList_WithFilters() {
	handler := NewAdminHandler()
	s.router.GET("/users", handler.GetUserList)

	req, _ := http.NewRequest("GET", "/users?page=1&page_size=10&email=test&status=active", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "list")
	assert.Contains(s.T(), data, "total")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetUser_Success() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testUser.ID), data["id"])
	assert.Equal(s.T(), s.testUser.Email, data["email"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetUser_NotFound() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestGetUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.GET("/users/:id", handler.GetUser)

	req, _ := http.NewRequest("GET", "/users/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestUpdateUser_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	body := map[string]any{
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "更新成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestUpdateUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	body := map[string]any{"balance": 1000}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/users/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestUpdateUser_InvalidBody() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	req, _ := http.NewRequest("PUT", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10), strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "参数错误")
}

func (s *AdminHandlerTestSuite) TestUpdateUser_NotFound() {
	handler := NewAdminHandler()
	s.router.PUT("/users/:id", handler.UpdateUser)

	body := map[string]any{"balance": 1000}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/users/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestBanUser_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/ban", handler.BanUser)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/ban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "封禁成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "解封成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestResetUserTraffic_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-traffic", handler.ResetUserTraffic)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/reset-traffic", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "流量重置成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestResetUserSubscribe_Success() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-subscribe", handler.ResetUserSubscribe)

	req, _ := http.NewRequest("POST", "/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/reset-subscribe", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["token"])
	assert.NotEqual(s.T(), s.testUser.Token, data["token"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetDashboard() {
	handler := NewAdminHandler()
	s.router.GET("/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "total_users")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetDashboard_DBErrorUsesPanelEnvelope() {
	s.Require().NoError(s.db.Migrator().DropTable(&model.User{}))
	defer func() {
		s.Require().NoError(s.db.AutoMigrate(&model.User{}))
	}()

	handler := NewAdminHandler()
	s.router.GET("/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/dashboard?refresh=true", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "获取统计失败")
}

func (s *AdminHandlerTestSuite) TestGetHourlyTraffic_UsesPanelEnvelope() {
	handler := NewAdminHandler()
	s.router.GET("/traffic/hourly", handler.GetHourlyTraffic)

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	logAt := currentHour + 60
	assert.NoError(s.T(), s.db.Create(&model.TrafficLog{
		UserID:     s.testUser.ID,
		ServerID:   1,
		ServerType: string(model.ServerTypeShadowsocks),
		U:          512,
		D:          1536,
		Rate:       1,
		LogAt:      logAt,
	}).Error)

	req, _ := http.NewRequest("GET", "/traffic/hourly?hours=2&user_id="+strconv.FormatUint(uint64(s.testUser.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	list := data["list"].([]any)
	assert.Len(s.T(), list, 2)
	meta := data["meta"].(map[string]any)
	assert.Equal(s.T(), float64(logAt), meta["latest_log_at"])
	assert.NotContains(s.T(), w.Body.String(), "\"error\"")
}

func (s *AdminHandlerTestSuite) TestGetUserTrafficRanking_UsesPanelEnvelope() {
	handler := NewAdminHandler()
	s.router.GET("/traffic/user-ranking", handler.GetUserTrafficRanking)

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	assert.NoError(s.T(), s.db.Create(&model.TrafficLog{
		UserID:     s.testUser.ID,
		ServerID:   1,
		ServerType: string(model.ServerTypeShadowsocks),
		U:          1024,
		D:          2048,
		Rate:       1,
		LogAt:      currentHour + 60,
	}).Error)

	req, _ := http.NewRequest("GET", "/traffic/user-ranking?hours=168&limit=500&include_zero_users=true", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	list := data["list"].([]any)
	if assert.NotEmpty(s.T(), list) {
		first := list[0].(map[string]any)
		assert.NotZero(s.T(), first["user_id"])
	}
	assert.Contains(s.T(), w.Body.String(), "adminhandler@example.com")
	assert.NotContains(s.T(), w.Body.String(), "\"error\"")
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

func (s *AdminHandlerTestSuite) TestGetPlans() {
	handler := NewAdminHandler()
	s.router.GET("/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.NotEmpty(s.T(), data)
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetPlan_Success() {
	handler := NewAdminHandler()
	s.router.GET("/plans/:id", handler.GetPlan)

	req, _ := http.NewRequest("GET", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testPlan.ID), data["id"])
	assert.Equal(s.T(), s.testPlan.Name, data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetPlan_NotFound() {
	handler := NewAdminHandler()
	s.router.GET("/plans/:id", handler.GetPlan)

	req, _ := http.NewRequest("GET", "/plans/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐不存在")
}

func (s *AdminHandlerTestSuite) TestGetPlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.GET("/plans/:id", handler.GetPlan)

	req, _ := http.NewRequest("GET", "/plans/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "ID")
}

func (s *AdminHandlerTestSuite) TestCreatePlan_Success() {
	handler := NewAdminHandler()
	s.router.POST("/plans", handler.CreatePlan)

	speedLimit := int64(100)
	deviceLimit := 3
	body := map[string]any{
		"name":            "New Plan",
		"group_id":        1,
		"transfer_enable": 100,
		"month_price":     1000,
		"speed_limit":     speedLimit,
		"device_limit":    deviceLimit,
		"show":            1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "New Plan", data["name"])
	assert.NotContains(s.T(), resp, "error")

	var created model.Plan
	assert.NoError(s.T(), s.db.Where("name = ?", "New Plan").First(&created).Error)
	if assert.NotNil(s.T(), created.SpeedLimit) {
		assert.Equal(s.T(), speedLimit, *created.SpeedLimit)
	}
	if assert.NotNil(s.T(), created.DeviceLimit) {
		assert.Equal(s.T(), deviceLimit, *created.DeviceLimit)
	}
}

func (s *AdminHandlerTestSuite) TestBanUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/ban", handler.BanUser)

	req, _ := http.NewRequest("POST", "/users/invalid/ban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestBanUser_NotFound() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/ban", handler.BanUser)

	req, _ := http.NewRequest("POST", "/users/99999/ban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestUnbanUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/unban", handler.UnbanUser)

	req, _ := http.NewRequest("POST", "/users/invalid/unban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestUnbanUser_NotFound() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/unban", handler.UnbanUser)

	req, _ := http.NewRequest("POST", "/users/99999/unban", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestResetUserTraffic_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-traffic", handler.ResetUserTraffic)

	req, _ := http.NewRequest("POST", "/users/invalid/reset-traffic", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestResetUserTraffic_NotFound() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-traffic", handler.ResetUserTraffic)

	req, _ := http.NewRequest("POST", "/users/99999/reset-traffic", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestResetUserSubscribe_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-subscribe", handler.ResetUserSubscribe)

	req, _ := http.NewRequest("POST", "/users/invalid/reset-subscribe", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestResetUserSubscribe_NotFound() {
	handler := NewAdminHandler()
	s.router.POST("/users/:id/reset-subscribe", handler.ResetUserSubscribe)

	req, _ := http.NewRequest("POST", "/users/99999/reset-subscribe", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestDeleteUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.DELETE("/users/:id", handler.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户ID")
}

func (s *AdminHandlerTestSuite) TestDeleteUser_NotFound() {
	handler := NewAdminHandler()
	s.router.DELETE("/users/:id", handler.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestCreatePlan_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/plans", handler.CreatePlan)

	req, _ := http.NewRequest("POST", "/plans", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "参数错误")
}

func (s *AdminHandlerTestSuite) TestGetUserStats() {
	handler := NewAdminHandler()
	s.router.GET("/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "total_users")
	assert.Contains(s.T(), data, "active_users")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestGetUserStats_DBErrorUsesPanelEnvelope() {
	s.Require().NoError(s.db.Migrator().DropTable(&model.User{}))
	defer func() {
		s.Require().NoError(s.db.AutoMigrate(&model.User{}))
	}()

	handler := NewAdminHandler()
	s.router.GET("/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "获取统计失败")
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_Success() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]any{
		"user_id": s.testUser.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "分配成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]any{
		"user_id": s.testUser.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/invalid/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐")
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_UserNotFound() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]any{
		"user_id": 99999,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func (s *AdminHandlerTestSuite) TestAssignPlanToUser_PlanNotFound() {
	handler := NewAdminHandler()
	s.router.POST("/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]any{
		"user_id": s.testUser.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/99999/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐不存在")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminHandlerTestSuite) TestDeletePlan_NotFound() {
	handler := NewAdminHandler()
	s.router.DELETE("/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/plans/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐不存在")
}

func (s *AdminHandlerTestSuite) TestUpdatePlan_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/plans/:id", handler.UpdatePlan)

	speedLimit := int64(120)
	deviceLimit := 4
	body := map[string]any{
		"name":            "Updated Plan",
		"transfer_enable": 200,
		"speed_limit":     speedLimit,
		"device_limit":    deviceLimit,
		"show":            1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "Updated Plan", data["name"])
	assert.NotContains(s.T(), resp, "error")

	var updated model.Plan
	assert.NoError(s.T(), s.db.First(&updated, s.testPlan.ID).Error)
	if assert.NotNil(s.T(), updated.SpeedLimit) {
		assert.Equal(s.T(), speedLimit, *updated.SpeedLimit)
	}
	if assert.NotNil(s.T(), updated.DeviceLimit) {
		assert.Equal(s.T(), deviceLimit, *updated.DeviceLimit)
	}
}

func (s *AdminHandlerTestSuite) TestUpdatePlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/plans/:id", handler.UpdatePlan)

	body := map[string]any{
		"name":            "Updated Plan",
		"transfer_enable": 200,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/plans/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "ID")
}

func (s *AdminHandlerTestSuite) TestUpdatePlan_NotFound() {
	handler := NewAdminHandler()
	s.router.PUT("/plans/:id", handler.UpdatePlan)

	body := map[string]any{
		"name":            "Missing Plan",
		"transfer_enable": 200,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/plans/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐不存在")
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

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroups_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups", handler.GetGroups)

	req, _ := http.NewRequest("GET", "/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotNil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscriptionAdminHandlerTestSuite) TestCreateGroup_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups", handler.CreateGroup)

	body := map[string]any{
		"name": "Test Group",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "Test Group", data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id", handler.GetGroup)

	req, _ := http.NewRequest("GET", "/groups/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroup_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id", handler.GetGroup)

	req, _ := http.NewRequest("GET", "/groups/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/groups/:id", handler.UpdateGroup)

	body := map[string]any{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/groups/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestDeleteGroup_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/groups/:id", handler.DeleteGroup)

	req, _ := http.NewRequest("DELETE", "/groups/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetTemplates_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id/templates", handler.GetTemplates)

	req, _ := http.NewRequest("GET", "/groups/invalid/templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroupProtocols_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/groups/:id/protocols", handler.GetGroupProtocols)

	req, _ := http.NewRequest("GET", "/groups/invalid/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateGroupProtocols_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups/:id/protocols", handler.UpdateGroupProtocols)

	body := map[string]any{"protocol_ids": []uint{1, 2}}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups/invalid/protocols", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetAvailableProtocols_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/protocols/available", handler.GetAvailableProtocols)

	req, _ := http.NewRequest("GET", "/protocols/available", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotNil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscriptionAdminHandlerTestSuite) TestCreateTemplate_InvalidGroupID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/groups/:id/templates", handler.CreateTemplate)

	body := map[string]any{"name": "Test Template"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/groups/invalid/templates", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/templates/:id", handler.GetTemplate)

	req, _ := http.NewRequest("GET", "/templates/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestUpdateTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/templates/:id", handler.UpdateTemplate)

	body := map[string]any{"name": "Updated"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/templates/invalid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestDeleteTemplate_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/templates/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscriptionAdminHandlerTestSuite) TestAssignGroupToUser_InvalidUserID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/users/:user_id/groups", handler.AssignGroupToUser)

	body := map[string]any{"group_id": 1}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/users/invalid/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户")
}

func (s *SubscriptionAdminHandlerTestSuite) TestRemoveGroupFromUser_InvalidIDs() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/users/:user_id/groups/:group_id", handler.RemoveGroupFromUser)

	req, _ := http.NewRequest("DELETE", "/users/invalid/groups/1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户")

	req, _ = http.NewRequest("DELETE", "/users/1/groups/invalid", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetUserGroups_InvalidUserID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/users/:user_id/groups", handler.GetUserGroups)

	req, _ := http.NewRequest("GET", "/users/invalid/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户")
}

func (s *SubscriptionAdminHandlerTestSuite) TestAssignGroupToPlan_InvalidPlanID() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/plans/:plan_id/groups", handler.AssignGroupToPlan)

	body := map[string]any{"group_id": 1}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/plans/invalid/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "套餐")
}

func (s *SubscriptionAdminHandlerTestSuite) TestRemoveGroupFromPlan_InvalidIDs() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/plans/:plan_id/groups/:group_id", handler.RemoveGroupFromPlan)

	req, _ := http.NewRequest("DELETE", "/plans/invalid/groups/1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "套餐")

	req, _ = http.NewRequest("DELETE", "/plans/1/groups/invalid", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetPlanGroups_InvalidPlanID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/plans/:plan_id/groups", handler.GetPlanGroups)

	req, _ := http.NewRequest("GET", "/plans/invalid/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "套餐")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetSubscriptionFormats_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/formats", handler.GetSubscriptionFormats)

	req, _ := http.NewRequest("GET", "/formats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.GreaterOrEqual(s.T(), len(data), 12) // At least 12 formats (including auto)
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetProtocolTypes_Success() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/protocols", handler.GetProtocolTypes)

	req, _ := http.NewRequest("GET", "/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.GreaterOrEqual(s.T(), len(data), 6) // At least 6 protocol types
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscriptionAdminHandlerTestSuite) TestGetGroupStats_UsesPanelEnvelope() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/stats", handler.GetGroupStats)

	req, _ := http.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.NotNil(s.T(), data)
	assert.NotContains(s.T(), resp, "error")

	if len(data) > 0 {
		first := data[0].(map[string]any)
		assert.Contains(s.T(), first, "group_id")
		assert.Contains(s.T(), first, "group_name")
		assert.Contains(s.T(), first, "user_count")
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

func (s *KnowledgeHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *KnowledgeHandlerTestSuite) TestGetArticles_Success() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/knowledge", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].([]any)
	if assert.Len(s.T(), data, 1) {
		first := data[0].(map[string]any)
		assert.Equal(s.T(), float64(s.testKnowledge.ID), first["id"])
		assert.Equal(s.T(), "test", first["category"])
		assert.Equal(s.T(), "Test Article", first["title"])
		assert.Equal(s.T(), "Test content", first["body"])
		assert.Contains(s.T(), first, "updated_at")
	}
}

func (s *KnowledgeHandlerTestSuite) TestGetArticles_WithCategory() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/knowledge?category=test", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_Success() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/"+strconv.FormatUint(uint64(s.testKnowledge.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testKnowledge.ID), data["id"])
	assert.Equal(s.T(), "test", data["category"])
	assert.Equal(s.T(), "Test Article", data["title"])
	assert.Equal(s.T(), "Test content", data["body"])
	assert.Contains(s.T(), data, "updated_at")
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_NotFound() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "文章不存在")
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_HiddenArticleNotFound() {
	hidden := &model.Knowledge{
		Category: "test",
		Title:    "Hidden Article",
		Body:     "Hidden content",
		Show:     0,
	}
	s.db.Create(hidden)
	s.db.Model(hidden).Update("show", 0)

	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/"+strconv.FormatUint(uint64(hidden.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "文章不存在")
}

func (s *KnowledgeHandlerTestSuite) TestGetArticle_InvalidID() {
	handler := NewKnowledgeHandler()
	s.router.GET("/knowledge/:id", handler.GetArticle)

	req, _ := http.NewRequest("GET", "/knowledge/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的文章ID")
}

func TestKnowledgeHandler(t *testing.T) {
	suite.Run(t, new(KnowledgeHandlerTestSuite))
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].([]any)
	if assert.Len(s.T(), data, 1) {
		first := data[0].(map[string]any)
		assert.Equal(s.T(), "Test Plan", first["name"])
		assert.Equal(s.T(), float64(1), first["show"])
	}
}

func (s *UserPlanHandlerTestSuite) TestGetPlans_WithShow() {
	handler := NewUserPlanHandler()
	s.router.GET("/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/plans?show=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
}

func (s *UserPlanHandlerTestSuite) TestGetPlans_DatabaseErrorUsesPanelEnvelope() {
	handler := NewUserPlanHandler()
	s.router.GET("/plans", handler.GetPlans)
	s.Require().NoError(s.db.Migrator().DropTable(&model.Plan{}))
	s.T().Cleanup(func() {
		s.Require().NoError(s.db.AutoMigrate(&model.Plan{}))
	})

	req, _ := http.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "获取套餐列表失败")
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func TestUserPlanHandler(t *testing.T) {
	suite.Run(t, new(UserPlanHandlerTestSuite))
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

func (s *TicketHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].([]any)
	if assert.Len(s.T(), data, 1) {
		first := data[0].(map[string]any)
		assert.Equal(s.T(), float64(s.testTicket.ID), first["id"])
		assert.Equal(s.T(), "Test Ticket", first["subject"])
		assert.Equal(s.T(), float64(1), first["level"])
		assert.Equal(s.T(), float64(0), first["status"])
	}
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].([]any)
	assert.Empty(s.T(), data)
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.NotZero(s.T(), data["id"])
	assert.Equal(s.T(), "New Ticket", data["subject"])
	assert.Equal(s.T(), float64(1), data["level"])
	assert.Equal(s.T(), float64(0), data["status"])
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

	s.assertPanelError(w, "参数错误")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testTicket.ID), data["id"])
	assert.Equal(s.T(), "Test Ticket", data["subject"])
	assert.Equal(s.T(), float64(s.testUser.ID), data["user_id"])
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

	s.assertPanelError(w, "工单不存在")
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

	s.assertPanelError(w, "无效的工单ID")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	assert.Equal(s.T(), "回复成功", resp["data"])
}

func (s *TicketHandlerTestSuite) TestReplyTicket_InvalidID() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/reply", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{"message": "Reply message"}`
	req, _ := http.NewRequest("POST", "/tickets/invalid/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的工单ID")
}

func (s *TicketHandlerTestSuite) TestReplyTicket_MissingMessage() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/reply", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{}`
	req, _ := http.NewRequest("POST", "/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
}

func (s *TicketHandlerTestSuite) TestReplyTicket_NotFound() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/reply", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.ReplyTicket)

	body := `{"message": "Reply message"}`
	req, _ := http.NewRequest("POST", "/tickets/99999/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "工单不存在")
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

	s.assertPanelError(w, "工单已关闭，无法回复")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "error")
	assert.Equal(s.T(), "工单已关闭", resp["data"])
}

func (s *TicketHandlerTestSuite) TestCloseTicket_InvalidID() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/close", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/tickets/invalid/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的工单ID")
}

func (s *TicketHandlerTestSuite) TestCloseTicket_NotFound() {
	handler := NewTicketHandler()
	s.router.POST("/tickets/:id/close", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/tickets/99999/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "工单不存在")
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

func (s *MFAHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), false, data["enabled"])
	assert.Equal(s.T(), false, data["has_backup_codes"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAHandlerTestSuite) TestGetAdminConfig() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/mfa/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Contains(s.T(), data, "enabled")
	assert.Contains(s.T(), data, "methods")
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), true, data["enabled"])
	assert.Equal(s.T(), "TestApp", data["totp_issuer"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAHandlerTestSuite) TestUpdateAdminConfig_InvalidBody() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	body := `invalid json`
	req, _ := http.NewRequest("PUT", "/admin/mfa/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
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

	s.assertPanelError(w, "Code")
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

	s.assertPanelError(w, "Password")
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

	s.assertPanelError(w, "Code")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.NotEmpty(s.T(), data["secret"])
	assert.NotEmpty(s.T(), data["url"])
	assert.NotEmpty(s.T(), data["backup_codes"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAHandlerTestSuite) TestSetupTOTP_MissingUserUsesPanelEnvelope() {
	handler := NewMFAHandler()
	s.router.POST("/mfa/totp/setup", func(c *gin.Context) {
		c.Set("user_id", uint(99999))
		c.Next()
	}, handler.SetupTOTP)

	req, _ := http.NewRequest("POST", "/mfa/totp/setup", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "user not found")
}

func (s *MFAHandlerTestSuite) TestEnableTOTP_Success() {
	handler := NewMFAHandler()
	setup, err := handler.mfaService.SetupTOTP(s.testUser.ID, s.testUser.Email)
	assert.NoError(s.T(), err)
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	assert.NoError(s.T(), err)

	s.router.POST("/mfa/totp/enable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.EnableTOTP)

	body := `{"code":"` + code + `"}`
	req, _ := http.NewRequest("POST", "/mfa/totp/enable", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "MFA enabled successfully", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAHandlerTestSuite) TestEnableTOTP_InvalidCodeUsesPanelEnvelope() {
	handler := NewMFAHandler()
	_, err := handler.mfaService.SetupTOTP(s.testUser.ID, s.testUser.Email)
	assert.NoError(s.T(), err)

	s.router.POST("/mfa/totp/enable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.EnableTOTP)

	req, _ := http.NewRequest("POST", "/mfa/totp/enable", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid code")
}

func (s *MFAHandlerTestSuite) TestDisableMFA_Success() {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	assert.NoError(s.T(), err)
	s.testUser.Password = string(hashedPassword)
	assert.NoError(s.T(), s.db.Save(s.testUser).Error)
	assert.NoError(s.T(), s.db.Create(&model.UserMFA{
		UserID:      s.testUser.ID,
		Enabled:     true,
		TOTPSecret:  "SECRET",
		BackupCodes: `["ABCD-EFGH"]`,
	}).Error)

	handler := NewMFAHandler()
	s.router.POST("/mfa/disable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.DisableMFA)

	body := `{"password":"correct-password"}`
	req, _ := http.NewRequest("POST", "/mfa/disable", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "MFA disabled successfully", data["message"])

	var count int64
	assert.NoError(s.T(), s.db.Model(&model.UserMFA{}).Where("user_id = ?", s.testUser.ID).Count(&count).Error)
	assert.Equal(s.T(), int64(0), count)
}

func (s *MFAHandlerTestSuite) TestDisableMFA_WrongPasswordUsesPanelEnvelope() {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	assert.NoError(s.T(), err)
	s.testUser.Password = string(hashedPassword)
	assert.NoError(s.T(), s.db.Save(s.testUser).Error)
	assert.NoError(s.T(), s.db.Create(&model.UserMFA{
		UserID:      s.testUser.ID,
		Enabled:     true,
		TOTPSecret:  "SECRET",
		BackupCodes: `["ABCD-EFGH"]`,
	}).Error)

	handler := NewMFAHandler()
	s.router.POST("/mfa/disable", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.DisableMFA)

	req, _ := http.NewRequest("POST", "/mfa/disable", strings.NewReader(`{"password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid password")
}

func (s *MFAHandlerTestSuite) TestVerifyMFA_BackupCodeSuccess() {
	assert.NoError(s.T(), s.db.Create(&model.UserMFA{
		UserID:      s.testUser.ID,
		Enabled:     true,
		TOTPSecret:  "SECRET",
		BackupCodes: `["ABCD-EFGH"]`,
	}).Error)

	handler := NewMFAHandler()
	s.router.POST("/mfa/verify", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.VerifyMFA)

	body := `{"code":"ABCD-EFGH","method":"backup"}`
	req, _ := http.NewRequest("POST", "/mfa/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "verified successfully", data["message"])

	remaining, err := handler.mfaService.GetRemainingBackupCodes(s.testUser.ID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, remaining)
}

func (s *MFAHandlerTestSuite) TestVerifyMFA_InvalidCodeUsesPanelEnvelope() {
	assert.NoError(s.T(), s.db.Create(&model.UserMFA{
		UserID:      s.testUser.ID,
		Enabled:     true,
		TOTPSecret:  "SECRET",
		BackupCodes: `["ABCD-EFGH"]`,
	}).Error)

	handler := NewMFAHandler()
	s.router.POST("/mfa/verify", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.VerifyMFA)

	req, _ := http.NewRequest("POST", "/mfa/verify", strings.NewReader(`{"code":"invalid","method":"backup"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid code")
}

func (s *MFAHandlerTestSuite) TestRegenerateBackupCodes_Success() {
	assert.NoError(s.T(), s.db.Create(&model.UserMFA{
		UserID:      s.testUser.ID,
		Enabled:     true,
		TOTPSecret:  "SECRET",
		BackupCodes: `["ABCD-EFGH"]`,
	}).Error)

	handler := NewMFAHandler()
	s.router.POST("/mfa/backup-codes/regenerate", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		c.Next()
	}, handler.RegenerateBackupCodes)

	req, _ := http.NewRequest("POST", "/mfa/backup-codes/regenerate", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	codes, ok := data["backup_codes"].([]any)
	assert.True(s.T(), ok)
	assert.Len(s.T(), codes, 10)
	assert.NotContains(s.T(), resp, "error")
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

	s.assertPanelError(w, "MFA not enabled")
}

func TestMFAHandler(t *testing.T) {
	suite.Run(t, new(MFAHandlerTestSuite))
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "notification not found")
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationHandlerTestSuite) TestUpdateTemplate_NotFound() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/templates/:id", handler.UpdateTemplate)

	body := `{"name": "Updated"}`
	req, _ := http.NewRequest("PUT", "/admin/notification/templates/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "template not found")
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramHandlerTestSuite) TestUpdateBot_NotConfigured() {
	handler := NewTelegramHandler()
	s.router.PUT("/admin/telegram/bot", handler.UpdateBot)

	body := `{"name": "Test Bot"}`
	req, _ := http.NewRequest("PUT", "/admin/telegram/bot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramHandlerTestSuite) TestSetWebhook_MissingURL() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/webhook", handler.SetWebhook)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/telegram/webhook", strings.NewReader(body))
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

func (s *AdminKnowledgeHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminKnowledgeHandlerTestSuite) TestGetArticles() {
	handler := NewAdminKnowledgeHandler()
	s.router.GET("/admin/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/admin/knowledge", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.Equal(s.T(), "操作成功", resp["msg"])
	assert.NotEmpty(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
	first := data[0].(map[string]any)
	assert.Equal(s.T(), float64(s.testArticle.ID), first["id"])
	assert.Equal(s.T(), "Test Article", first["title"])
	assert.Equal(s.T(), "公告", first["category"])
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

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "创建成功", data["message"])
	article := data["data"].(map[string]any)
	assert.Equal(s.T(), "New Article", article["title"])
	assert.Equal(s.T(), "公告", article["category"])
	assert.Equal(s.T(), float64(1), article["show"])
}

func (s *AdminKnowledgeHandlerTestSuite) TestCreateArticle_MissingFields() {
	handler := NewAdminKnowledgeHandler()
	s.router.POST("/admin/knowledge", handler.CreateArticle)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
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

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "更新成功", data["message"])

	var updated model.Knowledge
	assert.NoError(s.T(), s.db.First(&updated, s.testArticle.ID).Error)
	assert.Equal(s.T(), "Updated Title", updated.Title)
	assert.Equal(s.T(), "Updated content", updated.Body)
	assert.Equal(s.T(), 1, updated.Sort)
	assert.Equal(s.T(), 1, updated.Show)
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle_NotFound() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"title": "Updated Title"}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/99999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "文章不存在")
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle_InvalidID() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"title": "Updated Title"}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/invalid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的文章ID")
}

func (s *AdminKnowledgeHandlerTestSuite) TestUpdateArticle_InvalidBodyUsesPanelEnvelope() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := `{"show": 2}`
	req, _ := http.NewRequest("PUT", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
}

func (s *AdminKnowledgeHandlerTestSuite) TestDeleteArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])

	var deleted model.Knowledge
	assert.Error(s.T(), s.db.First(&deleted, s.testArticle.ID).Error)
}

func (s *AdminKnowledgeHandlerTestSuite) TestDeleteArticle_NotFound() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "文章不存在")
}

func (s *AdminKnowledgeHandlerTestSuite) TestDeleteArticle_InvalidIDUsesPanelEnvelope() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的文章ID")
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

func (s *AdminTicketHandlerTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], msgContains)
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminTicketHandlerTestSuite) TestGetTickets() {
	handler := NewAdminTicketHandler()
	s.router.GET("/admin/tickets", handler.GetTickets)

	req, _ := http.NewRequest("GET", "/admin/tickets", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.Equal(s.T(), "操作成功", resp["msg"])
	assert.NotEmpty(s.T(), resp["ts"])

	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
	first := data[0].(map[string]any)
	assert.Equal(s.T(), float64(s.testTicket.ID), first["id"])
	assert.Equal(s.T(), float64(s.testUser.ID), first["user_id"])
	assert.Equal(s.T(), "Test Ticket", first["subject"])
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

	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "回复成功", data["message"])

	var updated model.Ticket
	assert.NoError(s.T(), s.db.First(&updated, s.testTicket.ID).Error)
	assert.Equal(s.T(), 1, updated.Status)
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

	s.assertPanelError(w, "参数错误")
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

	s.assertPanelError(w, "工单不存在")
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]any
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "工单已关闭", data["message"])
	assert.Equal(s.T(), float64(s.testTicket.ID), data["id"])

	var updated model.Ticket
	assert.NoError(s.T(), s.db.First(&updated, s.testTicket.ID).Error)
	assert.Equal(s.T(), 2, updated.Status)
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket_NotFound() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/99999/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "工单不存在")
}

func (s *AdminTicketHandlerTestSuite) TestCloseTicket_InvalidID() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/invalid/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "无效的工单ID")
}

func TestAdminTicketHandler(t *testing.T) {
	suite.Run(t, new(AdminTicketHandlerTestSuite))
}

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

	assertPanelTestError(s.T(), w, "ID")
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

	assertPanelTestError(s.T(), w, "ID")
}

func (s *AdminPlanHandlerTestSuite) TestAssignPlanToUser() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	body := `{"user_id": ` + strconv.FormatUint(uint64(s.testUser.ID), 10) + `}`
	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *AdminPlanHandlerTestSuite) TestAssignPlanToUser_MissingUserID() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	body := `{}`
	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "参数错误")
}

func (s *AdminPlanHandlerTestSuite) TestGetUserStats() {
	handler := NewAdminHandler()
	s.router.GET("/admin/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/admin/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestAdminPlanHandler(t *testing.T) {
	suite.Run(t, new(AdminPlanHandlerTestSuite))
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

	body := map[string]any{
		"name":   "Updated Protocol",
		"enable": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/protocols/"+strconv.FormatUint(uint64(s.testProtocol.ID), 10), bytes.NewReader(jsonBody))
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

func (s *ProtocolHandlerTestSuite) TestUpdateProtocol_InvalidID() {
	handler := NewNodeHandler()
	s.router.PUT("/protocols/:protocol_id", handler.UpdateProtocol)

	body := map[string]any{"name": "Test"}
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "节点未连接 Agent Control，将保留旧版周期拉取同步", data["message"])
	assert.Equal(s.T(), "legacy-poll", data["transport"])
	assert.NotContains(s.T(), resp, "error")
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
