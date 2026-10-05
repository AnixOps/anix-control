package relay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// ALPN is the application protocol a TLS carrier negotiates. It names the
// prototype wire format of this package, which may change without notice and
// never interoperates with the production versions ("anixops/1" and later,
// anixops-protocol.md sections 6.7 and 9.1).
const ALPN = "anixops/0"

// The frame header and the hard caps of the wire format
// (anixops-protocol.md section 4.2). Every frame is an 8-byte big-endian
// header followed by Length bytes of payload:
//
//	| length (16) | type (8) | flags (8) | stream id (32) | payload |
const (
	// HeaderSize is the size of a frame header.
	HeaderSize = 8

	// DefaultMaxFrame is the largest payload a receiver accepts unless its
	// SETTINGS say otherwise.
	DefaultMaxFrame = 16 * 1024
	// MinMaxFrame and MaxMaxFrame bound SETTINGS_MAX_FRAME. The upper bound
	// is the largest value the 16-bit length field can carry.
	MinMaxFrame = 1024
	MaxMaxFrame = 65535

	// MaxStreamID is the largest stream id: ids are odd, increasing and
	// never reused, so a carrier carries at most 2^30 streams over its
	// lifetime; the dialler then moves to a new carrier.
	MaxStreamID = 1<<31 - 1
)

// FrameType is the type byte of a frame header.
type FrameType uint8

// The frame types of anixops-protocol.md section 4.2. A receiver ignores an
// unknown type on stream 0 and resets the stream it names otherwise.
const (
	TypeSettings FrameType = 0x0 // stream 0: key/value pairs, first frame each side sends
	TypeOpen     FrameType = 0x1 // new stream: stream kind and open parameters
	TypeResult   FrameType = 0x2 // existing stream: u16 result code, once per OPEN
	TypeData     FrameType = 0x3 // existing stream: payload bytes, flag FIN
	TypeWindow   FrameType = 0x4 // stream 0 or existing: u32 credit increment
	TypeReset    FrameType = 0x5 // existing stream: u16 reason
	TypePing     FrameType = 0x6 // stream 0: 8 opaque bytes, flag ACK on the answer
	TypeGoAway   FrameType = 0x7 // stream 0: last accepted stream id and a u16 reason
	TypeDatagram FrameType = 0x8 // existing UDP stream: one datagram
)

// String returns the name used in the protocol document.
func (t FrameType) String() string {
	switch t {
	case TypeSettings:
		return "SETTINGS"
	case TypeOpen:
		return "OPEN"
	case TypeResult:
		return "RESULT"
	case TypeData:
		return "DATA"
	case TypeWindow:
		return "WINDOW"
	case TypeReset:
		return "RESET"
	case TypePing:
		return "PING"
	case TypeGoAway:
		return "GOAWAY"
	case TypeDatagram:
		return "DATAGRAM"
	}
	return fmt.Sprintf("type(0x%02x)", uint8(t))
}

// Frame flags. A flag a frame type does not define is ignored.
const (
	// FlagFIN on DATA ends the sender's direction of the stream.
	FlagFIN uint8 = 0x1
	// FlagACK on PING marks the answer to a ping.
	FlagACK uint8 = 0x2
)

// Frame is one decoded frame. Payload is owned by the caller.
type Frame struct {
	Type     FrameType
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

// ErrFrameTooLarge reports a frame whose length exceeds the limit the
// reader was given; the carrier it arrived on cannot continue.
var ErrFrameTooLarge = errors.New("relay: frame exceeds the maximum frame size")

// AppendFrame appends the encoding of f to dst. It fails only when the
// payload does not fit the 16-bit length field.
func AppendFrame(dst []byte, f Frame) ([]byte, error) {
	n := len(f.Payload)
	if n > math.MaxUint16 {
		return dst, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}
	dst = binary.BigEndian.AppendUint16(dst, uint16(n))
	dst = append(dst, byte(f.Type), f.Flags)
	dst = binary.BigEndian.AppendUint32(dst, f.StreamID)
	return append(dst, f.Payload...), nil
}

// WriteFrame writes the encoding of f to w with one Write call.
func WriteFrame(w io.Writer, f Frame) error {
	buf, err := AppendFrame(make([]byte, 0, HeaderSize+len(f.Payload)), f)
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

// ReadFrame reads one frame. A payload longer than maxPayload is refused
// with ErrFrameTooLarge before any of it is read or allocated. A stream that
// ends before a frame starts returns io.EOF; one that ends inside a frame
// returns io.ErrUnexpectedEOF.
func ReadFrame(r io.Reader, maxPayload int) (Frame, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	return readFramePayload(r, hdr, maxPayload)
}

func readFramePayload(r io.Reader, hdr [HeaderSize]byte, maxPayload int) (Frame, error) {
	n := int(binary.BigEndian.Uint16(hdr[0:2]))
	if n > maxPayload {
		return Frame{}, fmt.Errorf("%w: %d > %d bytes", ErrFrameTooLarge, n, maxPayload)
	}
	f := Frame{Type: FrameType(hdr[2]), Flags: hdr[3], StreamID: binary.BigEndian.Uint32(hdr[4:8])}
	if n > 0 {
		f.Payload = make([]byte, n)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return Frame{}, err
		}
	}
	return f, nil
}
