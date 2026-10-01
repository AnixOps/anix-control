package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/machine-telemetry/native"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type bridgeStub struct {
	operation string
	modes     map[string]string
}

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	if s.modes == nil {
		return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
	}
	return packagebridgesdk.PackageConfig{Revision: 1, RouteModes: s.modes}, nil
}

func (s *bridgeStub) Invoke(_ context.Context, _ []byte, operation string, _ []byte) (packagebridgesdk.Response, error) {
	s.operation = operation
	return packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":null}`)}, nil
}

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler, bridged routes always; routes outside the package are refused.
func TestMachineTelemetryHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newMachineTelemetryService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"telemetry.admin.traffic.hourly.get", "telemetry.admin.dashboard.get"} {
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
	handlers := (&native.Service{Telemetry: kerneltelemetryv1.NewKernelTelemetryClient(nil)}).Handlers()
	require.Len(t, handlers, len(machineTelemetryRoutes))
	for route := range machineTelemetryRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, machineTelemetryRoutes, route)
	}
	require.NotContains(t, (&native.Service{}).Handlers(), native.DashboardRouteID, "without KernelTelemetry the dashboard stays legacy")
}

// The host accepts exactly the package's declared compatibility routes.
func TestMachineTelemetryHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range machineTelemetryRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package reads the traffic log and the user directory through kernel
// views, the dashboard through KernelTelemetry, and adopts no table;
// telemetry.read is its agent capability.
func TestMachineTelemetryManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"telemetry.read", "kernel.storage.v1", "kernel.view:kapi_traffic_log_v1", "kernel.view:kapi_user_directory_v1",
		"kernel.telemetry.dashboard.v1",
	}, manifest.Capabilities)
}

// The package's own plugin control routes (its /api/v3/plugins status
// route, dispatched as "machine-telemetry.control.<digest>") relay to the kernel;
// another package's are refused.
func TestHostRelaysThePackagePluginControlRoutes(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newMachineTelemetryService(bridge, "lease-1")
	require.NoError(t, err)
	route := "machine-telemetry.control." + strings.Repeat("ab", 32)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: route, BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, route, bridge.operation)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "gost-mesh.control." + strings.Repeat("ab", 32), BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
}

// connectedBridge is a bridge whose connection also carries the kernel's
// other contracts, as packagebridgesdk.Client and NetworkClient do.
type connectedBridge struct {
	*bridgeStub
	conn grpc.ClientConnInterface
}

func (b connectedBridge) Conn() grpc.ClientConnInterface { return b.conn }

// dashboardKernel is the kernel's KernelTelemetry as far as the host can
// tell: it answers a snapshot and records the requests.
type dashboardKernel struct {
	kerneltelemetryv1.UnimplementedKernelTelemetryServer
	requests chan *kerneltelemetryv1.GetDashboardRequest
}

func (k dashboardKernel) GetDashboard(_ context.Context, request *kerneltelemetryv1.GetDashboardRequest) (*kerneltelemetryv1.GetDashboardResponse, error) {
	k.requests <- request
	return &kerneltelemetryv1.GetDashboardResponse{TotalUsers: 7, OnlineUsers: 2, CachedAt: "2026-10-01T08:00:00.25+08:00"}, nil
}

// Set native, the dashboard is answered from KernelTelemetry on the bridge
// connection, with the request's refresh and the kernel's cached_at as is;
// a bridge without that connection keeps it legacy.
func TestMachineTelemetryHostServesTheDashboardThroughKernelTelemetry(t *testing.T) {
	kernel := dashboardKernel{requests: make(chan *kerneltelemetryv1.GetDashboardRequest, 2)}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	kerneltelemetryv1.RegisterKernelTelemetryServer(server, kernel)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	dispatchDashboard := func(service *pluginhostsdk.Router, refresh string) pluginhostsdk.DispatchResponse {
		response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
			RouteID: native.DashboardRouteID, Method: "GET", BridgeCapability: make([]byte, 32),
			DeadlineUnixMillis: time.Now().Add(5 * time.Second).UnixMilli(),
			Metadata:           pluginhostsdk.RequestMetadata{Query: map[string][]string{"refresh": {refresh}}},
		})
		require.NoError(t, err)
		return response
	}

	stub := &bridgeStub{modes: map[string]string{native.DashboardRouteID: "native"}}
	service, err := newMachineTelemetryService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	response := dispatchDashboard(service, "true")
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	require.True(t, (<-kernel.requests).GetRefresh())
	var answer struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.ResponseBody, &answer))
	require.JSONEq(t, `7`, string(answer.Data["total_users"]))
	require.JSONEq(t, `2`, string(answer.Data["online_users"]))
	require.Equal(t, `"2026-10-01T08:00:00.25+08:00"`, string(answer.Data["cached_at"]))
	dispatchDashboard(service, "1")
	require.False(t, (<-kernel.requests).GetRefresh(), "only refresh=true refreshes")

	legacy := &bridgeStub{modes: map[string]string{native.DashboardRouteID: "native"}}
	service, err = newMachineTelemetryService(legacy, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	dispatchDashboard(service, "true")
	require.Equal(t, native.DashboardRouteID, legacy.operation, "without the contract the dashboard stays legacy")
}
