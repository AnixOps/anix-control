package service

import (
	"github.com/anixops/v2board/internal/model"
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

func applyForwardNodeInventoryScope(query *gorm.DB, scope string) *gorm.DB {
	switch scope {
	case ForwardNodeInventoryScopeAnsible:
		return query.
			Where("type = ?", model.ForwardNodeTypeRelay).
			Where("(tags LIKE ? OR (api_port = 0 AND (api_token = '' OR api_token IS NULL)))", forwardNodeInventoryTagLike())
	case ForwardNodeInventoryScopeNodeX:
		return query.
			Where("NOT (type = ? AND (tags LIKE ? OR (api_port = 0 AND (api_token = '' OR api_token IS NULL))))", model.ForwardNodeTypeRelay, forwardNodeInventoryTagLike())
	default:
		return query
	}
}

func forwardNodeInventoryTagLike() string {
	return "%\"" + ForwardNodeInventoryTagAnsibleMachine + "\"%"
}
