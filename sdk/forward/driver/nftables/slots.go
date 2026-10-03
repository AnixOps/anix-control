package nftables

// Balancing maps. Every hop has one map per address family from a slot
// number (0 to Slots-1) to an upstream's address and port. The slot comes
// from numgen inc (round robin, failover), numgen random (random, least
// connections) or jhash of the client address (IP hash), always modulo
// Slots, so the rule never changes when the upstreams in rotation or their
// weights do: SetUpstreams (F2c) rewrites the map elements only.

// slotRun is a run of consecutive slots lo..hi that go to upstream idx.
type slotRun struct {
	lo, hi uint32
	idx    int
}

// slotCounts distributes n slots over the weights in proportion, every
// upstream getting at least one (callers guarantee len(weights) <= n).
// Remainders go to the largest fractional parts, ties to the lower index;
// the minimum of one is paid for by the largest shares, ties from the
// higher index. The result is deterministic and sums to n.
func slotCounts(weights []uint32, n uint32) []uint32 {
	var total uint64
	for _, w := range weights {
		total += uint64(w)
	}
	counts := make([]uint32, len(weights))
	rems := make([]uint64, len(weights))
	var sum uint32
	for i, w := range weights {
		exact := uint64(n) * uint64(w)
		counts[i] = uint32(exact / total) // #nosec G115 -- at most n
		rems[i] = exact % total
		sum += counts[i]
	}
	for sum < n {
		best := -1
		for i := range counts {
			if best < 0 || rems[i] > rems[best] {
				best = i
			}
		}
		counts[best]++
		rems[best] = 0
		sum++
		// Every remainder is used at most once; sum can fall short by at
		// most len(weights)-1, so best never repeats before they run out.
	}
	for i := range counts {
		if counts[i] == 0 {
			counts[i] = 1
			sum++
		}
	}
	for sum > n {
		big := -1
		for i := len(counts) - 1; i >= 0; i-- {
			if counts[i] > 1 && (big < 0 || counts[i] > counts[big]) {
				big = i
			}
		}
		counts[big]--
		sum--
	}
	return counts
}

// contiguousRuns lays the slots out as one run per upstream, in order:
// for random choice and hashing, where only the share matters.
func contiguousRuns(counts []uint32) []slotRun {
	var runs []slotRun
	var next uint32
	for i, c := range counts {
		runs = append(runs, slotRun{lo: next, hi: next + c - 1, idx: i})
		next += c
	}
	return runs
}

// interleavedRuns lays the slots out in smooth weighted round-robin order,
// so numgen inc spreads consecutive connections over the upstreams instead
// of sending a block of them to each in turn. Neighbouring slots of the
// same upstream are merged.
func interleavedRuns(counts []uint32, n uint32) []slotRun {
	cur := make([]int64, len(counts))
	var runs []slotRun
	for slot := range n {
		best := 0
		for i, c := range counts {
			cur[i] += int64(c)
			if cur[i] > cur[best] {
				best = i
			}
		}
		cur[best] -= int64(n)
		if k := len(runs); k > 0 && runs[k-1].idx == best && runs[k-1].hi == slot-1 {
			runs[k-1].hi = slot
			continue
		}
		runs = append(runs, slotRun{lo: slot, hi: slot, idx: best})
	}
	return runs
}

// balanceRuns answers the map elements of one family's upstreams (sorted by
// priority, address and port) under the strategy, and which upstreams the
// map holds: failover only holds the best priority's upstreams.
func balanceRuns(strategy interleaving, ups []upstream, n uint32) ([]slotRun, []upstream) {
	if strategy.primaryOnly {
		k := 1
		for k < len(ups) && ups[k].priority == ups[0].priority {
			k++
		}
		ups = ups[:k]
	}
	weights := make([]uint32, len(ups))
	for i, u := range ups {
		weights[i] = u.weight
	}
	counts := slotCounts(weights, n)
	if strategy.interleave {
		return interleavedRuns(counts, n), ups
	}
	return contiguousRuns(counts), ups
}

// interleaving is how a strategy uses its map.
type interleaving struct {
	interleave  bool // round-robin order rather than one run per upstream
	primaryOnly bool // failover: only the best priority's upstreams
}
