package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// A randomized end-to-end model: many streams with random sizes, windows,
// frame limits and endings (graceful, half-closed, reset by either side,
// refused) run over one pair of carriers. Whatever the schedule, every byte
// that arrives must be the byte that was sent, in order; graceful streams
// deliver everything; no goroutine may hang; and when the dust settles the
// carriers' credit must add up exactly and a fresh stream must work.

type streamMode uint8

const (
	modeGraceful      streamMode = iota
	modeDialerReset              // the dialler resets after reading cut bytes
	modeAcceptorReset            // the listener resets after reading cut bytes
	modeReject                   // the listener refuses the stream
	modeDialerClose              // the dialler closes the stream after writing cut bytes
	modeAcceptorClose            // the listener closes the stream after reading cut bytes
	modeCount
)

type streamPlan struct {
	up, down int // bytes dialler to listener, listener to dialler
	mode     streamMode
	cut      int
	seed     int64
}

type scenario struct {
	kind       string
	dcfg, acfg Config
	streams    []streamPlan
}

func patByte(seed byte, off int) byte { return byte(off*7) ^ seed }

// verifier checks a received byte stream against the pattern sent.
type verifier struct {
	seed byte
	off  int
}

func (v *verifier) check(b []byte) bool {
	for i, c := range b {
		if c != patByte(v.seed, v.off+i) {
			return false
		}
	}
	v.off += len(b)
	return true
}

// endState is how one direction of a stream ended.
func acceptableEnd(err error) bool {
	var re *StreamResetError
	var rs *ResultError
	return err == nil || err == io.EOF || errors.As(err, &re) || errors.As(err, &rs) ||
		errors.Is(err, ErrStreamClosed) || isTimeout(err) && false
}

func (sc scenario) run(t *testing.T) {
	t.Helper()
	d, a := newPair(t, sc.kind, sc.dcfg, sc.acfg)
	n := len(sc.streams)
	var wg sync.WaitGroup
	wg.Add(2 * n)

	acceptCtx, stopAccepting := context.WithCancel(context.Background())
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			s, err := a.Accept(acceptCtx)
			if err != nil {
				return
			}
			i := int(s.Params().HopIndex)
			if i >= n {
				t.Errorf("accepted a stream with hop index %d", i)
				continue
			}
			go func() {
				defer wg.Done()
				sc.acceptorSide(t, s, i)
			}()
		}
	}()
	for i := range sc.streams {
		go func() {
			defer wg.Done()
			sc.dialerSide(t, d, i)
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * testWait):
		t.Log(d.dump(), a.dump())
		buf := make([]byte, 1<<20)
		buf = buf[:runtime.Stack(buf, true)]
		t.Fatalf("scenario hung: %+v\n%s", sc, buf)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n" + d.dump() + a.dump())
		}
	})
	waitFor(t, "streams to end", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	if d.Err() != nil || a.Err() != nil {
		t.Fatalf("carriers ended: %v / %v", d.Err(), a.Err())
	}
	stopAccepting()
	<-acceptDone
	// Credit adds up once every WINDOW has been delivered.
	waitFor(t, "credit to add up", func() bool { return creditsBalanced(d, a) })
	// And a fresh stream works.
	serveEchoOnce(t, a)
	eqBytes(t, roundTrip(t, d, pattern(50_000, 77)), pattern(50_000, 77))
}

// creditsBalanced reports whether, with nothing in flight and nothing unread,
// each end's view of the other's credit agrees and the whole window is back.
func creditsBalanced(d, a *Carrier) bool {
	type view struct{ send, recv, consumed, window int64 }
	get := func(c *Carrier) view {
		c.mu.Lock()
		defer c.mu.Unlock()
		return view{c.sendCredit, c.recvCredit, c.recvConsumed, int64(c.local.CarrierWindow)}
	}
	dv, av := get(d), get(a)
	return av.recv+av.consumed == av.window && dv.recv+dv.consumed == dv.window &&
		dv.send == av.recv && av.send == dv.recv
}

func serveEchoOnce(t *testing.T, a *Carrier) { serveEcho(t, a) }

func chunks(rng *rand.Rand, total int, f func(n int) bool) {
	for left := total; left > 0; {
		n := min(left, 1+rng.IntN(5000))
		if !f(n) {
			return
		}
		left -= n
	}
}

