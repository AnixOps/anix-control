package handler

import (
	"net/http"

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
// @Success 200 {object} map[string]interface{} "注册成功"
// @Failure 400 {object} map[string]interface{} "参数错误"
// @Router /register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 验证密码长度
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "密码长度至少6位"})
		return
	}

	// 验证邮箱格式
	if len(req.Email) < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请输入有效的邮箱地址"})
		return
	}

	token, user, err := h.authService.Register(req.Email, req.Password, h.cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "注册成功",
		"data": gin.H{
			"token":    token,
			"is_admin": user.IsAdmin == 1,
			"user_id":  user.ID,
			"email":    user.Email,
		},
	})
}

// Login godoc
// @Summary 用户登录
// @Description 用户登录获取 JWT Token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.LoginRequest true "登录请求"
// @Success 200 {object} map[string]interface{} "登录成功"
// @Failure 401 {object} map[string]interface{} "认证失败"
// @Router /login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	token, user, err := h.authService.Login(req.Email, req.Password, h.cfg)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"token":    token,
			"is_admin": user.IsAdmin == 1,
			"user_id":  user.ID,
			"email":    user.Email,
		},
	})
}
