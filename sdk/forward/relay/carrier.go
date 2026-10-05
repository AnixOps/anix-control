package relay

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// Role is which end of a carrier this is. Only the dialler opens streams:
// traffic always flows from the previous hop to the next, so the listening
// end never needs to (anixops-protocol.md section 4.1).
type Role uint8

const (
	// RoleDialer is the previous hop's end: it opens streams.
	RoleDialer Role = 1
	// RoleAcceptor is the next hop's end: it accepts streams.
	RoleAcceptor Role = 2
)

// String returns the role's name.
func (r Role) String() string {
	switch r {
	case RoleDialer:
		return "dialer"
	case RoleAcceptor:
		return "acceptor"
	}
	return fmt.Sprintf("role(%d)", uint8(r))
}

const (
	// batchLimit is how many payload bytes the writer collects before it
	// writes them, so control frames never wait behind much data.
	batchLimit = 128 * 1024
	// maxControlQueue bounds the control frames waiting for the writer. Each
	// one answers something the peer did, so the queue only grows when the
	// peer stops reading while still sending; then the carrier ends.
	maxControlQueue = 8192

	// calmBurst and calmRate bound the answers a peer can extract by
	// misbehaving (pings, refused or malformed OPENs, unknown frames): a
	// well-behaved peer causes none of them in numbers.
	calmBurst = 64
	calmRate  = 64 // per second
)

// outFrame is a frame queued for the writer.
type outFrame struct {
	typ     FrameType
	flags   uint8
	id      uint32
	payload []byte
}

// A Carrier is one authenticated connection between two nodes for one hop of
// one route, multiplexing streams (anixops-protocol.md section 4). It
// speaks the frame protocol of section 4.2 over any net.Conn: the TLS and
// plaintext carriers of section 5 differ only in how the connection
// was made and what is known about the peer.
//
// A Carrier is safe for concurrent use. It owns its connection: closing the
// carrier closes it, and the carrier ends when the connection fails.
type ConnCarrier struct {
	conn  net.Conn
	role  Role
	cfg   Config
	local Settings
	peer  Settings
	start time.Time

	// Set by the transport glue before the carrier is shared; zero for a
	// carrier made with NewConnCarrier over a connection of the caller's own.
	linkPeer link.Peer
	ctype    CarrierType

	// mu guards everything below it, and the mutable state of every stream.
	mu      sync.Mutex
	streams map[uint32]*ConnStream
	active  int

	nextID       uint32 // dialler: the next stream id to use
	lastPeerID   uint32 // acceptor: the highest stream id the peer opened
	lastAccepted uint32 // acceptor: the highest stream id accepted, for GOAWAY

	sendCredit   int64 // carrier credit the peer granted this end
	recvCredit   int64 // carrier credit this end granted, still unused by the peer
	recvConsumed int64 // bytes the application consumed since the last carrier WINDOW
	carrierThres int64
	ctrl         []outFrame
	ring         []*ConnStream // streams with data to send, in service order
	ringSpare    []*ConnStream

	goAwaySent       bool
	goAwayRecv       bool
	goAwayRecvReason GoAwayReason
	drainTimer       *time.Timer
	closeTimer       *time.Timer
	err              error

	pingSeq    uint64
	pingSentAt time.Time
	pingOut    bool

	calmTokens float64
	calmAt     time.Time

	acceptCh chan *ConnStream
	wake     chan struct{} // wakes the writer
	dead     chan struct{} // closed when err is set
	draining chan struct{} // closed with the first GOAWAY, sent or received
	done     chan struct{} // closed when the connection is closed and the goroutines exited
	wg       sync.WaitGroup
	connOnce sync.Once

	lastRecv atomic.Int64 // time of the last frame received, since start
	n        counters
}

