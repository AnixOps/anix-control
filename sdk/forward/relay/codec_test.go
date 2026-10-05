package relay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand/v2"
	"net/netip"
	"testing"
	"testing/iotest"
)

func TestFrameHeaderLayout(t *testing.T) {
	b, err := AppendFrame(nil, Frame{Type: TypeData, Flags: FlagFIN, StreamID: 0x01020304, Payload: []byte{0xaa, 0xbb, 0xcc}})
	if err != nil {
		t.Fatal(err)
	}
	// | length (16) | type (8) | flags (8) | stream id (32) | payload |
	want := []byte{0, 3, byte(TypeData), FlagFIN, 1, 2, 3, 4, 0xaa, 0xbb, 0xcc}
	if !bytes.Equal(b, want) {
		t.Fatalf("encoding % x, want % x", b, want)
	}
}

func TestFrameTypeValuesAreTheDocuments(t *testing.T) {
	want := map[FrameType]struct {
		v    uint8
		name string
	}{
		TypeSettings: {0x0, "SETTINGS"}, TypeOpen: {0x1, "OPEN"}, TypeResult: {0x2, "RESULT"}, TypeData: {0x3, "DATA"},
		TypeWindow: {0x4, "WINDOW"}, TypeReset: {0x5, "RESET"}, TypePing: {0x6, "PING"}, TypeGoAway: {0x7, "GOAWAY"},
		TypeDatagram: {0x8, "DATAGRAM"},
	}
	for typ, w := range want {
		if uint8(typ) != w.v || typ.String() != w.name {
			t.Errorf("%v: value %#x name %q, want %#x %q", typ, uint8(typ), typ.String(), w.v, w.name)
		}
	}
	if FlagFIN != 0x1 || FlagACK != 0x2 {
		t.Error("flag values")
	}
	if got := FrameType(0x7f).String(); got != "type(0x7f)" {
		t.Error(got)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 255, 256, 16384, 65535} {
		for _, id := range []uint32{0, 1, 3, MaxStreamID, 1<<32 - 1} {
			for _, typ := range []FrameType{TypeData, TypeSettings, TypeDatagram, 0xfe} {
				f := Frame{Type: typ, Flags: byte(n), StreamID: id, Payload: pattern(n, byte(id))}
				if n == 0 {
					f.Payload = nil
				}
				var buf bytes.Buffer
				if err := WriteFrame(&buf, f); err != nil {
					t.Fatal(err)
				}
				got, err := ReadFrame(&buf, MaxMaxFrame)
				if err != nil {
					t.Fatalf("n=%d: %v", n, err)
				}
				if got.Type != f.Type || got.Flags != f.Flags || got.StreamID != f.StreamID || !bytes.Equal(got.Payload, f.Payload) {
					t.Fatalf("round trip changed the frame: %+v -> %+v", f, got)
				}
				if buf.Len() != 0 {
					t.Fatalf("%d bytes left over", buf.Len())
				}
			}
		}
	}
}

func TestFrameTooLargeToEncode(t *testing.T) {
	if _, err := AppendFrame(nil, Frame{Payload: make([]byte, MaxMaxFrame+1)}); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatal(err)
	}
	if err := WriteFrame(io.Discard, Frame{Payload: make([]byte, MaxMaxFrame+1)}); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatal(err)
	}
}

// countingReader fails the test if more than limit bytes are read.
type countingReader struct {
	t     *testing.T
	r     io.Reader
	n     int
	limit int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	if c.n > c.limit {
		c.t.Fatalf("read %d bytes, more than the %d allowed", c.n, c.limit)
	}
	return n, err
}

func TestReadFrameRefusesOversizeBeforeReadingIt(t *testing.T) {
	hdr := mustFrame(t, Frame{Type: TypeData, StreamID: 1, Payload: make([]byte, 60000)})
	r := &countingReader{t: t, r: bytes.NewReader(hdr), limit: HeaderSize}
	if _, err := ReadFrame(r, 1024); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err = %v", err)
	}
	// Exactly at the limit is fine.
	if _, err := ReadFrame(bytes.NewReader(mustFrame(t, Frame{Payload: make([]byte, 1024)})), 1024); err != nil {
		t.Fatal(err)
	}
}

