// Package nodeentry is the addresses subscribers reach a proxy node at,
// beyond its host: machines with several lines or address families, such
// as a domestic relay with a CN2 and a 9929 line where each line has a
// native IPv6 address and a provider NAT IPv4 address whose public ports
// map to the machine's.
//
// An entry is direct (the address on each protocol's own port) or NAT (a
// public address whose port ranges map to protocol ports). A node with
// enabled entries is offered in subscriptions once per entry and protocol,
// in place of its host; a protocol port that a NAT entry does not map is
// left out for that entry. The node's own configuration does not change:
// an Agent, or the forwarder of a relay without one, listens on the
// protocol ports as before (a relay that is entered over IPv6 and leaves
// over IPv4 needs a userspace forwarder that listens on [::], since kernel
// DNAT does not cross address families).
package nodeentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

var (
	// ErrNotFound: no node with that id.
	ErrNotFound = errors.New("node not found")
	// ErrInvalid: an entry is invalid; the error says which and why.
	ErrInvalid = errors.New("invalid node entry")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Limits of a node's entries.
const (
	MaxEntries       = 32
	MaxRanges        = 32
	maxLabelLength   = 64
	maxHostLength    = 253
	auditModule      = "node_entry"
	auditTarget      = "node"
	AuditActionWrite = "node_entries_replace"
)

// PortRange maps protocol ports From..To of a NAT entry to the public ports
// External..External+(To-From).
type PortRange struct {
	From     int `json:"from"`
	To       int `json:"to"`
	External int `json:"external"`
}

// Input is one entry as an administrator writes it.
type Input struct {
	Label   string      `json:"label"`
	Kind    string      `json:"kind"`
	Host    string      `json:"host"`
	PortMap []PortRange `json:"port_map"`
	Enabled *bool       `json:"enabled"`
}

// View is one entry as the API answers it.
type View struct {
	ID      uint        `json:"id"`
	Label   string      `json:"label"`
	Kind    string      `json:"kind"`
	Host    string      `json:"host"`
	Family  string      `json:"family"`
	PortMap []PortRange `json:"port_map"`
	Enabled bool        `json:"enabled"`
	Sort    int         `json:"sort"`
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages node entries on one database.
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

// List answers a node's entries in their order.
func (s *Service) List(ctx context.Context, nodeID uint) ([]View, error) {
	if err := s.requireNode(ctx, nodeID); err != nil {
		return nil, err
	}
	var rows []model.NodeEntry
	if err := s.DB.WithContext(ctx).Where("node_id = ?", nodeID).Order("sort ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		views = append(views, viewOf(row))
	}
	return views, nil
}

// Replace makes inputs the node's entries, in that order, audited. An empty
// list removes them all: the node is reached at its host again.
func (s *Service) Replace(ctx context.Context, nodeID uint, inputs []Input, actor Actor) ([]View, error) {
	rows, err := validate(inputs)
	if err != nil {
		return nil, err
	}
	if err := s.requireNode(ctx, nodeID); err != nil {
		return nil, err
	}
	now := s.now()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", nodeID).Delete(&model.NodeEntry{}).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].NodeID = nodeID
			rows[i].CreatedAt = now
			rows[i].UpdatedAt = now
			enabled := rows[i].Enabled
			if err := tx.Create(&rows[i]).Error; err != nil {
				return err
			}
			// GORM gives a false Enabled the column's default (true).
			if !enabled {
				if err := tx.Model(&rows[i]).Update("enabled", false).Error; err != nil {
					return err
				}
			}
		}
		return writeAudit(tx, actor, nodeID, rows)
	})
	if err != nil {
		return nil, err
	}
	return s.List(ctx, nodeID)
}

