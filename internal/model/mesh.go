package model

import "time"

// The intranet mesh's panel side (internal/meshregion,
// docs/design/mesh-regions): the regions an administrator defines, the
// members and their virtual addresses, the host groups and the names the
// gateways publish, and the region changes awaiting approval. The mesh
// itself runs in the node kernels; Control only plans it.

// MeshSettings is the one row (ID 1) of the mesh's global settings.
type MeshSettings struct {
	ID uint `gorm:"primaryKey" json:"-"`
	// NameSuffix is the intranet domain suffix, "mesh.internal" by default:
	// a member is <hostname>.<region code>.<suffix>.
	NameSuffix string `gorm:"size:200;not null" json:"name_suffix"`
	// HostGroupSupernet is the IPv4 range the host groups' segments are
	// taken from; it lies outside every region's subnet.
	HostGroupSupernet string `gorm:"size:64;not null" json:"host_group_supernet"`
	// HostGroupPrefix is the prefix length of a host group's segment.
	HostGroupPrefix int `gorm:"not null" json:"host_group_prefix"`
	// ReservedSubnets are IPv4 ranges no region or host group may overlap,
	// comma-separated: the existing network's virtual subnet during the
	// migration, for example.
	ReservedSubnets string    `gorm:"type:text;not null" json:"reserved_subnets"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshSettings) TableName() string { return "v5_mesh_settings" }

// MeshRegion is one region: an independent mesh network with its own
// network name and key and a virtual subnet no other region overlaps. The
// subnet is split into the server range (fixed addresses) and the device
// pool (addresses kept per device). Every region network uses forward
// secrecy.
type MeshRegion struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Code is the region's DNS label in intranet names, such as "cn".
	Code      string `gorm:"size:16;not null;uniqueIndex:idx_v5_mesh_region_code" json:"code"`
	Name      string `gorm:"size:64;not null" json:"name"`
	SortOrder int    `gorm:"not null;default:0" json:"sort_order"`
	// Subnet is the region's virtual IPv4 subnet; every member uses its
	// prefix length.
	Subnet      string `gorm:"size:64;not null" json:"subnet"`
	ServerRange string `gorm:"size:64;not null" json:"server_range"`
	DevicePool  string `gorm:"size:64;not null" json:"device_pool"`
	// NetworkName is generated when the region is created.
	NetworkName string `gorm:"size:64;not null" json:"network_name"`
	// NetworkKeySealed is the network key sealed with Control's
	// key-encryption key; empty until the first member is admitted.
	NetworkKeySealed string    `gorm:"type:text;not null" json:"-"`
	CreatedAt        time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshRegion) TableName() string { return "v5_mesh_region" }

// Mesh member roles.
const (
	MeshRoleBorder  = "border"
	MeshRoleGateway = "gateway"
	MeshRoleServer  = "server"
	MeshRoleDevice  = "device"
)

// Mesh address modes.
const (
	MeshAddressFixed = "fixed"
	MeshAddressPool  = "pool"
)

// MeshMember is a machine of the mesh: an infrastructure node (border,
// gateway, server) with a fixed address, or a personal device with an
// address from its region's device pool that it keeps.
type MeshMember struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// RegionID is nil until the member is admitted to a region.
	RegionID *uint `gorm:"index;uniqueIndex:idx_v5_mesh_member_address,priority:1" json:"region_id"`
	// NodeRef is the Agent node, proxy-<id> or forward-<id>; nil for a
	// machine without an Agent.
	NodeRef *string `gorm:"size:32;uniqueIndex:idx_v5_mesh_member_node" json:"node_ref"`
	// DeviceKey is the member's device public key once it joined; nil
	// before.
	DeviceKey   *string `gorm:"size:128;uniqueIndex:idx_v5_mesh_member_device_key" json:"device_key"`
	Hostname    string  `gorm:"size:63;not null" json:"hostname"`
	Role        string  `gorm:"size:16;not null" json:"role"`
	AddressMode string  `gorm:"size:16;not null" json:"address_mode"`
	// Address is the virtual IPv4 address in the region; nil while none is
	// assigned.
	Address *string `gorm:"size:64;uniqueIndex:idx_v5_mesh_member_address,priority:2" json:"address"`
	// AddressUsedAt is when the member last used its address; a pool
	// address unused for 30 days goes to the next device that needs one.
	AddressUsedAt *time.Time `json:"address_used_at"`
	// ProbeAddress is a border node's public host:port that members probe
	// and connect to.
	ProbeAddress string `gorm:"size:255;not null;default:''" json:"probe_address"`
	// Agentless is a machine set up with the installer and a join
	// credential: it only reports its state.
	Agentless bool      `gorm:"not null;default:false" json:"agentless"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshMember) TableName() string { return "v5_mesh_member" }

