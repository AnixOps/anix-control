package kernelforward

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// The one-shot migration of the gost routes onto the network model
// (network-model implementation.md step 5, "迁移"): every route with an
// ENGINE_GOST hop becomes an ENGINE_NETWORK route. Each pair of adjacent
// hops runs over a network model transport, reused when one already joins
// the two devices, else added: bare anx_line between two nodes labelled
// link=iepl or link=iplc (a trusted private line), the A2-jacketed anx
// kind otherwise, whose SNI is the gost link's server name. The added
// transports dial from a hop egress of the first device and reach the
// shared hop ingress of the second, both added when missing; the exit
// leaves by its device's internet egress, added (nat_direct) when missing.
// Routes on nftables alone are not touched. Lines are never invented: a
// device without one in the network model, like a hop served by several
// nodes, keeps its route on gost and is reported. A dry run answers the
// same report and changes nothing.

// MigrationIngressPort is where an added hop ingress listens.
const MigrationIngressPort = 24443

// migrationPrefix starts the ids of the components the migration adds.
const migrationPrefix = "mig-"

// MigratedRoute is a route the migration moves (or would move).
type MigratedRoute struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Owner      string   `json:"owner"`
	Transports []string `json:"transports"`
	Egress     string   `json:"egress"`
	// Notes are what changes beyond the engine: a direct mode dropped,
	// hop ports left to the transports.
	Notes []string `json:"notes,omitempty"`
}

// BlockedRoute is a gost route the migration leaves, and why.
type BlockedRoute struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Reasons []string `json:"reasons"`
}

// Migration is the migration's report.
type Migration struct {
	DryRun bool `json:"dry_run"`
	// Applied is true when the routes moved.
	Applied bool            `json:"applied"`
	Routes  []MigratedRoute `json:"routes"`
	Blocked []BlockedRoute  `json:"blocked"`
	// Added are the network model components the migration adds, by kind
	// ("egress", "ingress", "transport") and id.
	Added []string `json:"added"`
	// Untouched counts the routes not on gost.
	Untouched int `json:"untouched"`
}

// MigrateGostRoutes moves every gost route it can onto the network model,
// or with dryRun only answers what it would do. The network model document
// is replaced first (audited as actor), then every movable route changes
// in one plan, so the routes move together or not at all.
func (s *Service) MigrateGostRoutes(ctx context.Context, dryRun bool, actor networkmodel.Actor) (*Migration, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	network := s.network(db)
	view, err := network.Get(ctx)
	if err != nil {
		return nil, err
	}
	routes, err := loadRoutes(db)
	if err != nil {
		return nil, err
	}
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	entryPorts, err := allocatedEntryPorts(db)
	if err != nil {
		return nil, err
	}
	m := newMigrator(&view.Document, inv)
	report := &Migration{DryRun: dryRun, Routes: []MigratedRoute{}, Blocked: []BlockedRoute{}, Added: []string{}}
	moved := map[string]*forwardv1.Route{}
	for _, stored := range routes {
		if !onGost(stored.route) {
			report.Untouched++
			continue
		}
		next, migrated, reasons := m.route(stored.route, entryPorts[stored.route.GetId()])
		if len(reasons) > 0 {
			report.Blocked = append(report.Blocked, BlockedRoute{ID: stored.route.GetId(), Name: stored.route.GetName(), Reasons: reasons})
			continue
		}
		report.Routes = append(report.Routes, migrated)
		moved[stored.route.GetId()] = next
	}
	report.Added = m.added
	if dryRun || len(moved) == 0 {
		return report, nil
	}
	if len(m.added) > 0 {
		if _, err := network.Replace(ctx, m.doc, &view.Revision, actor); err != nil {
			return report, fmt.Errorf("kernel forward: migrate: network model: %w", err)
		}
	}
	err = s.write(ctx, "migrate_network", func(tx *gorm.DB, now time.Time) (string, bool, error) {
		ids := make([]string, 0, len(moved))
		for id := range moved {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			var rows []model.KernelForwardRoute
			if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
				return "", false, err
			}
			if len(rows) == 0 {
				continue
			}
			route := moved[id]
			if rows[0].Revision != route.GetRevision() {
				return "", false, fmt.Errorf("%w: route %s changed during the migration", ErrRevisionMismatch, id)
			}
			route.Revision, route.UpdatedAtUnixMs = rows[0].Revision+1, now.UnixMilli()
			encoded, err := encodeRoute(route)
			if err != nil {
				return "", false, err
			}
			if err := tx.Model(&model.KernelForwardRoute{}).Where("id = ?", id).Updates(map[string]any{
				"revision": route.GetRevision(), "route_json": encoded, "updated_at": now,
			}).Error; err != nil {
				return "", false, fmt.Errorf("kernel forward: store route: %w", err)
			}
		}
		return "", false, nil
	})
	if err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

