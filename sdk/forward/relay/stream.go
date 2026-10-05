package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// A Stream is one client connection (TCP) or one client UDP association
// inside a Carrier. A TCP stream implements net.Conn plus CloseWrite, so a
// proxy copies bytes into and out of it as it would a TCP connection,
// half-close included; a UDP stream carries datagrams with WriteDatagram
// and ReadDatagram.
//
// A stream may be used by one reader and one writer goroutine at a time (and
// any goroutine may call Close, Reset or set a deadline), as a net.Conn may.
type ConnStream struct {
	c      *ConnCarrier
	id     uint32
	kind   StreamKind
	params OpenParams

	// Everything below is guarded by c.mu.

	// receive
	recvQ        [][]byte // received DATA chunks, or datagrams on a UDP stream
	recvLen      int      // bytes in recvQ
	recvCredit   int64    // credit granted to the peer and not yet used
	recvConsumed int64    // bytes the application consumed since the last WINDOW
	remoteFin    bool     // the peer finished its direction

	// send
	sendQ      [][]byte // bytes (or datagrams) waiting for the writer
	sendLen    int
	sendCredit int64 // credit the peer granted
	finQueued  bool  // CloseWrite was called: FIN follows the queued data
	finSent    bool
	inRing     bool
	stalled    bool // counted as waiting for credit

	// lifecycle
	answered    bool // RESULT sent (acceptor) or received (dialler)
	resultCode  ResultCode
	awaiting    bool // dialler: OPEN sent, RESULT not yet received
	openedAt    time.Time
	err         error // set when the stream was aborted or refused
	closedByApp bool
	removed     bool // finished: no longer counted or looked up by the carrier

	readWake, writeWake, resultWake chan struct{}
	rdl, wdl                        deadline
}

var _ net.Conn = (*ConnStream)(nil)

func (c *ConnCarrier) newStreamLocked(id uint32, p OpenParams) *ConnStream {
	s := &ConnStream{
		c:          c,
		id:         id,
		kind:       p.Kind,
		params:     p,
		recvCredit: int64(c.local.StreamWindow),
		sendCredit: int64(c.peer.StreamWindow),
		rdl:        newDeadline(),
		wdl:        newDeadline(),
	}
	return s
}

// ID returns the stream id, unique within the carrier.
func (s *ConnStream) ID() uint64 { return uint64(s.id) }

// Kind returns whether the stream carries a TCP connection or UDP datagrams.
func (s *ConnStream) Kind() StreamKind { return s.kind }

// Params returns the OPEN parameters: the ones the dialler sent, or the ones
// this side sent.
func (s *ConnStream) Params() OpenParams { return s.params }

// Answered reports whether the listener's RESULT reached a dialler-side
// stream. A stream that failed without one (its carrier ended, or it was
// refused) never started as far as the dialler knows: nothing came back, and
// the caller may retry it on another carrier or upstream with the bytes it
// kept.
func (s *ConnStream) Answered() bool {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.answered
}

// Carrier returns the carrier the stream runs on.
func (s *ConnStream) Carrier() Carrier { return s.c }

// LocalAddr and RemoteAddr are the carrier connection's addresses.
func (s *ConnStream) LocalAddr() net.Addr  { return s.c.conn.LocalAddr() }
func (s *ConnStream) RemoteAddr() net.Addr { return s.c.conn.RemoteAddr() }

// wakeLocked wakes everyone waiting on ch.
func (s *ConnStream) wakeLocked(ch *chan struct{}) {
	if *ch != nil {
		close(*ch)
		*ch = nil
	}
}

// waitLocked returns the channel a waiter blocks on until the next wakeLocked
// of ch.
func (s *ConnStream) waitLocked(ch *chan struct{}) <-chan struct{} {
	if *ch == nil {
		*ch = make(chan struct{})
	}
	return *ch
}

func (s *ConnStream) wakeAllLocked() {
	s.wakeLocked(&s.readWake)
	s.wakeLocked(&s.writeWake)
	s.wakeLocked(&s.resultWake)
}

// wantsSendLocked reports whether the writer could still make progress on
// the stream if credit allows: data in the queue with stream credit left, or
// a FIN to send.
func (s *ConnStream) wantsSendLocked() bool {
	if s.err != nil || s.removed {
		return false
	}
	if s.sendLen > 0 {
		return s.sendCredit > 0
	}
	return s.finQueued && !s.finSent
}

