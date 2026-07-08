package handler

import (
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
	cfg         *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService: service.NewAuthService(),
		cfg:         cfg,
	}
}

// Register godoc
// @Summary 用户注册
// @Description 创建新用户账户
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.RegisterRequest true "注册请求"
// @Success 200 {object} map[string]any "注册成功"
// @Failure 400 {object} map[string]any "参数错误"
// @Router /register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	registrationPolicy := service.ResolveRegistrationPolicy(h.cfg)
	if !registrationPolicy.Enabled {
		panelError(c, "registration is disabled")
		return
	}

	registerRateLimitOptions := service.ResolveRegisterRateLimitOptions(h.cfg)
	registerRateLimitKey := service.BuildRegisterRateLimitKey(c.ClientIP())
	if blocked, retryAfter := service.GetLoginRateLimiter().Check(registerRateLimitKey, registerRateLimitOptions); blocked {
		retryAfterSeconds := int(retryAfter.Seconds())
		if retryAfterSeconds < 1 {
			retryAfterSeconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		panelError(c, "too many registration attempts, please try again later")
		return
	}
	service.GetLoginRateLimiter().RecordFailure(registerRateLimitKey, registerRateLimitOptions)

	// 验证密码长度
	if len(req.Password) < 6 {
		panelError(c, "密码长度至少6位")
		return
	}

	// 验证邮箱格式
	if len(req.Email) < 5 {
		panelError(c, "请输入有效的邮箱地址")
		return
	}

	if err := service.ValidateRegistrationEmail(req.Email, registrationPolicy); err != nil {
		panelError(c, err.Error())
		return
	}
	if registrationPolicy.RequireInvite && strings.TrimSpace(req.InviteCode) == "" {
		panelError(c, "invite code is required")
		return
	}

	token, user, err := h.authService.RegisterWithInvite(req.Email, req.Password, req.InviteCode, h.cfg)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{
		"token":    token,
		"is_admin": user.IsAdmin == 1,
		"user_id":  user.ID,
		"email":    user.Email,
	})
}

// Login godoc
// @Summary 用户登录
// @Description 用户登录获取 JWT Token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.LoginRequest true "登录请求"
// @Success 200 {object} map[string]any "登录成功"
// @Failure 401 {object} map[string]any "认证失败"
// @Router /login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	loginRateLimitOptions := service.ResolveLoginRateLimitOptions(h.cfg)
	loginRateLimitKey := service.BuildLoginRateLimitKey(req.Email, c.ClientIP())
	if blocked, retryAfter := service.GetLoginRateLimiter().Check(loginRateLimitKey, loginRateLimitOptions); blocked {
		retryAfterSeconds := int(retryAfter.Seconds())
		if retryAfterSeconds < 1 {
			retryAfterSeconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		panelError(c, "too many login attempts, please try again later")
		return
	}

	token, user, err := h.authService.Login(req.Email, req.Password, h.cfg)
	if err != nil {
		service.GetLoginRateLimiter().RecordFailure(loginRateLimitKey, loginRateLimitOptions)
		panelError(c, err.Error())
		return
	}
	service.GetLoginRateLimiter().RecordSuccess(loginRateLimitKey)

	panelSuccess(c, gin.H{
		"token":    token,
		"is_admin": user.IsAdmin == 1,
		"user_id":  user.ID,
		"email":    user.Email,
	})
}
