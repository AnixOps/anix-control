package model

import (
	"time"
)

// NodeStatus 节点状态
type NodeStatus int

const (
	NodeStatusPending  NodeStatus = 0 // 待审核
	NodeStatusOnline   NodeStatus = 1 // 在线
	NodeStatusOffline  NodeStatus = 2 // 离线
	NodeStatusDisabled NodeStatus = 3 // 已禁用
)

// Node 节点模型 (物理服务器)
type Node struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:255" json:"name"`           // 节点名称
	Host         string     `gorm:"size:255" json:"host"`           // 服务器地址 (IP或域名)
	Port         int        `gorm:"default:443" json:"port"`        // API通信端口
	APIKey       string     `gorm:"size:64;uniqueIndex" json:"-"`   // 节点API密钥 (用于身份验证)
	APIKeyHash   string     `gorm:"size:64" json:"-"`               // 密钥哈希 (用于快速查找)
	Secret       string     `gorm:"size:255" json:"-"`              // 共享密钥 (用于加密通信)
	Status       NodeStatus `gorm:"default:0" json:"status"`        // 节点状态
	Tags         *string    `gorm:"size:255" json:"tags"`           // 标签 (JSON array)
	GroupID      *uint      `gorm:"index" json:"group_id"`          // 节点分组
	Rate         float64    `gorm:"default:1" json:"rate"`          // 流量倍率
	TrafficRate  float64    `gorm:"default:1" json:"traffic_rate"`  // 计费倍率
	Sort         int        `gorm:"default:0" json:"sort"`          // 排序
	Show         int        `gorm:"default:1" json:"show"`          // 是否显示给用户
	AutoRegister int        `gorm:"default:0" json:"auto_register"` // 是否自动注册的节点

	// 高级配置 (直接JSON编辑)
	RawConfig *string `gorm:"type:text" json:"raw_config"` // 原始配置 (JSON, 优先级最高)

	// 服务器信息 (由节点上报)
	ServerIP      *string `gorm:"size:45" json:"server_ip"`      // 实际服务器IP
	ServerVersion *string `gorm:"size:50" json:"server_version"` // 节点程序版本
	ServerOS      *string `gorm:"size:100" json:"server_os"`     // 操作系统信息
	CPUUsage      float64 `gorm:"default:0" json:"cpu_usage"`    // CPU使用率
	MemoryUsage   float64 `gorm:"default:0" json:"memory_usage"` // 内存使用率
	DiskUsage     float64 `gorm:"default:0" json:"disk_usage"`   // 磁盘使用率
	Uptime        int64   `gorm:"default:0" json:"uptime"`       // 运行时间(秒)
	OnlineUsers   int     `gorm:"default:0" json:"online_users"` // 在线用户数

	// 流量统计
	TotalUpload   int64 `gorm:"default:0" json:"total_upload"`   // 总上传流量
	TotalDownload int64 `gorm:"default:0" json:"total_download"` // 总下载流量

	// 时间戳
	LastCheckAt *int64    `json:"last_check_at"` // 最后心跳时间
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 关联
	Protocols []NodeProtocol `gorm:"foreignKey:NodeID" json:"protocols,omitempty"`
}

func (Node) TableName() string {
	return "v2_node"
}

// IsOnline 检查节点是否在线 (5分钟内有心跳)
func (n *Node) IsOnline() bool {
	if n.LastCheckAt == nil {
		return false
	}
	return time.Now().Unix()-*n.LastCheckAt < 300
}

// ProtocolType 协议类型
type ProtocolType string

const (
	ProtocolVMess       ProtocolType = "vmess"
	ProtocolVLESS       ProtocolType = "vless"
	ProtocolTrojan      ProtocolType = "trojan"
	ProtocolShadowsocks ProtocolType = "shadowsocks"
	ProtocolHysteria2   ProtocolType = "hysteria2"
	ProtocolTUIC        ProtocolType = "tuic"
	ProtocolAnyTLS      ProtocolType = "anytls"
)

// NodeProtocol 节点协议配置
type NodeProtocol struct {
	ID     uint         `gorm:"primaryKey" json:"id"`
	NodeID uint         `gorm:"index" json:"node_id"`      // 所属节点
	Name   string       `gorm:"size:100" json:"name"`      // 协议名称
	Type   ProtocolType `gorm:"size:20;index" json:"type"` // 协议类型
	Port   int          `json:"port"`                      // 监听端口
	Enable int          `gorm:"default:1" json:"enable"`   // 是否启用
	Sort   int          `gorm:"default:0" json:"sort"`     // 排序

	// 通用配置
	Host *string `gorm:"size:255" json:"host"` // 连接地址 (可覆盖节点host)
	TLS  int     `gorm:"default:0" json:"tls"` // TLS模式: 0=无, 1=TLS, 2=Reality
	ALPN *string `gorm:"size:100" json:"alpn"` // ALPN设置

	// 协议特定配置 (JSON)
	Settings          *string `gorm:"type:text" json:"settings"`           // 协议配置 (JSON)
	TLSSettings       *string `gorm:"type:text" json:"tls_settings"`       // TLS配置 (JSON)
	Transport         *string `gorm:"size:20" json:"transport"`            // 传输层: tcp, ws, grpc, quic, h2
	TransportSettings *string `gorm:"type:text" json:"transport_settings"` // 传输层配置 (JSON)

	// Reality 配置
	RealitySettings *string `gorm:"type:text" json:"reality_settings"` // Reality配置 (JSON)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	Node *Node `gorm:"foreignKey:NodeID" json:"node,omitempty"`
}

