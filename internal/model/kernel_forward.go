package model

import "time"

// The kernel's forwarding state (docs/architecture/forward-sdk.md section
// 8, F3a; internal/kernelforward). Routes, their port and mark allocations,
// the node inventory with each Agent's reported capabilities, the desired
// state and generation of every node, each node's latest report and the
// traffic ledger. Every table is new, protected (service.protectedTables)
// and written only by internal/kernelforward; packages reach the data
// through ForwardControl (kernel.forward.v1), never through storage.

// KernelForwardRoute is one stored route: the contract's Route as protojson,
// with the columns the kernel filters on. Enforced is the kernel's own
// pause of the route, separate from the route's paused flag: "quota" once
// the entry hop's metered bytes reach limits.quota_bytes, "expired" once
// limits.expires_at_unix_ms has passed, empty otherwise. The planner sees
// an enforced route as paused; the stored route is unchanged.
type KernelForwardRoute struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Owner     string    `gorm:"size:64;not null;index" json:"owner"`
	Revision  uint64    `gorm:"not null" json:"revision"`
	RouteJSON string    `gorm:"type:text;not null" json:"route"`
	Enforced  string    `gorm:"size:16;not null;default:''" json:"enforced"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (KernelForwardRoute) TableName() string { return "v4_kernel_forward_route" }

// KernelForwardAllocation is the port and connection mark a hop of a route
// holds on a node (planner.Allocations). The planner passes the active ones
// back as the previous allocations so they stick. A deleted route's
// allocations get ReleasedAt and are held (planner Options.Taken) for the
// grace period, so a late packet never reaches a new route.
type KernelForwardAllocation struct {
	RouteID    string     `gorm:"primaryKey;size:64" json:"route_id"`
	HopIndex   uint32     `gorm:"primaryKey" json:"hop_index"`
	NodeRef    string     `gorm:"primaryKey;size:32;index" json:"node_ref"`
	Port       uint32     `gorm:"not null" json:"port"`
	Mark       uint32     `gorm:"not null" json:"mark"`
	ReleasedAt *time.Time `gorm:"index" json:"released_at"`
	UpdatedAt  time.Time  `gorm:"not null" json:"updated_at"`
}

func (KernelForwardAllocation) TableName() string { return "v4_kernel_forward_allocation" }

// KernelForwardNode is a node of the forwarding inventory: a proxy or
// forward node whose Agent negotiated forward.v1 at least once, or that an
// administrator configured for forwarding. Negotiated is whether its last
// Hello negotiated forward.v1, which decides its configuration format
// (anixops.nodeconfig/v2 or v1) for every builder. CapabilitiesJSON is the
// NodeCapabilities of the last forward.v1 Hello (protojson). The port
// range, reserved ports, addresses and labels are the kernel's settings
// for the node; zero or empty means the defaults (kernelforward).
type KernelForwardNode struct {
	NodeRef           string     `gorm:"primaryKey;size:32" json:"node_ref"`
	NodeKind          string     `gorm:"size:16;not null;uniqueIndex:ux_kernel_forward_node" json:"node_kind"`
	NodeID            uint64     `gorm:"not null;uniqueIndex:ux_kernel_forward_node" json:"node_id"`
	Negotiated        bool       `gorm:"not null;default:false" json:"negotiated"`
	CapabilitiesJSON  string     `gorm:"type:text;not null;default:''" json:"capabilities"`
	CapabilitiesHash  string     `gorm:"size:64;not null;default:''" json:"capabilities_hash"`
	AgentVersion      string     `gorm:"size:128;not null;default:''" json:"agent_version"`
	ReportedAt        *time.Time `json:"reported_at"`
	PortFirst         uint32     `gorm:"not null;default:0" json:"port_first"`
	PortLast          uint32     `gorm:"not null;default:0" json:"port_last"`
	ReservedPortsJSON string     `gorm:"type:text;not null;default:''" json:"reserved_ports"`
	AddressesJSON     string     `gorm:"type:text;not null;default:''" json:"addresses"`
	LabelsJSON        string     `gorm:"type:text;not null;default:''" json:"labels"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

func (KernelForwardNode) TableName() string { return "v4_kernel_forward_node" }

// KernelForwardNodeState is a node's desired forwarding state as the last
// accepted plan stamped it: its generation and state_hash
// (planner.Generation) and the NodeForwardState as protojson, which the
// node's anixops.nodeconfig/v2 configuration carries.
type KernelForwardNodeState struct {
	NodeRef    string    `gorm:"primaryKey;size:32" json:"node_ref"`
	Generation uint64    `gorm:"not null" json:"generation"`
	StateHash  string    `gorm:"size:64;not null" json:"state_hash"`
	StateJSON  string    `gorm:"type:text;not null" json:"state"`
	PlannedAt  time.Time `gorm:"not null" json:"planned_at"`
}

func (KernelForwardNodeState) TableName() string { return "v4_kernel_forward_node_state" }

