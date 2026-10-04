package kernelforward

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The node inventory (forward-sdk.md section 4.3). A node is in it once its
// Agent negotiated forward.v1 (RecordHello) or an administrator gave it
// forwarding settings (SetNodeSettings), while its proxy or forward node
// row exists and is enabled. Its engines are those its Agent reported in
// its last forward.v1 Hello; a node that never reported has none, so
// validation refuses a route on it (engine_not_advertised).

const (
	// DefaultPortFirst and DefaultPortLast are the port range the planner
	// allocates from on a node without its own (decided by default with
	// F3a, as in the contract fixtures; the owner may revisit).
	DefaultPortFirst uint32 = 30000
	DefaultPortLast  uint32 = 39999
	// MaxNodeAddresses, MaxReservedPorts and MaxNodeLabels bound a node's
	// settings.
	MaxNodeAddresses = 16
	MaxReservedPorts = 1024
	MaxNodeLabels    = 32
)

// DefaultReservedPorts are reserved on every node, with the node's own
// service ports (the forward node's port, API and metrics ports, the proxy
// node's port and protocol ports): SSH.
var DefaultReservedPorts = []uint32{22}

// ErrNodeNotFound: the node's proxy or forward node row does not exist.
var ErrNodeNotFound = errors.New("forward node not found")

// NodeSettings are an administrator's forwarding settings for one node.
// Zero values mean the defaults: DefaultPortFirst to DefaultPortLast, no
// extra reserved ports, the node row's host when it is an IP address (the
// proxy node's reported server IP otherwise), no labels.
type NodeSettings struct {
	PortFirst, PortLast uint32
	ReservedPorts       []uint32
	// Addresses the node is reachable at, primary first: IP addresses only,
	// since the planner admits the previous hop by address.
	Addresses []string
	// Labels such as link=iepl mark trusted links.
	Labels map[string]string
}

func (n NodeSettings) check() error {
	switch {
	case (n.PortFirst == 0) != (n.PortLast == 0):
		return fmt.Errorf("%w: set both ends of the port range, or neither", ErrInvalidRequest)
	case n.PortFirst > n.PortLast || n.PortLast > validate.MaxPort:
		return fmt.Errorf("%w: port range %d-%d is not within 1-65535", ErrInvalidRequest, n.PortFirst, n.PortLast)
	case len(n.ReservedPorts) > MaxReservedPorts:
		return fmt.Errorf("%w: more than %d reserved ports", ErrInvalidRequest, MaxReservedPorts)
	case len(n.Addresses) > MaxNodeAddresses:
		return fmt.Errorf("%w: more than %d addresses", ErrInvalidRequest, MaxNodeAddresses)
	case len(n.Labels) > MaxNodeLabels:
		return fmt.Errorf("%w: more than %d labels", ErrInvalidRequest, MaxNodeLabels)
	}
	for _, port := range n.ReservedPorts {
		if port == 0 || port > validate.MaxPort {
			return fmt.Errorf("%w: reserved port %d is not within 1-65535", ErrInvalidRequest, port)
		}
	}
	for _, address := range n.Addresses {
		if _, err := netip.ParseAddr(address); err != nil {
			return fmt.Errorf("%w: address %q is not an IP address", ErrInvalidRequest, address)
		}
	}
	for key, value := range n.Labels {
		if key == "" || len(key) > validate.MaxLabelKeyBytes || len(value) > validate.MaxLabelValueBytes {
			return fmt.Errorf("%w: label %q is empty or too long", ErrInvalidRequest, key)
		}
	}
	return nil
}

// SetNodeSettings stores node's forwarding settings, which puts the node in
// the inventory, and replans. ErrNodeNotFound when the node's row does not
// exist.
func (s *Service) SetNodeSettings(ctx context.Context, node agentcontrol.AgentNode, settings NodeSettings) (PlanOutcome, error) {
	if err := settings.check(); err != nil {
		return PlanOutcome{}, err
	}
	db, err := s.db(ctx)
	if err != nil {
		return PlanOutcome{}, err
	}
	if exists, err := nodeRowExists(db, node); err != nil {
		return PlanOutcome{}, err
	} else if !exists {
		return PlanOutcome{}, fmt.Errorf("%w: %s", ErrNodeNotFound, node)
	}
	if err := storeSettings(db, node, settings, s.now()); err != nil {
		return PlanOutcome{}, err
	}
	return s.Replan(ctx, "node_settings")
}

