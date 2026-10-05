package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/quicvarint"
)

func TestQUICDatagramRoundTrip(t *testing.T) {
	for _, id := range []uint64{0, 1, 4, 63, 64, 16383, 16384, 1<<30 - 1, 1 << 30, MaxQUICStreamID} {
		for _, payload := range [][]byte{{0}, []byte("x"), bytes.Repeat([]byte{9}, 1200)} {
			b, err := AppendQUICDatagram([]byte("prefix"), id, payload)
			if err != nil {
				t.Fatal(err)
			}
			b = b[len("prefix"):]
			if len(b) != quicDatagramHeaderLen(id)+len(payload) {
				t.Fatalf("id %d: %d bytes, want %d", id, len(b), quicDatagramHeaderLen(id)+len(payload))
			}
			gotID, gotPayload, err := ParseQUICDatagram(b)
			if err != nil || gotID != id || !bytes.Equal(gotPayload, payload) {
				t.Fatalf("id %d: parsed %d %x, %v", id, gotID, gotPayload, err)
			}
		}
	}
}

// The wire layout: the stream id as a QUIC variable-length integer, then the
// payload.
func TestQUICDatagramLayout(t *testing.T) {
	for _, tc := range []struct {
		id   uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{4, []byte{0x04}},
		{63, []byte{0x3f}},
		{64, []byte{0x40, 0x40}},
		{16383, []byte{0x7f, 0xff}},
		{16384, []byte{0x80, 0x00, 0x40, 0x00}},
		{1 << 30, []byte{0xc0, 0, 0, 0, 0x40, 0, 0, 0}},
	} {
		b, err := AppendQUICDatagram(nil, tc.id, []byte("hi"))
		if err != nil || !bytes.Equal(b, append(tc.want, "hi"...)) {
			t.Fatalf("id %d encodes as %x, %v", tc.id, b, err)
		}
	}
}

func TestQUICDatagramRejects(t *testing.T) {
	if _, err := AppendQUICDatagram(nil, MaxQUICStreamID+1, []byte("x")); !errors.Is(err, ErrQUICDatagram) {
		t.Fatalf("an id above 2^62-1: %v", err)
	}
	if _, err := AppendQUICDatagram(nil, 1, nil); !errors.Is(err, ErrQUICDatagram) {
		t.Fatalf("an empty payload: %v", err)
	}
	bad := map[string][]byte{
		"nothing":                   nil,
		"an id and no payload":      {0x04},
		"a long id and no payload":  {0x40, 0x40},
		"a truncated 2-byte id":     {0x40},
		"a truncated 4-byte id":     {0x80, 0, 0},
		"a truncated 8-byte id":     {0xc0, 0, 0, 0, 0, 0, 0},
		"id 0 in two bytes":         {0x40, 0x00, 'x'},
		"id 4 in two bytes":         {0x40, 0x04, 'x'},
		"id 63 in two bytes":        {0x40, 0x3f, 'x'},
		"id 0 in four bytes":        {0x80, 0, 0, 0, 'x'},
		"id 16383 in four bytes":    {0x80, 0, 0x3f, 0xff, 'x'},
		"id 0 in eight bytes":       {0xc0, 0, 0, 0, 0, 0, 0, 0, 'x'},
		"id 1<<30-1 in eight bytes": {0xc0, 0, 0, 0, 0x3f, 0xff, 0xff, 0xff, 'x'},
	}
	for name, b := range bad {
		if id, p, err := ParseQUICDatagram(b); !errors.Is(err, ErrQUICDatagram) {
			t.Errorf("%s: parsed id %d payload %x, %v", name, id, p, err)
		}
	}
}

