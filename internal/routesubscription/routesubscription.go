// Package routesubscription publishes forward routes in subscriptions
// (node-onboarding design Phase 10, D3). A forward route whose target is a
// protocol of a proxy node, published with 加入订阅, is offered to that
// protocol's subscription groups once per entry node: at the entry's
// address and port (or the route's entry HA hostname), with the protocol's
// settings and keys, named "<protocol's name> | <route>". It replaces the child relay node
// (parent_id) that never reported and so never reached a subscription.
//
// The protocol is found from the route's targets: a target whose host is the
// proxy node's host or the protocol's own host (compared as written, names
// unresolved), or a loopback target of a route whose last hop runs on the
// proxy node itself, on the protocol's port. A published route is offered
// while it is not paused or paused by Control, it forwards the transport the
// protocol runs on, the protocol is enabled and linked to the group, and the
// exit node serves (the subscription's own checks).
package routesubscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/entryaddr"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

var (
	// ErrNotFound: no route with that id.
	ErrNotFound = errors.New("route not found")
	// ErrInvalid: the route cannot be published, or the request is
	// invalid; the error says why.
	ErrInvalid = errors.New("invalid route subscription")
)

// Reasons a route cannot be offered (Status.Reason).
const (
	// ReasonNoProtocol: no target is a proxy node's enabled protocol.
	ReasonNoProtocol = "no_protocol"
	// ReasonL4Mismatch: the route does not forward the transport the
	// protocol runs on.
	ReasonL4Mismatch = "l4_mismatch"
	// ReasonPaused: the route is paused, or Control pauses it.
	ReasonPaused = "paused"
	// ReasonNoEntry: no entry node has an address and a port.
	ReasonNoEntry = "no_entry"
)

const (
	maxDisplayName   = 128
	auditModule      = "forward_route_subscription"
	auditTarget      = "forward_route"
	AuditActionWrite = "forward_route_subscription_set"
)

