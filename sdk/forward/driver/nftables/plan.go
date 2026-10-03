package nftables

import (
	"cmp"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
)

// routeIDPattern is what a route id must look like to become part of an nft
// name: Control's ids are ULIDs; anything else is refused rather than
// escaped, so no input can reach the script except as a checked literal.
var routeIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)

// hopPlan is one hop checked and reduced to what the script needs. Every
// field is a checked literal: numbers, netip values and names derived from
// a checked route id.
type hopPlan struct {
	key  driver.HopKey
	base string // name prefix of the hop's objects: r_<route>_h<hop>

	tcp, udp bool
	listen   netip.Addr // invalid for every local address
	port     uint32

	balance forwardv1.BalanceStrategy
	ups4    []upstream
	ups6    []upstream

	// sources is set when the listener admits only some sources.
	sources      bool
	src4, src6   []netip.Prefix
	bandwidth    bool
	bandwidthBps uint64
	quotaBytes   uint64
	maxConns     uint32
	paused       bool
	mark         uint32 // shifted into the mask
	markIndex    uint32 // as the planner allocated it
	listenFamily int    // 0 any, 4 or 6
}

type upstream struct {
	addr     netip.Addr
	port     uint32
	weight   uint32
	priority uint32
}

// hopName answers the name prefix of a hop's objects.
func hopName(k driver.HopKey) string {
	return "r_" + k.RouteID + "_h" + strconv.FormatUint(uint64(k.HopIndex), 10)
}

// planHop checks one hop of the driver's engine and reduces it to a plan.
// The order of the checks decides which error a hop with several problems
// gets; any one is correct.
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
	p := &hopPlan{key: k, base: hopName(k), paused: h.GetPaused()}

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
	if err := checkLink("ingress", h.GetIngress()); err != nil {
		return nil, err
	}
	for i, u := range h.GetUpstreams() {
		if err := checkLink(fmt.Sprintf("upstreams[%d].egress", i), u.GetEgress()); err != nil {
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
		p.listenFamily = family(addr)
	}

	// Upstreams.
	if len(h.GetUpstreams()) == 0 {
		return nil, invalid("no upstream")
	}
	if n := uint32(len(h.GetUpstreams())); n > d.cfg.Slots { // #nosec G115 -- bounded by the slot check
		return nil, unsupported("%d upstreams, more than the %d balancing slots", n, d.cfg.Slots)
	}
	seen := map[netip.AddrPort]bool{}
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
		if u.GetPort() == 0 || u.GetPort() > validate.MaxPort {
			return nil, invalid("upstreams[%d]: port %d outside 1..%d", i, u.GetPort(), validate.MaxPort)
		}
		ap := netip.AddrPortFrom(addr, uint16(u.GetPort())) // #nosec G115 -- checked above
		if seen[ap] {
			return nil, invalid("upstreams[%d]: %s twice", i, ap)
		}
		seen[ap] = true
		w := u.GetWeight()
		if w == 0 {
			w = model.DefaultWeight
		}
		if w > validate.MaxWeight {
			return nil, invalid("upstreams[%d]: weight %d over %d", i, w, validate.MaxWeight)
		}
		up := upstream{addr: addr, port: u.GetPort(), weight: w, priority: u.GetPriority()}
		if addr.Is4() {
			p.ups4 = append(p.ups4, up)
		} else {
			if !d.cfg.IPv6 {
				return nil, unsupported("ipv6 not enabled")
			}
			p.ups6 = append(p.ups6, up)
		}
	}
	if p.listenFamily == 4 && len(p.ups4) == 0 || p.listenFamily == 6 && len(p.ups6) == 0 {
		return nil, invalid("listen address %s has no upstream of its family", p.listen)
	}
	for _, ups := range [][]upstream{p.ups4, p.ups6} {
		slices.SortFunc(ups, func(a, b upstream) int {
			return cmp.Or(cmp.Compare(a.priority, b.priority), a.addr.Compare(b.addr), cmp.Compare(a.port, b.port))
		})
	}

	// Admission: relay and exit listeners serve the previous hop only.
	if len(h.GetIngressSources()) == 0 && role != forwardv1.HopRole_HOP_ROLE_ENTRY {
		return nil, invalid("%s hop without ingress sources would be an open relay", role)
	}
	if len(h.GetIngressSources()) > 0 {
		p.sources = true
		for i, s := range h.GetIngressSources() {
			pfx, err := parseSource(s)
			if err != nil {
				return nil, invalid("ingress_sources[%d]: %v", i, err)
			}
			if pfx.Addr().Is4() {
				p.src4 = append(p.src4, pfx)
			} else {
				p.src6 = append(p.src6, pfx)
			}
		}
		p.src4, p.src6 = minimalPrefixes(p.src4), minimalPrefixes(p.src6)
	}

	// Limits.
	lim := h.GetLimits()
	if lim.GetBandwidthBps() > 0 {
		if !d.cfg.BandwidthLimit {
			return nil, unsupported("bandwidth limit not enabled")
		}
		p.bandwidth = true
		p.bandwidthBps = lim.GetBandwidthBps()
	}
	if lim.GetQuotaBytes() > 0 {
		if !d.cfg.Quota {
			return nil, unsupported("quota not enabled")
		}
		p.quotaBytes = lim.GetQuotaBytes()
	}
	if lim.GetMaxConns() > 0 {
		if !d.cfg.MaxConns {
			return nil, unsupported("connection limit not enabled")
		}
		p.maxConns = lim.GetMaxConns()
	}

	// The connection mark.
	if m := h.GetMark(); m == 0 || m > d.cfg.maxMark() {
		return nil, invalid("mark %d outside 1..%d", m, d.cfg.maxMark())
	}
	p.markIndex = h.GetMark()
	if p.bandwidth && p.markIndex > maxTCMark {
		return nil, unsupported("mark %d of a rate-limited hop is over %d, the most the tc class ids hold", p.markIndex, maxTCMark)
	}
	p.mark = h.GetMark() << d.cfg.markShift()
	return p, nil
}