func (sc scenario) dialerSide(t *testing.T, d *Carrier, i int) {
	plan := sc.streams[i]
	rng := rand.New(rand.NewPCG(uint64(plan.seed), uint64(i)))
	readSize := 1 + rng.IntN(7000)
	s, err := d.Open(OpenParams{Kind: StreamTCP, RouteID: "scenario", HopIndex: uint32(i)})
	if err != nil {
		t.Errorf("stream %d: Open: %v", i, err)
		return
	}
	defer func() { _ = s.Close() }()
	_ = s.SetDeadline(time.Now().Add(testWait))
	if plan.mode == modeReject {
		var re *ResultError
		if err := s.AwaitResult(ctxTimeoutT(t)); !errors.As(err, &re) || re.Code != ResultPaused {
			t.Errorf("stream %d: AwaitResult = %v", i, err)
		}
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // writer
		defer wg.Done()
		sent := 0
		chunks(rng, plan.up, func(n int) bool {
			if plan.mode == modeDialerClose && sent >= plan.cut {
				_ = s.Close()
				return false
			}
			buf := make([]byte, n)
			for j := range buf {
				buf[j] = patByte(byte(i), sent+j)
			}
			w, err := s.Write(buf)
			sent += w
			if err != nil {
				if plan.mode == modeGraceful || !acceptableEnd(err) {
					t.Errorf("stream %d: dialler Write: %v", i, err)
				}
				return false
			}
			return true
		})
		switch plan.mode {
		case modeDialerClose:
			_ = s.Close()
		default:
			if err := s.CloseWrite(); err != nil && plan.mode == modeGraceful {
				t.Errorf("stream %d: CloseWrite: %v", i, err)
			}
		}
	}()
	go func() { // reader
		defer wg.Done()
		v := verifier{seed: byte(i) ^ 0x55}
		buf := make([]byte, readSize)
		for {
			n, err := s.Read(buf)
			if !v.check(buf[:n]) {
				t.Errorf("stream %d: dialler read wrong bytes at %d", i, v.off)
				return
			}
			if plan.mode == modeDialerReset && v.off >= plan.cut {
				_ = s.Reset(ResetPeerReset)
				return
			}
			if err != nil {
				if err == io.EOF && plan.mode == modeGraceful {
					if v.off != plan.down {
						t.Errorf("stream %d: dialler got %d of %d bytes", i, v.off, plan.down)
					}
				} else if plan.mode == modeGraceful || !acceptableEnd(err) {
					t.Errorf("stream %d: dialler Read: %v after %d bytes", i, err, v.off)
				}
				return
			}
		}
	}()
	wg.Wait()
}

func (sc scenario) acceptorSide(t *testing.T, s *Stream, i int) {
	plan := sc.streams[i]
	rng := rand.New(rand.NewPCG(uint64(plan.seed), uint64(i)+1<<20))
	readSize := 1 + rng.IntN(7000)
	answerNow := rng.IntN(2) == 0
	defer func() { _ = s.Close() }()
	_ = s.SetDeadline(time.Now().Add(testWait))
	if plan.mode == modeReject {
		if err := s.Reject(ResultPaused); err != nil {
			t.Errorf("stream %d: Reject: %v", i, err)
		}
		return
	}
	if answerNow {
		_ = s.Accept()
	} // else the first Write answers it
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // reader
		defer wg.Done()
		v := verifier{seed: byte(i)}
		buf := make([]byte, readSize)
		for {
			n, err := s.Read(buf)
			if !v.check(buf[:n]) {
				t.Errorf("stream %d: listener read wrong bytes at %d", i, v.off)
				return
			}
			switch {
			case plan.mode == modeAcceptorReset && v.off >= plan.cut:
				_ = s.Reset(ResetInternal)
				return
			case plan.mode == modeAcceptorClose && v.off >= plan.cut:
				_ = s.Close()
				return
			}
			if err != nil {
				if err == io.EOF && plan.mode == modeGraceful {
					if v.off != plan.up {
						t.Errorf("stream %d: listener got %d of %d bytes", i, v.off, plan.up)
					}
				} else if plan.mode == modeGraceful || !acceptableEnd(err) {
					t.Errorf("stream %d: listener Read: %v after %d bytes", i, err, v.off)
				}
				return
			}
		}
	}()
	go func() { // writer
		defer wg.Done()
		sent := 0
		chunks(rng, plan.down, func(n int) bool {
			buf := make([]byte, n)
			for j := range buf {
				buf[j] = patByte(byte(i)^0x55, sent+j)
			}
			w, err := s.Write(buf)
			sent += w
			if err != nil {
				if plan.mode == modeGraceful || !acceptableEnd(err) {
					t.Errorf("stream %d: listener Write: %v", i, err)
				}
				return false
			}
			return true
		})
		if err := s.CloseWrite(); err != nil && plan.mode == modeGraceful {
			t.Errorf("stream %d: listener CloseWrite: %v", i, err)
		}
	}()
	wg.Wait()
}

func ctxTimeoutT(t *testing.T) context.Context { return ctxTimeout(t) }

// cutFor picks where an aborting stream aborts: within the bytes the aborting
// side reads (or, for a closing dialler, writes). r is 0..65535.
func cutFor(p streamPlan, r int) int {
	limit := p.up
	if p.mode == modeDialerReset {
		limit = p.down
	}
	return limit * r / (1 << 16)
}

