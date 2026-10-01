package native

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// AdminCoupons is GET /api/v2/admin/coupon: every coupon, newest first, with
// an unlimited use count shown as -1 and the creation time in Unix seconds.
func (s *Service) AdminCoupons(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("admin coupon list failed: %v", err)
		return s.panelError("获取优惠券列表失败")
	}
	var coupons []Coupon
	if err := db.Order("created_at DESC").Find(&coupons).Error; err != nil {
		log.Printf("admin coupon list failed: %v", err)
		return s.panelError("获取优惠券列表失败")
	}
	result := make([]map[string]any, 0, len(coupons))
	for _, coupon := range coupons {
		limitUse := -1
		if coupon.LimitUse != nil {
			limitUse = *coupon.LimitUse
		}
		result = append(result, map[string]any{
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
	return s.panel(result)
}

// AdminCreateCoupon is POST /api/v2/admin/coupon. A coupon starts now and
// ends in 30 days unless the request says otherwise.
func (s *Service) AdminCreateCoupon(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Code      string `json:"code" binding:"required,min=1,max=64"`
		Name      string `json:"name" binding:"required,min=1,max=255"`
		Type      int    `json:"type" binding:"required,oneof=1 2"`
		Value     int    `json:"value" binding:"required,gte=0"`
		LimitUse  int    `json:"limit_use" binding:"gte=0"`
		StartedAt int64  `json:"started_at" binding:"gte=0"`
		EndedAt   int64  `json:"ended_at" binding:"gte=0"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("admin coupon duplicate check failed: %v", err)
		return s.panelError("检查优惠码失败")
	}
	var count int64
	if err := db.Model(&Coupon{}).Where("code = ?", req.Code).Count(&count).Error; err != nil {
		log.Printf("admin coupon duplicate check failed: %v", err)
		return s.panelError("检查优惠码失败")
	}
	if count > 0 {
		return s.panelError("优惠码已存在")
	}
	if req.Type == 0 {
		req.Type = 1
	}
	now := s.now()
	if req.StartedAt == 0 {
		req.StartedAt = now.Unix()
	}
	if req.EndedAt == 0 {
		req.EndedAt = now.Add(30 * 24 * time.Hour).Unix()
	}
	limitUse := req.LimitUse
	coupon := Coupon{
		Code: req.Code, Name: req.Name, Type: req.Type, Value: req.Value, LimitUse: &limitUse, UseCount: 0,
		StartedAt: req.StartedAt, EndedAt: req.EndedAt, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&coupon).Error; err != nil {
		log.Printf("admin coupon create failed: %v", err)
		return s.panelError("创建失败")
	}
	return s.panel(map[string]any{"message": "创建成功", "data": coupon})
}

// AdminDeleteCoupon is DELETE /api/v2/admin/coupon/:id.
func (s *Service) AdminDeleteCoupon(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("无效的优惠券ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("优惠券不存在")
	}
	var coupon Coupon
	if err := db.First(&coupon, id).Error; err != nil {
		return s.panelError("优惠券不存在")
	}
	if err := db.Delete(&coupon).Error; err != nil {
		log.Printf("admin coupon delete failed: %v", err)
		return s.panelError("删除失败")
	}
	return s.panel(map[string]any{"message": "删除成功"})
}

// CheckCoupon is POST /api/v2/user/coupon/check: whether a code is usable
// now. Coupons apply to every plan.
func (s *Service) CheckCoupon(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Code   string `json:"code" binding:"required,min=1"`
		PlanID uint   `json:"plan_id" binding:"gt=0"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("coupon lookup failed: %v", err)
		return s.panelError("优惠码查询失败")
	}
	var coupon Coupon
	if err := db.Where("code = ?", req.Code).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("无效的优惠码")
		}
		log.Printf("coupon lookup failed: %v", err)
		return s.panelError("优惠码查询失败")
	}
	now := s.now().Unix()
	if coupon.StartedAt > now {
		return s.panelError("该优惠码尚未开始使用")
	}
	if coupon.EndedAt < now {
		return s.panelError("该优惠码已过期")
	}
	if coupon.LimitUse != nil && *coupon.LimitUse > 0 && coupon.UseCount >= *coupon.LimitUse {
		return s.panelError("该优惠码已达到使用次数上限")
	}
	return s.panel(map[string]any{
		"id":    coupon.ID,
		"name":  coupon.Name,
		"type":  coupon.Type,
		"value": coupon.Value,
	})
}
