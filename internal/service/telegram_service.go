package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// TelegramBotService Telegram Bot服务
type TelegramBotService struct {
	db     *gorm.DB
	client *http.Client
	// apiBase overrides the Bot API address of ProbeSend (tests only).
	apiBase string
}

// NewTelegramBotService 创建服务
func NewTelegramBotService(db *gorm.DB) *TelegramBotService {
	return &TelegramBotService{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// The Bot API host is fixed; a redirect is never followed.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// GetBot 获取Bot配置
func (s *TelegramBotService) GetBot() (*model.TelegramBot, error) {
	var bot model.TelegramBot
	err := s.db.First(&bot).Error
	if err != nil {
		return nil, err
	}
	return &bot, nil
}

// UpdateBot 更新Bot配置
func (s *TelegramBotService) UpdateBot(bot *model.TelegramBot) error {
	return s.db.Save(bot).Error
}

// TelegramWebhookSecret is the secret_token registered with setWebhook. Telegram
// sends it back in X-Telegram-Bot-Api-Secret-Token with every update. It is derived
// from the bot token, so the webhook keeps no extra state. Changing the token
// changes the secret: set the webhook again afterwards.
func TelegramWebhookSecret(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("telegram-webhook-secret"))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidWebhookSecret reports whether an update carries the secret registered
// for the current bot token. With no bot configured nothing is valid.
func (s *TelegramBotService) ValidWebhookSecret(header string) bool {
	bot, err := s.GetBot()
	if err != nil || bot.Token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(TelegramWebhookSecret(bot.Token))) == 1
}

// SetWebhook 设置Webhook
func (s *TelegramBotService) SetWebhook(webhookURL string) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", bot.Token)
	payload := map[string]any{
		"url":          webhookURL,
		"secret_token": TelegramWebhookSecret(bot.Token),
	}

	_, err = s.apiRequest(url, payload)
	if err != nil {
		return err
	}

	// 更新配置
	bot.WebhookURL = webhookURL
	bot.WebhookSet = true
	return s.db.Save(bot).Error
}

// DeleteWebhook 删除Webhook
func (s *TelegramBotService) DeleteWebhook() error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", bot.Token)
	_, err = s.apiRequest(url, nil)
	if err != nil {
		return err
	}

	bot.WebhookSet = false
	return s.db.Save(bot).Error
}

// HandleUpdate 处理Webhook更新
func (s *TelegramBotService) HandleUpdate(update *TelegramUpdate) error {
	// 保存聊天记录
	if update.Message != nil {
		chat := &model.TelegramChat{
			TelegramID:  update.Message.Chat.ID,
			MessageID:   update.Message.MessageID,
			MessageType: "text",
			Content:     update.Message.Text,
		}
		if update.Message.From != nil {
			chat.TelegramID = update.Message.From.ID
		}
		if err := s.db.Create(chat).Error; err != nil {
			return err
		}
	}

	// 处理命令
	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/") {
		return s.handleCommand(update)
	}

	// 处理回调查询
	if update.CallbackQuery != nil {
		return s.handleCallback(update)
	}

	return nil
}

// handleCommand 处理命令
func (s *TelegramBotService) handleCommand(update *TelegramUpdate) error {
	if update.Message == nil || update.Message.From == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	text := update.Message.Text

	// 解析命令
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}
	// In groups Telegram sends "/cmd@BotName"; the command is the part
	// before the @.
	command := strings.ToLower(strings.SplitN(parts[0], "@", 2)[0])

	switch command {
	case "/start":
		if len(parts) > 1 && strings.HasPrefix(parts[1], telegramBindCodePrefix) {
			return s.handleStartBind(chatID, userID, update.Message.From, update.Message.Chat.Type, parts[1])
		}
		return s.handleStart(chatID, userID, update.Message.From)
	case "/help":
		return s.handleHelp(chatID)
	case "/bind":
		return s.handleBind(chatID, userID, update.Message.From, update.Message.Chat.Type, parts[1:])
	case "/unbind":
		return s.handleUnbind(chatID, userID)
	case "/info":
		return s.handleInfo(chatID, userID)
	case "/sub":
		return s.handleSub(chatID, userID)
	case "/ticket":
		return s.handleTicket(chatID, userID)
	case "/admin":
		return s.handleAdmin(chatID, userID)
	default:
		return s.SendMessage(chatID, "未知命令，请使用 /help 查看帮助")
	}
}

