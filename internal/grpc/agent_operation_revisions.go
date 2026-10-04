package grpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// NodeRevisionStore is the durable per-node allocator of Agent operation
// revisions (v3_kernel_node_operation_revision). Durable plugin operations
// take their revisions from it when they are created; with a store attached,
// AgentControlManager allocates the revisions of one-off stream operations
// from it too, so one node's revisions are strictly increasing across both
// kinds. The Agent supersedes any operation at or below its observed
// revision, and the manager refuses a revision that is not newer than the
// last one sent.
type NodeRevisionStore interface {
	// AllocateRevision reserves the next revision of nodeID, above floor.
	// ok is false when no durable store is available; the manager then
	// falls back to its in-memory counter.
	AllocateRevision(ctx context.Context, nodeID uint32, floor uint64) (revision uint64, ok bool, err error)
	// RaiseRevision raises the stored revision of nodeID to at least
	// revision (the revision a connecting Agent reports).
	RaiseRevision(ctx context.Context, nodeID uint32, revision uint64) error
}

// databaseRevisionStore allocates from the cursor table of the database
// that db returns when called; a nil database means no durable store.
type databaseRevisionStore struct {
	db func() *gorm.DB
}

// NewDatabaseRevisionStore returns the NodeRevisionStore over the database
// that db resolves at each call.
func NewDatabaseRevisionStore(db func() *gorm.DB) NodeRevisionStore {
	return databaseRevisionStore{db: db}
}

func (s databaseRevisionStore) AllocateRevision(ctx context.Context, nodeID uint32, floor uint64) (uint64, bool, error) {
	db := s.db()
	if db == nil {
		return 0, false, nil
	}
	storedFloor, ok := kernelObservedRevision(floor)
	if !ok {
		return 0, true, fmt.Errorf("revision %d exceeds the stored revision range", floor)
	}
	revision, err := service.AllocateNodeOperationRevision(db.WithContext(ctx), uint(nodeID), storedFloor)
	if err != nil {
		return 0, true, fmt.Errorf("allocate node %d operation revision: %w", nodeID, err)
	}
	allocated, err := kernelAgentRevision(revision)
	if err != nil {
		return 0, true, err
	}
	return allocated, true, nil
}

func (s databaseRevisionStore) RaiseRevision(ctx context.Context, nodeID uint32, revision uint64) error {
	db := s.db()
	if db == nil || revision == 0 {
		return nil
	}
	stored, ok := kernelObservedRevision(revision)
	if !ok {
		return fmt.Errorf("revision %d exceeds the stored revision range", revision)
	}
	if err := service.RaiseNodeOperationRevision(db.WithContext(ctx), uint(nodeID), stored); err != nil {
		return fmt.Errorf("raise node %d operation revision: %w", nodeID, err)
	}
	return nil
}

// UseRevisionStore attaches the durable revision allocator. Only the proxy
// node manager takes one: forward node ids overlap proxy node ids and have
// no cursor of their own.
func (m *AgentControlManager) UseRevisionStore(store NodeRevisionStore) {
	m.mu.Lock()
	m.revisionStore = store
	m.mu.Unlock()
}

func (m *AgentControlManager) currentRevisionStore() NodeRevisionStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revisionStore
}

// allocationLock serializes revision allocation and registration for one
// node, so two one-off operations reach the in-memory guard in the order
// their revisions were allocated.
func (m *AgentControlManager) allocationLock(nodeID uint32) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allocLocks == nil {
		m.allocLocks = make(map[uint32]*sync.Mutex)
	}
	lock := m.allocLocks[nodeID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.allocLocks[nodeID] = lock
	}
	return lock
}

// raiseStoredRevision raises the durable cursor to the revision a
// connecting Agent reports. Without a store it does nothing.
func (m *AgentControlManager) raiseStoredRevision(ctx context.Context, nodeID uint32, revision uint64) error {
	store := m.currentRevisionStore()
	if store == nil {
		return nil
	}
	return store.RaiseRevision(ctx, nodeID, revision)
}
