package grpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor 日志拦截器
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()

		// 获取客户端地址
		clientAddr := "unknown"
		if p, ok := peer.FromContext(ctx); ok {
			clientAddr = p.Addr.String()
		}

		resp, err := handler(ctx, req)

		// 记录请求
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		// 使用标准日志输出
		slog.Info("grpc unary request", "component", "grpc", "method", info.FullMethod, "addr", clientAddr, "duration", duration.Milliseconds(), "code", code.String())

		return resp, err
	}
}

// StreamLoggingInterceptor 流式日志拦截器
func StreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		// 获取客户端地址
		clientAddr := "unknown"
		if p, ok := peer.FromContext(ss.Context()); ok {
			clientAddr = p.Addr.String()
		}

		err := handler(srv, ss)

		// 记录请求
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		slog.Info("grpc stream request", "component", "grpc", "method", info.FullMethod, "addr", clientAddr, "duration", duration.Milliseconds(), "code", code.String())

		return err
	}
}