// NewConnCarrier runs the SETTINGS exchange over conn and returns the carrier.
// Both ends send SETTINGS first; neither returns before it has the peer's, so
// the limits of both ends are known before the first stream opens and a
// dialler that gets a carrier knows the peer is speaking this protocol.
// (For the TLS carrier it also means the listener has finished verifying the
// dialler: in TLS 1.3 the client's handshake completes before the server has
// checked its certificate.) NewConnCarrier takes ownership of conn and closes it
// on failure.
func NewConnCarrier(conn net.Conn, role Role, cfg Config) (*ConnCarrier, error) {
	cfg, err := cfg.withDefaults(role)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	local := cfg.settings(role)
	peer, err := exchangeSettings(conn, local, cfg.HandshakeTimeout)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	now := time.Now()
	c := &ConnCarrier{
		conn:         conn,
		role:         role,
		cfg:          cfg,
		local:        local,
		peer:         peer,
		start:        now,
		streams:      make(map[uint32]*ConnStream),
		nextID:       1,
		sendCredit:   int64(peer.CarrierWindow),
		recvCredit:   int64(local.CarrierWindow),
		carrierThres: int64(local.CarrierWindow / 2),
		calmTokens:   calmBurst,
		calmAt:       now,
		acceptCh:     make(chan *ConnStream, cfg.AcceptQueue),
		wake:         make(chan struct{}, 1),
		dead:         make(chan struct{}),
		draining:     make(chan struct{}),
		done:         make(chan struct{}),
	}
	c.wg.Add(3)
	go c.readLoop()
	go c.writeLoop()
	go c.keepaliveLoop()
	go func() {
		c.wg.Wait()
		c.mu.Lock()
		for _, t := range []*time.Timer{c.drainTimer, c.closeTimer} {
			if t != nil {
				t.Stop()
			}
		}
		c.mu.Unlock()
		close(c.done)
	}()
	return c, nil
}

// exchangeSettings sends local and reads the peer's SETTINGS, the first frame
// of each side, within timeout. The two run concurrently because a
// connection with no buffering (net.Pipe) would otherwise deadlock.
func exchangeSettings(conn net.Conn, local Settings, timeout time.Duration) (Settings, error) {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Settings{}, err
	}
	werr := make(chan error, 1)
	go func() {
		werr <- WriteFrame(conn, Frame{Type: TypeSettings, Payload: local.Marshal()})
	}()
	f, err := ReadFrame(conn, int(local.MaxFrame))
	if err != nil {
		return Settings{}, fmt.Errorf("relay: reading the peer's SETTINGS: %w", err)
	}
	if f.Type != TypeSettings || f.StreamID != 0 {
		return Settings{}, &ProtocolError{Reason: GoAwaySettings, Detail: "the first frame is not SETTINGS on stream 0"}
	}
	peer, err := ParseSettings(f.Payload)
	if err != nil {
		return Settings{}, &ProtocolError{Reason: GoAwaySettings, Detail: err.Error()}
	}
	if err := <-werr; err != nil {
		return Settings{}, fmt.Errorf("relay: sending SETTINGS: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return Settings{}, err
	}
	return peer, nil
}

// Role returns which end of the carrier this is.
func (c *ConnCarrier) Role() Role { return c.role }

// LocalSettings returns the limits this end announced.
func (c *ConnCarrier) LocalSettings() Settings { return c.local }

// PeerSettings returns the limits the peer announced.
func (c *ConnCarrier) PeerSettings() Settings { return c.peer }

// LocalAddr and RemoteAddr are the connection's addresses.
func (c *ConnCarrier) LocalAddr() net.Addr  { return c.conn.LocalAddr() }
func (c *ConnCarrier) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// Done is closed when the carrier has ended and released its connection and
// goroutines.
func (c *ConnCarrier) Done() <-chan struct{} { return c.done }

// Draining is closed when a GOAWAY was sent or received: the carrier takes no
// new streams and ends when its streams do. A dialler uses it to stop
// choosing the carrier and open another.
func (c *ConnCarrier) Draining() <-chan struct{} { return c.draining }

// Err returns why the carrier ended, wrapping ErrCarrierClosed, or nil while
// it runs.
func (c *ConnCarrier) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// PeerGoAway returns the reason of the GOAWAY the peer sent, if it did.
func (c *ConnCarrier) PeerGoAway() (GoAwayReason, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.goAwayRecvReason, c.goAwayRecv
}

