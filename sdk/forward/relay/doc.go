// Package relay is the transport library of the AnixOps relay protocol
// (docs/architecture/anixops-protocol.md, owner decision H22): the frame
// format, the stream multiplexer, the QUIC carrier with native UDP, carrier
// selection and the rules that keep a carrier from outliving its owner, between
// two nodes of one AnixOps deployment. It is plain Go: the standard library's
// cryptography and, for QUIC, quic-go (MIT, one pinned version, decision P2) and
// the golang.org/x modules it uses. It does not import the kernel
// (check_package_boundaries.sh).
//
// The ALPN protocol of the prototype is "anixops/0" (ALPN): its wire format
// may change without notice and it never interoperates with the production
// versions (anixops-protocol.md section 9.1).
//
// # Layers
//
// A Carrier is one authenticated connection between two nodes for one hop of
// one route, and a Stream is one client TCP connection (it implements net.Conn
// and CloseWrite) or one client UDP association (datagrams): both are
// interfaces, which is what a driver uses. Three carriers implement them, of
// the three link types of the document (section 5):
//
//   - ConnCarrier runs the frame protocol below over any net.Conn: the TLS over
//     TCP carrier (TLS_TCP) and the plaintext carrier of a trusted link (PLAIN).
//     NewConnCarrier runs the SETTINGS exchange over any net.Conn, so the
//     multiplexer is tested on its own over loopback TCP and net.Pipe.
//   - QUICCarrier maps streams to native QUIC streams and UDP to QUIC DATAGRAM
//     frames (QUIC); see "The QUIC carrier" below.
//
// How the connection is made is package link's business: TLS 1.3 with mutual
// authentication and per-identity pinning (CarrierTLS and CarrierQUIC), or a
// trusted plain link with source admission only (CarrierPlain). DialTLS,
// DialQUIC, DialPlain, Listen, ListenQUIC, ListenAuto and ListenPlain join the
// two: they make a verified link connection and run a carrier on it. A
// listener (Listener for TCP, QUICListener, AutoListener for both on one
// port, all CarrierListener) owns the carriers it accepted (L1): closing it
// sends GOAWAY on all of them and closes them when their streams end or after
// the drain; SetPeers closes the carriers of an identity removed from
// ingress_peers with GOAWAY peer_not_allowed; a trust bundle change closes the
// carriers whose peer lost its CA (WatchCredentials does the same for a
// dialler's carriers). A SETTINGS exchange that never finishes is bounded by
// the handshake timeout and by Config.MaxPending per listener, and a dial is
// bounded by its context in the exchange too. A Selector makes the carriers of
// one link with its carrier choice (AUTO, TLS_TCP, QUIC, PLAIN) and does AUTO's
// QUIC probe and fallback.
//
// Only the dialling end (the previous hop) opens streams. The listening end
// answers each one exactly once and never opens any, so a peer can never make
// a listener dial anything: OPEN carries no destination, only the route id,
// the hop index and the client's address (anixops-protocol.md section 4.3).
//
// # Frames
//
// The frame protocol is the one of ConnCarrier; a QUICCarrier uses its encoding
// for what QUIC does not carry (see "The QUIC carrier"). Every frame is an
// 8-byte big-endian header and up to 65535 bytes of
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
// # The QUIC carrier
//
// QUICCarrier (anixops-protocol.md section 5.2) runs on a QUIC version 1
// connection (quic-go, one pinned version; the TLS 1.3 inside it is crypto/tls's
// own, through the same link-layer checks as TLS_TCP: package link). QUIC already
// provides what DATA, WINDOW, RESET, PING and the close of the TCP frames do, so
// the mapping is:
//
//	control streams  each end opens one unidirectional stream first. It carries
//	                 SETTINGS (once, first: both ends wait for the other's, so a
//	                 dialler knows the listener accepted its certificate before
//	                 it opens a stream), then GOAWAY, then nothing but a FIN that
//	                 says "I have no streams left". QUIC allows the peer one such
//	                 stream, so a second one is a transport error.
//	stream           a native bidirectional stream, opened only by the dialler
//	                 (the dialler announces MaxIncomingStreams -1). Its first
//	                 frame is OPEN (dialler) and then RESULT (listener); after
//	                 that a TCP stream is raw bytes in both directions and a UDP
//	                 stream is DATAGRAM frames. Frames on QUIC streams have
//	                 stream id 0: the QUIC stream is the stream.
//	half-close       QUIC's FIN (CloseWrite); Close of a stream the peer has not
//	                 finished (the reader saw no EOF) cancels both directions,
//	                 and so does a Close while a Write is in progress, where
//	                 the TCP carrier would queue the FIN behind the data
//	reset            QUIC's RESET_STREAM and STOP_SENDING, whose error code is
//	                 the ResetReason (refused_stream for a carrier-level
//	                 refusal: stream limit, full accept queue, draining)
//	liveness (L4)    QUIC's keepalive (Config.PingInterval) and idle timeout
//	                 (Config.IdleTimeout); the carrier's RTT is QUIC's smoothed RTT
//	close (L3)       CONNECTION_CLOSE whose application error code is the
//	                 GoAwayReason; the peer reads it as PeerGoAway. A listener
//	                 restarted with the same StatelessResetKey answers the
//	                 packets of its old connections with a stateless reset, and
//	                 the peers' carriers end at once
//	ids              a stream's id is its QUIC stream id (0, 4, 8, ...), at most
//	                 2^29 streams per carrier (then ErrIDsExhausted); GOAWAY's id
//	                 is the first id the listening end did not accept, as in
//	                 HTTP/3, so the dialler fails those streams at once
//
// QUICConfig maps the limits of the TCP carrier onto QUIC's transport
// parameters (see its documentation); the windows start at the configured
// StreamWindow and CarrierWindow and grow with the measured bandwidth-delay
// product, which is quic-go's own auto-tuning, up to 16 MiB and 64 MiB by
// default (QUICConfig.StreamWindowCeiling and CarrierWindowCeiling; the carrier
// window ceiling bounds what a carrier buffers). As on the TCP carrier, credit
// returns when the application reads, so stalled streams can pin the carrier's
// window.
//
// Closing is the one place where QUIC differs in a way that matters. QUIC
// discards what a peer has received but not read when the connection closes, so
// a drained carrier is closed only when both ends are quiet: each end finishes
// its control stream after sending GOAWAY once it has no streams left, and the
// connection closes (application error code 0) when both control streams have
// finished. A peer that never does is waited for DrainTimeout. A GOAWAY at a
// carrier with no streams closes it at once; Close and CloseWithReason close it
// at once and tell the peer why, dropping what is in flight.
//
// The handshake is bounded as for TLS (package link): a listener checks the
// source before it spends anything on a connection, holds at most
// ListenerConfig.MaxPending handshakes, gives each a hard 10 s deadline, and
// asks addresses to prove themselves with QUIC's Retry when more than half of
// its slots are taken, so spoofed Initials cannot take more than half of them.
// QUIC version 1 only and no 0-RTT, on either end.
//
// # UDP on a QUIC carrier
//
// A UDP association is a UDP stream whose id names it (anixops-protocol.md
// section 4.8). Its datagrams ride QUIC DATAGRAM frames (RFC 9221) holding the
// stream id and the payload (AppendQUICDatagram, ParseQUICDatagram): unreliable
// and unordered, as UDP is. A datagram goes on the association's stream instead,
// in a DATAGRAM frame of the frame protocol (reliable and ordered, boundaries
// kept), when it is larger than the connection's current maximum DATAGRAM size
// (counted: Stats.DatagramsOversize, the document's udp_oversize_fallback),
// when either end did not enable DATAGRAM frames (QUICConfig.DisableDatagrams),
// and, from a dialler, until the listener's RESULT has arrived: a QUIC datagram
// could overtake the OPEN of its stream and be dropped by a listener that does
// not know it yet, so the first datagrams follow the OPEN on the stream.
// Stats.DatagramsOnStream counts every datagram that went on a stream, on TCP
// carriers all of them.
//
// WriteDatagram never blocks. QUIC's own queue blocks when full, so datagrams go
// through a bounded queue to a sender goroutine of the carrier and are dropped
// and counted (ErrDatagramDropped, Stats.DatagramsDropped) when it is full, as a
// full socket buffer drops them; a stream's own queue (Config.SendBuffer) does
// the same for datagrams on the stream. A datagram that arrives for no open UDP
// association (usually one that finished while it was in flight), a malformed
// one, and one that finds the association's receive queue (the stream window)
// full is dropped and counted (Stats.DatagramsRecvDropped); malformed ones draw
// on the peer's budget of answers. CloseWrite follows every datagram written
// before it, whichever way they travel.
//
// What is not here: the association's idle timeout (60 s after its last
// datagram, anixops-protocol.md section 4.8) belongs to the driver, which owns
// the table of associations and closes the stream; so do the carrier pool per
// upstream, the 7-day carrier age and PROXY v2 (phase A3).
//
// # Selection, AUTO and the listeners
//
// A Selector makes the carriers of one link (anixops-protocol.md section 5.4).
// With CarrierChoice Auto it dials QUIC first, bounded by a probe of 3 s; a QUIC
// dial that fails or times out is made up for at once with a TLS_TCP dial, and
// after three failures in a row it stops trying QUIC and dials TLS_TCP directly,
// trying QUIC again once every 5 minutes (one dial probes, the others keep
// using TLS_TCP). Nothing ever falls back to plaintext and Auto never selects
// PLAIN. A caller that gives up is no evidence about QUIC. SelectorStats count
// the QUIC attempts and failures, the fallbacks, and the state (the document's
// metric of QUIC to TLS fallbacks). The listener of an Auto link is ListenAuto:
// TLS over TCP and QUIC over UDP at the same port number, one Accept.
//
// # Socket buffers
//
// QUIC runs in user space on a UDP socket, and quic-go asks the kernel for 7 MiB
// (7340032 bytes) of receive buffer and of send buffer on every socket (a
// listener has one, each dialled carrier its own). The kernel grants no more than
// net.core.rmem_max and net.core.wmem_max unless the process has CAP_NET_ADMIN,
// which the relay unit does not (it has CAP_NET_BIND_SERVICE only,
// anixops-protocol.md section 6.2). With the usual limit of 208 KiB a loaded QUIC
// carrier drops packets in the socket and slows down, and quic-go logs once
// that it could not raise the buffer. A node that runs QUIC carriers therefore
// needs
//
//	net.core.rmem_max >= 7500000
//	net.core.wmem_max >= 7500000
//
// which the installer's forward-node sysctl drop-in sets (section 5.2); this
// library never changes a sysctl, and the driver's capability probe reports the
// limits (section 6.1). The tests of this package run with the default limits
// and print quic-go's warning.
//
// # Testing
//
// The tests run carriers over loopback TCP (real buffering), net.Pipe (none) and
// loopback QUIC, drive a carrier with a raw frame peer for every violation above
// (for QUIC the peer is a verified QUIC connection written by hand: control
// stream, stream and datagram violations, from either end), and check a
// randomized model of streams with random sizes, windows, endings and resets for
// data integrity, exact credit accounting and liveness (TCP) and for integrity,
// no hang and no leaked stream slot (QUIC). Time-dependent behaviour is tested
// with short configured timers and polling with generous deadlines, never with
// fixed sleeps that must be long enough; the selection logic runs on an
// injectable clock and injected dials. The fuzz targets (FuzzReadFrame,
// FuzzParseSettings, FuzzOpenParams, FuzzSmallPayloads, FuzzCarrierAcceptor,
// FuzzCarrierDialer, FuzzTransfers, and for QUIC FuzzQUICDatagram,
// FuzzQUICStreamFrames, FuzzQUICControlFrames, FuzzQUICCarrierAcceptor,
// FuzzQUICCarrierDialer) run with their seed corpus under go test; run one longer
// from the sdk module with
//
//	go test -run '^$' -fuzz '^FuzzCarrierAcceptor$' -fuzztime 60s ./forward/relay
//
// RELAY_MODEL_SEEDS=500 go test -race -run RandomizedTransfers ./forward/relay
// soaks the models.
package relay
