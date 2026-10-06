package relay

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

func TestQUICCarrierEndToEnd(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	serveEcho(t, a)
	if d.Type() != CarrierQUIC || a.Type() != CarrierQUIC || CarrierQUIC.String() != "quic" {
		t.Fatalf("types %v %v", d.Type(), a.Type())
	}
	if d.Role() != RoleDialer || a.Role() != RoleAcceptor {
		t.Fatalf("roles %v %v", d.Role(), a.Role())
	}
	if p := d.Peer(); p.Identity != nodeID("forward-2") || !p.Encrypted() {
		t.Fatalf("dialler's peer %+v", p)
	}
	if p := a.Peer(); p.Identity != nodeID("forward-1") || !p.Encrypted() {
		t.Fatalf("listener's peer %+v", p)
	}
	// each end knows the other's limits before the first stream
	if got, want := d.PeerSettings(), a.LocalSettings(); got != want {
		t.Fatalf("dialler sees the listener's settings as %+v, listener announced %+v", got, want)
	}
	if got, want := a.PeerSettings(), d.LocalSettings(); got != want {
		t.Fatalf("listener sees the dialler's settings as %+v, dialler announced %+v", got, want)
	}
	if d.PeerSettings().MaxStreams != DefaultMaxStreams || a.PeerSettings().MaxStreams != 0 {
		t.Fatalf("stream limits %d %d", d.PeerSettings().MaxStreams, a.PeerSettings().MaxStreams)
	}
	if !d.NativeDatagrams() || !a.NativeDatagrams() || d.Conn() == nil {
		t.Fatal("datagrams or connection accessor")
	}
	if d.LocalAddr() == nil || d.RemoteAddr().String() != e.l.Addr().String() {
		t.Fatalf("addresses %v %v", d.LocalAddr(), d.RemoteAddr())
	}
	for _, n := range []int{0, 1, 100_000, 3 * DefaultStreamWindow} {
		data := pattern(n, byte(n))
		eqBytes(t, roundTrip(t, d, data), data)
	}
	waitFor(t, "the streams to finish", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	st := d.Stats()
	if st.StreamsOpened != 4 || st.BytesSent < 100_000 || st.RTT < 0 {
		t.Fatalf("stats %+v", st)
	}
	if st := a.Stats(); st.StreamsAccepted != 4 || st.BytesRecv < 100_000 {
		t.Fatalf("stats %+v", st)
	}
	waitFor(t, "the counters to settle", func() bool {
		st := e.l.Stats()
		return st.Carriers == 1 && st.Accepted == 1 && st.Link.Accepted == 1
	})
	if e.l.Link() == nil || e.l.Addr() == nil {
		t.Fatal("accessors")
	}
}

// The dialler learns from DialQUIC that the listener refused it, with the
// reason, as with TLS.
func TestQUICCarrierRefusals(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	other := linkCreds(t, e.ca, e.ca.CAPEM(), "forward-3") // genuine, but not listed
	_, err := e.dial(other, QUICConfig{})
	if err == nil || link.ReasonOf(err) != link.ReasonRemoteRejected {
		t.Fatalf("err = %v (reason %v), want the listener's refusal", err, link.ReasonOf(err))
	}
	var he *link.HandshakeError
	if !errors.As(err, &he) {
		t.Fatalf("%T is not a *link.HandshakeError", err)
	}
	waitFor(t, "the refusal to be counted", func() bool { return e.l.Stats().Link.Failures[link.ReasonPeerNotAllowed] == 1 })
	if st := e.l.Stats(); st.Accepted != 0 || st.Carriers != 0 {
		t.Fatalf("stats %+v", st)
	}

	// a listener that is not the node the dialler pins
	creds := linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1")
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	_, err = DialQUIC(ctx, e.l.Addr().String(), link.DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: nodeID("forward-9")}, QUICConfig{})
	if link.ReasonOf(err) != link.ReasonIdentityMismatch {
		t.Fatalf("err = %v (reason %v)", err, link.ReasonOf(err))
	}
	// another ALPN protocol is a configuration error
	if _, err := DialQUIC(ctx, e.l.Addr().String(), link.DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: nodeID("forward-2"), Protocol: "anixops/1"}, QUICConfig{}); err == nil {
		t.Fatal("a dial with another ALPN protocol ran")
	}
	if _, err := ListenQUIC("127.0.0.1:0", link.ListenerConfig{Credentials: e.server, Protocol: "anixops/1", Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, QUICConfig{}); err == nil {
		t.Fatal("a listener with another ALPN protocol started")
	}
	// bad carrier configurations
	for _, bad := range []QUICConfig{
		{Config: Config{MaxFrame: 10}},
		{Config: Config{PingInterval: time.Minute, IdleTimeout: time.Second}},
		{StreamWindowCeiling: MaxStreamWindow + 1},
		{CarrierWindowCeiling: MaxCarrierWindow + 1},
	} {
		if _, err := e.dial(creds, bad); err == nil {
			t.Fatalf("a dial with %+v ran", bad)
		}
		if _, err := ListenQUIC("127.0.0.1:0", link.ListenerConfig{Credentials: e.server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, bad); err == nil {
			t.Fatalf("a listener with %+v started", bad)
		}
	}
	if _, err := (QUICConfig{Config: Config{MaxFrame: 10}}).Transport(RoleDialer); err == nil {
		t.Fatal("Transport accepted a bad configuration")
	}
}

func TestQUICOpenParamsReachTheListener(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	want := OpenParams{Kind: StreamTCP, RouteID: "01HZZZZZZZZZZZZZZZZZZZZZZZ", HopIndex: 3, Client: netipAP("198.51.100.7:4040")}
	s, err := d.Open(want)
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind() != StreamTCP || s.Params() != want || s.Carrier() != Carrier(d) {
		t.Fatalf("dialler's stream: %v %+v", s.Kind(), s.Params())
	}
	got, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Params() != want || got.ID() != s.ID() || got.Kind() != StreamTCP || got.Carrier() != Carrier(a) {
		t.Fatalf("listener sees %+v id %d (dialler's %d)", got.Params(), got.ID(), s.ID())
	}
	// the dialler cannot accept and the listener cannot open
	if _, err := d.Accept(ctxTimeout(t)); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("dialler Accept = %v", err)
	}
	if _, err := a.Open(want); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("listener Open = %v", err)
	}
	if err := s.Accept(); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("dialler Accept = %v", err)
	}
	if err := s.Reject(ResultPaused); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("dialler Reject = %v", err)
	}
	if err := got.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("listener AwaitResult = %v", err)
	}
	if err := got.Reject(ResultOK); err == nil {
		t.Fatal("Reject(ResultOK)")
	}
	// invalid parameters never go out
	if _, err := d.Open(OpenParams{Kind: 9, RouteID: "x"}); !errors.Is(err, ErrInvalidOpen) {
		t.Fatalf("invalid OPEN = %v", err)
	}
	if err := got.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := got.Accept(); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("second Accept = %v", err)
	}
	if err := got.Reject(ResultPaused); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("Reject after Accept = %v", err)
	}
	if err := s.AwaitResult(ctxTimeout(t)); err != nil || !s.Answered() {
		t.Fatalf("AwaitResult = %v answered %v", err, s.Answered())
	}
}

