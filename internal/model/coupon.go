package model

import "time"

// Coupon 优惠券模型
type Coupon struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Code         string    `gorm:"size:32;uniqueIndex" json:"code"`
	Name         string    `gorm:"size:255" json:"name"`
	Type         int       `gorm:"default:1" json:"type"`       // 1: percentage, 2: fixed amount
	Value        int       `json:"value"`                       // percentage (0-100) or amount in cents
	LimitUse     *int      `json:"limit_use"`                   // null or -1 = unlimited
	LimitUseWith *uint     `gorm:"index" json:"limit_use_with"` // limit to specific plan ID
	LimitPeriod  *string   `gorm:"size:20" json:"limit_period"` // limit to specific period
	UseCount     int       `gorm:"default:0" json:"use_count"`  // how many times used
	StartedAt    int64     `json:"started_at"`                  // unix timestamp
	EndedAt      int64     `json:"ended_at"`                    // unix timestamp
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Coupon) TableName() string {
	return "v2_coupon"
}

// CouponUsage 优惠券使用记录
type CouponUsage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CouponID  uint      `gorm:"index" json:"coupon_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	OrderID   uint      `gorm:"index" json:"order_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (CouponUsage) TableName() string {
	return "v2_coupon_usage"
}
