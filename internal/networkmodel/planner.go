package networkmodel

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
)

// Device is what the planner knows of a device beyond the document: the
// public keys its Agent reported (NetworkStatus.public_keys, by kind).
type Device struct {
	PublicKeys map[string]string
}

// Pending is a path the planner left out of the plans for now, and why:
// an ANX-Line transport whose accepting device has not reported its
// server key yet.
type Pending struct {
	Path string `json:"path,omitempty"`
	// Forward is a forward route left out instead of a path.
	Forward string `json:"forward,omitempty"`
	// ReverseProxy is a reverse proxy left out instead of a path.
	ReverseProxy string `json:"reverse_proxy,omitempty"`
	Reason       string `json:"reason"`
}

// Result is the planner's answer: one plan per device that has a line,
// unstamped (generation 0, no hash), and the paths left out.
type Result struct {
	Plans   map[string]*networkv1.NetworkPlan
	Pending []Pending
}

var idPattern = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// Plan turns the document into plans. keys are the transports' ANX-Line
// link keys (64 hex digits) by transport id; every anx_line transport of
// an active path needs one. devices are the devices' reported facts by
// node reference.
//
// Beyond sdk/network.Check, which every plan must pass, Plan enforces the
// rules that span devices (design section 10): a path's segments chain
// from the ingress's device through each transport's accepting device to
// the egress's device; a transport joins two different devices and
// carries at most one path (N3: one link per path); a disabled ingress,
// egress or path takes its paths out of the plans.
func Plan(doc *Document, keys map[string]string, devices map[string]Device, forwards []Forward) (*Result, error) {
	var v violations
	for name, n := range map[string]int{"lines": len(doc.Lines), "ingresses": len(doc.Ingresses), "egresses": len(doc.Egresses),
		"transports": len(doc.Transports), "paths": len(doc.Paths)} {
		if n > MaxComponents {
			v.add(name, "more than %d", MaxComponents)
		}
	}
	if err := v.err(); err != nil {
		return nil, err
	}

	lines := map[string]*Line{}
	for i := range doc.Lines {
		line := &doc.Lines[i]
		field := fmt.Sprintf("lines[%d]", i)
		uniqueID(&v, field, line.ID, lines)
		lines[line.ID] = line
		if node, err := agentcontrol.ParseAgentNode(line.Node); err != nil || node.ID == 0 || node.String() != line.Node {
			v.add(field+".node", "must be proxy-<id> or forward-<id>")
		}
	}
	ingresses := map[string]*Ingress{}
	for i := range doc.Ingresses {
		ingress := &doc.Ingresses[i]
		field := fmt.Sprintf("ingresses[%d]", i)
		uniqueID(&v, field, ingress.ID, ingresses)
		ingresses[ingress.ID] = ingress
		if lines[ingress.Line] == nil {
			v.add(field+".line", "%q is not a line", ingress.Line)
		}
		if ingress.Audience != "user" && ingress.Audience != "hop" {
			v.add(field+".audience", "must be user or hop")
		}
	}
	egresses := map[string]*Egress{}
	for i := range doc.Egresses {
		egress := &doc.Egresses[i]
		field := fmt.Sprintf("egresses[%d]", i)
		uniqueID(&v, field, egress.ID, egresses)
		egresses[egress.ID] = egress
		if lines[egress.Line] == nil {
			v.add(field+".line", "%q is not a line", egress.Line)
		}
		if egress.Audience != "internet" && egress.Audience != "hop" {
			v.add(field+".audience", "must be internet or hop")
		}
	}
	transports := map[string]*Transport{}
	for i := range doc.Transports {
		transport := &doc.Transports[i]
		field := fmt.Sprintf("transports[%d]", i)
		uniqueID(&v, field, transport.ID, transports)
		transports[transport.ID] = transport
		checkTransportKind(&v, field, transport)
		from, to := egresses[transport.FromEgress], ingresses[transport.ToIngress]
		if from == nil || from.Audience != "hop" {
			v.add(field+".from_egress", "%q is not a hop egress", transport.FromEgress)
		}
		if to == nil || to.Audience != "hop" {
			v.add(field+".to_ingress", "%q is not a hop ingress", transport.ToIngress)
		}
		if from != nil && to != nil && lines[from.Line] != nil && lines[to.Line] != nil && lines[from.Line].Node == lines[to.Line].Node {
			v.add(field, "joins %s to itself; a transport joins two devices", lines[from.Line].Node)
		}

	}
	paths := map[string]*Path{}
	for i := range doc.Paths {
		uniqueID(&v, fmt.Sprintf("paths[%d]", i), doc.Paths[i].ID, paths)
		paths[doc.Paths[i].ID] = &doc.Paths[i]
	}
	if err := v.err(); err != nil {
		return nil, err
	}
	nodeOf := func(line string) string { return lines[line].Node }

	// The active paths: chained, enabled, each transport carrying one.
	result := &Result{Plans: map[string]*networkv1.NetworkPlan{}}
	carried := map[string]string{}
	var active []*Path
	for i := range doc.Paths {
		path := &doc.Paths[i]
		field := fmt.Sprintf("paths[%d]", i)
		ingress, egress := ingresses[path.Ingress], egresses[path.Egress]
		if ingress == nil || ingress.Audience != "user" {
			v.add(field+".ingress", "%q is not a user ingress", path.Ingress)
			continue
		}
		if egress == nil || egress.Audience != "internet" {
			v.add(field+".egress", "%q is not an internet egress", path.Egress)
			continue
		}
		at := nodeOf(ingress.Line)
		chained := true
		for j, id := range path.Segments {
			transport := transports[id]
			if transport == nil {
				v.add(fmt.Sprintf("%s.segments[%d]", field, j), "%q is not a transport", id)
				chained = false
				break
			}
			if from := nodeOf(egresses[transport.FromEgress].Line); from != at {
				v.add(fmt.Sprintf("%s.segments[%d]", field, j), "transport %q leaves %s, but the path is at %s", id, from, at)
				chained = false
				break
			}
			at = nodeOf(ingresses[transport.ToIngress].Line)
		}
		if chained && nodeOf(egress.Line) != at {
			v.add(field+".egress", "%q is on %s, but the path ends at %s", path.Egress, nodeOf(egress.Line), at)
			chained = false
		}
		if !chained || path.Disabled || ingress.Disabled || egress.Disabled {
			continue
		}
		for _, id := range path.Segments {
			if other, taken := carried[id]; taken {
				v.add(field+".segments", "transport %q already carries path %q (one link per path)", id, other)
				chained = false
			}
			carried[id] = path.ID
			transport := transports[id]
			if egresses[transport.FromEgress].Disabled || ingresses[transport.ToIngress].Disabled {
				chained = false
			}
		}
		if !chained {
			continue
		}
		if reason := pendingReason(path, transports, ingresses, lines, devices, keys); reason != "" {
			result.Pending = append(result.Pending, Pending{Path: path.ID, Reason: reason})
			continue
		}
		active = append(active, path)
	}
	if err := v.err(); err != nil {
		return nil, err
	}

	plans := result.Plans
	planOf := func(node string) *networkv1.NetworkPlan {
		if plan := plans[node]; plan != nil {
			return plan
		}
		plan := &networkv1.NetworkPlan{DeviceRef: node}
		plans[node] = plan
		return plan
	}
	for i := range doc.Lines {
		line := &doc.Lines[i]
		planOf(line.Node).Lines = append(planOf(line.Node).Lines, lineProto(line))
	}
	for i := range doc.Egresses {
		if egress := &doc.Egresses[i]; !egress.Disabled {
			planOf(nodeOf(egress.Line)).Egresses = append(planOf(nodeOf(egress.Line)).Egresses, egressProto(egress))
		}
	}

	// Transports, paths and forwarding of the active paths, then the
	// forwards.
	p := &planned{plans: planOf, nodeOf: nodeOf, ingresses: ingresses, egresses: egresses, transports: transports,
		lines: lines, devices: devices, keys: keys, next: map[string]string{}, peers: map[string][]string{},
		listens: map[string]map[string]string{}}
	peers := p.peers // hop ingress id -> dialing sources
	for i := range doc.Ingresses {
		if ingress := &doc.Ingresses[i]; !ingress.Disabled {
			p.take(nodeOf(ingress.Line), listenKey(ingress.Listen.Address, ingress.Listen.Port), "ingress "+ingress.ID)
		}
	}
	finals := map[string]map[string]bool{}
	pathIDs := map[string]bool{}
	for _, path := range active {
		ingress := ingresses[path.Ingress]
		home := planOf(nodeOf(ingress.Line))
		out := &networkv1.Path{Id: path.ID, IngressId: path.Ingress, Labels: path.Labels, FinalEgressId: path.Egress}
		if len(path.Segments) == 0 {
			out.Next = &networkv1.Path_EgressId{EgressId: path.Egress}
		} else {
			out.Next = &networkv1.Path_TransportId{TransportId: path.Segments[0]}
		}
		home.Paths = append(home.Paths, out)
		pathIDs[path.ID] = true
		if finals[path.Ingress] == nil {
			finals[path.Ingress] = map[string]bool{}
		}
		finals[path.Ingress][path.Egress] = true
		for j := range path.Segments {
			p.addTransport(path.Segments[j], continuation(path.Segments, j, path.Egress))
		}
	}
	result.Pending = append(result.Pending, p.planForwards(forwards)...)
	for i := range doc.Ingresses {
		ingress := &doc.Ingresses[i]
		if ingress.Disabled {
			continue
		}
		planOf(nodeOf(ingress.Line)).Ingresses = append(planOf(nodeOf(ingress.Line)).Ingresses, ingressProto(ingress, peers[ingress.ID], finals[ingress.ID], pathIDs))
	}

	nodes := make([]string, 0, len(plans))
	for node := range plans {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if err := network.Check(plans[node]); err != nil {
			v.add("plans["+node+"]", "%v", err)
		}
	}
	if err := v.err(); err != nil {
		return nil, err
	}
	return result, nil
}