// storeSettings upserts node's settings into its inventory row.
func storeSettings(db *gorm.DB, node agentcontrol.AgentNode, settings NodeSettings, now time.Time) error {
	reserved, _ := json.Marshal(sortedPorts(settings.ReservedPorts))
	addresses, _ := json.Marshal(settings.Addresses)
	labels, _ := json.Marshal(settings.Labels)
	row := model.KernelForwardNode{
		NodeRef: node.String(), NodeKind: node.Kind, NodeID: uint64(node.ID),
		PortFirst: settings.PortFirst, PortLast: settings.PortLast,
		ReservedPortsJSON: string(reserved), AddressesJSON: string(addresses), LabelsJSON: string(labels),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_ref"}},
		DoUpdates: clause.Assignments(map[string]any{
			"port_first": row.PortFirst, "port_last": row.PortLast, "reserved_ports_json": row.ReservedPortsJSON,
			"addresses_json": row.AddressesJSON, "labels_json": row.LabelsJSON, "updated_at": now,
		}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("kernel forward: store node settings: %w", err)
	}
	return nil
}

// RecordHello records what a node's Agent said in its Hello. caps is nil
// when the session did not negotiate forward.v1: the node's configuration
// then falls back to anixops.nodeconfig/v1 and its inventory entry (with
// its last capabilities) stays. With caps the node joins the inventory,
// its configuration becomes anixops.nodeconfig/v2, and a change of its
// capabilities replans. replanned reports whether a plan ran.
func (s *Service) RecordHello(ctx context.Context, node agentcontrol.AgentNode, caps *forwardv1.NodeCapabilities, agentVersion string) (outcome PlanOutcome, replanned bool, err error) {
	db, err := s.db(ctx)
	if err != nil {
		return PlanOutcome{}, false, err
	}
	now := s.now()
	ref := node.String()
	if caps == nil {
		result := db.Model(&model.KernelForwardNode{}).Where("node_ref = ? AND negotiated = ?", ref, true).
			Updates(map[string]any{"negotiated": false, "updated_at": now})
		if result.Error != nil && !db.Migrator().HasTable(&model.KernelForwardNode{}) {
			// A database without the forwarding tables has no flag to clear.
			return PlanOutcome{}, false, nil
		}
		return PlanOutcome{}, false, result.Error
	}
	encoded, err := jsonWrite.Marshal(caps)
	if err != nil {
		return PlanOutcome{}, false, fmt.Errorf("kernel forward: encode capabilities: %w", err)
	}
	hash := capabilitiesHash(caps)
	var rows []model.KernelForwardNode
	if err := db.Where("node_ref = ?", ref).Limit(1).Find(&rows).Error; err != nil {
		return PlanOutcome{}, false, err
	}
	if len(agentVersion) > 128 {
		agentVersion = agentVersion[:128]
	}
	if len(rows) == 0 {
		row := model.KernelForwardNode{
			NodeRef: ref, NodeKind: node.Kind, NodeID: uint64(node.ID), Negotiated: true,
			CapabilitiesJSON: string(encoded), CapabilitiesHash: hash, AgentVersion: agentVersion, ReportedAt: &now,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return PlanOutcome{}, false, fmt.Errorf("kernel forward: record node: %w", err)
		}
	} else {
		if err := db.Model(&model.KernelForwardNode{}).Where("node_ref = ?", ref).Updates(map[string]any{
			"negotiated": true, "capabilities_json": string(encoded), "capabilities_hash": hash,
			"agent_version": agentVersion, "reported_at": now, "updated_at": now,
		}).Error; err != nil {
			return PlanOutcome{}, false, fmt.Errorf("kernel forward: record node: %w", err)
		}
		if rows[0].CapabilitiesHash == hash {
			return PlanOutcome{}, false, nil
		}
	}
	outcome, err = s.Replan(ctx, "hello")
	return outcome, true, err
}

// capabilitiesHash is the SHA-256 of caps' deterministic encoding, without
// the agent version, which does not change what the node can do.
func capabilitiesHash(caps *forwardv1.NodeCapabilities) string {
	clone := proto.Clone(caps).(*forwardv1.NodeCapabilities)
	clone.AgentVersion = ""
	encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// nodeRowExists tells whether node's proxy or forward node row exists.
func nodeRowExists(db *gorm.DB, node agentcontrol.AgentNode) (bool, error) {
	var count int64
	var err error
	switch node.Kind {
	case agentcontrol.NodeKindForward:
		err = db.Model(&model.ForwardNode{}).Where("id = ?", node.ID).Count(&count).Error
	case agentcontrol.NodeKindProxy:
		err = db.Model(&model.Node{}).Where("id = ?", node.ID).Count(&count).Error
	default:
		return false, nil
	}
	return count > 0, err
}

// inventory is the planner's view of the nodes.
type inventory struct {
	nodes    []*forwardv1.NodeInfo
	reserved map[string][]uint32
}

// proxyRow is the part of a proxy node row the inventory reads.
type proxyRow struct {
	ID       uint
	Host     string
	Port     int
	Status   model.NodeStatus
	ServerIP *string
}

// loadInventory builds the inventory from the inventory rows and the nodes'
// own rows; a node whose row is gone or disabled is left out.
func loadInventory(tx *gorm.DB) (inventory, error) {
	var rows []model.KernelForwardNode
	if err := tx.Order("node_ref").Find(&rows).Error; err != nil {
		return inventory{}, fmt.Errorf("kernel forward: load inventory: %w", err)
	}
	var forwardIDs, proxyIDs []uint64
	for _, row := range rows {
		switch row.NodeKind {
		case agentcontrol.NodeKindForward:
			forwardIDs = append(forwardIDs, row.NodeID)
		case agentcontrol.NodeKindProxy:
			proxyIDs = append(proxyIDs, row.NodeID)
		}
	}
	forwards := map[uint64]model.ForwardNode{}
	if len(forwardIDs) > 0 {
		var nodes []model.ForwardNode
		if err := tx.Select("id", "host", "port", "api_port", "metrics_port", "enabled").Where("id IN ?", forwardIDs).Find(&nodes).Error; err != nil {
			return inventory{}, fmt.Errorf("kernel forward: load forward nodes: %w", err)
		}
		for _, node := range nodes {
			forwards[uint64(node.ID)] = node
		}
	}
	proxies := map[uint64]proxyRow{}
	protocolPorts := map[uint64][]uint32{}
	if len(proxyIDs) > 0 {
		var nodes []proxyRow
		if err := tx.Model(&model.Node{}).Select("id", "host", "port", "status", "server_ip").Where("id IN ?", proxyIDs).Find(&nodes).Error; err != nil {
			return inventory{}, fmt.Errorf("kernel forward: load proxy nodes: %w", err)
		}
		for _, node := range nodes {
			proxies[uint64(node.ID)] = node
		}
		var ports []struct {
			NodeID uint
			Port   int
		}
		if err := tx.Model(&model.NodeProtocol{}).Select("node_id", "port").Where("node_id IN ?", proxyIDs).Find(&ports).Error; err != nil {
			return inventory{}, fmt.Errorf("kernel forward: load proxy protocol ports: %w", err)
		}
		for _, port := range ports {
			protocolPorts[uint64(port.NodeID)] = append(protocolPorts[uint64(port.NodeID)], portOf(port.Port)...)
		}
	}
	inv := inventory{reserved: map[string][]uint32{}}
	for _, row := range rows {
		var host string
		var own []uint32
		switch row.NodeKind {
		case agentcontrol.NodeKindForward:
			node, ok := forwards[row.NodeID]
			if !ok || !node.Enabled {
				continue
			}
			host = node.Host
			own = append(own, portOf(node.Port)...)
			own = append(own, portOf(node.APIPort)...)
			own = append(own, portOf(node.MetricsPort)...)
		case agentcontrol.NodeKindProxy:
			node, ok := proxies[row.NodeID]
			if !ok || node.Status == model.NodeStatusDisabled {
				continue
			}
			host = node.Host
			if _, err := netip.ParseAddr(host); err != nil && node.ServerIP != nil {
				host = *node.ServerIP
			}
			own = append(own, portOf(node.Port)...)
			own = append(own, protocolPorts[row.NodeID]...)
		default:
			continue
		}
		info, extra := nodeInfo(row, host)
		inv.nodes = append(inv.nodes, info)
		reserved := append(append(append([]uint32(nil), DefaultReservedPorts...), own...), extra...)
		inv.reserved[row.NodeRef] = sortedPorts(reserved)
	}
	return inv, nil
}

// nodeInfo is a row's NodeInfo and its settings' reserved ports.
func nodeInfo(row model.KernelForwardNode, host string) (*forwardv1.NodeInfo, []uint32) {
	info := &forwardv1.NodeInfo{NodeRef: row.NodeRef, PortRange: &forwardv1.PortRange{First: DefaultPortFirst, Last: DefaultPortLast}}
	if row.PortFirst != 0 {
		info.PortRange = &forwardv1.PortRange{First: row.PortFirst, Last: row.PortLast}
	}
	var addresses []string
	if row.AddressesJSON != "" {
		_ = json.Unmarshal([]byte(row.AddressesJSON), &addresses)
	}
	if len(addresses) == 0 {
		if address, err := netip.ParseAddr(host); err == nil {
			addresses = []string{address.String()}
		}
	}
	info.Addresses = addresses
	if row.LabelsJSON != "" {
		_ = json.Unmarshal([]byte(row.LabelsJSON), &info.Labels)
	}
	if row.CapabilitiesJSON != "" {
		caps := &forwardv1.NodeCapabilities{}
		if err := jsonRead.Unmarshal([]byte(row.CapabilitiesJSON), caps); err == nil {
			info.Engines = caps.GetEngines()
		}
	}
	var extra []uint32
	if row.ReservedPortsJSON != "" {
		_ = json.Unmarshal([]byte(row.ReservedPortsJSON), &extra)
	}
	return info, extra
}

func portOf(port int) []uint32 {
	if port <= 0 || port > validate.MaxPort {
		return nil
	}
	return []uint32{uint32(port)} // #nosec G115 -- bounded above.
}

func sortedPorts(ports []uint32) []uint32 {
	out := append([]uint32(nil), ports...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return slices.Compact(out)
}

// NodeStatus is one inventory node as the kernel sees it.
type NodeStatus struct {
	NodeRef      string
	Negotiated   bool
	Capabilities *forwardv1.NodeCapabilities
	ReportedAt   *time.Time
	// Info is the planner's NodeInfo; nil when the node is left out (its
	// row is gone or disabled).
	Info          *forwardv1.NodeInfo
	ReservedPorts []uint32
}

// Nodes answers the inventory rows with the NodeInfo the planner sees.
func (s *Service) Nodes(ctx context.Context) ([]NodeStatus, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardNode
	if err := db.Order("node_ref").Find(&rows).Error; err != nil {
		return nil, err
	}
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	infos := map[string]*forwardv1.NodeInfo{}
	for _, info := range inv.nodes {
		infos[info.GetNodeRef()] = info
	}
	out := make([]NodeStatus, 0, len(rows))
	for _, row := range rows {
		status := NodeStatus{NodeRef: row.NodeRef, Negotiated: row.Negotiated, ReportedAt: row.ReportedAt,
			Info: infos[row.NodeRef], ReservedPorts: inv.reserved[row.NodeRef]}
		if row.CapabilitiesJSON != "" {
			caps := &forwardv1.NodeCapabilities{}
			if err := jsonRead.Unmarshal([]byte(row.CapabilitiesJSON), caps); err == nil {
				status.Capabilities = caps
			}
		}
		out = append(out, status)
	}
	return out, nil
}
