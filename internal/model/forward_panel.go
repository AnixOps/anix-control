package model

import "time"

const (
	ForwardTunnelStatusDisabled = 0
	ForwardTunnelStatusActive   = 1
)

const (
	ForwardStatusPaused = 0
	ForwardStatusActive = 1
	ForwardStatusError  = -1
)

const (
	ForwardUserTunnelStatusDisabled = 0
	ForwardUserTunnelStatusActive   = 1
)

// ForwardTunnel is the minimal tunnel resource required by the flux-panel style forward page.
type ForwardTunnel struct {
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

func (ForwardTunnel) TableName() string {
	return "v2_forward_tunnel"
}

// ForwardUserTunnel stores flux-panel style per-user tunnel authorization.
type ForwardUserTunnel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"uniqueIndex:idx_forward_user_tunnel" json:"user_id"`
	TunnelID      uint      `gorm:"uniqueIndex:idx_forward_user_tunnel;index" json:"tunnel_id"`
	Flow          int64     `gorm:"default:0" json:"flow"`
	Num           int       `gorm:"default:0" json:"num"`
	FlowResetTime int64     `gorm:"default:0" json:"flowResetTime"`
	ExpTime       int64     `gorm:"default:0" json:"expTime"`
	SpeedID       *uint     `json:"speedId"`
	Status        int       `gorm:"default:1" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	User   *User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Tunnel *ForwardTunnel `gorm:"foreignKey:TunnelID" json:"tunnel,omitempty"`
}

func (ForwardUserTunnel) TableName() string {
	return "v2_forward_user_tunnel"
}

// Forward is the flux-panel compatible forward resource.
type Forward struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	UserID            uint           `gorm:"index;not null" json:"user_id"`
	UserName          string         `gorm:"size:255" json:"user_name"`
	Name              string         `gorm:"size:100;not null" json:"name"`
	TunnelID          uint           `gorm:"index;not null" json:"tunnel_id"`
	Tunnel            *ForwardTunnel `gorm:"foreignKey:TunnelID" json:"tunnel,omitempty"`
	InPort            int            `gorm:"not null" json:"in_port"`
	OutPort           int            `json:"out_port"`
	RemoteAddr        string         `gorm:"type:text;not null" json:"remote_addr"`
	InterfaceName     string         `gorm:"size:255" json:"interface_name"`
	Strategy          string         `gorm:"size:20;default:'fifo'" json:"strategy"`
	Status            int            `gorm:"default:1" json:"status"`
	RuntimeBackend    string         `gorm:"size:50;default:'gost';index" json:"runtimeBackend"`
	RuntimeStatus     int            `gorm:"default:0;index" json:"runtimeStatus"`
	RuntimeMessage    string         `gorm:"type:text" json:"runtimeMessage"`
	RuntimeLastSyncAt *time.Time     `json:"runtimeLastSyncAt"`
	InFlow            int64          `gorm:"default:0" json:"in_flow"`
	OutFlow           int64          `gorm:"default:0" json:"out_flow"`
	Inx               int            `gorm:"default:0" json:"inx"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	User              *User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (Forward) TableName() string {
	return "v2_forward"
}
