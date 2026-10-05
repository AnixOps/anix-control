package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// A randomized end-to-end model of the QUIC carrier: many streams with random
// sizes and endings (graceful, half-closed, reset or closed by either side,
// refused) and UDP associations run over one carrier, with random flow control
// windows and limits. Whatever the schedule, every byte that arrives is the byte
// that was sent, in order; graceful streams deliver everything; no operation
// hangs; and when the dust settles no stream is left on either end and a
// fresh stream works.

type qmode uint8

const (
	qGraceful qmode = iota
	qDiallerReset
	qAcceptorReset
	qReject
	qDiallerClose
	qAcceptorClose
	qModes
)

type qplan struct {
	mode     qmode
	up, down int
	cut      int
	seed     int64
}

func (p qplan) routeID() string {
	return fmt.Sprintf("m%du%dd%dc%ds%d", p.mode, p.up, p.down, p.cut, p.seed)
}

func parseQPlan(id string) (p qplan) {
	if _, err := fmt.Sscanf(id, "m%du%dd%dc%ds%d", &p.mode, &p.up, &p.down, &p.cut, &p.seed); err != nil {
		panic(err)
	}
	return p
}

// qverify checks bytes against the pattern of a seed.
type qverify struct {
	seed byte
	off  int
}

func (v *qverify) check(b []byte) bool {
	for i, c := range b {
		if c != byte((v.off+i)*7)^v.seed {
			return false
		}
	}
	v.off += len(b)
	return true
}

// readVerified reads s until it fails, or max bytes (when max is not negative),
// checking the pattern; it returns the bytes read and the error that ended the
// reads.
func readVerified(s Stream, seed byte, max int) (int, error) {
	v := qverify{seed: seed}
	buf := make([]byte, 4096)
	for {
		want := len(buf)
		if max >= 0 {
			if max-v.off <= 0 {
				return v.off, nil
			}
			want = min(want, max-v.off)
		}
		n, err := s.Read(buf[:want])
		if !v.check(buf[:n]) {
			return v.off, errors.New("the bytes are not the bytes that were sent")
		}
		if err != nil {
			return v.off, err
		}
	}
}

func writePattern(s Stream, seed byte, n int) error {
	buf := make([]byte, 4096)
	for off := 0; off < n; {
		m := min(len(buf), n-off)
		for i := range m {
			buf[i] = byte((off+i)*7) ^ seed
		}
		if _, err := s.Write(buf[:m]); err != nil {
			return err
		}
		off += m
	}
	return nil
}

// qAcceptable says whether err is a legitimate way for a stream to end.
func qAcceptable(err error) bool {
	var re *StreamResetError
	var rs *ResultError
	return err == nil || err == io.EOF || errors.As(err, &re) || errors.As(err, &rs) || errors.Is(err, ErrStreamClosed) || errors.Is(err, ErrRefused)
}

func runQUICModel(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, seed*7919+1))
	pick := func(of ...int) int { return of[rng.IntN(len(of))] }
	cfg := QUICConfig{Config: Config{
		StreamWindow: uint32(pick(4096, 16<<10, 256<<10)), CarrierWindow: uint32(pick(16<<10, 64<<10, 1<<20)),
		MaxStreams: uint32(pick(8, 64, 1024)), AcceptQueue: pick(8, 128), SendBuffer: pick(2048, 64<<10),
		ResultTimeout: -1, // slow schedules are not stuck carriers
	}, DisableDatagrams: rng.IntN(2) == 0}
	cfg.StreamWindowCeiling, cfg.CarrierWindowCeiling = cfg.StreamWindow, cfg.CarrierWindow
	e := newQUICEnv(t, cfg)
	d, a := e.pair(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n := 10 + rng.IntN(30)
	plans := make([]qplan, n)
	for i := range plans {
		plans[i] = qplan{
			mode: qmode(rng.IntN(int(qModes))), up: pick(0, 1, 1000, 50_000, 300_000), down: pick(0, 1, 1000, 50_000, 300_000),
			seed: rng.Int64N(250),
		}
		if rng.IntN(3) != 0 {
			plans[i].mode = qGraceful
		}
		plans[i].cut = rng.IntN(1 + max(plans[i].up, plans[i].down))
	}

	// the listening hop
	var accepted sync.WaitGroup
	loopCtx, stopLoop := context.WithCancel(ctx)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		for {
			s, err := a.Accept(loopCtx)
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() {
				defer accepted.Done()
				_ = s.SetDeadline(time.Now().Add(testWait * 3))
				p := parseQPlan(s.Params().RouteID)
				actErr := make(chan error, 1)
				go func() { actErr <- acceptorSide(s, p) }()
				if err := <-actErr; err != nil {
					t.Errorf("listener side %+v: %v", p, err)
				}
			}()
		}
	}()

	// the dialling hop
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, p := range plans {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var s Stream
			var err error
			for {
				s, err = d.Open(OpenParams{Kind: StreamTCP, RouteID: p.routeID(), HopIndex: 1})
				if !errors.Is(err, ErrStreamLimit) && !errors.Is(err, ErrRefused) {
					break
				}
				select {
				case <-ctx.Done():
					t.Errorf("never got a stream slot: %v\n%s%s", err, d.dump(), a.dump())
					return
				case <-time.After(time.Millisecond):
				}
			}
			if err != nil {
				t.Errorf("Open: %v", err)
				return
			}
			_ = s.SetDeadline(time.Now().Add(testWait * 3))
			if err := dialerSide(s, p); err != nil {
				t.Errorf("dialler side %+v: %v", p, err)
			}
		}()
	}
	wg.Wait()
	stopLoop()
	<-loopDone
	accepted.Wait()
	waitFor(t, "every stream to end on both sides", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })

	// a few UDP associations: datagrams that arrive are datagrams that were sent
	for i := range 3 {
		s, in := udpPair(t, d, a)
		sent := map[string]bool{}
		for j := range 20 {
			p := pattern(1+rng.IntN(2000), byte(i*20+j))
			sent[string(p)] = true
			_ = s.WriteDatagram(p)
		}
		_ = in.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		got := 0
		for {
			g, err := in.ReadDatagram()
			if err != nil {
				break
			}
			if !sent[string(g)] {
				t.Fatalf("association %d received a datagram nobody sent", i)
			}
			got++
		}
		// over the stream they are reliable, unless the send buffer is smaller
		// than the burst, when what does not fit is dropped like UDP's
		if cfg.DisableDatagrams && cfg.SendBuffer >= 64<<10 && got != 20 {
			t.Fatalf("association %d: %d of 20 datagrams over the stream", i, got)
		}
		_ = s.Close()
		_ = in.Close()
	}

	// the dust settles
	waitFor(t, "every stream to end on both sides", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	if d.Err() != nil || a.Err() != nil {
		t.Fatalf("a carrier ended: %v / %v", d.Err(), a.Err())
	}
	serveEcho(t, a)
	data := pattern(40_000, 5)
	eqBytes(t, roundTrip(t, d, data), data)
}

