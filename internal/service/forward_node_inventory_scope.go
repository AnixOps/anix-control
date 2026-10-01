package service

import (
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

const (
	ForwardNodeInventoryScopeAny     = ""
	ForwardNodeInventoryScopeNodeX   = "nodex"
	ForwardNodeInventoryScopeAnsible = "ansible"

	ForwardNodeInventoryTagAnsibleMachine = "ansible-machine"
)

func (s *ForwardNodeService) ListByInventoryScope(scope, nodeType string, status *int, page, pageSize int) ([]*model.ForwardNode, int64, error) {
	var nodes []*model.ForwardNode
	var total int64

	query := s.db.Model(&model.ForwardNode{})
	query = applyForwardNodeInventoryScope(query, scope)
	if nodeType != "" {
		query = query.Where("type = ?", nodeType)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&nodes).Error
	return nodes, total, err
}

func (s *ForwardNodeService) GetByIDForInventoryScope(id uint, scope string) (*model.ForwardNode, error) {
	var node model.ForwardNode
	query := applyForwardNodeInventoryScope(s.db.Model(&model.ForwardNode{}), scope)
	if err := query.First(&node, id).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// applyForwardNodeInventoryScope keeps the forward nodes of an inventory: an
// Ansible machine is a relay tagged so, or a relay without an API port and
// without a token. Whether a node has a token is read through the node
// credential split (nodesecrets.ForwardNodeTokenAbsent), never from the
// legacy column directly.
func applyForwardNodeInventoryScope(query *gorm.DB, scope string) *gorm.DB {
	switch scope {
	case ForwardNodeInventoryScopeAnsible:
		absent, args := nodesecrets.ForwardNodeTokenAbsent(query)
		return query.
			Where("type = ?", model.ForwardNodeTypeRelay).
			Where("(tags LIKE ? OR (api_port = 0 AND "+absent+"))", append([]any{forwardNodeInventoryTagLike()}, args...)...)
	case ForwardNodeInventoryScopeNodeX:
		absent, args := nodesecrets.ForwardNodeTokenAbsent(query)
		return query.
			Where("NOT (type = ? AND (tags LIKE ? OR (api_port = 0 AND "+absent+")))",
				append([]any{model.ForwardNodeTypeRelay, forwardNodeInventoryTagLike()}, args...)...)
	default:
		return query
	}
}

func forwardNodeInventoryTagLike() string {
	return "%\"" + ForwardNodeInventoryTagAnsibleMachine + "\"%"
}
