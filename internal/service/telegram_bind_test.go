package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSends answers every Bot API call with ok and records the text of
// each sendMessage.
func captureSends(svc *TelegramBotService) *[]string {
	sent := &[]string{}
	svc.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var payload map[string]any
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &payload)
		if text, ok := payload["text"].(string); ok {
			*sent = append(*sent, text)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`))}, nil
	})
	return sent
}

func bindUpdate(chatType, text string) *TelegramUpdate {
	return &TelegramUpdate{Message: &TelegramMessage{
		From: &TelegramUser{ID: 4242, FirstName: "T"},
		Chat: TelegramChat{ID: 4242, Type: chatType},
		Text: text,
	}}
}

func TestSubscribeTokenFromBindArg(t *testing.T) {
	for arg, want := range map[string]string{
		"abc123":                            "abc123",
		" abc123 ":                          "abc123",
		"https://sub.example.com/s/abc123":  "abc123",
		"https://sub.example.com/s/abc123/": "abc123",
		"https://sub.example.com/api/v1/client/subscribe?token=tok9": "tok9",
		"admin@example.com":        "",
		"https://sub.example.com/": "",
		"":                         "",
		strings.Repeat("a", 65):    "",
	} {
		assert.Equal(t, want, SubscribeTokenFromBindArg(arg), arg)
	}
}

func TestHandleBindBindsBySubscriptionLink(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "t", Name: "b"}).Error)
	user := &model.User{Email: "member@example.com", Token: "tok-member", UUID: "u-1"}
	require.NoError(t, db.Create(user).Error)
	svc := NewTelegramBotService(db)
	sent := captureSends(svc)

	require.NoError(t, svc.HandleUpdate(bindUpdate("private", "/bind@anix_bot https://sub.example.com/s/tok-member")))

	var binding model.TelegramUser
	require.NoError(t, db.Where("telegram_id = ?", 4242).First(&binding).Error)
	assert.Equal(t, user.ID, binding.UserID)
	require.Len(t, *sent, 1)
	assert.Contains(t, (*sent)[0], "绑定成功")
}

func TestHandleBindRefusesEmailUnknownTokenAndGroups(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "t", Name: "b"}).Error)
	require.NoError(t, db.Create(&model.User{Email: "member@example.com", Token: "tok-member", UUID: "u-1"}).Error)
	svc := NewTelegramBotService(db)
	sent := captureSends(svc)

	require.NoError(t, svc.HandleUpdate(bindUpdate("private", "/bind member@example.com")))
	require.NoError(t, svc.HandleUpdate(bindUpdate("private", "/bind nope")))
	require.NoError(t, svc.HandleUpdate(bindUpdate("group", "/bind tok-member")))

	var count int64
	db.Model(&model.TelegramUser{}).Count(&count)
	assert.Zero(t, count)
	require.Len(t, *sent, 3)
	assert.Contains(t, (*sent)[0], "订阅链接")
	assert.Contains(t, (*sent)[1], "订阅令牌无效")
	assert.Contains(t, (*sent)[2], "私聊")
}

func TestHandleBindHonoursAllowBind(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "t", Name: "b"}).Error)
	require.NoError(t, db.Model(&model.TelegramBot{}).Where("1 = 1").Update("allow_bind", false).Error)
	require.NoError(t, db.Create(&model.User{Email: "member@example.com", Token: "tok-member", UUID: "u-1"}).Error)
	svc := NewTelegramBotService(db)
	sent := captureSends(svc)

	require.NoError(t, svc.HandleUpdate(bindUpdate("private", "/bind tok-member")))
	require.Len(t, *sent, 1)
	assert.Contains(t, (*sent)[0], "关闭")
}

func TestTelegramBindCodeRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	code := telegramBindCode("tok", 42, now.Add(TelegramBindCodeTTL))
	assert.LessOrEqual(t, len(code), 64, "Telegram start parameters are at most 64 characters")
	assert.Regexp(t, `^[A-Za-z0-9_-]+$`, code)

	userID, ok := verifyTelegramBindCode("tok", code, now)
	require.True(t, ok)
	assert.Equal(t, uint(42), userID)

	_, ok = verifyTelegramBindCode("tok", code, now.Add(TelegramBindCodeTTL))
	assert.False(t, ok, "expired")
	_, ok = verifyTelegramBindCode("other-token", code, now)
	assert.False(t, ok, "another bot token")
	tampered := code[:len(code)-2] + "AA"
	_, ok = verifyTelegramBindCode("tok", tampered, now)
	assert.False(t, ok, "tampered")
	_, ok = verifyTelegramBindCode("tok", "b_short", now)
	assert.False(t, ok)
}

func TestStartWithBindCodeBindsTheSender(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "t", Name: "b"}).Error)
	user := &model.User{Email: "member@example.com", Token: "tok-member", UUID: "u-1"}
	require.NoError(t, db.Create(user).Error)
	svc := NewTelegramBotService(db)
	sent := captureSends(svc)

	code := telegramBindCode("t", user.ID, time.Now().Add(time.Minute))
	require.NoError(t, svc.HandleUpdate(bindUpdate("group", "/start "+code)))
	require.NoError(t, svc.HandleUpdate(bindUpdate("private", "/start "+code)))

	var binding model.TelegramUser
	require.NoError(t, db.Where("telegram_id = ?", 4242).First(&binding).Error)
	assert.Equal(t, user.ID, binding.UserID)
	require.Len(t, *sent, 2)
	assert.Contains(t, (*sent)[0], "私聊")
	assert.Contains(t, (*sent)[1], "绑定成功")

	// The same Telegram account cannot bind a second member.
	other := &model.User{Email: "other@example.com", Token: "tok-other", UUID: "u-2"}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Where("telegram_id = ?", 4242).Delete(&model.TelegramUser{}).Error)
	require.NoError(t, db.Create(&model.TelegramUser{UserID: user.ID, TelegramID: 4242}).Error)
	_, err := svc.AdminBind(other.ID, 4242)
	assert.ErrorIs(t, err, ErrTelegramIDAlreadyBound)
	_, err = svc.AdminBind(user.ID, 777)
	assert.ErrorIs(t, err, ErrTelegramUserAlreadyBound)
}

func TestBindLinkAndUnbind(t *testing.T) {
	db := setupTelegramTestDB(t)
	require.NoError(t, db.Create(&model.TelegramBot{Token: "t", Name: "b"}).Error)
	user := &model.User{Email: "member@example.com", Token: "tok-member", UUID: "u-1"}
	require.NoError(t, db.Create(user).Error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bott/getMe", r.URL.Path)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"anix_bot"}}`))
	}))
	defer server.Close()
	svc := NewTelegramBotService(db)
	svc.apiBase = server.URL

	now := time.Now()
	link, err := svc.BindLink(user.ID, now)
	require.NoError(t, err)
	assert.Equal(t, "anix_bot", link.BotUsername)
	assert.True(t, strings.HasPrefix(link.URL, "https://t.me/anix_bot?start=b_"))
	code := strings.TrimPrefix(link.Command, "/start ")
	bound, err := svc.BindUserByCode(99, code, &TelegramUser{ID: 99}, now)
	require.NoError(t, err)
	assert.Equal(t, user.ID, bound.ID)

	_, err = svc.BindLink(user.ID, now)
	assert.ErrorIs(t, err, ErrTelegramUserAlreadyBound)
	require.NoError(t, svc.UnbindUser(user.ID))
	assert.ErrorIs(t, svc.UnbindUser(user.ID), ErrTelegramNotBound)

	require.NoError(t, db.Model(&model.TelegramBot{}).Where("1 = 1").Update("allow_bind", false).Error)
	_, err = svc.BindLink(user.ID, now)
	assert.ErrorIs(t, err, ErrTelegramBindDisabled)
}
