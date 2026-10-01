package native

import "time"

// The rows below have the kernel models' columns, tags and column defaults,
// so the package writes what the kernel would and reads what it reads.

// Forward is a v2_forward row.
type Forward struct {
	ID                uint    `gorm:"primaryKey"`
	UserID            uint    `gorm:"index;not null"`
	UserName          string  `gorm:"size:255"`
	Name              string  `gorm:"size:100;not null"`
	TunnelID          uint    `gorm:"index;not null"`
	Tunnel            *Tunnel `gorm:"foreignKey:TunnelID"`
	InPort            int     `gorm:"not null"`
	OutPort           int
	RemoteAddr        string `gorm:"type:text;not null"`
	InterfaceName     string `gorm:"size:255"`
	Strategy          string `gorm:"size:20;default:'fifo'"`
	Status            int    `gorm:"default:1"`
	RuntimeBackend    string `gorm:"size:50;default:'gost';index"`
	RuntimeStatus     int    `gorm:"default:0;index"`
	RuntimeMessage    string `gorm:"type:text"`
	RuntimeLastSyncAt *time.Time
	InFlow            int64 `gorm:"default:0"`
	OutFlow           int64 `gorm:"default:0"`
	Inx               int   `gorm:"default:0"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TableName is the adopted kernel table.
func (Forward) TableName() string { return "v2_forward" }

// Tunnel is a v2_forward_tunnel row.
type Tunnel struct {
	ID            uint   `gorm:"primaryKey"`
	Name          string `gorm:"size:100;not null"`
	InNodeID      uint   `gorm:"index"`
	OutNodeID     *uint  `gorm:"index"`
	InIP          string `gorm:"size:255"`
	OutIP         string `gorm:"size:255"`
	InNodePortSta *int
	InNodePortEnd *int
	Type          int     `gorm:"default:1"`
	Flow          int     `gorm:"default:2"`
	Protocol      string  `gorm:"size:30;default:'tcp'"`
	TrafficRatio  float64 `gorm:"default:1"`
	InterfaceName string  `gorm:"size:255"`
	TCPListenAddr string  `gorm:"size:255;default:'0.0.0.0'"`
	UDPListenAddr string  `gorm:"size:255;default:'0.0.0.0'"`
	Status        int     `gorm:"default:1"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TableName is the adopted kernel table.
func (Tunnel) TableName() string { return "v2_forward_tunnel" }

// UserTunnel is a v2_forward_user_tunnel row: a user's permission to use a
// tunnel, with its quota and traffic.
type UserTunnel struct {
	ID            uint  `gorm:"primaryKey"`
	UserID        uint  `gorm:"uniqueIndex:idx_forward_user_tunnel"`
	TunnelID      uint  `gorm:"uniqueIndex:idx_forward_user_tunnel;index"`
	Flow          int64 `gorm:"default:0"`
	Num           int   `gorm:"default:0"`
	InFlow        int64 `gorm:"default:0"`
	OutFlow       int64 `gorm:"default:0"`
	FlowResetTime int64 `gorm:"default:0"`
	ExpTime       int64 `gorm:"default:0"`
	SpeedID       *uint
	Status        int `gorm:"default:1"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Tunnel *Tunnel `gorm:"foreignKey:TunnelID"`
}

// TableName is the adopted kernel table.
func (UserTunnel) TableName() string { return "v2_forward_user_tunnel" }

// SpeedLimit is a v2_speed_limit row: a tunnel's named speed limit. The
// plan package's /speed-limit routes, bridged, manage them; forward only
// reads them.
type SpeedLimit struct {
	ID          uint   `gorm:"primaryKey"`
	CreatedTime int64  `gorm:"index;not null"`
	UpdatedTime int64  `gorm:"index;not null"`
	Status      int    `gorm:"default:1"`
	Name        string `gorm:"size:100;not null"`
	Speed       int64  `gorm:"not null"`
	TunnelID    uint   `gorm:"index;not null"`
	TunnelName  string `gorm:"size:100;not null"`
}

// TableName is the adopted kernel table.
func (SpeedLimit) TableName() string { return "v2_speed_limit" }

// LatencyBucket is a v2_forward_latency_bucket row, written by the kernel's
// latency prober.
type LatencyBucket struct {
	ID              uint `gorm:"primaryKey"`
	TargetKey       string
	TargetType      string
	TargetID        uint
	Label           string
	Host            string
	Port            int
	BucketAt        time.Time
	IntervalSeconds int
	SampleCount     int
	SuccessCount    int
	MinRTT          float64
	AvgRTT          float64
	MaxRTT          float64
	P95RTT          float64
	LossPct         float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName is the adopted kernel table.
func (LatencyBucket) TableName() string { return "v2_forward_latency_bucket" }

// Rule is a v2_forward_rule row, as the legacy answers show it: with its
// relay and exit nodes when they are loaded, and never its user.
type Rule struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:100;not null" json:"name"`
	Enabled      bool       `gorm:"default:true" json:"enabled"`
	RelayNodeID  uint       `gorm:"not null" json:"relay_node_id"`
	RelayNode    *Node      `gorm:"foreignKey:RelayNodeID" json:"relay_node,omitempty"`
	ListenPort   int        `gorm:"not null" json:"listen_port"`
	Protocol     string     `gorm:"size:10;default:'tcp'" json:"protocol"`
	ExitNodeID   uint       `gorm:"not null" json:"exit_node_id"`
	ExitNode     *Node      `gorm:"foreignKey:ExitNodeID" json:"exit_node,omitempty"`
	TargetHost   string     `gorm:"size:255;not null" json:"target_host"`
	TargetPort   int        `gorm:"not null" json:"target_port"`
	UserID       *uint      `json:"user_id"`
	UserGroupID  *uint      `json:"user_group_id"`
	AllowedIPs   string     `gorm:"size:1000" json:"allowed_ips"`
	SpeedLimit   *int64     `json:"speed_limit"`
	TrafficLimit *int64     `json:"traffic_limit"`
	ExpireTime   *time.Time `json:"expire_time"`
	Upload       int64      `json:"upload"`
	Download     int64      `json:"download"`
	Connections  int        `json:"connections"`
	TotalConns   int64      `json:"total_conns"`
	Remark       string     `gorm:"size:500" json:"remark"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Rule) TableName() string { return "v2_forward_rule" }

// Node is a row of kapi_forward_node_v1: a forward node without its API
// token, which authenticates the node's agent and stays in the protected
// v2_forward_node. Its JSON is the kernel model's.
type Node struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	APIPort int    `json:"api_port"`
	// APIToken is never read; answers to users show it empty.
	APIToken      string    `gorm:"-" json:"api_token"`
	MetricsPort   int       `json:"metrics_port"`
	Region        string    `json:"region"`
	ISP           string    `json:"isp"`
	Datacenter    string    `json:"datacenter"`
	Bandwidth     int64     `json:"bandwidth"`
	Status        int       `json:"status"`
	LastCheck     time.Time `json:"last_check"`
	Latency       int       `json:"latency"`
	Load          float64   `json:"load"`
	Uptime        float64   `json:"uptime"`
	Tags          string    `json:"tags"`
	Weight        int       `json:"weight"`
	MaxConn       int       `json:"max_conn"`
	Enabled       bool      `json:"enabled"`
	TotalUpload   int64     `json:"total_upload"`
	TotalDownload int64     `json:"total_download"`
	CurrentConn   int       `json:"current_conn"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName is the kernel view.
func (Node) TableName() string { return "kapi_forward_node_v1" }

// RuntimeSetting is a row of kapi_forward_runtime_settings_v1: one of the
// system configuration keys that choose the forward runtime backend.
type RuntimeSetting struct {
	Key   string
	Value string
}

// TableName is the kernel view.
func (RuntimeSetting) TableName() string { return "kapi_forward_runtime_settings_v1" }

// DirectoryUser is the part of a kapi_user_directory_v1 row the routes
// read: that the user exists.
type DirectoryUser struct {
	ID uint `gorm:"primaryKey"`
}

// TableName is the kernel view.
func (DirectoryUser) TableName() string { return "kapi_user_directory_v1" }

// Entitlement is the part of a kapi_subscriber_entitlement_v1 row the
// routes read: the subscriber's speed limit.
type Entitlement struct {
	ID         uint `gorm:"primaryKey"`
	SpeedLimit *int64
}

// TableName is the kernel view.
func (Entitlement) TableName() string { return "kapi_subscriber_entitlement_v1" }

// Node and forward statuses and kinds, as the kernel model defines them.
const (
	nodeTypeRelay = "relay"
	nodeTypeExit  = "exit"
	nodeOnline    = 1

	tunnelActive = 1

	userTunnelActive = 1

	latencyTargetTunnelNode = "tunnel_node"

	backendGost             = "gost"
	backendNftablesAnsible  = "nftables_ansible"
	backendIptablesAnsible  = "iptables_ansible"
	backendCleanAgent       = "clean_agent"
	defaultLocalAnsibleMode = backendNftablesAnsible
)