// Every stream is answered exactly once: explicitly, or by what the
// application does with it, and a refusal reaches the dialler as a code it can
// retry on.
func TestQUICStreamsAreAnswered(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})

	open := func() (Stream, Stream) {
		t.Helper()
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		return s, in
	}

	t.Run("reject", func(t *testing.T) {
		s, in := open()
		_, _ = s.Write([]byte("first bytes, kept by the dialler for a retry"))
		if err := in.Reject(ResultUpstreamUnreachable); err != nil {
			t.Fatal(err)
		}
		var re *ResultError
		if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Code != ResultUpstreamUnreachable || !s.Answered() {
			t.Fatalf("AwaitResult = %v answered %v", err, s.Answered())
		}
		if _, err := s.Read(make([]byte, 1)); !errors.As(err, &re) {
			t.Fatalf("Read = %v", err)
		}
		if _, err := s.Write([]byte("x")); !errors.As(err, &re) {
			t.Fatalf("Write = %v", err)
		}
		if _, err := in.Write([]byte("x")); err == nil {
			t.Fatal("Write on a rejected stream")
		}
		waitFor(t, "the streams to be gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
		if st := a.Stats(); st.ResultFailures[ResultUpstreamUnreachable] != 1 {
			t.Fatalf("stats %+v", st.ResultFailures)
		}
		if st := d.Stats(); st.ResultFailures[ResultUpstreamUnreachable] != 1 {
			t.Fatalf("stats %+v", st.ResultFailures)
		}
	})

	t.Run("accept then data", func(t *testing.T) {
		s, in := open()
		if err := in.Accept(); err != nil {
			t.Fatal(err)
		}
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		go func() { _, _ = in.Write([]byte("hello")) }()
		buf := make([]byte, 5)
		_ = s.SetReadDeadline(time.Now().Add(testWait))
		if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "hello" {
			t.Fatalf("%q %v", buf, err)
		}
		_ = s.Close()
		_ = in.Close()
	})

	t.Run("writing answers", func(t *testing.T) {
		s, in := open()
		go func() { _, _ = in.Write([]byte("server first")) }()
		buf := make([]byte, len("server first"))
		_ = s.SetReadDeadline(time.Now().Add(testWait))
		if _, err := io.ReadFull(s, buf); err != nil || string(buf) != "server first" {
			t.Fatalf("%q %v", buf, err)
		}
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
		_ = in.Close()
	})

	t.Run("CloseWrite answers", func(t *testing.T) {
		s, in := open()
		if err := in.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatal(err)
		}
		_ = s.SetReadDeadline(time.Now().Add(testWait))
		if _, err := s.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("Read = %v, want EOF", err)
		}
		_ = s.Close()
		_ = in.Close()
	})

	t.Run("Close without an answer is an internal error", func(t *testing.T) {
		s, in := open()
		_ = in.Close()
		var re *ResultError
		if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Code != ResultInternal {
			t.Fatalf("AwaitResult = %v", err)
		}
	})

	t.Run("Reset without an answer is an internal error", func(t *testing.T) {
		s, in := open()
		_ = in.Reset(ResetCancel)
		var re *ResultError
		if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Code != ResultInternal {
			t.Fatalf("AwaitResult = %v", err)
		}
	})

	waitFor(t, "every stream to be gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
}

