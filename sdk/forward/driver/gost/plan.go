package gost

import (
	"cmp"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// routeIDPattern is what a route id must look like to become part of a
// gost name: Control's ids are ULIDs; anything else is refused rather than
// escaped, so no input reaches the configuration except as a checked
// literal.
var routeIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)

// serverNamePattern is a DNS name a TLS dialer may present and verify.
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// pathPattern is a WebSocket path or gRPC service path.
var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)

// link is one end of a link between two hops, reduced to what gost needs.
type link struct {
	security   forwardv1.LinkSecurity // RAW for an unset transport
	mux        bool
	serverName string // dialer only
	path       string
}

// raw reports whether the link carries the client's bytes as they are:
// RAW without mux. Every other link speaks gost's relay protocol.
func (l link) raw() bool {
	return l.security == forwardv1.LinkSecurity_LINK_SECURITY_RAW && !l.mux
}

// gostType answers the gost listener and dialer type of the link.
func (l link) gostType() string {
	switch l.security {
	case forwardv1.LinkSecurity_LINK_SECURITY_TLS:
		if l.mux {
			return "mtls"
		}
		return "tls"
	case forwardv1.LinkSecurity_LINK_SECURITY_WSS:
		if l.mux {
			return "mwss"
		}
		return "wss"
	case forwardv1.LinkSecurity_LINK_SECURITY_QUIC:
		return "quic" // QUIC multiplexes streams natively
	case forwardv1.LinkSecurity_LINK_SECURITY_GRPC:
		return "grpc" // HTTP/2 multiplexes streams natively
	}
	return "mtcp" // RAW with mux; RAW without mux has no dialer
}

// encrypted reports whether the link is mutual TLS.
func (l link) encrypted() bool { return l.security != forwardv1.LinkSecurity_LINK_SECURITY_RAW }

// upstream is one checked upstream of a hop.
type upstream struct {
	index    int // in the state's upstream list
	addr     netip.Addr
	port     uint32
	weight   uint32
	priority uint32
	egress   link
}

func (u upstream) addrPort() netip.AddrPort {
	return netip.AddrPortFrom(u.addr, uint16(u.port)) // #nosec G115 -- checked to be a port
}

// listener is one gost service of a hop.
type listener struct {
	gostType string // tcp, udp, mtcp, tls, mtls, wss, mwss, quic, grpc
	network  string // the socket: tcp or udp
	handler  string // tcp, udp or relay
}

// hopPlan is one hop checked and reduced to what the configuration needs.
// Every field is a checked literal: numbers, netip values and names derived
// from a checked route id.
type hopPlan struct {
	key  driver.HopKey
	name string // r<route>-h<hop>

	listen    netip.Addr // invalid for every local address
	port      uint32
	tcp, udp  bool
	ingress   link
	listeners []listener

	balance   forwardv1.BalanceStrategy
	upstreams []upstream // in the state's order
	chain     bool       // upstreams are dialled through a chain (not RAW)

	sources      []netip.Prefix // admission; nil admits every source
	paused       bool
	bandwidthBps uint64
	maxConns     uint32
	maxFails     uint32
	failTimeout  time.Duration
}

// hopName answers the name of a hop's gost objects.
func hopName(k driver.HopKey) string {
	return "r" + k.RouteID + "-h" + strconv.FormatUint(uint64(k.HopIndex), 10)
}

