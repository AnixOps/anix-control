package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取工单列表失败"})
		return
	}

	// 转换为响应格式
	var result []gin.H
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

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ReplyTicket 回复工单
func (h *AdminTicketHandler) ReplyTicket(c *gin.Context) {
	var req struct {
		TicketID uint   `json:"ticket_id" binding:"required"`
		Message  string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 检查工单是否存在
	var ticket model.Ticket
	if err := database.GetDB().First(&ticket, req.TicketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "工单不存在"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "回复失败"})
		return
	}

	// 更新工单状态为已回复
	database.GetDB().Model(&ticket).Update("status", 1)

	c.JSON(http.StatusOK, gin.H{"message": "回复成功"})
}

// CloseTicket 关闭工单
func (h *AdminTicketHandler) CloseTicket(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的工单ID"})
		return
	}

	var ticket model.Ticket
	if err := database.GetDB().First(&ticket, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "工单不存在"})
		return
	}

	// 更新状态为已关闭
	if err := database.GetDB().Model(&ticket).Update("status", 2).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "关闭失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "工单已关闭", "id": id})
}
