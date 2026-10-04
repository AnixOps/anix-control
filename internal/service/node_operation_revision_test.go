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
