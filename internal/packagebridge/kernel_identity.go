package packagebridge

import (
	"context"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
)

// KernelIdentityProvider returns the KernelIdentity server that one calling
// host reaches. The server authorizes the host on every call.
type KernelIdentityProvider func(HostIdentity) kernelidentityv1.KernelIdentityServer

// KernelIdentityServer serves KernelIdentity on the module listener: each
// call is resolved to the bound instance's generation, as the package bridge
// RPCs are, and handed to provider with that generation's host identity.
func (b *ModuleBridge) KernelIdentityServer(provider KernelIdentityProvider) kernelidentityv1.KernelIdentityServer {
	return &moduleKernelIdentity{bridge: b, provider: provider}
}

type moduleKernelIdentity struct {
	kernelidentityv1.UnimplementedKernelIdentityServer
	bridge   *ModuleBridge
	provider KernelIdentityProvider
}

func (m *moduleKernelIdentity) caller(ctx context.Context) (kernelidentityv1.KernelIdentityServer, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return m.provider(instance.generation.identity), nil
}

func (m *moduleKernelIdentity) CreateSubscriber(ctx context.Context, request *kernelidentityv1.CreateSubscriberRequest) (*kernelidentityv1.CreateSubscriberResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.CreateSubscriber(ctx, request)
}

func (m *moduleKernelIdentity) UpdateSubscriber(ctx context.Context, request *kernelidentityv1.UpdateSubscriberRequest) (*kernelidentityv1.UpdateSubscriberResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.UpdateSubscriber(ctx, request)
}

func (m *moduleKernelIdentity) ApplyAccountProjection(ctx context.Context, request *kernelidentityv1.ApplyAccountProjectionRequest) (*kernelidentityv1.ApplyAccountProjectionResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.ApplyAccountProjection(ctx, request)
}

func (m *moduleKernelIdentity) DeleteSubscriber(ctx context.Context, request *kernelidentityv1.DeleteSubscriberRequest) (*kernelidentityv1.DeleteSubscriberResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.DeleteSubscriber(ctx, request)
}

func (m *moduleKernelIdentity) PublishRevocation(ctx context.Context, request *kernelidentityv1.PublishRevocationRequest) (*kernelidentityv1.PublishRevocationResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.PublishRevocation(ctx, request)
}

func (m *moduleKernelIdentity) ResolveActorAccess(ctx context.Context, request *kernelidentityv1.ResolveActorAccessRequest) (*kernelidentityv1.ResolveActorAccessResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.ResolveActorAccess(ctx, request)
}

func (m *moduleKernelIdentity) GetIdentitySettings(ctx context.Context, request *kernelidentityv1.GetIdentitySettingsRequest) (*kernelidentityv1.GetIdentitySettingsResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.GetIdentitySettings(ctx, request)
}

func (m *moduleKernelIdentity) GetSubscriber(ctx context.Context, request *kernelidentityv1.GetSubscriberRequest) (*kernelidentityv1.GetSubscriberResponse, error) {
	server, err := m.caller(ctx)
	if err != nil {
		return nil, err
	}
	return server.GetSubscriber(ctx, request)
}
