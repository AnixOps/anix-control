package nodegroup

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveNodesBetweenGroups(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		a, _ := f.createGroup(t, GroupInput{Name: "a", MaxNodes: 1})
		b, _ := f.createGroup(t, GroupInput{Name: "b"})
		// Nodes an administrator added belong to no group.
		manual := model.Node{Name: "manual", Host: "198.51.100.1", Approval: model.NodeApprovalApproved}
		other := model.Node{Name: "other", Host: "198.51.100.2", Approval: model.NodeApprovalApproved}
		require.NoError(t, f.db.Create(&manual).Error)
		require.NoError(t, f.db.Create(&other).Error)
		proxy := func(node model.Node) agentcontrol.AgentNode {
			return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(node.ID)} // #nosec G115 -- test ids are small.
		}
		groupOf := func(node model.Node) *uint {
			var row model.Node
			require.NoError(t, f.db.First(&row, node.ID).Error)
			return row.NodeGroupID
		}

		// Moving in ignores the node cap, which limits joins only.
		moved, err := f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(manual), proxy(other)}, &a.ID, f.admin)
		require.NoError(t, err)
		assert.Equal(t, []string{proxy(manual).String(), proxy(other).String()}, moved.Nodes)
		assert.Equal(t, &a.ID, groupOf(manual))
		view, err := f.svc.Get(ctx, a.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), view.Nodes.Live)

		// Between groups, then out of every group.
		_, err = f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(manual)}, &b.ID, f.admin)
		require.NoError(t, err)
		assert.Equal(t, &b.ID, groupOf(manual))
		_, err = f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(manual)}, nil, f.admin)
		require.NoError(t, err)
		assert.Nil(t, groupOf(manual))
		assert.Len(t, f.audits(t, AuditActionNodeMove), 4)

		// A group emptied by moves can be deleted.
		_, err = f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(other)}, nil, f.admin)
		require.NoError(t, err)
		require.NoError(t, f.svc.Delete(ctx, a.ID, f.admin))

		// A deleted group, a missing node or no node moves nothing.
		_, err = f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(other)}, &a.ID, f.admin)
		assert.ErrorIs(t, err, ErrNotFound)
		_, err = f.svc.Move(ctx, []agentcontrol.AgentNode{proxy(other), {Kind: agentcontrol.NodeKindProxy, ID: 9999}}, &b.ID, f.admin)
		assert.ErrorIs(t, err, ErrNodeNotFound)
		assert.Nil(t, groupOf(other))
		_, err = f.svc.Move(ctx, nil, &b.ID, f.admin)
		assert.ErrorIs(t, err, ErrInvalid)
	})
}
