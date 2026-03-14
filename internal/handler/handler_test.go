package handler

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

	// 初始化缓存
	cache.InitMemory()

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
		&model.AuthorizedKey{},
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
	s.db.Exec("DELETE FROM v2_authorized_key")
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

// UserHandlerTestSuite 用户 Handler 测试套件
type UserHandlerTestSuite struct {
	HandlerTestSuite
	testUser *model.User
	testPlan *model.Plan
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