// allocatedEntryPorts answers each route's active entry port.
func allocatedEntryPorts(db *gorm.DB) (map[string]uint32, error) {
	var rows []model.KernelForwardAllocation
	if err := db.Where("hop_index = ? AND released_at IS NULL", 0).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load allocations: %w", err)
	}
	ports := map[string]uint32{}
	for _, row := range rows {
		ports[row.RouteID] = row.Port
	}
	return ports, nil
}

// onGost reports whether a hop of route runs gost.
func onGost(route *forwardv1.Route) bool {
	for _, hop := range route.GetHops() {
		if hop.GetEngine() == forwardv1.Engine_ENGINE_GOST {
			return true
		}
	}
	return false
}

// migrator adds to a copy of the network model document what the routes
// need.
type migrator struct {
	doc   *networkmodel.Document
	nodes map[string]*forwardv1.NodeInfo
	added []string
	ids   map[string]bool
}

func newMigrator(doc *networkmodel.Document, inv inventory) *migrator {
	m := &migrator{doc: doc, nodes: map[string]*forwardv1.NodeInfo{}, added: []string{}, ids: map[string]bool{}}
	for _, node := range inv.nodes {
		m.nodes[node.GetNodeRef()] = node
	}
	for _, line := range doc.Lines {
		m.ids[line.ID] = true
	}
	for _, ingress := range doc.Ingresses {
		m.ids[ingress.ID] = true
	}
	for _, egress := range doc.Egresses {
		m.ids[egress.ID] = true
	}
	for _, transport := range doc.Transports {
		m.ids[transport.ID] = true
	}
	for _, path := range doc.Paths {
		m.ids[path.ID] = true
	}
	return m
}

// line answers node's line: the first by id when it has several.
func (m *migrator) line(node string) *networkmodel.Line {
	var found *networkmodel.Line
	for i := range m.doc.Lines {
		line := &m.doc.Lines[i]
		if line.Node == node && (found == nil || line.ID < found.ID) {
			found = line
		}
	}
	return found
}

func (m *migrator) lineOf(id string) string {
	for _, line := range m.doc.Lines {
		if line.ID == id {
			return line.Node
		}
	}
	return ""
}

