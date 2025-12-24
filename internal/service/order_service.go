package service

import (
	"errors"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// OrderService 订单服务
type OrderService struct {
	db *gorm.DB
}

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

// OrderListResult 订单列表结果
type OrderListResult struct {
	Total int64         `json:"total"`
	List  []model.Order `json:"list"`
}

// GetList 获取订单列表
func (s *OrderService) GetList(params OrderListParams) (*OrderListResult, error) {
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
		query = query.Joins("JOIN v2_user ON v2_user.id = v2_order.user_id").
			Where("v2_user.email LIKE ?", "%"+params.Email+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序
	orderBy := "created_at DESC"
	if params.OrderBy != "" {
		orderBy = params.OrderBy
	}

	// 分页
	offset := (params.Page - 1) * params.PageSize
	if err := query.Preload("User").Preload("Plan").
		Order(orderBy).
		Offset(offset).
		Limit(params.PageSize).
		Find(&orders).Error; err != nil {
		return nil, err
	}

	return &OrderListResult{
		Total: total,
		List:  orders,
	}, nil
}

// GetByID 根据ID获取订单
func (s *OrderService) GetByID(id uint) (*model.Order, error) {
	var order model.Order
	if err := s.db.Preload("User").Preload("Plan").First(&order, id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

// GetByTradeNo 根据交易号获取订单
func (s *OrderService) GetByTradeNo(tradeNo string) (*model.Order, error) {
	var order model.Order
	if err := s.db.Preload("User").Preload("Plan").
		Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
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

	// 生成交易号
	tradeNo := generateTradeNo()

	order := &model.Order{
		UserID:      params.UserID,
		PlanID:      params.PlanID,
		CouponID:    params.CouponID,
		Type:        orderType,
		Period:      params.Period,
		TradeNo:     tradeNo,
		TotalAmount: price,
		Status:      0, // 待支付
	}

	if err := s.db.Create(order).Error; err != nil {
		return nil, err
	}

	return order, nil
}

// UpdateStatus 更新订单状态
func (s *OrderService) UpdateStatus(orderID uint, status int) error {
	updates := map[string]interface{}{
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
	order, err := s.GetByID(orderID)
	if err != nil {
		return err
	}

	if order.Status != 1 {
		return errors.New("订单状态错误")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// 获取套餐
		var plan model.Plan
		if err := tx.First(&plan, order.PlanID).Error; err != nil {
			return err
		}

		// 计算到期时间
		var expiredAt int64
		now := time.Now()
		switch order.Period {
		case "month":
			expiredAt = now.AddDate(0, 1, 0).Unix()
		case "quarter":
			expiredAt = now.AddDate(0, 3, 0).Unix()
		case "half_year":
			expiredAt = now.AddDate(0, 6, 0).Unix()
		case "year":
			expiredAt = now.AddDate(1, 0, 0).Unix()
		case "two_year":
			expiredAt = now.AddDate(2, 0, 0).Unix()
		case "three_year":
			expiredAt = now.AddDate(3, 0, 0).Unix()
		case "onetime":
			expiredAt = now.AddDate(100, 0, 0).Unix() // 100年
		}

		// 更新用户
		userUpdates := map[string]interface{}{
			"plan_id":         order.PlanID,
			"group_id":        plan.GroupID,
			"transfer_enable": plan.TransferEnable * 1024 * 1024 * 1024, // GB to bytes
			"expired_at":      expiredAt,
			"u":               0, // 重置上传流量
			"d":               0, // 重置下载流量
		}
		if plan.SpeedLimit != nil {
			userUpdates["speed_limit"] = *plan.SpeedLimit
		}
		if plan.DeviceLimit != nil {
			userUpdates["device_limit"] = *plan.DeviceLimit
		}

		if err := tx.Model(&model.User{}).Where("id = ?", order.UserID).Updates(userUpdates).Error; err != nil {
			return err
		}

		// 更新订单状态为已完成
		if err := tx.Model(&model.Order{}).Where("id = ?", orderID).Update("status", 3).Error; err != nil {
			return err
		}

		return nil
	})
}

// GetUserOrders 获取用户订单
func (s *OrderService) GetUserOrders(userID uint, page, pageSize int) (*OrderListResult, error) {
	return s.GetList(OrderListParams{
		Page:     page,
		PageSize: pageSize,
		UserID:   &userID,
	})
}

// GetStats 获取订单统计
func (s *OrderService) GetStats() (map[string]interface{}, error) {
	var totalOrders int64
	var pendingOrders int64
	var paidOrders int64
	var totalRevenue int64

	s.db.Model(&model.Order{}).Count(&totalOrders)
	s.db.Model(&model.Order{}).Where("status = 0").Count(&pendingOrders)
	s.db.Model(&model.Order{}).Where("status IN (1, 3)").Count(&paidOrders)
	s.db.Model(&model.Order{}).Where("status IN (1, 3)").Select("COALESCE(SUM(total_amount), 0)").Scan(&totalRevenue)

	// 今日收入
	today := time.Now().Truncate(24 * time.Hour).Unix()
	var todayRevenue int64
	s.db.Model(&model.Order{}).
		Where("status IN (1, 3)").
		Where("paid_at >= ?", today).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&todayRevenue)

	return map[string]interface{}{
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
