package packagebridge

import (
	"context"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
)

// KernelOrderProvider returns the KernelOrder server that one calling host
// reaches. The server authorizes the host on every call.
type KernelOrderProvider func(HostIdentity) kernelorderv1.KernelOrderServer

// KernelOrderServer serves KernelOrder on the module listener: each call is
// resolved to the bound instance's generation and handed to provider with
// that generation's host identity.
func (b *ModuleBridge) KernelOrderServer(provider KernelOrderProvider) kernelorderv1.KernelOrderServer {
	return &moduleKernelOrder{bridge: b, provider: provider}
}

type moduleKernelOrder struct {
	kernelorderv1.UnimplementedKernelOrderServer
	bridge   *ModuleBridge
	provider KernelOrderProvider
}

func (m *moduleKernelOrder) CompleteOrderPayment(ctx context.Context, request *kernelorderv1.CompleteOrderPaymentRequest) (*kernelorderv1.CompleteOrderPaymentResponse, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return m.provider(instance.generation.identity).CompleteOrderPayment(ctx, request)
}
