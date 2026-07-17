package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/stretchr/testify/require"
)

func TestKernelOperationBridgeDispatchesDependenciesInOrder(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "dependency-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	now := time.Now()
	first, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "b08a651e-b8dd-4a2d-b59a-70dfce72ca01", IdempotencyKey: "dependency-install", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.install", ConfigJSON: `{}`, DeadlineAt: timePointer(now.Add(time.Minute)),
	})
	require.NoError(t, err)
	second, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "b08a651e-b8dd-4a2d-b59a-70dfce72ca02", IdempotencyKey: "dependency-configure", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.configure", ConfigJSON: `{}`,
		DependsOnOperationID: first.ID, DeadlineAt: timePointer(now.Add(2 * time.Minute)),
	})
	require.NoError(t, err)
	third, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "b08a651e-b8dd-4a2d-b59a-70dfce72ca03", IdempotencyKey: "dependency-enable", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.enable", ConfigJSON: `{}`,
		DependsOnOperationID: second.ID, DeadlineAt: timePointer(now.Add(3 * time.Minute)),
	})
	require.NoError(t, err)

	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "dependency-session"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	dispatched, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, dispatched)
	require.Len(t, stream.operations, 1)
	require.Equal(t, first.ID, stream.operations[0].OperationId)

	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", first.ID).Update("state", "succeeded").Error)
	dispatched, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, dispatched)
	require.Len(t, stream.operations, 2)
	require.Equal(t, second.ID, stream.operations[1].OperationId)

	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", second.ID).Update("state", "failed").Error)
	dispatched, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, dispatched)
	require.Len(t, stream.operations, 2)
	require.NoError(t, db.First(third, "id = ?", third.ID).Error)
	require.Equal(t, "superseded", third.State)
	require.Contains(t, third.LastError, second.ID)
}

func TestKernelOperationBridgeSerializesIndependentNodeRevisions(t *testing.T) {
	db := newKernelOperationBridgeDB(t)
	node := model.Node{Name: "serialized-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedAgentPluginRelease(t, db, "wireguard")
	deadline := time.Now().Add(time.Minute)
	first, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "48d526ea-8c68-4b0e-a52f-dd388b9c9b01", IdempotencyKey: "serialized-install", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.install", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	second, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: "48d526ea-8c68-4b0e-a52f-dd388b9c9b02", IdempotencyKey: "serialized-disable", NodeID: &node.ID,
		PluginID: "wireguard", TargetVersion: "1.0.0", Kind: "plugin.disable", ConfigJSON: `{}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	stream := &kernelOperationStreamStub{connected: true, snapshot: AgentControlSnapshot{NodeID: uint32(node.ID), SessionID: "serialized-session"}}
	bridge, err := NewKernelOperationBridge(db, stream)
	require.NoError(t, err)
	dispatched, err := bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, dispatched)
	require.Len(t, stream.operations, 1)
	require.Equal(t, first.ID, stream.operations[0].OperationId)

	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", first.ID).Update("state", "failed").Error)
	dispatched, err = bridge.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, dispatched)
	require.Len(t, stream.operations, 2)
	require.Equal(t, second.ID, stream.operations[1].OperationId)
}

func timePointer(value time.Time) *time.Time { return &value }
