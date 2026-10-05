package relay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"
	"time"
)

func udpParams(hop uint32) OpenParams {
	return OpenParams{Kind: StreamUDP, RouteID: "01HZZZZZZZZZZZZZZZZZZZZZZZ", HopIndex: hop}
}

// udpPair opens a UDP association on d and answers it on a, returning both
// ends once the dialler has the answer.
func udpPair(t testing.TB, d Carrier, a Carrier) (Stream, Stream) {
	t.Helper()
	s, err := d.Open(udpParams(1))
	if err != nil {
		t.Fatal(err)
	}
	in, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	return s, in
}

// readDatagrams reads n datagrams from s within the test deadline.
func readDatagrams(t testing.TB, s Stream, n int) [][]byte {
	t.Helper()
	_ = s.SetReadDeadline(time.Now().Add(testWait))
	var got [][]byte
	for range n {
		d, err := s.ReadDatagram()
		if err != nil {
			t.Fatalf("datagram %d of %d: %v", len(got), n, err)
		}
		got = append(got, d)
	}
	return got
}

// sameSet checks that two lists hold the same datagrams, in any order:
// datagrams are unordered.
func sameSet(t testing.TB, got, want [][]byte) {
	t.Helper()
	key := func(b []byte) string { return string(b) }
	var g, w []string
	for _, b := range got {
		g = append(g, key(b))
	}
	for _, b := range want {
		w = append(w, key(b))
	}
	sort.Strings(g)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Fatalf("got %d datagrams, want %d", len(g), len(w))
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("datagram sets differ at %d (lengths %d and %d)", i, len(g[i]), len(w[i]))
		}
	}
}