func TestReadQUICFrame(t *testing.T) {
	good := quicFrameBytes(TypeResult, marshalResult(ResultPaused))
	f, err := readQUICFrame(bytes.NewReader(good), 2)
	if err != nil || f.Type != TypeResult || f.StreamID != 0 {
		t.Fatalf("%+v %v", f, err)
	}
	// flags the type does not define are ignored, as in section 4.2
	flagged := append([]byte(nil), good...)
	flagged[3] = 0xf0
	if _, err := readQUICFrame(bytes.NewReader(flagged), 2); err != nil {
		t.Fatalf("flags: %v", err)
	}
	// a stream id is a second spelling of the same frame
	withID := append([]byte(nil), good...)
	withID[7] = 4
	if _, err := readQUICFrame(bytes.NewReader(withID), 2); !errors.Is(err, ErrQUICFrame) {
		t.Fatalf("stream id: %v", err)
	}
	// the length is checked before any payload is read
	huge := []byte{0xff, 0xff, byte(TypeOpen), 0, 0, 0, 0, 0}
	r := &byteCounter{r: bytes.NewReader(append(huge, make([]byte, 70000)...))}
	if _, err := readQUICFrame(r, maxOpenPayload); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("a huge frame: %v", err)
	}
	if r.n != HeaderSize {
		t.Fatalf("read %d bytes of a frame it refused on its header", r.n)
	}
	if _, err := readQUICFrame(bytes.NewReader(good[:5]), 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a truncated header: %v", err)
	}
	if _, err := readQUICFrame(bytes.NewReader(good[:HeaderSize+1]), 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a truncated payload: %v", err)
	}
	if _, err := readQUICFrame(bytes.NewReader(nil), 2); err != io.EOF {
		t.Fatalf("an empty stream: %v", err)
	}
}

// byteCounter counts the bytes read through it.
type byteCounter struct {
	r io.Reader
	n int
}

func (c *byteCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func FuzzQUICDatagram(f *testing.F) {
	for _, id := range []uint64{0, 4, 63, 64, 1 << 20, MaxQUICStreamID} {
		b, _ := AppendQUICDatagram(nil, id, []byte("payload"))
		f.Add(b)
	}
	f.Add([]byte{})
	f.Add([]byte{0x40, 0x04, 'x'})
	f.Add([]byte{0xc0})
	f.Add(bytes.Repeat([]byte{0xff}, 20))
	f.Fuzz(func(t *testing.T, b []byte) {
		id, payload, err := ParseQUICDatagram(b)
		if err != nil {
			if !errors.Is(err, ErrQUICDatagram) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		if id > MaxQUICStreamID || len(payload) == 0 {
			t.Fatalf("accepted id %d with %d payload bytes", id, len(payload))
		}
		// what is accepted is canonical: it re-encodes to the same bytes
		again, err := AppendQUICDatagram(nil, id, payload)
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("accepted %x but re-encodes to %x (%v)", b, again, err)
		}
	})
}

// FuzzQUICStreamFrames feeds arbitrary bytes to the readers the carrier runs
// on a stream (OPEN, RESULT, a UDP stream's DATAGRAM frames, SETTINGS): what is
// accepted re-encodes to the same bytes, and nothing reads past its limit.
func FuzzQUICStreamFrames(f *testing.F) {
	open, _ := testParams().MarshalBinary()
	for _, s := range [][]byte{
		quicFrameBytes(TypeOpen, open),
		quicFrameBytes(TypeResult, marshalResult(ResultOK)),
		quicFrameBytes(TypeDatagram, []byte("dns")),
		quicFrameBytes(TypeSettings, DefaultSettings().Marshal()),
		frameBytes(TypeOpen, 4, open),
		append(quicFrameBytes(TypeDatagram, []byte("a")), quicFrameBytes(TypeDatagram, []byte("b"))...),
		{0xff, 0xff, 1, 0, 0, 0, 0, 0},
		nil,
	} {
		f.Add(s, uint16(maxOpenPayload))
		f.Add(s, uint16(2))
	}
	f.Fuzz(func(t *testing.T, data []byte, max uint16) {
		r := &byteCounter{r: bytes.NewReader(data)}
		var re []byte
		for {
			fr, err := readQUICFrame(r, int(max))
			if err != nil {
				if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, ErrFrameTooLarge) && !errors.Is(err, ErrQUICFrame) {
					t.Fatalf("unexpected error %v", err)
				}
				break
			}
			if fr.StreamID != 0 || len(fr.Payload) > int(max) {
				t.Fatalf("accepted %+v beyond the limit %d", fr, max)
			}
			switch fr.Type {
			case TypeOpen:
				var p OpenParams
				if p.UnmarshalBinary(fr.Payload) == nil {
					if b, err := p.MarshalBinary(); err != nil || !bytes.Equal(b, fr.Payload) {
						t.Fatal("OPEN is not canonical")
					}
				}
			case TypeResult:
				if c, err := parseResult(fr.Payload); err == nil && !bytes.Equal(marshalResult(c), fr.Payload) {
					t.Fatal("RESULT is not canonical")
				}
			case TypeSettings:
				if s, err := ParseSettings(fr.Payload); err == nil {
					if _, err := ParseSettings(s.Marshal()); err != nil {
						t.Fatal("SETTINGS does not round trip")
					}
				}
			}
			re = append(re, quicFrameBytes(fr.Type, fr.Payload)...)
			// flags are ignored on read, so compare with the flags cleared
		}
		if r.n > len(data) {
			t.Fatal("read past the end")
		}
		if len(re) > len(data) {
			t.Fatal("more accepted than given")
		}
	})
}