// KernelForwardNodeReport is a node's latest NodeForwardReport: the
// generation and state_hash it runs, whether it applied it, its hop errors
// and upstream health (ReportJSON, the report without its counters, which
// go to the traffic ledger). A report replaces the row unless the stored
// one was observed later.
type KernelForwardNodeReport struct {
	NodeRef    string    `gorm:"primaryKey;size:32" json:"node_ref"`
	Generation uint64    `gorm:"not null" json:"generation"`
	StateHash  string    `gorm:"size:64;not null;default:''" json:"state_hash"`
	Applied    bool      `gorm:"not null;default:false" json:"applied"`
	HopErrors  int       `gorm:"not null;default:0" json:"hop_errors"`
	ReportJSON string    `gorm:"type:text;not null" json:"report"`
	ObservedAt time.Time `gorm:"not null;index" json:"observed_at"`
	ReceivedAt time.Time `gorm:"not null" json:"received_at"`
}

func (KernelForwardNodeReport) TableName() string { return "v4_kernel_forward_node_report" }

// KernelForwardCounter is the traffic ledger's cursor and total for one
// counter epoch of one hop of a route on a node: the largest cumulative
// values the node reported within the epoch. Counters only grow within an
// epoch and a new epoch starts from zero, so a route's metered traffic is
// the sum of its epochs' rows, and a report adds only the growth over the
// stored values (forward-sdk.md sections 8.2 and 11).
type KernelForwardCounter struct {
	RouteID      string    `gorm:"primaryKey;size:64" json:"route_id"`
	HopIndex     uint32    `gorm:"primaryKey" json:"hop_index"`
	NodeRef      string    `gorm:"primaryKey;size:32;index" json:"node_ref"`
	CounterEpoch string    `gorm:"primaryKey;size:128" json:"counter_epoch"`
	UpBytes      uint64    `gorm:"not null" json:"up_bytes"`
	DownBytes    uint64    `gorm:"not null" json:"down_bytes"`
	UpPackets    uint64    `gorm:"not null" json:"up_packets"`
	DownPackets  uint64    `gorm:"not null" json:"down_packets"`
	TotalConns   uint64    `gorm:"not null" json:"total_conns"`
	ActiveConns  uint32    `gorm:"not null" json:"active_conns"`
	ObservedAt   time.Time `gorm:"not null" json:"observed_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (KernelForwardCounter) TableName() string { return "v4_kernel_forward_counter" }

// KernelForwardTraffic is the traffic ledger: the raw growth of a hop's
// counters on a node within one UTC hour, by direction, as reports
// delivered it. No multiplier is applied; billing applies them later.
type KernelForwardTraffic struct {
	RouteID     string    `gorm:"primaryKey;size:64" json:"route_id"`
	HopIndex    uint32    `gorm:"primaryKey" json:"hop_index"`
	NodeRef     string    `gorm:"primaryKey;size:32" json:"node_ref"`
	HourStart   time.Time `gorm:"primaryKey;index" json:"hour_start"`
	UpBytes     uint64    `gorm:"not null" json:"up_bytes"`
	DownBytes   uint64    `gorm:"not null" json:"down_bytes"`
	UpPackets   uint64    `gorm:"not null" json:"up_packets"`
	DownPackets uint64    `gorm:"not null" json:"down_packets"`
	Conns       uint64    `gorm:"not null" json:"conns"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (KernelForwardTraffic) TableName() string { return "v4_kernel_forward_traffic" }

// KernelForwardRequest records an applied ForwardControl write by its
// request id, with a hash of the request and the response, so a retry
// answers the same response once.
type KernelForwardRequest struct {
	RequestID    string    `gorm:"primaryKey;size:128" json:"request_id"`
	Method       string    `gorm:"size:32;not null" json:"method"`
	RouteID      string    `gorm:"size:64;not null;default:''" json:"route_id"`
	RequestHash  string    `gorm:"size:64;not null" json:"request_hash"`
	ResponseJSON string    `gorm:"type:text;not null" json:"response"`
	CreatedAt    time.Time `gorm:"not null;index" json:"created_at"`
}

func (KernelForwardRequest) TableName() string { return "v4_kernel_forward_request" }

// KernelForwardPlan is the one row every plan locks, so plans run one at a
// time across Control processes, and the outcome of the last one: Revision
// counts accepted plans; Refused, with ViolationsJSON, says the last
// replan (after an inventory change) was refused and every node kept its
// state.
type KernelForwardPlan struct {
	ID             string     `gorm:"primaryKey;size:16" json:"id"`
	Revision       uint64     `gorm:"not null;default:0" json:"revision"`
	Refused        bool       `gorm:"not null;default:false" json:"refused"`
	Reason         string     `gorm:"size:64;not null;default:''" json:"reason"`
	ViolationsJSON string     `gorm:"type:text;not null;default:''" json:"violations"`
	WarningsJSON   string     `gorm:"type:text;not null;default:''" json:"warnings"`
	PlannedAt      *time.Time `json:"planned_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
}

func (KernelForwardPlan) TableName() string { return "v4_kernel_forward_plan" }

// KernelForwardModels are the kernel's forwarding tables.
func KernelForwardModels() []any {
	return []any{
		&KernelForwardRoute{}, &KernelForwardAllocation{}, &KernelForwardNode{}, &KernelForwardNodeState{},
		&KernelForwardNodeReport{}, &KernelForwardCounter{}, &KernelForwardTraffic{}, &KernelForwardRequest{},
		&KernelForwardPlan{}, &KernelForwardDNSProvider{}, &KernelForwardDNSBinding{}, &KernelForwardDNSNode{},
	}
}
