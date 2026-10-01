package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// PaymentGatewayService 支付网关服务
type PaymentGatewayService struct {
	db *gorm.DB
}

// Errors of CheckOrderPayable, in the messages the payment routes answer.
var (
	ErrPaymentOrderNotFound   = errors.New("订单不存在")
	ErrPaymentOrderNotPending = errors.New("订单已支付或已取消")
	ErrPaymentAmountMismatch  = errors.New("支付金额与订单金额不符")
)

// AmountPaysOrder reports whether amount (yuan) is an order's total (cents)
// to the cent.
func AmountPaysOrder(amount float64, totalCents int64) bool {
	return math.Round(amount*100) == float64(totalCents)
}

// CheckOrderPayable checks that a payment of amount (yuan) that userID
// creates for orderID pays it: the order is the caller's, still pending, and
// the amount is its total. A payment record names the order it pays, and a
// successful callback marks that order paid and activates it, so a record
// may not name another user's order or pay less than its total.
func (s *PaymentGatewayService) CheckOrderPayable(orderID, userID uint, amount float64) error {
	var order model.Order
	if err := s.db.Select("id", "user_id", "status", "total_amount").Where("id = ?", orderID).Take(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPaymentOrderNotFound
		}
		return err
	}
	if order.UserID != userID {
		return ErrPaymentOrderNotFound
	}
	if order.Status != 0 {
		return ErrPaymentOrderNotPending
	}
	if !AmountPaysOrder(amount, order.TotalAmount) {
		return ErrPaymentAmountMismatch
	}
	return nil
}

func paymentGatewayEnabledTypes() []string {
	return []string{
		model.PaymentGatewayEPay,
		model.PaymentGatewayStripe,
		model.PaymentGatewayPayPal,
		model.PaymentGatewayX402,
	}
}

func paymentGatewayTypeCanBeEnabled(gatewayType string) bool {
	for _, enabledType := range paymentGatewayEnabledTypes() {
		if gatewayType == enabledType {
			return true
		}
	}
	return false
}

func validatePaymentGatewayCanBeEnabled(gateway *model.PaymentGateway) error {
	if gateway == nil || !gateway.Enabled {
		return nil
	}
	if paymentGatewayTypeCanBeEnabled(gateway.Type) {
		return nil
	}
	return fmt.Errorf("payment gateway type %q cannot be enabled until live callback implementation and tests are complete", gateway.Type)
}

// NewPaymentGatewayService 创建服务
func NewPaymentGatewayService(db *gorm.DB) *PaymentGatewayService {
	return &PaymentGatewayService{db: db}
}

// Create 创建网关
func (s *PaymentGatewayService) Create(gateway *model.PaymentGateway) error {
	if err := validatePaymentGatewayCanBeEnabled(gateway); err != nil {
		return err
	}
	return s.db.Create(gateway).Error
}

// Update 更新网关
func (s *PaymentGatewayService) Update(gateway *model.PaymentGateway) error {
	if err := validatePaymentGatewayCanBeEnabled(gateway); err != nil {
		return err
	}
	return s.db.Save(gateway).Error
}

// Delete 删除网关
func (s *PaymentGatewayService) Delete(id uint) error {
	return s.db.Delete(&model.PaymentGateway{}, id).Error
}

// GetByID 根据ID获取网关
func (s *PaymentGatewayService) GetByID(id uint) (*model.PaymentGateway, error) {
	var gateway model.PaymentGateway
	err := s.db.First(&gateway, id).Error
	if err != nil {
		return nil, err
	}
	return &gateway, nil
}

// GetByType 根据类型获取网关
func (s *PaymentGatewayService) GetByType(gatewayType string) (*model.PaymentGateway, error) {
	if !paymentGatewayTypeCanBeEnabled(gatewayType) {
		return nil, fmt.Errorf("payment gateway type %q cannot be enabled until live callback implementation and tests are complete", gatewayType)
	}

	var gateway model.PaymentGateway
	err := s.db.Where("type = ? AND enabled = ?", gatewayType, true).First(&gateway).Error
	if err != nil {
		return nil, err
	}
	return &gateway, nil
}

// List 获取网关列表
func (s *PaymentGatewayService) List() ([]*model.PaymentGateway, error) {
	var gateways []*model.PaymentGateway
	err := s.db.Order("sort ASC, id ASC").Find(&gateways).Error
	return gateways, err
}

// GetEnabled 获取启用的网关
func (s *PaymentGatewayService) GetEnabled() ([]*model.PaymentGateway, error) {
	var gateways []*model.PaymentGateway
	err := s.db.Where("enabled = ? AND type IN ?", true, paymentGatewayEnabledTypes()).
		Order("sort ASC, id ASC").Find(&gateways).Error
	return gateways, err
}

