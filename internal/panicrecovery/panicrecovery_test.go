package panicrecovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptorConvertsPanicToInternal(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Unary"}

	response, err := interceptor(context.Background(), "request", info, func(context.Context, any) (any, error) {
		panic("unary bug")
	})

	require.Nil(t, response)
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "internal error", status.Convert(err).Message())

	_, err = interceptor(context.Background(), "request", nil, func(context.Context, any) (any, error) { panic(nil) })
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestUnaryServerInterceptorPassesThroughResults(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	handlerErr := status.Error(codes.NotFound, "missing")

	response, err := interceptor(context.Background(), "request", &grpc.UnaryServerInfo{}, func(_ context.Context, request any) (any, error) {
		return request, handlerErr
	})

	require.Equal(t, "request", response)
	require.Equal(t, handlerErr, err)
}

func TestStreamServerInterceptorConvertsPanicToInternal(t *testing.T) {
	interceptor := StreamServerInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	err := interceptor(nil, nil, info, func(any, grpc.ServerStream) error { panic(errors.New("stream bug")) })
	require.Equal(t, codes.Internal, status.Code(err))

	err = interceptor(nil, nil, nil, func(any, grpc.ServerStream) error { panic("stream bug") })
	require.Equal(t, codes.Internal, status.Code(err))

	handlerErr := status.Error(codes.Canceled, "canceled")
	require.Equal(t, handlerErr, interceptor(nil, nil, info, func(any, grpc.ServerStream) error { return handlerErr }))
}

func TestServerOptionsInstallBothInterceptors(t *testing.T) {
	require.Len(t, ServerOptions(), 2)
}

func TestStackIsBounded(t *testing.T) {
	require.NotEmpty(t, Stack())
	require.Equal(t, "short", truncateStack([]byte("short")))
	truncated := truncateStack([]byte(strings.Repeat("x", MaxStackBytes+100)))
	require.Len(t, truncated, MaxStackBytes+len(truncatedSuffix))
	require.True(t, strings.HasSuffix(truncated, truncatedSuffix))
}
