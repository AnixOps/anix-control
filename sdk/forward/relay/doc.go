// Package relay is the transport library of the AnixOps relay protocol
// (docs/architecture/anixops-protocol.md, owner decision H22): the frame
// format, the stream multiplexer and the rules that keep a carrier from
// outliving its owner, between two nodes of one AnixOps deployment. It is
// plain Go with the standard library only, and it does not import the
// kernel (check_package_boundaries.sh).
//
// The ALPN protocol of the prototype is "anixops/0" (ALPN): its wire format
// may change without notice and it never interoperates with the production
// versions (anixops-protocol.md section 9.1).
//
// # Layers
//
// A Carrier is one authenticated connection between two nodes for one hop of
// one route. NewCarrier runs the SETTINGS exchange over any net.Conn and
// multiplexes streams on it; how the connection was made (TLS 1.3 with mutual
// authentication and identity pinning, or a trusted plain link) is outside
// this package's core, so the multiplexer is tested on its own over loopback
// TCP and net.Pipe. A Stream is one client TCP connection (it implements
// net.Conn and CloseWrite) or one client UDP association (datagrams).
//
// Only the dialling end (the previous hop) opens streams. The listening end
// answers each one exactly once and never opens any, so a peer can never make
// a listener dial anything: OPEN carries no destination, only the route id,
// the hop index and the client's address (anixops-protocol.md section 4.3).
//
// # Frames
//
// Every frame is an 8-byte big-endian header and up to 65535 bytes of
// payload (the receiver's SETTINGS_MAX_FRAME bounds it; the length is checked
// before any payload is read):
//
//	| length (16) | type (8) | flags (8) | stream id (32) | payload |
//
//	type  name      stream   payload
//	0x0   SETTINGS  0        (u16 key, u32 value) pairs; the first frame each side sends, once
//	0x1   OPEN      new      kind u8, hop index u32, route id (u8 length + bytes), client address
//	0x2   RESULT    stream   u16 result code; sent once by the listener per OPEN
//	0x3   DATA      stream   bytes; flag FIN (0x1) ends the sender's direction
//	0x4   WINDOW    0/stream u32 credit increment (never zero) for the carrier or the stream
//	0x5   RESET     stream   u16 reason; aborts both directions
//	0x6   PING      0        8 opaque bytes; flag ACK (0x2) on the answer
//	0x7   GOAWAY    0        last accepted stream id u32, reason u16
//	0x8   DATAGRAM  stream   one UDP datagram (UDP over a stream)
//
// Every payload has one valid encoding and the parsers reject anything else
// (a short or long payload, a trailing byte, a zero WINDOW increment, an
// IPv4-mapped address sent as IPv6), which is what the fuzz targets check:
// whatever parses re-encodes to the same bytes.
//
// Stream ids are odd, opened in increasing order and never reused, so a
// carrier carries at most 2^30 streams (Open then returns ErrIDsExhausted
// and the caller moves to a new carrier). Unknown frame types on stream 0 are
// ignored, on a stream they reset it (ResetUnknownFrame); unknown SETTINGS
// keys are ignored, so optional features need no new wire version.
//
// # Settings and limits
//
// Each end announces its receive-side limits in SETTINGS and neither
// NewCarrier returns before it has the peer's, so no limit is assumed about a
// peer and the first stream already runs within the real ones. (This costs a
// round trip once per carrier, never per stream; for the TLS carrier it also
// means a dialler learns the listener accepted its certificate, since in TLS
// 1.3 the client's handshake completes before the server has checked it.)
// A second SETTINGS, or one out of range, ends the carrier (settings_error).
//
//	limit                           default  bounds      where
//	concurrent streams per carrier  1024     0..65536    SETTINGS_MAX_STREAMS, listening end
//	frame payload                   16 KiB   1 KiB..65535 SETTINGS_MAX_FRAME
//	stream window (credit)          256 KiB  4 KiB..16 MiB SETTINGS_STREAM_WINDOW (key 0x3)
//	carrier window (credit)         1 MiB    4 KiB..64 MiB SETTINGS_CARRIER_WINDOW (key 0x4)
//	streams opened, not yet taken   128      Config.AcceptQueue; excess OPENs are refused
//	send buffer per stream          64 KiB   Config.SendBuffer; Write blocks beyond it
//	ping after nothing received     10 s     Config.PingInterval
//	close after nothing received    30 s     Config.IdleTimeout (also bounds a stuck write)
//	OPEN unanswered (retire)        5 s      Config.ResultTimeout (L5)
//	SETTINGS exchange               10 s     Config.HandshakeTimeout
//	listener drain on close         5 s      Config.DrainTimeout (L1)
//
// The document's windows grow up to 16 MiB and 64 MiB as the measured
// bandwidth-delay product needs; this package takes the windows as
// configuration and leaves automatic growth to the benchmarks (A5).
//
// # Flow control and backpressure
//
// Credit based, per stream and per carrier, as in HTTP/2. A sender may have
// at most the credit its peer granted in flight and never exceeds it; a peer
// that does ends the carrier (flow_control_error). The receiver grants credit
// back with WINDOW as the application consumes bytes (Stream.Read), in
// batches of half a window, so a receiver whose application is blocked writing
// to its own socket stops granting, and the sender (whose Write blocks once its
// send buffer is full) stops reading its source: backpressure is end to end.
//
// Credit returns when the application reads, not when the bytes arrive, so
// the bytes a carrier buffers are bounded by its carrier window exactly (the
// "buffered bytes per carrier" limit of section 4.5), and a stalled stream
// holds at most its stream window of that. One stalled stream therefore never
// slows its neighbours, but as many stalled streams as fit the carrier window
// (four at the defaults) hold every stream on the carrier until they are
// read: a property of the owner-approved design, and the reason the dialler
// spreads streams over a small pool of carriers (section 4.5). Closing or
// resetting a stream returns the credit its unread bytes held.
//
// Control frames (RESULT, WINDOW, RESET, PING, GOAWAY) go before data in the
// writer, and the goroutine that reads the connection never writes to it: it
// only queues control frames for the writer, so two ends that are both
// blocked writing while the other is not reading cannot deadlock. Data is
// sent one frame per ready stream in turn.
//
// # Opening, answering, half-close and resets
//
// Carrier.Open queues OPEN and returns at once; the caller writes the client's
// first bytes right behind it (no round trip before the first byte, and a
// server-first protocol works). The listener takes the stream with
// Carrier.Accept and answers it with Stream.Accept (RESULT success) or
// Stream.Reject (a ResultCode; the dialler's Stream.AwaitResult returns a
// *ResultError and it may retry another upstream). Write, CloseWrite and
// Close answer implicitly, so no stream is left unanswered (L2). A carrier
// that cannot take a stream refuses it with RESET (ResetRefusedStream, which
// matches ErrRefused), not RESULT: a stream limit, an accept queue that is
// full, or a draining carrier. Stream.Answered tells a dialler whether a
// RESULT arrived: a stream that failed without one never started as far as
// the dialler knows.
//
// A DATA frame with FIN (Stream.CloseWrite) ends one direction; the peer reads
// io.EOF after the data and may keep sending, and the stream ends when both
// directions have. Stream.Reset sends RESET and aborts both directions;
// Stream.Close on a stream the peer has not finished resets it (ResetCancel).
//
// A UDP stream carries datagrams one per DATAGRAM frame with boundaries
// preserved, flow controlled like data. WriteDatagram never blocks: when the
// stream's window or send buffer cannot take the datagram it is dropped and
// counted (ErrDatagramDropped), as a full socket buffer would drop it. Empty
// datagrams are not carried.
//
// # Liveness (anixops-protocol.md section 4.6)
//
//   - L1: GoAway(reason, drain) stops new streams (an acceptor refuses OPENs
//     already in flight with ResetRefusedStream, without blame) and ends the
//     carrier when its streams have ended or the drain deadline passes.
//   - L2: see above; an OPEN is always answered.
//   - L3: Close sends GOAWAY (GoAwayShutdown) where it can and closes the
//     connection; a crash is the kernel's FIN or RST.
//   - L4: PING after PingInterval without a received frame, close after
//     IdleTimeout (ErrIdleTimeout); the PING round trip is Carrier.RTT.
//   - L5: a dialler whose OPEN has no RESULT for ResultTimeout retires the
//     carrier (GOAWAY stuck, whether or not it still answers PING) and fails
//     the unanswered streams with ErrOpenTimeout so they can be retried.
//
// A GOAWAY from the peer fails the streams above its last accepted id with
// ErrRefused; the others continue. A stream's slot is freed when both its
// directions have finished, which the listener sees about half a round trip
// before the dialler does, so a dialler at the stream limit can still be
// refused (ResetRefusedStream) and retries on another carrier, as with HTTP/2. Carrier.Draining reports the first GOAWAY
// sent or received, and Carrier.Done the end of the carrier.
//
// # Peer misbehaviour
//
// Rules the peer breaks end the carrier with a GOAWAY of the reason
// (ProtocolError): frames on the wrong stream id or of the wrong type for the
// end, ids that do not increase, credit exceeded or overflowed (past 2^31-1),
// a frame above the announced maximum, a repeated SETTINGS. Mistakes confined
// to a stream (unknown type, malformed OPEN, DATA after FIN, DATA before
// RESULT, a datagram on a TCP stream) reset that stream and the carrier lives.
// Frames of a stream that already finished here are ignored (they were in
// flight), and the carrier credit they used is returned. The answers a peer
// can draw by misbehaving (pings, refused or malformed OPENs, stream
// errors) are rationed (a burst of 64, then 64 per second): beyond that the
// carrier ends with GoAwayCalm. The control queue is bounded too.
//
// # Testing
//
// The tests run carriers over loopback TCP (real buffering) and net.Pipe
// (none), drive a carrier with a raw frame peer for every violation above, and
// check a randomized model of streams with random sizes, windows, endings and
// resets for data integrity, exact credit accounting and liveness. The fuzz
// targets (FuzzReadFrame, FuzzParseSettings, FuzzOpenParams, FuzzSmallPayloads,
// FuzzCarrierAcceptor, FuzzCarrierDialer, FuzzTransfers) run with their seed
// corpus under go test; run one longer from the sdk module with
//
//	go test -run '^$' -fuzz '^FuzzCarrierAcceptor$' -fuzztime 60s ./forward/relay
//
// RELAY_MODEL_SEEDS=500 go test -race -run RandomizedTransfers ./forward/relay
// soaks the model.
package relay
