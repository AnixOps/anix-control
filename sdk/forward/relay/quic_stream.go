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
)

// A QUICStream is one client connection (TCP) or one client UDP association
// inside a QUICCarrier: a native QUIC bidirectional stream that begins with
// OPEN (dialler) and RESULT (listener). A TCP stream is a net.Conn plus
// CloseWrite and carries raw bytes after that; the half-close is QUIC's FIN. A
// UDP stream carries its datagrams in QUIC DATAGRAM frames or, for the ones
// that cannot, in DATAGRAM frames on the stream (quic_udp.go).
//
// The stream's errors are the TCP carrier's: *ResultError for a refusal by the
// listening hop, *StreamResetError for a reset (QUIC's RESET_STREAM or
// STOP_SENDING, whose error code is the ResetReason; errors.Is ErrRefused for
// a carrier-level refusal), the carrier's error when the carrier ended.
type QUICStream struct {
	c      *QUICCarrier
	st     *quic.Stream
	id     uint64
	kind   StreamKind
	params OpenParams

	// wmu serialises what is written to st; resultSent is set under it.
	wmu        sync.Mutex
	resultSent atomic.Bool

	// Everything below is guarded by c.mu.

	answered    bool // RESULT decided (acceptor) or received (dialler)
	resultCode  ResultCode
	awaiting    bool // dialler: OPEN sent, RESULT not yet received
	openedAt    time.Time
	err         error // set when the stream was aborted or refused
	closedByApp bool
	readDone    bool // the peer's direction ended, or the stream was aborted
	writeDone   bool // this end's direction ended, or the stream was aborted
	removed     bool // finished: no longer counted or looked up by the carrier
	settled     bool // dialler: the RESULT was read or the stream failed first
	settledCh   chan struct{}

	// UDP association: datagrams received and not yet read, datagrams waiting
	// to be written to the stream, and the FIN to follow them.
	recvQ      [][]byte
	recvLen    int
	remoteFin  bool
	outQ       [][]byte
	outLen     int
	outRunning bool
	finQueued  bool
	dgPending  int // datagrams handed to the carrier's sender and not yet dealt with

	readWake, spaceWake chan struct{}
	rdl, wdl            deadline
	rdlTime             time.Time // the read deadline, as set, for the QUIC stream
}

var _ Stream = (*QUICStream)(nil)

// newStreamLocked registers a stream of the carrier. An acceptor's stream has
// nothing to wait for; a dialler's is settled when its RESULT has been read.
func (c *QUICCarrier) newStreamLocked(st *quic.Stream, p OpenParams) *QUICStream {
	s := &QUICStream{
		c:         c,
		st:        st,
		id:        uint64(st.StreamID()), // #nosec G115 -- stream ids are not negative
		kind:      p.Kind,
		params:    p,
		settledCh: make(chan struct{}),
		rdl:       newDeadline(),
		wdl:       newDeadline(),
	}
	if c.role == RoleAcceptor {
		s.settled = true
		close(s.settledCh)
	}
	c.streams[st.StreamID()] = s
	c.active++
	return s
}

// ID returns the QUIC stream id: unique within the carrier, and what names a
// UDP association in a DATAGRAM frame.
func (s *QUICStream) ID() uint64 { return s.id }

// Kind returns whether the stream carries a TCP connection or UDP datagrams.
func (s *QUICStream) Kind() StreamKind { return s.kind }

// Params returns the OPEN parameters: the ones the dialler sent, or the ones
// this side sent.
func (s *QUICStream) Params() OpenParams { return s.params }

// Carrier returns the carrier the stream runs on.
func (s *QUICStream) Carrier() Carrier { return s.c }

// LocalAddr and RemoteAddr are the carrier connection's addresses.
func (s *QUICStream) LocalAddr() net.Addr  { return s.c.qc.LocalAddr() }
func (s *QUICStream) RemoteAddr() net.Addr { return s.c.qc.RemoteAddr() }

