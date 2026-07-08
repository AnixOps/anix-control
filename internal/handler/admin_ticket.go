package handler

import (
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminTicketHandler 工单处理器
type AdminTicketHandler struct{}

// NewAdminTicketHandler 创建工单处理器
func NewAdminTicketHandler() *AdminTicketHandler {
	return &AdminTicketHandler{}
}

// GetTickets 获取工单列表
func (h *AdminTicketHandler) GetTickets(c *gin.Context) {
	var tickets []model.Ticket
	if err := database.GetDB().Preload("User").Order("created_at DESC").Find(&tickets).Error; err != nil {
		log.Printf("admin ticket list failed: %v", err)
		panelError(c, "获取工单列表失败")
		return
	}

	// 转换为响应格式
	result := make([]gin.H, 0, len(tickets))
	for _, t := range tickets {
		result = append(result, gin.H{
			"id":         t.ID,
			"user_id":    t.UserID,
			"subject":    t.Subject,
			"level":      t.Level,
			"status":     t.Status,
			"created_at": t.CreatedAt.Unix(),
			"updated_at": t.UpdatedAt.Unix(),
		})
	}

	panelSuccess(c, result)
}

// ReplyTicket 回复工单
func (h *AdminTicketHandler) ReplyTicket(c *gin.Context) {
	var req struct {
		TicketID uint   `json:"ticket_id" binding:"required"`
		Message  string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	// 检查工单是否存在
	var ticket model.Ticket
	if err := database.GetDB().First(&ticket, req.TicketID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "工单不存在")
			return
		}
		log.Printf("admin ticket reply lookup failed: %v", err)
		panelError(c, "获取工单失败")
		return
	}

	// 创建回复消息
	userID := c.GetUint("user_id")
	message := model.TicketMessage{
		TicketID:  req.TicketID,
		UserID:    userID,
		Message:   req.Message,
		IsAdmin:   1,
		CreatedAt: time.Now(),
	}

	if err := database.GetDB().Create(&message).Error; err != nil {
		log.Printf("admin ticket reply create failed: %v", err)
		panelError(c, "回复失败")
		return
	}

	// 更新工单状态为已回复
	if err := database.GetDB().Model(&ticket).Update("status", 1).Error; err != nil {
		log.Printf("admin ticket status update after reply failed: %v", err)
		panelError(c, "更新工单状态失败")
		return
	}

	panelSuccess(c, gin.H{"message": "回复成功"})
}

// CloseTicket 关闭工单
func (h *AdminTicketHandler) CloseTicket(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的工单ID")
		return
	}

	var ticket model.Ticket
	if err := database.GetDB().First(&ticket, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "工单不存在")
			return
		}
		log.Printf("admin ticket close lookup failed: %v", err)
		panelError(c, "获取工单失败")
		return
	}

	// 更新状态为已关闭
	if err := database.GetDB().Model(&ticket).Update("status", 2).Error; err != nil {
		log.Printf("admin ticket close update failed: %v", err)
		panelError(c, "关闭失败")
		return
	}

	panelSuccess(c, gin.H{"message": "工单已关闭", "id": id})
}