func TestQUICHalfClose(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, err := d.Open(testParams())
	if err != nil {
		t.Fatal(err)
	}
	in, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = in.Accept()

	// the dialler's half-close reaches the listener as EOF, and the listener
	// can still send
	if _, err := s.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseWrite(); err != nil {
		t.Fatalf("second CloseWrite = %v", err)
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("Write after CloseWrite = %v", err)
	}
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	got, err := io.ReadAll(in)
	if err != nil || string(got) != "request" {
		t.Fatalf("listener read %q, %v", got, err)
	}
	if _, err := in.Write([]byte("response")); err != nil {
		t.Fatal(err)
	}
	if err := in.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	_ = s.SetReadDeadline(time.Now().Add(testWait))
	got, err = io.ReadAll(s)
	if err != nil || string(got) != "response" {
		t.Fatalf("dialler read %q, %v", got, err)
	}
	// both directions finished, both ends read to EOF: a graceful close
	_ = s.Close()
	_ = in.Close()
	waitFor(t, "the streams to finish", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
	if st := d.Stats(); st.ResetsSent != [numCodes]uint64{} {
		t.Fatalf("a graceful stream was reset: %+v", st.ResetsSent)
	}
}

func TestQUICResets(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	open := func() (Stream, Stream) {
		t.Helper()
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = in.Accept()
		_ = s.AwaitResult(ctxTimeout(t))
		return s, in
	}

	t.Run("peer reset", func(t *testing.T) {
		s, in := open()
		_ = s.Reset(ResetPeerReset)
		var re *StreamResetError
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.Read(make([]byte, 1)); !errors.As(err, &re) || re.Reason != ResetPeerReset || !re.Remote {
			t.Fatalf("Read = %v", err)
		}
		if _, err := in.Write([]byte("x")); err == nil {
			t.Fatal("Write after the peer's reset")
		}
		if _, err := s.Read(make([]byte, 1)); !errors.As(err, &re) || re.Remote {
			t.Fatalf("local Read = %v", err)
		}
		if st := d.Stats(); st.ResetsSent[ResetPeerReset] == 0 {
			t.Fatalf("resets sent %+v", st.ResetsSent)
		}
		waitFor(t, "the reset to be counted", func() bool { return a.Stats().ResetsReceived[ResetPeerReset] == 1 })
	})

	t.Run("Close on an unfinished stream resets it", func(t *testing.T) {
		s, in := open()
		_ = s.Close()
		var re *StreamResetError
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.Read(make([]byte, 1)); !errors.As(err, &re) || re.Reason != ResetCancel {
			t.Fatalf("Read = %v", err)
		}
		if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("Read after Close = %v", err)
		}
		if _, err := s.Write([]byte("x")); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("Write after Close = %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.Reset(ResetCancel); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Close after the peer finished sends a FIN", func(t *testing.T) {
		s, in := open()
		_ = s.CloseWrite()
		_ = in.SetReadDeadline(time.Now().Add(testWait))
		if _, err := in.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("Read = %v, want EOF", err)
		}
		if _, err := in.Write([]byte("the last words")); err != nil {
			t.Fatal(err)
		}
		_ = in.Close() // the peer has finished: FIN, not a reset
		_ = s.SetReadDeadline(time.Now().Add(testWait))
		got, err := io.ReadAll(s)
		if err != nil || string(got) != "the last words" {
			t.Fatalf("read %q, %v", got, err)
		}
	})

	t.Run("a reset reason this version does not know is a cancellation", func(t *testing.T) {
		if resetReasonOf(0) != ResetCancel || resetReasonOf(99) != ResetCancel || resetReasonOf(5) != ResetPeerReset {
			t.Fatal("resetReasonOf")
		}
	})
	waitFor(t, "every stream to be gone", func() bool { return d.ActiveStreams() == 0 && a.ActiveStreams() == 0 })
}

// Deadlines work like a net.Conn's.
func TestQUICDeadlines(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	_ = s.AwaitResult(ctxTimeout(t))

	_ = s.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, err := s.Read(make([]byte, 1)); !isTimeout(err) {
		t.Fatalf("Read = %v, want a timeout", err)
	}
	_ = s.SetReadDeadline(time.Time{})
	_ = s.SetDeadline(time.Now().Add(-time.Second))
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, errDeadlineExceeded) {
		t.Fatalf("Read = %v", err)
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, errDeadlineExceeded) {
		t.Fatalf("Write = %v", err)
	}
	_ = s.SetDeadline(time.Time{})
	go func() { _, _ = in.Write([]byte("ok")) }()
	buf := make([]byte, 2)
	_ = s.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatal(err)
	}
}

