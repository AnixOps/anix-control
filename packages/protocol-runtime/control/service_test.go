package control

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
	"github.com/AnixOps/anix-control/v5/packages/protocol-runtime/native"
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

// Every route is served natively whatever the modes say; routes outside
// the package are refused.
func TestProtocolRuntimeHostServesItsRoutesNatively(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newProtocolRuntimeService(bridge, "lease-1")
	require.NoError(t, err)
	// The stub bridge has no KernelNodeOps connection: the routes that
	// need none are the ones this host has handlers for.
	for route := range (&native.Service{}).Handlers() {
		_, effective := service.Mode(route)
		require.Equal(t, pluginhostsdk.RouteModeNative, effective, route)
	}
	_, _ = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: native.ProtocolsRouteID, Method: "GET", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		Metadata: pluginhostsdk.RequestMetadata{PathParams: map[string]string{"id": "1"}},
	})
	require.Empty(t, bridge.operation, "a native route is not relayed")

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(nil)}).Handlers()
	require.Len(t, handlers, len(protocolRuntimeRoutes))
	for route := range protocolRuntimeRoutes {
		require.Contains(t, handlers, route, "every route has a native handler")
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
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
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