func TestReadFrameTruncation(t *testing.T) {
	full := mustFrame(t, Frame{Type: TypeData, StreamID: 1, Payload: []byte("payload")})
	if _, err := ReadFrame(bytes.NewReader(nil), MaxMaxFrame); err != io.EOF {
		t.Fatalf("empty stream: %v, want io.EOF", err)
	}
	for cut := 1; cut < len(full); cut++ {
		if _, err := ReadFrame(bytes.NewReader(full[:cut]), MaxMaxFrame); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("cut at %d: %v, want io.ErrUnexpectedEOF", cut, err)
		}
	}
}

// Frames decode the same however the bytes arrive.
func TestFrameStreamProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for iter := range 200 {
		var want []Frame
		var wire bytes.Buffer
		for range 1 + rng.IntN(20) {
			f := Frame{Type: FrameType(rng.IntN(12)), Flags: byte(rng.IntN(256)), StreamID: rng.Uint32()}
			switch rng.IntN(3) {
			case 0:
			case 1:
				f.Payload = pattern(1+rng.IntN(64), byte(rng.IntN(256)))
			default:
				f.Payload = pattern(1+rng.IntN(MaxMaxFrame), byte(rng.IntN(256)))
			}
			want = append(want, f)
			if err := WriteFrame(&wire, f); err != nil {
				t.Fatal(err)
			}
		}
		readers := []io.Reader{
			bytes.NewReader(wire.Bytes()),
			iotest.OneByteReader(bytes.NewReader(wire.Bytes())),
			iotest.HalfReader(bytes.NewReader(wire.Bytes())),
			iotest.DataErrReader(bytes.NewReader(wire.Bytes())),
		}
		for ri, r := range readers {
			for i, w := range want {
				got, err := ReadFrame(r, MaxMaxFrame)
				if err != nil {
					t.Fatalf("iter %d reader %d frame %d: %v", iter, ri, i, err)
				}
				if got.Type != w.Type || got.Flags != w.Flags || got.StreamID != w.StreamID || !bytes.Equal(got.Payload, w.Payload) {
					t.Fatalf("iter %d reader %d frame %d differs", iter, ri, i)
				}
			}
			if _, err := ReadFrame(r, MaxMaxFrame); err != io.EOF {
				t.Fatalf("iter %d reader %d: after the last frame: %v", iter, ri, err)
			}
		}
	}
}

func TestSettings(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		for _, s := range []Settings{
			DefaultSettings(),
			{MaxStreams: 0, MaxFrame: MinMaxFrame, StreamWindow: MinWindow, CarrierWindow: MinWindow},
			{MaxStreams: MaxMaxStreams, MaxFrame: MaxMaxFrame, StreamWindow: MaxStreamWindow, CarrierWindow: MaxCarrierWindow},
		} {
			got, err := ParseSettings(s.Marshal())
			if err != nil || got != s {
				t.Fatalf("%+v -> %+v, %v", s, got, err)
			}
		}
	})
	t.Run("wire format", func(t *testing.T) {
		b := Settings{MaxStreams: 5, MaxFrame: 2048, StreamWindow: 8192, CarrierWindow: 16384}.Marshal()
		// (u16 key, u32 value) pairs
		want := []byte{0, 1, 0, 0, 0, 5, 0, 2, 0, 0, 8, 0, 0, 3, 0, 0, 0x20, 0, 0, 4, 0, 0, 0x40, 0}
		if !bytes.Equal(b, want) {
			t.Fatalf("% x", b)
		}
	})
	pair := func(k uint16, v uint32) []byte {
		return binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint16(nil, k), v)
	}
	t.Run("missing keys keep their defaults, unknown keys are ignored", func(t *testing.T) {
		b := append(pair(SettingMaxStreams, 3), pair(0x7777, 99)...)
		b = append(b, pair(0, 1)...)
		got, err := ParseSettings(b)
		want := DefaultSettings()
		want.MaxStreams = 3
		if err != nil || got != want {
			t.Fatalf("%+v %v", got, err)
		}
		if got, err := ParseSettings(nil); err != nil || got != DefaultSettings() {
			t.Fatalf("empty: %+v %v", got, err)
		}
	})
	t.Run("errors", func(t *testing.T) {
		for name, b := range map[string][]byte{
			"partial pair":            {0, 1, 0, 0, 0},
			"duplicate known key":     append(pair(SettingMaxFrame, 2048), pair(SettingMaxFrame, 2048)...),
			"max streams too large":   pair(SettingMaxStreams, MaxMaxStreams+1),
			"max frame too small":     pair(SettingMaxFrame, MinMaxFrame-1),
			"max frame too large":     pair(SettingMaxFrame, MaxMaxFrame+1),
			"stream window too small": pair(SettingStreamWindow, MinWindow-1),
			"stream window too large": pair(SettingStreamWindow, MaxStreamWindow+1),
			"carrier window small":    pair(SettingCarrierWindow, 0),
			"carrier window large":    pair(SettingCarrierWindow, MaxCarrierWindow+1),
		} {
			if _, err := ParseSettings(b); err == nil {
				t.Errorf("%s accepted", name)
			}
		}
	})
}

