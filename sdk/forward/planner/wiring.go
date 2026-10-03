package planner

import (
	"fmt"
	"math"
	"net/netip"
	"slices"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"google.golang.org/protobuf/proto"
)

// wiring turns one route's hops into node hops (forward-sdk.md section
// 5.3).
type wiring struct {
	nodes   map[string]*model.NodeInfo
	alloc   Allocations
	cluster string
	report  func(routeID, field string, code validate.Code, format string, args ...any)
	// warnings collects warnings for people.
	warnings []string
}

// nodeHop is a planned hop and the node that runs it.
type nodeHop struct {
	node string
	hop  *forwardv1.NodeHop
}

func (w *wiring) route(r *model.Route) ([]nodeHop, error) {
	policy := r.Policy.WithDefaults()
	n := len(r.Hops)
	var out []nodeHop
	for i, h := range r.Hops {
		index := uint32(i) // #nosec G115 -- i < validate.MaxHops
		last := i == n-1
		ingress := &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW}
		var sources, peers []string
		if i > 0 {
			prev := r.Hops[i-1]
			sources = w.addresses(prev.NodeRefs)
			if h.Ingress.Security != model.LinkSecurityRaw {
				for _, ref := range prev.NodeRefs {
					id, err := w.identity(r, i-1, ref)
					if err != nil {
						return nil, err
					}
					peers = append(peers, id)
				}
			}
		}
		var upstreams []*forwardv1.Upstream
		balance := policy.NextHop
		switch {
		case last:
			balance = policy.Target
			upstreams = targetUpstreams(r.Targets)
		default:
			next, err := w.nextHopUpstreams(r, i+1)
			if err != nil {
				return nil, err
			}
			upstreams = next
			if i == 0 && policy.Direct == model.DirectPreferred {
				// Section 5.5: the entry dials the targets while they are
				// healthy and falls back to the chain; the targets keep
				// their priorities and the next hop's nodes come after
				// the last of them.
				balance = policy.Target
				offset := uint64(maxPriority(r.Targets)) + 1
				for _, u := range upstreams {
					u.Priority = uint32(min(uint64(u.Priority)+offset, math.MaxUint32)) // #nosec G115 -- clamped
				}
				upstreams = append(targetUpstreams(r.Targets), upstreams...)
			}
		}
		for _, ref := range h.NodeRefs {
			key := Key{RouteID: r.ID, HopIndex: index, NodeRef: ref}
			s := w.alloc[key]
			hop := &forwardv1.NodeHop{
				RouteId:        r.ID,
				HopIndex:       index,
				Role:           forwardv1.HopRole(h.Role),
				Engine:         forwardv1.Engine(h.Engine),
				Upstreams:      cloneUpstreams(upstreams),
				Balance:        forwardv1.BalanceStrategy(balance),
				Health:         policy.Health.ToProto(),
				CircuitBreaker: policy.CircuitBreaker.ToProto(),
				TargetPolicy:   forwardv1.TargetPolicy(policy.TargetPolicy),
				Paused:         r.Paused,
				Mark:           s.Mark,
				IngressSources: slices.Clone(sources),
				IngressPeers:   slices.Clone(peers),
			}
			if i == 0 {
				hop.Listen = &forwardv1.Listen{
					Address: r.Listen.Address, Port: s.Port,
					Protocol: forwardv1.L4Protocol(r.Listen.Protocol), EntryHostname: r.Listen.EntryHostname,
				}
				hop.Ingress = ingress
				hop.Limits = r.Limits.ToProto()
			} else {
				hop.Listen = &forwardv1.Listen{Address: h.DialAddress, Port: s.Port, Protocol: forwardv1.L4Protocol(r.Listen.Protocol)}
				hop.Ingress = linkFor(h.Ingress, ref)
			}
			if h.Engine == model.EngineNFTables {
				w.warnFamilies(r.ID, index, ref, hop)
			}
			out = append(out, nodeHop{node: ref, hop: hop})
		}
	}
	return out, nil
}

