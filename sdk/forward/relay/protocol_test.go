package relay

import (
	"encoding/binary"
	"testing"
	"time"
)

// Rules of the protocol the peer can break, each answered as the document
// says: a carrier-level violation ends the carrier with a GOAWAY of the
// reason, a stream-level one resets the stream and the carrier lives on.
func TestPeerViolations(t *testing.T) {
	type ev struct {
		goAway GoAwayReason // the carrier ends with this GOAWAY ...
		reset  ResetReason  // ... or this stream is reset (stream 'on') and the carrier lives
		on     uint32
		alive  bool
	}
	goAway := func(r GoAwayReason) ev { return ev{goAway: r} }
	reset := func(on uint32, r ResetReason) ev { return ev{reset: r, on: on, alive: true} }
	ok := ev{alive: true}

	open1 := func(p *rawPeer) { p.open(1) }
	frame := func(typ FrameType, id uint32, payload []byte) func(p *rawPeer) {
		return func(p *rawPeer) { p.send(Frame{Type: typ, StreamID: id, Payload: payload}) }
	}
	steps := func(fs ...func(p *rawPeer)) func(p *rawPeer) {
		return func(p *rawPeer) {
			for _, f := range fs {
				f(p)
			}
		}
	}

	tests := []struct {
		name string
		role Role
		cfg  Config
		do   func(p *rawPeer)
		want ev
	}{
		// carrier-level
		{"second SETTINGS", RoleAcceptor, Config{}, frame(TypeSettings, 0, DefaultSettings().Marshal()), goAway(GoAwaySettings)},
		{"DATA on stream 0", RoleAcceptor, Config{}, frame(TypeData, 0, []byte("x")), goAway(GoAwayProtocolError)},
		{"OPEN on stream 0", RoleAcceptor, Config{}, frame(TypeOpen, 0, nil), goAway(GoAwayProtocolError)},
		{"PING on a stream", RoleAcceptor, Config{}, frame(TypePing, 1, make([]byte, 8)), goAway(GoAwayProtocolError)},
		{"short PING", RoleAcceptor, Config{}, frame(TypePing, 0, make([]byte, 7)), goAway(GoAwayProtocolError)},
		{"GOAWAY on a stream", RoleAcceptor, Config{}, frame(TypeGoAway, 1, marshalGoAway(0, 0)), goAway(GoAwayProtocolError)},
		{"short GOAWAY", RoleAcceptor, Config{}, frame(TypeGoAway, 0, []byte{1, 2}), goAway(GoAwayProtocolError)},
		{"GOAWAY with an impossible last id", RoleAcceptor, Config{}, frame(TypeGoAway, 0, binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint32(nil, 1<<31), 0)), goAway(GoAwayProtocolError)},
		{"even stream id", RoleAcceptor, Config{}, frame(TypeOpen, 2, nil), goAway(GoAwayProtocolError)},
		{"stream id above the maximum", RoleAcceptor, Config{}, frame(TypeOpen, 1<<31+1, nil), goAway(GoAwayProtocolError)},
		{"stream ids that do not increase", RoleAcceptor, Config{}, steps(open1, func(p *rawPeer) { p.open(3) }, func(p *rawPeer) { p.open(3) }), goAway(GoAwayProtocolError)},
		{"stream id reused", RoleAcceptor, Config{}, steps(open1, func(p *rawPeer) { p.open(1) }), goAway(GoAwayProtocolError)},
		{"DATA on a stream never opened", RoleAcceptor, Config{}, steps(open1, frame(TypeData, 7, []byte("x"))), goAway(GoAwayProtocolError)},
		{"WINDOW on a stream never opened", RoleAcceptor, Config{}, frame(TypeWindow, 5, marshalWindow(1)), goAway(GoAwayProtocolError)},
		{"RESET on a stream never opened", RoleAcceptor, Config{}, frame(TypeReset, 5, marshalReset(1)), goAway(GoAwayProtocolError)},
		{"short RESET", RoleAcceptor, Config{}, steps(open1, frame(TypeReset, 1, []byte{1})), goAway(GoAwayProtocolError)},
		{"RESULT sent to an acceptor", RoleAcceptor, Config{}, steps(open1, frame(TypeResult, 1, marshalResult(0))), goAway(GoAwayProtocolError)},
		{"OPEN sent to a dialler", RoleDialer, Config{}, frame(TypeOpen, 1, nil), goAway(GoAwayProtocolError)},
		{"carrier WINDOW of zero", RoleAcceptor, Config{}, frame(TypeWindow, 0, marshalWindow(0)), goAway(GoAwayProtocolError)},
		{"short carrier WINDOW", RoleAcceptor, Config{}, frame(TypeWindow, 0, []byte{1}), goAway(GoAwayProtocolError)},
		{"carrier WINDOW overflow", RoleAcceptor, Config{}, frame(TypeWindow, 0, marshalWindow(1<<31-1)), goAway(GoAwayFlowControl)},
		{"DATA beyond the stream window", RoleAcceptor, Config{StreamWindow: 4096, CarrierWindow: 64 * 1024}, steps(open1, func(p *rawPeer) {
			for range 5 {
				p.send(Frame{Type: TypeData, StreamID: 1, Payload: make([]byte, 1024)})
			}
		}), goAway(GoAwayFlowControl)},
		{"DATA beyond the carrier window", RoleAcceptor, Config{StreamWindow: 8192, CarrierWindow: 4096}, steps(open1, func(p *rawPeer) { p.open(3) }, func(p *rawPeer) {
			for range 5 {
				p.send(Frame{Type: TypeData, StreamID: 1, Payload: make([]byte, 1024)})
			}
		}), goAway(GoAwayFlowControl)},
		{"frame above MaxFrame", RoleAcceptor, Config{MaxFrame: 1024}, func(p *rawPeer) {
			p.sendRaw(mustFrame(t, Frame{Type: TypeData, StreamID: 1, Payload: make([]byte, 1025)})[:HeaderSize])
		}, goAway(GoAwayFrameSize)},

		// stream-level: the carrier survives
		{"unknown type on stream 0", RoleAcceptor, Config{}, frame(0x55, 0, []byte("future")), ok},
		{"unknown type on a stream", RoleAcceptor, Config{}, steps(open1, frame(0x55, 1, []byte("future"))), reset(1, ResetUnknownFrame)},
		{"malformed OPEN", RoleAcceptor, Config{}, frame(TypeOpen, 1, []byte{9, 9, 9}), reset(1, ResetProtocolError)},
		{"DATA after FIN", RoleAcceptor, Config{}, steps(open1, func(p *rawPeer) {
			p.send(Frame{Type: TypeData, Flags: FlagFIN, StreamID: 1})
			p.send(Frame{Type: TypeData, StreamID: 1, Payload: []byte("x")})
		}), reset(1, ResetProtocolError)},
		{"empty DATA without FIN", RoleAcceptor, Config{}, steps(open1, frame(TypeData, 1, nil)), reset(1, ResetProtocolError)},
		{"DATAGRAM on a TCP stream", RoleAcceptor, Config{}, steps(open1, frame(TypeDatagram, 1, []byte("x"))), reset(1, ResetProtocolError)},
		{"empty DATAGRAM", RoleAcceptor, Config{}, steps(func(p *rawPeer) {
			payload, _ := OpenParams{Kind: StreamUDP, RouteID: "r"}.MarshalBinary()
			p.send(Frame{Type: TypeOpen, StreamID: 1, Payload: payload})
		}, frame(TypeDatagram, 1, nil)), reset(1, ResetProtocolError)},
		{"stream WINDOW of zero", RoleAcceptor, Config{}, steps(open1, frame(TypeWindow, 1, marshalWindow(0))), reset(1, ResetProtocolError)},
		{"stream WINDOW overflow", RoleAcceptor, Config{}, steps(open1, frame(TypeWindow, 1, marshalWindow(1<<31-1))), reset(1, ResetFlowControl)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, p := newRawPeer(t, "tcp", tc.role, tc.cfg, DefaultSettings())
			tc.do(p)
			if tc.want.alive {
				if tc.want.reset != 0 {
					f := p.recvType(TypeReset)
					r, err := parseReset(f.Payload)
					if err != nil || f.StreamID != tc.want.on || r != tc.want.reset {
						t.Fatalf("RESET %v on %d (%v), want %v on %d", r, f.StreamID, err, tc.want.reset, tc.want.on)
					}
				} else {
					// A ping answered proves the carrier still reads.
					p.send(Frame{Type: TypePing, Payload: make([]byte, 8)})
					p.recvType(TypePing)
				}
				if err := c.Err(); err != nil {
					t.Fatalf("carrier ended: %v", err)
				}
				return
			}
			if got := p.expectGoAway(); got != tc.want.goAway {
				t.Fatalf("GOAWAY %v, want %v", got, tc.want.goAway)
			}
			waitDone(t, c)
			var pe *ProtocolError
			if err := c.Err(); !asProtocolError(err, &pe) || pe.Reason != tc.want.goAway {
				t.Fatalf("Err = %v", err)
			}
		})
	}
}

