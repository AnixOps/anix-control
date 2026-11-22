package migrations_test

import (
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// The flux forwarding models as the legacy start migrated them (they were
// internal/model's until the 2026-11 slimming); 0028_drop_flux_tables
// dropped their tables. legacyFluxPrepare builds them for the oracle.

type retiredForwardRule struct {
	ID           uint               `gorm:"primaryKey" json:"id"`
	Name         string             `gorm:"size:100;not null" json:"name"`
	Enabled      bool               `gorm:"default:true" json:"enabled"`
	RelayNodeID  uint               `gorm:"not null" json:"relay_node_id"`
	RelayNode    *model.ForwardNode `gorm:"foreignKey:RelayNodeID" json:"relay_node,omitempty"`
	ListenPort   int                `gorm:"not null" json:"listen_port"`
	Protocol     string             `gorm:"size:10;default:'tcp'" json:"protocol"`
	ExitNodeID   uint               `gorm:"not null" json:"exit_node_id"`
	ExitNode     *model.ForwardNode `gorm:"foreignKey:ExitNodeID" json:"exit_node,omitempty"`
	TargetHost   string             `gorm:"size:255;not null" json:"target_host"`
	TargetPort   int                `gorm:"not null" json:"target_port"`
	UserID       *uint              `json:"user_id"`
	User         *model.User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	UserGroupID  *uint              `json:"user_group_id"`
	AllowedIPs   string             `gorm:"size:1000" json:"allowed_ips"`
	SpeedLimit   *int64             `json:"speed_limit"`
	TrafficLimit *int64             `json:"traffic_limit"`
	ExpireTime   *time.Time         `json:"expire_time"`
	Upload       int64              `json:"upload"`
	Download     int64              `json:"download"`
	Connections  int                `json:"connections"`
	TotalConns   int64              `json:"total_conns"`
	Remark       string             `gorm:"size:500" json:"remark"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func (retiredForwardRule) TableName() string { return "v2_forward_rule" }

type retiredForwardRoute struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:100;not null" json:"name"`
	Priority    int    `gorm:"default:0" json:"priority"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	SourceIP    string `gorm:"size:100" json:"source_ip"`
	Region      string `gorm:"size:50" json:"region"`
	PortRange   string `gorm:"size:50" json:"port_range"`
	Protocol    string `gorm:"size:20" json:"protocol"`
	RelayGroup  string `gorm:"size:50" json:"relay_group"`
	ExitGroup   string `gorm:"size:50" json:"exit_group"`
	RelayNodeID *uint  `json:"relay_node_id"`
	ExitNodeID  *uint  `json:"exit_node_id"`
	BalanceMode string `gorm:"size:20;default:'round-robin'" json:"balance_mode"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (retiredForwardRoute) TableName() string { return "v2_forward_route" }

type retiredForwardLog struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	RuleID      uint   `gorm:"index" json:"rule_id"`
	UserID      *uint  `gorm:"index" json:"user_id"`
	SourceIP    string `gorm:"size:50" json:"source_ip"`
	SourcePort  int    `json:"source_port"`
	TargetHost  string `gorm:"size:255" json:"target_host"`
	TargetPort  int    `json:"target_port"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Duration    int64  `json:"duration"`
	RelayNodeID uint   `json:"relay_node_id"`
	ExitNodeID  uint   `json:"exit_node_id"`
	Status      string `gorm:"size:20" json:"status"`
	Error       string `gorm:"size:500" json:"error"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (retiredForwardLog) TableName() string { return "v2_forward_log" }

type retiredForwardStats struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	NodeID      uint      `gorm:"index" json:"node_id"`
	RuleID      uint      `gorm:"index" json:"rule_id"`
	UserID      *uint     `gorm:"index" json:"user_id"`
	Date        time.Time `gorm:"type:date" json:"date"`
	Hour        int       `json:"hour"`
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Connections int64     `json:"connections"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (retiredForwardStats) TableName() string { return "v2_forward_stats" }

type retiredForwardTunnel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:100;not null" json:"name"`
	InNodeID      uint      `gorm:"index" json:"inNodeId"`
	OutNodeID     *uint     `gorm:"index" json:"outNodeId"`
	InIP          string    `gorm:"size:255" json:"in_ip"`
	OutIP         string    `gorm:"size:255" json:"out_ip"`
	InNodePortSta *int      `json:"in_node_port_sta"`
	InNodePortEnd *int      `json:"in_node_port_end"`
	Type          int       `gorm:"default:1" json:"type"`
	Flow          int       `gorm:"default:2" json:"flow"`
	Protocol      string    `gorm:"size:30;default:'tcp'" json:"protocol"`
	TrafficRatio  float64   `gorm:"default:1" json:"traffic_ratio"`
	InterfaceName string    `gorm:"size:255" json:"interface_name"`
	TCPListenAddr string    `gorm:"size:255;default:'0.0.0.0'" json:"tcpListenAddr"`
	UDPListenAddr string    `gorm:"size:255;default:'0.0.0.0'" json:"udpListenAddr"`
	Status        int       `gorm:"default:1" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (retiredForwardTunnel) TableName() string { return "v2_forward_tunnel" }

type retiredForwardUserTunnel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"uniqueIndex:idx_forward_user_tunnel" json:"user_id"`
	TunnelID      uint      `gorm:"uniqueIndex:idx_forward_user_tunnel;index" json:"tunnel_id"`
	Flow          int64     `gorm:"default:0" json:"flow"`
	Num           int       `gorm:"default:0" json:"num"`
	InFlow        int64     `gorm:"default:0" json:"in_flow"`
	OutFlow       int64     `gorm:"default:0" json:"out_flow"`
	FlowResetTime int64     `gorm:"default:0" json:"flowResetTime"`
	ExpTime       int64     `gorm:"default:0" json:"expTime"`
	SpeedID       *uint     `json:"speedId"`
	Status        int       `gorm:"default:1" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	User   *model.User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Tunnel *retiredForwardTunnel `gorm:"foreignKey:TunnelID" json:"tunnel,omitempty"`
}

func (retiredForwardUserTunnel) TableName() string { return "v2_forward_user_tunnel" }

type retiredForward struct {
	ID                uint                  `gorm:"primaryKey" json:"id"`
	UserID            uint                  `gorm:"index;not null" json:"user_id"`
	UserName          string                `gorm:"size:255" json:"user_name"`
	Name              string                `gorm:"size:100;not null" json:"name"`
	TunnelID          uint                  `gorm:"index;not null" json:"tunnel_id"`
	Tunnel            *retiredForwardTunnel `gorm:"foreignKey:TunnelID" json:"tunnel,omitempty"`
	InPort            int                   `gorm:"not null" json:"in_port"`
	OutPort           int                   `json:"out_port"`
	RemoteAddr        string                `gorm:"type:text;not null" json:"remote_addr"`
	InterfaceName     string                `gorm:"size:255" json:"interface_name"`
	Strategy          string                `gorm:"size:20;default:'fifo'" json:"strategy"`
	Status            int                   `gorm:"default:1" json:"status"`
	RuntimeBackend    string                `gorm:"size:50;default:'gost';index" json:"runtimeBackend"`
	RuntimeStatus     int                   `gorm:"default:0;index" json:"runtimeStatus"`
	RuntimeMessage    string                `gorm:"type:text" json:"runtimeMessage"`
	RuntimeLastSyncAt *time.Time            `json:"runtimeLastSyncAt"`
	InFlow            int64                 `gorm:"default:0" json:"in_flow"`
	OutFlow           int64                 `gorm:"default:0" json:"out_flow"`
	Inx               int                   `gorm:"default:0" json:"inx"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	User              *model.User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (retiredForward) TableName() string { return "v2_forward" }

type retiredForwardPortBinding struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ForwardID  uint      `gorm:"index;not null" json:"forwardId"`
	NodeID     uint      `gorm:"not null;uniqueIndex:idx_forward_port_binding_socket,priority:1" json:"nodeId"`
	Transport  string    `gorm:"size:10;not null;uniqueIndex:idx_forward_port_binding_socket,priority:2" json:"transport"`
	ListenAddr string    `gorm:"size:255;not null;uniqueIndex:idx_forward_port_binding_socket,priority:3" json:"listenAddr"`
	InPort     int       `gorm:"not null;uniqueIndex:idx_forward_port_binding_socket,priority:4" json:"inPort"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`

	Forward *retiredForward `gorm:"foreignKey:ForwardID" json:"forward,omitempty"`
}

func (retiredForwardPortBinding) TableName() string { return "v2_forward_port_binding" }

type retiredSpeedLimit struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	CreatedTime int64  `gorm:"index;not null" json:"createdTime"`
	UpdatedTime int64  `gorm:"index;not null" json:"updatedTime"`
	Status      int    `gorm:"default:1" json:"status"`
	Name        string `gorm:"size:100;not null" json:"name"`
	Speed       int64  `gorm:"not null" json:"speed"`
	TunnelID    uint   `gorm:"index;not null" json:"tunnelId"`
	TunnelName  string `gorm:"size:100;not null" json:"tunnelName"`
}

