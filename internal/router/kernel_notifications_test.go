package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Telegram test sits in the v4 administrator group: no token is 401, a
// non-administrator is 403, and an administrator reaches the handler. The
// token it would send with never appears in the answer.
func TestSetup_TelegramTestRoute_RequiresAdmin(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, database.GetDB().AutoMigrate(&model.TelegramBot{}, &model.TelegramUser{}, &model.OperationLog{}))
	const path = "/api/v4/kernel/notifications/telegram/test"

	serve := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}

	assert.Equal(t, http.StatusUnauthorized, serve("").Code)
	assert.Equal(t, http.StatusForbidden, serve(testRouterJWT(t, cfg.JWT.Secret, false)).Code)

	require.NoError(t, database.GetDB().Create(&model.TelegramBot{Name: "bot", Token: "123456789:SECRETtokenValueForTests-0123456789abc"}).Error)
	admin := serve(testRouterJWT(t, cfg.JWT.Secret, true))
	// The administrator (user 1) has no bound Telegram account here.
	require.Equal(t, http.StatusConflict, admin.Code, admin.Body.String())
	assert.Contains(t, admin.Body.String(), "telegram_not_bound")
	assert.NotContains(t, admin.Body.String(), "SECRET")
}
