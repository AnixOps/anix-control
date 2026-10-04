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

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/forward/v4api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type bridgeStub struct {
	operation string
	modes     map[string]string
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

// connectedBridge is a bridge whose connection also carries the kernel's
// other contracts, as packagebridgesdk.Client and NetworkClient do.
type connectedBridge struct {
	*bridgeStub
	conn grpc.ClientConnInterface
}

func (b connectedBridge) Conn() grpc.ClientConnInterface { return b.conn }

func dispatch(t *testing.T, service pluginhostsdk.Package, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
	t.Helper()
	request.RouteID = route
	request.BridgeCapability = make([]byte, 32)
	request.DeadlineUnixMillis = time.Now().Add(5 * time.Second).UnixMilli()
	response, err := service.Dispatch(context.Background(), request)
	require.NoError(t, err)
	return response
}

// Every v2 route of the package relays to the kernel's legacy handler, even
// with a stored native mode (v4.1 stored native for the old native-flagged
// routes): the package has no native handler since v4.2 (F5d). Routes
// outside the package are refused.
func TestForwardHostRelaysEveryV2Route(t *testing.T) {
	bridge := &bridgeStub{modes: map[string]string{"forward.forward.list.post": "native", "forward.user.reset.post": "native", "forward.speed_limit.create.post": "shadow"}}
	service, err := newForwardService(bridge, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	for _, route := range []string{
		"forward.forward.list.post", "forward.user.reset.post", "forward.admin.forward.runtime.status.get", "forward.forward_agent.report.post",
		"forward.speed_limit.create.post",
	} {
		_, effective := service.Mode(route)
		require.Equal(t, pluginhostsdk.RouteModeLegacy, effective, route)
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}
	// The flux routes removed in v4.2 (F5d) are no longer the package's.
	for _, route := range []string{"forward.forward.create.post", "forward.admin.forward.nodes.get", "forward.speed_limit.update.post"} {
		_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
			RouteID: route, BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
		})
		require.Error(t, err, route)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "plan.speed_limit.list.post", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
}

// The host accepts exactly the package's declared compatibility routes.
func TestForwardHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
	require.Len(t, want, 32)
}

// Since v4.2 (F5d) the package needs only ForwardControl for its v4 API: it
// adopts no table and reads no kernel view, so the legacy cleanup (F5c) can
// drop the flux tables without breaking its storage lease.
func TestForwardManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, []string{"kernel.forward.v1"}, manifest.Capabilities)
}

// listOnly is a ForwardControl server that answers ListRoutes.
type listOnly struct {
	forwardv1.UnimplementedForwardControlServer
}

func (listOnly) ListRoutes(context.Context, *forwardv1.ListRoutesRequest) (*forwardv1.ListRoutesResponse, error) {
	return &forwardv1.ListRoutesResponse{Routes: []*forwardv1.Route{{Id: "01R", Owner: "admin", Name: "hk"}}}, nil
}

// The v4 API is the package's own control route: the host answers it on
// ForwardControl over the bridge connection whatever the route modes, to
// administrators only, and 503 without the connection.
func TestForwardHostServesTheV4APIOnForwardControl(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	forwardv1.RegisterForwardControlServer(server, listOnly{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	stub := &bridgeStub{}
	host, err := newForwardService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	request := pluginhostsdk.DispatchRequest{
		Method: "GET", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`),
		Metadata: pluginhostsdk.RequestMetadata{Path: "/api/v4/plugins/forward/routes"},
	}
	response := dispatch(t, host, v4api.RouteID, request)
	require.EqualValues(t, 200, response.StatusCode, "%s", response.ResponseBody)
	require.Contains(t, string(response.ResponseBody), `"name":"hk"`)
	require.Contains(t, string(response.ResponseBody), `"can_delete":false`)
	require.Empty(t, stub.operation, "the v4 API never reaches a legacy handler")

	// The kernel's super_admin reaches the list answer as can_delete (F5b D7).
	request.PrincipalJSON = []byte(`{"actor_id":1,"admin":true,"super_admin":true}`)
	require.Contains(t, string(dispatch(t, host, v4api.RouteID, request).ResponseBody), `"can_delete":true`)

	request.PrincipalJSON = []byte(`{"actor_id":2,"admin":false}`)
	require.EqualValues(t, 403, dispatch(t, host, v4api.RouteID, request).StatusCode)

	unconnected, err := newForwardService(&bridgeStub{}, "lease-1")
	require.NoError(t, err)
	request.PrincipalJSON = []byte(`{"actor_id":1,"admin":true}`)
	require.EqualValues(t, 503, dispatch(t, unconnected, v4api.RouteID, request).StatusCode)

	require.True(t, strings.HasPrefix(v4api.RouteID, "forward.control."))
	require.Len(t, v4api.RouteID, len("forward.control.")+64)
}
