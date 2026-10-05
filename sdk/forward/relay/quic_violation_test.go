package relay

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// Every rule a QUIC peer can break, from a raw peer: the carrier ends with the
// rule as the application error code of its close when the rule is the
// carrier's, resets the stream when it is the stream's, and lives on in the
// second case. The peers' bytes are the ones of quicwire.go, wrong in one way
// each.

func frameBytes(typ FrameType, id uint32, payload []byte) []byte {
	b := make([]byte, 0, HeaderSize+len(payload))
	b = binary.BigEndian.AppendUint16(b, uint16(len(payload)))
	b = append(b, byte(typ), 0)
	b = binary.BigEndian.AppendUint32(b, id)
	return append(b, payload...)
}

func settingsPayload(mod func(*Settings)) []byte {
	s := DefaultSettings()
	if mod != nil {
		mod(&s)
	}
	return s.Marshal()
}

// A raw dialler's first frame on its control stream must be a valid SETTINGS.
func TestQUICSettingsViolations(t *testing.T) {
	tests := []struct {
		name string
		send []byte
		fin  bool
	}{
		{"an empty control stream", nil, true},
		{"a truncated header", []byte{0, 12, byte(TypeSettings)}, true},
		{"a truncated payload", frameBytes(TypeSettings, 0, settingsPayload(nil))[:HeaderSize+5], true},
		{"OPEN as the first frame", frameBytes(TypeOpen, 0, settingsPayload(nil)), false},
		{"GOAWAY as the first frame", frameBytes(TypeGoAway, 0, marshalGoAway(0, GoAwayNoError)), false},
		{"SETTINGS on a stream id", frameBytes(TypeSettings, 4, settingsPayload(nil)), false},
		{"a window below its bound", frameBytes(TypeSettings, 0, settingsPayload(func(s *Settings) { s.StreamWindow = 1 })), false},
		{"a frame size above its bound", frameBytes(TypeSettings, 0, settingsPayload(func(s *Settings) { s.MaxFrame = 70000 })), false},
		{"a key twice", frameBytes(TypeSettings, 0, append(settingsPayload(nil), settingsPayload(nil)[:6]...)), false},
		{"half a pair", frameBytes(TypeSettings, 0, settingsPayload(nil)[:23]), false},
		{"a frame longer than the listener's maximum", frameBytes(TypeSettings, 0, make([]byte, 20000)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, QUICConfig{})
			p := &qraw{t: t, conn: rawDiallerConn(t, e, QUICConfig{})}
			out, err := p.conn.OpenUniStream()
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.send) > 0 {
				// the listener may close the connection on the header, before the
				// rest of a long frame is written
				_, _ = out.Write(tc.send)
			}
			if tc.fin {
				_ = out.Close()
			}
			if r := p.expectEnd(); r != GoAwaySettings {
				t.Fatalf("the carrier closed with %v, want settings_error", r)
			}
			waitFor(t, "the failure to be counted", func() bool { return e.l.Stats().SettingsFailed == 1 })
			if st := e.l.Stats(); st.Accepted != 0 || st.Carriers != 0 {
				t.Fatalf("stats %+v", st)
			}
		})
	}
}

