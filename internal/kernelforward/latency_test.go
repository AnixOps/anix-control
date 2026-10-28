package kernelforward

import (
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func check(routeID string, checkedAt time.Time, rttUs uint32) *forwardv1.UpstreamHealth {
	state := forwardv1.HealthState_HEALTH_STATE_HEALTHY
	if rttUs == 0 {
		state = forwardv1.HealthState_HEALTH_STATE_UNHEALTHY
	}
	return &forwardv1.UpstreamHealth{
		RouteId: routeID, HopIndex: 0, Address: "198.51.100.21", Port: 30000, State: state,
		RttUs: rttUs, CheckedAtUnixMs: checkedAt.UnixMilli(),
	}
}

func (f *fixture) healthReport(observed time.Time, health ...*forwardv1.UpstreamHealth) {
	f.t.Helper()
	_, err := f.service.RecordReport(f.ctx, entry, &forwardv1.NodeForwardReport{
		NodeRef: entry.String(), Generation: 2, Applied: true, Health: health,
	}, observed, observed)
	require.NoError(f.t, err)
}

// The latency history: each check is counted once into its five-minute
// bucket, failures apart from the round trips; future and stale checks are
// ignored, the window is bounded and old buckets are pruned.
func TestLatencyHistory(t *testing.T)         { runLatencyHistory(t, openSQLite(t)) }
func TestPostgresLatencyHistory(t *testing.T) { runLatencyHistory(t, openPostgres(t)) }

func runLatencyHistory(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(0))
	id := route.GetId()

	first := start.Add(time.Minute)
	f.healthReport(first, check(id, first, 20000))
	// The same check again adds nothing.
	f.healthReport(first.Add(10*time.Second), check(id, first, 20000))
	second := first.Add(time.Minute)
	f.healthReport(second, check(id, second, 40000))
	third := second.Add(time.Minute)
	f.healthReport(third, check(id, third, 0))
	// The next bucket.
	fourth := start.Add(6 * time.Minute)
	f.healthReport(fourth, check(id, fourth, 30000))
	// A check dated in the future and one older than the retention.
	f.healthReport(fourth.Add(time.Second), check(id, fourth.Add(time.Hour), 1), check(id, fourth.Add(-8*24*time.Hour), 1))

	f.clock.Advance(10 * time.Minute)
	buckets, truncated, err := f.service.Latency(f.ctx, id, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, buckets, 2)
	b := buckets[0]
	assert.Equal(t, start.UnixMilli(), b.GetBucketStartUnixMs())
	assert.Equal(t, "forward-11", b.GetNodeRef())
	assert.Equal(t, "198.51.100.21", b.GetAddress())
	assert.EqualValues(t, 30000, b.GetPort())
	assert.EqualValues(t, 3, b.GetSamples())
	assert.EqualValues(t, 1, b.GetFailures())
	assert.EqualValues(t, 20000, b.GetRttMinUs())
	assert.EqualValues(t, 30000, b.GetRttAvgUs())
	assert.EqualValues(t, 40000, b.GetRttMaxUs())
	assert.Equal(t, start.Add(5*time.Minute).UnixMilli(), buckets[1].GetBucketStartUnixMs())
	assert.EqualValues(t, 1, buckets[1].GetSamples())
	assert.EqualValues(t, 30000, buckets[1].GetRttAvgUs())

	// The window is bounded, and an unknown route is not found.
	_, _, err = f.service.Latency(f.ctx, id, start.Add(-8*24*time.Hour), start)
	assert.ErrorIs(t, err, ErrInvalidRequest)
	_, _, err = f.service.Latency(f.ctx, "missing", time.Time{}, time.Time{})
	assert.ErrorIs(t, err, ErrNotFound)

	// The maintenance prunes buckets older than the retention.
	f.clock.Advance(LatencyRetention - 7*time.Minute)
	require.NoError(t, f.service.Maintain(f.ctx))
	var rows int64
	require.NoError(t, db.Model(&model.KernelForwardLatency{}).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "the bucket that ended more than seven days ago is pruned")
}