// planHop checks one hop of the driver's engine and reduces it to a plan.
// The order of the checks decides which error a hop with several problems
// gets; unknown enumerations always come first, as ErrUnsupported.
func (d *Driver) planHop(h *forwardv1.NodeHop) (*hopPlan, error) {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrInvalidState, fmt.Sprintf(format, args...))
	}
	unsupported := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrUnsupported, fmt.Sprintf(format, args...))
	}
	k := driver.KeyOf(h)
	if !routeIDPattern.MatchString(k.RouteID) {
		return nil, invalid("route id must be 1 to 64 letters and digits")
	}
	p := &hopPlan{key: k, name: hopName(k), paused: h.GetPaused()}

	role := h.GetRole()
	switch {
	case role == forwardv1.HopRole_HOP_ROLE_UNSPECIFIED:
		return nil, invalid("role unspecified")
	case !model.HopRole(role).IsKnown():
		return nil, unsupported("unknown role %d", role)
	}

	// Enumerations first, so unknown values are always ErrUnsupported.
	l := h.GetListen()
	proto := l.GetProtocol()
	if proto != forwardv1.L4Protocol_L4_PROTOCOL_UNSPECIFIED && !model.L4Protocol(proto).IsKnown() {
		return nil, unsupported("unknown listen protocol %d", proto)
	}
	ingress, err := d.checkLink("ingress", h.GetIngress())
	if err != nil {
		return nil, err
	}
	p.ingress = ingress
	egress := make([]link, len(h.GetUpstreams()))
	for i, u := range h.GetUpstreams() {
		if egress[i], err = d.checkLink(fmt.Sprintf("upstreams[%d].egress", i), u.GetEgress()); err != nil {
			return nil, err
		}
	}
	p.balance = h.GetBalance()
	if p.balance == forwardv1.BalanceStrategy_BALANCE_STRATEGY_UNSPECIFIED {
		p.balance = forwardv1.BalanceStrategy(model.DefaultBalance)
	}
	if !model.BalanceStrategy(p.balance).IsKnown() {
		return nil, unsupported("unknown balance strategy %d", p.balance)
	}
	if !slices.Contains(d.cfg.Strategies, p.balance) {
		return nil, unsupported("balance strategy %s not enabled", p.balance)
	}

	// The listener.
	if l == nil {
		return nil, invalid("no listen")
	}
	if l.GetPort() == 0 || l.GetPort() > validate.MaxPort {
		return nil, invalid("listen port %d outside 1..%d", l.GetPort(), validate.MaxPort)
	}
	p.port = l.GetPort()
	switch proto {
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP:
		p.tcp = true
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		p.udp = true
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		p.tcp, p.udp = true, true
	default:
		return nil, invalid("listen protocol unspecified")
	}
	if p.udp && !d.cfg.UDP {
		return nil, unsupported("udp not enabled")
	}
	if a := l.GetAddress(); a != "" {
		addr, err := parseLiteral(a)
		if err != nil {
			return nil, invalid("listen address: %v", err)
		}
		if addr.IsUnspecified() || addr.IsMulticast() {
			return nil, invalid("listen address %s is not a unicast address", addr)
		}
		if addr.Is6() && !d.cfg.IPv6 {
			return nil, unsupported("ipv6 not enabled")
		}
		p.listen = addr
	}
	p.listeners = listenersOf(p)
	if ingress.encrypted() && role != forwardv1.HopRole_HOP_ROLE_ENTRY && len(h.GetIngressPeers()) == 0 {
		return nil, invalid("encrypted ingress without ingress_peers")
	}

	// Upstreams.
	if len(h.GetUpstreams()) == 0 {
		return nil, invalid("no upstream")
	}
	if n := len(h.GetUpstreams()); n > MaxEntries {
		return nil, unsupported("%d upstreams, more than %d", n, MaxEntries)
	}
	seen := map[netip.AddrPort]bool{}
	direct := 0
	for i, u := range h.GetUpstreams() {
		addr, err := parseLiteral(u.GetAddress())
		if err != nil {
			if isHostName(u.GetAddress()) {
				return nil, unsupported("upstreams[%d]: %q is a name; the Agent resolves names before Render", i, u.GetAddress())
			}
			return nil, invalid("upstreams[%d]: %v", i, err)
		}
		if addr.IsLoopback() || addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalUnicast() {
			return nil, invalid("upstreams[%d]: %s cannot be forwarded to", i, addr)
		}
		if u.GetNodeRef() == "" {
			if err := validate.CheckTargetAddress(addr, model.TargetPolicy(h.GetTargetPolicy())); err != nil {
				return nil, invalid("upstreams[%d]: target %s: %v", i, addr, err)
			}
		}
		if addr.Is6() && !d.cfg.IPv6 {
			return nil, unsupported("ipv6 not enabled")
		}
		if u.GetPort() == 0 || u.GetPort() > validate.MaxPort {
			return nil, invalid("upstreams[%d]: port %d outside 1..%d", i, u.GetPort(), validate.MaxPort)
		}
		up := upstream{index: i, addr: addr, port: u.GetPort(), weight: u.GetWeight(), priority: u.GetPriority(), egress: egress[i]}
		if seen[up.addrPort()] {
			return nil, invalid("upstreams[%d]: %s twice", i, up.addrPort())
		}
		seen[up.addrPort()] = true
		if up.weight == 0 {
			up.weight = model.DefaultWeight
		}
		if up.weight > validate.MaxWeight {
			return nil, invalid("upstreams[%d]: weight %d over %d", i, up.weight, validate.MaxWeight)
		}
		if up.egress.encrypted() {
			// The dialer verifies the server's certificate for this name:
			// the planner sets it, or the next node's identity name.
			if up.egress.serverName == "" {
				up.egress.serverName = u.GetNodeRef()
			}
			if !serverNamePattern.MatchString(up.egress.serverName) || len(up.egress.serverName) > validate.MaxHostnameBytes {
				return nil, invalid("upstreams[%d]: encrypted link without a valid server name", i)
			}
			if u.GetPeerIdentity() == "" {
				return nil, invalid("upstreams[%d]: encrypted link without peer_identity", i)
			}
		}
		if up.egress.raw() {
			direct++
		}
		p.upstreams = append(p.upstreams, up)
	}
	switch direct {
	case len(p.upstreams):
	case 0:
		p.chain = true
	default:
		return nil, unsupported("the hop mixes RAW upstreams with relayed ones; gost balances one kind per hop")
	}

	// Admission: relay and exit listeners serve the previous hop only.
	if len(h.GetIngressSources()) == 0 && role != forwardv1.HopRole_HOP_ROLE_ENTRY {
		return nil, invalid("%s hop without ingress sources would be an open relay", role)
	}
	for i, s := range h.GetIngressSources() {
		pfx, err := parseSource(s)
		if err != nil {
			return nil, invalid("ingress_sources[%d]: %v", i, err)
		}
		p.sources = append(p.sources, pfx)
	}
	p.sources = minimalPrefixes(p.sources)

	// Limits.
	lim := h.GetLimits()
	if lim.GetBandwidthBps() > 0 {
		if !d.cfg.BandwidthLimit {
			return nil, unsupported("bandwidth limit not enabled")
		}
		p.bandwidthBps = lim.GetBandwidthBps()
	}
	if lim.GetQuotaBytes() > 0 {
		return nil, unsupported("gost has no byte quota (an exact quota needs an NFTABLES entry)")
	}
	if lim.GetMaxConns() > 0 {
		if !d.cfg.MaxConns {
			return nil, unsupported("connection limit not enabled")
		}
		p.maxConns = lim.GetMaxConns()
	}

	// The circuit breaker becomes the selector's fail filter. The health
	// check is the Agent's (forward-sdk.md section 7.3).
	cb := h.GetCircuitBreaker()
	p.maxFails = cmp.Or(cb.GetFailureThreshold(), model.DefaultFailureThreshold)
	if p.maxFails > validate.MaxFailureThreshold {
		return nil, invalid("circuit_breaker.failure_threshold %d over %d", p.maxFails, validate.MaxFailureThreshold)
	}
	p.failTimeout = model.DefaultBreakerOpen
	if ms := cb.GetOpenMs(); ms > 0 {
		p.failTimeout = time.Duration(ms) * time.Millisecond
	}
	if p.failTimeout < validate.MinBreakerOpen || p.failTimeout > validate.MaxBreakerOpen {
		return nil, invalid("circuit_breaker.open_ms %d outside %v..%v", cb.GetOpenMs(), validate.MinBreakerOpen, validate.MaxBreakerOpen)
	}
	return p, nil
}

