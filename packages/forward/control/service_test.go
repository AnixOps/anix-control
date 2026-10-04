package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/forward/native"
	"github.com/AnixOps/anix-control/v4/packages/forward/v4api"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type bridgeStub struct {
	operation string
	modes     map[string]string
	lease     packagebridgesdk.StorageLease
}

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	if s.lease.Driver == "" {
		return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
	}
	return s.lease, nil
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

// resetRecorder is the kernel's KernelSubscriber as far as the host can
// tell: it records the traffic resets it receives.
type resetRecorder struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	mu    sync.Mutex
	calls []*kernelsubscriberv1.ResetTrafficRequest
}

func (r *resetRecorder) ResetTraffic(_ context.Context, request *kernelsubscriberv1.ResetTrafficRequest) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, request)
	return &kernelsubscriberv1.ResetTrafficResponse{Applied: true, ResetUsers: 1}, nil
}

func dispatch(t *testing.T, service pluginhostsdk.Package, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
	t.Helper()
	request.RouteID = route
	request.BridgeCapability = make([]byte, 32)
	request.DeadlineUnixMillis = time.Now().Add(5 * time.Second).UnixMilli()
	response, err := service.Dispatch(context.Background(), request)
	require.NoError(t, err)
	return response
}

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler, bridged routes always; routes outside the package are refused.
func TestForwardHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newForwardService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{
		"forward.forward.list.post", native.ResetRouteID, "forward.admin.forward.runtime.status.get", "forward.forward_agent.report.post",
		native.SpeedLimitCreateRouteID,
	} {
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
	handlers := (&native.Service{Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil)}).Handlers()
	require.Len(t, handlers, len(forwardRoutes))
	for route := range forwardRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, forwardRoutes, route)
	}
	require.NotContains(t, (&native.Service{}).Handlers(), native.ResetRouteID, "without KernelSubscriber the reset stays legacy")
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
	for route := range forwardRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
	require.Len(t, want, 32)
}

// The package adopts the forward tables its native routes use, reads the
// forward node, runtime settings, directory and entitlement views, and may
// reset traffic, and calls ForwardControl for its v4 API. It adopts none of the tables that hold agent credentials
// (v2_forward_node, v2_forward_clean_agent, v2_forward_runtime_job), nor
// v2_user or v2_system_config.
func TestForwardManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_forward", "kernel.storage.adopt:v2_forward_tunnel",
		"kernel.storage.adopt:v2_forward_user_tunnel", "kernel.storage.adopt:v2_speed_limit", "kernel.storage.adopt:v2_forward_rule",
		"kernel.storage.adopt:v2_forward_latency_bucket", "kernel.view:kapi_forward_node_v1", "kernel.view:kapi_forward_runtime_settings_v1",
		"kernel.view:kapi_user_directory_v1", "kernel.view:kapi_subscriber_entitlement_v1", "kernel.subscriber.traffic.v1",
		"kernel.forward.v1",
	}, manifest.Capabilities)
	for _, capability := range manifest.Capabilities {
		for _, table := range []string{"v2_forward_node", "v2_forward_clean_agent", "v2_forward_runtime_job", "v2_user", "v2_system_config"} {
			require.NotEqual(t, "kernel.storage.adopt:"+table, capability)
		}
	}
}

// A native subscriber traffic reset goes to KernelSubscriber on the bridge
// connection, under the ledger id the kernel derives from the request's
// Idempotency-Key; a tunnel permission's reset changes only the adopted
// tables.
func TestForwardHostResetsTrafficThroughKernelSubscriberOverTheBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// A table stands in for the kernel view here.
	require.NoError(t, db.AutoMigrate(&native.Forward{}, &native.UserTunnel{}, &native.DirectoryUser{}))
	require.NoError(t, db.Create(&native.DirectoryUser{ID: 2}).Error)
	require.NoError(t, db.Create(&native.UserTunnel{ID: 1, UserID: 2, TunnelID: 1, InFlow: 50, OutFlow: 60, Status: 1}).Error)
	require.NoError(t, db.Create(&native.Forward{ID: 1, UserID: 2, TunnelID: 1, Name: "f", RemoteAddr: "203.0.113.1:443", InFlow: 5, OutFlow: 6}).Error)

	recorder := &resetRecorder{}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(server, recorder)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	stub := &bridgeStub{
		modes: map[string]string{native.ResetRouteID: "native"},
		lease: packagebridgesdk.StorageLease{
			Driver: "sqlite", DSN: path, TablePrefix: "pkg_forward_", AdoptedTables: []string{"v2_forward", "v2_forward_user_tunnel"},
		},
	}
	service, err := newForwardService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	configured, effective := service.Mode(native.ResetRouteID)
	require.Equal(t, "native", configured)
	require.Equal(t, "native", effective)

	reset := func(body string) map[string]any {
		response := dispatch(t, service, native.ResetRouteID, pluginhostsdk.DispatchRequest{
			Method: "POST", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`), RequestBody: []byte(body),
			Metadata: pluginhostsdk.RequestMetadata{Path: "/api/v2/user/reset", Headers: map[string][]string{"Idempotency-Key": {"key-1"}}},
		})
		require.EqualValues(t, 200, response.StatusCode)
		var answer map[string]any
		require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
		return answer
	}

	answer := reset(`{"id":2,"type":1}`)
	require.EqualValues(t, 0, answer["code"], "%v", answer)
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	require.Len(t, recorder.calls, 1)
	want := &kernelsubscriberv1.ResetTrafficRequest{
		RequestId: native.ResetRequestID(2, "key-1"), UserIds: []uint64{2}, Reason: "administrator reset",
	}
	require.True(t, proto.Equal(want, recorder.calls[0]), "reset %v", recorder.calls[0])
	require.True(t, strings.HasPrefix(want.GetRequestId(), "forward.reset_traffic:2:"))

	answer = reset(`{"id":3,"type":1}`)
	require.Equal(t, "用户不存在", answer["msg"])
	require.Len(t, recorder.calls, 1, "an unknown subscriber is not reset")

	answer = reset(`{"id":1,"type":2}`)
	require.EqualValues(t, 0, answer["code"], "%v", answer)
	require.Len(t, recorder.calls, 1)
	var permission native.UserTunnel
	require.NoError(t, db.Take(&permission, 1).Error)
	require.Zero(t, permission.InFlow+permission.OutFlow)
	var forward native.Forward
	require.NoError(t, db.Take(&forward, 1).Error)
	require.Zero(t, forward.InFlow+forward.OutFlow)
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
