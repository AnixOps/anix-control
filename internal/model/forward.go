package model

import (
	"time"
)

// ForwardNodeType 节点类型
const (
	ForwardNodeTypeRelay = "relay" // 中转节点
	ForwardNodeTypeExit  = "exit"  // 落地节点
)

// ForwardNodeStatus 节点状态
const (
	ForwardNodeStatusOffline = 0
	ForwardNodeStatusOnline  = 1
)

// ForwardNode 中转/落地节点
type ForwardNode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Type      string    `gorm:"size:20;not null;default:'exit'" json:"type"` // relay/exit
	Host      string    `gorm:"size:255;not null" json:"host"`
	Port      int       `gorm:"not null" json:"port"`
	APIPort   int       `json:"api_port"`         // 管理API端口
	APIToken  string    `gorm:"size:100" json:"api_token"`

	// 节点信息
	Region    string    `gorm:"size:50" json:"region"`   // 地区: HK, US, JP, SG等
	ISP       string    `gorm:"size:50" json:"isp"`      // 运营商
	Datacenter string   `gorm:"size:100" json:"datacenter"` // 数据中心
	Bandwidth int64     `json:"bandwidth"`               // 带宽(Mbps)

	// 状态
	Status    int       `gorm:"default:0" json:"status"`
	LastCheck time.Time `json:"last_check"`
	Latency   int       `json:"latency"`      // 延迟(ms)
	Load      float64   `json:"load"`         // 负载 0-1
	Uptime    float64   `json:"uptime"`       // 在线率 0-100

	// 配置
	Tags      string    `gorm:"type:text" json:"tags"`    // JSON数组
	Weight    int       `gorm:"default:1" json:"weight"` // 负载均衡权重
	MaxConn   int       `json:"max_conn"`                // 最大连接数
	Enabled   bool      `gorm:"default:true" json:"enabled"`

	// 统计
	TotalUpload   int64 `json:"total_upload"`
	TotalDownload int64 `json:"total_download"`
	CurrentConn   int   `json:"current_conn"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardNode) TableName() string {
	return "v2_forward_node"
}

// ForwardRule 转发规则
type ForwardRule struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"size:100;not null" json:"name"`
	Enabled bool   `gorm:"default:true" json:"enabled"`

	// 入口配置 (中转节点)
	RelayNodeID uint  `gorm:"not null" json:"relay_node_id"`
	RelayNode   *ForwardNode `gorm:"foreignKey:RelayNodeID" json:"relay_node,omitempty"`
	ListenPort  int   `gorm:"not null" json:"listen_port"`   // 监听端口
	Protocol    string `gorm:"size:10;default:'tcp'" json:"protocol"` // tcp/udp/both

	// 出口配置 (落地节点)
	ExitNodeID uint  `gorm:"not null" json:"exit_node_id"`
	ExitNode   *ForwardNode `gorm:"foreignKey:ExitNodeID" json:"exit_node,omitempty"`
	TargetHost string `gorm:"size:255;not null" json:"target_host"` // 目标地址
	TargetPort int   `gorm:"not null" json:"target_port"`          // 目标端口

	// 用户绑定 (可选，为空则为公共规则)
	UserID      *uint  `json:"user_id"`
	User        *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	UserGroupID *uint  `json:"user_group_id"`

	// 流量控制
	SpeedLimit   *int64    `json:"speed_limit"`    // 速度限制(KB/s), nil为不限
	TrafficLimit *int64    `json:"traffic_limit"`  // 流量限制(字节), nil为不限
	ExpireTime   *time.Time `json:"expire_time"`   // 过期时间, nil为永不过期

	// 统计
	Upload     int64 `json:"upload"`
	Download   int64 `json:"download"`
	Connections int  `json:"connections"`   // 当前连接数
	TotalConns  int64 `json:"total_conns"`  // 累计连接数

	// 其他
	Remark     string    `gorm:"size:500" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardRule) TableName() string {
	return "v2_forward_rule"
}

// ForwardRoute 智能路由规则
type ForwardRoute struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:100;not null" json:"name"`
	Priority int    `gorm:"default:0" json:"priority"` // 优先级，数字越大越优先
	Enabled  bool   `gorm:"default:true" json:"enabled"`

	// 匹配条件 (所有条件为AND关系)
	SourceIP  string `gorm:"size:100" json:"source_ip"`   // 源IP/网段，支持CIDR，多个用逗号分隔
	Region    string `gorm:"size:50" json:"region"`       // 来源地区
	PortRange string `gorm:"size:50" json:"port_range"`   // 端口范围: 80-443,8080
	Protocol  string `gorm:"size:20" json:"protocol"`     // 协议: tcp/udp/both

	// 目标节点
	RelayGroup string `gorm:"size:50" json:"relay_group"` // 中转节点组名
	ExitGroup  string `gorm:"size:50" json:"exit_group"`  // 落地节点组名
	RelayNodeID *uint `json:"relay_node_id"`              // 或指定特定节点
	ExitNodeID  *uint `json:"exit_node_id"`

	// 负载均衡
	BalanceMode string `gorm:"size:20;default:'round-robin'" json:"balance_mode"`
	// round-robin: 轮询
	// least-conn: 最少连接
	// latency: 最低延迟
	// weight: 加权轮询
	// random: 随机

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardRoute) TableName() string {
	return "v2_forward_route"
}

// ForwardLog 转发日志
type ForwardLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	RuleID     uint      `gorm:"index" json:"rule_id"`
	UserID     *uint     `gorm:"index" json:"user_id"`

	// 连接信息
	SourceIP   string    `gorm:"size:50" json:"source_ip"`
	SourcePort int       `json:"source_port"`
	TargetHost string    `gorm:"size:255" json:"target_host"`
	TargetPort int       `json:"target_port"`

	// 流量统计
	Upload     int64     `json:"upload"`
	Download   int64     `json:"download"`
	Duration   int64     `json:"duration"` // 持续时间(秒)

	// 节点信息
	RelayNodeID uint `json:"relay_node_id"`
	ExitNodeID  uint `json:"exit_node_id"`

	// 状态
	Status     string    `gorm:"size:20" json:"status"` // success/timeout/error
	Error      string    `gorm:"size:500" json:"error"`

	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (ForwardLog) TableName() string {
	return "v2_forward_log"
}

// ForwardStats 转发统计 (按小时聚合)
type ForwardStats struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	NodeID       uint      `gorm:"index" json:"node_id"`
	RuleID       uint      `gorm:"index" json:"rule_id"`
	UserID       *uint     `gorm:"index" json:"user_id"`

	// 时间
	Date         time.Time `gorm:"type:date" json:"date"`    // 日期
	Hour         int       `json:"hour"`                     // 小时 0-23

	// 流量
	Upload       int64     `json:"upload"`
	Download     int64     `json:"download"`
	Connections  int64     `json:"connections"`

	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardStats) TableName() string {
	return "v2_forward_stats"
}