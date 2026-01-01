package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

// AdminCouponHandler 优惠券处理器
type AdminCouponHandler struct{}

// NewAdminCouponHandler 创建优惠券处理器
func NewAdminCouponHandler() *AdminCouponHandler {
	return &AdminCouponHandler{}
}

// GetCoupons 获取优惠券列表
func (h *AdminCouponHandler) GetCoupons(c *gin.Context) {
	var coupons []model.Coupon
	if err := database.GetDB().Order("created_at DESC").Find(&coupons).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取优惠券列表失败"})
		return
	}

	// 转换为响应格式
	var result []gin.H
	for _, coupon := range coupons {
		limitUse := -1
		if coupon.LimitUse != nil {
			limitUse = *coupon.LimitUse
		}

		result = append(result, gin.H{
			"id":         coupon.ID,
			"code":       coupon.Code,
			"name":       coupon.Name,
			"type":       coupon.Type,
			"value":      coupon.Value,
			"limit_use":  limitUse,
			"use_count":  coupon.UseCount,
			"started_at": coupon.StartedAt,
			"ended_at":   coupon.EndedAt,
			"created_at": coupon.CreatedAt.Unix(),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateCoupon 创建优惠券
func (h *AdminCouponHandler) CreateCoupon(c *gin.Context) {
	var req struct {
		Code      string `json:"code" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Type      int    `json:"type"`
		Value     int    `json:"value"`
		LimitUse  int    `json:"limit_use"`
		StartedAt int64  `json:"started_at"`
		EndedAt   int64  `json:"ended_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 检查优惠码是否已存在
	var count int64
	database.GetDB().Model(&model.Coupon{}).Where("code = ?", req.Code).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "优惠码已存在"})
		return
	}

	// 设置默认值
	if req.Type == 0 {
		req.Type = 1 // 默认折扣类型
	}
	if req.StartedAt == 0 {
		req.StartedAt = time.Now().Unix()
	}
	if req.EndedAt == 0 {
		req.EndedAt = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	limitUse := req.LimitUse
	coupon := model.Coupon{
		Code:      req.Code,
		Name:      req.Name,
		Type:      req.Type,
		Value:     req.Value,
		LimitUse:  &limitUse,
		UseCount:  0,
		StartedAt: req.StartedAt,
		EndedAt:   req.EndedAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := database.GetDB().Create(&coupon).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": coupon})
}

// DeleteCoupon 删除优惠券
func (h *AdminCouponHandler) DeleteCoupon(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的优惠券ID"})
		return
	}

	var coupon model.Coupon
	if err := database.GetDB().First(&coupon, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "优惠券不存在"})
		return
	}

	if err := database.GetDB().Delete(&coupon).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
