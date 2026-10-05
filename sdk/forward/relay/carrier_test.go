package relay

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func eachConn(t *testing.T, f func(t *testing.T, kind string)) {
	for _, kind := range []string{"tcp", "pipe"} {
		t.Run(kind, func(t *testing.T) { f(t, kind) })
	}
}

func TestSettingsExchange(t *testing.T) {
	dcfg := Config{StreamWindow: 64 * 1024, CarrierWindow: 128 * 1024, MaxFrame: 4096}
	acfg := Config{MaxStreams: 7, MaxFrame: 2048}
	d, a := newPair(t, "tcp", dcfg, acfg)
	if got := a.PeerSettings(); got.MaxStreams != 0 || got.StreamWindow != 64*1024 || got.CarrierWindow != 128*1024 || got.MaxFrame != 4096 {
		t.Fatalf("acceptor sees dialler settings %+v", got)
	}
	if got := d.PeerSettings(); got.MaxStreams != 7 || got.MaxFrame != 2048 || got.StreamWindow != DefaultStreamWindow {
		t.Fatalf("dialler sees acceptor settings %+v", got)
	}
	if d.Role() != RoleDialer || a.Role() != RoleAcceptor {
		t.Fatalf("roles %v %v", d.Role(), a.Role())
	}
}

func TestHandshakeFailures(t *testing.T) {
	tests := []struct {
		name string
		send []byte
	}{
		{"not settings", mustFrame(t, Frame{Type: TypePing, Payload: make([]byte, 8)})},
		{"settings on a stream", mustFrame(t, Frame{Type: TypeSettings, StreamID: 1})},
		{"bad settings", mustFrame(t, Frame{Type: TypeSettings, Payload: Settings{MaxFrame: 10}.Marshal()})},
		{"truncated", mustFrame(t, Frame{Type: TypeSettings, Payload: DefaultSettings().Marshal()})[:10]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc, pc := tcpPair(t)
			go func() {
				_, _ = pc.Write(tc.send)
				if tc.name == "truncated" {
					_ = pc.Close()
				}
			}()
			_, err := NewConnCarrier(cc, RoleAcceptor, Config{HandshakeTimeout: time.Second})
			if err == nil {
				t.Fatal("handshake succeeded")
			}
		})
	}
	t.Run("silent peer times out", func(t *testing.T) {
		cc, _ := tcpPair(t)
		start := time.Now()
		_, err := NewConnCarrier(cc, RoleDialer, Config{HandshakeTimeout: 50 * time.Millisecond})
		if err == nil || !isTimeout(errors.Unwrap(err)) && !isTimeout(err) {
			t.Fatalf("err = %v, want a timeout", err)
		}
		if time.Since(start) > testWait {
			t.Fatal("took too long")
		}
	})
	t.Run("bad config", func(t *testing.T) {
		cc, _ := tcpPair(t)
		if _, err := NewConnCarrier(cc, RoleDialer, Config{MaxFrame: 10}); err == nil {
			t.Fatal("accepted a max frame of 10")
		}
		cc, _ = tcpPair(t)
		if _, err := NewConnCarrier(cc, RoleDialer, Config{PingInterval: time.Minute, IdleTimeout: time.Second}); err == nil {
			t.Fatal("accepted a ping interval longer than the idle timeout")
		}
	})
}

func mustFrame(t testing.TB, f Frame) []byte {
	t.Helper()
	b, err := AppendFrame(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEchoAndZeroRoundTrip(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		serveEcho(t, a)
		for _, n := range []int{0, 1, 100, DefaultMaxFrame, DefaultMaxFrame + 1, 3*DefaultStreamWindow + 17} {
			data := pattern(n, byte(n))
			eqBytes(t, roundTrip(t, d, data), data)
		}
		waitFor(t, "streams to finish", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	})
}

func TestOpenParamsReachTheListener(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	want := OpenParams{Kind: StreamTCP, RouteID: "01HZZZZZZZZZZZZZZZZZZZZZZZ", HopIndex: 3, Client: netip.MustParseAddrPort("[2001:db8::1]:4242")}
	s, err := d.Open(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Params() != want || got.ID() != s.ID() || got.Kind() != StreamTCP {
		t.Fatalf("listener sees %+v id %d", got.Params(), got.ID())
	}
	if err := got.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	if err := got.Accept(); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("second Accept = %v", err)
	}
}

// The dialler's bytes are on the wire right behind OPEN: a server-first
// protocol and a client-first one both work without waiting for RESULT.
func TestWriteBeforeResult(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Write([]byte("hello")); err != nil {
			t.Fatal(err)
		}
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(in, buf); err != nil || string(buf) != "hello" {
			t.Fatalf("read %q, %v", buf, err)
		}
		// Writing on the listener side answers the stream implicitly.
		if _, err := in.Write([]byte("hi")); err != nil {
			t.Fatal(err)
		}
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		buf = buf[:2]
		if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "hi" {
			t.Fatalf("read %q, %v", buf, err)
		}
	})
}

