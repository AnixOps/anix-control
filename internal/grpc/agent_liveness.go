package grpc

import (
	"sync"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
)

// AgentLivenessWriteInterval is the least time between two writes of one
// node's liveness (last_check_at and the node status) from its Agent Control
// stream. An agent heartbeats every 20 seconds and reports traffic and logs
// on top, each of them used to write the row; now the first one of an
// interval does, and the rest only update the stream's memory. Everything
// that reads the column asks whether a node was seen in the last 5 minutes
// (stats, subscriptions, the node list), so a write at most a minute old is
// as good as the latest one. A stream's Hello always writes: a node that
// connects is seen at once.
const AgentLivenessWriteInterval = 60 * time.Second

// livenessThrottle remembers when each node's liveness was last written, so
// touches within AgentLivenessWriteInterval do not reach the database. It is
// per server (a test or a second listener starts afresh); the zero value
// and nil write every touch.
type livenessThrottle struct {
	mu       sync.Mutex
	interval time.Duration
	now      func() time.Time
	last     map[agentcontrol.AgentNode]time.Time
}

// livenessMemoryLimit bounds the throttle's memory: past it, entries older
// than the interval are dropped, which only costs the node one more write.
const livenessMemoryLimit = 100_000

func newLivenessThrottle(interval time.Duration) *livenessThrottle {
	return &livenessThrottle{interval: interval, now: time.Now, last: map[agentcontrol.AgentNode]time.Time{}}
}

// due reports whether node's liveness is to be written now, and when it is,
// marks it written: the caller writes, and calls forget if the write fails.
func (t *livenessThrottle) due(node agentcontrol.AgentNode) bool {
	if t == nil || t.interval <= 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if last, ok := t.last[node]; ok && now.Sub(last) < t.interval && !now.Before(last) {
		return false
	}
	if len(t.last) >= livenessMemoryLimit {
		for other, at := range t.last {
			if now.Sub(at) >= t.interval {
				delete(t.last, other)
			}
		}
	}
	t.last[node] = now
	return true
}

// written marks node's liveness as written now, by a write that was not a
// touch (a status report sets last_check_at as well).
func (t *livenessThrottle) written(node agentcontrol.AgentNode) {
	if t == nil || t.interval <= 0 {
		return
	}
	t.mu.Lock()
	t.last[node] = t.now()
	t.mu.Unlock()
}

// forget drops node's mark, so the next touch writes: after a write failed.
func (t *livenessThrottle) forget(node agentcontrol.AgentNode) {
	if t == nil || t.interval <= 0 {
		return
	}
	t.mu.Lock()
	delete(t.last, node)
	t.mu.Unlock()
}
