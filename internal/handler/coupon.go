package handler

import (
	"errors"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
		Code   string `json:"code" binding:"required,min=1"`
		PlanID uint   `json:"plan_id" binding:"gt=0"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误")
		return
	}

	var coupon model.Coupon
	if err := database.GetDB().Where("code = ?", req.Code).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "无效的优惠码")
			return
		}
		log.Printf("coupon lookup failed: %v", err)
		panelError(c, "优惠码查询失败")
		return
	}

	now := time.Now().Unix()
	if coupon.StartedAt > now {
		panelError(c, "该优惠码尚未开始使用")
		return
	}
	if coupon.EndedAt < now {
		panelError(c, "该优惠码已过期")
		return
	}

	if coupon.LimitUse != nil && *coupon.LimitUse > 0 && coupon.UseCount >= *coupon.LimitUse {
		panelError(c, "该优惠码已达到使用次数上限")
		return
	}

	// Coupon-plan applicability check stub:
	// If the coupon model gains plan-specific or category-specific fields, validate here:
	//   1. Check if coupon has a list of applicable plan IDs
	//   2. Verify the target planID is in that list
	//   3. Return an error if the coupon cannot be applied to this plan
	// Current implementation has no plan-coupon mapping, so all coupons are considered applicable.

	panelSuccess(c, gin.H{
		"id":    coupon.ID,
		"name":  coupon.Name,
		"type":  coupon.Type,  // 1: 百分比, 2: 固定金额
		"value": coupon.Value, // 折扣值 (百分比[1-99] 或 金额[分])
	})
}
