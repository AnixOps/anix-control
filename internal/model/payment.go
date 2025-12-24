package model

import "time"

// PaymentMethod 支付方式枚举
const (
	PaymentMethodCrypto = "crypto" // 虚拟货币支付 (X402)
	PaymentMethodFiat   = "fiat"   // 法币支付 (Stripe/PayPal等)
)

// PaymentProvider 支付提供商枚举
const (
	// 虚拟货币支付提供商
	PaymentProviderX402 = "x402" // X402 协议

	// 法币支付提供商
	PaymentProviderStripe    = "stripe"
	PaymentProviderPayPal    = "paypal"
	PaymentProviderAlipay    = "alipay"
	PaymentProviderWechatPay = "wechat_pay"
)

// PaymentStatus 支付状态枚举
const (
	PaymentStatusPending   = 0 // 待支付
	PaymentStatusPaid      = 1 // 已支付
	PaymentStatusCancelled = 2 // 已取消
	PaymentStatusRefunded  = 3 // 已退款
	PaymentStatusExpired   = 4 // 已过期
)

// Payment 支付配置模型
type Payment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100" json:"name"`    // 支付名称
	Icon      *string   `gorm:"size:255" json:"icon"`    // 支付图标
	Method    string    `gorm:"size:20" json:"method"`   // crypto 或 fiat
	Provider  string    `gorm:"size:50" json:"provider"` // x402, stripe, paypal 等
	Config    string    `gorm:"type:text" json:"config"` // JSON 配置
	Notify    *string   `gorm:"size:255" json:"notify"`  // 回调地址
	Sort      int       `gorm:"default:0" json:"sort"`   // 排序
	Enable    int       `gorm:"default:1" json:"enable"` // 是否启用
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Payment) TableName() string {
	return "v2_payment"
}

// PaymentLog 支付日志
type PaymentLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	OrderID       uint      `gorm:"index" json:"order_id"`
	PaymentID     uint      `gorm:"index" json:"payment_id"`
	TradeNo       string    `gorm:"size:64;index" json:"trade_no"`  // 内部交易号
	CallbackNo    *string   `gorm:"size:255" json:"callback_no"`    // 外部交易号
	Method        string    `gorm:"size:20" json:"method"`          // crypto 或 fiat
	Provider      string    `gorm:"size:50" json:"provider"`        // 支付提供商
	Amount        int64     `json:"amount"`                         // 支付金额 (分)
	Currency      string    `gorm:"size:10" json:"currency"`        // 货币类型 (USD, CNY, ETH, USDT等)
	Status        int       `gorm:"default:0" json:"status"`        // 支付状态
	RawData       *string   `gorm:"type:text" json:"raw_data"`      // 原始回调数据
	TxHash        *string   `gorm:"size:255" json:"tx_hash"`        // 区块链交易哈希 (仅虚拟货币)
	WalletAddress *string   `gorm:"size:255" json:"wallet_address"` // 钱包地址 (仅虚拟货币)
	Network       *string   `gorm:"size:50" json:"network"`         // 区块链网络 (仅虚拟货币)
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (PaymentLog) TableName() string {
	return "v2_payment_log"
}

// X402Config X402 虚拟货币支付配置
type X402Config struct {
	WalletAddress string   `json:"wallet_address"` // 收款钱包地址
	Network       string   `json:"network"`        // 网络 (ethereum, polygon, arbitrum, base, sepolia等)
	AcceptTokens  []string `json:"accept_tokens"`  // 接受的代币 (ETH, USDT, USDC等)
	TestMode      bool     `json:"test_mode"`      // 是否测试模式 (使用测试链)
	WebhookSecret string   `json:"webhook_secret"` // Webhook 密钥
	ConfirmBlocks int      `json:"confirm_blocks"` // 确认区块数
}

// StripeConfig Stripe 支付配置
type StripeConfig struct {
	PublishableKey string `json:"publishable_key"`
	SecretKey      string `json:"secret_key"`
	WebhookSecret  string `json:"webhook_secret"`
	Currency       string `json:"currency"` // USD, EUR 等
}

// PayPalConfig PayPal 支付配置
type PayPalConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	SandboxMode  bool   `json:"sandbox_mode"`
	Currency     string `json:"currency"`
}
