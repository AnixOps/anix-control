package model

import (
	"time"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
)

// Line is a line as stored (v5_line, docs/design/network-lines): from its
// entries over its chain to its exit. A forward route is a line whose
// Route holds the forward.v1 Route as protojson; the forward API writes
// it, and the line's other columns are the route as internal/netline reads
// it. Enforced is the kernel's own pause of the route, separate from the
// route's paused flag: "quota" once the entry hop's metered bytes reach
// limits.quota_bytes, "expired" once limits.expires_at_unix_ms has passed,
// empty otherwise. The planner sees an enforced route as paused; the
// stored route is unchanged.
type Line struct {
	ID            string                     `gorm:"primaryKey;size:64"`
	Owner         string                     `gorm:"size:64;not null;default:'';index"`
	Revision      uint64                     `gorm:"not null;default:0"`
	Route         string                     `gorm:"type:text;not null;default:''"`
	Enforced      string                     `gorm:"size:16;not null;default:''"`
	Sort          int                        `gorm:"not null;default:0"`
	Name          string                     `gorm:"size:191;not null;default:''"`
	Enabled       bool                       `gorm:"not null;default:false"`
	Show          bool                       `gorm:"not null;default:false"`
	Entries       []string                   `gorm:"type:text;serializer:json"`
	EntryHostname string                     `gorm:"size:255;not null;default:''"`
	Listen        string                     `gorm:"size:64;not null;default:''"`
	Port          uint32                     `gorm:"not null;default:0"`
	L4            []string                   `gorm:"type:text;serializer:json"`
	Chain         []LineHop                  `gorm:"type:text;serializer:json"`
	Exit          string                     `gorm:"size:64;not null;default:''"`
	Rules         []LineRule                 `gorm:"type:text;serializer:json"`
	ProtocolID    *uint                      `gorm:"index"`
	Targets       []*networkv1.ForwardTarget `gorm:"type:text;serializer:json"`
	Policy        *networkv1.ForwardPolicy   `gorm:"type:text;serializer:json"`
	HopBalance    string                     `gorm:"size:32;not null;default:''"`
	Rate          float64                    `gorm:"not null;default:1"`
	// ServeAt is the device the line's protocol is served at when the
	// administrator chose it; empty is the first userspace device.
	ServeAt   string    `gorm:"size:32;not null;default:''"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (Line) TableName() string { return "v5_line" }

// LineRule sends a line's traffic for some domains to another exit.
type LineRule struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Exit    string   `json:"exit"`
	Enabled bool     `json:"enabled"`
}

// LineHop is a step of a line's chain: its devices (several on the entry
// hop are entry HA) and how the hop before reaches it.
type LineHop struct {
	Devices []string `json:"devices"`
	// Port is where a hop after the entry listens for a kernel hop before
	// it; 0 is the line's port.
	Port uint32 `json:"port,omitempty"`
	// Dial replaces the device's host for the hop before.
	Dial string `json:"dial,omitempty"`
	// Transport is the transport from the hop before; empty after a kernel
	// device, which forwards without one, and on the entry hop.
	Transport string `json:"transport,omitempty"`
}

// Transport is a link between two devices as stored (v5_transport).
type Transport struct {
	ID        string    `gorm:"primaryKey;size:191"`
	From      string    `gorm:"column:from_device;size:32;not null"`
	To        string    `gorm:"column:to_device;size:32;not null"`
	Kind      string    `gorm:"size:16;not null"`
	Carriers  []string  `gorm:"type:text;serializer:json"`
	Multipath bool      `gorm:"not null;default:false"`
	FEC       bool      `gorm:"column:fec;not null;default:false"`
	Declared  bool      `gorm:"not null;default:false"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (Transport) TableName() string { return "v5_transport" }
