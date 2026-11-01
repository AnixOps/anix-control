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

func strPtr(v string) *string {
	return &v
}

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

func (s *TelegramHandlerExtendedTestSuite) TestGetBot() {
	handler := NewTelegramHandler()
	s.router.GET("/admin/telegram/bot", handler.GetBot)

	req, _ := http.NewRequest("GET", "/admin/telegram/bot", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "test-token", data["token"])
	assert.Equal(s.T(), "testbot", data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *TelegramHandlerExtendedTestSuite) TestUpdateBot() {
	handler := NewTelegramHandler()
	s.router.PUT("/admin/telegram/bot", handler.UpdateBot)

	body := map[string]any{
		"name":            "Panel Bot",
		"token":           "updated-token",
		"welcome_message": "Welcome",
		"admin_ids":       []any{"12345", float64(67890)},
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/admin/telegram/bot", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "Panel Bot", data["name"])
	assert.Equal(s.T(), "updated-token", data["token"])
	assert.NotContains(s.T(), resp, "error")

	var updated model.TelegramBot
	err := s.db.First(&updated, s.testBot.ID).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "Panel Bot", updated.Name)
	assert.JSONEq(s.T(), `[12345,67890]`, updated.AdminIDs)
}

func (s *TelegramHandlerExtendedTestSuite) TestDeleteWebhook() {
	handler := NewTelegramHandler()
	s.router.DELETE("/admin/telegram/webhook", handler.DeleteWebhook)

	req, _ := http.NewRequest("DELETE", "/admin/telegram/webhook", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "webhook deleted", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *TelegramHandlerExtendedTestSuite) TestSendNotification() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/notify", handler.SendNotification)

	body := map[string]any{
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

	body := map[string]any{
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "broadcast completed", data["message"])
	assert.Equal(s.T(), float64(0), data["success"])
	assert.Equal(s.T(), float64(0), data["failed"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *TelegramHandlerExtendedTestSuite) TestBroadcast_NestedMessage() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/broadcast", handler.Broadcast)

	body := map[string]any{
		"message": map[string]any{
			"message": "Nested message payload",
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/telegram/broadcast", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "broadcast completed", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *TelegramHandlerExtendedTestSuite) TestBroadcast_InvalidBody() {
	handler := NewTelegramHandler()
	s.router.POST("/admin/telegram/broadcast", handler.Broadcast)

	req, _ := http.NewRequest("POST", "/admin/telegram/broadcast", strings.NewReader("invalid"))
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

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	list, ok := data["list"].([]any)
	assert.True(s.T(), ok)
	assert.GreaterOrEqual(s.T(), len(list), 1)
	first, ok := list[0].(map[string]any)
	assert.True(s.T(), ok)
	_, hasEmail := first["user_email"]
	_, hasNotify := first["notify_enabled"]
	assert.True(s.T(), hasEmail)
	assert.True(s.T(), hasNotify)
	assert.NotContains(s.T(), resp, "list")
	assert.NotContains(s.T(), resp, "error")
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

	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	list, ok := data["list"].([]any)
	assert.True(s.T(), ok)
	assert.GreaterOrEqual(s.T(), len(list), 25)
	assert.NotContains(s.T(), resp, "list")
	assert.NotContains(s.T(), resp, "error")
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

	body := map[string]any{
		"notify_enabled": false,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/admin/telegram/users/"+strconv.FormatUint(uint64(tgUser.ID), 10)+"/notify", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), false, data["notify_enabled"])
	assert.NotContains(s.T(), resp, "error")

	var refreshed model.TelegramUser
	err := s.db.First(&refreshed, tgUser.ID).Error
	assert.NoError(s.T(), err)
	assert.False(s.T(), refreshed.NotifyExpire)
	assert.False(s.T(), refreshed.NotifyTraffic)
	assert.False(s.T(), refreshed.NotifyTicket)
}

func (s *TelegramHandlerExtendedTestSuite) TestGetTelegramStatus_NoBinding() {
	user := &model.User{
		Email:          "tg-status@example.com",
		Token:          "tg-status-token",
		UUID:           "tg-status-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(user)

	handler := NewTelegramHandler()
	s.router.GET("/user/telegram/status", func(c *gin.Context) {
		c.Set("user_id", user.ID)
		handler.GetTelegramStatus(c)
	})

	req, _ := http.NewRequest("GET", "/user/telegram/status", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), false, data["bound"])
	assert.Equal(s.T(), "", data["username"])
	assert.NotContains(s.T(), resp, "error")
}

// The user's unbind and notify-settings routes are not implemented: each
// answers an error envelope and leaves the binding and its settings alone. A
// success answer here once made a user believe they were unbound.
func (s *TelegramHandlerExtendedTestSuite) TestUserTelegramActionsAreNotImplemented() {
	user := &model.User{
		Email:          "tg-actions@example.com",
		Token:          "tg-actions-token",
		UUID:           "tg-actions-uuid",
		TransferEnable: 1073741824,
	}
	s.db.Create(user)
	// The notify switches default to on.
	binding := &model.TelegramUser{UserID: user.ID, TelegramID: 424242, Username: "tg-actions"}
	s.Require().NoError(s.db.Create(binding).Error)

	handler := NewTelegramHandler()
	s.router.POST("/user/telegram/unbind", func(c *gin.Context) {
		c.Set("user_id", user.ID)
		handler.UnbindTelegram(c)
	})
	s.router.POST("/user/telegram/notify", func(c *gin.Context) {
		c.Set("user_id", user.ID)
		handler.UpdateNotifySettings(c)
	})

	unbindReq, _ := http.NewRequest("POST", "/user/telegram/unbind", nil)
	unbindResp := httptest.NewRecorder()
	s.router.ServeHTTP(unbindResp, unbindReq)
	assert.Equal(s.T(), http.StatusOK, unbindResp.Code)
	unbindPayload := decodePanelTestResponse(s.T(), unbindResp)
	assert.Equal(s.T(), float64(-1), unbindPayload["code"])
	assert.Nil(s.T(), unbindPayload["data"])
	assert.Contains(s.T(), unbindPayload["msg"], "Telegram 解绑尚未实现")
	assert.NotContains(s.T(), unbindResp.Body.String(), "unbound successfully")

	notifyJSON, _ := json.Marshal(map[string]any{"notify_expire": false})
	notifyReq, _ := http.NewRequest("POST", "/user/telegram/notify", bytes.NewReader(notifyJSON))
	notifyReq.Header.Set("Content-Type", "application/json")
	notifyResp := httptest.NewRecorder()
	s.router.ServeHTTP(notifyResp, notifyReq)
	assert.Equal(s.T(), http.StatusOK, notifyResp.Code)
	notifyPayload := decodePanelTestResponse(s.T(), notifyResp)
	assert.Equal(s.T(), float64(-1), notifyPayload["code"])
	assert.Nil(s.T(), notifyPayload["data"])
	assert.Contains(s.T(), notifyPayload["msg"], "Telegram 通知设置尚未实现")
	assert.NotContains(s.T(), notifyResp.Body.String(), "settings updated")

	// Nothing was changed: the binding is still there with its settings.
	var stored model.TelegramUser
	s.Require().NoError(s.db.Where("user_id = ?", user.ID).First(&stored).Error)
	assert.Equal(s.T(), binding.TelegramID, stored.TelegramID)
	assert.True(s.T(), stored.NotifyExpire, "the switch the request turned off is unchanged")
	assert.True(s.T(), stored.NotifyTraffic)
	assert.True(s.T(), stored.NotifyTicket)

	// A body that does not parse is still refused as a bad request.
	badReq, _ := http.NewRequest("POST", "/user/telegram/notify", bytes.NewReader([]byte(`{"notify_expire":"no"}`)))
	badReq.Header.Set("Content-Type", "application/json")
	badResp := httptest.NewRecorder()
	s.router.ServeHTTP(badResp, badReq)
	badPayload := decodePanelTestResponse(s.T(), badResp)
	assert.Equal(s.T(), float64(-1), badPayload["code"])
	assert.NotContains(s.T(), badPayload["msg"], "尚未实现")
}

func TestTelegramHandlerExtended(t *testing.T) {
	suite.Run(t, new(TelegramHandlerExtendedTestSuite))
}

// ========== Payment Gateway Extended Tests ==========

// ========== Forward User Rules Tests ==========

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
		RawConfig: strPtr(`{"log_level":"value"}`),
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), float64(s.testNode.ID), data["node_id"])
	assert.Equal(s.T(), s.testNode.Name, data["name"])
	rawConfig := data["raw_config"].(map[string]any)
	assert.Equal(s.T(), "value", rawConfig["log_level"])
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "配置更新成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
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
	body := map[string]any{
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func TestTelegramUser(t *testing.T) {
	suite.Run(t, new(TelegramUserTestSuite))
}

// ========== Admin Coupon Extended Tests ==========

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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "参数错误")
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

	body := map[string]any{
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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "参数错误")
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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

func (s *SubscribeExtendedTestSuite) TestGetGroups() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups", handler.GetGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.NotEmpty(s.T(), data)
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "New Group", data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestCreateGroup_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups", handler.CreateGroup)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestGetGroup_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id", handler.GetGroup)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestUpdateGroup_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/admin/subscription/groups/:id", handler.UpdateGroup)

	req, _ := http.NewRequest("PUT", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestDeleteGroup() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/groups/:id", handler.DeleteGroup)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestDeleteGroup_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/groups/:id", handler.DeleteGroup)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/groups/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetTemplates() {
	tpl := &model.SubscriptionTemplate{
		Name:    "Test Template",
		Type:    "vless",
		GroupID: s.testGroup.ID,
	}
	s.db.Create(tpl)

	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id/templates", handler.GetTemplates)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.NotEmpty(s.T(), data)
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestGetTemplates_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id/templates", handler.GetTemplates)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/invalid/templates", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscribeExtendedTestSuite) TestCreateTemplate() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/templates", handler.CreateTemplate)

	body := map[string]any{
		"name":   "Created Template",
		"type":   "vless",
		"server": "example.com",
		"port":   443,
		"enable": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/templates", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "Created Template", data["name"])
	assert.Equal(s.T(), float64(s.testGroup.ID), data["group_id"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestCreateTemplate_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/templates", handler.CreateTemplate)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/templates", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestGetTemplate() {
	tpl := &model.SubscriptionTemplate{
		Name:    "Get Template",
		Type:    "vless",
		GroupID: s.testGroup.ID,
	}
	s.db.Create(tpl)

	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/templates/:id", handler.GetTemplate)

	req, _ := http.NewRequest("GET", "/admin/subscription/templates/"+strconv.FormatUint(uint64(tpl.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "Get Template", data["name"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestGetTemplate_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/templates/:id", handler.GetTemplate)

	req, _ := http.NewRequest("GET", "/admin/subscription/templates/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "模板不存在")
}

func (s *SubscribeExtendedTestSuite) TestUpdateTemplate() {
	tpl := &model.SubscriptionTemplate{
		Name:    "Old Template",
		Type:    "vless",
		GroupID: s.testGroup.ID,
		Enable:  1,
	}
	s.db.Create(tpl)

	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/admin/subscription/templates/:id", handler.UpdateTemplate)

	body := map[string]any{"name": "Updated Template", "enable": 0}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/subscription/templates/"+strconv.FormatUint(uint64(tpl.ID), 10), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "Updated Template", data["name"])
	assert.Equal(s.T(), float64(0), data["enable"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestUpdateTemplate_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/admin/subscription/templates/:id", handler.UpdateTemplate)

	req, _ := http.NewRequest("PUT", "/admin/subscription/templates/99999", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestUpdateTemplate_EmptyBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/admin/subscription/templates/:id", handler.UpdateTemplate)

	req, _ := http.NewRequest("PUT", "/admin/subscription/templates/99999", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "参数错误")
}

func (s *SubscribeExtendedTestSuite) TestUpdateTemplate_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.PUT("/admin/subscription/templates/:id", handler.UpdateTemplate)

	body := map[string]any{"name": "Missing Template"}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/subscription/templates/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "模板不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetGroupProtocols() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id/protocols", handler.GetGroupProtocols)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/protocols", nil)
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

func (s *SubscribeExtendedTestSuite) TestGetGroupProtocols_InvalidID() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/groups/:id/protocols", handler.GetGroupProtocols)

	req, _ := http.NewRequest("GET", "/admin/subscription/groups/invalid/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "id")
}

func (s *SubscribeExtendedTestSuite) TestUpdateGroupProtocols() {
	node := &model.Node{
		Name: "Protocol Bind Node",
		Host: "127.0.0.1",
		Port: 443,
		Rate: 1,
		Show: 1,
	}
	s.db.Create(node)
	protocol := &model.NodeProtocol{
		NodeID: node.ID,
		Name:   "Protocol Bind",
		Type:   model.ProtocolVLESS,
		Port:   443,
		Enable: 1,
		Show:   1,
	}
	s.db.Create(protocol)

	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/protocols", handler.UpdateGroupProtocols)

	body := map[string]any{"protocol_ids": []uint{protocol.ID}}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/protocols", bytes.NewReader(jsonBody))
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

func (s *SubscribeExtendedTestSuite) TestUpdateGroupProtocols_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/protocols", handler.UpdateGroupProtocols)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/protocols", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestUpdateGroupProtocols_GroupNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/protocols", handler.UpdateGroupProtocols)

	body := map[string]any{"protocol_ids": []uint{}}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/99999/protocols", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestUpdateGroupProtocols_ProtocolNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/groups/:id/protocols", handler.UpdateGroupProtocols)

	body := map[string]any{"protocol_ids": []uint{99999}}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10)+"/protocols", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "协议不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetAvailableProtocols() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/protocols/available", handler.GetAvailableProtocols)

	req, _ := http.NewRequest("GET", "/admin/subscription/protocols/available", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotNil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToUser() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/users/:user_id/groups", handler.AssignGroupToUser)

	body := map[string]any{"group_id": s.testGroup.ID}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "分配成功", data["message"])
	assert.NotContains(s.T(), resp, "error")

	var count int64
	s.db.Model(&model.UserSubscriptionGroup{}).
		Where("user_id = ? AND group_id = ?", s.testUser.ID, s.testGroup.ID).
		Count(&count)
	assert.Equal(s.T(), int64(1), count)
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToUser_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/users/:user_id/groups", handler.AssignGroupToUser)

	req, _ := http.NewRequest("POST", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToUser_GroupNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/users/:user_id/groups", handler.AssignGroupToUser)

	body := map[string]any{"group_id": 99999}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestRemoveGroupFromUser() {
	assert.NoError(s.T(), s.db.Create(&model.UserSubscriptionGroup{
		UserID:  s.testUser.ID,
		GroupID: s.testGroup.ID,
	}).Error)

	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/users/:user_id/groups/:group_id", handler.RemoveGroupFromUser)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "移除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestRemoveGroupFromUser_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/users/:user_id/groups/:group_id", handler.RemoveGroupFromUser)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户订阅分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetUserGroups() {
	assert.NoError(s.T(), s.db.Create(&model.UserSubscriptionGroup{
		UserID:  s.testUser.ID,
		GroupID: s.testGroup.ID,
	}).Error)

	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/users/:user_id/groups", handler.GetUserGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/users/"+strconv.FormatUint(uint64(s.testUser.ID), 10)+"/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
	group := data[0].(map[string]any)
	assert.Equal(s.T(), float64(s.testGroup.ID), group["id"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestGetUserGroups_UserNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/users/:user_id/groups", handler.GetUserGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/users/99999/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "用户不存在")
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToPlan() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/plans/:plan_id/groups", handler.AssignGroupToPlan)

	body := map[string]any{"group_id": s.testGroup.ID}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "分配成功", data["message"])
	assert.NotContains(s.T(), resp, "error")

	var count int64
	s.db.Model(&model.PlanSubscriptionGroup{}).
		Where("plan_id = ? AND group_id = ?", s.testPlan.ID, s.testGroup.ID).
		Count(&count)
	assert.Equal(s.T(), int64(1), count)
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToPlan_InvalidBody() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/plans/:plan_id/groups", handler.AssignGroupToPlan)

	req, _ := http.NewRequest("POST", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "invalid")
}

func (s *SubscribeExtendedTestSuite) TestAssignGroupToPlan_GroupNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.POST("/admin/subscription/plans/:plan_id/groups", handler.AssignGroupToPlan)

	body := map[string]any{"group_id": 99999}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestRemoveGroupFromPlan() {
	assert.NoError(s.T(), s.db.Create(&model.PlanSubscriptionGroup{
		PlanID:  s.testPlan.ID,
		GroupID: s.testGroup.ID,
	}).Error)

	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/plans/:plan_id/groups/:group_id", handler.RemoveGroupFromPlan)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "移除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestRemoveGroupFromPlan_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/plans/:plan_id/groups/:group_id", handler.RemoveGroupFromPlan)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups/"+strconv.FormatUint(uint64(s.testGroup.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "套餐订阅分组不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetPlanGroups() {
	assert.NoError(s.T(), s.db.Create(&model.PlanSubscriptionGroup{
		PlanID:  s.testPlan.ID,
		GroupID: s.testGroup.ID,
	}).Error)

	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/plans/:plan_id/groups", handler.GetPlanGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/plans/"+strconv.FormatUint(uint64(s.testPlan.ID), 10)+"/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].([]any)
	assert.Len(s.T(), data, 1)
	group := data[0].(map[string]any)
	assert.Equal(s.T(), float64(s.testGroup.ID), group["id"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestGetPlanGroups_PlanNotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/plans/:plan_id/groups", handler.GetPlanGroups)

	req, _ := http.NewRequest("GET", "/admin/subscription/plans/99999/groups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "套餐不存在")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].(map[string]any)
	assert.Equal(s.T(), "删除成功", data["message"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestDeleteTemplate_NotFound() {
	handler := NewSubscriptionAdminHandler()
	s.router.DELETE("/admin/subscription/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("DELETE", "/admin/subscription/templates/99999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	s.assertPanelError(w, "模板不存在")
}

func (s *SubscribeExtendedTestSuite) TestGetSubscriptionFormats() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/formats", handler.GetSubscriptionFormats)

	req, _ := http.NewRequest("GET", "/admin/subscription/formats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.GreaterOrEqual(s.T(), len(data), 12)
	assert.NotContains(s.T(), resp, "error")
}

func (s *SubscribeExtendedTestSuite) TestGetProtocolTypes() {
	handler := NewSubscriptionAdminHandler()
	s.router.GET("/admin/subscription/protocols", handler.GetProtocolTypes)

	req, _ := http.NewRequest("GET", "/admin/subscription/protocols", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data := resp["data"].([]any)
	assert.GreaterOrEqual(s.T(), len(data), 6)
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	list, ok := data["list"].([]any)
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 0)
	assert.Equal(s.T(), float64(0), data["total"])
	assert.Equal(s.T(), float64(1), data["page"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationExtendedTestSuite) TestMarkAsRead() {
	logItem := &model.NotificationLog{
		UserID:  &s.testUser.ID,
		Type:    "email",
		Event:   "ticket.reply",
		Title:   "Reply",
		Content: "body",
		Status:  1,
	}
	s.db.Create(logItem)

	handler := NewNotificationHandler()
	s.router.POST("/user/notifications/:id/read", func(c *gin.Context) {
		c.Set("user_id", s.testUser.ID)
		handler.MarkAsRead(c)
	})

	req, _ := http.NewRequest(
		"POST",
		"/user/notifications/"+strconv.FormatUint(uint64(logItem.ID), 10)+"/read",
		nil,
	)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "marked as read", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")

	var updated model.NotificationLog
	err := s.db.First(&updated, logItem.ID).Error
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), updated.ReadAt)
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "all notifications marked as read", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(0), data["count"])
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	list, ok := data["list"].([]any)
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 1)
	assert.Equal(s.T(), float64(1), data["total"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationExtendedTestSuite) TestCreateTemplate() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/templates", handler.CreateTemplate)

	body := map[string]any{
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "Test Template", data["name"])
	assert.Equal(s.T(), "email", data["type"])
	assert.NotContains(s.T(), resp, "error")
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

	body := map[string]any{
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "Template After", data["name"])
	assert.Equal(s.T(), "telegram", data["type"])
	assert.NotContains(s.T(), resp, "error")

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

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	assert.Nil(s.T(), resp["data"])
	assert.NotContains(s.T(), resp, "error")
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.NotEmpty(s.T(), resp["msg"])
	assert.NotZero(s.T(), resp["ts"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	list, ok := data["list"].([]any)
	assert.True(s.T(), ok)
	assert.Len(s.T(), list, 1)

	row, ok := list[0].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "failed", row["status"])
	assert.Equal(s.T(), float64(2), row["status_code"])
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationExtendedTestSuite) TestSendTestNotification_TelegramNotImplemented() {
	handler := NewNotificationHandler()
	s.router.POST("/admin/notification/test", handler.SendTestNotification)

	body := map[string]any{
		"type":    "telegram",
		"title":   "Test",
		"content": "Body",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/admin/notification/test", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// A Telegram test send is not implemented yet: report it, never a success
	// that delivered nothing.
	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(-1), resp["code"])
	assert.Contains(s.T(), resp["msg"], "telegram")
	assert.Contains(s.T(), resp["msg"], "not implemented")
}

func (s *NotificationExtendedTestSuite) TestGetEmailConfig() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/email/config", handler.GetEmailConfig)

	req, _ := http.NewRequest("GET", "/admin/notification/email/config", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Contains(s.T(), data, "host")
	assert.Contains(s.T(), data, "encryption")
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationExtendedTestSuite) TestUpdateEmailConfig() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	body := map[string]any{
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "email config updated", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
}

func (s *NotificationExtendedTestSuite) TestUpdateEmailConfig_InvalidBody() {
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	req, _ := http.NewRequest("PUT", "/admin/notification/email/config", strings.NewReader("invalid"))
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

func (s *NotificationExtendedTestSuite) TestEmailConfig_RoundTrip_EncryptionStringAndBool_KeepPasswordOnEmpty() {
	handler := NewNotificationHandler()
	s.router.GET("/admin/notification/email/config", handler.GetEmailConfig)
	s.router.PUT("/admin/notification/email/config", handler.UpdateEmailConfig)

	initialBody := map[string]any{
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

	getPayload1 := decodePanelTestResponse(s.T(), getResp1)
	assert.Equal(s.T(), float64(0), getPayload1["code"])
	assert.NotContains(s.T(), getPayload1, "error")
	data1, ok := getPayload1["data"].(map[string]any)
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
	err := s.db.Where("key = ?", notificationEmailConfigKey).First(&storedEmailConfig).Error
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), storedEmailConfig.Value, "KeepPassword#1")

	// bool encryption input should be accepted; empty password should keep previous secret.
	updateBody := map[string]any{
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

	getPayload2 := decodePanelTestResponse(s.T(), getResp2)
	assert.Equal(s.T(), float64(0), getPayload2["code"])
	assert.NotContains(s.T(), getPayload2, "error")
	data2, ok := getPayload2["data"].(map[string]any)
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
	resp := decodePanelTestResponse(s.T(), w)
	assert.Equal(s.T(), float64(0), resp["code"])
	data, ok := resp["data"].(map[string]any)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "deleted", data["message"])
	assert.NotContains(s.T(), resp, "message")
	assert.NotContains(s.T(), resp, "error")
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

	assertPanelTestError(s.T(), w, "参数错误")
}

func (s *AdminExtendedTestSuite) TestUpdatePlan_Success() {
	handler := NewAdminHandler()
	s.router.PUT("/admin/plans/:id", handler.UpdatePlan)

	body := map[string]any{
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

	body := map[string]any{
		"name": "Updated Plan",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/admin/plans/99999", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assertPanelTestError(s.T(), w, "套餐不存在")
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

	assertPanelTestError(s.T(), w, "ID")
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

	body := map[string]any{
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

	assertPanelTestError(s.T(), w, "参数错误")
}

func TestAdminExtended(t *testing.T) {
	suite.Run(t, new(AdminExtendedTestSuite))
}

// A template id that is not a number must never reach GORM, which would run
// it as an inline SQL condition.
func (s *NotificationExtendedTestSuite) TestTemplateIDsAreNotSQL() {
	for _, name := range []string{"first", "second"} {
		s.Require().NoError(s.db.Create(&model.NotificationTemplate{Name: name, Type: "email", Event: "e", Content: "c"}).Error)
	}
	handler := NewNotificationHandler()
	s.router.PUT("/admin/notification/templates/:id", handler.UpdateTemplate)
	s.router.DELETE("/admin/notification/templates/:id", handler.DeleteTemplate)

	req, _ := http.NewRequest("PUT", "/admin/notification/templates/0%20OR%201=1", strings.NewReader(`{"name":"renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), "template not found", decodePanelTestResponse(s.T(), w)["msg"])

	req, _ = http.NewRequest("DELETE", "/admin/notification/templates/0%20OR%201=1", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(s.T(), "invalid template id", decodePanelTestResponse(s.T(), w)["msg"])

	var names []string
	s.Require().NoError(s.db.Model(&model.NotificationTemplate{}).Order("id").Pluck("name", &names).Error)
	assert.Equal(s.T(), []string{"first", "second"}, names)
}