// add records an added component, answering base or, when taken, base
// with a number.
func (m *migrator) add(kind, base string) string {
	id := base
	for n := 2; m.ids[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	m.ids[id] = true
	m.added = append(m.added, kind+" "+id)
	return id
}

// egress answers node's egress of audience on line, added when missing.
func (m *migrator) egress(line *networkmodel.Line, audience string, add bool) string {
	for _, egress := range m.doc.Egresses {
		if egress.Line == line.ID && egress.Audience == audience && !egress.Disabled {
			return egress.ID
		}
	}
	if !add {
		return ""
	}
	id := m.add("egress", migrationPrefix+"eg-"+audience+"-"+line.Node)
	kind := "hop"
	if audience == "internet" {
		kind = "nat_direct"
	}
	m.doc.Egresses = append(m.doc.Egresses, networkmodel.Egress{
		ID: id, Line: line.ID, Kind: kind, Audience: audience, Family: "v4", Source: "auto",
	})
	return id
}

// hopIngress answers the hop ingress on line, added at publicHost when
// missing.
func (m *migrator) hopIngress(line *networkmodel.Line, publicHost string) string {
	for _, ingress := range m.doc.Ingresses {
		if ingress.Line == line.ID && ingress.Audience == "hop" && !ingress.Disabled {
			return ingress.ID
		}
	}
	id := m.add("ingress", migrationPrefix+"in-hop-"+line.Node)
	m.doc.Ingresses = append(m.doc.Ingresses, networkmodel.Ingress{
		ID: id, Line: line.ID, Kind: "direct", Audience: "hop", Family: "v4",
		Listen: networkmodel.Endpoint{Address: "0.0.0.0", Port: MigrationIngressPort}, PublicHost: publicHost,
	})
	return id
}

// transport answers a link transport from node from to node to, reused or
// added; serverName is the gost link's SNI, for an added anx transport.
func (m *migrator) transport(from, to *networkmodel.Line, publicHost, serverName string) (string, string) {
	ingresses := map[string]string{}
	for _, ingress := range m.doc.Ingresses {
		ingresses[ingress.ID] = ingress.Line
	}
	egresses := map[string]string{}
	for _, egress := range m.doc.Egresses {
		egresses[egress.ID] = egress.Line
	}
	for _, transport := range m.doc.Transports {
		if (transport.Kind == "anx_line" || transport.Kind == networkmodel.KindAnx) &&
			m.lineOf(egresses[transport.FromEgress]) == from.Node && m.lineOf(ingresses[transport.ToIngress]) == to.Node {
			return transport.ID, ""
		}
	}
	trusted := validate.IsTrustedLink(m.nodes[from.Node].GetLabels()) && validate.IsTrustedLink(m.nodes[to.Node].GetLabels())
	if !trusted && serverName == "" {
		return "", fmt.Sprintf("the link %s → %s crosses an untrusted network: the A2 jacket needs a server name (set one on the gost link, or add the transport)", from.Node, to.Node)
	}
	transport := networkmodel.Transport{FromEgress: m.egress(from, "hop", true), ToIngress: m.hopIngress(to, publicHost)}
	id := m.add("transport", migrationPrefix+"tr-"+from.Node+"-"+to.Node)
	transport.ID = id
	if trusted {
		transport.Kind, transport.Trusted = "anx_line", true
	} else {
		transport.Kind = networkmodel.KindAnx
		transport.AnxLine = &networkmodel.AnxLine{ServerName: serverName}
	}
	m.doc.Transports = append(m.doc.Transports, transport)
	return id, ""
}

// route answers route moved onto the network model, or why it cannot be.
// The document grows only for a route that moves.
func (m *migrator) route(route *forwardv1.Route, entryPort uint32) (*forwardv1.Route, MigratedRoute, []string) {
	var reasons []string
	hops := route.GetHops()
	lines := make([]*networkmodel.Line, len(hops))
	for i, hop := range hops {
		switch {
		case len(hop.GetNodeRefs()) != 1:
			reasons = append(reasons, fmt.Sprintf("hop %d is served by %d nodes; a network hop is one device", i, len(hop.GetNodeRefs())))
		case m.line(hop.GetNodeRefs()[0]) == nil:
			reasons = append(reasons, fmt.Sprintf("%s has no line in the network model", hop.GetNodeRefs()[0]))
		default:
			lines[i] = m.line(hop.GetNodeRefs()[0])
		}
	}
	if len(reasons) > 0 {
		return nil, MigratedRoute{}, reasons
	}
	// Checked on a copy, so a blocked route leaves the document alone.
	saved := *m.doc
	saved.Ingresses = append([]networkmodel.Ingress(nil), m.doc.Ingresses...)
	saved.Egresses = append([]networkmodel.Egress(nil), m.doc.Egresses...)
	saved.Transports = append([]networkmodel.Transport(nil), m.doc.Transports...)
	savedAdded := len(m.added)
	savedIDs := make(map[string]bool, len(m.ids))
	for id := range m.ids {
		savedIDs[id] = true
	}

	next := proto.Clone(route).(*forwardv1.Route)
	migrated := MigratedRoute{ID: route.GetId(), Name: route.GetName(), Owner: route.GetOwner(), Transports: []string{}}
	for i, hop := range next.GetHops() {
		hop.Engine = forwardv1.Engine_ENGINE_NETWORK
		if i == 0 {
			continue
		}
		node := m.nodes[hop.GetNodeRefs()[0]]
		publicHost := hop.GetDialAddress()
		if publicHost == "" && len(node.GetAddresses()) > 0 {
			publicHost = node.GetAddresses()[0]
		}
		if publicHost == "" {
			reasons = append(reasons, fmt.Sprintf("%s has no address to be dialed on", hop.GetNodeRefs()[0]))
			continue
		}
		id, reason := m.transport(lines[i-1], lines[i], publicHost, hop.GetIngress().GetServerName())
		if reason != "" {
			reasons = append(reasons, reason)
			continue
		}
		if hop.GetPort() != 0 {
			migrated.Notes = append(migrated.Notes, fmt.Sprintf("hop %d: port %d is left to transport %s", i, hop.GetPort(), id))
		}
		hop.Ingress = &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_NETWORK, NetworkTransportId: id}
		hop.Port, hop.DialAddress = 0, ""
		migrated.Transports = append(migrated.Transports, id)
	}
	last := next.GetHops()[len(next.GetHops())-1]
	last.NetworkEgressId = m.egress(lines[len(lines)-1], "internet", len(reasons) == 0)
	migrated.Egress = last.GetNetworkEgressId()
	if direct := next.GetPolicy().GetDirect(); direct == forwardv1.DirectMode_DIRECT_MODE_PREFERRED || direct == forwardv1.DirectMode_DIRECT_MODE_FORCED {
		next.Policy.Direct = forwardv1.DirectMode_DIRECT_MODE_OFF
		migrated.Notes = append(migrated.Notes, "the direct mode is dropped: a network route always runs its hops")
	}
	if next.GetListen().GetPort() == 0 {
		// The clients keep the port the planner allocated.
		if entryPort == 0 {
			reasons = append(reasons, "the route's listen port is not allocated yet; a network route listens on an explicit port")
		} else if next.Listen != nil {
			next.Listen.Port = entryPort
			migrated.Notes = append(migrated.Notes, fmt.Sprintf("listens on its allocated port %d", entryPort))
		}
	}
	if len(reasons) == 0 {
		if violations := validate.Proto(next, validate.Options{EnableNetwork: true}); len(violations) > 0 {
			for _, v := range violations {
				reasons = append(reasons, strings.TrimSpace(v.Field+": "+v.Message))
			}
		}
	}
	if len(reasons) > 0 {
		*m.doc, m.added, m.ids = saved, m.added[:savedAdded], savedIDs
		return nil, MigratedRoute{}, reasons
	}
	return next, migrated, nil
}
