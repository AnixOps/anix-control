package model

import (
	"encoding/json"
	"time"
)

// SubscriptionGroup 订阅分组 (权限组)
// 不同分组可以看到不同的节点配置
type SubscriptionGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;uniqueIndex" json:"name"` // 分组名称
	Description *string   `gorm:"size:500" json:"description"`      // 分组描述
	Priority    int       `gorm:"default:0" json:"priority"`        // 优先级 (数值越大优先级越高)
	Enable      int       `gorm:"default:1" json:"enable"`          // 是否启用
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 关联
	Templates []SubscriptionTemplate `gorm:"foreignKey:GroupID" json:"templates,omitempty"`
	Protocols []NodeProtocol         `gorm:"many2many:v2_subscription_group_node_protocols;" json:"protocols,omitempty"`
}

func (SubscriptionGroup) TableName() string {
	return "v2_subscription_group"
}

// SubscriptionTemplate 订阅模板 (JSON 配置模板)
// 每个模板定义一种节点配置，通过函数变量赋值后下发给用户
// 符合 V2bX/Xray 协议配置规范
type SubscriptionTemplate struct {
	ID      uint    `gorm:"primaryKey" json:"id"`
	GroupID uint    `gorm:"index" json:"group_id"`   // 所属分组
	Name    string  `gorm:"size:255" json:"name"`    // 模板名称 (显示给用户的节点名)
	Type    string  `gorm:"size:30" json:"type"`     // 协议类型: vmess, vless, trojan, shadowsocks, hysteria2, tuic, anytls
	Enable  int     `gorm:"default:1" json:"enable"` // 是否启用
	Sort    int     `gorm:"default:0" json:"sort"`   // 排序
	Tags    *string `gorm:"size:255" json:"tags"`    // 标签 (JSON array)

	// JSON 模板配置 (高级功能)
	// 支持变量占位符: {{.UUID}}, {{.UserID}}, {{.Email}}, {{.ExpiredAt}}, {{.SpeedLimit}}, {{.DeviceLimit}}
	// 以及自定义变量: {{.Custom.xxx}}
	TemplateJSON string `gorm:"type:text" json:"template_json"`

	// 服务器基础配置 (对应 V2bX CommonNode)
	Server     string  `gorm:"size:255" json:"server"`      // 服务器地址 (host)
	Port       int     `json:"port"`                        // 端口 (server_port)
	ServerName *string `gorm:"size:255" json:"server_name"` // SNI 服务器名称

	// TLS 配置 (对应 V2bX tls 字段)
	TLS            int     `gorm:"default:0" json:"tls"`           // 0=无, 1=TLS, 2=Reality
	TLSFingerprint *string `gorm:"size:50" json:"tls_fingerprint"` // TLS 指纹 (chrome, firefox, safari, ios, android, edge, 360, qq, random)
	ALPN           *string `gorm:"size:100" json:"alpn"`           // ALPN (h2,http/1.1)

	// Reality 配置 (当 TLS=2 时使用, 对应 V2bX tls_settings)
	RealityPublicKey *string `gorm:"size:100" json:"reality_public_key"` // X25519 公钥 (对应 pbk)
	RealityShortID   *string `gorm:"size:50" json:"reality_short_id"`    // Short ID (对应 sid)
	RealitySpiderX   *string `gorm:"size:255" json:"reality_spider_x"`   // SpiderX (对应 spx)
	RealityDest      *string `gorm:"size:255" json:"reality_dest"`       // 回落目标 (dest)

	// 传输层配置 (对应 V2bX network, network_settings)
	Transport         string  `gorm:"size:20;default:tcp" json:"transport"`          // 传输协议: tcp, ws, grpc, httpupgrade, xhttp
	TransportSettings *string `gorm:"type:text" json:"transport_settings,omitempty"` // 传输层配置 JSON

	// VLESS 特有配置 (对应 V2bX flow, encryption, encryption_settings)
	Flow               *string `gorm:"size:50" json:"flow"`                  // 流控: xtls-rprx-vision
	Encryption         *string `gorm:"size:50" json:"encryption"`            // 加密: 空 或 mlkem768x25519plus
	EncryptionSettings *string `gorm:"type:text" json:"encryption_settings"` // 加密配置 JSON

	// Shadowsocks 配置 (对应 V2bX cipher, server_key)
	SSCipher    *string `gorm:"size:50" json:"ss_cipher"`      // SS 加密方式
	SSServerKey *string `gorm:"size:100" json:"ss_server_key"` // SS2022 服务器密钥

	// 协议特定配置 (存储其他不在通用字段中的配置)
	ProtocolSettings *string `gorm:"type:text" json:"protocol_settings"` // 协议配置 JSON

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	Group *SubscriptionGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (SubscriptionTemplate) TableName() string {
	return "v2_subscription_template"
}

// UserSubscriptionGroup 用户-订阅分组关联 (多对多)
// 用户可以属于多个分组，拥有多个分组的权限
type UserSubscriptionGroup struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"uniqueIndex:idx_user_group" json:"user_id"`
	GroupID        uint      `gorm:"uniqueIndex:idx_user_group" json:"group_id"`
	ExpireAt       *int64    `json:"expire_at"`        // 分组权限过期时间 (null=永不过期)
	TransferEnable *int64    `json:"transfer_enable"`  // 自定义总流量 (bytes), null 表示不覆盖用户默认
	NextRenewPrice *int64    `json:"next_renew_price"` // 下次续费价格 (单位分)
	CreatedAt      time.Time `json:"created_at"`

	// 关联
	User  *User              `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Group *SubscriptionGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (UserSubscriptionGroup) TableName() string {
	return "v2_user_subscription_group"
}

// PlanSubscriptionGroup 套餐-订阅分组关联
// 购买某个套餐自动获得对应分组权限
type PlanSubscriptionGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PlanID    uint      `gorm:"uniqueIndex:idx_plan_group" json:"plan_id"`
	GroupID   uint      `gorm:"uniqueIndex:idx_plan_group" json:"group_id"`
	CreatedAt time.Time `json:"created_at"`

	// 关联
	Plan  *Plan              `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	Group *SubscriptionGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

func (PlanSubscriptionGroup) TableName() string {
	return "v2_plan_subscription_group"
}

// TemplateRenderContext 模板渲染上下文
// 传递给模板引擎进行变量替换
type TemplateRenderContext struct {
	// 用户信息
	UUID        string `json:"uuid"`
	UserID      uint   `json:"user_id"`
	Email       string `json:"email"`
	ExpiredAt   int64  `json:"expired_at"`
	SpeedLimit  int64  `json:"speed_limit"` // bytes/s
	DeviceLimit int    `json:"device_limit"`

	// 流量信息
	TransferEnable int64 `json:"transfer_enable"` // 总流量
	UsedTraffic    int64 `json:"used_traffic"`    // 已用流量

	// 自定义变量 (管理员可扩展)
	Custom map[string]interface{} `json:"custom,omitempty"`

	// 订阅元信息 (用于生成客户端特定配置，如 Surge managed-config)
	SubscribeURL    string `json:"subscribe_url,omitempty"`
	SubscribeDomain string `json:"subscribe_domain,omitempty"`
}

// ParsedNode 解析后的统一节点格式
// 符合 V2bX/Xray 协议配置规范
type ParsedNode struct {
	ID     string `json:"id"`     // 唯一标识 (hash)
	Name   string `json:"name"`   // 节点名称
	Type   string `json:"type"`   // 协议类型: vless, vmess, trojan, shadowsocks, hysteria2, tuic, anytls
	Server string `json:"server"` // 服务器地址 (host)
	Port   int    `json:"port"`   // 服务器端口 (server_port)

	// 认证信息 (渲染后填入)
	UUID     string `json:"uuid,omitempty"`     // VMess/VLESS UUID (用户ID)
	Password string `json:"password,omitempty"` // Trojan/SS 密码

	// TLS 配置 (tls: 0=None, 1=TLS, 2=Reality)
	TLSMode        int    `json:"tls_mode"`                   // TLS 模式: 0=无, 1=TLS, 2=Reality
	TLS            bool   `json:"tls"`                        // 是否启用 TLS (兼容字段)
	TLSFingerprint string `json:"tls_fingerprint,omitempty"`  // TLS 指纹 (fp)
	SkipCertVerify bool   `json:"skip_cert_verify,omitempty"` // 跳过证书验证
	ServerName     string `json:"server_name,omitempty"`      // SNI 服务器名称
	ALPN           string `json:"alpn,omitempty"`             // ALPN 设置

	// Reality 配置 (当 TLSMode=2 时使用)
	RealityPublicKey string `json:"reality_public_key,omitempty"` // Reality 公钥 (pbk)
	RealityShortID   string `json:"reality_short_id,omitempty"`   // Reality ShortId (sid)
	RealitySpiderX   string `json:"reality_spider_x,omitempty"`   // Reality SpiderX (spx)

	// VLESS 特有配置
	Flow       string `json:"flow,omitempty"`       // VLESS 流控: xtls-rprx-vision
	Encryption string `json:"encryption,omitempty"` // VLESS 加密: 空 或 mlkem768x25519plus

	// VLESS Encryption Settings (当 Encryption 非空时)
	EncryptionSettings *EncryptionSettings `json:"encryption_settings,omitempty"`

	// Shadowsocks 配置
	Cipher    string `json:"cipher,omitempty"`     // SS 加密方式
	ServerKey string `json:"server_key,omitempty"` // SS2022 服务器密钥

	// 传输层配置 (network)
	Transport         string                 `json:"transport,omitempty"`          // 传输协议: tcp, ws, grpc, httpupgrade, xhttp
	TransportSettings map[string]interface{} `json:"transport_settings,omitempty"` // 传输层配置

	// 协议特定配置 (兼容旧字段)
	Settings map[string]interface{} `json:"settings,omitempty"`

	// 来源信息
	SourceType string `json:"source_type"` // template, node, external
	SourceID   uint   `json:"source_id"`   // 来源 ID
	SourceName string `json:"source_name"` // 来源名称
	GroupID    uint   `json:"group_id"`    // 所属分组
	GroupName  string `json:"group_name"`  // 分组名称
}

// EncryptionSettings VLESS 加密设置 (mlkem768x25519plus)
type EncryptionSettings struct {
	Mode          string `json:"mode,omitempty"`           // 模式: native
	Ticket        string `json:"ticket,omitempty"`         // Ticket: 0rtt
	ServerPadding string `json:"server_padding,omitempty"` // 服务器填充
	PrivateKey    string `json:"private_key,omitempty"`    // 加密私钥
}

// GetTransportSettings 获取传输层配置
func (t *SubscriptionTemplate) GetTransportSettings() map[string]interface{} {
	if t.TransportSettings == nil || *t.TransportSettings == "" {
		return nil
	}
	var settings map[string]interface{}
	json.Unmarshal([]byte(*t.TransportSettings), &settings)
	return settings
}

// GetProtocolSettings 获取协议配置
func (t *SubscriptionTemplate) GetProtocolSettings() map[string]interface{} {
	if t.ProtocolSettings == nil || *t.ProtocolSettings == "" {
		return nil
	}
	var settings map[string]interface{}
	json.Unmarshal([]byte(*t.ProtocolSettings), &settings)
	return settings
}

// GetTags 获取标签列表
func (t *SubscriptionTemplate) GetTags() []string {
	if t.Tags == nil || *t.Tags == "" {
		return nil
	}
	var tags []string
	json.Unmarshal([]byte(*t.Tags), &tags)
	return tags
}

// SubscriptionFormat 订阅格式类型
type SubscriptionFormat string

const (
	FormatV2Ray        SubscriptionFormat = "v2ray"        // Base64 编码的链接列表
	FormatClash        SubscriptionFormat = "clash"        // Clash YAML
	FormatStash        SubscriptionFormat = "stash"        // Stash YAML (兼容 Clash)
	FormatEgern        SubscriptionFormat = "egern"        // Egern YAML (兼容 Clash)
	FormatSurge        SubscriptionFormat = "surge"        // Surge 配置
	FormatLoon         SubscriptionFormat = "loon"         // Loon 配置
	FormatShadowrocket SubscriptionFormat = "shadowrocket" // Shadowrocket 配置
	FormatQuantumultX  SubscriptionFormat = "quantumultx"  // Quantumult X 配置
	FormatJSON         SubscriptionFormat = "json"         // 原始 JSON
	FormatBase64JSON   SubscriptionFormat = "base64json"   // Base64 编码的 JSON (你的自定义格式)
	FormatSingBox      SubscriptionFormat = "sing-box"     // Sing-box JSON
)

// SubscriptionRequest 订阅请求参数
type SubscriptionRequest struct {
	Token           string             `json:"token"`             // 用户 Token
	Format          SubscriptionFormat `json:"format,omitempty"`  // 输出格式
	Groups          []uint             `json:"groups,omitempty"`  // 指定分组 (可选)
	Include         string             `json:"include,omitempty"` // 包含关键词
	Exclude         string             `json:"exclude,omitempty"` // 排除关键词
	SubscribeURL    string             `json:"-"`                 // 当前请求订阅 URL（供格式化器使用）
	SubscribeDomain string             `json:"-"`                 // 当前请求订阅域名（供格式化器使用）
}

// SubscriptionResponse 订阅响应
type SubscriptionResponse struct {
	Content      string `json:"content"`                 // 订阅内容
	ContentType  string `json:"content_type"`            // MIME 类型
	Filename     string `json:"filename,omitempty"`      // 下载文件名
	UpdatedAt    int64  `json:"updated_at"`              // 更新时间
	ExpireAt     int64  `json:"expire_at,omitempty"`     // 用户过期时间
	UsedTraffic  int64  `json:"used_traffic,omitempty"`  // 已用流量
	TotalTraffic int64  `json:"total_traffic,omitempty"` // 总流量
}
