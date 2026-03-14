package model

import (
	"time"
)

// NotificationTemplate 通知模板
type NotificationTemplate struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Type        string    `gorm:"size:20;not null" json:"type"` // email/telegram/webhook
	Event       string    `gorm:"size:50;not null" json:"event"` // 触发事件
	Title       string    `gorm:"size:200" json:"title"`         // 标题模板
	Content     string    `gorm:"type:text" json:"content"`      // 内容模板
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (NotificationTemplate) TableName() string {
	return "v2_notification_template"
}

// NotificationLog 通知日志
type NotificationLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     *uint     `gorm:"index" json:"user_id"`
	Type       string    `gorm:"size:20" json:"type"`    // email/telegram/webhook
	Event      string    `gorm:"size:50" json:"event"`   // 触发事件
	Title      string    `gorm:"size:200" json:"title"`
	Content    string    `gorm:"type:text" json:"content"`
	Status     int       `json:"status"` // 0=待发送 1=已发送 2=失败
	Error      string    `gorm:"size:500" json:"error"`
	SentAt     *time.Time `json:"sent_at"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (NotificationLog) TableName() string {
	return "v2_notification_log"
}

// NotificationEvent 通知事件
const (
	// 用户相关
	EventUserRegister    = "user.register"     // 用户注册
	EventUserLogin       = "user.login"        // 用户登录 (新设备)
	EventUserPasswordReset = "user.password_reset" // 密码重置
	EventUserMFAEnabled  = "user.mfa_enabled"  // MFA启用

	// 账户状态
	EventUserExpire      = "user.expire"       // 用户到期 (提前通知)
	EventUserExpired     = "user.expired"      // 用户已到期
	EventUserTrafficLow  = "user.traffic_low"  // 流量不足
	EventUserTrafficExhausted = "user.traffic_exhausted" // 流量耗尽
	EventUserBanned      = "user.banned"       // 用户被封禁

	// 订单支付
	EventOrderCreated    = "order.created"     // 订单创建
	EventOrderPaid       = "order.paid"        // 订单支付成功
	EventOrderExpired    = "order.expired"     // 订单过期
	EventOrderRefunded   = "order.refunded"    // 订单退款

	// 工单系统
	EventTicketCreated   = "ticket.created"    // 工单创建
	EventTicketReplied   = "ticket.replied"    // 工单回复
	EventTicketClosed    = "ticket.closed"     // 工单关闭

	// 节点相关
	EventNodeOffline     = "node.offline"      // 节点离线
	EventNodeOnline      = "node.online"       // 节点上线
	EventNodeHighLoad    = "node.high_load"    // 节点高负载

	// 系统通知
	EventSystemBroadcast = "system.broadcast"  // 系统广播
	EventSystemMaintenance = "system.maintenance" // 维护通知
)

// EmailConfig 邮件配置
type EmailConfig struct {
	Host        string `json:"host"`         // SMTP服务器
	Port        int    `json:"port"`         // 端口
	Username    string `json:"username"`     // 用户名
	Password    string `json:"password"`     // 密码/授权码
	FromName    string `json:"from_name"`    // 发件人名称
	FromAddress string `json:"from_address"` // 发件人地址
	Encryption  string `json:"encryption"`   // ssl/tls/none
}