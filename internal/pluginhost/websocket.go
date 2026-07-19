package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
)

type WebSocketInput struct {
	PackageID        string
	Version          string
	Generation       uint64
	RouteID          string
	PrincipalJSON    []byte
	Metadata         RequestMetadata
	RequestID        string
	IdempotencyKey   string
	BridgeCapability []byte
	Deadline         time.Time
}

type WebSocketClose struct {
	Code   uint32
	Reason string
}

// WebSocketFrame is a post-open relay frame. A nil Close represents a data
// frame, including an empty opaque payload.
type WebSocketFrame struct {
	Data  []byte
	Close *WebSocketClose
}

type webSocketTransport interface {
	Send(WebSocketFrame) error
	Recv() (WebSocketFrame, error)
	CloseSend() error
}

const (
	webSocketHostHealthInterval = 100 * time.Millisecond
	webSocketHostHealthTimeout  = time.Second
)

// WebSocketRelay owns one verified package-host stream. Callers may make one
// Send and one Recv call concurrently; concurrent writes are serialized.
type WebSocketRelay struct {
	supervisor *Supervisor
	host       *hostProcess
	input      WebSocketInput
	stream     webSocketTransport
	cancel     context.CancelFunc

	mu       sync.Mutex
	writeMu  sync.Mutex
	readMu   sync.Mutex
	closed   bool
	released bool
	closeErr error
}

var _ WebSocketManager = (*Supervisor)(nil)

func (m *Supervisor) OpenWebSocket(ctx context.Context, input WebSocketInput) (*WebSocketRelay, error) {
	if m == nil {
		return nil, ErrHostUnavailable
	}
	if err := validateWebSocketInput(input); err != nil {
		return nil, err
	}
	host, err := m.hostForGeneration(input.PackageID, input.Version, input.Generation)
	if err != nil {
		return nil, err
	}
	capability, err := host.mintWebSocketCapability(input)
	if err != nil {
		return nil, err
	}
	if len(capability) > 0 {
		input.BridgeCapability = capability
		defer host.bridge.Revoke(capability)
	}
	setupCtx, cancelSetupDeadline := context.WithDeadline(ctx, input.Deadline)
	defer cancelSetupDeadline()
	health, err := host.client.Health(setupCtx, input.Generation)
	if err != nil {
		return nil, err
	}
	if !health.Healthy || health.LeaseID == "" || health.LeaseID != host.leaseID {
		return nil, fmt.Errorf("%w: health lease does not match active generation", ErrHostIncompatible)
	}
	// The caller context bounds health and stream establishment. Once the
	// stream is attached, the relay lives until its explicit session deadline.
	relayCtx, cancel := context.WithDeadline(context.Background(), input.Deadline)
	stopSetupCancellation := context.AfterFunc(setupCtx, cancel)
	relay := &WebSocketRelay{supervisor: m, host: host, input: input, cancel: cancel}
	if err := host.registerWebSocketRelay(relay); err != nil {
		stopSetupCancellation()
		cancel()
		return nil, err
	}
	stream, err := host.client.OpenWebSocket(relayCtx, input)
	if err != nil {
		stopSetupCancellation()
		relay.finish(err)
		return nil, err
	}
	if err := relay.attachStream(stream); err != nil {
		stopSetupCancellation()
		_ = stream.CloseSend()
		relay.finish(err)
		return nil, err
	}
	if !stopSetupCancellation() {
		err := setupCtx.Err()
		if err == nil {
			err = ErrHostUnavailable
		}
		relay.finish(err)
		return nil, err
	}
	return relay, nil
}

