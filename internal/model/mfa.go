package model

import (
	"time"
)

// UserMFA 用户多因素认证
type UserMFA struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"uniqueIndex" json:"user_id"`
	User        *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Enabled     bool      `gorm:"default:false" json:"enabled"`

	// TOTP配置
	TOTPSecret  string    `gorm:"size:100" json:"totp_secret"`    // TOTP密钥 (base32)
	BackupCodes string    `gorm:"type:text" json:"backup_codes"`  // 备用码 (JSON数组)

	// 短信/邮箱验证
	Phone       string    `gorm:"size:20" json:"phone"`           // 手机号
	PhoneVerified bool    `gorm:"default:false" json:"phone_verified"`

	// 最后使用时间
	LastUsed    *time.Time `json:"last_used"`
	LastMethod  string    `gorm:"size:20" json:"last_method"`     // totp/sms/email

	// 恢复信息
	RecoveryEmail string  `gorm:"size:100" json:"recovery_email"` // 恢复邮箱

	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (UserMFA) TableName() string {
	return "v2_user_mfa"
}

// MFALoginAttempt MFA登录尝试
type MFALoginAttempt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	IP        string    `gorm:"size:50" json:"ip"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	Success   bool      `json:"success"`
	Method    string    `gorm:"size:20" json:"method"` // totp/sms/email/backup
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (MFALoginAttempt) TableName() string {
	return "v2_mfa_login_attempt"
}

// MFAMethod MFA方式
const (
	MFAMethodTOTP   = "totp"   // TOTP验证器
	MFAMethodSMS    = "sms"    // 短信验证
	MFAMethodEmail  = "email"  // 邮箱验证
	MFAMethodBackup = "backup" // 备用码
)

// MFAConfig MFA全局配置
type MFAConfig struct {
	Enabled         bool   `json:"enabled"`           // 是否启用MFA
	EnforceForAll   bool   `json:"enforce_for_all"`   // 强制所有用户启用
	EnforceForAdmin bool   `json:"enforce_for_admin"` // 强制管理员启用
	AllowedMethods  string `json:"allowed_methods"`   // 允许的方式 (JSON数组)
	TOTPIssuer      string `json:"totp_issuer"`       // TOTP发行者名称
	BackupCodeCount int    `json:"backup_code_count"` // 备用码数量
}