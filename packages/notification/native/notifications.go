package native

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
)

// Template is a v2_notification_template row. Its JSON matches the kernel
// model, which the template answers return as is.
type Template struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Type      string    `gorm:"size:20;not null" json:"type"`
	Event     string    `gorm:"size:50;not null" json:"event"`
	Title     string    `gorm:"size:200" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Template) TableName() string { return "v2_notification_template" }

// Log is a v2_notification_log row. Its JSON matches the kernel model, which
// the member's notification list returns as is.
type Log struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    *uint      `gorm:"index" json:"user_id"`
	Type      string     `gorm:"size:20" json:"type"`
	Event     string     `gorm:"size:50" json:"event"`
	Title     string     `gorm:"size:200" json:"title"`
	Content   string     `gorm:"type:text" json:"content"`
	Status    int        `json:"status"`
	Error     string     `gorm:"size:500" json:"error"`
	SentAt    *time.Time `json:"sent_at"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
}

// TableName is the adopted kernel table.
func (Log) TableName() string { return "v2_notification_log" }

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

// UserNotifications is GET /api/v2/user/notifications: the caller's
// notifications, newest first. Unlike the admin log list it does not clamp
// the page.
func (s *Service) UserNotifications(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := actor(request)
	page, pageSize := pagination(request)

	var logs []Log
	var total int64

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("failed to get notification count")
	}
	if err := db.Model(&Log{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return s.panelError("failed to get notification count")
	}

	offset := (page - 1) * pageSize
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		return s.panelError("failed to get notifications")
	}

	return s.panel(map[string]any{
		"list":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// UserMarkRead is POST /api/v2/user/notifications/:id/read. The id is
// compared as given, so one that is not a number matches no row.
func (s *Service) UserMarkRead(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := actor(request)
	id := request.Metadata.PathParams["id"]

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("notification not found")
	}
	result := db.Model(&Log{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("read_at", s.now())

	if result.RowsAffected == 0 {
		return s.panelError("notification not found")
	}

	return s.panel(map[string]any{"message": "marked as read"})
}

// UserMarkAllRead is POST /api/v2/user/notifications/read-all. Like the
// legacy handler it answers success even when the update fails.
func (s *Service) UserMarkAllRead(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := actor(request)

	if db, err := s.Open(ctx); err != nil {
		log.Printf("notification mark all read: open storage: %v", err)
	} else {
		db.Model(&Log{}).
			Where("user_id = ? AND read_at IS NULL", userID).
			Update("read_at", s.now())
	}

	return s.panel(map[string]any{"message": "all notifications marked as read"})
}

// UserUnreadCount is GET /api/v2/user/notifications/unread-count. Like the
// legacy handler it answers 0 when the count fails.
func (s *Service) UserUnreadCount(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := actor(request)

	var count int64
	if db, err := s.Open(ctx); err != nil {
		log.Printf("notification unread count: open storage: %v", err)
	} else {
		db.Model(&Log{}).
			Where("user_id = ? AND read_at IS NULL", userID).
			Count(&count)
	}

	return s.panel(map[string]any{"count": count})
}

// AdminTemplates is GET /api/v2/admin/notification/templates.
func (s *Service) AdminTemplates(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	notifyType := query(request, "type")

	var templates []Template
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("failed to load templates")
	}

	if notifyType != "" {
		db = db.Where("type = ?", notifyType)
	}

	if err := db.Find(&templates).Error; err != nil {
		return s.panelError("failed to load templates")
	}

	return s.panel(map[string]any{
		"list":  templates,
		"total": len(templates),
	})
}

// AdminCreateTemplate is POST /api/v2/admin/notification/templates.
func (s *Service) AdminCreateTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Type    string `json:"type" binding:"required"`
		Event   string `json:"event" binding:"required"`
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	template := &Template{
		Name:    req.Name,
		Type:    req.Type,
		Event:   req.Event,
		Title:   req.Title,
		Content: req.Content,
		Enabled: req.Enabled,
	}

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := db.Create(template).Error; err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(template)
}

// AdminUpdateTemplate is PUT /api/v2/admin/notification/templates/:id.
//
// The id goes to GORM as the legacy handler passes it: a numeric id is the
// primary key, anything else becomes GORM's inline SQL condition. The native
// route keeps that for parity with the legacy handler (admin only); a fix
// belongs in both at once.
func (s *Service) AdminUpdateTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id := request.Metadata.PathParams["id"]

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("template not found")
	}
	var template Template
	if err := db.First(&template, id).Error; err != nil {
		return s.panelError("template not found")
	}

	var req struct {
		Type    string `json:"type" binding:"omitempty,oneof=email telegram webhook"`
		Event   string `json:"event" binding:"omitempty,max=128"`
		Name    string `json:"name" binding:"omitempty,min=1,max=255"`
		Title   string `json:"title" binding:"omitempty,max=255"`
		Content string `json:"content" binding:"omitempty"`
		Enabled *bool  `json:"enabled"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
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

	if err := db.Save(&template).Error; err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(template)
}

// AdminDeleteTemplate is DELETE /api/v2/admin/notification/templates/:id.
// The id reaches GORM as in AdminUpdateTemplate.
func (s *Service) AdminDeleteTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id := request.Metadata.PathParams["id"]

	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := db.Delete(&Template{}, id).Error; err != nil {
		return s.panelError(err.Error())
	}

	return s.panel(map[string]any{"message": "deleted"})
}

// AdminLogs is GET /api/v2/admin/notification/logs.
func (s *Service) AdminLogs(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, pageSize := clampPagination(pagination(request))
	notifyType := query(request, "type")
	status := query(request, "status")

	var logs []Log
	var total int64

	conn, err := s.Open(ctx)
	if err != nil {
		return s.panelError("failed to count notification logs")
	}
	db := conn.Model(&Log{})

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

	if err := db.Count(&total).Error; err != nil {
		return s.panelError("failed to count notification logs")
	}

	offset := (page - 1) * pageSize
	if err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		return s.panelError("failed to load notification logs")
	}

	list := make([]map[string]any, 0, len(logs))
	for _, entry := range logs {
		recipient := ""
		if entry.UserID != nil {
			recipient = "user:" + strconv.FormatUint(uint64(*entry.UserID), 10)
		}
		list = append(list, map[string]any{
			"id":          entry.ID,
			"user_id":     entry.UserID,
			"type":        entry.Type,
			"event":       entry.Event,
			"title":       entry.Title,
			"content":     entry.Content,
			"status":      notificationStatusToText(entry.Status),
			"status_code": entry.Status,
			"recipient":   recipient,
			"error":       entry.Error,
			"sent_at":     entry.SentAt,
			"read_at":     entry.ReadAt,
			"created_at":  entry.CreatedAt,
		})
	}

	return s.panel(map[string]any{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
