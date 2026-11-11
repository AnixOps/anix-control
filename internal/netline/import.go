package netline

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

var jsonRead = protojson.UnmarshalOptions{DiscardUnknown: true}

func ref(id uint) string {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)}.String() // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
}

func host(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "["), "]"))
}

// importer reads the stored network as a snapshot.
type importer struct {
	s         Snapshot
	devices   map[string]int
	protocols []model.NodeProtocol
	// byHost are the nodes a host names: a node's host or a protocol's.
	byHost     map[string]map[uint]bool
	transports map[string]bool
	exits      map[string]bool
}

// Import reads what Control stores today as lines: a line per enabled
// protocol of a node, at the node's entries, and a line per forward route.
// A route whose target is a protocol of a node serves that protocol; a
// published one is shown under the name its subscription has. Egress routes
// become rules of the lines served at their source node.
func Import(db *gorm.DB) (*Snapshot, error) {
	in := &importer{devices: map[string]int{}, byHost: map[string]map[uint]bool{}, transports: map[string]bool{}, exits: map[string]bool{}}
	in.s = Snapshot{Devices: []Device{}, Uplinks: []Uplink{}, Entries: []Entry{}, Transports: []Transport{}, Exits: []Exit{}, Lines: []Line{}, Skipped: []Skipped{}}
	var nodes []model.Node
	if err := db.Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	var policies []model.NodeExitPolicy
	if err := db.Find(&policies).Error; err != nil {
		return nil, err
	}
	block := map[uint]bool{}
	for _, policy := range policies {
		block[policy.NodeID] = policy.BlockMainland
	}
	rates := map[string]float64{}
	for _, node := range nodes {
		r := ref(node.ID)
		in.devices[r] = len(in.s.Devices)
		in.s.Devices = append(in.s.Devices, Device{Ref: r, Name: node.Name, Host: node.Host})
		in.s.Entries = append(in.s.Entries, Entry{ID: r, Device: r, Kind: EntryDirect, Host: node.Host})
		in.s.Exits = append(in.s.Exits, Exit{ID: r, Device: r, BlockMainland: block[node.ID]})
		in.exits[r] = true
		in.addHost(node.Host, node.ID)
		rates[r] = node.Rate
	}
	entries, err := in.entries(db)
	if err != nil {
		return nil, err
	}
	if err := in.uplinks(db); err != nil {
		return nil, err
	}
	if err := db.Order("sort, id").Find(&in.protocols).Error; err != nil {
		return nil, err
	}
	for _, protocol := range in.protocols {
		if protocol.Host != nil {
			in.addHost(*protocol.Host, protocol.NodeID)
		}
	}
	for _, protocol := range in.protocols {
		r := ref(protocol.NodeID)
		if _, ok := in.devices[r]; !ok || protocol.Enable != 1 {
			continue
		}
		at := entries[r]
		if len(at) == 0 {
			at = []string{r}
		}
		port := uint32(protocol.Port) // #nosec G115 -- a port.
		in.s.Lines = append(in.s.Lines, Line{
			ID: fmt.Sprintf("protocol-%d", protocol.ID), Name: protocol.Name, Enabled: true, Show: protocol.Show == 1,
			Entries: at, Port: port, L4: l4(string(protocol.Type)), Chain: []Hop{{Devices: []string{r}}}, Exit: r,
			Protocol: &Protocol{ID: protocol.ID, Type: string(protocol.Type), Device: r, Port: port}, Rate: rates[r],
		})
	}
	if err := in.routes(db, rates); err != nil {
		return nil, err
	}
	if err := in.rules(db); err != nil {
		return nil, err
	}
	return &in.s, nil
}

// l4 answers the transports a protocol type runs on.
func l4(protocol string) []string {
	switch model.ProtocolType(protocol) {
	case model.ProtocolHysteria2, model.ProtocolTUIC, model.ProtocolWireGuard:
		return []string{"udp"}
	case model.ProtocolShadowsocks:
		return []string{"tcp", "udp"}
	}
	return []string{"tcp"}
}

func (in *importer) addHost(value string, node uint) {
	key := host(value)
	if key == "" {
		return
	}
	if in.byHost[key] == nil {
		in.byHost[key] = map[uint]bool{}
	}
	in.byHost[key][node] = true
}

