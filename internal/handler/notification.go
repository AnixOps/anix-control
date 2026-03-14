package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// NotificationHandler 通知处理器
type NotificationHandler struct {
	notificationService *service.NotificationService
}

// NewNotificationHandler 创建处理器
func NewNotificationHandler() *NotificationHandler {
	db := database.Get()
	cfg := config.Get()
	return &NotificationHandler{
		notificationService: service.NewNotificationService(db, cfg),
	}
}

// ========== 用户接口 ==========

// GetUserNotifications godoc
// @Summary 获取用户通知列表
// @Description 用户获取自己的通知列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /user/notifications [get]
func (h *NotificationHandler) GetUserNotifications(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var logs []model.NotificationLog
	var total int64

	db := database.Get()
	db.Model(&model.NotificationLog{}).Where("user_id = ?", userID).Count(&total)

	offset := (page - 1) * pageSize
	db.Where("user_id = ?", userID).Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"data":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// MarkAsRead godoc
// @Summary 标记通知为已读
// @Description 用户将指定通知标记为已读
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "通知ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /user/notifications/{id}/read [post]
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userID := c.GetUint("user_id")
	id := c.Param("id")

	result := database.Get().Model(&model.NotificationLog{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("read_at", time.Now())

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
}

// MarkAllAsRead godoc
// @Summary 标记所有通知为已读
// @Description 用户将所有通知标记为已读
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user/notifications/read-all [post]
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	userID := c.GetUint("user_id")

	database.Get().Model(&model.NotificationLog{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now())

	c.JSON(http.StatusOK, gin.H{"message": "all notifications marked as read"})
}

// GetUnreadCount godoc
// @Summary 获取未读通知数量
// @Description 用户获取未读通知的数量
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /user/notifications/unread-count [get]
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userID := c.GetUint("user_id")

	var count int64
	database.Get().Model(&model.NotificationLog{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&count)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"count": count}})
}

// ========== 管理员接口 ==========

// ListTemplates godoc
// @Summary 获取通知模板列表
// @Description 管理员获取通知模板列表
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param type query string false "通知类型"
// @Success 200 {object} map[string]interface{}
// @Router /admin/notification/templates [get]
func (h *NotificationHandler) ListTemplates(c *gin.Context) {
	notifyType := c.Query("type")

	var templates []model.NotificationTemplate
	db := database.Get()

	if notifyType != "" {
		db = db.Where("type = ?", notifyType)
	}

	db.Find(&templates)

	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// CreateTemplate godoc
// @Summary 创建通知模板
// @Description 管理员创建新的通知模板
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "模板信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/notification/templates [post]
func (h *NotificationHandler) CreateTemplate(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Type    string `json:"type" binding:"required"`
		Event   string `json:"event" binding:"required"`
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template := &model.NotificationTemplate{
		Name:    req.Name,
		Type:    req.Type,
		Event:   req.Event,
		Title:   req.Title,
		Content: req.Content,
		Enabled: req.Enabled,
	}

	if err := database.Get().Create(template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": template})
}

// UpdateTemplate godoc
// @Summary 更新通知模板
// @Description 管理员更新指定通知模板
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Param request body map[string]interface{} true "模板更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/notification/templates/{id} [put]
func (h *NotificationHandler) UpdateTemplate(c *gin.Context) {
	id := c.Param("id")

	var template model.NotificationTemplate
	if err := database.Get().First(&template, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	var req struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Content string `json:"content"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		template.Name = req.Name
	}
	if req.Title != "" {
		template.Title = req.Title
	}
	if req.Content != "" {
		template.Content = req.Content
	}
	if req.Enabled != nil {
		template.Enabled = *req.Enabled
	}

	if err := database.Get().Save(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": template})
}

// DeleteTemplate godoc
// @Summary 删除通知模板
// @Description 管理员删除指定通知模板
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "模板ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/notification/templates/{id} [delete]
func (h *NotificationHandler) DeleteTemplate(c *gin.Context) {
	id := c.Param("id")

	if err := database.Get().Delete(&model.NotificationTemplate{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListLogs godoc
// @Summary 获取通知日志列表
// @Description 管理员获取通知日志列表
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param type query string false "通知类型"
// @Param status query string false "发送状态"
// @Success 200 {object} map[string]interface{}
// @Router /admin/notification/logs [get]
func (h *NotificationHandler) ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	notifyType := c.Query("type")
	status := c.Query("status")

	var logs []model.NotificationLog
	var total int64

	db := database.Get().Model(&model.NotificationLog{})

	if notifyType != "" {
		db = db.Where("type = ?", notifyType)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}

	db.Count(&total)

	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"data":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// SendTestNotification godoc
// @Summary 发送测试通知
// @Description 管理员发送测试通知
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "测试通知请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/notification/test [post]
func (h *NotificationHandler) SendTestNotification(c *gin.Context) {
	var req struct {
		Type    string `json:"type" binding:"required"`
		To      string `json:"to"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := req.Title
	if title == "" {
		title = "Test Notification"
	}
	content := req.Content
	if content == "" {
		content = "This is a test notification from V2Board."
	}

	var err error
	switch req.Type {
	case "email":
		if req.To == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email address required"})
			return
		}
		err = h.notificationService.SendEmail(req.To, title, content)
	case "telegram":
		// TODO: 实现Telegram测试通知
		err = nil
	case "webhook":
		// TODO: 实现Webhook测试通知
		err = nil
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification type"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "test notification sent"})
}

// GetEmailConfig godoc
// @Summary 获取邮件配置
// @Description 管理员获取邮件发送配置
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/notification/email/config [get]
func (h *NotificationHandler) GetEmailConfig(c *gin.Context) {
	// TODO: 从数据库读取邮件配置
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"host":         "",
			"port":         587,
			"username":     "",
			"from_address": "",
			"from_name":    "V2Board",
			"encryption":   "tls",
		},
	})
}

// UpdateEmailConfig godoc
// @Summary 更新邮件配置
// @Description 管理员更新邮件发送配置
// @Tags 管理端-通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "邮件配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/notification/email/config [put]
func (h *NotificationHandler) UpdateEmailConfig(c *gin.Context) {
	var req struct {
		Host        string `json:"host" binding:"required"`
		Port        int    `json:"port" binding:"required"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		FromAddress string `json:"from_address" binding:"required"`
		FromName    string `json:"from_name"`
		Encryption  string `json:"encryption"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 保存到数据库

	c.JSON(http.StatusOK, gin.H{"message": "email config updated"})
}