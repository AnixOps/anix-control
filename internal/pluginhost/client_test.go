package pluginhost

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	hostTestChildEnvironment           = "ANIX_PLUGINHOST_TEST_CHILD"
	hostTestForbiddenSecretEnvironment = "ANIX_PLUGINHOST_TEST_SERVER_SECRET"
)

func TestClientDispatchesOverUnixSocket(t *testing.T) {
	socketPath := startTestHostServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	response, err := client.Dispatch(ctx, DispatchInput{
		PackageID:     "knowledge",
		Version:       "4.0.0",
		Generation:    7,
		RequestID:     "request-1",
		RouteID:       "knowledge.article.list",
		Method:        "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`),
		Deadline:      time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 202, response.StatusCode)
	require.Equal(t, []byte(`{"transport":"unix"}`), response.Body)
}

func TestClientDispatchesRequestMetadataOverUnixSocket(t *testing.T) {
	host := &metadataDispatchHostServer{requests: make(chan *pluginhostv1.DispatchRequest, 1)}
	socketPath := startTestHostServerWithService(t, host)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	_, err = client.Dispatch(ctx, DispatchInput{
		PackageID: "knowledge", Version: "4.0.0", Generation: 7, RequestID: "request-metadata",
		RouteID: "knowledge.article.list", Method: "GET", PrincipalJSON: []byte(`{"actor_id":7}`),
		Metadata: RequestMetadata{
			Path: "/api/v2/user/knowledge", Query: map[string][]string{"tag": {"stable", "v4"}},
			PathParams: map[string]string{"article_id": "42"},
		},
		Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)

	select {
	case request := <-host.requests:
		require.JSONEq(t, `{"path":"/api/v2/user/knowledge","query":{"tag":["stable","v4"]},"path_params":{"article_id":"42"}}`, string(request.GetRequestMetadataJson()))
	case <-ctx.Done():
		t.Fatal("host did not receive request metadata")
	}
}

func TestWebSocketRelaySendsVerifiedOpenBeforeData(t *testing.T) {
	host := &webSocketTestHostServer{opened: make(chan *pluginhostv1.WebSocketOpen, 1)}
	socketPath := startTestHostServerWithService(t, host)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	manager := &Supervisor{hosts: map[string]*hostProcess{
		hostKey("machine-telemetry", "4.0.0"): {
			packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: client, leaseID: "lease-7",
		},
	}}
	relay, err := manager.OpenWebSocket(ctx, WebSocketInput{
		PackageID: "machine-telemetry", Version: "4.0.0", Generation: 9,
		RouteID: "telemetry.monitor.ws", PrincipalJSON: []byte(`{"actor_id":7}`),
		Metadata:  RequestMetadata{Path: "/api/v2/admin/ws/monitor"},
		RequestID: "request-9", IdempotencyKey: "socket-9", Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = relay.Close(WebSocketClose{Code: 1000, Reason: "test complete"}) })

	select {
	case open := <-host.opened:
		require.Equal(t, "machine-telemetry", open.GetPackageId())
		require.Equal(t, "4.0.0", open.GetPackageVersion())
		require.Equal(t, uint64(9), open.GetRouteGeneration())
		require.Equal(t, "telemetry.monitor.ws", open.GetRouteId())
		require.JSONEq(t, `{"path":"/api/v2/admin/ws/monitor"}`, string(open.GetRequestMetadataJson()))
	case <-ctx.Done():
		t.Fatal("host did not receive the verified WebSocket opening frame")
	}

	require.NoError(t, relay.Send(WebSocketFrame{Data: []byte("ping")}))
	frame, err := relay.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), frame.Data)
}

func TestPluginHostProcess(t *testing.T) {
	if os.Getenv(hostTestChildEnvironment) != "1" {
		return
	}
	if os.Getenv(hostTestForbiddenSecretEnvironment) != "" {
		t.Fatal("host process inherited a parent secret")
	}
	socketPath := os.Getenv(hostSocketEnvironment)
	if socketPath == "" {
		t.Fatal("host socket path is required")
	}
	if os.Getenv(hostRuntimeDirectoryFDEnvironment) != "3" {
		t.Fatal("host runtime directory descriptor is required")
	}
	runtimeInfo, err := os.Stat(childRuntimeDirectoryPath)
	require.NoError(t, err)
	if !runtimeInfo.IsDir() {
		t.Fatal("host runtime descriptor is not a directory")
	}
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, testHostServer{})
	require.NoError(t, server.Serve(listener))
}

func startTestHostServer(t *testing.T) string {
	return startTestHostServerWithService(t, testHostServer{})
}

func startTestHostServerWithService(t *testing.T, service pluginhostv1.ControlPackageHostServer) string {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "host.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
	})
	return socketPath
}

type testHostServer struct {
	pluginhostv1.UnimplementedControlPackageHostServer
}

func (testHostServer) Dispatch(context.Context, *pluginhostv1.DispatchRequest) (*pluginhostv1.DispatchResponse, error) {
	return &pluginhostv1.DispatchResponse{
		StatusCode:   202,
		ResponseBody: []byte(`{"transport":"unix"}`),
	}, nil
}

func (testHostServer) Health(context.Context, *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	return &pluginhostv1.HealthResponse{Healthy: true, LeaseId: "lease-7", DetailsJson: `{}`}, nil
}

func (testHostServer) Drain(context.Context, *pluginhostv1.DrainRequest) (*pluginhostv1.DrainResponse, error) {
	return &pluginhostv1.DrainResponse{Drained: true}, nil
}

type webSocketTestHostServer struct {
	testHostServer
	opened chan *pluginhostv1.WebSocketOpen
}

type metadataDispatchHostServer struct {
	testHostServer
	requests chan *pluginhostv1.DispatchRequest
}

func (s *metadataDispatchHostServer) Dispatch(_ context.Context, request *pluginhostv1.DispatchRequest) (*pluginhostv1.DispatchResponse, error) {
	s.requests <- request
	return &pluginhostv1.DispatchResponse{StatusCode: http.StatusAccepted}, nil
}

func (s *webSocketTestHostServer) OpenWebSocket(stream pluginhostv1.ControlPackageHost_OpenWebSocketServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	if open := frame.GetOpen(); open != nil {
		s.opened <- open
	} else {
		return status.Error(codes.InvalidArgument, "first frame must be open")
	}
	frame, err = stream.Recv()
	if err != nil {
		return err
	}
	if string(frame.GetData()) != "ping" {
		return status.Error(codes.InvalidArgument, "expected data after open")
	}
	return stream.Send(&pluginhostv1.WebSocketFrame{Value: &pluginhostv1.WebSocketFrame_Data{Data: []byte("pong")}})
}
