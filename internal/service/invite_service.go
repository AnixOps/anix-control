package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
)

// Withdrawal errors. Their messages are the v2 answers.
var (
	// ErrWithdrawAmountNotWhole: the commission balance is a whole number
	// of cents, so a withdrawal is too.
	ErrWithdrawAmountNotWhole = errors.New("amount must be a whole number")
	// ErrWithdrawalProcessed: the withdrawal is no longer pending, because
	// another decision processed it first.
	ErrWithdrawalProcessed = errors.New("withdrawal already processed")
)

// maxWithdrawCents bounds a withdrawal or refund amount to integers a
// float64 holds exactly.
const maxWithdrawCents = 1 << 53

// WithdrawDebitRequestID names a withdrawal's debit of the commission
// balance in the subscriber request ledger (v4_kernel_subscriber_request).
// The affiliate package's native route uses the same id.
func WithdrawDebitRequestID(withdrawID uint) string {
	return fmt.Sprintf("affiliate.withdraw:%d", withdrawID)
}

// WithdrawRefundRequestID names the refund of a rejected withdrawal, so a
// withdrawal is refunded once.
func WithdrawRefundRequestID(withdrawID uint) string {
	return fmt.Sprintf("affiliate.withdraw.refund:%d", withdrawID)
}

// WithdrawRefundCents is what rejecting a withdrawal returns to the
// commission balance. Withdrawals are whole amounts; one stored with a
// fraction, possible before they had to be, is refunded its whole part,
// which is what its debit took on PostgreSQL: the driver truncated the
// amount bound to "commission_balance - ?".
func WithdrawRefundCents(amount float64) (int64, bool) {
	whole := math.Trunc(amount)
	if math.IsNaN(whole) || whole < 0 || whole > maxWithdrawCents {
		return 0, false
	}
	return int64(whole), true
}

// InviteService 邀请服务
type InviteService struct {
	db     *gorm.DB
	config *model.InviteConfig
}

// NewInviteService 创建服务
func NewInviteService(db *gorm.DB) *InviteService {
	return &InviteService{db: db}
}

// SetConfig 设置配置
func (s *InviteService) SetConfig(cfg *model.InviteConfig) {
	s.config = cfg
}

// GetConfig 获取配置
func (s *InviteService) GetConfig() (*model.InviteConfig, error) {
	if s.config != nil {
		return s.config, nil
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	s.config = cfg
	return cfg, nil
}

// loadConfig reads the configuration from the database, creating the
// default one when there is none.
func (s *InviteService) loadConfig() (*model.InviteConfig, error) {
	var cfg model.InviteConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// 创建默认配置
		cfg = model.InviteConfig{
			Enabled:             true,
			AutoGenerate:        true,
			CodeCount:           5,
			CodeExpireDays:      0,
			CommissionEnabled:   true,
			CommissionType:      1,
			CommissionRate:      0.1,
			CommissionFixed:     0,
			CommissionMinAmount: 10,
		}
		if createErr := s.db.Create(&cfg).Error; createErr != nil {
			return nil, createErr
		}
	} else if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GenerateInviteCode 生成邀请码
func (s *InviteService) GenerateInviteCode(userID *uint) (*model.InviteCode, error) {
	code, err := generateInviteCodeStr(8)
	if err != nil {
		return nil, err
	}

	inviteCode := &model.InviteCode{
		Code:   code,
		UserID: userID,
		Status: 0,
	}

	// 设置过期时间
	cfg, _ := s.GetConfig()
	if cfg != nil && cfg.CodeExpireDays > 0 {
		expiredAt := time.Now().AddDate(0, 0, cfg.CodeExpireDays)
		inviteCode.ExpiredAt = &expiredAt
	}

	if err := s.db.Create(inviteCode).Error; err != nil {
		return nil, err
	}

	return inviteCode, nil
}

// GetUserInviteCodes 获取用户邀请码
func (s *InviteService) GetUserInviteCodes(userID uint) ([]model.InviteCode, error) {
	var codes []model.InviteCode
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&codes).Error
	return codes, err
}

// GetInviteStats 获取邀请统计
func (s *InviteService) GetInviteStats(userID uint) (map[string]any, error) {
	var inviteCount int64
	var paidCount int64
	var totalCommission float64

	// 邀请人数
	s.db.Model(&model.User{}).Where("invite_user_id = ?", userID).Count(&inviteCount)

	// 付费人数
	s.db.Table("v2_user u").
		Joins("JOIN v2_order o ON o.user_id = u.id").
		Where("u.invite_user_id = ? AND o.status = ?", userID, 1).
		Distinct("u.id").
		Count(&paidCount)

	// 总佣金
	s.db.Model(&model.CommissionRecord{}).
		Where("user_id = ? AND status IN (1, 2)", userID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalCommission)

	return map[string]any{
		"invite_count":     inviteCount,
		"paid_count":       paidCount,
		"total_commission": totalCommission,
	}, nil
}

