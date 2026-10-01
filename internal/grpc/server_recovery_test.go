package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type panickingHealthServer struct {
	healthpb.UnimplementedHealthServer
}

func (panickingHealthServer) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	panic("unary handler bug")
}

func (panickingHealthServer) Watch(*healthpb.HealthCheckRequest, healthpb.Health_WatchServer) error {
	panic("stream handler bug")
}

func TestNodeServerInterceptorChainsRecoverHandlerPanics(t *testing.T) {
	unary, stream := NewServer(&ServerConfig{APIToken: "test"}).interceptorChains()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))
	healthpb.RegisterHealthServer(server, panickingHealthServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	connection, err := grpc.NewClient("passthrough:///node-server-recovery",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	client := healthpb.NewHealthClient(connection)
	// The global token passes the auth interceptor and the call reaches the
	// panicking handler.
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer test")

	for range 2 {
		_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		require.Equal(t, codes.Internal, status.Code(err))
		require.Equal(t, "internal error", status.Convert(err).Message())
	}

	watch, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = watch.Recv()
	require.Equal(t, codes.Internal, status.Code(err))
}
