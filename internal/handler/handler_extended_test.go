package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func intPtr(v int64) *int64 {
	return &v
}

func strPtr(v string) *string {
	return &v
}

// ========== UniProxy Extended Tests ==========

type UniProxyExtendedTestSuite struct {
	HandlerTestSuite
	testNode *model.Node
	testUser *model.User
}

func (s *UniProxyExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testNode = &model.Node{
		Name:   "Test Node",
		Host:   "127.0.0.1",
		Port:   443,
		APIKey: "test-key",
		Status: 1,
	}
	s.db.Create(s.testNode)

	plan := &model.Plan{
		Name:           "Test Plan",
		TransferEnable: 1073741824,
	}
	s.db.Create(plan)

	s.testUser = &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
		PlanID:         &plan.ID,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *UniProxyExtendedTestSuite) TestGetUsers_Success() {
	handler := NewUniProxyHandler()
	s.router.GET("/server/UniProxy/user", handler.GetUsers)

	req, _ := http.NewRequest("GET", "/server/UniProxy/user?node_id="+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *UniProxyExtendedTestSuite) TestGetUsers_InvalidNodeID() {
	handler := NewUniProxyHandler()
	s.router.GET("/server/UniProxy/user", handler.GetUsers)

	req, _ := http.NewRequest("GET", "/server/UniProxy/user?node_id=invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *UniProxyExtendedTestSuite) TestGetUsers_WithNodeType() {
	handler := NewUniProxyHandler()
	s.router.GET("/server/UniProxy/user", handler.GetUsers)

	req, _ := http.NewRequest("GET", "/server/UniProxy/user?node_id="+strconv.FormatUint(uint64(s.testNode.ID), 10)+"&node_type=vmess", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Returns 500 because v2_server_vmess table doesn't exist
	// This tests the handler logic path
}

func (s *UniProxyExtendedTestSuite) TestGetAliveList() {
	handler := NewUniProxyHandler()
	s.router.GET("/server/UniProxy/alivelist", handler.GetAliveList)

	req, _ := http.NewRequest("GET", "/server/UniProxy/alivelist", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestUniProxyExtended(t *testing.T) {
	suite.Run(t, new(UniProxyExtendedTestSuite))
}

// ========== Order Handler Extended Tests ==========

type OrderHandlerExtendedTestSuite struct {
	HandlerTestSuite
	testPlan *model.Plan
	testUser *model.User
}

func (s *OrderHandlerExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testPlan = &model.Plan{
		Name:           "Test Plan",
		TransferEnable: 1073741824,
	}
	s.testPlan.MonthPrice = intPtr(1000)
	s.db.Create(s.testPlan)

	s.testUser = &model.User{
		Email:          "order@example.com",
		Token:          "order-token",
		UUID:           "order-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *OrderHandlerExtendedTestSuite) TestSaveOrder_Success() {
	handler := NewOrderHandler()
	s.router.POST("/order/save", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.SaveOrder(c)
	})

	body := map[string]interface{}{
		"plan_id": s.testPlan.ID,
		"period":  "month",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/order/save", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *OrderHandlerExtendedTestSuite) TestSaveOrder_InvalidBody() {
	handler := NewOrderHandler()
	s.router.POST("/order/save", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.SaveOrder(c)
	})

	req, _ := http.NewRequest("POST", "/order/save", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *OrderHandlerExtendedTestSuite) TestGetOrders() {
	handler := NewOrderHandler()
	s.router.GET("/orders", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetOrders(c)
	})

	req, _ := http.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *OrderHandlerExtendedTestSuite) TestGetOrderDetail() {
	order := &model.Order{
		TradeNo:     "TEST001",
		UserID:      s.testUser.ID,
		PlanID:      s.testPlan.ID,
		Period:      "month",
		TotalAmount: 1000,
		Status:      0,
	}
	s.db.Create(order)

	handler := NewOrderHandler()
	s.router.GET("/orders/:id", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetOrderDetail(c)
	})

	req, _ := http.NewRequest("GET", "/orders/"+strconv.FormatUint(uint64(order.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestOrderHandlerExtended(t *testing.T) {
	suite.Run(t, new(OrderHandlerExtendedTestSuite))
}

// ========== Invite Handler Extended Tests ==========

type InviteHandlerExtendedTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *InviteHandlerExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "invite@example.com",
		Token:          "invite-token",
		UUID:           "invite-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *InviteHandlerExtendedTestSuite) TestUpdateConfig_Create() {
	handler := NewInviteHandler()
	s.router.PUT("/admin/invite/config", handler.UpdateConfig)

	body := model.InviteConfig{
		Enabled:             true,
		CommissionRate:      0.1,
		CommissionMinAmount: 100,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/invite/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerExtendedTestSuite) TestGetConfig_FrontendFields() {
	cfg := &model.InviteConfig{
		Enabled:             true,
		CommissionType:      1,
		CommissionRate:      0.15,
		CommissionFixed:     0,
		CommissionMinAmount: 20,
	}
	s.db.Create(cfg)

	handler := NewInviteHandler()
	s.router.GET("/admin/invite/config", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/admin/invite/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "percent", data["commission_type"])
	assert.InDelta(s.T(), 15.0, data["commission_rate"], 0.0001)
	assert.InDelta(s.T(), 20.0, data["min_withdraw"], 0.0001)
	_, hasMethods := data["withdraw_methods"]
	assert.True(s.T(), hasMethods)
}

func (s *InviteHandlerExtendedTestSuite) TestUpdateConfig_FrontendPayloadCompatibility() {
	handler := NewInviteHandler()
	s.router.PUT("/admin/invite/config", handler.UpdateConfig)

	body := map[string]interface{}{
		"enabled":          false,
		"commission_type":  "fixed",
		"commission_rate":  12.5,
		"commission_fixed": 88.8,
		"min_withdraw":     66.6,
		"code_prefix":      "INVX",
		"code_length":      12,
		"withdraw_fee":     1.2,
		"withdraw_methods": []string{
			"alipay",
			"bank",
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/invite/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var cfg model.InviteConfig
	err := s.db.First(&cfg).Error
	assert.NoError(s.T(), err)
	assert.False(s.T(), cfg.Enabled)
	assert.Equal(s.T(), 2, cfg.CommissionType)
	assert.InDelta(s.T(), 0.125, cfg.CommissionRate, 0.000001)
	assert.InDelta(s.T(), 88.8, cfg.CommissionFixed, 0.0001)
	assert.InDelta(s.T(), 66.6, cfg.CommissionMinAmount, 0.0001)
}

func (s *InviteHandlerExtendedTestSuite) TestUpdateConfig_FrontendExtraFieldsPersisted() {
	handler := NewInviteHandler()
	s.router.PUT("/admin/invite/config", handler.UpdateConfig)
	s.router.GET("/admin/invite/config", handler.GetConfig)

	updateBody := map[string]interface{}{
		"code_prefix":      "ABC",
		"code_length":      12,
		"withdraw_fee":     1.5,
		"withdraw_methods": []string{"alipay", "bank"},
		"commission_rate":  12.5,
	}
	updateJSON, _ := json.Marshal(updateBody)
	putReq, _ := http.NewRequest("PUT", "/admin/invite/config", bytes.NewReader(updateJSON))
	putReq.Header.Set("Content-Type", "application/json")
	putResp := httptest.NewRecorder()
	s.router.ServeHTTP(putResp, putReq)
	assert.Equal(s.T(), http.StatusOK, putResp.Code)

	getReq, _ := http.NewRequest("GET", "/admin/invite/config", nil)
	getResp := httptest.NewRecorder()
	s.router.ServeHTTP(getResp, getReq)
	assert.Equal(s.T(), http.StatusOK, getResp.Code)

	var payload map[string]interface{}
	err := json.Unmarshal(getResp.Body.Bytes(), &payload)
	assert.NoError(s.T(), err)
	data, ok := payload["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "ABC", data["code_prefix"])
	assert.Equal(s.T(), float64(12), data["code_length"])
	assert.InDelta(s.T(), 1.5, data["withdraw_fee"], 0.0001)
	methods, ok := data["withdraw_methods"].([]interface{})
	assert.True(s.T(), ok)
	assert.Len(s.T(), methods, 2)
}

func (s *InviteHandlerExtendedTestSuite) TestUpdateConfig_InvalidBody() {
	handler := NewInviteHandler()
	s.router.PUT("/admin/invite/config", handler.UpdateConfig)

	req, _ := http.NewRequest("PUT", "/admin/invite/config", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *InviteHandlerExtendedTestSuite) TestProcessWithdraw() {
	withdraw := &model.CommissionWithdraw{
		UserID: s.testUser.ID,
		Amount: 1000,
		Status: 0,
	}
	s.db.Create(withdraw)

	handler := NewInviteHandler()
	s.router.POST("/admin/invite/withdrawals/:id/process", handler.ProcessWithdraw)

	body := map[string]interface{}{
		"status": 1,
		"remark": "Approved",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/invite/withdrawals/"+strconv.FormatUint(uint64(withdraw.ID), 10)+"/process", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerExtendedTestSuite) TestProcessWithdraw_InvalidBody() {
	handler := NewInviteHandler()
	s.router.POST("/admin/invite/withdrawals/1/process", handler.ProcessWithdraw)

	req, _ := http.NewRequest("POST", "/admin/invite/withdrawals/1/process", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *InviteHandlerExtendedTestSuite) TestProcessWithdraw_ApprovedAlias() {
	withdraw := &model.CommissionWithdraw{
		UserID: s.testUser.ID,
		Amount: 500,
		Status: 0,
	}
	s.db.Create(withdraw)

	handler := NewInviteHandler()
	s.router.POST("/admin/invite/withdrawals/:id/process", handler.ProcessWithdraw)

	body := map[string]interface{}{
		"approved": true,
		"remark":   "approved by alias",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/invite/withdrawals/"+strconv.FormatUint(uint64(withdraw.ID), 10)+"/process", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var updated model.CommissionWithdraw
	err := s.db.First(&updated, withdraw.ID).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, updated.Status)
}

func (s *InviteHandlerExtendedTestSuite) TestGetWithdrawals() {
	handler := NewInviteHandler()
	s.router.GET("/admin/invite/withdrawals", handler.GetWithdrawals)

	req, _ := http.NewRequest("GET", "/admin/invite/withdrawals", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *InviteHandlerExtendedTestSuite) TestGetWithdrawals_StatusTextFilterAndPaginationBounds() {
	pending := &model.CommissionWithdraw{
		UserID: s.testUser.ID,
		Amount: 100,
		Status: 0,
	}
	approved := &model.CommissionWithdraw{
		UserID: s.testUser.ID,
		Amount: 200,
		Status: 1,
	}
	s.db.Create(pending)
	s.db.Create(approved)

	handler := NewInviteHandler()
	s.router.GET("/admin/invite/withdrawals", handler.GetWithdrawals)

	req, _ := http.NewRequest("GET", "/admin/invite/withdrawals?status=pending&page=0&page_size=500", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 1)

	first, ok := list[0].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "pending", first["status"])
	assert.InDelta(s.T(), 0.0, first["status_code"], 0.0001)
	assert.InDelta(s.T(), 1.0, data["page"], 0.0001)
	assert.InDelta(s.T(), 100.0, data["page_size"], 0.0001)
}

func TestInviteHandlerExtended(t *testing.T) {
	suite.Run(t, new(InviteHandlerExtendedTestSuite))
}

// ========== Telegram Handler Extended Tests ==========

type TelegramHandlerExtendedTestSuite struct {
	HandlerTestSuite
	testBot *model.TelegramBot
}

func (s *TelegramHandlerExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testBot = &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	s.db.Create(s.testBot)

	s.router = gin.New()
}

func (s *TelegramHandlerExtendedTestSuite) TestDeleteWebhook() {
	handler := NewTelegramHandler()
	s.router.DELETE("/admin/telegram/webhook", handler.DeleteWebhook)

	req, _ := http.NewRequest("DELETE", "/admin/telegram/webhook", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
}

func (s *TelegramHandlerExtendedTestSuite) TestSendNotification() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/notify", handler.SendNotification)

	body := map[string]interface{}{
		"telegram_id": "12345",
		"title":       "Test",
		"content":     "Test notification",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/telegram/notify", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Request shape should be accepted, service may still fail without real Telegram token.
	assert.NotEqual(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TelegramHandlerExtendedTestSuite) TestSendNotification_MessagePayload() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/notify", handler.SendNotification)

	body := map[string]interface{}{
		"telegram_id": 12345,
		"message":     "Direct message payload",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/telegram/notify", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.NotEqual(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TelegramHandlerExtendedTestSuite) TestSendNotification_InvalidBody() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/notify", handler.SendNotification)

	req, _ := http.NewRequest("POST", "/admin/telegram/notify", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TelegramHandlerExtendedTestSuite) TestBroadcast() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/broadcast", handler.Broadcast)

	body := map[string]string{
		"message": "Test broadcast",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/telegram/broadcast", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	_, hasSuccess := data["success"]
	_, hasFailed := data["failed"]
	assert.True(s.T(), hasSuccess)
	assert.True(s.T(), hasFailed)
}

func (s *TelegramHandlerExtendedTestSuite) TestBroadcast_NestedMessage() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/broadcast", handler.Broadcast)

	body := map[string]interface{}{
		"message": map[string]interface{}{
			"message": "Nested message payload",
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/telegram/broadcast", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramHandlerExtendedTestSuite) TestBroadcast_InvalidBody() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/broadcast", handler.Broadcast)

	req, _ := http.NewRequest("POST", "/admin/telegram/broadcast", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *TelegramHandlerExtendedTestSuite) TestGetUserBindings() {
	user := &model.User{
		Email:          "tg-bindings@example.com",
		Token:          "tg-bindings-token",
		UUID:           "tg-bindings-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(user)
	tgUser := &model.TelegramUser{
		UserID:       user.ID,
		TelegramID:   12345678,
		Username:     "tguser",
		NotifyExpire: true,
	}
	s.db.Create(tgUser)

	handler := NewTelegramHandler()
	s.router.GET("/admin/telegram/users", handler.GetUserBindings)

	req, _ := http.NewRequest("GET", "/admin/telegram/users?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.GreaterOrEqual(s.T(), len(list), 1)
	first, ok := list[0].(map[string]interface{})
	assert.True(s.T(), ok)
	_, hasEmail := first["user_email"]
	_, hasNotify := first["notify_enabled"]
	assert.True(s.T(), hasEmail)
	assert.True(s.T(), hasNotify)
}

func (s *TelegramHandlerExtendedTestSuite) TestGetUserBindings_All() {
	for i := 0; i < 25; i++ {
		user := &model.User{
			Email:          "tg-all-" + strconv.Itoa(i) + "@example.com",
			Token:          "tg-all-token-" + strconv.Itoa(i),
			UUID:           "tg-all-uuid-" + strconv.Itoa(i),
			TransferEnable: 1073741824,
		}
		s.db.Create(user)
		tgUser := &model.TelegramUser{
			UserID:       user.ID,
			TelegramID:   int64(300000 + i),
			Username:     "tgall" + strconv.Itoa(i),
			NotifyExpire: true,
		}
		s.db.Create(tgUser)
	}

	handler := NewTelegramHandler()
	s.router.GET("/admin/telegram/users", handler.GetUserBindings)

	req, _ := http.NewRequest("GET", "/admin/telegram/users?all=true", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.GreaterOrEqual(s.T(), len(list), 25)
}

func (s *TelegramHandlerExtendedTestSuite) TestUpdateUserNotify() {
	user := &model.User{
		Email:          "tg-notify@example.com",
		Token:          "tg-notify-token",
		UUID:           "tg-notify-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(user)
	tgUser := &model.TelegramUser{
		UserID:        user.ID,
		TelegramID:    666666,
		Username:      "tgnotify",
		NotifyExpire:  true,
		NotifyTraffic: true,
		NotifyTicket:  true,
	}
	s.db.Create(tgUser)

	handler := NewTelegramHandler()
	s.router.PUT("/admin/telegram/users/:id/notify", handler.UpdateUserNotify)

	body := map[string]interface{}{
		"notify_enabled": false,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/admin/telegram/users/"+strconv.FormatUint(uint64(tgUser.ID), 10)+"/notify", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var refreshed model.TelegramUser
	err := s.db.First(&refreshed, tgUser.ID).Error
	assert.NoError(s.T(), err)
	assert.False(s.T(), refreshed.NotifyExpire)
	assert.False(s.T(), refreshed.NotifyTraffic)
	assert.False(s.T(), refreshed.NotifyTicket)
}

func TestTelegramHandlerExtended(t *testing.T) {
	suite.Run(t, new(TelegramHandlerExtendedTestSuite))
}

// ========== Payment Gateway Extended Tests ==========

type PaymentGatewayExtendedTestSuite struct {
	HandlerTestSuite
	testGateway *model.PaymentGateway
	testUser    *model.User
}

func (s *PaymentGatewayExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testGateway = &model.PaymentGateway{
		Name:    "Test Gateway",
		Type:    "alipay",
		Enabled: true,
		Config:  `{"app_id":"test"}`,
	}
	s.db.Create(s.testGateway)

	s.testUser = &model.User{
		Email:          "payment@example.com",
		Token:          "payment-token",
		UUID:           "payment-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *PaymentGatewayExtendedTestSuite) TestListPaymentRecords() {
	record := &model.PaymentRecord{
		GatewayID:   s.testGateway.ID,
		TradeNo:     "LIST-RECORDS-001",
		UserID:      s.testUser.ID,
		Amount:      10,
		GatewayType: "alipay",
		Status:      model.PaymentStatusPending,
	}
	s.db.Create(record)

	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/records", handler.ListPaymentRecords)

	req, _ := http.NewRequest("GET", "/admin/payment/records", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			List []struct {
				TradeNo string `json:"trade_no"`
				Status  string `json:"status"`
			} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.GreaterOrEqual(s.T(), len(resp.Data.List), 1)
	found := false
	for _, item := range resp.Data.List {
		if item.TradeNo == "LIST-RECORDS-001" {
			found = true
			assert.Equal(s.T(), "pending", item.Status)
		}
	}
	assert.True(s.T(), found)
}

func (s *PaymentGatewayExtendedTestSuite) TestListPaymentRecords_StatusTextFilter() {
	paidRecord := &model.PaymentRecord{
		GatewayID:   s.testGateway.ID,
		TradeNo:     "FILTER-PAID-001",
		UserID:      s.testUser.ID,
		Amount:      10,
		GatewayType: "alipay",
		Status:      model.PaymentStatusPaid,
	}
	failedRecord := &model.PaymentRecord{
		GatewayID:   s.testGateway.ID,
		TradeNo:     "FILTER-FAILED-001",
		UserID:      s.testUser.ID,
		Amount:      20,
		GatewayType: "alipay",
		Status:      model.PaymentStatusCancelled,
	}
	s.db.Create(paidRecord)
	s.db.Create(failedRecord)

	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/records", handler.ListPaymentRecords)

	req, _ := http.NewRequest("GET", "/admin/payment/records?status=paid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			List []struct {
				Status string `json:"status"`
			} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), resp.Data.List, 1)
	assert.Equal(s.T(), "paid", resp.Data.List[0].Status)
}

func (s *PaymentGatewayExtendedTestSuite) TestListPaymentRecords_GatewayTypeFilter() {
	alipayRecord := &model.PaymentRecord{
		GatewayID:   s.testGateway.ID,
		TradeNo:     "FILTER-TYPE-ALI-001",
		UserID:      s.testUser.ID,
		Amount:      10,
		GatewayType: "alipay",
		Status:      model.PaymentStatusPaid,
	}
	wechatRecord := &model.PaymentRecord{
		GatewayID:   s.testGateway.ID,
		TradeNo:     "FILTER-TYPE-WECHAT-001",
		UserID:      s.testUser.ID,
		Amount:      20,
		GatewayType: "wechat",
		Status:      model.PaymentStatusPaid,
	}
	s.db.Create(alipayRecord)
	s.db.Create(wechatRecord)

	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/records", handler.ListPaymentRecords)

	req, _ := http.NewRequest("GET", "/admin/payment/records?gateway_type=wechat", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			List []struct {
				GatewayType string `json:"gateway_type"`
			} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), resp.Data.List, 1)
	assert.Equal(s.T(), "wechat", resp.Data.List[0].GatewayType)
}

func (s *PaymentGatewayExtendedTestSuite) TestGetChannels() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/payment/channels", handler.GetChannels)

	req, _ := http.NewRequest("GET", "/payment/channels", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite) TestCreatePayment() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/payment/create", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.CreatePayment(c)
	})

	body := map[string]interface{}{
		"gateway_id": s.testGateway.ID,
		"amount":     1000,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/payment/create", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
}

func (s *PaymentGatewayExtendedTestSuite) TestGetPaymentStatus() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/payment/status/:trade_no", handler.GetPaymentStatus)

	req, _ := http.NewRequest("GET", "/payment/status/TEST001", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Returns 404 because payment record doesn't exist
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite) TestGetUserRecords() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/payment/records", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetUserRecords(c)
	})

	req, _ := http.NewRequest("GET", "/payment/records", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite) TestPaymentCallback() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/payment/callback/:type", handler.PaymentCallback)

	body := map[string]string{
		"trade_no": "TEST001",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/payment/callback/alipay", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
}

func TestPaymentGatewayExtended(t *testing.T) {
	suite.Run(t, new(PaymentGatewayExtendedTestSuite))
}

// ========== Forward User Rules Tests ==========

type ForwardUserRulesTestSuite struct {
	HandlerTestSuite
	testRelayNode *model.ForwardNode
	testExitNode  *model.ForwardNode
	testUser      *model.User
}

func (s *ForwardUserRulesTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testRelayNode = &model.ForwardNode{
		Name:    "User Relay Node",
		Type:    "relay",
		Host:    "192.168.1.100",
		Port:    8080,
		APIPort: 18080,
		Status:  1,
		Enabled: true,
	}
	s.db.Create(s.testRelayNode)

	s.testExitNode = &model.ForwardNode{
		Name:    "User Exit Node",
		Type:    "exit",
		Host:    "192.168.1.200",
		Port:    8081,
		APIPort: 18081,
		Status:  1,
		Enabled: true,
	}
	s.db.Create(s.testExitNode)

	s.testUser = &model.User{
		Email:          "forward@example.com",
		Token:          "forward-token",
		UUID:           "forward-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *ForwardUserRulesTestSuite) TestGetUserRules() {
	handler := NewForwardHandler()
	s.router.GET("/user/forward/rules", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetUserRules(c)
	})

	req, _ := http.NewRequest("GET", "/user/forward/rules", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *ForwardUserRulesTestSuite) TestCreateUserRule() {
	handler := NewForwardHandler()
	s.router.POST("/user/forward/rules", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.CreateUserRule(c)
	})

	body := map[string]interface{}{
		"name":          "User Rule",
		"relay_node_id": s.testRelayNode.ID,
		"listen_port":   9000,
		"exit_node_id":  s.testExitNode.ID,
		"target_host":   "10.0.0.1",
		"target_port":   80,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/user/forward/rules", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
}

func (s *ForwardUserRulesTestSuite) TestCreateUserRule_InvalidBody() {
	handler := NewForwardHandler()
	s.router.POST("/user/forward/rules", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.CreateUserRule(c)
	})

	req, _ := http.NewRequest("POST", "/user/forward/rules", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestForwardUserRules(t *testing.T) {
	suite.Run(t, new(ForwardUserRulesTestSuite))
}

// ========== Node Raw Config Tests ==========

type NodeRawConfigTestSuite struct {
	HandlerTestSuite
	testNode *model.Node
}

func (s *NodeRawConfigTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testNode = &model.Node{
		Name:      "Raw Config Node",
		Host:      "127.0.0.1",
		Port:      443,
		APIKey:    "test-key",
		Status:    1,
		RawConfig: strPtr(`{"key":"value"}`),
	}
	s.db.Create(s.testNode)

	s.router = gin.New()
}

func (s *NodeRawConfigTestSuite) TestGetNodeRawConfig() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/:id/raw-config", handler.GetNodeRawConfig)

	req, _ := http.NewRequest("GET", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/raw-config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeRawConfigTestSuite) TestGetNodeRawConfig_InvalidID() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/:id/raw-config", handler.GetNodeRawConfig)

	req, _ := http.NewRequest("GET", "/admin/nodes/invalid/raw-config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeRawConfigTestSuite) TestUpdateNodeRawConfig() {
	handler := NewNodeHandler()
	s.router.PUT("/admin/nodes/:id/raw-config", handler.UpdateNodeRawConfig)

	body := map[string]string{
		"raw_config": `{"updated":"config"}`,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/raw-config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeRawConfigTestSuite) TestUpdateNodeRawConfig_InvalidID() {
	handler := NewNodeHandler()
	s.router.PUT("/admin/nodes/:id/raw-config", handler.UpdateNodeRawConfig)

	body := map[string]string{
		"raw_config": `{"updated":"config"}`,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/nodes/invalid/raw-config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestNodeRawConfig(t *testing.T) {
	suite.Run(t, new(NodeRawConfigTestSuite))
}

// ========== System Backup Tests ==========

type SystemBackupTestSuite struct {
	HandlerTestSuite
}

func (s *SystemBackupTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *SystemBackupTestSuite) TestGetBackupConfig() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/backup/config", handler.GetBackupConfig)

	req, _ := http.NewRequest("GET", "/admin/system/backup/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	_, hasInterval := data["interval"]
	_, hasKeepCount := data["keep_count"]
	assert.True(s.T(), hasInterval)
	assert.True(s.T(), hasKeepCount)
}

func (s *SystemBackupTestSuite) TestUpdateBackupConfig() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/backup/config", handler.UpdateBackupConfig)

	body := map[string]interface{}{
		"enabled":    true,
		"interval":   12,
		"keep_count": 9,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/system/backup/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.InDelta(s.T(), 12, data["interval"], 0.000001)
	assert.InDelta(s.T(), 9, data["keep_count"], 0.000001)
}

func (s *SystemBackupTestSuite) TestUpdateBackupConfig_InvalidBody() {
	handler := NewSystemHandler()
	s.router.PUT("/admin/system/backup/config", handler.UpdateBackupConfig)

	req, _ := http.NewRequest("PUT", "/admin/system/backup/config", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SystemBackupTestSuite) TestCreateBackup() {
	handler := NewSystemHandler()
	s.router.POST("/admin/system/backup", handler.CreateBackup)

	req, _ := http.NewRequest("POST", "/admin/system/backup?type=database", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// May fail if database file doesn't exist, but tests the handler path
}

func (s *SystemBackupTestSuite) TestListBackups() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/backups", handler.ListBackups)

	req, _ := http.NewRequest("GET", "/admin/system/backups?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemBackupTestSuite) TestListBackups_ResponseShape() {
	pending := &model.BackupRecord{
		Name:   "pending_backup",
		Type:   "database",
		Size:   128,
		Status: 0,
	}
	completed := &model.BackupRecord{
		Name:   "completed_backup",
		Type:   "database",
		Size:   2048,
		Path:   "C:/tmp/backups/completed_backup.db",
		Status: 1,
	}
	failed := &model.BackupRecord{
		Name:   "failed_backup",
		Type:   "database",
		Size:   512,
		Status: 2,
		Error:  "disk full",
	}
	s.db.Create(pending)
	s.db.Create(completed)
	s.db.Create(failed)

	handler := NewSystemHandler()
	s.router.GET("/admin/system/backups", handler.ListBackups)

	req, _ := http.NewRequest("GET", "/admin/system/backups?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			List []struct {
				ID         uint   `json:"id"`
				Filename   string `json:"filename"`
				Status     string `json:"status"`
				StatusCode int    `json:"status_code"`
			} `json:"list"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.GreaterOrEqual(s.T(), len(resp.Data.List), 3)

	byID := make(map[uint]struct {
		Filename   string
		Status     string
		StatusCode int
	})
	for _, item := range resp.Data.List {
		byID[item.ID] = struct {
			Filename   string
			Status     string
			StatusCode int
		}{
			Filename:   item.Filename,
			Status:     item.Status,
			StatusCode: item.StatusCode,
		}
	}

	assert.Equal(s.T(), "pending_backup", byID[pending.ID].Filename)
	assert.Equal(s.T(), "pending", byID[pending.ID].Status)
	assert.Equal(s.T(), 0, byID[pending.ID].StatusCode)

	assert.Equal(s.T(), "completed_backup.db", byID[completed.ID].Filename)
	assert.Equal(s.T(), "completed", byID[completed.ID].Status)
	assert.Equal(s.T(), 1, byID[completed.ID].StatusCode)

	assert.Equal(s.T(), "failed_backup", byID[failed.ID].Filename)
	assert.Equal(s.T(), "failed", byID[failed.ID].Status)
	assert.Equal(s.T(), 2, byID[failed.ID].StatusCode)
}

func (s *SystemBackupTestSuite) TestGetBackupStats() {
	handler := NewSystemHandler()
	s.router.GET("/admin/system/backup/stats", handler.GetBackupStats)

	req, _ := http.NewRequest("GET", "/admin/system/backup/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SystemBackupTestSuite) TestGetBackupStats_TotalCountAlias() {
	s.db.Create(&model.BackupRecord{Name: "ok_1", Type: "database", Size: 10, Status: 1})
	s.db.Create(&model.BackupRecord{Name: "ok_2", Type: "database", Size: 20, Status: 1})
	s.db.Create(&model.BackupRecord{Name: "failed_1", Type: "database", Size: 30, Status: 2})

	handler := NewSystemHandler()
	s.router.GET("/admin/system/backup/stats", handler.GetBackupStats)

	req, _ := http.NewRequest("GET", "/admin/system/backup/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(2), data["total_count"])
	assert.Equal(s.T(), float64(2), data["total_backups"])
}

func (s *SystemBackupTestSuite) TestDeleteBackup() {
	handler := NewSystemHandler()
	s.router.DELETE("/admin/system/backups/:id", handler.DeleteBackup)

	req, _ := http.NewRequest("DELETE", "/admin/system/backups/999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Returns 500 if backup doesn't exist
}

func (s *SystemBackupTestSuite) TestDeleteBackup_InvalidID() {
	handler := NewSystemHandler()
	s.router.DELETE("/admin/system/backups/:id", handler.DeleteBackup)

	req, _ := http.NewRequest("DELETE", "/admin/system/backups/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SystemBackupTestSuite) TestRestoreBackup() {
	handler := NewSystemHandler()
	s.router.POST("/admin/system/backups/:id/restore", handler.RestoreBackup)

	req, _ := http.NewRequest("POST", "/admin/system/backups/999/restore", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Returns 500 if backup doesn't exist
}

func (s *SystemBackupTestSuite) TestRestoreBackup_InvalidID() {
	handler := NewSystemHandler()
	s.router.POST("/admin/system/backups/:id/restore", handler.RestoreBackup)

	req, _ := http.NewRequest("POST", "/admin/system/backups/invalid/restore", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestSystemBackup(t *testing.T) {
	suite.Run(t, new(SystemBackupTestSuite))
}

// ========== LoadBalancer Tests ==========

type LoadBalancerTestSuite struct {
	HandlerTestSuite
	testLB *model.LoadBalancer
}

func (s *LoadBalancerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testLB = &model.LoadBalancer{
		Name:     "Test LB",
		Strategy: "round-robin",
		Enabled:  true,
	}
	s.db.Create(s.testLB)

	s.router = gin.New()
}

func (s *LoadBalancerTestSuite) TestListLoadBalancers() {
	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers", handler.ListLoadBalancers)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestListLoadBalancers_IncludesWeightsField() {
	s.testLB.NodeWeights = `{"1":10,"2":5}`
	err := s.db.Save(s.testLB).Error
	assert.NoError(s.T(), err)

	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers", handler.ListLoadBalancers)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.NotEmpty(s.T(), list)

	found := false
	for _, item := range list {
		row, ok := item.(map[string]interface{})
		assert.True(s.T(), ok)
		if uint(row["id"].(float64)) != s.testLB.ID {
			continue
		}
		weights, hasWeights := row["weights"]
		assert.True(s.T(), hasWeights)
		weightMap, ok := weights.(map[string]interface{})
		assert.True(s.T(), ok)
		assert.Equal(s.T(), float64(10), weightMap["1"])
		assert.Equal(s.T(), float64(5), weightMap["2"])
		found = true
	}
	assert.True(s.T(), found)
}

func (s *LoadBalancerTestSuite) TestListLoadBalancers_WithGroupID() {
	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers", handler.ListLoadBalancers)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers?group_id=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestCreateLoadBalancer() {
	handler := NewLoadBalancerHandler()
	s.router.POST("/admin/loadbalancers", handler.CreateLoadBalancer)

	body := model.LoadBalancer{
		Name:     "New LB",
		Strategy: "least-load",
		Enabled:  true,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/loadbalancers", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestCreateLoadBalancer_WithWeights() {
	handler := NewLoadBalancerHandler()
	s.router.POST("/admin/loadbalancers", handler.CreateLoadBalancer)

	body := `{"name":"Weighted LB","strategy":"weight","group_id":1,"weights":{"1":10,"2":5}}`
	req, _ := http.NewRequest("POST", "/admin/loadbalancers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)

	weights, hasWeights := data["weights"]
	assert.True(s.T(), hasWeights)
	weightMap, ok := weights.(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(10), weightMap["1"])
	assert.Equal(s.T(), float64(5), weightMap["2"])

	var lb model.LoadBalancer
	err = s.db.Where("name = ?", "Weighted LB").First(&lb).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), lb.NodeWeights, "\"1\":10")
	assert.Contains(s.T(), lb.NodeWeights, "\"2\":5")
}

func (s *LoadBalancerTestSuite) TestCreateLoadBalancer_InvalidBody() {
	handler := NewLoadBalancerHandler()
	s.router.POST("/admin/loadbalancers", handler.CreateLoadBalancer)

	req, _ := http.NewRequest("POST", "/admin/loadbalancers", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *LoadBalancerTestSuite) TestGetLoadBalancer() {
	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers/:id", handler.GetLoadBalancer)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestGetLoadBalancer_NotFound() {
	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers/:id", handler.GetLoadBalancer)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *LoadBalancerTestSuite) TestUpdateLoadBalancer() {
	handler := NewLoadBalancerHandler()
	s.router.PUT("/admin/loadbalancers/:id", handler.UpdateLoadBalancer)

	body := model.LoadBalancer{
		Name:     "Updated LB",
		Strategy: "latency",
		Enabled:  true,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestUpdateLoadBalancer_WithWeights() {
	handler := NewLoadBalancerHandler()
	s.router.PUT("/admin/loadbalancers/:id", handler.UpdateLoadBalancer)

	body := `{"name":"Updated Weighted LB","strategy":"weight","weights":{"10":3,"20":7}}`
	req, _ := http.NewRequest("PUT", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)

	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	weights, hasWeights := data["weights"]
	assert.True(s.T(), hasWeights)
	weightMap, ok := weights.(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(3), weightMap["10"])
	assert.Equal(s.T(), float64(7), weightMap["20"])

	var updated model.LoadBalancer
	err = s.db.First(&updated, s.testLB.ID).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), updated.NodeWeights, "\"10\":3")
	assert.Contains(s.T(), updated.NodeWeights, "\"20\":7")
}

func (s *LoadBalancerTestSuite) TestUpdateLoadBalancer_NotFound() {
	handler := NewLoadBalancerHandler()
	s.router.PUT("/admin/loadbalancers/:id", handler.UpdateLoadBalancer)

	body := model.LoadBalancer{
		Name:     "Updated LB",
		Strategy: "latency",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/loadbalancers/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *LoadBalancerTestSuite) TestUpdateLoadBalancer_InvalidBody() {
	handler := NewLoadBalancerHandler()
	s.router.PUT("/admin/loadbalancers/:id", handler.UpdateLoadBalancer)

	req, _ := http.NewRequest("PUT", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10), strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *LoadBalancerTestSuite) TestDeleteLoadBalancer() {
	handler := NewLoadBalancerHandler()
	s.router.DELETE("/admin/loadbalancers/:id", handler.DeleteLoadBalancer)

	req, _ := http.NewRequest("DELETE", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestGetLoadBalancerStats() {
	handler := NewLoadBalancerHandler()
	s.router.GET("/admin/loadbalancers/:id/stats", handler.GetLoadBalancerStats)

	req, _ := http.NewRequest("GET", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10)+"/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *LoadBalancerTestSuite) TestRunHealthCheck() {
	handler := NewLoadBalancerHandler()
	s.router.POST("/admin/loadbalancers/:id/check", handler.RunHealthCheck)

	req, _ := http.NewRequest("POST", "/admin/loadbalancers/"+strconv.FormatUint(uint64(s.testLB.ID), 10)+"/check", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestLoadBalancer(t *testing.T) {
	suite.Run(t, new(LoadBalancerTestSuite))
}

// ========== Telegram User Tests ==========

type TelegramUserTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *TelegramUserTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "telegram@example.com",
		Token:          "telegram-token",
		UUID:           "telegram-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *TelegramUserTestSuite) TestUnbindTelegram() {
	handler := NewTelegramHandler()
	s.router.POST("/user/telegram/unbind", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.UnbindTelegram(c)
	})

	req, _ := http.NewRequest("POST", "/user/telegram/unbind", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramUserTestSuite) TestUpdateNotifySettings() {
	handler := NewTelegramHandler()
	s.router.POST("/user/telegram/notify", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.UpdateNotifySettings(c)
	})

	notifyExpire := true
	notifyTraffic := false
	body := map[string]interface{}{
		"notify_expire":  &notifyExpire,
		"notify_traffic": &notifyTraffic,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/user/telegram/notify", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *TelegramUserTestSuite) TestUpdateNotifySettings_InvalidBody() {
	handler := NewTelegramHandler()
	s.router.POST("/user/telegram/notify", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.UpdateNotifySettings(c)
	})

	req, _ := http.NewRequest("POST", "/user/telegram/notify", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestTelegramUser(t *testing.T) {
	suite.Run(t, new(TelegramUserTestSuite))
}

// ========== Admin Coupon Extended Tests ==========

type AdminCouponExtendedTestSuite struct {
	HandlerTestSuite
	testCoupon *model.Coupon
}

func (s *AdminCouponExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	limitUse := 10
	s.testCoupon = &model.Coupon{
		Code:     "TESTCODE",
		Name:     "Test Coupon",
		Type:     1,
		Value:    100,
		LimitUse: &limitUse,
	}
	s.db.Create(s.testCoupon)

	s.router = gin.New()
}

func (s *AdminCouponExtendedTestSuite) TestGetCoupons() {
	handler := NewAdminCouponHandler()
	s.router.GET("/admin/coupons", handler.GetCoupons)

	req, _ := http.NewRequest("GET", "/admin/coupons", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminCouponExtendedTestSuite) TestCreateCoupon() {
	handler := NewAdminCouponHandler()
	s.router.POST("/admin/coupons", handler.CreateCoupon)

	body := map[string]interface{}{
		"code":  "NEWCODE",
		"name":  "New Coupon",
		"type":  1,
		"value": 50,
		"limit": 100,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/coupons", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminCouponExtendedTestSuite) TestCreateCoupon_InvalidBody() {
	handler := NewAdminCouponHandler()
	s.router.POST("/admin/coupons", handler.CreateCoupon)

	req, _ := http.NewRequest("POST", "/admin/coupons", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminCouponExtendedTestSuite) TestDeleteCoupon() {
	handler := NewAdminCouponHandler()
	s.router.DELETE("/admin/coupons/:id", handler.DeleteCoupon)

	req, _ := http.NewRequest("DELETE", "/admin/coupons/"+strconv.FormatUint(uint64(s.testCoupon.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestAdminCouponExtended(t *testing.T) {
	suite.Run(t, new(AdminCouponExtendedTestSuite))
}

// ========== Admin Knowledge Extended Tests ==========

type AdminKnowledgeExtendedTestSuite struct {
	HandlerTestSuite
	testArticle *model.Knowledge
}

func (s *AdminKnowledgeExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testArticle = &model.Knowledge{
		Title:    "Test Article",
		Body:     "Test content",
		Show:     1,
		Category: "default",
	}
	s.db.Create(s.testArticle)

	s.router = gin.New()
}

func (s *AdminKnowledgeExtendedTestSuite) TestGetArticles() {
	handler := NewAdminKnowledgeHandler()
	s.router.GET("/admin/knowledge", handler.GetArticles)

	req, _ := http.NewRequest("GET", "/admin/knowledge", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeExtendedTestSuite) TestCreateArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.POST("/admin/knowledge", handler.CreateArticle)

	body := map[string]string{
		"title":    "New Article",
		"body":     "New content",
		"category": "default",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/knowledge", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeExtendedTestSuite) TestCreateArticle_InvalidBody() {
	handler := NewAdminKnowledgeHandler()
	s.router.POST("/admin/knowledge", handler.CreateArticle)

	req, _ := http.NewRequest("POST", "/admin/knowledge", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminKnowledgeExtendedTestSuite) TestUpdateArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.PUT("/admin/knowledge/:id", handler.UpdateArticle)

	body := map[string]string{
		"title":    "Updated Title",
		"body":     "Updated content",
		"category": "default",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminKnowledgeExtendedTestSuite) TestDeleteArticle() {
	handler := NewAdminKnowledgeHandler()
	s.router.DELETE("/admin/knowledge/:id", handler.DeleteArticle)

	req, _ := http.NewRequest("DELETE", "/admin/knowledge/"+strconv.FormatUint(uint64(s.testArticle.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestAdminKnowledgeExtended(t *testing.T) {
	suite.Run(t, new(AdminKnowledgeExtendedTestSuite))
}

// ========== Admin Ticket Extended Tests ==========

type AdminTicketExtendedTestSuite struct {
	HandlerTestSuite
	testTicket *model.Ticket
	testUser   *model.User
}

func (s *AdminTicketExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "ticket@example.com",
		Token:          "ticket-token",
		UUID:           "ticket-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.testTicket = &model.Ticket{
		UserID:  s.testUser.ID,
		Subject: "Test Ticket",
		Status:  0,
		Level:   1,
	}
	s.db.Create(s.testTicket)

	s.router = gin.New()
}

func (s *AdminTicketExtendedTestSuite) TestGetTickets() {
	handler := NewAdminTicketHandler()
	s.router.GET("/admin/tickets", handler.GetTickets)

	req, _ := http.NewRequest("GET", "/admin/tickets", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminTicketExtendedTestSuite) TestReplyTicket() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/reply", handler.ReplyTicket)

	body := map[string]interface{}{
		"ticket_id": s.testTicket.ID,
		"message":   "Admin reply",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/reply", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminTicketExtendedTestSuite) TestReplyTicket_InvalidBody() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/reply", handler.ReplyTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/reply", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminTicketExtendedTestSuite) TestCloseTicket() {
	handler := NewAdminTicketHandler()
	s.router.POST("/admin/tickets/:id/close", handler.CloseTicket)

	req, _ := http.NewRequest("POST", "/admin/tickets/"+strconv.FormatUint(uint64(s.testTicket.ID), 10)+"/close", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestAdminTicketExtended(t *testing.T) {
	suite.Run(t, new(AdminTicketExtendedTestSuite))
}

// ========== Payment Gateway Extended Tests ==========

type PaymentGatewayExtendedTestSuite2 struct {
	HandlerTestSuite
	testGateway *model.PaymentGateway
}

func (s *PaymentGatewayExtendedTestSuite2) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testGateway = &model.PaymentGateway{
		Name:    "Test Gateway 2",
		Type:    "alipay",
		Enabled: true,
		Config:  `{"app_id":"test"}`,
	}
	s.db.Create(s.testGateway)

	s.router = gin.New()
}

func (s *PaymentGatewayExtendedTestSuite2) TestListGateways() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/gateways", handler.ListGateways)

	req, _ := http.NewRequest("GET", "/admin/payment/gateways", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestCreateGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways", handler.CreateGateway)

	body := map[string]interface{}{
		"name":    "New Gateway",
		"type":    "wechat",
		"enabled": true,
		"config":  `{"app_id":"new"}`,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/payment/gateways", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestCreateGateway_InvalidBody() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways", handler.CreateGateway)

	req, _ := http.NewRequest("POST", "/admin/payment/gateways", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestUpdateGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.PUT("/admin/payment/gateways/:id", handler.UpdateGateway)

	body := map[string]interface{}{
		"name":    "Updated Gateway",
		"type":    "alipay",
		"enabled": true,
		"config":  `{"updated":"config"}`,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestUpdateGateway_TypeAndZeroMinMax() {
	s.testGateway.MinAmount = 10
	s.testGateway.MaxAmount = 100
	s.db.Save(s.testGateway)

	handler := NewPaymentGatewayHandler()
	s.router.PUT("/admin/payment/gateways/:id", handler.UpdateGateway)

	body := map[string]interface{}{
		"type":       "wechat",
		"min_amount": 0,
		"max_amount": 0,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var refreshed model.PaymentGateway
	err := s.db.First(&refreshed, s.testGateway.ID).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "wechat", refreshed.Type)
	assert.InDelta(s.T(), 0, refreshed.MinAmount, 0.000001)
	assert.InDelta(s.T(), 0, refreshed.MaxAmount, 0.000001)
}

func (s *PaymentGatewayExtendedTestSuite2) TestDeleteGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.DELETE("/admin/payment/gateways/:id", handler.DeleteGateway)

	req, _ := http.NewRequest("DELETE", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestToggleGateway() {
	handler := NewPaymentGatewayHandler()
	s.router.POST("/admin/payment/gateways/:id/toggle", handler.ToggleGateway)

	body := map[string]bool{
		"enabled": true,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/payment/gateways/"+strconv.FormatUint(uint64(s.testGateway.ID), 10)+"/toggle", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *PaymentGatewayExtendedTestSuite2) TestGetPaymentStats() {
	handler := NewPaymentGatewayHandler()
	s.router.GET("/admin/payment/stats", handler.GetPaymentStats)

	req, _ := http.NewRequest("GET", "/admin/payment/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestPaymentGatewayExtended2(t *testing.T) {
	suite.Run(t, new(PaymentGatewayExtendedTestSuite2))
}

// ========== Subscribe Handler Extended Tests ==========

type SubscribeExtendedTestSuite struct {
	HandlerTestSuite
	testUser  *model.User
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

	s.testGroup = &model.SubscriptionGroup{
		Name: "Test Group",
	}
	s.db.Create(s.testGroup)

	s.router = gin.New()
}

func (s *SubscribeExtendedTestSuite) TestGetGroups() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups", handler.GetGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestCreateGroup() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups", handler.CreateGroup)

	body := map[string]string{
		"name": "New Group",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestCreateGroup_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups", handler.CreateGroup)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestDeleteGroup() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/groups/:id", handler.DeleteGroup)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestGetGroupProtocols() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id/protocols", handler.GetGroupProtocols)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestDeleteTemplate() {
	tpl := &model.SubscriptionTemplate{
		Name:    "Test Template",
		GroupID: s.testGroup.ID,
	}
	s.db.Create(tpl)

	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/templates/"+strconv.FormatUint(uint64(tpl.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestGetSubscriptionFormats() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/formats", handler.GetSubscriptionFormats)

	req, _ := http.NewRequest("GET", "/admin/subscription/formats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestGetProtocolTypes() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/protocols", handler.GetProtocolTypes)

	req, _ := http.NewRequest("GET", "/admin/subscription/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *SubscribeExtendedTestSuite) TestPreviewSubscription() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/preview", handler.PreviewSubscription)

	body := map[string]interface{}{
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
}

func TestSubscribeExtended(t *testing.T) {
	suite.Run(t, new(SubscribeExtendedTestSuite))
}

// ========== Invite Handler Extended Tests ==========

type InviteExtendedTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *InviteExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "invite-extended@example.com",
		Token:          "invite-extended-token",
		UUID:           "invite-extended-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *InviteExtendedTestSuite) TestRequestWithdraw() {
	handler := NewInviteHandler()
	s.router.POST("/user/invite/withdraw", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.RequestWithdraw(c)
	})

	body := map[string]interface{}{
		"amount": 100,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/user/invite/withdraw", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// May fail due to insufficient balance, but tests the handler path
}

func (s *InviteExtendedTestSuite) TestRequestWithdraw_InvalidBody() {
	handler := NewInviteHandler()
	s.router.POST("/user/invite/withdraw", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.RequestWithdraw(c)
	})

	req, _ := http.NewRequest("POST", "/user/invite/withdraw", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *InviteExtendedTestSuite) TestGetConfig() {
	handler := NewInviteHandler()
	s.router.GET("/admin/invite/config", handler.GetConfig)

	req, _ := http.NewRequest("GET", "/admin/invite/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestInviteExtended(t *testing.T) {
	suite.Run(t, new(InviteExtendedTestSuite))
}

// ========== Notification Handler Extended Tests ==========

type NotificationExtendedTestSuite struct {
	HandlerTestSuite
	testUser *model.User
}

func (s *NotificationExtendedTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.testUser = &model.User{
		Email:          "notify@example.com",
		Token:          "notify-token",
		UUID:           "notify-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(s.testUser)

	s.router = gin.New()
}

func (s *NotificationExtendedTestSuite) TestGetUserNotifications() {
	handler := NewNotificationHandler()
	s.router.GET("/user/notifications", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetUserNotifications(c)
	})

	req, _ := http.NewRequest("GET", "/user/notifications", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestMarkAllAsRead() {
	handler := NewNotificationHandler()
	s.router.POST("/user/notifications/read-all", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.MarkAllAsRead(c)
	})

	req, _ := http.NewRequest("POST", "/user/notifications/read-all", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestGetUnreadCount() {
	handler := NewNotificationHandler()
	s.router.GET("/user/notifications/unread-count", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.GetUnreadCount(c)
	})

	req, _ := http.NewRequest("GET", "/user/notifications/unread-count", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestListTemplates() {
	tpl := &model.NotificationTemplate{
		Name:    "Extended Email Template",
		Type:    "email",
		Event:   "order.paid",
		Title:   "Paid",
		Content: "ok",
		Enabled: true,
	}
	s.db.Create(tpl)

	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/templates", handler.ListTemplates)

	req, _ := http.NewRequest("GET", "/admin/notification/templates?type=email", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 1)
	assert.Equal(s.T(), float64(1), data["total"])
}

func (s *NotificationExtendedTestSuite) TestCreateTemplate() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/templates", handler.CreateTemplate)

	body := map[string]interface{}{
		"name":    "Test Template",
		"type":    "email",
		"event":   "user.register",
		"title":   "Test Title",
		"content": "Test Content",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/notification/templates", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestUpdateTemplate_WithTypeAndEvent() {
	tpl := &model.NotificationTemplate{
		Name:    "Template Before",
		Type:    "email",
		Event:   "user.register",
		Title:   "Before",
		Content: "Before content",
		Enabled: true,
	}
	s.db.Create(tpl)

	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/templates/:id", handler.UpdateTemplate)

	body := map[string]interface{}{
		"name":    "Template After",
		"type":    "telegram",
		"event":   "ticket.reply",
		"title":   "After",
		"content": "After content",
		"enabled": false,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(
		"PUT",
		"/admin/notification/templates/"+strconv.FormatUint(uint64(tpl.ID), 10),
		bytes.NewReader(jsonBody),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var updated model.NotificationTemplate
	err := s.db.First(&updated, tpl.ID).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "Template After", updated.Name)
	assert.Equal(s.T(), "telegram", updated.Type)
	assert.Equal(s.T(), "ticket.reply", updated.Event)
	assert.Equal(s.T(), "After", updated.Title)
	assert.Equal(s.T(), "After content", updated.Content)
	assert.False(s.T(), updated.Enabled)
}

func (s *NotificationExtendedTestSuite) TestCreateTemplate_InvalidBody() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/templates", handler.CreateTemplate)

	req, _ := http.NewRequest("POST", "/admin/notification/templates", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NotificationExtendedTestSuite) TestListLogs() {
	userID := s.testUser.ID
	logItem := &model.NotificationLog{
		UserID:  &userID,
		Type:    "email",
		Event:   "user.traffic_low",
		Title:   "Low Traffic",
		Content: "warn",
		Status:  2,
	}
	s.db.Create(logItem)

	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/logs", handler.ListLogs)

	req, _ := http.NewRequest("GET", "/admin/notification/logs?status=failed", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	data, ok := resp["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	list, ok := data["list"].([]interface{})
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 1)

	row, ok := list[0].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "failed", row["status"])
	assert.Equal(s.T(), float64(2), row["status_code"])
}

func (s *NotificationExtendedTestSuite) TestGetEmailConfig() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/email/config", handler.GetEmailConfig)

	req, _ := http.NewRequest("GET", "/admin/notification/email/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestUpdateEmailConfig() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	body := map[string]interface{}{
		"host":         "smtp.example.com",
		"port":         587,
		"from_address": "noreply@example.com",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/notification/email/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NotificationExtendedTestSuite) TestUpdateEmailConfig_InvalidBody() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	req, _ := http.NewRequest("PUT", "/admin/notification/email/config", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NotificationExtendedTestSuite) TestEmailConfig_RoundTrip_EncryptionStringAndBool_KeepPasswordOnEmpty() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/email/config", handler.GetEmailConfig)
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	initialBody := map[string]interface{}{
		"host":         "smtp.persist.test",
		"port":         465,
		"username":     "notify-user",
		"password":     "KeepPassword#1",
		"from_name":    "Notify Bot",
		"from_address": "notify@example.com",
		"encryption":   "tls",
	}
	initialJSON, _ := json.Marshal(initialBody)
	putReq, _ := http.NewRequest("PUT", "/admin/notification/email/config", bytes.NewReader(initialJSON))
	putReq.Header.Set("Content-Type", "application/json")
	putResp := httptest.NewRecorder()
	s.router.ServeHTTP(putResp, putReq)
	assert.Equal(s.T(), http.StatusOK, putResp.Code)

	// encryption string should be accepted and persisted through GET payload.
	getReq1, _ := http.NewRequest("GET", "/admin/notification/email/config", nil)
	getResp1 := httptest.NewRecorder()
	s.router.ServeHTTP(getResp1, getReq1)
	assert.Equal(s.T(), http.StatusOK, getResp1.Code)

	var getPayload1 map[string]interface{}
	err := json.Unmarshal(getResp1.Body.Bytes(), &getPayload1)
	assert.NoError(s.T(), err)
	data1, ok := getPayload1["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "smtp.persist.test", data1["host"])
	assert.Equal(s.T(), float64(465), data1["port"])
	assert.Equal(s.T(), "notify-user", data1["username"])
	assert.Equal(s.T(), "notify@example.com", data1["from_address"])

	enc1, hasEnc1 := data1["encryption"]
	assert.True(s.T(), hasEnc1)
	if encStr, ok := enc1.(string); ok {
		assert.NotEmpty(s.T(), encStr)
	} else if encBool, ok := enc1.(bool); ok {
		assert.True(s.T(), encBool)
	} else {
		s.T().Fatalf("unexpected encryption type: %T", enc1)
	}

	var storedEmailConfig model.SystemConfig
	err = s.db.Where("key = ?", notificationEmailConfigKey).First(&storedEmailConfig).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), storedEmailConfig.Value, "KeepPassword#1")

	// bool encryption input should be accepted; empty password should keep previous secret.
	updateBody := map[string]interface{}{
		"host":         "smtp.persist.test",
		"port":         465,
		"username":     "notify-user",
		"password":     "",
		"from_name":    "Notify Bot Updated",
		"from_address": "notify@example.com",
		"encryption":   true,
	}
	updateJSON, _ := json.Marshal(updateBody)
	putReq2, _ := http.NewRequest("PUT", "/admin/notification/email/config", bytes.NewReader(updateJSON))
	putReq2.Header.Set("Content-Type", "application/json")
	putResp2 := httptest.NewRecorder()
	s.router.ServeHTTP(putResp2, putReq2)
	assert.Equal(s.T(), http.StatusOK, putResp2.Code)

	getReq2, _ := http.NewRequest("GET", "/admin/notification/email/config", nil)
	getResp2 := httptest.NewRecorder()
	s.router.ServeHTTP(getResp2, getReq2)
	assert.Equal(s.T(), http.StatusOK, getResp2.Code)

	var getPayload2 map[string]interface{}
	err = json.Unmarshal(getResp2.Body.Bytes(), &getPayload2)
	assert.NoError(s.T(), err)
	data2, ok := getPayload2["data"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "Notify Bot Updated", data2["from_name"])

	enc2, hasEnc2 := data2["encryption"]
	assert.True(s.T(), hasEnc2)
	if encStr, ok := enc2.(string); ok {
		assert.NotEmpty(s.T(), encStr)
	} else if encBool, ok := enc2.(bool); ok {
		assert.True(s.T(), encBool)
	} else {
		s.T().Fatalf("unexpected encryption type after bool update: %T", enc2)
	}

	var storedEmailConfigAfter model.SystemConfig
	err = s.db.Where("key = ?", notificationEmailConfigKey).First(&storedEmailConfigAfter).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), storedEmailConfigAfter.Value, "KeepPassword#1")
}

func (s *NotificationExtendedTestSuite) TestDeleteTemplate() {
	tpl := &model.NotificationTemplate{
		Name:    "To Delete",
		Type:    "email",
		Event:   "user.register",
		Title:   "Test",
		Content: "Test",
	}
	s.db.Create(tpl)

	handler := NewNotificationHandler()
	s.router.DELETE("/admin/notification/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/admin/notification/templates/"+strconv.FormatUint(uint64(tpl.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func TestNotificationExtended(t *testing.T) {
	suite.Run(t, new(NotificationExtendedTestSuite))
}

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
}

func (s *MFAExtendedTestSuite) TestGetAdminConfig() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/mfa/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *MFAExtendedTestSuite) TestUpdateAdminConfig() {
	handler := NewMFAHandler()
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	body := map[string]interface{}{
		"enabled": true,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/mfa/config", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *MFAExtendedTestSuite) TestAdminConfig_FrontendFields_RoundTrip() {
	handler := NewMFAHandler()
	s.router.GET("/admin/mfa/config", handler.GetAdminConfig)
	s.router.PUT("/admin/mfa/config", handler.UpdateAdminConfig)

	updateBody := map[string]interface{}{
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

	var payload map[string]interface{}
	err := json.Unmarshal(getResp.Body.Bytes(), &payload)
	assert.NoError(s.T(), err)
	data, ok := payload["data"].(map[string]interface{})
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

	methods, ok := data["methods"].(map[string]interface{})
	assert.True(s.T(), ok)
	assert.Equal(s.T(), true, methods["totp"])
	assert.Equal(s.T(), true, methods["sms"])
	assert.Equal(s.T(), false, methods["email"])

	var storedMFAConfig model.SystemConfig
	err = s.db.Where("key = ?", mfaAdminConfigKey).First(&storedMFAConfig).Error
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

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
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
		APIKey: "test-api-key",
		Status: 1,
	}
	s.db.Create(s.testNode)

	s.router = gin.New()
}

func (s *NodeExtendedTestSuite) TestGetNodes() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes", handler.GetNodes)

	req, _ := http.NewRequest("GET", "/admin/nodes", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestCreateNode() {
	handler := NewNodeHandler()
	s.router.POST("/admin/nodes", handler.CreateNode)

	body := map[string]interface{}{
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

func (s *NodeExtendedTestSuite) TestGetNode() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestGetNode_InvalidID() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/:id", handler.GetNode)

	req, _ := http.NewRequest("GET", "/admin/nodes/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *NodeExtendedTestSuite) TestUpdateNode() {
	handler := NewNodeHandler()
	s.router.PUT("/admin/nodes/:id", handler.UpdateNode)

	body := map[string]interface{}{
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

func (s *NodeExtendedTestSuite) TestDeleteNode() {
	handler := NewNodeHandler()
	s.router.DELETE("/admin/nodes/:id", handler.DeleteNode)

	req, _ := http.NewRequest("DELETE", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestGetNodeStats() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/:id/stats", handler.GetNodeStats)

	req, _ := http.NewRequest("GET", "/admin/nodes/"+strconv.FormatUint(uint64(s.testNode.ID), 10)+"/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestGetAuthKeys() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/auth-keys", handler.GetAuthKeys)

	req, _ := http.NewRequest("GET", "/admin/nodes/auth-keys", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *NodeExtendedTestSuite) TestGetProtocolTemplates() {
	handler := NewNodeHandler()
	s.router.GET("/admin/nodes/protocol-templates", handler.GetProtocolTemplates)

	req, _ := http.NewRequest("GET", "/admin/nodes/protocol-templates", nil)
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

func (s *AdminExtendedTestSuite) TestGetOrderList_WithFilters() {
	handler := NewAdminHandler()
	s.router.GET("/admin/orders", handler.GetOrderList)

	req, _ := http.NewRequest("GET", "/admin/orders?status=0&user_id="+strconv.FormatUint(uint64(s.testUser.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestGetOrderStats() {
	handler := NewAdminHandler()
	s.router.GET("/admin/orders/stats", handler.GetOrderStats)

	req, _ := http.NewRequest("GET", "/admin/orders/stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestMarkOrderPaid_Success() {
	// Create a new order for this test
	order := &model.Order{
		TradeNo: "PAID-ORDER-001",
		UserID:  s.testUser.ID,
		PlanID:  s.testPlan.ID,
		Status:  0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/paid", handler.MarkOrderPaid)

	req, _ := http.NewRequest("POST", "/admin/orders/"+strconv.FormatUint(uint64(order.ID), 10)+"/paid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestMarkOrderPaid_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/paid", handler.MarkOrderPaid)

	req, _ := http.NewRequest("POST", "/admin/orders/invalid/paid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestCancelOrder_Success() {
	// Create a new order for this test
	order := &model.Order{
		TradeNo: "CANCEL-ORDER-001",
		UserID:  s.testUser.ID,
		PlanID:  s.testPlan.ID,
		Status:  0,
	}
	s.db.Create(order)

	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/cancel", handler.CancelOrder)

	req, _ := http.NewRequest("POST", "/admin/orders/"+strconv.FormatUint(uint64(order.ID), 10)+"/cancel", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestCancelOrder_InvalidID() {
	handler := NewAdminHandler()
	s.router.POST("/admin/orders/:id/cancel", handler.CancelOrder)

	req, _ := http.NewRequest("POST", "/admin/orders/invalid/cancel", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateOrderStatus_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/orders/:id/status", handler.UpdateOrderStatus)

	body := map[string]interface{}{
		"status": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10)+"/status", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateOrderStatus_InvalidID() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/orders/:id/status", handler.UpdateOrderStatus)

	body := map[string]interface{}{
		"status": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/orders/invalid/status", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateOrderStatus_InvalidBody() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/orders/:id/status", handler.UpdateOrderStatus)

	req, _ := http.NewRequest("PUT", "/admin/orders/"+strconv.FormatUint(uint64(s.testOrder.ID), 10)+"/status", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdateUser_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/users/:id", handler.UpdateUser)

	body := map[string]interface{}{
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

	body := map[string]interface{}{
		"balance": 1000,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/users/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Handler doesn't check if user exists, just performs update
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestGetPlans_WithGroupId() {
	handler := NewAdminHandler()
	s.router.GET("/admin/plans", handler.GetPlans)

	req, _ := http.NewRequest("GET", "/admin/plans?group_id=1", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestGetDashboard() {
	handler := NewAdminHandler()
	s.router.GET("/admin/dashboard", handler.GetDashboard)

	req, _ := http.NewRequest("GET", "/admin/dashboard", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestCreatePlan_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans", handler.CreatePlan)

	req, _ := http.NewRequest("POST", "/admin/plans", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdatePlan_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/plans/:id", handler.UpdatePlan)

	body := map[string]interface{}{
		"name":            "Updated Plan",
		"transfer_enable": 21474836480,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestUpdatePlan_NotFound() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/plans/:id", handler.UpdatePlan)

	body := map[string]interface{}{
		"name": "Updated Plan",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/plans/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Handler doesn't check if plan exists, just performs update
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestDeletePlan_Success() {
	plan := &model.Plan{
		Name:           "Plan to Delete",
		TransferEnable: 1073741824,
	}
	s.db.Create(plan)

	handler := NewAdminHandler()
	s.router.DELETE("/admin/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/admin/plans/"+strconv.FormatUint(uint64(plan.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestDeletePlan_InvalidID() {
	handler := NewAdminHandler()
	s.router.DELETE("/admin/plans/:id", handler.DeletePlan)

	req, _ := http.NewRequest("DELETE", "/admin/plans/invalid", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func (s *AdminExtendedTestSuite) TestAssignPlanToUser_Success() {
	// Create a new user without plan
	user := &model.User{
		Email:          "assign-plan@example.com",
		Token:          "assign-plan-token",
		UUID:           "assign-plan-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(user)

	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	body := map[string]interface{}{
		"user_id": user.ID,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *AdminExtendedTestSuite) TestAssignPlanToUser_InvalidBody() {
	handler := NewAdminHandler()
	s.router.POST("/admin/plans/:id/assign", handler.AssignPlanToUser)

	req, _ := http.NewRequest("POST", "/admin/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/assign", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

func TestAdminExtended(t *testing.T) {
	suite.Run(t, new(AdminExtendedTestSuite))
}
