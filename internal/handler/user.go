package handler

import (
	"errors"
	"log"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UserHandler 用户处理器
type UserHandler struct {
	statsService  *service.StatsService
	userService   *service.UserService
	configService *service.SystemConfigService
	cfg           *config.Config
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	return &UserHandler{
		statsService:  service.NewStatsService(),
		userService:   service.NewUserService(),
		configService: service.NewSystemConfigService(database.Get()),
		cfg:           config.Get(),
	}
}

func currentPanelUserID(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		panelError(c, "未登录")
		return 0, false
	}

	uid, ok := userID.(uint)
	if !ok {
		panelError(c, "用户ID无效")
		return 0, false
	}

	return uid, true
}

// GetSubscription godoc
// @Summary 获取用户订阅详情
// @Description 获取当前登录用户的订阅信息，包括流量、到期时间等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param refresh query bool false "是否强制刷新缓存"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /user/subscription [get]
func (h *UserHandler) GetSubscription(c *gin.Context) {
	uid, ok := currentPanelUserID(c)
	if !ok {
		return
	}

	// 检查是否强制刷新
	forceRefresh := c.Query("refresh") == "true"

	sub, err := h.statsService.GetUserSubscription(uid, forceRefresh)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "用户不存在")
			return
		}
		log.Printf("user subscription failed: %v", err)
		panelError(c, "获取订阅信息失败")
		return
	}

	settings := service.GetSubscriptionSettings(h.configService, h.cfg)
	sub.SubscribePath = settings.SubscribePath
	sub.SubscribeDomains = settings.SubscribeDomains

	panelSuccess(c, sub)
}

// GetProfile godoc
// @Summary 获取用户基本信息
// @Description 获取当前登录用户的基本信息，包括邮箱、UUID、Token等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /user/profile [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
	uid, ok := currentPanelUserID(c)
	if !ok {
		return
	}

	user, err := h.userService.GetByID(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "用户不存在")
			return
		}
		log.Printf("user profile failed: %v", err)
		panelError(c, "获取用户信息失败")
		return
	}
	pluginAccess, err := service.ResolveActorPluginAccess(database.Get(), uid, user.IsAdmin == 1)
	if err != nil {
		log.Printf("user plugin permissions failed: %v", err)
		panelError(c, "获取用户权限失败")
		return
	}

	panelSuccess(c, gin.H{
		"id":                 user.ID,
		"email":              user.Email,
		"uuid":               user.UUID,
		"token":              user.Token,
		"is_admin":           user.IsAdmin == 1,
		"permission_mode":    pluginAccess.PermissionMode(),
		"permissions":        pluginAccess.ProfilePermissions(),
		"restricted_plugins": pluginAccess.ProfileRestrictedPluginList(),
	})
}

// GetDashboard godoc
// @Summary 获取用户仪表盘
// @Description 获取当前登录用户的仪表盘数据，包括订阅信息等
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /user/dashboard [get]
func (h *UserHandler) GetDashboard(c *gin.Context) {
	uid, ok := currentPanelUserID(c)
	if !ok {
		return
	}

	// 获取订阅信息
	sub, err := h.statsService.GetUserSubscription(uid, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "用户不存在")
			return
		}
		log.Printf("user dashboard failed: %v", err)
		panelError(c, "获取信息失败")
		return
	}

	panelSuccess(c, gin.H{
		"subscription": sub,
	})
}
