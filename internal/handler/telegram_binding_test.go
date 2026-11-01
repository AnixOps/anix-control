package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func telegramBindingRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TelegramBot{}, &model.TelegramUser{}))
	h := &TelegramBindingHandler{db: func() *gorm.DB { return db }, bots: service.NewTelegramBotService, now: time.Now}
	r := gin.New()
	asUser := func(id uint) gin.HandlerFunc { return func(c *gin.Context) { c.Set("user_id", id); c.Next() } }
	r.POST("/admin/telegram/bindings", h.AdminBind)
	r.DELETE("/admin/telegram/bindings/:user_id", h.AdminUnbind)
	r.GET("/user/telegram", asUser(1), h.UserStatus)
	r.DELETE("/user/telegram", asUser(1), h.UserUnbind)
	r.POST("/user/telegram/bind-link", asUser(1), h.UserBindLink)
	return r, db
}

func telegramBindingDo(r *gin.Engine, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestTelegramBindingAdminAndUserRoutes(t *testing.T) {
	r, db := telegramBindingRouter(t)
	require.NoError(t, db.Create(&model.User{Email: "Member@Example.com", Token: "tok-1", UUID: "u-1"}).Error)

	w, _ := telegramBindingDo(r, http.MethodPost, "/user/telegram/bind-link", nil)
	assert.Equal(t, http.StatusConflict, w.Code, "no bot yet")

	w, body := telegramBindingDo(r, http.MethodPost, "/admin/telegram/bindings", map[string]any{"email": "member@example.com", "telegram_id": "123456789"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, 123456789, body["data"].(map[string]any)["telegram_id"])

	w, _ = telegramBindingDo(r, http.MethodPost, "/admin/telegram/bindings", map[string]any{"user_id": 1, "telegram_id": 5})
	assert.Equal(t, http.StatusConflict, w.Code)
	w, _ = telegramBindingDo(r, http.MethodPost, "/admin/telegram/bindings", map[string]any{"user_id": 9, "telegram_id": 5})
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = telegramBindingDo(r, http.MethodPost, "/admin/telegram/bindings", map[string]any{"user_id": 1, "telegram_id": "abc"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w, body = telegramBindingDo(r, http.MethodGet, "/user/telegram", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, body["data"].(map[string]any)["bound"])

	w, _ = telegramBindingDo(r, http.MethodDelete, "/user/telegram", nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = telegramBindingDo(r, http.MethodDelete, "/user/telegram", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	_, _ = telegramBindingDo(r, http.MethodPost, "/admin/telegram/bindings", map[string]any{"user_id": 1, "telegram_id": 7})
	w, _ = telegramBindingDo(r, http.MethodDelete, "/admin/telegram/bindings/1", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}
