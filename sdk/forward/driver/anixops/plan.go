package anixops

import (
	"cmp"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// routeIDPattern is what a route id must look like to go into the relay's
// configuration: Control's ids are ULIDs; anything else is refused rather than
// escaped, so no input reaches the configuration except as a checked literal.
var routeIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)

// serverNamePattern is a DNS name a link's dialler verifies.
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// transport is one end of a link between two hops, reduced to what the relay
// needs.
type transport struct {
	anixops    bool
	carrier    string
	serverName string
}

// planHop checks one hop of the driver's engine and reduces it to the relay's
// hop. The order of the checks decides which error a hop with several problems
// gets; unknown enumerations always come first, as ErrUnsupported.
func (d *Driver) planHop(h *forwardv1.NodeHop) (relayctl.Hop, error) {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrInvalidState, fmt.Sprintf(format, args...))
	}
	unsupported := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrUnsupported, fmt.Sprintf(format, args...))
	}
	k := driver.KeyOf(h)
	if !routeIDPattern.MatchString(k.RouteID) {
		return relayctl.Hop{}, invalid("route id must be 1 to 64 letters and digits")
	}
	out := relayctl.Hop{Route: k.RouteID, Hop: k.HopIndex, Paused: h.GetPaused()}

	role := h.GetRole()
	switch {
	case role == forwardv1.HopRole_HOP_ROLE_UNSPECIFIED:
		return out, invalid("role unspecified")
	case !model.HopRole(role).IsKnown():
		return out, unsupported("unknown role %d", role)
	}
	out.Role = map[forwardv1.HopRole]string{
		forwardv1.HopRole_HOP_ROLE_ENTRY: "entry",
		forwardv1.HopRole_HOP_ROLE_RELAY: "relay",
		forwardv1.HopRole_HOP_ROLE_EXIT:  "exit",
	}[role]

	// Enumerations first, so unknown values are always ErrUnsupported.
	l := h.GetListen()
	proto := l.GetProtocol()
	if proto != forwardv1.L4Protocol_L4_PROTOCOL_UNSPECIFIED && !model.L4Protocol(proto).IsKnown() {
		return out, unsupported("unknown listen protocol %d", proto)
	}
	ingress, err := d.checkTransport("ingress", h.GetIngress())
	if err != nil {
		return out, err
	}
	egress := make([]transport, len(h.GetUpstreams()))
	for i, u := range h.GetUpstreams() {
		if egress[i], err = d.checkTransport(fmt.Sprintf("upstreams[%d].egress", i), u.GetEgress()); err != nil {
			return out, err
		}
	}
	balance := h.GetBalance()
	if balance == forwardv1.BalanceStrategy_BALANCE_STRATEGY_UNSPECIFIED {
		balance = forwardv1.BalanceStrategy(model.DefaultBalance)
	}
	if !model.BalanceStrategy(balance).IsKnown() {
		return out, unsupported("unknown balance strategy %d", balance)
	}
	if !slices.Contains(d.cfg.Strategies, balance) {
		return out, unsupported("balance strategy %s not enabled", balance)
	}
	out.Balance = balanceName(balance)

	// The listener.
	if l == nil {
		return out, invalid("no listen")
	}
	if l.GetPort() == 0 || l.GetPort() > validate.MaxPort {
		return out, invalid("listen port %d outside 1..%d", l.GetPort(), validate.MaxPort)
	}
	out.Listen.Port = l.GetPort()
	var tcp, udp bool
	switch proto {
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP:
		tcp = true
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		udp = true
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		tcp, udp = true, true
	default:
		return out, invalid("listen protocol unspecified")
	}
	if udp && !d.cfg.UDP {
		return out, unsupported("udp not enabled")
	}
	if a := l.GetAddress(); a != "" {
		addr, err := parseLiteral(a)
		if err != nil {
			return out, invalid("listen address: %v", err)
		}
		if addr.IsUnspecified() || addr.IsMulticast() {
			return out, invalid("listen address %s is not a unicast address", addr)
		}
		if addr.Is6() && !d.cfg.IPv6 {
			return out, unsupported("ipv6 not enabled")
		}
		out.Listen.Address = addr.String()
	}
	if ingress.anixops {
		// The sockets are the carrier's: TCP for TLS_TCP and PLAIN, UDP for
		// QUIC, both at one port for AUTO. The traffic the streams carry
		// (TCP or UDP) is not the listener's business.
		out.Ingress = relayctl.Ingress{Security: relayctl.SecurityAnixOps, Carrier: ingress.carrier}
		out.Listen.TCP = ingress.carrier != relayctl.CarrierQUIC
		out.Listen.UDP = ingress.carrier == relayctl.CarrierQUIC || ingress.carrier == relayctl.CarrierAuto
		if ingress.carrier != relayctl.CarrierPlain && len(h.GetIngressPeers()) == 0 {
			return out, invalid("encrypted ingress without ingress_peers")
		}
		if len(h.GetIngressSources()) == 0 {
			return out, invalid("an AnixOps ingress without ingress sources would be open to every address")
		}
	} else {
		out.Ingress = relayctl.Ingress{Security: relayctl.SecurityRaw}
		out.Listen.TCP, out.Listen.UDP = tcp, udp
	}

	// Upstreams.
	ups := h.GetUpstreams()
	if len(ups) == 0 {
		return out, invalid("no upstream")
	}
	if len(ups) > MaxUpstreams {
		return out, unsupported("%d upstreams, more than %d", len(ups), MaxUpstreams)
	}
	seen := map[netip.AddrPort]bool{}
	direct := 0
	for i, u := range ups {
		addr, err := parseLiteral(u.GetAddress())
		if err != nil {
			if isHostName(u.GetAddress()) {
				return out, unsupported("upstreams[%d]: %q is a name; the Agent resolves names before Render", i, u.GetAddress())
			}
			return out, invalid("upstreams[%d]: %v", i, err)
		}
		if addr.IsLoopback() || addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalUnicast() {
			return out, invalid("upstreams[%d]: %s cannot be forwarded to", i, addr)
		}
		if u.GetNodeRef() == "" {
			if err := validate.CheckTargetAddress(addr, model.TargetPolicy(h.GetTargetPolicy())); err != nil {
				return out, invalid("upstreams[%d]: target %s: %v", i, addr, err)
			}
		}
		if addr.Is6() && !d.cfg.IPv6 {
			return out, unsupported("ipv6 not enabled")
		}
		if u.GetPort() == 0 || u.GetPort() > validate.MaxPort {
			return out, invalid("upstreams[%d]: port %d outside 1..%d", i, u.GetPort(), validate.MaxPort)
		}
		ap := netip.AddrPortFrom(addr, uint16(u.GetPort())) // #nosec G115 -- checked to be a port
		if seen[ap] {
			return out, invalid("upstreams[%d]: %s twice", i, ap)
		}
		seen[ap] = true
		weight := u.GetWeight()
		if weight == 0 {
			weight = model.DefaultWeight
		}
		if weight > validate.MaxWeight {
			return out, invalid("upstreams[%d]: weight %d over %d", i, weight, validate.MaxWeight)
		}
		ru := relayctl.Upstream{Address: addr.String(), Port: u.GetPort(), Weight: weight, Priority: u.GetPriority()}
		if egress[i].anixops {
			name := egress[i].serverName
			if name == "" {
				name = u.GetNodeRef()
			}
			ru.Egress = relayctl.Egress{Security: relayctl.SecurityAnixOps, Carrier: egress[i].carrier, ServerName: name}
			ru.NodeRef = u.GetNodeRef()
			ru.PeerIdentity = u.GetPeerIdentity()
			if egress[i].carrier != relayctl.CarrierPlain {
				// The dialler verifies the listener's certificate for this
				// name and this identity: the planner sets them.
				if !serverNamePattern.MatchString(name) || len(name) > validate.MaxHostnameBytes {
					return out, invalid("upstreams[%d]: encrypted link without a valid server name", i)
				}
				if _, err := link.ParseIdentity(ru.PeerIdentity); err != nil {
					return out, invalid("upstreams[%d]: encrypted link without a valid peer_identity: %v", i, err)
				}
			}
		} else {
			ru.Egress = relayctl.Egress{Security: relayctl.SecurityRaw}
			direct++
		}
		out.Upstreams = append(out.Upstreams, ru)
	}
	if direct != 0 && direct != len(ups) {
		return out, unsupported("the hop mixes RAW upstreams with AnixOps ones; the relay balances one kind per hop")
	}

	// Admission: relay and exit listeners serve the previous hop only.
	if len(h.GetIngressSources()) == 0 && role != forwardv1.HopRole_HOP_ROLE_ENTRY {
		return out, invalid("%s hop without ingress sources would be an open relay", role)
	}
	var prefixes []netip.Prefix
	for i, s := range h.GetIngressSources() {
		p, err := parseSource(s)
		if err != nil {
			return out, invalid("ingress_sources[%d]: %v", i, err)
		}
		prefixes = append(prefixes, p)
	}
	for _, p := range minimalPrefixes(prefixes) {
		out.Sources = append(out.Sources, p.String())
	}
	peers := slices.Clone(h.GetIngressPeers())
	for i, id := range peers {
		if _, err := link.ParseIdentity(id); err != nil {
			return out, invalid("ingress_peers[%d]: %v", i, err)
		}
	}
	slices.Sort(peers)
	out.Peers = slices.Compact(peers)

	// Limits.
	lim := h.GetLimits()
	if lim.GetBandwidthBps() > 0 {
		if !d.cfg.BandwidthLimit {
			return out, unsupported("bandwidth limit not enabled")
		}
		out.Limits.BandwidthBps = lim.GetBandwidthBps()
	}
	if lim.GetQuotaBytes() > 0 {
		if !d.cfg.Quota {
			return out, unsupported("quota not enabled")
		}
		out.Limits.QuotaBytes = lim.GetQuotaBytes()
	}
	if lim.GetMaxConns() > 0 {
		if !d.cfg.MaxConns {
			return out, unsupported("connection limit not enabled")
		}
		out.Limits.MaxConns = lim.GetMaxConns()
	}

	// PROXY protocol v2 toward targets is deferred; capabilities say so, so
	// validation refuses it before it gets here.
	if pp := h.GetProxyProtocol(); pp != forwardv1.ProxyProtocol_PROXY_PROTOCOL_UNSPECIFIED && pp != forwardv1.ProxyProtocol_PROXY_PROTOCOL_OFF {
		return out, unsupported("PROXY protocol %s is not implemented by the anixops relay yet", pp)
	}

	// The circuit breaker, with the decided defaults of model (H21: 3
	// failures, 30 s). The health check is the Agent's (forward-sdk.md
	// section 7.3).
	cb := h.GetCircuitBreaker()
	fails := cmp.Or(cb.GetFailureThreshold(), model.DefaultFailureThreshold)
	if fails > validate.MaxFailureThreshold {
		return out, invalid("circuit_breaker.failure_threshold %d over %d", fails, validate.MaxFailureThreshold)
	}
	open := model.DefaultBreakerOpen
	if ms := cb.GetOpenMs(); ms > 0 {
		open = time.Duration(ms) * time.Millisecond
	}
	if open < validate.MinBreakerOpen || open > validate.MaxBreakerOpen {
		return out, invalid("circuit_breaker.open_ms %d outside %v..%v", cb.GetOpenMs(), validate.MinBreakerOpen, validate.MaxBreakerOpen)
	}
	out.Breaker = relayctl.Breaker{MaxFails: fails, OpenMs: uint32(open / time.Millisecond)} // #nosec G115 -- bounded by MaxBreakerOpen
	return out, nil
}