// GetChannels 获取支付渠道 (前端显示用)
func (s *PaymentGatewayService) GetChannels() ([]*model.PaymentChannel, error) {
	gateways, err := s.GetEnabled()
	if err != nil {
		return nil, err
	}

	channels := make([]*model.PaymentChannel, len(gateways))
	for i, g := range gateways {
		channels[i] = &model.PaymentChannel{
			ID:          g.ID,
			Name:        g.Name,
			Type:        g.Type,
			Icon:        g.Icon,
			MinAmount:   g.MinAmount,
			MaxAmount:   g.MaxAmount,
			FeeRate:     g.FeeRate,
			FeeFixed:    g.FeeFixed,
			Description: g.Description,
		}
	}
	return channels, nil
}

// Toggle 切换网关状态
func (s *PaymentGatewayService) Toggle(id uint, enabled bool) error {
	if enabled {
		gateway, err := s.GetByID(id)
		if err != nil {
			return err
		}
		gateway.Enabled = true
		if err := validatePaymentGatewayCanBeEnabled(gateway); err != nil {
			return err
		}
	}
	return s.db.Model(&model.PaymentGateway{}).Where("id = ?", id).
		Update("enabled", enabled).Error
}

// ValidateGatewayUsable verifies that a gateway can be shown to users and used
// to create new payment records.
func (s *PaymentGatewayService) ValidateGatewayUsable(gateway *model.PaymentGateway) error {
	if gateway == nil {
		return fmt.Errorf("invalid gateway")
	}
	if !gateway.Enabled {
		return fmt.Errorf("gateway is disabled")
	}
	return validatePaymentGatewayCanBeEnabled(gateway)
}

// ParseConfig 解析网关配置
func (s *PaymentGatewayService) ParseConfig(gateway *model.PaymentGateway) (any, error) {
	switch gateway.Type {
	case model.PaymentGatewayAlipay:
		var config model.AlipayConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentGatewayWechat:
		var config model.WechatPayConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentProviderStripe:
		var config model.StripeConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentGatewayPayPal:
		var config model.PayPalConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentGatewayX402:
		var config model.X402Config
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentGatewayUSDT:
		var config model.USDTConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	case model.PaymentGatewayEPay:
		var config model.EPayConfig
		if err := json.Unmarshal([]byte(gateway.Config), &config); err != nil {
			return nil, err
		}
		return &config, nil
	}
	return nil, fmt.Errorf("unknown gateway type: %s", gateway.Type)
}

// CalculateFee 计算手续费
func (s *PaymentGatewayService) CalculateFee(gateway *model.PaymentGateway, amount float64) float64 {
	return amount*gateway.FeeRate + gateway.FeeFixed
}

// ValidateAmount 验证金额范围
func (s *PaymentGatewayService) ValidateAmount(gateway *model.PaymentGateway, amount float64) error {
	if amount < gateway.MinAmount {
		return fmt.Errorf("金额不能小于 %.2f", gateway.MinAmount)
	}
	if amount > gateway.MaxAmount {
		return fmt.Errorf("金额不能大于 %.2f", gateway.MaxAmount)
	}
	return nil
}

// ========== 支付记录管理 ==========

// CreateRecord 创建支付记录
func (s *PaymentGatewayService) CreateRecord(record *model.PaymentRecord) error {
	if record.TradeNo == "" {
		record.TradeNo = s.GenerateTradeNo()
	}
	return s.db.Create(record).Error
}

