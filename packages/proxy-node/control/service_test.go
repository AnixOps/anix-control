package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"github.com/stretchr/testify/require"
)

var errRelayed = errors.New("relayed to the kernel")

type bridgeStub struct{ operation, webSocket string }

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

func (s *bridgeStub) OpenWebSocket(_ context.Context, _ []byte, operation string) (packagebridgesdk.WebSocketStream, error) {
	s.webSocket = operation
	return nil, errRelayed
}

type streamStub struct{}

func (streamStub) Recv() (pluginhostsdk.WebSocketFrame, error) {
	return pluginhostsdk.WebSocketFrame{}, errRelayed
}
func (streamStub) Send(pluginhostsdk.WebSocketFrame) error { return nil }

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler, bridged routes always; routes outside the package are refused.
func TestProxyNodeHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newProxyNodeService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"proxy.loadbalancer.get", "proxy.admin.nodes.id.logs.get", "proxy.admin.nodes.get", "proxy.server.uniproxy.user.get"} {
		response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
			RouteID: route, BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		})
		require.NoError(t, err)
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "protocol.admin.nodes.id.protocols.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(proxyNodeRoutes))
	for route := range proxyNodeRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, proxyNodeRoutes, route)
	}
}

// The agent WebSocket is relayed to the kernel, which holds the connection.
func TestProxyNodeHostRelaysTheAgentWebSocket(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newProxyNodeService(bridge, "lease-1")
	require.NoError(t, err)
	err = service.OpenWebSocket(context.Background(), pluginhostsdk.WebSocketOpen{
		RouteID: "proxy.node.ws.get", BridgeCapability: make([]byte, 32),
	}, streamStub{})
	require.ErrorIs(t, err, errRelayed)
	require.Equal(t, "proxy.node.ws.get", bridge.webSocket)

	err = service.OpenWebSocket(context.Background(), pluginhostsdk.WebSocketOpen{
		RouteID: "protocol.agent.ws.get", BridgeCapability: make([]byte, 32),
	}, streamStub{})
	require.ErrorContains(t, err, "unsupported")
}

// The host accepts exactly the package's declared compatibility routes.
func TestProxyNodeHostRoutesAreThePackageRoutes(t *testing.T) {
	raw, err := os.ReadFile("../compat/v2-routes.json")
	require.NoError(t, err)
	var declared struct {
		Routes []struct {
			PackageRoute string `json:"package_route"`
		} `json:"routes"`
	}
	require.NoError(t, json.Unmarshal(raw, &declared))
	var want, got []string
	for _, route := range declared.Routes {
		want = append(want, route.PackageRoute)
	}
	for route := range proxyNodeRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package reads node status only through the kernel view and adopts
// only the tables its native routes use; it never declares the tables that
// hold node credentials.
func TestProxyNodeManifestGrantsNoNodeCredentials(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, []string{
		"kernel.storage.v1",
		"kernel.storage.adopt:v2_load_balancer",
		"kernel.storage.adopt:v2_node_log",
		"kernel.view:kapi_node_status_v1",
	}, manifest.Capabilities)
}
