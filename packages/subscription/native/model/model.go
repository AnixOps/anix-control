// Package model mirrors the kernel model types the subscription routes bind
// request bodies to and answer with: the same type names, fields and tags,
// in a package of the same name. Answers are then the same bytes, and so are
// JSON binding errors, which name a field's type with its package
// ("[]model.SubscriptionTemplate", "model.ProtocolType").
//
// SubscriptionGroup, SubscriptionTemplate, PlanSubscriptionGroup and
// GroupProtocol are the kernel's tables, adopted in place. NodeProtocol and
// Node are the proxy-node package's: they only give a body's nested
// "protocols" and "node" the shape the kernel decodes them to. The kernel
// does not save a body's associations, and neither does the package
// (packages/subscription/native).
package model

import "time"

// SubscriptionGroup is a v2_subscription_group row.
type SubscriptionGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;uniqueIndex" json:"name"`
	Description *string   `gorm:"size:500" json:"description"`
	Priority    int       `gorm:"default:0" json:"priority"`
	Enable      int       `gorm:"default:1" json:"enable"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Templates []SubscriptionTemplate `gorm:"foreignKey:GroupID" json:"templates,omitempty"`
	// Protocols is decoded from a body and never stored or loaded here:
	// linking protocols is its own route, on GroupProtocol.
	Protocols []NodeProtocol `gorm:"-" json:"protocols,omitempty"`
}

// TableName is the adopted kernel table.
func (SubscriptionGroup) TableName() string { return "v2_subscription_group" }

// SubscriptionTemplate is a v2_subscription_template row.
type SubscriptionTemplate struct {
	ID      uint    `gorm:"primaryKey" json:"id"`
	GroupID uint    `gorm:"index" json:"group_id"`
	Name    string  `gorm:"size:255" json:"name"`
	Type    string  `gorm:"size:30" json:"type"`
	Enable  int     `gorm:"default:1" json:"enable"`
	Sort    int     `gorm:"default:0" json:"sort"`
	Tags    *string `gorm:"size:255" json:"tags"`

	TemplateJSON string `gorm:"type:text" json:"template_json"`

	Server     string  `gorm:"size:255" json:"server"`
	Port       int     `json:"port"`
	ServerName *string `gorm:"size:255" json:"server_name"`

	TLS            int     `gorm:"default:0" json:"tls"`
	TLSFingerprint *string `gorm:"size:50" json:"tls_fingerprint"`
	ALPN           *string `gorm:"size:100" json:"alpn"`

	RealityPublicKey *string `gorm:"size:100" json:"reality_public_key"`
	RealityShortID   *string `gorm:"size:50" json:"reality_short_id"`
	RealitySpiderX   *string `gorm:"size:255" json:"reality_spider_x"`
	RealityDest      *string `gorm:"size:255" json:"reality_dest"`

	Transport         string  `gorm:"size:20;default:tcp" json:"transport"`
	TransportSettings *string `gorm:"type:text" json:"transport_settings,omitempty"`

	Flow               *string `gorm:"size:50" json:"flow"`
	Encryption         *string `gorm:"size:50" json:"encryption"`
	EncryptionSettings *string `gorm:"type:text" json:"encryption_settings"`

	SSCipher    *string `gorm:"size:50" json:"ss_cipher"`
	SSServerKey *string `gorm:"size:100" json:"ss_server_key"`

	ProtocolSettings *string `gorm:"type:text" json:"protocol_settings"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Group is a relation, as in the kernel model, so a partial update names
	// it as GORM does there; it is never saved.
	Group *SubscriptionGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

// TableName is the adopted kernel table.
func (SubscriptionTemplate) TableName() string { return "v2_subscription_template" }

// PlanSubscriptionGroup is a v2_plan_subscription_group row: a subscription
// group a plan grants.
type PlanSubscriptionGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PlanID    uint      `gorm:"uniqueIndex:idx_plan_group" json:"plan_id"`
	GroupID   uint      `gorm:"uniqueIndex:idx_plan_group" json:"group_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName is the adopted kernel table.
func (PlanSubscriptionGroup) TableName() string { return "v2_plan_subscription_group" }

// GroupProtocol is a v2_subscription_group_node_protocols row, the kernel's
// many-to-many join of subscription groups and node protocols.
type GroupProtocol struct {
	SubscriptionGroupID uint `gorm:"primaryKey"`
	NodeProtocolID      uint `gorm:"primaryKey"`
}

// TableName is the adopted kernel table.
func (GroupProtocol) TableName() string { return "v2_subscription_group_node_protocols" }

// ProtocolType is the kernel's node protocol type.
type ProtocolType string

// NodeStatus is the kernel's node status.
type NodeStatus int

// NodeProtocol is the shape the kernel decodes a group body's "protocols"
// to. It is never stored.
type NodeProtocol struct {
	ID      uint         `json:"id"`
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

	Node               *Node               `json:"node,omitempty"`
	SubscriptionGroups []SubscriptionGroup `json:"subscription_groups,omitempty"`
}

// Node is the shape the kernel decodes a nested "node" to. It is never
// stored. The kernel model's API key and secrets are not decoded (json:"-").
type Node struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Status       NodeStatus `json:"status"`
	Tags         *string    `json:"tags"`
	GroupID      *uint      `json:"group_id"`
	Rate         float64    `json:"rate"`
	TrafficRate  float64    `json:"traffic_rate"`
	Sort         int        `json:"sort"`
	Show         int        `json:"show"`
	AutoRegister int        `json:"auto_register"`

	ParentID *uint `json:"parent_id"`

	MonthlyLimit    *int64 `json:"monthly_limit"`
	MonthlyUpload   int64  `json:"monthly_upload"`
	MonthlyDownload int64  `json:"monthly_download"`
	MonthlyResetDay int    `json:"monthly_reset_day"`

	RawConfig *string `json:"raw_config"`

	ServerIP      *string `json:"server_ip"`
	ServerVersion *string `json:"server_version"`
	ServerOS      *string `json:"server_os"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
	DiskUsage     float64 `json:"disk_usage"`
	Uptime        int64   `json:"uptime"`
	OnlineUsers   int     `json:"online_users"`

	RuntimeHealthy   bool   `json:"runtime_healthy"`
	RuntimeError     string `json:"runtime_error"`
	RuntimeCheckedAt *int64 `json:"runtime_checked_at"`

	TotalUpload   int64 `json:"total_upload"`
	TotalDownload int64 `json:"total_download"`

	LastCheckAt *int64    `json:"last_check_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Protocols []NodeProtocol `json:"protocols,omitempty"`
}
