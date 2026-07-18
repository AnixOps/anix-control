package pluginhost

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const hostTestChildEnvironment = "ANIX_PLUGINHOST_TEST_CHILD"

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

func TestPluginHostProcess(t *testing.T) {
	if os.Getenv(hostTestChildEnvironment) != "1" {
		return
	}
	socketPath := os.Getenv(hostSocketEnvironment)
	if socketPath == "" {
		t.Fatal("host socket path is required")
	}
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, testHostServer{})
	require.NoError(t, server.Serve(listener))
}

func startTestHostServer(t *testing.T) string {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "host.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, testHostServer{})
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