func (s *Service) requireNode(ctx context.Context, nodeID uint) error {
	var count int64
	if err := s.DB.WithContext(ctx).Model(&model.Node{}).Where("id = ?", nodeID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

var labelPattern = regexp.MustCompile(`^[^\x00-\x1f\x7f|]+$`)

// validate checks the inputs and builds their rows.
func validate(inputs []Input) ([]model.NodeEntry, error) {
	if len(inputs) > MaxEntries {
		return nil, invalid("a node has at most %d entries", MaxEntries)
	}
	rows := make([]model.NodeEntry, 0, len(inputs))
	seen := map[string]bool{}
	for i, input := range inputs {
		n := i + 1
		label := strings.TrimSpace(input.Label)
		if label == "" || len(label) > maxLabelLength || !labelPattern.MatchString(label) {
			return nil, invalid("entry %d: the label must be 1 to %d characters, without | or control characters", n, maxLabelLength)
		}
		if seen[label] {
			return nil, invalid("entry %d: the label %q is used twice", n, label)
		}
		seen[label] = true
		host, err := normalizeHost(input.Host)
		if err != nil {
			return nil, invalid("entry %d: %v", n, err)
		}
		row := model.NodeEntry{Label: label, Kind: input.Kind, Host: host, Enabled: input.Enabled == nil || *input.Enabled, Sort: i}
		switch input.Kind {
		case model.NodeEntryDirect:
			if len(input.PortMap) > 0 {
				return nil, invalid("entry %d: a direct entry uses the protocols' own ports and has no port map", n)
			}
		case model.NodeEntryNAT:
			if err := validateRanges(input.PortMap); err != nil {
				return nil, invalid("entry %d: %v", n, err)
			}
			encoded, err := json.Marshal(input.PortMap)
			if err != nil {
				return nil, err
			}
			row.PortMap = string(encoded)
		default:
			return nil, invalid("entry %d: kind must be %q or %q", n, model.NodeEntryDirect, model.NodeEntryNAT)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func validateRanges(ranges []PortRange) error {
	if len(ranges) == 0 {
		return errors.New("a NAT entry needs at least one port range")
	}
	if len(ranges) > MaxRanges {
		return fmt.Errorf("a NAT entry has at most %d port ranges", MaxRanges)
	}
	sorted := append([]PortRange(nil), ranges...)
	for _, r := range sorted {
		if r.From < 1 || r.To > 65535 || r.From > r.To {
			return fmt.Errorf("port range %d-%d: ports are 1 to 65535, from not above to", r.From, r.To)
		}
		if r.External < 1 || r.External+(r.To-r.From) > 65535 {
			return fmt.Errorf("port range %d-%d: the public ports from %d leave 1 to 65535", r.From, r.To, r.External)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].From <= sorted[i-1].To {
			return fmt.Errorf("port ranges %d-%d and %d-%d overlap", sorted[i-1].From, sorted[i-1].To, sorted[i].From, sorted[i].To)
		}
	}
	return nil
}

var hostnamePattern = regexp.MustCompile(`^(?i:[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?)(\.(?i:[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?))*\.?$`)

// normalizeHost accepts a domain or an IP address (an IPv6 address with or
// without brackets) and answers it without brackets.
func normalizeHost(value string) (string, error) {
	host := strings.TrimSpace(value)
	if len(host) >= 2 && strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" {
		return "", errors.New("the host is empty")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	if len(host) > maxHostLength || !hostnamePattern.MatchString(host) {
		return "", fmt.Errorf("the host %q is neither a domain nor an IP address", host)
	}
	return strings.ToLower(host), nil
}

// Family is "ipv4", "ipv6" or "domain".
func Family(host string) string {
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return "domain"
	case ip.To4() != nil:
		return "ipv4"
	default:
		return "ipv6"
	}
}

func viewOf(row model.NodeEntry) View {
	return View{
		ID: row.ID, Label: row.Label, Kind: row.Kind, Host: row.Host, Family: Family(row.Host),
		PortMap: Ranges(row), Enabled: row.Enabled, Sort: row.Sort,
	}
}

// Ranges answers a NAT entry's port ranges (none for a direct entry or a
// map that does not decode).
func Ranges(row model.NodeEntry) []PortRange {
	ranges := []PortRange{}
	if row.Kind != model.NodeEntryNAT || row.PortMap == "" {
		return ranges
	}
	if err := json.Unmarshal([]byte(row.PortMap), &ranges); err != nil {
		return []PortRange{}
	}
	return ranges
}

// Port answers the port subscribers dial to reach protocol port port
// through the entry, and false when a NAT entry does not map it.
func Port(row model.NodeEntry, port int) (int, bool) {
	if row.Kind != model.NodeEntryNAT {
		return port, port > 0
	}
	for _, r := range Ranges(row) {
		if port >= r.From && port <= r.To {
			return r.External + (port - r.From), true
		}
	}
	return 0, false
}

// Uncovered is an enabled protocol that subscriptions leave out because
// the node has enabled entries and none of them offers its port.
type Uncovered struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Port int    `json:"port"`
}

// Uncovered answers the node's enabled, shown protocols that no enabled
// entry offers, by port. A node without enabled entries answers none: it is
// reached at its host. The panel warns about them, since the protocol still
// shows online while every subscription skips it.
func (s *Service) Uncovered(ctx context.Context, nodeID uint) ([]Uncovered, error) {
	uncovered := []Uncovered{}
	byNode, err := EnabledByNode(s.DB.WithContext(ctx), []uint{nodeID})
	if err != nil {
		return nil, err
	}
	entries := byNode[nodeID]
	if len(entries) == 0 {
		return uncovered, nil
	}
	var protocols []model.NodeProtocol
	if err := s.DB.WithContext(ctx).Where("node_id = ? AND enable = ? AND show = ?", nodeID, 1, 1).Order("sort ASC, id ASC").Find(&protocols).Error; err != nil {
		return nil, err
	}
	for _, protocol := range protocols {
		offered := false
		for _, entry := range entries {
			if _, ok := Port(entry, protocol.Port); ok {
				offered = true
				break
			}
		}
		if !offered {
			uncovered = append(uncovered, Uncovered{ID: protocol.ID, Name: protocol.Name, Type: string(protocol.Type), Port: protocol.Port})
		}
	}
	return uncovered, nil
}

// EnabledByNode answers the enabled entries of the nodes, by node id in
// their order.
func EnabledByNode(db *gorm.DB, nodeIDs []uint) (map[uint][]model.NodeEntry, error) {
	byNode := map[uint][]model.NodeEntry{}
	if len(nodeIDs) == 0 {
		return byNode, nil
	}
	var rows []model.NodeEntry
	if err := db.Where("node_id IN ? AND enabled = ?", nodeIDs, true).Order("node_id ASC, sort ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		byNode[row.NodeID] = append(byNode[row.NodeID], row)
	}
	return byNode, nil
}

func writeAudit(tx *gorm.DB, actor Actor, nodeID uint, rows []model.NodeEntry) error {
	entries := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, map[string]any{"label": row.Label, "kind": row.Kind, "family": Family(row.Host), "ranges": len(Ranges(row)), "enabled": row.Enabled})
	}
	encoded, err := json.Marshal(map[string]any{"entries": entries})
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	target := nodeID
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: AuditActionWrite, Module: auditModule,
		TargetType: auditTarget, TargetID: &target, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
