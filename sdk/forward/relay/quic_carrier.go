package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// QUICCarrier is a Carrier on a QUIC connection (anixops-protocol.md section
// 5.2). Streams are native QUIC bidirectional streams: QUIC's own stream data,
// flow control, resets and close replace DATA, WINDOW and RESET, and its
// keepalive and idle timeout are the carrier's liveness (L4). What QUIC lacks
// travels in the frames of section 4.2 as quicwire.go describes: OPEN and
// RESULT at the start of each stream, SETTINGS and GOAWAY on a control stream
// that each end opens first, and UDP either in QUIC DATAGRAM frames or, where
// they cannot go, in DATAGRAM frames on the association's stream.
//
// The behaviour a driver relies on is the TCP carrier's, the same Carrier and
// Stream interfaces: Open never blocks and the client's first bytes follow OPEN
// at once; the listening end answers every stream exactly once (L2) and never
// opens any; GOAWAY drains (L1); a peer that breaks a rule ends the carrier,
// with the rule as the QUIC application error code, and one that merely
// misuses a stream loses that stream; the answers a peer can draw by
// misbehaving are rationed.
//
// One difference is closing. QUIC discards the data a peer has received but not
// read when the connection closes, so a drained carrier is closed only when
// both ends are quiet: each end finishes its control stream once it has sent
// GOAWAY and has no streams left, and the connection is closed when both
// control streams have finished. A peer that never does is waited for at most
// DrainTimeout.
type QUICCarrier struct {
	conn      *link.QUICConn
	qc        *quic.Conn
	role      Role
	cfg       QUICConfig
	local     Settings
	peer      Settings
	datagrams bool // both ends enabled DATAGRAM frames and UDP may use them
	start     time.Time

	ctrlQ chan ctrlItem // frames for the control stream writer

	// mu guards everything below it and the mutable state of every stream.
	mu      sync.Mutex
	streams map[quic.StreamID]*QUICStream
	active  int
	opened  uint64 // dialler: streams opened, which fixes the next id

	accepted     bool          // acceptor: a stream was accepted
	lastAccepted quic.StreamID // acceptor: the highest accepted stream id

	goAwaySent       bool
	goAwayRecv       bool
	goAwayRecvReason GoAwayReason
	ctrlFinSent      bool // this end is quiet: GOAWAY sent, no streams left
	peerCtrlFin      bool // the peer is quiet
	drainTimer       *time.Timer
	quietTimer       *time.Timer
	err              error
	calm             calmBucket

	acceptCh chan *QUICStream
	dead     chan struct{} // closed when err is set
	draining chan struct{} // closed with the first GOAWAY, sent or received
	done     chan struct{} // closed when the connection is closed and every goroutine has ended
	closeReq chan closeRequest
	wg       sync.WaitGroup

	dgq     chan outDatagram // datagrams waiting for the sender
	dgBytes atomic.Int64

	n counters
}

// ctrlItem is what the control stream writer sends: a frame, and after it,
// when fin is set, the FIN of the stream.
type ctrlItem struct {
	frame []byte
	fin   bool
}

type closeRequest struct {
	reason GoAwayReason
}

var _ Carrier = (*QUICCarrier)(nil)

const (
	// dgQueueLen and dgQueueBytes bound the datagrams a carrier holds for
	// QUIC's sender: more are dropped and counted, as a full socket buffer
	// drops them.
	dgQueueLen   = 256
	dgQueueBytes = 256 * 1024
)

// NewQUICCarrier runs the SETTINGS exchange over conn and returns the carrier.
// Each end opens its control stream and sends SETTINGS first, and neither returns
// before it has the peer's, so the limits of both ends are known before the
// first stream opens and a dialler that gets a carrier knows the listener
// accepted its certificate (the QUIC handshake completes at the dialler before
// the listener has verified it, as TLS 1.3's does). conn must have been made
// with QUICConfig.Transport for role, which DialQUIC and the QUIC listener do.
// NewQUICCarrier takes ownership of conn and closes it on failure.
func NewQUICCarrier(conn *link.QUICConn, role Role, cfg QUICConfig) (*QUICCarrier, error) {
	return NewQUICCarrierContext(context.Background(), conn, role, cfg)
}

