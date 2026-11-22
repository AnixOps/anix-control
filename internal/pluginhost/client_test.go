package pluginhost

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClientDispatchesToThePackage(t *testing.T) {
	client := newPackageClient(fakePackage{}, 0)
	response, err := client.Dispatch(context.Background(), DispatchInput{
		PackageID: "knowledge", Version: "4.0.0", Generation: 7, RequestID: "request-1",
		RouteID: "knowledge.article.list", Method: "GET", PrincipalJSON: []byte(`{"actor_id":7}`),
		Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 202, response.StatusCode)
	require.Equal(t, []byte(`{"transport":"direct"}`), response.Body)
}

// The package sees the request metadata and the original request's scheme
// and host as the kernel resolved them.
func TestClientPassesTheRequestMetadataAndAddress(t *testing.T) {
	host := &recordingPackage{requests: make(chan recordedRequest, 2)}
	client := newPackageClient(host, 0)
	for _, input := range []DispatchInput{
		{Metadata: RequestMetadata{
			Path: "/api/v2/user/knowledge", Query: map[string][]string{"tag": {"stable", "v4"}},
			PathParams: map[string]string{"article_id": "42"},
		}},
		{Metadata: RequestMetadata{Path: "/api/v2/admin/telegram/webhook", Host: "panel.example.test:8443", TLS: true}},
	} {
		input.PackageID, input.Version, input.Generation, input.RouteID = "knowledge", "4.0.0", 7, "knowledge.article.list"
		input.Deadline = time.Now().Add(time.Second)
		_, err := client.Dispatch(context.Background(), input)
		require.NoError(t, err)
	}
	first, second := <-host.requests, <-host.requests
	require.Equal(t, pluginhostsdk.RequestMetadata{
		Path: "/api/v2/user/knowledge", Query: map[string][]string{"tag": {"stable", "v4"}},
		PathParams: map[string]string{"article_id": "42"}, Scheme: "http",
	}, first.Metadata)
	require.Equal(t, "https", second.Metadata.Scheme)
	require.Equal(t, "panel.example.test:8443", second.Metadata.Host)
	deadline, ok := first.Context.Deadline()
	require.True(t, ok, "the request deadline reaches the package")
	require.WithinDuration(t, time.UnixMilli(first.DeadlineUnixMillis), deadline, time.Millisecond)
}

func TestClientClassifiesPackageFailures(t *testing.T) {
	for name, test := range map[string]struct {
		dispatch func() (pluginhostsdk.DispatchResponse, error)
		want     []error
	}{
		"plain error": {func() (pluginhostsdk.DispatchResponse, error) {
			return pluginhostsdk.DispatchResponse{}, errors.New("boom")
		}, []error{ErrHostUnavailable, ErrPackageFailed}},
		"panic": {func() (pluginhostsdk.DispatchResponse, error) { panic("boom") }, []error{ErrHostUnavailable, ErrPackageFailed}},
		"invalid argument": {func() (pluginhostsdk.DispatchResponse, error) {
			return pluginhostsdk.DispatchResponse{}, status.Error(codes.InvalidArgument, "bad")
		}, []error{ErrHostIncompatible}},
		"bad status code": {func() (pluginhostsdk.DispatchResponse, error) {
			return pluginhostsdk.DispatchResponse{StatusCode: 42}, nil
		}, []error{ErrHostIncompatible}},
		"body over limit": {func() (pluginhostsdk.DispatchResponse, error) {
			return pluginhostsdk.DispatchResponse{StatusCode: 200, ResponseBody: []byte(strings.Repeat("x", 11))}, nil
		}, []error{ErrHostUnavailable, ErrResponseTooLarge}},
	} {
		t.Run(name, func(t *testing.T) {
			client := newPackageClient(fakePackage{dispatch: test.dispatch}, 10)
			_, err := client.Dispatch(context.Background(), DispatchInput{Deadline: time.Now().Add(time.Second)})
			for _, want := range test.want {
				require.ErrorIs(t, err, want)
			}
		})
	}
	client := newPackageClient(fakePackage{dispatch: func() (pluginhostsdk.DispatchResponse, error) {
		return pluginhostsdk.DispatchResponse{StatusCode: 200, ResponseBody: []byte(strings.Repeat("x", 10))}, nil
	}}, 10)
	_, err := client.Dispatch(context.Background(), DispatchInput{Deadline: time.Now().Add(time.Second)})
	require.NoError(t, err, "a body at the limit passes")
}

func TestWebSocketRelaySendsTheOpenBeforeData(t *testing.T) {
	host := &webSocketTestPackage{opened: make(chan pluginhostsdk.WebSocketOpen, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	manager := &Supervisor{hosts: map[string]*hostProcess{
		hostKey("machine-telemetry", "4.0.0"): {
			packageID: "machine-telemetry", version: "4.0.0", generation: 9, client: newPackageClient(host, 0), leaseID: "lease-7",
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

	open := <-host.opened
	require.Equal(t, "machine-telemetry", open.PackageID)
	require.Equal(t, "4.0.0", open.PackageVersion)
	require.Equal(t, uint64(9), open.RouteGeneration)
	require.Equal(t, "telemetry.monitor.ws", open.RouteID)
	require.Equal(t, "/api/v2/admin/ws/monitor", open.Metadata.Path)
	require.Equal(t, "http", open.Metadata.Scheme)

	require.NoError(t, relay.Send(WebSocketFrame{Data: []byte("ping")}))
	frame, err := relay.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), frame.Data)
}

// fakePackage answers Dispatch with dispatch, or 202 {"transport":"direct"}.
type fakePackage struct {
	dispatch func() (pluginhostsdk.DispatchResponse, error)
}

func (p fakePackage) Dispatch(context.Context, pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	if p.dispatch != nil {
		return p.dispatch()
	}
	return pluginhostsdk.DispatchResponse{StatusCode: http.StatusAccepted, ResponseBody: []byte(`{"transport":"direct"}`)}, nil
}

func (fakePackage) Migrate(context.Context, pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	return pluginhostsdk.MigrationResponse{}, nil
}

func (fakePackage) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: "lease-7", DetailsJSON: `{}`}, nil
}

func (fakePackage) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}

// recordedRequest is a dispatch request with the context it arrived with.
type recordedRequest struct {
	pluginhostsdk.DispatchRequest
	Context context.Context
}

type recordingPackage struct {
	fakePackage
	requests chan recordedRequest
}

func (p *recordingPackage) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	p.requests <- recordedRequest{DispatchRequest: request, Context: ctx}
	return pluginhostsdk.DispatchResponse{StatusCode: http.StatusAccepted}, nil
}

type webSocketTestPackage struct {
	fakePackage
	opened chan pluginhostsdk.WebSocketOpen
}

func (p *webSocketTestPackage) OpenWebSocket(_ context.Context, open pluginhostsdk.WebSocketOpen, stream pluginhostsdk.WebSocketStream) error {
	p.opened <- open
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	if string(frame.Data) != "ping" {
		return status.Error(codes.InvalidArgument, "expected data after open")
	}
	return stream.Send(pluginhostsdk.WebSocketFrame{Data: []byte("pong")})
}
