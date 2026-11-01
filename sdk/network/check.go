package network

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"unicode/utf8"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
)

// Kinds of the first batch (design section 8.1). A plan may name others; a
// device runs a kind only when its DeviceCapabilities list it.
const (
	IngressDirect = "direct"
	IngressNAT    = "nat"

	EgressNATDirect   = "nat_direct"
	EgressDedicatedIP = "dedicated_ip"
	EgressHop         = "hop"

	TransportLocal        = "local"
	TransportWireGuard    = "wireguard"
	TransportAnxLine      = "anx_line"
	TransportIWAN         = "iwan"
	TransportSS2022Relay  = "ss2022_relay"
	TransportAnixOpsRelay = "anixops_relay"
	TransportExternal     = "external"

	// SourceAuto is an egress source the Agent resolves to the line
	// interface's preferred address of the family.
	SourceAuto = "auto"
)

const (
	// DefaultRoutingFirst and DefaultRoutingLast are the policy-routing
	// table and rule-priority range an Agent owns unless its capabilities
	// name another (design section 6.5).
	DefaultRoutingFirst = 20000
	DefaultRoutingLast  = 20999

	// MaxComponents caps each component list of a plan, a status and the
	// kinds of the capabilities.
	MaxComponents = 4096
	// MaxTextBytes caps the free-text fields; longer ones are refused.
	MaxTextBytes = 1024
	// MaxKeyBytes caps a public key in its text form: an anx-server hybrid
	// key (X25519 and ML-KEM-768) is about 2.5 KB of text.
	MaxKeyBytes = 8192
	// MaxPort is the highest port.
	MaxPort = 65535
)

var (
	idPattern   = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)
	kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// carriers of AnxLineSettings.carrier_order (the mgmt.proto LinePolicy).
	anxCarriers = map[string]bool{"c1": true, "c2": true, "c3": true, "tcp": true, "quic": true, "faketcp": true}
)

// CheckDeviceCapabilities bounds caps: device_ref empty or deviceRef, at
// most MaxComponents kinds, each a known component with a kind name, at
// least one version and no (component, kind) twice; a routing range either
// unset or within 1..2^31-1 with first <= last.
func CheckDeviceCapabilities(caps *networkv1.DeviceCapabilities, deviceRef string) error {
	if caps == nil {
		return invalid("device capabilities are required")
	}
	if caps.GetDeviceRef() != "" && caps.GetDeviceRef() != deviceRef {
		return invalid("device capabilities name %q, not the stream's device %q", caps.GetDeviceRef(), deviceRef)
	}
	if len(caps.GetKinds()) > MaxComponents {
		return invalid("more than %d kinds", MaxComponents)
	}
	type key struct {
		component networkv1.Component
		kind      string
	}
	seen := map[key]bool{}
	for i, support := range caps.GetKinds() {
		if !knownComponent(support.GetComponent()) {
			return invalid("kinds[%d]: unknown component %d", i, support.GetComponent())
		}
		if !kindPattern.MatchString(support.GetKind()) {
			return invalid("kinds[%d]: kind must be 1 to 32 lower-case letters, digits and '_'", i)
		}
		if len(support.GetVersions()) == 0 || len(support.GetVersions()) > 64 {
			return invalid("kinds[%d]: %s needs 1 to 64 versions", i, support.GetKind())
		}
		if err := checkText(support.GetExecutor()); err != nil {
			return invalid("kinds[%d]: %v", i, err)
		}
		k := key{support.GetComponent(), support.GetKind()}
		if seen[k] {
			return invalid("kinds[%d]: %s %s listed twice", i, k.component, k.kind)
		}
		seen[k] = true
	}
	first, last := caps.GetRoutingFirst(), caps.GetRoutingLast()
	if first != 0 || last != 0 {
		if first == 0 || first > last || last > 1<<31-1 {
			return invalid("routing range %d–%d is not 1 <= first <= last < 2^31", first, last)
		}
	}
	return nil
}

// RoutingRange answers the table and priority range caps names, or the
// default.
func RoutingRange(caps *networkv1.DeviceCapabilities) (first, last uint32) {
	if caps.GetRoutingFirst() == 0 {
		return DefaultRoutingFirst, DefaultRoutingLast
	}
	return caps.GetRoutingFirst(), caps.GetRoutingLast()
}

