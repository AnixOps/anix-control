package kernelnodeops

import (
	"context"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// eventStream collects a watch's events in memory.
type eventStream struct {
	grpc.ServerStream
	ctx    context.Context
	events chan *kernelnodeopsv1.OperationEvent
}

func newEventStream(ctx context.Context) *eventStream {
	return &eventStream{ctx: ctx, events: make(chan *kernelnodeopsv1.OperationEvent, 256)}
}

func (s *eventStream) Context() context.Context { return s.ctx }
func (s *eventStream) Send(event *kernelnodeopsv1.OperationEvent) error {
	s.events <- event
	return nil
}
func (s *eventStream) SetHeader(metadata.MD) error  { return nil }
func (s *eventStream) SendHeader(metadata.MD) error { return nil }
func (s *eventStream) SetTrailer(metadata.MD)       {}

func (s *eventStream) next(t *testing.T) *kernelnodeopsv1.OperationEvent {
	t.Helper()
	select {
	case event := <-s.events:
		return event
	case <-time.After(10 * time.Second):
		require.FailNow(t, "no event")
		return nil
	}
}

// watch runs WatchOperations until the test ends and returns its stream and
// a function that waits for its result (at most once).
func watch(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, request *kernelnodeopsv1.WatchOperationsRequest) (*eventStream, func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream := newEventStream(ctx)
	result := make(chan error, 1)
	go func() { result <- client.WatchOperations(request, stream) }()
	var once sync.Once
	var err error
	wait := func() error {
		once.Do(func() { err = <-result })
		return err
	}
	t.Cleanup(func() {
		cancel()
		_ = wait()
	})
	return stream, wait
}

// A watch streams each change of the caller's operations with the
// operation as it is; a client resumes from the last cursor it saw.
func TestWatchStreamsChangesAfterACursor(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		executor := newHeld()
		h.serve(KindNodeSync, executor)
		h.serve(KindForwardApply, newScript(succeed))
		h.start(t)
		client := h.client(protocolHost, allFamilies())
		stream, _ := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{})

		// Another package's operations are not shown.
		submit(t, h.client(forwardHost, allFamilies()), "forward.apply:watch", applyForward(40, forwardAction))
		operation := submit(t, client, "node.sync:watch", syncNode(proxyKind, 1, false)).GetOperation()
		executor.next(t)
		var cursors []uint64
		for {
			event := stream.next(t)
			require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_CHANGED, event.GetKind())
			require.Equal(t, operation.GetOperationId(), event.GetOperation().GetOperationId())
			cursors = append(cursors, event.GetCursor())
			if event.GetOperation().GetState() == running {
				break
			}
		}
		require.IsIncreasing(t, cursors)
		executor.release <- Succeeded(nodeSyncResult("watched"))
		var last *kernelnodeopsv1.OperationEvent
		for last == nil || last.GetOperation().GetState() != succeeded {
			last = stream.next(t)
		}
		require.Equal(t, "watched", last.GetOperation().GetResult().GetNodeSync().GetConfigHash())

		// Resuming from a cursor replays what came after it.
		resumed, _ := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{AfterCursor: cursors[0]})
		event := resumed.next(t)
		require.Greater(t, event.GetCursor(), cursors[0])
		require.Equal(t, succeeded, event.GetOperation().GetState(), "each event carries the operation as it is")

		// The family filter.
		filtered, _ := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{
			Families: []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE},
		})
		h.serve(KindAgentDiagnostic, newScript(succeed))
		diagnostic := submit(t, client, "agent.diag:1:d", sampleOperation(KindAgentDiagnostic)).GetOperation()
		event = filtered.next(t)
		require.Equal(t, diagnostic.GetOperationId(), event.GetOperation().GetOperationId())
	})
}

// A cursor the log no longer holds, pruned or past its end, is told to
// resynchronize, with the cursor to watch from after listing.
func TestWatchResyncsAStaleCursor(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindAgentOperation, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		for _, id := range []string{"a", "b", "c"} {
			submit(t, client, "agent.operation:"+id, agentOperation(1, "agent.ping"))
		}
		var events []model.KernelNodeOperationEvent
		require.NoError(t, db.Order("id").Find(&events).Error)
		require.Len(t, events, 3)
		latest := events[2].ID

		beyond, result := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{AfterCursor: latest + 10})
		event := beyond.next(t)
		require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_RESYNC, event.GetKind())
		require.Equal(t, latest, event.GetCursor())
		require.NoError(t, result())

		require.NoError(t, db.Model(&model.KernelNodeOperationEvent{}).Where("id < ?", latest).
			Update("created_at", time.Now().Add(-EventRetention-time.Hour)).Error)
		_, err := h.engine.Prune(context.Background(), time.Now())
		require.NoError(t, err)
		pruned, result := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{AfterCursor: events[0].ID})
		event = pruned.next(t)
		require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_RESYNC, event.GetKind())
		require.Equal(t, latest, event.GetCursor())
		require.NoError(t, result())

		// From the event just before the oldest kept, nothing is missing.
		current, _ := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{AfterCursor: latest - 1})
		event = current.next(t)
		require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_CHANGED, event.GetKind())
		require.Equal(t, latest, event.GetCursor())
	})
}

// A watch is authorized again while it streams: a fenced generation's watch
// ends PERMISSION_DENIED.
func TestWatchIsReauthorized(t *testing.T) {
	previous := watchReauthorize
	watchReauthorize = 50 * time.Millisecond
	t.Cleanup(func() { watchReauthorize = previous })
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		authorizer := allFamilies()
		client := h.client(protocolHost, authorizer)
		_, result := watch(t, client, &kernelnodeopsv1.WatchOperationsRequest{})
		authorizer.set("fenced", true)
		h.engine.changed()
		require.Equal(t, codes.PermissionDenied, status.Code(result()), "the watch was reauthorized")
		_, err := client.ListOperations(context.Background(), &kernelnodeopsv1.ListOperationsRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err))

		none := h.client(protocolHost, allow())
		err = none.WatchOperations(&kernelnodeopsv1.WatchOperationsRequest{}, newEventStream(context.Background()))
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		err = h.client(protocolHost, allFamilies()).WatchOperations(&kernelnodeopsv1.WatchOperationsRequest{
			Families: []kernelnodeopsv1.OperationFamily{9},
		}, newEventStream(context.Background()))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
