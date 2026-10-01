package packagebridge

import (
	"context"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"google.golang.org/grpc"
)

// KernelSubscriberProvider returns the KernelSubscriber server that one
// calling host reaches. The server authorizes the host on every call.
type KernelSubscriberProvider func(HostIdentity) kernelsubscriberv1.KernelSubscriberServer

// KernelSubscriberServer serves KernelSubscriber on the module listener:
// each call is resolved to the bound instance's generation and handed to
// provider with that generation's host identity.
func (b *ModuleBridge) KernelSubscriberServer(provider KernelSubscriberProvider) kernelsubscriberv1.KernelSubscriberServer {
	return &moduleKernelSubscriber{bridge: b, provider: provider}
}

type moduleKernelSubscriber struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	bridge   *ModuleBridge
	provider KernelSubscriberProvider
}

func (m *moduleKernelSubscriber) caller(ctx context.Context) (kernelsubscriberv1.KernelSubscriberServer, error) {
	instance, err := m.bridge.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return m.provider(instance.generation.identity), nil
}

// call resolves the caller and runs one unary method on it.
func call[Request, Response any](m *moduleKernelSubscriber, ctx context.Context, request Request,
	method func(kernelsubscriberv1.KernelSubscriberServer, context.Context, Request) (Response, error)) (Response, error) {
	server, err := m.caller(ctx)
	if err != nil {
		var zero Response
		return zero, err
	}
	return method(server, ctx, request)
}

func (m *moduleKernelSubscriber) ApplyEntitlement(ctx context.Context, request *kernelsubscriberv1.ApplyEntitlementRequest) (*kernelsubscriberv1.ApplyEntitlementResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.ApplyEntitlement)
}

func (m *moduleKernelSubscriber) AdjustEntitlement(ctx context.Context, request *kernelsubscriberv1.AdjustEntitlementRequest) (*kernelsubscriberv1.AdjustEntitlementResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.AdjustEntitlement)
}

func (m *moduleKernelSubscriber) RecordTraffic(ctx context.Context, request *kernelsubscriberv1.RecordTrafficRequest) (*kernelsubscriberv1.RecordTrafficResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.RecordTraffic)
}

func (m *moduleKernelSubscriber) ResetTraffic(ctx context.Context, request *kernelsubscriberv1.ResetTrafficRequest) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.ResetTraffic)
}

func (m *moduleKernelSubscriber) ResetCredentials(ctx context.Context, request *kernelsubscriberv1.ResetCredentialsRequest) (*kernelsubscriberv1.ResetCredentialsResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.ResetCredentials)
}

func (m *moduleKernelSubscriber) AdjustBalance(ctx context.Context, request *kernelsubscriberv1.AdjustBalanceRequest) (*kernelsubscriberv1.AdjustBalanceResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.AdjustBalance)
}

func (m *moduleKernelSubscriber) GetSubscribers(ctx context.Context, request *kernelsubscriberv1.GetSubscribersRequest) (*kernelsubscriberv1.GetSubscribersResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.GetSubscribers)
}

func (m *moduleKernelSubscriber) LookupBySubscriptionToken(ctx context.Context, request *kernelsubscriberv1.LookupBySubscriptionTokenRequest) (*kernelsubscriberv1.LookupBySubscriptionTokenResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.LookupBySubscriptionToken)
}

func (m *moduleKernelSubscriber) ListActiveSubscribers(ctx context.Context, request *kernelsubscriberv1.ListActiveSubscribersRequest) (*kernelsubscriberv1.ListActiveSubscribersResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.ListActiveSubscribers)
}

func (m *moduleKernelSubscriber) GrantSubscriptionGroup(ctx context.Context, request *kernelsubscriberv1.GrantSubscriptionGroupRequest) (*kernelsubscriberv1.GrantSubscriptionGroupResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.GrantSubscriptionGroup)
}

func (m *moduleKernelSubscriber) RevokeSubscriptionGroup(ctx context.Context, request *kernelsubscriberv1.RevokeSubscriptionGroupRequest) (*kernelsubscriberv1.RevokeSubscriptionGroupResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.RevokeSubscriptionGroup)
}

func (m *moduleKernelSubscriber) RemoveSubscriptionGroupMembers(ctx context.Context, request *kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest) (*kernelsubscriberv1.RemoveSubscriptionGroupMembersResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.RemoveSubscriptionGroupMembers)
}

func (m *moduleKernelSubscriber) GetSubscriptionSummary(ctx context.Context, request *kernelsubscriberv1.GetSubscriptionSummaryRequest) (*kernelsubscriberv1.GetSubscriptionSummaryResponse, error) {
	return call(m, ctx, request, kernelsubscriberv1.KernelSubscriberServer.GetSubscriptionSummary)
}

func (m *moduleKernelSubscriber) WatchSubscriberChanges(request *kernelsubscriberv1.WatchSubscriberChangesRequest, stream grpc.ServerStreamingServer[kernelsubscriberv1.SubscriberChange]) error {
	server, err := m.caller(stream.Context())
	if err != nil {
		return err
	}
	return server.WatchSubscriberChanges(request, stream)
}
