package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrderService 订单服务
type OrderService struct {
	db *gorm.DB
}

var (
	ErrOrderNotFound     = errors.New("订单不存在")
	ErrOrderPlanNotFound = errors.New("关联套餐不存在")
)

// NewOrderService 创建订单服务
func NewOrderService() *OrderService {
	return &OrderService{
		db: database.Get(),
	}
}

// OrderListParams 订单列表查询参数
type OrderListParams struct {
	Page     int
	PageSize int
	UserID   *uint
	Status   *int
	Type     *int
	TradeNo  string
	Email    string
	OrderBy  string
}

// OrderPlanRef names an order's plan in the order list and detail answers.
type OrderPlanRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// OrderBuyerRef names an order's buyer in an administrator's order list and
// detail answers.
type OrderBuyerRef struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
}

// OrderView is an order as the order list and detail routes answer it: the
// order's own columns, its plan's id and name, and, for an administrator,
// its buyer's id and e-mail. A plan or buyer that no longer exists is left
// out. The answers used to embed the buyer's whole v2_user row
// (subscription token and UUID included) and the whole v2_plan row.
type OrderView struct {
	model.Order
	// User and Plan shadow the relations of model.Order, which are never
	// loaded here.
	User *OrderBuyerRef `json:"user,omitempty"`
	Plan *OrderPlanRef  `json:"plan,omitempty"`
}

// OrderListResult 订单列表结果
type OrderListResult struct {
	Total int64       `json:"total"`
	List  []OrderView `json:"list"`
}

// GetList is an administrator's order list, newest first, each order with
// its buyer.
func (s *OrderService) GetList(params OrderListParams) (*OrderListResult, error) {
	return s.list(params, true)
}

func (s *OrderService) list(params OrderListParams, withBuyer bool) (*OrderListResult, error) {
	var orders []model.Order
	var total int64

	query := s.db.Model(&model.Order{})

	// 筛选条件
	if params.UserID != nil {
		query = query.Where("user_id = ?", *params.UserID)
	}
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	if params.Type != nil {
		query = query.Where("type = ?", *params.Type)
	}
	if params.TradeNo != "" {
		query = query.Where("trade_no = ?", params.TradeNo)
	}
	if params.Email != "" {
		// A subquery, not a join: the joined v2_user made created_at
		// ambiguous in the ordering, and the filter always failed.
		query = query.Where("user_id IN (?)",
			s.db.Model(&model.User{}).Select("id").Where("email LIKE ?", "%"+params.Email+"%"))
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序: orders created in the same second keep a stable order.
	orderBy := "created_at DESC, id DESC"
	if params.OrderBy != "" {
		orderBy = params.OrderBy
	}

	// 分页
	offset := (params.Page - 1) * params.PageSize
	if err := query.
		Order(orderBy).
		Offset(offset).
		Limit(params.PageSize).
		Find(&orders).Error; err != nil {
		return nil, err
	}
	views, err := s.views(orders, withBuyer)
	if err != nil {
		return nil, err
	}
	return &OrderListResult{
		Total: total,
		List:  views,
	}, nil
}

// views names each order's plan and, withBuyer, its buyer: their id and
// name or e-mail, never the rest of their rows.
func (s *OrderService) views(orders []model.Order, withBuyer bool) ([]OrderView, error) {
	planIDs := make([]uint, 0, len(orders))
	userIDs := make([]uint, 0, len(orders))
	for _, order := range orders {
		planIDs = append(planIDs, order.PlanID)
		userIDs = append(userIDs, order.UserID)
	}
	plans := map[uint]*OrderPlanRef{}
	if len(planIDs) > 0 {
		var rows []OrderPlanRef
		if err := s.db.Model(&model.Plan{}).Select("id", "name").Where("id IN ?", planIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			plans[rows[i].ID] = &rows[i]
		}
	}
	buyers := map[uint]*OrderBuyerRef{}
	if withBuyer && len(userIDs) > 0 {
		var rows []OrderBuyerRef
		if err := s.db.Model(&model.User{}).Select("id", "email").Where("id IN ?", userIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			buyers[rows[i].ID] = &rows[i]
		}
	}
	views := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		order.User, order.Plan = nil, nil
		views = append(views, OrderView{Order: order, User: buyers[order.UserID], Plan: plans[order.PlanID]})
	}
	return views, nil
}

