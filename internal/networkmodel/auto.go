package networkmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"gorm.io/gorm"
)

// Control completes the model for the forward routes (owner, 2026-10-30):
// a route names its devices, and may leave its transports and its exit
// egress to Control. What a route leaves out, Control adds to the plans
// under ids that start with AutoPrefix, which the document cannot use:
//
//   - a line auto-<node> on a device without one;
//   - an internet egress auto-<node>-internet (nat_direct) on a device
//     whose route exit, or extra entry, has none;
//   - for a hop without a transport, a hop egress auto-<node>-hop on the
//     dialing device, a hop ingress auto-<node>-hop on the next device
//     (listening on AutoHopPort, reached at the node's address) and the
//     transport auto-<from>-<to> between them: kind anx (ANX-A, the A2
//     jacket), since Control cannot tell a trusted segment.
//
// A device's own network settings (model.NodeNetwork, owner 2026-10-30:
// NAT and multi-line machines) shape them: the auto line is on the
// settings' interface and the auto egresses leave from its source address
// (policy routed through its gateway) instead of by the default route; the
// hop ingress listens on the settings' hop port, and when a NAT node entry
// maps that port it is a NAT ingress reached at the entry's address and
// public port.
//
// The auto transports get link keys like declared ones (syncAutoKeys).
// Declared components are never changed: a device with lines keeps them,
// and a route that names a transport or an egress runs on it.
const (
	AutoPrefix = "auto-"
	// AutoHopPort is where an auto hop ingress listens.
	AutoHopPort = 24443
	// AutoServerName is the jacket's server name of an auto transport.
	AutoServerName = "www.apple.com"
)

// Link is how Control links two devices for a hop that names no
// transport (forward.v1 LinkTransport.network_mode): the A2 jacket by
// default, the bare ANX-Line on trusted segments, or the bare ANX-Line
// dialed back by the next device.
type Link struct {
	Mode LinkMode
	// Carriers orders a bare ANX-Line's carriers ("c1" TCP, "c2" QUIC);
	// empty is TCP only.
	Carriers  []string
	Multipath bool
	FEC       bool
}

// LinkMode is the kind of transport of a Link.
type LinkMode string

const (
	// LinkAnxA is ANX-Line under the A2 jacket, on TCP (kind anx).
	LinkAnxA LinkMode = ""
	// LinkAnxLine is the bare ANX-Line (kind anx_line, trusted), its
	// accepting end listening on TCP and QUIC at AutoLinePort.
	LinkAnxLine LinkMode = "line"
	// LinkAnxLineReverse is the bare ANX-Line on TCP that the accepting
	// device dials back to the dialing device's DefaultReversePort.
	LinkAnxLineReverse LinkMode = "reverse"
)

// AutoLinePort is where an auto bare ANX-Line hop ingress listens (TCP and
// QUIC), apart from the A2 jacket's AutoHopPort: one anx-server serves one
// set of listen addresses and server name.
const AutoLinePort = AutoHopPort + 2

