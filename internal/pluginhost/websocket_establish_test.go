package pluginhost

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWebSocketRelayRegistersBeforeOpeningStream(t *testing.T) {
	client := &blockingOpenHostClient{
		openEntered: make(chan struct{}),
		releaseOpen: make(chan struct{}),
	}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	result := make(chan error, 1)
	go func() {
		_, err := manager.OpenWebSocket(context.Background(), WebSocketInput{
			PackageID: "machine-telemetry", Version: "4.0.0", Generation: 9,
			RouteID: "telemetry.monitor.ws", PrincipalJSON: []byte(`{"actor_id":7}`),
			RequestID: "request-9", Deadline: time.Now().Add(time.Second),
		})
		result <- err
	}()
	select {
	case <-client.openEntered:
	case <-time.After(time.Second):
		t.Fatal("relay did not begin host stream establishment")
	}

	require.NoError(t, manager.Drain(context.Background(), "machine-telemetry", "4.0.0", 10, time.Now().Add(time.Second)))
	close(client.releaseOpen)
	err := <-result

	require.ErrorIs(t, err, ErrHostUnavailable)
	require.False(t, client.openReached.Load(), "drain must cancel pending relay before its opening frame reaches the host")
}

func TestCanceledHealthWatcherCannotDrainNewRelay(t *testing.T) {
	client := &watcherRaceHostClient{
		watcherEntered:  make(chan struct{}),
		releaseWatcher:  make(chan struct{}),
		watcherReturned: make(chan struct{}),
	}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	first, err := manager.OpenWebSocket(context.Background(), testWebSocketInput())
	require.NoError(t, err)
	select {
	case <-client.watcherEntered:
	case <-time.After(time.Second):
		t.Fatal("first relay health watcher did not enter its health RPC")
	}
	first.finish(nil)

	second, err := manager.OpenWebSocket(context.Background(), testWebSocketInput())
	require.NoError(t, err)
	t.Cleanup(func() { second.finish(nil) })
	close(client.releaseWatcher)
	select {
	case <-client.watcherReturned:
	case <-time.After(time.Second):
		t.Fatal("canceled health watcher did not return")
	}

	require.NoError(t, second.Send(WebSocketFrame{Data: []byte("new relay remains active")}))
	require.False(t, host.isWebSocketDraining())
}

