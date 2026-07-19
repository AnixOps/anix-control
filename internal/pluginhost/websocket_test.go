package pluginhost

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
)

func TestWebSocketRelayRejectsLeaseLoss(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(time.Second))
	host.health.Store(false)

	err := relay.Send(WebSocketFrame{Data: []byte("after-health-loss")})

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Eventually(t, func() bool {
		select {
		case <-host.done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayCloseRejectsLeaseLoss(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(time.Second))
	host.health.Store(false)

	err := relay.Close(WebSocketClose{Code: 1000, Reason: "after health loss"})

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Eventually(t, func() bool {
		select {
		case <-host.done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayClosesOnDrain(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	manager, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(time.Second))

	err := manager.Drain(context.Background(), "machine-telemetry", "4.0.0", 10, time.Now().Add(time.Second))

	require.NoError(t, err)
	require.ErrorIs(t, relay.Send(WebSocketFrame{Data: []byte("after-drain")}), ErrHostUnavailable)
	require.Eventually(t, func() bool {
		select {
		case <-host.done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayClosesAtDeadline(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(30*time.Millisecond))

	_, err := relay.Recv()

	require.ErrorIs(t, err, ErrHostUnavailable)
	require.Eventually(t, func() bool {
		select {
		case <-host.done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestWebSocketRelayRejectsUnexpectedOpeningFrame(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	host.sendUnexpectedOpen = true
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(time.Second))

	_, err := relay.Recv()

	require.ErrorIs(t, err, ErrHostIncompatible)
}

func TestWebSocketRelayBoundsHealthChecksByDeadline(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(100*time.Millisecond))
	host.blockHealth.Store(true)
	result := make(chan error, 1)
	go func() { result <- relay.Send(WebSocketFrame{Data: []byte("blocked-health")}) }()

	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrHostUnavailable)
	case <-time.After(250 * time.Millisecond):
		close(host.releaseHealth)
		err := <-result
		t.Fatalf("health check outlived WebSocket deadline: %v", err)
	}
}

func TestWebSocketRelayClosesBlockedReceiveOnHealthLoss(t *testing.T) {
	host := newLifecycleWebSocketHostServer()
	_, relay := openLifecycleWebSocketRelay(t, host, time.Now().Add(5*time.Second))
	result := make(chan error, 1)
	go func() {
		_, err := relay.Recv()
		result <- err
	}()
	require.Eventually(t, func() bool { return host.healthCalls.Load() >= 2 }, time.Second, 10*time.Millisecond)
	host.health.Store(false)

	require.Eventually(t, func() bool {
		select {
		case <-host.done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrHostUnavailable)
	case <-time.After(time.Second):
		t.Fatal("blocked receive did not return after health loss")
	}
}

func TestWebSocketRelayFinishCancelsWithoutConcurrentCloseSend(t *testing.T) {
	transport := &blockingWebSocketTransport{closeCalled: make(chan struct{})}
	canceled := make(chan struct{})
	relay := &WebSocketRelay{
		stream: transport,
		cancel: func() { close(canceled) },
	}
	relay.writeMu.Lock()
	finished := make(chan struct{})
	go func() {
		relay.finish(ErrHostUnavailable)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("relay finish did not complete while a write was active")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("relay finish did not cancel the stream context")
	}
	select {
	case <-transport.closeCalled:
		t.Fatal("relay finish called CloseSend concurrently with a write")
	default:
	}
	relay.writeMu.Unlock()
}

func openLifecycleWebSocketRelay(t *testing.T, host *lifecycleWebSocketHostServer, deadline time.Time) (*Supervisor, *WebSocketRelay) {
	t.Helper()
	socketPath := startTestHostServerWithService(t, host)
	dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	client, err := dialHostClient(dialCtx, socketPath)
	cancelDial()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	manager := &Supervisor{hosts: map[string]*hostProcess{
		hostKey("machine-telemetry", "4.0.0"): {
			packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-7",
		},
	}}
	relay, err := manager.OpenWebSocket(context.Background(), WebSocketInput{
		PackageID: "machine-telemetry", Version: "4.0.0", Generation: 9,
		RouteID: "telemetry.monitor.ws", PrincipalJSON: []byte(`{"actor_id":7}`),
		RequestID: "request-9", IdempotencyKey: "socket-9", Deadline: deadline,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = relay.Close(WebSocketClose{Code: 1000, Reason: "test complete"}) })
	select {
	case <-host.opened:
	case <-time.After(time.Second):
		t.Fatal("host did not receive the WebSocket opening frame")
	}
	return manager, relay
}

type lifecycleWebSocketHostServer struct {
	testHostServer
	health             atomic.Bool
	healthCalls        atomic.Int32
	blockHealth        atomic.Bool
	opened             chan *pluginhostv1.WebSocketOpen
	done               chan struct{}
	doneOnce           sync.Once
	releaseHealth      chan struct{}
	sendUnexpectedOpen bool
}

type blockingWebSocketTransport struct {
	closeCalled chan struct{}
}

func (s *blockingWebSocketTransport) Send(WebSocketFrame) error { return nil }

func (s *blockingWebSocketTransport) Recv() (WebSocketFrame, error) { return WebSocketFrame{}, io.EOF }

func (s *blockingWebSocketTransport) CloseSend() error {
	close(s.closeCalled)
	return nil
}

func newLifecycleWebSocketHostServer() *lifecycleWebSocketHostServer {
	host := &lifecycleWebSocketHostServer{
		opened:        make(chan *pluginhostv1.WebSocketOpen, 1),
		done:          make(chan struct{}),
		releaseHealth: make(chan struct{}),
	}
	host.health.Store(true)
	return host
}

func (s *lifecycleWebSocketHostServer) Health(ctx context.Context, _ *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	s.healthCalls.Add(1)
	if s.blockHealth.Load() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.releaseHealth:
		}
	}
	return &pluginhostv1.HealthResponse{Healthy: s.health.Load(), LeaseId: "lease-7", DetailsJson: `{}`}, nil
}

func (s *lifecycleWebSocketHostServer) OpenWebSocket(stream pluginhostv1.ControlPackageHost_OpenWebSocketServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	if open := frame.GetOpen(); open != nil {
		s.opened <- open
	} else {
		return ErrHostIncompatible
	}
	if s.sendUnexpectedOpen {
		return stream.Send(&pluginhostv1.WebSocketFrame{Value: &pluginhostv1.WebSocketFrame_Open{Open: &pluginhostv1.WebSocketOpen{}}})
	}
	<-stream.Context().Done()
	s.doneOnce.Do(func() { close(s.done) })
	return stream.Context().Err()
}