func TestHalfCloseBothWays(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		s, _ := d.Open(testParams())
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		// The dialler finishes first; the listener still sends, as a target
		// answering a client that half-closed.
		_, _ = s.Write([]byte("request"))
		if err := s.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(in)
		if err != nil || string(got) != "request" {
			t.Fatalf("listener read %q, %v", got, err)
		}
		if _, err := s.Write([]byte("late")); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("write after CloseWrite = %v", err)
		}
		_, _ = in.Write([]byte("response"))
		if err := in.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		got, err = io.ReadAll(s)
		if err != nil || string(got) != "response" {
			t.Fatalf("dialler read %q, %v", got, err)
		}
		waitFor(t, "both directions finished", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		_ = in.Close()
	})
}

func TestRejectAnswersTheDialler(t *testing.T) {
	for _, code := range []ResultCode{ResultUpstreamUnreachable, ResultAdmissionDenied, ResultPaused, ResultQuotaExceeded, ResultLimitExceeded, ResultRouteMismatch, ResultNoHop, ResultCode(999)} {
		t.Run(code.String(), func(t *testing.T) {
			d, a := newPair(t, "tcp", Config{}, Config{})
			s, _ := d.Open(testParams())
			_, _ = s.Write([]byte("first bytes kept by the dialler for a retry"))
			in, err := a.Accept(ctxTimeout(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Reject(code); err != nil {
				t.Fatal(err)
			}
			var re *ResultError
			if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Code != code {
				t.Fatalf("AwaitResult = %v", err)
			}
			if _, err := s.Read(make([]byte, 1)); !errors.As(err, &re) {
				t.Fatalf("Read = %v", err)
			}
			if _, err := s.Write([]byte("x")); !errors.As(err, &re) {
				t.Fatalf("Write = %v", err)
			}
			waitFor(t, "stream finished", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
			if code != ResultOK && d.Stats().ResultFailures[codeIndex(uint16(code))] != 1 || a.Stats().ResultFailures[codeIndex(uint16(code))] != 1 {
				t.Fatalf("result failure counters: dialler %v, listener %v", d.Stats().ResultFailures, a.Stats().ResultFailures)
			}
			// The carrier is unharmed.
			serveEcho(t, a)
			eqBytes(t, roundTrip(t, d, []byte("after")), []byte("after"))
		})
	}
	t.Run("OK is not a rejection", func(t *testing.T) {
		d, a := newPair(t, "tcp", Config{}, Config{})
		_, _ = d.Open(testParams())
		in, _ := a.Accept(ctxTimeout(t))
		if err := in.Reject(ResultOK); err == nil {
			t.Fatal("Reject(ResultOK) succeeded")
		}
	})
}

func TestCloseUnansweredStreamIsAnswered(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Close() // L2: never left unanswered
	var re *ResultError
	if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Code != ResultInternal {
		t.Fatalf("AwaitResult = %v", err)
	}
}

func TestResetPropagates(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		for _, side := range []string{"dialler", "listener"} {
			t.Run(side, func(t *testing.T) {
				d, a := newPair(t, kind, Config{}, Config{})
				s, _ := d.Open(testParams())
				_, _ = s.Write([]byte("data"))
				in, _ := a.Accept(ctxTimeout(t))
				_ = in.Accept()
				_ = s.AwaitResult(ctxTimeout(t))
				first, second := s, in
				if side == "listener" {
					first, second = in, s
				}
				readErr := make(chan error, 1)
				go func() { _, err := second.Read(make([]byte, 100)); readErr <- err }()
				if side == "listener" {
					// the dialler's pending read must see the reset, after
					// the "data" the listener never read is discarded
					go func() { time.Sleep(10 * time.Millisecond) }()
				}
				_ = first.Reset(ResetPeerReset)
				var re *StreamResetError
				var err error
				if side == "dialler" {
					// the listener's Read returns the 4 buffered bytes or the reset
					err = <-readErr
					if err == nil {
						_, err = second.Read(make([]byte, 100))
					}
				} else {
					err = <-readErr
					if err == nil {
						_, err = second.Read(make([]byte, 100))
					}
				}
				if !errors.As(err, &re) || re.Reason != ResetPeerReset || !re.Remote {
					t.Fatalf("peer error = %v", err)
				}
				if _, err := second.Write([]byte("x")); !errors.As(err, &re) {
					t.Fatalf("peer write after reset = %v", err)
				}
				if _, err := first.Write([]byte("x")); err == nil {
					t.Fatal("write after local reset succeeded")
				}
				waitFor(t, "streams gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
			})
		}
	})
}

func TestRefusedAtTheStreamLimit(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{MaxStreams: 2})
	var open []Stream
	for range 2 {
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		open = append(open, s)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("third Open = %v, want ErrStreamLimit", err)
	}
	// A finished stream frees its slot.
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Reject(ResultPaused)
	waitFor(t, "slot freed", func() bool { return d.ActiveStreams() == 1 })
	if _, err := d.Open(testParams()); err != nil {
		t.Fatalf("Open after a stream finished: %v", err)
	}
	_ = open
}

func TestListenerEnforcesItsStreamLimit(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleAcceptor, Config{MaxStreams: 2}, DefaultSettings())
	_ = c
	p.open(1)
	p.open(3)
	p.open(5) // over the limit: refused, carrier stays up
	f := p.recvType(TypeReset)
	if f.StreamID != 5 {
		t.Fatalf("RESET for stream %d, want 5", f.StreamID)
	}
	if r, _ := parseReset(f.Payload); r != ResetRefusedStream {
		t.Fatalf("reason %v", r)
	}
	if c.Stats().StreamsRefused != 1 || c.Stats().StreamsAccepted != 2 {
		t.Fatalf("stats %+v", c.Stats())
	}
	if c.Err() != nil {
		t.Fatal(c.Err())
	}
}

