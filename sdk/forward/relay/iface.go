package relay

import (
	"context"
	"net"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// Carrier is one authenticated connection between two nodes for one hop of
// one route, multiplexing streams (anixops-protocol.md sections 4.1 and 5). It
// is what the driver uses: the carriers of the three link types differ only
// in how they are made and in what they carry below the streams.
//
//   - ConnCarrier runs the frame protocol of section 4.2 over a net.Conn: the
//     TLS over TCP carrier (TLS_TCP) and the plaintext carrier (PLAIN).
//   - QUICCarrier maps streams to native QUIC streams and UDP to QUIC
//     datagrams (QUIC).
//
// A Carrier is safe for concurrent use. It owns its connection: closing the
// carrier closes it, and the carrier ends when the connection fails.
type Carrier interface {
	// Role is which end this is; only a RoleDialer end can Open.
	Role() Role
	// Type is how the carrier's connection was made; Peer is the node at the
	// other end as the link layer verified it (the zero value on a plaintext
	// carrier).
	Type() CarrierType
	Peer() link.Peer

	// LocalAddr and RemoteAddr are the connection's addresses.
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	// LocalSettings are the limits this end announced, PeerSettings the
	// ones the peer announced.
	LocalSettings() Settings
	PeerSettings() Settings

	// Open starts a stream and returns at once, without waiting for the
	// listener's answer, so the caller can send the client's first bytes right
	// behind it (stream-level zero round trip, section 4.3). A carrier that
	// cannot take the stream says so with ErrGoAway, ErrStreamLimit or
	// ErrIDsExhausted, and the caller opens it on another carrier.
	Open(p OpenParams) (Stream, error)
	// Accept returns the next stream the peer opened (listening end only).
	// The caller must answer it with Stream.Accept or Stream.Reject; Write,
	// CloseWrite and Close answer it implicitly (L2).
	Accept(ctx context.Context) (Stream, error)

	// GoAway tells the peer this carrier takes no new streams; the carrier
	// ends when its streams have, or after drain when drain is positive.
	GoAway(reason GoAwayReason, drain time.Duration) error
	// Shutdown retires the carrier and waits for its streams, for at most
	// Config.DrainTimeout or until ctx is done, then closes it.
	Shutdown(ctx context.Context) error
	// Close ends the carrier at once (L3): every stream fails and the peer is
	// told with GoawayShutdown where the carrier can. CloseWithReason names
	// another reason. Both return when the carrier has released its goroutines.
	Close() error
	CloseWithReason(reason GoAwayReason) error

	// Done is closed when the carrier has ended and released everything;
	// Draining when a GOAWAY was sent or received; Err says why the carrier
	// ended (wrapping ErrCarrierClosed), nil while it is up.
	Done() <-chan struct{}
	Draining() <-chan struct{}
	Err() error
	// PeerGoAway returns the reason of the GOAWAY the peer sent, if it did.
	PeerGoAway() (GoAwayReason, bool)

	// RTT is the latest round trip measurement, zero before the first. Stats
	// is a snapshot of the carrier's counters; ActiveStreams counts the streams
	// that have not finished.
	RTT() time.Duration
	Stats() Stats
	ActiveStreams() int
}

// Stream is one client connection (TCP) or one client UDP association inside a
// Carrier. A TCP stream is a net.Conn plus CloseWrite, so a proxy copies bytes
// into and out of it as it would a TCP connection, half-close included; a UDP
// stream carries datagrams with WriteDatagram and ReadDatagram. A stream may be
// used by one reader and one writer goroutine at a time, and any goroutine may
// call Close, Reset or set a deadline, as with a net.Conn.
type Stream interface {
	// Read, Write, Close, the deadlines and the addresses are net.Conn's. Read
	// and Write are for TCP streams; on a UDP stream they return ErrStreamKind.
	net.Conn
	// CloseWrite ends this side's direction: the peer reads the queued data
	// and then io.EOF, and can keep sending (section 4.7).
	CloseWrite() error

	// ID is the stream's id, unique within the carrier. On a QUIC carrier it
	// is the QUIC stream id, which is also what names a UDP association in the
	// DATAGRAM frames.
	ID() uint64
	// Kind says whether the stream carries a TCP connection or UDP datagrams,
	// and Params are the OPEN parameters.
	Kind() StreamKind
	Params() OpenParams
	// Carrier returns the carrier the stream runs on.
	Carrier() Carrier

	// ReadDatagram returns the next datagram of a UDP stream, which the caller
	// owns; io.EOF after the peer's FIN once the queue is empty. WriteDatagram
	// sends one and never blocks: when the datagram cannot be taken it is
	// dropped and counted (ErrDatagramDropped), as a full socket buffer would
	// drop it (section 4.8).
	ReadDatagram() ([]byte, error)
	WriteDatagram(p []byte) error

	// Reset aborts both directions at once with the given reason.
	Reset(reason ResetReason) error

	// Accept answers a listening-end stream with success, Reject with a failure
	// that ends it. AwaitResult waits for the answer on a dialling-end stream:
	// nil for success, a *ResultError for a refusal, the stream's error if it
	// was reset or refused by the carrier (errors.Is ErrRefused) or the
	// carrier ended first. Answered reports whether the answer arrived: a
	// stream that failed without one never started as far as the dialler
	// knows, and may be retried on another carrier with the bytes it kept.
	Accept() error
	Reject(code ResultCode) error
	AwaitResult(ctx context.Context) error
	Answered() bool
}

var (
	_ Carrier = (*ConnCarrier)(nil)
	_ Stream  = (*ConnStream)(nil)
)
