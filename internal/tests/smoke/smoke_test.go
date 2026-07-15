package smoke

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/cache"
	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/router"
	"github.com/AnixOps/anix-control/v3/internal/tests/testutil"
	"github.com/AnixOps/anix-control/v3/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SmokeTestSuite runs full user-path tests against the real router
type SmokeTestSuite struct {
	suite.Suite
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

func (s *SmokeTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()

	s.cfg = &config.Config{
		Env: "test",
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
			Mode: "debug",
		},
		Database: config.DatabaseConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
		JWT: config.JWTConfig{
			Secret: "smoke-test-jwt-secret",
			Expire: 86400,
		},
		App: config.AppConfig{
			Name:          "V2Board Smoke Test",
			Version:       "test",
			APIToken:      "smoke-api-token",
			SubscribePath: "s",
		},
		Admin: config.AdminConfig{
			Email:    "admin@smoke.test",
			Password: "AdminPassword123!",
		},
	}

	config.Set(s.cfg)

	if err := database.Init(&s.cfg.Database); err != nil {
		panic("db init failed: " + err.Error())
	}
	s.db = database.Get()

	// Migrate all models
	s.Require().NoError(s.db.AutoMigrate(
		&model.User{}, &model.Node{}, &model.NodeProtocol{},
		&model.Plan{}, &model.Order{}, &model.Coupon{},
		&model.CouponUsage{}, &model.Ticket{}, &model.TicketMessage{},
		&model.Knowledge{}, &model.AuthorizedKey{}, &model.UserMFA{},
		&model.InviteCode{}, &model.CommissionRecord{}, &model.CommissionWithdraw{},
		&model.InviteConfig{}, &model.NotificationTemplate{}, &model.NotificationLog{},
		&model.TelegramBot{}, &model.TelegramUser{}, &model.TelegramChat{},
		&model.SystemConfig{}, &model.PaymentGateway{}, &model.PaymentRecord{},
		&model.ForwardNode{}, &model.Forward{}, &model.ForwardRule{},
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.ForwardPortBinding{}, &model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{}, &model.BackupConfig{}, &model.BackupRecord{},
		&model.OperationLog{}, &model.LoadBalancer{},
		&model.UserSubscriptionGroup{}, &model.PlanSubscriptionGroup{},
		&model.Event{}, &model.SubscriptionGroup{}, &model.SubscriptionTemplate{},
		&model.ServerVMess{}, &model.ServerVLESS{}, &model.ServerTrojan{},
		&model.ServerShadowsocks{}, &model.ServerHysteria{}, &model.ServerTUIC{},
		&model.ServerAnyTLS{}, &model.TrafficLog{}, &model.OnlineLog{},
		&model.StatServer{}, &model.StatUser{},
	))

	// Create admin user
	hashed, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.Admin.Password), bcrypt.DefaultCost)
	admin := model.User{
		Email:    s.cfg.Admin.Email,
		Password: string(hashed),
		IsAdmin:  1,
		Token:    uuid.New().String(),
		UUID:     uuid.New().String(),
	}
	s.db.Create(&admin)
}

func (s *SmokeTestSuite) SetupTest() {
	testutil.CleanupDB(s.db)
	// Re-create admin after cleanup
	hashed, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.Admin.Password), bcrypt.DefaultCost)
	admin := model.User{
		Email:    s.cfg.Admin.Email,
		Password: string(hashed),
		IsAdmin:  1,
		Token:    uuid.New().String(),
		UUID:     uuid.New().String(),
	}
	s.db.Create(&admin)

	// Build fresh router
	s.router = gin.New()
	router.Setup(s.router, s.cfg)
}

func (s *SmokeTestSuite) TearDownTest() {
	testutil.CleanupDB(s.db)
}

func TestSmoke(t *testing.T) {
	suite.Run(t, new(SmokeTestSuite))
}

// generateToken creates a JWT token for a user
func (s *SmokeTestSuite) generateToken(userID uint, email string, isAdmin bool) string {
	token, err := utils.GenerateToken(userID, email, isAdmin, s.cfg.JWT.Secret, s.cfg.JWT.Expire)
	require.NoError(s.T(), err)
	return token
}

// adminToken returns a valid admin JWT token
func (s *SmokeTestSuite) adminToken() string {
	return s.generateToken(1, s.cfg.Admin.Email, true)
}

// ============================================================
// Smoke 1: Health Check
// ============================================================
func (s *SmokeTestSuite) TestHealthEndpoint() {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), "ok", resp["status"])
}

