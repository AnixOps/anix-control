package service

import (
	"math"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CT-006: the rate is applied in integer arithmetic, exactly, and a result
// beyond the int64 counters is refused instead of wrapping.
func TestScaleTraffic(t *testing.T) {
	for _, test := range []struct {
		bytes int64
		rate  float64
		want  int64
	}{
		{0, 1.5, 0},
		{10, 1, 10},
		{10, 3, 30},
		{10, 0.1, 1},
		{3, 0.333, 0},
		{1 << 53, 1, 1 << 53},
		{1<<53 + 1, 1, 1<<53 + 1}, // float64 would round this to 1<<53
		{math.MaxInt64, 1, math.MaxInt64},
		{math.MaxInt64 / 2, 2, math.MaxInt64 - 1},
		{1_000_000_007, 1.5, 1_500_000_010},
	} {
		got, err := scaleTraffic(test.bytes, test.rate)
		require.NoError(t, err, "%d x %v", test.bytes, test.rate)
		assert.Equal(t, test.want, got, "%d x %v", test.bytes, test.rate)
	}
	for _, test := range []struct {
		bytes int64
		rate  float64
	}{
		{math.MaxInt64, 1.001},
		{math.MaxInt64, 2},
		{1, math.Inf(1)},
		{1, 1e30},
	} {
		_, err := scaleTraffic(test.bytes, test.rate)
		assert.ErrorIs(t, err, ErrTrafficOverflow, "%d x %v", test.bytes, test.rate)
	}
}

// Totals over several users that pass int64 are refused, never negative; a
// NaN or non-positive rate counts as 1.
func TestTrafficPlanRefusesOverflowingTotals(t *testing.T) {
	_, err := planNodeTrafficReport(model.ServerType("node"), 1, map[uint][2]int64{1: {math.MaxInt64, 0}, 2: {1, 0}}, 1, time.Unix(1_800_000_000, 0))
	assert.ErrorIs(t, err, ErrTrafficOverflow)
	_, err = planNodeTrafficReport(model.ServerType("node"), 1, map[uint][2]int64{1: {0, math.MaxInt64 / 2}}, 3, time.Unix(1_800_000_000, 0))
	assert.ErrorIs(t, err, ErrTrafficOverflow)

	for _, rate := range []float64{math.NaN(), 0, -2} {
		plan, err := planNodeTrafficReport(model.ServerType("node"), 1, map[uint][2]int64{1: {7, 9}}, rate, time.Unix(1_800_000_000, 0))
		require.NoError(t, err)
		assert.Equal(t, [2]int64{7, 9}, [2]int64{plan.totalUpload, plan.totalDownload}, "rate %v", rate)
	}
}