// A dialler's stream waits for the listener's RESULT before it reads, and a
// read deadline applies to that wait too.
func TestQUICReadWaitsForTheResult(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	_ = s.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if _, err := s.Read(make([]byte, 1)); !isTimeout(err) {
		t.Fatalf("Read before RESULT = %v, want a timeout", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := s.AwaitResult(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AwaitResult = %v", err)
	}
	if s.Answered() {
		t.Fatal("answered")
	}
	_ = in.Accept()
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
}

// A listener-side stream's read before the application answers delivers what
// the dialler sent; the dialler sent it right behind OPEN.
func TestQUICFirstBytesFollowOpen(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	s, _ := d.Open(testParams())
	if _, err := s.Write([]byte("GET / HTTP/1.1")); err != nil {
		t.Fatal(err)
	}
	in, _ := a.Accept(ctxTimeout(t))
	buf := make([]byte, len("GET / HTTP/1.1"))
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(in, buf); err != nil || string(buf) != "GET / HTTP/1.1" {
		t.Fatalf("%q %v", buf, err)
	}
	if s.Answered() {
		t.Fatal("answered before the listener said so")
	}
}

func TestQUICStreamLimit(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{Config: Config{MaxStreams: 2}})
	d, a := e.pair(QUICConfig{})
	var open []Stream
	for range 2 {
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = in.Accept()
		open = append(open, s, in)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("Open at the limit = %v", err)
	}
	// finishing a stream gives the slot back
	_ = open[0].Close()
	_ = open[1].Close()
	waitFor(t, "a free slot", func() bool {
		s, err := d.Open(testParams())
		if err == nil {
			_ = s.Close()
		}
		return err == nil
	})
}

func TestQUICAcceptQueueIsBounded(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{Config: Config{AcceptQueue: 2}})
	d, a := e.pair(QUICConfig{})
	var streams []Stream
	for range 8 {
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, s)
	}
	// two fit the queue, the rest are refused in a way a dialler can retry
	waitFor(t, "the refusals", func() bool { return a.Stats().StreamsRefused == 6 })
	for range 2 {
		in, err := a.Accept(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = in.Accept()
	}
	refused, answered := 0, 0
	for _, s := range streams {
		err := s.AwaitResult(ctxTimeout(t))
		switch {
		case err == nil && s.Answered():
			answered++
		case errors.Is(err, ErrRefused) && !s.Answered():
			refused++
		default:
			t.Fatalf("a stream ended with %v (answered %v)", err, s.Answered())
		}
	}
	if refused != 6 || answered != 2 {
		t.Fatalf("%d streams were refused and %d answered, want 6 and 2", refused, answered)
	}
	if st := a.Stats(); st.StreamsAccepted != 2 || st.ResetsSent[ResetRefusedStream] != 6 {
		t.Fatalf("stats %+v", st)
	}
}

