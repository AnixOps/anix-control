// Package model mirrors the kernel model types the node routes answer
// with: the same type names, fields and JSON tags, in a package of the
// same name, so the answers are the same bytes.
//
// Node is the kernel's v2_node, which the package adopts once the node
// credential split finalized it. It leaves out the node's API key, key
// hash and shared secret: their columns hold tombstones then, and the
// credentials are the kernel's (v4_kernel_node_credential); the kernel
// model never answers them either (json:"-"). Its raw configuration holds
// the placeholder at every secret position. NodeProtocol reads a node's
// protocols through the kernel view kapi_node_protocol_public_v1: the
// protocols are protocol-runtime's.
package model

import "time"

// NodeStatus is the kernel's node status.
type NodeStatus int

// The kernel's node statuses (model.NodeStatus*).
const (
	NodeStatusPending  NodeStatus = 0
	NodeStatusOnline   NodeStatus = 1
	NodeStatusOffline  NodeStatus = 2
	NodeStatusDisabled NodeStatus = 3
)

// ProtocolType is the kernel's node protocol type.
type ProtocolType string

// Node is a v2_node row without its credentials.
type Node struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:255" json:"name"`
	Host         string     `gorm:"size:255" json:"host"`
	Port         int        `gorm:"default:443" json:"port"`
	Status       NodeStatus `gorm:"default:0" json:"status"`
	Tags         *string    `gorm:"size:255" json:"tags"`
	GroupID      *uint      `gorm:"index" json:"group_id"`
	Rate         float64    `gorm:"default:1" json:"rate"`
	TrafficRate  float64    `gorm:"default:1" json:"traffic_rate"`
	Sort         int        `gorm:"default:0" json:"sort"`
	Show         int        `gorm:"default:1" json:"show"`
	AutoRegister int        `gorm:"default:0" json:"auto_register"`

	ParentID *uint `gorm:"index" json:"parent_id"`

	MonthlyLimit    *int64 `json:"monthly_limit"`
	MonthlyUpload   int64  `gorm:"default:0" json:"monthly_upload"`
	MonthlyDownload int64  `gorm:"default:0" json:"monthly_download"`
	MonthlyResetDay int    `gorm:"default:1" json:"monthly_reset_day"`

	RawConfig *string `gorm:"type:text" json:"raw_config"`

	ServerIP      *string `gorm:"size:45" json:"server_ip"`
	ServerVersion *string `gorm:"size:50" json:"server_version"`
	ServerOS      *string `gorm:"size:100" json:"server_os"`
	CPUUsage      float64 `gorm:"default:0" json:"cpu_usage"`
	MemoryUsage   float64 `gorm:"default:0" json:"memory_usage"`
	DiskUsage     float64 `gorm:"default:0" json:"disk_usage"`
	Uptime        int64   `gorm:"default:0" json:"uptime"`
	OnlineUsers   int     `gorm:"default:0" json:"online_users"`

	RuntimeHealthy   bool   `gorm:"default:true" json:"runtime_healthy"`
	RuntimeError     string `gorm:"type:text" json:"runtime_error"`
	RuntimeCheckedAt *int64 `json:"runtime_checked_at"`

	TotalUpload   int64 `gorm:"default:0" json:"total_upload"`
	TotalDownload int64 `gorm:"default:0" json:"total_download"`

	LastCheckAt *int64    `json:"last_check_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Protocols []NodeProtocol `gorm:"foreignKey:NodeID" json:"protocols,omitempty"`
}

// TableName is the adopted kernel table.
func (Node) TableName() string { return "v2_node" }

// IsOnline is the kernel's: the node checked in within five minutes.
func (n *Node) IsOnline(now time.Time) bool {
	return n.LastCheckAt != nil && now.Unix()-*n.LastCheckAt < 300
}

// NodeProtocol is a row of kapi_node_protocol_public_v1: a node protocol,
// its settings as a finalized v2_node_protocol keeps them.
type NodeProtocol struct {
	ID      uint         `gorm:"primaryKey" json:"id"`
	NodeID  uint         `json:"node_id"`
	Name    string       `json:"name"`
	Type    ProtocolType `json:"type"`
	Port    int          `json:"port"`
	Enable  int          `json:"enable"`
	Show    int          `json:"show"`
	Sort    int          `json:"sort"`
	GroupID *uint        `json:"group_id"`

	Host *string `json:"host"`
	TLS  int     `json:"tls"`
	ALPN *string `json:"alpn"`

	Settings          *string `json:"settings"`
	TLSSettings       *string `json:"tls_settings"`
	Transport         *string `json:"transport"`
	TransportSettings *string `json:"transport_settings"`

	RealitySettings *string `json:"reality_settings"`

	CustomConfig *string `json:"custom_config"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Node and SubscriptionGroups are never loaded here; the kernel's
	// answers leave them out too (omitempty).
	Node               *Node `gorm:"-" json:"node,omitempty"`
	SubscriptionGroups []any `gorm:"-" json:"subscription_groups,omitempty"`
}

// TableName is the kernel view.
func (NodeProtocol) TableName() string { return "kapi_node_protocol_public_v1" }

// AuthorizedKey is a row of kapi_registration_key_v1 answered as the
// kernel's model.AuthorizedKey: the key itself is the kernel's, so the
// answer shows the placeholder where the key exists, as the kernel's
// masked answer does.
type AuthorizedKey struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Used      int       `json:"used"`
	ExpireAt  *int64    `json:"expire_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