// handleStart 处理 /start 命令
func (s *TelegramBotService) handleStart(chatID int64, userID int64, from *TelegramUser) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	// 检查是否已绑定
	var tgUser model.TelegramUser
	result := s.db.Where("telegram_id = ?", userID).First(&tgUser)

	welcomeMsg := bot.WelcomeMsg
	if welcomeMsg == "" {
		welcomeMsg = `欢迎使用 {app_name} Bot!

可用命令:
/start - 开始使用
/help - 帮助信息
/bind <订阅链接> - 绑定账户
/unbind - 解绑账户
/info - 查看账户信息
/sub - 获取订阅链接
/ticket - 创建工单`
	}

	if result.Error == nil {
		welcomeMsg += "\n\n✅ 您已绑定账户"
	} else {
		welcomeMsg += "\n\n⚠️ 您尚未绑定账户，请私聊发送 /bind <订阅链接> 绑定"
	}

	return s.SendMessage(chatID, welcomeMsg)
}

// handleStartBind binds the account of a panel bind link: the member opened
// https://t.me/<bot>?start=<code>, and Telegram sent "/start <code>".
func (s *TelegramBotService) handleStartBind(chatID int64, userID int64, from *TelegramUser, chatType string, code string) error {
	if chatType != "" && chatType != "private" {
		return s.SendMessage(chatID, "❌ 请私聊机器人完成绑定")
	}
	user, err := s.BindUserByCode(userID, code, from, time.Now())
	if err != nil {
		return s.SendMessage(chatID, "❌ "+err.Error())
	}
	return s.SendMessage(chatID, fmt.Sprintf("✅ 绑定成功: `%s`\n\n现在可以使用 /info 和 /sub", markdownCode(user.Email)))
}

// handleHelp 处理 /help 命令
func (s *TelegramBotService) handleHelp(chatID int64) error {
	helpText := `📖 命令帮助

/start - 开始使用Bot
/help - 显示此帮助信息
/bind <订阅链接> - 绑定您的账户
  也可以在面板「账户」页点「绑定 Telegram」
/unbind - 解除账户绑定
/info - 查看账户信息
  - 套餐信息
  - 流量使用情况
  - 到期时间
/sub - 获取订阅链接
/ticket - 创建工单
  - 回复工单消息

💡 提示:
- 绑定后可接收到期、流量不足通知
- 订阅链接请勿泄露给他人`

	return s.SendMessage(chatID, helpText)
}

// handleBind 处理 /bind 命令: /bind <订阅链接或订阅令牌>.
//
// The subscription token is the proof of ownership: an e-mail address alone
// is not, since anyone who knows a member's address could bind the account
// and then read its subscription link with /sub.
func (s *TelegramBotService) handleBind(chatID int64, userID int64, from *TelegramUser, chatType string, args []string) error {
	// 检查是否已绑定
	var existingUser model.TelegramUser
	if err := s.db.Where("telegram_id = ?", userID).First(&existingUser).Error; err == nil {
		return s.SendMessage(chatID, "❌ 您已绑定账户，如需更换请先 /unbind")
	}

	bot, err := s.GetBot()
	if err != nil {
		return err
	}
	if !bot.AllowBind {
		return s.SendMessage(chatID, "❌ 管理员已关闭账户绑定")
	}
	if chatType != "" && chatType != "private" {
		return s.SendMessage(chatID, "❌ 为保护您的订阅令牌，请私聊机器人发送 /bind")
	}

	if len(args) == 0 {
		return s.SendMessage(chatID, telegramBindUsage)
	}
	token := SubscribeTokenFromBindArg(args[0])
	if token == "" {
		return s.SendMessage(chatID, telegramBindUsage)
	}

	user, err := s.BindUserByToken(userID, token, from)
	if err != nil {
		return s.SendMessage(chatID, "❌ "+err.Error())
	}
	return s.SendMessage(chatID, fmt.Sprintf("✅ 绑定成功: `%s`\n\n现在可以使用 /info 和 /sub", markdownCode(user.Email)))
}