func TestAcceptQueueFull(t *testing.T) {
	_, p := newRawPeer(t, "tcp", RoleAcceptor, Config{AcceptQueue: 1}, DefaultSettings())
	p.open(1)
	p.open(3)
	f := p.recvType(TypeReset)
	if f.StreamID != 3 {
		t.Fatalf("RESET for stream %d, want 3", f.StreamID)
	}
}

func TestOpenAfterGoAwayIsRefusedWithoutBlame(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleAcceptor, Config{}, DefaultSettings())
	p.open(1) // keeps the carrier draining instead of ended
	waitFor(t, "stream", func() bool { return c.ActiveStreams() == 1 })
	if err := c.GoAway(GoAwayListenerClosed, time.Minute); err != nil {
		t.Fatal(err)
	}
	// Many OPENs raced the GOAWAY; none of them is misbehaviour.
	for i := range 3 * calmBurst {
		p.open(uint32(2*i + 3))
	}
	refused := 0
	for refused < 3*calmBurst {
		if f := p.recv(); f.Type == TypeReset {
			refused++
		}
	}
	if c.Err() != nil {
		t.Fatalf("carrier ended: %v", c.Err())
	}
}

func TestGoAwayDrains(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		serveEcho(t, a)
		s, _ := d.Open(testParams())
		_, _ = s.Write([]byte("ping"))
		buf := make([]byte, 4)
		if _, err := io.ReadFull(s, buf); err != nil {
			t.Fatal(err)
		}
		if err := a.GoAway(GoAwayListenerClosed, 0); err != nil {
			t.Fatal(err)
		}
		select {
		case <-d.Draining():
		case <-time.After(testWait):
			t.Fatal("dialler did not see GOAWAY")
		}
		if reason, ok := d.PeerGoAway(); !ok || reason != GoAwayListenerClosed {
			t.Fatalf("PeerGoAway = %v %v", reason, ok)
		}
		if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
			t.Fatalf("Open on a draining carrier = %v", err)
		}
		// The open stream keeps working, and the carrier ends with it.
		_, _ = s.Write([]byte("more"))
		if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "more" {
			t.Fatalf("stream after GOAWAY: %q %v", buf, err)
		}
		_ = s.CloseWrite()
		_, _ = io.Copy(io.Discard, s)
		_ = s.Close()
		waitDone(t, d)
		waitDone(t, a)
	})
}