// Check validates a plan on its own, the invariants of design section 10
// that one device's plan can show:
//
//   - ids of 1 to 64 ASCII letters, digits, '-' and '_', unique per
//     component type; kinds of 1 to 32 lower-case letters, digits and '_';
//     known enums; at most MaxComponents of each component;
//   - every reference (line, egress, ingress, transport, path) names a
//     component of the plan of the right kind and audience;
//   - an egress's source and gateway are addresses of its family (or
//     source "auto"); its table and rule priority (checked against the
//     device's routing range by Unsupported);
//   - no two ingresses and accepting transports' ingresses bind the same
//     address and port; a nat ingress's port maps are of equal length and
//     do not overlap;
//   - a user ingress has a pairing; a hop ingress has none and no
//     protocols; cross names one egress, failover at least one;
//   - a transport's role matches its kind and references (dial: a hop
//     egress; accept: a hop ingress; local: neither); anx_line settings
//     are present exactly for kind anx_line, with link_id equal to the
//     transport id, a 64-hex link key, and for the dialing end the server
//     key and at least one carrier address (a reverse link has none: its
//     dialing end has reverse_listen, its accepting end reverse_dial and,
//     on l2, the dialing end's server key);
//   - a path starts at a user ingress and goes to an internet egress or a
//     dialing transport; forwarding leaves an accepting transport the same
//     way, each transport at most once.
//
// Kinds the device does not run are Unsupported's to report, not Check's.
func Check(plan *networkv1.NetworkPlan) error {
	if plan == nil {
		return invalid("plan is required")
	}
	if plan.GetDeviceRef() == "" || len(plan.GetDeviceRef()) > MaxTextBytes {
		return invalid("device_ref is required")
	}
	if hash := plan.GetHash(); hash != "" && !hashPattern.MatchString(hash) {
		return invalid("hash is not a lowercase hex SHA-256")
	}
	for name, n := range map[string]int{
		"lines": len(plan.GetLines()), "ingresses": len(plan.GetIngresses()), "egresses": len(plan.GetEgresses()),
		"transports": len(plan.GetTransports()), "paths": len(plan.GetPaths()), "forwarding": len(plan.GetForwarding()),
		"port_forwards": len(plan.GetPortForwards()), "reverse_proxies": len(plan.GetReverseProxies()),
	} {
		if n > MaxComponents {
			return invalid("more than %d %s", MaxComponents, name)
		}
	}

	lines := map[string]*networkv1.Line{}
	for i, line := range plan.GetLines() {
		if err := checkID(line.GetId(), lines); err != nil {
			return invalid("lines[%d]: %v", i, err)
		}
		lines[line.GetId()] = line
		for _, text := range []string{line.GetName(), line.GetInterface(), line.GetCarrier()} {
			if err := checkText(text); err != nil {
				return invalid("lines[%d]: %v", i, err)
			}
		}
		for _, family := range line.GetFamilies() {
			if !knownFamily(family) {
				return invalid("lines[%d]: unknown family %d", i, family)
			}
		}
		if err := checkGateway(line.GetGatewayV4(), networkv1.Family_FAMILY_V4); err != nil {
			return invalid("lines[%d]: gateway_v4: %v", i, err)
		}
		if err := checkGateway(line.GetGatewayV6(), networkv1.Family_FAMILY_V6); err != nil {
			return invalid("lines[%d]: gateway_v6: %v", i, err)
		}
	}

	egresses := map[string]*networkv1.Egress{}
	for i, egress := range plan.GetEgresses() {
		if err := checkEgress(egress, egresses, lines); err != nil {
			return invalid("egresses[%d]: %v", i, err)
		}
		egresses[egress.GetId()] = egress
	}

	ingresses := map[string]*networkv1.Ingress{}
	listens := map[string]string{}
	for i, ingress := range plan.GetIngresses() {
		if err := checkIngress(ingress, ingresses, lines, egresses, listens); err != nil {
			return invalid("ingresses[%d]: %v", i, err)
		}
		ingresses[ingress.GetId()] = ingress
	}

	transports := map[string]*networkv1.Transport{}
	for i, transport := range plan.GetTransports() {
		if err := checkTransport(transport, transports, egresses, ingresses); err != nil {
			return invalid("transports[%d]: %v", i, err)
		}
		transports[transport.GetId()] = transport
	}

	paths := map[string]*networkv1.Path{}
	labels := map[string]bool{}
	finals := map[string]bool{} // ingress id NUL final egress id
	for i, path := range plan.GetPaths() {
		if err := checkID(path.GetId(), paths); err != nil {
			return invalid("paths[%d]: %v", i, err)
		}
		paths[path.GetId()] = path
		ingress := ingresses[path.GetIngressId()]
		if ingress == nil || ingress.GetAudience() != networkv1.Audience_AUDIENCE_USER {
			return invalid("paths[%d]: ingress_id %q is not a user ingress of the plan", i, path.GetIngressId())
		}
		if err := checkNext(path.GetEgressId(), path.GetTransportId(), egresses, transports); err != nil {
			return invalid("paths[%d]: %v", i, err)
		}
		final := path.GetFinalEgressId()
		if final != "" && !idPattern.MatchString(final) {
			return invalid("paths[%d]: final_egress_id %q is not an id", i, final)
		}
		if path.GetEgressId() != "" && final != "" && final != path.GetEgressId() {
			return invalid("paths[%d]: final_egress_id %q is not the path's egress %q", i, final, path.GetEgressId())
		}
		if t := transports[path.GetTransportId()]; t != nil && t.GetRole() == networkv1.TransportRole_TRANSPORT_ROLE_LOCAL {
			if egress := egresses[final]; egress == nil || egress.GetAudience() != networkv1.Audience_AUDIENCE_INTERNET {
				return invalid("paths[%d]: a path over a local transport ends at an internet egress of the plan, not %q", i, final)
			}
		}
		if final == "" {
			final = path.GetEgressId()
		}
		if final != "" {
			finals[path.GetIngressId()+"\x00"+final] = true
		}
		for _, label := range path.GetLabels() {
			if label == "" {
				return invalid("paths[%d]: empty label", i)
			}
			if err := checkText(label); err != nil {
				return invalid("paths[%d]: %v", i, err)
			}
			labels[label] = true
		}
	}

	from := map[string]bool{}
	for i, forwarding := range plan.GetForwarding() {
		transport := transports[forwarding.GetFromTransportId()]
		if transport == nil || transport.GetRole() != networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT {
			return invalid("forwarding[%d]: from_transport_id %q is not an accepting transport of the plan", i, forwarding.GetFromTransportId())
		}
		if from[transport.GetId()] {
			return invalid("forwarding[%d]: transport %q is forwarded twice", i, transport.GetId())
		}
		from[transport.GetId()] = true
		if err := checkNext(forwarding.GetEgressId(), forwarding.GetTransportId(), egresses, transports); err != nil {
			return invalid("forwarding[%d]: %v", i, err)
		}
	}

	forwards := map[string]*networkv1.PortForward{}
	for i, forward := range plan.GetPortForwards() {
		if err := checkPortForward(forward, forwards, egresses, transports, listens); err != nil {
			return invalid("port_forwards[%d]: %v", i, err)
		}
		forwards[forward.GetId()] = forward
	}

	proxies := map[string]*networkv1.ReverseProxy{}
	for i, proxy := range plan.GetReverseProxies() {
		if err := checkReverseProxy(proxy, proxies, egresses, transports, listens); err != nil {
			return invalid("reverse_proxies[%d]: %v", i, err)
		}
		proxies[proxy.GetId()] = proxy
	}

	// Pairing references, now that paths are known.
	for i, ingress := range plan.GetIngresses() {
		pairing := ingress.GetPairing()
		if pairing == nil {
			continue
		}
		if id := pairing.GetFallbackPathId(); id != "" {
			if path := paths[id]; path == nil || path.GetIngressId() != ingress.GetId() {
				return invalid("ingresses[%d]: fallback_path_id %q is not a path of this ingress", i, id)
			}
		}
		for _, id := range pairing.GetEgressIds() {
			if !finals[ingress.GetId()+"\x00"+id] {
				return invalid("ingresses[%d]: pairing egress %q is not the final egress of a path of this ingress", i, id)
			}
		}
		for j, rule := range pairing.GetRules() {
			if !labels[rule.GetPathLabel()] && pairing.GetFallbackPathId() == "" {
				return invalid("ingresses[%d]: rules[%d]: no path is labelled %q and there is no fallback", i, j, rule.GetPathLabel())
			}
		}
	}
	return nil
}

