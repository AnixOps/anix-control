// Package egressroute is domain-based egress between proxy nodes: an
// egress route sends what a source node's users open to some domains out
// through an exit node instead of the source node's own address (for
// example AI services from Hong Kong through Japan).
//
// The hop is Shadowsocks 2022 (model.EgressRouteMethod) with a key Control
// generates: the exit node's Agent serves a relay inbound on the route's
// port, the source node's Agent sends the route's domains to it. Neither
// is a protocol of a node, so neither is in a subscription. Both nodes
// get the route in the member "egress" of their desired configuration
// (NodeMember), which reaches their Agents with the next configuration
// refresh.
package egressroute

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

var (
	// ErrNotFound: no egress route with that id.
	ErrNotFound = errors.New("egress route not found")
	// ErrInvalid: a field is invalid; the error says which.
	ErrInvalid = errors.New("invalid egress route")
	// ErrPortTaken: the exit node already uses the relay port.
	ErrPortTaken = errors.New("the exit node already uses this port")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Limits of a route.
const (
	maxNameLength   = 100
	maxDomains      = 1000
	maxDomainLength = 253
)

// Audit actions of the operation log (v2_operation_log, module
// egress_route). No entry carries a relay key.
const (
	auditModule       = "egress_route"
	auditTarget       = "egress_route"
	AuditActionCreate = "egress_route_create"
	AuditActionUpdate = "egress_route_update"
	AuditActionDelete = "egress_route_delete"
)

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages egress routes on one database.
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

// Input is a route as an administrator writes it.
type Input struct {
	Name         string   `json:"name"`
	SourceNodeID uint     `json:"source_node_id"`
	ExitNodeID   uint     `json:"exit_node_id"`
	ExitHost     string   `json:"exit_host"`
	RelayPort    int      `json:"relay_port"`
	Domains      []string `json:"domains"`
	Enabled      *bool    `json:"enabled"`
	// RotateKey replaces the relay key (update only); a new route always
	// gets one.
	RotateKey bool `json:"rotate_key"`
}

// NodeRef names a node of a route.
type NodeRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	// Exists is false when the node was deleted after the route was made.
	Exists bool `json:"exists"`
}