func TestOpenParams(t *testing.T) {
	const route = "01HZZZZZZZZZZZZZZZZZZZZZZZ"
	valid := []OpenParams{
		{Kind: StreamTCP, RouteID: route},
		{Kind: StreamUDP, RouteID: "r", HopIndex: 1<<32 - 1},
		{Kind: StreamTCP, RouteID: route, HopIndex: 2, Client: netip.MustParseAddrPort("192.0.2.1:65535")},
		{Kind: StreamTCP, RouteID: route, Client: netip.MustParseAddrPort("[2001:db8::1]:0")},
		{Kind: StreamTCP, RouteID: string(bytes.Repeat([]byte("a"), MaxRouteIDLen))},
	}
	for _, p := range valid {
		b, err := p.MarshalBinary()
		if err != nil {
			t.Fatalf("%+v: %v", p, err)
		}
		var got OpenParams
		if err := got.UnmarshalBinary(b); err != nil || got != p {
			t.Fatalf("%+v -> %+v, %v", p, got, err)
		}
	}
	t.Run("wire format", func(t *testing.T) {
		b, _ := OpenParams{Kind: StreamTCP, RouteID: "ab", HopIndex: 7, Client: netip.MustParseAddrPort("1.2.3.4:80")}.MarshalBinary()
		want := []byte{1, 0, 0, 0, 7, 2, 'a', 'b', 4, 1, 2, 3, 4, 0, 80}
		if !bytes.Equal(b, want) {
			t.Fatalf("% x", b)
		}
		b, _ = OpenParams{Kind: StreamUDP, RouteID: "ab"}.MarshalBinary()
		if want := []byte{2, 0, 0, 0, 0, 2, 'a', 'b', 0}; !bytes.Equal(b, want) {
			t.Fatalf("% x", b)
		}
	})
	t.Run("an IPv4-mapped client is sent as IPv4", func(t *testing.T) {
		b, err := OpenParams{Kind: StreamTCP, RouteID: "r", Client: netip.MustParseAddrPort("[::ffff:192.0.2.1]:9")}.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		var got OpenParams
		if err := got.UnmarshalBinary(b); err != nil || got.Client != netip.MustParseAddrPort("192.0.2.1:9") {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("cannot be sent", func(t *testing.T) {
		for name, p := range map[string]OpenParams{
			"no kind":       {RouteID: route},
			"unknown kind":  {Kind: 3, RouteID: route},
			"no route id":   {Kind: StreamTCP},
			"long route id": {Kind: StreamTCP, RouteID: string(bytes.Repeat([]byte("a"), MaxRouteIDLen+1))},
			"space":         {Kind: StreamTCP, RouteID: "a b"},
			"newline":       {Kind: StreamTCP, RouteID: "a\nb"},
			"non-ASCII":     {Kind: StreamTCP, RouteID: "r\xc3\xa9"},
			"zoned client":  {Kind: StreamTCP, RouteID: route, Client: netip.MustParseAddrPort("[fe80::1%eth0]:1")},
		} {
			if _, err := p.MarshalBinary(); !errors.Is(err, ErrInvalidOpen) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
	t.Run("cannot be accepted", func(t *testing.T) {
		good, _ := OpenParams{Kind: StreamTCP, RouteID: "ab", Client: netip.MustParseAddrPort("1.2.3.4:80")}.MarshalBinary()
		v6, _ := OpenParams{Kind: StreamTCP, RouteID: "ab", Client: netip.MustParseAddrPort("[2001:db8::1]:80")}.MarshalBinary()
		mapped := append([]byte(nil), v6...)
		copy(mapped[9:], netip.MustParseAddr("::ffff:1.2.3.4").AsSlice())
		clone := func(f func(b []byte) []byte) []byte { return f(append([]byte(nil), good...)) }
		for name, b := range map[string][]byte{
			"empty":                 nil,
			"short":                 good[:5],
			"truncated route id":    good[:7],
			"missing family":        good[:8],
			"truncated address":     good[:len(good)-1],
			"trailing byte":         append(append([]byte(nil), good...), 0),
			"trailing after none":   {1, 0, 0, 0, 0, 2, 'a', 'b', 0, 0},
			"unknown kind":          clone(func(b []byte) []byte { b[0] = 9; return b }),
			"zero route id length":  {1, 0, 0, 0, 0, 0, 0},
			"unknown family":        clone(func(b []byte) []byte { b[8] = 5; return b }),
			"mapped address as v6":  mapped,
			"control byte in route": clone(func(b []byte) []byte { b[6] = 0x07; return b }),
		} {
			var p OpenParams
			if err := p.UnmarshalBinary(b); !errors.Is(err, ErrInvalidOpen) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

func TestSmallPayloads(t *testing.T) {
	for _, c := range []ResultCode{0, 1, 8, 65535} {
		if got, err := parseResult(marshalResult(c)); err != nil || got != c {
			t.Fatal(c, got, err)
		}
	}
	for _, r := range []ResetReason{0, 1, 7, 65535} {
		if got, err := parseReset(marshalReset(r)); err != nil || got != r {
			t.Fatal(r, got, err)
		}
	}
	for _, n := range []uint32{1, 4096, 1<<32 - 1} {
		if got, err := parseWindow(marshalWindow(n)); err != nil || got != n {
			t.Fatal(n, got, err)
		}
	}
	if _, err := parseWindow(marshalWindow(0)); err == nil {
		t.Fatal("zero WINDOW accepted")
	}
	for _, last := range []uint32{0, 1, MaxStreamID} {
		l, r, err := parseGoAway(marshalGoAway(last, GoAwayStuck))
		if err != nil || l != last || r != GoAwayStuck {
			t.Fatal(last, l, r, err)
		}
	}
	if _, _, err := parseGoAway(marshalGoAway(MaxStreamID+1, 0)); err == nil {
		t.Fatal("GOAWAY past the last stream id accepted")
	}
	for _, f := range []func([]byte) error{
		func(b []byte) error { _, err := parseResult(b); return err },
		func(b []byte) error { _, err := parseReset(b); return err },
		func(b []byte) error { _, err := parseWindow(b); return err },
		func(b []byte) error { _, _, err := parseGoAway(b); return err },
	} {
		for _, n := range []int{0, 1, 3, 5, 7, 8} {
			b := bytes.Repeat([]byte{1}, n)
			_ = f(b) // none may panic; lengths other than the fixed one are errors
		}
	}
	if _, err := parseResult(make([]byte, 3)); err == nil {
		t.Fatal("long RESULT accepted")
	}
}

func TestCodeNames(t *testing.T) {
	for c := ResultCode(0); c <= ResultInternal; c++ {
		if c.String() == "" || c.String()[0] == 'r' && len(c.String()) > 6 && c.String()[:7] == "result(" {
			t.Errorf("result %d has no name", c)
		}
	}
	for r := ResetProtocolError; r <= ResetInternal; r++ {
		if len(r.String()) > 6 && r.String()[:6] == "reset(" {
			t.Errorf("reset %d has no name", r)
		}
	}
	for r := GoAwayNoError; r <= GoAwayInternal; r++ {
		if len(r.String()) > 7 && r.String()[:7] == "goaway(" {
			t.Errorf("goaway %d has no name", r)
		}
	}
	if ResultCode(77).String() != "result(77)" || ResetReason(77).String() != "reset(77)" || GoAwayReason(77).String() != "goaway(77)" {
		t.Error("unknown values")
	}
	if StreamTCP.String() != "tcp" || StreamUDP.String() != "udp" || StreamKind(9).String() != "kind(9)" {
		t.Error("kinds")
	}
	if RoleDialer.String() != "dialer" || RoleAcceptor.String() != "acceptor" || Role(9).String() != "role(9)" {
		t.Error("roles")
	}
}