func TestGoAwayDrainTimeoutClosesAnyway(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	serveEcho(t, a)
	s, _ := d.Open(testParams())
	_, _ = s.Write([]byte("x"))
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	if err := a.GoAway(GoAwayListenerClosed, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitDone(t, a)
	waitDone(t, d)
	if _, err := s.Read(make([]byte, 10)); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("stream read after drain timeout = %v", err)
	}
}

func TestGoAwayRefusesTheStreamsItNeverAccepted(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleDialer, Config{}, withSettings(func(s *Settings) { s.MaxStreams = 10 }))
	s1, _ := c.Open(testParams())
	s2, _ := c.Open(testParams())
	f1, f2 := p.recvType(TypeOpen), p.recvType(TypeOpen)
	if f1.StreamID != 1 || f2.StreamID != 3 {
		t.Fatalf("ids %d %d", f1.StreamID, f2.StreamID)
	}
	p.send(Frame{Type: TypeResult, StreamID: 1, Payload: marshalResult(ResultOK)})
	// The peer accepted stream 1 only.
	p.send(Frame{Type: TypeGoAway, Payload: marshalGoAway(1, GoAwayListenerClosed)})
	if err := s1.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatalf("stream 1: %v", err)
	}
	if err := s2.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrRefused) {
		t.Fatalf("stream 2 = %v, want ErrRefused", err)
	}
}

// L5: OPENs left unanswered on a carrier that answers pings retire it.
func TestStuckCarrierIsRetired(t *testing.T) {
	cfg := Config{ResultTimeout: 50 * time.Millisecond}
	d, a := newPair(t, "tcp", cfg, Config{})
	s, _ := d.Open(testParams())
	// The listener never answers: nobody calls Accept.
	if err := s.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrOpenTimeout) {
		t.Fatalf("AwaitResult = %v, want ErrOpenTimeout", err)
	}
	select {
	case <-d.Draining():
	case <-time.After(testWait):
		t.Fatal("carrier not retired")
	}
	// With its stuck stream gone the retired carrier has nothing left to
	// drain and ends.
	if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) && !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Open = %v", err)
	}
	waitFor(t, "peer to see GOAWAY(stuck)", func() bool { r, ok := a.PeerGoAway(); return ok && r == GoAwayStuck })
}

func TestLivenessPingsAndIdleTimeout(t *testing.T) {
	t.Run("idle but alive", func(t *testing.T) {
		cfg := Config{PingInterval: 50 * time.Millisecond, IdleTimeout: 400 * time.Millisecond}
		d, a := newPair(t, "tcp", cfg, cfg)
		time.Sleep(1200 * time.Millisecond) // three idle timeouts
		if d.Err() != nil || a.Err() != nil {
			t.Fatalf("idle carriers ended: %v %v", d.Err(), a.Err())
		}
		if d.RTT() <= 0 {
			t.Fatal("no RTT sample")
		}
		if d.Stats().PingsSent == 0 {
			t.Fatal("no pings sent")
		}
	})
	t.Run("dead peer", func(t *testing.T) {
		cfg := Config{PingInterval: 20 * time.Millisecond, IdleTimeout: 100 * time.Millisecond}
		c, p := newRawPeer(t, "tcp", RoleDialer, cfg, DefaultSettings())
		s, _ := c.Open(testParams())
		waitDone(t, c)
		if !errors.Is(c.Err(), ErrIdleTimeout) {
			t.Fatalf("Err = %v", c.Err())
		}
		// nothing was received after the SETTINGS exchange, so the carrier
		// can only have ended once the idle timeout had run, counted from
		// its own start
		if time.Since(c.start) < 100*time.Millisecond {
			t.Fatal("closed before the idle timeout")
		}
		if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrCarrierClosed) {
			t.Fatalf("stream = %v", err)
		}
		if c.Stats().KeepaliveTimeout != 1 {
			t.Fatalf("stats %+v", c.Stats())
		}
		_ = p
	})
}

