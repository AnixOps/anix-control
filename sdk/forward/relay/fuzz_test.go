package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/netip"
	"testing"
	"time"
)

// Fuzz targets. `go test` runs each with its seed corpus (the f.Add inputs
// and anything under testdata/fuzz) and stays fast; run one longer with
//
//	go test -fuzz=FuzzReadFrame -fuzztime=30s ./forward/relay
//
// from the sdk module (anixops-protocol.md section 6.6: the frame decoder,
// OPEN parameters and SETTINGS, plus the carrier state machine behind them).

func seedFrames(t testing.TB) [][]byte {
	open, _ := testParams().MarshalBinary()
	udp, _ := OpenParams{Kind: StreamUDP, RouteID: "r", Client: netip.MustParseAddrPort("192.0.2.1:53")}.MarshalBinary()
	cat := func(fs ...Frame) []byte {
		var b []byte
		for _, f := range fs {
			var err error
			if b, err = AppendFrame(b, f); err != nil {
				t.Fatal(err)
			}
		}
		return b
	}
	return [][]byte{
		nil,
		{0},
		cat(Frame{Type: TypeOpen, StreamID: 1, Payload: open}, Frame{Type: TypeData, StreamID: 1, Payload: []byte("hello")}, Frame{Type: TypeData, Flags: FlagFIN, StreamID: 1}),
		cat(Frame{Type: TypeOpen, StreamID: 1, Payload: udp}, Frame{Type: TypeDatagram, StreamID: 1, Payload: []byte("dns")}, Frame{Type: TypeDatagram, StreamID: 1, Payload: bytes.Repeat([]byte{7}, 512)}),
		cat(Frame{Type: TypeOpen, StreamID: 1, Payload: open}, Frame{Type: TypeOpen, StreamID: 3, Payload: open}, Frame{Type: TypeReset, StreamID: 1, Payload: marshalReset(ResetCancel)}, Frame{Type: TypeWindow, StreamID: 3, Payload: marshalWindow(4096)}, Frame{Type: TypeWindow, Payload: marshalWindow(1 << 20)}),
		cat(Frame{Type: TypePing, Payload: make([]byte, 8)}, Frame{Type: TypePing, Flags: FlagACK, Payload: make([]byte, 8)}, Frame{Type: TypeGoAway, Payload: marshalGoAway(1, GoAwayNoError)}),
		cat(Frame{Type: TypeSettings, Payload: DefaultSettings().Marshal()}),
		cat(Frame{Type: TypeResult, StreamID: 1, Payload: marshalResult(ResultOK)}, Frame{Type: TypeResult, StreamID: 3, Payload: marshalResult(ResultPaused)}),
		cat(Frame{Type: 0x55, Payload: []byte("future")}, Frame{Type: 0x55, StreamID: 1}),
		{0xff, 0xff, byte(TypeData), 0, 0, 0, 0, 1},                     // a huge length, no payload
		{0, 4, byte(TypeWindow), 0, 0, 0, 0, 1, 0xff, 0xff, 0xff, 0xff}, // stream credit overflow
		{0, 4, byte(TypeWindow), 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}, // carrier credit overflow
		{0, 0, byte(TypeOpen), 0, 0, 0, 0, 2},                           // even stream id
		bytes.Repeat([]byte{0xa5}, 64),
	}
}

func FuzzReadFrame(f *testing.F) {
	for _, s := range seedFrames(f) {
		f.Add(s, uint16(1024))
		f.Add(s, uint16(MaxMaxFrame))
	}
	f.Fuzz(func(t *testing.T, data []byte, max uint16) {
		limit := int(max)
		r := bytes.NewReader(data)
		var re []byte
		for {
			fr, err := ReadFrame(r, limit)
			if err != nil {
				if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, ErrFrameTooLarge) {
					t.Fatalf("unexpected error %v", err)
				}
				break
			}
			if len(fr.Payload) > limit {
				t.Fatalf("payload of %d bytes beyond the limit %d", len(fr.Payload), limit)
			}
			var eerr error
			if re, eerr = AppendFrame(re, fr); eerr != nil {
				t.Fatal(eerr)
			}
		}
		// Whatever was parsed re-encodes to exactly the bytes it came from.
		if !bytes.HasPrefix(data, re) {
			t.Fatalf("re-encoding differs from the input")
		}
	})
}

func FuzzParseSettings(f *testing.F) {
	f.Add(DefaultSettings().Marshal())
	f.Add([]byte{})
	f.Add([]byte{0, 9, 0, 0, 0, 1})
	f.Add([]byte{0, 1, 0, 0, 0, 1, 0, 1, 0, 0, 0, 2})
	f.Add(bytes.Repeat([]byte{0xff}, 12))
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := ParseSettings(b)
		if err != nil {
			return
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("accepted settings that do not validate: %v", err)
		}
		again, err := ParseSettings(s.Marshal())
		if err != nil || again != s {
			t.Fatalf("round trip: %+v %v", again, err)
		}
	})
}

func FuzzOpenParams(f *testing.F) {
	for _, p := range []OpenParams{
		testParams(),
		{Kind: StreamUDP, RouteID: "r", HopIndex: 1<<32 - 1, Client: netip.MustParseAddrPort("[2001:db8::1]:443")},
		{Kind: StreamTCP, RouteID: "abc", Client: netip.MustParseAddrPort("203.0.113.7:1")},
	} {
		b, _ := p.MarshalBinary()
		f.Add(b)
	}
	f.Add([]byte{})
	f.Add([]byte{1, 0, 0, 0, 0, 255})
	f.Fuzz(func(t *testing.T, b []byte) {
		var p OpenParams
		if err := p.UnmarshalBinary(b); err != nil {
			return
		}
		// What is accepted is canonical: it encodes to the same bytes.
		again, err := p.MarshalBinary()
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("accepted %x but re-encodes to %x (%v)", b, again, err)
		}
		if n := len(p.RouteID); n == 0 || n > MaxRouteIDLen {
			t.Fatalf("route id of %d bytes accepted", n)
		}
	})
}

