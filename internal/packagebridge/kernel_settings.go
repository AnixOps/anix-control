package packagebridge

import (
	"context"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
)

// KernelSettingsProvider returns the KernelSettings server that one calling
// host reaches. The server authorizes the host on every call.
type KernelSettingsProvider func(HostIdentity) kernelsettingsv1.KernelSettingsServer

// KernelSettingsServer serves KernelSettings on the module listener: each
// call is resolved to the bound instance's generation and handed to
// provider with that generation's host identity.
func (b *ModuleBridge) KernelSettingsServer(provider KernelSettingsProvider) kernelsettingsv1.KernelSettingsServer {
	return &moduleKernelSettings{bridge: b, provider: provider}
}

type moduleKernelSettings struct {
	kernelsettingsv1.UnimplementedKernelSettingsServer
	bridge   *ModuleBridge
	provider KernelSettingsProvider
}

// callSettings resolves the caller and runs one unary method on it.
func callSettings[Request, Response any](m *moduleKernelSettings, ctx context.Context, request Request,
	method func(kernelsettingsv1.KernelSettingsServer, context.Context, Request) (Response, error)) (Response, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		var zero Response
		return zero, err
	}
	return method(m.provider(instance.generation.identity), ctx, request)
}

func (m *moduleKernelSettings) GetSettings(ctx context.Context, request *kernelsettingsv1.GetSettingsRequest) (*kernelsettingsv1.GetSettingsResponse, error) {
	return callSettings(m, ctx, request, kernelsettingsv1.KernelSettingsServer.GetSettings)
}

func (m *moduleKernelSettings) PutSettings(ctx context.Context, request *kernelsettingsv1.PutSettingsRequest) (*kernelsettingsv1.PutSettingsResponse, error) {
	return callSettings(m, ctx, request, kernelsettingsv1.KernelSettingsServer.PutSettings)
}

func (m *moduleKernelSettings) DeleteSettings(ctx context.Context, request *kernelsettingsv1.DeleteSettingsRequest) (*kernelsettingsv1.DeleteSettingsResponse, error) {
	return callSettings(m, ctx, request, kernelsettingsv1.KernelSettingsServer.DeleteSettings)
}