// Peer returns the node at the other end as the link layer verified it: its
// SPIFFE identity and certificate chain on a TLS carrier, the zero value on
// a plaintext carrier or one made with NewConnCarrier over the caller's own
// connection.
func (c *ConnCarrier) Peer() link.Peer { return c.linkPeer }

// Type returns how the carrier's connection was made.
func (c *ConnCarrier) Type() CarrierType { return c.ctype }

// RTT returns the latest PING round trip, zero before the first. It is a
// free latency sample for the health checks (anixops-protocol.md section
// 6.4).
func (c *ConnCarrier) RTT() time.Duration { return time.Duration(c.n.rtt.Load()) }

// Stats returns a snapshot of the carrier's counters.
func (c *ConnCarrier) Stats() Stats {
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	return c.n.snapshot(active)
}

// ActiveStreams returns the number of streams that have not finished.
func (c *ConnCarrier) ActiveStreams() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Open starts a stream: it queues the OPEN frame and returns at once, without
// waiting for the listener's answer, so the caller can send the client's first
// bytes right behind it (stream-level zero round trip, section 4.3). Use
// Stream.AwaitResult to learn the answer. It never blocks: a carrier that
// cannot take the stream says so with ErrGoAway, ErrStreamLimit or
// ErrIDsExhausted, and the caller opens it on another carrier.
func (c *ConnCarrier) Open(p OpenParams) (Stream, error) {
	payload, err := p.MarshalBinary()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.role != RoleDialer:
		return nil, ErrWrongRole
	case c.err != nil:
		return nil, c.err
	case c.goAwaySent || c.goAwayRecv:
		return nil, ErrGoAway
	case c.nextID > MaxStreamID:
		return nil, ErrIDsExhausted
	case c.active >= int(c.peer.MaxStreams):
		return nil, ErrStreamLimit
	case len(c.ctrl) >= maxControlQueue:
		return nil, ErrStreamLimit
	}
	s := c.newStreamLocked(c.nextID, p)
	s.awaiting = true
	s.openedAt = time.Now()
	c.nextID += 2
	c.streams[s.id] = s
	c.active++
	c.queueLocked(outFrame{typ: TypeOpen, id: s.id, payload: payload})
	c.n.streamsOpened.Add(1)
	return s, nil
}