// nextFrameLocked takes the next frame the stream has to send, within its
// own and the carrier's credit, and spends that credit.
func (s *ConnStream) nextFrameLocked() (outFrame, bool) {
	c := s.c
	if s.err != nil || s.removed {
		return outFrame{}, false
	}
	if s.sendLen > 0 {
		first := s.sendQ[0]
		avail := min(s.sendCredit, c.sendCredit, int64(c.peer.MaxFrame))
		if s.kind == StreamTCP {
			if avail <= 0 {
				s.noteStallLocked()
				return outFrame{}, false
			}
			n := int(min(int64(len(first)), avail))
			payload := first[:n]
			if n == len(first) {
				s.sendQ[0] = nil
				s.sendQ = s.sendQ[1:]
			} else {
				s.sendQ[0] = first[n:]
			}
			s.sendLen -= n
			s.sendCredit -= int64(n)
			c.sendCredit -= int64(n)
			s.stalled = false
			f := outFrame{typ: TypeData, id: s.id, payload: payload}
			if s.sendLen == 0 {
				s.sendQ = nil
				if s.finQueued {
					f.flags = FlagFIN
					s.finSent = true
					s.afterFinLocked()
				}
			}
			s.wakeLocked(&s.writeWake)
			return f, true
		}
		// A datagram goes whole or not at all.
		n := len(first)
		if int64(n) > min(s.sendCredit, c.sendCredit) {
			s.noteStallLocked()
			return outFrame{}, false
		}
		s.sendQ[0] = nil
		s.sendQ = s.sendQ[1:]
		s.sendLen -= n
		s.sendCredit -= int64(n)
		c.sendCredit -= int64(n)
		s.stalled = false
		s.wakeLocked(&s.writeWake)
		return outFrame{typ: TypeDatagram, id: s.id, payload: first}, true
	}
	if s.finQueued && !s.finSent {
		s.finSent = true
		s.afterFinLocked()
		return outFrame{typ: TypeData, flags: FlagFIN, id: s.id}, true
	}
	return outFrame{}, false
}

// noteStallLocked counts the first time the stream waits for credit.
func (s *ConnStream) noteStallLocked() {
	if s.stalled {
		return
	}
	s.stalled = true
	if s.sendCredit <= 0 {
		s.c.n.streamStalls.Add(1)
	} else {
		s.c.n.carrierStalls.Add(1)
	}
}

// afterFinLocked finishes the stream once both directions have: its FIN sent
// and the peer's received.
func (s *ConnStream) afterFinLocked() {
	if s.finSent && s.remoteFin {
		s.finishLocked()
	}
}

// finishLocked takes the stream out of the carrier: it no longer counts
// against the stream limit, and the carrier ends if it was only draining.
// Data the application has not read yet stays readable.
func (s *ConnStream) finishLocked() {
	if s.removed {
		return
	}
	s.removed = true
	c := s.c
	delete(c.streams, s.id)
	c.active--
	s.wakeAllLocked()
	c.drainCheckLocked()
}

// discardLocked drops what the application will never read or send and gives
// the carrier the credit the unread bytes held.
func (s *ConnStream) discardLocked() {
	n := s.recvLen
	s.recvQ, s.recvLen = nil, 0
	s.sendQ, s.sendLen = nil, 0
	if n > 0 {
		s.c.releaseLocked(n)
	}
}

// abortLocked ends both directions at once: err is what the stream's
// callers see (the first error wins), and a nonzero reset sends RESET.
func (s *ConnStream) abortLocked(err error, reset ResetReason) {
	if s.err == nil {
		s.err = err
	}
	if reset != 0 && !s.removed {
		s.c.queueLocked(outFrame{typ: TypeReset, id: s.id, payload: marshalReset(reset)})
	}
	s.discardLocked()
	s.finishLocked()
}

// ensureAnsweredLocked sends the stream's RESULT if it is a listener-side
// stream the application has not answered yet.
func (s *ConnStream) ensureAnsweredLocked() {
	if s.c.role == RoleAcceptor && !s.answered {
		s.answered = true
		s.c.queueLocked(outFrame{typ: TypeResult, id: s.id, payload: marshalResult(ResultOK)})
	}
}

