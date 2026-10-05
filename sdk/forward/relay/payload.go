package relay

import (
	"encoding/binary"
	"fmt"
	"math"
)

// The fixed-size payloads of the small frame types. Every parser is strict:
// a payload of any other length is an error, so a frame has one encoding.

// pingSize is the payload of PING: 8 opaque bytes the answer repeats.
const pingSize = 8

func marshalResult(c ResultCode) []byte { return binary.BigEndian.AppendUint16(nil, uint16(c)) }

func parseResult(b []byte) (ResultCode, error) {
	if len(b) != 2 {
		return 0, fmt.Errorf("RESULT payload of %d bytes, want 2", len(b))
	}
	return ResultCode(binary.BigEndian.Uint16(b)), nil
}

func marshalReset(r ResetReason) []byte { return binary.BigEndian.AppendUint16(nil, uint16(r)) }

func parseReset(b []byte) (ResetReason, error) {
	if len(b) != 2 {
		return 0, fmt.Errorf("RESET payload of %d bytes, want 2", len(b))
	}
	return ResetReason(binary.BigEndian.Uint16(b)), nil
}

func marshalWindow(increment uint32) []byte { return binary.BigEndian.AppendUint32(nil, increment) }

// parseWindow returns the credit increment, which must be positive.
func parseWindow(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, fmt.Errorf("WINDOW payload of %d bytes, want 4", len(b))
	}
	n := binary.BigEndian.Uint32(b)
	if n == 0 {
		return 0, fmt.Errorf("WINDOW increment is zero")
	}
	return n, nil
}

func marshalGoAway(last uint32, r GoAwayReason) []byte {
	b := binary.BigEndian.AppendUint32(make([]byte, 0, 6), last)
	return binary.BigEndian.AppendUint16(b, uint16(r))
}

func parseGoAway(b []byte) (last uint32, r GoAwayReason, err error) {
	if len(b) != 6 {
		return 0, 0, fmt.Errorf("GOAWAY payload of %d bytes, want 6", len(b))
	}
	last = binary.BigEndian.Uint32(b)
	if last > MaxStreamID {
		return 0, 0, fmt.Errorf("GOAWAY last stream id %d exceeds %d", last, MaxStreamID)
	}
	return last, GoAwayReason(binary.BigEndian.Uint16(b[4:])), nil
}

// creditIncrement converts the credit a receiver returns, bounded by the
// window sizes, to the u32 of a WINDOW frame.
func creditIncrement(n int64) uint32 {
	switch {
	case n < 1:
		return 1
	case n > math.MaxUint32:
		return math.MaxUint32
	}
	return uint32(n)
}
