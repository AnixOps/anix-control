package handler

import (
	"net/http"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

// CouponHandler 用户端优惠券处理器
type CouponHandler struct{}

// NewCouponHandler 创建优惠券处理器
func NewCouponHandler() *CouponHandler {
	return &CouponHandler{}
}

// CheckCoupon 校验优惠券是否可用
func (h *CouponHandler) CheckCoupon(c *gin.Context) {
	var req struct {
		Code   string `json:"code" binding:"required"`
		PlanID uint   `json:"plan_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	var coupon model.Coupon
	if err := database.GetDB().Where("code = ?", req.Code).First(&coupon).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "无效的优惠码"})
		return
	}

	now := time.Now().Unix()
	if coupon.StartedAt > now {
		c.JSON(http.StatusBadRequest, gin.H{"message": "该优惠码尚未开始使用"})
		return
	}
	if coupon.EndedAt < now {
		c.JSON(http.StatusBadRequest, gin.H{"message": "该优惠码已过期"})
		return
	}

	if coupon.LimitUse != nil && *coupon.LimitUse > 0 && coupon.UseCount >= *coupon.LimitUse {
		c.JSON(http.StatusBadRequest, gin.H{"message": "该优惠码已达到使用次数上限"})
		return
	}

	// TODO: 校验优惠券是否适用于指定套餐 (如果模型支持的话)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":    coupon.ID,
			"name":  coupon.Name,
			"type":  coupon.Type,  // 1: 百分比, 2: 固定金额
			"value": coupon.Value, // 折扣值 (百分比[1-99] 或 金额[分])
		},
	})
}
