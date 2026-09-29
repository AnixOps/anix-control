// Package panicrecovery converts panics on kernel request paths into ordinary
// errors so one faulty handler cannot terminate the kernel process.
package panicrecovery

import (
	"context"
	"log"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MaxStackBytes bounds the stack trace written for one recovered panic.
const MaxStackBytes = 16 << 10

const truncatedSuffix = "\n... stack truncated ..."

// Stack returns the calling goroutine's stack trace, truncated to
// MaxStackBytes.
func Stack() string {
	return truncateStack(debug.Stack())
}

func truncateStack(stack []byte) string {
	if len(stack) <= MaxStackBytes {
		return string(stack)
	}
	return string(stack[:MaxStackBytes]) + truncatedSuffix
}

// InternalError is the status returned to a gRPC caller after a recovered
// panic. It deliberately carries no panic value or stack.
func InternalError() error {
	return status.Error(codes.Internal, "internal error")
}

// UnaryServerInterceptor recovers a panic raised by the unary handler or by
// any interceptor chained after it, logs it with a bounded stack trace, and
// returns codes.Internal.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered in gRPC unary %s: %v\n%s", fullMethod(info), recovered, Stack())
				response, err = nil, InternalError()
			}
		}()
		return handler(ctx, req)
	}
}

// StreamServerInterceptor is the streaming counterpart of
// UnaryServerInterceptor.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				method := "unknown"
				if info != nil {
					method = info.FullMethod
				}
				log.Printf("panic recovered in gRPC stream %s: %v\n%s", method, recovered, Stack())
				err = InternalError()
			}
		}()
		return handler(srv, stream)
	}
}

// ServerOptions installs only the recovery interceptors. Servers that chain
// other interceptors should place UnaryServerInterceptor and
// StreamServerInterceptor first in their chains instead.
func ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(StreamServerInterceptor()),
	}
}

func fullMethod(info *grpc.UnaryServerInfo) string {
	if info == nil {
		return "unknown"
	}
	return info.FullMethod
}
