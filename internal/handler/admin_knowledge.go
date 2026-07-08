package handler

import (
	"log"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

// AdminKnowledgeHandler 知识库处理器
type AdminKnowledgeHandler struct{}

// NewAdminKnowledgeHandler 创建知识库处理器
func NewAdminKnowledgeHandler() *AdminKnowledgeHandler {
	return &AdminKnowledgeHandler{}
}

// GetArticles 获取文章列表
func (h *AdminKnowledgeHandler) GetArticles(c *gin.Context) {
	var articles []model.Knowledge
	if err := database.GetDB().Order("sort ASC, created_at DESC").Find(&articles).Error; err != nil {
		log.Printf("admin knowledge list failed: %v", err)
		panelError(c, "获取文章列表失败")
		return
	}

	// 转换为响应格式
	result := make([]gin.H, 0, len(articles))
	for _, a := range articles {
		result = append(result, gin.H{
			"id":         a.ID,
			"category":   a.Category,
			"title":      a.Title,
			"body":       a.Body,
			"sort":       a.Sort,
			"show":       a.Show,
			"created_at": a.CreatedAt.Unix(),
			"updated_at": a.UpdatedAt.Unix(),
		})
	}

	panelSuccess(c, result)
}

// CreateArticle 创建文章
func (h *AdminKnowledgeHandler) CreateArticle(c *gin.Context) {
	var req struct {
		Category string `json:"category"`
		Title    string `json:"title" binding:"required"`
		Body     string `json:"body" binding:"required"`
		Sort     int    `json:"sort"`
		Show     int    `json:"show"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	// 设置默认值
	if req.Category == "" {
		req.Category = "公告"
	}
	if req.Show == 0 {
		req.Show = 1
	}

	article := model.Knowledge{
		Category:  req.Category,
		Title:     req.Title,
		Body:      req.Body,
		Sort:      req.Sort,
		Show:      req.Show,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := database.GetDB().Create(&article).Error; err != nil {
		log.Printf("admin knowledge create failed: %v", err)
		panelError(c, "创建失败")
		return
	}

	panelSuccess(c, gin.H{
		"message": "创建成功",
		"data":    article,
	})
}

// UpdateArticle 更新文章
func (h *AdminKnowledgeHandler) UpdateArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的文章ID")
		return
	}

	var article model.Knowledge
	if err := database.GetDB().First(&article, id).Error; err != nil {
		panelError(c, "文章不存在")
		return
	}

	var req struct {
		Category string `json:"category" binding:"omitempty,max=64"`
		Title    string `json:"title" binding:"omitempty,min=1,max=255"`
		Body     string `json:"body" binding:"omitempty"`
		Sort     *int   `json:"sort"`
		Show     *int   `json:"show" binding:"omitempty,oneof=0 1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	// 更新字段
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Body != "" {
		updates["body"] = req.Body
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Show != nil {
		updates["show"] = *req.Show
	}

	if err := database.GetDB().Model(&article).Updates(updates).Error; err != nil {
		log.Printf("admin knowledge update failed: %v", err)
		panelError(c, "更新失败")
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// DeleteArticle 删除文章
func (h *AdminKnowledgeHandler) DeleteArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的文章ID")
		return
	}

	var article model.Knowledge
	if err := database.GetDB().First(&article, id).Error; err != nil {
		panelError(c, "文章不存在")
		return
	}

	if err := database.GetDB().Delete(&article).Error; err != nil {
		log.Printf("admin knowledge delete failed: %v", err)
		panelError(c, "删除失败")
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}
