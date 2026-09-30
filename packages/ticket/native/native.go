// Package native implements the ticket package's v2 routes in the package
// itself, on the kernel's v2_ticket and v2_ticket_message tables adopted in
// place (kernel.storage.adopt). Legacy handlers and native routes share the
// tables, so a route can switch between them at any time. Responses are
// byte-compatible with the legacy handlers (internal/tests/ticketcompat).
package native

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Ticket is a v2_ticket row. Its JSON matches the kernel model, which the
// create and detail answers return as is (the kernel's User relation is
// never loaded there, so it is omitted).
type Ticket struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Subject   string    `gorm:"size:255" json:"subject"`
	Level     int       `gorm:"default:1" json:"level"`
	Status    int       `gorm:"default:0" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Messages []Message `gorm:"foreignKey:TicketID" json:"messages,omitempty"`
}

// TableName is the adopted kernel table.
func (Ticket) TableName() string { return "v2_ticket" }

// Message is a v2_ticket_message row.
type Message struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TicketID  uint      `gorm:"index" json:"ticket_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Message   string    `gorm:"type:text" json:"message"`
	IsAdmin   int       `gorm:"default:0" json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName is the adopted kernel table.
func (Message) TableName() string { return "v2_ticket_message" }

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"ticket.admin.ticket.get":           s.AdminList,
		"ticket.admin.ticket.reply.post":    s.AdminReply,
		"ticket.admin.ticket.id.close.post": s.AdminClose,
		"ticket.user.ticket.get":            s.UserList,
		"ticket.user.ticket.post":           s.UserCreate,
		"ticket.user.ticket.id.get":         s.UserGet,
		"ticket.user.ticket.id.reply.post":  s.UserReply,
		"ticket.user.ticket.id.close.post":  s.UserClose,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

func parseID(raw string) (uint64, bool) {
	id, err := strconv.ParseUint(raw, 10, 32)
	return id, err == nil
}

// actor is the caller's user id; the kernel authenticated the request.
func actor(request pluginhostsdk.NativeRequest) uint {
	return request.Principal.ActorID
}

// AdminList is GET /api/v2/admin/ticket.
func (s *Service) AdminList(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单列表失败")
	}
	var tickets []Ticket
	if err := db.Order("created_at DESC").Find(&tickets).Error; err != nil {
		log.Printf("admin ticket list failed: %v", err)
		return s.panelError("获取工单列表失败")
	}
	result := make([]map[string]any, 0, len(tickets))
	for _, t := range tickets {
		result = append(result, map[string]any{
			"id": t.ID, "user_id": t.UserID, "subject": t.Subject, "level": t.Level, "status": t.Status,
			"created_at": t.CreatedAt.Unix(), "updated_at": t.UpdatedAt.Unix(),
		})
	}
	return s.panel(result)
}

// AdminReply is POST /api/v2/admin/ticket/reply.
func (s *Service) AdminReply(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		TicketID uint   `json:"ticket_id" binding:"required"`
		Message  string `json:"message" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var ticket Ticket
	if err := db.First(&ticket, req.TicketID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("工单不存在")
		}
		log.Printf("admin ticket reply lookup failed: %v", err)
		return s.panelError("获取工单失败")
	}
	message := Message{TicketID: req.TicketID, UserID: actor(request), Message: req.Message, IsAdmin: 1, CreatedAt: s.now()}
	if err := db.Create(&message).Error; err != nil {
		log.Printf("admin ticket reply create failed: %v", err)
		return s.panelError("回复失败")
	}
	if err := db.Model(&ticket).Update("status", 1).Error; err != nil {
		log.Printf("admin ticket status update after reply failed: %v", err)
		return s.panelError("更新工单状态失败")
	}
	return s.panel(map[string]any{"message": "回复成功"})
}

// AdminClose is POST /api/v2/admin/ticket/:id/close.
func (s *Service) AdminClose(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的工单ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var ticket Ticket
	if err := db.First(&ticket, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("工单不存在")
		}
		log.Printf("admin ticket close lookup failed: %v", err)
		return s.panelError("获取工单失败")
	}
	if err := db.Model(&ticket).Update("status", 2).Error; err != nil {
		log.Printf("admin ticket close update failed: %v", err)
		return s.panelError("关闭失败")
	}
	return s.panel(map[string]any{"message": "工单已关闭", "id": id})
}