func checkEgress(egress *networkv1.Egress, egresses map[string]*networkv1.Egress, lines map[string]*networkv1.Line) error {
	if err := checkID(egress.GetId(), egresses); err != nil {
		return err
	}
	if lines[egress.GetLineId()] == nil {
		return fmt.Errorf("line_id %q is not a line of the plan", egress.GetLineId())
	}
	if !kindPattern.MatchString(egress.GetKind()) {
		return fmt.Errorf("kind %q is not a kind name", egress.GetKind())
	}
	switch egress.GetAudience() {
	case networkv1.Audience_AUDIENCE_INTERNET, networkv1.Audience_AUDIENCE_HOP:
	default:
		return fmt.Errorf("audience must be internet or hop")
	}
	if (egress.GetKind() == EgressHop) != (egress.GetAudience() == networkv1.Audience_AUDIENCE_HOP) {
		return fmt.Errorf("kind hop and audience hop go together")
	}
	if !knownFamily(egress.GetFamily()) || egress.GetFamily() == networkv1.Family_FAMILY_UNSPECIFIED {
		return fmt.Errorf("family is required")
	}
	if source := egress.GetSource(); source != SourceAuto {
		if err := checkAddress(source, egress.GetFamily()); err != nil {
			return fmt.Errorf("source: %w", err)
		}
	}
	if err := checkGateway(egress.GetGateway(), egress.GetFamily()); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	if routing := egress.GetRouting(); routing != nil {
		for name, value := range map[string]uint32{"table": routing.GetTable(), "rule_priority": routing.GetRulePriority()} {
			if value > 1<<31-1 {
				return fmt.Errorf("routing %s %d is out of range", name, value)
			}
		}
	}
	if health := egress.GetHealth(); health != nil {
		if len(health.GetTargets()) > 16 {
			return fmt.Errorf("more than 16 health targets")
		}
		for _, target := range health.GetTargets() {
			if err := checkHostPort(target); err != nil {
				return fmt.Errorf("health target %q: %w", target, err)
			}
		}
		if len(health.GetTargets()) > 0 && (health.GetIntervalMs() == 0 || health.GetTimeoutMs() == 0 || health.GetTimeoutMs() > health.GetIntervalMs()) {
			return fmt.Errorf("health needs 0 < timeout_ms <= interval_ms")
		}
	}
	if egress.GetMtu() > MaxPort || egress.GetAdvmss() > MaxPort {
		return fmt.Errorf("mtu or advmss out of range")
	}
	return nil
}

