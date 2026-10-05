package model

import "time"

// Route is one forwarding route (forwardv1.Route).
type Route struct {
	// ID is a ULID assigned by Control; empty before the route is stored.
	ID string
	// Owner is "admin" or "user:<id>".
	Owner  string
	Name   string
	Listen Listen
	// Hops are ordered from the entry: ENTRY, then RELAY hops, then EXIT.
	Hops []Hop
	// Targets are dialled by the last hop.
	Targets   []Target
	Policy    Policy
	Limits    Limits
	Labels    map[string]string
	Paused    bool
	Revision  uint64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsAdmin reports whether the route belongs to an administrator.
func (r *Route) IsAdmin() bool { return r.Owner == OwnerAdmin }

// Entry answers the entry hop, or false for a route without hops.
func (r *Route) Entry() (Hop, bool) {
	if len(r.Hops) == 0 {
		return Hop{}, false
	}
	return r.Hops[0], true
}

// OwnerAdmin is the owner of an administrator's route; a user's route is
// owned by OwnerUserPrefix followed by the user's id.
const (
	OwnerAdmin      = "admin"
	OwnerUserPrefix = "user:"
)

// Listen is a client-facing listener (forwardv1.Listen).
type Listen struct {
	// Address is the local address to bind; empty for every address.
	Address string
	// Port is 0 for "the planner allocates one".
	Port     uint32
	Protocol L4Protocol
	// EntryHostname is the DNS name clients use (entry HA).
	EntryHostname string
}

// Hop is one stage of a route (forwardv1.Hop).
type Hop struct {
	Role   HopRole
	Engine Engine
	// NodeRefs are the Agent identity names of the nodes serving the hop
	// ("forward-<id>", "proxy-<id>").
	NodeRefs []string
	// Ingress is how the previous hop reaches this one; ignored on ENTRY.
	Ingress LinkTransport
	// Port is where this hop listens for the previous one; 0 allocates.
	// Ignored on ENTRY.
	Port uint32
	// DialAddress overrides the address the previous hop dials; only with
	// one node.
	DialAddress string
}

// LinkTransport is the transport between two hops
// (forwardv1.LinkTransport).
type LinkTransport struct {
	Security LinkSecurity
	Mux      bool
	// ServerName is the TLS SNI and HTTP host; empty for the node's Agent
	// identity name.
	ServerName string
	// Path is the WebSocket path or gRPC service name.
	Path string
	// Carrier is the carrier of an ANIXOPS link; unspecified means AUTO.
	Carrier AnixOpsCarrier
}

// Target is a destination behind the last hop (forwardv1.Target).
type Target struct {
	// Host is an IPv4 or IPv6 address or a DNS name.
	Host string
	Port uint32
	// Weight is used by ROUND_ROBIN, RANDOM and LEAST_CONN; 0 means 1.
	Weight uint32
	// Priority orders FAILOVER: lower first, ties broken by order.
	Priority uint32
}

// Policy is a route's balancing, health and failover behaviour
// (forwardv1.Policy). WithDefaults fills in unset fields.
type Policy struct {
	// NextHop balances over a RELAY or EXIT hop's nodes (exit level).
	NextHop BalanceStrategy
	// Target balances over the targets (target level).
	Target         BalanceStrategy
	Health         HealthCheck
	CircuitBreaker CircuitBreaker
	Direct         DirectMode
	TargetPolicy   TargetPolicy
	// ProxyProtocol is the PROXY header the last hop writes toward the
	// targets; unspecified means off.
	ProxyProtocol ProxyProtocol
}

// HealthCheck is the active check each node runs against its upstreams
// (forwardv1.HealthCheck). Zero durations mean the defaults.
type HealthCheck struct {
	Interval time.Duration
	Timeout  time.Duration
	// Disabled turns active checks off; passive failures still count.
	Disabled bool
}

// CircuitBreaker takes a failing upstream out of rotation
// (forwardv1.CircuitBreaker). Zero fields mean the defaults.
type CircuitBreaker struct {
	FailureThreshold uint32
	// OpenFor is how long the upstream is skipped before one trial.
	OpenFor time.Duration
}

// Limits are a route's enforced limits (forwardv1.Limits). The planner puts
// them on the ENTRY hop only. Zero means "no limit" and, for ExpiresAt,
// "never".
type Limits struct {
	// BandwidthBPS caps each direction, per entry node.
	BandwidthBPS uint64
	// QuotaBytes caps upload plus download over the route's life, across
	// every entry node.
	QuotaBytes uint64
	// MaxConns caps concurrent connections, per entry node.
	MaxConns  uint32
	ExpiresAt time.Time
}

// IsZero reports whether no limit is set.
func (l Limits) IsZero() bool {
	return l.BandwidthBPS == 0 && l.QuotaBytes == 0 && l.MaxConns == 0 && l.ExpiresAt.IsZero()
}

// Counters are a hop's raw, cumulative traffic on one node
// (forwardv1.Counters). Up is client to target, down is target to client.
type Counters struct {
	RouteID      string
	HopIndex     uint32
	NodeRef      string
	UpBytes      uint64
	DownBytes    uint64
	UpPackets    uint64
	DownPackets  uint64
	ActiveConns  uint32
	TotalConns   uint64
	CounterEpoch string
	ObservedAt   time.Time
}

// NodeInfo is what the planner and validation know about a node
// (forwardv1.NodeInfo).
type NodeInfo struct {
	NodeRef string
	// Addresses the node is reachable at, primary first.
	Addresses []string
	PortRange PortRange
	Engines   []EngineCapabilities
	Labels    map[string]string
}

// Engine answers the node's capabilities for engine e, or false when the
// node does not advertise it.
func (n *NodeInfo) Engine(e Engine) (EngineCapabilities, bool) {
	for _, caps := range n.Engines {
		if caps.Engine == e {
			return caps, true
		}
	}
	return EngineCapabilities{}, false
}

// PortRange is the inclusive range the planner may allocate from.
type PortRange struct {
	First uint32
	Last  uint32
}

// IsZero reports whether no range is set.
func (p PortRange) IsZero() bool { return p.First == 0 && p.Last == 0 }

// Contains reports whether port is in the range.
func (p PortRange) Contains(port uint32) bool { return port >= p.First && port <= p.Last }

// EngineCapabilities is what one driver on a node can do
// (forwardv1.EngineCapabilities).
type EngineCapabilities struct {
	Engine            Engine
	Version           string
	Available         bool
	UnavailableReason string
	IPv6              bool
	UDP               bool
	Strategies        []BalanceStrategy
	LinkSecurities    []LinkSecurity
	BandwidthLimit    bool
	Quota             bool
	MaxConns          bool
	// Carriers are the concrete carriers an ANIXOPS driver serves and dials
	// (TLS_TCP, QUIC, PLAIN); AUTO is never listed.
	Carriers []AnixOpsCarrier
	// ProxyProtocol is true when the driver writes PROXY protocol v2
	// toward targets.
	ProxyProtocol bool
	// ProtocolVersions are the wire versions of the engine's link protocol
	// the node speaks.
	ProtocolVersions []uint32
}

// Supports reports whether the driver offers strategy b.
func (c EngineCapabilities) Supports(b BalanceStrategy) bool {
	for _, s := range c.Strategies {
		if s == b {
			return true
		}
	}
	return false
}

// SupportsLink reports whether the driver can originate and terminate s.
func (c EngineCapabilities) SupportsLink(s LinkSecurity) bool {
	for _, l := range c.LinkSecurities {
		if l == s {
			return true
		}
	}
	return false
}

// SupportsCarrier reports whether the driver lists carrier x.
func (c EngineCapabilities) SupportsCarrier(x AnixOpsCarrier) bool {
	for _, v := range c.Carriers {
		if v == x {
			return true
		}
	}
	return false
}

// SharesProtocolVersion reports whether c and o speak a common wire
// version.
func (c EngineCapabilities) SharesProtocolVersion(o EngineCapabilities) bool {
	for _, v := range c.ProtocolVersions {
		for _, w := range o.ProtocolVersions {
			if v == w {
				return true
			}
		}
	}
	return false
}
