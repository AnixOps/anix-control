package handler

import (
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// subscriptionResetRequest is the re-authentication a user's own
// subscription reset needs: the current password, or a TOTP or recovery
// code when the account has a second factor (method "totp" or "backup";
// empty tries TOTP, then a recovery code).
type subscriptionResetRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
	Method   string `json:"method"`
}

// ResetSubscription godoc
// @Summary 重置我的订阅链接
// @Description 用户为自己重新生成订阅 token，旧订阅链接立即失效（UUID 不变，与管理员重置相同）。需重新验证身份：未开启两步验证时提交当前密码，已开启时提交验证码或恢复码。每个用户每小时最多尝试 3 次。
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body subscriptionResetRequest true "password，或 code 与可选的 method（totp / backup）"
// @Success 200 {object} map[string]any "data.token 为新的订阅 token"
// @Failure 400 {object} map[string]any
// @Router /user/subscription/reset [post]
func (h *UserHandler) ResetSubscription(c *gin.Context) {
	var req subscriptionResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}
	userID, ok := currentPanelUserID(c)
	if !ok {
		return
	}
	limiter, key, limit := service.GetSubscriptionResetLimiter(), service.SubscriptionResetRateLimitKey(userID), service.SubscriptionResetRateLimit
	if blocked, wait := limiter.Check(key, limit); blocked {
		c.Header("Retry-After", strconv.Itoa(max(int(wait.Seconds()), 1)))
		panelError(c, service.SubscriptionResetRateLimitedMessage)
		return
	}
	checked, err := h.userService.VerifyStepUp(userID, req.Password, req.Code, req.Method)
	if checked {
		limiter.RecordFailure(key, limit)
	}
	if err != nil {
		panelError(c, err.Error())
		return
	}
	token, err := h.userService.ResetToken(userID, service.UserSubscriptionResetRequestID(userID, assignmentToken(c)))
	if err != nil {
		panelAdminUserError(c, service.SubscriptionResetFailedMessage, err)
		return
	}
	panelSuccess(c, gin.H{"token": token})
}