func checkIngress(ingress *networkv1.Ingress, ingresses map[string]*networkv1.Ingress, lines map[string]*networkv1.Line,
	egresses map[string]*networkv1.Egress, listens map[string]string,
) error {
	if err := checkID(ingress.GetId(), ingresses); err != nil {
		return err
	}
	if lines[ingress.GetLineId()] == nil {
		return fmt.Errorf("line_id %q is not a line of the plan", ingress.GetLineId())
	}
	if !kindPattern.MatchString(ingress.GetKind()) {
		return fmt.Errorf("kind %q is not a kind name", ingress.GetKind())
	}
	if !knownFamily(ingress.GetFamily()) || ingress.GetFamily() == networkv1.Family_FAMILY_UNSPECIFIED {
		return fmt.Errorf("family is required")
	}
	listen := ingress.GetListen()
	if err := checkListen(listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	key := net.JoinHostPort(canonical(listen.GetAddress()), strconv.Itoa(int(listen.GetPort())))
	if other, taken := listens[key]; taken {
		return fmt.Errorf("listen %s is taken by ingress %q", key, other)
	}
	listens[key] = ingress.GetId()
	if err := checkText(ingress.GetPublicHost()); err != nil {
		return err
	}
	if err := checkPortMaps(ingress.GetPortMap()); err != nil {
		return err
	}
	for _, protocol := range ingress.GetProtocols() {
		if !kindPattern.MatchString(protocol) {
			return fmt.Errorf("protocol %q is not a protocol name", protocol)
		}
	}
	for _, peer := range ingress.GetAllowedPeers() {
		if _, err := netip.ParsePrefix(peer); err != nil {
			if _, err := netip.ParseAddr(peer); err != nil {
				return fmt.Errorf("allowed peer %q is not an address or prefix", peer)
			}
		}
	}
	switch ingress.GetAudience() {
	case networkv1.Audience_AUDIENCE_USER:
		if len(ingress.GetAllowedPeers()) > 0 {
			return fmt.Errorf("a user ingress admits everyone; allowed_peers is for hop ingresses")
		}
		return checkPairing(ingress.GetPairing())
	case networkv1.Audience_AUDIENCE_HOP:
		if ingress.GetPairing() != nil || len(ingress.GetProtocols()) > 0 {
			return fmt.Errorf("a hop ingress has no pairing and no user protocols")
		}
		return nil
	default:
		return fmt.Errorf("audience must be user or hop")
	}
}

func checkPairing(pairing *networkv1.Pairing) error {
	if pairing == nil {
		return fmt.Errorf("a user ingress needs a pairing")
	}
	n := len(pairing.GetEgressIds())
	switch pairing.GetMode() {
	case networkv1.PairingMode_PAIRING_MODE_SAME_LINE:
		if n != 0 || len(pairing.GetRules()) != 0 {
			return fmt.Errorf("same_line takes no egresses and no rules")
		}
	case networkv1.PairingMode_PAIRING_MODE_CROSS:
		if n != 1 || len(pairing.GetRules()) != 0 {
			return fmt.Errorf("cross takes exactly one egress and no rules")
		}
	case networkv1.PairingMode_PAIRING_MODE_FAILOVER:
		if n == 0 || len(pairing.GetRules()) != 0 {
			return fmt.Errorf("failover takes at least one egress and no rules")
		}
		seen := map[string]bool{}
		for _, id := range pairing.GetEgressIds() {
			if seen[id] {
				return fmt.Errorf("failover lists egress %q twice", id)
			}
			seen[id] = true
		}
	case networkv1.PairingMode_PAIRING_MODE_RULES:
		if n != 0 || len(pairing.GetRules()) == 0 {
			return fmt.Errorf("rules takes rules and no egresses")
		}
		for _, rule := range pairing.GetRules() {
			if rule.GetMatch() == "" || rule.GetPathLabel() == "" {
				return fmt.Errorf("a rule needs a match and a path label")
			}
			if err := checkText(rule.GetMatch()); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("pairing mode is required")
	}
	return nil
}

func checkTransport(transport *networkv1.Transport, transports map[string]*networkv1.Transport,
	egresses map[string]*networkv1.Egress, ingresses map[string]*networkv1.Ingress,
) error {
	if err := checkID(transport.GetId(), transports); err != nil {
		return err
	}
	if !kindPattern.MatchString(transport.GetKind()) {
		return fmt.Errorf("kind %q is not a kind name", transport.GetKind())
	}
	if err := checkText(transport.GetPeerDeviceRef()); err != nil {
		return err
	}
	from, to := transport.GetFromEgressId(), transport.GetToIngressId()
	switch transport.GetRole() {
	case networkv1.TransportRole_TRANSPORT_ROLE_LOCAL:
		if transport.GetKind() != TransportLocal || from != "" || to != "" {
			return fmt.Errorf("role local is kind local, with neither from_egress_id nor to_ingress_id")
		}
	case networkv1.TransportRole_TRANSPORT_ROLE_DIAL:
		if egress := egresses[from]; egress == nil || egress.GetAudience() != networkv1.Audience_AUDIENCE_HOP {
			return fmt.Errorf("a dialing end leaves by a hop egress of the plan, not %q", from)
		}
		if to != "" {
			return fmt.Errorf("a dialing end has no local to_ingress_id")
		}
	case networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT:
		if ingress := ingresses[to]; ingress == nil || ingress.GetAudience() != networkv1.Audience_AUDIENCE_HOP {
			return fmt.Errorf("an accepting end arrives at a hop ingress of the plan, not %q", to)
		}
		if from != "" {
			return fmt.Errorf("an accepting end has no local from_egress_id")
		}
	default:
		return fmt.Errorf("role is required")
	}
	if transport.GetKind() == TransportLocal && transport.GetRole() != networkv1.TransportRole_TRANSPORT_ROLE_LOCAL {
		return fmt.Errorf("kind local has role local")
	}
	if transport.GetRole() != networkv1.TransportRole_TRANSPORT_ROLE_LOCAL && transport.GetPeerDeviceRef() == "" {
		return fmt.Errorf("peer_device_ref is required")
	}
	if transport.GetMtu() > MaxPort {
		return fmt.Errorf("mtu out of range")
	}
	anx := transport.GetAnxLine()
	if (transport.GetKind() == TransportAnxLine) != (anx != nil) {
		return fmt.Errorf("anx_line settings go with kind anx_line only")
	}
	if anx != nil {
		return checkAnxLine(transport, anx)
	}
	return nil
}

func checkAnxLine(transport *networkv1.Transport, anx *networkv1.AnxLineSettings) error {
	if anx.GetLinkId() != transport.GetId() {
		return fmt.Errorf("anx_line link_id %q is not the transport id", anx.GetLinkId())
	}
	if key, err := hex.DecodeString(anx.GetLinkKey()); err != nil || len(key) != 32 || hex.EncodeToString(key) != anx.GetLinkKey() {
		return fmt.Errorf("anx_line link_key is not 64 lower-case hex digits")
	}
	for _, carrier := range anx.GetCarrierOrder() {
		if !anxCarriers[carrier] {
			return fmt.Errorf("anx_line carrier %q is not c1, c2, c3, tcp, quic or faketcp", carrier)
		}
	}
	switch anx.GetEgress() {
	case "", "l1":
	case "l2":
		if anx.GetServerName() == "" || anx.GetQuicAddr() != "" || anx.GetFaketcpAddr() != "" {
			return fmt.Errorf("anx_line l2 takes tcp_addr only and needs server_name")
		}
	default:
		return fmt.Errorf("anx_line egress must be l1 or l2")
	}
	if err := checkKey(anx.GetServerPublic()); err != nil {
		return err
	}
	if err := checkText(anx.GetServerName()); err != nil {
		return err
	}
	switch transport.GetRole() {
	case networkv1.TransportRole_TRANSPORT_ROLE_DIAL:
		if anx.GetServerPublic() == "" {
			return fmt.Errorf("anx_line server_public is required on the dialing end")
		}
	case networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT:
		// The accepting end gets the addresses it listens on. A reversed
		// l2 link's accepting end dials back under the A2 jacket and gets
		// the dialing end's key, the jacket server it authenticates to.
		if anx.GetServerPublic() != "" && (!anx.GetReverse() || anx.GetEgress() != "l2") {
			return fmt.Errorf("anx_line server_public is for the dialing end")
		}
	default:
		if anx.GetServerPublic() != "" || anx.GetQuicAddr() != "" || anx.GetFaketcpAddr() != "" || anx.GetTcpAddr() != "" {
			return fmt.Errorf("anx_line peer key and addresses are for the transport's ends")
		}
		return nil
	}
	if anx.GetReverse() {
		return checkAnxLineReverse(transport, anx)
	}
	if anx.GetReverseListen() != "" || anx.GetReverseDial() != "" {
		return fmt.Errorf("anx_line reverse_listen and reverse_dial need reverse")
	}
	if anx.GetQuicAddr() == "" && anx.GetFaketcpAddr() == "" && anx.GetTcpAddr() == "" {
		return fmt.Errorf("anx_line needs at least one carrier address")
	}
	for _, addr := range []string{anx.GetQuicAddr(), anx.GetFaketcpAddr(), anx.GetTcpAddr()} {
		if addr == "" {
			continue
		}
		if err := checkHostPort(addr); err != nil {
			return fmt.Errorf("anx_line address %q: %w", addr, err)
		}
	}
	if addr := anx.GetFaketcpAddr(); addr != "" {
		host, _, _ := net.SplitHostPort(addr)
		if ip, err := netip.ParseAddr(host); err == nil && !ip.Is4() {
			return fmt.Errorf("anx_line faketcp_addr is IPv4 only")
		}
	}
	return nil
}

// MaxForwardTargets caps a port forward's targets.
const MaxForwardTargets = 64

func checkPortForward(forward *networkv1.PortForward, forwards map[string]*networkv1.PortForward,
	egresses map[string]*networkv1.Egress, transports map[string]*networkv1.Transport, listens map[string]string,
) error {
	if err := checkID(forward.GetId(), forwards); err != nil {
		return err
	}
	if err := checkPortForwardFields(forward); err != nil {
		return err
	}
	listen := forward.GetListen()
	key := net.JoinHostPort(canonical(listen.GetAddress()), strconv.Itoa(int(listen.GetPort())))
	if other, taken := listens[key]; taken {
		return fmt.Errorf("listen %s is taken by %q", key, other)
	}
	listens[key] = forward.GetId()
	return checkNext(forward.GetEgressId(), forward.GetTransportId(), egresses, transports)
}

// CheckPortForward checks what a port forward says on its own: its id,
// listen endpoint, protocols, targets and policy; Check also checks its
// listen address and next hop against the plan.
func CheckPortForward(forward *networkv1.PortForward) error {
	if !idPattern.MatchString(forward.GetId()) {
		return invalid("id %q must be 1 to 64 ASCII letters, digits, '-' and '_'", forward.GetId())
	}
	if err := checkPortForwardFields(forward); err != nil {
		return invalid("%v", err)
	}
	return nil
}

func checkPortForwardFields(forward *networkv1.PortForward) error {
	if err := checkListen(forward.GetListen()); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if len(forward.GetProtocols()) == 0 || len(forward.GetProtocols()) > 2 {
		return fmt.Errorf("protocols must be tcp, udp or both")
	}
	seen := map[string]bool{}
	for _, protocol := range forward.GetProtocols() {
		if (protocol != "tcp" && protocol != "udp") || seen[protocol] {
			return fmt.Errorf("protocols must be tcp, udp or both")
		}
		seen[protocol] = true
	}
	if len(forward.GetTargets()) == 0 || len(forward.GetTargets()) > MaxForwardTargets {
		return fmt.Errorf("1 to %d targets are required", MaxForwardTargets)
	}
	for j, target := range forward.GetTargets() {
		if err := checkHostPort(net.JoinHostPort(target.GetHost(), strconv.Itoa(int(target.GetPort())))); err != nil {
			return fmt.Errorf("targets[%d]: %v", j, err)
		}
	}
	if err := checkForwardPolicy(forward); err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	return nil
}

// Forward policy bounds.
const (
	MaxTargetWeight     = 1000
	MaxHealthIntervalMs = 3_600_000
	MaxBreakerOpenMs    = 3_600_000
	MaxBreakerThreshold = 100
)

var (
	forwardBalances = map[string]bool{"": true, "round_robin": true, "random": true, "ip_hash": true, "least_conn": true, "failover": true}
	proxyProtocols  = map[string]bool{"": true, "off": true, "v1": true, "v2": true}
)

func checkForwardPolicy(forward *networkv1.PortForward) error {
	for j, target := range forward.GetTargets() {
		if target.GetWeight() > MaxTargetWeight || target.GetPriority() > MaxTargetWeight {
			return fmt.Errorf("targets[%d]: weight and priority are at most %d", j, MaxTargetWeight)
		}
	}
	policy := forward.GetPolicy()
	if !forwardBalances[policy.GetBalance()] {
		return fmt.Errorf("balance %q must be round_robin, random, ip_hash, least_conn or failover", policy.GetBalance())
	}
	if !proxyProtocols[policy.GetProxyProtocol()] {
		return fmt.Errorf("proxy_protocol %q must be off, v1 or v2", policy.GetProxyProtocol())
	}
	if p := policy.GetProxyProtocol(); p == "v1" || p == "v2" {
		tcp := false
		for _, protocol := range forward.GetProtocols() {
			tcp = tcp || protocol == "tcp"
		}
		if !tcp {
			return fmt.Errorf("the PROXY header is written on TCP; this forward is UDP only")
		}
	}
	health := policy.GetHealth()
	if health.GetIntervalMs() > MaxHealthIntervalMs || health.GetTimeoutMs() > MaxHealthIntervalMs {
		return fmt.Errorf("health interval_ms and timeout_ms are at most %d", MaxHealthIntervalMs)
	}
	if health.GetIntervalMs() > 0 && health.GetTimeoutMs() > health.GetIntervalMs() {
		return fmt.Errorf("health timeout_ms is longer than interval_ms")
	}
	breaker := policy.GetCircuitBreaker()
	if breaker.GetFailureThreshold() > MaxBreakerThreshold || breaker.GetOpenMs() > MaxBreakerOpenMs {
		return fmt.Errorf("circuit_breaker failure_threshold is at most %d and open_ms at most %d", MaxBreakerThreshold, MaxBreakerOpenMs)
	}
	return nil
}

func checkNext(egressID, transportID string, egresses map[string]*networkv1.Egress, transports map[string]*networkv1.Transport) error {
	switch {
	case egressID != "" && transportID != "":
		return fmt.Errorf("next is an egress or a transport, not both")
	case egressID != "":
		if egress := egresses[egressID]; egress == nil || egress.GetAudience() != networkv1.Audience_AUDIENCE_INTERNET {
			return fmt.Errorf("egress_id %q is not an internet egress of the plan", egressID)
		}
	case transportID != "":
		transport := transports[transportID]
		if transport == nil || transport.GetRole() == networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT {
			return fmt.Errorf("transport_id %q is not a dialing or local transport of the plan", transportID)
		}
	default:
		return fmt.Errorf("next is required")
	}
	return nil
}

func checkID[T any](id string, seen map[string]T) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("id %q must be 1 to 64 ASCII letters, digits, '-' and '_'", id)
	}
	if _, dup := seen[id]; dup {
		return fmt.Errorf("id %q is listed twice", id)
	}
	return nil
}

