package model

import (
	"time"
)

// UserLevel 用户等级
type UserLevel struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:50" json:"name"`
	Level       int       `gorm:"uniqueIndex" json:"level"` // 等级数值
	Icon        string    `gorm:"size:255" json:"icon"`     // 图标
	Color       string    `gorm:"size:20" json:"color"`     // 颜色

	// 权益
	Discount    int       `gorm:"default:0" json:"discount"`      // 折扣百分比
	SpeedLimit  *int64    `json:"speed_limit"`                    // 专属速度限制
	DeviceLimit *int      `json:"device_limit"`                   // 专属设备限制

	// 升级条件
	MinDays     int       `gorm:"default:0" json:"min_days"`      // 最少使用天数
	MinTraffic  int64     `gorm:"default:0" json:"min_traffic"`   // 最少使用流量
	MinOrders   int       `gorm:"default:0" json:"min_orders"`    // 最少订单数
	MinAmount   float64   `gorm:"default:0" json:"min_amount"`    // 最少消费金额

	Enabled      bool     `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (UserLevel) TableName() string {
	return "v2_user_level"
}

// InviteCode 邀请码
type InviteCode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:32;uniqueIndex" json:"code"`
	UserID    *uint     `gorm:"index" json:"user_id"`    // 所属用户 (null=公共)
	Status    int       `gorm:"default:0" json:"status"` // 0=未使用 1=已使用
	UsedBy    *uint     `json:"used_by"`                 // 使用者ID
	UsedAt    *time.Time `json:"used_at"`
	ExpiredAt *time.Time `json:"expired_at"` // 过期时间 (null=永不过期)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (InviteCode) TableName() string {
	return "v2_invite_code"
}

// CommissionRecord 佣金记录
type CommissionRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`       // 获得佣金的用户
	OrderID     uint      `gorm:"index" json:"order_id"`      // 关联订单
	FromUserID  uint      `gorm:"index" json:"from_user_id"`  // 被邀请用户
	Amount      float64   `json:"amount"`                     // 佣金金额
	Type        int       `json:"type"`                       // 1=订单佣金 2=充值佣金 3=系统赠送
	Status      int       `gorm:"default:0" json:"status"`    // 0=待确认 1=已到账 2=已提现 3=已取消
	Remark      string    `gorm:"size:255" json:"remark"`

	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (CommissionRecord) TableName() string {
	return "v2_commission_record"
}

// CommissionWithdraw 佣金提现
type CommissionWithdraw struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	Amount      float64   `json:"amount"`         // 提现金额
	Method      string    `gorm:"size:20" json:"method"` // alipay/wechat/bank
	Account     string    `gorm:"size:100" json:"account"` // 收款账号
	Name        string    `gorm:"size:50" json:"name"`     // 收款人姓名
	Status      int       `gorm:"default:0" json:"status"` // 0=待处理 1=已处理 2=已拒绝
	Remark      string    `gorm:"size:255" json:"remark"`

	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (CommissionWithdraw) TableName() string {
	return "v2_commission_withdraw"
}

// InviteConfig 邀请配置
type InviteConfig struct {
	ID                    uint    `gorm:"primaryKey" json:"id"`
	Enabled               bool    `gorm:"default:true" json:"enabled"`

	// 邀请码设置
	AutoGenerate          bool    `gorm:"default:true" json:"auto_generate"`      // 新用户自动生成邀请码
	CodeCount             int     `gorm:"default:5" json:"code_count"`            // 每个用户邀请码数量
	CodeExpireDays        int     `gorm:"default:0" json:"code_expire_days"`      // 邀请码过期天数 (0=永不过期)

	// 佣金设置
	CommissionEnabled     bool    `gorm:"default:true" json:"commission_enabled"` // 是否启用佣金
	CommissionType        int     `gorm:"default:1" json:"commission_type"`       // 1=按比例 2=固定金额
	CommissionRate        float64 `gorm:"default:0.1" json:"commission_rate"`     // 佣金比例 (0.1 = 10%)
	CommissionFixed       float64 `gorm:"default:0" json:"commission_fixed"`      // 固定佣金金额
	CommissionMinAmount   float64 `gorm:"default:10" json:"commission_min"`       // 最低提现金额

	// 首次奖励
	FirstOrderBonus       float64 `gorm:"default:0" json:"first_order_bonus"`     // 首单奖励
	FirstTrafficBonus     int64   `gorm:"default:0" json:"first_traffic_bonus"`   // 首次流量奖励 (bytes)

	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// TableName 指定表名
func (InviteConfig) TableName() string {
	return "v2_invite_config"
}