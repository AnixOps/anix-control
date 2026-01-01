package handler

import (
	"net/http"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

// UserPlanHandler 用户端套餐处理器
type UserPlanHandler struct{}

// NewUserPlanHandler 创建套餐处理器
func NewUserPlanHandler() *UserPlanHandler {
	return &UserPlanHandler{}
}

// GetPlans 获取可购买的套餐列表
func (h *UserPlanHandler) GetPlans(c *gin.Context) {
	var plans []model.Plan
	// 仅显示已上架的套餐 (show=1)
	if err := database.GetDB().Where("show = ?", 1).Order("sort ASC").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取套餐列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plans})
}