// A connection that never opens its control stream is dropped at the deadline
// and gives its slot back.
func TestQUICSettingsTimeout(t *testing.T) {
	cfg := QUICConfig{Config: Config{HandshakeTimeout: 300 * time.Millisecond, MaxPending: 1}}
	e := newQUICEnv(t, cfg)
	p := &qraw{t: t, conn: rawDiallerConn(t, e, cfg)}
	select {
	case <-p.conn.Context().Done():
	case <-time.After(testWait):
		t.Fatal("the listener kept a connection that never sent SETTINGS")
	}
	waitFor(t, "the failure to be counted", func() bool { return e.l.Stats().SettingsFailed == 1 })
	// the slot is back
	d, err := e.dial(linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	_ = acceptQUIC(t, e.l)
}

// Too many connections waiting in their SETTINGS exchange: the next is closed.
func TestQUICSettingsExchangeIsBounded(t *testing.T) {
	cfg := QUICConfig{Config: Config{HandshakeTimeout: 5 * time.Second, MaxPending: 2}}
	e := newQUICEnv(t, cfg)
	var held []*qraw
	for range 2 {
		held = append(held, &qraw{t: t, conn: rawDiallerConn(t, e, cfg)})
	}
	waitFor(t, "both exchanges in flight", func() bool { return e.l.Stats().Link.Accepted == 2 })
	third := &qraw{t: t, conn: rawDiallerConn(t, e, cfg)}
	waitFor(t, "the overload to be counted", func() bool { return e.l.Stats().Overloaded == 1 })
	select {
	case <-third.conn.Context().Done():
	case <-time.After(testWait):
		t.Fatal("the third connection was not closed")
	}
	for _, h := range held {
		select {
		case <-h.conn.Context().Done():
			t.Fatal("a connection within the bound was closed")
		default:
		}
	}
}

// Rules of the control stream after SETTINGS, from either end.
func controlViolations() []struct {
	name string
	do   func(t *testing.T, p *qraw)
	want GoAwayReason
} {
	return []struct {
		name string
		do   func(t *testing.T, p *qraw)
		want GoAwayReason
	}{
		{"GOAWAY with a short payload", func(t *testing.T, p *qraw) { p.send(frameBytes(TypeGoAway, 0, []byte{1, 2, 3, 4, 5})) }, GoAwayProtocolError},
		{"GOAWAY with a long payload", func(t *testing.T, p *qraw) { p.send(frameBytes(TypeGoAway, 0, make([]byte, 7))) }, GoAwayProtocolError},
		{"GOAWAY with an id out of range", func(t *testing.T, p *qraw) {
			p.send(frameBytes(TypeGoAway, 0, marshalGoAwayRaw(MaxStreamID+1, GoAwayNoError)))
		}, GoAwayProtocolError},
		{"a frame on a stream id", func(t *testing.T, p *qraw) { p.send(frameBytes(TypeGoAway, 7, marshalGoAway(0, GoAwayNoError))) }, GoAwayProtocolError},
		{"SETTINGS again", func(t *testing.T, p *qraw) { p.send(frameBytes(TypeSettings, 0, settingsPayload(nil))) }, GoAwaySettings},
		{"a frame above the maximum", func(t *testing.T, p *qraw) { p.send(frameBytes(TypeGoAway, 0, make([]byte, 5000))) }, GoAwayFrameSize},
		{"a control stream that ends without GOAWAY", func(t *testing.T, p *qraw) { _ = p.out.Close() }, GoAwayProtocolError},
		{"a control stream that ends inside a frame", func(t *testing.T, p *qraw) {
			p.send(frameBytes(TypeGoAway, 0, marshalGoAway(0, GoAwayNoError))[:5])
			_ = p.out.Close()
		}, GoAwayProtocolError},
		{"a control stream that is reset", func(t *testing.T, p *qraw) { p.out.CancelWrite(7) }, GoAwayProtocolError},
	}
}

// marshalGoAwayRaw encodes a GOAWAY payload without the range check the
// parser makes.
func marshalGoAwayRaw(last uint32, r GoAwayReason) []byte {
	b := binary.BigEndian.AppendUint32(nil, last)
	return binary.BigEndian.AppendUint16(b, uint16(r))
}

func TestQUICControlStreamViolationsByTheDialler(t *testing.T) {
	cfg := QUICConfig{Config: Config{MaxFrame: 1024}}
	for _, tc := range controlViolations() {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, cfg)
			a, p := rawDialler(t, e, cfg)
			tc.do(t, p)
			if r := p.expectEnd(); r != tc.want {
				t.Fatalf("the carrier closed with %v, want %v", r, tc.want)
			}
			waitDone(t, a)
			var pe *ProtocolError
			_ = pe
			if !errors.Is(a.Err(), ErrCarrierClosed) {
				t.Fatalf("Err = %v", a.Err())
			}
		})
	}
}

func TestQUICControlStreamViolationsByTheAcceptor(t *testing.T) {
	cfg := QUICConfig{Config: Config{MaxFrame: 1024}}
	for _, tc := range controlViolations() {
		t.Run(tc.name, func(t *testing.T) {
			d, p := rawAcceptor(t, cfg)
			tc.do(t, p)
			if r := p.expectEnd(); r != tc.want {
				t.Fatalf("the carrier closed with %v, want %v", r, tc.want)
			}
			waitDone(t, d)
		})
	}
}