// writeErrLocked returns the error a write on the stream gets, or nil.
func (s *ConnStream) writeErrLocked() error {
	switch {
	case s.err != nil:
		return s.err
	case s.closedByApp, s.finQueued:
		return ErrStreamClosed
	}
	return nil
}

// readErrLocked returns the error a read gets once the buffered data is
// gone: the end of the stream, an abort, or the application's own Close.
func (s *ConnStream) readErrLocked() error {
	switch {
	case s.closedByApp:
		return ErrStreamClosed
	case s.err != nil:
		return s.err
	case s.remoteFin:
		return io.EOF
	}
	return nil
}

// consumedLocked accounts for n received bytes the application took: it
// returns the carrier's credit at once and the stream's once half the window
// is back, so a slow reader slows only its own stream until the carrier's
// window, shared by all streams, runs out.
func (s *ConnStream) consumedLocked(n int) {
	c := s.c
	if !s.remoteFin && s.err == nil {
		s.recvConsumed += int64(n)
		if s.recvConsumed >= int64(c.local.StreamWindow/2) && c.err == nil {
			inc := s.recvConsumed
			s.recvConsumed = 0
			s.recvCredit += inc
			c.queueLocked(outFrame{typ: TypeWindow, id: s.id, payload: marshalWindow(creditIncrement(inc))})
		}
	}
	c.releaseLocked(n)
}

// Read reads received bytes of a TCP stream. It returns io.EOF after the
// peer's FIN once the data is read, and the stream's error after a reset, a
// refusal or the loss of the carrier.
func (s *ConnStream) Read(p []byte) (int, error) {
	if s.kind != StreamTCP {
		return 0, ErrStreamKind
	}
	c := s.c
	for {
		if isClosed(s.rdl.wait()) {
			return 0, os.ErrDeadlineExceeded
		}
		c.mu.Lock()
		if s.recvLen > 0 && !s.closedByApp {
			if len(p) == 0 {
				c.mu.Unlock()
				return 0, nil
			}
			n := 0
			for n < len(p) && len(s.recvQ) > 0 {
				chunk := s.recvQ[0]
				m := copy(p[n:], chunk)
				n += m
				if m == len(chunk) {
					s.recvQ[0] = nil
					s.recvQ = s.recvQ[1:]
				} else {
					s.recvQ[0] = chunk[m:]
				}
			}
			s.recvLen -= n
			if s.recvLen == 0 {
				s.recvQ = nil
			}
			s.consumedLocked(n)
			c.mu.Unlock()
			return n, nil
		}
		if err := s.readErrLocked(); err != nil {
			c.mu.Unlock()
			return 0, err
		}
		if len(p) == 0 {
			c.mu.Unlock()
			return 0, nil
		}
		w := s.waitLocked(&s.readWake)
		c.mu.Unlock()
		select {
		case <-w:
		case <-s.rdl.wait():
			return 0, os.ErrDeadlineExceeded
		}
	}
}

// ReadDatagram returns the next datagram of a UDP stream, which the caller
// owns. It returns io.EOF after the peer's FIN once the queue is empty.
func (s *ConnStream) ReadDatagram() ([]byte, error) {
	if s.kind != StreamUDP {
		return nil, ErrStreamKind
	}
	c := s.c
	for {
		if isClosed(s.rdl.wait()) {
			return nil, os.ErrDeadlineExceeded
		}
		c.mu.Lock()
		if len(s.recvQ) > 0 && !s.closedByApp {
			d := s.recvQ[0]
			s.recvQ[0] = nil
			s.recvQ = s.recvQ[1:]
			s.recvLen -= len(d)
			if len(s.recvQ) == 0 {
				s.recvQ = nil
			}
			s.consumedLocked(len(d))
			c.mu.Unlock()
			return d, nil
		}
		if err := s.readErrLocked(); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		w := s.waitLocked(&s.readWake)
		c.mu.Unlock()
		select {
		case <-w:
		case <-s.rdl.wait():
			return nil, os.ErrDeadlineExceeded
		}
	}
}

