package networkmodel

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"google.golang.org/protobuf/proto"
)

// Forward is a forward route on the network model (forward.v1
// ENGINE_NETWORK; design implementation.md step 5) as the planner takes
// it: a port on each entry device, the transports its hops chain through,
// and the internet egress on the last device that dials the targets.
// Unlike a path, a forward may share its transports with paths and other
// forwards as long as each transport continues the same way.
//
// Several entries are entry HA: each runs the same port forward. The
// first entry starts on Segments[0] (or leaves by Egress when there are no
// segments); every other entry starts on a transport of its own that
// reaches the same ingress and then continues on Segments[1:] (or leaves
// by an internet egress of its own device), chosen by the planner: see
// entryChain.
type Forward struct {
	// ID is the forward.v1 route id.
	ID string
	// Entries are the entry hop's nodes (proxy-<id> or forward-<id>).
	Entries   []string
	Listen    Endpoint
	Protocols []string
	Targets   []*networkv1.ForwardTarget
	Policy    *networkv1.ForwardPolicy
	// Segments are the transports in order; none when the entry device
	// dials the targets itself.
	Segments []string
	// Via are the nodes of the hops after the entry, when known: segment j
	// must reach Via[j].
	Via []string
	// Egress is the internet egress on the last device.
	Egress string
	// Paused routes are left out.
	Paused bool
}

// planned is what Plan has put into the plans so far, for the forwards
// to join.
type planned struct {
	plans      func(node string) *networkv1.NetworkPlan
	nodeOf     func(line string) string
	ingresses  map[string]*Ingress
	egresses   map[string]*Egress
	transports map[string]*Transport
	lines      map[string]*Line
	devices    map[string]Device
	keys       map[string]string
	// next is how each planned transport continues: "t:<id>" into a
	// transport, "e:<id>" to an egress.
	next map[string]string
	// peers are the dialing sources per hop ingress.
	peers map[string][]string
	// listens are the taken listen keys per node.
	listens map[string]map[string]string
}

func listenKey(address string, port uint32) string {
	if ip, err := netip.ParseAddr(address); err == nil {
		address = ip.Unmap().String()
	}
	return net.JoinHostPort(address, strconv.FormatUint(uint64(port), 10))
}

// take records node's listen key for owner, answering the owner already
// there, if any.
func (p *planned) take(node, key, owner string) string {
	if p.listens[node] == nil {
		p.listens[node] = map[string]string{}
	}
	if other, taken := p.listens[node][key]; taken {
		return other
	}
	p.listens[node][key] = owner
	return ""
}

// addTransport puts transport id, continuing by next, into the plans of
// its two devices, once.
func (p *planned) addTransport(id, next string) {
	if _, done := p.next[id]; done {
		return
	}
	p.next[id] = next
	transport := p.transports[id]
	from, to := p.egresses[transport.FromEgress], p.ingresses[transport.ToIngress]
	dialer, accepter := p.plans(p.nodeOf(from.Line)), p.plans(p.nodeOf(to.Line))
	dialer.Transports = append(dialer.Transports, transportProto(transport, networkv1.TransportRole_TRANSPORT_ROLE_DIAL,
		p.nodeOf(to.Line), p.keys[id], to, p.devices[p.nodeOf(to.Line)]))
	accepter.Transports = append(accepter.Transports, transportProto(transport, networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT,
		p.nodeOf(from.Line), p.keys[id], to, Device{}))
	forwarding := &networkv1.Forwarding{FromTransportId: id}
	if kind, target := next[:2], next[2:]; kind == "t:" {
		forwarding.Next = &networkv1.Forwarding_TransportId{TransportId: target}
	} else {
		forwarding.Next = &networkv1.Forwarding_EgressId{EgressId: target}
	}
	accepter.Forwarding = append(accepter.Forwarding, forwarding)
	if ip, err := netip.ParseAddr(from.Source); err == nil {
		p.peers[to.ID] = append(p.peers[to.ID], netip.PrefixFrom(ip, ip.BitLen()).String())
	}
}

// continuation answers how segment j of segments continues.
func continuation(segments []string, j int, egress string) string {
	if j+1 < len(segments) {
		return "t:" + segments[j+1]
	}
	return "e:" + egress
}

// entryChain answers the segments and egress entry k of forward runs, or
// why it has none. The first entry runs the forward's own. Another entry
// starts on a transport leaving its device for the ingress Segments[0]
// reaches (by id, the first that is enabled, has a link key and does not
// continue elsewhere), or, without segments, leaves by an enabled internet
// egress of its device (one of the named egress's kind first, then by
// id).
func (p *planned) entryChain(forward *Forward, k int) ([]string, string, string) {
	if k == 0 {
		return forward.Segments, forward.Egress, ""
	}
	entry := forward.Entries[k]
	if len(forward.Segments) == 0 {
		var kind string
		if named := p.egresses[forward.Egress]; named != nil {
			kind = named.Kind
		}
		var candidates []*Egress
		for _, egress := range p.egresses {
			if egress.Audience == "internet" && !egress.Disabled && p.nodeOf(egress.Line) == entry {
				candidates = append(candidates, egress)
			}
		}
		if len(candidates) == 0 {
			return nil, "", fmt.Sprintf("%s has no enabled internet egress", entry)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if a, b := candidates[i].Kind == kind, candidates[j].Kind == kind; a != b {
				return a
			}
			return candidates[i].ID < candidates[j].ID
		})
		return nil, candidates[0].ID, ""
	}
	first := p.transports[forward.Segments[0]]
	if first == nil {
		return nil, "", fmt.Sprintf("%q is not a transport", forward.Segments[0])
	}
	next := continuation(forward.Segments, 0, forward.Egress)
	ids := make([]string, 0, len(p.transports))
	for id := range p.transports {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		transport := p.transports[id]
		from, to := p.egresses[transport.FromEgress], p.ingresses[transport.ToIngress]
		if transport.ToIngress != first.ToIngress || from == nil || to == nil || p.nodeOf(from.Line) != entry ||
			from.Disabled || to.Disabled || p.keys[id] == "" {
			continue
		}
		if planned, ok := p.next[id]; ok && planned != next {
			continue
		}
		return append([]string{id}, forward.Segments[1:]...), forward.Egress, ""
	}
	return nil, "", fmt.Sprintf("no enabled transport with a link key leaves %s for ingress %s (as %s does)", entry, first.ToIngress, first.ID)
}

