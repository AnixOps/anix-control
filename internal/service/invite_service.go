package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

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
		if createErr := s.db.Create(&cfg); createErr != nil {
			return nil, createErr.Error
		}
	} else if err != nil {
		return nil, err
	}
	s.config = &cfg
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

// GenerateCodesForUser 为用户生成邀请码
func (s *InviteService) GenerateCodesForUser(userID uint, count int) error {
	for i := 0; i < count; i++ {
		_, err := s.GenerateInviteCode(&userID)
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateInviteCode 验证邀请码
func (s *InviteService) ValidateInviteCode(code string) (*model.InviteCode, error) {
	var inviteCode model.InviteCode
	err := s.db.Where("code = ? AND status = 0", code).First(&inviteCode).Error
	if err == gorm.ErrRecordNotFound {
		return nil, errors.New("invalid or used invite code")
	}
	if err != nil {
		return nil, err
	}

	// 检查是否过期
	if inviteCode.ExpiredAt != nil && inviteCode.ExpiredAt.Before(time.Now()) {
		return nil, errors.New("invite code expired")
	}

	return &inviteCode, nil
}

// UseInviteCode 使用邀请码
func (s *InviteService) UseInviteCode(code string, userID uint) error {
	inviteCode, err := s.ValidateInviteCode(code)
	if err != nil {
		return err
	}

	now := time.Now()
	inviteCode.Status = 1
	inviteCode.UsedBy = &userID
	inviteCode.UsedAt = &now

	return s.db.Save(inviteCode).Error
}

// GetUserInviteCodes 获取用户邀请码
func (s *InviteService) GetUserInviteCodes(userID uint) ([]model.InviteCode, error) {
	var codes []model.InviteCode
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&codes).Error
	return codes, err
}

// GetInviteStats 获取邀请统计
func (s *InviteService) GetInviteStats(userID uint) (map[string]interface{}, error) {
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

	return map[string]interface{}{
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

// RequestWithdraw 申请提现
func (s *InviteService) RequestWithdraw(userID uint, amount float64, method, account, name string) (*model.CommissionWithdraw, error) {
	cfg, _ := s.GetConfig()
	if cfg != nil && amount < cfg.CommissionMinAmount {
		return nil, errors.New("amount below minimum")
	}

	// 检查余额
	var balance float64
	s.db.Model(&model.User{}).Where("id = ?", userID).
		Select("commission_balance").Scan(&balance)

	if amount > balance {
		return nil, errors.New("insufficient balance")
	}

	withdraw := &model.CommissionWithdraw{
		UserID:  userID,
		Amount:  amount,
		Method:  method,
		Account: account,
		Name:    name,
		Status:  0,
	}

	tx := s.db.Begin()
	if err := tx.Create(withdraw).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 扣除余额
	if err := tx.Model(&model.User{}).Where("id = ?", userID).
		Update("commission_balance", gorm.Expr("commission_balance - ?", amount)).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return withdraw, nil
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