// Accept returns the next stream the peer opened. The caller must answer it
// with Stream.Accept or Stream.Reject; Write, CloseWrite and Close answer it
// implicitly, so no stream is ever left unanswered (L2).
func (c *ConnCarrier) Accept(ctx context.Context) (Stream, error) {
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
// GoAwayShutdown). The carrier ends when its streams have, or after drain
// when drain is positive, whichever comes first. A retired dialler carrier
// keeps serving its open streams; an acceptor refuses OPENs from then on.
func (c *ConnCarrier) GoAway(reason GoAwayReason, drain time.Duration) error {
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
func (c *ConnCarrier) Shutdown(ctx context.Context) error {
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

// Close ends the carrier at once: every stream fails, a GOAWAY of
// GoAwayShutdown is sent if the writer can, and the connection is closed. It
// returns when the carrier has released its goroutines.
func (c *ConnCarrier) Close() error { return c.CloseWithReason(GoAwayShutdown) }

// CloseWithReason is Close with the reason the GOAWAY names: a listener that
// removed the carrier's peer from its ingress_peers closes it with
// GoAwayPeerNotAllowed (anixops-protocol.md section 3.5).
func (c *ConnCarrier) CloseWithReason(reason GoAwayReason) error {
	c.mu.Lock()
	c.failLocked(errLocalClose, reason, true)
	c.mu.Unlock()
	<-c.done
	return nil
}

var errLocalClose = errors.New("closed locally")

// closeWith ends the carrier for the given reason, telling the peer.
func (c *ConnCarrier) closeWith(cause error, reason GoAwayReason) {
	c.mu.Lock()
	c.failLocked(cause, reason, true)
	c.mu.Unlock()
}

// goAwayLocked sends GOAWAY once and arms the drain deadline.
func (c *ConnCarrier) goAwayLocked(reason GoAwayReason, drain time.Duration) {
	if !c.goAwaySent {
		c.goAwaySent = true
		last := uint32(0)
		if c.role == RoleAcceptor {
			last = c.lastAccepted
		}
		c.queueLocked(outFrame{typ: TypeGoAway, payload: marshalGoAway(last, reason)})
		c.markDrainingLocked()
	}
	if drain > 0 && c.drainTimer == nil {
		c.drainTimer = time.AfterFunc(drain, func() {
			c.mu.Lock()
			c.failLocked(errors.New("drain timeout"), GoAwayNoError, false)
			c.mu.Unlock()
		})
	}
	c.drainCheckLocked()
}

func (c *ConnCarrier) markDrainingLocked() {
	select {
	case <-c.draining:
	default:
		close(c.draining)
	}
}

// drainCheckLocked ends a draining carrier whose last stream finished.
func (c *ConnCarrier) drainCheckLocked() {
	if c.err == nil && (c.goAwaySent || c.goAwayRecv) && c.active == 0 {
		c.failLocked(errors.New("drained"), GoAwayNoError, false)
	}
}

// failLocked ends the carrier: it records the cause, fails every stream,
// queues a final GOAWAY when asked, and lets the writer flush and close the
// connection (the grace timer closes it regardless).
func (c *ConnCarrier) failLocked(cause error, reason GoAwayReason, sendGoAway bool) {
	if c.err != nil {
		return
	}
	c.err = carrierError(cause)
	if sendGoAway && !c.goAwaySent {
		c.goAwaySent = true
		last := uint32(0)
		if c.role == RoleAcceptor {
			last = c.lastAccepted
		}
		c.ctrl = append(c.ctrl, outFrame{typ: TypeGoAway, payload: marshalGoAway(last, reason)})
	}
	c.markDrainingLocked()
	for _, s := range c.streams {
		s.abortLocked(c.err, 0)
	}
	c.ring = nil
	close(c.dead)
	if c.drainTimer != nil {
		c.drainTimer.Stop()
	}
	c.closeTimer = time.AfterFunc(closeGrace, c.abortConn)
	c.wakeWriterLocked()
}

func (c *ConnCarrier) closeConn() {
	c.connOnce.Do(func() { _ = c.conn.Close() })
}

// abortConn closes the connection below any TLS layer, at once. The grace
// timer uses it: a TLS Close first tries to send close_notify, which blocks
// for seconds when the peer is dead and its socket full, and a Close that is
// stuck there is released when the TCP connection under it goes away. The
// frame protocol ends with GOAWAY, so nothing depends on close_notify.
func (c *ConnCarrier) abortConn() {
	conn := c.conn
	for {
		u, ok := conn.(interface{ NetConn() net.Conn })
		if !ok {
			break
		}
		next := u.NetConn()
		if next == nil || next == conn {
			break
		}
		conn = next
	}
	_ = conn.Close()
}

// protoErr builds a carrier-ending protocol error.
func protoErr(reason GoAwayReason, format string, args ...any) *ProtocolError {
	return &ProtocolError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// queueLocked appends a control frame for the writer. A carrier whose peer
// has stopped reading while still sending ends instead of queueing without
// bound.
func (c *ConnCarrier) queueLocked(f outFrame) {
	if c.err != nil {
		return
	}
	if len(c.ctrl) >= maxControlQueue {
		c.failLocked(errors.New("the peer does not read: control queue full"), GoAwayCalm, false)
		return
	}
	c.ctrl = append(c.ctrl, f)
	c.wakeWriterLocked()
}

func (c *ConnCarrier) wakeWriterLocked() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// takeCalmLocked spends one answer from the peer's budget and reports whether
// there was one.
func (c *ConnCarrier) takeCalmLocked() bool {
	now := time.Now()
	c.calmTokens = min(calmBurst, c.calmTokens+now.Sub(c.calmAt).Seconds()*calmRate)
	c.calmAt = now
	if c.calmTokens < 1 {
		return false
	}
	c.calmTokens--
	return true
}

// releaseLocked returns n bytes of carrier credit: the application consumed
// or discarded n received bytes. WINDOW is sent once half the window is back.
func (c *ConnCarrier) releaseLocked(n int) {
	c.recvConsumed += int64(n)
	if c.recvConsumed >= c.carrierThres && c.err == nil {
		inc := c.recvConsumed
		c.recvConsumed = 0
		c.recvCredit += inc
		c.queueLocked(outFrame{typ: TypeWindow, payload: marshalWindow(creditIncrement(inc))})
	}
}

// markReadyLocked puts the stream in the writer's service order.
func (c *ConnCarrier) markReadyLocked(s *ConnStream) {
	if !s.inRing && c.err == nil {
		s.inRing = true
		c.ring = append(c.ring, s)
	}
	c.wakeWriterLocked()
}

// collectLocked gathers the next batch for the writer: every control frame,
// then one frame per ready stream in turn, until batchLimit bytes. Streams
// out of stream credit leave the service order and come back with a WINDOW;
// streams only short of carrier credit stay.
func (c *ConnCarrier) collectLocked(batch []outFrame) []outFrame {
	batch = append(batch, c.ctrl...)
	clear(c.ctrl)
	c.ctrl = c.ctrl[:0]
	if c.err != nil {
		c.ring = nil
		return batch
	}
	bytes := 0
	next := c.ringSpare[:0]
	for _, s := range c.ring {
		if bytes < batchLimit {
			if f, ok := s.nextFrameLocked(); ok {
				batch = append(batch, f)
				bytes += len(f.payload)
			}
		}
		if s.wantsSendLocked() {
			next = append(next, s)
		} else {
			s.inRing = false
		}
	}
	clear(c.ring)
	c.ring, c.ringSpare = next, c.ring[:0]
	return batch
}

func (c *ConnCarrier) writeLoop() {
	defer c.wg.Done()
	defer c.closeConn()
	bw := bufio.NewWriterSize(c.conn, 64*1024)
	var batch []outFrame
	var buf []byte
	for {
		c.mu.Lock()
		batch = c.collectLocked(batch[:0])
		dead := c.err != nil
		c.mu.Unlock()
		if len(batch) == 0 {
			if dead {
				return
			}
			select {
			case <-c.wake:
			case <-c.dead:
			}
			continue
		}
		timeout := c.cfg.IdleTimeout
		if dead {
			timeout = closeGrace
		}
		_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
		var payload, frames uint64
		var datagrams uint64
		for _, f := range batch {
			var err error
			buf, err = AppendFrame(buf[:0], Frame{Type: f.typ, Flags: f.flags, StreamID: f.id, Payload: f.payload})
			if err == nil {
				_, err = bw.Write(buf)
			}
			if err != nil {
				c.closeWith(fmt.Errorf("write: %w", err), GoAwayInternal)
				return
			}
			frames++
			switch f.typ {
			case TypeData:
				payload += uint64(len(f.payload))
			case TypeDatagram:
				payload += uint64(len(f.payload))
				datagrams++
			case TypeReset:
				if len(f.payload) == 2 {
					c.n.resetsSent[codeIndex(binary.BigEndian.Uint16(f.payload))].Add(1)
				}
			}
		}
		if err := bw.Flush(); err != nil {
			c.closeWith(fmt.Errorf("write: %w", err), GoAwayInternal)
			return
		}
		c.n.framesSent.Add(frames)
		c.n.bytesSent.Add(payload)
		c.n.datagramsSent.Add(datagrams)
		if dead {
			return
		}
	}
}

func (c *ConnCarrier) readLoop() {
	defer c.wg.Done()
	br := bufio.NewReaderSize(c.conn, 32*1024)
	for {
		f, err := ReadFrame(br, int(c.local.MaxFrame))
		if err != nil {
			c.mu.Lock()
			if errors.Is(err, ErrFrameTooLarge) {
				c.failLocked(&ProtocolError{Reason: GoAwayFrameSize, Detail: err.Error()}, GoAwayFrameSize, true)
			} else {
				c.failLocked(err, GoAwayNoError, false)
			}
			c.mu.Unlock()
			return
		}
		c.lastRecv.Store(int64(time.Since(c.start)))
		c.n.framesRecv.Add(1)
		c.mu.Lock()
		perr := c.handleLocked(f)
		if perr != nil {
			c.failLocked(perr, perr.Reason, true)
		}
		dead := c.err != nil
		c.mu.Unlock()
		if dead {
			return
		}
	}
}

// handleLocked processes one received frame. A non-nil result is a rule the
// peer broke that ends the carrier.
func (c *ConnCarrier) handleLocked(f Frame) *ProtocolError {
	if c.err != nil {
		return nil
	}
	id := f.StreamID
	switch f.Type {
	case TypeSettings:
		return protoErr(GoAwaySettings, "SETTINGS repeated")
	case TypePing:
		return c.onPingLocked(f)
	case TypeGoAway:
		return c.onGoAwayLocked(f)
	case TypeWindow:
		if id == 0 {
			return c.onCarrierWindowLocked(f)
		}
	case TypeOpen, TypeResult, TypeData, TypeReset, TypeDatagram:
		if id == 0 {
			return protoErr(GoAwayProtocolError, "%s on stream 0", f.Type)
		}
	default:
		if id == 0 {
			return nil // an unknown type on stream 0 is ignored
		}
	}
	if id&1 == 0 || id > MaxStreamID {
		return protoErr(GoAwayProtocolError, "%s on invalid stream id %d", f.Type, id)
	}
	if f.Type == TypeOpen {
		return c.onOpenLocked(f)
	}
	if f.Type == TypeResult && c.role != RoleDialer {
		return protoErr(GoAwayProtocolError, "RESULT sent to an acceptor")
	}
	s := c.streams[id]
	if s == nil {
		closed := id <= c.lastPeerID
		if c.role == RoleDialer {
			closed = id < c.nextID
		}
		if !closed {
			return protoErr(GoAwayProtocolError, "%s on stream %d, which was never opened", f.Type, id)
		}
		// A stream that finished here: the peer's frames were already in
		// flight. DATA still counts against the carrier's credit, and the
		// credit comes straight back.
		switch f.Type {
		case TypeData, TypeDatagram:
			n := len(f.Payload)
			if int64(n) > c.recvCredit {
				return protoErr(GoAwayFlowControl, "carrier credit exceeded")
			}
			c.recvCredit -= int64(n)
			c.releaseLocked(n)
		}
		return nil
	}
	switch f.Type {
	case TypeResult:
		return c.onResultLocked(s, f)
	case TypeData:
		return c.onDataLocked(s, f)
	case TypeDatagram:
		return c.onDatagramLocked(s, f)
	case TypeWindow:
		return c.onStreamWindowLocked(s, f)
	case TypeReset:
		return c.onResetLocked(s, f)
	}
	// An unknown type on a stream resets the stream.
	return c.streamErrorLocked(s, ResetUnknownFrame)
}

// streamErrorLocked resets a stream the peer misused. The reset is an answer
// the peer caused, so it draws on the peer's budget.
func (c *ConnCarrier) streamErrorLocked(s *ConnStream, reason ResetReason) *ProtocolError {
	s.abortLocked(&StreamResetError{Reason: reason}, reason)
	if !c.takeCalmLocked() {
		return protoErr(GoAwayCalm, "too many stream errors")
	}
	return nil
}

func (c *ConnCarrier) onPingLocked(f Frame) *ProtocolError {
	if f.StreamID != 0 {
		return protoErr(GoAwayProtocolError, "PING on stream %d", f.StreamID)
	}
	if len(f.Payload) != pingSize {
		return protoErr(GoAwayProtocolError, "PING payload of %d bytes", len(f.Payload))
	}
	if f.Flags&FlagACK != 0 {
		if c.pingOut && binary.BigEndian.Uint64(f.Payload) == c.pingSeq {
			c.pingOut = false
			c.n.rtt.Store(int64(time.Since(c.pingSentAt)))
		}
		return nil
	}
	if !c.takeCalmLocked() {
		return protoErr(GoAwayCalm, "too many pings")
	}
	c.queueLocked(outFrame{typ: TypePing, flags: FlagACK, payload: f.Payload})
	return nil
}

func (c *ConnCarrier) onGoAwayLocked(f Frame) *ProtocolError {
	if f.StreamID != 0 {
		return protoErr(GoAwayProtocolError, "GOAWAY on stream %d", f.StreamID)
	}
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
		// Streams above the last one the peer accepted never started: they
		// can be retried on another carrier.
		for id, s := range c.streams {
			if id > last {
				s.abortLocked(&StreamResetError{Reason: ResetRefusedStream, Remote: true}, 0)
			}
		}
	}
	c.drainCheckLocked()
	return nil
}

func (c *ConnCarrier) onCarrierWindowLocked(f Frame) *ProtocolError {
	inc, err := parseWindow(f.Payload)
	if err != nil {
		return protoErr(GoAwayProtocolError, "%v", err)
	}
	if c.sendCredit+int64(inc) > maxCreditValue {
		return protoErr(GoAwayFlowControl, "carrier credit overflow")
	}
	c.sendCredit += int64(inc)
	c.wakeWriterLocked()
	return nil
}

// onOpenLocked accepts or refuses a new stream. Every OPEN is answered: with
// the stream in the accept queue (the application then answers with RESULT),
// or at once with a RESET the dialler can retry on another carrier.
func (c *ConnCarrier) onOpenLocked(f Frame) *ProtocolError {
	if c.role != RoleAcceptor {
		return protoErr(GoAwayProtocolError, "OPEN sent to a dialler")
	}
	id := f.StreamID
	if id <= c.lastPeerID {
		return protoErr(GoAwayProtocolError, "stream id %d does not increase (last %d)", id, c.lastPeerID)
	}
	c.lastPeerID = id
	refuse := func(reason ResetReason, charge bool) *ProtocolError {
		c.n.streamsRefused.Add(1)
		c.queueLocked(outFrame{typ: TypeReset, id: id, payload: marshalReset(reason)})
		if charge && !c.takeCalmLocked() {
			return protoErr(GoAwayCalm, "too many refused streams")
		}
		return nil
	}
	if c.goAwaySent || c.goAwayRecv {
		// OPENs already in flight when the carrier started to drain: a race
		// every dialler can lose, not misbehaviour.
		return refuse(ResetRefusedStream, false)
	}
	var p OpenParams
	if err := p.UnmarshalBinary(f.Payload); err != nil {
		return refuse(ResetProtocolError, true)
	}
	if c.active >= int(c.local.MaxStreams) || len(c.acceptCh) >= cap(c.acceptCh) {
		return refuse(ResetRefusedStream, true)
	}
	s := c.newStreamLocked(id, p)
	c.streams[id] = s
	c.active++
	c.lastAccepted = id
	c.n.streamsAccepted.Add(1)
	c.acceptCh <- s // never blocks: the queue's room was just checked, and only this method sends
	return nil
}

func (c *ConnCarrier) onResultLocked(s *ConnStream, f Frame) *ProtocolError {
	code, err := parseResult(f.Payload)
	if err != nil {
		return protoErr(GoAwayProtocolError, "%v", err)
	}
	if s.answered {
		return c.streamErrorLocked(s, ResetProtocolError)
	}
	s.answered, s.awaiting, s.resultCode = true, false, code
	if code != ResultOK {
		c.n.resultFailures[codeIndex(uint16(code))].Add(1)
		s.abortLocked(&ResultError{Code: code}, 0)
		return nil
	}
	s.wakeLocked(&s.resultWake)
	return nil
}

func (c *ConnCarrier) onDataLocked(s *ConnStream, f Frame) *ProtocolError {
	n := len(f.Payload)
	if int64(n) > c.recvCredit {
		return protoErr(GoAwayFlowControl, "carrier credit exceeded")
	}
	c.recvCredit -= int64(n)
	fin := f.Flags&FlagFIN != 0
	switch {
	case !s.answered && c.role == RoleDialer:
		c.releaseLocked(n)
		return c.streamErrorLocked(s, ResetProtocolError)
	case s.remoteFin, n == 0 && !fin, n > 0 && s.kind != StreamTCP:
		c.releaseLocked(n)
		return c.streamErrorLocked(s, ResetProtocolError)
	case int64(n) > s.recvCredit:
		return protoErr(GoAwayFlowControl, "stream %d credit exceeded", s.id)
	}
	s.recvCredit -= int64(n)
	c.n.bytesRecv.Add(uint64(n))
	if n > 0 {
		s.recvQ = append(s.recvQ, f.Payload)
		s.recvLen += n
	}
	if fin {
		s.remoteFin = true
		s.afterFinLocked()
	}
	s.wakeLocked(&s.readWake)
	return nil
}

func (c *ConnCarrier) onDatagramLocked(s *ConnStream, f Frame) *ProtocolError {
	n := len(f.Payload)
	if int64(n) > c.recvCredit {
		return protoErr(GoAwayFlowControl, "carrier credit exceeded")
	}
	c.recvCredit -= int64(n)
	switch {
	case !s.answered && c.role == RoleDialer:
		c.releaseLocked(n)
		return c.streamErrorLocked(s, ResetProtocolError)
	case s.remoteFin, n == 0, s.kind != StreamUDP:
		c.releaseLocked(n)
		return c.streamErrorLocked(s, ResetProtocolError)
	case int64(n) > s.recvCredit:
		return protoErr(GoAwayFlowControl, "stream %d credit exceeded", s.id)
	}
	s.recvCredit -= int64(n)
	c.n.bytesRecv.Add(uint64(n))
	c.n.datagramsRecv.Add(1)
	s.recvQ = append(s.recvQ, f.Payload)
	s.recvLen += n
	s.wakeLocked(&s.readWake)
	return nil
}

func (c *ConnCarrier) onStreamWindowLocked(s *ConnStream, f Frame) *ProtocolError {
	inc, err := parseWindow(f.Payload)
	if err != nil {
		return c.streamErrorLocked(s, ResetProtocolError)
	}
	if s.sendCredit+int64(inc) > maxCreditValue {
		return c.streamErrorLocked(s, ResetFlowControl)
	}
	s.sendCredit += int64(inc)
	if s.wantsSendLocked() {
		c.markReadyLocked(s)
	}
	return nil
}

func (c *ConnCarrier) onResetLocked(s *ConnStream, f Frame) *ProtocolError {
	reason, err := parseReset(f.Payload)
	if err != nil {
		return protoErr(GoAwayProtocolError, "%v", err)
	}
	c.n.resetsRecv[codeIndex(uint16(reason))].Add(1)
	s.abortLocked(&StreamResetError{Reason: reason, Remote: true}, 0)
	return nil
}

// keepaliveLoop sends PING after PingInterval without a received frame,
// closes the carrier after IdleTimeout (L4), and retires a carrier whose
// OPENs go unanswered (L5).
func (c *ConnCarrier) keepaliveLoop() {
	defer c.wg.Done()
	tick := min(c.cfg.PingInterval, c.cfg.IdleTimeout) / 4
	if c.cfg.ResultTimeout > 0 {
		tick = min(tick, c.cfg.ResultTimeout/4)
	}
	t := time.NewTicker(max(tick, time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-c.dead:
			return
		}
		idle := time.Since(c.start) - time.Duration(c.lastRecv.Load())
		c.mu.Lock()
		switch {
		case c.err != nil:
			c.mu.Unlock()
			return
		case idle >= c.cfg.IdleTimeout:
			c.n.keepaliveTimeouts.Add(1)
			c.failLocked(ErrIdleTimeout, GoAwayIdleTimeout, true)
			c.mu.Unlock()
			return
		}
		now := time.Now()
		if idle >= c.cfg.PingInterval && (!c.pingOut || now.Sub(c.pingSentAt) >= c.cfg.PingInterval) {
			c.pingSeq++
			c.pingSentAt, c.pingOut = now, true
			c.queueLocked(outFrame{typ: TypePing, payload: binary.BigEndian.AppendUint64(nil, c.pingSeq)})
			c.n.pingsSent.Add(1)
		}
		if c.role == RoleDialer && c.cfg.ResultTimeout > 0 && !c.goAwaySent {
			c.retireStuckLocked(now)
		}
		c.mu.Unlock()
	}
}

// retireStuckLocked is L5: a carrier that is alive (it would have been closed
// otherwise) but leaves an OPEN unanswered for ResultTimeout is retired, and
// the unanswered streams fail so their callers retry on another carrier.
func (c *ConnCarrier) retireStuckLocked(now time.Time) {
	var stuck []*ConnStream
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