// NewQUICCarrierContext is NewQUICCarrier that gives up when ctx ends before the
// SETTINGS exchange has finished: the connection is closed and the error is
// ctx's. DialQUIC uses it, so that a dial bounded by a deadline (AUTO's probe)
// is bounded in the exchange after the handshake too.
func NewQUICCarrierContext(ctx context.Context, conn *link.QUICConn, role Role, cfg QUICConfig) (*QUICCarrier, error) {
	cfg, err := cfg.withDefaults(role)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	local := cfg.settings(role)
	// A caller that stops waiting closes the connection, which ends whatever
	// the exchange is blocked on.
	stopWatch := context.AfterFunc(ctx, func() { _ = conn.CloseWithError(quic.ApplicationErrorCode(GoAwayInternal), "dial ended") })
	defer stopWatch()
	fail := func(reason GoAwayReason, err error) (*QUICCarrier, error) {
		_ = conn.CloseWithError(quic.ApplicationErrorCode(reason), reason.String())
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", ctx.Err(), err)
		}
		return nil, err
	}
	out, err := conn.OpenUniStream()
	if err != nil {
		return fail(GoAwayInternal, fmt.Errorf("relay: opening the control stream: %w", err))
	}
	if _, err := out.Write(quicFrameBytes(TypeSettings, local.Marshal())); err != nil {
		return fail(GoAwayInternal, fmt.Errorf("relay: sending SETTINGS: %w", err))
	}
	wctx, cancel := context.WithTimeout(ctx, cfg.HandshakeTimeout)
	defer cancel()
	in, err := conn.AcceptUniStream(wctx)
	if err != nil {
		return fail(GoAwayInternal, fmt.Errorf("relay: waiting for the peer's control stream: %w", err))
	}
	_ = in.SetReadDeadline(time.Now().Add(cfg.HandshakeTimeout))
	peer, err := readQUICSettings(in, int(local.MaxFrame))
	if err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return fail(pe.Reason, err)
		}
		return fail(GoAwayInternal, fmt.Errorf("relay: reading the peer's SETTINGS: %w", err))
	}
	_ = in.SetReadDeadline(time.Time{})
	if !stopWatch() { // the caller gave up just now: the connection is closing
		return fail(GoAwayInternal, errors.New("relay: the dial ended during the SETTINGS exchange"))
	}

	now := time.Now()
	ss := conn.ConnectionState().SupportsDatagrams
	c := &QUICCarrier{
		conn:      conn,
		qc:        conn.Conn,
		role:      role,
		cfg:       cfg,
		local:     local,
		peer:      peer,
		datagrams: ss.Local && ss.Remote,
		start:     now,
		ctrlQ:     make(chan ctrlItem, 4),
		streams:   make(map[quic.StreamID]*QUICStream),
		calm:      newCalmBucket(now),
		acceptCh:  make(chan *QUICStream, cfg.AcceptQueue),
		dead:      make(chan struct{}),
		draining:  make(chan struct{}),
		done:      make(chan struct{}),
		closeReq:  make(chan closeRequest, 1),
		dgq:       make(chan outDatagram, dgQueueLen),
	}
	c.wg.Add(2)
	go c.ctrlWriter(out)
	go c.controlLoop(in)
	if c.datagrams {
		c.wg.Add(2)
		go c.datagramRecvLoop()
		go c.datagramSendLoop()
	}
	if role == RoleAcceptor {
		c.wg.Add(1)
		go c.acceptLoop()
	} else if cfg.ResultTimeout > 0 {
		c.wg.Add(1)
		go c.keepaliveLoop()
	}
	go c.run()
	return c, nil
}

