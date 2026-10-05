package relay

import (
	"crypto/sha256"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeAll writes data to s in the background and returns when the writer is
// done (the channel), and how many bytes were accepted so far.
func writeAll(s *Stream, data []byte) (done <-chan error, written *atomic.Int64) {
	ch := make(chan error, 1)
	written = new(atomic.Int64)
	go func() {
		const step = 8 * 1024
		for off := 0; off < len(data); off += step {
			n, err := s.Write(data[off:min(off+step, len(data))])
			written.Add(int64(n))
			if err != nil {
				ch <- err
				return
			}
		}
		ch <- s.CloseWrite()
	}()
	return ch, written
}

// settle waits until f stops changing for quiet.
func settle(t testing.TB, quiet time.Duration, f func() int64) int64 {
	t.Helper()
	last, since := f(), time.Now()
	deadline := time.Now().Add(testWait)
	for time.Since(since) < quiet {
		if time.Now().After(deadline) {
			t.Fatal("never settled")
		}
		time.Sleep(5 * time.Millisecond)
		if v := f(); v != last {
			last, since = v, time.Now()
		}
	}
	return last
}

// A receiver that does not read holds back only its own stream: the stalled
// stream stops at its window, and its neighbour on the same carrier runs at
// full speed (anixops-protocol.md section 4.4).
func TestStalledStreamDoesNotStallItsNeighbour(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		slow, _ := d.Open(testParams())
		stalledData := pattern(4*DefaultStreamWindow, 9)
		done, written := writeAll(slow, stalledData)
		slowIn, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = slowIn.Accept()
		// Nobody reads slowIn. The writer fills the stream window plus its
		// send buffer and stops.
		got := settle(t, 150*time.Millisecond, written.Load)
		if got < DefaultStreamWindow || got > int64(DefaultStreamWindow+DefaultSendBuffer+DefaultMaxFrame) {
			t.Fatalf("stalled writer got %d bytes in, want about the stream window %d", got, DefaultStreamWindow)
		}
		if a.Stats().BytesRecv > DefaultStreamWindow {
			t.Fatalf("listener buffered %d bytes of a stalled stream, window is %d", a.Stats().BytesRecv, DefaultStreamWindow)
		}
		select {
		case err := <-done:
			t.Fatalf("stalled writer finished: %v", err)
		default:
		}

		// The neighbour echoes 2 MiB meanwhile.
		serveEcho(t, a)
		data := pattern(2<<20, 3)
		start := time.Now()
		eqBytes(t, roundTrip(t, d, data), data)
		if time.Since(start) > testWait/2 {
			t.Fatalf("neighbour took %v", time.Since(start))
		}
		if d.Stats().StreamStalls == 0 {
			t.Fatal("no stream stall counted")
		}

		// Reading the stalled stream releases the writer, and every byte
		// arrives in order.
		var wg sync.WaitGroup
		wg.Add(1)
		var gotData []byte
		go func() {
			defer wg.Done()
			gotData, _ = io.ReadAll(slowIn)
		}()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		eqBytes(t, gotData, stalledData)
	})
}

// The carrier window bounds what a carrier buffers, however many streams the
// peer fills: with nothing read, the listener holds at most CarrierWindow
// bytes and every sender waits (anixops-protocol.md section 4.5).
func TestCarrierWindowBoundsBufferedBytes(t *testing.T) {
	const streams = 8
	acfg := Config{StreamWindow: 32 * 1024, CarrierWindow: 64 * 1024}
	d, a := newPair(t, "tcp", Config{}, acfg)
	data := make([][]byte, streams)
	var done []<-chan error
	for i := range streams {
		s, _ := d.Open(testParams())
		data[i] = pattern(256*1024, byte(i))
		ch, _ := writeAll(s, data[i])
		done = append(done, ch)
	}
	var in []*Stream
	for range streams {
		s, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = s.Accept()
		in = append(in, s)
	}
	settle(t, 200*time.Millisecond, func() int64 { return int64(a.Stats().BytesRecv) })
	if got := a.Stats().BytesRecv; got != uint64(acfg.CarrierWindow) {
		t.Fatalf("listener buffered %d bytes, want exactly the carrier window %d", got, acfg.CarrierWindow)
	}
	if d.Stats().CarrierStalls == 0 {
		t.Fatal("no carrier stall counted")
	}
	// Draining every stream completes every transfer, intact.
	sums := make([][32]byte, streams)
	var wg sync.WaitGroup
	for i, s := range in {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := io.ReadAll(s)
			if err != nil {
				t.Errorf("stream %d: %v", i, err)
			}
			sums[i] = sha256.Sum256(b)
		}()
	}
	wg.Wait()
	for i := range streams {
		if err := <-done[i]; err != nil {
			t.Fatal(err)
		}
		if sums[i] != sha256.Sum256(data[i]) {
			t.Fatalf("stream %d corrupted", i)
		}
	}
}