// checkTransport checks a transport: RAW always, ANIXOPS with a carrier the
// host serves. Other link securities are the other engines'.
func (d *Driver) checkTransport(field string, t *forwardv1.LinkTransport) (transport, error) {
	switch t.GetSecurity() {
	case forwardv1.LinkSecurity_LINK_SECURITY_UNSPECIFIED, forwardv1.LinkSecurity_LINK_SECURITY_RAW:
		if t.GetCarrier() != forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_UNSPECIFIED {
			return transport{}, fmt.Errorf("%w: %s: a carrier needs LINK_SECURITY_ANIXOPS", driver.ErrInvalidState, field)
		}
		if t.GetMux() {
			return transport{}, fmt.Errorf("%w: %s: a RAW link carries one connection per connection; mux needs an engine with a multiplexer link", driver.ErrUnsupported, field)
		}
		return transport{}, d.checkRawLink(field, t)
	case forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS:
	default:
		if !model.LinkSecurity(t.GetSecurity()).IsKnown() {
			return transport{}, fmt.Errorf("%w: %s: unknown link security %d", driver.ErrUnsupported, field, t.GetSecurity())
		}
		return transport{}, fmt.Errorf("%w: %s: the anixops relay does not carry %s", driver.ErrUnsupported, field, t.GetSecurity())
	}
	if t.GetPath() != "" {
		return transport{}, fmt.Errorf("%w: %s: an AnixOps link has no path", driver.ErrInvalidState, field)
	}
	if n := t.GetServerName(); n != "" && (!serverNamePattern.MatchString(n) || len(n) > validate.MaxHostnameBytes) {
		return transport{}, fmt.Errorf("%w: %s: server name %q is not a DNS name", driver.ErrInvalidState, field, n)
	}
	tr := transport{anixops: true, serverName: t.GetServerName()}
	need := forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP
	switch c := t.GetCarrier(); c {
	case forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_UNSPECIFIED, forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_AUTO:
		tr.carrier = relayctl.CarrierAuto // AUTO falls back to TLS_TCP, so it needs it
	case forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP:
		tr.carrier = relayctl.CarrierTLSTCP
	case forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC:
		tr.carrier, need = relayctl.CarrierQUIC, c
	case forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN:
		tr.carrier, need = relayctl.CarrierPlain, c
	default:
		return transport{}, fmt.Errorf("%w: %s: unknown carrier %d", driver.ErrUnsupported, field, c)
	}
	if !d.cfg.hasCarrier(need) {
		why := "is not available on this host"
		if need != forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN && !d.cfg.linkFiles() {
			why = "needs the node's link certificate, which is not configured"
		}
		return transport{}, fmt.Errorf("%w: %s: the %s carrier %s", driver.ErrUnsupported, field, need, why)
	}
	return tr, nil
}

