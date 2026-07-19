package pluginhostsdk

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	testPackageID      = "knowledge"
	testPackageVersion = "4.0.0"
	testGeneration     = uint64(7)
)

func TestServerDispatchRejectsInvalidEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*pluginhostv1.DispatchRequest)
		wantCode codes.Code
	}{
		{
			name: "package ID mismatch",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.PackageId = "order"
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "package version mismatch",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.PackageVersion = "4.0.1"
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "zero route generation",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.RouteGeneration = 0
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "expired deadline",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.DeadlineUnixMillis = time.Now().Add(-time.Second).UnixMilli()
			},
			wantCode: codes.DeadlineExceeded,
		},
		{
			name: "invalid principal JSON",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.PrincipalJson = []byte(`{"subject":`)
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "invalid request metadata",
			mutate: func(request *pluginhostv1.DispatchRequest) {
				request.RequestMetadataJson = []byte(`{"path":`)
			},
			wantCode: codes.InvalidArgument,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			packageCalls := 0
			server := newTestServer(t, &testPackage{
				dispatch: func(context.Context, DispatchRequest) (DispatchResponse, error) {
					packageCalls++
					return DispatchResponse{StatusCode: http.StatusNoContent}, nil
				},
			}, 16)
			request := validDispatchRequest()
			test.mutate(request)

			response, err := server.Dispatch(context.Background(), request)

			require.Nil(t, response)
			require.Equal(t, test.wantCode, status.Code(err))
			require.Zero(t, packageCalls)
		})
	}
}

func TestServerDispatchRejectsInvalidPackageResponse(t *testing.T) {
	tests := []struct {
		name     string
		response DispatchResponse
		limit    int
		wantCode codes.Code
	}{
		{
			name:     "status below HTTP range",
			response: DispatchResponse{StatusCode: 99},
			limit:    16,
			wantCode: codes.FailedPrecondition,
		},
		{
			name:     "status above HTTP range",
			response: DispatchResponse{StatusCode: 600},
			limit:    16,
			wantCode: codes.FailedPrecondition,
		},
		{
			name: "response body exceeds host limit",
			response: DispatchResponse{
				StatusCode:   http.StatusOK,
				ResponseBody: []byte("oversized"),
			},
			limit:    4,
			wantCode: codes.ResourceExhausted,
		},
		{
			name: "response headers exceed host limit",
			response: DispatchResponse{
				StatusCode: http.StatusOK,
				Headers: []Header{{
					Name:  "x-package-header",
					Value: "oversized",
				}},
			},
			limit:    4,
			wantCode: codes.ResourceExhausted,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t, &testPackage{
				dispatch: func(context.Context, DispatchRequest) (DispatchResponse, error) {
					return test.response, nil
				},
			}, test.limit)

			response, err := server.Dispatch(context.Background(), validDispatchRequest())

			require.Nil(t, response)
			require.Equal(t, test.wantCode, status.Code(err))
		})
	}
}

