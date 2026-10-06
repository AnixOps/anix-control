package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNodeOperationRevisionAllocateAndRaise(t *testing.T) {
	db := newKernelTestDB(t)

	revision, err := AllocateNodeOperationRevision(db, 7, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision, "the first revision of a node is 1")
	revision, err = AllocateNodeOperationRevision(db, 7, 5)
	require.NoError(t, err)
	require.Equal(t, int64(6), revision, "the allocation is above the caller's floor")
	revision, err = AllocateNodeOperationRevision(db, 7, 2)
	require.NoError(t, err)
	require.Equal(t, int64(7), revision, "a lower floor never reuses a revision")

	require.NoError(t, RaiseNodeOperationRevision(db, 7, 20))
	require.NoError(t, RaiseNodeOperationRevision(db, 7, 3), "a lower revision leaves the cursor alone")
	require.NoError(t, RaiseNodeOperationRevision(db, 8, 0))
	var cursor model.NodeOperationRevision
	require.NoError(t, db.First(&cursor, "node_id = ?", 7).Error)
	require.Equal(t, int64(20), cursor.DesiredRevision)
	require.Zero(t, cursor.ObservedRevision, "allocation never moves the observed revision")
	revision, err = AllocateNodeOperationRevision(db, 7, 0)
	require.NoError(t, err)
	require.Equal(t, int64(21), revision)

	revision, err = AllocateNodeOperationRevision(db, 9, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision, "nodes have cursors of their own")

	_, err = AllocateNodeOperationRevision(db, 7, -1)
	require.Error(t, err)
	_, err = AllocateNodeOperationRevision(nil, 7, 0)
	require.Error(t, err)
	require.Error(t, RaiseNodeOperationRevision(nil, 7, 1))
}

func TestRenumberPendingNodeOperations(t *testing.T) {
	db := newKernelTestDB(t)
	node := uint(7)
	seed := func(id string, revision int64, state string) {
		t.Helper()
		require.NoError(t, db.Create(&model.KernelOperation{
			ID: id, IdempotencyKey: "renumber:" + id, NodeID: &node, Revision: revision, State: state,
			SessionID: "session", Kind: "plugin.configure",
		}).Error)
	}
	revisionOf := func(id string) (int64, string) {
		t.Helper()
		var operation model.KernelOperation
		require.NoError(t, db.First(&operation, "id = ?", id).Error)
		return operation.Revision, operation.State
	}
	seed("done", 1, "succeeded")
	seed("stuck", 2, "dispatching")
	seed("behind-a", 3, "pending")
	seed("behind-b", 4, "pending")
	require.NoError(t, RaiseNodeOperationRevision(db, node, 4))

	revision, err := RenumberPendingNodeOperations(db, node, "stuck", 9)
	require.NoError(t, err)
	require.Equal(t, int64(10), revision, "above the caller's floor")
	for id, want := range map[string]int64{"done": 1, "stuck": 10, "behind-a": 11, "behind-b": 12} {
		got, _ := revisionOf(id)
		require.Equal(t, want, got, id)
	}
	_, state := revisionOf("stuck")
	require.Equal(t, "pending", state, "the operation is dispatched again")
	var cursor model.NodeOperationRevision
	require.NoError(t, db.First(&cursor, "node_id = ?", node).Error)
	require.Equal(t, int64(12), cursor.DesiredRevision)

	_, err = RenumberPendingNodeOperations(db, node, "behind-a", 0)
	require.ErrorContains(t, err, "earlier active operation", "an operation behind an active one keeps its place")
	_, err = RenumberPendingNodeOperations(db, node, "done", 0)
	require.ErrorContains(t, err, "cannot be renumbered")
	_, err = RenumberPendingNodeOperations(nil, node, "stuck", 0)
	require.Error(t, err)
	_, err = RenumberPendingNodeOperations(db, node, "stuck", -1)
	require.Error(t, err)
}
