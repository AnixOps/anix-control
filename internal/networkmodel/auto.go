package networkmodel

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Control completes the model for the forward routes (owner, 2026-10-30):
// a route names its devices, and may leave its transports and its exit
// egress to Control. What a route leaves out, Control adds to the plans
// under ids that start with AutoPrefix, which the document cannot use:
//
//   - a line auto-<node> on a device without one;
//   - an internet egress auto-<node>-internet (nat_direct, source auto) on
//     a device whose route exit, or extra entry, has none;
//   - for a hop without a transport, a hop egress auto-<node>-hop on the
//     dialing device, a hop ingress auto-<node>-hop on the next device
//     (listening on AutoHopPort, reached at the node's address) and the
//     transport auto-<from>-<to> between them: kind anx (ANX-A, the A2
//     jacket), since Control cannot tell a trusted segment.
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

// NodeAddress is a device's address for the hops that reach it: the
// node's host.
type NodeAddress struct {
	Node string
	Host string
}

// autoCompleter adds the auto components to a copy of a document.
type autoCompleter struct {
	doc   *Document
	hosts map[string]string
	// lineOf is the first line of each device.
	lineOf map[string]string
	nodeOf map[string]string
	ids    map[string]bool
}

// complete answers doc with the components forwards need and do not name,
// and forwards with their transports and exit egress filled in. A hop
// Control cannot link (its device has no address) keeps an empty
// transport and waits.
func complete(doc *Document, forwards []Forward, hosts map[string]string) (*Document, []Forward) {
	out := &Document{
		Lines:      append([]Line(nil), doc.Lines...),
		Ingresses:  append([]Ingress(nil), doc.Ingresses...),
		Egresses:   append([]Egress(nil), doc.Egresses...),
		Transports: append([]Transport(nil), doc.Transports...),
		Paths:      doc.Paths,
	}
	a := &autoCompleter{doc: out, hosts: hosts, lineOf: map[string]string{}, nodeOf: map[string]string{}, ids: map[string]bool{}}
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
			forward.Segments[j] = a.transport(at, forward.Via[j])
			auto0 = auto0 || j == 0
			at = forward.Via[j]
		}
		for _, entry := range forward.Entries[min(1, len(forward.Entries)):] {
			a.line(entry)
			switch {
			case len(forward.Segments) == 0:
				a.internetEgress(entry)
			case auto0 && len(forward.Via) > 0:
				a.transport(entry, forward.Via[0])
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
	a.doc.Lines = append(a.doc.Lines, Line{ID: id, Node: node, Name: "auto"})
	a.lineOf[node], a.nodeOf[id] = id, node
	return id
}

// family answers the address family of node's host: v6 for an IPv6
// literal, else v4.
func (a *autoCompleter) family(node string) string {
	if ip, err := netip.ParseAddr(strings.Trim(a.hosts[node], "[]")); err == nil && ip.Unmap().Is6() {
		return "v6"
	}
	return "v4"
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
			Audience: "internet", Family: a.family(node), Source: network.SourceAuto})
	}
	return id
}

// transport answers the auto transport from one device to another, adding
// it with its hop egress and hop ingress; "" when the next device has no
// address to be reached at.
func (a *autoCompleter) transport(from, to string) string {
	host := strings.TrimSpace(a.hosts[to])
	if host == "" || from == to {
		return ""
	}
	egress, ingress := AutoPrefix+from+"-hop", AutoPrefix+to+"-hop"
	if a.family(to) == "v6" {
		egress += "6"
	}
	if a.add("e:" + egress) {
		a.doc.Egresses = append(a.doc.Egresses, Egress{ID: egress, Line: a.line(from), Kind: network.EgressHop,
			Audience: "hop", Family: a.family(to), Source: network.SourceAuto})
	}
	if a.add("i:" + ingress) {
		address := "0.0.0.0"
		if a.family(to) == "v6" {
			address = "::"
		}
		a.doc.Ingresses = append(a.doc.Ingresses, Ingress{ID: ingress, Line: a.line(to), Kind: network.IngressDirect,
			Audience: "hop", Family: a.family(to), Listen: Endpoint{Address: address, Port: AutoHopPort},
			PublicHost: strings.Trim(host, "[]")})
	}
	id := AutoPrefix + from + "-" + to
	if a.add("t:" + id) {
		a.doc.Transports = append(a.doc.Transports, Transport{ID: id, Kind: KindAnx, FromEgress: egress, ToIngress: ingress,
			AnxLine: &AnxLine{ServerName: AutoServerName}})
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

// hostsFor answers the hosts of the devices forwards name after their
// entries, the ones Control may link to.
func hostsFor(db *gorm.DB, forwards []Forward) (map[string]string, error) {
	hosts := map[string]string{}
	var proxies, others []uint
	for _, forward := range forwards {
		for _, ref := range forward.Via {
			node, err := agentcontrol.ParseAgentNode(ref)
			if err != nil || node.ID == 0 {
				continue
			}
			if node.Kind == agentcontrol.NodeKindProxy {
				proxies = append(proxies, uint(node.ID))
			} else {
				others = append(others, uint(node.ID))
			}
		}
	}
	if len(proxies) > 0 {
		var rows []model.Node
		if err := db.Select("id", "host").Where("id IN ?", proxies).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			hosts[agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(row.ID)}.String()] = row.Host // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
		}
	}
	if len(others) > 0 {
		var rows []model.ForwardNode
		if err := db.Select("id", "host").Where("id IN ?", others).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			hosts[agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(row.ID)}.String()] = row.Host // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
		}
	}
	return hosts, nil
}
