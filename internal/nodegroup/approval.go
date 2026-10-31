package nodegroup

import (
	"context"
	"sort"
	"sync"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Approval of the nodes that joined a group with require_approval (owner
// decision D4). A pending node holds a certificate and connects, but Control
// sends it neither its configuration (config.v1) nor its users (users.v1)
// until an administrator approves it. Approving lets them flow at once
// (OnApproval); rejecting disables the node and revokes its certificates.

// NodeDecision is an administrator's decision on a pending node.
type NodeDecision struct {
	Node     string `json:"node"`
	Name     string `json:"name"`
	Approval string `json:"approval"`
}

// pendingNode loads and locks node, which must be pending approval.
func pendingNode(tx *gorm.DB, node agentcontrol.AgentNode) (string, error) {
	locked := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	var (
		name     string
		approval string
	)
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		var rows []model.Node
		if err := locked.Select("id", "name", "approval").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "", ErrNodeNotFound
		}
		name, approval = rows[0].Name, rows[0].Approval
	default:
		return "", ErrNodeNotFound
	}
	if approval != model.NodeApprovalPending {
		return "", ErrNotPending
	}
	return name, nil
}

func nodeTable(node agentcontrol.AgentNode) (any, string) {
	if node.Kind == agentcontrol.NodeKindForward {
		return &model.ForwardNode{}, "forward_node"
	}
	return &model.Node{}, auditTargetProxyNode
}

// Approve approves a pending node. Once committed, the listeners of
// OnApproval hear of it: the Agent Control stream then sends the node's
// configuration and starts its users.
func (s *Service) Approve(ctx context.Context, node agentcontrol.AgentNode, actor Actor) (NodeDecision, error) {
	if !node.Valid() {
		return NodeDecision{}, ErrNodeNotFound
	}
	var decision NodeDecision
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		name, err := pendingNode(tx, node)
		if err != nil {
			return err
		}
		table, target := nodeTable(node)
		if err := tx.Model(table).Where("id = ?", node.ID).Update("approval", model.NodeApprovalApproved).Error; err != nil {
			return err
		}
		decision = NodeDecision{Node: node.String(), Name: name, Approval: model.NodeApprovalApproved}
		return writeAudit(tx, actor, AuditActionNodeApprove, target, uint(node.ID), map[string]any{"node": node.String(), "name": name})
	})
	if err != nil {
		return NodeDecision{}, err
	}
	if node.Kind == agentcontrol.NodeKindProxy {
		service.DropNodeCache(uint(node.ID))
	}
	notifyApproval(node)
	return decision, nil
}

// Reject rejects a pending node: the node is disabled (its status), marked rejected, so it no longer
// counts toward its group, and its certificates, enrollments and link
// certificates are revoked; its open stream ends at the next heartbeat.
// The node row stays: deleting it is the node soft delete of the next
// phase of the 5.0 onboarding work.
func (s *Service) Reject(ctx context.Context, node agentcontrol.AgentNode, actor Actor) (NodeDecision, error) {
	if !node.Valid() {
		return NodeDecision{}, ErrNodeNotFound
	}
	var decision NodeDecision
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		name, err := pendingNode(tx, node)
		if err != nil {
			return err
		}
		table, target := nodeTable(node)
		updates := map[string]any{"approval": model.NodeApprovalRejected, "status": model.NodeStatusDisabled}
		if err := tx.Model(table).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := agentpki.RevokeNode(ctx, tx, node, agentpki.RevokeReasonNodeRejected); err != nil {
			return err
		}
		decision = NodeDecision{Node: node.String(), Name: name, Approval: model.NodeApprovalRejected}
		return writeAudit(tx, actor, AuditActionNodeReject, target, uint(node.ID), map[string]any{"node": node.String(), "name": name})
	})
	if err != nil {
		return NodeDecision{}, err
	}
	agentpki.ForgetRevocations()
	if node.Kind == agentcontrol.NodeKindProxy {
		service.DropNodeCache(uint(node.ID))
	}
	return decision, nil
}

// PendingApproval tells whether node is pending approval. A node that does
// not exist is not pending (the stream refuses it on its own), and neither
// is one in a schema without the approval column, where no node can wait
// for approval. Any other read error is returned: the caller withholds.
func PendingApproval(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode) (bool, error) {
	table, _ := nodeTable(node)
	var approvals []string
	if err := db.WithContext(ctx).Model(table).Where("id = ?", node.ID).Limit(1).Pluck("approval", &approvals).Error; err != nil {
		if ctx.Err() == nil && !db.WithContext(ctx).Migrator().HasColumn(table, "approval") {
			return false, nil
		}
		return false, err
	}
	return len(approvals) == 1 && approvals[0] == model.NodeApprovalPending, nil
}

// ApprovalListener hears of every node an administrator approved, after the
// approval committed.
type ApprovalListener func(node agentcontrol.AgentNode)

var (
	approvalMu        sync.Mutex
	approvalListeners = map[int]ApprovalListener{}
	approvalNext      int
)

// OnApproval registers listener for every approval in the process. The
// Agent Control stream registers one that pushes the approved node's
// configuration. cancel removes it.
func OnApproval(listener ApprovalListener) (cancel func()) {
	approvalMu.Lock()
	defer approvalMu.Unlock()
	approvalNext++
	id := approvalNext
	approvalListeners[id] = listener
	return func() {
		approvalMu.Lock()
		delete(approvalListeners, id)
		approvalMu.Unlock()
	}
}

func notifyApproval(node agentcontrol.AgentNode) {
	approvalMu.Lock()
	ids := make([]int, 0, len(approvalListeners))
	for id := range approvalListeners {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	listeners := make([]ApprovalListener, 0, len(ids))
	for _, id := range ids {
		listeners = append(listeners, approvalListeners[id])
	}
	approvalMu.Unlock()
	for _, listener := range listeners {
		listener(node)
	}
}
