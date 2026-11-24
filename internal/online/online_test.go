package online

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func clock(t *testing.T) *time.Time {
	t.Helper()
	Reset()
	current := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	now = func() time.Time { return current }
	t.Cleanup(func() { now = time.Now; Reset() })
	return &current
}

// A node that stops reporting drops out TTL after its last set (CT-001).
func TestSetsExpire(t *testing.T) {
	current := clock(t)
	assert.True(t, Replace(1, time.Time{}, map[uint][]string{7: {"192.0.2.1", "192.0.2.1", "192.0.2.2"}}))
	assert.Equal(t, map[uint64]int{7: 2}, Counts(), "distinct IPs")

	*current = current.Add(TTL - time.Second)
	assert.Equal(t, map[uint64]int{7: 2}, Counts())
	*current = current.Add(time.Second)
	assert.Empty(t, Counts())

	assert.True(t, Replace(2, time.Time{}, nil))
	mu.RLock()
	assert.NotContains(t, nodes, uint(1), "expired sets are dropped from memory")
	mu.RUnlock()
}

// Counts add up per user across nodes and never mix users (CT-002).
func TestCountsPerUserAcrossNodes(t *testing.T) {
	clock(t)
	Replace(1, time.Time{}, map[uint][]string{7: {"192.0.2.1"}, 8: {"192.0.2.9"}})
	Replace(2, time.Time{}, map[uint][]string{7: {"198.51.100.1"}})
	Replace(3, time.Time{}, map[uint][]string{7: {"203.0.113.1"}, 9: {}})
	assert.Equal(t, map[uint64]int{7: 3, 8: 1}, Counts())
}

// A replayed report older than the node's current set changes nothing; a
// newer one replaces the whole set (CT-008).
func TestOlderWindowNeverReplacesNewer(t *testing.T) {
	current := clock(t)
	live := current.Add(-time.Second)
	assert.True(t, Replace(1, live, map[uint][]string{7: {"192.0.2.1"}}))
	assert.False(t, Replace(1, live.Add(-time.Hour), map[uint][]string{8: {"192.0.2.8"}}))
	assert.Equal(t, map[uint64]int{7: 1}, Counts())

	assert.True(t, Replace(1, live.Add(time.Minute), map[uint][]string{9: {"192.0.2.9"}}))
	assert.Equal(t, map[uint64]int{9: 1}, Counts(), "replaced as a whole, not merged")

	// Once the current set has expired, any report starts afresh.
	*current = current.Add(TTL)
	assert.True(t, Replace(1, live.Add(-time.Hour), map[uint][]string{8: {"192.0.2.8"}}))
	assert.Equal(t, map[uint64]int{8: 1}, Counts())
}
