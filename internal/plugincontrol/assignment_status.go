package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

type observedPluginResult struct {
	ObservedVersion  string `json:"observed_version"`
	ObservedRevision int64  `json:"observed_revision"`
	Health           string `json:"health"`
	CleanupPending   bool   `json:"cleanup_pending"`
	LastError        string `json:"last_error"`
}

func decodeObservedPluginResult(raw string) observedPluginResult {
	var value observedPluginResult
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}

func loadPluginAssignmentState(db *gorm.DB, pluginID string, limit int) ([]model.NodeServiceAssignment, map[uint]model.Node, map[uint]model.NodeOperationRevision, map[uint]model.KernelOperation, error) {
	var assignments []model.NodeServiceAssignment
	if err := db.Where("plugin_id = ?", pluginID).Order("id ASC").Limit(limit).Find(&assignments).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	nodeIDs := make([]uint, 0, len(assignments))
	for _, assignment := range assignments {
		nodeIDs = append(nodeIDs, assignment.NodeID)
	}
	nodes := make(map[uint]model.Node, len(nodeIDs))
	cursors := make(map[uint]model.NodeOperationRevision, len(nodeIDs))
	operations := make(map[uint]model.KernelOperation, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return assignments, nodes, cursors, operations, nil
	}
	var nodeRows []model.Node
	if err := db.Where("id IN ?", nodeIDs).Find(&nodeRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	for _, node := range nodeRows {
		nodes[node.ID] = node
	}
	var cursorRows []model.NodeOperationRevision
	if err := db.Where("node_id IN ?", nodeIDs).Find(&cursorRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	for _, cursor := range cursorRows {
		cursors[cursor.NodeID] = cursor
	}
	var operationRows []model.KernelOperation
	if err := db.Where("plugin_id = ? AND node_id IN ?", pluginID, nodeIDs).
		Order("updated_at DESC, id DESC").Find(&operationRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	for _, operation := range operationRows {
		if operation.NodeID == nil {
			continue
		}
		if _, exists := operations[*operation.NodeID]; !exists {
			operations[*operation.NodeID] = operation
		}
	}
	return assignments, nodes, cursors, operations, nil
}

func executeReadOnlyAssignmentLifecycle(ctx context.Context, request LifecycleRequest, inspect func(context.Context) (any, error)) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Config) == 0 {
		request.Config = json.RawMessage(`{}`)
	}
	trimmed := strings.TrimSpace(string(request.Config))
	if !json.Valid(request.Config) || !strings.HasPrefix(trimmed, "{") {
		return nil, ErrInvalidPluginInput
	}
	switch request.Kind {
	case "plugin.install":
		return json.RawMessage(`{"state":"installed"}`), nil
	case "plugin.disable":
		return json.RawMessage(`{"state":"disabled"}`), nil
	case "plugin.configure":
		return append(json.RawMessage(nil), request.Config...), nil
	case "plugin.enable", "plugin.update", "plugin.rollback", "plugin.health", "plugin.inspect":
		value, err := inspect(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	default:
		return nil, errors.New("unsupported plugin lifecycle operation")
	}
}