// pendingReason answers why a path cannot be planned yet, or "".
func pendingReason(path *Path, transports map[string]*Transport, ingresses map[string]*Ingress, lines map[string]*Line,
	devices map[string]Device, keys map[string]string,
) string {
	for _, id := range path.Segments {
		if keys[id] == "" {
			return fmt.Sprintf("transport %s has no link key", id)
		}
		accepter := lines[ingresses[transports[id].ToIngress].Line].Node
		if devices[accepter].PublicKeys[network.TransportAnxLine] == "" {
			return fmt.Sprintf("%s has not reported its ANX-Line server key", accepter)
		}
	}
	return ""
}

// checkTransportKind applies the transport rules: bare anx_line on trusted
// segments only, the jacketed anx (A2 jacket, c1) anywhere.
func checkTransportKind(v *violations, field string, transport *Transport) {
	options := transport.AnxLine
	if options == nil {
		options = &AnxLine{}
	}
	if options.Reverse {
		if transport.Kind != network.TransportAnxLine {
			v.add(field+".anx_line.reverse", "only bare anx_line (l1, c1) runs reversed")
		}
		if options.QUICPort != 0 || options.FakeTCPPort != 0 || options.Multipath {
			v.add(field+".anx_line", "a reversed link has no carrier ports and no multipath; carrier_order c2 / c3 dial back on the reverse port")
		}
		if options.ReverseHost == "" {
			v.add(field+".anx_line.reverse_host", "a reversed link needs the dialing device's address")
		}
		// c3 dials back over fake TCP to reverse_port + 1, IPv4 only (anx
		// ANX-LINE-REVERSE §6.2).
		for _, carrier := range options.CarrierOrder {
			if carrier != "c3" && carrier != "faketcp" {
				continue
			}
			if addr, err := netip.ParseAddr(options.ReverseHost); err == nil && addr.Is6() {
				v.add(field+".anx_line.carrier_order", "c3 dials back over IPv4 only; reverse_host is IPv6")
			}
			if options.ReversePort == 65535 {
				v.add(field+".anx_line.reverse_port", "c3 dials back to reverse_port + 1; 65535 leaves none")
			}
			break
		}
	} else if options.ReverseHost != "" || options.ReversePort != 0 {
		v.add(field+".anx_line", "reverse_host and reverse_port need reverse")
	}
	switch transport.Kind {
	case network.TransportAnxLine:
		if !transport.Trusted {
			v.add(field+".kind", "anx_line is bare and only for trusted segments (IX, private line, domestic); set trusted, or use kind anx (A2 jacket) off them")
		}
	case KindAnx:
		if options.Egress != "" && options.Egress != "l2" {
			v.add(field+".anx_line.egress", "kind anx is the A2 jacket (l2)")
		}
		if options.ServerName == "" {
			v.add(field+".anx_line.server_name", "kind anx needs the jacket's server name")
		}
		if options.QUICPort != 0 || options.FakeTCPPort != 0 {
			v.add(field+".anx_line", "kind anx runs on c1 (TCP) only; no quic_port or faketcp_port")
		}
		for _, carrier := range options.CarrierOrder {
			if carrier != "c1" {
				v.add(field+".anx_line.carrier_order", "kind anx runs on c1 (TCP) only")
				break
			}
		}
	default:
		v.add(field+".kind", "must be anx_line or anx")
	}
}

