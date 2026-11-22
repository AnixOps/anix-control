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
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:100;not null" json:"name"`
	Type        string `gorm:"size:20;not null;default:'exit'" json:"type"` // relay/exit
	Host        string `gorm:"size:255;not null" json:"host"`
	Port        int    `gorm:"not null" json:"port"`
	APIPort     int    `json:"api_port"`     // 管理API端口
	MetricsPort int    `json:"metrics_port"` // Prometheus /metrics 端口，0 表示未配置

	// 节点信息
	Region     string `gorm:"size:50" json:"region"`      // 地区: HK, US, JP, SG等
	ISP        string `gorm:"size:50" json:"isp"`         // 运营商
	Datacenter string `gorm:"size:100" json:"datacenter"` // 数据中心
	Bandwidth  int64  `json:"bandwidth"`                  // 带宽(Mbps)

	// 状态
	Status    int       `gorm:"default:0" json:"status"`
	LastCheck time.Time `json:"last_check"`
	Latency   int       `json:"latency"` // 延迟(ms)
	Load      float64   `json:"load"`    // 负载 0-1
	Uptime    float64   `json:"uptime"`  // 在线率 0-100

	// 配置
	Tags    string `gorm:"type:text" json:"tags"`   // JSON数组
	Weight  int    `gorm:"default:1" json:"weight"` // 负载均衡权重
	MaxConn int    `json:"max_conn"`                // 最大连接数
	Enabled bool   `gorm:"default:true" json:"enabled"`

	// Node groups (internal/nodegroup), as on v5_node: the group whose join
	// token created the node, the machine fingerprint it joined with, its
	// approval (NodeApproval*) and the public address it joined from.
	// Adoption (internal/database/migrations) adds them to tables an older
	// release created; the frozen v2 forward answers do not show them.
	NodeGroupID        *uint  `gorm:"index" json:"-"`
	MachineFingerprint string `gorm:"size:64;index" json:"-"`
	Approval           string `gorm:"size:16;not null;default:'approved'" json:"-"`
	LastPublicAddress  string `gorm:"size:64" json:"-"`

	// 统计
	TotalUpload   int64 `json:"total_upload"`
	TotalDownload int64 `json:"total_download"`
	CurrentConn   int   `json:"current_conn"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardNode) TableName() string {
	return "v5_forward_node"
}
