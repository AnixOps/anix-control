package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
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

func dispatch(t *testing.T, service *pluginhostsdk.Router, route string, request pluginhostsdk.DispatchRequest) pluginhostsdk.DispatchResponse {
	t.Helper()
	request.RouteID = route
	request.BridgeCapability = make([]byte, 32)
	request.DeadlineUnixMillis = time.Now().Add(5 * time.Second).UnixMilli()
	response, err := service.Dispatch(context.Background(), request)
	require.NoError(t, err)
	return response
}

// callbackRoutes complete orders through KernelOrder.
var callbackRoutes = []string{native.CallbackRouteID, native.X402CallbackRouteID, native.StripeWebhookRouteID, native.PayPalWebhookRouteID}

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler. Routes outside the package are refused.
func TestPaymentHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newPaymentService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"payment.admin.payment.gateways.get", "payment.user.payment.create.post", native.CallbackRouteID} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}
	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "order.user.order.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{Orders: kernelorderv1.NewKernelOrderClient(nil)}).Handlers()
	require.Len(t, handlers, len(paymentRoutes))
	for route := range paymentRoutes {
		require.Contains(t, handlers, route, "every route has a native handler")
	}
	withoutKernel := (&native.Service{}).Handlers()
	require.Len(t, withoutKernel, len(paymentRoutes)-len(callbackRoutes))
	for _, route := range callbackRoutes {
		require.NotContains(t, withoutKernel, route, "without KernelOrder the callbacks stay legacy")
	}
}

// The host accepts exactly the package's declared compatibility routes.
func TestPaymentHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range paymentRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its three tables and reads orders through the billing
// view only; it neither adopts v2_order nor changes subscribers, and asks
// the kernel to complete the orders its payments pay.
func TestPaymentManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_payment_gateway", "kernel.storage.adopt:v2_payment_record",
		"kernel.storage.adopt:v2_payment", "kernel.view:kapi_order_billing_v1", "kernel.order.complete.v1",
	}, manifest.Capabilities)
}

// A native gateway list reads the adopted table over the lease and shows no
// secret.
func TestPaymentHostListsGatewaysWithoutSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&native.PaymentGateway{}))
	require.NoError(t, db.Create(&native.PaymentGateway{Name: "EPay", Type: "epay", Config: `{"pid":"1","key":"merchant-secret"}`}).Error)

	route := "payment.admin.payment.gateways.get"
	stub := &bridgeStub{
		modes: map[string]string{route: "native"},
		lease: packagebridgesdk.StorageLease{
			Driver: "sqlite", DSN: path, TablePrefix: "pkg_payment_",
			AdoptedTables: []string{"v2_payment", "v2_payment_gateway", "v2_payment_record"}, Views: []string{"kapi_order_billing_v1"},
		},
	}
	service, err := newPaymentService(stub, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	_, effective := service.Mode(route)
	require.Equal(t, "native", effective)

	response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{Method: "GET", PrincipalJSON: []byte(`{"actor_id":1,"admin":true}`)})
	require.EqualValues(t, 200, response.StatusCode)
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	require.False(t, strings.Contains(string(response.ResponseBody), "merchant-secret"), "%s", response.ResponseBody)
	var answer struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Config string `json:"config"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
	require.Equal(t, 0, answer.Code)
	require.Len(t, answer.Data.List, 1)
	require.JSONEq(t, `{"key":"********","pid":"1"}`, answer.Data.List[0].Config)
}

// connectedBridge is a bridge whose connection also carries the kernel's
// other contracts, as packagebridgesdk.Client and NetworkClient do.
type connectedBridge struct {
	*bridgeStub
	conn grpc.ClientConnInterface
}

func (b connectedBridge) Conn() grpc.ClientConnInterface { return b.conn }

// completionRecorder is the kernel's KernelOrder as far as the host can
// tell: it records the completions it receives.
type completionRecorder struct {
	kernelorderv1.UnimplementedKernelOrderServer
	mu       sync.Mutex
	requests []*kernelorderv1.CompleteOrderPaymentRequest
}

func (r *completionRecorder) CompleteOrderPayment(_ context.Context, request *kernelorderv1.CompleteOrderPaymentRequest) (*kernelorderv1.CompleteOrderPaymentResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	return &kernelorderv1.CompleteOrderPaymentResponse{
		Applied: len(r.requests) == 1, OrderId: request.GetOrderId(), Outcome: kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_COMPLETED,
	}, nil
}

// A paid callback served natively records the payment in the adopted
// tables, then asks the kernel, over the bridge connection, to complete the
// order the payment names; a repeat asks again and changes nothing more.
func TestPaymentHostCompletesOrdersThroughKernelOrderOverTheBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&native.PaymentGateway{}, &native.PaymentRecord{}))
	require.NoError(t, db.Create(&native.PaymentGateway{ID: 1, Name: "Stripe", Type: "stripe", Enabled: true, Config: `{"webhook_secret":"whsec"}`}).Error)
	orderID := uint(5)
	require.NoError(t, db.Create(&native.PaymentRecord{TradeNo: "FIAT1", UserID: 2, Amount: 30, ActualAmount: 30, Currency: "USD", OrderID: &orderID}).Error)

	recorder := &completionRecorder{}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	kernelorderv1.RegisterKernelOrderServer(server, recorder)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	stub := &bridgeStub{
		modes: map[string]string{native.StripeWebhookRouteID: "native"},
		lease: packagebridgesdk.StorageLease{
			Driver: "sqlite", DSN: path, TablePrefix: "pkg_payment_",
			AdoptedTables: []string{"v2_payment", "v2_payment_gateway", "v2_payment_record"}, Views: []string{"kapi_order_billing_v1"},
		},
	}
	service, err := newPaymentService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	_, effective := service.Mode(native.StripeWebhookRouteID)
	require.Equal(t, "native", effective)

	body := []byte(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","metadata":{"trade_no":"FIAT1"}}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("whsec"))
	mac.Write([]byte(timestamp + "." + string(body)))
	request := pluginhostsdk.DispatchRequest{
		Method: "POST", PrincipalJSON: []byte(`{}`), RequestBody: body,
		Metadata: pluginhostsdk.RequestMetadata{
			Path:    "/api/v2/payment/stripe/webhook",
			Headers: map[string][]string{"Stripe-Signature": {"t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))}},
		},
	}
	response := dispatch(t, service, native.StripeWebhookRouteID, request)
	require.EqualValues(t, 200, response.StatusCode, "%s", response.ResponseBody)
	require.JSONEq(t, `{"received":true}`, string(response.ResponseBody))
	require.Empty(t, stub.operation, "a native callback does not reach the legacy handler")
	var record native.PaymentRecord
	require.NoError(t, db.Take(&record, "trade_no = ?", "FIAT1").Error)
	require.Equal(t, native.PaymentStatusPaid, record.Status)
	require.Equal(t, "cs_1", record.GatewayTradeNo)

	response = dispatch(t, service, native.StripeWebhookRouteID, request)
	require.JSONEq(t, `{"received":true,"message":"already processed or error: payment already processed"}`, string(response.ResponseBody))
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.requests, 2, "the repeat asks the kernel again")
	for _, request := range recorder.requests {
		require.True(t, proto.Equal(&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "FIAT1", OrderId: 5}, request), "%v", request)
	}
}
