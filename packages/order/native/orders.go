package native

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// AdminOrderStats is GET /api/v2/admin/orders/stats. Today starts at
// midnight UTC.
func (s *Service) AdminOrderStats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stats, err := s.orderStats(ctx)
	if err != nil {
		return s.panelError("获取统计失败: " + err.Error())
	}
	return s.panel(stats)
}

func (s *Service) orderStats(ctx context.Context) (map[string]any, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	var totalOrders, pendingOrders, paidOrders, totalRevenue int64
	if err := db.Model(&Order{}).Count(&totalOrders).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&Order{}).Where("status = 0").Count(&pendingOrders).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&Order{}).Where("status IN (1, 3)").Count(&paidOrders).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&Order{}).Where("status IN (1, 3)").Select("COALESCE(SUM(total_amount), 0)").Scan(&totalRevenue).Error; err != nil {
		return nil, err
	}
	today := s.now().Truncate(24 * time.Hour).Unix()
	var todayRevenue int64
	if err := db.Model(&Order{}).
		Where("status IN (1, 3)").
		Where("paid_at >= ?", today).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&todayRevenue).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"total_orders":   totalOrders,
		"pending_orders": pendingOrders,
		"paid_orders":    paidOrders,
		"total_revenue":  totalRevenue,
		"today_revenue":  todayRevenue,
	}, nil
}

