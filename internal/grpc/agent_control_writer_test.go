package grpc

import (
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gatedControlStream records sends; each Send first waits for a token on
// gate, so a test holds the writer inside a Send (the Agent not reading).
type gatedControlStream struct {
	recordingControlStream
	entered chan struct{}
	gate    chan struct{}
}

func (s *gatedControlStream) Send(message *agentv1pb.ControlToAgent) error {
	s.entered <- struct{}{}
	<-s.gate
	return s.recordingControlStream.Send(message)
}

func bulkMessage(id string) *agentv1pb.ControlToAgent {
	return &agentv1pb.ControlToAgent{RequestId: id, Payload: &agentv1pb.ControlToAgent_AliveList{AliveList: &agentv1pb.AliveList{}}}
}

func ackMessage(id string) *agentv1pb.ControlToAgent {
	return &agentv1pb.ControlToAgent{RequestId: id, Payload: &agentv1pb.ControlToAgent_HeartbeatAck{HeartbeatAck: &agentv1pb.HeartbeatAck{}}}
}

// ARC-001: an acknowledgement waiting with bulk messages is written first.
func TestAgentWriterWritesAcknowledgementsBeforeWaitingBulk(t *testing.T) {
	stream := &gatedControlStream{entered: make(chan struct{}, 8), gate: make(chan struct{})}
	writer := newAgentWriter(stream)
	defer writer.end(errAgentSessionEnded)

	var wg sync.WaitGroup
	write := func(message *agentv1pb.ControlToAgent) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, writer.write(message))
		}()
	}
	write(bulkMessage("page-1"))
	<-stream.entered // the writer is inside page-1's Send
	write(bulkMessage("page-2"))
	write(ackMessage("ack"))
	// Both are queued once neither lane takes more (the lanes are
	// unbuffered, so this waits for the senders to block).
	time.Sleep(50 * time.Millisecond)
	close(stream.gate)
	wg.Wait()

	sent := stream.messages()
	require.Len(t, sent, 3)
	assert.Equal(t, []string{"page-1", "ack", "page-2"}, []string{sent[0].RequestId, sent[1].RequestId, sent[2].RequestId})
}

// ARC-001: a write that blocks past agentWriteLimit ends the session, and
// every waiting sender is released with that error at once.
func TestAgentWriterEndsTheSessionOnAStuckWrite(t *testing.T) {
	previous := agentWriteLimit
	agentWriteLimit = 50 * time.Millisecond
	t.Cleanup(func() { agentWriteLimit = previous })
	stream := &gatedControlStream{entered: make(chan struct{}, 8), gate: make(chan struct{})}
	defer close(stream.gate)
	writer := newAgentWriter(stream)

	results := make(chan error, 2)
	go func() { results <- writer.write(bulkMessage("stuck")) }()
	<-stream.entered
	go func() { results <- writer.write(ackMessage("waiting")) }()

	select {
	case <-writer.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end on a stuck write")
	}
	assert.Equal(t, codes.DeadlineExceeded, status.Code(writer.err))
	for range 2 {
		assert.Equal(t, codes.DeadlineExceeded, status.Code(<-results))
	}
	// Later writes fail at once.
	assert.Equal(t, codes.DeadlineExceeded, status.Code(writer.write(bulkMessage("late"))))
}

// A failed Send is the sender's error and ends the session.
func TestAgentWriterEndsTheSessionOnASendError(t *testing.T) {
	writer := newAgentWriter(&recordingControlStream{err: status.Error(codes.Unavailable, "gone")})
	assert.Equal(t, codes.Unavailable, status.Code(writer.write(bulkMessage("one"))))
	<-writer.ended
	assert.Equal(t, codes.Unavailable, status.Code(writer.write(ackMessage("two"))))
}
