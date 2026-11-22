package packagebridge

import (
	"context"
	"errors"
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
// only on the generation the call arrived on (its session), without consuming it. Contracts that act
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
