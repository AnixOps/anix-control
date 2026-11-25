package nodegroup

import (
	"context"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// AuditActionNodeMove is the operation log action of a node moved into a
// group or out of every group.
const AuditActionNodeMove = "node_group_move"

// maxMoveNodes caps the nodes of one move.
const maxMoveNodes = 500

// MoveResult is a move as committed: the group (nil when the nodes left
// every group) and the nodes, in the request's order.
type MoveResult struct {
	GroupID *uint    `json:"group_id"`
	Nodes   []string `json:"nodes"`
}

// Move puts nodes into the live group groupID, or takes them out of every
// group when groupID is nil: an administrator's grouping, independent of
// the token a node joined with. The join defaults of the group (rate, tags,
// traffic limit) are not applied; its relays are, at the node's next
// configuration refresh (control_route). A group's node cap limits joins
// only. ErrNotFound for a group that is not live, ErrNodeNotFound when a
// node does not exist; nothing moves then.
func (s *Service) Move(ctx context.Context, nodes []agentcontrol.AgentNode, groupID *uint, actor Actor) (MoveResult, error) {
	if len(nodes) == 0 || len(nodes) > maxMoveNodes {
		return MoveResult{}, invalid("nodes must list 1 to %d nodes", maxMoveNodes)
	}
	result := MoveResult{GroupID: groupID, Nodes: make([]string, 0, len(nodes))}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		groupName := ""
		if groupID != nil {
			group, err := lockGroup(tx, *groupID)
			if err != nil {
				return err
			}
			groupName = group.Name
		}
		for _, node := range nodes {
			if !node.Valid() {
				return ErrNodeNotFound
			}
			table, target := nodeTable(node)
			var rows []struct {
				Name        string
				NodeGroupID *uint
			}
			if err := tx.Model(table).Select("name", "node_group_id").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				return ErrNodeNotFound
			}
			if err := tx.Model(table).Where("id = ?", node.ID).Update("node_group_id", groupID).Error; err != nil {
				return err
			}
			result.Nodes = append(result.Nodes, node.String())
			if err := writeAudit(tx, actor, AuditActionNodeMove, target, uint(node.ID), map[string]any{
				"node": node.String(), "name": rows[0].Name, "from": rows[0].NodeGroupID, "to": groupID, "group": groupName,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MoveResult{}, err
	}
	for _, node := range nodes {
		if node.Kind == agentcontrol.NodeKindProxy {
			service.DropNodeCache(uint(node.ID))
		}
	}
	return result, nil
}
