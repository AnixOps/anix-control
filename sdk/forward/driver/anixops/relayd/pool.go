package relayd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay"
	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// Carrier pool bounds (anixops-protocol.md section 4.5: a small pool per
// upstream, so one stalled stream cannot hold every stream of a hop).
const (
	// maxCarriers is the carriers a pool keeps to one upstream.
	maxCarriers = 4
	// perCarrier is the streams a carrier takes before the pool dials another
	// (while it is below maxCarriers).
	perCarrier = 256
	// dialTimeout bounds one carrier dial, TLS or QUIC probe included.
	dialTimeout = 10 * time.Second
	// retireDrain is how long a retired carrier lets its streams finish.
	retireDrain = 30 * time.Second
)

// pool is the carriers of one upstream: every connection through the upstream
// is one stream on one of them. A Selector makes them with the link's carrier
// choice (AUTO's QUIC probe and fallback included).
type pool struct {
	sel   *relay.Selector
	creds *link.Credentials // nil on a plaintext link

	mu       sync.Mutex
	carriers []relay.Carrier
	dialing  chan struct{}
	closed   bool

	dials        atomic.Uint64
	dialFailures atomic.Uint64
}

func newPool(sel *relay.Selector, creds *link.Credentials) *pool {
	return &pool{sel: sel, creds: creds}
}

var errPoolClosed = errors.New("relayd: upstream retired")

// open opens a stream with params on a carrier, dialling one when none has
// room. A carrier that cannot take the stream (GOAWAY, stream limit, ended) is
// dropped from the pool and the stream goes to another.
func (p *pool) open(ctx context.Context, params relay.OpenParams) (relay.Stream, error) {
	var last error
	for range 3 {
		c, err := p.carrier(ctx)
		if err != nil {
			return nil, err
		}
		s, err := c.Open(params)
		if err == nil {
			return s, nil
		}
		last = err
		if !errors.Is(err, relay.ErrGoAway) && !errors.Is(err, relay.ErrStreamLimit) &&
			!errors.Is(err, relay.ErrIDsExhausted) && !errors.Is(err, relay.ErrCarrierClosed) {
			return nil, err
		}
		p.drop(c)
	}
	return nil, last
}

// usable reports whether c may take new streams.
func usable(c relay.Carrier) bool {
	select {
	case <-c.Done():
		return false
	case <-c.Draining():
		return false
	default:
		return true
	}
}

// carrier answers a carrier with room for a stream.
func (p *pool) carrier(ctx context.Context) (relay.Carrier, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, errPoolClosed
		}
		keep := p.carriers[:0]
		var best relay.Carrier
		for _, c := range p.carriers {
			if !usable(c) {
				continue
			}
			keep = append(keep, c)
			if best == nil || c.ActiveStreams() < best.ActiveStreams() {
				best = c
			}
		}
		p.carriers = keep
		if best != nil && (best.ActiveStreams() < perCarrier || len(keep) >= maxCarriers) {
			p.mu.Unlock()
			return best, nil
		}
		if p.dialing != nil {
			if best != nil {
				p.mu.Unlock()
				return best, nil
			}
			wait := p.dialing
			p.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		p.dialing = make(chan struct{})
		p.mu.Unlock()

		dctx, cancel := context.WithTimeout(ctx, dialTimeout)
		c, err := p.sel.Dial(dctx)
		cancel()
		p.dials.Add(1)

		p.mu.Lock()
		close(p.dialing)
		p.dialing = nil
		if err != nil {
			p.dialFailures.Add(1)
			p.mu.Unlock()
			if best != nil {
				return best, nil
			}
			return nil, err
		}
		if p.closed {
			p.mu.Unlock()
			_ = c.Close()
			return nil, errPoolClosed
		}
		p.carriers = append(p.carriers, c)
		p.mu.Unlock()
		relay.WatchCredentials(p.creds, c)
		return c, nil
	}
}

// drop takes c out of the pool and retires it: it ends when its streams do.
func (p *pool) drop(c relay.Carrier) {
	p.mu.Lock()
	for i, x := range p.carriers {
		if x == c {
			p.carriers = append(p.carriers[:i], p.carriers[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
	go func() { _ = c.GoAway(relay.GoAwayNoError, retireDrain) }()
}

// retire stops the pool taking new streams and lets its carriers finish.
func (p *pool) retire() {
	p.mu.Lock()
	p.closed = true
	cs := p.carriers
	p.carriers = nil
	p.mu.Unlock()
	for _, c := range cs {
		go func() { _ = c.GoAway(relay.GoAwayNoError, retireDrain) }()
	}
}

// close ends every carrier at once (the relay stops, L3).
func (p *pool) close() {
	p.mu.Lock()
	p.closed = true
	cs := p.carriers
	p.carriers = nil
	p.mu.Unlock()
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.CloseWithReason(relay.GoAwayShutdown)
		}()
	}
	wg.Wait()
}
