package handler

import (
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// TelegramHandler Telegram Bot处理器
type TelegramHandler struct {
	botService *service.TelegramBotService
}

// NewTelegramHandler 创建处理器
func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		botService: service.NewTelegramBotService(database.Get()),
	}
}

// GetBot godoc
// @Summary 获取Telegram Bot配置
// @Description 管理员获取Telegram Bot的配置信息
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/telegram/bot [get]
func (h *TelegramHandler) GetBot(c *gin.Context) {
	bot, err := h.botService.GetBot()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not configured"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": bot})
}

// UpdateBot godoc
// @Summary 更新Telegram Bot配置
// @Description 管理员更新Telegram Bot的配置信息
// @Tags 管理端-通知
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
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not configured"})
		return
	}

	if req.Name != "" {
		bot.Name = req.Name
	}
	if req.Token != "" {
		bot.Token = req.Token
	}
	if req.WelcomeMsg != "" {
		bot.WelcomeMsg = req.WelcomeMsg
	}
	if req.AdminIDs != "" {
		bot.AdminIDs = req.AdminIDs
	}
	bot.AllowBind = req.AllowBind
	bot.AllowSub = req.AllowSub
	bot.AllowTicket = req.AllowTicket
	bot.AllowInfo = req.AllowInfo

	if err := h.botService.UpdateBot(bot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": bot})
}

// SetWebhook godoc
// @Summary 设置Telegram Webhook
// @Description 管理员设置Telegram Bot的Webhook地址
// @Tags 管理端-通知
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.botService.SetWebhook(req.URL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "webhook set successfully"})
}

// DeleteWebhook godoc
// @Summary 删除Telegram Webhook
// @Description 管理员删除Telegram Bot的Webhook配置
// @Tags 管理端-通知
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
// @Description 管理员通过Telegram发送通知给指定用户
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SendNotificationRequest true "通知请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/telegram/notify [post]
func (h *TelegramHandler) SendNotification(c *gin.Context) {
	var req SendNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.botService.SendNotification(req.TelegramID, req.Title, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notification sent"})
}

// Broadcast godoc
// @Summary 广播Telegram消息
// @Description 管理员通过Telegram广播消息给所有绑定用户
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BroadcastRequest true "广播请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/telegram/broadcast [post]
func (h *TelegramHandler) Broadcast(c *gin.Context) {
	var req BroadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go h.botService.Broadcast(req.Message)

	c.JSON(http.StatusOK, gin.H{"message": "broadcast started"})
}

// GetUserBindings godoc
// @Summary 获取用户绑定列表
// @Description 管理员获取Telegram用户绑定列表
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /admin/telegram/users [get]
func (h *TelegramHandler) GetUserBindings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	userService := h.botService.GetTelegramUserService()

	// TODO: 实现分页查询
	_ = page
	_ = pageSize
	_ = userService

	c.JSON(http.StatusOK, gin.H{
		"data":  []interface{}{},
		"total": 0,
	})
}

// ========== 用户接口 ==========

// GetTelegramStatus godoc
// @Summary 获取Telegram绑定状态
// @Description 用户获取自己的Telegram绑定状态
// @Tags 用户端
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
			"bound":       true,
			"username":    tgUser.Username,
			"notify_expire": tgUser.NotifyExpire,
			"notify_traffic": tgUser.NotifyTraffic,
			"notify_ticket": tgUser.NotifyTicket,
		},
	})
}

// UnbindTelegram godoc
// @Summary 解绑Telegram
// @Description 用户解绑自己的Telegram账号
// @Tags 用户端
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
// @Tags 用户端
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

// ========== 请求结构体 ==========

// UpdateBotRequest 更新Bot请求
type UpdateBotRequest struct {
	Name       string `json:"name"`
	Token      string `json:"token"`
	WelcomeMsg string `json:"welcome_msg"`
	AdminIDs   string `json:"admin_ids"`
	AllowBind  bool   `json:"allow_bind"`
	AllowSub   bool   `json:"allow_sub"`
	AllowTicket bool  `json:"allow_ticket"`
	AllowInfo  bool   `json:"allow_info"`
}

// SetWebhookRequest 设置Webhook请求
type SetWebhookRequest struct {
	URL string `json:"url" binding:"required"`
}

// SendNotificationRequest 发送通知请求
type SendNotificationRequest struct {
	TelegramID int64  `json:"telegram_id" binding:"required"`
	Title      string `json:"title" binding:"required"`
	Content    string `json:"content" binding:"required"`
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