// suffix answers the part of an auto transport's id that tells l from
// the A2 jacket and from other shapes of the same mode: "" for the
// jacket.
func (l Link) suffix() string {
	switch l.Mode {
	case LinkAnxLine:
		if len(l.Carriers) == 0 && !l.Multipath && !l.FEC {
			return "-line"
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v|%t|%t", l.Carriers, l.Multipath, l.FEC)))
		return "-line-" + hex.EncodeToString(sum[:3])
	case LinkAnxLineReverse:
		return "-rev"
	}
	return ""
}

// natEntry is a NAT node entry: the public address and port ranges a
// device is reached at.
type natEntry struct {
	Host   string
	Ranges []nodeentry.PortRange
}

// device is what the completer knows of a device: the node's host, its
// network settings and its enabled NAT entries.
type device struct {
	Host    string
	Network model.NodeNetwork
	NAT     []natEntry
}

// hopPort answers the port the device's hop ingress listens on.
func (d device) hopPort() uint32 {
	if d.Network.HopPort != 0 {
		return d.Network.HopPort
	}
	return AutoHopPort
}

// natFor answers the NAT entry of family that maps port, and the public
// port it maps it to.
func (d device) natFor(family string, port uint32) (natEntry, uint32, bool) {
	for _, entry := range d.NAT {
		if hostFamily(entry.Host) != family {
			continue
		}
		for _, r := range entry.Ranges {
			if int(port) >= r.From && int(port) <= r.To {
				return entry, uint32(r.External + int(port) - r.From), true // #nosec G115 -- nodeentry validates ports to 1-65535.
			}
		}
	}
	return natEntry{}, 0, false
}

// source answers the source of the device's auto egresses of family: the
// settings' address on their interface, or auto (the default route).
func (d device) source(family string) string {
	address := d.Network.SourceV4
	if family == "v6" {
		address = d.Network.SourceV6
	}
	if d.Network.Interface == "" || address == "" {
		return network.SourceAuto
	}
	return address
}

// hostFamily answers v6 for an IPv6 literal, v4 for anything else.
func hostFamily(host string) string {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil && ip.Unmap().Is6() {
		return "v6"
	}
	return "v4"
}

// autoCompleter adds the auto components to a copy of a document.
type autoCompleter struct {
	doc     *Document
	devices map[string]device
	// lineOf is the first line of each device.
	lineOf map[string]string
	nodeOf map[string]string
	ids    map[string]bool
}

// complete answers doc with the components forwards need and do not name,
// and forwards with their transports and exit egress filled in. A hop
// Control cannot link (its device has no address) keeps an empty
// transport and waits.
func complete(doc *Document, forwards []Forward, devices map[string]device) (*Document, []Forward) {
	out := &Document{
		Lines:      append([]Line(nil), doc.Lines...),
		Ingresses:  append([]Ingress(nil), doc.Ingresses...),
		Egresses:   append([]Egress(nil), doc.Egresses...),
		Transports: append([]Transport(nil), doc.Transports...),
		Paths:      doc.Paths,
	}
	a := &autoCompleter{doc: out, devices: devices, lineOf: map[string]string{}, nodeOf: map[string]string{}, ids: map[string]bool{}}
	for _, line := range out.Lines {
		if _, ok := a.lineOf[line.Node]; !ok {
			a.lineOf[line.Node] = line.ID
		}
		a.nodeOf[line.ID] = line.Node
	}
	filled := make([]Forward, len(forwards))
	for i, forward := range forwards {
		forward.Segments = append([]string(nil), forward.Segments...)
		at := ""
		if len(forward.Entries) > 0 {
			at = forward.Entries[0]
			a.line(at)
		}
		auto0 := false
		for j, id := range forward.Segments {
			if id != "" {
				at = a.reaches(id, at)
				continue
			}
			if j >= len(forward.Via) || at == "" {
				break
			}
			forward.Segments[j] = a.transport(at, forward.Via[j], forward.link(j))
			auto0 = auto0 || j == 0
			at = forward.Via[j]
		}
		for _, entry := range forward.Entries[min(1, len(forward.Entries)):] {
			a.line(entry)
			switch {
			case len(forward.Segments) == 0:
				a.internetEgress(entry)
			case auto0 && len(forward.Via) > 0:
				a.transport(entry, forward.Via[0], forward.link(0))
			}
		}
		if forward.Egress == "" && at != "" {
			forward.Egress = a.internetEgress(at)
		}
		filled[i] = forward
	}
	return out, filled
}

// reaches answers the device transport id leads to, or at when it is not
// a declared transport.
func (a *autoCompleter) reaches(id, at string) string {
	for _, transport := range a.doc.Transports {
		if transport.ID != id {
			continue
		}
		for _, ingress := range a.doc.Ingresses {
			if ingress.ID == transport.ToIngress {
				if node := a.nodeOf[ingress.Line]; node != "" {
					return node
				}
			}
		}
	}
	return at
}

func (a *autoCompleter) add(id string) bool {
	if a.ids[id] {
		return false
	}
	a.ids[id] = true
	return true
}

// line answers node's first line, adding auto-<node> when it has none.
func (a *autoCompleter) line(node string) string {
	if id, ok := a.lineOf[node]; ok {
		return id
	}
	id := AutoPrefix + node
	settings := a.devices[node].Network
	line := Line{ID: id, Node: node, Name: "auto", Interface: settings.Interface,
		GatewayV4: settings.GatewayV4, GatewayV6: settings.GatewayV6}
	if settings.SourceV4 != "" {
		line.Families = append(line.Families, "v4")
	}
	if settings.SourceV6 != "" {
		line.Families = append(line.Families, "v6")
	}
	a.doc.Lines = append(a.doc.Lines, line)
	a.lineOf[node], a.nodeOf[id] = id, node
	return id
}

// family answers the address family of node's host: v6 for an IPv6
// literal, else v4.
func (a *autoCompleter) family(node string) string {
	return hostFamily(a.devices[node].Host)
}

// internetEgress answers node's enabled internet egress (the first by id),
// adding auto-<node>-internet when it has none.
func (a *autoCompleter) internetEgress(node string) string {
	var ids []string
	for _, egress := range a.doc.Egresses {
		if egress.Audience == "internet" && !egress.Disabled && a.nodeOf[egress.Line] == node {
			ids = append(ids, egress.ID)
		}
	}
	if len(ids) > 0 {
		sort.Strings(ids)
		return ids[0]
	}
	id := AutoPrefix + node + "-internet"
	if a.add(id) {
		a.doc.Egresses = append(a.doc.Egresses, Egress{ID: id, Line: a.line(node), Kind: network.EgressNATDirect,
			Audience: "internet", Family: a.family(node), Source: a.devices[node].source(a.family(node))})
	}
	return id
}

// link answers how Control links segment j: the forward's Link, or the
// A2 jacket.
func (f *Forward) link(j int) Link {
	if j < len(f.Links) {
		return f.Links[j]
	}
	return Link{}
}

// transport answers the auto transport from one device to another as
// link says, adding it with its hop egress and hop ingress; "" when the
// next device (or, reversed, the dialing one) has no address to be
// reached at.
func (a *autoCompleter) transport(from, to string, link Link) string {
	next := a.devices[to]
	host := strings.TrimSpace(next.Host)
	if host == "" || from == to {
		return ""
	}
	if link.Mode == LinkAnxLineReverse && strings.TrimSpace(a.devices[from].Host) == "" {
		return ""
	}
	family := a.family(to)
	port, ingressID := next.hopPort(), AutoPrefix+to+"-hop"
	if link.Mode == LinkAnxLine {
		port, ingressID = next.hopPort()+AutoLinePort-AutoHopPort, ingressID+"-line"
	}
	ingress := Ingress{Line: a.line(to), Kind: network.IngressDirect, Audience: "hop", Family: family,
		Listen: Endpoint{Address: "0.0.0.0", Port: port}, PublicHost: strings.Trim(host, "[]")}
	if family == "v6" {
		ingress.Listen.Address = "::"
	}
	if entry, public, ok := next.natFor(family, port); ok {
		ingress.Kind, ingress.PublicHost = network.IngressNAT, strings.Trim(entry.Host, "[]")
		ingress.PortMap = []PortMap{{Public: PortRange{First: public, Last: public}, Local: PortRange{First: port, Last: port}}}
	}
	egress := AutoPrefix + from + "-hop"
	if family == "v6" {
		egress += "6"
	}
	if a.add("e:" + egress) {
		a.doc.Egresses = append(a.doc.Egresses, Egress{ID: egress, Line: a.line(from), Kind: network.EgressHop,
			Audience: "hop", Family: family, Source: a.devices[from].source(family)})
	}
	if a.add("i:" + ingressID) {
		ingress.ID = ingressID
		a.doc.Ingresses = append(a.doc.Ingresses, ingress)
	}
	id := AutoPrefix + from + "-" + to + link.suffix()
	if a.add("t:" + id) {
		transport := Transport{ID: id, Kind: KindAnx, FromEgress: egress, ToIngress: ingressID,
			AnxLine: &AnxLine{ServerName: AutoServerName}}
		switch link.Mode {
		case LinkAnxLine:
			// The accepting end offers TCP and QUIC on the same port; the
			// dialing end's carrier order picks.
			transport.Kind, transport.Trusted = network.TransportAnxLine, true
			transport.AnxLine = &AnxLine{CarrierOrder: link.Carriers, Multipath: link.Multipath, FEC: link.FEC, QUICPort: port}
		case LinkAnxLineReverse:
			transport.Kind, transport.Trusted = network.TransportAnxLine, true
			transport.AnxLine = &AnxLine{Reverse: true, ReverseHost: strings.Trim(strings.TrimSpace(a.devices[from].Host), "[]")}
		}
		a.doc.Transports = append(a.doc.Transports, transport)
	}
	return id
}

// checkNoAuto refuses document ids that start with AutoPrefix.
func checkNoAuto(doc *Document) error {
	var v violations
	check := func(field, id string) {
		if strings.HasPrefix(id, AutoPrefix) {
			v.add(field+".id", "ids starting with %q are Control's own", AutoPrefix)
		}
	}
	for i, x := range doc.Lines {
		check(fmt.Sprintf("lines[%d]", i), x.ID)
	}
	for i, x := range doc.Ingresses {
		check(fmt.Sprintf("ingresses[%d]", i), x.ID)
	}
	for i, x := range doc.Egresses {
		check(fmt.Sprintf("egresses[%d]", i), x.ID)
	}
	for i, x := range doc.Transports {
		check(fmt.Sprintf("transports[%d]", i), x.ID)
	}
	for i, x := range doc.Paths {
		check(fmt.Sprintf("paths[%d]", i), x.ID)
	}
	return v.err()
}

// AgentNode is a device a route may use: a node whose Agent was seen on
// the Agent Control stream and is not disabled.
type AgentNode struct {
	Node string `json:"node"`
	Name string `json:"name"`
}

// agentNodes answers the devices a route may use, by node reference.
func agentNodes(db *gorm.DB) ([]AgentNode, error) {
	var seen []model.AgentTransport
	if err := db.Where("transport IN ?", []string{model.AgentTransportMTLSStream, model.AgentTransportAPIKeyStream}).
		Find(&seen).Error; err != nil {
		return nil, err
	}
	proxies, forwards := []uint{}, []uint{}
	for _, row := range seen {
		switch row.NodeKind {
		case agentcontrol.NodeKindProxy:
			proxies = append(proxies, row.NodeID)
		case agentcontrol.NodeKindForward:
			forwards = append(forwards, row.NodeID)
		}
	}
	var out []AgentNode
	if len(proxies) > 0 {
		var rows []model.Node
		if err := db.Select("id", "name", "status").Where("id IN ?", proxies).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Status != model.NodeStatusDisabled {
				ref := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(row.ID)}.String() // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
				out = append(out, AgentNode{Node: ref, Name: row.Name})
			}
		}
	}
	if len(forwards) > 0 {
		var rows []model.ForwardNode
		if err := db.Select("id", "name", "enabled").Where("id IN ?", forwards).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Enabled {
				ref := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(row.ID)}.String() // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
				out = append(out, AgentNode{Node: ref, Name: row.Name})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out, nil
}