// UDP rides QUIC DATAGRAM frames both ways, whole datagrams, to the right
// association.
func TestQUICNativeDatagrams(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, in := udpPair(t, d, a)
	if s.Kind() != StreamUDP || in.Kind() != StreamUDP {
		t.Fatal("kinds")
	}
	var up, down [][]byte
	for i := range 40 {
		up = append(up, pattern(1+i*25, byte(i)))
		down = append(down, pattern(1+i*20, byte(100+i)))
	}
	for _, p := range up {
		if err := s.WriteDatagram(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range down {
		if err := in.WriteDatagram(p); err != nil {
			t.Fatal(err)
		}
	}
	sameSet(t, readDatagrams(t, in, len(up)), up)
	sameSet(t, readDatagrams(t, s, len(down)), down)

	ds, as := d.Stats(), a.Stats()
	if ds.DatagramsSent != 40 || ds.DatagramsOnStream != 0 || ds.DatagramsOversize != 0 || ds.DatagramsDropped != 0 {
		t.Fatalf("dialler stats %+v", ds)
	}
	if as.DatagramsRecv != 40 || as.DatagramsSent != 40 || as.DatagramsOnStream != 0 || as.DatagramsRecvDropped != 0 {
		t.Fatalf("listener stats %+v", as)
	}
	if ds.BytesSent == 0 || as.BytesRecv != ds.BytesSent {
		t.Fatalf("bytes %d sent, %d received", ds.BytesSent, as.BytesRecv)
	}
}

// Until the listener has answered, a dialler's datagrams follow the OPEN on the
// stream (a QUIC datagram could overtake the OPEN and be dropped); once it has,
// they ride DATAGRAM frames.
func TestQUICDatagramsFollowOpenUntilTheAnswer(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, err := d.Open(udpParams(1))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := s.WriteDatagram([]byte(fmt.Sprintf("early %d", i))); err != nil {
			t.Fatal(err)
		}
	}
	in, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	// the listener has not answered: it still reads what came on the stream,
	// in order
	got := readDatagrams(t, in, 3)
	for i, g := range got {
		if string(g) != fmt.Sprintf("early %d", i) {
			t.Fatalf("early datagram %d is %q", i, g)
		}
	}
	if st := d.Stats(); st.DatagramsOnStream != 3 || st.DatagramsSent != 3 {
		t.Fatalf("dialler stats %+v", st)
	}
	if err := in.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteDatagram([]byte("late")); err != nil {
		t.Fatal(err)
	}
	if got := readDatagrams(t, in, 1); string(got[0]) != "late" {
		t.Fatalf("%q", got[0])
	}
	if st := d.Stats(); st.DatagramsOnStream != 3 || st.DatagramsSent != 4 {
		t.Fatalf("after the answer: %+v", st)
	}
	// a listener's first datagram answers the stream, so it can go native at once
	s2, _ := d.Open(udpParams(1))
	in2, _ := a.Accept(ctxTimeout(t))
	if err := in2.WriteDatagram([]byte("server first")); err != nil {
		t.Fatal(err)
	}
	if got := readDatagrams(t, s2, 1); string(got[0]) != "server first" {
		t.Fatalf("%q", got[0])
	}
	if !s2.Answered() && s2.AwaitResult(ctxTimeout(t)) != nil {
		t.Fatal("the dialler never saw the answer")
	}
	if st := a.Stats(); st.DatagramsOnStream != 0 {
		t.Fatalf("a datagram of an answered stream went on the stream: %+v", st)
	}
}

// A datagram larger than the connection's maximum DATAGRAM size goes on the
// association's stream instead of being dropped, and is counted.
func TestQUICOversizeDatagramsFallBackToTheStream(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, in := udpPair(t, d, a)
	small := pattern(200, 1)
	big := pattern(3000, 2)
	huge := pattern(DefaultMaxFrame, 3)
	for _, p := range [][]byte{small, big, huge, small} {
		if err := s.WriteDatagram(p); err != nil {
			t.Fatal(err)
		}
	}
	sameSet(t, readDatagrams(t, in, 4), [][]byte{small, big, huge, small})
	waitFor(t, "the counters", func() bool { return d.Stats().DatagramsSent == 4 })
	st := d.Stats()
	if st.DatagramsOversize != 2 || st.DatagramsOnStream != 2 || st.DatagramsDropped != 0 {
		t.Fatalf("stats %+v", st)
	}
	// the other direction too
	if err := in.WriteDatagram(big); err != nil {
		t.Fatal(err)
	}
	if got := readDatagrams(t, s, 1); !bytes.Equal(got[0], big) {
		t.Fatal("the oversize datagram arrived changed")
	}
	if st := a.Stats(); st.DatagramsOversize != 1 {
		t.Fatalf("listener stats %+v", st)
	}
	// above the peer's frame limit it is an error, not a datagram
	if err := s.WriteDatagram(pattern(DefaultMaxFrame+1, 4)); !errors.Is(err, ErrDatagramSize) {
		t.Fatalf("WriteDatagram above MaxFrame = %v", err)
	}
}

// Without DATAGRAM support on either end UDP rides the association's stream.
func TestQUICUDPOverTheStreamWithoutDatagramSupport(t *testing.T) {
	for _, tc := range []struct {
		name   string
		d, a   QUICConfig
		native bool
	}{
		{"dialler without", QUICConfig{DisableDatagrams: true}, QUICConfig{}, false},
		{"listener without", QUICConfig{}, QUICConfig{DisableDatagrams: true}, false},
		{"neither", QUICConfig{DisableDatagrams: true}, QUICConfig{DisableDatagrams: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, tc.a)
			d, a := e.pair(tc.d)
			if d.NativeDatagrams() != tc.native || a.NativeDatagrams() != tc.native {
				t.Fatalf("NativeDatagrams %v %v", d.NativeDatagrams(), a.NativeDatagrams())
			}
			s, in := udpPair(t, d, a)
			var up, down [][]byte
			for i := range 30 {
				up = append(up, pattern(1+i*40, byte(i)))
				down = append(down, pattern(1+i*30, byte(50+i)))
			}
			for _, p := range up {
				if err := s.WriteDatagram(p); err != nil {
					t.Fatal(err)
				}
			}
			for _, p := range down {
				if err := in.WriteDatagram(p); err != nil {
					t.Fatal(err)
				}
			}
			// reliable and ordered on the stream, boundaries kept
			for i, g := range readDatagrams(t, in, len(up)) {
				eqBytes(t, g, up[i])
			}
			for i, g := range readDatagrams(t, s, len(down)) {
				eqBytes(t, g, down[i])
			}
			waitFor(t, "the counters", func() bool { return d.Stats().DatagramsOnStream == 30 && a.Stats().DatagramsOnStream == 30 })
			if st := d.Stats(); st.DatagramsOversize != 0 || st.DatagramsSent != 30 {
				t.Fatalf("stats %+v", st)
			}
		})
	}
}

// A TCP carrier carries UDP the same way and counts it.
func TestTLSCarrierCountsUDPOverTheStream(t *testing.T) {
	e := newTLSEnv(t, Config{})
	d, a := e.pair(Config{})
	s, in := udpPair(t, d, a)
	for i := range 5 {
		if err := s.WriteDatagram(pattern(100+i, 1)); err != nil {
			t.Fatal(err)
		}
	}
	readDatagrams(t, in, 5)
	waitFor(t, "the counters", func() bool { return d.Stats().DatagramsOnStream == 5 })
	if st := d.Stats(); st.DatagramsSent != 5 || st.DatagramsOversize != 0 {
		t.Fatalf("stats %+v", st)
	}
}

