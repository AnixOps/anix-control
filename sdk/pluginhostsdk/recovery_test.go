package pluginhostsdk

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func requireInternal(t *testing.T, err error, message string) {
	t.Helper()
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, message, status.Convert(err).Message())
}

func TestServerRecoversPackagePanics(t *testing.T) {
	packageServer := &testPackage{
		dispatch: func(context.Context, DispatchRequest) (DispatchResponse, error) { panic("dispatch bug") },
		migrate:  func(context.Context, MigrationRequest) (MigrationResponse, error) { panic("migrate bug") },
		health:   func(context.Context) (HealthResponse, error) { panic("health bug") },
		drain:    func(context.Context) (DrainResponse, error) { panic(nil) },
		openWebSocket: func(context.Context, WebSocketOpen, WebSocketStream) error {
			panic("WebSocket bug")
		},
	}
	server := newTestServer(t, packageServer, 1024)

	dispatchResponse, err := server.Dispatch(context.Background(), validDispatchRequest())
	require.Nil(t, dispatchResponse)
	requireInternal(t, err, "package dispatch panicked")

	migrationResponse, err := server.Migrate(context.Background(), &pluginhostv1.MigrationRequest{
		PackageId: testPackageID, PackageVersion: testPackageVersion, MigrationId: "migration-42", RouteGeneration: testGeneration,
	})
	require.Nil(t, migrationResponse)
	requireInternal(t, err, "package migrate panicked")

	healthResponse, err := server.Health(context.Background(), &pluginhostv1.HealthRequest{RouteGeneration: testGeneration})
	require.Nil(t, healthResponse)
	requireInternal(t, err, "package health panicked")

	drainResponse, err := server.Drain(context.Background(), &pluginhostv1.DrainRequest{
		RouteGeneration: testGeneration, DeadlineUnixMillis: time.Now().Add(time.Minute).UnixMilli(),
	})
	require.Nil(t, drainResponse)
	requireInternal(t, err, "package drain panicked")

	err = server.OpenWebSocket(&testWebSocketServerStream{frames: []*pluginhostv1.WebSocketFrame{
		{Value: &pluginhostv1.WebSocketFrame_Open{Open: &pluginhostv1.WebSocketOpen{
			PackageId: testPackageID, PackageVersion: testPackageVersion, RouteGeneration: testGeneration,
			RouteId: "telemetry.monitor.ws", PrincipalJson: []byte(`{}`), RequestId: "request-42",
			DeadlineUnixMillis: time.Now().Add(time.Minute).UnixMilli(),
		}}},
	}})
	requireInternal(t, err, "package open WebSocket panicked")
}

// panickingHostServer bypasses Server so only the interceptors can recover.
type panickingHostServer struct {
	pluginhostv1.UnimplementedControlPackageHostServer
}

func (panickingHostServer) Health(context.Context, *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	panic("raw unary bug")
}

func (panickingHostServer) OpenWebSocket(pluginhostv1.ControlPackageHost_OpenWebSocketServer) error {
	panic("raw stream bug")
}

func TestRecoveryServerOptionsConvertPanicsToInternal(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(RecoveryServerOptions()...)
	pluginhostv1.RegisterControlPackageHostServer(server, panickingHostServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///plugin-host-recovery",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	client := pluginhostv1.NewControlPackageHostClient(connection)

	for range 2 {
		_, err = client.Health(context.Background(), &pluginhostv1.HealthRequest{RouteGeneration: testGeneration})
		requireInternal(t, err, "internal error")
	}

	stream, err := client.OpenWebSocket(context.Background())
	require.NoError(t, err)
	_, err = stream.Recv()
	requireInternal(t, err, "internal error")
}

func TestPanicStackIsBounded(t *testing.T) {
	require.NotEmpty(t, panicStack())
	require.Equal(t, "short", truncatePanicStack([]byte("short")))
	truncated := truncatePanicStack([]byte(strings.Repeat("x", maxPanicStackBytes+100)))
	require.Len(t, truncated, maxPanicStackBytes+len(truncatedStackSuffix))
	require.True(t, strings.HasSuffix(truncated, truncatedStackSuffix))
}