// What the control stream tolerates: frame types this version does not use, and
// GOAWAY twice.
func TestQUICControlStreamToleratesUnknownFrames(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	p.send(frameBytes(FrameType(0x42), 0, []byte("from a later version")))
	p.send(frameBytes(TypePing, 0, make([]byte, 8)))
	p.send(frameBytes(TypeWindow, 0, marshalWindow(1)))
	p.send(frameBytes(TypeGoAway, 0, marshalGoAway(0, GoAwayNoError)))
	p.send(frameBytes(TypeGoAway, 0, marshalGoAway(0, GoAwayShutdown))) // the first one counts
	select {
	case <-a.Draining():
	case <-time.After(testWait):
		t.Fatal("GOAWAY after the unknown frames was not processed")
	}
	if r, ok := a.PeerGoAway(); !ok || r != GoAwayNoError {
		t.Fatalf("PeerGoAway = %v %v", r, ok)
	}
	select {
	case <-a.Done():
		// the carrier has no streams: it is quiet and so is the peer once it
		// finishes its control stream, which this peer does not, so it waits
	default:
	}
}

// QUIC itself limits the peer's unidirectional streams to one, its control
// stream, and a listening end opens no bidirectional stream at all: the
// transport parameters each end announces make a second control stream, or a
// stream from the listener, impossible for any QUIC implementation that
// respects them. A peer that does not is closed by QUIC with
// STREAM_LIMIT_ERROR, before the carrier sees anything.
func TestQUICStreamLimitsOfTheMapping(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	_, p := rawDialler(t, e, QUICConfig{})
	var limit *quic.StreamLimitReachedError
	if _, err := p.conn.OpenUniStream(); !errors.As(err, &limit) {
		t.Fatalf("a second control stream: %v, want the stream limit", err)
	}

	d, q := rawAcceptor(t, QUICConfig{})
	if _, err := q.conn.OpenStream(); !errors.As(err, &limit) {
		t.Fatalf("a stream opened by the listening end: %v, want the stream limit", err)
	}
	if _, err := q.conn.OpenUniStream(); !errors.As(err, &limit) {
		t.Fatalf("a second control stream from the listening end: %v, want the stream limit", err)
	}
	select {
	case <-d.Done():
		t.Fatal("the dialler ended")
	default:
	}
}

// The first frame of a stream must be a valid OPEN.
func TestQUICOpenViolations(t *testing.T) {
	badParams := func(mod func(b []byte) []byte) []byte {
		b, err := testParams().MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		return mod(b)
	}
	tests := []struct {
		name  string
		bytes []byte
		fin   bool
	}{
		{"an HTTP request", []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"), false},
		{"DATA first", frameBytes(TypeData, 0, []byte("hello")), false},
		{"RESULT first", frameBytes(TypeResult, 0, marshalResult(ResultOK)), false},
		{"OPEN on a stream id", func() []byte { b, _ := testParams().MarshalBinary(); return frameBytes(TypeOpen, 4, b) }(), false},
		{"OPEN longer than any OPEN", frameBytes(TypeOpen, 0, make([]byte, maxOpenPayload+1)), false},
		{"a truncated OPEN", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { return b }))[:HeaderSize+3], true},
		{"a stream with nothing on it", nil, true},
		{"an unknown stream kind", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { b[0] = 9; return b })), false},
		{"kind zero", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { b[0] = 0; return b })), false},
		{"an empty route id", frameBytes(TypeOpen, 0, []byte{byte(StreamTCP), 0, 0, 0, 1, 0, 0}), false},
		{"a route id with a control character", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { b[7] = 0x01; return b })), false},
		{"a trailing byte", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { return append(b, 0) })), false},
		{"an unknown address family", frameBytes(TypeOpen, 0, badParams(func(b []byte) []byte { b[len(b)-1] = 5; return b })), false},
		{"an IPv4-mapped address as IPv6", frameBytes(TypeOpen, 0, func() []byte {
			p := OpenParams{Kind: StreamTCP, RouteID: "r", HopIndex: 1}
			b, _ := p.MarshalBinary()
			b[len(b)-1] = 6
			mapped := append(make([]byte, 10), 0xff, 0xff, 1, 2, 3, 4)
			return append(append(b, mapped...), 0, 80)
		}()), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, QUICConfig{})
			a, p := rawDialler(t, e, QUICConfig{})
			st := p.openRaw(tc.bytes...)
			if tc.fin {
				_ = st.Close()
			}
			expectReset(t, st, ResetProtocolError)
			waitFor(t, "the refusal to be counted", func() bool {
				s := a.Stats()
				return s.StreamsRefused == 1 && s.ResetsSent[ResetProtocolError] == 1
			})
			// the carrier lives and takes a good stream
			good := p.openStream(testParams())
			in, err := a.Accept(ctxTimeout(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Accept(); err != nil {
				t.Fatal(err)
			}
			if code := readResultFrame(t, good); code != ResultOK {
				t.Fatalf("RESULT %v", code)
			}
			if a.Stats().StreamsAccepted != 1 {
				t.Fatalf("stats %+v", a.Stats())
			}
		})
	}
}

