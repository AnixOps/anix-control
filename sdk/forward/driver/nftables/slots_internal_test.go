package nftables

import (
	"math/rand/v2"
	"testing"
)

// TestSlotCounts: the counts sum to n, give every upstream at least one
// slot and stay within one slot of the exact share (or at 1 when the share
// is below one slot).
func TestSlotCounts(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- deterministic test data
	for range 2000 {
		n := uint32(1 + r.IntN(300))
		k := 1 + r.IntN(int(min(n, 80)))
		weights := make([]uint32, k)
		var total float64
		for i := range weights {
			weights[i] = uint32(1 + r.IntN(1000))
			total += float64(weights[i])
		}
		counts := slotCounts(weights, n)
		var sum uint32
		for i, c := range counts {
			sum += c
			if c < 1 {
				t.Fatalf("weights %v n %d: upstream %d has no slot", weights, n, i)
			}
			exact := float64(n) * float64(weights[i]) / total
			// The minimum of one slot can push others down by up to k-1.
			if float64(c) > exact+1 || float64(c) < exact-float64(k) {
				t.Fatalf("weights %v n %d: upstream %d has %d slots, exact share %.2f", weights, n, i, c, exact)
			}
		}
		if sum != n {
			t.Fatalf("weights %v n %d: %d slots", weights, n, sum)
		}
		for _, runs := range [][]slotRun{contiguousRuns(counts), interleavedRuns(counts, n)} {
			got := make([]uint32, k)
			var next uint32
			for _, run := range runs {
				if run.lo != next || run.hi < run.lo {
					t.Fatalf("runs %v do not cover 0..%d in order", runs, n-1)
				}
				got[run.idx] += run.hi - run.lo + 1
				next = run.hi + 1
			}
			if next != n {
				t.Fatalf("runs cover 0..%d, want 0..%d", next-1, n-1)
			}
			for i := range got {
				if got[i] != counts[i] {
					t.Fatalf("runs give upstream %d %d slots, want %d", i, got[i], counts[i])
				}
			}
		}
	}
}

// TestInterleavedSpreads: with equal weights, round robin alternates.
func TestInterleavedSpreads(t *testing.T) {
	runs := interleavedRuns(slotCounts([]uint32{1, 1, 1}, 6), 6)
	want := []int{0, 1, 2, 0, 1, 2}
	if len(runs) != 6 {
		t.Fatalf("runs %v", runs)
	}
	for i, r := range runs {
		if r.idx != want[i] {
			t.Fatalf("runs %v, want order %v", runs, want)
		}
	}
}

// TestFailoverHoldsBestPriority: failover maps only the best priority's
// upstreams, spread by weight.
func TestFailoverHoldsBestPriority(t *testing.T) {
	ups := []upstream{{priority: 0, weight: 1}, {priority: 0, weight: 3}, {priority: 5, weight: 1}}
	runs, held := balanceRuns(strategyLayout(5 /* FAILOVER */), ups, 8)
	if len(held) != 2 {
		t.Fatalf("held %v", held)
	}
	var n [2]uint32
	for _, r := range runs {
		n[r.idx] += r.hi - r.lo + 1
	}
	if n != [2]uint32{2, 6} {
		t.Fatalf("slots %v, want [2 6]", n)
	}
}
