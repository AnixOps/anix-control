package model

import (
	"math"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/proto"
)

// FromProto converts a contract route. A nil route converts to the zero
// Route.
func FromProto(p *forwardv1.Route) Route {
	if p == nil {
		return Route{}
	}
	r := Route{
		ID:        p.GetId(),
		Owner:     p.GetOwner(),
		Name:      p.GetName(),
		Listen:    ListenFromProto(p.GetListen()),
		Policy:    PolicyFromProto(p.GetPolicy()),
		Limits:    LimitsFromProto(p.GetLimits()),
		Labels:    cloneMap(p.GetLabels()),
		Paused:    p.GetPaused(),
		Revision:  p.GetRevision(),
		CreatedAt: timeFromMillis(p.GetCreatedAtUnixMs()),
		UpdatedAt: timeFromMillis(p.GetUpdatedAtUnixMs()),
	}
	if hops := p.GetHops(); hops != nil {
		r.Hops = make([]Hop, len(hops))
		for i, h := range hops {
			r.Hops[i] = HopFromProto(h)
		}
	}
	if targets := p.GetTargets(); targets != nil {
		r.Targets = make([]Target, len(targets))
		for i, t := range targets {
			r.Targets[i] = TargetFromProto(t)
		}
	}
	return r
}

// ToProto converts the route to the contract.
func (r *Route) ToProto() *forwardv1.Route {
	p := &forwardv1.Route{
		Id:              r.ID,
		Owner:           r.Owner,
		Name:            r.Name,
		Listen:          r.Listen.ToProto(),
		Policy:          r.Policy.ToProto(),
		Limits:          r.Limits.ToProto(),
		Labels:          cloneMap(r.Labels),
		Paused:          r.Paused,
		Revision:        r.Revision,
		CreatedAtUnixMs: millisFromTime(r.CreatedAt),
		UpdatedAtUnixMs: millisFromTime(r.UpdatedAt),
	}
	if r.Hops != nil {
		p.Hops = make([]*forwardv1.Hop, len(r.Hops))
		for i := range r.Hops {
			p.Hops[i] = r.Hops[i].ToProto()
		}
	}
	if r.Targets != nil {
		p.Targets = make([]*forwardv1.Target, len(r.Targets))
		for i := range r.Targets {
			p.Targets[i] = r.Targets[i].ToProto()
		}
	}
	return p
}

// ListenFromProto converts a contract listener.
func ListenFromProto(p *forwardv1.Listen) Listen {
	return Listen{
		Address:       p.GetAddress(),
		Port:          p.GetPort(),
		Protocol:      L4Protocol(p.GetProtocol()),
		EntryHostname: p.GetEntryHostname(),
	}
}

// ToProto converts the listener; an empty one converts to nil.
func (l Listen) ToProto() *forwardv1.Listen {
	return nonEmpty(&forwardv1.Listen{
		Address:       l.Address,
		Port:          l.Port,
		Protocol:      forwardv1.L4Protocol(l.Protocol),
		EntryHostname: l.EntryHostname,
	})
}

// HopFromProto converts a contract hop.
func HopFromProto(p *forwardv1.Hop) Hop {
	return Hop{
		Role:        HopRole(p.GetRole()),
		Engine:      Engine(p.GetEngine()),
		NodeRefs:    cloneSlice(p.GetNodeRefs()),
		Ingress:     LinkTransportFromProto(p.GetIngress()),
		Port:        p.GetPort(),
		DialAddress: p.GetDialAddress(),
	}
}

// ToProto converts the hop. A hop is always present in its list, so an
// empty hop converts to an empty message, not nil.
func (h *Hop) ToProto() *forwardv1.Hop {
	return &forwardv1.Hop{
		Role:        forwardv1.HopRole(h.Role),
		Engine:      forwardv1.Engine(h.Engine),
		NodeRefs:    cloneSlice(h.NodeRefs),
		Ingress:     h.Ingress.ToProto(),
		Port:        h.Port,
		DialAddress: h.DialAddress,
	}
}

// LinkTransportFromProto converts a contract link transport.
func LinkTransportFromProto(p *forwardv1.LinkTransport) LinkTransport {
	return LinkTransport{
		Security:   LinkSecurity(p.GetSecurity()),
		Mux:        p.GetMux(),
		ServerName: p.GetServerName(),
		Path:       p.GetPath(),
	}
}

// ToProto converts the link transport; an empty one converts to nil.
func (t LinkTransport) ToProto() *forwardv1.LinkTransport {
	return nonEmpty(&forwardv1.LinkTransport{
		Security:   forwardv1.LinkSecurity(t.Security),
		Mux:        t.Mux,
		ServerName: t.ServerName,
		Path:       t.Path,
	})
}

// TargetFromProto converts a contract target.
func TargetFromProto(p *forwardv1.Target) Target {
	return Target{Host: p.GetHost(), Port: p.GetPort(), Weight: p.GetWeight(), Priority: p.GetPriority()}
}

