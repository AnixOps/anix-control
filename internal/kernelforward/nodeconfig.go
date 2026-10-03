package kernelforward

import (
	"fmt"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/wire"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// NodeConfigMember answers what a node's desired configuration carries for
// forwarding (forward-sdk.md section 8.1). negotiated is the node's
// persisted flag: whether its Agent's last Hello negotiated forward.v1.
// Every builder of the node's configuration (the Hello reconcile, the
// periodic refresh, node.sync) decides the format from it, never from a
// live session, so they all build the same document and the configuration
// revision does not flap. With negotiated, member is the value of the
// anixops.nodeconfig/v2 member "forward": the node's stamped
// NodeForwardState, or a state holding only node_ref (generation 0: Control
// has no state for the node yet, and the Agent keeps what it runs).
func NodeConfigMember(db *gorm.DB, node agentcontrol.AgentNode) (negotiated bool, member map[string]any, err error) {
	var rows []model.KernelForwardNode
	if err := db.Select("node_ref", "negotiated").Where("node_ref = ?", node.String()).Limit(1).Find(&rows).Error; err != nil {
		if !db.Migrator().HasTable(&model.KernelForwardNode{}) {
			// A database without the forwarding tables carries none.
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("kernel forward: node %s: %w", node, err)
	}
	if len(rows) == 0 || !rows[0].Negotiated {
		return false, nil, nil
	}
	state, found, err := loadState(db, node.String())
	if err != nil {
		return false, nil, err
	}
	if !found {
		state = &forwardv1.NodeForwardState{NodeRef: node.String()}
	}
	member, err = wire.StateMember(state)
	if err != nil {
		return false, nil, err
	}
	return true, member, nil
}
