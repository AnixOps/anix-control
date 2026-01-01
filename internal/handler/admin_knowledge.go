package handler

import (
	"net/http"
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取文章列表失败"})
		return
	}

	// 转换为响应格式
	var result []gin.H
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

	c.JSON(http.StatusOK, gin.H{"data": result})
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
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": article})
}

// UpdateArticle 更新文章
func (h *AdminKnowledgeHandler) UpdateArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的文章ID"})
		return
	}

	var article model.Knowledge
	if err := database.GetDB().First(&article, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "文章不存在"})
		return
	}

	var req struct {
		Category string `json:"category"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		Sort     int    `json:"sort"`
		Show     int    `json:"show"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 更新字段
	updates := map[string]interface{}{
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
	updates["sort"] = req.Sort
	updates["show"] = req.Show

	if err := database.GetDB().Model(&article).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteArticle 删除文章
func (h *AdminKnowledgeHandler) DeleteArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的文章ID"})
		return
	}

	var article model.Knowledge
	if err := database.GetDB().First(&article, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "文章不存在"})
		return
	}

	if err := database.GetDB().Delete(&article).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
