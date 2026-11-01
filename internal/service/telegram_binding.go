package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Binding a Telegram account to a member, besides /bind with the
// subscription link:
//
//   - The member asks the panel for a bind link: a t.me deep link to the bot
//     whose start parameter is a bind code. Opening it sends "/start <code>"
//     from the member's Telegram account, and the bot binds that account.
//   - An administrator binds a member to a Telegram user ID by hand.
//
// A bind code is stateless: the member ID and an expiry, signed with a key
// derived from the bot token. It is valid for TelegramBindCodeTTL; changing
// the bot token invalidates every code.

// TelegramBindCodeTTL is how long a bind link works.
const TelegramBindCodeTTL = 10 * time.Minute

// telegramBindCodePrefix starts every bind code, so /start can tell a code
// from any other start parameter.
const telegramBindCodePrefix = "b_"

// Errors of the binding operations. Their messages are shown to members and
// administrators as they are.
var (
	ErrTelegramBotNotConfigured = errors.New("尚未配置 Telegram 机器人")
	ErrTelegramBindDisabled     = errors.New("管理员已关闭账户绑定")
	ErrTelegramBindCodeInvalid  = errors.New("绑定链接无效或已过期，请在面板重新获取")
	ErrTelegramUserNotFound     = errors.New("用户不存在")
	ErrTelegramUserAlreadyBound = errors.New("该账户已绑定 Telegram，请先解绑")
	ErrTelegramIDAlreadyBound   = errors.New("该 Telegram 账号已绑定其他账户，请先解绑")
	ErrTelegramNotBound         = errors.New("尚未绑定 Telegram")
)

func telegramBindKey(botToken string) []byte {
	mac := hmac.New(sha256.New, []byte(botToken))
	mac.Write([]byte("telegram-bind-code"))
	return mac.Sum(nil)
}

// telegramBindCode is the bind code of userID that expires at expires.
func telegramBindCode(botToken string, userID uint, expires time.Time) string {
	payload := binary.AppendUvarint(nil, uint64(userID))
	payload = binary.AppendVarint(payload, expires.Unix())
	mac := hmac.New(sha256.New, telegramBindKey(botToken))
	mac.Write(payload)
	signed := append(payload, mac.Sum(nil)[:telegramBindMACSize]...)
	return telegramBindCodePrefix + base64.RawURLEncoding.EncodeToString(signed)
}

// telegramBindMACSize is the length of a bind code's truncated HMAC.
const telegramBindMACSize = 16

// verifyTelegramBindCode is the member ID of a valid, unexpired code.
func verifyTelegramBindCode(botToken, code string, now time.Time) (uint, bool) {
	if !strings.HasPrefix(code, telegramBindCodePrefix) {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, telegramBindCodePrefix))
	if err != nil || len(raw) <= telegramBindMACSize {
		return 0, false
	}
	payload, sum := raw[:len(raw)-telegramBindMACSize], raw[len(raw)-telegramBindMACSize:]
	mac := hmac.New(sha256.New, telegramBindKey(botToken))
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil)[:telegramBindMACSize], sum) {
		return 0, false
	}
	id, n := binary.Uvarint(payload)
	if n <= 0 || id == 0 || id > math.MaxUint32 {
		return 0, false
	}
	unix, m := binary.Varint(payload[n:])
	if m <= 0 || n+m != len(payload) || !now.Before(time.Unix(unix, 0)) {
		return 0, false
	}
	return uint(id), true
}

// TelegramBindLink is what a member opens to bind their Telegram account.
type TelegramBindLink struct {
	// URL is the t.me deep link; it is empty when the bot's username could
	// not be read, and the member sends Command to the bot instead.
	URL         string    `json:"url"`
	Command     string    `json:"command"`
	BotUsername string    `json:"bot_username"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// BindLink is a bind link for userID, valid for TelegramBindCodeTTL.
func (s *TelegramBotService) BindLink(userID uint, now time.Time) (*TelegramBindLink, error) {
	bot, err := s.GetBot()
	if err != nil || strings.TrimSpace(bot.Token) == "" {
		return nil, ErrTelegramBotNotConfigured
	}
	if !bot.AllowBind {
		return nil, ErrTelegramBindDisabled
	}
	var user model.User
	if err := s.db.Select("id").First(&user, userID).Error; err != nil {
		return nil, ErrTelegramUserNotFound
	}
	if s.userBound(userID) {
		return nil, ErrTelegramUserAlreadyBound
	}
	expires := now.Add(TelegramBindCodeTTL)
	code := telegramBindCode(bot.Token, userID, expires)
	link := &TelegramBindLink{Command: "/start " + code, ExpiresAt: expires.UTC()}
	if username := s.BotUsername(); username != "" {
		link.BotUsername = username
		link.URL = fmt.Sprintf("https://t.me/%s?start=%s", username, code)
	}
	return link, nil
}

// BotUsername is the bot's @username from getMe, or "" when Telegram could
// not be asked.
func (s *TelegramBotService) BotUsername() string {
	bot, err := s.GetBot()
	if err != nil || strings.TrimSpace(bot.Token) == "" {
		return ""
	}
	base := s.apiBase
	if base == "" {
		base = telegramAPIHost
	}
	resp, err := s.client.Get(base + "/bot" + bot.Token + "/getMe")
	if err != nil {
		// The error carries the URL and so the token: never log it.
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var answer struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&answer); err != nil || !answer.OK {
		return ""
	}
	username := strings.TrimSpace(answer.Result.Username)
	for _, r := range username {
		if !telegramUsernameRune(r) {
			return ""
		}
	}
	return username
}

// BindUserByCode binds the Telegram account to the member of a bind code.
func (s *TelegramBotService) BindUserByCode(telegramID int64, code string, from *TelegramUser, now time.Time) (*model.User, error) {
	bot, err := s.GetBot()
	if err != nil {
		return nil, ErrTelegramBotNotConfigured
	}
	if !bot.AllowBind {
		return nil, ErrTelegramBindDisabled
	}
	userID, ok := verifyTelegramBindCode(bot.Token, code, now)
	if !ok {
		return nil, ErrTelegramBindCodeInvalid
	}
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, ErrTelegramBindCodeInvalid
	}
	if err := s.bind(telegramID, &user, from); err != nil {
		return nil, err
	}
	return &user, nil
}

// AdminBind binds member userID to the Telegram user telegramID by hand.
// The administrator's AllowBind switch does not apply.
func (s *TelegramBotService) AdminBind(userID uint, telegramID int64) (*model.TelegramUser, error) {
	if telegramID == 0 {
		return nil, errors.New("无效的 Telegram ID")
	}
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramUserNotFound
		}
		return nil, err
	}
	if err := s.bind(telegramID, &user, &TelegramUser{ID: telegramID}); err != nil {
		return nil, err
	}
	var binding model.TelegramUser
	if err := s.db.Where("user_id = ?", userID).First(&binding).Error; err != nil {
		return nil, err
	}
	return &binding, nil
}

// UnbindUser removes the binding of member userID.
func (s *TelegramBotService) UnbindUser(userID uint) error {
	result := s.db.Where("user_id = ?", userID).Delete(&model.TelegramUser{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTelegramNotBound
	}
	return nil
}

func (s *TelegramBotService) userBound(userID uint) bool {
	var count int64
	s.db.Model(&model.TelegramUser{}).Where("user_id = ?", userID).Count(&count)
	return count > 0
}

func telegramUsernameRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
