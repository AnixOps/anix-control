package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/plan/native"
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

// subscriberRecorder is the kernel's KernelSubscriber as far as the host can
// tell: it records the grants it receives.
type subscriberRecorder struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	mu     sync.Mutex
	grants []*kernelsubscriberv1.ApplyEntitlementRequest
}

func (r *subscriberRecorder) ApplyEntitlement(_ context.Context, request *kernelsubscriberv1.ApplyEntitlementRequest) (*kernelsubscriberv1.ApplyEntitlementResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants = append(r.grants, request)
	return &kernelsubscriberv1.ApplyEntitlementResponse{Applied: true}, nil
}

func dispatch(t *testing.T, service *pluginhostsdk.Router, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
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
func TestPlanHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newPlanService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"plan.admin.plans.get", "plan.admin.plans.id.assign.post"} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "ticket.user.ticket.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil)}).Handlers()
	require.Len(t, handlers, len(planRoutes))
	for route := range planRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, planRoutes, route)
	}
	require.NotContains(t, (&native.Service{}).Handlers(), native.AssignRouteID, "without KernelSubscriber the assignment stays legacy")
}

// The host accepts exactly the package's declared compatibility routes.
func TestPlanHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range planRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its two tables and may apply entitlements; it neither
// adopts nor reads v2_user.
func TestPlanManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_plan", "kernel.storage.adopt:v2_event",
		"kernel.subscriber.entitlements.v1",
	}, manifest.Capabilities)
}

// A native assignment reads the plan from the package's storage and grants
// it through KernelSubscriber on the bridge connection; the plan's event
// goes to the adopted v2_event.
func TestPlanHostAssignsThroughKernelSubscriberOverTheBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&native.Plan{}, &native.Event{}))
	speed, devices := int64(100), 3
	require.NoError(t, db.Create(&native.Plan{ID: 7, GroupID: 4, TransferEnable: 50, SpeedLimit: &speed, DeviceLimit: &devices, Name: "Pro"}).Error)

	recorder := &subscriberRecorder{}
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
		modes: map[string]string{native.AssignRouteID: "native"},
		lease: packagebridgesdk.StorageLease{Driver: "sqlite", DSN: path, TablePrefix: "pkg_plan_", AdoptedTables: []string{"v2_event", "v2_plan"}},
	}
	service, err := newPlanService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	configured, effective := service.Mode(native.AssignRouteID)
	require.Equal(t, "native", configured)
	require.Equal(t, "native", effective)

	request := pluginhostsdk.DispatchRequest{
		Method: "POST", RequestBody: []byte(`{"user_id":2,"expire_at":1893456000}`), PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`),
		Metadata: pluginhostsdk.RequestMetadata{
			Path: "/api/v2/admin/plans/7/assign", PathParams: map[string]string{"id": "7"},
			Headers: map[string][]string{"Idempotency-Key": {"assign-once"}},
		},
	}
	response := dispatch(t, service, native.AssignRouteID, request)
	require.EqualValues(t, 200, response.StatusCode)
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	var answer struct {
		Code int `json:"code"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
	require.Equal(t, 0, answer.Code)
	require.Equal(t, "分配成功", answer.Data.Message)

	expires := int64(1893456000)
	require.Len(t, recorder.grants, 1)
	want := &kernelsubscriberv1.ApplyEntitlementRequest{
		RequestId: native.AssignRequestID(7, 2, &expires, "assign-once"), UserId: 2,
		Plan: &kernelsubscriberv1.PlanSnapshot{
			PlanId: 7, GroupId: 4, TransferBytes: 50 << 30, SpeedLimitMbps: proto.Int64(100), DeviceLimit: proto.Int32(3),
		},
		Expiry:       &kernelsubscriberv1.ApplyEntitlementRequest_ExpiresAtUnix{ExpiresAtUnix: expires},
		ResetTraffic: true, KeepSubscriptionGroups: true, Reason: "admin plan assignment",
	}
	require.True(t, proto.Equal(want, recorder.grants[0]), "grant %v", recorder.grants[0])

	// A retry carries the same request id, so the kernel applies it once.
	dispatch(t, service, native.AssignRouteID, request)
	require.Len(t, recorder.grants, 2)
	require.Equal(t, recorder.grants[0].GetRequestId(), recorder.grants[1].GetRequestId())

	var events []native.Event
	require.NoError(t, db.Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, "plan.assigned", events[0].Type)
	require.JSONEq(t, `{"plan_id":7,"user_id":2,"expired_at":1893456000}`, *events[0].Payload)
}
