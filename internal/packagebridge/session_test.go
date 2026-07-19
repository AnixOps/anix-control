package packagebridge

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestSessionInvokesOnlyItsMintedRouteCapabilityOnce(t *testing.T) {
	invocations := make(chan Call, 1)
	allowlist, err := NewAllowlist(Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(_ context.Context, call Call) (Response, error) {
			invocations <- call
			return Response{
				StatusCode: 200,
				Body:       []byte(`{"code":0,"data":{"token":"issued"}}`),
				Headers:    []Header{{Name: "Retry-After", Value: "1"}},
			}, nil
		},
	})
	require.NoError(t, err)
	session, child, err := NewSession(HostIdentity{
		PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
	}, allowlist)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	deadline := time.Now().Add(time.Second)
	capability, err := session.Mint(Request{
		RequestID: "request-7", RouteID: "identity.auth.login", Method: "POST",
		Body: []byte(`{"email":"u@example.test","password":"secret"}`), Deadline: deadline,
	})
	require.NoError(t, err)
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	response, err := client.Invoke(context.Background(), capability, "identity.auth.login", []byte(`{"email":"u@example.test","password":"secret"}`))
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"code":0,"data":{"token":"issued"}}`, string(response.Body))
	require.Equal(t, []packagebridgesdk.Header{{Name: "Retry-After", Value: "1"}}, response.Headers)

	select {
	case call := <-invocations:
		require.Equal(t, "identity-platform", call.Host.PackageID)
		require.Equal(t, uint64(7), call.Host.Generation)
		require.Equal(t, "identity.auth.login", call.Request.RouteID)
		require.Equal(t, "identity.auth.login", call.Operation)
		require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), call.Payload)
	case <-time.After(time.Second):
		t.Fatal("bridge handler was not invoked")
	}

	_, err = client.Invoke(context.Background(), capability, "identity.auth.login", []byte(`{}`))
	require.ErrorIs(t, err, packagebridgesdk.ErrCapabilityRejected)
}

func TestSessionRejectsOperationOutsideTheMintedRoute(t *testing.T) {
	allowlist, err := NewAllowlist(Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(context.Context, Call) (Response, error) {
			return Response{StatusCode: 200, Body: []byte(`{}`)}, nil
		},
	})
	require.NoError(t, err)
	session, child, err := NewSession(HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7}, allowlist)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	capability, err := session.Mint(Request{RequestID: "request-8", RouteID: "identity.auth.login", Deadline: time.Now().Add(time.Second)})
	require.NoError(t, err)
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	_, err = client.Invoke(context.Background(), capability, "identity.auth.register", []byte(`{}`))
	require.ErrorIs(t, err, packagebridgesdk.ErrCapabilityRejected)
}

func TestSessionCloseInvalidatesOutstandingCapabilities(t *testing.T) {
	allowlist, err := NewAllowlist(Operation{
		PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
		Handler: func(context.Context, Call) (Response, error) {
			return Response{StatusCode: 200, Body: []byte(`{}`)}, nil
		},
	})
	require.NoError(t, err)
	session, child, err := NewSession(HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7}, allowlist)
	require.NoError(t, err)
	capability, err := session.Mint(Request{RequestID: "request-9", RouteID: "identity.auth.login", Deadline: time.Now().Add(time.Second)})
	require.NoError(t, err)
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	require.NoError(t, session.Close())
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Invoke(context.Background(), capability, "identity.auth.login", []byte(`{}`))
	require.Error(t, err)
	require.False(t, errors.Is(err, context.Canceled))
}

func TestSessionRelaysWebSocketThroughMintedCapability(t *testing.T) {
	calls := make(chan Call, 1)
	allowlist, err := NewAllowlist()
	require.NoError(t, err)
	session, child, err := NewSession(
		HostIdentity{PackageID: "machine-telemetry", Version: "4.0.0", Generation: 7},
		allowlist,
		webSocketResolverStub{packageID: "machine-telemetry", routeID: "telemetry.monitor.ws", handler: func(_ context.Context, call Call, stream WebSocketStream) error {
			calls <- call
			frame, err := stream.Recv()
			if err != nil {
				return err
			}
			if string(frame.Data) != "ping" || frame.Close != nil {
				return errors.New("unexpected WebSocket frame")
			}
			return stream.Send(WebSocketFrame{Data: []byte("pong")})
		}},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	capability, err := session.Mint(Request{
		RequestID: "request-ws-7", RouteID: "telemetry.monitor.ws", Method: "GET", Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	stream, closeClient := sessionWebSocketClient(t, child)
	t.Cleanup(closeClient)
	require.NoError(t, stream.Send(&packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Open{Open: &packagebridgev1.WebSocketOpen{
		Capability: capability, Operation: "telemetry.monitor.ws",
	}}}))
	require.NoError(t, stream.Send(&packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Data{Data: []byte("ping")}}))

	frame, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), frame.GetData())
	select {
	case call := <-calls:
		require.Equal(t, "machine-telemetry", call.Host.PackageID)
		require.Equal(t, "telemetry.monitor.ws", call.Request.RouteID)
	case <-time.After(time.Second):
		t.Fatal("WebSocket bridge handler was not invoked")
	}
}

type webSocketResolverStub struct {
	packageID string
	routeID   string
	handler   WebSocketOperationHandler
}

func (s webSocketResolverStub) ResolveWebSocket(packageID, routeID, operation string) (WebSocketOperationHandler, bool) {
	if packageID != s.packageID || routeID != s.routeID || operation != s.routeID || s.handler == nil {
		return nil, false
	}
	return s.handler, true
}

func sessionWebSocketClient(t *testing.T, file *os.File) (packagebridgev1.KernelPackageBridge_OpenWebSocketClient, func()) {
	t.Helper()
	connection, err := net.FileConn(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	var once sync.Once
	grpcConnection, err := grpc.DialContext(
		context.Background(),
		"passthrough:///package-bridge-session-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			used := false
			once.Do(func() { used = true })
			if !used {
				return nil, net.ErrClosed
			}
			return connection, nil
		}),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	stream, err := packagebridgev1.NewKernelPackageBridgeClient(grpcConnection).OpenWebSocket(context.Background())
	require.NoError(t, err)
	return stream, func() { require.NoError(t, grpcConnection.Close()) }
}
