package relay

import (
	"errors"
	"fmt"
	"io"

	"github.com/quic-go/quic-go/quicvarint"
)

// The QUIC carrier maps streams to native QUIC streams (anixops-protocol.md
// section 4.1), so most of the frame protocol of section 4.2 is QUIC's own
// (data, flow control, reset, ping, close). What QUIC does not carry still
// travels in frames of the encoding of section 4.2, with stream id 0 and no
// flags, because the QUIC stream is the stream:
//
//	control streams  one unidirectional stream per end, opened at once:
//	                 SETTINGS first, once; then GOAWAY; then (when the end has no
//	                 streams left) a FIN, never anything else
//	stream           the dialler's first frame on a new bidirectional stream is
//	                 OPEN; the listener's first is RESULT; from then on a TCP
//	                 stream carries raw bytes in both directions, and a UDP
//	                 stream carries DATAGRAM frames (UDP over a stream)
//	datagram         a QUIC DATAGRAM frame holding the association's stream id
//	                 (a QUIC variable-length integer) and the payload
//
// Every parser here accepts exactly one encoding of what it parses, like the
// parsers of the TCP carrier, which the fuzz targets check.

// maxOpenPayload is the largest valid OPEN payload: kind, hop index, route
// id (length byte and MaxRouteIDLen bytes), the address family and an IPv6
// address with its port.
const maxOpenPayload = 1 + 4 + 1 + MaxRouteIDLen + 1 + 16 + 2

// resultPayloadLen is the size of a RESULT payload.
const resultPayloadLen = 2

// ErrQUICFrame reports a frame on a QUIC stream that breaks the mapping: a
// stream id other than zero.
var ErrQUICFrame = errors.New("relay: invalid frame on a QUIC stream")

// readQUICFrame reads one frame from a QUIC stream. The payload length is
// checked against maxPayload before any of it is read, and the header must
// have stream id 0: the stream is the QUIC stream, and an encoding with an id
// would be a second spelling of the same frame. Flags the frame type does not
// define are ignored, as in section 4.2.
func readQUICFrame(r io.Reader, maxPayload int) (Frame, error) {
	f, err := ReadFrame(r, maxPayload)
	if err != nil {
		return Frame{}, err
	}
	if f.StreamID != 0 {
		return Frame{}, fmt.Errorf("%w: %s with stream id %d", ErrQUICFrame, f.Type, f.StreamID)
	}
	return f, nil
}

// quicFrameBytes encodes one frame of the QUIC mapping.
func quicFrameBytes(typ FrameType, payload []byte) []byte {
	b, err := AppendFrame(make([]byte, 0, HeaderSize+len(payload)), Frame{Type: typ, Payload: payload})
	if err != nil { // a payload of more than 65535 bytes: callers bound theirs
		panic("relay: " + err.Error())
	}
	return b
}

// MaxQUICStreamID is the largest value of a QUIC variable-length integer:
// 2^62-1, the range of a stream id.
const MaxQUICStreamID = quicvarint.Max

// ErrQUICDatagram reports a QUIC DATAGRAM frame that is not a valid UDP
// datagram of an association.
var ErrQUICDatagram = errors.New("relay: invalid UDP datagram on a QUIC carrier")

// AppendQUICDatagram appends the content of a QUIC DATAGRAM frame, the way a
// UDP datagram of an association rides a QUIC carrier (anixops-protocol.md
// section 4.8): the association's stream id as a QUIC variable-length
// integer, in its shortest encoding, followed by the payload, which is never
// empty.
func AppendQUICDatagram(dst []byte, streamID uint64, payload []byte) ([]byte, error) {
	if streamID > MaxQUICStreamID {
		return dst, fmt.Errorf("%w: stream id %d", ErrQUICDatagram, streamID)
	}
	if len(payload) == 0 {
		return dst, fmt.Errorf("%w: empty payload", ErrQUICDatagram)
	}
	return append(quicvarint.Append(dst, streamID), payload...), nil
}

// ParseQUICDatagram decodes the content of a QUIC DATAGRAM frame. It accepts
// exactly what AppendQUICDatagram produces: a variable-length integer that is
// not in its shortest form, or no payload after it, is an error. The payload
// aliases b.
func ParseQUICDatagram(b []byte) (streamID uint64, payload []byte, err error) {
	id, n, err := quicvarint.Parse(b)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrQUICDatagram, err)
	}
	if n != quicvarint.Len(id) {
		return 0, nil, fmt.Errorf("%w: stream id %d in a longer form than needed", ErrQUICDatagram, id)
	}
	if len(b) == n {
		return 0, nil, fmt.Errorf("%w: empty payload", ErrQUICDatagram)
	}
	return id, b[n:], nil
}

// quicDatagramHeaderLen is the length of the stream id prefix of a datagram.
func quicDatagramHeaderLen(streamID uint64) int { return quicvarint.Len(streamID) }
