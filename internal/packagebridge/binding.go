package packagebridge

import (
	"context"
	"errors"

	"google.golang.org/grpc"
)

// ErrBindingRejected is a request binding that names no live request of
// the calling host: an unknown, consumed, revoked or expired capability, or
// one minted for another host generation.
var ErrBindingRejected = errors.New("package bridge request binding rejected")

type generationContextKey struct{}

// withGeneration records on a call's context the generation it arrived on.
func withGeneration(ctx context.Context, generation *GenerationSession) context.Context {
	return context.WithValue(ctx, generationContextKey{}, generation)
}

// BoundRequest returns the live request a request binding names: a
// capability the kernel minted for a dispatch to the calling host, found
// only on the generation the call arrived on (its local session or its
// module instance's generation), without consuming it. Contracts that act
// for a v2 request (KernelNodeOps) verify their RequestBinding with it.
func BoundRequest(ctx context.Context, raw []byte) (HostIdentity, Request, error) {
	generation, _ := ctx.Value(generationContextKey{}).(*GenerationSession)
	if generation == nil {
		return HostIdentity{}, Request{}, ErrBindingRejected
	}
	request, ok := generation.Peek(raw)
	if !ok {
		return HostIdentity{}, Request{}, ErrBindingRejected
	}
	return generation.Identity(), request, nil
}

// serverOptions put the generation on the context of every call its local
// session serves.
func (s *GenerationSession) serverOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(withGeneration(ctx, s), request)
		}),
		grpc.ChainStreamInterceptor(func(server any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(server, &generationStream{ServerStream: stream, ctx: withGeneration(stream.Context(), s)})
		}),
	}
}

type generationStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *generationStream) Context() context.Context { return s.ctx }