// GetRecordByTradeNo 根据订单号获取记录
func (s *PaymentGatewayService) GetRecordByTradeNo(tradeNo string) (*model.PaymentRecord, error) {
	var record model.PaymentRecord
	err := s.db.Where("trade_no = ?", tradeNo).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// MarkOrderPaid 标记支付并更新订单状态 (事务)
func (s *PaymentGatewayService) MarkOrderPaid(tradeNo string, gatewayTradeNo string, notifyData string) error {
	return s.markOrderPaid(tradeNo, gatewayTradeNo, notifyData, nil)
}

// MarkOrderPaidWithAmount 标记支付并核对回调金额。
func (s *PaymentGatewayService) MarkOrderPaidWithAmount(tradeNo string, gatewayTradeNo string, notifyData string, paidAmount *float64) error {
	var check func(model.PaymentRecord) error
	if paidAmount != nil {
		check = func(record model.PaymentRecord) error {
			if math.Abs(record.ActualAmount-*paidAmount) > 0.01 {
				return fmt.Errorf("payment amount mismatch")
			}
			return nil
		}
	}
	return s.markOrderPaid(tradeNo, gatewayTradeNo, notifyData, check)
}

// ErrPaymentNotCovered is returned, wrapped with the reason, when a paid
// callback does not pay what its payment record asks for. Nothing is
// written: the record stays pending and its order unpaid.
var ErrPaymentNotCovered = errors.New("payment not applied")

// MarkOrderPaidIfCovered marks the payment paid, as MarkOrderPaid does,
// only if covers accepts its pending record, read inside the transaction.
// covers returns an error wrapping ErrPaymentNotCovered to refuse it.
func (s *PaymentGatewayService) MarkOrderPaidIfCovered(tradeNo string, gatewayTradeNo string, notifyData string, covers func(model.PaymentRecord) error) error {
	if covers == nil {
		return fmt.Errorf("%w: no payment check", ErrPaymentNotCovered)
	}
	return s.markOrderPaid(tradeNo, gatewayTradeNo, notifyData, covers)
}

// errPaymentProcessed answers a callback for a payment that is no longer
// pending.
var errPaymentProcessed = errors.New("payment already processed")

func (s *PaymentGatewayService) markOrderPaid(tradeNo string, gatewayTradeNo string, notifyData string, check func(model.PaymentRecord) error) error {
	var processed *model.PaymentRecord
	err := WithRetryableTransaction(s.db, func(tx *gorm.DB) error {
		processed = nil
		// 查询支付记录
		var record model.PaymentRecord
		if err := tx.Where("trade_no = ?", tradeNo).First(&record).Error; err != nil {
			return err
		}

		if record.Status != model.PaymentStatusPending {
			processed = &record
			return errPaymentProcessed
		}

		if check != nil {
			if err := check(record); err != nil {
				return err
			}
		}

		now := time.Now()
		// 更新支付记录
		if err := tx.Model(&record).Updates(map[string]any{
			"status":           model.PaymentStatusPaid,
			"gateway_trade_no": gatewayTradeNo,
			"notify_data":      notifyData,
			"paid_at":          now,
		}).Error; err != nil {
			return err
		}

		// 更新网关统计
		if record.GatewayID != 0 {
			if err := tx.Model(&model.PaymentGateway{}).Where("id = ?", record.GatewayID).Updates(map[string]any{
				"total_orders": gorm.Expr("total_orders + 1"),
				"total_amount": gorm.Expr("total_amount + ?", record.ActualAmount),
			}).Error; err != nil {
				return err
			}
		}

		// 更新关联订单状态: the order it pays is marked paid and completed,
		// as KernelOrder.CompleteOrderPayment does for the payment module.
		if record.OrderID != nil {
			if _, err := CompleteOrderPaymentTx(tx, record.TradeNo, uint64(*record.OrderID), now); err != nil {
				return err
			}
		}

		return nil
	})
	if processed != nil {
		s.FinishPaidOrder(*processed)
	}
	return err
}

// GetUserRecords 获取用户支付记录
func (s *PaymentGatewayService) GetUserRecords(userID uint, page, pageSize int) ([]*model.PaymentRecord, int64, error) {
	var records []*model.PaymentRecord
	var total int64

	query := s.db.Model(&model.PaymentRecord{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// ListRecords 获取支付记录列表
func (s *PaymentGatewayService) ListRecords(page, pageSize int, status *int, gatewayType string) ([]*model.PaymentRecord, int64, error) {
	var records []*model.PaymentRecord
	var total int64

	query := s.db.Model(&model.PaymentRecord{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if gatewayType != "" {
		query = query.Where("gateway_type = ?", gatewayType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&records).Error
	return records, total, err
}

// GenerateTradeNo 生成商户订单号
func (s *PaymentGatewayService) GenerateTradeNo() string {
	timestamp := time.Now().Format("20060102150405")
	b := make([]byte, 4)
	rand.Read(b)
	random := hex.EncodeToString(b)
	return fmt.Sprintf("PAY%s%s", timestamp, random)
}

// GetStats 获取支付统计
func (s *PaymentGatewayService) GetStats(start, end time.Time) (*PaymentStats, error) {
	stats := &PaymentStats{}

	// 总支付金额
	var totalAmount float64
	s.db.Model(&model.PaymentRecord{}).
		Where("status = ? AND paid_at BETWEEN ? AND ?", model.PaymentStatusPaid, start, end).
		Select("COALESCE(SUM(actual_amount), 0)").
		Scan(&totalAmount)
	stats.TotalAmount = totalAmount

	// 支付笔数
	var totalCount int64
	s.db.Model(&model.PaymentRecord{}).
		Where("status = ? AND paid_at BETWEEN ? AND ?", model.PaymentStatusPaid, start, end).
		Count(&totalCount)
	stats.TotalCount = totalCount

	// 待支付金额
	var pendingAmount float64
	s.db.Model(&model.PaymentRecord{}).
		Where("status = ? AND created_at BETWEEN ? AND ?", model.PaymentStatusPending, start, end).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&pendingAmount)
	stats.PendingAmount = pendingAmount

	// 待支付笔数
	var pendingCount int64
	s.db.Model(&model.PaymentRecord{}).
		Where("status = ? AND created_at BETWEEN ? AND ?", model.PaymentStatusPending, start, end).
		Count(&pendingCount)
	stats.PendingCount = pendingCount

	return stats, nil
}

// PaymentStats 支付统计
type PaymentStats struct {
	TotalAmount   float64 `json:"total_amount"`
	TotalCount    int64   `json:"total_count"`
	PendingAmount float64 `json:"pending_amount"`
	PendingCount  int64   `json:"pending_count"`
}
