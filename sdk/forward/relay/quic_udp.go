package relay

import (
	"errors"
	"io"
	"os"

	"github.com/quic-go/quic-go"
)

// UDP on a QUIC carrier (anixops-protocol.md section 4.8). An association is a
// UDP stream, whose id names it. Its datagrams ride QUIC DATAGRAM frames
// holding the stream id (a QUIC variable-length integer) and the payload:
// unreliable and unordered, as UDP is. Two things send a datagram on the
// association's stream instead, in a DATAGRAM frame of section 4.2, which is
// reliable and ordered:
//
//   - the datagram is larger than the connection's current maximum DATAGRAM
//     size (QUIC reports it as quic.DatagramTooLargeError): it is sent on the
//     stream rather than dropped, and counted (Stats.DatagramsOversize, the
//     document's udp_oversize_fallback);
//   - DATAGRAM frames cannot be used: one end did not enable them
//     (QUICConfig.DisableDatagrams, or a peer without support), or, from a
//     dialler, the stream's RESULT has not arrived yet. A QUIC datagram can
//     overtake the OPEN of its stream and would be dropped by a listener that
//     does not know the stream, so until the listener has answered, the
//     dialler's first datagrams follow the OPEN on the stream.
//
// WriteDatagram never blocks: QUIC's own send queue blocks when full, so
// datagrams go through a bounded queue and a sender of the carrier, and are
// dropped and counted when it is full. A datagram that arrives for a stream
// that is not an open UDP association here (usually one that finished while it
// was in flight) is dropped and counted too, as is one that finds the
// association's receive queue full.

// outDatagram is a datagram waiting for the carrier's sender: the content of a
// QUIC DATAGRAM frame, whose payload starts at off.
type outDatagram struct {
	s     *QUICStream
	frame []byte
	off   int
}

