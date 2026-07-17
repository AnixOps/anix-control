package service

import (
	"sync"
	"time"
)

// AgentTask represents a task dispatched to an agent node.
type AgentTask struct {
	ID        uint64    `json:"id"`
	NodeID    uint32    `json:"node_id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"` // pending, running, completed, failed
	Command   string    `json:"command"`
	Action    string    `json:"action"`
	Params    string    `json:"params"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MonitorEntry holds a single monitor data push from an agent.
type MonitorEntry struct {
	NodeID    uint32         `json:"node_id"`
	System    map[string]any `json:"system"`
	Timestamp time.Time      `json:"timestamp"`
}

// TaskStore provides a thread-safe in-memory task queue per node.
type TaskStore struct {
	mu     sync.RWMutex
	tasks  map[uint32][]*AgentTask // nodeID -> pending tasks
	seqNum uint64
}

// NewTaskStore creates an empty TaskStore.
func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[uint32][]*AgentTask),
	}
}

// Enqueue adds a new pending task for the given node.
func (ts *TaskStore) Enqueue(nodeID uint32, taskType, command, action string) *AgentTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.seqNum++
	task := &AgentTask{
		ID:        ts.seqNum,
		NodeID:    nodeID,
		Type:      taskType,
		Status:    "pending",
		Command:   command,
		Action:    action,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ts.tasks[nodeID] = append(ts.tasks[nodeID], task)
	return task
}

// DequeuePending returns up to n pending tasks for the given node and marks them as "running".
func (ts *TaskStore) DequeuePending(nodeID uint32, n int) []*AgentTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	pending := ts.tasks[nodeID]
	if len(pending) == 0 {
		return nil
	}

	if n > len(pending) {
		n = len(pending)
	}

	result := make([]*AgentTask, 0, n)
	remaining := pending[n:]

	for i := 0; i < n; i++ {
		pending[i].Status = "running"
		pending[i].UpdatedAt = time.Now()
		result = append(result, pending[i])
	}

	ts.tasks[nodeID] = remaining
	return result
}

// CompleteTask marks a running task as completed with the given result.
func (ts *TaskStore) CompleteTask(nodeID uint32, taskID uint64, result, errMsg string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, t := range ts.tasks[nodeID] {
		if t.ID == taskID {
			t.Status = "completed"
			t.Result = result
			t.Error = errMsg
			t.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// FailTask marks a task as failed.
func (ts *TaskStore) FailTask(nodeID uint32, taskID uint64, errMsg string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, t := range ts.tasks[nodeID] {
		if t.ID == taskID {
			t.Status = "failed"
			t.Error = errMsg
			t.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// ListTasksByNode returns all tasks (pending + completed) for a node.
func (ts *TaskStore) ListTasksByNode(nodeID uint32) []*AgentTask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	out := make([]*AgentTask, len(ts.tasks[nodeID]))
	copy(out, ts.tasks[nodeID])
	return out
}

// MonitorStore keeps a ring buffer of the last N monitor entries per node.
type MonitorStore struct {
	mu       sync.RWMutex
	buffers  map[uint32]*monitorRing
	capacity int
}

type monitorRing struct {
	buf   []MonitorEntry
	head  int
	count int
}

func newMonitorRing(capacity int) *monitorRing {
	return &monitorRing{
		buf: make([]MonitorEntry, capacity),
	}
}

func (r *monitorRing) push(e MonitorEntry) {
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
}

func (r *monitorRing) entries() []MonitorEntry {
	if r.count == 0 {
		return nil
	}
	out := make([]MonitorEntry, r.count)
	start := (r.head - r.count + len(r.buf)) % len(r.buf)
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}

// NewMonitorStore creates a MonitorStore with the given per-node capacity.
func NewMonitorStore(capacity int) *MonitorStore {
	if capacity <= 0 {
		capacity = 100
	}
	return &MonitorStore{
		buffers:  make(map[uint32]*monitorRing),
		capacity: capacity,
	}
}

// Push adds a monitor entry for the node.
func (ms *MonitorStore) Push(nodeID uint32, system map[string]any, ts time.Time) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ring, ok := ms.buffers[nodeID]
	if !ok {
		ring = newMonitorRing(ms.capacity)
		ms.buffers[nodeID] = ring
	}

	ring.push(MonitorEntry{
		NodeID:    nodeID,
		System:    system,
		Timestamp: ts,
	})
}

// Get returns the last N monitor entries for the node (up to all available).
func (ms *MonitorStore) Get(nodeID uint32, n int) []MonitorEntry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	ring, ok := ms.buffers[nodeID]
	if !ok {
		return nil
	}

	all := ring.entries()
	if n <= 0 || n > len(all) {
		n = len(all)
	}
	if n == 0 {
		return nil
	}
	return all[len(all)-n:]
}

// Latest returns the most recent monitor entry for the node.
func (ms *MonitorStore) Latest(nodeID uint32) *MonitorEntry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	ring, ok := ms.buffers[nodeID]
	if !ok || ring.count == 0 {
		return nil
	}

	idx := (ring.head - 1 + len(ring.buf)) % len(ring.buf)
	entry := ring.buf[idx]
	return &entry
}