func (NodeProtocol) TableName() string {
	return "v2_node_protocol"
}

// NodeGroup 节点分组
type NodeGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100" json:"name"`
	Sort      int       `gorm:"default:0" json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (NodeGroup) TableName() string {
	return "v2_node_group"
}

// AuthorizedKey 授权密钥 (用于节点自动注册)
type AuthorizedKey struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:100" json:"name"`         // 密钥名称/备注
	Key          string    `gorm:"size:64;uniqueIndex" json:"-"` // 授权密钥
	KeyHash      string    `gorm:"size:64" json:"-"`             // 密钥哈希
	Used         int       `gorm:"default:0" json:"used"`        // 是否已使用
	UsedByNodeID *uint     `json:"used_by_node_id"`              // 使用此密钥的节点ID
	ExpireAt     *int64    `json:"expire_at"`                    // 过期时间 (null=永不过期)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AuthorizedKey) TableName() string {
	return "v2_authorized_key"
}

// NodeRegisterRequest 节点注册请求
type NodeRegisterRequest struct {
	AuthKey       string `json:"auth_key" binding:"required"` // 授权密钥
	Name          string `json:"name"`                        // 节点名称
	Host          string `json:"host"`                        // 节点地址
	Port          int    `json:"port"`                        // API端口
	ServerVersion string `json:"server_version"`              // 节点版本
	ServerOS      string `json:"server_os"`                   // 操作系统
}

// NodeRegisterResponse 节点注册响应
type NodeRegisterResponse struct {
	NodeID  uint   `json:"node_id"`
	APIKey  string `json:"api_key"` // 返回给节点的API密钥
	Secret  string `json:"secret"`  // 共享密钥
	Message string `json:"message"`
}

// NodeHeartbeatRequest 节点心跳请求
type NodeHeartbeatRequest struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	Uptime      int64   `json:"uptime"`
	OnlineUsers int     `json:"online_users"`
	Upload      int64   `json:"upload"`   // 本次上传流量增量
	Download    int64   `json:"download"` // 本次下载流量增量
}

// ProtocolTemplate 协议模板 (快速添加)
type ProtocolTemplate struct {
	Type        ProtocolType `json:"type"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	DefaultPort int          `json:"default_port"`
	Settings    string       `json:"settings"` // 默认配置JSON
}

// GetProtocolTemplates 获取协议模板列表
func GetProtocolTemplates() []ProtocolTemplate {
	return []ProtocolTemplate{
		{
			Type:        ProtocolVMess,
			Name:        "VMess",
			Description: "V2Ray VMess 协议",
			DefaultPort: 443,
			Settings:    `{"security":"auto"}`,
		},
		{
			Type:        ProtocolVLESS,
			Name:        "VLESS",
			Description: "V2Ray VLESS 协议 (推荐)",
			DefaultPort: 443,
			Settings:    `{"flow":"xtls-rprx-vision"}`,
		},
		{
			Type:        ProtocolTrojan,
			Name:        "Trojan",
			Description: "Trojan 协议",
			DefaultPort: 443,
			Settings:    `{}`,
		},
		{
			Type:        ProtocolShadowsocks,
			Name:        "Shadowsocks",
			Description: "Shadowsocks 协议",
			DefaultPort: 8388,
			Settings:    `{"method":"2022-blake3-aes-128-gcm"}`,
		},
		{
			Type:        ProtocolHysteria2,
			Name:        "Hysteria2",
			Description: "Hysteria2 协议 (高速)",
			DefaultPort: 443,
			Settings:    `{"up_mbps":100,"down_mbps":100}`,
		},
		{
			Type:        ProtocolTUIC,
			Name:        "TUIC",
			Description: "TUIC v5 协议",
			DefaultPort: 443,
			Settings:    `{"congestion_control":"bbr"}`,
		},
		{
			Type:        ProtocolAnyTLS,
			Name:        "AnyTLS",
			Description: "AnyTLS 协议",
			DefaultPort: 443,
			Settings:    `{}`,
		},
	}
}