func (h *hostProcess) mintWebSocketCapability(input WebSocketInput) ([]byte, error) {
	if h == nil || h.bridge == nil {
		return nil, nil
	}
	metadata, err := marshalRequestMetadata(input.Metadata)
	if err != nil {
		return nil, err
	}
	capability, err := h.bridge.Mint(packagebridge.Request{
		RequestID: input.RequestID, RouteID: input.RouteID, Method: "GET",
		PrincipalJSON: input.PrincipalJSON, MetadataJSON: metadata, Deadline: input.Deadline,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: mint package WebSocket bridge capability: %v", ErrHostUnavailable, err)
	}
	return capability, nil
}

func validateWebSocketInput(input WebSocketInput) error {
	if input.PackageID == "" || input.Version == "" {
		return fmt.Errorf("%w: package identity is required", ErrHostIncompatible)
	}
	if input.Generation == 0 {
		return ErrGenerationUnavailable
	}
	if input.RouteID == "" || input.RequestID == "" {
		return fmt.Errorf("%w: WebSocket route and request IDs are required", ErrHostIncompatible)
	}
	if !input.Deadline.After(time.Now()) {
		return fmt.Errorf("%w: WebSocket deadline has expired", ErrHostUnavailable)
	}
	if !json.Valid(input.PrincipalJSON) {
		return fmt.Errorf("%w: principal JSON is invalid", ErrHostIncompatible)
	}
	return nil
}

func (r *WebSocketRelay) Send(frame WebSocketFrame) error {
	if frame.Close != nil {
		if len(frame.Data) != 0 {
			return fmt.Errorf("%w: WebSocket frame cannot contain data and close", ErrHostIncompatible)
		}
		return r.Close(*frame.Close)
	}
	if err := r.ensureActive(); err != nil {
		return err
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	if err := r.currentError(); err != nil {
		return err
	}
	if err := r.stream.Send(frame); err != nil {
		r.finish(err)
		return err
	}
	return nil
}

func (r *WebSocketRelay) Recv() (WebSocketFrame, error) {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	if err := r.ensureActive(); err != nil {
		return WebSocketFrame{}, err
	}
	frame, err := r.stream.Recv()
	if err != nil {
		r.finish(err)
		return WebSocketFrame{}, err
	}
	if frame.Close != nil {
		r.finish(nil)
	}
	return frame, nil
}

func (r *WebSocketRelay) Close(close WebSocketClose) error {
	if err := r.ensureActive(); err != nil {
		return err
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	if err := r.currentError(); err != nil {
		return err
	}
	if err := r.stream.Send(WebSocketFrame{Close: &close}); err != nil {
		r.finish(err)
		return err
	}
	if err := r.stream.CloseSend(); err != nil {
		r.finish(err)
		return err
	}
	r.finishGracefully()
	return nil
}

func (r *WebSocketRelay) ensureActive() error {
	if err := r.currentError(); err != nil {
		return err
	}
	if !r.input.Deadline.After(time.Now()) {
		err := fmt.Errorf("%w: WebSocket deadline has expired", ErrHostUnavailable)
		r.finish(err)
		return err
	}
	host, err := r.supervisor.hostForGeneration(r.input.PackageID, r.input.Version, r.input.Generation)
	if err != nil || host != r.host {
		if err == nil {
			err = ErrGenerationUnavailable
		}
		r.finish(err)
		return err
	}
	healthCtx, cancel := context.WithDeadline(context.Background(), r.input.Deadline)
	defer cancel()
	health, err := r.host.client.Health(healthCtx, r.input.Generation)
	if err != nil {
		r.finish(err)
		return err
	}
	if !health.Healthy || health.LeaseID == "" || health.LeaseID != r.host.leaseID {
		err = fmt.Errorf("%w: health lease does not match active generation", ErrHostIncompatible)
		r.finish(err)
		return err
	}
	return nil
}

func (r *WebSocketRelay) currentError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		return nil
	}
	if r.closeErr != nil {
		return r.closeErr
	}
	return ErrHostUnavailable
}

func (r *WebSocketRelay) attachStream(stream webSocketTransport) error {
	if stream == nil {
		return ErrHostUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		if r.closeErr != nil {
			return r.closeErr
		}
		return ErrHostUnavailable
	}
	r.stream = stream
	return nil
}

func (r *WebSocketRelay) finish(err error) {
	r.finishWithCancellation(err, true)
}

func (r *WebSocketRelay) finishGracefully() {
	r.mu.Lock()
	if r.closed || r.released {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()
	go r.awaitGracefulTermination()
}

func (r *WebSocketRelay) finishWithCancellation(err error, cancelContext bool) {
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.closeErr = err
	r.released = true
	cancel := r.cancel
	r.mu.Unlock()

	if cancelContext && cancel != nil {
		cancel()
	}
	if r.host != nil {
		r.host.releaseWebSocketRelay(r)
	}
}

func (r *WebSocketRelay) awaitGracefulTermination() {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	for {
		if r.isReleased() {
			return
		}
		frame, err := r.stream.Recv()
		if errors.Is(err, io.EOF) {
			r.releaseGracefully()
			return
		}
		if err != nil || frame.Close != nil {
			r.finish(err)
			return
		}
	}
}

func (r *WebSocketRelay) isReleased() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.released
}

func (r *WebSocketRelay) releaseGracefully() {
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return
	}
	r.released = true
	r.mu.Unlock()
	if r.host != nil {
		r.host.releaseWebSocketRelay(r)
	}
}