func TestWebSocketRelayGracefulCloseHalfClosesWithoutCancelingContext(t *testing.T) {
	transport := &closeRecordingTransport{closeSent: make(chan struct{})}
	client := &closeDeliveryHostClient{transport: transport, contextCanceled: make(chan struct{})}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	parentCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	relay, err := manager.OpenWebSocket(parentCtx, testWebSocketInput())
	require.NoError(t, err)

	require.NoError(t, relay.Close(WebSocketClose{Code: 1000, Reason: "normal close"}))

	require.Equal(t, []byte(nil), transport.sentData())
	require.Equal(t, &WebSocketClose{Code: 1000, Reason: "normal close"}, transport.sentClose())
	select {
	case <-transport.closeSent:
	case <-time.After(time.Second):
		t.Fatal("graceful close did not half-close the client stream")
	}
	select {
	case <-client.contextCanceled:
		t.Fatal("graceful close canceled the stream before the host could receive its close frame")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWebSocketRelayOutlivesSetupContextAfterEstablish(t *testing.T) {
	transport := &closeRecordingTransport{closeSent: make(chan struct{})}
	client := &closeDeliveryHostClient{transport: transport, contextCanceled: make(chan struct{})}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	setupCtx, cancelSetup := context.WithCancel(context.Background())
	relay, err := manager.OpenWebSocket(setupCtx, WebSocketInput{
		PackageID: "machine-telemetry", Version: "4.0.0", Generation: 9,
		RouteID: "telemetry.monitor.ws", PrincipalJSON: []byte(`{"actor_id":7}`),
		RequestID: "request-9", Deadline: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = relay.Close(WebSocketClose{Code: 1000, Reason: "test complete"}) })

	cancelSetup()
	require.NoError(t, relay.Send(WebSocketFrame{Data: []byte("session remains active")}))
	select {
	case <-client.contextCanceled:
		t.Fatal("setup context cancellation terminated the established WebSocket relay")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWebSocketRelayGracefulCloseRemainsTrackedUntilLifecycleTerminal(t *testing.T) {
	transport := &trackedCloseTransport{closeSent: make(chan struct{}), recvEntered: make(chan struct{})}
	client := &trackedCloseHostClient{transport: transport, contextCanceled: make(chan struct{})}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	parentCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	relay, err := manager.OpenWebSocket(parentCtx, testWebSocketInput())
	require.NoError(t, err)
	require.NoError(t, relay.Close(WebSocketClose{Code: 1000, Reason: "normal close"}))
	select {
	case <-transport.recvEntered:
	case <-time.After(time.Second):
		t.Fatal("graceful close did not wait for host stream termination")
	}
	require.True(t, hostTracksRelay(host, relay))

	require.NoError(t, manager.Drain(context.Background(), "machine-telemetry", "4.0.0", 10, time.Now().Add(time.Second)))
	select {
	case <-client.contextCanceled:
	case <-time.After(time.Second):
		t.Fatal("drain did not cancel the graceful-close stream")
	}
	require.Eventually(t, func() bool { return !hostTracksRelay(host, relay) }, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayGracefulCloseCancelsOnHostFrameError(t *testing.T) {
	transport := &trackedCloseTransport{
		closeSent:   make(chan struct{}),
		recvEntered: make(chan struct{}),
		recvErr:     fmt.Errorf("%w: unexpected opening frame", ErrHostIncompatible),
	}
	client := &trackedCloseHostClient{transport: transport, contextCanceled: make(chan struct{})}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	parentCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	relay, err := manager.OpenWebSocket(parentCtx, testWebSocketInput())
	require.NoError(t, err)
	require.NoError(t, relay.Close(WebSocketClose{Code: 1000, Reason: "normal close"}))
	select {
	case <-transport.recvEntered:
	case <-time.After(time.Second):
		t.Fatal("graceful close did not observe the host stream")
	}
	select {
	case <-client.contextCanceled:
	case <-time.After(time.Second):
		t.Fatal("host frame error did not cancel the graceful-close stream")
	}
	require.Eventually(t, func() bool { return !hostTracksRelay(host, relay) }, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayGracefulCloseCancelsOnHostClose(t *testing.T) {
	transport := &trackedCloseTransport{
		closeSent:   make(chan struct{}),
		recvEntered: make(chan struct{}),
		recvFrame:   &WebSocketFrame{Close: &WebSocketClose{Code: 1000, Reason: "host close"}},
	}
	client := &trackedCloseHostClient{transport: transport, contextCanceled: make(chan struct{})}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-9",
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	parentCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	relay, err := manager.OpenWebSocket(parentCtx, testWebSocketInput())
	require.NoError(t, err)
	require.NoError(t, relay.Close(WebSocketClose{Code: 1000, Reason: "normal close"}))
	select {
	case <-client.contextCanceled:
	case <-time.After(time.Second):
		t.Fatal("host close did not cancel the graceful-close stream")
	}
	require.Eventually(t, func() bool { return !hostTracksRelay(host, relay) }, time.Second, 10*time.Millisecond)
}

func hostTracksRelay(host *hostProcess, relay *WebSocketRelay) bool {
	host.relayMu.Lock()
	defer host.relayMu.Unlock()
	_, found := host.relays[relay]
	return found
}

func testWebSocketInput() WebSocketInput {
	return WebSocketInput{
		PackageID: "machine-telemetry", Version: "4.0.0", Generation: 9,
		RouteID: "telemetry.monitor.ws", PrincipalJSON: []byte(`{"actor_id":7}`),
		RequestID: "request-9", Deadline: time.Now().Add(time.Second),
	}
}

type blockingOpenHostClient struct {
	openEntered chan struct{}
	openOnce    sync.Once
	releaseOpen chan struct{}
	openReached atomic.Bool
}

func (c *blockingOpenHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (c *blockingOpenHostClient) Health(context.Context, uint64) (HostHealth, error) {
	return HostHealth{Healthy: true, LeaseID: "lease-9", DetailsJSON: `{}`}, nil
}

func (c *blockingOpenHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (c *blockingOpenHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (c *blockingOpenHostClient) OpenWebSocket(ctx context.Context, _ WebSocketInput) (webSocketTransport, error) {
	c.openOnce.Do(func() { close(c.openEntered) })
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrHostUnavailable, ctx.Err())
	case <-c.releaseOpen:
		c.openReached.Store(true)
		return noOpWebSocketTransport{}, nil
	}
}

func (c *blockingOpenHostClient) Close() error { return nil }

type watcherRaceHostClient struct {
	healthCalls     atomic.Int32
	watcherEntered  chan struct{}
	releaseWatcher  chan struct{}
	watcherReturned chan struct{}
}

func (c *watcherRaceHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (c *watcherRaceHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (c *watcherRaceHostClient) Health(context.Context, uint64) (HostHealth, error) {
	if c.healthCalls.Add(1) == 2 {
		close(c.watcherEntered)
		<-c.releaseWatcher
		close(c.watcherReturned)
		return HostHealth{}, fmt.Errorf("stale watcher health request completed")
	}
	return HostHealth{Healthy: true, LeaseID: "lease-9", DetailsJSON: `{}`}, nil
}

func (c *watcherRaceHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (c *watcherRaceHostClient) OpenWebSocket(context.Context, WebSocketInput) (webSocketTransport, error) {
	return noOpWebSocketTransport{}, nil
}

func (c *watcherRaceHostClient) Close() error { return nil }

type closeDeliveryHostClient struct {
	transport       *closeRecordingTransport
	contextCanceled chan struct{}
}

func (c *closeDeliveryHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (c *closeDeliveryHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (c *closeDeliveryHostClient) Health(context.Context, uint64) (HostHealth, error) {
	return HostHealth{Healthy: true, LeaseID: "lease-9", DetailsJSON: `{}`}, nil
}

func (c *closeDeliveryHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (c *closeDeliveryHostClient) OpenWebSocket(ctx context.Context, _ WebSocketInput) (webSocketTransport, error) {
	c.transport.ctx = ctx
	go func() {
		<-ctx.Done()
		close(c.contextCanceled)
	}()
	return c.transport, nil
}

func (c *closeDeliveryHostClient) Close() error { return nil }

type closeRecordingTransport struct {
	mu        sync.Mutex
	ctx       context.Context
	frames    []WebSocketFrame
	closeSent chan struct{}
}

func (s *closeRecordingTransport) Send(frame WebSocketFrame) error {
	s.mu.Lock()
	s.frames = append(s.frames, frame)
	s.mu.Unlock()
	return nil
}

func (s *closeRecordingTransport) Recv() (WebSocketFrame, error) {
	<-s.ctx.Done()
	return WebSocketFrame{}, s.ctx.Err()
}

func (s *closeRecordingTransport) CloseSend() error {
	close(s.closeSent)
	return nil
}

func (s *closeRecordingTransport) sentData() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.frames) == 0 {
		return nil
	}
	return s.frames[0].Data
}

func (s *closeRecordingTransport) sentClose() *WebSocketClose {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.frames) == 0 {
		return nil
	}
	return s.frames[0].Close
}

type trackedCloseHostClient struct {
	transport       *trackedCloseTransport
	contextCanceled chan struct{}
}

func (c *trackedCloseHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (c *trackedCloseHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (c *trackedCloseHostClient) Health(context.Context, uint64) (HostHealth, error) {
	return HostHealth{Healthy: true, LeaseID: "lease-9", DetailsJSON: `{}`}, nil
}

func (c *trackedCloseHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (c *trackedCloseHostClient) OpenWebSocket(ctx context.Context, _ WebSocketInput) (webSocketTransport, error) {
	c.transport.ctx = ctx
	go func() {
		<-ctx.Done()
		close(c.contextCanceled)
	}()
	return c.transport, nil
}

func (c *trackedCloseHostClient) Close() error { return nil }

type trackedCloseTransport struct {
	ctx         context.Context
	closeSent   chan struct{}
	recvEntered chan struct{}
	recvErr     error
	recvFrame   *WebSocketFrame
}

func (s *trackedCloseTransport) Send(WebSocketFrame) error { return nil }

func (s *trackedCloseTransport) Recv() (WebSocketFrame, error) {
	close(s.recvEntered)
	if s.recvErr != nil {
		return WebSocketFrame{}, s.recvErr
	}
	if s.recvFrame != nil {
		return *s.recvFrame, nil
	}
	<-s.ctx.Done()
	return WebSocketFrame{}, s.ctx.Err()
}

func (s *trackedCloseTransport) CloseSend() error {
	close(s.closeSent)
	return nil
}

type noOpWebSocketTransport struct{}

func (noOpWebSocketTransport) Send(WebSocketFrame) error { return nil }

func (noOpWebSocketTransport) Recv() (WebSocketFrame, error) {
	return WebSocketFrame{}, context.Canceled
}

func (noOpWebSocketTransport) CloseSend() error { return nil }
