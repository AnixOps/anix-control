package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ========== Payment Gateway Extended Tests ==========

// ========== Forward User Rules Tests ==========

// ========== Telegram User Tests ==========

// ========== Admin Coupon Extended Tests ==========

// ========== Admin Knowledge Extended Tests ==========

// ========== Admin Ticket Extended Tests ==========

// ========== Payment Gateway Extended Tests ==========

type PaymentGatewayExtendedTestSuite2 struct {
	HandlerTestSuite
	testGateway *model.PaymentGateway
}

func (s *PaymentGatewayExtendedTestSuite2) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testGateway = &model.PaymentGateway{
		Name:    "Test Gateway 2",
		Type:    model.PaymentGatewayEPay,
		Enabled: true,
		Config:  `{}`,
	}
	s.db.Create(s.testGateway)

	s.router = gin.New()
}

func TestPaymentGatewayExtended2(t *testing.T) {
	suite.Run(t, new(PaymentGatewayExtendedTestSuite2))
}

// ========== Subscribe Handler Extended Tests ==========

type SubscribeExtendedTestSuite struct {
	HandlerTestSuite
	testUser  *model.User
	testPlan  *model.Plan
	testGroup *model.SubscriptionGroup
}

func (s *SubscribeExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "subscribe@example.com",
		Token:          "subscribe-token",
		UUID:           "subscribe-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.testPlan = &model.Plan{
		Name:           "Subscribe Test Plan",
		TransferEnable: 1073741824,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	s.testGroup = &model.SubscriptionGroup{
		Name: "Test Group",
	}
	s.db.Create(s.testGroup)

	s.router = gin.New()
}

func (s *SubscribeExtendedTestSuite) assertPanelError(w *httptest.ResponseRecorder, msgContains string) {
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

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", handler.PreviewSubscription)

	body := map[string]any{
		"user_id":   s.testUser.ID,
		"format":    "clash",
		"group_ids": []uint{s.testGroup.ID},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.NotEmpty(s.T(), data["content"])
	assert.NotEmpty(s.T(), data["content_type"])
	assert.NotEmpty(s.T(), data["filename"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription_UsesContextUserAndGroupIDs() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.PreviewSubscription(c)
	})

	body := map[string]any{
		"format":    "v2ray",
		"group_ids": []uint{s.testGroup.ID},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Contains(s.T(), data, "content")
	assert.NotEmpty(s.T(), data["content_type"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription_MissingUserContext() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", handler.PreviewSubscription)

	body := map[string]any{
		"format":    "v2ray",
		"group_ids": []uint{s.testGroup.ID},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "未登录")
}

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", handler.PreviewSubscription)

	req, _ := http.NewRequest("POST", "/admin/subscription/preview", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription_UserNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", handler.PreviewSubscription)

	body := map[string]any{
		"user_id": 99999,
		"format":  "v2ray",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/preview", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
}

func TestSubscribeExtended(t *testing.T) {
	suite.Run(t, new(SubscribeExtendedTestSuite))
}

// ========== Invite Handler Extended Tests ==========

// ========== Notification Handler Extended Tests ==========

// ========== MFA Handler Extended Tests ==========

type MFAExtendedTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *MFAExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "mfa@example.com",
		Token:          "mfa-token",
		UUID:           "mfa-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *MFAExtendedTestSuite) TestGetStatus() {
	handler := NewMFAHandler()
	s.router.GET("/user/mfa/status", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetStatus(c)
	})

	req, _ := http.NewRequest("GET", "/user/mfa/status", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), false, data["enabled"])
	assert.Equal(s.T(), false, data["has_backup_codes"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAExtendedTestSuite) TestSetupTOTP() {
	handler := NewMFAHandler()
	s.router.POST("/user/mfa/totp/setup", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.SetupTOTP(c)
	})

	req, _ := http.NewRequest("POST", "/user/mfa/totp/setup", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.NotEmpty(s.T(), data["secret"])
	assert.NotEmpty(s.T(), data["url"])
	assert.NotEmpty(s.T(), data["qr_code"])
	assert.NotEmpty(s.T(), data["backup_codes"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAExtendedTestSuite) TestGetAdminConfig() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/mfa/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Contains(s.T(), data, "enabled")
	assert.Contains(s.T(), data, "methods")
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAExtendedTestSuite) TestUpdateAdminConfig() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	body := map[string]any{
		"enabled": true,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/mfa/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), true, data["enabled"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *MFAExtendedTestSuite) TestAdminConfig_FrontendFields_RoundTrip() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	updateBody := map[string]any{
		"enabled":            true,
		"required":           true,
		"methods":            map[string]bool{"totp": true, "sms": true, "email": false},
		"backup_codes_count": 12,
		"max_attempts":       6,
		"lockout_duration":   30,
	}
	updateJSON, _ := json.Marshal(updateBody)
	putReq, _ := http.NewRequest("PUT", "/admin/mfa/config", bytes.NewReader(updateJSON))
	putReq.Header.Set("Content-Type", "application/json")
	putResp := httptest.NewRecorder()
	s.router.ServeHTTP(putResp, putReq)
	assert.Equal(s.T(), http.StatusOK, putResp.Code)

	getReq, _ := http.NewRequest("GET", "/admin/mfa/config", nil)
	getResp := httptest.NewRecorder()
	s.router.ServeHTTP(getResp, getReq)
	assert.Equal(s.T(), http.StatusOK, getResp.Code)

	resp := decodePanelTestResponse(s.T(), getResp)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)

	_, hasRequired := data["required"]
	_, hasMethods := data["methods"]
	_, hasBackupCodesCount := data["backup_codes_count"]
	_, hasMaxAttempts := data["max_attempts"]
	_, hasLockoutDuration := data["lockout_duration"]

	assert.True(s.T(), hasRequired)
	assert.True(s.T(), hasMethods)
	assert.True(s.T(), hasBackupCodesCount)
	assert.True(s.T(), hasMaxAttempts)
	assert.True(s.T(), hasLockoutDuration)

	assert.Equal(s.T(), true, data["enabled"])
	assert.Equal(s.T(), true, data["required"])
	assert.Equal(s.T(), float64(12), data["backup_codes_count"])
	assert.Equal(s.T(), float64(6), data["max_attempts"])
	assert.Equal(s.T(), float64(30), data["lockout_duration"])

	methods, ok := data["methods"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), true, methods["totp"])
	assert.Equal(s.T(), true, methods["sms"])
	assert.Equal(s.T(), false, methods["email"])

	var storedMFAConfig model.SystemConfig
	err := s.db.Where("key = ?", mfaAdminConfigKey).First(&storedMFAConfig).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), storedMFAConfig.Value, "\"backup_codes_count\":12")
	assert.Contains(s.T(), storedMFAConfig.Value, "\"max_attempts\":6")
	assert.Contains(s.T(), storedMFAConfig.Value, "\"lockout_duration\":30")
}

