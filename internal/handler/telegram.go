package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TelegramHandler Telegram Bot处理�?
type TelegramHandler struct {
	botService *service.TelegramBotService
}

// NewTelegramHandler 创建处理�?
func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		botService: service.NewTelegramBotService(database.Get()),
	}
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

func parseTelegramAdminIDsInput(raw interface{}) ([]int64, bool, error) {
	if raw == nil {
		return nil, false, nil
	}

	switch v := raw.(type) {
	case string:
		return parseTelegramBotAdminIDs(v), true, nil
	case []interface{}:
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

func telegramBotResponse(bot *model.TelegramBot) gin.H {
	adminIDs := parseTelegramBotAdminIDs(bot.AdminIDs)
	return gin.H{
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

func telegramUserBindingResponse(user *model.TelegramUser) gin.H {
	userEmail := ""
	if user.User != nil {
		userEmail = user.User.Email
	}
	notifyEnabled := user.NotifyExpire || user.NotifyTraffic || user.NotifyTicket
	return gin.H{
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

func requestScheme(c *gin.Context) string {
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}

func parseTelegramID(raw interface{}) (int64, error) {
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

func extractMessage(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		if inner, ok := v["message"].(string); ok {
			return strings.TrimSpace(inner)
		}
	}
	return ""
}

// GetBot godoc
// @Summary 获取Telegram Bot配置
// @Description 管理员获取Telegram Bot的配置信�?
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/telegram/bot [get]
func (h *TelegramHandler) GetBot(c *gin.Context) {
	bot, err := h.botService.GetBot()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"token":           "",
					"admin_ids":       []int64{},
					"welcome_msg":     "",
					"welcome_message": "",
					"allow_bind":      true,
					"allow_sub":       true,
					"allow_ticket":    true,
					"allow_info":      true,
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": telegramBotResponse(bot)})
}

// UpdateBot godoc
// @Summary 更新Telegram Bot配置
// @Description 管理员更新Telegram Bot的配置信�?
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateBotRequest true "Bot配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/telegram/bot [put]
func (h *TelegramHandler) UpdateBot(c *gin.Context) {
	var req UpdateBotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bot, err := h.botService.GetBot()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		bot = &model.TelegramBot{
			Name:        "V2Board Bot",
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if provided {
		encodedIDs, _ := json.Marshal(adminIDs)
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
		bot.Name = "V2Board Bot"
	}

	if err := h.botService.UpdateBot(bot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": telegramBotResponse(bot)})
}

// SetWebhook godoc
// @Summary 设置Telegram Webhook
// @Description 管理员设置Telegram Bot的Webhook地址
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SetWebhookRequest true "Webhook配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/telegram/webhook [post]
func (h *TelegramHandler) SetWebhook(c *gin.Context) {
	var req SetWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	webhookURL := strings.TrimSpace(req.URL)
	if webhookURL == "" {
		webhookURL = fmt.Sprintf("%s://%s/api/v2/telegram/webhook", requestScheme(c), c.Request.Host)
	}

	if err := h.botService.SetWebhook(webhookURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "webhook set successfully",
		"data": gin.H{
			"url": webhookURL,
		},
	})
}

// DeleteWebhook godoc
// @Summary 删除Telegram Webhook
// @Description 管理员删除Telegram Bot的Webhook配置
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/telegram/webhook [delete]
func (h *TelegramHandler) DeleteWebhook(c *gin.Context) {
	if err := h.botService.DeleteWebhook(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "webhook deleted"})
}

// TelegramWebhook godoc
// @Summary Telegram Webhook回调
// @Description 接收Telegram Bot的Webhook回调
// @Tags Telegram
// @Accept json
// @Produce json
// @Param request body service.TelegramUpdate true "Telegram Update"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /telegram/webhook [post]
func (h *TelegramHandler) TelegramWebhook(c *gin.Context) {
	var update service.TelegramUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 异步处理更新
	go h.botService.HandleUpdate(&update)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// SendNotification godoc
// @Summary 发送Telegram通知
// @Description 管理员通过Telegram发送通知给指定用�?
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SendNotificationRequest true "通知请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/telegram/notify [post]
func (h *TelegramHandler) SendNotification(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	telegramID, err := parseTelegramID(req["telegram_id"])
	if err != nil || telegramID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram_id"})
		return
	}

	message := extractMessage(req["message"])
	title, _ := req["title"].(string)
	content, _ := req["content"].(string)

	if message == "" {
		title = strings.TrimSpace(title)
		content = strings.TrimSpace(content)
		if title == "" && content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
			return
		}
		if title != "" && content == "" {
			message = title
		} else if title == "" && content != "" {
			message = content
		} else {
			message = fmt.Sprintf("*%s*\n\n%s", title, content)
		}
	}

	if err := h.botService.SendMessage(telegramID, message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "notification sent",
		"data": gin.H{
			"success": true,
		},
	})
}

// Broadcast godoc
// @Summary 广播Telegram消息
// @Description 管理员通过Telegram广播消息给所有绑定用�?
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BroadcastRequest true "广播请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/telegram/broadcast [post]
func (h *TelegramHandler) Broadcast(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message := extractMessage(req["message"])
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	var users []model.TelegramUser
	if err := database.Get().
		Where("is_banned = ?", false).
		Where("notify_expire = ? OR notify_traffic = ? OR notify_ticket = ?", true, true, true).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	success := 0
	failed := 0
	for _, user := range users {
		if err := h.botService.SendMessage(user.TelegramID, message); err != nil {
			failed++
		} else {
			success++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "broadcast completed",
		"data": gin.H{
			"success": success,
			"failed":  failed,
		},
	})
}

// GetUserBindings godoc
// @Summary 获取用户绑定列表
// @Description 管理员获取Telegram用户绑定列表
// @Tags 管理�?通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /admin/telegram/users [get]
func (h *TelegramHandler) GetUserBindings(c *gin.Context) {
	if strings.EqualFold(c.DefaultQuery("all", "false"), "true") {
		var users []model.TelegramUser
		if err := database.Get().
			Preload("User").
			Order("id DESC").
			Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		list := make([]gin.H, 0, len(users))
		for i := range users {
			list = append(list, telegramUserBindingResponse(&users[i]))
		}
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"list":      list,
				"total":     len(list),
				"page":      1,
				"page_size": len(list),
			},
			"list":      list,
			"total":     len(list),
			"page":      1,
			"page_size": len(list),
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	db := database.Get().Model(&model.TelegramUser{})
	var total int64
	if err := db.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var users []model.TelegramUser
	offset := (page - 1) * pageSize
	if err := database.Get().
		Preload("User").
		Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	list := make([]gin.H, 0, len(users))
	for i := range users {
		list = append(list, telegramUserBindingResponse(&users[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// UpdateUserNotify godoc
// @Summary 鏇存柊Telegram鐢ㄦ埛閫氱煡璁剧疆
// @Description 绠＄悊鍛樻洿鏂版寚瀹歍elegram鐢ㄦ埛鐨勯€氱煡寮€鍏?// @Tags 绠＄悊绔?閫氱煡
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Telegram缁戝畾ID"
// @Param request body map[string]interface{} true "閫氱煡璁剧疆"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/telegram/users/{id}/notify [put]
func (h *TelegramHandler) UpdateUserNotify(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user model.TelegramUser
	if err := database.Get().Preload("User").First(&user, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "telegram user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	appliedAnyField := false
	notifyEnabledProvided := false
	notifyEnabled := false
	if raw, ok := req["notify_enabled"]; ok {
		value, ok := raw.(bool)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notify_enabled must be boolean"})
			return
		}
		notifyEnabledProvided = true
		notifyEnabled = value
	}

	if raw, ok := req["notify_expire"]; ok {
		value, ok := raw.(bool)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notify_expire must be boolean"})
			return
		}
		user.NotifyExpire = value
		appliedAnyField = true
	}
	if raw, ok := req["notify_traffic"]; ok {
		value, ok := raw.(bool)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notify_traffic must be boolean"})
			return
		}
		user.NotifyTraffic = value
		appliedAnyField = true
	}
	if raw, ok := req["notify_ticket"]; ok {
		value, ok := raw.(bool)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notify_ticket must be boolean"})
			return
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no notify fields provided"})
		return
	}

	if err := database.Get().Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": telegramUserBindingResponse(&user)})
}

// ========== 用户接口 ==========

// GetTelegramStatus godoc
// @Summary 获取Telegram绑定状�?
// @Description 用户获取自己的Telegram绑定状�?
// @Tags 用户�?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user/telegram/status [get]
func (h *TelegramHandler) GetTelegramStatus(c *gin.Context) {
	userID := c.GetUint("user_id")

	userService := h.botService.GetTelegramUserService()
	tgUser, err := userService.GetByUserID(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"bound":    false,
				"username": "",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"bound":          true,
			"username":       tgUser.Username,
			"notify_expire":  tgUser.NotifyExpire,
			"notify_traffic": tgUser.NotifyTraffic,
			"notify_ticket":  tgUser.NotifyTicket,
		},
	})
}

// UnbindTelegram godoc
// @Summary 解绑Telegram
// @Description 用户解绑自己的Telegram账号
// @Tags 用户�?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user/telegram/unbind [post]
func (h *TelegramHandler) UnbindTelegram(c *gin.Context) {
	userID := c.GetUint("user_id")

	userService := h.botService.GetTelegramUserService()
	var tgUser service.TelegramUser
	// TODO: 实现解绑逻辑
	_ = userID
	_ = userService
	_ = tgUser

	c.JSON(http.StatusOK, gin.H{"message": "unbound successfully"})
}

// UpdateNotifySettings godoc
// @Summary 更新Telegram通知设置
// @Description 用户更新自己的Telegram通知设置
// @Tags 用户�?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body NotifySettingsRequest true "通知设置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/telegram/notify [post]
func (h *TelegramHandler) UpdateNotifySettings(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req NotifySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 实现更新通知设置
	_ = userID

	c.JSON(http.StatusOK, gin.H{"message": "settings updated"})
}

// ========== 请求结构�?==========

// UpdateBotRequest 更新Bot请求
type UpdateBotRequest struct {
	Name           string      `json:"name"`
	Token          string      `json:"token"`
	WelcomeMsg     string      `json:"welcome_msg"`
	WelcomeMessage string      `json:"welcome_message"`
	AdminIDs       interface{} `json:"admin_ids"`
	AllowBind      *bool       `json:"allow_bind"`
	AllowSub       *bool       `json:"allow_sub"`
	AllowTicket    *bool       `json:"allow_ticket"`
	AllowInfo      *bool       `json:"allow_info"`
}

// SetWebhookRequest 设置Webhook请求
type SetWebhookRequest struct {
	URL string `json:"url"`
}

// SendNotificationRequest 发送通知请求
type SendNotificationRequest struct {
	TelegramID int64  `json:"telegram_id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Message    string `json:"message"`
}

// BroadcastRequest 广播请求
type BroadcastRequest struct {
	Message string `json:"message" binding:"required"`
}

// NotifySettingsRequest 通知设置请求
type NotifySettingsRequest struct {
	NotifyExpire  *bool `json:"notify_expire"`
	NotifyTraffic *bool `json:"notify_traffic"`
	NotifyTicket  *bool `json:"notify_ticket"`
}
