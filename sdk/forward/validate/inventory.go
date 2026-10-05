package validate

import (
	"net/netip"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// checkPorts refuses explicit ports outside a node's range (with an
// inventory) or reserved on it (forward-sdk.md sections 4.4 and 14). The
// entry listens on listen.port on every entry node; another hop on its own
// port on each of its nodes.
func checkPorts(c *collector, r *model.Route, opts Options) {
	nodes := nodeIndex(opts.Nodes)
	for i, h := range r.Hops {
		field, port := hopField(i, "port"), h.Port
		if i == 0 {
			field, port = "listen.port", r.Listen.Port
		}
		if port == 0 || port > MaxPort {
			continue
		}
		for _, ref := range h.NodeRefs {
			if n, ok := nodes[ref]; ok && !n.PortRange.IsZero() && !n.PortRange.Contains(port) {
				c.add(field, CodePortOutOfRange, "%d is outside %s's range %d-%d", port, ref, n.PortRange.First, n.PortRange.Last)
			}
			for _, reserved := range opts.ReservedPorts[ref] {
				if reserved == port {
					c.add(field, CodePortReserved, "%d is reserved on %s", port, ref)
					break
				}
			}
		}
	}
}

func nodeIndex(nodes []model.NodeInfo) map[string]*model.NodeInfo {
	index := make(map[string]*model.NodeInfo, len(nodes))
	for i := range nodes {
		index[nodes[i].NodeRef] = &nodes[i]
	}
	return index
}

// hopNode is one node of one hop whose engine the node offers.
type hopNode struct {
	ref  string
	caps model.EngineCapabilities
}

// checkInventory applies the rules that need the node inventory: every node
// exists and advertises the hop's engine, available, with every feature
// the route needs (forward-sdk.md section 4.4).
func checkInventory(c *collector, r *model.Route, opts Options) {
	served := resolveHopNodes(c, r, nodeIndex(opts.Nodes))
	n := len(r.Hops)
	if n == 0 {
		return
	}
	checkInventoryLinks(c, r, served)
	if opts.EnableAnixOps {
		checkInventoryAnixOps(c, r, served, nodeIndex(opts.Nodes))
	}
	policy := r.Policy.WithDefaults()
	for i := 0; i < n-1; i++ {
		requireStrategy(c, "policy.next_hop", policy.NextHop, served[i])
	}
	requireStrategy(c, "policy.target", policy.Target, served[n-1])
	if n > 1 && policy.Direct == model.DirectPreferred {
		// PREFERRED makes the entry balance over the targets too.
		requireStrategy(c, "policy.target", policy.Target, served[0])
	}
	if r.Listen.Protocol.HasUDP() {
		for i := range served {
			requireFeature(c, "listen.protocol", served[i], "does not forward UDP",
				func(caps model.EngineCapabilities) bool { return caps.UDP })
		}
	}
	ipv6 := func(caps model.EngineCapabilities) bool { return caps.IPv6 }
	if isIPv6(r.Listen.Address) {
		requireFeature(c, "listen.address", served[0], "has no IPv6", ipv6)
	}
	for i, t := range r.Targets {
		if isIPv6(t.Host) {
			requireFeature(c, targetField(i, "host"), served[n-1], "has no IPv6", ipv6)
		}
	}
	limits := r.Limits
	if limits.BandwidthBPS > 0 {
		requireFeature(c, "limits.bandwidth_bps", served[0], "cannot limit bandwidth",
			func(caps model.EngineCapabilities) bool { return caps.BandwidthLimit })
	}
	if limits.QuotaBytes > 0 {
		requireFeature(c, "limits.quota_bytes", served[0], "cannot enforce a byte quota",
			func(caps model.EngineCapabilities) bool { return caps.Quota })
	}
	if limits.MaxConns > 0 {
		requireFeature(c, "limits.max_conns", served[0], "cannot limit connections",
			func(caps model.EngineCapabilities) bool { return caps.MaxConns })
	}
}

// resolveHopNodes answers, per hop, the nodes that offer the hop's engine,
// and reports the others.
func resolveHopNodes(c *collector, r *model.Route, nodes map[string]*model.NodeInfo) [][]hopNode {
	served := make([][]hopNode, len(r.Hops))
	for i, h := range r.Hops {
		for j, ref := range h.NodeRefs {
			node, ok := nodes[ref]
			if !ok {
				if nodeRefPattern.MatchString(ref) {
					c.add(hopNodeField(i, j), CodeUnknownNode, "%s is not in the node inventory", ref)
				}
				continue
			}
			if !knownEngine(h.Engine) {
				continue
			}
			caps, ok := node.Engine(h.Engine)
			switch {
			case !ok:
				c.add(hopField(i, "engine"), CodeEngineNotAdvertised, "%s does not advertise %s", ref, h.Engine)
			case !caps.Available:
				c.add(hopField(i, "engine"), CodeEngineUnavailable, "%s reports %s unavailable: %s", ref, h.Engine, caps.UnavailableReason)
			default:
				served[i] = append(served[i], hopNode{ref: ref, caps: caps})
			}
		}
	}
	return served
}

// checkInventoryLinks requires each dialling node to originate, and each
// listening node to terminate, the link's security.
func checkInventoryLinks(c *collector, r *model.Route, served [][]hopNode) {
	for i := 1; i < len(r.Hops); i++ {
		sec := r.Hops[i].Ingress.Security
		if sec == model.LinkSecurityUnspecified || !CanCarry(r.Hops[i-1].Engine, sec) || !CanCarry(r.Hops[i].Engine, sec) {
			continue // reported by the static rules
		}
		field := hopField(i, "ingress.security")
		for _, hn := range served[i-1] {
			if !hn.caps.SupportsLink(sec) {
				c.add(field, CodeCapabilityMissing, "%s (%s) cannot dial %s", hn.ref, hn.caps.Engine, sec)
			}
		}
		for _, hn := range served[i] {
			if !hn.caps.SupportsLink(sec) {
				c.add(field, CodeCapabilityMissing, "%s (%s) cannot terminate %s", hn.ref, hn.caps.Engine, sec)
			}
		}
	}
}

// checkInventoryAnixOps applies the carrier, wire version and PROXY
// protocol rules of anixops-protocol.md sections 6.5 and 6.7 to the nodes
// that serve an ANIXOPS link or exit. Every node of the dialling hop may
// dial every node of the listening hop, so every pair must agree.
func checkInventoryAnixOps(c *collector, r *model.Route, served [][]hopNode, nodes map[string]*model.NodeInfo) {
	for i := 1; i < len(r.Hops); i++ {
		t := r.Hops[i].Ingress
		if t.Security != model.LinkSecurityAnixOps || !t.Carrier.IsKnown() {
			continue
		}
		carrierField := hopField(i, "ingress.carrier")
		// AUTO falls back to TLS_TCP, so both ends must offer it; a QUIC
		// listener or dialler only makes AUTO faster.
		need := t.Carrier.Effective()
		if need == model.CarrierAuto {
			need = model.CarrierTLSTCP
		}
		for j, side := range [][]hopNode{served[i-1], served[i]} {
			verb := "dial"
			if j == 1 {
				verb = "terminate"
			}
			for _, hn := range side {
				if hn.caps.Engine == model.EngineAnixOps && !hn.caps.SupportsCarrier(need) {
					c.add(carrierField, CodeCarrierUnsupported, "%s (%s) cannot %s the %s carrier", hn.ref, hn.caps.Engine, verb, need)
				}
			}
		}
		if t.Carrier == model.CarrierPlain {
			for _, h := range []int{i - 1, i} {
				for _, ref := range r.Hops[h].NodeRefs {
					if n, ok := nodes[ref]; ok && !IsTrustedLink(n.Labels) {
						c.add(carrierField, CodePlainUntrusted, "%s is not labelled link=iepl or link=iplc", ref)
					}
				}
			}
		}
		for _, d := range served[i-1] {
			for _, l := range served[i] {
				if !d.caps.SharesProtocolVersion(l.caps) {
					c.add(hopField(i, "ingress.security"), CodeCapabilityMissing,
						"%s and %s share no wire protocol version (%v, %v)", d.ref, l.ref, d.caps.ProtocolVersions, l.caps.ProtocolVersions)
				}
			}
		}
	}
	if n := len(r.Hops); n > 0 && r.Policy.ProxyProtocol.Enabled() {
		for _, hn := range served[n-1] {
			if !hn.caps.ProxyProtocol {
				c.add("policy.proxy_protocol", CodeProxyProtocolUnsupported, "%s (%s) cannot write a PROXY protocol header", hn.ref, hn.caps.Engine)
			}
		}
	}
}

func requireStrategy(c *collector, field string, strategy model.BalanceStrategy, nodes []hopNode) {
	if !strategy.IsKnown() {
		return
	}
	for _, hn := range nodes {
		if !hn.caps.Supports(strategy) {
			c.add(field, CodeCapabilityMissing, "%s (%s) does not offer %s", hn.ref, hn.caps.Engine, strategy)
		}
	}
}

func requireFeature(c *collector, field string, nodes []hopNode, missing string, has func(model.EngineCapabilities) bool) {
	for _, hn := range nodes {
		if !has(hn.caps) {
			c.add(field, CodeCapabilityMissing, "%s (%s) %s", hn.ref, hn.caps.Engine, missing)
		}
	}
}

func isIPv6(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is6() && !addr.Is4In6()
}