// A stream the peer gave up before saying anything is not a violation.
func TestQUICOpenAbandonedByThePeer(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	st := p.openRaw(0) // one byte of a frame header
	st.CancelWrite(3)
	good := p.openStream(testParams())
	in, err := a.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = in.Accept()
	_ = readResultFrame(t, good)
	if s := a.Stats(); s.StreamsRefused != 0 {
		t.Fatalf("an abandoned stream was counted as refused: %+v", s)
	}
}

// A peer that opens a stream and says nothing loses it at the deadline.
func TestQUICOpenTimeout(t *testing.T) {
	cfg := QUICConfig{Config: Config{HandshakeTimeout: 300 * time.Millisecond}}
	e := newQUICEnv(t, cfg)
	a, p := rawDialler(t, e, cfg)
	st := p.openRaw(0, 0) // two bytes of a frame header: never completed
	expectReset(t, st, ResetProtocolError)
	if s := a.Stats(); s.StreamsRefused != 1 {
		t.Fatalf("stats %+v", s)
	}
}

// Refusals and malformed OPENs draw on the peer's budget: beyond it the
// carrier ends.
func TestQUICOpenFlood(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	for range 20 * calmBurst {
		st, err := p.conn.OpenStream()
		if err != nil {
			time.Sleep(time.Millisecond) // the connection's stream limit: let resets free slots
			continue
		}
		_, _ = st.Write(frameBytes(TypeOpen, 0, []byte{0xff})) // malformed
		select {
		case <-a.Done():
			if r := p.expectEnd(); r != GoAwayCalm {
				t.Fatalf("the carrier closed with %v, want enhance_your_calm", r)
			}
			return
		default:
		}
	}
	waitDone(t, a)
	if r := p.expectEnd(); r != GoAwayCalm {
		t.Fatalf("the carrier closed with %v, want enhance_your_calm", r)
	}
}