func TestQUICOpenIDsAreNativeStreamIDs(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	for want := uint64(0); want < 5*4; want += 4 {
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
	// ids run out at 2^29 streams (GOAWAY carries a 32-bit id)
	d.mu.Lock()
	d.opened = MaxStreamID / 4
	d.mu.Unlock()
	if _, err := d.Open(testParams()); !errors.Is(err, ErrIDsExhausted) {
		t.Fatalf("Open with the ids used up = %v", err)
	}
}

// QUIC's flow control is end to end: a listener that does not read holds the
// dialler's writes, and a stalled stream does not hold its neighbour.
func TestQUICBackpressure(t *testing.T) {
	cfg := QUICConfig{Config: Config{StreamWindow: MinWindow * 4, CarrierWindow: 1 << 20}, StreamWindowCeiling: MinWindow * 4, CarrierWindowCeiling: 1 << 20}
	e := newQUICEnv(t, cfg)
	d, a := e.pair(cfg)
	stalled, _ := d.Open(testParams())
	inStalled, _ := a.Accept(ctxTimeout(t))
	_ = inStalled.Accept()
	live, _ := d.Open(testParams())
	inLive, _ := a.Accept(ctxTimeout(t))
	_ = inLive.Accept()

	// The first never reads: its writer blocks once the windows are full.
	var wrote atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := stalled.Write(buf)
			wrote.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()
	waitFor(t, "the stalled writer to fill its window", func() bool {
		a := wrote.Load()
		time.Sleep(20 * time.Millisecond)
		return a > 0 && a == wrote.Load()
	})
	if w := wrote.Load(); w < MinWindow*4 || w > 8*MinWindow*4+1<<20 {
		t.Fatalf("a stalled stream took %d bytes; its window is %d", w, MinWindow*4)
	}
	// the neighbour still works
	data := pattern(100_000, 9)
	go func() { _, _ = live.Write(data); _ = live.CloseWrite() }()
	_ = inLive.SetReadDeadline(time.Now().Add(testWait))
	got, err := io.ReadAll(inLive)
	if err != nil {
		t.Fatalf("the neighbour: %v", err)
	}
	eqBytes(t, got, data)
	// reading releases the stalled one
	go func() { _, _ = io.Copy(io.Discard, inStalled) }()
	waitFor(t, "the stalled writer to move on", func() bool { return wrote.Load() > 2*int64(MinWindow*4) })
	_ = stalled.Close()
	<-done
}

// quic_udp_test.go and the following tests need a path whose packets the
// test can drop: a UDP forwarder between a dialler and a listener.
type udpProxy struct {
	front  *net.UDPConn // faces the dialler
	back   *net.UDPConn // faces the listener
	server *net.UDPAddr

	mu     sync.Mutex
	client *net.UDPAddr
	paused atomic.Bool
	closed atomic.Bool
}

func newUDPProxy(t testing.TB, server net.Addr) *udpProxy {
	t.Helper()
	front, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	back, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	p := &udpProxy{front: front, back: back, server: server.(*net.UDPAddr)}
	go p.pump(front, func(b []byte, from *net.UDPAddr) {
		p.mu.Lock()
		p.client = from
		p.mu.Unlock()
		_, _ = back.WriteToUDP(b, p.server)
	})
	go p.pump(back, func(b []byte, _ *net.UDPAddr) {
		p.mu.Lock()
		c := p.client
		p.mu.Unlock()
		if c != nil {
			_, _ = front.WriteToUDP(b, c)
		}
	})
	t.Cleanup(func() { p.closed.Store(true); _ = front.Close(); _ = back.Close() })
	return p
}

func (p *udpProxy) pump(c *net.UDPConn, forward func([]byte, *net.UDPAddr)) {
	buf := make([]byte, 65536)
	for {
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !p.paused.Load() {
			forward(buf[:n], from)
		}
	}
}

func (p *udpProxy) addr() string { return p.front.LocalAddr().String() }
func (p *udpProxy) pause()       { p.paused.Store(true) }
func (p *udpProxy) resume()      { p.paused.Store(false) }

// dialVia dials the environment's listener through a proxy.
func (e *quicEnv) dialVia(p *udpProxy, qcfg QUICConfig) (*QUICCarrier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	return DialQUIC(ctx, p.addr(), link.DialConfig{Credentials: linkCreds(e.t, e.ca, e.ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}, qcfg)
}

// A carrier that hears nothing is closed at the idle timeout on both ends,
// and says so (L4).
func TestQUICIdleTimeout(t *testing.T) {
	cfg := QUICConfig{Config: Config{PingInterval: 50 * time.Millisecond, IdleTimeout: 400 * time.Millisecond}}
	e := newQUICEnv(t, cfg)
	p := newUDPProxy(t, e.l.Addr())
	d, err := e.dialVia(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := acceptQUIC(t, e.l)
	s, _ := d.Open(testParams())
	_ = s.SetReadDeadline(time.Time{})
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	p.pause() // the path goes dark
	waitDone(t, d)
	waitDone(t, a)
	if !errors.Is(d.Err(), ErrIdleTimeout) || !errors.Is(d.Err(), ErrCarrierClosed) || !errors.Is(a.Err(), ErrIdleTimeout) {
		t.Fatalf("errors: %v / %v", d.Err(), a.Err())
	}
	if d.Stats().KeepaliveTimeout != 1 {
		t.Fatalf("stats %+v", d.Stats())
	}
	// every stream failed with the carrier's error
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Write after the carrier ended = %v", err)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Open = %v", err)
	}
	if _, err := a.Accept(ctxTimeout(t)); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Accept = %v", err)
	}
	if err := d.GoAway(GoAwayNoError, 0); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("GoAway = %v", err)
	}
	waitFor(t, "the listener to forget the carrier", func() bool { return e.l.Stats().Carriers == 0 })
}

// An idle carrier keeps itself alive: QUIC's keepalive packets flow with no
// stream open.
func TestQUICKeepalive(t *testing.T) {
	cfg := QUICConfig{Config: Config{PingInterval: 50 * time.Millisecond, IdleTimeout: 2 * time.Second}}
	e := newQUICEnv(t, cfg)
	d, _ := e.pair(cfg)
	start := d.Conn().ConnectionStats().PacketsSent
	waitFor(t, "keepalive packets", func() bool { return d.Conn().ConnectionStats().PacketsSent >= start+3 })
	if d.RTT() <= 0 || d.Stats().RTT <= 0 {
		t.Fatalf("no round trip time: %v", d.RTT())
	}
}