// devicesFor answers what the completer needs of the devices forwards
// name: their hosts, network settings and enabled NAT entries.
func devicesFor(db *gorm.DB, forwards []Forward) (map[string]device, error) {
	devices := map[string]device{}
	var proxies, others []uint
	refs := map[string]bool{}
	for _, forward := range forwards {
		for _, ref := range append(append([]string(nil), forward.Entries...), forward.Via...) {
			node, err := agentcontrol.ParseAgentNode(ref)
			if err != nil || node.ID == 0 || refs[ref] {
				continue
			}
			refs[ref] = true
			if node.Kind == agentcontrol.NodeKindProxy {
				proxies = append(proxies, uint(node.ID))
			} else {
				others = append(others, uint(node.ID))
			}
		}
	}
	if len(refs) == 0 {
		return devices, nil
	}
	set := func(ref string, edit func(d *device)) {
		d := devices[ref]
		edit(&d)
		devices[ref] = d
	}
	if len(proxies) > 0 {
		var rows []model.Node
		err := db.Select("id", "host").Where("id IN ?", proxies).Find(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			set(proxyRef(row.ID), func(d *device) { d.Host = row.Host })
		}
		entries := map[uint][]model.NodeEntry{}
		if db.Migrator().HasTable(&model.NodeEntry{}) {
			if entries, err = nodeentry.EnabledByNode(db, proxies); err != nil {
				return nil, err
			}
		}
		for id, rows := range entries {
			for _, row := range rows {
				if row.Kind == model.NodeEntryNAT {
					set(proxyRef(id), func(d *device) { d.NAT = append(d.NAT, natEntry{Host: row.Host, Ranges: nodeentry.Ranges(row)}) })
				}
			}
		}
	}
	if len(others) > 0 {
		var rows []model.ForwardNode
		if err := db.Select("id", "host").Where("id IN ?", others).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			ref := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(row.ID)}.String() // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
			set(ref, func(d *device) { d.Host = row.Host })
		}
	}
	if db.Migrator().HasTable(&model.NodeNetwork{}) {
		keys := make([]string, 0, len(refs))
		for ref := range refs {
			keys = append(keys, ref)
		}
		var rows []model.NodeNetwork
		if err := db.Where("node_ref IN ?", keys).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			set(row.NodeRef, func(d *device) { d.Network = row })
		}
	}
	return devices, nil
}

func proxyRef(id uint) string {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)}.String() // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
}