// UserList is GET /api/v2/user/ticket: the caller's tickets.
func (s *Service) UserList(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var tickets []Ticket
	if err := db.Where("user_id = ?", actor(request)).Order("updated_at DESC").Find(&tickets).Error; err != nil {
		log.Printf("user ticket list failed: %v", err)
		return s.panelError("获取工单失败")
	}
	result := make([]map[string]any, 0, len(tickets))
	for _, t := range tickets {
		result = append(result, map[string]any{
			"id": t.ID, "subject": t.Subject, "level": t.Level, "status": t.Status,
			"created_at": t.CreatedAt.Unix(), "updated_at": t.UpdatedAt.Unix(),
		})
	}
	return s.panel(result)
}

// UserCreate is POST /api/v2/user/ticket: a ticket with its first message.
func (s *Service) UserCreate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Subject string `json:"subject" binding:"required"`
		Level   int    `json:"level"`
		Message string `json:"message" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("创建工单失败")
	}
	now := s.now()
	ticket := Ticket{UserID: actor(request), Subject: req.Subject, Level: req.Level, Status: 0, CreatedAt: now, UpdatedAt: now}
	failure := ""
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ticket).Error; err != nil {
			log.Printf("user ticket create failed: %v", err)
			failure = "创建工单失败"
			return err
		}
		message := Message{TicketID: ticket.ID, UserID: actor(request), Message: req.Message, IsAdmin: 0, CreatedAt: s.now()}
		if err := tx.Create(&message).Error; err != nil {
			log.Printf("user ticket initial message create failed: %v", err)
			failure = "发送消息失败"
			return err
		}
		return nil
	})
	if err != nil {
		if failure == "" {
			log.Printf("user ticket create commit failed: %v", err)
			failure = "创建工单失败"
		}
		return s.panelError(failure)
	}
	return s.panel(ticket)
}

// UserGet is GET /api/v2/user/ticket/:id: one of the caller's tickets with
// its messages.
func (s *Service) UserGet(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的工单ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var ticket Ticket
	if err := db.Preload("Messages").Where("id = ? AND user_id = ?", id, actor(request)).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("工单不存在")
		}
		log.Printf("user ticket detail failed: %v", err)
		return s.panelError("获取工单失败")
	}
	return s.panel(ticket)
}

// UserReply is POST /api/v2/user/ticket/:id/reply: it reopens the ticket.
func (s *Service) UserReply(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的工单ID")
	}
	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var ticket Ticket
	if err := db.Where("id = ? AND user_id = ?", id, actor(request)).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("工单不存在")
		}
		log.Printf("user ticket reply lookup failed: %v", err)
		return s.panelError("获取工单失败")
	}
	if ticket.Status == 2 {
		return s.panelError("工单已关闭，无法回复")
	}
	message := Message{TicketID: uint(id), UserID: actor(request), Message: req.Message, IsAdmin: 0, CreatedAt: s.now()}
	if err := db.Create(&message).Error; err != nil {
		log.Printf("user ticket reply create failed: %v", err)
		return s.panelError("回复失败")
	}
	if err := db.Model(&ticket).Updates(map[string]any{"status": 0, "updated_at": s.now()}).Error; err != nil {
		log.Printf("user ticket status update after reply failed: %v", err)
		return s.panelError("更新工单状态失败")
	}
	return s.panel("回复成功")
}

// UserClose is POST /api/v2/user/ticket/:id/close.
func (s *Service) UserClose(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的工单ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取工单失败")
	}
	var ticket Ticket
	if err := db.Where("id = ? AND user_id = ?", id, actor(request)).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("工单不存在")
		}
		log.Printf("user ticket close lookup failed: %v", err)
		return s.panelError("获取工单失败")
	}
	if err := db.Model(&ticket).Update("status", 2).Error; err != nil {
		log.Printf("user ticket close update failed: %v", err)
		return s.panelError("操作失败")
	}
	return s.panel("工单已关闭")
}
