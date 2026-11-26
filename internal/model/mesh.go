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
	NodeRef  string `gorm:"size:32;not null;uniqueIndex:idx_v5_mesh_member_region_node,priority:2;index" json:"node_ref"`
	Address  string `gorm:"size:15;not null;uniqueIndex:idx_v5_mesh_member_region_address,priority:2" json:"address"`
	Home     bool   `gorm:"not null" json:"home"`
	Boundary bool   `gorm:"not null" json:"boundary"`
	// StaticKey is the node's static public key in this region's network
	// as its Agent last reported it (0050_mesh_native_keys): 32 bytes of
	// X25519 in standard base64; nil before the first report.
	StaticKey *string   `gorm:"size:44" json:"static_key"`
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

// MeshSettings is the one row (ID 1) of the mesh's global settings
// (0046_mesh_addresses). Before an administrator first saves them the
// defaults of internal/meshregion apply.
type MeshSettings struct {
	ID uint `gorm:"primaryKey" json:"-"`
	// NameSuffix is the intranet domain suffix: a name is
	// <name>.<region code>.<suffix>.
	NameSuffix string `gorm:"size:200;not null" json:"name_suffix"`
	// HostGroupSupernet is the IPv4 range the host groups' segments come
	// from, outside every region's range.
	HostGroupSupernet string `gorm:"size:43;not null" json:"host_group_supernet"`
	// HostGroupPrefix is the prefix length of a host group's segment.
	HostGroupPrefix int `gorm:"not null" json:"host_group_prefix"`
	// ReservedRanges are IPv4 ranges no region or host group may overlap,
	// comma-separated.
	ReservedRanges string `gorm:"type:text;not null" json:"-"`
	// PreferredRegionID is the panel-wide default preferred region of the
	// automatic region choice (0055_mesh_region_assignment): its latency
	// counts 0.8 times. NULL: none.
	PreferredRegionID *uint     `json:"-"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshSettings) TableName() string { return "v5_mesh_settings" }

// MeshDevice is a personal device (phone, computer) with an address from
// its region's device pool, the upper half of the region's range. The
// address stays the device's while it is used; one unused for 30 days goes
// to the next device that needs one when the pool has no free address.
type MeshDevice struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	RegionID uint `gorm:"not null;uniqueIndex:idx_v5_mesh_device_region_address,priority:1" json:"region_id"`
	// DeviceKey is the device's identity key once it enrolled; nil for a
	// device an administrator added.
	DeviceKey *string `gorm:"size:128;uniqueIndex" json:"device_key"`
	// Hostname is the device's DNS label in the intranet names.
	Hostname string `gorm:"size:63;not null" json:"hostname"`
	// Address is nil after the address went to another device.
	Address       *string    `gorm:"size:15;uniqueIndex:idx_v5_mesh_device_region_address,priority:2" json:"address"`
	AddressUsedAt *time.Time `json:"address_used_at"`
	// StaticKey is the device's static public key in its region's network
	// (0050_mesh_native_keys), from its enrollment or its first report
	// after it; nil before either and after a move to another region.
	StaticKey *string   `gorm:"size:44" json:"static_key"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshDevice) TableName() string { return "v5_mesh_device" }

// MeshHostGroup is a host group: the virtual machines or containers behind
// a gateway node, on a segment of the host group supernet outside every
// region's range. A standby gateway takes over when the gateway fails; it
// has the gateway's home region.
type MeshHostGroup struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"size:63;not null" json:"name"`
	Segment string `gorm:"size:43;not null;uniqueIndex" json:"segment"`
	// GatewayRef and StandbyRef are nodes ("proxy-<id>", "forward-<id>").
	GatewayRef string    `gorm:"size:32;not null;index" json:"gateway_ref"`
	StandbyRef *string   `gorm:"size:32;index" json:"standby_ref"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshHostGroup) TableName() string { return "v5_mesh_host_group" }

