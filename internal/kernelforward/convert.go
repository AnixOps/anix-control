package kernelforward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Converting a forward node into a regular node (docs/design/node-unification).
// One machine is one node: a relay is a proxy node (v2_node) without user
// protocols. A forward node's Agent runs no core, so it cannot run the
// network model (ANX relays); ConvertForwardNode moves such a node to a
// proxy node in one transaction under the plan lock:
//
//   - a proxy node is created from the forward node's name, host, group,
//     tags, machine fingerprint and approval, without protocols and hidden
//     from subscribers (show 0), with fresh node credentials;
//   - its forwarding settings (port range, reserved ports, addresses,
//     labels) are copied, and every route hop, port allocation and DNS
//     node row naming forward-<id> names proxy-<new id> instead, so the
//     route keeps its ports;
//   - the forward node is deleted, which revokes its Agent's certificates.
//
// The plan of every route with the change must be accepted, or nothing
// changes. The machine then needs the Agent reinstalled as the new node
// (its install command, with --forward); until it connects, its hops are
// down. A network model that names the forward node, or an Ansible
// machine, is refused: the first must be edited first, the second runs no
// Agent.

// ErrNotConvertible refuses a conversion the node cannot take.
var ErrNotConvertible = errors.New("kernel forward: node cannot be converted")

// Conversion answers ConvertForwardNode.
type Conversion struct {
	From   string `json:"from"`
	To     string `json:"to"`
	DryRun bool   `json:"dry_run"`
	// Routes are the ids of the routes whose hops moved.
	Routes []string `json:"routes"`
}

// ConvertForwardNode converts forward-<id> into a new proxy node. With
// dryRun it answers what would change (To is "proxy-<new>") and changes
// nothing.
func (s *Service) ConvertForwardNode(ctx context.Context, id uint64, dryRun bool, actor networkmodel.Actor) (*Conversion, error) {
	if id == 0 || id > maxNodeID {
		return nil, fmt.Errorf("%w: node id is required", ErrInvalidRequest)
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	from := forwardRef(uint(id))
	answer := &Conversion{From: from, DryRun: dryRun, Routes: []string{}}
	var outcome PlanOutcome
	errDryRun := errors.New("dry run")
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if _, err := lockPlan(tx, now); err != nil {
			return err
		}
		node, err := loadForwardNode(tx, id)
		if err != nil {
			return err
		}
		ansible, err := ansibleIDs(tx)
		if err != nil {
			return err
		}
		if ansible[node.ID] {
			return fmt.Errorf("%w: %s is an Ansible machine, which runs no Agent", ErrNotConvertible, from)
		}
		var models []model.NetworkModel
		if err := tx.Limit(1).Find(&models).Error; err != nil {
			return fmt.Errorf("kernel forward: load network model: %w", err)
		}
		if len(models) > 0 && strings.Contains(models[0].DocumentJSON, `"`+from+`"`) {
			return fmt.Errorf("%w: the network model names %s; move its lines to another node first", ErrNotConvertible, from)
		}

		created, err := createConvertedNode(tx, node)
		if err != nil {
			return err
		}
		to := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(created.ID)}.String() // #nosec G115 -- node ids are below 2^32.
		answer.To = to

		if err := moveInventoryRow(tx, from, created.ID, now); err != nil {
			return err
		}
		for _, row := range []any{&model.KernelForwardAllocation{}, &model.KernelForwardDNSNode{}} {
			if err := tx.Model(row).Where("node_ref = ?", from).Update("node_ref", to).Error; err != nil {
				return fmt.Errorf("kernel forward: move %s rows: %w", from, err)
			}
		}
		routes, err := loadRoutes(tx)
		if err != nil {
			return err
		}
		for _, stored := range routes {
			moved := false
			for _, hop := range stored.route.GetHops() {
				for j, ref := range hop.GetNodeRefs() {
					if ref == from {
						hop.NodeRefs[j], moved = to, true
					}
				}
			}
			if !moved {
				continue
			}
			stored.route.Revision++
			stored.route.UpdatedAtUnixMs = now.UnixMilli()
			encoded, err := encodeRoute(stored.route)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.KernelForwardRoute{}).Where("id = ?", stored.row.ID).
				Updates(map[string]any{"route_json": encoded, "revision": stored.route.GetRevision(), "updated_at": now}).Error; err != nil {
				return fmt.Errorf("kernel forward: update route %s: %w", stored.row.ID, err)
			}
			answer.Routes = append(answer.Routes, stored.row.ID)
		}

		if err := service.NewForwardNodeService(tx).Delete(node.ID); err != nil {
			return fmt.Errorf("kernel forward: delete forward node: %w", err)
		}
		for _, row := range []any{&model.KernelForwardNode{}, &model.KernelForwardNodeState{}, &model.KernelForwardNodeReport{}} {
			if err := tx.Where("node_ref = ?", from).Delete(row).Error; err != nil {
				return fmt.Errorf("kernel forward: delete node rows: %w", err)
			}
		}

		routes, err = loadRoutes(tx)
		if err != nil {
			return err
		}
		if outcome, err = s.replanTx(tx, routes, now); err != nil {
			return err
		}
		if outcome.Refused {
			return refused(outcome.Violations, "")
		}
		if dryRun {
			return errDryRun
		}
		if err := recordPlan(tx, outcome, "node_converted", now); err != nil {
			return err
		}
		return auditConversion(tx, actor, created.ID, answer)
	})
	if errors.Is(err, errDryRun) {
		return answer, nil
	}
	if err != nil {
		return nil, err
	}
	outcome.notify()
	return answer, nil
}