func (retiredSpeedLimit) TableName() string { return "v2_speed_limit" }

type retiredForwardAgentBridgeTask struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TaskID       string    `gorm:"size:80;uniqueIndex" json:"taskId"`
	RuntimeJobID uint      `gorm:"uniqueIndex" json:"runtimeJobId"`
	NodeID       uint      `gorm:"index" json:"nodeId"`
	ForwardID    *uint     `gorm:"index" json:"forwardId"`
	Action       string    `gorm:"size:30" json:"action"`
	Type         string    `gorm:"size:30" json:"type"`
	Params       string    `gorm:"type:text" json:"params"`
	Status       string    `gorm:"size:20;index" json:"status"`
	Dispatched   bool      `gorm:"default:false" json:"dispatched"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (retiredForwardAgentBridgeTask) TableName() string { return "v2_forward_agent_bridge_task" }

type retiredForwardRuntimeJob struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Backend      string     `gorm:"size:50;not null;index" json:"backend"`
	Action       string     `gorm:"size:30;not null;index" json:"action"`
	ResourceType string     `gorm:"size:30;index" json:"resourceType"`
	ResourceID   *uint      `json:"resourceId"`
	ForwardID    *uint      `gorm:"index" json:"forwardId"`
	TunnelID     *uint      `gorm:"index" json:"tunnelId"`
	NodeID       *uint      `gorm:"index" json:"nodeId"`
	AgentID      *uint      `gorm:"index" json:"agentId"`
	Status       int        `gorm:"default:0;index" json:"status"`
	Payload      string     `gorm:"type:text" json:"payload"`
	Result       string     `gorm:"type:text" json:"result"`
	Error        string     `gorm:"type:text" json:"error"`
	StartedAt    *time.Time `json:"startedAt"`
	ClaimedAt    *time.Time `json:"claimedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (retiredForwardRuntimeJob) TableName() string { return "v2_forward_runtime_job" }

type retiredForwardTrafficCursor struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ForwardID     uint      `gorm:"uniqueIndex:idx_forward_traffic_cursor" json:"forwardId"`
	Backend       string    `gorm:"size:50;uniqueIndex:idx_forward_traffic_cursor" json:"backend"`
	UploadTotal   int64     `gorm:"default:0" json:"uploadTotal"`
	DownloadTotal int64     `gorm:"default:0" json:"downloadTotal"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (retiredForwardTrafficCursor) TableName() string { return "v2_forward_traffic_cursor" }
