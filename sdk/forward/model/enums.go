package model

import forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"

// Engine is what moves a hop's traffic on its node (forwardv1.Engine).
type Engine int32

const (
	EngineUnspecified Engine = Engine(forwardv1.Engine_ENGINE_UNSPECIFIED)
	// EngineNFTables is kernel nftables DNAT: RAW links only.
	EngineNFTables Engine = Engine(forwardv1.Engine_ENGINE_NFTABLES)
	// EngineGost is gost v3, managed by the Agent.
	EngineGost Engine = Engine(forwardv1.Engine_ENGINE_GOST)
	// EngineAnixOps is the AnixOps relay protocol (v4.3).
	EngineAnixOps Engine = Engine(forwardv1.Engine_ENGINE_ANIXOPS)
)

// String answers the contract's name, or the number for an unknown value.
func (e Engine) String() string { return forwardv1.Engine(e).String() }

// IsKnown reports whether e is one of the contract's values.
func (e Engine) IsKnown() bool { _, ok := forwardv1.Engine_name[int32(e)]; return ok }

// HopRole is a hop's place in a route (forwardv1.HopRole). Nodes have no
// fixed role: roles belong to hops, so one node can be the entry of one
// route and the exit of another.
type HopRole int32

const (
	HopRoleUnspecified HopRole = HopRole(forwardv1.HopRole_HOP_ROLE_UNSPECIFIED)
	HopRoleEntry       HopRole = HopRole(forwardv1.HopRole_HOP_ROLE_ENTRY)
	HopRoleRelay       HopRole = HopRole(forwardv1.HopRole_HOP_ROLE_RELAY)
	HopRoleExit        HopRole = HopRole(forwardv1.HopRole_HOP_ROLE_EXIT)
)

func (r HopRole) String() string { return forwardv1.HopRole(r).String() }

// IsKnown reports whether r is one of the contract's values.
func (r HopRole) IsKnown() bool { _, ok := forwardv1.HopRole_name[int32(r)]; return ok }

// L4Protocol is what a listener accepts (forwardv1.L4Protocol).
type L4Protocol int32

const (
	L4ProtocolUnspecified L4Protocol = L4Protocol(forwardv1.L4Protocol_L4_PROTOCOL_UNSPECIFIED)
	L4ProtocolTCP         L4Protocol = L4Protocol(forwardv1.L4Protocol_L4_PROTOCOL_TCP)
	L4ProtocolUDP         L4Protocol = L4Protocol(forwardv1.L4Protocol_L4_PROTOCOL_UDP)
	L4ProtocolTCPUDP      L4Protocol = L4Protocol(forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP)
)

func (p L4Protocol) String() string { return forwardv1.L4Protocol(p).String() }

// IsKnown reports whether p is one of the contract's values.
func (p L4Protocol) IsKnown() bool { _, ok := forwardv1.L4Protocol_name[int32(p)]; return ok }

// HasUDP reports whether the listener accepts UDP.
func (p L4Protocol) HasUDP() bool { return p == L4ProtocolUDP || p == L4ProtocolTCPUDP }

// LinkSecurity is how one hop carries traffic to the next
// (forwardv1.LinkSecurity).
type LinkSecurity int32

const (
	LinkSecurityUnspecified LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_UNSPECIFIED)
	LinkSecurityRaw         LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_RAW)
	LinkSecurityTLS         LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_TLS)
	LinkSecurityWSS         LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_WSS)
	LinkSecurityQUIC        LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_QUIC)
	LinkSecurityGRPC        LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_GRPC)
	LinkSecurityAnixOps     LinkSecurity = LinkSecurity(forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS)
)

func (s LinkSecurity) String() string { return forwardv1.LinkSecurity(s).String() }

// IsKnown reports whether s is one of the contract's values.
func (s LinkSecurity) IsKnown() bool { _, ok := forwardv1.LinkSecurity_name[int32(s)]; return ok }

// BalanceStrategy picks an upstream for a new connection
// (forwardv1.BalanceStrategy).
type BalanceStrategy int32

const (
	BalanceUnspecified BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_UNSPECIFIED)
	BalanceRoundRobin  BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN)
	BalanceRandom      BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM)
	BalanceIPHash      BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH)
	BalanceLeastConn   BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN)
	BalanceFailover    BalanceStrategy = BalanceStrategy(forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER)
)

func (b BalanceStrategy) String() string { return forwardv1.BalanceStrategy(b).String() }

