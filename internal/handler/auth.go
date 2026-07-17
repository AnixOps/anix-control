package handler

import (
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
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

	user, err := h.authService.Authenticate(req.Email, req.Password)
	if err != nil {
		service.GetLoginRateLimiter().RecordFailure(loginRateLimitKey, loginRateLimitOptions)
		panelError(c, err.Error())
		return
	}

	mfaHandled, err := h.handleLoginMFA(c, user, req, loginRateLimitKey, loginRateLimitOptions)
	if err != nil {
		panelError(c, err.Error())
		return
	}
	if mfaHandled {
		return
	}

	token, err := h.authService.IssueToken(user, h.cfg)
	if err != nil {
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

func (h *AuthHandler) handleLoginMFA(c *gin.Context, user *model.User, req model.LoginRequest, loginRateLimitKey string, loginRateLimitOptions service.LoginRateLimitOptions) (bool, error) {
	db := database.GetDB()
	mfaService := service.NewMFAService(db, nil)
	mfaConfig, err := loadMFAAdminConfig(db)
	if err != nil {
		return false, err
	}
	mfaService.SetConfig(mfaRuntimeConfig(mfaConfig))

	mfa, err := mfaService.GetUserMFA(user.ID)
	if err != nil {
		return false, err
	}
	if mfa == nil || !mfa.Enabled {
		if mfaService.IsEnforcedForUser(user) {
			methods := loginMFAEnrollmentMethods(mfaConfig)
			panelSuccess(c, gin.H{
				"mfa_enrollment_required": true,
				"mfa_setup_required":      true,
				"methods":                 methods,
				"mfa_methods":             methods,
				"user_id":                 user.ID,
				"email":                   user.Email,
			})
			return true, nil
		}
		return false, nil
	}

	code := strings.TrimSpace(req.MFACode)
	method := strings.ToLower(strings.TrimSpace(req.MFAMethod))
	if code == "" {
		methods := loginMFAMethods(mfa)
		panelSuccess(c, gin.H{
			"mfa_required": true,
			"methods":      methods,
			"mfa_methods":  methods,
			"user_id":      user.ID,
			"email":        user.Email,
		})
		return true, nil
	}

	valid, err := mfaService.Verify(user.ID, code, method)
	if err != nil {
		return false, err
	}

	attemptMethod := method
	if attemptMethod == "" {
		attemptMethod = "auto"
	}
	if err := mfaService.RecordLoginAttempt(user.ID, c.ClientIP(), c.GetHeader("User-Agent"), valid, attemptMethod); err != nil {
		return false, err
	}

	if !valid {
		service.GetLoginRateLimiter().RecordFailure(loginRateLimitKey, loginRateLimitOptions)
		panelError(c, "invalid mfa code")
		return true, nil
	}

	return false, nil
}

func loginMFAEnrollmentMethods(cfg mfaAdminConfig) []string {
	methods := make([]string, 0, 1)
	if cfg.Methods[model.MFAMethodTOTP] || len(cfg.Methods) == 0 {
		methods = append(methods, model.MFAMethodTOTP)
	}
	if len(methods) == 0 {
		methods = append(methods, model.MFAMethodTOTP)
	}
	return methods
}

func loginMFAMethods(mfa *model.UserMFA) []string {
	methods := make([]string, 0, 2)
	if strings.TrimSpace(mfa.TOTPSecret) != "" {
		methods = append(methods, model.MFAMethodTOTP)
	}
	if strings.TrimSpace(mfa.BackupCodes) != "" {
		methods = append(methods, model.MFAMethodBackup)
	}
	if len(methods) == 0 {
		methods = append(methods, model.MFAMethodTOTP)
	}
	return methods
}