func dialerSide(s Stream, p qplan) error {
	up, down := byte(p.seed), byte(p.seed+1)
	switch p.mode {
	case qReject:
		var re *ResultError
		if err := s.AwaitResult(context.Background()); !errors.As(err, &re) || re.Code != ResultPaused {
			return fmt.Errorf("AwaitResult = %v, want a refusal", err)
		}
		return nil
	case qDiallerReset, qDiallerClose:
		go func() { _ = writePattern(s, up, p.up) }()
		_, err := readVerified(s, down, min(p.cut, p.down))
		if !qAcceptable(err) {
			return err
		}
		if p.mode == qDiallerReset {
			_ = s.Reset(ResetPeerReset)
		} else {
			_ = s.Close()
		}
		return nil
	case qAcceptorReset, qAcceptorClose:
		go func() { _ = writePattern(s, up, p.up); _ = s.CloseWrite() }()
		n, err := readVerified(s, down, -1)
		if !qAcceptable(err) {
			return err
		}
		_ = n
		_ = s.Close()
		return nil
	}
	// graceful: everything both ways
	werr := make(chan error, 1)
	go func() {
		err := writePattern(s, up, p.up)
		if err == nil {
			err = s.CloseWrite()
		}
		werr <- err
	}()
	n, err := readVerified(s, down, -1)
	if err != io.EOF || n != p.down {
		return fmt.Errorf("read %d of %d bytes, then %v", n, p.down, err)
	}
	if err := <-werr; err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return s.Close()
}

func acceptorSide(s Stream, p qplan) error {
	up, down := byte(p.seed), byte(p.seed+1)
	switch p.mode {
	case qReject:
		return s.Reject(ResultPaused)
	case qDiallerReset, qDiallerClose:
		// the dialler may have reset the stream before it was answered
		if err := s.Accept(); err != nil {
			if qAcceptable(err) {
				return s.Close()
			}
			return err
		}
		go func() { _ = writePattern(s, down, p.down) }()
		_, err := readVerified(s, up, -1)
		if !qAcceptable(err) {
			return err
		}
		return s.Close()
	case qAcceptorReset, qAcceptorClose:
		if err := s.Accept(); err != nil {
			return err
		}
		go func() { _ = writePattern(s, down, p.down); _ = s.CloseWrite() }()
		_, err := readVerified(s, up, min(p.cut, p.up))
		if !qAcceptable(err) {
			return err
		}
		if p.mode == qAcceptorReset {
			return s.Reset(ResetPeerReset)
		}
		return s.Close()
	}
	if err := s.Accept(); err != nil {
		return err
	}
	werr := make(chan error, 1)
	go func() {
		err := writePattern(s, down, p.down)
		if err == nil {
			err = s.CloseWrite()
		}
		werr <- err
	}()
	n, err := readVerified(s, up, -1)
	if err != io.EOF || n != p.up {
		return fmt.Errorf("read %d of %d bytes, then %v", n, p.up, err)
	}
	if err := <-werr; err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return s.Close()
}

func TestQUICRandomizedTransfers(t *testing.T) {
	seeds := 8
	if v := os.Getenv("RELAY_MODEL_SEEDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		seeds = n
	}
	for seed := range seeds {
		t.Run(strconv.Itoa(seed), func(t *testing.T) { runQUICModel(t, uint64(seed)) })
	}
}

var _ = bytes.Equal
