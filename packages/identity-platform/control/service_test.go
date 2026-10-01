package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type bridgeStub struct {
	capability []byte
	operation  string
	payload    []byte
	response   packagebridgesdk.Response
}

func (s *bridgeStub) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	return packagebridgesdk.StorageLease{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func newTestService(t *testing.T, bridge identityBridge) *pluginhostsdk.Router {
	t.Helper()
	service, err := newIdentityService(bridge, "lease-1", packagestoresdk.SharedOpener(bridge))
	require.NoError(t, err)
	return service
}

func (s *bridgeStub) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{}, packagebridgesdk.ErrSessionOperationUnsupported
}

func (s *bridgeStub) Invoke(_ context.Context, capability []byte, operation string, payload []byte) (packagebridgesdk.Response, error) {
	s.capability = append([]byte(nil), capability...)
	s.operation = operation
	s.payload = append([]byte(nil), payload...)
	return s.response, nil
}

func TestIdentityServiceDispatchesLoginOnlyThroughBridgeCapability(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{
		StatusCode: 200,
		Body:       []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"issued"}}`),
		Headers:    []packagebridgesdk.Header{{Name: "Retry-After", Value: "1"}},
	}}
	service := newTestService(t, bridge)
	capability := make([]byte, 32)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "identity.auth.login", RequestBody: []byte(`{"email":"u@example.test","password":"secret"}`),
		BridgeCapability: capability, DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})

	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, "identity.auth.login", bridge.operation)
	require.Equal(t, capability, bridge.capability)
	require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), bridge.payload)
	require.Equal(t, []pluginhostsdk.Header{{Name: "Retry-After", Value: "1"}}, response.Headers)
}

func TestIdentityServiceRejectsUnknownRoutesAndMissingCapability(t *testing.T) {
	service := newTestService(t, &bridgeStub{})
	_, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{RouteID: "identity.admin.users.list"})
	require.Error(t, err)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{RouteID: "identity.auth.login"})
	require.Error(t, err)
}

func TestIdentityServiceDispatchesDeclaredAdministrativeRouteThroughBridge(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":[]}`)}}
	service := newTestService(t, bridge)
	capability := make([]byte, 32)
	_, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "identity.admin.users.get", BridgeCapability: capability,
	})

	require.NoError(t, err)
	require.Equal(t, "identity.admin.users.get", bridge.operation, "a route with no native mode relays to the legacy handler")
}

func TestNilIdentityServiceHealthIsUnhealthyWithoutPanicking(t *testing.T) {
	var service *pluginhostsdk.Router

	response, err := service.Health(context.Background())

	require.NoError(t, err)
	require.False(t, response.Healthy)
	require.Empty(t, response.LeaseID)
}

