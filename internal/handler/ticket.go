package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

// TicketHandler 用户端工单处理器
type TicketHandler struct{}

// NewTicketHandler 创建工单处理器
func NewTicketHandler() *TicketHandler {
	return &TicketHandler{}
}

// GetTickets 获取用户自己的工单列表
func (h *TicketHandler) GetTickets(c *gin.Context) {
	userID := c.GetUint("user_id")
	var tickets []model.Ticket
	if err := database.GetDB().Where("user_id = ?", userID).Order("updated_at DESC").Find(&tickets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取工单失败"})
		return
	}

	var result []gin.H
	for _, t := range tickets {
		result = append(result, gin.H{
			"id":         t.ID,
			"subject":    t.Subject,
			"level":      t.Level,
			"status":     t.Status,
			"created_at": t.CreatedAt.Unix(),
			"updated_at": t.UpdatedAt.Unix(),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateTicket 提交新工单
func (h *TicketHandler) CreateTicket(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Subject string `json:"subject" binding:"required"`
		Level   int    `json:"level"`
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	tx := database.GetDB().Begin()

	ticket := model.Ticket{
		UserID:    userID,
		Subject:   req.Subject,
		Level:     req.Level,
		Status:    0, // open
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := tx.Create(&ticket).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建工单失败"})
		return
	}

	message := model.TicketMessage{
		TicketID:  ticket.ID,
		UserID:    userID,
		Message:   req.Message,
		IsAdmin:   0,
		CreatedAt: time.Now(),
	}

	if err := tx.Create(&message).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "发送消息失败"})
		return
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "工单提交成功", "data": ticket})
}

// GetTicket 获取工单详情及回话
func (h *TicketHandler) GetTicket(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的工单ID"})
		return
	}

	var ticket model.Ticket
	if err := database.GetDB().Preload("Messages").Where("id = ? AND user_id = ?", id, userID).First(&ticket).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "工单不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

// ReplyTicket 回复自己的工单
func (h *TicketHandler) ReplyTicket(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的工单ID"})
		return
	}

	var req struct {
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	var ticket model.Ticket
	if err := database.GetDB().Where("id = ? AND user_id = ?", id, userID).First(&ticket).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "工单不存在"})
		return
	}

	if ticket.Status == 2 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "工单已关闭，无法回复"})
		return
	}

	message := model.TicketMessage{
		TicketID:  uint(id),
		UserID:    userID,
		Message:   req.Message,
		IsAdmin:   0,
		CreatedAt: time.Now(),
	}

	if err := database.GetDB().Create(&message).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "回复失败"})
		return
	}

	// 用户回复后，状态再次变回待理 (0: open)
	database.GetDB().Model(&ticket).Updates(map[string]interface{}{
		"status":     0,
		"updated_at": time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"message": "回复成功"})
}

// CloseTicket 用户主动关闭工单
func (h *TicketHandler) CloseTicket(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的工单ID"})
		return
	}

	var ticket model.Ticket
	if err := database.GetDB().Where("id = ? AND user_id = ?", id, userID).First(&ticket).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "工单不存在"})
		return
	}

	if err := database.GetDB().Model(&ticket).Update("status", 2).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "操作失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "工单已关闭"})
}
