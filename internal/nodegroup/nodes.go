package nodegroup

import (
	"context"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// onlineWindow is how recent a node's last check-in must be for it to
// count as online, the rule of the node list (model.Node.IsOnline).
const onlineWindow = 5 * time.Minute

// GroupNode is one live node of a node group, as the console's join list
// shows it while an administrator waits for a machine to join. It carries
// no credential and no configuration.
type GroupNode struct {
	ID uint `json:"id"`
	// Node is the node's name for the Agent APIs: proxy-<id> or
	// forward-<id>; approve and reject take it.
	Node string `json:"node"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Host is the address clients connect to.
	Host string `json:"host"`
	// Approval is approved or pending (rejected nodes are not live).
	Approval string `json:"approval"`
	// Online is true when the node is enabled and checked in within five
	// minutes, as the node list computes it.
	Online       bool       `json:"online"`
	LastSeen     *time.Time `json:"last_seen"`
	AgentVersion string     `json:"agent_version"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Nodes returns the live nodes of a group, newest first: every node of the
// group but the rejected ones (the nodes its counts include). ErrNotFound
// for a group that does not exist.
func (s *Service) Nodes(ctx context.Context, id uint) ([]GroupNode, error) {
	db := s.DB.WithContext(ctx)
	var group model.NodeGroup
	if err := db.Where("id = ?", id).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	now := s.now()
	nodes := []GroupNode{}
	if group.Kind == model.NodeGroupKindForward {
		var rows []model.ForwardNode
		if err := liveNodes(db, group).Order("id DESC").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			node := GroupNode{
				ID: row.ID, Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(row.ID)}.String(), // #nosec G115 -- node ids are below 2^32.
				Kind: model.NodeGroupKindForward, Name: row.Name, Host: row.Host, Approval: row.Approval, CreatedAt: row.CreatedAt,
			}
			if !row.LastCheck.IsZero() {
				seen := row.LastCheck.UTC()
				node.LastSeen = &seen
				node.Online = row.Enabled && row.Status == model.ForwardNodeStatusOnline && now.Sub(seen) < onlineWindow
			}
			nodes = append(nodes, node)
		}
	} else {
		var rows []model.Node
		if err := liveNodes(db, group).Order("id DESC").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			node := GroupNode{
				ID: row.ID, Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(row.ID)}.String(), // #nosec G115 -- node ids are below 2^32.
				Kind: model.NodeGroupKindProxy, Name: row.Name, Host: row.Host, Approval: row.Approval, CreatedAt: row.CreatedAt,
			}
			if row.ServerVersion != nil {
				node.AgentVersion = *row.ServerVersion
			}
			if row.LastCheckAt != nil {
				seen := time.Unix(*row.LastCheckAt, 0).UTC()
				node.LastSeen = &seen
				node.Online = row.Status != model.NodeStatusDisabled && now.Sub(seen) < onlineWindow
			}
			nodes = append(nodes, node)
		}
	}
	if err := s.fillAgentVersions(ctx, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// fillAgentVersions gives the nodes that report no version of their own
// (forward nodes have no such column) the one their Agent said in its
// newest stream Hello, from the agent transport inventory's table.
func (s *Service) fillAgentVersions(ctx context.Context, nodes []GroupNode) error {
	var missing []uint
	kind := ""
	for _, node := range nodes {
		if node.AgentVersion == "" {
			missing = append(missing, node.ID)
			kind = node.Kind
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var rows []model.AgentTransport
	if err := s.DB.WithContext(ctx).Where("node_kind = ? AND node_id IN ? AND agent_version <> ''", kind, missing).
		Order("last_seen_at DESC").Find(&rows).Error; err != nil {
		return err
	}
	versions := map[uint]string{}
	for _, row := range rows {
		if _, ok := versions[row.NodeID]; !ok {
			versions[row.NodeID] = row.AgentVersion
		}
	}
	for i := range nodes {
		if nodes[i].AgentVersion == "" {
			nodes[i].AgentVersion = versions[nodes[i].ID]
		}
	}
	return nil
}