// CalculateCommission 计算佣金
func (s *InviteService) CalculateCommission(orderAmount float64) float64 {
	cfg, _ := s.GetConfig()
	if cfg == nil || !cfg.CommissionEnabled {
		return 0
	}

	switch cfg.CommissionType {
	case 1: // 按比例
		return orderAmount * cfg.CommissionRate
	case 2: // 固定金额
		return cfg.CommissionFixed
	default:
		return 0
	}
}

// AddCommission 添加佣金
func (s *InviteService) AddCommission(userID, orderID, fromUserID uint, amount float64, recordType int, remark string) error {
	record := &model.CommissionRecord{
		UserID:     userID,
		OrderID:    orderID,
		FromUserID: fromUserID,
		Amount:     amount,
		Type:       recordType,
		Status:     1, // 直接到账
		Remark:     remark,
	}

	return s.db.Create(record).Error
}

// GetCommissionRecords 获取佣金记录
func (s *InviteService) GetCommissionRecords(userID uint, page, pageSize int) ([]model.CommissionRecord, int64, error) {
	var records []model.CommissionRecord
	var total int64

	db := s.db.Model(&model.CommissionRecord{}).Where("user_id = ?", userID)
	db.Count(&total)

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error

	return records, total, err
}

// RequestWithdraw 申请提现. The amount is a whole number in the commission
// balance's unit (cents) and at least the configured minimum, read for each
// request so an administrator's change applies at once. The withdrawal and
// its debit (WithdrawDebitRequestID) are one transaction, and the debit is
// taken under the subscriber's row lock and refused below zero
// (subscriber.AdjustBalanceTx), so concurrent requests cannot overdraw.
func (s *InviteService) RequestWithdraw(userID uint, amount float64, method, account, name string) (*model.CommissionWithdraw, error) {
	if amount != math.Trunc(amount) {
		return nil, ErrWithdrawAmountNotWhole
	}
	cfg, _ := s.loadConfig()
	if cfg != nil && amount < cfg.CommissionMinAmount {
		return nil, errors.New("amount below minimum")
	}

	// 检查余额
	var balance float64
	s.db.Model(&model.User{}).Where("id = ?", userID).
		Select("commission_balance").Scan(&balance)

	if amount > balance || amount > maxWithdrawCents {
		return nil, subscriber.ErrInsufficientBalance
	}

	withdraw := &model.CommissionWithdraw{
		UserID:  userID,
		Amount:  amount,
		Method:  method,
		Account: account,
		Name:    name,
		Status:  0,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(withdraw).Error; err != nil {
			return err
		}
		// 扣除余额
		debit, err := subscriber.AdjustBalanceTx(tx, WithdrawDebitRequestID(withdraw.ID), userID,
			subscriber.BalanceCommission, -int64(amount), time.Now())
		if err != nil {
			return err
		}
		if !debit.Applied {
			return fmt.Errorf("withdrawal %d was debited before", withdraw.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return withdraw, nil
}

// ProcessWithdraw approves (status 1) or rejects (status 2) a withdrawal
// that is still pending, as of now. The decision applies only while the
// withdrawal is pending, so of two concurrent decisions one fails with
// ErrWithdrawalProcessed. A rejection returns the amount to the commission
// balance in the same transaction, once (WithdrawRefundRequestID); if that
// fails, nothing changes. A withdrawal whose user no longer exists has no
// one to refund and is rejected all the same. withdraw is updated on
// success.
func (s *InviteService) ProcessWithdraw(withdraw *model.CommissionWithdraw, status int, remark string, now time.Time) error {
	var refund int64
	if status == 2 {
		cents, ok := WithdrawRefundCents(withdraw.Amount)
		if !ok {
			return fmt.Errorf("withdrawal %d amount %v cannot be refunded", withdraw.ID, withdraw.Amount)
		}
		refund = cents
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		claimed := tx.Model(&model.CommissionWithdraw{}).Where("id = ? AND status = ?", withdraw.ID, 0).
			Updates(map[string]any{"status": status, "remark": remark, "processed_at": now, "updated_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected == 0 {
			return ErrWithdrawalProcessed
		}
		if status != 2 {
			return nil
		}
		_, err := subscriber.AdjustBalanceTx(tx, WithdrawRefundRequestID(withdraw.ID), withdraw.UserID,
			subscriber.BalanceCommission, refund, now)
		if errors.Is(err, subscriber.ErrSubscriberNotFound) {
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	withdraw.Status = status
	withdraw.Remark = remark
	withdraw.ProcessedAt = &now
	withdraw.UpdatedAt = now
	return nil
}

// GetWithdrawRecords 获取提现记录
func (s *InviteService) GetWithdrawRecords(userID uint, page, pageSize int) ([]model.CommissionWithdraw, int64, error) {
	var records []model.CommissionWithdraw
	var total int64

	db := s.db.Model(&model.CommissionWithdraw{}).Where("user_id = ?", userID)
	db.Count(&total)

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error

	return records, total, err
}

// generateInviteCodeStr 生成随机码
func generateInviteCodeStr(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:length], nil
}