func TestIdentityServiceRunsMigrationOnlyThroughBridgeCapability(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"checkpoint":"identity-platform/001","validation_digest":"migration-digest","complete":true}`)}}
	service := newTestService(t, bridge)
	capability := make([]byte, 32)

	response, err := service.Migrate(context.Background(), pluginhostsdk.MigrationRequest{
		MigrationID: "001_identity_platform", BridgeCapability: capability,
	})

	require.NoError(t, err)
	require.True(t, response.Complete)
	require.Equal(t, "identity-platform/001", response.Checkpoint)
	require.Equal(t, "migration-digest", response.ValidationDigest)
	require.Equal(t, "migration.identity-platform.001_identity_platform", bridge.operation)
	require.Equal(t, capability, bridge.capability)
}

// The host accepts exactly the package's declared compatibility routes, each
// either with a native handler or bridged.
func TestIdentityHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range identityRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		require.NotContains(t, identityRoutes, route)
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// Every native route has a handler, a bridged one has none, and without
// KernelSubscriber the resets stay legacy.
func TestIdentityHostNativeHandlersAreTheNativeRoutes(t *testing.T) {
	handlers := (&native.Service{Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil)}).Handlers()
	require.Len(t, handlers, len(identityRoutes))
	for route := range identityRoutes {
		require.Contains(t, handlers, route)
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route)
	}
	withoutSubscriber := (&native.Service{}).Handlers()
	require.NotContains(t, withoutSubscriber, native.ResetTrafficRouteID)
	require.NotContains(t, withoutSubscriber, native.ResetSubscribeRouteID)
	require.Contains(t, withoutSubscriber, native.ProfileRouteID)
}

// identity-platform reads the user directory view (login's expiry check,
// the administrator's user directory) and the entitlement view (the
// directory's subscription summary), and may reset traffic and subscription
// credentials; it holds no other subscriber family and adopts no kernel
// table.
func TestIdentityManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.identity.v1", "kernel.storage.v1", "kernel.view:kapi_user_directory_v1", "kernel.view:kapi_subscriber_entitlement_v1",
		"kernel.subscriber.traffic.v1", "kernel.subscriber.credentials.v1",
	}, manifest.Capabilities)
}

// kernelRecorder is Control's KernelIdentity and KernelSubscriber as far as
// the host can tell, on one connection as the bridge provides them.
type kernelRecorder struct {
	kernelidentityv1.UnimplementedKernelIdentityServer
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	mu      sync.Mutex
	token   string
	resets  []*kernelsubscriberv1.ResetCredentialsRequest
	traffic []*kernelsubscriberv1.ResetTrafficRequest
}

func (r *kernelRecorder) GetSubscriber(_ context.Context, request *kernelidentityv1.GetSubscriberRequest) (*kernelidentityv1.GetSubscriberResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.GetUserId() != 2 {
		return nil, status.Error(codes.NotFound, "subscriber is not linked to an identity account")
	}
	return &kernelidentityv1.GetSubscriberResponse{SubscriberJson: []byte(`{"id":2,"token":"` + r.token + `","uuid":"u2"}`)}, nil
}

func (r *kernelRecorder) ResetCredentials(_ context.Context, request *kernelsubscriberv1.ResetCredentialsRequest) (*kernelsubscriberv1.ResetCredentialsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resets = append(r.resets, request)
	if request.GetUserId() != 2 {
		return nil, status.Error(codes.NotFound, "subscriber not found")
	}
	r.token = "issued-" + strconv.Itoa(len(r.resets))
	return &kernelsubscriberv1.ResetCredentialsResponse{Applied: true}, nil
}

func (r *kernelRecorder) ResetTraffic(_ context.Context, request *kernelsubscriberv1.ResetTrafficRequest) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traffic = append(r.traffic, request)
	return &kernelsubscriberv1.ResetTrafficResponse{Applied: true, ResetUsers: 1}, nil
}

// connectedBridge leases storage, hands out route modes and carries the
// kernel's contracts on its connection, as packagebridgesdk.Client does.
type connectedBridge struct {
	*storageBridge
	modes map[string]string
	conn  grpc.ClientConnInterface
}

func (b connectedBridge) GetPackageConfig(context.Context) (packagebridgesdk.PackageConfig, error) {
	return packagebridgesdk.PackageConfig{Revision: 1, RouteModes: b.modes}, nil
}

func (b connectedBridge) Conn() grpc.ClientConnInterface { return b.conn }

// The resets reach KernelSubscriber over the bridge connection, under the
// request's ledger id, and never the legacy handler.
func TestIdentityHostResetsThroughKernelSubscriberOverTheBridge(t *testing.T) {
	recorder := &kernelRecorder{token: "before"}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	kernelidentityv1.RegisterKernelIdentityServer(server, recorder)
	kernelsubscriberv1.RegisterKernelSubscriberServer(server, recorder)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	storage := &storageBridge{path: filepath.Join(t.TempDir(), "kernel.db")}
	bridge := connectedBridge{storageBridge: storage, conn: conn, modes: map[string]string{
		native.ResetSubscribeRouteID: "native", native.ResetTrafficRouteID: "native",
	}}
	host, err := newIdentityHost(bridge, "lease-1", environment(map[string]string{envKEK: testKEK}), t.Logf)
	require.NoError(t, err)
	host.Refresh(context.Background())
	_, effective := host.Mode(native.ResetSubscribeRouteID)
	require.Equal(t, "native", effective)

	reset := func(route, path string) map[string]any {
		response := dispatchRoute(t, host.Router, route, pluginhostsdk.DispatchRequest{
			Method: "POST", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`),
			Metadata: pluginhostsdk.RequestMetadata{
				Path: path, PathParams: map[string]string{"id": "2"}, Headers: map[string][]string{"Idempotency-Key": {"key-1"}},
			},
		})
		require.EqualValues(t, 200, response.StatusCode)
		var answer map[string]any
		require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
		return answer
	}
	answer := reset(native.ResetSubscribeRouteID, "/api/v2/admin/users/2/reset-subscribe")
	require.Equal(t, map[string]any{"token": "issued-1"}, answer["data"])
	answer = reset(native.ResetTrafficRouteID, "/api/v2/admin/users/2/reset-traffic")
	require.Equal(t, map[string]any{"message": "流量重置成功"}, answer["data"])
	require.Empty(t, storage.operation, "a native route does not reach the legacy handler")

	require.Len(t, recorder.resets, 1)
	require.True(t, proto.Equal(&kernelsubscriberv1.ResetCredentialsRequest{
		RequestId: native.ResetRequestID("reset_subscribe", 2, "key-1"), UserId: 2, SubscriptionToken: true,
	}, recorder.resets[0]), "%v", recorder.resets[0])
	require.Len(t, recorder.traffic, 1)
	require.Equal(t, native.ResetRequestID("reset_traffic", 2, "key-1"), recorder.traffic[0].GetRequestId())
	require.Equal(t, []uint64{2}, recorder.traffic[0].GetUserIds())

	// The bridged invite list still relays to the legacy handler, and so
	// does the user list while its mode is legacy.
	dispatchRoute(t, host.Router, "identity.user.invite.get", pluginhostsdk.DispatchRequest{Method: "GET"})
	require.Equal(t, "identity.user.invite.get", storage.operation)
	dispatchRoute(t, host.Router, native.AdminUsersRouteID, pluginhostsdk.DispatchRequest{Method: "GET"})
	require.Equal(t, native.AdminUsersRouteID, storage.operation)
}

func dispatchRoute(t *testing.T, router *pluginhostsdk.Router, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
	t.Helper()
	request.RouteID = route
	request.BridgeCapability = make([]byte, 32)
	request.DeadlineUnixMillis = time.Now().Add(5 * time.Second).UnixMilli()
	response, err := router.Dispatch(context.Background(), request)
	require.NoError(t, err)
	return response
}