func randomScenario(seed uint64) scenario {
	rng := rand.New(rand.NewPCG(seed, 99))
	pick := func(xs ...uint32) uint32 { return xs[rng.IntN(len(xs))] }
	sc := scenario{kind: []string{"pipe", "tcp"}[rng.IntN(2)]}
	sc.dcfg = Config{
		StreamWindow:  pick(4096, 8192, 65536, 0),
		CarrierWindow: pick(8192, 16384, 131072, 0),
		MaxFrame:      pick(1024, 4096, 0),
		SendBuffer:    int(pick(512, 4096, 0)),
	}
	sc.acfg = Config{
		StreamWindow:  pick(4096, 8192, 65536, 0),
		CarrierWindow: pick(8192, 16384, 131072, 0),
		MaxFrame:      pick(1024, 4096, 0),
		SendBuffer:    int(pick(512, 4096, 0)),
		MaxStreams:    64,
		AcceptQueue:   64,
	}
	for range 1 + rng.IntN(16) {
		p := streamPlan{
			up:   rng.IntN(60_000),
			down: rng.IntN(60_000),
			mode: streamMode(rng.IntN(int(modeCount))),
			seed: rng.Int64(),
		}
		if rng.IntN(3) > 0 {
			p.mode = modeGraceful
		}
		p.cut = cutFor(p, rng.IntN(1<<16))
		sc.streams = append(sc.streams, p)
	}
	return sc
}

// RELAY_MODEL_SEEDS raises the number of scenarios for a longer soak.
func TestRandomizedTransfers(t *testing.T) {
	seeds := 40
	if v, err := strconv.Atoi(os.Getenv("RELAY_MODEL_SEEDS")); err == nil && v > 0 {
		seeds = v
	}
	for seed := range uint64(seeds) {
		t.Run("", func(t *testing.T) {
			sc := randomScenario(seed)
			t.Logf("seed %d: %s, %d streams, windows %d/%d vs %d/%d", seed, sc.kind, len(sc.streams),
				sc.dcfg.StreamWindow, sc.dcfg.CarrierWindow, sc.acfg.StreamWindow, sc.acfg.CarrierWindow)
			sc.run(t)
		})
	}
}

// scenarioFromBytes turns fuzz input into a scenario: a few bytes pick the
// configuration, then three bytes per stream.
func scenarioFromBytes(data []byte) (scenario, bool) {
	if len(data) < 7 {
		return scenario{}, false
	}
	pick := func(b byte, xs ...uint32) uint32 { return xs[int(b)%len(xs)] }
	sc := scenario{kind: "pipe"}
	if data[0]&1 == 1 {
		sc.kind = "tcp"
	}
	sc.dcfg = Config{StreamWindow: pick(data[1], 4096, 8192, 0), CarrierWindow: pick(data[2], 8192, 16384, 0), MaxFrame: pick(data[3], 1024, 4096, 0), SendBuffer: int(pick(data[4], 512, 4096, 0))}
	sc.acfg = Config{StreamWindow: pick(data[2], 4096, 8192, 0), CarrierWindow: pick(data[3], 8192, 16384, 0), MaxFrame: pick(data[1], 1024, 4096, 0), SendBuffer: int(pick(data[5], 512, 4096, 0)), MaxStreams: 64, AcceptQueue: 64}
	rest := data[6:]
	for len(rest) >= 4 && len(sc.streams) < 12 {
		p := streamPlan{
			up:   int(rest[0]) * int(rest[0]) / 2,
			down: int(rest[1]) * int(rest[1]) / 2,
			mode: streamMode(rest[2] % byte(modeCount)),
			seed: int64(rest[3]),
		}
		p.cut = cutFor(p, int(rest[3])<<8)
		sc.streams = append(sc.streams, p)
		rest = rest[4:]
	}
	return sc, len(sc.streams) > 0
}

// dump describes a carrier's streams and credit for a failing test.
func (c *Carrier) dump() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := fmt.Sprintf("%s: sendCredit=%d recvCredit=%d recvConsumed=%d active=%d ctrl=%d ring=%d goAway=%v/%v err=%v\n",
		c.role, c.sendCredit, c.recvCredit, c.recvConsumed, c.active, len(c.ctrl), len(c.ring), c.goAwaySent, c.goAwayRecv, c.err)
	for id, s := range c.streams {
		out += fmt.Sprintf("  stream %d: recvLen=%d recvCredit=%d recvConsumed=%d remoteFin=%v sendLen=%d sendCredit=%d finQueued=%v finSent=%v inRing=%v answered=%v closedByApp=%v err=%v\n",
			id, s.recvLen, s.recvCredit, s.recvConsumed, s.remoteFin, s.sendLen, s.sendCredit, s.finQueued, s.finSent, s.inRing, s.answered, s.closedByApp, s.err)
	}
	return out
}