// ToProto converts the target; it is always present in its list.
func (t *Target) ToProto() *forwardv1.Target {
	return &forwardv1.Target{Host: t.Host, Port: t.Port, Weight: t.Weight, Priority: t.Priority}
}

// PolicyFromProto converts a contract policy.
func PolicyFromProto(p *forwardv1.Policy) Policy {
	return Policy{
		NextHop:        BalanceStrategy(p.GetNextHop()),
		Target:         BalanceStrategy(p.GetTarget()),
		Health:         HealthCheckFromProto(p.GetHealth()),
		CircuitBreaker: CircuitBreakerFromProto(p.GetCircuitBreaker()),
		Direct:         DirectMode(p.GetDirect()),
		TargetPolicy:   TargetPolicy(p.GetTargetPolicy()),
	}
}

// ToProto converts the policy; an empty one converts to nil.
func (p Policy) ToProto() *forwardv1.Policy {
	return nonEmpty(&forwardv1.Policy{
		NextHop:        forwardv1.BalanceStrategy(p.NextHop),
		Target:         forwardv1.BalanceStrategy(p.Target),
		Health:         p.Health.ToProto(),
		CircuitBreaker: p.CircuitBreaker.ToProto(),
		Direct:         forwardv1.DirectMode(p.Direct),
		TargetPolicy:   forwardv1.TargetPolicy(p.TargetPolicy),
	})
}

// HealthCheckFromProto converts a contract health check.
func HealthCheckFromProto(p *forwardv1.HealthCheck) HealthCheck {
	return HealthCheck{
		Interval: durationFromMillis(p.GetIntervalMs()),
		Timeout:  durationFromMillis(p.GetTimeoutMs()),
		Disabled: p.GetDisabled(),
	}
}

// ToProto converts the health check; an empty one converts to nil.
func (h HealthCheck) ToProto() *forwardv1.HealthCheck {
	return nonEmpty(&forwardv1.HealthCheck{
		IntervalMs: millisFromDuration(h.Interval),
		TimeoutMs:  millisFromDuration(h.Timeout),
		Disabled:   h.Disabled,
	})
}

// CircuitBreakerFromProto converts a contract circuit breaker.
func CircuitBreakerFromProto(p *forwardv1.CircuitBreaker) CircuitBreaker {
	return CircuitBreaker{FailureThreshold: p.GetFailureThreshold(), OpenFor: durationFromMillis(p.GetOpenMs())}
}

// ToProto converts the circuit breaker; an empty one converts to nil.
func (c CircuitBreaker) ToProto() *forwardv1.CircuitBreaker {
	return nonEmpty(&forwardv1.CircuitBreaker{FailureThreshold: c.FailureThreshold, OpenMs: millisFromDuration(c.OpenFor)})
}

// LimitsFromProto converts contract limits.
func LimitsFromProto(p *forwardv1.Limits) Limits {
	return Limits{
		BandwidthBPS: p.GetBandwidthBps(),
		QuotaBytes:   p.GetQuotaBytes(),
		MaxConns:     p.GetMaxConns(),
		ExpiresAt:    timeFromMillis(p.GetExpiresAtUnixMs()),
	}
}

// ToProto converts the limits; no limit converts to nil.
func (l Limits) ToProto() *forwardv1.Limits {
	return nonEmpty(&forwardv1.Limits{
		BandwidthBps:    l.BandwidthBPS,
		QuotaBytes:      l.QuotaBytes,
		MaxConns:        l.MaxConns,
		ExpiresAtUnixMs: millisFromTime(l.ExpiresAt),
	})
}

// CountersFromProto converts contract counters.
func CountersFromProto(p *forwardv1.Counters) Counters {
	return Counters{
		RouteID:      p.GetRouteId(),
		HopIndex:     p.GetHopIndex(),
		NodeRef:      p.GetNodeRef(),
		UpBytes:      p.GetUpBytes(),
		DownBytes:    p.GetDownBytes(),
		UpPackets:    p.GetUpPackets(),
		DownPackets:  p.GetDownPackets(),
		ActiveConns:  p.GetActiveConns(),
		TotalConns:   p.GetTotalConns(),
		CounterEpoch: p.GetCounterEpoch(),
		ObservedAt:   timeFromMillis(p.GetObservedAtUnixMs()),
	}
}

// ToProto converts the counters.
func (c *Counters) ToProto() *forwardv1.Counters {
	return &forwardv1.Counters{
		RouteId:          c.RouteID,
		HopIndex:         c.HopIndex,
		NodeRef:          c.NodeRef,
		UpBytes:          c.UpBytes,
		DownBytes:        c.DownBytes,
		UpPackets:        c.UpPackets,
		DownPackets:      c.DownPackets,
		ActiveConns:      c.ActiveConns,
		TotalConns:       c.TotalConns,
		CounterEpoch:     c.CounterEpoch,
		ObservedAtUnixMs: millisFromTime(c.ObservedAt),
	}
}

