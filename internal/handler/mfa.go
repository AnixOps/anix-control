package handler

import (
	"net/http"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// MFAHandler MFA处理器
type MFAHandler struct {
	mfaService *service.MFAService
}

// NewMFAHandler 创建处理器
func NewMFAHandler() *MFAHandler {
	return &MFAHandler{
		mfaService: service.NewMFAService(database.Get(), nil),
	}
}

// GetStatus godoc
// @Summary 获取MFA状态
// @Description 用户获取自己的多因素认证状态
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/mfa/status [get]
func (h *MFAHandler) GetStatus(c *gin.Context) {
	userID := c.GetUint("user_id")

	mfa, err := h.mfaService.GetUserMFA(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if mfa == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"enabled":            false,
				"has_backup_codes":   false,
				"remaining_codes":    0,
				"last_used":          nil,
				"enforced":           false,
			},
		})
		return
	}

	remainingCodes, _ := h.mfaService.GetRemainingBackupCodes(userID)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"enabled":          mfa.Enabled,
			"has_backup_codes": len(mfa.BackupCodes) > 0,
			"remaining_codes":  remainingCodes,
			"last_used":        mfa.LastUsed,
			"last_method":      mfa.LastMethod,
		},
	})
}

// SetupTOTP godoc
// @Summary 设置TOTP
// @Description 用户设置TOTP多因素认证，返回密钥和二维码
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/mfa/totp/setup [post]
func (h *MFAHandler) SetupTOTP(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 获取用户邮箱
	var user model.User
	if err := database.Get().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	setup, err := h.mfaService.SetupTOTP(userID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"secret":       setup.Secret,
			"url":          setup.URL,
			"qr_code":      setup.QRCode,
			"backup_codes": setup.BackupCodes,
		},
	})
}

// EnableTOTP godoc
// @Summary 启用TOTP
// @Description 用户启用TOTP多因素认证，需要验证码确认
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "验证码请求 {code: string}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/mfa/totp/enable [post]
func (h *MFAHandler) EnableTOTP(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.mfaService.EnableTOTP(userID, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "MFA enabled successfully"})
}

// DisableMFA godoc
// @Summary 禁用MFA
// @Description 用户禁用多因素认证，需要密码确认
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "密码请求 {password: string}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/mfa/disable [post]
func (h *MFAHandler) DisableMFA(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.mfaService.DisableMFA(userID, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "MFA disabled successfully"})
}

// VerifyMFA godoc
// @Summary 验证MFA代码
// @Description 用户验证多因素认证代码
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "验证请求 {code: string, method: string}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/mfa/verify [post]
func (h *MFAHandler) VerifyMFA(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Code   string `json:"code" binding:"required"`
		Method string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	valid, err := h.mfaService.Verify(userID, req.Code, req.Method)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "verified successfully"})
}

// RegenerateBackupCodes godoc
// @Summary 重新生成备用码
// @Description 用户重新生成MFA备用码
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/mfa/backup-codes/regenerate [post]
func (h *MFAHandler) RegenerateBackupCodes(c *gin.Context) {
	userID := c.GetUint("user_id")

	codes, err := h.mfaService.RegenerateBackupCodes(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"backup_codes": codes,
		},
	})
}

// ========== 管理员接口 ==========

// GetAdminConfig godoc
// @Summary 获取MFA配置
// @Description 管理员获取全局MFA配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/mfa/config [get]
func (h *MFAHandler) GetAdminConfig(c *gin.Context) {
	// TODO: 从数据库或配置读取全局MFA配置
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"enabled":           h.mfaService.IsEnabled(),
			"enforce_for_all":   false,
			"enforce_for_admin": false,
			"totp_issuer":       "V2Board",
			"backup_code_count": 10,
		},
	})
}

// UpdateAdminConfig godoc
// @Summary 更新MFA配置
// @Description 管理员更新全局MFA配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "MFA配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /admin/mfa/config [put]
func (h *MFAHandler) UpdateAdminConfig(c *gin.Context) {
	var req struct {
		Enabled          bool   `json:"enabled"`
		EnforceForAll    bool   `json:"enforce_for_all"`
		EnforceForAdmin  bool   `json:"enforce_for_admin"`
		TOTPIssuer       string `json:"totp_issuer"`
		BackupCodeCount  int    `json:"backup_code_count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: 保存到数据库或配置

	c.JSON(http.StatusOK, gin.H{"message": "MFA config updated"})
}