// WriteDatagram never blocks: when the path is blocked the queue fills and
// datagrams are dropped and counted.
func TestQUICWriteDatagramNeverBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  QUICConfig
	}{
		{"native", QUICConfig{}},
		{"over the stream", QUICConfig{DisableDatagrams: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.IdleTimeout = time.Minute
			tc.cfg.PingInterval = 10 * time.Second
			e := newQUICEnv(t, tc.cfg)
			p := newUDPProxy(t, e.l.Addr())
			d, err := e.dialVia(p, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = d.Close() }()
			a := acceptQUIC(t, e.l)
			s, _ := udpPair(t, d, a)
			p.pause() // nothing gets through any more
			done := make(chan error, 1)
			go func() {
				payload := pattern(1000, 7)
				for range 200000 {
					if err := s.WriteDatagram(payload); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			select {
			case err := <-done:
				if !errors.Is(err, ErrDatagramDropped) {
					t.Fatalf("WriteDatagram = %v, want a drop", err)
				}
			case <-time.After(testWait):
				t.Fatal("WriteDatagram blocked")
			}
			if st := d.Stats(); st.DatagramsDropped == 0 {
				t.Fatalf("stats %+v", st)
			}
			p.resume()
		})
	}
}

// A datagram that finds the association's receive queue full is dropped and
// counted; the queue is bounded by the stream window.
func TestQUICReceiveQueueIsBounded(t *testing.T) {
	acfg := QUICConfig{Config: Config{StreamWindow: MinWindow}, StreamWindowCeiling: MinWindow}
	e := newQUICEnv(t, acfg)
	d, a := e.pair(QUICConfig{})
	s, in := udpPair(t, d, a)
	const n = 100
	for range n {
		if err := s.WriteDatagram(pattern(1000, 1)); err != nil {
			t.Fatal(err)
		}
	}
	// the application reads nothing: the queue takes about a window, the rest
	// is dropped
	waitFor(t, "every datagram to be accounted for", func() bool {
		st := a.Stats()
		return st.DatagramsRecv+st.DatagramsRecvDropped == n
	})
	st := a.Stats()
	if st.DatagramsRecvDropped == 0 || st.DatagramsRecv < 1 || st.DatagramsRecv > 6 {
		t.Fatalf("stats %+v", st)
	}
	got := readDatagrams(t, in, int(st.DatagramsRecv))
	if len(got) == 0 {
		t.Fatal("nothing queued")
	}
}

func TestQUICDatagramErrors(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, in := udpPair(t, d, a)
	if err := s.WriteDatagram(nil); !errors.Is(err, ErrDatagramSize) {
		t.Fatalf("an empty datagram = %v", err)
	}
	tcp, _ := d.Open(testParams())
	if err := tcp.WriteDatagram([]byte("x")); !errors.Is(err, ErrStreamKind) {
		t.Fatalf("WriteDatagram on a TCP stream = %v", err)
	}
	if _, err := tcp.ReadDatagram(); !errors.Is(err, ErrStreamKind) {
		t.Fatalf("ReadDatagram on a TCP stream = %v", err)
	}
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrStreamKind) {
		t.Fatalf("Read on a UDP stream = %v", err)
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrStreamKind) {
		t.Fatalf("Write on a UDP stream = %v", err)
	}
	// a read deadline applies to ReadDatagram
	_ = in.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, err := in.ReadDatagram(); !isTimeout(err) {
		t.Fatalf("ReadDatagram = %v, want a timeout", err)
	}
	_ = in.SetReadDeadline(time.Time{})
	// a closed stream takes nothing
	_ = s.Close()
	if err := s.WriteDatagram([]byte("x")); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("WriteDatagram after Close = %v", err)
	}
	if _, err := s.ReadDatagram(); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("ReadDatagram after Close = %v", err)
	}
}