// View is a route as the API answers it: never the relay key.
type View struct {
	ID         uint     `json:"id"`
	Name       string   `json:"name"`
	SourceNode NodeRef  `json:"source_node"`
	ExitNode   NodeRef  `json:"exit_node"`
	ExitHost   string   `json:"exit_host"`
	RelayPort  int      `json:"relay_port"`
	Method     string   `json:"method"`
	Domains    []string `json:"domains"`
	Enabled    bool     `json:"enabled"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// List answers every route by id.
func (s *Service) List(ctx context.Context) ([]View, error) {
	var rows []model.EgressRoute
	if err := s.DB.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	nodes, err := s.nodesOf(ctx, rows)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		views = append(views, viewOf(row, nodes))
	}
	return views, nil
}

// Get answers one route.
func (s *Service) Get(ctx context.Context, id uint) (View, error) {
	row, err := s.load(s.DB.WithContext(ctx), id)
	if err != nil {
		return View{}, err
	}
	nodes, err := s.nodesOf(ctx, []model.EgressRoute{row})
	if err != nil {
		return View{}, err
	}
	return viewOf(row, nodes), nil
}

// Create stores a new route with a new relay key.
func (s *Service) Create(ctx context.Context, input Input, actor Actor) (View, error) {
	key, err := newRelayKey()
	if err != nil {
		return View{}, err
	}
	var id uint
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.EgressRoute{Method: model.EgressRouteMethod, RelayKey: key, Enabled: true}
		if err := s.apply(tx, &row, input); err != nil {
			return err
		}
		now := s.now()
		row.CreatedAt, row.UpdatedAt = now, now
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
		return writeAudit(tx, actor, AuditActionCreate, row.ID, auditContent(row))
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, id)
}

// Update replaces every field of the route; its key only with RotateKey.
func (s *Service) Update(ctx context.Context, id uint, input Input, actor Actor) (View, error) {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.load(tx, id)
		if err != nil {
			return err
		}
		if err := s.apply(tx, &row, input); err != nil {
			return err
		}
		if input.RotateKey {
			if row.RelayKey, err = newRelayKey(); err != nil {
				return err
			}
		}
		row.UpdatedAt = s.now()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		content := auditContent(row)
		content["key_rotated"] = input.RotateKey
		return writeAudit(tx, actor, AuditActionUpdate, row.ID, content)
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes the route; both nodes drop it with their next
// configuration.
func (s *Service) Delete(ctx context.Context, id uint, actor Actor) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.load(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Delete(&model.EgressRoute{}, row.ID).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionDelete, row.ID, auditContent(row))
	})
}

func (s *Service) load(tx *gorm.DB, id uint) (model.EgressRoute, error) {
	var rows []model.EgressRoute
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return model.EgressRoute{}, err
	}
	if len(rows) == 0 {
		return model.EgressRoute{}, ErrNotFound
	}
	return rows[0], nil
}

// apply validates input and writes it into row.
func (s *Service) apply(tx *gorm.DB, row *model.EgressRoute, input Input) error {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > maxNameLength {
		return invalid("name: 1 to %d characters", maxNameLength)
	}
	if input.SourceNodeID == 0 || input.ExitNodeID == 0 {
		return invalid("source_node_id and exit_node_id are required")
	}
	if input.SourceNodeID == input.ExitNodeID {
		return invalid("exit_node_id: the exit node must be another node")
	}
	source, err := loadNode(tx, input.SourceNodeID)
	if err != nil {
		return err
	}
	if source == nil {
		return invalid("source_node_id: no node %d", input.SourceNodeID)
	}
	exit, err := loadNode(tx, input.ExitNodeID)
	if err != nil {
		return err
	}
	if exit == nil {
		return invalid("exit_node_id: no node %d", input.ExitNodeID)
	}
	exitHost := strings.TrimSpace(input.ExitHost)
	if len(exitHost) > 255 || strings.ContainsAny(exitHost, " /\t") {
		return invalid("exit_host: a host name or address")
	}
	if exitHost == "" && strings.TrimSpace(exit.Host) == "" {
		return invalid("exit_host: the exit node has no host; give the address the source node dials")
	}
	if input.RelayPort < 1 || input.RelayPort > 65535 {
		return invalid("relay_port: 1 to 65535")
	}
	domains, err := NormalizeDomains(input.Domains)
	if err != nil {
		return err
	}
	if err := checkPort(tx, row.ID, input.ExitNodeID, input.RelayPort); err != nil {
		return err
	}
	encoded, err := json.Marshal(domains)
	if err != nil {
		return err
	}
	row.Name, row.SourceNodeID, row.ExitNodeID, row.ExitHost = name, input.SourceNodeID, input.ExitNodeID, exitHost
	row.RelayPort, row.Domains = input.RelayPort, string(encoded)
	if input.Enabled != nil {
		row.Enabled = *input.Enabled
	}
	return nil
}

// checkPort refuses a relay port another route of the exit node, or one of
// its enabled protocols, listens on.
func checkPort(tx *gorm.DB, routeID, exitNodeID uint, port int) error {
	var routes int64
	if err := tx.Model(&model.EgressRoute{}).Where("exit_node_id = ? AND relay_port = ? AND id <> ?", exitNodeID, port, routeID).
		Count(&routes).Error; err != nil {
		return err
	}
	if routes > 0 {
		return fmt.Errorf("%w: another egress route's relay listens on %d", ErrPortTaken, port)
	}
	var protocols int64
	if err := tx.Model(&model.NodeProtocol{}).Where("node_id = ? AND port = ? AND enable = 1", exitNodeID, port).
		Count(&protocols).Error; err != nil {
		return err
	}
	if protocols > 0 {
		return fmt.Errorf("%w: a protocol of the node listens on %d", ErrPortTaken, port)
	}
	return nil
}

// domainPattern is a domain name: labels of letters, digits, "-" and "_".
var domainPattern = regexp.MustCompile(`^[a-z0-9_]([a-z0-9_-]*[a-z0-9_])?(\.[a-z0-9_]([a-z0-9_-]*[a-z0-9_])?)*$`)

// NormalizeDomains checks and cleans a route's domains: trimmed, a bare
// name lowercased without a leading dot, empty lines and repeats dropped.
// A bare name matches the domain and its subdomains; full:, domain:,
// keyword: and regexp: entries are kept as the Agent's matchers.
func NormalizeDomains(domains []string) ([]string, error) {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(domains))
	for i, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if len(domain) > maxDomainLength {
			return nil, invalid("domains[%d]: longer than %d characters", i, maxDomainLength)
		}
		entry, err := normalizeDomain(domain)
		if err != nil {
			return nil, invalid("domains[%d]: %v", i, err)
		}
		if seen[entry] {
			continue
		}
		seen[entry] = true
		cleaned = append(cleaned, entry)
	}
	if len(cleaned) == 0 {
		return nil, invalid("domains: at least one domain")
	}
	if len(cleaned) > maxDomains {
		return nil, invalid("domains: at most %d", maxDomains)
	}
	return cleaned, nil
}

func normalizeDomain(domain string) (string, error) {
	prefix, value, prefixed := strings.Cut(domain, ":")
	if !prefixed {
		value = strings.TrimPrefix(strings.ToLower(domain), ".")
		if !domainPattern.MatchString(value) {
			return "", fmt.Errorf("%q is not a domain name", domain)
		}
		return value, nil
	}
	prefix = strings.ToLower(prefix)
	switch prefix {
	case "full", "domain":
		value = strings.TrimPrefix(strings.ToLower(value), ".")
		if !domainPattern.MatchString(value) {
			return "", fmt.Errorf("%q is not a domain name", value)
		}
	case "keyword":
		value = strings.ToLower(value)
		if value == "" || strings.ContainsAny(value, " \t") {
			return "", errors.New("keyword: a word without spaces")
		}
	case "regexp":
		if _, err := regexp.Compile(value); err != nil || value == "" {
			return "", fmt.Errorf("regexp: %q does not compile", value)
		}
	default:
		return "", fmt.Errorf("unknown matcher %q (full, domain, keyword or regexp)", prefix)
	}
	return prefix + ":" + value, nil
}

// newRelayKey is a Shadowsocks 2022 key for model.EgressRouteMethod.
func newRelayKey() (string, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func loadNode(tx *gorm.DB, id uint) (*model.Node, error) {
	var nodes []model.Node
	if err := tx.Where("id = ?", id).Limit(1).Find(&nodes).Error; err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return &nodes[0], nil
}

func (s *Service) nodesOf(ctx context.Context, rows []model.EgressRoute) (map[uint]model.Node, error) {
	ids := make([]uint, 0, 2*len(rows))
	for _, row := range rows {
		ids = append(ids, row.SourceNodeID, row.ExitNodeID)
	}
	nodes := map[uint]model.Node{}
	if len(ids) == 0 {
		return nodes, nil
	}
	var found []model.Node
	if err := s.DB.WithContext(ctx).Select("id", "name", "host").Where("id IN ?", ids).Find(&found).Error; err != nil {
		return nil, err
	}
	for _, node := range found {
		nodes[node.ID] = node
	}
	return nodes, nil
}

func nodeRef(id uint, nodes map[uint]model.Node) NodeRef {
	node, ok := nodes[id]
	return NodeRef{ID: id, Name: node.Name, Host: node.Host, Exists: ok}
}

func viewOf(row model.EgressRoute, nodes map[uint]model.Node) View {
	return View{
		ID: row.ID, Name: row.Name, SourceNode: nodeRef(row.SourceNodeID, nodes), ExitNode: nodeRef(row.ExitNodeID, nodes),
		ExitHost: row.ExitHost, RelayPort: row.RelayPort, Method: row.Method, Domains: decodeDomains(row.Domains),
		Enabled: row.Enabled, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func decodeDomains(encoded string) []string {
	var domains []string
	if err := json.Unmarshal([]byte(encoded), &domains); err != nil || domains == nil {
		return []string{}
	}
	return domains
}

// NodeMember is the member "egress" of a proxy node's desired
// configuration: the routes of the node's enabled egress routes as their
// source node (with the exit node's address and the relay key) and the
// relays of those it is the exit node of. Nil when the node takes part in
// none, so the member is left out and other nodes' configurations do not
// change.
//
// The member names no kernel: domains in the matcher vocabulary of
// NormalizeDomains, relays by protocol, host, port, method and key. Each
// Agent core (xray today, networkcore later) translates it into its own
// routing.
func NodeMember(db *gorm.DB, nodeID uint) (map[string]any, error) {
	var rows []model.EgressRoute
	if err := db.Where("enabled = ? AND (source_node_id = ? OR exit_node_id = ?)", true, nodeID, nodeID).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	routes := []map[string]any{}
	relays := []map[string]any{}
	for _, row := range rows {
		if row.ExitNodeID == nodeID {
			relays = append(relays, map[string]any{
				"id": row.ID, "protocol": model.EgressRouteProtocol, "port": row.RelayPort, "method": row.Method, "password": row.RelayKey,
			})
		}
		if row.SourceNodeID != nodeID {
			continue
		}
		host := row.ExitHost
		if host == "" {
			exit, err := loadNode(db, row.ExitNodeID)
			if err != nil {
				return nil, err
			}
			if exit == nil || strings.TrimSpace(exit.Host) == "" {
				// The exit node is gone or has no address: nothing to dial.
				continue
			}
			host = strings.TrimSpace(exit.Host)
		}
		routes = append(routes, map[string]any{
			"id": row.ID, "domains": decodeDomains(row.Domains),
			"server": map[string]any{"protocol": model.EgressRouteProtocol, "host": host, "port": row.RelayPort, "method": row.Method, "password": row.RelayKey},
		})
	}
	if len(routes) == 0 && len(relays) == 0 {
		return nil, nil
	}
	return map[string]any{"routes": routes, "relays": relays}, nil
}

// auditContent is what an audit entry says about a route: never its key.
func auditContent(row model.EgressRoute) map[string]any {
	return map[string]any{
		"name": row.Name, "source_node_id": row.SourceNodeID, "exit_node_id": row.ExitNodeID, "relay_port": row.RelayPort,
		"domains": len(decodeDomains(row.Domains)), "enabled": row.Enabled,
	}
}

func writeAudit(tx *gorm.DB, actor Actor, action string, targetID uint, content map[string]any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	target := targetID
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, TargetID: &target, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