func TestPingFloodIsCalmed(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleAcceptor, Config{}, DefaultSettings())
	ping := mustFrame(t, Frame{Type: TypePing, Payload: make([]byte, 8)})
	go func() {
		for range 4 * calmBurst {
			if _, err := p.conn.Write(ping); err != nil {
				return // the carrier already ended
			}
		}
	}()
	if r := p.expectGoAway(); r != GoAwayCalm {
		t.Fatalf("GOAWAY reason %v", r)
	}
	waitDone(t, c)
}

func TestCloseFailsStreamsAndReleasesEverything(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	readErr := make(chan error, 1)
	go func() { _, err := s.Read(make([]byte, 1)); readErr <- err }()
	time.Sleep(10 * time.Millisecond)
	_ = d.Close()
	select {
	case err := <-readErr:
		if !errors.Is(err, ErrCarrierClosed) {
			t.Fatalf("blocked Read = %v", err)
		}
	case <-time.After(testWait):
		t.Fatal("Read still blocked after Close")
	}
	waitDone(t, d)
	waitDone(t, a) // the peer saw GOAWAY or EOF
	if _, err := in.Read(make([]byte, 1)); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("listener stream = %v", err)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Open after Close = %v", err)
	}
	if _, err := a.Accept(context.Background()); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Accept after close = %v", err)
	}
	if r, ok := a.PeerGoAway(); !ok || r != GoAwayShutdown {
		t.Fatalf("peer GOAWAY = %v %v", r, ok)
	}
	_ = d.Close() // idempotent
}

func TestShutdownWaitsForStreams(t *testing.T) {
	d, a := newPair(t, "tcp", Config{DrainTimeout: 2 * time.Second}, Config{})
	serveEcho(t, a)
	s, _ := d.Open(testParams())
	_, _ = s.Write([]byte("abc"))
	done := make(chan error, 1)
	go func() { done <- d.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Shutdown returned with a stream open")
	default:
	}
	_ = s.CloseWrite()
	_, _ = io.Copy(io.Discard, s)
	_ = s.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(testWait):
		t.Fatal("Shutdown did not return")
	}
}

func TestDeadlines(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()

	_ = s.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	start := time.Now()
	if _, err := s.Read(make([]byte, 1)); !isTimeout(err) {
		t.Fatalf("Read = %v, want a timeout", err)
	}
	if time.Since(start) > testWait {
		t.Fatal("slow")
	}
	// The stream survives a timeout and the deadline can be cleared.
	_ = s.SetReadDeadline(time.Time{})
	go func() { time.Sleep(20 * time.Millisecond); _, _ = in.Write([]byte("z")) }()
	buf := make([]byte, 1)
	if _, err := io.ReadFull(s, buf); err != nil || buf[0] != 'z' {
		t.Fatalf("read after timeout: %v", err)
	}

	// A write blocked on the peer's credit times out.
	_ = s.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	big := make([]byte, 4*DefaultCarrierWindow)
	n, err := s.Write(big)
	if !isTimeout(err) || n >= len(big) {
		t.Fatalf("Write = %d, %v, want a timeout", n, err)
	}
	// An expired deadline fails at once.
	_ = s.SetDeadline(time.Now().Add(-time.Second))
	if _, err := s.Read(buf); !isTimeout(err) {
		t.Fatal(err)
	}
	if _, err := s.Write(buf); !isTimeout(err) {
		t.Fatal(err)
	}
}