// Write queues p for the peer, blocking while the stream's send buffer is
// full, which is while the peer's credit is spent. It returns when all of p
// is queued. Writing to a listener-side stream the application has not
// answered answers it with success.
func (s *ConnStream) Write(p []byte) (int, error) {
	if s.kind != StreamTCP {
		return 0, ErrStreamKind
	}
	c := s.c
	total := 0
	for {
		if isClosed(s.wdl.wait()) {
			return total, os.ErrDeadlineExceeded
		}
		c.mu.Lock()
		if err := s.writeErrLocked(); err != nil {
			c.mu.Unlock()
			return total, err
		}
		if len(p) == 0 {
			c.mu.Unlock()
			return total, nil
		}
		s.ensureAnsweredLocked()
		if space := c.cfg.SendBuffer - s.sendLen; space > 0 {
			n := min(space, len(p))
			s.appendSendLocked(p[:n])
			p = p[n:]
			total += n
			c.markReadyLocked(s)
			if len(p) == 0 {
				c.mu.Unlock()
				return total, nil
			}
		}
		w := s.waitLocked(&s.writeWake)
		c.mu.Unlock()
		select {
		case <-w:
		case <-s.wdl.wait():
			return total, os.ErrDeadlineExceeded
		}
	}
}

// appendSendLocked copies p into the send queue as chunks no larger than the
// peer's frame limit, filling the last chunk first.
func (s *ConnStream) appendSendLocked(p []byte) {
	maxChunk := int(s.c.peer.MaxFrame)
	for len(p) > 0 {
		if n := len(s.sendQ); n > 0 {
			last := s.sendQ[n-1]
			if room := cap(last) - len(last); room > 0 {
				m := min(room, len(p))
				s.sendQ[n-1] = append(last, p[:m]...)
				p = p[m:]
				s.sendLen += m
				continue
			}
		}
		m := min(maxChunk, len(p))
		chunk := make([]byte, 0, min(maxChunk, max(m, 4096)))
		chunk = append(chunk, p[:m]...)
		s.sendQ = append(s.sendQ, chunk)
		s.sendLen += m
		p = p[m:]
	}
}

// WriteDatagram queues one datagram on a UDP stream. It never blocks: when
// the stream's window or send buffer cannot take the datagram it is dropped
// and counted, as a full socket buffer would drop it, and ErrDatagramDropped
// is returned (anixops-protocol.md section 4.8).
func (s *ConnStream) WriteDatagram(p []byte) error {
	if s.kind != StreamUDP {
		return ErrStreamKind
	}
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := s.writeErrLocked(); err != nil {
		return err
	}
	if len(p) == 0 || len(p) > int(c.peer.MaxFrame) {
		return ErrDatagramSize
	}
	s.ensureAnsweredLocked()
	if int64(s.sendLen+len(p)) > min(s.sendCredit, c.sendCredit) || s.sendLen+len(p) > c.cfg.SendBuffer {
		c.n.datagramsDropped.Add(1)
		return ErrDatagramDropped
	}
	s.sendQ = append(s.sendQ, append([]byte(nil), p...))
	s.sendLen += len(p)
	c.markReadyLocked(s)
	return nil
}

// CloseWrite ends this side's direction: the peer reads the queued data and
// then io.EOF, and can keep sending. It is the half-close of the protocol
// (anixops-protocol.md section 4.7).
func (s *ConnStream) CloseWrite() error {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.finQueued && s.err == nil && !s.closedByApp {
		return nil
	}
	if err := s.writeErrLocked(); err != nil {
		return err
	}
	s.ensureAnsweredLocked()
	s.finQueued = true
	c.markReadyLocked(s)
	return nil
}

// Close ends the stream from this side. A stream the peer has not finished is
// reset (ResetCancel, since nobody reads it any more), and data still queued
// to send is dropped: call CloseWrite and read to io.EOF first for a graceful
// end, as a proxy does when both of its copy loops are done. Once the peer
// has finished, the FIN follows the queued data. Unread data is dropped and
// its credit returned. A listener-side stream not yet answered is answered
// with ResultInternal, so no stream is left unanswered.
func (s *ConnStream) Close() error {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.closedByApp {
		return nil
	}
	s.closedByApp = true
	s.wakeAllLocked()
	if s.removed {
		s.discardLocked()
		return nil
	}
	if c.role == RoleAcceptor && !s.answered {
		s.answered = true
		c.queueLocked(outFrame{typ: TypeResult, id: s.id, payload: marshalResult(ResultInternal)})
		c.n.resultFailures[codeIndex(uint16(ResultInternal))].Add(1)
		s.abortLocked(ErrStreamClosed, 0)
		return nil
	}
	if !s.remoteFin {
		s.abortLocked(ErrStreamClosed, ResetCancel)
		return nil
	}
	n := s.recvLen
	s.recvQ, s.recvLen = nil, 0
	if n > 0 {
		c.releaseLocked(n)
	}
	if !s.finQueued {
		s.finQueued = true
		c.markReadyLocked(s)
	}
	return nil
}

