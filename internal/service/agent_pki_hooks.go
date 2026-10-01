package service

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// The kernel paths that revoke, replace or disable a node's credentials, or
// delete the node, revoke the node's agent certificates and enrollments in
// the same transaction (node-ops-service.md, section 5.3).

// revokeProxyNodeAgentsIfDisabled revokes a proxy node's agent certificates
// when an update set its status to disabled.
func revokeProxyNodeAgentsIfDisabled(tx *gorm.DB, nodeID uint, updates map[string]any) error {
	if _, ok := updates["status"]; !ok {
		return nil
	}
	var row model.Node
	if err := tx.Select("id", "status").Where("id = ?", nodeID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if row.Status != model.NodeStatusDisabled {
		return nil
	}
	return revokeNodeAgents(tx, agentcontrol.NodeKindProxy, nodeID, agentpki.RevokeReasonNodeDisabled)
}

// forwardNodeAgentRevocation returns why saving node revokes its agent
// certificates: its token changed, or it is disabled. It reads the stored
// row, so call it before saving.
func forwardNodeAgentRevocation(tx *gorm.DB, node *model.ForwardNode) (string, error) {
	if node == nil || node.ID == 0 {
		return "", nil
	}
	var stored model.ForwardNode
	if err := tx.Select("id", "api_token").Where("id = ?", node.ID).First(&stored).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	// The stored token is read through the node credential split, so a
	// change is seen whichever form holds it.
	switch {
	case nodesecrets.ForwardNodeToken(tx, &stored) != node.APIToken:
		return agentpki.RevokeReasonCredentialsReplaced, nil
	case !node.Enabled:
		return agentpki.RevokeReasonNodeDisabled, nil
	default:
		return "", nil
	}
}

// revokeNodeAgents revokes every agent certificate and enrollment of a node.
func revokeNodeAgents(tx *gorm.DB, kind string, nodeID uint, reason string) error {
	if nodeID == 0 || uint64(nodeID) > uint64(^uint32(0)) {
		return nil
	}
	ctx := tx.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return agentpki.RevokeNode(ctx, tx, agentcontrol.AgentNode{Kind: kind, ID: uint32(nodeID)}, reason)
}