// The listener's UDP port is free again once Close has returned, and a carrier
// on it never outlives it.
func TestQUICListenerOwnsItsCarriers(t *testing.T) {
	cfg := QUICConfig{Config: Config{DrainTimeout: 2 * time.Second}}
	e := newQUICEnv(t, cfg)
	d, a := e.pair(cfg)
	serveEcho(t, a)
	data := pattern(50_000, 1)
	eqBytes(t, roundTrip(t, d, data), data)
	addr := e.l.Addr().String()
	if err := e.l.Close(); err != nil {
		t.Fatal(err)
	}
	waitDone(t, a)
	waitDone(t, d)
	if r, ok := d.PeerGoAway(); !ok || r != GoAwayListenerClosed {
		t.Fatalf("the dialler was told %v %v, want listener_closed", r, ok)
	}
	if _, err := e.l.Accept(ctxTimeout(t)); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close = %v", err)
	}
	if err := e.l.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	// the port is free
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("the UDP port is still held: %v", err)
	}
	_ = pc.Close()
}

// A closing listener lets streams finish, for at most the drain timeout.
func TestQUICListenerCloseDrainsStreams(t *testing.T) {
	cfg := QUICConfig{Config: Config{DrainTimeout: 300 * time.Millisecond}}
	e := newQUICEnv(t, cfg)
	d, a := e.pair(cfg)
	s, _ := d.Open(testParams())
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	_ = s.AwaitResult(ctxTimeout(t))
	closed := make(chan struct{})
	go func() { _ = e.l.Close(); close(closed) }()
	select {
	case <-d.Draining():
	case <-time.After(testWait):
		t.Fatal("the dialler was not told to drain")
	}
	// the stream keeps working during the drain
	go func() { _, _ = s.Write([]byte("during the drain")) }()
	buf := make([]byte, len("during the drain"))
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.ReadFull(in, buf); err != nil {
		t.Fatalf("a stream died during the drain: %v", err)
	}
	// new streams are not taken
	if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
		t.Fatalf("Open during the drain = %v", err)
	}
	// the stream never ends: Close returns after the drain timeout
	select {
	case <-closed:
	case <-time.After(testWait):
		t.Fatal("Close did not return")
	}
	waitDone(t, d)
	waitDone(t, a)
}

// SetPeers is the revocation path: the carriers of a removed identity close
// with peer_not_allowed, the others are left alone.
func TestQUICSetPeersClosesCarriers(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{}, nodeID("forward-3"))
	d1, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), QUICConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d1.Close() }()
	a1 := acceptQUIC(t, e.l)
	d3, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-3"), QUICConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d3.Close() }()
	a3 := acceptQUIC(t, e.l)
	serveEcho(t, a1)
	serveEcho(t, a3)
	s, _ := d1.Open(testParams())
	_, _ = s.Write([]byte("x"))
	_ = s.AwaitResult(ctxTimeout(t))

	removed, err := e.l.SetPeers([]string{nodeID("forward-3")})
	if err != nil || len(removed) != 1 || removed[0] != nodeID("forward-1") {
		t.Fatalf("removed %v, %v", removed, err)
	}
	waitDone(t, a1)
	waitDone(t, d1)
	if r, ok := d1.PeerGoAway(); !ok || r != GoAwayPeerNotAllowed {
		t.Fatalf("the removed peer was told %v %v", r, ok)
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("a stream on the closed carrier: %v", err)
	}
	// the other peer keeps working
	data := pattern(1000, 3)
	eqBytes(t, roundTrip(t, d3, data), data)
	select {
	case <-a3.Done():
		t.Fatal("an allowed peer's carrier was closed")
	default:
	}
	// and the removed identity cannot come back
	if _, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), QUICConfig{}); link.ReasonOf(err) != link.ReasonRemoteRejected {
		t.Fatalf("redial = %v", err)
	}
	if _, err := e.l.SetPeers(nil); err == nil {
		t.Fatal("SetPeers(nil) succeeded")
	}
}