// A UDP stream carries DATAGRAM frames, and nothing else, on its stream.
func TestQUICUDPStreamViolations(t *testing.T) {
	udp := OpenParams{Kind: StreamUDP, RouteID: "r", HopIndex: 1}
	tests := []struct {
		name  string
		bytes []byte
		fin   bool
	}{
		{"DATA on a UDP stream", frameBytes(TypeData, 0, []byte("hello")), false},
		{"OPEN again", frameBytes(TypeOpen, 0, []byte{1}), false},
		{"an empty datagram", frameBytes(TypeDatagram, 0, nil), false},
		{"a datagram above the maximum", frameBytes(TypeDatagram, 0, make([]byte, DefaultMaxFrame+1)), false},
		{"a frame on a stream id", frameBytes(TypeDatagram, 4, []byte("x")), false},
		{"a truncated frame", frameBytes(TypeDatagram, 0, []byte("hello"))[:HeaderSize+2], true},
		{"raw bytes", []byte("not frames at all"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newQUICEnv(t, QUICConfig{})
			a, p := rawDialler(t, e, QUICConfig{})
			st := p.openStream(udp)
			in, err := a.Accept(ctxTimeout(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Accept(); err != nil {
				t.Fatal(err)
			}
			_ = readResultFrame(t, st)
			// one good datagram first: the stream works until the violation
			_, _ = st.Write(frameBytes(TypeDatagram, 0, []byte("good")))
			if d, err := in.ReadDatagram(); err != nil || string(d) != "good" {
				t.Fatalf("ReadDatagram = %q %v", d, err)
			}
			_, _ = st.Write(tc.bytes)
			if tc.fin {
				_ = st.Close()
			}
			expectReset(t, st, ResetProtocolError)
			var re *StreamResetError
			_ = in.SetReadDeadline(time.Now().Add(testWait))
			if _, err := in.ReadDatagram(); !errors.As(err, &re) || re.Reason != ResetProtocolError {
				t.Fatalf("ReadDatagram after the violation = %v", err)
			}
			// the carrier lives
			good := p.openStream(testParams())
			in2, err := a.Accept(ctxTimeout(t))
			if err != nil {
				t.Fatal(err)
			}
			_ = in2.Accept()
			_ = readResultFrame(t, good)
		})
	}
}

// DATAGRAM frames on a UDP stream reach ReadDatagram, boundaries kept, and the
// FIN ends it.
func TestQUICUDPOverTheStream(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	st := p.openStream(OpenParams{Kind: StreamUDP, RouteID: "r", HopIndex: 1})
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	_ = readResultFrame(t, st)
	want := [][]byte{[]byte("a"), pattern(1500, 1), pattern(DefaultMaxFrame, 2), []byte("last")}
	for _, d := range want {
		if _, err := st.Write(frameBytes(TypeDatagram, 0, d)); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	for i, w := range want {
		d, err := in.ReadDatagram()
		if err != nil {
			t.Fatalf("datagram %d: %v", i, err)
		}
		eqBytes(t, d, w)
	}
	if _, err := in.ReadDatagram(); err != io.EOF {
		t.Fatalf("after the FIN: %v", err)
	}
	st2 := a.Stats()
	if st2.DatagramsRecv != uint64(len(want)) || st2.DatagramsRecvDropped != 0 {
		t.Fatalf("stats %+v", st2)
	}
}

// QUIC datagrams the carrier cannot place are dropped and counted; malformed
// ones draw on the peer's budget.
func TestQUICDatagramViolations(t *testing.T) {
	e := newQUICEnv(t, QUICConfig{})
	a, p := rawDialler(t, e, QUICConfig{})
	udp := p.openStream(OpenParams{Kind: StreamUDP, RouteID: "r", HopIndex: 1})
	in, _ := a.Accept(ctxTimeout(t))
	_ = in.Accept()
	_ = readResultFrame(t, udp)
	tcp := p.openStream(testParams())
	tcpIn, _ := a.Accept(ctxTimeout(t))
	_ = tcpIn.Accept()
	_ = readResultFrame(t, tcp)

	send := func(b []byte) {
		t.Helper()
		if err := p.conn.SendDatagram(b); err != nil {
			t.Fatal(err)
		}
	}
	dropped := func() uint64 { return a.Stats().DatagramsRecvDropped }
	var want uint64

	// not an open association: never opened, a TCP stream, a finished one
	send(append(quicVarint(4000), "x"...))
	send(append(quicVarint(tcp.StreamID()), "x"...))
	finished := p.openStream(OpenParams{Kind: StreamUDP, RouteID: "r", HopIndex: 1})
	fin, _ := a.Accept(ctxTimeout(t))
	_ = fin.Accept()
	_ = readResultFrame(t, finished)
	fid := finished.StreamID()
	_ = fin.Reset(ResetCancel)
	waitFor(t, "the association to finish", func() bool { return a.ActiveStreams() == 2 })
	send(append(quicVarint(fid), "x"...))
	want += 3
	waitFor(t, "the drops", func() bool { return dropped() == want })

	// malformed: a stream id in a longer form than needed, no payload, nothing
	send(append([]byte{0x40, byte(udp.StreamID())}, "x"...))
	send(quicVarint(udp.StreamID()))
	send([]byte{0xc0}) // a truncated 8-byte varint
	want += 3
	waitFor(t, "the malformed ones", func() bool { return dropped() == want })

	// a good one still arrives
	send(append(quicVarint(udp.StreamID()), "ok"...))
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	if d, err := in.ReadDatagram(); err != nil || string(d) != "ok" {
		t.Fatalf("ReadDatagram = %q %v", d, err)
	}
	select {
	case <-a.Done():
		t.Fatalf("the carrier ended: %v", a.Err())
	default:
	}

	// a stream of malformed datagrams ends the carrier
	for range 20 * calmBurst {
		if err := p.conn.SendDatagram([]byte{0xc0}); err != nil {
			break
		}
		select {
		case <-a.Done():
		case <-time.After(time.Millisecond):
			continue
		}
		break
	}
	waitDone(t, a)
	if r := p.expectEnd(); r != GoAwayCalm {
		t.Fatalf("the carrier closed with %v, want enhance_your_calm", r)
	}
}

func quicVarint(id quic.StreamID) []byte {
	b, err := AppendQUICDatagram(nil, uint64(id), []byte{0}) // #nosec G115 -- test ids are small
	if err != nil {
		panic(err)
	}
	return b[:len(b)-1]
}

// What a raw acceptor can get wrong in answering a dialler's stream.
func TestQUICResultViolations(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		fin   bool
	}{
		{"DATA before RESULT", frameBytes(TypeData, 0, []byte("hello")), false},
		{"raw bytes before RESULT", []byte("HTTP/1.1 200 OK\r\n"), false},
		{"OPEN instead of RESULT", frameBytes(TypeOpen, 0, []byte{1}), false},
		{"RESULT on a stream id", frameBytes(TypeResult, 4, marshalResult(ResultOK)), false},
		{"a RESULT payload of three bytes", frameBytes(TypeResult, 0, []byte{0, 0, 0}), false},
		{"a RESULT payload of one byte", frameBytes(TypeResult, 0, []byte{0}), false},
		{"a truncated RESULT", frameBytes(TypeResult, 0, marshalResult(ResultOK))[:HeaderSize+1], true},
		{"a stream that ends with no RESULT", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, p := rawAcceptor(t, QUICConfig{})
			s, err := d.Open(testParams())
			if err != nil {
				t.Fatal(err)
			}
			st, _ := p.acceptStream()
			if len(tc.bytes) > 0 {
				_, _ = st.Write(tc.bytes)
			}
			if tc.fin {
				_ = st.Close()
			}
			var re *StreamResetError
			if err := s.AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Reason != ResetProtocolError || s.Answered() {
				t.Fatalf("AwaitResult = %v (answered %v)", err, s.Answered())
			}
			if _, err := s.Write([]byte("x")); err == nil {
				t.Fatal("Write on a stream whose RESULT was bad")
			}
			// the carrier lives and the next stream works
			s2, err := d.Open(testParams())
			if err != nil {
				t.Fatal(err)
			}
			st2, _ := p.acceptStream()
			answer(t, st2, ResultOK)
			if err := s2.AwaitResult(ctxTimeout(t)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-d.Done():
				t.Fatalf("the carrier ended: %v", d.Err())
			default:
			}
		})
	}
}

// A stream the raw acceptor resets before answering is a carrier-level refusal
// the dialler can retry on another carrier.
func TestQUICResetBeforeResultIsARefusal(t *testing.T) {
	d, p := rawAcceptor(t, QUICConfig{})
	s, _ := d.Open(testParams())
	st, _ := p.acceptStream()
	st.CancelRead(quic.StreamErrorCode(ResetRefusedStream))
	st.CancelWrite(quic.StreamErrorCode(ResetRefusedStream))
	if err := s.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrRefused) || s.Answered() {
		t.Fatalf("AwaitResult = %v (answered %v)", err, s.Answered())
	}
}