func (s *MFAExtendedTestSuite) TestUpdateAdminConfig_InvalidBody() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	req, _ := http.NewRequest("PUT", "/admin/mfa/config", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "invalid")
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func TestMFAExtended(t *testing.T) {
	suite.Run(t, new(MFAExtendedTestSuite))
}

// ========== Node Handler Extended Tests ==========

type NodeExtendedTestSuite struct {
	HandlerTestSuite
	testNode *model.Node
}

func (s *NodeExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testNode = &model.Node{
		Name:   "Extended Test Node",
		Host:   "127.0.0.1",
		Port:   443,
		Status: 1,
	}
	s.db.Create(s.testNode)

	s.router = gin.New()
}

func (s *NodeExtendedTestSuite) TestCreateNode() {
	handler := NewNodeHandler()
	s.router.POST("/admin/nodes", handler.CreateNode)

	body := map[string]any{
		"name":    "New Node",
		"host":    "192.168.1.1",
		"port":    443,
		"api_key": "new-api-key",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/nodes", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestCreateNode_InvalidBody() {
	handler := NewNodeHandler()
	s.router.POST("/admin/nodes", handler.CreateNode)

	req, _ := http.NewRequest("POST", "/admin/nodes", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeExtendedTestSuite) TestUpdateNode() {
	handler := NewNodeHandler()
	s.router.PUT("/admin/nodes/:id", handler.UpdateNode)

	body := map[string]any{
		"name": "Updated Node",
		"host": "192.168.1.2",
		"port": 8443,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestNodeExtended(t *testing.T) {
	suite.Run(t, new(NodeExtendedTestSuite))
}

// ========== Admin Extended Tests ==========

type AdminExtendedTestSuite struct {
	HandlerTestSuite
	testUser  *model.User
	testPlan  *model.Plan
	testOrder *model.Order
}

func (s *AdminExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testPlan = &model.Plan{
		Name:           "Test Plan Extended",
		TransferEnable: 10737418240,
		Show:           1,
	}
	s.db.Create(s.testPlan)

	s.testUser = &model.User{
		Email:          "admin-ext@example.com",
		Token:          "admin-ext-token",
		UUID:           "admin-ext-uuid",
		TransferEnable: 10737418240,
		PlanID:         &s.testPlan.ID,
	}
	s.db.Create(s.testUser)

	s.testOrder = &model.Order{
		TradeNo: "EXT-ORDER-001",
		UserID:  s.testUser.ID,
		PlanID:  s.testPlan.ID,
		Status:  0,
	}
	s.db.Create(s.testOrder)

	s.router = gin.New()
}

func (s *AdminExtendedTestSuite) TestGetUserStats() {
	handler := NewAdminHandler()
	s.router.GET("/admin/users/stats", handler.GetUserStats)

	req, _ := http.NewRequest("GET", "/admin/users/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateUser_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/users/:id", handler.UpdateUser)

	body := map[string]any{
		"balance":         1000,
		"transfer_enable": 21474836480,
		"remark_content":  "Test remark",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateUser_NotFound() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/users/:id", handler.UpdateUser)

	body := map[string]any{
		"balance": 1000,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/users/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "用户不存在")
}

func TestAdminExtended(t *testing.T) {
	suite.Run(t, new(AdminExtendedTestSuite))
}
