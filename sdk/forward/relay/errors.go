package relay

import (
	"errors"
	"fmt"
	"net"
)

// Errors a Carrier or a Stream returns. Use errors.Is and errors.As: most
// are wrapped with detail.
var (
	// ErrCarrierClosed means the carrier ended; the error wraps the cause
	// (the peer's GOAWAY, an I/O error, a protocol error, the idle timeout).
	ErrCarrierClosed = errors.New("relay: carrier closed")
	// ErrGoAway means the carrier takes no new streams: a GOAWAY was sent or
	// received, so open the stream on another carrier.
	ErrGoAway = errors.New("relay: carrier takes no new streams")
	// ErrStreamLimit means the carrier is at the peer's concurrent stream
	// limit (SETTINGS_MAX_STREAMS); open the stream on another carrier.
	ErrStreamLimit = errors.New("relay: carrier is at its stream limit")
	// ErrIDsExhausted means every stream id of the carrier has been used (at
	// 2^31 the dialler moves to a new carrier).
	ErrIDsExhausted = errors.New("relay: carrier is out of stream ids")
	// ErrRefused marks a stream that never started: the carrier refused the
	// OPEN (stream limit, draining) or went away before answering it. The
	// caller may retry it on another carrier without anything having been
	// delivered.
	ErrRefused = errors.New("relay: stream refused by the carrier")
	// ErrOpenTimeout means no RESULT came for an OPEN within the result
	// timeout on a carrier that otherwise answers pings (L5); the carrier is
	// retired and the stream may be retried on another.
	ErrOpenTimeout = errors.New("relay: no RESULT for OPEN in time")
	// ErrIdleTimeout means nothing was received for the idle timeout (L4).
	ErrIdleTimeout = errors.New("relay: nothing received for the idle timeout")
	// ErrStreamClosed means the stream was closed by this side. It matches
	// net.ErrClosed.
	ErrStreamClosed = fmt.Errorf("relay: stream closed: %w", net.ErrClosed)
	// ErrStreamKind means the operation does not fit the stream's kind
	// (Read and Write are for TCP streams, datagrams for UDP streams).
	ErrStreamKind = errors.New("relay: operation does not fit the stream kind")
	// ErrWrongRole means a dialler-only or listener-only operation was
	// called on the other end of the carrier.
	ErrWrongRole = errors.New("relay: operation not available to this end of the carrier")
	// ErrAlreadyAnswered means the stream already got its RESULT.
	ErrAlreadyAnswered = errors.New("relay: stream already answered")
	// ErrDatagramDropped means a datagram was dropped because the stream's
	// window or send buffer was full, as a full socket buffer would drop it
	// (anixops-protocol.md section 4.8). It is counted in Stats.
	ErrDatagramDropped = errors.New("relay: datagram dropped, no credit")
	// ErrDatagramSize means a datagram is empty or larger than the peer's
	// frame limit.
	ErrDatagramSize = errors.New("relay: datagram is empty or larger than the frame limit")
)

// ProtocolError is a rule of the protocol the peer broke (or this end's
// own settings that cannot be used): the carrier ended with a GOAWAY of the
// given reason.
type ProtocolError struct {
	Reason GoAwayReason
	Detail string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("relay: protocol error (%s): %s", e.Reason, e.Detail)
}

// ResultError is a RESULT other than success: the listening hop refused the
// stream.
type ResultError struct {
	Code ResultCode
}

func (e *ResultError) Error() string { return "relay: stream refused by the hop: " + e.Code.String() }

// StreamResetError is a stream aborted by a RESET, from the peer (Remote) or
// from this side.
type StreamResetError struct {
	Reason ResetReason
	Remote bool
}

func (e *StreamResetError) Error() string {
	who := "local"
	if e.Remote {
		who = "remote"
	}
	return fmt.Sprintf("relay: stream reset (%s, %s)", who, e.Reason)
}

// Is makes a stream the peer reset as refused match ErrRefused.
func (e *StreamResetError) Is(target error) bool {
	return target == ErrRefused && e.Remote && e.Reason == ResetRefusedStream
}

// carrierError wraps the cause that ended a carrier for the streams on it.
func carrierError(cause error) error {
	if errors.Is(cause, ErrCarrierClosed) {
		return cause
	}
	return fmt.Errorf("%w: %w", ErrCarrierClosed, cause)
}