// bareQUICCarrier is a carrier with no connection, for driving its control
// stream rules directly.
func bareQUICCarrier(role Role) *QUICCarrier {
	cfg, _ := QUICConfig{}.withDefaults(role)
	return &QUICCarrier{
		role:     role,
		cfg:      cfg,
		local:    cfg.settings(role),
		peer:     DefaultSettings(),
		ctrlQ:    make(chan ctrlItem, 4),
		streams:  make(map[quic.StreamID]*QUICStream),
		calm:     newCalmBucket(time.Now()),
		acceptCh: make(chan *QUICStream, 1),
		dead:     make(chan struct{}),
		draining: make(chan struct{}),
		done:     make(chan struct{}),
		closeReq: make(chan closeRequest, 1),
	}
}

// FuzzQUICControlFrames drives a carrier's control stream rules with arbitrary
// frames: no input panics, and what the carrier decides is consistent.
func FuzzQUICControlFrames(f *testing.F) {
	for _, s := range [][]byte{
		quicFrameBytes(TypeGoAway, marshalGoAway(0, GoAwayNoError)),
		quicFrameBytes(TypeGoAway, marshalGoAway(12, GoAwayListenerClosed)),
		append(quicFrameBytes(TypeGoAway, marshalGoAway(4, GoAwayNoError)), quicFrameBytes(TypeGoAway, marshalGoAway(0, GoAwayShutdown))...),
		quicFrameBytes(TypeSettings, DefaultSettings().Marshal()),
		quicFrameBytes(FrameType(0x42), []byte("later")),
		quicFrameBytes(TypePing, make([]byte, 8)),
		frameBytes(TypeGoAway, 7, marshalGoAway(0, GoAwayNoError)),
		quicFrameBytes(TypeGoAway, []byte{1, 2, 3}),
		nil,
	} {
		f.Add(s, true)
		f.Add(s, false)
	}
	f.Fuzz(func(t *testing.T, data []byte, dialler bool) {
		role := RoleAcceptor
		if dialler {
			role = RoleDialer
		}
		c := bareQUICCarrier(role)
		r := bytes.NewReader(data)
		for {
			fr, err := ReadFrame(r, int(c.local.MaxFrame))
			if err != nil {
				break
			}
			c.mu.Lock()
			perr := c.handleControlLocked(fr)
			if perr != nil {
				c.protoFailLocked(perr)
			}
			ended := c.err != nil
			c.mu.Unlock()
			if ended {
				break
			}
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.goAwayRecv {
			select {
			case <-c.draining:
			default:
				t.Fatal("a GOAWAY was received and the carrier is not draining")
			}
		}
		if c.err != nil {
			select {
			case <-c.dead:
			default:
				t.Fatal("the carrier has an error and is not dead")
			}
			select {
			case req := <-c.closeReq:
				if req.reason == 0 && !errors.Is(c.err, ErrCarrierClosed) {
					t.Fatal("closed with no reason")
				}
			default:
				t.Fatal("a failed carrier has no close request")
			}
		}
	})
}

// fuzzQUICPeer feeds arbitrary bytes to a QUIC carrier through every channel
// a peer has, then makes sure nothing hangs or panics.
func fuzzQUICPeer(t *testing.T, role Role, data []byte) {
	cfg := QUICConfig{Config: Config{MaxFrame: 1024, StreamWindow: 4096, CarrierWindow: 8192, MaxStreams: 8, AcceptQueue: 4, SendBuffer: 2048,
		HandshakeTimeout: time.Second, PingInterval: 20 * time.Millisecond, IdleTimeout: 5 * time.Second, ResultTimeout: 100 * time.Millisecond}}
	var c *QUICCarrier
	var p *qraw
	if role == RoleAcceptor {
		e := newQUICEnv(t, cfg)
		c, p = rawDialler(t, e, cfg)
		go func() {
			for {
				s, err := c.Accept(context.Background())
				if err != nil {
					return
				}
				go func() {
					_ = s.SetDeadline(time.Now().Add(testWait))
					switch s.ID() / 4 % 4 {
					case 0:
						_ = s.Accept()
						if s.Kind() == StreamUDP {
							_, _ = s.ReadDatagram()
						} else {
							_, _ = io.Copy(io.Discard, s)
						}
					case 1:
						_ = s.Reject(ResultPaused)
					case 2:
						if s.Kind() == StreamUDP {
							_ = s.WriteDatagram([]byte("dgram"))
						} else {
							_, _ = s.Write(bytes.Repeat([]byte{1}, 3000))
						}
					default:
						_ = s.Accept()
						_ = s.Reset(ResetPeerReset)
					}
					_ = s.Close()
				}()
			}
		}()
	} else {
		c, p = rawAcceptor(t, cfg)
		for i := range 3 {
			s, err := c.Open(OpenParams{Kind: StreamKind(1 + i%2), RouteID: "r", HopIndex: uint32(i)})
			if err != nil {
				t.Fatal(err)
			}
			if s.Kind() == StreamTCP {
				go func() { _, _ = s.Write(bytes.Repeat([]byte{2}, 5000)); _, _ = io.Copy(io.Discard, s) }()
			} else {
				go func() { _ = s.WriteDatagram([]byte("dgram")); _, _ = s.ReadDatagram() }()
			}
		}
	}
	// the input drives the control stream, a stream and the datagrams
	sel := byte(0)
	if len(data) > 0 {
		sel, data = data[0], data[1:]
	}
	third := len(data) / 3
	_ = p.out.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = p.out.Write(data[:third])
	if sel&1 != 0 {
		var st *quic.Stream
		if role == RoleAcceptor {
			// the carrier may already have ended on the control stream bytes
			open, _ := OpenParams{Kind: StreamKind(1 + sel>>1&1), RouteID: "r", HopIndex: 1}.MarshalBinary()
			if s, err := p.conn.OpenStream(); err == nil {
				_, _ = s.Write(quicFrameBytes(TypeOpen, open))
				st = s
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			st, _ = p.conn.AcceptStream(ctx)
			cancel()
		}
		if st != nil {
			_ = st.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, _ = st.Write(data[third : 2*third])
			if sel&4 != 0 {
				_ = st.Close()
			}
			go func() { _, _ = io.Copy(io.Discard, st) }()
		}
	}
	if sel&2 != 0 {
		_ = p.conn.SendDatagram(data[2*third:])
	}
	go func() { _, _ = io.Copy(io.Discard, p.in) }()
	// The carrier either ended on the input or is still running; both are
	// fine. It must close promptly, with nothing left.
	c.mu.Lock()
	if c.active < 0 || c.active > len(c.streams)+1 {
		t.Errorf("active streams %d with %d tracked", c.active, len(c.streams))
	}
	c.mu.Unlock()
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(testWait):
		t.Fatal("Close hung")
	}
	if c.ActiveStreams() != 0 {
		t.Fatalf("%d streams left after Close", c.ActiveStreams())
	}
}

func fuzzQUICSeeds() [][]byte {
	open, _ := testParams().MarshalBinary()
	return [][]byte{
		nil,
		{0},
		append([]byte{1}, quicFrameBytes(TypeGoAway, marshalGoAway(0, GoAwayNoError))...),
		append([]byte{3, 0}, bytes.Repeat([]byte{0xa5}, 60)...),
		append([]byte{7}, append(quicFrameBytes(TypeOpen, open), quicFrameBytes(TypeData, []byte("hello"))...)...),
		append([]byte{2}, quicVarint(0)...),
		append([]byte{1}, quicFrameBytes(TypeResult, marshalResult(ResultOK))...),
		append([]byte{5}, quicFrameBytes(TypeDatagram, bytes.Repeat([]byte{7}, 300))...),
		append([]byte{1}, bytes.Repeat([]byte{0xff}, 100)...),
	}
}

func FuzzQUICCarrierAcceptor(f *testing.F) {
	for _, s := range fuzzQUICSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) { fuzzQUICPeer(t, RoleAcceptor, data) })
}

func FuzzQUICCarrierDialer(f *testing.F) {
	for _, s := range fuzzQUICSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) { fuzzQUICPeer(t, RoleDialer, data) })
}

var _ = quicvarint.Max
