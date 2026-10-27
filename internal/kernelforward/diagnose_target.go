package kernelforward

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	internalmodel "github.com/AnixOps/anix-control/v5/internal/model"
)

// Step codes of the target check (stage 1).
const (
	// CodeTargetProtocol: the target is a proxy node's enabled protocol.
	CodeTargetProtocol = "target_protocol"
	// CodeTargetNoProtocol: the target's host is a proxy node, but no
	// enabled protocol of the node listens on the target's port.
	CodeTargetNoProtocol = "target_no_protocol"
	// CodeTargetL4Mismatch: the target's protocol runs on TCP (or UDP) and
	// the route does not forward it.
	CodeTargetL4Mismatch = "target_l4_mismatch"
)

// targetSteps checks, from Control's records, each target whose host is a
// proxy node's address (the node's host, or a protocol's own host): that an
// enabled protocol of that node listens on the target's port, and that the
// route forwards the transport the protocol runs on. A route to a proxy
// node's port where nothing listens connects and then fails in the client,
// and a UDP probe of it is only ever inconclusive. Hosts are compared as
// written (an IP address or a name), without resolving names.
func (d *diagnosis) targetSteps(ctx context.Context) {
	targets := d.route.GetTargets()
	if len(targets) == 0 {
		return
	}
	db, err := d.s.db(ctx)
	if err != nil {
		return
	}
	var nodes []internalmodel.Node
	if err := db.Select("id", "name", "host").Find(&nodes).Error; err != nil {
		return
	}
	var protocols []internalmodel.NodeProtocol
	if err := db.Select("id", "node_id", "name", "type", "port", "enable", "host").Order("sort ASC, id ASC").Find(&protocols).Error; err != nil {
		return
	}
	names := map[uint]string{}
	byHost := map[string]map[uint]bool{}
	addHost := func(host string, nodeID uint) {
		key := normalizeTargetHost(host)
		if key == "" {
			return
		}
		if byHost[key] == nil {
			byHost[key] = map[uint]bool{}
		}
		byHost[key][nodeID] = true
	}
	for _, node := range nodes {
		names[node.ID] = node.Name
		addHost(node.Host, node.ID)
	}
	for _, protocol := range protocols {
		if protocol.Host != nil {
			addHost(*protocol.Host, protocol.NodeID)
		}
	}
	tcp, udp := routeProtocols(d.route)
	last := uint32(len(d.route.GetHops()) - 1) // #nosec G115 -- validate bounds hops to MaxHops.
	if len(d.route.GetHops()) == 0 {
		last = 0
	}
	for _, target := range targets {
		matched := byHost[normalizeTargetHost(target.GetHost())]
		if len(matched) == 0 {
			continue
		}
		address := hostPort(target.GetHost(), target.GetPort())
		step := &forwardv1.DiagnoseStep{HopIndex: last, Kind: forwardv1.ProbeKind_PROBE_KIND_CONFIG, Vantage: vantageControl, Target: address}
		var found *internalmodel.NodeProtocol
		var ports []string
		var nodeNames []string
		for _, protocol := range protocols {
			if !matched[protocol.NodeID] || protocol.Enable != 1 {
				continue
			}
			if protocol.Port == int(target.GetPort()) && found == nil {
				p := protocol
				found = &p
			}
			ports = append(ports, strconv.Itoa(protocol.Port))
		}
		for id := range matched {
			nodeNames = append(nodeNames, names[id])
		}
		sort.Strings(nodeNames)
		node := strings.Join(nodeNames, ", ")
		switch {
		case found == nil && len(ports) == 0:
			step.Result = result(statusFailed, CodeTargetNoProtocol, fmt.Sprintf("the target is proxy node %s, which has no enabled protocol: nothing listens on port %d", node, target.GetPort()))
		case found == nil:
			step.Result = result(statusFailed, CodeTargetNoProtocol, fmt.Sprintf("the target is proxy node %s, but none of its enabled protocols listens on port %d (its ports: %s): point the route at a protocol's port",
				node, target.GetPort(), strings.Join(uniqueStrings(ports), ", ")))
		default:
			needTCP, needUDP := protocolTransports(found)
			description := fmt.Sprintf("%s %s (%s, port %d)", names[found.NodeID], found.Name, found.Type, found.Port)
			switch {
			case needTCP && !tcp:
				step.Protocol = protoTCP
				step.Result = result(statusFailed, CodeTargetL4Mismatch, "the target is "+description+", which runs on TCP, but the route forwards only UDP: make the route TCP (or TCP and UDP)")
			case needUDP && !udp:
				step.Protocol = protoUDP
				step.Result = result(statusFailed, CodeTargetL4Mismatch, "the target is "+description+", which runs on UDP, but the route forwards only TCP: make the route UDP (or TCP and UDP)")
			default:
				step.Result = result(statusOK, CodeTargetProtocol, "the target is "+description)
			}
		}
		d.add(step)
	}
}

// protocolTransports answers whether clients reach the protocol over TCP
// and over UDP. Shadowsocks relays UDP too, but a client always needs its
// TCP port.
func protocolTransports(protocol *internalmodel.NodeProtocol) (tcp, udp bool) {
	switch protocol.Type {
	case internalmodel.ProtocolHysteria2, internalmodel.ProtocolTUIC, internalmodel.ProtocolWireGuard, internalmodel.ProtocolIWAN:
		return false, true
	}
	return true, false
}

// normalizeTargetHost is a host as the target check compares it: an IP
// address in its canonical form, or a lower-case name without the final dot.
func normalizeTargetHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