// entries adds the nodes' enabled entries and answers each device's
// addresses (direct and NAT entries), which replace its default entry.
func (in *importer) entries(db *gorm.DB) (map[string][]string, error) {
	var rows []model.NodeEntry
	if err := db.Where("enabled = ?", true).Order("node_id, sort, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	addresses := map[string][]string{}
	for _, row := range rows {
		r := ref(row.NodeID)
		if _, ok := in.devices[r]; !ok {
			continue
		}
		entry := Entry{ID: fmt.Sprintf("%s/%d", r, row.ID), Device: r, Name: row.Label, Kind: row.Kind, Host: row.Host, PortMap: nodeentry.Ranges(row), Pool: row.Pool}
		if row.IWANLineID != nil {
			entry.IWANLine = *row.IWANLineID
		}
		in.s.Entries = append(in.s.Entries, entry)
		if row.Kind != model.NodeEntryIWAN {
			addresses[r] = append(addresses[r], entry.ID)
		}
	}
	return addresses, nil
}

// uplinks adds each device's uplinks, or the one interface of its network
// settings.
func (in *importer) uplinks(db *gorm.DB) error {
	var rows []model.NodeUplinks
	if err := db.Find(&rows).Error; err != nil {
		return err
	}
	has := map[string]bool{}
	for _, row := range rows {
		var config struct {
			Uplinks []networkmodel.Uplink `json:"uplinks"`
		}
		if row.ConfigJSON != "" {
			if err := json.Unmarshal([]byte(row.ConfigJSON), &config); err != nil {
				return fmt.Errorf("netline: uplinks of %s: %w", row.NodeRef, err)
			}
		}
		for _, u := range config.Uplinks {
			has[row.NodeRef] = true
			in.s.Uplinks = append(in.s.Uplinks, Uplink{Device: row.NodeRef, Name: u.Name, Interface: u.Interface, SourceV4: u.SourceV4, GatewayV4: u.GatewayV4, SourceV6: u.SourceV6, GatewayV6: u.GatewayV6})
		}
	}
	var settings []model.NodeNetwork
	if err := db.Find(&settings).Error; err != nil {
		return err
	}
	for _, row := range settings {
		if i, ok := in.devices[row.NodeRef]; ok {
			in.s.Devices[i].Region = row.Region
		}
		if row.Interface != "" && !has[row.NodeRef] {
			in.s.Uplinks = append(in.s.Uplinks, Uplink{Device: row.NodeRef, Name: "default", Interface: row.Interface, SourceV4: row.SourceV4, GatewayV4: row.GatewayV4, SourceV6: row.SourceV6, GatewayV6: row.GatewayV6})
		}
	}
	return nil
}

func (in *importer) skip(id, format string, args ...any) {
	in.s.Skipped = append(in.s.Skipped, Skipped{ID: id, Reason: fmt.Sprintf(format, args...)})
}

// routes adds a line per forward route on the kernel or the network model.
func (in *importer) routes(db *gorm.DB, rates map[string]float64) error {
	var rows []model.KernelForwardRoute
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return err
	}
	var published []model.ForwardRouteSubscription
	if err := db.Find(&published).Error; err != nil {
		return err
	}
	shown := map[string]*model.ForwardRouteSubscription{}
	for i := range published {
		shown[published[i].RouteID] = &published[i]
	}
	engines := map[string]forwardv1.Engine{}
	for _, row := range rows {
		route := &forwardv1.Route{}
		if err := jsonRead.Unmarshal([]byte(row.RouteJSON), route); err != nil {
			return fmt.Errorf("netline: route %s: %w", row.ID, err)
		}
		line, ok := in.route(route, engines)
		if !ok {
			continue
		}
		line.Enabled = !route.GetPaused() && row.Enforced == ""
		line.Rate = 1
		if line.Protocol = in.match(route); line.Protocol != nil {
			line.Rate = rates[line.Protocol.Device]
		}
		if sub := shown[row.ID]; sub != nil && line.Protocol != nil {
			line.Show = true
			name := sub.DisplayName
			if name == "" {
				name = line.Name
			}
			line.Name = in.protocolName(line.Protocol.ID) + " | " + name
		}
		in.s.Lines = append(in.s.Lines, line)
	}
	return nil
}

func (in *importer) protocolName(id uint) string {
	for _, protocol := range in.protocols {
		if protocol.ID == id {
			return protocol.Name
		}
	}
	return ""
}

