package gost

// spread answers the upstream indices a weighted round robin visits in one
// cycle over weights (each at least 1): the weights reduced by their
// greatest common divisor and, when they still sum to more than
// maxEntries, scaled down to about maxEntries with at least one entry per
// upstream, then interleaved in smooth weighted round-robin order (each
// step the upstream with the highest running credit, ties to the lower
// index). gost's round and hash selectors ignore weights, so a weighted
// hop lists an upstream once per entry (forward-sdk.md section 6.2). The
// result is deterministic; equal weights give 0, 1, ..., n-1.
func spread(weights []uint32, maxEntries int) []int {
	ws := reduce(weights)
	total := sum(ws)
	if maxEntries >= len(ws) && total > uint64(maxEntries) { // #nosec G115 -- maxEntries >= len(ws) >= 0
		scaled := make([]uint32, len(ws))
		for i, w := range ws {
			scaled[i] = max(1, uint32(uint64(w)*uint64(maxEntries)/total)) // #nosec G115 -- at most maxEntries
		}
		ws = reduce(scaled)
		total = sum(ws)
	}
	out := make([]int, 0, total)
	credit := make([]int64, len(ws))
	for range total {
		best := 0
		for i, w := range ws {
			credit[i] += int64(w)
			if credit[i] > credit[best] {
				best = i
			}
		}
		credit[best] -= int64(total) // #nosec G115 -- total is small
		out = append(out, best)
	}
	return out
}

func sum(ws []uint32) uint64 {
	var t uint64
	for _, w := range ws {
		t += uint64(w)
	}
	return t
}

// reduce divides the weights by their greatest common divisor.
func reduce(weights []uint32) []uint32 {
	g := uint32(0)
	for _, w := range weights {
		g = gcd(g, w)
	}
	out := make([]uint32, len(weights))
	for i, w := range weights {
		if g > 1 {
			w /= g
		}
		out[i] = max(w, 1)
	}
	return out
}

func gcd(a, b uint32) uint32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
