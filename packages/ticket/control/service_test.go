package main

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/ticket/native"
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

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler; routes outside the package are refused.
func TestTicketHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newTicketService(bridge, "lease-1")
	require.NoError(t, err)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, "ticket.user.ticket.get", bridge.operation)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "knowledge.article.list", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(ticketRoutes))
	for route := range ticketRoutes {
		require.Contains(t, handlers, route, "every route has a native handler")
	}
}
