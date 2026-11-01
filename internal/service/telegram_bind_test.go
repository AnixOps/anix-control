package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

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
