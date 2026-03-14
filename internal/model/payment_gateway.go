package model

import (
	"time"
)

// PaymentGatewayType 支付网关类型
const (
	PaymentGatewayAlipay  = "alipay"   // 支付宝
	PaymentGatewayWechat  = "wechat"   // 微信支付
	PaymentGatewayEPay    = "epay"     // EPay通用支付
	PaymentGatewayUSDT    = "usdt"     // USDT加密货币 (TRC20/ERC20)
)

// PaymentGateway 支付网关配置 (扩展现有Payment模型)
type PaymentGateway struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Type        string    `gorm:"size:20;not null" json:"type"` // alipay/wechat/stripe/usdt/epay
	Enabled     bool      `gorm:"default:false" json:"enabled"`
	Icon        string    `gorm:"size:255" json:"icon"`         // 图标URL

	// 配置 (JSON格式存储不同支付方式的配置)
	Config      string    `gorm:"type:text" json:"config"`

	// 费率设置
	FeeRate     float64   `gorm:"default:0" json:"fee_rate"`     // 手续费率 (0-1)
	FeeFixed    float64   `gorm:"default:0" json:"fee_fixed"`    // 固定手续费

	// 限额设置
	MinAmount   float64   `gorm:"default:1" json:"min_amount"`   // 最小金额
	MaxAmount   float64   `gorm:"default:10000" json:"max_amount"` // 最大金额

	// 显示设置
	Sort        int       `gorm:"default:0" json:"sort"`         // 排序
	Description string    `gorm:"size:500" json:"description"`   // 说明文字

	// 统计
	TotalOrders int64     `json:"total_orders"`
	TotalAmount float64   `json:"total_amount"`

	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (PaymentGateway) TableName() string {
	return "v2_payment_gateway"
}

// AlipayConfig 支付宝配置
type AlipayConfig struct {
	AppID        string `json:"app_id"`         // 应用ID
	PrivateKey   string `json:"private_key"`    // 应用私钥
	PublicKey    string `json:"public_key"`     // 支付宝公钥
	NotifyURL    string `json:"notify_url"`     // 异步通知地址
	ReturnURL    string `json:"return_url"`     // 同步跳转地址
	Sandbox      bool   `json:"sandbox"`        // 沙箱模式
}

// WechatPayConfig 微信支付配置
type WechatPayConfig struct {
	AppID       string `json:"app_id"`        // 应用ID
	MchID       string `json:"mch_id"`        // 商户号
	APIKey      string `json:"api_key"`       // API密钥
	APIV3Key    string `json:"api_v3_key"`    // APIv3密钥
	CertPath    string `json:"cert_path"`     // 商户证书路径
	KeyPath     string `json:"key_path"`      // 商户私钥路径
	NotifyURL   string `json:"notify_url"`    // 异步通知地址
	Sandbox     bool   `json:"sandbox"`       // 沙箱模式
}

// USDTConfig USDT支付配置
type USDTConfig struct {
	Network       string `json:"network"`        // 网络 (TRC20/ERC20/BEP20)
	WalletAddress string `json:"wallet_address"` // 收款钱包地址
	APIKey        string `json:"api_key"`        // 区块链API密钥 (可选)
	Confirmations int    `json:"confirmations"`  // 确认数
}

// EPayConfig EPay配置
type EPayConfig struct {
	APIURL      string `json:"api_url"`       // API地址
	PID         string `json:"pid"`           // 商户ID
	Key         string `json:"key"`           // 商户密钥
	NotifyURL   string `json:"notify_url"`    // 通知地址
	ReturnURL   string `json:"return_url"`    // 跳转地址
}

// PaymentChannel 支付渠道 (用于前端显示)
type PaymentChannel struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Icon        string  `json:"icon"`
	MinAmount   float64 `json:"min_amount"`
	MaxAmount   float64 `json:"max_amount"`
	FeeRate     float64 `json:"fee_rate"`
	FeeFixed    float64 `json:"fee_fixed"`
	Description string  `json:"description"`
}

// PaymentRecord 支付记录
type PaymentRecord struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TradeNo       string    `gorm:"size:64;uniqueIndex" json:"trade_no"` // 商户订单号
	GatewayID     uint      `json:"gateway_id"`       // 支付网关ID
	GatewayType   string    `json:"gateway_type"`     // 支付类型
	GatewayTradeNo string   `gorm:"size:100" json:"gateway_trade_no"` // 第三方订单号
	UserID        uint      `json:"user_id"`

	// 金额信息
	Amount        float64   `json:"amount"`           // 订单金额
	FeeAmount     float64   `json:"fee_amount"`       // 手续费
	ActualAmount  float64   `json:"actual_amount"`    // 实际支付金额
	Currency      string    `gorm:"size:10;default:'CNY'" json:"currency"` // 货币类型

	// 状态 (复用 PaymentStatus 常量)
	Status        int       `json:"status"`           // 0=待支付 1=已支付 2=已取消 3=已退款
	PaidAt        *time.Time `json:"paid_at"`
	CancelledAt   *time.Time `json:"cancelled_at"`
	RefundedAt    *time.Time `json:"refunded_at"`

	// 回调信息
	NotifyData    string    `gorm:"type:text" json:"notify_data"` // 回调原始数据
	ClientIP      string    `gorm:"size:50" json:"client_ip"`

	// 关联
	OrderID       *uint     `json:"order_id"`         // 关联订单

	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名
func (PaymentRecord) TableName() string {
	return "v2_payment_record"
}