// nextHopUpstreams answers hop i's nodes as upstreams of hop i-1, in
// node_refs order, which is also their failover order (priority).
func (w *wiring) nextHopUpstreams(r *model.Route, i int) ([]*forwardv1.Upstream, error) {
	h := r.Hops[i]
	var out []*forwardv1.Upstream
	for j, ref := range h.NodeRefs {
		address := h.DialAddress
		if address == "" {
			if n := w.nodes[ref]; n != nil && len(n.Addresses) > 0 {
				address = n.Addresses[0]
			}
		}
		if address == "" {
			w.report(r.ID, fmt.Sprintf("hops[%d].node_refs[%d]", i, j), validate.CodeNoAddress,
				"hop %d must dial %s, which has no address; set dial_address or the node's addresses", i-1, ref)
			continue
		}
		u := &forwardv1.Upstream{
			Address:  address,
			Port:     w.alloc[Key{RouteID: r.ID, HopIndex: uint32(i), NodeRef: ref}].Port, // #nosec G115 -- i < validate.MaxHops
			Weight:   model.DefaultWeight,
			Priority: uint32(j), // #nosec G115 -- j < validate.MaxNodesPerHop
			Egress:   linkFor(h.Ingress, ref),
			NodeRef:  ref,
		}
		if h.Ingress.Security != model.LinkSecurityRaw {
			id, err := w.identity(r, i, ref)
			if err != nil {
				return nil, err
			}
			u.PeerIdentity = id
		}
		out = append(out, u)
	}
	return out, nil
}

// identity answers the Agent identity of node ref, which serves hop i.
func (w *wiring) identity(r *model.Route, i int, ref string) (string, error) {
	if w.cluster == "" {
		return "", ErrNoCluster
	}
	node, err := agentcontrol.ParseAgentNode(ref)
	if err == nil {
		var id agentcontrol.AgentIdentity
		if id, err = agentcontrol.NewAgentIdentity(w.cluster, node); err == nil {
			return id.String(), nil
		}
	}
	if node.Valid() {
		// The node is fine, so the cluster is not.
		return "", fmt.Errorf("planner: Options.Cluster: %w", err)
	}
	w.report(r.ID, fmt.Sprintf("hops[%d].node_refs[%d]", i, slices.Index(r.Hops[i].NodeRefs, ref)), validate.CodeInvalidFormat,
		"%s cannot be named as an Agent identity: %v", ref, err)
	return "", nil
}

// addresses answers every address of the nodes, in order, once each.
func (w *wiring) addresses(refs []string) []string {
	var out []string
	for _, ref := range refs {
		if n := w.nodes[ref]; n != nil {
			for _, a := range n.Addresses {
				if !slices.Contains(out, a) {
					out = append(out, a)
				}
			}
		}
	}
	return out
}

// linkFor answers the link to or on node ref: an encrypted link without a
// server name presents the node's Agent identity name.
func linkFor(t model.LinkTransport, ref string) *forwardv1.LinkTransport {
	if t.Security != model.LinkSecurityRaw && t.ServerName == "" {
		t.ServerName = ref
	}
	return t.ToProto()
}

func targetUpstreams(targets []model.Target) []*forwardv1.Upstream {
	out := make([]*forwardv1.Upstream, len(targets))
	for i, t := range targets {
		out[i] = &forwardv1.Upstream{
			Address:  t.Host,
			Port:     t.Port,
			Weight:   t.EffectiveWeight(),
			Priority: t.Priority,
			Egress:   &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		}
	}
	return out
}

func maxPriority(targets []model.Target) uint32 {
	var m uint32
	for _, t := range targets {
		m = max(m, t.Priority)
	}
	return m
}

func cloneUpstreams(in []*forwardv1.Upstream) []*forwardv1.Upstream {
	out := make([]*forwardv1.Upstream, len(in))
	for i, u := range in {
		out[i] = proto.Clone(u).(*forwardv1.Upstream)
	}
	return out
}

// warnFamilies warns when an nftables hop accepts clients of an address
// family it has no upstream for: the driver keeps one DNAT map per family
// (forward-sdk.md section 6.1), so those clients are not forwarded. Hops
// with a DNS name upstream are skipped, since the name may resolve to
// either family.
func (w *wiring) warnFamilies(routeID string, index uint32, ref string, hop *forwardv1.NodeHop) {
	var listens, upstreams [2]bool // IPv4, IPv6
	if a, err := netip.ParseAddr(hop.GetListen().GetAddress()); err == nil {
		listens[family(a)] = true
	} else if n := w.nodes[ref]; n != nil {
		for _, s := range n.Addresses {
			if a, err := netip.ParseAddr(s); err == nil {
				listens[family(a)] = true
			}
		}
	}
	for _, u := range hop.GetUpstreams() {
		a, err := netip.ParseAddr(u.GetAddress())
		if err != nil {
			return
		}
		upstreams[family(a)] = true
	}
	for f, name := range []string{"IPv4", "IPv6"} {
		if listens[f] && !upstreams[f] {
			w.warnings = append(w.warnings, fmt.Sprintf("route %s hop %d on %s: no %s upstream, so %s clients are not forwarded",
				routeID, index, ref, name, name))
		}
	}
}

// family answers 0 for IPv4 (and IPv4-mapped IPv6), 1 for IPv6.
func family(a netip.Addr) int {
	if a.Unmap().Is4() {
		return 0
	}
	return 1
}