func (h *hostProcess) registerWebSocketRelay(relay *WebSocketRelay) error {
	if h == nil {
		return ErrHostUnavailable
	}
	h.relayMu.Lock()
	if h.draining {
		h.relayMu.Unlock()
		return ErrHostUnavailable
	}
	if h.relays == nil {
		h.relays = make(map[*WebSocketRelay]struct{})
	}
	var healthCtx context.Context
	if h.relayHealthCancel == nil {
		healthCtx, h.relayHealthCancel = context.WithCancel(context.Background())
	}
	h.relays[relay] = struct{}{}
	h.relayMu.Unlock()
	if healthCtx != nil {
		go h.watchWebSocketHealth(healthCtx)
	}
	return nil
}

func (h *hostProcess) releaseWebSocketRelay(relay *WebSocketRelay) {
	if h == nil {
		return
	}
	h.relayMu.Lock()
	delete(h.relays, relay)
	var cancel context.CancelFunc
	if len(h.relays) == 0 && h.relayHealthCancel != nil {
		cancel = h.relayHealthCancel
		h.relayHealthCancel = nil
	}
	h.relayMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *hostProcess) beginWebSocketDrain() {
	h.failWebSocketRelays(ErrHostUnavailable)
}

func (h *hostProcess) failWebSocketRelays(err error) {
	if h == nil {
		return
	}
	h.relayMu.Lock()
	if h.draining {
		h.relayMu.Unlock()
		return
	}
	h.draining = true
	cancel := h.relayHealthCancel
	h.relayHealthCancel = nil
	relays := make([]*WebSocketRelay, 0, len(h.relays))
	for relay := range h.relays {
		relays = append(relays, relay)
	}
	h.relayMu.Unlock()
	if cancel != nil {
		cancel()
	}
	for _, relay := range relays {
		relay.finish(err)
	}
}

func (h *hostProcess) isWebSocketDraining() bool {
	if h == nil {
		return true
	}
	h.relayMu.Lock()
	defer h.relayMu.Unlock()
	return h.draining
}

func (h *hostProcess) watchWebSocketHealth(ctx context.Context) {
	ticker := time.NewTicker(webSocketHostHealthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		healthCtx, cancel := context.WithTimeout(ctx, webSocketHostHealthTimeout)
		health, err := h.client.Health(healthCtx, h.generation)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			h.failWebSocketRelays(err)
			return
		}
		if !health.Healthy || health.LeaseID == "" || health.LeaseID != h.leaseID {
			h.failWebSocketRelays(fmt.Errorf("%w: health lease does not match active generation", ErrHostIncompatible))
			return
		}
	}
}
