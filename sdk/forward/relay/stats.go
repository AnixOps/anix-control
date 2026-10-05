package relay

import (
	"sync/atomic"
	"time"
)

// numCodes sizes the per-reason counter arrays; a reason or code at or
// beyond it counts in the last slot.
const numCodes = 16

func codeIndex(v uint16) int { return min(int(v), numCodes-1) }

// Stats are the counters of one carrier since it was created. The labels
// are bounded (reason codes), never per client, as anixops-protocol.md
// section 7.3 requires.
type Stats struct {
	StreamsOpened   uint64 // OPENs sent (dialler)
	StreamsAccepted uint64 // OPENs accepted into the accept queue (listener)
	StreamsRefused  uint64 // OPENs refused by the carrier: limit, queue, draining, bad parameters
	StreamsActive   int    // streams not finished yet

	// ResultFailures counts RESULTs other than success, by ResultCode: sent
	// by a listener, received by a dialler.
	ResultFailures [numCodes]uint64
	ResetsSent     [numCodes]uint64 // by ResetReason
	ResetsReceived [numCodes]uint64 // by ResetReason

	FramesSent    uint64
	FramesRecv    uint64
	BytesSent     uint64 // DATA and DATAGRAM payload bytes
	BytesRecv     uint64
	DatagramsSent uint64
	DatagramsRecv uint64
	// DatagramsDropped counts datagrams WriteDatagram dropped for want of
	// credit (anixops-protocol.md section 4.8).
	DatagramsDropped uint64

	// StreamStalls and CarrierStalls count the times a stream with data to
	// send had to wait for stream or carrier credit.
	StreamStalls  uint64
	CarrierStalls uint64

	PingsSent        uint64
	KeepaliveTimeout uint64 // carriers closed for the idle timeout (0 or 1)
	// RTT is the latest PING round trip, zero before the first.
	RTT time.Duration
}

type counters struct {
	streamsOpened, streamsAccepted, streamsRefused            atomic.Uint64
	resultFailures, resetsSent, resetsRecv                    [numCodes]atomic.Uint64
	framesSent, framesRecv, bytesSent, bytesRecv              atomic.Uint64
	datagramsSent, datagramsRecv, datagramsDropped            atomic.Uint64
	streamStalls, carrierStalls, pingsSent, keepaliveTimeouts atomic.Uint64
	rtt                                                       atomic.Int64
}

func (n *counters) snapshot(active int) Stats {
	s := Stats{
		StreamsOpened:    n.streamsOpened.Load(),
		StreamsAccepted:  n.streamsAccepted.Load(),
		StreamsRefused:   n.streamsRefused.Load(),
		StreamsActive:    active,
		FramesSent:       n.framesSent.Load(),
		FramesRecv:       n.framesRecv.Load(),
		BytesSent:        n.bytesSent.Load(),
		BytesRecv:        n.bytesRecv.Load(),
		DatagramsSent:    n.datagramsSent.Load(),
		DatagramsRecv:    n.datagramsRecv.Load(),
		DatagramsDropped: n.datagramsDropped.Load(),
		StreamStalls:     n.streamStalls.Load(),
		CarrierStalls:    n.carrierStalls.Load(),
		PingsSent:        n.pingsSent.Load(),
		KeepaliveTimeout: n.keepaliveTimeouts.Load(),
		RTT:              time.Duration(n.rtt.Load()),
	}
	for i := range numCodes {
		s.ResultFailures[i] = n.resultFailures[i].Load()
		s.ResetsSent[i] = n.resetsSent[i].Load()
		s.ResetsReceived[i] = n.resetsRecv[i].Load()
	}
	return s
}