// Reset aborts both directions at once with the given reason; the peer's
// reads and writes fail with it. A proxy resets a stream whose client or
// target reset its connection (ResetPeerReset).
func (s *ConnStream) Reset(reason ResetReason) error {
	if reason == 0 {
		reason = ResetCancel
	}
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.removed || s.closedByApp {
		s.closedByApp = true
		s.discardLocked()
		return nil
	}
	s.closedByApp = true
	if c.role == RoleAcceptor && !s.answered {
		// A reset before any RESULT would read as a carrier-level refusal;
		// the answer says what happened.
		s.answered = true
		c.queueLocked(outFrame{typ: TypeResult, id: s.id, payload: marshalResult(ResultInternal)})
		c.n.resultFailures[codeIndex(uint16(ResultInternal))].Add(1)
		s.abortLocked(&StreamResetError{Reason: reason}, 0)
		return nil
	}
	s.abortLocked(&StreamResetError{Reason: reason}, reason)
	return nil
}

// Accept answers a listener-side stream with success. Writing, closing the
// write side or closing the stream answers it implicitly.
func (s *ConnStream) Accept() error {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.role != RoleAcceptor {
		return ErrWrongRole
	}
	if s.answered {
		return ErrAlreadyAnswered
	}
	if s.err != nil {
		return s.err
	}
	s.ensureAnsweredLocked()
	return nil
}

// Reject answers a listener-side stream with a failure and ends it. The
// dialler sees the code with Stream.AwaitResult (and in Read and Write) and
// may retry on another upstream.
func (s *ConnStream) Reject(code ResultCode) error {
	if code == ResultOK {
		return fmt.Errorf("relay: Reject needs a failure code")
	}
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.role != RoleAcceptor {
		return ErrWrongRole
	}
	if s.answered {
		return ErrAlreadyAnswered
	}
	if s.err != nil {
		return s.err
	}
	s.answered = true
	c.queueLocked(outFrame{typ: TypeResult, id: s.id, payload: marshalResult(code)})
	c.n.resultFailures[codeIndex(uint16(code))].Add(1)
	s.closedByApp = true
	s.abortLocked(ErrStreamClosed, 0)
	return nil
}

// AwaitResult waits for the listener's answer to OPEN on a dialler-side
// stream. It returns nil for success, a *ResultError for a refusal, and the
// stream's error if it was reset, refused by the carrier (errors.Is ErrRefused)
// or the carrier ended first.
func (s *ConnStream) AwaitResult(ctx context.Context) error {
	c := s.c
	for {
		c.mu.Lock()
		switch {
		case c.role != RoleDialer:
			c.mu.Unlock()
			return ErrWrongRole
		case s.answered:
			code := s.resultCode
			c.mu.Unlock()
			if code == ResultOK {
				return nil
			}
			return &ResultError{Code: code}
		case s.err != nil:
			err := s.err
			c.mu.Unlock()
			return err
		}
		w := s.waitLocked(&s.resultWake)
		c.mu.Unlock()
		select {
		case <-w:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// SetDeadline sets both the read and the write deadline, as net.Conn does.
func (s *ConnStream) SetDeadline(t time.Time) error {
	s.rdl.set(t)
	s.wdl.set(t)
	return nil
}

// SetReadDeadline sets the deadline of Read and ReadDatagram.
func (s *ConnStream) SetReadDeadline(t time.Time) error {
	s.rdl.set(t)
	return nil
}

// SetWriteDeadline sets the deadline of Write.
func (s *ConnStream) SetWriteDeadline(t time.Time) error {
	s.wdl.set(t)
	return nil
}
