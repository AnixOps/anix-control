package relay

import "time"

// calmBucket is the budget of answers a peer can draw by misbehaving (pings,
// refused or malformed OPENs, stream errors): a burst of calmBurst, refilled at
// calmRate a second. A peer that behaves never notices it; beyond it the
// carrier ends with GoAwayCalm. Not safe for concurrent use: the carrier's
// lock guards it.
type calmBucket struct {
	tokens float64
	at     time.Time
}

func newCalmBucket(now time.Time) calmBucket { return calmBucket{tokens: calmBurst, at: now} }

// take spends one answer and reports whether there was one.
func (b *calmBucket) take(now time.Time) bool {
	b.tokens = min(calmBurst, b.tokens+now.Sub(b.at).Seconds()*calmRate)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