// Protocol is the proxy node protocol a route reaches.
type Protocol struct {
	ID       uint   `json:"id"`
	NodeID   uint   `json:"node_id"`
	NodeName string `json:"node_name"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Port     int    `json:"port"`
}

// Entry is one address subscribers dial: an entry node of the route.
type Entry struct {
	NodeRef  string `json:"node_ref"`
	NodeName string `json:"node_name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

// Status is a route's subscription state.
type Status struct {
	RouteID     string `json:"route_id"`
	RouteName   string `json:"route_name"`
	Published   bool   `json:"published"`
	DisplayName string `json:"display_name"`
	// Protocol is nil when no target is a proxy node's protocol.
	Protocol *Protocol `json:"protocol"`
	Entries  []Entry   `json:"entries"`
	// Reason is empty when the route can be offered, else why not
	// (Reason* constants); Message says it for people.
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Offer is a published route as a subscription offers it, for one entry.
type Offer struct {
	RouteID string
	// Label follows " | " in the node's name.
	Label string
	Host  string
	Port  int
	// Entry is the entry node's index among the route's entries.
	Entry int
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages route subscriptions on one database.
type Service struct {
	DB *gorm.DB
	// Now defaults to time.Now.
	Now func() time.Time
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

var jsonRead = protojson.UnmarshalOptions{DiscardUnknown: true}

// route is a stored route with Control's pause.
type route struct {
	route    *forwardv1.Route
	enforced string
}

func loadRoutes(db *gorm.DB, ids []string) (map[string]route, error) {
	routes := map[string]route{}
	if len(ids) == 0 {
		return routes, nil
	}
	var rows []model.KernelForwardRoute
	if err := db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		decoded := &forwardv1.Route{}
		if err := jsonRead.Unmarshal([]byte(row.RouteJSON), decoded); err != nil {
			return nil, fmt.Errorf("route subscription: route %s: %w", row.ID, err)
		}
		routes[row.ID] = route{route: decoded, enforced: row.Enforced}
	}
	return routes, nil
}

// Get answers the route's subscription state.
func (s *Service) Get(ctx context.Context, routeID string) (Status, error) {
	db := s.DB.WithContext(ctx)
	routes, err := loadRoutes(db, []string{routeID})
	if err != nil {
		return Status{}, err
	}
	r, ok := routes[routeID]
	if !ok {
		return Status{}, ErrNotFound
	}
	var row model.ForwardRouteSubscription
	published := true
	if err := db.Where("route_id = ?", routeID).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		published = false
	} else if err != nil {
		return Status{}, err
	}
	world, err := loadWorld(db)
	if err != nil {
		return Status{}, err
	}
	status := world.status(r, routeID)
	status.Published = published
	status.DisplayName = row.DisplayName
	return status, nil
}

var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// Set publishes the route (publish true) with an optional display name, or
// withdraws it, audited. Publishing needs a target that is a proxy node's
// protocol; a route that cannot be offered for another reason (paused, a
// transport mismatch) is published and offered once that is fixed.
func (s *Service) Set(ctx context.Context, routeID string, publish bool, displayName string, actor Actor) (Status, error) {
	displayName = strings.TrimSpace(displayName)
	if len(displayName) > maxDisplayName || controlChars.MatchString(displayName) || strings.Contains(displayName, "|") {
		return Status{}, fmt.Errorf("%w: the display name is at most %d characters, without | or control characters", ErrInvalid, maxDisplayName)
	}
	status, err := s.Get(ctx, routeID)
	if err != nil {
		return Status{}, err
	}
	if publish && status.Protocol == nil {
		return Status{}, fmt.Errorf("%w: %s", ErrInvalid, status.Message)
	}
	now := s.now()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("route_id = ?", routeID).Delete(&model.ForwardRouteSubscription{}).Error; err != nil {
			return err
		}
		if publish {
			if err := tx.Create(&model.ForwardRouteSubscription{RouteID: routeID, DisplayName: displayName, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				return err
			}
		}
		return writeAudit(tx, actor, routeID, publish, status.Protocol)
	})
	if err != nil {
		return Status{}, err
	}
	return s.Get(ctx, routeID)
}

// Offers answers the published routes that can be offered, by protocol id,
// in route id order.
func Offers(db *gorm.DB) (map[uint][]Offer, error) {
	offers := map[uint][]Offer{}
	// A database without the table (a test's own schema) publishes none.
	if !db.Migrator().HasTable(&model.ForwardRouteSubscription{}) {
		return offers, nil
	}
	var rows []model.ForwardRouteSubscription
	if err := db.Order("route_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return offers, nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RouteID)
	}
	routes, err := loadRoutes(db, ids)
	if err != nil {
		return nil, err
	}
	world, err := loadWorld(db)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		r, ok := routes[row.RouteID]
		if !ok {
			continue
		}
		status := world.status(r, row.RouteID)
		if status.Reason != "" || status.Protocol == nil {
			continue
		}
		label := row.DisplayName
		if label == "" {
			label = status.RouteName
		}
		for i, entry := range status.Entries {
			entryLabel := label
			if len(status.Entries) > 1 {
				entryLabel = fmt.Sprintf("%s %s", label, entry.NodeName)
			}
			offers[status.Protocol.ID] = append(offers[status.Protocol.ID], Offer{RouteID: row.RouteID, Label: entryLabel, Host: entry.Host, Port: entry.Port, Entry: i})
		}
	}
	return offers, nil
}

// world is what matching a route needs: the proxy nodes, their protocols,
// the forward nodes and the entry ports the planner allocated.
type world struct {
	nodes       map[uint]model.Node
	forwards    map[uint]model.ForwardNode
	protocols   []model.NodeProtocol
	byHost      map[string]map[uint]bool
	allocations map[string]map[string]uint32
	// nat are the nodes' enabled entries (NAT mappings), by proxy node id.
	nat map[uint][]model.NodeEntry
}