// listenersOf answers the gost services of a hop: a RAW ingress listens
// for each protocol and forwards the bytes as they are; every other
// ingress is one carrier listener whose relay handler carries TCP and UDP.
func listenersOf(p *hopPlan) []listener {
	if p.ingress.raw() {
		var out []listener
		if p.tcp {
			out = append(out, listener{gostType: "tcp", network: "tcp", handler: "tcp"})
		}
		if p.udp {
			out = append(out, listener{gostType: "udp", network: "udp", handler: "udp"})
		}
		return out
	}
	network := "tcp"
	if p.ingress.security == forwardv1.LinkSecurity_LINK_SECURITY_QUIC {
		network = "udp"
	}
	return []listener{{gostType: p.ingress.gostType(), network: network, handler: "relay"}}
}

// checkLink checks a transport: a known security the driver carries (RAW
// always; TLS, WSS, QUIC and gRPC with a link certificate), and a valid
// server name and path.
func (d *Driver) checkLink(field string, t *forwardv1.LinkTransport) (link, error) {
	l := link{security: t.GetSecurity(), mux: t.GetMux(), serverName: t.GetServerName(), path: t.GetPath()}
	switch l.security {
	case forwardv1.LinkSecurity_LINK_SECURITY_UNSPECIFIED:
		l.security = forwardv1.LinkSecurity_LINK_SECURITY_RAW
	case forwardv1.LinkSecurity_LINK_SECURITY_RAW:
	case forwardv1.LinkSecurity_LINK_SECURITY_TLS, forwardv1.LinkSecurity_LINK_SECURITY_WSS,
		forwardv1.LinkSecurity_LINK_SECURITY_QUIC, forwardv1.LinkSecurity_LINK_SECURITY_GRPC:
		if !d.cfg.linkTLS() {
			return link{}, fmt.Errorf("%w: %s: %s needs the node's link certificate, which is not configured", driver.ErrUnsupported, field, l.security)
		}
	default:
		if !model.LinkSecurity(l.security).IsKnown() {
			return link{}, fmt.Errorf("%w: %s: unknown link security %d", driver.ErrUnsupported, field, l.security)
		}
		return link{}, fmt.Errorf("%w: %s: gost does not carry %s", driver.ErrUnsupported, field, l.security)
	}
	if l.path != "" {
		if l.security != forwardv1.LinkSecurity_LINK_SECURITY_WSS && l.security != forwardv1.LinkSecurity_LINK_SECURITY_GRPC {
			return link{}, fmt.Errorf("%w: %s: a path needs WSS or GRPC", driver.ErrInvalidState, field)
		}
		if !pathPattern.MatchString(l.path) || len(l.path) > validate.MaxPathBytes {
			return link{}, fmt.Errorf("%w: %s: path %q is not an absolute path of letters, digits and ._~/-", driver.ErrInvalidState, field, l.path)
		}
	}
	if l.serverName != "" && (!serverNamePattern.MatchString(l.serverName) || len(l.serverName) > validate.MaxHostnameBytes) {
		return link{}, fmt.Errorf("%w: %s: server name %q is not a DNS name", driver.ErrInvalidState, field, l.serverName)
	}
	return l, nil
}