func checkListen(listen *networkv1.Endpoint) error {
	if listen == nil {
		return fmt.Errorf("required")
	}
	if listen.GetPort() == 0 || listen.GetPort() > MaxPort {
		return fmt.Errorf("port must be 1 to %d", MaxPort)
	}
	if _, err := netip.ParseAddr(listen.GetAddress()); err != nil {
		return fmt.Errorf("address %q is not an IP address", listen.GetAddress())
	}
	return nil
}

func checkPortMaps(maps []*networkv1.PortMap) error {
	type span struct{ first, last uint32 }
	var publics, locals []span
	for _, m := range maps {
		p, l := m.GetPublic(), m.GetLocal()
		for _, r := range []*networkv1.PortRange{p, l} {
			if r.GetFirst() == 0 || r.GetFirst() > r.GetLast() || r.GetLast() > MaxPort {
				return fmt.Errorf("port_map range %d–%d is not 1 <= first <= last <= %d", r.GetFirst(), r.GetLast(), MaxPort)
			}
		}
		if p.GetLast()-p.GetFirst() != l.GetLast()-l.GetFirst() {
			return fmt.Errorf("port_map %d–%d and %d–%d differ in length", p.GetFirst(), p.GetLast(), l.GetFirst(), l.GetLast())
		}
		for _, other := range publics {
			if p.GetFirst() <= other.last && other.first <= p.GetLast() {
				return fmt.Errorf("port_map public ranges overlap at %d–%d", p.GetFirst(), p.GetLast())
			}
		}
		for _, other := range locals {
			if l.GetFirst() <= other.last && other.first <= l.GetLast() {
				return fmt.Errorf("port_map local ranges overlap at %d–%d", l.GetFirst(), l.GetLast())
			}
		}
		publics = append(publics, span{p.GetFirst(), p.GetLast()})
		locals = append(locals, span{l.GetFirst(), l.GetLast()})
	}
	return nil
}