// A trust bundle change closes the carriers whose peer lost its CA.
func TestQUICCredentialsChange(t *testing.T) {
	t.Run("listener", func(t *testing.T) {
		e := newQUICEnv(t, QUICConfig{})
		ca2, err := relaytest.New("b")
		if err != nil {
			t.Fatal(err)
		}
		d, a := e.pair(QUICConfig{})
		// the listener moves to the next CA and drops the old one
		certPEM, keyPEM, _ := ca2.Issue(relaytest.Cert{Node: "forward-2"})
		if err := e.server.Reload(certPEM, keyPEM, ca2.CAPEM()); err != nil {
			t.Fatal(err)
		}
		waitDone(t, a)
		waitDone(t, d)
		if r, ok := d.PeerGoAway(); !ok || r != GoAwayCredentials {
			t.Fatalf("GOAWAY %v %v", r, ok)
		}
	})

	t.Run("dialler", func(t *testing.T) {
		e := newQUICEnv(t, QUICConfig{})
		ca2, _ := relaytest.New("b")
		creds := linkCreds(t, e.ca, relaytest.Bundle(e.ca, ca2), "forward-1")
		d, err := e.dial(creds, QUICConfig{})
		if err != nil {
			t.Fatal(err)
		}
		a := acceptQUIC(t, e.l)
		defer func() { _ = a.Close() }()
		stop := WatchCredentials(creds, d)
		defer stop()
		// a reload that still trusts the listener's CA changes nothing
		certPEM, keyPEM, _ := e.ca.Issue(relaytest.Cert{Node: "forward-1"})
		if err := creds.Reload(certPEM, keyPEM, relaytest.Bundle(e.ca)); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Open(testParams()); err != nil {
			t.Fatalf("the carrier was closed by a reload that kept its CA: %v", err)
		}
		// the dialler drops the CA its listener's certificate chains to
		cert2, key2, _ := ca2.Issue(relaytest.Cert{Node: "forward-1"})
		if err := creds.Reload(cert2, key2, ca2.CAPEM()); err != nil {
			t.Fatal(err)
		}
		waitDone(t, d)
		waitDone(t, a)
		if r, ok := a.PeerGoAway(); !ok || r != GoAwayCredentials {
			t.Fatalf("GOAWAY %v %v", r, ok)
		}
	})
}

// Close tells the peer why, as the application error code of the close.
func TestQUICCloseTellsThePeer(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	if _, ok := a.PeerGoAway(); ok {
		t.Fatal("a GOAWAY nobody sent")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	waitDone(t, a)
	if r, ok := a.PeerGoAway(); !ok || r != GoAwayShutdown {
		t.Fatalf("PeerGoAway = %v %v", r, ok)
	}
	if !errors.Is(a.Err(), ErrCarrierClosed) {
		t.Fatalf("Err = %v", a.Err())
	}
	waitFor(t, "the listener to forget the carrier", func() bool { return e.l.Stats().Carriers == 0 })

	d2, a2 := e.pair(QUICConfig{})
	_ = d2.CloseWithReason(GoAwayCarrierAge)
	waitDone(t, a2)
	if r, _ := a2.PeerGoAway(); r != GoAwayCarrierAge {
		t.Fatalf("reason %v", r)
	}
}

// A GOAWAY retires a carrier gracefully: the streams it has continue, nothing
// new starts, and when both ends have no streams left the carrier closes.
func TestQUICGoAwayDrains(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first func(d, a *QUICCarrier) error
	}{
		{"dialler retires", func(d, a *QUICCarrier) error { return d.GoAway(GoAwayCarrierAge, 0) }},
		{"listener retires", func(d, a *QUICCarrier) error { return a.GoAway(GoAwayNoError, 0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, QUICConfig{})
			d, a := e.pair(QUICConfig{})
			serveEcho(t, a)
			s, err := d.Open(testParams())
			if err != nil {
				t.Fatal(err)
			}
			_, _ = s.Write([]byte("before"))
			if err := s.AwaitResult(ctxTimeout(t)); err != nil {
				t.Fatal(err)
			}
			if err := tc.first(d, a); err != nil {
				t.Fatal(err)
			}
			for _, c := range []*QUICCarrier{d, a} {
				select {
				case <-c.Draining():
				case <-time.After(testWait):
					t.Fatal("not draining")
				}
			}
			if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
				t.Fatalf("Open on a draining carrier = %v", err)
			}
			// the stream still carries data both ways
			_, _ = s.Write([]byte(" and after"))
			_ = s.CloseWrite()
			_ = s.SetReadDeadline(time.Now().Add(testWait))
			got, err := io.ReadAll(s)
			if err != nil || string(got) != "before and after" {
				t.Fatalf("echo %q, %v", got, err)
			}
			select {
			case <-d.Done():
				t.Fatal("the carrier closed with a stream still open")
			default:
			}
			_ = s.Close()
			waitDone(t, d)
			waitDone(t, a)
			if !errors.Is(d.Err(), ErrCarrierClosed) {
				t.Fatalf("Err = %v", d.Err())
			}
		})
	}
}

