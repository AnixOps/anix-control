package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
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
// handler, bridged routes always; routes outside the package are refused.
func TestProtocolRuntimeHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newProtocolRuntimeService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"protocol.admin.agent.tasks.get", "protocol.admin.nodes.id.protocols.get"} {
		response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
			RouteID: route, BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		})
		require.NoError(t, err)
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(protocolRuntimeRoutes))
	for route := range protocolRuntimeRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, protocolRuntimeRoutes, route)
	}
}

// The host accepts exactly the package's declared compatibility routes.
func TestProtocolRuntimeHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range protocolRuntimeRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

type streamStub struct{}

func (streamStub) Recv() (pluginhostsdk.WebSocketFrame, error) {
	return pluginhostsdk.WebSocketFrame{}, nil
}
func (streamStub) Send(pluginhostsdk.WebSocketFrame) error { return nil }

// The agent WebSocket is one of the package's routes and relays to the
// kernel; the stub bridge has no WebSocket relay, so it gets that far.
func TestProtocolRuntimeHostRelaysTheAgentWebSocket(t *testing.T) {
	service, err := newProtocolRuntimeService(&bridgeStub{}, "lease-1")
	require.NoError(t, err)
	err = service.OpenWebSocket(context.Background(), pluginhostsdk.WebSocketOpen{RouteID: "protocol.agent.ws.get", BridgeCapability: make([]byte, 32)}, streamStub{})
	require.EqualError(t, err, "package WebSocket bridge is unavailable")
	err = service.OpenWebSocket(context.Background(), pluginhostsdk.WebSocketOpen{RouteID: "proxy.node.ws.get", BridgeCapability: make([]byte, 32)}, streamStub{})
	require.EqualError(t, err, `package route "proxy.node.ws.get" is unsupported`)
}

// The package adopts the diagnostic task table only: never the node
// protocols with their Reality and WireGuard keys, the WireGuard peers or
// the node credentials, which are protected kernel tables.
func TestProtocolRuntimeManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{"kernel.storage.v1", "kernel.storage.adopt:v2_agent_diagnostic_task"}, manifest.Capabilities)
}