// checkLink accepts RAW (and an unset transport, which is RAW); nftables
// carries nothing else.
func checkLink(field string, t *forwardv1.LinkTransport) error {
	s := t.GetSecurity()
	switch {
	case !model.LinkSecurity(s).IsKnown():
		return fmt.Errorf("%w: %s: unknown link security %d", driver.ErrUnsupported, field, s)
	case s != forwardv1.LinkSecurity_LINK_SECURITY_UNSPECIFIED && s != forwardv1.LinkSecurity_LINK_SECURITY_RAW:
		return fmt.Errorf("%w: %s: nftables carries RAW links only, not %s", driver.ErrUnsupported, field, s)
	case t.GetMux():
		return fmt.Errorf("%w: %s: nftables cannot multiplex", driver.ErrUnsupported, field)
	}
	return nil
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
		a := p.Addr().Unmap()
		if p.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("%q is not an IP address or prefix", s)
		}
		p = netip.PrefixFrom(a, p.Bits()-96)
	}
	return p.Masked(), nil
}

// minimalPrefixes sorts prefixes and drops the ones another covers, so the
// interval set gets no overlapping elements.
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

func family(a netip.Addr) int {
	if a.Is4() {
		return 4
	}
	return 6
}

// rejectCollisions drops every hop that shares a connection mark or a
// listener with another hop: rejecting all of them, not all but the first,
// keeps Render independent of the order of the hops.
func rejectCollisions(plans []*hopPlan) ([]*hopPlan, []*driver.HopError) {
	bad := map[driver.HopKey]string{}
	byMark := map[uint32][]*hopPlan{}
	for _, p := range plans {
		byMark[p.markIndex] = append(byMark[p.markIndex], p)
	}
	for m, ps := range byMark {
		if len(ps) > 1 {
			for _, p := range ps {
				bad[p.key] = fmt.Sprintf("mark %d is used by %d hops", m, len(ps))
			}
		}
	}
	type listener struct {
		udp  bool
		port uint32
	}
	byPort := map[listener][]*hopPlan{}
	for _, p := range plans {
		if p.tcp {
			byPort[listener{false, p.port}] = append(byPort[listener{false, p.port}], p)
		}
		if p.udp {
			byPort[listener{true, p.port}] = append(byPort[listener{true, p.port}], p)
		}
	}
	for l, ps := range byPort {
		for i, a := range ps {
			for _, b := range ps[i+1:] {
				if !a.listen.IsValid() || !b.listen.IsValid() || a.listen == b.listen {
					proto := "tcp"
					if l.udp {
						proto = "udp"
					}
					msg := fmt.Sprintf("listener %s port %d overlaps another hop's", proto, l.port)
					if _, ok := bad[a.key]; !ok {
						bad[a.key] = msg
					}
					if _, ok := bad[b.key]; !ok {
						bad[b.key] = msg
					}
				}
			}
		}
	}
	var keep []*hopPlan
	var errs []*driver.HopError
	for _, p := range plans {
		if msg, ok := bad[p.key]; ok {
			errs = append(errs, &driver.HopError{Key: p.key, Engine: forwardv1.Engine_ENGINE_NFTABLES, Err: fmt.Errorf("%w: %s", driver.ErrInvalidState, msg)})
			continue
		}
		keep = append(keep, p)
	}
	return keep, errs
}
