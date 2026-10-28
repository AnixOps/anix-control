package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A diagnosis starts no probe after its context ended, and waits for those
// it started.
func TestDiagnosisProbesStopStartingWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var ran []int
	err := DiagnosisProbes{Concurrency: 1}.each(ctx, 10, func(i int) {
		mu.Lock()
		ran = append(ran, i)
		mu.Unlock()
		if i == 2 {
			cancel()
		}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []int{0, 1, 2}, ran)

	require.NoError(t, DiagnosisProbes{}.each(context.Background(), 0, func(int) { t.Fatal("no probe to run") }))
}

// A diagnosis runs at most Concurrency probes at once, and every probe when
// its context does not end.
func TestDiagnosisProbesBoundTheirConcurrency(t *testing.T) {
	for _, limit := range []int{1, 4, 0} {
		var mu sync.Mutex
		inFlight, most, total := 0, 0, 0
		require.NoError(t, DiagnosisProbes{Concurrency: limit}.each(context.Background(), 40, func(int) {
			mu.Lock()
			inFlight++
			total++
			most = max(most, inFlight)
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
		}))
		want := limit
		if want == 0 {
			want = DefaultDiagnosisConcurrency
		}
		require.Equal(t, 40, total)
		require.LessOrEqual(t, most, want, "limit %d", limit)
	}
}