// readQUICSettings reads the peer's SETTINGS, the first frame of its control
// stream.
func readQUICSettings(r io.Reader, maxFrame int) (Settings, error) {
	f, err := readQUICFrame(r, maxFrame)
	switch {
	case err == nil:
	case errors.Is(err, ErrQUICFrame), errors.Is(err, ErrFrameTooLarge), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return Settings{}, &ProtocolError{Reason: GoAwaySettings, Detail: "the first frame of the control stream is not a valid SETTINGS: " + err.Error()}
	default:
		return Settings{}, err
	}
	if f.Type != TypeSettings {
		return Settings{}, &ProtocolError{Reason: GoAwaySettings, Detail: "the first frame of the control stream is not SETTINGS"}
	}
	s, err := ParseSettings(f.Payload)
	if err != nil {
		return Settings{}, &ProtocolError{Reason: GoAwaySettings, Detail: err.Error()}
	}
	return s, nil
}

// run waits for the connection to end, by this end's request or by itself,
// and then ends the carrier.
func (c *QUICCarrier) run() {
	select {
	case req := <-c.closeReq:
		_ = c.qc.CloseWithError(quic.ApplicationErrorCode(req.reason), req.reason.String())
	case <-c.qc.Context().Done():
	}
	<-c.qc.Context().Done()
	c.mu.Lock()
	c.endLocked(context.Cause(c.qc.Context()))
	c.mu.Unlock()
	c.wg.Wait()
	c.mu.Lock()
	for _, t := range []*time.Timer{c.drainTimer, c.quietTimer} {
		if t != nil {
			t.Stop()
		}
	}
	c.mu.Unlock()
	close(c.done)
}

// endLocked records the end of the connection when it ended by itself (the
// peer closed it, it idled out, the peer restarted, a transport error): the
// carrier's error says why, and an application error code the peer closed with
// is its GOAWAY reason.
func (c *QUICCarrier) endLocked(cause error) {
	if c.err != nil {
		return
	}
	var app *quic.ApplicationError
	var idle *quic.IdleTimeoutError
	switch {
	case errors.As(cause, &app) && app.Remote:
		reason := GoAwayInternal
		if app.ErrorCode <= 0xffff {
			reason = GoAwayReason(app.ErrorCode)
		}
		if !c.goAwayRecv {
			c.goAwayRecv, c.goAwayRecvReason = true, reason
		}
		cause = fmt.Errorf("the peer closed the carrier (%s)", reason)
	case errors.As(cause, &idle):
		c.n.keepaliveTimeouts.Add(1)
		cause = ErrIdleTimeout
	}
	c.failLocked(cause, 0, false)
}

// connErrLocked returns the carrier's error for a failure a stream operation
// reported about the connection, recording the end of the connection first if
// the watcher has not yet.
func (c *QUICCarrier) connErrLocked(err error) error {
	if c.err == nil {
		switch {
		case isConnectionError(err):
			c.endLocked(err)
		case c.qc.Context().Err() != nil:
			c.endLocked(context.Cause(c.qc.Context()))
		default:
			// Not an error of the connection and not a stream's: nothing
			// this carrier can continue from.
			c.failLocked(fmt.Errorf("unexpected error: %w", err), GoAwayInternal, true)
		}
	}
	return c.err
}

// isConnectionError reports whether err is one of the errors with which QUIC
// ends a connection, as opposed to one that ends a stream.
func isConnectionError(err error) bool {
	var (
		app *quic.ApplicationError
		tr  *quic.TransportError
		idl *quic.IdleTimeoutError
		hs  *quic.HandshakeTimeoutError
		rst *quic.StatelessResetError
		ver *quic.VersionNegotiationError
	)
	return errors.As(err, &app) || errors.As(err, &tr) || errors.As(err, &idl) || errors.As(err, &hs) || errors.As(err, &rst) || errors.As(err, &ver)
}

// spawnLocked runs f in a goroutine the carrier waits for, unless the
// carrier has ended. The caller holds c.mu.
func (c *QUICCarrier) spawnLocked(f func()) bool {
	if c.err != nil {
		return false
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		f()
	}()
	return true
}

// Role returns which end of the carrier this is.
func (c *QUICCarrier) Role() Role { return c.role }

// Type is CarrierQUIC.
func (c *QUICCarrier) Type() CarrierType { return CarrierQUIC }

