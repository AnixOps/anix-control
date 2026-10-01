package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
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

// Until the kernel sets a route's mode, the host relays it to the legacy
// handler; the callbacks always. Routes outside the package are refused.
func TestPaymentHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newPaymentService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"payment.admin.payment.gateways.get", "payment.user.payment.create.post", "payment.callback"} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}
	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "order.user.order.get", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{}).Handlers()
	require.Len(t, handlers, len(paymentRoutes))
	for route := range paymentRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, paymentRoutes, route)
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
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its three tables and reads orders through the billing
// view only; it neither adopts v2_order nor changes subscribers.
func TestPaymentManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_payment_gateway", "kernel.storage.adopt:v2_payment_record",
		"kernel.storage.adopt:v2_payment", "kernel.view:kapi_order_billing_v1",
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