// Answered reports whether the listener's RESULT reached a dialler-side
// stream. A stream that failed without one never started as far as the
// dialler knows: nothing came back, and the caller may retry it on another
// carrier or upstream with the bytes it kept.
func (s *QUICStream) Answered() bool {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.answered
}

// settleLocked marks a dialler-side stream settled: its RESULT was read, or it
// failed before one came.
func (s *QUICStream) settleLocked() {
	if !s.settled {
		s.settled = true
		close(s.settledCh)
	}
}

func (s *QUICStream) wakeLocked(ch *chan struct{}) {
	if *ch != nil {
		close(*ch)
		*ch = nil
	}
}

func (s *QUICStream) waitLocked(ch *chan struct{}) <-chan struct{} {
	if *ch == nil {
		*ch = make(chan struct{})
	}
	return *ch
}

func (s *QUICStream) wakeAllLocked() {
	s.wakeLocked(&s.readWake)
	s.wakeLocked(&s.spaceWake)
}

// finishLocked takes the stream out of the carrier: it no longer counts
// against the stream limit, and the carrier's drain may complete.
func (s *QUICStream) finishLocked() {
	if s.removed {
		return
	}
	s.removed = true
	c := s.c
	delete(c.streams, s.st.StreamID())
	c.active--
	s.wakeAllLocked()
	c.drainCheckLocked()
}

// finishCheckLocked finishes the stream once both directions have ended.
func (s *QUICStream) finishCheckLocked() {
	if s.readDone && s.writeDone {
		s.finishLocked()
	}
}

// abortLocked ends both directions at once: err is what the stream's callers
// see (the first error wins), and both directions of the QUIC stream are
// cancelled, with reset as the error code when it is nonzero (which counts as
// a reset sent, and tells the peer why) and as a cancellation otherwise. QUIC
// gives a stream's slot back (MAX_STREAMS) only when both of its directions are
// complete, which for one that is not finished by FINs read and sent takes a
// cancellation of each direction, whoever ended it: a stream the peer reset
// still has a read side that nobody will read.
func (s *QUICStream) abortLocked(err error, reset ResetReason) {
	if s.err == nil {
		s.err = err
	}
	if !s.removed {
		code := reset
		if code == 0 {
			code = ResetCancel
		} else {
			s.c.n.resetsSent[codeIndex(uint16(reset))].Add(1)
		}
		s.st.CancelWrite(quic.StreamErrorCode(code))
		s.st.CancelRead(quic.StreamErrorCode(code))
	}
	s.endLocked()
}

// abortQuietLocked ends the stream for its callers like abortLocked but leaves
// the QUIC stream alone: when the carrier ends the connection's close tells the
// peer (a reset sent first would race it and show up as the stream's error), and
// a listener-side stream answered with a failure still has its RESULT to write,
// which endAnswered does, followed by FIN and the cancellation of the read side.
func (s *QUICStream) abortQuietLocked(err error) {
	if s.err == nil {
		s.err = err
	}
	s.endLocked()
}

// endLocked is what both aborts share: the stream's queues are dropped and its
// waiters released, and the carrier stops counting it.
func (s *QUICStream) endLocked() {
	s.readDone, s.writeDone = true, true
	s.recvQ, s.recvLen = nil, 0
	s.outQ, s.outLen = nil, 0
	s.settleLocked()
	s.finishLocked()
	s.wakeAllLocked()
}

// failedLocked turns an error from the stream's I/O into the error its
// caller sees, ending the stream when the error ended it. A QUIC stream error
// (a reset or a stop from either end) ends that stream; an error of the
// connection ends the carrier, and with it the stream; any other error is one
// of this stream alone that QUIC has no type for (a Close on a send side that
// was cancelled, say) and ends only the stream: a peer must never be able to
// end the carrier by what it does to a stream.
func (s *QUICStream) failedLocked(err error) error {
	c := s.c
	switch {
	case s.err != nil:
		return s.err
	case s.closedByApp:
		return ErrStreamClosed
	case errors.Is(err, os.ErrDeadlineExceeded):
		return os.ErrDeadlineExceeded
	}
	var se *quic.StreamError
	switch {
	case errors.As(err, &se):
		e := &StreamResetError{Reason: resetReasonOf(se.ErrorCode), Remote: se.Remote}
		if se.Remote {
			c.n.resetsRecv[codeIndex(uint16(e.Reason))].Add(1)
		}
		s.abortLocked(e, 0)
		return e
	case isConnectionError(err) || c.qc.Context().Err() != nil:
		return c.connErrLocked(err)
	}
	e := &StreamResetError{Reason: ResetInternal}
	s.abortLocked(e, ResetInternal)
	return e
}

