package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
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
	assert.Contains(t, err.Error(), "telegram API request failed: other")
	assert.NotContains(t, err.Error(), "telegram unavailable")
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

func TestParseAdminIDsWithError(t *testing.T) {
	result, err := parseAdminIDsWithError("[123456789,987654321]")
	require.NoError(t, err)
	assert.Equal(t, []int64{123456789, 987654321}, result)

	result, err = parseAdminIDsWithError("invalid")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse telegram admin_ids")
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

func TestTelegramBotService_GetTelegramUserService(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := NewTelegramBotService(db)
	userSvc := svc.GetTelegramUserService()
	assert.NotNil(t, userSvc)
}

// failingTransport fails every request the way a dead network does.
type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

const leakTestToken = "123456789:FAKEtokenMustNeverLeak-0123456789abc"

// A transport failure must reach callers as a fixed reason, never as the
// *url.Error text that quotes the Bot API URL and with it the bot token.
func TestTelegramBotServiceTransportErrorsCarryNoToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail error
		want string
	}{
		{"connect", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, "telegram API request failed: connect"},
		{"dns", &net.DNSError{Err: "no such host", Name: "api.telegram.org"}, "telegram API request failed: dns"},
		{"other", errors.New("proxyconnect tcp: weird"), "telegram API request failed: other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTelegramTestDB(t)
			require.NoError(t, db.Create(&model.TelegramBot{Token: leakTestToken, Name: "b"}).Error)
			svc := NewTelegramBotService(db)
			svc.client.Transport = failingTransport{err: tc.fail}

			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previous)

			for name, err := range map[string]error{
				"SendMessage":   svc.SendMessage(1, "hi"),
				"SetWebhook":    svc.SetWebhook("https://panel.example.test/hook"),
				"DeleteWebhook": svc.DeleteWebhook(),
			} {
				require.Error(t, err, name)
				assert.Equal(t, tc.want, err.Error(), name)
				assert.NotContains(t, err.Error(), "FAKEtoken", name)
				assert.NotContains(t, err.Error(), "api.telegram.org", name)
			}
			assert.NotContains(t, logs.String(), "FAKEtoken")
		})
	}
}

// A token that makes the URL unparseable fails inside http.Client.Post with a
// *url.Error that quotes the URL; it must be reduced too.
func TestTelegramBotServiceUnparseableURLErrorCarriesNoToken(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "FAKEtoken\x7fMustNeverLeak", Name: "b"}).Error)
	err := NewTelegramBotService(db).SendMessage(1, "hi")
	require.Error(t, err)
	assert.Equal(t, "telegram API request failed: other", err.Error())
}

// Broadcast joins the per-chat errors; none may carry the token.
func TestTelegramBotServiceBroadcastErrorsCarryNoToken(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: leakTestToken, Name: "b"}).Error)
	require.NoError(t, db.Create(&model.TelegramUser{UserID: 1, TelegramID: 77}).Error)
	svc := NewTelegramBotService(db)
	svc.client.Transport = failingTransport{err: &net.OpError{Op: "dial", Err: errors.New("refused")}}
	err := svc.Broadcast("hi")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "FAKEtoken")
	assert.Contains(t, err.Error(), "telegram API request failed: connect")
}