// WriteDatagram queues one datagram on a UDP stream. It never blocks: when the
// datagram cannot be taken it is dropped and counted, as a full socket buffer
// would drop it, and ErrDatagramDropped is returned (anixops-protocol.md
// section 4.8). An empty datagram, or one larger than the peer's MaxFrame, is
// ErrDatagramSize.
func (s *QUICStream) WriteDatagram(p []byte) error {
	if s.kind != StreamUDP {
		return ErrStreamKind
	}
	c := s.c
	c.mu.Lock()
	err := s.writeErrLocked()
	if err == nil && (len(p) == 0 || len(p) > int(c.peer.MaxFrame)) {
		err = ErrDatagramSize
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	// A listener-side stream the application has not answered answers itself
	// first, as for TCP, and the RESULT precedes everything else.
	if c.role == RoleAcceptor && !s.resultSent.Load() {
		s.wmu.Lock()
		err = s.sendResult()
		s.wmu.Unlock()
		if err != nil {
			return s.ioFailed(err)
		}
	}
	c.mu.Lock()
	native := c.datagrams && s.answered && s.err == nil
	c.mu.Unlock()
	if !native {
		return s.queueOnStream(p, false)
	}
	return c.queueDatagram(s, p)
}

// queueDatagram hands a datagram to the carrier's sender.
func (c *QUICCarrier) queueDatagram(s *QUICStream, p []byte) error {
	frame, err := AppendQUICDatagram(make([]byte, 0, quicDatagramHeaderLen(s.id)+len(p)), s.id, p)
	if err != nil {
		return ErrDatagramSize
	}
	n := int64(len(frame))
	// The stream's pending count goes up before the datagram is queued, under
	// the lock, so a CloseWrite that follows waits for it.
	c.mu.Lock()
	if err := s.writeErrLocked(); err != nil || s.finQueued {
		c.mu.Unlock()
		return ErrStreamClosed
	}
	s.dgPending++
	c.mu.Unlock()
	if c.dgBytes.Add(n) <= dgQueueBytes {
		select {
		case c.dgq <- outDatagram{s: s, frame: frame, off: len(frame) - len(p)}:
			return nil
		default:
		}
	}
	c.dgBytes.Add(-n)
	c.n.datagramsDropped.Add(1)
	c.mu.Lock()
	s.datagramDoneLocked()
	c.mu.Unlock()
	return ErrDatagramDropped
}

// datagramDoneLocked notes that a datagram handed to the carrier's sender has
// been dealt with, sent or dropped or moved to the stream's queue; a FIN that
// waited for it can go.
func (s *QUICStream) datagramDoneLocked() {
	s.dgPending--
	if s.dgPending == 0 && s.finQueued {
		s.startWriterLocked()
	}
}

// datagramSendLoop sends the queued datagrams in QUIC DATAGRAM frames; one
// that is too large for the connection goes to its association's stream.
func (c *QUICCarrier) datagramSendLoop() {
	defer c.wg.Done()
	for {
		select {
		case d := <-c.dgq:
			c.dgBytes.Add(-int64(len(d.frame)))
			c.sendDatagram(d)
		case <-c.dead:
			return
		}
	}
}

// sendDatagram sends one queued datagram; if it is too large for the connection
// it goes to its association's stream.
func (c *QUICCarrier) sendDatagram(d outDatagram) {
	c.mu.Lock()
	live := d.s.err == nil && !d.s.removed
	c.mu.Unlock()
	var err error
	if live {
		err = c.qc.SendDatagram(d.frame)
	}
	var tooLarge *quic.DatagramTooLargeError
	switch {
	case !live:
		c.n.datagramsDropped.Add(1)
	case err == nil:
		c.n.datagramsSent.Add(1)
		c.n.bytesSent.Add(uint64(len(d.frame[d.off:])))
	case errors.As(err, &tooLarge):
		c.n.datagramsOversize.Add(1)
		_ = d.s.queueOnStream(d.frame[d.off:], true)
	default: // the connection is going away
		c.n.datagramsDropped.Add(1)
	}
	c.mu.Lock()
	d.s.datagramDoneLocked()
	c.mu.Unlock()
}

// datagramRecvLoop delivers the QUIC datagrams the peer sends to their
// associations.
func (c *QUICCarrier) datagramRecvLoop() {
	defer c.wg.Done()
	for {
		b, err := c.qc.ReceiveDatagram(c.qc.Context()) // returns when the connection ends
		if err != nil {
			return
		}
		c.onDatagram(b)
	}
}

// onDatagram routes one received QUIC datagram. A malformed one is the peer's
// mistake and draws on its budget of answers; one for no open association is
// only dropped, since it may simply have outlived its stream.
func (c *QUICCarrier) onDatagram(b []byte) {
	id, payload, err := ParseQUICDatagram(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	if err != nil {
		c.n.datagramsRecvDrop.Add(1)
		c.chargeLocked()
		return
	}
	s := c.streams[quic.StreamID(id)] // #nosec G115 -- ParseQUICDatagram bounds the id by 2^62-1
	if s == nil || s.kind != StreamUDP || s.removed || s.remoteFin || s.closedByApp ||
		(s.recvLen > 0 && s.recvLen+len(payload) > int(c.local.StreamWindow)) {
		c.n.datagramsRecvDrop.Add(1)
		return
	}
	c.n.datagramsRecv.Add(1)
	c.n.bytesRecv.Add(uint64(len(payload)))
	s.recvQ = append(s.recvQ, payload)
	s.recvLen += len(payload)
	s.wakeLocked(&s.readWake)
}

// ReadDatagram returns the next datagram of a UDP stream, which the caller
// owns. It returns io.EOF after the peer's FIN once the queue is empty.
func (s *QUICStream) ReadDatagram() ([]byte, error) {
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
			s.wakeLocked(&s.spaceWake)
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

// queueOnStream queues a datagram for the association's stream, where it
// travels in a DATAGRAM frame. It is dropped, and counted, when the stream's
// send buffer is full. fromSender says the datagram comes from the carrier's
// sender, because it did not fit a QUIC DATAGRAM frame (already counted).
func (s *QUICStream) queueOnStream(p []byte, fromSender bool) error {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := s.writeErrLocked(); err != nil {
		return err
	}
	// A datagram the application wrote before CloseWrite still goes ahead of
	// the FIN, even when it reaches the stream by way of the sender.
	if s.finQueued && !fromSender {
		return ErrStreamClosed
	}
	if s.outLen+len(p) > c.cfg.SendBuffer {
		c.n.datagramsDropped.Add(1)
		return ErrDatagramDropped
	}
	s.outQ = append(s.outQ, append([]byte(nil), p...))
	s.outLen += len(p)
	s.startWriterLocked()
	return nil
}

// startWriterLocked makes sure a goroutine is writing the stream's queue.
func (s *QUICStream) startWriterLocked() {
	if s.outRunning {
		return
	}
	s.outRunning = true
	if !s.c.spawnLocked(s.streamWriter) {
		s.outRunning = false
	}
}

// streamWriter writes the queued datagrams to the stream, one DATAGRAM frame
// each, then the FIN if CloseWrite asked for it.
func (s *QUICStream) streamWriter() {
	c := s.c
	for {
		c.mu.Lock()
		// Nothing to do: the queue is empty and no FIN is due, or the FIN waits
		// for datagrams still with the carrier's sender, which wakes the writer.
		if s.err != nil || s.removed || (len(s.outQ) == 0 && (!s.finQueued || s.dgPending > 0)) {
			s.outRunning = false
			c.mu.Unlock()
			return
		}
		if len(s.outQ) == 0 { // the queue is written: the FIN
			s.finQueued = false
			c.mu.Unlock()
			s.wmu.Lock()
			err := s.sendResult()
			if err == nil {
				err = s.st.Close()
			}
			s.wmu.Unlock()
			c.mu.Lock()
			if err != nil {
				_ = s.failedLocked(err)
			} else {
				s.writeDone = true
				s.finishCheckLocked()
			}
			s.outRunning = false
			c.mu.Unlock()
			return
		}
		p := s.outQ[0]
		s.outQ[0] = nil
		s.outQ = s.outQ[1:]
		s.outLen -= len(p)
		c.mu.Unlock()

		s.wmu.Lock()
		err := s.sendResult()
		if err == nil {
			_, err = s.st.Write(quicFrameBytes(TypeDatagram, p))
		}
		s.wmu.Unlock()
		if err != nil {
			c.mu.Lock()
			_ = s.failedLocked(err)
			s.outRunning = false
			c.mu.Unlock()
			return
		}
		c.n.datagramsSent.Add(1)
		c.n.datagramsOnStream.Add(1)
		c.n.bytesSent.Add(uint64(len(p)))
	}
}

// udpRead reads the DATAGRAM frames the peer sends on a UDP stream (the ones
// that did not ride QUIC datagrams) into the stream's receive queue. It
// blocks while the queue is full, which is QUIC's backpressure on the sender.
func (c *QUICCarrier) udpRead(s *QUICStream) {
	for {
		f, err := readQUICFrame(s.st, int(c.local.MaxFrame))
		c.mu.Lock()
		if err != nil {
			s.udpReadEndedLocked(err)
			c.mu.Unlock()
			return
		}
		if f.Type != TypeDatagram || len(f.Payload) == 0 {
			// Anything but a datagram on a UDP stream is the stream's end.
			s.abortLocked(&StreamResetError{Reason: ResetProtocolError}, ResetProtocolError)
			c.chargeLocked()
			c.mu.Unlock()
			return
		}
		c.n.framesRecv.Add(1)
		for s.err == nil && !s.removed && s.recvLen > 0 && s.recvLen+len(f.Payload) > int(c.local.StreamWindow) {
			w := s.waitLocked(&s.spaceWake)
			c.mu.Unlock()
			<-w
			c.mu.Lock()
		}
		if s.err != nil || s.removed {
			c.mu.Unlock()
			return
		}
		c.n.datagramsRecv.Add(1)
		c.n.bytesRecv.Add(uint64(len(f.Payload)))
		s.recvQ = append(s.recvQ, f.Payload)
		s.recvLen += len(f.Payload)
		s.wakeLocked(&s.readWake)
		c.mu.Unlock()
	}
}

// udpReadEndedLocked handles the end of a UDP stream's receive direction: the
// peer's FIN, a reset, a frame that breaks the mapping, or the connection.
func (s *QUICStream) udpReadEndedLocked(err error) {
	c := s.c
	if s.removed || s.err != nil {
		return
	}
	switch {
	case errors.Is(err, io.EOF):
		s.remoteFin, s.readDone = true, true
		s.wakeLocked(&s.readWake)
		s.finishCheckLocked()
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, ErrQUICFrame), errors.Is(err, ErrFrameTooLarge):
		s.abortLocked(&StreamResetError{Reason: ResetProtocolError}, ResetProtocolError)
		c.chargeLocked()
	default:
		_ = s.failedLocked(err)
	}
}
