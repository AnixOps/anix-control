package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Bot is a v2_telegram_bot row.
type Bot struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"size:100;not null" json:"name"`
	Token   string `gorm:"size:200;not null" json:"token"`
	Enabled bool   `gorm:"default:false" json:"enabled"`

	WebhookURL string `gorm:"size:255" json:"webhook_url"`
	WebhookSet bool   `gorm:"default:false" json:"webhook_set"`

	AdminIDs string `gorm:"type:text" json:"admin_ids"`

	WelcomeMsg string `gorm:"type:text" json:"welcome_msg"`

	AllowBind   bool `gorm:"default:true" json:"allow_bind"`
	AllowSub    bool `gorm:"default:true" json:"allow_sub"`
	AllowTicket bool `gorm:"default:true" json:"allow_ticket"`
	AllowInfo   bool `gorm:"default:true" json:"allow_info"`

	TotalUsers int64 `json:"total_users"`
	TotalChats int64 `json:"total_chats"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Bot) TableName() string { return "v2_telegram_bot" }

// Binding is a v2_telegram_user row: a member's Telegram account. The
// kernel model's User relation is not loaded; the answers that show the
// member's e-mail read it from kapi_user_directory_v1.
type Binding struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	UserID       uint   `gorm:"uniqueIndex" json:"user_id"`
	TelegramID   int64  `gorm:"uniqueIndex" json:"telegram_id"`
	Username     string `gorm:"size:100" json:"username"`
	FirstName    string `gorm:"size:100" json:"first_name"`
	LastName     string `gorm:"size:100" json:"last_name"`
	LanguageCode string `gorm:"size:10" json:"language_code"`

	IsBanned bool       `gorm:"default:false" json:"is_banned"`
	BannedAt *time.Time `json:"banned_at"`

	NotifyExpire  bool `gorm:"default:true" json:"notify_expire"`
	NotifyTraffic bool `gorm:"default:true" json:"notify_traffic"`
	NotifyTicket  bool `gorm:"default:true" json:"notify_ticket"`

	MessageCount int64     `json:"message_count"`
	LastActive   time.Time `json:"last_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Binding) TableName() string { return "v2_telegram_user" }

// UpdateBotRequest is the bot update body. Its name is part of the
// validation messages, so it matches the kernel's handler.UpdateBotRequest.
type UpdateBotRequest struct {
	Name           string `json:"name" binding:"omitempty,max=255"`
	Token          string `json:"token" binding:"omitempty,min=1"`
	WelcomeMsg     string `json:"welcome_msg" binding:"omitempty,max=1024"`
	WelcomeMessage string `json:"welcome_message" binding:"omitempty,max=1024"`
	AdminIDs       any    `json:"admin_ids"`
	AllowBind      *bool  `json:"allow_bind"`
	AllowSub       *bool  `json:"allow_sub"`
	AllowTicket    *bool  `json:"allow_ticket"`
	AllowInfo      *bool  `json:"allow_info"`
}

// NotifySettingsRequest is the member's notification settings body; its
// name is part of the decoding messages, as in the kernel's handler.
type NotifySettingsRequest struct {
	NotifyExpire  *bool `json:"notify_expire"`
	NotifyTraffic *bool `json:"notify_traffic"`
	NotifyTicket  *bool `json:"notify_ticket"`
}

func parseTelegramBotAdminIDs(raw string) []int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []int64{}
	}

	var ids []int64
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		return ids
	}

	for _, part := range strings.Split(trimmed, ",") {
		parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			ids = append(ids, parsed)
		}
	}
	return ids
}

func parseTelegramAdminIDsInput(raw any) ([]int64, bool, error) {
	if raw == nil {
		return nil, false, nil
	}

	switch v := raw.(type) {
	case string:
		return parseTelegramBotAdminIDs(v), true, nil
	case []any:
		ids := make([]int64, 0, len(v))
		for _, item := range v {
			switch typed := item.(type) {
			case float64:
				ids = append(ids, int64(typed))
			case string:
				parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
				if err != nil {
					return nil, true, fmt.Errorf("invalid admin id: %s", typed)
				}
				ids = append(ids, parsed)
			default:
				return nil, true, fmt.Errorf("invalid admin_ids element type")
			}
		}
		return ids, true, nil
	default:
		return nil, true, fmt.Errorf("invalid admin_ids format")
	}
}

func telegramBotResponse(bot *Bot) map[string]any {
	adminIDs := parseTelegramBotAdminIDs(bot.AdminIDs)
	return map[string]any{
		"id":              bot.ID,
		"name":            bot.Name,
		"token":           bot.Token,
		"enabled":         bot.Enabled,
		"webhook_url":     bot.WebhookURL,
		"webhook_set":     bot.WebhookSet,
		"admin_ids":       adminIDs,
		"welcome_msg":     bot.WelcomeMsg,
		"welcome_message": bot.WelcomeMsg,
		"allow_bind":      bot.AllowBind,
		"allow_sub":       bot.AllowSub,
		"allow_ticket":    bot.AllowTicket,
		"allow_info":      bot.AllowInfo,
		"total_users":     bot.TotalUsers,
		"total_chats":     bot.TotalChats,
		"created_at":      bot.CreatedAt,
		"updated_at":      bot.UpdatedAt,
	}
}

func telegramUserBindingResponse(user *Binding, userEmail string) map[string]any {
	notifyEnabled := user.NotifyExpire || user.NotifyTraffic || user.NotifyTicket
	return map[string]any{
		"id":             user.ID,
		"user_id":        user.UserID,
		"user_email":     userEmail,
		"telegram_id":    user.TelegramID,
		"username":       user.Username,
		"first_name":     user.FirstName,
		"last_name":      user.LastName,
		"language_code":  user.LanguageCode,
		"is_banned":      user.IsBanned,
		"notify_expire":  user.NotifyExpire,
		"notify_traffic": user.NotifyTraffic,
		"notify_ticket":  user.NotifyTicket,
		"notify_enabled": notifyEnabled,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	}
}

func parseTelegramID(raw any) (int64, error) {
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid telegram_id")
	}
}

func extractMessage(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if inner, ok := v["message"].(string); ok {
			return strings.TrimSpace(inner)
		}
	}
	return ""
}

// userEmails reads the members' e-mail addresses from kapi_user_directory_v1
// the way the legacy handlers preload the User relation: members without a
// row (or bindings without a member) show no address.
func userEmails(db *gorm.DB, bindings []Binding) (map[uint]string, error) {
	emails := map[uint]string{}
	seen := map[uint]bool{}
	ids := make([]uint, 0, len(bindings))
	for _, binding := range bindings {
		if binding.UserID == 0 || seen[binding.UserID] {
			continue
		}
		seen[binding.UserID] = true
		ids = append(ids, binding.UserID)
	}
	if len(ids) == 0 {
		return emails, nil
	}
	var rows []struct {
		ID    uint   `gorm:"column:id"`
		Email string `gorm:"column:email"`
	}
	if err := db.Table("kapi_user_directory_v1").Select("id", "email").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		emails[row.ID] = row.Email
	}
	return emails, nil
}

// AdminGetBot is GET /api/v2/admin/telegram/bot.
func (s *Service) AdminGetBot(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	bot, err := firstBot(db)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panel(map[string]any{
				"token":           "",
				"admin_ids":       []int64{},
				"welcome_msg":     "",
				"welcome_message": "",
				"allow_bind":      true,
				"allow_sub":       true,
				"allow_ticket":    true,
				"allow_info":      true,
			})
		}
		return s.panelError(err.Error())
	}

	return s.panel(telegramBotResponse(bot))
}

// AdminUpdateBot is PUT /api/v2/admin/telegram/bot. Without a bot row it
// creates one.
func (s *Service) AdminUpdateBot(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req UpdateBotRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	bot, err := firstBot(db)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(err.Error())
		}
		bot = &Bot{
			Name:        controlName + " Bot",
			Enabled:     true,
			AllowBind:   true,
			AllowSub:    true,
			AllowTicket: true,
			AllowInfo:   true,
		}
	}

	if req.Name != "" {
		bot.Name = req.Name
	}
	if req.Token != "" {
		bot.Token = req.Token
	}
	welcomeMessage := req.WelcomeMsg
	if welcomeMessage == "" {
		welcomeMessage = req.WelcomeMessage
	}
	if welcomeMessage != "" {
		bot.WelcomeMsg = welcomeMessage
	}

	if adminIDs, provided, err := parseTelegramAdminIDsInput(req.AdminIDs); err != nil {
		return s.panelError(err.Error())
	} else if provided {
		encodedIDs, marshalErr := json.Marshal(adminIDs)
		if marshalErr != nil {
			return s.panelError("failed to encode admin_ids")
		}
		bot.AdminIDs = string(encodedIDs)
	}
	if req.AllowBind != nil {
		bot.AllowBind = *req.AllowBind
	}
	if req.AllowSub != nil {
		bot.AllowSub = *req.AllowSub
	}
	if req.AllowTicket != nil {
		bot.AllowTicket = *req.AllowTicket
	}
	if req.AllowInfo != nil {
		bot.AllowInfo = *req.AllowInfo
	}

	if bot.Name == "" {
		bot.Name = controlName + " Bot"
	}

	if err := db.Save(bot).Error; err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(telegramBotResponse(bot))
}

// SetWebhookRequest is the kernel's SetWebhookRequest. Its name and tags are
// the kernel's, so binding and validation errors read the same.
type SetWebhookRequest struct {
	URL string `json:"url" binding:"omitempty,url,max=512"`
}

// SetWebhookRouteID is POST /api/v2/admin/telegram/webhook.
const SetWebhookRouteID = "notification.admin.telegram.webhook.post"

// AdminSetWebhook is POST /api/v2/admin/telegram/webhook: it calls the Bot
// API's setWebhook and records the webhook. Without a url in the body the
// webhook is this Control's /api/v2/telegram/webhook, at the scheme and host
// of the administrator's request, as the kernel's handler builds it: the
// request_scheme and request_host the kernel sends. The kernel resolves
// them from the connection, or from X-Forwarded-Proto/Host only when the
// peer is a trusted reverse proxy (server.trusted_proxies); forwarding
// headers never reach the package and are not read here.
//
// A kernel that sends no request scheme or host leaves the request to the
// legacy handler (pluginhostsdk.ErrNativeUnavailable), which builds the URL
// from what the kernel's bridge saw.
func (s *Service) AdminSetWebhook(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req SetWebhookRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil && !errors.Is(err, io.EOF) {
		return s.panelError(err.Error())
	}

	webhookURL := strings.TrimSpace(req.URL)
	if webhookURL == "" {
		scheme, host, ok := requestOrigin(request.Metadata)
		if !ok {
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
		webhookURL = fmt.Sprintf("%s://%s/api/v2/telegram/webhook", scheme, host)
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := setWebhook(db, webhookURL); err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(map[string]any{
		"message": "webhook set successfully",
		"url":     webhookURL,
	})
}

// requestOrigin is the request's scheme and host as the kernel resolved
// them. It is false when the kernel sent no scheme or host.
func requestOrigin(metadata pluginhostsdk.RequestMetadata) (string, string, bool) {
	if metadata.Scheme == "" || metadata.Host == "" {
		return "", "", false
	}
	return metadata.Scheme, metadata.Host, true
}

// AdminDeleteWebhook is DELETE /api/v2/admin/telegram/webhook: it calls the
// Bot API's deleteWebhook and records that no webhook is set.
func (s *Service) AdminDeleteWebhook(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := deleteWebhook(db); err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(map[string]any{"message": "webhook deleted"})
}

// AdminTelegramNotify is POST /api/v2/admin/telegram/notify: one message
// to one Telegram chat.
func (s *Service) AdminTelegramNotify(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	telegramID, err := parseTelegramID(req["telegram_id"])
	if err != nil || telegramID == 0 {
		return s.panelError("invalid telegram_id")
	}

	message := extractMessage(req["message"])
	title, _ := req["title"].(string)
	content, _ := req["content"].(string)

	if message == "" {
		title = strings.TrimSpace(title)
		content = strings.TrimSpace(content)
		if title == "" && content == "" {
			return s.panelError("message is required")
		}
		if title != "" && content == "" {
			message = title
		} else if title == "" && content != "" {
			message = content
		} else {
			message = fmt.Sprintf("*%s*\n\n%s", title, content)
		}
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := sendMessage(db, telegramID, message); err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(map[string]any{
		"message": "notification sent",
		"success": true,
	})
}

// AdminTelegramBroadcast is POST /api/v2/admin/telegram/broadcast: the
// message goes, one after another, to every member who is not banned and
// takes at least one kind of notification.
func (s *Service) AdminTelegramBroadcast(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	message := extractMessage(req["message"])
	if message == "" {
		return s.panelError("message is required")
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var users []Binding
	if err := db.
		Where("is_banned = ?", false).
		Where("notify_expire = ? OR notify_traffic = ? OR notify_ticket = ?", true, true, true).
		Find(&users).Error; err != nil {
		return s.panelError(err.Error())
	}
	success := 0
	failed := 0
	for _, user := range users {
		if err := sendMessage(db, user.TelegramID, message); err != nil {
			failed++
		} else {
			success++
		}
	}

	return s.panel(map[string]any{
		"message": "broadcast completed",
		"success": success,
		"failed":  failed,
	})
}

// AdminBindings is GET /api/v2/admin/telegram/users: the members' Telegram
// bindings, newest first, one page or (all=true) every one.
func (s *Service) AdminBindings(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if strings.EqualFold(defaultQuery(request, "all", "false"), "true") {
		var users []Binding
		if err := db.Order("id DESC").Find(&users).Error; err != nil {
			return s.panelError(err.Error())
		}
		emails, err := userEmails(db, users)
		if err != nil {
			return s.panelError(err.Error())
		}
		list := make([]map[string]any, 0, len(users))
		for i := range users {
			list = append(list, telegramUserBindingResponse(&users[i], emails[users[i].UserID]))
		}
		return s.panel(map[string]any{
			"list":      list,
			"total":     len(list),
			"page":      1,
			"page_size": len(list),
		})
	}

	page, pageSize := pagination(request)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var total int64
	if err := db.Model(&Binding{}).Count(&total).Error; err != nil {
		return s.panelError(err.Error())
	}

	var users []Binding
	offset := (page - 1) * pageSize
	if err := db.
		Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&users).Error; err != nil {
		return s.panelError(err.Error())
	}
	emails, err := userEmails(db, users)
	if err != nil {
		return s.panelError(err.Error())
	}

	list := make([]map[string]any, 0, len(users))
	for i := range users {
		list = append(list, telegramUserBindingResponse(&users[i], emails[users[i].UserID]))
	}

	return s.panel(map[string]any{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// AdminUpdateBindingNotify is PUT /api/v2/admin/telegram/users/:id/notify.
// notify_enabled sets all three switches unless one is given on its own.
func (s *Service) AdminUpdateBindingNotify(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	if err != nil {
		return s.panelError("invalid id")
	}

	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var user Binding
	if err := db.First(&user, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("telegram user not found")
		}
		return s.panelError(err.Error())
	}
	emails, err := userEmails(db, []Binding{user})
	if err != nil {
		return s.panelError(err.Error())
	}

	appliedAnyField := false
	notifyEnabledProvided := false
	notifyEnabled := false
	if raw, ok := req["notify_enabled"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return s.panelError("notify_enabled must be boolean")
		}
		notifyEnabledProvided = true
		notifyEnabled = value
	}

	if raw, ok := req["notify_expire"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return s.panelError("notify_expire must be boolean")
		}
		user.NotifyExpire = value
		appliedAnyField = true
	}
	if raw, ok := req["notify_traffic"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return s.panelError("notify_traffic must be boolean")
		}
		user.NotifyTraffic = value
		appliedAnyField = true
	}
	if raw, ok := req["notify_ticket"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return s.panelError("notify_ticket must be boolean")
		}
		user.NotifyTicket = value
		appliedAnyField = true
	}

	if notifyEnabledProvided && !appliedAnyField {
		user.NotifyExpire = notifyEnabled
		user.NotifyTraffic = notifyEnabled
		user.NotifyTicket = notifyEnabled
		appliedAnyField = true
	}

	if !appliedAnyField {
		return s.panelError("no notify fields provided")
	}

	if err := db.Save(&user).Error; err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(telegramUserBindingResponse(&user, emails[user.UserID]))
}

// UserTelegramStatus is GET /api/v2/user/telegram/status.
func (s *Service) UserTelegramStatus(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := actor(request)

	var tgUser Binding
	db, err := s.Open(ctx)
	if err == nil {
		err = db.Where("user_id = ?", userID).First(&tgUser).Error
	}
	if err != nil {
		return s.panel(map[string]any{
			"bound":    false,
			"username": "",
		})
	}

	return s.panel(map[string]any{
		"bound":          true,
		"username":       tgUser.Username,
		"notify_expire":  tgUser.NotifyExpire,
		"notify_traffic": tgUser.NotifyTraffic,
		"notify_ticket":  tgUser.NotifyTicket,
	})
}

// Messages of the two user routes below: neither is implemented, so a request
// is answered with an error and nothing is changed. They are the kernel
// handler's messages (internal/tests/notificationcompat).
const (
	telegramUnbindNotImplementedMessage = "Telegram 解绑尚未实现，绑定未被修改；请在 Telegram 机器人中发送 /unbind (Telegram unbind is not implemented here; your binding was not changed, send /unbind to the bot)"
	telegramNotifyNotImplementedMessage = "Telegram 通知设置尚未实现，设置未被修改 (Telegram notification settings are not implemented here; your settings were not changed)"
)

// UserTelegramUnbind is POST /api/v2/user/telegram/unbind. It is not
// implemented: it deletes nothing and says so.
func (s *Service) UserTelegramUnbind(_ context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	log.Printf("Telegram unbind refused for user_id=%d: not implemented, the binding is unchanged", actor(request))

	return s.panelError(telegramUnbindNotImplementedMessage)
}

// UserTelegramNotify is POST /api/v2/user/telegram/notify. The body is
// validated as in the kernel; a valid one is then refused, as the feature is
// not implemented: no setting is changed.
func (s *Service) UserTelegramNotify(_ context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req NotifySettingsRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	log.Printf("Telegram notify settings update refused for user_id=%d: not implemented, the settings are unchanged", actor(request))

	return s.panelError(telegramNotifyNotImplementedMessage)
}
