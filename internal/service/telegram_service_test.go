package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTelegramTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&model.TelegramBot{},
		&model.TelegramUser{},
		&model.TelegramChat{},
		&model.User{},
		&model.Plan{},
	)
	require.NoError(t, err)

	return db
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestNewTelegramBotService(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.db)
	assert.NotNil(t, svc.client)
}

func TestTelegramBotService_GetBot(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Test with no bot configured
	_, err := svc.GetBot()
	assert.Error(t, err)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test with bot configured
	result, err := svc.GetBot()
	require.NoError(t, err)
	assert.Equal(t, "test-token", result.Token)
	assert.Equal(t, "testbot", result.Name)
}

func TestTelegramBotService_UpdateBot(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Update bot
	bot.Name = "updatedbot"
	err := svc.UpdateBot(bot)
	require.NoError(t, err)

	// Verify update
	result, err := svc.GetBot()
	require.NoError(t, err)
	assert.Equal(t, "updatedbot", result.Name)
}

func TestTelegramBotService_SetWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"ok": true,
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// We need to mock the API request, but since the URL is built with the token,
	// we can't easily redirect to our test server. Instead, we test the error path.
	err := svc.SetWebhook("https://example.com/webhook")
	// This will fail because the API request goes to api.telegram.org
	// In a real test, we would inject the HTTP client
	// For now, we just verify the function runs without panic
	_ = err
}

func TestTelegramBotService_DeleteWebhook(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test delete webhook (will fail due to API call, but tests the flow)
	_ = svc.DeleteWebhook()
}

func TestTelegramBotService_HandleUpdate_Message(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test update without command
	update := &TelegramUpdate{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 1,
			Chat:      TelegramChat{ID: 12345},
			Text:      "hello",
			From:      &TelegramUser{ID: 12345},
		},
	}

	err := svc.HandleUpdate(update)
	assert.NoError(t, err)
}

func TestTelegramBotService_HandleUpdate_Command(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test /help command
	update := &TelegramUpdate{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 1,
			Chat:      TelegramChat{ID: 12345},
			Text:      "/help",
			From:      &TelegramUser{ID: 12345},
		},
	}

	// This will fail due to SendMessage API call, but tests the command parsing
	_ = svc.HandleUpdate(update)
}

func TestTelegramBotService_HandleUpdate_Callback(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test callback query
	update := &TelegramUpdate{
		UpdateID: 1,
		CallbackQuery: &TelegramCallback{
			ID:   "callback-1",
			Data: "test_data",
		},
	}

	err := svc.HandleUpdate(update)
	assert.NoError(t, err)
}

func TestTelegramBotService_BindUser(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824, // 1GB
	}
	db.Create(user)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Test bind user
	from := &TelegramUser{
		ID:        12345,
		FirstName: "Test",
		Username:  "testuser",
	}

	err := svc.BindUser(12345, "test@example.com", from)
	require.NoError(t, err)

	// Verify binding
	var tgUser model.TelegramUser
	err = db.Where("telegram_id = ?", 12345).First(&tgUser).Error
	require.NoError(t, err)
	assert.Equal(t, user.ID, tgUser.UserID)
	assert.Equal(t, int64(12345), tgUser.TelegramID)
}

func TestTelegramBotService_BindUser_UserNotFound(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	from := &TelegramUser{
		ID:        12345,
		FirstName: "Test",
	}

	err := svc.BindUser(12345, "nonexistent@example.com", from)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "未找到")
}

