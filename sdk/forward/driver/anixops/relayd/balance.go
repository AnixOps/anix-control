package relayd

import (
	"hash/fnv"
	"math/rand/v2"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

// upstream is one place a hop sends traffic, with the state the relay keeps
// about it: connections in flight and the breaker. An upstream object lives
// as long as its rendered definition does, across applies that change other
// things.
type upstream struct {
	def relayctl.Upstream
	id  string // address:port
	// nextHop is the hop index the next node serves (OPEN names it).
	nextHop uint32
	pool    *pool // AnixOps egress only

	active atomic.Int64

	mu        sync.Mutex
	fails     uint32
	openUntil time.Time
}

// same reports whether the upstream's transport is unchanged, so its state
// (connections, breaker, carriers) carries over an apply; weight and priority
// are hot and live in the rotation's entries.
func (u *upstream) same(d relayctl.Upstream, nextHop uint32) bool {
	a, b := u.def, d
	a.Weight, a.Priority, b.Weight, b.Priority = 0, 0, 0, 0
	return a == b && u.nextHop == nextHop
}

// open reports whether the breaker is open at now.
func (u *upstream) open(now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return now.Before(u.openUntil)
}

// failure records a failed dial or refused stream: after maxFails in a row the
// breaker opens for openFor.
func (u *upstream) failure(now time.Time, maxFails uint32, openFor time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.fails++
	if u.fails >= maxFails {
		u.openUntil = now.Add(openFor)
	}
}

// success clears the failures.
func (u *upstream) success() {
	u.mu.Lock()
	u.fails = 0
	u.openUntil = time.Time{}
	u.mu.Unlock()
}

func (u *upstream) health() (fails uint32, openUntil time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.fails, u.openUntil
}

// entry is an upstream in rotation with the weight to balance by and its
// priority (FAILOVER: lower first).
type entry struct {
	up       *upstream
	weight   uint32
	priority uint32
}

// rotation is the upstreams a hop balances over now. It is immutable; the
// smooth round-robin state lives next to it under its own lock.
type rotation struct {
	entries []entry

	mu  sync.Mutex
	cur map[*upstream]int64
}

func newRotation(entries []entry) *rotation {
	return &rotation{entries: entries, cur: make(map[*upstream]int64, len(entries))}
}

// pick chooses one upstream among the rotation's, not in skip, by the
// strategy; upstreams whose breaker is open are passed over unless none else
// is left. client is the client's address (IP_HASH); it may be invalid.
func (r *rotation) pick(strategy string, client netip.Addr, skip map[*upstream]bool, now time.Time) *upstream {
	var live, all []entry
	for _, e := range r.entries {
		if skip[e.up] {
			continue
		}
		all = append(all, e)
		if !e.up.open(now) {
			live = append(live, e)
		}
	}
	cands := live
	if len(cands) == 0 {
		cands = all
	}
	switch len(cands) {
	case 0:
		return nil
	case 1:
		return cands[0].up
	}
	switch strategy {
	case relayctl.BalanceFailover:
		best := cands[0]
		for _, e := range cands[1:] {
			if e.priority < best.priority {
				best = e
			}
		}
		return best.up
	case relayctl.BalanceRandom:
		return weightedAt(cands, rand.Uint64N(totalWeight(cands))) // #nosec G404 -- a load-balancing choice, not a secret
	case relayctl.BalanceIPHash:
		if !client.IsValid() {
			return weightedAt(cands, rand.Uint64N(totalWeight(cands))) // #nosec G404 -- a load-balancing choice, not a secret
		}
		h := fnv.New64a()
		b := client.Unmap().AsSlice()
		_, _ = h.Write(b)
		return weightedAt(cands, h.Sum64()%totalWeight(cands))
	case relayctl.BalanceLeastConn:
		best := cands[0]
		bestLoad := load(best)
		for _, e := range cands[1:] {
			// load_e/weight_e < load_best/weight_best, cross-multiplied.
			if l := load(e); l*int64(best.weight) < bestLoad*int64(e.weight) {
				best, bestLoad = e, l
			}
		}
		return best.up
	}
	return r.smooth(cands)
}

func load(e entry) int64 { return e.up.active.Load() }

func totalWeight(es []entry) uint64 {
	var t uint64
	for _, e := range es {
		t += uint64(e.weight)
	}
	return max(t, 1)
}

func weightedAt(es []entry, at uint64) *upstream {
	for _, e := range es {
		if at < uint64(e.weight) {
			return e.up
		}
		at -= uint64(e.weight)
	}
	return es[len(es)-1].up
}

// smooth is the smooth weighted round-robin: every pick adds each candidate's
// weight to its running value, takes the largest and subtracts the total, so
// the sequence spreads each upstream's share evenly.
func (r *rotation) smooth(cands []entry) *upstream {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	var best *entry
	for i := range cands {
		e := &cands[i]
		r.cur[e.up] += int64(e.weight)
		total += int64(e.weight)
		if best == nil || r.cur[e.up] > r.cur[best.up] {
			best = e
		}
	}
	r.cur[best.up] -= total
	return best.up
}