// MeshHostGroup is the virtual machines of one physical host, represented
// in the mesh by one gateway (and an optional standby gateway) that
// advertises the group's segment. The group follows its gateway's region.
type MeshHostGroup struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:64;not null" json:"name"`
	// Segment is the group's IPv4 segment, taken from the host group
	// supernet, outside every region's subnet.
	Segment         string    `gorm:"size:64;not null;uniqueIndex:idx_v5_mesh_host_group_segment" json:"segment"`
	GatewayMemberID uint      `gorm:"not null;index" json:"gateway_member_id"`
	StandbyMemberID *uint     `gorm:"index" json:"standby_member_id"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshHostGroup) TableName() string { return "v5_mesh_host_group" }

// MeshDeviceName is a name an administrator gave a machine that is not a
// member: a virtual machine of a host group or a device in a gateway's
// local network. The gateway publishes it in the intranet names.
type MeshDeviceName struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// MemberID is the gateway that publishes the name.
	MemberID uint `gorm:"not null;index" json:"member_id"`
	// HostGroupID is the host group of a virtual machine; nil for a device
	// in the gateway's local network.
	HostGroupID *uint     `gorm:"index" json:"host_group_id"`
	Name        string    `gorm:"size:63;not null" json:"name"`
	Address     string    `gorm:"size:64;not null" json:"address"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (MeshDeviceName) TableName() string { return "v5_mesh_device_name" }

// Mesh region change states.
const (
	MeshChangePending  = "pending"
	MeshChangeApproved = "approved"
	MeshChangeRejected = "rejected"
)

// Who asked for a region change.
const (
	MeshChangeByMember = "member"
	MeshChangeByAdmin  = "admin"
)

// MeshRegionChange is a request to move a member to another region, which
// changes its virtual address. A member asks; an administrator approves or
// rejects. An administrator's own move is recorded approved.
type MeshRegionChange struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	MemberID     uint   `gorm:"not null;index" json:"member_id"`
	FromRegionID uint   `gorm:"not null" json:"from_region_id"`
	ToRegionID   uint   `gorm:"not null" json:"to_region_id"`
	Source       string `gorm:"size:16;not null" json:"source"`
	Status       string `gorm:"size:16;not null;index" json:"status"`
	// Reason is the request's text, such as the measured latencies.
	Reason string `gorm:"size:500;not null;default:''" json:"reason"`
	// Note is the administrator's note on the decision.
	Note string `gorm:"size:500;not null;default:''" json:"note"`
	// OldAddress and NewAddress are the member's addresses before and
	// after an approved move.
	OldAddress  string     `gorm:"size:64;not null;default:''" json:"old_address"`
	NewAddress  string     `gorm:"size:64;not null;default:''" json:"new_address"`
	RequestedBy *uint      `json:"requested_by"`
	DecidedBy   *uint      `json:"decided_by"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
}

func (MeshRegionChange) TableName() string { return "v5_mesh_region_change" }

// MeshModels are the mesh tables (migration 0045_mesh_regions).
func MeshModels() []any {
	return []any{&MeshSettings{}, &MeshRegion{}, &MeshMember{}, &MeshHostGroup{}, &MeshDeviceName{}, &MeshRegionChange{}}
}