func TestServerValidatesLifecycleEnvelopes(t *testing.T) {
	server := newTestServer(t, &testPackage{}, 16)

	_, err := server.Migrate(context.Background(), &pluginhostv1.MigrationRequest{
		PackageId:       "order",
		PackageVersion:  testPackageVersion,
		RouteGeneration: testGeneration,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.Health(context.Background(), &pluginhostv1.HealthRequest{})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.Drain(context.Background(), &pluginhostv1.DrainRequest{
		RouteGeneration:    testGeneration,
		DeadlineUnixMillis: time.Now().Add(-time.Second).UnixMilli(),
	})
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
}

func TestServerForwardsValidatedRequestsToPackage(t *testing.T) {
	packageServer := &testPackage{
		dispatch: func(_ context.Context, request DispatchRequest) (DispatchResponse, error) {
			require.Equal(t, testPackageID, request.PackageID)
			require.Equal(t, "/api/v2/user/knowledge", request.Metadata.Path)
			require.Equal(t, []string{"stable", "v4"}, request.Metadata.Query["tag"])
			require.Equal(t, map[string]string{"article_id": "42"}, request.Metadata.PathParams)
			return DispatchResponse{StatusCode: http.StatusCreated, OperationID: "operation-42"}, nil
		},
		migrate: func(_ context.Context, request MigrationRequest) (MigrationResponse, error) {
			require.Equal(t, testGeneration, request.RouteGeneration)
			return MigrationResponse{Checkpoint: "checkpoint-2", Complete: true}, nil
		},
		health: func(context.Context) (HealthResponse, error) {
			return HealthResponse{Healthy: true, LeaseID: "lease-42"}, nil
		},
		drain: func(context.Context) (DrainResponse, error) {
			return DrainResponse{Drained: true}, nil
		},
	}
	server := newTestServer(t, packageServer, 1024)

	dispatchResponse, err := server.Dispatch(context.Background(), validDispatchRequest())
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, int(dispatchResponse.StatusCode))
	require.Equal(t, "operation-42", dispatchResponse.OperationId)

	migrationResponse, err := server.Migrate(context.Background(), &pluginhostv1.MigrationRequest{
		PackageId:       testPackageID,
		PackageVersion:  testPackageVersion,
		MigrationId:     "migration-42",
		RouteGeneration: testGeneration,
	})
	require.NoError(t, err)
	require.True(t, migrationResponse.Complete)
	require.Equal(t, "checkpoint-2", migrationResponse.Checkpoint)

	healthResponse, err := server.Health(context.Background(), &pluginhostv1.HealthRequest{RouteGeneration: testGeneration})
	require.NoError(t, err)
	require.True(t, healthResponse.Healthy)
	require.Equal(t, "lease-42", healthResponse.LeaseId)

	drainResponse, err := server.Drain(context.Background(), &pluginhostv1.DrainRequest{
		RouteGeneration:    testGeneration,
		DeadlineUnixMillis: time.Now().Add(time.Minute).UnixMilli(),
	})
	require.NoError(t, err)
	require.True(t, drainResponse.Drained)
}

func TestServerOpenWebSocketForwardsVerifiedOpenBeforeData(t *testing.T) {
	packageServer := &testPackage{
		openWebSocket: func(_ context.Context, open WebSocketOpen, stream WebSocketStream) error {
			require.Equal(t, testPackageID, open.PackageID)
			require.Equal(t, testPackageVersion, open.PackageVersion)
			require.Equal(t, testGeneration, open.RouteGeneration)
			require.Equal(t, "telemetry.monitor.ws", open.RouteID)
			require.Equal(t, []byte(`{"subject":"operator-42"}`), open.PrincipalJSON)

			frame, err := stream.Recv()
			require.NoError(t, err)
			require.Equal(t, []byte("ping"), frame.Data)
			return stream.Send(WebSocketFrame{Data: []byte("pong")})
		},
	}
	server := newTestServer(t, packageServer, 16)
	stream := &testWebSocketServerStream{frames: []*pluginhostv1.WebSocketFrame{
		{Value: &pluginhostv1.WebSocketFrame_Open{Open: &pluginhostv1.WebSocketOpen{
			PackageId:          testPackageID,
			PackageVersion:     testPackageVersion,
			RouteGeneration:    testGeneration,
			RouteId:            "telemetry.monitor.ws",
			PrincipalJson:      []byte(`{"subject":"operator-42"}`),
			RequestId:          "request-42",
			IdempotencyKey:     "idempotency-42",
			DeadlineUnixMillis: time.Now().Add(time.Minute).UnixMilli(),
		}}},
		{Value: &pluginhostv1.WebSocketFrame_Data{Data: []byte("ping")}},
	}}

	err := server.OpenWebSocket(stream)

	require.NoError(t, err)
	require.Len(t, stream.sent, 1)
	require.Equal(t, []byte("pong"), stream.sent[0].GetData())
}

func newTestServer(t *testing.T, packageServer Package, maxResponseBytes int) *Server {
	t.Helper()

	server, err := NewServer(ServerConfig{
		PackageID:        testPackageID,
		PackageVersion:   testPackageVersion,
		MaxResponseBytes: maxResponseBytes,
	}, packageServer)
	require.NoError(t, err)
	return server
}

func validDispatchRequest() *pluginhostv1.DispatchRequest {
	return &pluginhostv1.DispatchRequest{
		PackageId:           testPackageID,
		PackageVersion:      testPackageVersion,
		RouteGeneration:     testGeneration,
		RequestId:           "request-42",
		IdempotencyKey:      "idempotency-42",
		RouteId:             "knowledge.article.list",
		Method:              http.MethodGet,
		PrincipalJson:       []byte(`{"subject":"operator-42","roles":["admin"]}`),
		RequestMetadataJson: []byte(`{"path":"/api/v2/user/knowledge","query":{"tag":["stable","v4"]},"path_params":{"article_id":"42"}}`),
		DeadlineUnixMillis:  time.Now().Add(time.Minute).UnixMilli(),
	}
}

type testPackage struct {
	dispatch      func(context.Context, DispatchRequest) (DispatchResponse, error)
	migrate       func(context.Context, MigrationRequest) (MigrationResponse, error)
	health        func(context.Context) (HealthResponse, error)
	drain         func(context.Context) (DrainResponse, error)
	openWebSocket func(context.Context, WebSocketOpen, WebSocketStream) error
}

func (p *testPackage) Dispatch(ctx context.Context, request DispatchRequest) (DispatchResponse, error) {
	if p.dispatch == nil {
		return DispatchResponse{StatusCode: http.StatusNoContent}, nil
	}
	return p.dispatch(ctx, request)
}

func (p *testPackage) Migrate(ctx context.Context, request MigrationRequest) (MigrationResponse, error) {
	if p.migrate == nil {
		return MigrationResponse{}, nil
	}
	return p.migrate(ctx, request)
}

func (p *testPackage) Health(ctx context.Context) (HealthResponse, error) {
	if p.health == nil {
		return HealthResponse{Healthy: true}, nil
	}
	return p.health(ctx)
}

func (p *testPackage) Drain(ctx context.Context) (DrainResponse, error) {
	if p.drain == nil {
		return DrainResponse{Drained: true}, nil
	}
	return p.drain(ctx)
}

func (p *testPackage) OpenWebSocket(ctx context.Context, open WebSocketOpen, stream WebSocketStream) error {
	if p.openWebSocket == nil {
		return nil
	}
	return p.openWebSocket(ctx, open, stream)
}

type testWebSocketServerStream struct {
	grpc.ServerStream
	frames []*pluginhostv1.WebSocketFrame
	sent   []*pluginhostv1.WebSocketFrame
}

func (s *testWebSocketServerStream) Send(frame *pluginhostv1.WebSocketFrame) error {
	s.sent = append(s.sent, frame)
	return nil
}

func (s *testWebSocketServerStream) Recv() (*pluginhostv1.WebSocketFrame, error) {
	if len(s.frames) == 0 {
		return nil, io.EOF
	}
	frame := s.frames[0]
	s.frames = s.frames[1:]
	return frame, nil
}

func (s *testWebSocketServerStream) Context() context.Context { return context.Background() }

func (s *testWebSocketServerStream) SetHeader(metadata.MD) error { return nil }

func (s *testWebSocketServerStream) SendHeader(metadata.MD) error { return nil }

func (s *testWebSocketServerStream) SetTrailer(metadata.MD) {}

func (s *testWebSocketServerStream) SendMsg(any) error { return nil }

func (s *testWebSocketServerStream) RecvMsg(any) error { return io.EOF }
