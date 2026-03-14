package model

import (
	"time"
)

// TelegramBot Telegram Bot配置
type TelegramBot struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Token       string    `gorm:"size:200;not null" json:"token"`
	Enabled     bool      `gorm:"default:false" json:"enabled"`

	// Webhook配置
	WebhookURL  string    `gorm:"size:255" json:"webhook_url"`
	WebhookSet  bool      `gorm:"default:false" json:"webhook_set"`

	// 管理员
	AdminIDs    string    `gorm:"type:text" json:"admin_ids"`  // JSON数组

	// 欢迎消息
	WelcomeMsg  string    `gorm:"type:text" json:"welcome_msg"`

	// 功能开关
	AllowBind   bool      `gorm:"default:true" json:"allow_bind"`   // 允许绑定账户
	AllowSub    bool      `gorm:"default:true" json:"allow_sub"`    // 允许获取订阅
	AllowTicket bool      `gorm:"default:true" json:"allow_ticket"` // 允许创建工单
	AllowInfo   bool      `gorm:"default:true" json:"allow_info"`   // 允许查看信息

	// 统计
	TotalUsers  int64     `json:"total_users"`
	TotalChats  int64     `json:"total_chats"`

	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (TelegramBot) TableName() string {
	return "v2_telegram_bot"
}

// TelegramUser Telegram用户绑定
type TelegramUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"uniqueIndex" json:"user_id"`       // 系统用户ID
	User         *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TelegramID   int64     `gorm:"uniqueIndex" json:"telegram_id"`   // Telegram用户ID
	Username     string    `gorm:"size:100" json:"username"`          // Telegram用户名
	FirstName    string    `gorm:"size:100" json:"first_name"`        // 名
	LastName     string    `gorm:"size:100" json:"last_name"`         // 姓
	LanguageCode string    `gorm:"size:10" json:"language_code"`      // 语言代码

	// 状态
	IsBanned     bool      `gorm:"default:false" json:"is_banned"`
	BannedAt     *time.Time `json:"banned_at"`

	// 通知设置
	NotifyExpire bool      `gorm:"default:true" json:"notify_expire"` // 到期通知
	NotifyTraffic bool     `gorm:"default:true" json:"notify_traffic"` // 流量通知
	NotifyTicket bool      `gorm:"default:true" json:"notify_ticket"`  // 工单通知

	// 统计
	MessageCount int64     `json:"message_count"`
	LastActive   time.Time `json:"last_active"`

	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (TelegramUser) TableName() string {
	return "v2_telegram_user"
}

// TelegramChat Telegram聊天记录
type TelegramChat struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	BotID        uint      `gorm:"index" json:"bot_id"`
	TelegramID   int64     `gorm:"index" json:"telegram_id"`
	UserID       *uint     `json:"user_id"`

	// 消息
	MessageID    int64     `json:"message_id"`
	MessageType  string    `gorm:"size:20" json:"message_type"` // text/command/callback
	Content      string    `gorm:"type:text" json:"content"`

	// 回复
	ReplyToID    *uint     `json:"reply_to_id"`

	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (TelegramChat) TableName() string {
	return "v2_telegram_chat"
}

// TelegramCommand Bot命令定义
type TelegramCommand struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	BotID       uint   `json:"bot_id"`
	Command     string `gorm:"size:50;not null" json:"command"`     // 命令 (不含/)
	Description string `gorm:"size:200" json:"description"`         // 描述
	Handler     string `gorm:"size:100" json:"handler"`             // 处理函数名
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	IsAdmin     bool   `gorm:"default:false" json:"is_admin"`       // 是否管理员命令
	Sort        int    `gorm:"default:0" json:"sort"`
}

// TableName 指定表名
func (TelegramCommand) TableName() string {
	return "v2_telegram_command"
}

// TelegramNotification Telegram通知记录
type TelegramNotification struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TelegramID int64     `gorm:"index" json:"telegram_id"`
	UserID     *uint     `json:"user_id"`

	// 通知内容
	Type       string    `gorm:"size:50" json:"type"` // expire/traffic/ticket/system
	Title      string    `gorm:"size:200" json:"title"`
	Content    string    `gorm:"type:text" json:"content"`

	// 状态
	Status     int       `json:"status"` // 0=待发送 1=已发送 2=发送失败
	Error      string    `gorm:"size:500" json:"error"`
	SentAt     *time.Time `json:"sent_at"`

	CreatedAt  time.Time `json:"created_at"`
}

// TableName 指定表名
func (TelegramNotification) TableName() string {
	return "v2_telegram_notification"
}

// NotificationType 通知类型
const (
	NotificationTypeExpire  = "expire"   // 到期通知
	NotificationTypeTraffic = "traffic"  // 流量不足
	NotificationTypeTicket  = "ticket"   // 工单回复
	NotificationTypeSystem  = "system"   // 系统通知
	NotificationTypePayment = "payment"  // 支付通知
	NotificationTypeNode    = "node"     // 节点通知
)