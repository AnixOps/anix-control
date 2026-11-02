package control

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/knowledge/native"
	"github.com/stretchr/testify/require"
)

type bridgeStub struct{ operation string }

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) Invoke(_ context.Context, _ []byte, operation string, _ []byte) (packagebridgesdk.Response, error) {
	s.operation = operation
	return packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":null}`)}, nil
}

// Every route runs natively from the start and never reaches the kernel's
// legacy path; routes outside the package are refused.
func TestKnowledgeHostServesEveryRouteNatively(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newKnowledgeService(bridge, "lease-1")
	require.NoError(t, err)
	for route := range knowledgeRoutes {
		configured, effective := service.Mode(route)
		require.Equal(t, pluginhostsdk.RouteModeNative, configured, route)
		require.Equal(t, pluginhostsdk.RouteModeNative, effective, route)
	}
	_, _ = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "knowledge.article.list", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Empty(t, bridge.operation, "no route is relayed to a legacy handler")

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.list", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(knowledgeRoutes))
	for route := range knowledgeRoutes {
		require.Contains(t, handlers, route, "every route has a native handler")
	}
}