// parseLiteral parses an IP address literal without a zone; IPv4-mapped
// IPv6 addresses become IPv4.
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

// isHostName reports whether s looks like a DNS name (letters, digits,
// hyphens and dots), as opposed to garbage.
func isHostName(s string) bool {
	if s == "" || len(s) > validate.MaxHostnameBytes {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-.") == ""
}

// rejectCollisions drops every hop whose listener overlaps another hop's
// (the same socket type and port, and the same or a wildcard address):
// rejecting all of them keeps Render independent of the order of the
// hops.
func rejectCollisions(plans []*hopPlan) ([]*hopPlan, []*driver.HopError) {
	type sock struct {
		network string
		port    uint32
	}
	by := map[sock][]*hopPlan{}
	for _, p := range plans {
		for _, l := range p.listeners {
			s := sock{l.network, p.port}
			if !slices.Contains(by[s], p) {
				by[s] = append(by[s], p)
			}
		}
	}
	bad := map[driver.HopKey]string{}
	for s, ps := range by {
		for i, a := range ps {
			for _, b := range ps[i+1:] {
				if !a.listen.IsValid() || !b.listen.IsValid() || a.listen == b.listen {
					msg := fmt.Sprintf("listener %s port %d overlaps another hop's", s.network, s.port)
					for _, k := range []driver.HopKey{a.key, b.key} {
						if _, ok := bad[k]; !ok {
							bad[k] = msg
						}
					}
				}
			}
		}
	}
	var keep []*hopPlan
	var errs []*driver.HopError
	for _, p := range plans {
		if msg, ok := bad[p.key]; ok {
			errs = append(errs, &driver.HopError{Key: p.key, Engine: forwardv1.Engine_ENGINE_GOST, Err: fmt.Errorf("%w: %s", driver.ErrInvalidState, msg)})
			continue
		}
		keep = append(keep, p)
	}
	return keep, errs
}
