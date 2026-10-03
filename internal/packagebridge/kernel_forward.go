package packagebridge

import (
	"context"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// KernelForwardProvider returns the ForwardControl server that one calling
// host reaches. The server authorizes the host on every call.
type KernelForwardProvider func(HostIdentity) forwardv1.ForwardControlServer

// KernelForwardServer serves ForwardControl on the module listener: each
// call is resolved to the bound instance's generation and handed to
// provider with that generation's host identity.
func (b *ModuleBridge) KernelForwardServer(provider KernelForwardProvider) forwardv1.ForwardControlServer {
	return &moduleKernelForward{bridge: b, provider: provider}
}

type moduleKernelForward struct {
	forwardv1.UnimplementedForwardControlServer
	bridge   *ModuleBridge
	provider KernelForwardProvider
}

// callForward resolves the caller and runs one unary method on it.
func callForward[Request, Response any](m *moduleKernelForward, ctx context.Context, request Request,
	method func(forwardv1.ForwardControlServer, context.Context, Request) (Response, error)) (Response, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		var zero Response
		return zero, err
	}
	return method(m.provider(instance.generation.identity), ctx, request)
}

func (m *moduleKernelForward) CreateRoute(ctx context.Context, request *forwardv1.CreateRouteRequest) (*forwardv1.CreateRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.CreateRoute)
}

func (m *moduleKernelForward) UpdateRoute(ctx context.Context, request *forwardv1.UpdateRouteRequest) (*forwardv1.UpdateRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.UpdateRoute)
}

func (m *moduleKernelForward) DeleteRoute(ctx context.Context, request *forwardv1.DeleteRouteRequest) (*forwardv1.DeleteRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.DeleteRoute)
}

func (m *moduleKernelForward) GetRoute(ctx context.Context, request *forwardv1.GetRouteRequest) (*forwardv1.GetRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.GetRoute)
}

func (m *moduleKernelForward) ListRoutes(ctx context.Context, request *forwardv1.ListRoutesRequest) (*forwardv1.ListRoutesResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.ListRoutes)
}

func (m *moduleKernelForward) PlanRoute(ctx context.Context, request *forwardv1.PlanRouteRequest) (*forwardv1.PlanRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.PlanRoute)
}

func (m *moduleKernelForward) GetRouteStats(ctx context.Context, request *forwardv1.GetRouteStatsRequest) (*forwardv1.GetRouteStatsResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.GetRouteStats)
}

func (m *moduleKernelForward) GetRouteHealth(ctx context.Context, request *forwardv1.GetRouteHealthRequest) (*forwardv1.GetRouteHealthResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.GetRouteHealth)
}

func (m *moduleKernelForward) DiagnoseRoute(ctx context.Context, request *forwardv1.DiagnoseRouteRequest) (*forwardv1.DiagnoseRouteResponse, error) {
	return callForward(m, ctx, request, forwardv1.ForwardControlServer.DiagnoseRoute)
}