// Peer returns the node at the other end as the link layer verified it.
func (c *QUICCarrier) Peer() link.Peer { return c.conn.Peer() }

// LocalAddr and RemoteAddr are the connection's addresses.
func (c *QUICCarrier) LocalAddr() net.Addr  { return c.qc.LocalAddr() }
func (c *QUICCarrier) RemoteAddr() net.Addr { return c.qc.RemoteAddr() }

// LocalSettings returns the limits this end announced, PeerSettings the
// peer's. On QUIC the limits are QUIC's transport parameters; SETTINGS only
// announces them (MaxFrame, the largest UDP datagram on a stream, is the one
// that is not QUIC's).
func (c *QUICCarrier) LocalSettings() Settings { return c.local }
func (c *QUICCarrier) PeerSettings() Settings  { return c.peer }

// Conn returns the underlying QUIC connection.
func (c *QUICCarrier) Conn() *quic.Conn { return c.qc }

// NativeDatagrams reports whether UDP associations ride QUIC DATAGRAM frames
// on this carrier: both ends enabled them. Otherwise they ride their streams.
func (c *QUICCarrier) NativeDatagrams() bool { return c.datagrams }

// Done is closed when the carrier has ended and released its connection and
// goroutines.
func (c *QUICCarrier) Done() <-chan struct{} { return c.done }

// Draining is closed when a GOAWAY was sent or received: the carrier takes no
// new streams.
func (c *QUICCarrier) Draining() <-chan struct{} { return c.draining }

// Err returns why the carrier ended, wrapping ErrCarrierClosed, or nil while
// it is up.
func (c *QUICCarrier) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// PeerGoAway returns the reason of the GOAWAY the peer sent, or the
// application error code it closed the connection with.
func (c *QUICCarrier) PeerGoAway() (GoAwayReason, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.goAwayRecvReason, c.goAwayRecv
}

// RTT returns QUIC's smoothed round trip time, zero before the first sample.
func (c *QUICCarrier) RTT() time.Duration { return c.qc.ConnectionStats().SmoothedRTT }

// Stats returns a snapshot of the carrier's counters.
func (c *QUICCarrier) Stats() Stats {
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	s := c.n.snapshot(active)
	s.RTT = c.RTT()
	return s
}

// ActiveStreams returns the number of streams that have not finished.
func (c *QUICCarrier) ActiveStreams() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Open starts a stream: it opens a QUIC stream, writes OPEN and returns at
// once, without waiting for the listener's answer, so the caller can send the
// client's first bytes right behind it. It never blocks: a carrier that cannot
// take the stream says so with ErrGoAway, ErrStreamLimit or ErrIDsExhausted,
// and the caller opens it on another carrier.
func (c *QUICCarrier) Open(p OpenParams) (Stream, error) {
	payload, err := p.MarshalBinary()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	switch {
	case c.role != RoleDialer:
		c.mu.Unlock()
		return nil, ErrWrongRole
	case c.err != nil:
		err := c.err
		c.mu.Unlock()
		return nil, err
	case c.goAwaySent || c.goAwayRecv:
		c.mu.Unlock()
		return nil, ErrGoAway
	case 4*(c.opened+1) > MaxStreamID: // ids are 0, 4, 8...; GOAWAY carries them in 32 bits
		c.mu.Unlock()
		return nil, ErrIDsExhausted
	case c.active >= int(c.peer.MaxStreams):
		c.mu.Unlock()
		return nil, ErrStreamLimit
	}
	st, err := c.qc.OpenStream()
	if err != nil {
		var limit *quic.StreamLimitReachedError
		if errors.As(err, &limit) {
			c.mu.Unlock()
			return nil, ErrStreamLimit
		}
		err = c.connErrLocked(err)
		c.mu.Unlock()
		return nil, err
	}
	s := c.newStreamLocked(st, p)
	s.awaiting, s.openedAt = true, time.Now()
	c.opened++
	c.n.streamsOpened.Add(1)
	c.spawnLocked(func() { c.readResult(s) })
	c.mu.Unlock()

	// The first bytes of the stream: nothing waits for a round trip.
	s.wmu.Lock()
	_, err = st.Write(quicFrameBytes(TypeOpen, payload))
	s.wmu.Unlock()
	if err != nil {
		c.mu.Lock()
		err = s.failedLocked(err)
		c.mu.Unlock()
		return nil, err
	}
	c.n.framesSent.Add(1)
	return s, nil
}