const telegramBindUsage = "请发送您的订阅链接或订阅令牌来绑定账户（在面板的订阅页面复制）:\n格式: /bind 订阅链接\n\n⚠️ 订阅令牌等同于密码，请只在私聊中发送"

// SubscribeTokenFromBindArg is the subscription token in a /bind argument:
// the argument itself, or, for a subscription link, its token query
// parameter or else its last path segment. It is "" when nothing usable is
// there.
func SubscribeTokenFromBindArg(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return ""
	}
	if strings.Contains(arg, "://") {
		parsed, err := url.Parse(arg)
		if err != nil {
			return ""
		}
		if token := strings.TrimSpace(parsed.Query().Get("token")); token != "" {
			arg = token
		} else {
			segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			arg = segments[len(segments)-1]
		}
	}
	if arg == "" || len(arg) > 64 || strings.ContainsAny(arg, "/?#@ ") {
		return ""
	}
	return arg
}

// markdownCode makes text safe inside a Markdown (legacy) code span.
func markdownCode(text string) string {
	return strings.NewReplacer("`", "'", "\\", "/").Replace(text)
}

// BindUserByToken binds the Telegram account to the member whose
// subscription token is token.
func (s *TelegramBotService) BindUserByToken(telegramID int64, token string, from *TelegramUser) (*model.User, error) {
	var user model.User
	if err := s.db.Where("token = ?", token).First(&user).Error; err != nil {
		return nil, fmt.Errorf("订阅令牌无效，请从面板重新复制订阅链接")
	}
	if err := s.bind(telegramID, &user, from); err != nil {
		return nil, err
	}
	return &user, nil
}

// BindUser 绑定用户
func (s *TelegramBotService) BindUser(telegramID int64, email string, from *TelegramUser) error {
	// 查找用户
	var user model.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return fmt.Errorf("未找到该邮箱对应的账户")
	}
	return s.bind(telegramID, &user, from)
}

// bind records the binding of a Telegram account to user.
func (s *TelegramBotService) bind(telegramID int64, user *model.User, from *TelegramUser) error {
	// 检查是否已被其他Telegram绑定
	var existingBind model.TelegramUser
	if err := s.db.Where("user_id = ?", user.ID).First(&existingBind).Error; err == nil {
		return ErrTelegramUserAlreadyBound
	}
	if err := s.db.Where("telegram_id = ?", telegramID).First(&existingBind).Error; err == nil {
		return ErrTelegramIDAlreadyBound
	}

	// 创建绑定
	tgUser := &model.TelegramUser{
		UserID:       user.ID,
		TelegramID:   telegramID,
		Username:     from.Username,
		FirstName:    from.FirstName,
		LastName:     from.LastName,
		LanguageCode: from.LanguageCode,
	}

	if err := s.db.Create(tgUser).Error; err != nil {
		return err
	}

	// 更新Bot统计
	if err := s.db.Model(&model.TelegramBot{}).Where("id = ?", 1).
		UpdateColumn("total_users", gorm.Expr("total_users + 1")).Error; err != nil {
		return err
	}

	return nil
}

// handleUnbind 处理 /unbind 命令
func (s *TelegramBotService) handleUnbind(chatID int64, userID int64) error {
	result := s.db.Where("telegram_id = ?", userID).Delete(&model.TelegramUser{})
	if result.RowsAffected == 0 {
		return s.SendMessage(chatID, "❌ 您尚未绑定账户")
	}
	return s.SendMessage(chatID, "✅ 已解除绑定")
}

