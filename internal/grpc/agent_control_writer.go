package grpc

import (
	"sync"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// agentWriteLimit is how long one write to an Agent stream may block (the
// peer's flow-control window full: the Agent stopped reading) before the
// session ends. Variable so tests can lower it.
var agentWriteLimit = 2 * time.Minute

// errAgentSessionEnded answers writes after the session's handler returned.
var errAgentSessionEnded = status.Error(codes.Canceled, "agent session ended")

// agentWriter is the one goroutine that writes a session's stream
// (backend review ARC-001). Senders still wait for their message to be
// written and get the stream's error, as with a direct Send, but:
//
//   - acknowledgements (heartbeat, report, maintenance) are written before
//     waiting bulk messages (configuration, user pages, alive lists,
//     operations);
//   - a write blocked for agentWriteLimit ends the session, and every
//     waiting sender, the handler's receive loop included, is released at
//     once with that error. The handler then returns, which cancels the
//     stream and unblocks the stuck Send.
//
// Messages of one lane keep their order; operations are bulk, so they stay
// in the order their senders wrote them.
type agentWriter struct {
	stream  agentv1pb.AgentControlService_ControlStreamServer
	urgent  chan agentWrite
	bulk    chan agentWrite
	ended   chan struct{}
	endOnce sync.Once
	err     error
}

type agentWrite struct {
	message *agentv1pb.ControlToAgent
	result  chan error
}

func newAgentWriter(stream agentv1pb.AgentControlService_ControlStreamServer) *agentWriter {
	w := &agentWriter{stream: stream, urgent: make(chan agentWrite), bulk: make(chan agentWrite), ended: make(chan struct{})}
	go w.run()
	return w
}

func (w *agentWriter) run() {
	for {
		var next agentWrite
		select {
		case next = <-w.urgent:
		default:
			select {
			case next = <-w.urgent:
			case next = <-w.bulk:
			case <-w.ended:
				return
			}
		}
		watchdog := time.AfterFunc(agentWriteLimit, func() {
			w.end(status.Errorf(codes.DeadlineExceeded, "agent did not read its stream for %s", agentWriteLimit))
		})
		err := w.stream.Send(next.message)
		watchdog.Stop()
		next.result <- err
		if err != nil {
			w.end(err)
			return
		}
	}
}

// end ends the session with err; the first error wins.
func (w *agentWriter) end(err error) {
	w.endOnce.Do(func() {
		w.err = err
		close(w.ended)
	})
}

// write queues message in its lane and waits until it is written or the
// session ends.
func (w *agentWriter) write(message *agentv1pb.ControlToAgent) error {
	lane := w.bulk
	switch message.Payload.(type) {
	case *agentv1pb.ControlToAgent_HeartbeatAck, *agentv1pb.ControlToAgent_ReportAck, *agentv1pb.ControlToAgent_MaintenanceAck:
		lane = w.urgent
	}
	request := agentWrite{message: message, result: make(chan error, 1)}
	select {
	case lane <- request:
	case <-w.ended:
		return w.err
	}
	select {
	case err := <-request.result:
		return err
	case <-w.ended:
		return w.err
	}
}