// Accept returns the next stream the peer opened. The caller must answer it
// with Stream.Accept or Stream.Reject; Write, CloseWrite and Close answer it
// implicitly, so no stream is ever left unanswered (L2).
func (c *QUICCarrier) Accept(ctx context.Context) (Stream, error) {
	if c.role != RoleAcceptor {
		return nil, ErrWrongRole
	}
	select {
	case <-c.dead: // an ended carrier hands out nothing, even if streams are queued
		return nil, c.Err()
	default:
	}
	select {
	case s := <-c.acceptCh:
		return s, nil
	case <-c.dead:
		return nil, c.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GoAway tells the peer this carrier takes no new streams (reason, usually
// GoAwayNoError for a graceful retirement, GoAwayListenerClosed or
// GoAwayShutdown). The carrier ends when both ends have no streams left, or
// after drain when drain is positive, whichever comes first. A retired dialler
// carrier keeps serving its open streams; an acceptor refuses OPENs from then
// on.
func (c *QUICCarrier) GoAway(reason GoAwayReason, drain time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.goAwayLocked(reason, drain)
	return nil
}

// Shutdown retires the carrier with GoAwayShutdown and waits for its
// streams, for at most Config.DrainTimeout or until ctx is done, then
// closes it.
func (c *QUICCarrier) Shutdown(ctx context.Context) error {
	if err := c.GoAway(GoAwayShutdown, c.cfg.DrainTimeout); err != nil {
		<-c.done
		return nil
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		_ = c.Close()
		return ctx.Err()
	}
	return nil
}

// Close ends the carrier at once: every stream fails and the connection is
// closed with GoAwayShutdown as the application error code. It returns when the
// carrier has released its goroutines.
func (c *QUICCarrier) Close() error { return c.CloseWithReason(GoAwayShutdown) }

// CloseWithReason is Close with the reason the peer is told: a listener that
// removed the carrier's peer from its ingress_peers closes it with
// GoAwayPeerNotAllowed (anixops-protocol.md section 3.5).
func (c *QUICCarrier) CloseWithReason(reason GoAwayReason) error {
	c.mu.Lock()
	c.failLocked(errLocalClose, reason, true)
	c.mu.Unlock()
	<-c.done
	return nil
}

// goAwayLocked sends GOAWAY once and arms the drain deadline.
func (c *QUICCarrier) goAwayLocked(reason GoAwayReason, drain time.Duration) {
	if !c.goAwaySent {
		c.goAwaySent = true
		var last uint32
		if c.role == RoleAcceptor && c.accepted {
			// The first id not accepted (HTTP/3's convention): the
			// dialler retries every stream from there on elsewhere.
			last = uint32(min(int64(c.lastAccepted)+4, MaxStreamID)) // #nosec G115 -- clamped to MaxStreamID, which fits
		}
		c.ctrlQ <- ctrlItem{frame: quicFrameBytes(TypeGoAway, marshalGoAway(last, reason))}
		c.markDrainingLocked()
	}
	if drain > 0 && c.drainTimer == nil {
		c.drainTimer = time.AfterFunc(drain, func() {
			c.mu.Lock()
			c.failLocked(errors.New("drain timeout"), GoAwayNoError, true)
			c.mu.Unlock()
		})
	}
	c.drainCheckLocked()
}

func (c *QUICCarrier) markDrainingLocked() {
	select {
	case <-c.draining:
	default:
		close(c.draining)
	}
}

// drainCheckLocked is the end of a drain. An end with no streams left that has
// told the peer (GOAWAY) finishes its control stream: it is quiet, nothing
// will be opened or is unread by it. When the peer is quiet as well nothing is
// left in flight to lose, and the connection closes with no error. An end
// whose peer does not follow waits for it for DrainTimeout.
func (c *QUICCarrier) drainCheckLocked() {
	if c.err != nil || (!c.goAwaySent && !c.goAwayRecv) || c.active != 0 {
		return
	}
	if !c.goAwaySent {
		c.goAwayLocked(GoAwayNoError, 0)
	}
	if !c.ctrlFinSent {
		c.ctrlFinSent = true
		c.ctrlQ <- ctrlItem{fin: true}
		if !c.peerCtrlFin && c.quietTimer == nil {
			c.quietTimer = time.AfterFunc(c.cfg.DrainTimeout, func() {
				c.mu.Lock()
				c.failLocked(errors.New("drain: the peer did not finish its control stream"), GoAwayNoError, true)
				c.mu.Unlock()
			})
		}
	}
	if c.peerCtrlFin {
		c.failLocked(errors.New("drained"), GoAwayNoError, true)
	}
}

// failLocked ends the carrier: it records the cause, fails every stream and,
// when asked, has the connection closed with the reason as the application
// error code (run does it, outside the lock).
func (c *QUICCarrier) failLocked(cause error, reason GoAwayReason, closeConn bool) {
	if c.err != nil {
		return
	}
	c.err = carrierError(cause)
	c.markDrainingLocked()
	for _, s := range c.streams {
		s.abortQuietLocked(c.err)
	}
	close(c.dead)
	for _, t := range []*time.Timer{c.drainTimer, c.quietTimer} {
		if t != nil {
			t.Stop()
		}
	}
	if closeConn {
		c.closeReq <- closeRequest{reason: reason}
	}
}

// protoFail ends the carrier because the peer broke a rule.
func (c *QUICCarrier) protoFailLocked(perr *ProtocolError) {
	c.failLocked(perr, perr.Reason, true)
}

// ctrlWriter writes what the carrier queues for the control stream: GOAWAY,
// and the FIN that says this end is quiet.
func (c *QUICCarrier) ctrlWriter(out *quic.SendStream) {
	defer c.wg.Done()
	for {
		select {
		case it := <-c.ctrlQ:
			if len(it.frame) > 0 {
				_ = out.SetWriteDeadline(time.Now().Add(c.cfg.IdleTimeout))
				if _, err := out.Write(it.frame); err != nil {
					c.mu.Lock()
					c.failLocked(fmt.Errorf("control stream: %w", err), GoAwayInternal, true)
					c.mu.Unlock()
					return
				}
				c.n.framesSent.Add(1)
			}
			if it.fin {
				_ = out.Close()
				return
			}
		case <-c.dead:
			return
		}
	}
}

// controlLoop reads the peer's control stream after its SETTINGS: GOAWAY,
// whose handling is the drain, and the FIN that says the peer is quiet.
func (c *QUICCarrier) controlLoop(in *quic.ReceiveStream) {
	defer c.wg.Done()
	for {
		f, err := ReadFrame(in, int(c.local.MaxFrame))
		if err != nil {
			c.mu.Lock()
			switch {
			case errors.Is(err, io.EOF):
				// The peer finished its control stream: valid only once it
				// has told us it is draining.
				if !c.goAwayRecv {
					c.protoFailLocked(protoErr(GoAwayProtocolError, "the control stream ended without GOAWAY"))
				} else {
					c.peerCtrlFin = true
					c.drainCheckLocked()
				}
			case errors.Is(err, ErrFrameTooLarge):
				c.protoFailLocked(&ProtocolError{Reason: GoAwayFrameSize, Detail: err.Error()})
			case errors.Is(err, io.ErrUnexpectedEOF):
				c.protoFailLocked(protoErr(GoAwayProtocolError, "the control stream ended inside a frame"))
			default:
				var se *quic.StreamError
				if errors.As(err, &se) {
					// The control stream is critical: it may not be reset.
					c.protoFailLocked(protoErr(GoAwayProtocolError, "the peer reset its control stream"))
				} else {
					_ = c.connErrLocked(err)
				}
			}
			c.mu.Unlock()
			return
		}
		c.n.framesRecv.Add(1)
		c.mu.Lock()
		perr := c.handleControlLocked(f)
		if perr != nil {
			c.protoFailLocked(perr)
		}
		dead := c.err != nil
		c.mu.Unlock()
		if dead {
			return
		}
	}
}

// handleControlLocked processes one frame of the peer's control stream. A
// non-nil result is a rule the peer broke that ends the carrier.
func (c *QUICCarrier) handleControlLocked(f Frame) *ProtocolError {
	if c.err != nil {
		return nil
	}
	if f.StreamID != 0 {
		return protoErr(GoAwayProtocolError, "%s on stream %d of the control stream", f.Type, f.StreamID)
	}
	switch f.Type {
	case TypeSettings:
		return protoErr(GoAwaySettings, "SETTINGS repeated")
	case TypeGoAway:
		return c.onGoAwayLocked(f)
	}
	return nil // other types are not used on QUIC; unknown ones are ignored
}

func (c *QUICCarrier) onGoAwayLocked(f Frame) *ProtocolError {
	last, reason, err := parseGoAway(f.Payload)
	if err != nil {
		return protoErr(GoAwayProtocolError, "%v", err)
	}
	if c.goAwayRecv {
		return nil
	}
	c.goAwayRecv, c.goAwayRecvReason = true, reason
	c.markDrainingLocked()
	if c.role == RoleDialer {
		// Streams from the first id the peer did not accept never started:
		// they can be retried on another carrier.
		for id, s := range c.streams {
			if id >= quic.StreamID(last) {
				s.abortLocked(&StreamResetError{Reason: ResetRefusedStream, Remote: true}, ResetCancel)
			}
		}
	}
	c.drainCheckLocked()
	return nil
}

// acceptLoop takes the streams the dialler opens and reads their OPEN.
func (c *QUICCarrier) acceptLoop() {
	defer c.wg.Done()
	for {
		st, err := c.qc.AcceptStream(c.qc.Context()) // returns when the connection ends
		if err != nil {
			return
		}
		c.mu.Lock()
		ok := c.spawnLocked(func() { c.readOpen(st) })
		c.mu.Unlock()
		if !ok {
			return
		}
	}
}

// readOpen reads the OPEN that must begin a stream and answers it: the stream
// goes to the accept queue (the application then answers with RESULT), or is
// refused at once in a way the dialler can retry on another carrier.
func (c *QUICCarrier) readOpen(st *quic.Stream) {
	_ = st.SetReadDeadline(time.Now().Add(c.cfg.HandshakeTimeout))
	f, err := readQUICFrame(st, maxOpenPayload)
	if err == nil {
		_ = st.SetReadDeadline(time.Time{})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	refuse := func(reason ResetReason, charge bool) {
		c.n.streamsRefused.Add(1)
		c.n.resetsSent[codeIndex(uint16(reason))].Add(1)
		st.CancelRead(quic.StreamErrorCode(reason))
		st.CancelWrite(quic.StreamErrorCode(reason))
		if charge && !c.calm.take(time.Now()) {
			c.protoFailLocked(protoErr(GoAwayCalm, "too many refused streams"))
		}
	}
	var p OpenParams
	switch {
	case err != nil:
		var se *quic.StreamError
		if errors.As(err, &se) && se.Remote {
			return // the dialler gave up the stream before saying anything
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, ErrQUICFrame) || errors.Is(err, ErrFrameTooLarge) ||
			errors.Is(err, os.ErrDeadlineExceeded) {
			refuse(ResetProtocolError, true)
		} // else the connection ended and the carrier with it
		return
	case c.goAwaySent || c.goAwayRecv:
		// OPENs already in flight when the carrier started to drain: a race
		// every dialler can lose, not misbehaviour.
		refuse(ResetRefusedStream, false)
		return
	case f.Type != TypeOpen || p.UnmarshalBinary(f.Payload) != nil:
		refuse(ResetProtocolError, true)
		return
	case c.active >= int(c.local.MaxStreams) || len(c.acceptCh) >= cap(c.acceptCh):
		refuse(ResetRefusedStream, true)
		return
	case st.StreamID() > MaxStreamID-4:
		// ids past what GOAWAY can name: a dialler of this package never gets
		// here (Open stops first), so the peer is not one.
		refuse(ResetRefusedStream, true)
		return
	}
	c.n.framesRecv.Add(1)
	s := c.newStreamLocked(st, p)
	if id := st.StreamID(); !c.accepted || id > c.lastAccepted {
		c.accepted, c.lastAccepted = true, id
	}
	c.n.streamsAccepted.Add(1)
	if p.Kind == StreamUDP {
		c.spawnLocked(func() { c.udpRead(s) })
	}
	c.acceptCh <- s // never blocks: the queue's room was just checked, and only readOpen sends under c.mu
}

// keepaliveLoop is L5: a carrier that is alive (QUIC's idle timeout would
// have closed it otherwise) but leaves an OPEN unanswered for ResultTimeout is
// retired, and the unanswered streams fail so their callers retry on another
// carrier.
func (c *QUICCarrier) keepaliveLoop() {
	defer c.wg.Done()
	t := time.NewTicker(max(c.cfg.ResultTimeout/4, time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-c.dead:
			return
		}
		c.mu.Lock()
		if c.err != nil {
			c.mu.Unlock()
			return
		}
		if !c.goAwaySent {
			c.retireStuckLocked(time.Now())
		}
		c.mu.Unlock()
	}
}

func (c *QUICCarrier) retireStuckLocked(now time.Time) {
	var stuck []*QUICStream
	for _, s := range c.streams {
		if s.awaiting && now.Sub(s.openedAt) >= c.cfg.ResultTimeout {
			stuck = append(stuck, s)
		}
	}
	if len(stuck) == 0 {
		return
	}
	c.goAwayLocked(GoAwayStuck, 0)
	for _, s := range stuck {
		s.abortLocked(ErrOpenTimeout, ResetCancel)
	}
}

// readResult reads the RESULT that must begin the listener's side of a stream
// the dialler opened.
func (c *QUICCarrier) readResult(s *QUICStream) {
	f, err := readQUICFrame(s.st, resultPayloadLen)
	c.mu.Lock()
	defer c.mu.Unlock()
	defer s.settleLocked()
	if s.removed || s.err != nil {
		return
	}
	if err != nil {
		var se *quic.StreamError
		switch {
		case errors.As(err, &se):
			s.abortLocked(&StreamResetError{Reason: resetReasonOf(se.ErrorCode), Remote: se.Remote}, 0)
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, ErrQUICFrame), errors.Is(err, ErrFrameTooLarge):
			s.abortLocked(&StreamResetError{Reason: ResetProtocolError}, ResetProtocolError)
			c.chargeLocked()
		default:
			_ = s.failedLocked(err)
		}
		return
	}
	var code ResultCode
	if f.Type == TypeResult {
		code, err = parseResult(f.Payload)
	}
	if f.Type != TypeResult || err != nil {
		s.abortLocked(&StreamResetError{Reason: ResetProtocolError}, ResetProtocolError)
		c.chargeLocked()
		return
	}
	c.n.framesRecv.Add(1)
	s.answered, s.awaiting, s.resultCode = true, false, code
	if code != ResultOK {
		c.n.resultFailures[codeIndex(uint16(code))].Add(1)
		s.abortLocked(&ResultError{Code: code}, ResetCancel)
		return
	}
	if s.kind == StreamUDP {
		c.spawnLocked(func() { c.udpRead(s) })
	}
}

// chargeLocked draws on the peer's budget of answers for a stream error.
func (c *QUICCarrier) chargeLocked() {
	if !c.calm.take(time.Now()) {
		c.protoFailLocked(protoErr(GoAwayCalm, "too many stream errors"))
	}
}

// resetReasonOf converts a QUIC stream error code to a ResetReason; a code
// that is not one this version knows counts as a cancellation (section 4.10).
func resetReasonOf(code quic.StreamErrorCode) ResetReason {
	if code >= 1 && code <= quic.StreamErrorCode(ResetInternal) {
		return ResetReason(code)
	}
	return ResetCancel
}
