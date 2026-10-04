package main

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
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
	handlers := (&native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(nil)}).Handlers()
	require.Len(t, handlers, len(protocolRuntimeRoutes))
	for route := range protocolRuntimeRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, protocolRuntimeRoutes, route)
	}
	withoutNodeOps := (&native.Service{}).Handlers()
	for _, route := range []string{
		native.CreateProtocolRouteID, native.UpdateProtocolRouteID, native.DeleteProtocolRouteID, native.SyncRouteID,
		native.AgentControlRouteID, native.AgentControlOpsRouteID, native.AgentListRouteID, native.AgentMonitorRouteID,
		native.AgentTasksCreateRouteID, native.AgentExecuteRouteID,
	} {
		require.NotContains(t, withoutNodeOps, route, "without KernelNodeOps the route stays legacy")
	}
}

// In native mode, a node's protocols answer from the legacy handler while
// the lease does not adopt v2_node_protocol: the node credential split is
// not finalized, or the host has no storage.
func TestProtocolRuntimeHostKeepsProtocolsLegacyWithoutTheAdoption(t *testing.T) {
	bridge := &modesStub{modes: map[string]string{native.ProtocolsRouteID: "native"}}
	service, err := newProtocolRuntimeService(bridge, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	_, effective := service.Mode(native.ProtocolsRouteID)
	require.Equal(t, "native", effective)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: native.ProtocolsRouteID, Method: "GET", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		Metadata: pluginhostsdk.RequestMetadata{PathParams: map[string]string{"id": "1"}},
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, native.ProtocolsRouteID, bridge.operation)
}

// modesStub is bridgeStub with route modes.
type modesStub struct {
	bridgeStub
	modes map[string]string
}

func (s *modesStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{Revision: 1, RouteModes: s.modes}, nil
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

// The package adopts the diagnostic task table, and v2_node_protocol,
// which the kernel grants only once the node credential split finalized it
// (its secret positions then hold the placeholder). It never adopts the
// WireGuard peers or the node credentials, which stay protected kernel
// tables, and reads a node's existence through kapi_node_status_v1. Of
// KernelNodeOps it holds the families its routes use: nodeconfig (secrets,
// protocol retirement, node sync, validation), agents (Agent Control and
// the session RPCs) and diagnose (agent diagnostics); never credentials or
// forward.
func TestProtocolRuntimeManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_agent_diagnostic_task", "kernel.storage.adopt:v2_node_protocol",
		"kernel.view:kapi_node_status_v1", "kernel.nodeops.nodeconfig.v1", "kernel.nodeops.agents.v1", "kernel.nodeops.diagnose.v1",
	}, manifest.Capabilities)
}