// IsKnown reports whether b is one of the contract's values.
func (b BalanceStrategy) IsKnown() bool { _, ok := forwardv1.BalanceStrategy_name[int32(b)]; return ok }

// DirectMode is the private-line "direct from entry" policy
// (forwardv1.DirectMode).
type DirectMode int32

const (
	DirectUnspecified DirectMode = DirectMode(forwardv1.DirectMode_DIRECT_MODE_UNSPECIFIED)
	DirectOff         DirectMode = DirectMode(forwardv1.DirectMode_DIRECT_MODE_OFF)
	DirectPreferred   DirectMode = DirectMode(forwardv1.DirectMode_DIRECT_MODE_PREFERRED)
	DirectForced      DirectMode = DirectMode(forwardv1.DirectMode_DIRECT_MODE_FORCED)
)

func (d DirectMode) String() string { return forwardv1.DirectMode(d).String() }

// IsKnown reports whether d is one of the contract's values.
func (d DirectMode) IsKnown() bool { _, ok := forwardv1.DirectMode_name[int32(d)]; return ok }

// TargetPolicy says which target addresses a route may reach
// (forwardv1.TargetPolicy).
type TargetPolicy int32

const (
	TargetPolicyUnspecified TargetPolicy = TargetPolicy(forwardv1.TargetPolicy_TARGET_POLICY_UNSPECIFIED)
	// TargetPolicyPublicOnly refuses loopback, private, link-local,
	// multicast and other special-purpose addresses.
	TargetPolicyPublicOnly TargetPolicy = TargetPolicy(forwardv1.TargetPolicy_TARGET_POLICY_PUBLIC_ONLY)
	// TargetPolicyAllowPrivate also allows private addresses, on an
	// administrator's route only. Never loopback.
	TargetPolicyAllowPrivate TargetPolicy = TargetPolicy(forwardv1.TargetPolicy_TARGET_POLICY_ALLOW_PRIVATE)
)

func (t TargetPolicy) String() string { return forwardv1.TargetPolicy(t).String() }

// IsKnown reports whether t is one of the contract's values.
func (t TargetPolicy) IsKnown() bool { _, ok := forwardv1.TargetPolicy_name[int32(t)]; return ok }

// AnixOpsCarrier is the transport carrier of a LINK_SECURITY_ANIXOPS link
// (forwardv1.AnixOpsCarrier). Unspecified means CarrierAuto.
type AnixOpsCarrier int32

const (
	CarrierUnspecified AnixOpsCarrier = AnixOpsCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_UNSPECIFIED)
	CarrierAuto        AnixOpsCarrier = AnixOpsCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_AUTO)
	CarrierTLSTCP      AnixOpsCarrier = AnixOpsCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP)
	CarrierQUIC        AnixOpsCarrier = AnixOpsCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC)
	CarrierPlain       AnixOpsCarrier = AnixOpsCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN)
)

func (c AnixOpsCarrier) String() string { return forwardv1.AnixOpsCarrier(c).String() }

// IsKnown reports whether c is one of the contract's values.
func (c AnixOpsCarrier) IsKnown() bool { _, ok := forwardv1.AnixOpsCarrier_name[int32(c)]; return ok }

// Effective answers the carrier a link uses: Unspecified is Auto.
func (c AnixOpsCarrier) Effective() AnixOpsCarrier {
	if c == CarrierUnspecified {
		return CarrierAuto
	}
	return c
}

// ProxyProtocol says whether the last hop writes a PROXY protocol header
// toward the targets (forwardv1.ProxyProtocol). Unspecified means off.
type ProxyProtocol int32

const (
	ProxyProtocolUnspecified ProxyProtocol = ProxyProtocol(forwardv1.ProxyProtocol_PROXY_PROTOCOL_UNSPECIFIED)
	ProxyProtocolOff         ProxyProtocol = ProxyProtocol(forwardv1.ProxyProtocol_PROXY_PROTOCOL_OFF)
	ProxyProtocolV2          ProxyProtocol = ProxyProtocol(forwardv1.ProxyProtocol_PROXY_PROTOCOL_V2)
)

func (p ProxyProtocol) String() string { return forwardv1.ProxyProtocol(p).String() }

// IsKnown reports whether p is one of the contract's values.
func (p ProxyProtocol) IsKnown() bool { _, ok := forwardv1.ProxyProtocol_name[int32(p)]; return ok }

// Enabled reports whether a header is written.
func (p ProxyProtocol) Enabled() bool { return p == ProxyProtocolV2 }