func checkAddress(address string, family networkv1.Family) error {
	ip, err := netip.ParseAddr(address)
	if err != nil || ip.Zone() != "" {
		return fmt.Errorf("%q is not an IP address", address)
	}
	if ip.Is4() != (family == networkv1.Family_FAMILY_V4) {
		return fmt.Errorf("%q is not of family %s", address, family)
	}
	return nil
}

func checkGateway(gateway string, family networkv1.Family) error {
	if gateway == "" {
		return nil
	}
	return checkAddress(gateway, family)
}

func checkHostPort(target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" {
		return fmt.Errorf("not host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > MaxPort {
		return fmt.Errorf("port must be 1 to %d", MaxPort)
	}
	return checkText(host)
}

func canonical(address string) string {
	if ip, err := netip.ParseAddr(address); err == nil {
		return ip.Unmap().String()
	}
	return address
}

// checkKey is checkText for a public key, which may run to MaxKeyBytes.
func checkKey(key string) error {
	if len(key) > MaxKeyBytes {
		return fmt.Errorf("a key exceeds %d bytes", MaxKeyBytes)
	}
	if !utf8.ValidString(key) {
		return fmt.Errorf("a key is not UTF-8")
	}
	return nil
}

func checkText(text string) error {
	if len(text) > MaxTextBytes {
		return fmt.Errorf("a text field exceeds %d bytes", MaxTextBytes)
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("a text field is not UTF-8")
	}
	return nil
}