func TestStreamKindMismatch(t *testing.T) {
	d, _ := newPair(t, "tcp", Config{}, Config{})
	tcp, _ := d.Open(testParams())
	udp, _ := d.Open(OpenParams{Kind: StreamUDP, RouteID: "r1"})
	if _, err := udp.Read(nil); !errors.Is(err, ErrStreamKind) {
		t.Fatal(err)
	}
	if _, err := udp.Write(nil); !errors.Is(err, ErrStreamKind) {
		t.Fatal(err)
	}
	if err := tcp.WriteDatagram([]byte("x")); !errors.Is(err, ErrStreamKind) {
		t.Fatal(err)
	}
	if _, err := tcp.ReadDatagram(); !errors.Is(err, ErrStreamKind) {
		t.Fatal(err)
	}
}

func TestWrongRole(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	if _, err := a.Open(testParams()); !errors.Is(err, ErrWrongRole) {
		t.Fatal(err)
	}
	if _, err := d.Accept(ctxTimeout(t)); !errors.Is(err, ErrWrongRole) {
		t.Fatal(err)
	}
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	if err := s.Accept(); !errors.Is(err, ErrWrongRole) {
		t.Fatal(err)
	}
	if err := s.Reject(ResultPaused); !errors.Is(err, ErrWrongRole) {
		t.Fatal(err)
	}
	if err := in.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrWrongRole) {
		t.Fatal(err)
	}
}

func TestAcceptHonoursContext(t *testing.T) {
	_, a := newPair(t, "tcp", Config{}, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := a.Accept(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestStreamIDsAreOddIncreasingAndExhaust(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	for want := uint64(1); want < 20; want += 2 {
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		if s.ID() != want {
			t.Fatalf("stream id %d, want %d", s.ID(), want)
		}
		in, _ := a.Accept(ctxTimeout(t))
		_ = in.Reject(ResultPaused)
	}
	d.mu.Lock()
	d.nextID = MaxStreamID
	d.mu.Unlock()
	s, err := d.Open(testParams())
	if err != nil || s.ID() != MaxStreamID {
		t.Fatalf("last id: %v %v", s, err)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrIDsExhausted) {
		t.Fatalf("Open past the last id = %v", err)
	}
	in, err := a.Accept(ctxTimeout(t))
	if err != nil || in.ID() != MaxStreamID {
		t.Fatalf("listener got %v %v", in, err)
	}
}

func TestDatagrams(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		s, err := d.Open(OpenParams{Kind: StreamUDP, RouteID: "r1", Client: netip.MustParseAddrPort("192.0.2.9:5000")})
		if err != nil {
			t.Fatal(err)
		}
		in, _ := a.Accept(ctxTimeout(t))
		msgs := [][]byte{[]byte("a"), pattern(1200, 1), []byte("ccc"), pattern(DefaultMaxFrame, 2)}
		for _, m := range msgs {
			if err := s.WriteDatagram(m); err != nil {
				t.Fatal(err)
			}
		}
		for i, m := range msgs {
			got, err := in.ReadDatagram()
			if err != nil {
				t.Fatal(err)
			}
			eqBytes(t, got, m)
			_ = i
		}
		// answers flow back, the stream is answered implicitly
		if err := in.WriteDatagram([]byte("pong")); err != nil {
			t.Fatal(err)
		}
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		if got, err := s.ReadDatagram(); err != nil || string(got) != "pong" {
			t.Fatalf("%q %v", got, err)
		}
		if err := s.WriteDatagram(nil); !errors.Is(err, ErrDatagramSize) {
			t.Fatal(err)
		}
		if err := s.WriteDatagram(make([]byte, DefaultMaxFrame+1)); !errors.Is(err, ErrDatagramSize) {
			t.Fatal(err)
		}
		_ = s.CloseWrite()
		if _, err := in.ReadDatagram(); err != io.EOF {
			t.Fatalf("after FIN: %v", err)
		}
		if st := s.Carrier().Stats(); st.DatagramsSent != uint64(len(msgs)) {
			t.Fatalf("stats %+v", st)
		}
	})
}

// A datagram never waits for credit: with the listener not reading, the
// stream's window fills and further datagrams are dropped and counted.
func TestDatagramsDropWhenTheWindowIsFull(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{StreamWindow: 16 * 1024, CarrierWindow: 64 * 1024})
	s, _ := d.Open(OpenParams{Kind: StreamUDP, RouteID: "r1"})
	in, _ := a.Accept(ctxTimeout(t))
	sent, dropped := 0, 0
	msg := make([]byte, 1000)
	for range 100 {
		switch err := s.WriteDatagram(msg); {
		case err == nil:
			sent++
		case errors.Is(err, ErrDatagramDropped):
			dropped++
		default:
			t.Fatal(err)
		}
	}
	if dropped == 0 || sent == 0 || sent*len(msg) > 16*1024 {
		t.Fatalf("sent %d dropped %d", sent, dropped)
	}
	if d.Stats().DatagramsDropped != uint64(dropped) {
		t.Fatalf("stats %+v", d.Stats())
	}
	// Reading frees the window again.
	for range sent {
		if _, err := in.ReadDatagram(); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "window to reopen", func() bool { return s.WriteDatagram(msg) == nil })
}

