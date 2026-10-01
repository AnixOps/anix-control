package packagebridge

import (
	"context"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
)

// KernelTelemetryProvider returns the KernelTelemetry server that one
// calling host reaches. The server authorizes the host on every call.
type KernelTelemetryProvider func(HostIdentity) kerneltelemetryv1.KernelTelemetryServer

// KernelTelemetryServer serves KernelTelemetry on the module listener: each
// call is resolved to the bound instance's generation and handed to
// provider with that generation's host identity.
func (b *ModuleBridge) KernelTelemetryServer(provider KernelTelemetryProvider) kerneltelemetryv1.KernelTelemetryServer {
	return &moduleKernelTelemetry{bridge: b, provider: provider}
}

type moduleKernelTelemetry struct {
	kerneltelemetryv1.UnimplementedKernelTelemetryServer
	bridge   *ModuleBridge
	provider KernelTelemetryProvider
}

func (m *moduleKernelTelemetry) GetDashboard(ctx context.Context, request *kerneltelemetryv1.GetDashboardRequest) (*kerneltelemetryv1.GetDashboardResponse, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return m.provider(instance.generation.identity).GetDashboard(ctx, request)
}