// updateStatus sets an order's status as the kernel's
// OrderService.UpdateStatus does; a paid order gets its payment time.
func (s *Service) updateStatus(db *gorm.DB, orderID uint, orderStatus int) error {
	if orderID == 0 {
		return errOrderNotFound
	}
	var order Order
	if err := db.Select("id").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errOrderNotFound
		}
		return err
	}
	updates := map[string]any{"status": orderStatus}
	if orderStatus == 1 {
		updates["paid_at"] = s.now().Unix()
	}
	return db.Model(&Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// AdminUpdateOrderStatus is PUT /api/v2/admin/orders/:id/status. Setting
// "completed" this way grants nothing, as in the kernel.
func (s *Service) AdminUpdateOrderStatus(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("无效的订单ID")
	}
	var req struct {
		Status int `json:"status" binding:"required,oneof=0 1 2 3"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.adminOrderError("更新失败", err)
	}
	if err := s.updateStatus(db, uint(id), req.Status); err != nil {
		return s.adminOrderError("更新失败", err)
	}
	return s.panel(map[string]any{"message": "更新成功"})
}

// AdminCancelOrder is POST /api/v2/admin/orders/:id/cancel.
func (s *Service) AdminCancelOrder(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("无效的订单ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.adminOrderError("取消失败", err)
	}
	if err := s.updateStatus(db, uint(id), 2); err != nil {
		return s.adminOrderError("取消失败", err)
	}
	return s.panel(map[string]any{"message": "取消成功"})
}

// AdminMarkOrderPaid is POST /api/v2/admin/orders/:id/paid: the order is
// marked paid, then completed, as the kernel's handler does.
func (s *Service) AdminMarkOrderPaid(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("无效的订单ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.adminOrderError("更新失败", err)
	}
	if err := s.updateStatus(db, uint(id), 1); err != nil {
		return s.adminOrderError("更新失败", err)
	}
	if err := s.complete(ctx, db, uint(id)); err != nil {
		return s.adminOrderError("订单完成失败", err)
	}
	return s.panel(map[string]any{"message": "订单已开通"})
}

// periodMonths maps an order period to months, as the kernel's
// subscriber.PeriodMonths; an unknown period is no extension.
var periodMonths = map[string]int32{
	"month": 1, "quarter": 3, "half_year": 6, "year": 12, "two_year": 24, "three_year": 36, "onetime": 1200,
}

// errSubscriberNotFound is the kernel's subscriber.ErrSubscriberNotFound.
var errSubscriberNotFound = errors.New("subscriber not found")

// CompleteRequestID names an order's completion in the kernel's subscriber
// request ledger, as the kernel's completion (OrderService.Complete and the
// payment callbacks) does, so an order's plan is granted once whichever
// side completes it.
func CompleteRequestID(orderID uint) string {
	return fmt.Sprintf("order:%d", orderID)
}

// complete grants a paid order's plan through KernelSubscriber and marks it
// completed, as the kernel's completeOrderTx: the plan's group, transfer,
// limits and subscription groups, traffic reset, and the period added to
// the current expiry when the buyer holds the same, unexpired plan, else to
// now.
//
// The kernel does it in one transaction under the order's row lock. Here
// the grant is the kernel's transaction and the status change follows it;
// if the latter fails the order stays paid with its plan granted, and a
// retry grants nothing again (the request id is applied once) and completes
// it.
func (s *Service) complete(ctx context.Context, db *gorm.DB, orderID uint) error {
	if orderID == 0 {
		return errOrderNotFound
	}
	var order Order
	if err := db.First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errOrderNotFound
		}
		return err
	}
	if order.Status != 1 {
		return errors.New("订单状态不处于已支付，无法完成")
	}
	var plan CatalogPlan
	if err := db.First(&plan, order.PlanID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errOrderPlanNotFound
		}
		return err
	}
	var groups []PlanGroup
	if err := db.Where("plan_id = ?", plan.ID).Find(&groups).Error; err != nil {
		return err
	}
	// Subscriber ids fit the contract's range; another id names no one.
	if order.UserID == 0 || uint64(order.UserID) > math.MaxUint32 {
		return errSubscriberNotFound
	}
	_, err := s.Subscriber.ApplyEntitlement(ctx, &kernelsubscriberv1.ApplyEntitlementRequest{
		RequestId: CompleteRequestID(order.ID), UserId: uint64(order.UserID), Plan: snapshot(plan, groups),
		Expiry:        &kernelsubscriberv1.ApplyEntitlementRequest_Period{Period: &kernelsubscriberv1.Period{Months: periodMonths[order.Period]}},
		RenewSamePlan: true, ResetTraffic: true, Reason: "order completion",
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errSubscriberNotFound
		}
		return errors.New(status.Convert(err).Message())
	}
	return db.Model(&order).Update("status", 3).Error
}

// bytesPerGiB converts a plan's transfer (GiB) to bytes.
const bytesPerGiB = 1 << 30

// maxTransferBytes is the kernel's ceiling for a snapshot's transfer.
const maxTransferBytes = 1 << 62

// snapshot is what completing an order grants, as the kernel's
// subscriber.PlanSnapshotFromPlan builds it.
//
// The contract carries the transfer as unsigned bytes: a plan with a
// negative transfer grants none, where the kernel stored the negative value;
// both serve nothing. A device limit beyond int32 is capped.
func snapshot(plan CatalogPlan, groups []PlanGroup) *kernelsubscriberv1.PlanSnapshot {
	out := &kernelsubscriberv1.PlanSnapshot{PlanId: uint64(plan.ID), GroupId: uint64(plan.GroupID)}
	switch {
	case plan.TransferEnable <= 0:
	case plan.TransferEnable >= maxTransferBytes/bytesPerGiB:
		out.TransferBytes = maxTransferBytes
	default:
		out.TransferBytes = uint64(plan.TransferEnable) * bytesPerGiB
	}
	if plan.SpeedLimit != nil {
		speed := *plan.SpeedLimit
		out.SpeedLimitMbps = &speed
	}
	if plan.DeviceLimit != nil {
		devices := int32(max(min(*plan.DeviceLimit, math.MaxInt32), math.MinInt32)) // #nosec G115 -- clamped.
		out.DeviceLimit = &devices
	}
	for _, group := range groups {
		out.SubscriptionGroupIds = append(out.SubscriptionGroupIds, uint64(group.GroupID))
	}
	return out
}

// SaveOrder is POST /api/v2/user/order/save: the caller orders a plan for a
// period, optionally with a coupon. An order for the plan the caller holds
// is a renewal, for another plan an upgrade, else a new purchase.
func (s *Service) SaveOrder(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		PlanID   uint   `json:"plan_id" binding:"required,gt=0"`
		Period   string `json:"period" binding:"required,min=1"`
		CouponID *uint  `json:"coupon_id"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	order, err := s.createOrder(ctx, request.Principal.ActorID, req.PlanID, req.Period, req.CouponID)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(order)
}

// periodPrice is a plan's price for a period, or the kernel's refusal.
func periodPrice(plan CatalogPlan, period string) (int64, error) {
	prices := map[string]struct {
		price *int64
		name  string
	}{
		"month":      {plan.MonthPrice, "月付"},
		"quarter":    {plan.QuarterPrice, "季付"},
		"half_year":  {plan.HalfYearPrice, "半年付"},
		"year":       {plan.YearPrice, "年付"},
		"two_year":   {plan.TwoYearPrice, "两年付"},
		"three_year": {plan.ThreeYearPrice, "三年付"},
		"onetime":    {plan.OnetimePrice, "一次性付费"},
	}
	entry, ok := prices[period]
	if !ok {
		return 0, errors.New("无效的付费周期")
	}
	if entry.price == nil {
		return 0, errors.New("该套餐不支持" + entry.name)
	}
	return *entry.price, nil
}

// createOrder creates an order as the kernel's OrderService.Create: a
// coupon that is valid now takes its discount off the price (never below
// zero) and counts one use; an invalid one is recorded on the order without
// a discount.
func (s *Service) createOrder(ctx context.Context, userID, planID uint, period string, couponID *uint) (*Order, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	var plan CatalogPlan
	if err := db.First(&plan, planID).Error; err != nil {
		return nil, errors.New("套餐不存在")
	}
	price, err := periodPrice(plan, period)
	if err != nil {
		return nil, err
	}
	var user DirectoryUser
	if err := db.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	orderType := 1 // new
	if user.PlanID != nil && *user.PlanID > 0 {
		if *user.PlanID == planID {
			orderType = 2 // renewal
		} else {
			orderType = 3 // upgrade
		}
	}
	var discountAmount int64
	if couponID != nil && *couponID > 0 {
		var coupon Coupon
		if err := db.First(&coupon, *couponID).Error; err == nil {
			now := s.now().Unix()
			if coupon.StartedAt <= now && coupon.EndedAt >= now &&
				(coupon.LimitUse == nil || *coupon.LimitUse == 0 || coupon.UseCount < *coupon.LimitUse) {
				switch coupon.Type {
				case 1: // percentage
					discountAmount = price * int64(coupon.Value) / 100
				case 2: // fixed amount
					discountAmount = int64(coupon.Value)
				}
				if discountAmount > price {
					discountAmount = price
				}
				price -= discountAmount
				// The kernel ignores a failed count too.
				db.Model(&coupon).UpdateColumn("use_count", gorm.Expr("use_count + 1"))
			}
		}
	}
	tradeNo, err := s.tradeNo()
	if err != nil {
		return nil, err
	}
	order := &Order{
		UserID: userID, PlanID: planID, CouponID: couponID, Type: orderType, Period: period, TradeNo: tradeNo,
		TotalAmount: price, DiscountAmount: &discountAmount, Status: 0,
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// tradeNo is an order's trade number in the kernel's format: the local time
// to the second and 8 characters of A-Z0-9. The kernel draws them from its
// clock; here they come from crypto/rand.
func (s *Service) tradeNo() (string, error) {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range random {
		random[i] = letters[int(random[i])%len(letters)]
	}
	return s.now().Format("20060102150405") + string(random), nil
}