// With no streams at all a GOAWAY closes the carrier at once, and a drain
// timeout bounds a drain whose peer never gets quiet.
func TestQUICGoAwayWithoutStreams(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	d, a := e.pair(QUICConfig{})
	if err := d.GoAway(GoAwayNoError, 0); err != nil {
		t.Fatal(err)
	}
	waitDone(t, d)
	waitDone(t, a)
	if r, ok := a.PeerGoAway(); !ok || r != GoAwayNoError {
		t.Fatalf("PeerGoAway = %v %v", r, ok)
	}

	// a drain timeout cuts a stream that never ends
	d2, a2 := e.pair(QUICConfig{})
	s, _ := d2.Open(testParams())
	in, _ := a2.Accept(ctxTimeout(t))
	_ = in.Accept()
	_ = s.AwaitResult(ctxTimeout(t))
	start := time.Now()
	_ = d2.GoAway(GoAwayNoError, 150*time.Millisecond)
	waitDone(t, d2)
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("the carrier closed after %v, before its drain timeout", time.Since(start))
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrCarrierClosed) {
		t.Fatalf("Write after the drain = %v", err)
	}
	waitDone(t, a2)

	// Shutdown waits for the streams
	d3, a3 := e.pair(QUICConfig{})
	serveEcho(t, a3)
	data := pattern(1000, 5)
	eqBytes(t, roundTrip(t, d3, data), data)
	if err := d3.Shutdown(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	waitDone(t, d3)
	if err := d3.Shutdown(ctxTimeout(t)); err != nil {
		t.Fatalf("Shutdown of an ended carrier = %v", err)
	}
	// and gives up at the context
	d4, a4 := e.pair(QUICConfig{})
	s4, _ := d4.Open(testParams())
	in4, _ := a4.Accept(ctxTimeout(t))
	_ = in4.Accept()
	_ = s4.AwaitResult(ctxTimeout(t))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := d4.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown = %v", err)
	}
	waitDone(t, d4)
}

// A stream's first bytes race a GOAWAY: an OPEN that arrives after the
// carrier started to drain is refused so the dialler can retry elsewhere, and
// is not charged against the peer.
func TestQUICOpenRacingGoAway(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	if err := a.GoAway(GoAwayNoError, 0); err != nil {
		t.Fatal(err)
	}
	if r := p.expectGoAway(); r != GoAwayNoError {
		t.Fatalf("GOAWAY %v", r)
	}
	// more OPENs than the budget of answers: draining is not misbehaviour
	for range 3 * calmBurst {
		st, err := p.conn.OpenStream()
		if err != nil {
			// the stream limit of the connection: wait for slots
			break
		}
		payload, _ := testParams().MarshalBinary()
		_, _ = st.Write(quicFrameBytes(TypeOpen, payload))
		expectReset(t, st, ResetRefusedStream)
	}
	select {
	case <-a.Done():
		t.Fatalf("the carrier ended: %v", a.Err())
	default:
	}
	if a.Stats().StreamsRefused == 0 {
		t.Fatal("nothing was refused")
	}
}

// The QUIC carrier's L3 for crashes: a listener that restarts with its
// stateless reset key ends its peers' carriers at once.
func TestQUICStatelessResetEndsTheCarrier(t *testing.T) {
	var key [32]byte
	copy(key[:], "relay test stateless reset key.")
	cfg := QUICConfig{StatelessResetKey: &key, Config: Config{PingInterval: 100 * time.Millisecond, IdleTimeout: 20 * time.Second}}
	e := newQUICEnv(t, cfg)
	d, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	_ = acceptQUIC(t, e.l)
	addr := e.l.Addr().String()
	// "crash": the process goes away without closing anything gracefully
	// (the listener's Close would drain), and its replacement takes the port.
	// The unit's sockets are closed by the kernel; here the underlying link
	// listener is closed abruptly.
	_ = e.l.Link().Close()
	var l2 *QUICListener
	waitFor(t, "the port to be free", func() bool {
		var err error
		l2, err = ListenQUIC(addr, link.ListenerConfig{Credentials: e.server, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}}, cfg)
		return err == nil
	})
	defer func() { _ = l2.Close() }()
	// A stateless reset answers a packet the new listener cannot place, and
	// only one longer than the reset itself: quic-go sends none for packets of
	// 42 bytes or less, such as keep-alive PINGs, so an idle carrier ends in
	// the idle timeout instead. The dialler's larger packets right after the
	// restart (a path MTU probe, retransmitted data) may fall into the moment
	// before the new listener holds the port, so, like a carrier in use, the
	// dialler keeps sending until it is reset.
	sendDone := make(chan struct{})
	var sendWG sync.WaitGroup
	sendWG.Add(1)
	go func() {
		defer sendWG.Done()
		for {
			select {
			case <-d.Done():
				return
			case <-sendDone:
				return
			case <-time.After(50 * time.Millisecond):
				_ = d.qc.SendDatagram(make([]byte, 100))
			}
		}
	}()
	defer func() { close(sendDone); sendWG.Wait() }()
	waitDone(t, d) // far sooner than the 20 s idle timeout
	if errors.Is(d.Err(), ErrIdleTimeout) {
		t.Fatalf("the carrier idled out instead of being reset: %v", d.Err())
	}
}

// The carriers a TLS listener and a QUIC listener share a hop's port number.
func TestQUICDialerHasNoDestination(t *testing.T) {
	// OPEN carries no destination: a listener sends a stream only where its
	// own hop does. Here the "hop" is the test's accept loop, which is the
	// only thing that ever decides; there is nothing in OpenParams to steer.
	p := OpenParams{Kind: StreamTCP, RouteID: "r", HopIndex: 1}
	b, err := p.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > maxOpenPayload {
		t.Fatalf("an OPEN payload is %d bytes, the carrier's limit is %d", len(b), maxOpenPayload)
	}
}