func FuzzSmallPayloads(f *testing.F) {
	f.Add([]byte{0, 1})
	f.Add([]byte{0, 0, 0, 1})
	f.Add([]byte{0, 0, 0, 3, 0, 7})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if c, err := parseResult(b); err == nil && !bytes.Equal(marshalResult(c), b) {
			t.Fatal("RESULT")
		}
		if r, err := parseReset(b); err == nil && !bytes.Equal(marshalReset(r), b) {
			t.Fatal("RESET")
		}
		if n, err := parseWindow(b); err == nil && (n == 0 || !bytes.Equal(marshalWindow(n), b)) {
			t.Fatal("WINDOW")
		}
		if l, r, err := parseGoAway(b); err == nil && (l > MaxStreamID || !bytes.Equal(marshalGoAway(l, r), b)) {
			t.Fatal("GOAWAY")
		}
	})
}

// fuzzApp serves the streams a carrier under fuzzing accepts: some are read
// to the end, some refused, some written to, some reset.
func fuzzApp(c *ConnCarrier) {
	for {
		s, err := c.Accept(context.Background())
		if err != nil {
			return
		}
		go func() {
			_ = s.SetDeadline(time.Now().Add(testWait))
			switch s.ID() / 2 % 4 {
			case 0:
				_ = s.Accept()
				_, _ = io.Copy(io.Discard, s)
			case 1:
				_ = s.Reject(ResultPaused)
			case 2:
				_, _ = s.Write(bytes.Repeat([]byte{1}, 3000))
				_, _ = io.Copy(io.Discard, s)
			default:
				_ = s.Accept()
				_ = s.Reset(ResetPeerReset)
			}
			_ = s.Close()
		}()
	}
}

// fuzzPeer feeds arbitrary bytes to a carrier that has completed its
// SETTINGS exchange, and then makes sure nothing hangs or panics.
func fuzzPeer(t *testing.T, role Role, data []byte, prepare func(c *ConnCarrier)) {
	cfg := Config{MaxFrame: 1024, StreamWindow: 4096, CarrierWindow: 8192, MaxStreams: 8, AcceptQueue: 4, SendBuffer: 2048,
		HandshakeTimeout: time.Second, PingInterval: 10 * time.Millisecond, IdleTimeout: 5 * time.Second, ResultTimeout: 100 * time.Millisecond}
	c, p := newRawPeer(t, "pipe", role, cfg, withSettings(func(s *Settings) { s.MaxStreams = 8 }))
	go func() { _, _ = io.Copy(io.Discard, p.conn) }()
	if role == RoleAcceptor {
		go fuzzApp(c)
	}
	if prepare != nil {
		prepare(c)
	}
	_ = p.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = p.conn.Write(data)
	// The carrier either ended on the input or is still running; both are
	// fine. It must close promptly, with its credit within bounds.
	c.mu.Lock()
	if c.recvCredit < 0 || c.recvCredit > int64(c.local.CarrierWindow) || c.sendCredit < 0 || c.sendCredit > maxCreditValue {
		t.Errorf("credit out of bounds: recv %d send %d", c.recvCredit, c.sendCredit)
	}
	if c.active < 0 || c.active > len(c.streams)+1 {
		t.Errorf("active streams %d with %d tracked", c.active, len(c.streams))
	}
	c.mu.Unlock()
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(testWait):
		t.Fatalf("Close hung:\n%s", c.dump())
	}
	if c.ActiveStreams() != 0 {
		t.Fatalf("%d streams left after Close", c.ActiveStreams())
	}
}

func FuzzCarrierAcceptor(f *testing.F) {
	for _, s := range seedFrames(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) { fuzzPeer(t, RoleAcceptor, data, nil) })
}

func FuzzCarrierDialer(f *testing.F) {
	for _, s := range seedFrames(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzPeer(t, RoleDialer, data, func(c *ConnCarrier) {
			for i := range 3 {
				s, err := c.Open(OpenParams{Kind: StreamKind(1 + i%2), RouteID: "r", HopIndex: uint32(i)})
				if err != nil {
					return // retired by L5 on a slow machine: nothing to open
				}
				if s.Kind() == StreamTCP {
					go func() { _, _ = s.Write(bytes.Repeat([]byte{2}, 5000)); _, _ = io.Copy(io.Discard, s) }()
				} else {
					go func() {
						_ = s.WriteDatagram([]byte("dgram"))
						_, _ = s.ReadDatagram()
					}()
				}
			}
		})
	})
}

// FuzzTransfers drives two real carriers with a scenario of streams read from
// the input (see scenarioFromBytes) and checks the model's invariants.
func FuzzTransfers(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0, 0, 100, 100, 0, 1})
	f.Add([]byte{1, 1, 1, 1, 1, 1, 200, 50, 0, 2, 30, 30, 1, 3, 50, 90, 2, 4, 10, 10, 3, 5, 40, 40, 4, 6, 60, 10, 5, 7})
	f.Add([]byte{0, 2, 2, 2, 2, 2, 255, 255, 0, 9, 255, 0, 0, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		sc, ok := scenarioFromBytes(data)
		if !ok {
			t.Skip()
		}
		sc.run(t)
	})
}
