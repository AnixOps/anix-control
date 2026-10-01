package packagebridge

import (
	"context"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"google.golang.org/grpc"
)

// KernelNodeOpsProvider returns the KernelNodeOps server that one calling
// host reaches. The server authorizes the host on every call.
type KernelNodeOpsProvider func(HostIdentity) kernelnodeopsv1.KernelNodeOpsServer

// KernelNodeOpsServer serves KernelNodeOps on the module listener: each
// call, unary or streaming, is resolved to the bound instance's generation
// and handed to provider with that generation's host identity.
func (b *ModuleBridge) KernelNodeOpsServer(provider KernelNodeOpsProvider) kernelnodeopsv1.KernelNodeOpsServer {
	return &moduleKernelNodeOps{bridge: b, provider: provider}
}

type moduleKernelNodeOps struct {
	kernelnodeopsv1.UnimplementedKernelNodeOpsServer
	bridge   *ModuleBridge
	provider KernelNodeOpsProvider
}

// caller resolves the bound instance's server, and a context that carries
// its generation, on which request bindings are verified.
func (m *moduleKernelNodeOps) caller(ctx context.Context) (kernelnodeopsv1.KernelNodeOpsServer, context.Context, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		return nil, ctx, err
	}
	return m.provider(instance.generation.identity), withGeneration(ctx, instance.generation), nil
}

// callNodeOps resolves the caller and runs one unary method on it.
func callNodeOps[Request, Response any](m *moduleKernelNodeOps, ctx context.Context, request Request,
	method func(kernelnodeopsv1.KernelNodeOpsServer, context.Context, Request) (Response, error)) (Response, error) {
	server, ctx, err := m.caller(ctx)
	if err != nil {
		var zero Response
		return zero, err
	}
	return method(server, ctx, request)
}

func (m *moduleKernelNodeOps) SubmitOperation(ctx context.Context, request *kernelnodeopsv1.SubmitOperationRequest) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.SubmitOperation)
}

func (m *moduleKernelNodeOps) GetOperation(ctx context.Context, request *kernelnodeopsv1.GetOperationRequest) (*kernelnodeopsv1.GetOperationResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.GetOperation)
}

func (m *moduleKernelNodeOps) ListOperations(ctx context.Context, request *kernelnodeopsv1.ListOperationsRequest) (*kernelnodeopsv1.ListOperationsResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.ListOperations)
}

func (m *moduleKernelNodeOps) WatchOperations(request *kernelnodeopsv1.WatchOperationsRequest, stream grpc.ServerStreamingServer[kernelnodeopsv1.OperationEvent]) error {
	server, _, err := m.caller(stream.Context())
	if err != nil {
		return err
	}
	return server.WatchOperations(request, stream)
}

func (m *moduleKernelNodeOps) CancelOperation(ctx context.Context, request *kernelnodeopsv1.CancelOperationRequest) (*kernelnodeopsv1.CancelOperationResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.CancelOperation)
}

func (m *moduleKernelNodeOps) ValidateNodeConfig(ctx context.Context, request *kernelnodeopsv1.ValidateNodeConfigRequest) (*kernelnodeopsv1.ValidateNodeConfigResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.ValidateNodeConfig)
}

func (m *moduleKernelNodeOps) ListAgentSessions(ctx context.Context, request *kernelnodeopsv1.ListAgentSessionsRequest) (*kernelnodeopsv1.ListAgentSessionsResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.ListAgentSessions)
}

func (m *moduleKernelNodeOps) GetAgentSession(ctx context.Context, request *kernelnodeopsv1.GetAgentSessionRequest) (*kernelnodeopsv1.GetAgentSessionResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.GetAgentSession)
}

func (m *moduleKernelNodeOps) GetAgentMonitor(ctx context.Context, request *kernelnodeopsv1.GetAgentMonitorRequest) (*kernelnodeopsv1.GetAgentMonitorResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.GetAgentMonitor)
}

func (m *moduleKernelNodeOps) GetCapabilities(ctx context.Context, request *kernelnodeopsv1.GetCapabilitiesRequest) (*kernelnodeopsv1.GetCapabilitiesResponse, error) {
	return callNodeOps(m, ctx, request, kernelnodeopsv1.KernelNodeOpsServer.GetCapabilities)
}