// checkRawLink checks a RAW link's fields, which carry no meaning for it.
func (d *Driver) checkRawLink(field string, t *forwardv1.LinkTransport) error {
	if t.GetPath() != "" {
		return fmt.Errorf("%w: %s: a path needs WSS or GRPC", driver.ErrInvalidState, field)
	}
	return nil
}

func balanceName(s forwardv1.BalanceStrategy) string {
	switch s {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM:
		return relayctl.BalanceRandom
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH:
		return relayctl.BalanceIPHash
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:
		return relayctl.BalanceLeastConn
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:
		return relayctl.BalanceFailover
	}
	return relayctl.BalanceRoundRobin
}

// parseLiteral parses an IP address literal without a zone; IPv4-mapped IPv6
// addresses become IPv4.
func parseLiteral(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%q is not an IP address", s)
	}
	if a.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("%q has a zone", s)
	}
	return a.Unmap(), nil
}

// parseSource parses an address or a prefix without a zone.
func parseSource(s string) (netip.Prefix, error) {
	if a, err := parseLiteral(s); err == nil {
		return netip.PrefixFrom(a, a.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || p.Addr().Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%q is not an IP address or prefix", s)
	}
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("%q is not an IP address or prefix", s)
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), nil
}

// minimalPrefixes sorts prefixes and drops the ones another covers.
func minimalPrefixes(ps []netip.Prefix) []netip.Prefix {
	slices.SortFunc(ps, func(a, b netip.Prefix) int {
		return cmp.Or(a.Addr().Compare(b.Addr()), cmp.Compare(a.Bits(), b.Bits()))
	})
	var out []netip.Prefix
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].Contains(p.Addr()) && out[n-1].Bits() <= p.Bits() {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isHostName reports whether s looks like a DNS name, as opposed to garbage.
func isHostName(s string) bool {
	if s == "" || len(s) > validate.MaxHostnameBytes {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-.") == ""
}

// listenSocket is one socket a hop holds.
type listenSocket struct {
	network string
	port    uint32
}

// sockets answers the sockets of a hop's listener.
func sockets(h relayctl.Hop) []listenSocket {
	var out []listenSocket
	if h.Listen.TCP {
		out = append(out, listenSocket{"tcp", h.Listen.Port})
	}
	if h.Listen.UDP {
		out = append(out, listenSocket{"udp", h.Listen.Port})
	}
	return out
}

// rejectCollisions drops every hop whose listener overlaps another hop's (the
// same socket type and port, and the same or a wildcard address): rejecting
// all of them keeps Render independent of the order of the hops.
func rejectCollisions(hops []relayctl.Hop) ([]relayctl.Hop, []*driver.HopError) {
	by := map[listenSocket][]int{}
	for i, h := range hops {
		for _, s := range sockets(h) {
			by[s] = append(by[s], i)
		}
	}
	bad := map[int]string{}
	for s, idx := range by {
		for x, i := range idx {
			for _, j := range idx[x+1:] {
				a, b := hops[i], hops[j]
				if a.Listen.Address == "" || b.Listen.Address == "" || a.Listen.Address == b.Listen.Address {
					msg := fmt.Sprintf("listener %s port %d overlaps another hop's", s.network, s.port)
					for _, k := range []int{i, j} {
						if _, ok := bad[k]; !ok {
							bad[k] = msg
						}
					}
				}
			}
		}
	}
	var keep []relayctl.Hop
	var errs []*driver.HopError
	for i, h := range hops {
		if msg, ok := bad[i]; ok {
			errs = append(errs, &driver.HopError{Key: driver.HopKey{RouteID: h.Route, HopIndex: h.Hop}, Engine: forwardv1.Engine_ENGINE_ANIXOPS, Err: fmt.Errorf("%w: %s", driver.ErrInvalidState, msg)})
			continue
		}
		keep = append(keep, h)
	}
	return keep, errs
}