func uniqueID[T any](v *violations, field, id string, seen map[string]T) {
	if !idPattern.MatchString(id) {
		v.add(field+".id", "must be 1 to 64 ASCII letters, digits, '-' and '_'")
		return
	}
	if _, dup := seen[id]; dup {
		v.add(field+".id", "%q is listed twice", id)
	}
}

func family(name string) networkv1.Family {
	switch name {
	case "v4":
		return networkv1.Family_FAMILY_V4
	case "v6":
		return networkv1.Family_FAMILY_V6
	}
	return networkv1.Family_FAMILY_UNSPECIFIED
}

func version(v uint32) uint32 {
	if v == 0 {
		return 1
	}
	return v
}

func lineProto(line *Line) *networkv1.Line {
	out := &networkv1.Line{Id: line.ID, Name: line.Name, Interface: line.Interface, Carrier: line.Carrier,
		GatewayV4: line.GatewayV4, GatewayV6: line.GatewayV6}
	for _, f := range line.Families {
		out.Families = append(out.Families, family(f))
	}
	return out
}

func egressProto(egress *Egress) *networkv1.Egress {
	out := &networkv1.Egress{
		Id: egress.ID, LineId: egress.Line, Kind: egress.Kind, KindVersion: version(egress.KindVersion),
		Family: family(egress.Family), Source: egress.Source, Gateway: egress.Gateway,
		Mtu: egress.MTU, Advmss: egress.AdvMSS, Masquerade: egress.Masquerade,
		Audience: networkv1.Audience_AUDIENCE_INTERNET,
	}
	if egress.Audience == "hop" {
		out.Audience = networkv1.Audience_AUDIENCE_HOP
	}
	if egress.Table != 0 || egress.RulePriority != 0 {
		out.Routing = &networkv1.Routing{Table: egress.Table, RulePriority: egress.RulePriority}
	}
	if h := egress.Health; h != nil {
		out.Health = &networkv1.HealthCheck{Targets: h.Targets, IntervalMs: h.IntervalMs, TimeoutMs: h.TimeoutMs,
			FailureThreshold: h.FailureThreshold, RecoveryThreshold: h.RecoveryThreshold}
	}
	return out
}