// GOAWAY's id says which streams the listening end never took: the dialler
// fails those at once, so they can be retried elsewhere, and keeps the rest.
func TestQUICGoAwayFailsTheStreamsItRefused(t *testing.T) {
	d, p := rawAcceptor(t, QUICConfig{})
	var streams []Stream
	var raws []*quic.Stream
	for range 3 {
		s, err := d.Open(testParams())
		if err != nil {
			t.Fatal(err)
		}
		st, _ := p.acceptStream()
		streams, raws = append(streams, s), append(raws, st)
	}
	// the raw acceptor took the first two and refuses from the third's id on
	last := uint32(streams[2].ID())
	p.send(frameBytes(TypeGoAway, 0, marshalGoAway(last, GoAwayListenerClosed)))
	var re *StreamResetError
	if err := streams[2].AwaitResult(ctxTimeout(t)); !errors.As(err, &re) || re.Reason != ResetRefusedStream || !errors.Is(err, ErrRefused) || streams[2].Answered() {
		t.Fatalf("the refused stream: %v", err)
	}
	// the other two are untouched and finish normally
	answer(t, raws[0], ResultOK)
	answer(t, raws[1], ResultOK)
	for _, s := range streams[:2] {
		if err := s.AwaitResult(ctxTimeout(t)); err != nil {
			t.Fatalf("a stream the listener took: %v", err)
		}
	}
	select {
	case <-d.Draining():
	default:
		t.Fatal("not draining")
	}
	if r, ok := d.PeerGoAway(); !ok || r != GoAwayListenerClosed {
		t.Fatalf("PeerGoAway %v %v", r, ok)
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
		t.Fatalf("Open = %v", err)
	}
	// the dialler cancels what the listener will not process
	_ = raws[2].SetReadDeadline(time.Now().Add(testWait))
	if _, err := io.Copy(io.Discard, raws[2]); err == nil {
		t.Fatal("the refused stream ended cleanly")
	}
}