// NodeInfoFromProto converts a contract node.
func NodeInfoFromProto(p *forwardv1.NodeInfo) NodeInfo {
	n := NodeInfo{
		NodeRef:   p.GetNodeRef(),
		Addresses: cloneSlice(p.GetAddresses()),
		PortRange: PortRange{First: p.GetPortRange().GetFirst(), Last: p.GetPortRange().GetLast()},
		Labels:    cloneMap(p.GetLabels()),
	}
	if engines := p.GetEngines(); engines != nil {
		n.Engines = make([]EngineCapabilities, len(engines))
		for i, e := range engines {
			n.Engines[i] = EngineCapabilitiesFromProto(e)
		}
	}
	return n
}

// NodesFromProto converts a list of contract nodes, such as
// PlanRouteRequest.nodes.
func NodesFromProto(nodes []*forwardv1.NodeInfo) []NodeInfo {
	out := make([]NodeInfo, len(nodes))
	for i, n := range nodes {
		out[i] = NodeInfoFromProto(n)
	}
	return out
}

// ToProto converts the node.
func (n *NodeInfo) ToProto() *forwardv1.NodeInfo {
	p := &forwardv1.NodeInfo{
		NodeRef:   n.NodeRef,
		Addresses: cloneSlice(n.Addresses),
		PortRange: nonEmpty(&forwardv1.PortRange{First: n.PortRange.First, Last: n.PortRange.Last}),
		Labels:    cloneMap(n.Labels),
	}
	if n.Engines != nil {
		p.Engines = make([]*forwardv1.EngineCapabilities, len(n.Engines))
		for i := range n.Engines {
			p.Engines[i] = n.Engines[i].ToProto()
		}
	}
	return p
}

// EngineCapabilitiesFromProto converts contract engine capabilities.
func EngineCapabilitiesFromProto(p *forwardv1.EngineCapabilities) EngineCapabilities {
	c := EngineCapabilities{
		Engine:            Engine(p.GetEngine()),
		Version:           p.GetVersion(),
		Available:         p.GetAvailable(),
		UnavailableReason: p.GetUnavailableReason(),
		IPv6:              p.GetIpv6(),
		UDP:               p.GetUdp(),
		BandwidthLimit:    p.GetBandwidthLimit(),
		Quota:             p.GetQuota(),
		MaxConns:          p.GetMaxConns(),
	}
	if s := p.GetStrategies(); s != nil {
		c.Strategies = make([]BalanceStrategy, len(s))
		for i, v := range s {
			c.Strategies[i] = BalanceStrategy(v)
		}
	}
	if s := p.GetLinkSecurities(); s != nil {
		c.LinkSecurities = make([]LinkSecurity, len(s))
		for i, v := range s {
			c.LinkSecurities[i] = LinkSecurity(v)
		}
	}
	return c
}

// ToProto converts the engine capabilities.
func (c *EngineCapabilities) ToProto() *forwardv1.EngineCapabilities {
	p := &forwardv1.EngineCapabilities{
		Engine:            forwardv1.Engine(c.Engine),
		Version:           c.Version,
		Available:         c.Available,
		UnavailableReason: c.UnavailableReason,
		Ipv6:              c.IPv6,
		Udp:               c.UDP,
		BandwidthLimit:    c.BandwidthLimit,
		Quota:             c.Quota,
		MaxConns:          c.MaxConns,
	}
	if c.Strategies != nil {
		p.Strategies = make([]forwardv1.BalanceStrategy, len(c.Strategies))
		for i, v := range c.Strategies {
			p.Strategies[i] = forwardv1.BalanceStrategy(v)
		}
	}
	if c.LinkSecurities != nil {
		p.LinkSecurities = make([]forwardv1.LinkSecurity, len(c.LinkSecurities))
		for i, v := range c.LinkSecurities {
			p.LinkSecurities[i] = forwardv1.LinkSecurity(v)
		}
	}
	return p
}

// nonEmpty answers m, or nil when m has no field set: the contract does not
// tell an absent sub-message from an empty one.
func nonEmpty[M proto.Message](m M) M {
	if proto.Size(m) == 0 {
		var none M
		return none
	}
	return m
}

func timeFromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func millisFromTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func durationFromMillis(ms uint32) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// millisFromDuration answers d in whole milliseconds, clamped to the
// contract's uint32 range.
func millisFromDuration(d time.Duration) uint32 {
	ms := d.Milliseconds()
	switch {
	case ms <= 0:
		return 0
	case ms >= math.MaxUint32:
		return math.MaxUint32
	default:
		return uint32(ms)
	}
}

func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append(make([]T, 0, len(s)), s...)
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