// handleInfo 处理 /info 命令
func (s *TelegramBotService) handleInfo(chatID int64, userID int64) error {
	// 获取绑定用户
	var tgUser model.TelegramUser
	if err := s.db.Where("telegram_id = ?", userID).Preload("User").First(&tgUser).Error; err != nil {
		return s.SendMessage(chatID, "❌ 您尚未绑定账户，请先使用 /bind 绑定")
	}

	// 获取用户信息
	user := tgUser.User
	info := fmt.Sprintf(`📊 账户信息

邮箱: %s
余额: %.2f 元
流量: %.2f GB / %.2f GB
上行: %.2f GB
下行: %.2f GB`,
		user.Email,
		float64(user.Balance)/100,
		float64(user.U+user.D)/float64(1073741824),
		float64(user.TransferEnable)/float64(1073741824),
		float64(user.U)/float64(1073741824),
		float64(user.D)/float64(1073741824),
	)

	if user.ExpiredAt != nil && *user.ExpiredAt > 0 {
		expireTime := time.Unix(*user.ExpiredAt, 0)
		info += fmt.Sprintf("\n到期: %s", expireTime.Format("2006-01-02"))
	}

	return s.SendMessage(chatID, info)
}

// handleSub 处理 /sub 命令
func (s *TelegramBotService) handleSub(chatID int64, userID int64) error {
	var tgUser model.TelegramUser
	if err := s.db.Where("telegram_id = ?", userID).Preload("User").First(&tgUser).Error; err != nil {
		return s.SendMessage(chatID, "❌ 您尚未绑定账户，请先使用 /bind 绑定")
	}

	// 获取用户token
	user := tgUser.User
	subURL := s.subscribeURL(user.Token)
	if subURL == "" {
		return s.SendMessage(chatID, "❌ 面板未配置订阅域名，请在网页端复制订阅链接")
	}

	return s.SendMessage(chatID, fmt.Sprintf("🔗 您的订阅链接:\n\n`%s`\n\n⚠️ 请勿泄露给他人", subURL))
}

// subscribeURL is the member's subscription link: the first configured
// subscription domain (app.subscribe_domains), else the panel's domain, and
// the subscription path. It is "" when neither domain is set.
func (s *TelegramBotService) subscribeURL(token string) string {
	cfg := config.Get()
	settings := GetSubscriptionSettings(NewSystemConfigService(s.db), cfg)
	host := ""
	if len(settings.SubscribeDomains) > 0 {
		host = settings.SubscribeDomains[0]
	} else if cfg != nil {
		host = normalizeSubscriptionHost(cfg.Domain)
	}
	if host == "" || token == "" {
		return ""
	}
	return fmt.Sprintf("https://%s%s/%s", host, settings.SubscribePath, token)
}

// handleTicket 处理 /ticket 命令
func (s *TelegramBotService) handleTicket(chatID int64, userID int64) error {
	return s.SendMessage(chatID, "工单功能开发中，请通过网页端提交工单")
}

// handleAdmin 处理 /admin 命令
func (s *TelegramBotService) handleAdmin(chatID int64, userID int64) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	// 检查是否为管理员
	adminIDs, err := parseAdminIDsWithError(bot.AdminIDs)
	if err != nil {
		return err
	}
	isAdmin := false
	for _, id := range adminIDs {
		if id == userID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return s.SendMessage(chatID, "❌ 权限不足")
	}

	adminText := `🔧 管理员面板

/stat - 查看统计
/broadcast <消息> - 广播消息
/user <邮箱> - 查询用户
/ban <邮箱> - 封禁用户
/unban <邮箱> - 解封用户`

	return s.SendMessage(chatID, adminText)
}

// handleCallback 处理回调查询
func (s *TelegramBotService) handleCallback(update *TelegramUpdate) error {
	// Unimplemented: Telegram inline button callback handling.
	// When implemented, this should parse update.CallbackQuery.Data to determine
	// the action (e.g., confirm subscription, view account, ticket operations)
	// and respond with an appropriate AnswerCallbackQuery + edit message.
	log.Printf("[telegram] callback query received but handler not implemented: chat_id=%d", update.CallbackQuery.Message.Chat.ID)
	return nil
}