func loadWorld(db *gorm.DB) (*world, error) {
	w := &world{nodes: map[uint]model.Node{}, forwards: map[uint]model.ForwardNode{}, byHost: map[string]map[uint]bool{}, allocations: map[string]map[string]uint32{}}
	var nodes []model.Node
	if err := db.Select("id", "name", "host").Find(&nodes).Error; err != nil {
		return nil, err
	}
	for _, node := range nodes {
		w.nodes[node.ID] = node
		w.addHost(node.Host, node.ID)
	}
	var forwards []model.ForwardNode
	if err := db.Select("id", "name", "host").Find(&forwards).Error; err != nil {
		return nil, err
	}
	for _, node := range forwards {
		w.forwards[node.ID] = node
	}
	if err := db.Select("id", "node_id", "name", "type", "port", "enable", "host").Order("sort ASC, id ASC").Find(&w.protocols).Error; err != nil {
		return nil, err
	}
	for _, protocol := range w.protocols {
		if protocol.Host != nil {
			w.addHost(*protocol.Host, protocol.NodeID)
		}
	}
	var allocations []model.KernelForwardAllocation
	if err := db.Where("hop_index = ? AND released_at IS NULL", 0).Find(&allocations).Error; err != nil {
		return nil, err
	}
	for _, a := range allocations {
		if w.allocations[a.RouteID] == nil {
			w.allocations[a.RouteID] = map[string]uint32{}
		}
		w.allocations[a.RouteID][a.NodeRef] = a.Port
	}
	if db.Migrator().HasTable(&model.NodeEntry{}) {
		ids := make([]uint, 0, len(w.nodes))
		for id := range w.nodes {
			ids = append(ids, id)
		}
		var err error
		if w.nat, err = nodeentry.EnabledByNode(db, ids); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (w *world) addHost(host string, nodeID uint) {
	key := NormalizeHost(host)
	if key == "" {
		return
	}
	if w.byHost[key] == nil {
		w.byHost[key] = map[uint]bool{}
	}
	w.byHost[key][nodeID] = true
}

// status matches the route's targets and lists its entries.
func (w *world) status(r route, routeID string) Status {
	status := Status{RouteID: routeID, RouteName: r.route.GetName(), Entries: []Entry{}}
	if status.RouteName == "" {
		status.RouteName = routeID
	}
	status.Protocol = w.match(r.route)
	status.Entries = w.entries(r.route, routeID)
	tcp, udp := routeTransports(r.route)
	switch {
	case status.Protocol == nil:
		status.Reason, status.Message = ReasonNoProtocol, "no target of the route is an enabled protocol of a proxy node: point the route at a protocol's address and port"
	case r.route.GetPaused() || r.enforced != "":
		status.Reason, status.Message = ReasonPaused, "the route is paused"
	case protocolNeedsTCP(status.Protocol.Type) && !tcp:
		status.Reason, status.Message = ReasonL4Mismatch, fmt.Sprintf("%s runs on TCP, but the route forwards only UDP", status.Protocol.Type)
	case !protocolNeedsTCP(status.Protocol.Type) && !udp:
		status.Reason, status.Message = ReasonL4Mismatch, fmt.Sprintf("%s runs on UDP, but the route forwards only TCP", status.Protocol.Type)
	case len(status.Entries) == 0:
		status.Reason, status.Message = ReasonNoEntry, "no entry node of the route has an address and a port yet"
	}
	return status
}

// match answers the enabled protocol the route's targets reach, the first
// target that reaches one.
func (w *world) match(r *forwardv1.Route) *Protocol {
	hops := r.GetHops()
	var exitNodes map[uint]bool
	if len(hops) > 0 {
		exitNodes = map[uint]bool{}
		for _, ref := range hops[len(hops)-1].GetNodeRefs() {
			if node, err := agentcontrol.ParseAgentNode(ref); err == nil && node.Kind == agentcontrol.NodeKindProxy {
				exitNodes[uint(node.ID)] = true
			}
		}
	}
	for _, target := range r.GetTargets() {
		host := NormalizeHost(target.GetHost())
		nodes := w.byHost[host]
		if isLoopback(host) {
			nodes = exitNodes
		}
		for _, protocol := range w.protocols {
			if protocol.Enable != 1 || !nodes[protocol.NodeID] || protocol.Port != int(target.GetPort()) {
				continue
			}
			return &Protocol{ID: protocol.ID, NodeID: protocol.NodeID, NodeName: w.nodes[protocol.NodeID].Name, Name: protocol.Name, Type: string(protocol.Type), Port: protocol.Port}
		}
	}
	return nil
}

// entries lists the addresses subscribers dial: the route's entry hostname
// (entry HA, one name over every entry node) when it has one, else each
// entry node, at the route's listen address when it names one or the node's
// host, on the port the planner allocated on the node or the route's listen
// port.
func (w *world) entries(r *forwardv1.Route, routeID string) []Entry {
	entries := []Entry{}
	hops := r.GetHops()
	if len(hops) == 0 {
		return entries
	}
	if hostname := strings.TrimSuffix(strings.TrimSpace(r.GetListen().GetEntryHostname()), "."); hostname != "" {
		port := int(r.GetListen().GetPort())
		for _, ref := range hops[0].GetNodeRefs() {
			if allocated := w.allocations[routeID][ref]; port == 0 && allocated != 0 {
				port = int(allocated)
			}
		}
		if port != 0 {
			entries = append(entries, Entry{NodeName: hostname, Host: strings.ToLower(hostname), Port: port})
		}
		return entries
	}
	listen := r.GetListen().GetAddress()
	if listen == "0.0.0.0" || listen == "::" || listen == "[::]" {
		listen = ""
	}
	seen := map[string]bool{}
	for _, ref := range hops[0].GetNodeRefs() {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		node, err := agentcontrol.ParseAgentNode(ref)
		if err != nil {
			continue
		}
		var name, host string
		var nat []model.NodeEntry
		switch node.Kind {
		case agentcontrol.NodeKindForward:
			name, host = w.forwards[uint(node.ID)].Name, w.forwards[uint(node.ID)].Host
		case agentcontrol.NodeKindProxy:
			name, host = w.nodes[uint(node.ID)].Name, w.nodes[uint(node.ID)].Host
			nat = w.nat[uint(node.ID)]
		}
		if listen != "" {
			host = listen
		}
		port := int(r.GetListen().GetPort())
		if allocated := w.allocations[routeID][ref]; allocated != 0 {
			port = int(allocated)
		}
		// A NAT machine is reached at its public address and mapped port:
		// the route's own override first, then the node's NAT entry.
		host, port, _ = entryaddr.Resolve(r.GetLabels(), ref, host, port, nat)
		if name == "" {
			name = ref
		}
		if host == "" || port == 0 {
			continue
		}
		entries = append(entries, Entry{NodeRef: ref, NodeName: name, Host: strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"), Port: port})
	}
	return entries
}

func routeTransports(r *forwardv1.Route) (tcp, udp bool) {
	switch r.GetListen().GetProtocol() {
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP:
		return false, true
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		return true, true
	}
	return true, false
}

// protocolNeedsTCP: clients reach the protocol over TCP (Shadowsocks relays
// UDP too, but a client always needs its TCP port); else over UDP.
func protocolNeedsTCP(protocolType string) bool {
	switch model.ProtocolType(protocolType) {
	case model.ProtocolHysteria2, model.ProtocolTUIC, model.ProtocolWireGuard, model.ProtocolIWAN:
		return false
	}
	return true
}

// NormalizeHost is a host as targets and node hosts are compared: an IP
// address in its canonical form, or a lower-case name without the final dot.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func isLoopback(host string) bool {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsLoopback()
	}
	return host == "localhost"
}

func writeAudit(tx *gorm.DB, actor Actor, routeID string, publish bool, protocol *Protocol) error {
	content := map[string]any{"route_id": routeID, "published": publish}
	if protocol != nil {
		content["protocol_id"] = protocol.ID
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: AuditActionWrite, Module: auditModule,
		TargetType: auditTarget, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