// MeshDeviceName is a name a gateway node publishes for a host behind it:
// a host of one of its host groups, or a device of its local network.
type MeshDeviceName struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	GatewayRef  string    `gorm:"size:32;not null;index" json:"gateway_ref"`
	HostGroupID *uint     `gorm:"index" json:"host_group_id"`
	Name        string    `gorm:"size:63;not null" json:"name"`
	Address     string    `gorm:"size:15;not null" json:"address"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshDeviceName) TableName() string { return "v5_mesh_device_name" }

// MeshRegionChange is a request to move a node (NodeRef) or a device
// (DeviceID) to another region, which changes its address: the member
// asks, an administrator approves or rejects. A move an administrator
// makes directly is recorded approved.
type MeshRegionChange struct {
	ID           uint    `gorm:"primaryKey" json:"id"`
	NodeRef      *string `gorm:"size:32;index" json:"node_ref"`
	DeviceID     *uint   `gorm:"index" json:"device_id"`
	FromRegionID uint    `gorm:"not null" json:"from_region_id"`
	ToRegionID   uint    `gorm:"not null" json:"to_region_id"`
	// Source is "member" or "admin".
	Source string `gorm:"size:16;not null" json:"source"`
	// Status is "pending", "approved" or "rejected".
	Status      string     `gorm:"size:16;not null;index" json:"status"`
	Reason      string     `gorm:"size:500;not null" json:"reason"`
	Note        string     `gorm:"size:500;not null" json:"note"`
	OldAddress  string     `gorm:"size:15;not null" json:"old_address"`
	NewAddress  string     `gorm:"size:15;not null" json:"new_address"`
	RequestedBy *uint      `json:"requested_by"`
	DecidedBy   *uint      `json:"decided_by"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshRegionChange) TableName() string { return "v5_mesh_region_change" }

// Region change sources and states.
const (
	MeshChangeMember   = "member"
	MeshChangeAdmin    = "admin"
	MeshChangePending  = "pending"
	MeshChangeApproved = "approved"
	MeshChangeRejected = "rejected"
)

// MeshEnrollment is a one-time enrollment credential for a device without
// an Agent (R31): it is used once before ExpiresAt, and DeviceID is the
// device that used it. Only the SHA-256 of its token is kept.
type MeshEnrollment struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// RegionID is the device's region; 0 lets the latencies the device
	// reports choose among the enabled regions.
	RegionID uint `gorm:"not null" json:"region_id"`
	// PreferredRegionID is the region whose latency counts 0.8 times when
	// the latencies choose the region (0055_mesh_region_assignment); NULL:
	// the settings' default.
	PreferredRegionID *uint      `json:"preferred_region_id"`
	Hostname          string     `gorm:"size:63;not null" json:"hostname"`
	TokenHash         string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt         time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt            *time.Time `json:"used_at"`
	DeviceID          *uint      `json:"device_id"`
	CreatedBy         *uint      `json:"created_by"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshEnrollment) TableName() string { return "v5_mesh_enrollment" }

// MeshDeviceToken is the token an enrolled device reads its networks and
// sends its status with; deleting the row revokes it. Only the SHA-256 of
// the token is kept.
type MeshDeviceToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	DeviceID   uint       `gorm:"not null;uniqueIndex" json:"device_id"`
	TokenHash  string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshDeviceToken) TableName() string { return "v5_mesh_device_token" }

// MeshFullNode marks the credential EnrollmentID, and the device DeviceID
// it enrolled, as a full node without an Agent (R32: an OpenWrt router or a
// server that relays and advertises its local networks) instead of an
// access member: its fixed Address comes from the region's server segment
// (empty on the credential: the lowest free one), and it advertises the
// Subnets an administrator confirmed (R17), comma separated
// (0048_mesh_full_nodes).
type MeshFullNode struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	EnrollmentID uint   `gorm:"not null;uniqueIndex" json:"enrollment_id"`
	DeviceID     *uint  `gorm:"uniqueIndex" json:"device_id"`
	Address      string `gorm:"size:15;not null" json:"address"`
	Subnets      string `gorm:"type:text;not null" json:"-"`
	// RequestedSubnets are the subnets the node itself asked to advertise
	// (comma separated, R35; 0052_mesh_subnet_requests), pending until an
	// administrator approves or rejects them; RequestedAt is when. Both are
	// NULL while nothing is pending.
	RequestedSubnets *string    `gorm:"type:text" json:"-"`
	RequestedAt      *time.Time `json:"-"`
	CreatedAt        time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updated_at"`
}

