package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// TelegramBotService Telegram Bot服务
type TelegramBotService struct {
	db     *gorm.DB
	bot    *model.TelegramBot
	client *http.Client
	mu     sync.RWMutex
}

// NewTelegramBotService 创建服务
func NewTelegramBotService(db *gorm.DB) *TelegramBotService {
	return &TelegramBotService{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
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

// SetWebhook 设置Webhook
func (s *TelegramBotService) SetWebhook(webhookURL string) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", bot.Token)
	payload := map[string]interface{}{
		"url": webhookURL,
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
		s.db.Create(chat)
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
	command := parts[0]

	switch command {
	case "/start":
		return s.handleStart(chatID, userID, update.Message.From)
	case "/help":
		return s.handleHelp(chatID)
	case "/bind":
		return s.handleBind(chatID, userID, update.Message.From)
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
/bind <邮箱> - 绑定账户
/unbind - 解绑账户
/info - 查看账户信息
/sub - 获取订阅链接
/ticket - 创建工单`
	}

	if result.Error == nil {
		welcomeMsg += "\n\n✅ 您已绑定账户"
	} else {
		welcomeMsg += "\n\n⚠️ 您尚未绑定账户，请使用 /bind <邮箱> 绑定"
	}

	return s.SendMessage(chatID, welcomeMsg)
}

// handleHelp 处理 /help 命令
func (s *TelegramBotService) handleHelp(chatID int64) error {
	helpText := `📖 命令帮助

/start - 开始使用Bot
/help - 显示此帮助信息
/bind <邮箱> - 绑定您的账户
  示例: /bind user@example.com
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

// handleBind 处理 /bind 命令
func (s *TelegramBotService) handleBind(chatID int64, userID int64, from *TelegramUser) error {
	// 检查是否已绑定
	var existingUser model.TelegramUser
	if err := s.db.Where("telegram_id = ?", userID).First(&existingUser).Error; err == nil {
		return s.SendMessage(chatID, "❌ 您已绑定账户，如需更换请先 /unbind")
	}

	// 需要用户提供邮箱
	return s.SendMessage(chatID, "请输入您的注册邮箱:\n格式: /bind your@email.com")
}

// BindUser 绑定用户
func (s *TelegramBotService) BindUser(telegramID int64, email string, from *TelegramUser) error {
	// 查找用户
	var user model.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return fmt.Errorf("未找到该邮箱对应的账户")
	}

	// 检查是否已被其他Telegram绑定
	var existingBind model.TelegramUser
	if err := s.db.Where("user_id = ?", user.ID).First(&existingBind).Error; err == nil {
		return fmt.Errorf("该账户已被其他Telegram绑定")
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
	s.db.Model(&model.TelegramBot{}).Where("id = ?", 1).
		UpdateColumn("total_users", gorm.Expr("total_users + 1"))

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
	subURL := fmt.Sprintf("https://your-domain.com/s/%s", user.Token)

	return s.SendMessage(chatID, fmt.Sprintf("🔗 您的订阅链接:\n\n`%s`\n\n⚠️ 请勿泄露给他人", subURL))
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
	adminIDs := parseAdminIDs(bot.AdminIDs)
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
	// TODO: 实现回调处理
	return nil
}

// SendMessage 发送消息
func (s *TelegramBotService) SendMessage(chatID int64, text string) error {
	bot, err := s.GetBot()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", bot.Token)
	payload := map[string]interface{}{
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
	s.db.Where("is_banned = ?", false).Find(&users)

	var wg sync.WaitGroup
	for _, user := range users {
		wg.Add(1)
		go func(telegramID int64) {
			defer wg.Done()
			s.SendMessage(telegramID, text)
			time.Sleep(50 * time.Millisecond) // 避免频率限制
		}(user.TelegramID)
	}
	wg.Wait()

	return nil
}

// apiRequest 发送API请求
func (s *TelegramBotService) apiRequest(url string, payload interface{}) (map[string]interface{}, error) {
	var body bytes.Buffer
	if payload != nil {
		json.NewEncoder(&body).Encode(payload)
	}

	resp, err := s.client.Post(url, "application/json", &body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// parseAdminIDs 解析管理员ID
func parseAdminIDs(ids string) []int64 {
	if ids == "" {
		return []int64{}
	}
	var adminIDs []int64
	json.Unmarshal([]byte(ids), &adminIDs)
	return adminIDs
}

// ========== Telegram API 类型定义 ==========

// TelegramUpdate Telegram更新
type TelegramUpdate struct {
	UpdateID      int64               `json:"update_id"`
	Message       *TelegramMessage    `json:"message,omitempty"`
	CallbackQuery *TelegramCallback   `json:"callback_query,omitempty"`
}

// TelegramMessage Telegram消息
type TelegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      *TelegramUser `json:"from,omitempty"`
	Chat      TelegramChat `json:"chat"`
	Date      int64        `json:"date"`
	Text      string       `json:"text,omitempty"`
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

// GetByTelegramID 根据TelegramID获取用户
func (s *TelegramUserService) GetByTelegramID(telegramID int64) (*model.TelegramUser, error) {
	var user model.TelegramUser
	err := s.db.Where("telegram_id = ?", telegramID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
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

// UpdateLastActive 更新最后活跃时间
func (s *TelegramUserService) UpdateLastActive(telegramID int64) error {
	return s.db.Model(&model.TelegramUser{}).
		Where("telegram_id = ?", telegramID).
		Updates(map[string]interface{}{
			"last_active":   time.Now(),
			"message_count": gorm.Expr("message_count + 1"),
		}).Error
}

// Ban 封禁用户
func (s *TelegramUserService) Ban(telegramID int64) error {
	now := time.Now()
	return s.db.Model(&model.TelegramUser{}).
		Where("telegram_id = ?", telegramID).
		Updates(map[string]interface{}{
			"is_banned": true,
			"banned_at": now,
		}).Error
}

// Unban 解封用户
func (s *TelegramUserService) Unban(telegramID int64) error {
	return s.db.Model(&model.TelegramUser{}).
		Where("telegram_id = ?", telegramID).
		Updates(map[string]interface{}{
			"is_banned": false,
			"banned_at": nil,
		}).Error
}