func TestDataIsNotAcceptedOnTheWrongKind(t *testing.T) {
	c, p := newRawPeer(t, "tcp", RoleAcceptor, Config{}, DefaultSettings())
	payload, _ := OpenParams{Kind: StreamUDP, RouteID: "r"}.MarshalBinary()
	p.send(Frame{Type: TypeOpen, StreamID: 1, Payload: payload})
	p.send(Frame{Type: TypeData, StreamID: 1, Payload: []byte("tcp data on a udp stream")})
	f := p.recvType(TypeReset)
	if r, _ := parseReset(f.Payload); f.StreamID != 1 || r != ResetProtocolError {
		t.Fatalf("got %v on %d", r, f.StreamID)
	}
	if c.Err() != nil {
		t.Fatal(c.Err())
	}
}

func TestConcurrentStreamsKeepTheirData(t *testing.T) {
	eachConn(t, func(t *testing.T, kind string) {
		d, a := newPair(t, kind, Config{}, Config{})
		serveEcho(t, a)
		var wg sync.WaitGroup
		for i := range 40 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				data := pattern(1000+i*7919, byte(i))
				eqBytes(t, roundTrip(t, d, data), data)
			}()
		}
		wg.Wait()
		waitFor(t, "streams to finish", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	})
}

func TestAwaitResultSurvivesALaterReset(t *testing.T) {
	d, a := newPair(t, "tcp", Config{}, Config{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	if err := in.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	if !s.Answered() {
		t.Fatal("not Answered after RESULT")
	}
	if err := in.Reset(ResetInternal); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stream reset", func() bool { return d.ActiveStreams() == 0 })
	// The answer was success, whatever happened to the stream afterwards.
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatalf("AwaitResult after a later reset = %v", err)
	}
}

// Close on a stream the peer has not finished resets it and drops what is
// still queued; after CloseWrite and the peer's FIN it is graceful.
func TestCloseSemantics(t *testing.T) {
	t.Run("peer unfinished: reset", func(t *testing.T) {
		d, a := newPair(t, "tcp", Config{}, Config{})
		s, _ := d.Open(testParams())
		in, _ := a.Accept(ctxTimeout(t))
		_ = in.Accept()
		_ = s.Close()
		_, err := in.Read(make([]byte, 1))
		var re *StreamResetError
		if !errors.As(err, &re) || re.Reason != ResetCancel || !re.Remote {
			t.Fatalf("peer read = %v", err)
		}
		if _, err := s.Write([]byte("x")); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("Write after Close = %v", err)
		}
		if _, err := s.Read(make([]byte, 1)); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Read after Close = %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("peer finished: queued data is delivered", func(t *testing.T) {
		d, a := newPair(t, "tcp", Config{}, Config{})
		s, _ := d.Open(testParams())
		in, _ := a.Accept(ctxTimeout(t))
		_ = in.Accept()
		_ = in.CloseWrite()
		waitFor(t, "FIN", func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			return s.(*ConnStream).remoteFin
		})
		_, _ = s.Write([]byte("the last words"))
		_ = s.Close()
		got, err := io.ReadAll(in)
		if err != nil || string(got) != "the last words" {
			t.Fatalf("peer read %q, %v", got, err)
		}
		waitFor(t, "streams gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	})
}
