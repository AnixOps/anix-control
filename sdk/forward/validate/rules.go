package validate

import (
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

var (
	// ulidPattern is a ULID: 26 Crockford base32 characters, the first at
	// most 7 so the timestamp fits 48 bits.
	ulidPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	// nodeRefPattern is an Agent identity name (node-ops-service.md D9).
	nodeRefPattern = regexp.MustCompile(`^(forward|proxy)-[1-9][0-9]{0,18}$`)
	// userIDPattern is the id after "user:" in Route.owner.
	userIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// labelKeyPattern is a label key: letters, digits, '.', '_', '-' and
	// '/', starting and ending with a letter or digit.
	labelKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._/-]*[A-Za-z0-9])?$`)
	// grpcServicePattern is a gRPC service name, optionally with its
	// package ("anixops.Tunnel").
	grpcServicePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
)

func checkMeta(c *collector, r *model.Route) {
	if r.ID != "" && !ulidPattern.MatchString(r.ID) {
		c.add("id", CodeInvalidFormat, "must be a ULID assigned by Control, or empty before the route is stored")
	}
	switch {
	case r.Owner == "":
		c.add("owner", CodeRequired, `must be "admin" or "user:<id>"`)
	case r.Owner == model.OwnerAdmin:
	case strings.HasPrefix(r.Owner, model.OwnerUserPrefix) &&
		userIDPattern.MatchString(strings.TrimPrefix(r.Owner, model.OwnerUserPrefix)):
	default:
		c.add("owner", CodeInvalidFormat, `must be "admin" or "user:<id>"`)
	}
	checkText(c, "name", r.Name, MaxNameRunes)
	if !r.CreatedAt.IsZero() && r.CreatedAt.UnixMilli() < 0 {
		c.add("created_at_unix_ms", CodeOutOfRange, "must not be negative")
	}
	if !r.UpdatedAt.IsZero() && r.UpdatedAt.UnixMilli() < 0 {
		c.add("updated_at_unix_ms", CodeOutOfRange, "must not be negative")
	}
}

// checkText refuses invalid UTF-8, control characters and more than limit
// runes.
func checkText(c *collector, field, s string, limit int) {
	switch {
	case !utf8.ValidString(s) || strings.IndexFunc(s, unicode.IsControl) >= 0:
		c.add(field, CodeInvalidFormat, "must be UTF-8 text without control characters")
	case utf8.RuneCountInString(s) > limit:
		c.add(field, CodeTooLong, "must be at most %d characters", limit)
	}
}

func checkPort(c *collector, field string, port uint32) {
	if port > MaxPort {
		c.add(field, CodeOutOfRange, "must be in 1..%d, or 0 to allocate one", MaxPort)
	}
}

func checkListen(c *collector, r *model.Route) {
	l := r.Listen
	if l.Address != "" {
		if addr, err := netip.ParseAddr(l.Address); err != nil || addr.Zone() != "" {
			c.add("listen.address", CodeInvalidFormat, "must be an IP address without a zone, or empty for every address")
		}
	}
	checkPort(c, "listen.port", l.Port)
	switch {
	case l.Protocol == model.L4ProtocolUnspecified:
		c.add("listen.protocol", CodeRequired, "must be TCP, UDP or TCP_UDP")
	case !l.Protocol.IsKnown():
		c.add("listen.protocol", CodeInvalidEnum, "unknown protocol %d", int32(l.Protocol))
	}
	if l.EntryHostname != "" && !isHostname(l.EntryHostname) {
		c.add("listen.entry_hostname", CodeInvalidFormat, "must be a DNS name")
	}
	if entry, ok := r.Entry(); ok && len(entry.NodeRefs) > 1 && l.EntryHostname == "" {
		c.add("listen.entry_hostname", CodeRequired, "a route with several entry nodes needs the DNS name clients use")
	}
}

// expectedRole answers the role of hop i of n.
func expectedRole(i, n int) model.HopRole {
	if i == 0 {
		return model.HopRoleEntry
	}
	if i == n-1 {
		return model.HopRoleExit
	}
	return model.HopRoleRelay
}

func checkHops(c *collector, r *model.Route, opts Options) {
	n := len(r.Hops)
	switch {
	case n == 0:
		c.add("hops", CodeRequired, "a route needs at least its entry hop")
		return
	case n > MaxHops:
		c.add("hops", CodeTooMany, "at most %d hops", MaxHops)
	}
	for i := range r.Hops {
		h := &r.Hops[i]
		checkHopRole(c, i, n, h.Role)
		checkHopEngine(c, i, h.Engine, opts)
		checkHopNodes(c, i, h.NodeRefs)
		if i == 0 {
			// Ingress, port and dial_address are ignored on the entry.
			continue
		}
		checkPort(c, hopField(i, "port"), h.Port)
		if h.DialAddress != "" {
			if len(h.NodeRefs) != 1 {
				c.add(hopField(i, "dial_address"), CodeRequiresSingleNode, "dial_address needs exactly one node in node_refs")
			}
			if !isHostOrAddress(h.DialAddress) {
				c.add(hopField(i, "dial_address"), CodeInvalidFormat, "must be an IP address or a DNS name")
			}
		}
		checkIngress(c, i, h.Ingress, opts)
	}
}

func checkHopRole(c *collector, i, n int, role model.HopRole) {
	want := expectedRole(i, n)
	switch {
	case role == want:
	case role == model.HopRoleUnspecified:
		c.add(hopField(i, "role"), CodeRequired, "hop %d must be %s", i, want)
	case !role.IsKnown():
		c.add(hopField(i, "role"), CodeInvalidEnum, "unknown role %d", int32(role))
	default:
		c.add(hopField(i, "role"), CodeInvalidRole,
			"hop %d must be %s: the first hop is the entry, the last of several is the exit, the others relay", i, want)
	}
}

func checkHopEngine(c *collector, i int, engine model.Engine, opts Options) {
	switch {
	case engine == model.EngineUnspecified:
		c.add(hopField(i, "engine"), CodeRequired, "must be ENGINE_NFTABLES or ENGINE_GOST")
	case !engine.IsKnown():
		c.add(hopField(i, "engine"), CodeInvalidEnum, "unknown engine %d", int32(engine))
	case engine == model.EngineAnixOps && !opts.EnableAnixOps:
		c.add(hopField(i, "engine"), CodeEngineNotEnabled, "ENGINE_ANIXOPS is not enabled before v4.3")
	}
}

// checkHopNodes checks a hop's node references. One node may serve several
// hops of a route (an nftables entry handing over to a gost relay on the
// same host); the planner keys its hops by (route, hop, node).
func checkHopNodes(c *collector, i int, refs []string) {
	switch {
	case len(refs) == 0:
		c.add(hopField(i, "node_refs"), CodeRequired, "a hop needs at least one node")
	case len(refs) > MaxNodesPerHop:
		c.add(hopField(i, "node_refs"), CodeTooMany, "at most %d nodes per hop", MaxNodesPerHop)
	}
	seen := map[string]bool{}
	for j, ref := range refs {
		field := hopNodeField(i, j)
		if !nodeRefPattern.MatchString(ref) {
			c.add(field, CodeInvalidFormat, `must be an Agent identity name, "forward-<id>" or "proxy-<id>"`)
			continue
		}
		if seen[ref] {
			c.add(field, CodeDuplicate, "%s is listed twice", ref)
		}
		seen[ref] = true
	}
}

func checkIngress(c *collector, i int, t model.LinkTransport, opts Options) {
	field := hopField(i, "ingress.")
	switch {
	case t.Security == model.LinkSecurityUnspecified:
		c.add(field+"security", CodeRequired, "the link from hop %d needs a security", i-1)
	case !t.Security.IsKnown():
		c.add(field+"security", CodeInvalidEnum, "unknown link security %d", int32(t.Security))
	case t.Security == model.LinkSecurityAnixOps && !opts.EnableAnixOps:
		c.add(field+"security", CodeEngineNotEnabled, "LINK_SECURITY_ANIXOPS is not enabled before v4.3")
	}
	if t.ServerName != "" {
		switch {
		case t.Security == model.LinkSecurityRaw:
			c.add(field+"server_name", CodeNotApplicable, "a RAW link has no server name")
		case !isHostname(t.ServerName):
			c.add(field+"server_name", CodeInvalidFormat, "must be a DNS name")
		}
	}
	if t.Path != "" {
		checkLinkPath(c, field+"path", t)
	}
}

func checkLinkPath(c *collector, field string, t model.LinkTransport) {
	switch {
	case len(t.Path) > MaxPathBytes:
		c.add(field, CodeTooLong, "must be at most %d bytes", MaxPathBytes)
	case t.Security == model.LinkSecurityWSS:
		if !strings.HasPrefix(t.Path, "/") || strings.IndexFunc(t.Path, func(r rune) bool { return r <= ' ' || r >= 0x7f }) >= 0 {
			c.add(field, CodeInvalidFormat, "a WebSocket path starts with / and has no spaces or control characters")
		}
	case t.Security == model.LinkSecurityGRPC:
		if !grpcServicePattern.MatchString(t.Path) {
			c.add(field, CodeInvalidFormat, "must be a gRPC service name")
		}
	default:
		c.add(field, CodeNotApplicable, "only WSS and GRPC links have a path")
	}
}

// checkLinks applies the link rule: hop i originates and hop i+1
// terminates hops[i+1].ingress (forward-sdk.md section 4.2).
func checkLinks(c *collector, r *model.Route) {
	for i := 1; i < len(r.Hops); i++ {
		dialer, listener := r.Hops[i-1], r.Hops[i]
		sec := listener.Ingress.Security
		if sec != model.LinkSecurityUnspecified && sec.IsKnown() {
			switch {
			case knownEngine(dialer.Engine) && !CanCarry(dialer.Engine, sec):
				c.add(hopField(i, "ingress.security"), CodeLinkUnsupported,
					"hop %d runs %s, which only dials %s", i-1, dialer.Engine, linkList(dialer.Engine))
			case knownEngine(listener.Engine) && !CanCarry(listener.Engine, sec):
				c.add(hopField(i, "ingress.security"), CodeLinkUnsupported,
					"hop %d runs %s, which only terminates %s", i, listener.Engine, linkList(listener.Engine))
			}
		}
		if listener.Ingress.Mux {
			for _, h := range []struct {
				index  int
				engine model.Engine
			}{{i - 1, dialer.Engine}, {i, listener.Engine}} {
				if knownEngine(h.engine) && !CanMux(h.engine) {
					c.add(hopField(i, "ingress.mux"), CodeLinkUnsupported, "hop %d runs %s, which cannot multiplex", h.index, h.engine)
					break
				}
			}
		}
	}
}

func knownEngine(e model.Engine) bool {
	return e != model.EngineUnspecified && e.IsKnown()
}

func linkList(e model.Engine) string {
	links := engineLinks[e]
	names := make([]string, len(links))
	for i, l := range links {
		names[i] = l.String()
	}
	return strings.Join(names, ", ")
}

// effectiveTargetPolicy answers the policy targets are checked under, after
// reporting a policy the owner may not use.
func effectiveTargetPolicy(c *collector, r *model.Route) model.TargetPolicy {
	p := r.Policy.TargetPolicy
	switch {
	case p == model.TargetPolicyUnspecified:
		return model.DefaultTargetPolicy
	case !p.IsKnown():
		c.add("policy.target_policy", CodeInvalidEnum, "unknown target policy %d", int32(p))
		return model.TargetPolicyPublicOnly
	case p == model.TargetPolicyAllowPrivate && !r.IsAdmin():
		c.add("policy.target_policy", CodeForbidden, "only an administrator's route may reach private targets")
		return model.TargetPolicyPublicOnly
	}
	return p
}

func checkTargets(c *collector, r *model.Route) {
	policy := effectiveTargetPolicy(c, r)
	switch {
	case len(r.Targets) == 0:
		c.add("targets", CodeRequired, "a route needs at least one target")
	case len(r.Targets) > MaxTargets:
		c.add("targets", CodeTooMany, "at most %d targets", MaxTargets)
	}
	seen := map[string]int{}
	for i, t := range r.Targets {
		if t.Host == "" {
			c.add(targetField(i, "host"), CodeRequired, "must be an IP address or a DNS name")
		} else if code, message := checkTargetHost(t.Host, policy); code != "" {
			c.add(targetField(i, "host"), code, "%s", message)
		}
		switch {
		case t.Port == 0:
			c.add(targetField(i, "port"), CodeRequired, "must be in 1..%d", MaxPort)
		case t.Port > MaxPort:
			c.add(targetField(i, "port"), CodeOutOfRange, "must be in 1..%d", MaxPort)
		}
		if t.Weight > MaxWeight {
			c.add(targetField(i, "weight"), CodeOutOfRange, "must be at most %d (0 means 1)", MaxWeight)
		}
		key := targetKey(t.Host, t.Port)
		if prev, dup := seen[key]; dup && t.Host != "" {
			c.add(fmt.Sprintf("targets[%d]", i), CodeDuplicate, "same host and port as targets[%d]", prev)
		} else {
			seen[key] = i
		}
	}
}

// targetKey normalizes a target for duplicate detection: addresses in
// canonical form, names in lower case without the final dot.
func targetKey(host string, port uint32) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	} else {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
	}
	return host + "#" + strconv.FormatUint(uint64(port), 10)
}

func checkPolicy(c *collector, r *model.Route) {
	p := r.Policy
	for _, s := range []struct {
		field    string
		strategy model.BalanceStrategy
	}{{"policy.next_hop", p.NextHop}, {"policy.target", p.Target}} {
		if !s.strategy.IsKnown() {
			c.add(s.field, CodeInvalidEnum, "unknown balance strategy %d", int32(s.strategy))
		}
	}
	switch {
	case !p.Direct.IsKnown():
		c.add("policy.direct", CodeInvalidEnum, "unknown direct mode %d", int32(p.Direct))
	case p.Direct == model.DirectForced && len(r.Hops) > 1:
		c.add("policy.direct", CodeUnusedHops,
			"DIRECT_MODE_FORCED makes the entry dial the targets, so hops 1..%d would be unused", len(r.Hops)-1)
	}
	checkDuration(c, "policy.health.interval_ms", p.Health.Interval, MinHealthInterval, MaxHealthInterval)
	checkDuration(c, "policy.health.timeout_ms", p.Health.Timeout, MinHealthTimeout, MaxHealthTimeout)
	if h := p.Health.WithDefaults(); h.Timeout >= h.Interval {
		c.add("policy.health.timeout_ms", CodeInvalidRelation,
			"the timeout (%s) must be shorter than the interval (%s)", h.Timeout, h.Interval)
	}
	if p.CircuitBreaker.FailureThreshold > MaxFailureThreshold {
		c.add("policy.circuit_breaker.failure_threshold", CodeOutOfRange,
			"must be at most %d, or 0 for the default (%d)", MaxFailureThreshold, model.DefaultFailureThreshold)
	}
	checkDuration(c, "policy.circuit_breaker.open_ms", p.CircuitBreaker.OpenFor, MinBreakerOpen, MaxBreakerOpen)
}

// checkDuration refuses a set duration outside [lo, hi]; 0 is the
// default.
func checkDuration(c *collector, field string, d, lo, hi time.Duration) {
	if d != 0 && (d < lo || d > hi) {
		c.add(field, CodeOutOfRange, "must be between %s and %s, or 0 for the default", lo, hi)
	}
}

func checkLimits(c *collector, r *model.Route, opts Options) {
	expires := r.Limits.ExpiresAt
	if expires.IsZero() {
		return
	}
	switch {
	case expires.UnixMilli() <= 0:
		c.add("limits.expires_at_unix_ms", CodeOutOfRange, "must be a Unix time in milliseconds, or 0 for never")
	case opts.OnCreate && !opts.Now.IsZero() && !expires.After(opts.Now):
		c.add("limits.expires_at_unix_ms", CodeExpired, "must be in the future when the route is created")
	}
}

func checkLabels(c *collector, r *model.Route) {
	if len(r.Labels) > MaxLabels {
		c.add("labels", CodeTooMany, "at most %d labels", MaxLabels)
	}
	keys := make([]string, 0, len(r.Labels))
	for k := range r.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		field := "labels[" + strconv.Quote(k) + "]"
		switch {
		case len(k) > MaxLabelKeyBytes:
			c.add(field, CodeTooLong, "a key is at most %d bytes", MaxLabelKeyBytes)
		case !labelKeyPattern.MatchString(k):
			c.add(field, CodeInvalidFormat, "a key is letters, digits, '.', '_', '-' and '/', starting and ending with a letter or digit")
		}
		v := r.Labels[k]
		switch {
		case !utf8.ValidString(v) || strings.IndexFunc(v, unicode.IsControl) >= 0:
			c.add(field, CodeInvalidFormat, "a value is UTF-8 text without control characters")
		case len(v) > MaxLabelValueBytes:
			c.add(field, CodeTooLong, "a value is at most %d bytes", MaxLabelValueBytes)
		}
	}
}
