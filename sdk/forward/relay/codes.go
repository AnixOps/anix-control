package relay

import "fmt"

// ResultCode is the u16 of a RESULT frame: how the listening hop answered an
// OPEN. The listener sends exactly one per stream; zero is success and
// every other value refuses the stream (anixops-protocol.md section 4.3).
// A receiver treats a value it does not know as a failure.
type ResultCode uint16

const (
	ResultOK                  ResultCode = 0
	ResultUpstreamUnreachable ResultCode = 1 // no upstream of the hop could be dialled
	ResultAdmissionDenied     ResultCode = 2 // the hop's admission refuses the stream
	ResultPaused              ResultCode = 3 // the route is paused
	ResultQuotaExceeded       ResultCode = 4 // the route's byte quota is used up
	ResultLimitExceeded       ResultCode = 5 // a connection or resource limit of the hop
	ResultRouteMismatch       ResultCode = 6 // the (route id, hop index) pair is not the listener's own
	ResultNoHop               ResultCode = 7 // the process runs no hop for the stream (L2)
	ResultInternal            ResultCode = 8 // the listening hop failed
)

// String returns the name used in logs and metrics labels.
func (c ResultCode) String() string {
	switch c {
	case ResultOK:
		return "ok"
	case ResultUpstreamUnreachable:
		return "upstream_unreachable"
	case ResultAdmissionDenied:
		return "admission_denied"
	case ResultPaused:
		return "paused"
	case ResultQuotaExceeded:
		return "quota_exceeded"
	case ResultLimitExceeded:
		return "limit_exceeded"
	case ResultRouteMismatch:
		return "route_mismatch"
	case ResultNoHop:
		return "no_hop"
	case ResultInternal:
		return "internal"
	}
	return fmt.Sprintf("result(%d)", uint16(c))
}

// ResetReason is the u16 of a RESET frame, which aborts both directions of a
// stream. A receiver treats a value it does not know like ResetCancel.
type ResetReason uint16

const (
	ResetProtocolError ResetReason = 1 // the stream's peer broke a rule of the protocol
	ResetFlowControl   ResetReason = 2 // the stream's credit was exceeded or overflowed
	ResetUnknownFrame  ResetReason = 3 // a frame type this version does not know arrived on the stream
	ResetRefusedStream ResetReason = 4 // carrier-level refusal (stream limit, draining); retry on another carrier
	ResetPeerReset     ResetReason = 5 // the client or target behind the stream reset its connection
	ResetCancel        ResetReason = 6 // one side closed the stream before the other finished
	ResetInternal      ResetReason = 7 // the sending side failed
)

// String returns the name used in logs and metrics labels.
func (r ResetReason) String() string {
	switch r {
	case ResetProtocolError:
		return "protocol_error"
	case ResetFlowControl:
		return "flow_control_error"
	case ResetUnknownFrame:
		return "unknown_frame"
	case ResetRefusedStream:
		return "refused_stream"
	case ResetPeerReset:
		return "peer_reset"
	case ResetCancel:
		return "cancel"
	case ResetInternal:
		return "internal"
	}
	return fmt.Sprintf("reset(%d)", uint16(r))
}

// GoAwayReason is the u16 of a GOAWAY frame: why a carrier takes no new
// streams, and usually closes.
type GoAwayReason uint16

const (
	GoAwayNoError        GoAwayReason = 0  // graceful: the carrier is retired and drains
	GoAwayProtocolError  GoAwayReason = 1  // the peer broke a rule of the protocol
	GoAwayFlowControl    GoAwayReason = 2  // the peer sent beyond its credit
	GoAwayFrameSize      GoAwayReason = 3  // a frame exceeded the receiver's SETTINGS_MAX_FRAME
	GoAwaySettings       GoAwayReason = 4  // SETTINGS missing, repeated or out of range
	GoAwayListenerClosed GoAwayReason = 5  // L1: the listener that accepted the carrier closed
	GoAwayPeerNotAllowed GoAwayReason = 6  // the peer's identity left the listener's ingress_peers
	GoAwayStuck          GoAwayReason = 7  // L5: OPEN frames went unanswered on a carrier that answers pings
	GoAwayCalm           GoAwayReason = 8  // the peer asked for too many answers (refusals, pings)
	GoAwayShutdown       GoAwayReason = 9  // L3: the process stops
	GoAwayCarrierAge     GoAwayReason = 10 // the carrier reached its maximum age
	GoAwayCredentials    GoAwayReason = 11 // the carrier's credentials were replaced; open a new one
	GoAwayIdleTimeout    GoAwayReason = 12 // L4: nothing was received for the idle timeout
	GoAwayInternal       GoAwayReason = 13 // the sending side failed
)

// String returns the name used in logs and metrics labels.
func (r GoAwayReason) String() string {
	switch r {
	case GoAwayNoError:
		return "no_error"
	case GoAwayProtocolError:
		return "protocol_error"
	case GoAwayFlowControl:
		return "flow_control_error"
	case GoAwayFrameSize:
		return "frame_size_error"
	case GoAwaySettings:
		return "settings_error"
	case GoAwayListenerClosed:
		return "listener_closed"
	case GoAwayPeerNotAllowed:
		return "peer_not_allowed"
	case GoAwayStuck:
		return "stuck"
	case GoAwayCalm:
		return "enhance_your_calm"
	case GoAwayShutdown:
		return "shutdown"
	case GoAwayCarrierAge:
		return "carrier_age"
	case GoAwayCredentials:
		return "credentials_changed"
	case GoAwayIdleTimeout:
		return "idle_timeout"
	case GoAwayInternal:
		return "internal"
	}
	return fmt.Sprintf("goaway(%d)", uint16(r))
}