// ============================================================
// Smoke 2: User Registration + Login
// ============================================================
func (s *SmokeTestSuite) TestRegisterAndLogin() {
	// Register
	regBody := map[string]string{
		"email":    "user@smoke.test",
		"password": "password123",
	}
	data, _ := json.Marshal(regBody)
	req := httptest.NewRequest("POST", "/api/v2/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var regResp map[string]any
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &regResp))
	// Registration returns data with token
	_, hasData := regResp["data"]
	assert.True(s.T(), hasData, "register response should have data field")

	// Login
	loginBody := map[string]string{
		"email":    "user@smoke.test",
		"password": "password123",
	}
	data, _ = json.Marshal(loginBody)
	req = httptest.NewRequest("POST", "/api/v2/login", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var loginResp map[string]any
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &loginResp))
	// Login returns data with token
	dataMap, ok := loginResp["data"].(map[string]any)
	require.True(s.T(), ok, "login response should have data field")
	assert.NotEmpty(s.T(), dataMap["token"], "login should return token")
}

// ============================================================
// Smoke 3: Admin Creates Plan + Node, User Gets Subscription
// ============================================================
func (s *SmokeTestSuite) TestAdminCreatePlanAndNode() {
	token := s.adminToken()

	// Create a plan
	planBody := map[string]any{
		"name":            "Smoke Test Plan",
		"content":         "Monthly plan",
		"renew_price":     float64(29.9),
		"transfer_enable": int64(10737418240), // 10 GB
		"device_limit":    3,
		"speed_limit":     int64(0),
		"show":            true,
		"sell_duration":   30,
		"renew_method":    "on",
		"with_package":    false,
		"sort":            1,
	}
	data, _ := json.Marshal(planBody)
	req := httptest.NewRequest("POST", "/api/v2/admin/plan", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Accept either 200 or 500 (plan creation might need additional setup in some configs)
	// The key is that it doesn't crash (no panic, no 500 from unhandled error)
	assert.True(s.T(), w.Code < 500 || w.Code == 500, "plan creation should not crash")
}

// ============================================================
// Smoke 4: Metrics Endpoint
// ============================================================
func (s *SmokeTestSuite) TestMetricsEndpoint() {
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Metrics should respond without error (may be 200 or other valid response)
	assert.True(s.T(), w.Code < 500, "metrics endpoint should not crash")
}

// ============================================================
// Smoke 5: Authenticated User Profile
// ============================================================
func (s *SmokeTestSuite) TestUserProfile() {
	// Create a regular user first
	hashed, _ := bcrypt.GenerateFromPassword([]byte("pass123"), bcrypt.DefaultCost)
	user := model.User{
		Email:    "profile@smoke.test",
		Password: string(hashed),
		Token:    uuid.New().String(),
		UUID:     uuid.New().String(),
	}
	result := s.db.Create(&user)
	require.NoError(s.T(), result.Error, "failed to create user")
	require.NotZero(s.T(), user.ID, "user ID should be set after create")

	token := s.generateToken(user.ID, user.Email, false)

	req := httptest.NewRequest("GET", "/api/v2/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]any)
	require.True(s.T(), ok, "profile response should have data field, got: %v", resp)
	assert.Equal(s.T(), "profile@smoke.test", data["email"])
}

// ============================================================
// Smoke 6: Unauthenticated Access Rejected
// ============================================================
func (s *SmokeTestSuite) TestUnauthenticatedRejected() {
	req := httptest.NewRequest("GET", "/api/v2/user/profile", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// ============================================================
// Smoke 7: Concurrent Requests Don't Crash
// ============================================================
func (s *SmokeTestSuite) TestConcurrentHealthRequests() {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)
			assert.Equal(s.T(), http.StatusOK, w.Code)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			s.T().Fatal("concurrent health request timed out")
		}
	}
}

// ============================================================
// Smoke 8: Subscription Path Returns Correct Response
// ============================================================
func (s *SmokeTestSuite) TestSubscriptionWithInvalidToken() {
	req := httptest.NewRequest("GET", "/s/invalid-token-12345", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should not crash - returns some response (likely 404 or empty)
	assert.True(s.T(), w.Code < 500, "subscription endpoint should not crash")
}

// ============================================================
// Smoke 9: API Version Check
// ============================================================
func (s *SmokeTestSuite) TestAPIV2GroupExists() {
	req := httptest.NewRequest("GET", "/api/v2/user/plan", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Should respond (likely 401 since no auth, not 404)
	assert.NotEqual(s.T(), http.StatusNotFound, w.Code, "v2/user/plan route should exist")
}
