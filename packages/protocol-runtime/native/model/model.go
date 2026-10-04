// Package model mirrors the kernel model types the protocol routes bind
// request bodies to, write and answer with: the same type names, fields and
// tags, in a package of the same name. Answers are then the same bytes, and
// so are JSON binding errors, which name a field's type with its package
// ("model.ProtocolType").
//
// NodeProtocol is the kernel's v2_node_protocol, which the package adopts
// once the node credential split finalized it (its secret positions hold
// the placeholder; the secrets are the kernel's). Its gorm tags are the
// kernel model's, so the package writes what the kernel would write
// (column defaults included) and resolves update keys as the kernel does.
// Node, SubscriptionGroup and SubscriptionTemplate only give a body's
// nested "node" and "subscription_groups" the shape the kernel decodes them
// to; neither side saves a body's associations.
package model

import "time"

// ProtocolType is the kernel's node protocol type.
type ProtocolType string

// NodeStatus is the kernel's node status.
type NodeStatus int

// NodeProtocol is a v2_node_protocol row.
type NodeProtocol struct {
	ID      uint         `gorm:"primaryKey" json:"id"`
	NodeID  uint         `gorm:"index" json:"node_id"`
	Name    string       `gorm:"size:100" json:"name"`
	Type    ProtocolType `gorm:"size:20;index" json:"type"`
	Port    int          `json:"port"`
	Enable  int          `gorm:"default:1" json:"enable"`
	Show    int          `gorm:"default:1" json:"show"`
	Sort    int          `gorm:"default:0" json:"sort"`
	GroupID *uint        `gorm:"index" json:"group_id"`

	Host *string `gorm:"size:255" json:"host"`
	TLS  int     `gorm:"default:0" json:"tls"`
	ALPN *string `gorm:"size:100" json:"alpn"`

	Settings          *string `gorm:"type:text" json:"settings"`
	TLSSettings       *string `gorm:"type:text" json:"tls_settings"`
	Transport         *string `gorm:"size:20" json:"transport"`
	TransportSettings *string `gorm:"type:text" json:"transport_settings"`

	RealitySettings *string `gorm:"type:text" json:"reality_settings"`

	CustomConfig *string `gorm:"type:text" json:"custom_config"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Node               *Node               `gorm:"foreignKey:NodeID" json:"node,omitempty"`
	SubscriptionGroups []SubscriptionGroup `gorm:"many2many:v2_subscription_group_node_protocols;" json:"subscription_groups,omitempty"`
}

// TableName is the adopted kernel table.
func (NodeProtocol) TableName() string { return "v2_node_protocol" }

// SecretColumns are the columns of a node protocol that can hold secrets,
// in the order the package stores them: the settings first, which the
// kernel validates with the typed secrets.
var SecretColumns = []string{"settings", "tls_settings", "transport_settings", "reality_settings", "custom_config"}

// Column returns the address of a secret column's value.
func (p *NodeProtocol) Column(name string) **string {
	switch name {
	case "settings":
		return &p.Settings
	case "tls_settings":
		return &p.TLSSettings
	case "transport_settings":
		return &p.TransportSettings
	case "reality_settings":
		return &p.RealitySettings
	case "custom_config":
		return &p.CustomConfig
	}
	return nil
}

// Node is the shape the kernel decodes a nested "node" to. It is never
// stored or loaded here; the kernel model's API key and secrets are not
// decoded (json:"-").
type Node struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
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

	Protocols []NodeProtocol `gorm:"foreignKey:NodeID" json:"protocols,omitempty"`
}

// TableName names the kernel table the shape is the kernel's of; the
// package never reads or writes it.
func (Node) TableName() string { return "v2_node" }

// SubscriptionGroup is the shape the kernel decodes a nested subscription
// group to.
type SubscriptionGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;uniqueIndex" json:"name"`
	Description *string   `gorm:"size:500" json:"description"`
	Priority    int       `gorm:"default:0" json:"priority"`
	Enable      int       `gorm:"default:1" json:"enable"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Templates []SubscriptionTemplate `gorm:"foreignKey:GroupID" json:"templates,omitempty"`
	Protocols []NodeProtocol         `gorm:"many2many:v2_subscription_group_node_protocols;" json:"protocols,omitempty"`
}

// TableName names the kernel table.
func (SubscriptionGroup) TableName() string { return "v2_subscription_group" }

// SubscriptionTemplate is the shape the kernel decodes a nested
// subscription template to.
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

	Group *SubscriptionGroup `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

// TableName names the kernel table.
func (SubscriptionTemplate) TableName() string { return "v2_subscription_template" }