func knownComponent(c networkv1.Component) bool {
	return c != networkv1.Component_COMPONENT_UNSPECIFIED && networkv1.Component_name[int32(c)] != ""
}

func knownFamily(f networkv1.Family) bool {
	return networkv1.Family_name[int32(f)] != ""
}

// checkAnxLineReverse checks a reversed link: the accepting end dials the
// dialing end's reverse listener, so neither end has carrier addresses;
// the dialing end has reverse_listen, the accepting end reverse_dial. l2
// runs reversed on c1 only (anx ANX-LINE-REVERSE §6.4): the accepting end
// dials back as the A2 jacket's client, so it also needs the dialing end's
// server_public, and both ends the jacket's server_name.
func checkAnxLineReverse(transport *networkv1.Transport, anx *networkv1.AnxLineSettings) error {
	if anx.GetEgress() == "l2" {
		for _, carrier := range anx.GetCarrierOrder() {
			if carrier != "c1" && carrier != "tcp" {
				return fmt.Errorf("anx_line l2 reverse runs on c1 only")
			}
		}
		if transport.GetRole() == networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT && anx.GetServerPublic() == "" {
			return fmt.Errorf("anx_line l2 reverse: the accepting end needs the dialing end's server_public")
		}
	}
	if anx.GetQuicAddr() != "" || anx.GetFaketcpAddr() != "" || anx.GetTcpAddr() != "" {
		return fmt.Errorf("anx_line reverse takes no carrier address")
	}
	addr, other := anx.GetReverseListen(), anx.GetReverseDial()
	if transport.GetRole() == networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT {
		addr, other = other, addr
	}
	if addr == "" || other != "" {
		return fmt.Errorf("anx_line reverse: the dialing end needs reverse_listen, the accepting end reverse_dial")
	}
	if err := checkHostPort(addr); err != nil {
		return fmt.Errorf("anx_line reverse address %q: %w", addr, err)
	}
	return nil
}