// ioFailed is failedLocked for a failure the dialler's I/O reported on a
// stream whose RESULT may still be on its way: the listener's refusal comes
// as RESULT and a stop, so the RESULT is waited for to say which it was.
func (s *QUICStream) ioFailed(err error) error {
	c := s.c
	c.mu.Lock()
	if s.err == nil && !s.settled && !errors.Is(err, os.ErrDeadlineExceeded) {
		ch := s.settledCh
		c.mu.Unlock()
		select {
		case <-ch:
		case <-time.After(c.cfg.HandshakeTimeout):
		}
		c.mu.Lock()
	}
	defer c.mu.Unlock()
	return s.failedLocked(err)
}

func (s *QUICStream) writeErrLocked() error {
	switch {
	case s.err != nil:
		return s.err
	case s.closedByApp, s.writeDone:
		return ErrStreamClosed
	}
	return nil
}

func (s *QUICStream) readErrLocked() error {
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

// sendResult writes the RESULT of an answered listener-side stream, once, ahead
// of anything else. The caller holds s.wmu.
func (s *QUICStream) sendResult() error {
	c := s.c
	c.mu.Lock()
	need := c.role == RoleAcceptor && !s.resultSent.Load()
	if need && !s.answered {
		s.answered, s.resultCode = true, ResultOK
	}
	code := s.resultCode
	c.mu.Unlock()
	if !need {
		return nil
	}
	s.resultSent.Store(true)
	if _, err := s.st.Write(quicFrameBytes(TypeResult, marshalResult(code))); err != nil {
		return err
	}
	c.n.framesSent.Add(1)
	return nil
}

// awaitSettled waits, for a dialler-side stream, until its RESULT has been
// read or it failed, or the deadline passes.
func (s *QUICStream) awaitSettled(dl <-chan struct{}) error {
	select {
	case <-s.settledCh:
		return nil
	default:
	}
	select {
	case <-s.settledCh:
		return nil
	case <-dl:
		return os.ErrDeadlineExceeded
	}
}

// Read reads received bytes of a TCP stream. It returns io.EOF after the
// peer's FIN once the data is read, and the stream's error after a reset, a
// refusal or the loss of the carrier. On a dialler's stream it first waits for
// the listener's RESULT, which precedes everything the listener sends.
func (s *QUICStream) Read(p []byte) (int, error) {
	if s.kind != StreamTCP {
		return 0, ErrStreamKind
	}
	if isClosed(s.rdl.wait()) {
		return 0, os.ErrDeadlineExceeded
	}
	if err := s.awaitSettled(s.rdl.wait()); err != nil {
		return 0, err
	}
	c := s.c
	c.mu.Lock()
	err := s.err
	if err == nil && s.closedByApp {
		err = ErrStreamClosed
	}
	dl := s.rdlTime
	c.mu.Unlock()
	if err != nil {
		return 0, err
	}
	// The deadline reaches the QUIC stream only now: until the RESULT was
	// read the carrier itself reads it, and must not be cut short.
	_ = s.st.SetReadDeadline(dl)
	n, err := s.st.Read(p)
	if n > 0 {
		c.n.bytesRecv.Add(uint64(n))
	}
	if err == nil {
		return n, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if errors.Is(err, io.EOF) {
		s.readDone = true
		s.finishCheckLocked()
		return n, io.EOF
	}
	return n, s.failedLocked(err)
}

// Write sends p to the peer, blocking while QUIC's flow control holds the
// stream back. Writing to a listener-side stream the application has not
// answered answers it with success first.
func (s *QUICStream) Write(p []byte) (int, error) {
	if s.kind != StreamTCP {
		return 0, ErrStreamKind
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if isClosed(s.wdl.wait()) {
		return 0, os.ErrDeadlineExceeded
	}
	c := s.c
	c.mu.Lock()
	err := s.writeErrLocked()
	c.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := s.sendResult(); err != nil {
		return 0, s.ioFailed(err)
	}
	n, err := s.st.Write(p)
	c.n.bytesSent.Add(uint64(n)) // #nosec G115 -- n is a count of bytes written, never negative
	if err != nil {
		return n, s.ioFailed(err)
	}
	return n, nil
}

// CloseWrite ends this side's direction: the peer reads the queued data and
// then io.EOF, and can keep sending. It is the half-close of the protocol
// (anixops-protocol.md section 4.7).
func (s *QUICStream) CloseWrite() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	c := s.c
	c.mu.Lock()
	if s.writeDone && s.err == nil && !s.closedByApp {
		c.mu.Unlock()
		return nil
	}
	if err := s.writeErrLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if s.kind == StreamUDP {
		defer c.mu.Unlock()
		s.finQueued = true
		s.startWriterLocked()
		return nil
	}
	c.mu.Unlock()
	if err := s.sendResult(); err != nil {
		return s.ioFailed(err)
	}
	if err := s.st.Close(); err != nil {
		return s.ioFailed(err)
	}
	c.mu.Lock()
	s.writeDone = true
	s.finishCheckLocked()
	c.mu.Unlock()
	return nil
}

// Close ends the stream from this side. A stream the peer has not finished is
// reset (ResetCancel, since nobody reads it any more), and data still queued
// to send is dropped: call CloseWrite and read to io.EOF first for a graceful
// end, as a proxy does when both of its copy loops are done. Once the peer has
// finished (the reader has seen io.EOF), the FIN follows what was written.
// Unread data is dropped. A listener-side stream not yet answered is answered
// with ResultInternal, so no stream is left unanswered.
func (s *QUICStream) Close() error {
	c := s.c
	c.mu.Lock()
	if s.closedByApp {
		c.mu.Unlock()
		return nil
	}
	s.closedByApp = true
	s.wakeAllLocked()
	switch {
	case s.removed:
		c.mu.Unlock()
		return nil
	case c.role == RoleAcceptor && !s.answered:
		s.answered, s.resultCode = true, ResultInternal
		c.n.resultFailures[codeIndex(uint16(ResultInternal))].Add(1)
		s.abortQuietLocked(ErrStreamClosed)
		c.mu.Unlock()
		s.endAnswered()
	case !s.readDone:
		s.abortLocked(ErrStreamClosed, ResetCancel)
		c.mu.Unlock()
	default:
		// The peer finished: the FIN follows what was written. A write still
		// in progress (a concurrent Write) cannot be waited for without
		// blocking, so that stream is reset instead.
		fin := !s.writeDone
		s.writeDone = true
		s.recvQ, s.recvLen = nil, 0
		s.finishCheckLocked()
		c.mu.Unlock()
		if fin {
			if s.wmu.TryLock() {
				_ = s.st.Close()
				s.wmu.Unlock()
			} else {
				s.st.CancelWrite(quic.StreamErrorCode(ResetCancel))
			}
		}
	}
	return nil
}

// endAnswered finishes a listener-side stream that was answered with a failure
// in Reject, Close or Reset: the RESULT, then FIN, and a stop to the dialler's
// side, which has nothing more to say. Failures to write are the carrier's
// problem, which ends the stream anyway.
func (s *QUICStream) endAnswered() {
	s.wmu.Lock()
	_ = s.sendResult()
	_ = s.st.Close()
	s.wmu.Unlock()
	s.st.CancelRead(quic.StreamErrorCode(ResetCancel))
}

// Reset aborts both directions at once with the given reason; the peer's
// reads and writes fail with it. A proxy resets a stream whose client or
// target reset its connection (ResetPeerReset).
func (s *QUICStream) Reset(reason ResetReason) error {
	if reason == 0 {
		reason = ResetCancel
	}
	c := s.c
	c.mu.Lock()
	if s.removed || s.closedByApp {
		s.closedByApp = true
		c.mu.Unlock()
		return nil
	}
	s.closedByApp = true
	if c.role == RoleAcceptor && !s.answered {
		// A reset before any RESULT would read as a carrier-level refusal;
		// the answer says what happened.
		s.answered, s.resultCode = true, ResultInternal
		c.n.resultFailures[codeIndex(uint16(ResultInternal))].Add(1)
		s.abortQuietLocked(&StreamResetError{Reason: reason})
		c.mu.Unlock()
		s.endAnswered()
		return nil
	}
	s.abortLocked(&StreamResetError{Reason: reason}, reason)
	c.mu.Unlock()
	return nil
}

// Accept answers a listener-side stream with success. Writing, closing the
// write side or closing the stream answers it implicitly.
func (s *QUICStream) Accept() error {
	c := s.c
	c.mu.Lock()
	switch {
	case c.role != RoleAcceptor:
		c.mu.Unlock()
		return ErrWrongRole
	case s.answered:
		c.mu.Unlock()
		return ErrAlreadyAnswered
	case s.err != nil:
		err := s.err
		c.mu.Unlock()
		return err
	}
	s.answered, s.resultCode = true, ResultOK
	c.mu.Unlock()
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.sendResult(); err != nil {
		return s.ioFailed(err)
	}
	return nil
}

// Reject answers a listener-side stream with a failure and ends it. The
// dialler sees the code with Stream.AwaitResult (and in Read and Write) and
// may retry on another upstream.
func (s *QUICStream) Reject(code ResultCode) error {
	if code == ResultOK {
		return fmt.Errorf("relay: Reject needs a failure code")
	}
	c := s.c
	c.mu.Lock()
	switch {
	case c.role != RoleAcceptor:
		c.mu.Unlock()
		return ErrWrongRole
	case s.answered:
		c.mu.Unlock()
		return ErrAlreadyAnswered
	case s.err != nil:
		err := s.err
		c.mu.Unlock()
		return err
	}
	s.answered, s.resultCode = true, code
	c.n.resultFailures[codeIndex(uint16(code))].Add(1)
	s.closedByApp = true
	s.abortQuietLocked(ErrStreamClosed)
	c.mu.Unlock()
	s.endAnswered()
	return nil
}

// AwaitResult waits for the listener's answer to OPEN on a dialler-side
// stream. It returns nil for success, a *ResultError for a refusal, and the
// stream's error if it was reset, refused by the carrier (errors.Is ErrRefused)
// or the carrier ended first.
func (s *QUICStream) AwaitResult(ctx context.Context) error {
	c := s.c
	if c.role != RoleDialer {
		return ErrWrongRole
	}
	select {
	case <-s.settledCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case s.answered && s.resultCode == ResultOK:
		return nil
	case s.answered:
		return &ResultError{Code: s.resultCode}
	case s.err != nil:
		return s.err
	}
	return ErrStreamClosed
}

// SetDeadline sets both the read and the write deadline, as net.Conn does.
func (s *QUICStream) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	return s.SetWriteDeadline(t)
}

// SetReadDeadline sets the deadline of Read and ReadDatagram.
func (s *QUICStream) SetReadDeadline(t time.Time) error {
	s.rdl.set(t)
	if s.kind != StreamTCP { // a UDP stream's QUIC stream is read by the carrier
		return nil
	}
	c := s.c
	c.mu.Lock()
	s.rdlTime = t
	settled := s.settled
	c.mu.Unlock()
	if settled { // a Read in progress picks the new deadline up
		return s.st.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets the deadline of Write.
func (s *QUICStream) SetWriteDeadline(t time.Time) error {
	s.wdl.set(t)
	if s.kind == StreamTCP {
		return s.st.SetWriteDeadline(t)
	}
	return nil
}