var pairingModes = map[string]networkv1.PairingMode{
	"":          networkv1.PairingMode_PAIRING_MODE_SAME_LINE,
	"same_line": networkv1.PairingMode_PAIRING_MODE_SAME_LINE,
	"cross":     networkv1.PairingMode_PAIRING_MODE_CROSS,
	"failover":  networkv1.PairingMode_PAIRING_MODE_FAILOVER,
	"rules":     networkv1.PairingMode_PAIRING_MODE_RULES,
}

// ingressProto writes an ingress. A user ingress's pairing keeps only the
// egresses and fallback that planned paths reach (finals, paths): a
// failover or cross pairing whose paths are all pending falls back to
// same_line until they are planned.
func ingressProto(ingress *Ingress, peers []string, finals map[string]bool, paths map[string]bool) *networkv1.Ingress {
	out := &networkv1.Ingress{
		Id: ingress.ID, LineId: ingress.Line, Kind: ingress.Kind, KindVersion: version(ingress.KindVersion),
		Family: family(ingress.Family), Listen: &networkv1.Endpoint{Address: ingress.Listen.Address, Port: ingress.Listen.Port},
		PublicHost: ingress.PublicHost, Protocols: ingress.Protocols, AllowedPeers: peers,
		Audience: networkv1.Audience_AUDIENCE_HOP,
	}
	for _, m := range ingress.PortMap {
		out.PortMap = append(out.PortMap, &networkv1.PortMap{
			Public: &networkv1.PortRange{First: m.Public.First, Last: m.Public.Last},
			Local:  &networkv1.PortRange{First: m.Local.First, Last: m.Local.Last},
		})
	}
	if ingress.Audience != "user" {
		return out
	}
	out.Audience = networkv1.Audience_AUDIENCE_USER
	pairing := ingress.Pairing
	if pairing == nil {
		pairing = &Pairing{}
	}
	mode, ok := pairingModes[pairing.Mode]
	if !ok {
		mode = networkv1.PairingMode_PAIRING_MODE_UNSPECIFIED // Check refuses it
	}
	out.Pairing = &networkv1.Pairing{Mode: mode, FailbackDelayMs: pairing.FailbackDelayMs}
	for _, id := range pairing.Egresses {
		if finals[id] {
			out.Pairing.EgressIds = append(out.Pairing.EgressIds, id)
		}
	}
	for _, rule := range pairing.Rules {
		out.Pairing.Rules = append(out.Pairing.Rules, &networkv1.PairingRule{Match: rule.Match, PathLabel: rule.PathLabel})
	}
	if paths[pairing.FallbackPath] {
		out.Pairing.FallbackPathId = pairing.FallbackPath
	}
	if (mode == networkv1.PairingMode_PAIRING_MODE_FAILOVER || mode == networkv1.PairingMode_PAIRING_MODE_CROSS) && len(out.Pairing.EgressIds) == 0 && len(pairing.Egresses) > 0 {
		out.Pairing.Mode, out.Pairing.EgressIds = networkv1.PairingMode_PAIRING_MODE_SAME_LINE, nil
	}
	return out
}

