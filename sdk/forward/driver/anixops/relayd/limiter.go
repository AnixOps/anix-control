package relayd

import (
	"sync"
	"time"
)

// bucket is a token bucket for a hop's bandwidth in one direction: rate bytes
// per second, burst of a tenth of a second (at least 64 KiB). take never
// refuses: it answers how long the caller must wait before it may move the
// bytes, which may leave the balance negative, so one copy buffer larger than
// the burst still passes (more slowly than the rate allows, never faster).
type bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func newBucket(bps uint64, now func() time.Time) *bucket {
	b := &bucket{now: now}
	b.setRate(bps)
	b.last = now()
	b.tokens = b.burst
	return b
}

// setRate changes the rate (bytes per second as bits per second divided by
// eight); 0 means unlimited.
func (b *bucket) setRate(bps uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rate = float64(bps) / 8
	b.burst = max(b.rate/10, 64*1024)
	b.tokens = min(b.tokens, b.burst)
}

func (b *bucket) take(n int) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rate <= 0 {
		return 0
	}
	now := b.now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	b.tokens -= float64(n)
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(-b.tokens / b.rate * float64(time.Second))
}

// wait takes n bytes and sleeps as long as the rate needs, or until done.
func (b *bucket) wait(done <-chan struct{}, n int) {
	d := b.take(n)
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-done:
	}
}
