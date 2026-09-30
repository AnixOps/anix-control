package pluginhostsdk

import (
	"context"
	"log"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxPanicStackBytes bounds the stack trace logged for one recovered panic.
const maxPanicStackBytes = 16 << 10

const truncatedStackSuffix = "\n... stack truncated ..."

// RecoveryServerOptions returns gRPC server options whose unary and stream
// interceptors turn a panic anywhere in a host RPC into codes.Internal
// instead of terminating the package host process. Hosts pass them to
// grpc.NewServer; chain any further interceptors after them so recovery stays
// outermost.
func RecoveryServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(recoveryUnaryServerInterceptor),
		grpc.ChainStreamInterceptor(recoveryStreamServerInterceptor),
	}
}

func recoveryUnaryServerInterceptor(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
	method := "unknown"
	if info != nil {
		method = info.FullMethod
	}
	defer recoverPanic("gRPC "+method, "internal error", &err)
	return handler(ctx, request)
}

func recoveryStreamServerInterceptor(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	method := "unknown"
	if info != nil {
		method = info.FullMethod
	}
	defer recoverPanic("gRPC "+method, "internal error", &err)
	return handler(server, stream)
}

// recoverPanic must be deferred directly. It logs a recovered panic with a
// bounded stack trace and replaces *err with codes.Internal carrying message;
// the panic value and stack are never sent to the kernel.
func recoverPanic(operation, message string, err *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	log.Printf("panic recovered in package host %s: %v\n%s", operation, recovered, panicStack())
	*err = status.Error(codes.Internal, message)
}

func panicStack() string {
	return truncatePanicStack(debug.Stack())
}

func truncatePanicStack(stack []byte) string {
	if len(stack) <= maxPanicStackBytes {
		return string(stack)
	}
	return string(stack[:maxPanicStackBytes]) + truncatedStackSuffix
}
