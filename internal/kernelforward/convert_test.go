package kernelforward

import (
	"errors"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConvertForwardNode(t *testing.T)         { runConvertForwardNode(t, openSQLite(t)) }
func TestPostgresConvertForwardNode(t *testing.T) { runConvertForwardNode(t, openPostgres(t)) }

func runConvertForwardNode(t *testing.T, db *gorm.DB) {
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	f := newFixture(t, db)
	_, err := f.service.SetNodeSettings(f.ctx, exit, NodeSettings{PortFirst: 15301, PortLast: 15399})
	require.NoError(t, err)
	route := f.create("r1", twoHop(30000))
	before := map[uint32]uint32{}
	for _, row := range f.allocations() {
		before[row.HopIndex] = row.Port
	}

	t.Run("dry run changes nothing", func(t *testing.T) {
		answer, err := f.service.ConvertForwardNode(f.ctx, uint64(exit.ID), true, networkmodel.Actor{})
		require.NoError(t, err)
		require.True(t, answer.DryRun)
		require.Equal(t, "forward-12", answer.From)
		require.Equal(t, []string{route.GetId()}, answer.Routes)
		var count int64
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", exit.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
		require.NoError(t, db.Model(&model.Node{}).Count(&count).Error)
		require.EqualValues(t, 1, count, "only the fixture's proxy node")
	})

	answer, err := f.service.ConvertForwardNode(f.ctx, uint64(exit.ID), false, networkmodel.Actor{UserID: 1, Name: "admin@example.com"})
	require.NoError(t, err)
	to, err := agentcontrol.ParseAgentNode(answer.To)
	require.NoError(t, err)
	require.Equal(t, agentcontrol.NodeKindProxy, to.Kind)

	var node model.Node
	require.NoError(t, db.First(&node, to.ID).Error)
	require.Equal(t, "exit", node.Name)
	require.Equal(t, "192.0.2.12", node.Host)
	require.Zero(t, node.Show, "a relay is hidden from subscribers")
	require.NotEmpty(t, node.APIKey)
	var protocols int64
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("node_id = ?", to.ID).Count(&protocols).Error)
	require.Zero(t, protocols)

	var forwardNodes int64
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", exit.ID).Count(&forwardNodes).Error)
	require.Zero(t, forwardNodes)

	stored, err := f.service.GetRoute(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Equal(t, []string{answer.To}, stored.GetHops()[1].GetNodeRefs())
	require.Greater(t, stored.GetRevision(), route.GetRevision())
	for _, row := range f.allocations() {
		require.NotEqual(t, "forward-12", row.NodeRef)
		if row.ReleasedAt == nil {
			require.Equal(t, before[row.HopIndex], row.Port, "hop %d keeps its port", row.HopIndex)
		}
	}

	var audit model.OperationLog
	require.NoError(t, db.Where("module = ? AND action = ?", "forward", "node_convert").Take(&audit).Error)
	require.Contains(t, audit.Content, answer.To)

	var inventory model.KernelForwardNode
	require.NoError(t, db.Where("node_ref = ?", answer.To).Take(&inventory).Error)
	require.EqualValues(t, 15301, inventory.PortFirst)
	require.False(t, inventory.Negotiated)
	require.Contains(t, inventory.CapabilitiesJSON, answer.To)

	// The new node's Agent says hello and the exit hop is its.
	f.hello(to, nftCaps())
	state := f.state(to)
	require.Len(t, state.GetHops(), 1)
	require.Equal(t, route.GetId(), state.GetHops()[0].GetRouteId())

	_, err = f.service.ConvertForwardNode(f.ctx, uint64(exit.ID), false, networkmodel.Actor{UserID: 1, Name: "admin@example.com"})
	require.True(t, errors.Is(err, ErrNodeNotFound), err)
}

func TestConvertForwardNodeRefusals(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	require.NoError(t, f.db.Save(&model.NetworkModel{ID: 1, Revision: 1, DocumentJSON: `{"lines":[{"id":"l1","node":"forward-11"}]}`}).Error)
	_, err := f.service.ConvertForwardNode(f.ctx, 11, false, networkmodel.Actor{})
	require.True(t, errors.Is(err, ErrNotConvertible), err)

	_, err = f.service.ConvertForwardNode(f.ctx, 0, false, networkmodel.Actor{})
	require.True(t, errors.Is(err, ErrInvalidRequest), err)

	// A route whose plan the new node cannot take is refused whole: the
	// forward node stays.
	f.create("r1", twoHop(30000))
	require.NoError(t, f.db.Model(&model.KernelForwardNode{}).Where("node_ref = ?", "forward-12").Update("capabilities_json", "").Error)
	_, err = f.service.ConvertForwardNode(f.ctx, 12, false, networkmodel.Actor{})
	var refusedErr *RefusedError
	require.True(t, errors.As(err, &refusedErr), err)
	var count int64
	require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", 12).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