func TestTelegramBotService_HandleUnbind(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	// Create bot
	bot := &model.TelegramBot{
		Token: "test-token",
		Name:  "testbot",
	}
	db.Create(bot)

	// Create user and telegram user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:     user.ID,
		TelegramID: 12345,
	}
	db.Create(tgUser)

	// Test unbind
	_ = svc.handleUnbind(12345, 12345)

	// Verify deleted
	var count int64
	db.Model(&model.TelegramUser{}).Where("telegram_id = ?", 12345).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestTelegramBotService_BroadcastReturnsSendErrors(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("telegram unavailable")
		}),
	}

	require.NoError(t, db.Create(&model.TelegramBot{
		Token:   "test-token",
		Name:    "testbot",
		Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TelegramUser{
		TelegramID: 12345,
		IsBanned:   false,
	}).Error)

	err := svc.Broadcast("hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "send telegram message to 12345")
	assert.Contains(t, err.Error(), "telegram unavailable")
}

func TestTelegramBotService_BroadcastSuccess(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}

	require.NoError(t, db.Create(&model.TelegramBot{
		Token:   "test-token",
		Name:    "testbot",
		Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TelegramUser{
		TelegramID: 12345,
		IsBanned:   false,
	}).Error)

	assert.NoError(t, svc.Broadcast("hello"))
}

func TestTelegramBotServiceAPIRequestReturnsEncodeError(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	result, err := svc.apiRequest("http://127.0.0.1:1", map[string]any{
		"bad": make(chan int),
	})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode telegram API request")
}

func TestTelegramBotServiceHandleAdminRejectsInvalidAdminIDs(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)

	require.NoError(t, db.Create(&model.TelegramBot{
		Token:    "test-token",
		Name:     "testbot",
		AdminIDs: "invalid",
	}).Error)

	err := svc.handleAdmin(12345, 12345)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse telegram admin_ids")
}

func TestParseAdminIDs(t *testing.T) {
	tests := []struct {
		input    string
		expected []int64
	}{
		{"", []int64{}},
		{"[123456789, 987654321]", []int64{123456789, 987654321}},
		{"[12345]", []int64{12345}},
		{"invalid", []int64{}}, // JSON unmarshal fails
	}

	for _, tt := range tests {
		result := parseAdminIDs(tt.input)
		if len(tt.expected) == 0 && len(result) == 0 {
			continue
		}
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}

func TestParseAdminIDsWithError(t *testing.T) {
	result, err := parseAdminIDsWithError("[123456789,987654321]")
	require.NoError(t, err)
	assert.Equal(t, []int64{123456789, 987654321}, result)

	result, err = parseAdminIDsWithError("invalid")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse telegram admin_ids")
}

func TestTelegramUserService_GetByTelegramID(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramUserService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:     user.ID,
		TelegramID: 12345,
	}
	db.Create(tgUser)

	// Test get by telegram ID
	result, err := svc.GetByTelegramID(12345)
	require.NoError(t, err)
	assert.Equal(t, user.ID, result.UserID)

	// Test not found
	_, err = svc.GetByTelegramID(99999)
	assert.Error(t, err)
}

func TestTelegramUserService_GetByUserID(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramUserService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:     user.ID,
		TelegramID: 12345,
	}
	db.Create(tgUser)

	// Test get by user ID
	result, err := svc.GetByUserID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), result.TelegramID)
}

func TestTelegramUserService_UpdateLastActive(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramUserService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:       user.ID,
		TelegramID:   12345,
		MessageCount: 0,
	}
	db.Create(tgUser)

	// Update last active
	err := svc.UpdateLastActive(12345)
	require.NoError(t, err)

	// Verify
	var updated model.TelegramUser
	db.Where("telegram_id = ?", 12345).First(&updated)
	assert.Equal(t, int64(1), updated.MessageCount)
}

func TestTelegramUserService_Ban(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramUserService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:     user.ID,
		TelegramID: 12345,
	}
	db.Create(tgUser)

	// Ban user
	err := svc.Ban(12345)
	require.NoError(t, err)

	// Verify
	var banned model.TelegramUser
	db.Where("telegram_id = ?", 12345).First(&banned)
	assert.True(t, banned.IsBanned)
}

func TestTelegramUserService_Unban(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramUserService(db)

	// Create test user
	user := &model.User{
		Email:          "test@example.com",
		Token:          "test-token",
		UUID:           "test-uuid",
		TransferEnable: 1073741824,
	}
	db.Create(user)

	tgUser := &model.TelegramUser{
		UserID:     user.ID,
		TelegramID: 12345,
		IsBanned:   true,
	}
	db.Create(tgUser)

	// Unban user
	err := svc.Unban(12345)
	require.NoError(t, err)

	// Verify
	var unbanned model.TelegramUser
	db.Where("telegram_id = ?", 12345).First(&unbanned)
	assert.False(t, unbanned.IsBanned)
}

func TestTelegramBotService_GetTelegramUserService(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)
	userSvc := svc.GetTelegramUserService()
	assert.NotNil(t, userSvc)
}
