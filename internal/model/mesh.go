package model

import "time"

// MeshRegion is a region of the intranet mesh (组网, internal/meshregion;
// docs/design/mesh-regions): one network of its own with a virtual IPv4
// range no other region overlaps. The lower half of the range is the
// server segment (fixed addresses of servers and boundary nodes), the upper
// half the device pool. Nodes of a region get the network in the member
// "mesh" of their desired configuration while the region is enabled.
type MeshRegion struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Code is the network id the nodes see, such as "cn" or "eu".
	Code string `gorm:"size:16;not null;uniqueIndex" json:"code"`
	Name string `gorm:"size:64;not null" json:"name"`
	// CIDR is the region's virtual range, such as 10.150.0.0/16.
	CIDR string `gorm:"column:cidr;size:43;not null" json:"cidr"`
	// Secret is the network key; it is only in the nodes' desired
	// configurations. Control generates it when the region is first used.
	Secret    string    `gorm:"size:64;not null" json:"-"`
	Enabled   bool      `gorm:"not null" json:"enabled"`
	Sort      int       `gorm:"not null" json:"sort"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshRegion) TableName() string { return "v5_mesh_region" }

// MeshMember is a node's fixed address in a region. A node has one home
// membership (Home); a boundary node (Boundary, set on its home row) also
// has a membership in every other region, which Control keeps.
type MeshMember struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	RegionID uint `gorm:"not null;uniqueIndex:idx_v5_mesh_member_region_node,priority:1;uniqueIndex:idx_v5_mesh_member_region_address,priority:1" json:"region_id"`
	// NodeRef is the node ("proxy-<id>" or "forward-<id>").
	NodeRef   string    `gorm:"size:32;not null;uniqueIndex:idx_v5_mesh_member_region_node,priority:2;index" json:"node_ref"`
	Address   string    `gorm:"size:15;not null;uniqueIndex:idx_v5_mesh_member_region_address,priority:2" json:"address"`
	Home      bool      `gorm:"not null" json:"home"`
	Boundary  bool      `gorm:"not null" json:"boundary"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (MeshMember) TableName() string { return "v5_mesh_member" }

// MeshStatus is the latest mesh status a node's Agent reported (the
// package report "mesh.status"), as the Agent sent it.
type MeshStatus struct {
	NodeRef    string    `gorm:"primaryKey;size:32" json:"node_ref"`
	Payload    string    `gorm:"type:text;not null" json:"-"`
	ObservedAt time.Time `gorm:"not null" json:"observed_at"`
	ReceivedAt time.Time `gorm:"not null" json:"received_at"`
}

func (MeshStatus) TableName() string { return "v5_mesh_status" }

// MeshEnrollment is a one-time enrollment credential for a mesh device: a
// node without an Agent (its install command uses it) or a personal device.
// Only the SHA-256 of the token is kept; enrolling sets UsedAt and DeviceID.
type MeshEnrollment struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// RegionID is the device's region; 0 picks the region of lowest latency
	// the device reports when it enrolls.
	RegionID uint `gorm:"not null" json:"region_id"`
	// Kind is "node" (a full node with a server segment address) or
	// "personal" (an access device with a device pool address).
	Kind      string     `gorm:"size:16;not null" json:"kind"`
	Name      string     `gorm:"size:64;not null" json:"name"`
	TokenHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	DeviceID  *uint      `json:"device_id"`
	CreatedBy *uint      `json:"created_by"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshEnrollment) TableName() string { return "v5_mesh_enrollment" }

// MeshDevice is an enrolled mesh device without an Agent. DeviceKey is the
// identity the device chose; its address stays while it keeps the key. A
// personal device unused for 30 days gives up its address, and a revoked
// device (RevokedAt) is refused. Only the SHA-256 of its token is kept.
type MeshDevice struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	RegionID  uint   `gorm:"not null;index" json:"region_id"`
	Kind      string `gorm:"size:16;not null" json:"kind"`
	Name      string `gorm:"size:64;not null" json:"name"`
	DeviceKey string `gorm:"size:128;not null;uniqueIndex" json:"device_key"`
	TokenHash string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	// Address is "" while the device has none (reclaimed or revoked).
	Address    string     `gorm:"size:15;not null" json:"address"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshDevice) TableName() string { return "v5_mesh_device" }

// MeshRegionChange is a device's request to move to another region, which
// an administrator approves (the device moves and gets a new address) or
// rejects. State is "pending", "approved" or "rejected".
type MeshRegionChange struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	DeviceID     uint       `gorm:"not null;index" json:"device_id"`
	FromRegionID uint       `gorm:"not null" json:"from_region_id"`
	ToRegionID   uint       `gorm:"not null" json:"to_region_id"`
	Reason       string     `gorm:"size:200;not null" json:"reason"`
	State        string     `gorm:"size:16;not null" json:"state"`
	DecidedBy    *uint      `json:"decided_by"`
	DecidedAt    *time.Time `json:"decided_at"`
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshRegionChange) TableName() string { return "v5_mesh_region_change" }