// Documented behaviour: credit comes back as the application reads, which
// bounds memory strictly, so as many stalled streams as fit the carrier
// window stall every other stream on that carrier until they are read.
func TestStalledStreamsCanPinTheCarrier(t *testing.T) {
	acfg := Config{StreamWindow: 32 * 1024, CarrierWindow: 64 * 1024}
	d, a := newPair(t, "tcp", Config{}, acfg)
	var stalled []*Stream
	for range 2 {
		s, _ := d.Open(testParams())
		writeAll(s, pattern(64*1024, 1))
		in, _ := a.Accept(ctxTimeout(t))
		_ = in.Accept()
		stalled = append(stalled, in)
	}
	settle(t, 150*time.Millisecond, func() int64 { return int64(a.Stats().BytesRecv) })
	if got := a.Stats().BytesRecv; got != 64*1024 {
		t.Fatalf("buffered %d", got)
	}
	// A third stream gets nothing through until a stalled one is read.
	s, _ := d.Open(testParams())
	_, w := writeAll(s, []byte("neighbour"))
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	time.Sleep(100 * time.Millisecond)
	if got := a.Stats().BytesRecv; got != 64*1024 {
		t.Fatalf("buffered %d with the carrier window spent", got)
	}
	go func() { _, _ = io.Copy(io.Discard, stalled[0]) }()
	buf := make([]byte, 9)
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(in, buf); err != nil || string(buf) != "neighbour" {
		t.Fatalf("neighbour after release: %q %v", buf, err)
	}
	_ = w
}

// Closing or resetting a stream returns the credit its unread bytes held;
// without that the carrier would run dry after a few abandoned streams.
func TestCreditIsReturnedWhenUnreadDataIsDiscarded(t *testing.T) {
	acfg := Config{StreamWindow: 32 * 1024, CarrierWindow: 64 * 1024}
	d, a := newPair(t, "tcp", Config{}, acfg)
	for i := range 20 {
		s, _ := d.Open(testParams())
		if _, err := s.Write(pattern(30*1024, byte(i))); err != nil {
			t.Fatal(err)
		}
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = in.Accept()
		// let the data arrive unread, then abandon it
		waitFor(t, "data", func() bool { return a.Stats().BytesRecv >= uint64(30*1024*(i+1)) })
		if i%2 == 0 {
			_ = in.Close()
		} else {
			_ = in.Reset(ResetPeerReset)
		}
		_ = s.Close()
	}
	serveEcho(t, a)
	data := pattern(300*1024, 5)
	eqBytes(t, roundTrip(t, d, data), data)
}

// Frames of a stream that finished here are still in flight when the peer
// learns of it: they must not end the carrier, and their credit comes back.
func TestFramesForAFinishedStreamAreIgnored(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleAcceptor, Config{CarrierWindow: 8192, StreamWindow: 8192}, DefaultSettings())
	p.open(1)
	in, err := c.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = in.Accept()
	_ = in.Reset(ResetCancel)
	f := p.recvType(TypeReset)
	if f.StreamID != 1 {
		t.Fatalf("RESET for %d", f.StreamID)
	}
	// The peer had not seen the RESET yet.
	p.send(Frame{Type: TypeData, StreamID: 1, Payload: make([]byte, 5000)})
	p.send(Frame{Type: TypeWindow, StreamID: 1, Payload: marshalWindow(100)})
	p.send(Frame{Type: TypeDatagram, StreamID: 1, Payload: []byte("late")})
	p.send(Frame{Type: TypeReset, StreamID: 1, Payload: marshalReset(ResetCancel)})
	w := p.recvType(TypeWindow)
	if w.StreamID != 0 {
		t.Fatalf("WINDOW for stream %d, want the carrier's", w.StreamID)
	}
	if inc, _ := parseWindow(w.Payload); inc < 4096 {
		t.Fatalf("returned %d bytes of credit", inc)
	}
	// And the carrier works.
	p.open(3)
	if _, err := c.Accept(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	if c.Err() != nil {
		t.Fatal(c.Err())
	}
}

func TestSlowWriterNeverExceedsCredit(t *testing.T) {
	// A listener-side Write is flow controlled too: the dialler reads
	// slowly and the listener's sends stop at the dialler's window.
	d, a := newPair(t, "tcp", Config{StreamWindow: 32 * 1024, CarrierWindow: 64 * 1024}, Config{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	data := pattern(1<<20, 4)
	done, written := writeAll(in, data)
	got := settle(t, 150*time.Millisecond, written.Load)
	if got > int64(32*1024+DefaultSendBuffer+DefaultMaxFrame) {
		t.Fatalf("listener wrote %d bytes ahead of a 32 KiB window", got)
	}
	if d.Stats().BytesRecv > 32*1024 {
		t.Fatalf("dialler buffered %d", d.Stats().BytesRecv)
	}
	b, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}
	eqBytes(t, b, data)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWriteToAClosedCarrierFails(t *testing.T) {
	d, a := newPair(t, "pipe", Config{}, Config{})
	s, _ := d.Open(testParams())
	if _, err := a.Accept(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	_ = a.Close()
	waitDone(t, d)
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrCarrierClosed) {
		t.Fatal(err)
	}
	if s.Answered() {
		t.Fatal("Answered after the carrier died before RESULT")
	}
}
