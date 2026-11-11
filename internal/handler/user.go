package handler

import (
	"github.com/gin-gonic/gin"
)

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