// An association lives and ends as a stream does: the peer's CloseWrite is an
// EOF after the queued datagrams, a reset fails both ends, and the slots free.
func TestQUICAssociationLifecycle(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})

	t.Run("half-close", func(t *testing.T) {
		s, in := udpPair(t, d, a)
		for i := range 3 {
			_ = s.WriteDatagram([]byte(fmt.Sprintf("d%d", i)))
		}
		got := readDatagrams(t, in, 3)
		_ = got
		_ = s.CloseWrite()
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.ReadDatagram(); err != io.EOF {
			t.Fatalf("after the peer's CloseWrite: %v", err)
		}
		// the other direction still works
		if err := in.WriteDatagram([]byte("still")); err != nil {
			t.Fatal(err)
		}
		if got := readDatagrams(t, s, 1); string(got[0]) != "still" {
			t.Fatalf("%q", got[0])
		}
		if err := s.WriteDatagram([]byte("x")); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("WriteDatagram after CloseWrite = %v", err)
		}
		_ = in.CloseWrite()
		_ = s.SetReadDeadline(time.Now().Add(testWait))
		if _, err := s.ReadDatagram(); err != io.EOF {
			t.Fatalf("after the other CloseWrite: %v", err)
		}
		_ = s.Close()
		_ = in.Close()
		waitFor(t, "the streams to finish", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	})

	t.Run("queued datagrams precede the FIN on the stream", func(t *testing.T) {
		// oversize datagrams travel the stream; CloseWrite must come after them
		s, in := udpPair(t, d, a)
		big := pattern(5000, 3)
		for range 5 {
			_ = s.WriteDatagram(big)
		}
		_ = s.CloseWrite()
		for i, g := range readDatagrams(t, in, 5) {
			eqBytes(t, g, big)
			_ = i
		}
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.ReadDatagram(); err != io.EOF {
			t.Fatalf("after the datagrams: %v", err)
		}
		_ = s.Close()
		_ = in.Close()
	})

	t.Run("close and reset", func(t *testing.T) {
		s, in := udpPair(t, d, a)
		_ = s.Close()
		var re *StreamResetError
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.ReadDatagram(); !errors.As(err, &re) || re.Reason != ResetCancel {
			t.Fatalf("ReadDatagram after the peer's Close = %v", err)
		}
		if err := in.WriteDatagram([]byte("x")); err == nil {
			t.Fatal("WriteDatagram on a closed association")
		}
		s2, in2 := udpPair(t, d, a)
		_ = in2.Reset(ResetPeerReset)
		_ = s2.SetReadDeadline(time.Now().Add(testWait))
		if _, err := s2.ReadDatagram(); !errors.As(err, &re) || re.Reason != ResetPeerReset || !re.Remote {
			t.Fatalf("ReadDatagram after the peer's Reset = %v", err)
		}
	})
	waitFor(t, "every stream to be gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
}

// Many associations at once: every datagram reaches its own. Each association
// sends its next datagram when the last one arrived, so no queue overflows and
// nothing may be lost.
func TestQUICManyAssociationsDoNotMix(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	const assocs, per = 40, 8
	type pair struct{ s, in Stream }
	var pairs []pair
	for range assocs {
		s, in := udpPair(t, d, a)
		pairs = append(pairs, pair{s, in})
	}
	var wg sync.WaitGroup
	errs := make(chan error, assocs)
	for i, p := range pairs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.in.SetReadDeadline(time.Now().Add(testWait))
			for j := range per {
				b := binary.BigEndian.AppendUint32(nil, uint32(i))
				b = binary.BigEndian.AppendUint32(b, uint32(j))
				if err := p.s.WriteDatagram(append(b, pattern(50+j, byte(i))...)); err != nil {
					errs <- err
					return
				}
				g, err := p.in.ReadDatagram()
				if err != nil {
					errs <- fmt.Errorf("association %d datagram %d: %w", i, j, err)
					return
				}
				if binary.BigEndian.Uint32(g) != uint32(i) || binary.BigEndian.Uint32(g[4:]) != uint32(j) || !bytes.Equal(g[8:], pattern(50+j, byte(i))) {
					errs <- fmt.Errorf("association %d datagram %d arrived as %d/%d", i, j, binary.BigEndian.Uint32(g), binary.BigEndian.Uint32(g[4:]))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if st := a.Stats(); st.DatagramsRecv != assocs*per || st.DatagramsRecvDropped != 0 {
		t.Fatalf("stats %+v", st)
	}
}

// Closing a carrier ends its associations with the carrier's error.
func TestQUICAssociationsEndWithTheCarrier(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, in := udpPair(t, d, a)
	_ = d.Close()
	waitDone(t, a)
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	if _, err := in.ReadDatagram(); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("ReadDatagram = %v", err)
	}
	if err := s.WriteDatagram([]byte("x")); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("WriteDatagram = %v", err)
	}
	_ = context.Background
}
