package pluginhost

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

func TestManagerDispatchesOneShotBridgeCapabilityToHost(t *testing.T) {
	payloads := make(chan []byte, 1)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			payloads <- call.Payload
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"issued"}}`)}, nil
		},
	})
	require.NoError(t, err)
	manager := newHostTestManager(t, ManagerConfig{BridgeFactory: packagebridge.NewFactory(allowlist), InProcess: serveBridgePackage})
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeVerifiedArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
		RequestID: "request-bridge-1", RouteID: "identity.auth.login", Method: "POST",
		Body: []byte(`{"email":"u@example.test","password":"secret"}`), Deadline: time.Now().Add(time.Second),
		PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"code":0,"msg":"操作成功","ts":1,"data":{"token":"issued"}}`, string(response.Body))
	select {
	case payload := <-payloads:
		require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), payload)
	case <-time.After(time.Second):
		t.Fatal("bridge payload was not delivered")
	}
}

func TestManagerMigrateDispatchesOneShotBridgeCapabilityToHost(t *testing.T) {
	calls := make(chan packagebridge.Call, 1)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "migration.identity-platform.001_identity_platform", Name: "migration.identity-platform.001_identity_platform",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte("migration-digest")}, nil
		},
	})
	require.NoError(t, err)
	manager := newHostTestManager(t, ManagerConfig{BridgeFactory: packagebridge.NewFactory(allowlist), InProcess: serveBridgePackage})
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeVerifiedArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))

	response, err := manager.Migrate(context.Background(), MigrationInput{
		PackageID: "identity-platform", Version: "4.0.0", MigrationID: "001_identity_platform", Generation: 7,
	}, func(context.Context, MigrationOutput) error { return nil })

	require.NoError(t, err)
	require.True(t, response.Complete)
	require.Equal(t, "migration-digest", response.ValidationDigest)
	select {
	case call := <-calls:
		require.Equal(t, "migration.identity-platform.001_identity_platform", call.Request.RouteID)
		require.Equal(t, "migration.identity-platform.001_identity_platform", call.Operation)
	case <-time.After(time.Second):
		t.Fatal("migration bridge operation was not invoked")
	}
}

func TestManagerMintsWebSocketBridgeCapabilityBeforeOpeningTheHostStream(t *testing.T) {
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	session, child, err := packagebridge.NewSession(
		packagebridge.HostIdentity{PackageID: "machine-telemetry", Version: "4.0.0", Generation: 7}, allowlist,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, child.Close())
		require.NoError(t, session.Close())
	})
	client := &bridgeCapabilityCaptureHostClient{opened: make(chan WebSocketInput, 1)}
	host := &hostProcess{
		packageID: "machine-telemetry", version: "4.0.0", generation: 7, leaseID: "lease-7", bridge: session, client: client,
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	relay, err := manager.OpenWebSocket(context.Background(), WebSocketInput{
		PackageID: "machine-telemetry", Version: "4.0.0", Generation: 7, RouteID: "telemetry.monitor.ws",
		PrincipalJSON: []byte(`{"actor_id":9,"admin":true,"package_id":"machine-telemetry"}`),
		Metadata:      RequestMetadata{Path: "/api/v2/admin/ws/monitor"}, RequestID: "websocket-capability-1", Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	t.Cleanup(func() { relay.finish(nil) })

	select {
	case input := <-client.opened:
		require.Len(t, input.BridgeCapability, 32)
		// The host redeems the capability after OpenWebSocket returns, so it
		// must stay live for the relay and end with it.
		_, live := session.Peek(input.BridgeCapability)
		require.True(t, live, "the WebSocket bridge capability was revoked before the host could redeem it")
		relay.finish(nil)
		_, live = session.Peek(input.BridgeCapability)
		require.False(t, live, "the WebSocket bridge capability outlived its relay")
	case <-time.After(time.Second):
		t.Fatal("host did not receive the WebSocket opening input")
	}
}

type bridgeCapabilityCaptureHostClient struct {
	opened chan WebSocketInput
}

func (*bridgeCapabilityCaptureHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (*bridgeCapabilityCaptureHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (*bridgeCapabilityCaptureHostClient) Health(context.Context, uint64) (HostHealth, error) {
	return HostHealth{Healthy: true, LeaseID: "lease-7"}, nil
}

func (*bridgeCapabilityCaptureHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (s *bridgeCapabilityCaptureHostClient) OpenWebSocket(_ context.Context, input WebSocketInput) (webSocketTransport, error) {
	s.opened <- input
	return bridgeCapabilityTransport{}, nil
}

func (*bridgeCapabilityCaptureHostClient) Close() error { return nil }

type bridgeCapabilityTransport struct{}

func (bridgeCapabilityTransport) Send(WebSocketFrame) error { return nil }

func (bridgeCapabilityTransport) Recv() (WebSocketFrame, error) { return WebSocketFrame{}, io.EOF }

func (bridgeCapabilityTransport) CloseSend() error { return nil }

// serveBridgePackage serves bridgePackage: it answers through the package
// bridge with the capability each request carries.
func serveBridgePackage(ctx context.Context, packageID, version string) error {
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: packageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return bridgePackage{bridge: host.Bridge, leaseID: host.LeaseID}, nil
		},
	})
}

type bridgePackage struct {
	bridge  modulesdk.Bridge
	leaseID string
}

func (p bridgePackage) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	response, err := p.bridge.Invoke(ctx, request.BridgeCapability, "identity.auth.login", request.RequestBody)
	if err != nil {
		return pluginhostsdk.DispatchResponse{}, err
	}
	headers := make([]pluginhostsdk.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = pluginhostsdk.Header{Name: header.Name, Value: header.Value}
	}
	return pluginhostsdk.DispatchResponse{StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: headers}, nil
}

func (p bridgePackage) Migrate(ctx context.Context, request pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	response, err := p.bridge.Invoke(ctx, request.BridgeCapability, "migration.identity-platform.001_identity_platform", nil)
	if err != nil {
		return pluginhostsdk.MigrationResponse{}, err
	}
	return pluginhostsdk.MigrationResponse{Checkpoint: "identity-platform/001", ValidationDigest: string(response.Body), Complete: true}, nil
}

func (p bridgePackage) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: p.leaseID}, nil
}