// auditConversion records the conversion in the operation log (module
// forward), against the new proxy node.
func auditConversion(tx *gorm.DB, actor networkmodel.Actor, nodeID uint, answer *Conversion) error {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	username, ip := actor.Name, actor.IP
	if len(username) > 100 {
		username = username[:100]
	}
	if len(ip) > 45 {
		ip = ip[:45]
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: username, Action: "node_convert", Module: "forward",
		TargetType: "node", TargetID: &nodeID, Content: string(encoded), IP: ip, Status: 1,
	}).Error
}

// createConvertedNode writes the proxy node a forward node becomes.
func createConvertedNode(tx *gorm.DB, from model.ForwardNode) (model.Node, error) {
	status := model.NodeStatusPending
	if !from.Enabled {
		status = model.NodeStatusDisabled
	}
	approval := from.Approval
	if approval == "" {
		approval = model.NodeApprovalApproved
	}
	node := model.Node{
		Name: from.Name, Host: from.Host, Port: 443, Status: status, Rate: 1, TrafficRate: 1, Show: 0,
		MonthlyResetDay: 1, NodeGroupID: from.NodeGroupID, MachineFingerprint: from.MachineFingerprint,
		Approval: approval, LastPublicAddress: from.LastPublicAddress, RuntimeHealthy: true,
	}
	if tags := strings.TrimSpace(from.Tags); tags != "" && tags != "[]" {
		node.Tags = &tags
	}
	credentials, err := service.GenerateProxyNodeCredentials(true, true)
	if err != nil {
		return model.Node{}, err
	}
	credentials.Apply(&node)
	if err := tx.Omit(clause.Associations).Create(&node).Error; err != nil {
		return model.Node{}, fmt.Errorf("kernel forward: create proxy node: %w", err)
	}
	// show has a database default of 1, which a zero value on create keeps.
	if err := tx.Model(&model.Node{}).Where("id = ?", node.ID).Update("show", 0).Error; err != nil {
		return model.Node{}, fmt.Errorf("kernel forward: hide proxy node: %w", err)
	}
	node.Show = 0
	if err := nodesecrets.Sync(tx, nodesecrets.TableNode, node.ID); err != nil {
		return model.Node{}, err
	}
	return node, nil
}

// moveInventoryRow copies the forward node's inventory settings to the new
// proxy node, with the engines its Agent last reported (renamed to the new
// node), so the routes plan before the Agent reconnects as the new node.
// The node is not negotiated until it does.
func moveInventoryRow(tx *gorm.DB, from string, to uint, now time.Time) error {
	var rows []model.KernelForwardNode
	if err := tx.Where("node_ref = ?", from).Limit(1).Find(&rows).Error; err != nil {
		return fmt.Errorf("kernel forward: load %s settings: %w", from, err)
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	ref := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(to)}.String() // #nosec G115 -- node ids are below 2^32.
	moved := model.KernelForwardNode{
		NodeRef: ref, NodeKind: agentcontrol.NodeKindProxy, NodeID: uint64(to), AgentVersion: row.AgentVersion,
		PortFirst: row.PortFirst, PortLast: row.PortLast, ReservedPortsJSON: row.ReservedPortsJSON,
		AddressesJSON: row.AddressesJSON, LabelsJSON: row.LabelsJSON, CreatedAt: now, UpdatedAt: now,
	}
	if row.CapabilitiesJSON != "" {
		caps := &forwardv1.NodeCapabilities{}
		if err := jsonRead.Unmarshal([]byte(row.CapabilitiesJSON), caps); err != nil {
			return fmt.Errorf("kernel forward: %s capabilities: %w", from, err)
		}
		caps.NodeRef = ref
		encoded, err := jsonWrite.Marshal(caps)
		if err != nil {
			return fmt.Errorf("kernel forward: encode capabilities: %w", err)
		}
		moved.CapabilitiesJSON, moved.CapabilitiesHash = string(encoded), capabilitiesHash(caps)
	}
	return tx.Create(&moved).Error
}