// whyNot answers why entry cannot run forward over segments and egress
// now, or "": a broken chain, a disabled part, a missing key, a transport
// that continues elsewhere for a path or another forward. A forward is
// never an error of the document: it waits instead.
func (p *planned) whyNot(forward *Forward, entry string, segments []string, egressID string) string {
	at := entry
	known := false
	for _, line := range p.lines {
		known = known || line.Node == at
	}
	if !known {
		return fmt.Sprintf("%s has no line in the network model", at)
	}
	egress := p.egresses[egressID]
	switch {
	case egress == nil || egress.Audience != "internet":
		return fmt.Sprintf("%q is not an internet egress", egressID)
	case egress.Disabled:
		return fmt.Sprintf("egress %s is disabled", egressID)
	}
	for j, id := range segments {
		transport := p.transports[id]
		if transport == nil {
			return fmt.Sprintf("%q is not a transport", id)
		}
		from, to := p.egresses[transport.FromEgress], p.ingresses[transport.ToIngress]
		if from.Disabled || to.Disabled {
			return fmt.Sprintf("transport %s is disabled", id)
		}
		if node := p.nodeOf(from.Line); node != at {
			return fmt.Sprintf("transport %s leaves %s, but the route is at %s", id, node, at)
		}
		at = p.nodeOf(to.Line)
		if j < len(forward.Via) && forward.Via[j] != at {
			return fmt.Sprintf("transport %s reaches %s, but hop %d is on %s", id, at, j+1, forward.Via[j])
		}
		if p.keys[id] == "" {
			return fmt.Sprintf("transport %s has no link key", id)
		}
		if p.devices[at].PublicKeys[network.TransportAnxLine] == "" {
			return fmt.Sprintf("%s has not reported its ANX-Line server key", at)
		}
		if next, planned := p.next[id]; planned && next != continuation(segments, j, egressID) {
			return fmt.Sprintf("transport %s already continues elsewhere (%s)", id, next[2:])
		}
	}
	if node := p.nodeOf(egress.Line); node != at {
		return fmt.Sprintf("egress %s is on %s, but the route ends at %s", egressID, node, at)
	}
	return ""
}

// planForwards adds the forwards to the plans: each entry device gets a
// port forward, each transport is planned once, and an entry that cannot
// be planned now is pending (named in the reason when the forward has
// several), while the others run.
func (p *planned) planForwards(forwards []Forward) []Pending {
	var pending []Pending
	sorted := append([]Forward(nil), forwards...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	for i := range sorted {
		forward := &sorted[i]
		if forward.Paused {
			continue
		}
		if !idPattern.MatchString(forward.ID) {
			pending = append(pending, Pending{Forward: forward.ID, Reason: "the route id is not a component id"})
			continue
		}
		for k, entry := range forward.Entries {
			wait := func(reason string) {
				if len(forward.Entries) > 1 {
					reason = "entry " + entry + ": " + reason
				}
				pending = append(pending, Pending{Forward: forward.ID, Reason: reason})
			}
			segments, egress, reason := p.entryChain(forward, k)
			if reason == "" {
				reason = p.whyNot(forward, entry, segments, egress)
			}
			if reason != "" {
				wait(reason)
				continue
			}
			out := &networkv1.PortForward{
				Id: forward.ID, Listen: &networkv1.Endpoint{Address: forward.Listen.Address, Port: forward.Listen.Port},
				Protocols: forward.Protocols, Policy: forward.Policy,
			}
			for _, target := range forward.Targets {
				out.Targets = append(out.Targets, proto.Clone(target).(*networkv1.ForwardTarget))
			}
			if len(segments) == 0 {
				out.Next = &networkv1.PortForward_EgressId{EgressId: egress}
			} else {
				out.Next = &networkv1.PortForward_TransportId{TransportId: segments[0]}
			}
			// Checked alone first, so a bad forward waits instead of failing
			// the entry's plan.
			if err := network.CheckPortForward(out); err != nil {
				wait(err.Error())
				continue
			}
			if other := p.take(entry, listenKey(forward.Listen.Address, forward.Listen.Port), "forward "+forward.ID); other != "" {
				wait(fmt.Sprintf("listen %s:%d is taken by %s", forward.Listen.Address, forward.Listen.Port, other))
				continue
			}
			for j, id := range segments {
				p.addTransport(id, continuation(segments, j, egress))
			}
			plan := p.plans(entry)
			plan.PortForwards = append(plan.PortForwards, out)
		}
	}
	return pending
}
