package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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
	systemConfigService *service.SystemConfigService
}

const notificationEmailConfigKey = "notification.email.config"

func notificationStatusToText(status int) string {
	switch status {
	case 1:
		return "success"
	case 2:
		return "failed"
	default:
		return "pending"
	}
}

func notificationStatusFromText(status string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return 0, true
	case "success":
		return 1, true
	case "failed":
		return 2, true
	default:
		return 0, false
	}
}

func parseStringField(raw map[string]interface{}, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseIntField(raw map[string]interface{}, key string) int {
	v, ok := raw[key]
	if !ok || v == nil {
		return 0
	}
	switch vv := v.(type) {
	case int:
		return vv
	case int32:
		return int(vv)
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(vv))
		return n
	default:
		return 0
	}
}

func normalizeEmailEncryption(raw interface{}) (string, bool) {
	switch v := raw.(type) {
	case bool:
		if v {
			return "tls", true
		}
		return "none", true
	case float64:
		if int(v) == 1 {
			return "tls", true
		}
		if int(v) == 0 {
			return "none", true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "none", "false", "0", "off", "no":
			return "none", true
		case "tls", "true", "1", "on", "yes", "starttls":
			return "tls", true
		case "ssl":
			return "ssl", true
		}
	}
	return "", false
}

func emailEncryptionBool(enc string) bool {
	return enc == "tls" || enc == "ssl"
}

func (h *NotificationHandler) loadEmailConfig() (*model.EmailConfig, error) {
	cfg := &model.EmailConfig{
		Host:        "",
		Port:        587,
		Username:    "",
		Password:    "",
		FromAddress: "",
		FromName:    "V2Board",
		Encryption:  "tls",
	}

	value, err := h.systemConfigService.Get(notificationEmailConfigKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return cfg, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}

	if host := parseStringField(raw, "host"); host != "" {
		cfg.Host = host
	}
	if port := parseIntField(raw, "port"); port > 0 {
		cfg.Port = port
	}
	if username := parseStringField(raw, "username"); username != "" {
		cfg.Username = username
	}
	if password, ok := raw["password"]; ok {
		if p, ok := password.(string); ok {
			cfg.Password = p
		}
	}
	if fromAddress := parseStringField(raw, "from_address"); fromAddress != "" {
		cfg.FromAddress = fromAddress
	}
	if fromName := parseStringField(raw, "from_name"); fromName != "" {
		cfg.FromName = fromName
	}

	if enc, ok := raw["encryption_type"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
			return cfg, nil
		}
	}
	if enc, ok := raw["encryption"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
		}
	}

	return cfg, nil
}

// NewNotificationHandler 创建处理器
func NewNotificationHandler() *NotificationHandler {
	db := database.Get()
	cfg := config.Get()
	return &NotificationHandler{
		notificationService: service.NewNotificationService(db, cfg),
		systemConfigService: service.NewSystemConfigService(db),
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

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":  templates,
			"total": len(templates),
		},
		"list":  templates,
		"total": len(templates),
	})
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
		Type    string `json:"type"`
		Event   string `json:"event"`
		Name    string `json:"name"`
		Title   string `json:"title"`
		Content string `json:"content"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type != "" {
		template.Type = req.Type
	}
	if req.Event != "" {
		template.Event = req.Event
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
		if numericStatus, err := strconv.Atoi(status); err == nil {
			db = db.Where("status = ?", numericStatus)
		} else if mappedStatus, ok := notificationStatusFromText(status); ok {
			db = db.Where("status = ?", mappedStatus)
		}
	}

	db.Count(&total)

	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs)

	list := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		recipient := ""
		if log.UserID != nil {
			recipient = "user:" + strconv.FormatUint(uint64(*log.UserID), 10)
		}
		list = append(list, gin.H{
			"id":          log.ID,
			"user_id":     log.UserID,
			"type":        log.Type,
			"event":       log.Event,
			"title":       log.Title,
			"content":     log.Content,
			"status":      notificationStatusToText(log.Status),
			"status_code": log.Status,
			"recipient":   recipient,
			"error":       log.Error,
			"sent_at":     log.SentAt,
			"read_at":     log.ReadAt,
			"created_at":  log.CreatedAt,
		})
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
		Type      string `json:"type" binding:"required"`
		To        string `json:"to"`
		Recipient string `json:"recipient"`
		Title     string `json:"title"`
		Subject   string `json:"subject"`
		Content   string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := req.Title
	if title == "" {
		title = req.Subject
	}
	if title == "" {
		title = "Test Notification"
	}
	recipient := req.To
	if recipient == "" {
		recipient = req.Recipient
	}
	content := req.Content
	if content == "" {
		content = "This is a test notification from V2Board."
	}

	var err error
	switch req.Type {
	case "email":
		if recipient == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email address required"})
			return
		}
		cfg, cfgErr := h.loadEmailConfig()
		if cfgErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": cfgErr.Error()})
			return
		}
		if cfg.Host == "" || cfg.FromAddress == "" {
			h.notificationService.SetEmailConfig(nil)
		} else {
			h.notificationService.SetEmailConfig(cfg)
		}
		err = h.notificationService.SendEmail(recipient, title, content)
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

	c.JSON(http.StatusOK, gin.H{
		"message": "test notification sent",
		"data": gin.H{
			"success": true,
		},
	})
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
	cfg, err := h.loadEmailConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Keep service config in sync for test-send path.
	h.notificationService.SetEmailConfig(cfg)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"host":            cfg.Host,
			"port":            cfg.Port,
			"username":        cfg.Username,
			"password":        cfg.Password,
			"from_address":    cfg.FromAddress,
			"from_name":       cfg.FromName,
			"encryption":      emailEncryptionBool(cfg.Encryption),
			"encryption_type": cfg.Encryption,
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
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host := parseStringField(req, "host")
	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host is required"})
		return
	}

	port := parseIntField(req, "port")
	if port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port is required"})
		return
	}

	fromAddress := parseStringField(req, "from_address")
	if fromAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from_address is required"})
		return
	}

	existing, err := h.loadEmailConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg := &model.EmailConfig{
		Host:        host,
		Port:        port,
		Username:    parseStringField(req, "username"),
		Password:    existing.Password,
		FromAddress: fromAddress,
		FromName:    parseStringField(req, "from_name"),
		Encryption:  existing.Encryption,
	}

	if cfg.FromName == "" {
		cfg.FromName = existing.FromName
	}
	if cfg.FromName == "" {
		cfg.FromName = "V2Board"
	}

	if passwordRaw, ok := req["password"]; ok {
		if password, ok := passwordRaw.(string); ok {
			if strings.TrimSpace(password) != "" {
				cfg.Password = password
			}
		}
	}

	hasEncryption := false
	if encRaw, ok := req["encryption_type"]; ok {
		normalized, valid := normalizeEmailEncryption(encRaw)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid encryption_type"})
			return
		}
		cfg.Encryption = normalized
		hasEncryption = true
	}
	if !hasEncryption {
		if encRaw, ok := req["encryption"]; ok {
			normalized, valid := normalizeEmailEncryption(encRaw)
			if !valid {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid encryption"})
				return
			}
			cfg.Encryption = normalized
		}
	}
	if cfg.Encryption == "" {
		cfg.Encryption = "none"
	}

	if err := h.systemConfigService.SetJSON(
		notificationEmailConfigKey,
		cfg,
		"notification",
		"Email notification config",
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure send-test uses newest config immediately.
	h.notificationService.SetEmailConfig(cfg)

	c.JSON(http.StatusOK, gin.H{"message": "email config updated"})
}