func (bridgePackage) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}

// The bridge capability keeps the request as the client sent it for the
// legacy handler, and the sealed request it carries, while the host relays
// the body it was sent; the kernel's legacy dispatch serves the route
// without the host.
func TestManagerKeepsTheOriginalBodyForTheLegacyHandler(t *testing.T) {
	calls := make(chan packagebridge.Call, 2)
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"legacy":true}`)}, nil
		},
	})
	require.NoError(t, err)
	manager := newHostTestManager(t, ManagerConfig{BridgeFactory: packagebridge.NewFactory(allowlist), InProcess: serveBridgePackage})
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeVerifiedArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	original, sealed := []byte(`{"password":"typed"}`), []byte(`{"password":"anix-sealed:v1:handle"}`)
	input := DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7, RequestID: "request-sealed-1", RouteID: "identity.auth.login",
		Method: "POST", Body: sealed, BridgeBody: original, SealedRequest: "sealed-key", Deadline: time.Now().Add(5 * time.Second),
		PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
	}

	_, err = manager.Dispatch(context.Background(), input)
	require.NoError(t, err)
	call := <-calls
	require.Equal(t, original, call.Request.Body, "the legacy handler reads the request as sent")
	require.Equal(t, sealed, call.Payload, "the host relayed only what it was sent")
	require.Equal(t, "sealed-key", call.Request.SealedRequest)

	legacy := input
	legacy.Body, legacy.BridgeBody, legacy.SealedRequest = original, nil, ""
	response, err := manager.DispatchLegacy(context.Background(), legacy)
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, string(response.Body))
	call = <-calls
	require.Equal(t, original, call.Request.Body)
	require.Empty(t, call.Payload, "no host took part")

	other := legacy
	other.RouteID = "identity.auth.register"
	_, err = manager.DispatchLegacy(context.Background(), other)
	require.ErrorIs(t, err, ErrLegacyUnavailable, "a route without a legacy operation")
	stale := legacy
	stale.Generation = 6
	_, err = manager.DispatchLegacy(context.Background(), stale)
	require.ErrorIs(t, err, ErrGenerationUnavailable)

	request, err := BridgeRequest(DispatchInput{RequestID: "r", RouteID: "x", Body: sealed})
	require.NoError(t, err)
	require.Equal(t, sealed, request.Body, "without a bridge body the capability keeps the body")
}
