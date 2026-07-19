package pluginhost

import (
	"context"
	"io"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const hostTestBridgeChildEnvironment = "ANIX_PLUGINHOST_TEST_BRIDGE_CHILD"

func TestManagerDispatchesOneShotBridgeCapabilityToHost(t *testing.T) {
	payloads := make(chan []byte, 1)
	allowlist, err := packagebridge.NewAllowlist(packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			payloads <- call.Payload
			return packagebridge.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"issued"}}`)}, nil
		},
	})
	require.NoError(t, err)
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: packagebridge.NewFactory(allowlist),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeBridgeHostArtifactRef(t, "identity-platform", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
		RequestID: "request-bridge-1", RouteID: "identity.auth.login", Method: "POST",
		Body: []byte(`{"email":"u@example.test","password":"secret"}`), Deadline: time.Now().Add(time.Second),
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
	allowlist, err := packagebridge.NewAllowlist(packagebridge.Operation{
		PackageID: "identity-platform", RouteID: "migration.identity-platform.001_identity_platform", Name: "migration.identity-platform.001_identity_platform",
		Handler: func(_ context.Context, call packagebridge.Call) (packagebridge.Response, error) {
			calls <- call
			return packagebridge.Response{StatusCode: 200, Body: []byte("migration-digest")}, nil
		},
	})
	require.NoError(t, err)
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: shortHostTempDir(t), BridgeFactory: packagebridge.NewFactory(allowlist),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeBridgeHostArtifactRef(t, "identity-platform", "4.0.0")
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
	allowlist, err := packagebridge.NewAllowlist()
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

func TestPluginHostBridgeProcess(t *testing.T) {
	if os.Getenv(hostTestBridgeChildEnvironment) != "1" {
		return
	}
	bridge, err := packagebridgesdk.DialFromEnvironment(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bridge.Close()) })
	serveBridgeHost(t, bridge)
}

func serveBridgeHost(t *testing.T, bridge *packagebridgesdk.Client) {
	t.Helper()
	socketPath := os.Getenv(hostSocketEnvironment)
	require.NotEmpty(t, socketPath)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, bridgeHostServer{bridge: bridge})
	require.NoError(t, server.Serve(listener))
}

type bridgeHostServer struct {
	pluginhostv1.UnimplementedControlPackageHostServer
	bridge *packagebridgesdk.Client
}

func (s bridgeHostServer) Dispatch(ctx context.Context, request *pluginhostv1.DispatchRequest) (*pluginhostv1.DispatchResponse, error) {
	response, err := s.bridge.Invoke(ctx, request.GetBridgeCapability(), "identity.auth.login", request.GetRequestBody())
	if err != nil {
		return nil, err
	}
	headers := make([]*pluginhostv1.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = &pluginhostv1.Header{Name: header.Name, Value: header.Value}
	}
	return &pluginhostv1.DispatchResponse{StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: headers}, nil
}

func (s bridgeHostServer) Migrate(ctx context.Context, request *pluginhostv1.MigrationRequest) (*pluginhostv1.MigrationResponse, error) {
	response, err := s.bridge.Invoke(ctx, request.GetBridgeCapability(), "migration.identity-platform.001_identity_platform", nil)
	if err != nil {
		return nil, err
	}
	return &pluginhostv1.MigrationResponse{
		Checkpoint: "identity-platform/001", ValidationDigest: string(response.Body), Complete: true,
	}, nil
}

func (bridgeHostServer) Health(context.Context, *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	return &pluginhostv1.HealthResponse{Healthy: true, LeaseId: "bridge-lease"}, nil
}

func (bridgeHostServer) Drain(context.Context, *pluginhostv1.DrainRequest) (*pluginhostv1.DrainResponse, error) {
	return &pluginhostv1.DrainResponse{Drained: true}, nil
}

func writeBridgeHostArtifactRef(t *testing.T, packageID, version string) ArtifactRef {
	t.Helper()
	ref := writeVerifiedArtifactRef(t, packageID, version)
	entrypoint := []byte("#!/bin/sh\n" + hostTestBridgeChildEnvironment + "=1 exec " + strconv.Quote(os.Args[0]) + " -test.run=^TestPluginHostBridgeProcess$ --\n")
	require.NoError(t, os.WriteFile(ref.EntrypointPath, entrypoint, 0o700))
	ref.EntrypointSHA256 = testDigest(entrypoint)
	return ref
}