// GetByID 根据ID获取订单, without its plan or buyer.
func (s *OrderService) GetByID(id uint) (*model.Order, error) {
	if id == 0 {
		return nil, ErrOrderNotFound
	}

	var order model.Order
	if err := s.db.First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// GetAdminView is an administrator's order detail: the order with its plan
// and buyer.
func (s *OrderService) GetAdminView(id uint) (*OrderView, error) {
	order, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	views, err := s.views([]model.Order{*order}, true)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// GetUserView is a user's order detail: the order, if it is the user's,
// with its plan. Another user's order is ErrOrderNotFound, as an unknown one.
func (s *OrderService) GetUserView(userID, id uint) (*OrderView, error) {
	if userID == 0 || id == 0 {
		return nil, ErrOrderNotFound
	}
	var order model.Order
	if err := s.db.Where("user_id = ?", userID).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	views, err := s.views([]model.Order{order}, false)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// CreateOrderParams 创建订单参数
type CreateOrderParams struct {
	UserID   uint
	PlanID   uint
	Period   string // month, quarter, half_year, year, two_year, three_year, onetime
	CouponID *uint
}

// Create 创建订单
func (s *OrderService) Create(params CreateOrderParams) (*model.Order, error) {
	// 获取套餐
	var plan model.Plan
	if err := s.db.First(&plan, params.PlanID).Error; err != nil {
		return nil, errors.New("套餐不存在")
	}

	// 计算价格
	var price int64
	switch params.Period {
	case "month":
		if plan.MonthPrice == nil {
			return nil, errors.New("该套餐不支持月付")
		}
		price = *plan.MonthPrice
	case "quarter":
		if plan.QuarterPrice == nil {
			return nil, errors.New("该套餐不支持季付")
		}
		price = *plan.QuarterPrice
	case "half_year":
		if plan.HalfYearPrice == nil {
			return nil, errors.New("该套餐不支持半年付")
		}
		price = *plan.HalfYearPrice
	case "year":
		if plan.YearPrice == nil {
			return nil, errors.New("该套餐不支持年付")
		}
		price = *plan.YearPrice
	case "two_year":
		if plan.TwoYearPrice == nil {
			return nil, errors.New("该套餐不支持两年付")
		}
		price = *plan.TwoYearPrice
	case "three_year":
		if plan.ThreeYearPrice == nil {
			return nil, errors.New("该套餐不支持三年付")
		}
		price = *plan.ThreeYearPrice
	case "onetime":
		if plan.OnetimePrice == nil {
			return nil, errors.New("该套餐不支持一次性付费")
		}
		price = *plan.OnetimePrice
	default:
		return nil, errors.New("无效的付费周期")
	}

	// 检查用户是否有正在处理的订单
	var user model.User
	if err := s.db.First(&user, params.UserID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	// 确定订单类型
	orderType := 1 // 新购
	if user.PlanID != nil && *user.PlanID > 0 {
		if *user.PlanID == params.PlanID {
			orderType = 2 // 续费
		} else {
			orderType = 3 // 升级
		}
	}

	// 应用优惠券
	var discountAmount int64
	if params.CouponID != nil && *params.CouponID > 0 {
		var coupon model.Coupon
		if err := s.db.First(&coupon, *params.CouponID).Error; err == nil {
			now := time.Now().Unix()
			if coupon.StartedAt <= now && coupon.EndedAt >= now {
				if coupon.LimitUse == nil || *coupon.LimitUse == 0 || coupon.UseCount < *coupon.LimitUse {
					switch coupon.Type {
					case 1: // 百分比
						discountAmount = price * int64(coupon.Value) / 100
					case 2: // 固定金额
						discountAmount = int64(coupon.Value)
					}

					// 确保不会减成负数
					if discountAmount > price {
						discountAmount = price
					}
					price -= discountAmount

					// 增加优惠券使用计数
					s.db.Model(&coupon).UpdateColumn("use_count", gorm.Expr("use_count + 1"))
				}
			}
		}
	}

	// 生成交易号
	tradeNo := generateTradeNo()

	order := &model.Order{
		UserID:         params.UserID,
		PlanID:         params.PlanID,
		CouponID:       params.CouponID,
		Type:           orderType,
		Period:         params.Period,
		TradeNo:        tradeNo,
		TotalAmount:    price,
		DiscountAmount: &discountAmount,
		Status:         0, // 待支付
	}

	if err := s.db.Create(order).Error; err != nil {
		return nil, err
	}

	return order, nil
}

// UpdateStatus 更新订单状态
func (s *OrderService) UpdateStatus(orderID uint, status int) error {
	if orderID == 0 {
		return ErrOrderNotFound
	}

	var order model.Order
	if err := s.db.Select("id").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}

	updates := map[string]any{
		"status": status,
	}
	if status == 1 { // 已支付
		now := time.Now().Unix()
		updates["paid_at"] = now
	}
	return s.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// Cancel 取消订单
func (s *OrderService) Cancel(orderID uint) error {
	return s.UpdateStatus(orderID, 2)
}

// Complete 完成订单 (支付成功后处理)
func (s *OrderService) Complete(orderID uint) error {
	if orderID == 0 {
		return ErrOrderNotFound
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return completeOrderTx(tx, orderID, time.Now())
	})
}

// completeOrderTx grants a paid order's plan through the subscriber engine
// (once per order) and marks the order completed. A purchase resets
// traffic and renews on top of an unexpired identical plan.
func completeOrderTx(tx *gorm.DB, orderID uint, now time.Time) error {
	var order model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}
	if order.Status != 1 {
		return errors.New("订单状态不处于已支付，无法完成")
	}
	var plan model.Plan
	if err := tx.First(&plan, order.PlanID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderPlanNotFound
		}
		return err
	}
	var planGroups []model.PlanSubscriptionGroup
	if err := tx.Where("plan_id = ?", plan.ID).Find(&planGroups).Error; err != nil {
		return err
	}
	if _, err := subscriber.ApplyEntitlementTx(tx, subscriber.Entitlement{
		RequestID: fmt.Sprintf("order:%d", order.ID), UserID: order.UserID,
		Plan:   subscriber.PlanSnapshotFromPlan(plan, planGroups),
		Period: &subscriber.Period{Months: subscriber.PeriodMonths[order.Period]}, RenewSamePlan: true, ResetTraffic: true,
	}, now); err != nil {
		return err
	}
	return tx.Model(&order).Update("status", 3).Error
}

// GetUserOrders is a user's own orders, newest first, each with its plan
// and without the buyer. A user id of zero names no one and has no orders.
func (s *OrderService) GetUserOrders(userID uint, page, pageSize int) (*OrderListResult, error) {
	if userID == 0 {
		return &OrderListResult{List: []OrderView{}}, nil
	}
	return s.list(OrderListParams{
		Page:     page,
		PageSize: pageSize,
		UserID:   &userID,
	}, false)
}

// GetStats 获取订单统计
func (s *OrderService) GetStats() (map[string]any, error) {
	var totalOrders int64
	var pendingOrders int64
	var paidOrders int64
	var totalRevenue int64

	if err := s.db.Model(&model.Order{}).Count(&totalOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Order{}).Where("status = 0").Count(&pendingOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Order{}).Where("status IN (1, 3)").Count(&paidOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Order{}).Where("status IN (1, 3)").Select("COALESCE(SUM(total_amount), 0)").Scan(&totalRevenue).Error; err != nil {
		return nil, err
	}

	// 今日收入
	today := time.Now().Truncate(24 * time.Hour).Unix()
	var todayRevenue int64
	if err := s.db.Model(&model.Order{}).
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

// generateTradeNo 生成交易号
func generateTradeNo() string {
	return time.Now().Format("20060102150405") + randomString(8)
}

// randomString 生成随机字符串
func randomString(n int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