// A peer's close code is its GOAWAY reason, whatever it told the control
// stream.
func TestQUICPeersCloseCodeIsItsGoAwayReason(t *testing.T) {
	d, p := rawAcceptor(t, QUICConfig{})
	_ = p.conn.CloseWithError(quic.ApplicationErrorCode(GoAwayCredentials), "")
	waitDone(t, d)
	if r, ok := d.PeerGoAway(); !ok || r != GoAwayCredentials {
		t.Fatalf("PeerGoAway = %v %v", r, ok)
	}
	if !errors.Is(d.Err(), ErrCarrierClosed) {
		t.Fatalf("Err = %v", d.Err())
	}
	// a code that is no GOAWAY reason is internal
	d2, p2 := rawAcceptor(t, QUICConfig{})
	_ = p2.conn.CloseWithError(quic.ApplicationErrorCode(1<<40), "")
	waitDone(t, d2)
	if r, ok := d2.PeerGoAway(); !ok || r != GoAwayInternal {
		t.Fatalf("PeerGoAway = %v %v", r, ok)
	}
}

// L5: a carrier that is alive but never answers an OPEN is retired, and the
// streams fail so they can be retried on another carrier.
func TestQUICUnansweredOpenRetiresTheCarrier(t *testing.T) {
	cfg := QUICConfig{Config: Config{ResultTimeout: 150 * time.Millisecond}}
	d, p := rawAcceptor(t, cfg)
	s1, _ := d.Open(testParams())
	s2, _ := d.Open(testParams())
	p.acceptStream()
	p.acceptStream()
	for _, s := range []Stream{s1, s2} {
		if err := s.AwaitResult(ctxTimeout(t)); !errors.Is(err, ErrOpenTimeout) || s.Answered() {
			t.Fatalf("AwaitResult = %v (answered %v)", err, s.Answered())
		}
	}
	if r := p.expectGoAway(); r != GoAwayStuck {
		t.Fatalf("GOAWAY %v, want stuck", r)
	}
	select {
	case <-d.Draining():
	default:
		t.Fatal("not draining")
	}
	if _, err := d.Open(testParams()); !errors.Is(err, ErrGoAway) {
		t.Fatalf("Open on a retired carrier = %v", err)
	}
}

// An answered stream is not stuck, however slow the data.
func TestQUICAnsweredOpenDoesNotRetire(t *testing.T) {
	cfg := QUICConfig{Config: Config{ResultTimeout: 100 * time.Millisecond}}
	d, p := rawAcceptor(t, cfg)
	s, _ := d.Open(testParams())
	st, _ := p.acceptStream()
	answer(t, st, ResultOK)
	if err := s.AwaitResult(ctxTimeout(t)); err != nil {
		t.Fatal(err)
	}
	// several result-timeout periods pass with the stream open and quiet: let
	// the keepalive loop run through them by waiting for it to tick
	time.Sleep(5 * cfg.ResultTimeout)
	select {
	case <-d.Draining():
		t.Fatal("a carrier with only answered streams was retired")
	default:
	}
	_ = context.Background
}
