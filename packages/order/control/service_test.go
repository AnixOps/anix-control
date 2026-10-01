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
	"github.com/AnixOps/anix-control/v4/packages/order/native"
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
	return &kernelsubscriberv1.ApplyEntitlementResponse{Applied: len(r.grants) == 1}, nil
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
func TestOrderHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newOrderService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"order.admin.coupon.get", native.MarkPaidRouteID, "order.user.order.get"} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "payment.user.payment.records.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil)}).Handlers()
	require.Len(t, handlers, len(orderRoutes))
	for route := range orderRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, orderRoutes, route)
	}
	require.NotContains(t, (&native.Service{}).Handlers(), native.MarkPaidRouteID, "without KernelSubscriber completion stays legacy")
}

// The host accepts exactly the package's declared compatibility routes.
func TestOrderHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range orderRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its two tables, reads the plan and user views, and may
// apply entitlements; it neither adopts nor reads v2_user or v2_plan.
func TestOrderManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_order", "kernel.storage.adopt:v2_coupon",
		"kernel.view:kapi_plan_catalog_v1", "kernel.view:kapi_plan_subscription_group_v1", "kernel.view:kapi_user_directory_v1",
		"kernel.subscriber.entitlements.v1",
	}, manifest.Capabilities)
}

// Marking an order paid natively grants its plan through KernelSubscriber
// on the bridge connection, once per order, and completes the order.
func TestOrderHostCompletesThroughKernelSubscriberOverTheBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// Tables stand in for the kernel views here.
	require.NoError(t, db.AutoMigrate(&native.Order{}, &native.CatalogPlan{}, &native.PlanGroup{}))
	speed, devices := int64(100), 3
	require.NoError(t, db.Create(&native.CatalogPlan{ID: 7, GroupID: 4, TransferEnable: 50, SpeedLimit: &speed, DeviceLimit: &devices}).Error)
	require.NoError(t, db.Create(&[]native.PlanGroup{{PlanID: 7, GroupID: 11}, {PlanID: 7, GroupID: 12}}).Error)
	require.NoError(t, db.Create(&native.Order{ID: 5, UserID: 2, PlanID: 7, Period: "quarter", TradeNo: "T5", TotalAmount: 3000}).Error)

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
		modes: map[string]string{native.MarkPaidRouteID: "native"},
		lease: packagebridgesdk.StorageLease{Driver: "sqlite", DSN: path, TablePrefix: "pkg_order_", AdoptedTables: []string{"v2_coupon", "v2_order"}},
	}
	service, err := newOrderService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	configured, effective := service.Mode(native.MarkPaidRouteID)
	require.Equal(t, "native", configured)
	require.Equal(t, "native", effective)

	request := pluginhostsdk.DispatchRequest{
		Method: "POST", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`),
		Metadata: pluginhostsdk.RequestMetadata{Path: "/api/v2/admin/orders/5/paid", PathParams: map[string]string{"id": "5"}},
	}
	response := dispatch(t, service, native.MarkPaidRouteID, request)
	require.EqualValues(t, 200, response.StatusCode)
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	var answer struct {
		Code int `json:"code"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
	require.Equal(t, 0, answer.Code, "%s", response.ResponseBody)
	require.Equal(t, "订单已开通", answer.Data.Message)

	require.Len(t, recorder.grants, 1)
	want := &kernelsubscriberv1.ApplyEntitlementRequest{
		RequestId: "order:5", UserId: 2,
		Plan: &kernelsubscriberv1.PlanSnapshot{
			PlanId: 7, GroupId: 4, SubscriptionGroupIds: []uint64{11, 12}, TransferBytes: 50 << 30,
			SpeedLimitMbps: proto.Int64(100), DeviceLimit: proto.Int32(3),
		},
		Expiry:        &kernelsubscriberv1.ApplyEntitlementRequest_Period{Period: &kernelsubscriberv1.Period{Months: 3}},
		RenewSamePlan: true, ResetTraffic: true, Reason: "order completion",
	}
	require.True(t, proto.Equal(want, recorder.grants[0]), "grant %v", recorder.grants[0])
	var order native.Order
	require.NoError(t, db.Take(&order, 5).Error)
	require.Equal(t, 3, order.Status)
	require.NotNil(t, order.PaidAt)

	// Marking it paid again names the same grant, which the kernel applies
	// once.
	dispatch(t, service, native.MarkPaidRouteID, request)
	require.Len(t, recorder.grants, 2)
	require.Equal(t, "order:5", recorder.grants[1].GetRequestId())
}