func (MeshFullNode) TableName() string { return "v5_mesh_full_node" }

// MeshExternalBridge marks the node NodeRef as an external network bridge
// in its home region (R36, spec section 15.12; 0051_mesh_external_bridges):
// it advertises Routes, the external addresses and subnets an
// administrator listed (comma separated), and lets only the sources of
// Allow reach them through Interface, its network interface on the
// external network. Allow lists the members by identity, comma separated:
// node:<node ref>, device:<device id> or host_group:<host group id>;
// Control expands them to their current addresses for the node.
type MeshExternalBridge struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeRef   string    `gorm:"size:32;not null;uniqueIndex" json:"node_ref"`
	Interface string    `gorm:"size:15;not null" json:"interface"`
	Routes    string    `gorm:"type:text;not null" json:"-"`
	Allow     string    `gorm:"type:text;not null" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshExternalBridge) TableName() string { return "v5_mesh_external_bridge" }

// MeshBridgeHold keeps Prefix (an address /32 or a host group segment) of
// the region RegionID from being given out again after it left the allow
// list the bridge node NodeRef was given (spec section 15.12 item 4): until
// the node reports a list without it applied, observed after CreatedAt
// (0051_mesh_external_bridges).
type MeshBridgeHold struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	NodeRef   string    `gorm:"size:32;not null;uniqueIndex:idx_v5_mesh_bridge_hold_entry,priority:1" json:"node_ref"`
	RegionID  uint      `gorm:"not null;uniqueIndex:idx_v5_mesh_bridge_hold_entry,priority:2" json:"region_id"`
	Prefix    string    `gorm:"size:43;not null;uniqueIndex:idx_v5_mesh_bridge_hold_entry,priority:3" json:"prefix"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (MeshBridgeHold) TableName() string { return "v5_mesh_bridge_hold" }

// MeshPath is a custom path of a region network (0054_mesh_paths): traffic
// of the member SourceRef to Target goes through the waypoint members of Via
// in order. TargetKind is "peer" (Target is a member entry) or "subnet"
// (Target is a CIDR). Members are stored by identity, as node:<node ref> or
// device:<device id>, and Via lists them comma separated; their addresses
// are resolved when the path is delivered.
type MeshPath struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SourceRef  string    `gorm:"size:32;not null;uniqueIndex:idx_v5_mesh_path_target,priority:1" json:"source"`
	TargetKind string    `gorm:"size:8;not null" json:"target_kind"`
	Target     string    `gorm:"size:64;not null;uniqueIndex:idx_v5_mesh_path_target,priority:2" json:"target"`
	Via        string    `gorm:"type:text;not null" json:"-"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshPath) TableName() string { return "v5_mesh_path" }

// MeshModels are the mesh's tables (0045_mesh_regions, 0046_mesh_addresses,
// 0047_mesh_enrollment, 0048_mesh_full_nodes, 0051_mesh_external_bridges,
// 0054_mesh_paths).
func MeshModels() []any {
	return []any{&MeshRegion{}, &MeshMember{}, &MeshStatus{}, &MeshSettings{}, &MeshDevice{}, &MeshHostGroup{}, &MeshDeviceName{}, &MeshRegionChange{}, &MeshEnrollment{}, &MeshDeviceToken{}, &MeshFullNode{}, &MeshExternalBridge{}, &MeshBridgeHold{}, &MeshPath{}}
}