// route reads a route's hops as a chain; false skips the route. engines
// remembers how each device forwarded so far: a device forwards in the
// kernel or runs the userspace kernel, never both.
func (in *importer) route(route *forwardv1.Route, engines map[string]forwardv1.Engine) (Line, bool) {
	id, hops := route.GetId(), route.GetHops()
	if len(hops) == 0 {
		in.skip(id, "the route has no hops")
		return Line{}, false
	}
	for i, hop := range hops {
		engine := hop.GetEngine()
		if engine != forwardv1.Engine_ENGINE_NETWORK && engine != forwardv1.Engine_ENGINE_NFTABLES {
			in.skip(id, "hop %d runs on %s", i, engine)
			return Line{}, false
		}
		if len(hop.GetNodeRefs()) == 0 {
			in.skip(id, "hop %d has no node", i)
			return Line{}, false
		}
		for _, node := range hop.GetNodeRefs() {
			if _, ok := in.devices[node]; !ok {
				in.skip(id, "hop %d is on %s, which is not a node", i, node)
				return Line{}, false
			}
			if before, seen := engines[node]; seen && before != engine {
				in.skip(id, "%s forwards in the kernel on one route and in userspace on another", node)
				return Line{}, false
			}
		}
	}
	name := route.GetName()
	if name == "" {
		name = id
	}
	last := hops[len(hops)-1]
	line := Line{
		ID: id, Name: name, EntryHostname: route.GetListen().GetEntryHostname(), Listen: route.GetListen().GetAddress(),
		Port: route.GetListen().GetPort(), L4: kernelforward.NetworkProtocols(route.GetListen().GetProtocol()),
		Entries: hops[0].GetNodeRefs(), Exit: last.GetNodeRefs()[0], Policy: kernelforward.NetworkPolicy(route),
		HopBalance: kernelforward.NetworkBalances[route.GetPolicy().GetNextHop()],
	}
	for _, target := range route.GetTargets() {
		line.Targets = append(line.Targets, &networkv1.ForwardTarget{Host: target.GetHost(), Port: target.GetPort(), Weight: target.GetWeight(), Priority: target.GetPriority()})
	}
	if egress := last.GetNetworkEgressId(); egress != "" && last.GetEngine() == forwardv1.Engine_ENGINE_NETWORK {
		line.Exit = egress
		if !in.exits[egress] {
			in.exits[egress] = true
			in.s.Exits = append(in.s.Exits, Exit{ID: egress, Device: last.GetNodeRefs()[0], Declared: true})
		}
	}
	for i, hop := range hops {
		kernel := hop.GetEngine() == forwardv1.Engine_ENGINE_NFTABLES
		for _, node := range hop.GetNodeRefs() {
			engines[node] = hop.GetEngine()
			in.s.Devices[in.devices[node]].Kernel = kernel
		}
		step := Hop{Devices: hop.GetNodeRefs()}
		if i > 0 {
			step.Port, step.Dial = hop.GetPort(), hop.GetDialAddress()
			if hops[i-1].GetEngine() == forwardv1.Engine_ENGINE_NETWORK {
				step.Transport = in.transport(hops[i-1].GetNodeRefs()[0], hop.GetNodeRefs()[0], hop.GetIngress())
			}
		}
		line.Chain = append(line.Chain, step)
	}
	return line, true
}

// transport answers the id of the transport of a link, adding it once.
func (in *importer) transport(from, to string, link *forwardv1.LinkTransport) string {
	t := Transport{ID: link.GetNetworkTransportId(), From: from, To: to, Kind: TransportJacket, Declared: true}
	if t.ID == "" {
		t.Declared = false
		switch link.GetNetworkMode() {
		case forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE:
			t.Kind, t.Multipath, t.FEC = TransportLine, link.GetNetworkMultipath(), link.GetNetworkFec()
			for _, carrier := range link.GetNetworkCarriers() {
				switch carrier {
				case forwardv1.NetworkCarrier_NETWORK_CARRIER_TCP:
					t.Carriers = append(t.Carriers, "c1")
				case forwardv1.NetworkCarrier_NETWORK_CARRIER_QUIC:
					t.Carriers = append(t.Carriers, "c2")
				}
			}
		case forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE_REVERSE:
			t.Kind = TransportReverse
		}
		t.ID = fmt.Sprintf("%s>%s/%s", from, to, t.Kind)
		if len(t.Carriers) > 0 || t.Multipath || t.FEC {
			t.ID += fmt.Sprintf("/%s/%t/%t", strings.Join(t.Carriers, "+"), t.Multipath, t.FEC)
		}
	}
	if !in.transports[t.ID] {
		in.transports[t.ID] = true
		in.s.Transports = append(in.s.Transports, t)
	}
	return t.ID
}

// match answers the enabled protocol the route's targets reach: a target
// at a node's host (or a protocol's own), or a loopback target on the
// route's last node, on the protocol's port.
func (in *importer) match(route *forwardv1.Route) *Protocol {
	hops := route.GetHops()
	exit := map[uint]bool{}
	for _, name := range hops[len(hops)-1].GetNodeRefs() {
		if node, err := agentcontrol.ParseAgentNode(name); err == nil && node.Kind == agentcontrol.NodeKindProxy {
			exit[uint(node.ID)] = true
		}
	}
	for _, target := range route.GetTargets() {
		key := host(target.GetHost())
		nodes := in.byHost[key]
		if ip := net.ParseIP(key); key == "localhost" || (ip != nil && ip.IsLoopback()) {
			nodes = exit
		}
		for _, protocol := range in.protocols {
			if protocol.Enable == 1 && nodes[protocol.NodeID] && protocol.Port == int(target.GetPort()) {
				return &Protocol{ID: protocol.ID, Type: string(protocol.Type), Device: ref(protocol.NodeID), Port: target.GetPort()}
			}
		}
	}
	return nil
}

// rules adds each egress route to the lines served at its source node.
func (in *importer) rules(db *gorm.DB) error {
	var rows []model.EgressRoute
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		rule := Rule{Name: row.Name, Exit: ref(row.ExitNodeID), Enabled: row.Enabled}
		if err := json.Unmarshal([]byte(row.Domains), &rule.Domains); err != nil {
			return fmt.Errorf("netline: egress route %d: %w", row.ID, err)
		}
		source := ref(row.SourceNodeID)
		for i := range in.s.Lines {
			if line := &in.s.Lines[i]; line.Protocol != nil && line.Protocol.Device == source {
				line.Rules = append(line.Rules, rule)
			}
		}
	}
	return nil
}