// transportProto writes one end of a transport. The dialing end gets the
// accepting device's server key and its hop ingress's addresses (the public
// host when there is one); the accepting end gets the addresses it listens
// on: the hop ingress's listen address with the port of each carrier.
func transportProto(transport *Transport, role networkv1.TransportRole, peer, key string, to *Ingress, accepter Device) *networkv1.Transport {
	out := &networkv1.Transport{
		Id: transport.ID, Kind: network.TransportAnxLine, KindVersion: version(transport.KindVersion), Role: role,
		PeerDeviceRef: peer, Mtu: transport.MTU,
	}
	if role == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
		out.FromEgressId = transport.FromEgress
	} else {
		out.ToIngressId = transport.ToIngress
	}
	options := transport.AnxLine
	if options == nil {
		options = &AnxLine{}
	}
	anx := &networkv1.AnxLineSettings{LinkId: transport.ID, LinkKey: key, CarrierOrder: options.CarrierOrder,
		Multipath: options.Multipath, Fec: options.FEC, Egress: options.Egress, ServerName: options.ServerName}
	if transport.Kind == KindAnx {
		anx.Egress = "l2"
	}
	if options.Reverse {
		// The accepting end dials back; no carrier addresses either way.
		// c2 / c3 in the order dial back too, on the reverse port (UDP) and
		// the next TCP port (anx ANX-LINE-REVERSE §6.3); none is c1 only.
		anx.Reverse = true
		if len(anx.CarrierOrder) == 0 {
			anx.CarrierOrder = []string{"c1"}
		}
		port := options.ReversePort
		if port == 0 {
			port = DefaultReversePort
		}
		p := strconv.FormatUint(uint64(port), 10)
		if role == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
			anx.ServerPublic = accepter.PublicKeys[network.TransportAnxLine]
			listen := "0.0.0.0"
			if addr, err := netip.ParseAddr(options.ReverseHost); err == nil && addr.Is6() {
				listen = "::"
			}
			anx.ReverseListen = net.JoinHostPort(listen, p)
		} else {
			anx.ReverseDial = net.JoinHostPort(options.ReverseHost, p)
		}
		out.Settings = &networkv1.Transport_AnxLine{AnxLine: anx}
		return out
	}
	if to != nil {
		host := to.Listen.Address
		if role == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
			if to.PublicHost != "" {
				host = to.PublicHost
			}
			anx.ServerPublic = accepter.PublicKeys[network.TransportAnxLine]
		}
		port := to.Listen.Port
		if role == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
			port = publicPort(to, port)
		}
		anx.TcpAddr = net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
		for _, carrier := range []struct {
			port uint32
			addr *string
		}{{options.QUICPort, &anx.QuicAddr}, {options.FakeTCPPort, &anx.FaketcpAddr}} {
			if carrier.port == 0 {
				continue
			}
			p := carrier.port
			if role == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
				p = publicPort(to, p)
			}
			*carrier.addr = net.JoinHostPort(host, strconv.FormatUint(uint64(p), 10))
		}
	}
	out.Settings = &networkv1.Transport_AnxLine{AnxLine: anx}
	return out
}

// publicPort answers the port a dialer reaches ingress's local port at: the
// public port a NAT ingress maps it to, or the port itself.
func publicPort(ingress *Ingress, port uint32) uint32 {
	for _, m := range ingress.PortMap {
		if port >= m.Local.First && port <= m.Local.Last {
			return m.Public.First + (port - m.Local.First)
		}
	}
	return port
}