func asProtocolError(err error, target **ProtocolError) bool {
	for err != nil {
		if pe, ok := err.(*ProtocolError); ok {
			*target = pe
			return true
		}
		u, ok := err.(interface{ Unwrap() []error })
		if ok {
			for _, e := range u.Unwrap() {
				if asProtocolError(e, target) {
					return true
				}
			}
			return false
		}
		next, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = next.Unwrap()
	}
	return false
}

// A dialler sees violations by the listener on streams it opened.
func TestListenerViolationsOnADiallerStream(t *testing.T) {
	setup := func(t *testing.T) (*ConnCarrier, *rawPeer, Stream) {
		c, p := newRawPeer(t, "tcp", RoleDialer, Config{}, withSettings(func(s *Settings) { s.MaxStreams = 10 }))
		s, err := c.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		if f := p.recvType(TypeOpen); f.StreamID != 1 {
			t.Fatalf("OPEN for %d", f.StreamID)
		}
		return c, p, s
	}
	t.Run("DATA before RESULT", func(t *testing.T) {
		c, p, s := setup(t)
		p.send(Frame{Type: TypeData, StreamID: 1, Payload: []byte("x")})
		if f := p.recvType(TypeReset); f.StreamID != 1 {
			t.Fatal("no RESET")
		}
		if _, err := s.Read(make([]byte, 1)); err == nil {
			t.Fatal("read succeeded")
		}
		if c.Err() != nil {
			t.Fatal(c.Err())
		}
	})
	t.Run("RESULT twice", func(t *testing.T) {
		c, p, _ := setup(t)
		p.send(Frame{Type: TypeResult, StreamID: 1, Payload: marshalResult(ResultOK)})
		p.send(Frame{Type: TypeResult, StreamID: 1, Payload: marshalResult(ResultOK)})
		if f := p.recvType(TypeReset); f.StreamID != 1 {
			t.Fatal("no RESET")
		}
		if c.Err() != nil {
			t.Fatal(c.Err())
		}
	})
	t.Run("short RESULT", func(t *testing.T) {
		c, p, _ := setup(t)
		p.send(Frame{Type: TypeResult, StreamID: 1, Payload: []byte{0}})
		if r := p.expectGoAway(); r != GoAwayProtocolError {
			t.Fatal(r)
		}
		waitDone(t, c)
	})
	t.Run("RESULT for a stream never opened", func(t *testing.T) {
		c, p, _ := setup(t)
		p.send(Frame{Type: TypeResult, StreamID: 9, Payload: marshalResult(ResultOK)})
		if r := p.expectGoAway(); r != GoAwayProtocolError {
			t.Fatal(r)
		}
		waitDone(t, c)
	})
	t.Run("even stream id", func(t *testing.T) {
		c, p, _ := setup(t)
		p.send(Frame{Type: TypeResult, StreamID: 2, Payload: marshalResult(ResultOK)})
		if r := p.expectGoAway(); r != GoAwayProtocolError {
			t.Fatal(r)
		}
		waitDone(t, c)
	})
	t.Run("RESULT for a stream the dialler reset is ignored", func(t *testing.T) {
		c, p, s := setup(t)
		_ = s.Close()
		if f := p.recvType(TypeReset); f.StreamID != 1 {
			t.Fatal("no RESET")
		}
		p.send(Frame{Type: TypeResult, StreamID: 1, Payload: marshalResult(ResultOK)})
		p.send(Frame{Type: TypeData, StreamID: 1, Payload: []byte("late")})
		p.send(Frame{Type: TypePing, Payload: make([]byte, 8)})
		p.recvType(TypePing)
		if c.Err() != nil {
			t.Fatal(c.Err())
		}
	})
	t.Run("a listener that never opens is not blamed for GOAWAY twice", func(t *testing.T) {
		c, p, _ := setup(t)
		p.send(Frame{Type: TypeGoAway, Payload: marshalGoAway(1, GoAwayListenerClosed)})
		p.send(Frame{Type: TypeGoAway, Payload: marshalGoAway(0, GoAwayShutdown)})
		select {
		case <-c.Draining():
		case <-time.After(testWait):
			t.Fatal("not draining")
		}
		if r, _ := c.PeerGoAway(); r != GoAwayListenerClosed {
			t.Fatalf("PeerGoAway = %v, want the first", r)
		}
	})
}
