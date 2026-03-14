package handler

import (
	"net/http"

	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户处理器
type UserHandler struct {
	statsService *service.StatsService
	userService  *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	return &UserHandler{
		statsService: service.NewStatsService(),
		userService:  service.NewUserService(),
	}
}

// GetSubscription godoc
// @Summary 获取用户订阅详情
// @Description 获取当前登录用户的订阅信息，包括流量、到期时间等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param refresh query bool false "是否强制刷新缓存"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/subscription [get]
func (h *UserHandler) GetSubscription(c *gin.Context) {
	// 从 JWT 中获取用户 ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户ID无效"})
		return
	}

	// 检查是否强制刷新
	forceRefresh := c.Query("refresh") == "true"

	sub, err := h.statsService.GetUserSubscription(uid, forceRefresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取订阅信息失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sub})
}

// GetProfile godoc
// @Summary 获取用户基本信息
// @Description 获取当前登录用户的基本信息，包括邮箱、UUID、Token等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /user/profile [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户ID无效"})
		return
	}

	user, err := h.userService.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"uuid":     user.UUID,
			"token":    user.Token,
			"is_admin": user.IsAdmin == 1,
		},
	})
}

// GetDashboard godoc
// @Summary 获取用户仪表盘
// @Description 获取当前登录用户的仪表盘数据，包括订阅信息等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/dashboard [get]
func (h *UserHandler) GetDashboard(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户ID无效"})
		return
	}

	// 获取订阅信息
	sub, err := h.statsService.GetUserSubscription(uid, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取信息失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"subscription": sub,
		},
	})
}
