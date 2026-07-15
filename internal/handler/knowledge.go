package handler

import (
	"errors"
	"log"
	"strconv"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// KnowledgeHandler 用户端知识库处理器
type KnowledgeHandler struct{}

// NewKnowledgeHandler 创建知识库处理器
func NewKnowledgeHandler() *KnowledgeHandler {
	return &KnowledgeHandler{}
}

// GetArticles 获取文章列表 (仅可见且按排序)
func (h *KnowledgeHandler) GetArticles(c *gin.Context) {
	var articles []model.Knowledge
	if err := database.GetDB().Where("show = ?", 1).Order("sort ASC, created_at DESC").Find(&articles).Error; err != nil {
		log.Printf("user knowledge list failed: %v", err)
		panelError(c, "获取知识库失败")
		return
	}

	// 转换为响应格式，隐藏敏感或不需要的后端字段
	var result []gin.H
	for _, a := range articles {
		result = append(result, gin.H{
			"id":         a.ID,
			"category":   a.Category,
			"title":      a.Title,
			"body":       a.Body, // 普通用户也需要看到内容
			"updated_at": a.UpdatedAt.Unix(),
		})
	}

	panelSuccess(c, result)
}

// GetArticle 获取单篇文章详情
func (h *KnowledgeHandler) GetArticle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "无效的文章ID")
		return
	}

	var article model.Knowledge
	if err := database.GetDB().Where("id = ? AND show = ?", id, 1).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "文章不存在")
			return
		}
		log.Printf("user knowledge detail failed: %v", err)
		panelError(c, "获取知识库失败")
		return
	}

	panelSuccess(c, gin.H{
		"id":         article.ID,
		"category":   article.Category,
		"title":      article.Title,
		"body":       article.Body,
		"updated_at": article.UpdatedAt.Unix(),
	})
}