// SendMessage 发送消息
func (s *TelegramBotService) SendMessage(chatID int64, text string) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", bot.Token)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	_, err = s.apiRequest(url, payload)
	return err
}

// SendNotification 发送通知
func (s *TelegramBotService) SendNotification(telegramID int64, title, content string) error {
	text := fmt.Sprintf("📢 *%s*\n\n%s", title, content)
	return s.SendMessage(telegramID, text)
}

// Broadcast 广播消息
func (s *TelegramBotService) Broadcast(text string) error {
	var users []model.TelegramUser
	if err := s.db.Where("is_banned = ?", false).Find(&users).Error; err != nil {
		return err
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(users))
	for _, user := range users {
		wg.Add(1)
		go func(telegramID int64) {
			defer wg.Done()
			if err := s.SendMessage(telegramID, text); err != nil {
				errs <- fmt.Errorf("send telegram message to %d: %w", telegramID, err)
			}
			time.Sleep(50 * time.Millisecond) // 避免频率限制
		}(user.TelegramID)
	}
	wg.Wait()
	close(errs)

	var sendErrs []error
	for err := range errs {
		sendErrs = append(sendErrs, err)
	}
	return errors.Join(sendErrs...)
}

// apiRequest 发送API请求
func (s *TelegramBotService) apiRequest(url string, payload any) (map[string]any, error) {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return nil, fmt.Errorf("encode telegram API request: %w", err)
		}
	}

	resp, err := s.client.Post(url, "application/json", &body)
	if err != nil {
		// err is a *url.Error whose text quotes the URL, and with it the bot token.
		return nil, telegramTransportError(err)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("decode telegram API response: %w; close response body: %v", err, closeErr)
		}
		return nil, err
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close telegram API response body: %w", err)
	}

	return result, nil
}

func parseAdminIDsWithError(ids string) ([]int64, error) {
	if ids == "" {
		return []int64{}, nil
	}
	var adminIDs []int64
	if err := json.Unmarshal([]byte(ids), &adminIDs); err != nil {
		return nil, fmt.Errorf("parse telegram admin_ids: %w", err)
	}
	return adminIDs, nil
}

// ========== Telegram API 类型定义 ==========

// TelegramUpdate Telegram更新
type TelegramUpdate struct {
	UpdateID      int64             `json:"update_id"`
	Message       *TelegramMessage  `json:"message,omitempty"`
	CallbackQuery *TelegramCallback `json:"callback_query,omitempty"`
}

// TelegramMessage Telegram消息
type TelegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *TelegramUser `json:"from,omitempty"`
	Chat      TelegramChat  `json:"chat"`
	Date      int64         `json:"date"`
	Text      string        `json:"text,omitempty"`
}

// TelegramUser Telegram用户
type TelegramUser struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// TelegramChat Telegram聊天
type TelegramChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// TelegramCallback Telegram回调
type TelegramCallback struct {
	ID      string          `json:"id"`
	From    TelegramUser    `json:"from"`
	Message TelegramMessage `json:"message"`
	Data    string          `json:"data"`
}

// GetTelegramUserService 获取Telegram用户服务
func (s *TelegramBotService) GetTelegramUserService() *TelegramUserService {
	return NewTelegramUserService(s.db)
}

// TelegramUserService Telegram用户服务
type TelegramUserService struct {
	db *gorm.DB
}

// NewTelegramUserService 创建服务
func NewTelegramUserService(db *gorm.DB) *TelegramUserService {
	return &TelegramUserService{db: db}
}

// GetByUserID 根据系统用户ID获取绑定
func (s *TelegramUserService) GetByUserID(userID uint) (*model.TelegramUser, error) {
	var user model.TelegramUser
	err := s.db.Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
