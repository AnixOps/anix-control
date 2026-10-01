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

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/affiliate/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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

// balanceRecorder is the kernel's KernelSubscriber as far as the host can
// tell: it records the balance changes it receives and refuses a debit
// below zero.
type balanceRecorder struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	mu      sync.Mutex
	balance int64
	calls   []*kernelsubscriberv1.AdjustBalanceRequest
}

func (r *balanceRecorder) AdjustBalance(_ context.Context, request *kernelsubscriberv1.AdjustBalanceRequest) (*kernelsubscriberv1.AdjustBalanceResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, request)
	if r.balance+request.GetAmountCents() < 0 {
		return nil, status.Error(codes.FailedPrecondition, "insufficient balance")
	}
	r.balance += request.GetAmountCents()
	return &kernelsubscriberv1.AdjustBalanceResponse{Applied: true, BalanceCents: r.balance}, nil
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
func TestAffiliateHostRelaysRoutesUntilTheyAreSwitchedToNative(t *testing.T) {
	bridge := &bridgeStub{}
	service, err := newAffiliateService(bridge, "lease-1")
	require.NoError(t, err)
	for _, route := range []string{"affiliate.admin.invite.stats.get", native.WithdrawRouteID, "affiliate.admin.invite.config.put", native.InviteGenerateRouteID} {
		response := dispatch(t, service, route, pluginhostsdk.DispatchRequest{})
		require.EqualValues(t, 200, response.StatusCode)
		require.Equal(t, route, bridge.operation)
	}

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "identity.user.invite.generate.post", BridgeCapability: make([]byte, 32), DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})
	require.Error(t, err)
	handlers := (&native.Service{
		Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil), KernelSettings: kernelsettingsv1.NewKernelSettingsClient(nil),
	}).Handlers()
	require.Len(t, handlers, len(affiliateRoutes))
	for route := range affiliateRoutes {
		require.Contains(t, handlers, route, "every native route has a handler")
	}
	for route := range bridgedRoutes {
		require.NotContains(t, handlers, route, "a bridged route has no native handler")
		require.NotContains(t, affiliateRoutes, route)
	}
	withoutSubscriber := (&native.Service{}).Handlers()
	require.NotContains(t, withoutSubscriber, native.WithdrawRouteID, "without KernelSubscriber balance changes stay legacy")
	require.NotContains(t, withoutSubscriber, native.ProcessRouteID, "without KernelSubscriber balance changes stay legacy")
	require.NotContains(t, withoutSubscriber, native.ConfigUpdateRouteID, "without KernelSettings the configuration update stays legacy")
}

// The host accepts exactly the package's declared compatibility routes.
func TestAffiliateHostRoutesAreThePackageRoutes(t *testing.T) {
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
	for route := range affiliateRoutes {
		got = append(got, route)
	}
	for route := range bridgedRoutes {
		got = append(got, route)
	}
	sort.Strings(want)
	sort.Strings(got)
	require.Equal(t, want, got)
}

// The package adopts its four tables, reads the referral, order billing,
// entitlement and settings views, and may adjust balances; it neither adopts
// nor reads v2_user, v2_order or v2_system_config.
func TestAffiliateManifestCapabilities(t *testing.T) {
	raw, err := os.ReadFile("../manifest.template.json")
	require.NoError(t, err)
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.ElementsMatch(t, []string{
		"kernel.storage.v1", "kernel.storage.adopt:v2_commission_record", "kernel.storage.adopt:v2_commission_withdraw",
		"kernel.storage.adopt:v2_invite_config", "kernel.storage.adopt:v2_invite_code",
		"kernel.view:kapi_user_referral_v1", "kernel.view:kapi_order_billing_v1", "kernel.view:kapi_subscriber_entitlement_v1",
		"kernel.view:kapi_affiliate_settings_v1", "kernel.subscriber.balance.v1",
		// The frontend settings are written through KernelSettings; they
		// are read through kapi_affiliate_settings_v1.
		"kernel.settings.invite.write.v1",
	}, manifest.Capabilities)
}

// A native withdrawal debits the commission balance through KernelSubscriber
// on the bridge connection, under the withdrawal's ledger id, and ends
// pending; a debit the kernel refuses leaves no withdrawal.
func TestAffiliateHostWithdrawsThroughKernelSubscriberOverTheBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// A table stands in for the kernel view here.
	require.NoError(t, db.AutoMigrate(&native.Withdrawal{}, &native.InviteConfig{}, &native.Entitlement{}))
	require.NoError(t, db.Create(&native.Entitlement{ID: 2, CommissionBalance: 100}).Error)

	recorder := &balanceRecorder{balance: 60}
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
		modes: map[string]string{native.WithdrawRouteID: "native"},
		lease: packagebridgesdk.StorageLease{
			Driver: "sqlite", DSN: path, TablePrefix: "pkg_affiliate_",
			AdoptedTables: []string{"v2_commission_record", "v2_commission_withdraw", "v2_invite_config"},
		},
	}
	service, err := newAffiliateService(connectedBridge{bridgeStub: stub, conn: conn}, "lease-1")
	require.NoError(t, err)
	service.Refresh(context.Background())
	configured, effective := service.Mode(native.WithdrawRouteID)
	require.Equal(t, "native", configured)
	require.Equal(t, "native", effective)

	withdraw := func(amount string) (pluginhostsdk.DispatchResponse, map[string]any) {
		response := dispatch(t, service, native.WithdrawRouteID, pluginhostsdk.DispatchRequest{
			Method: "POST", PrincipalJSON: []byte(`{"actor_id":2}`),
			RequestBody: []byte(`{"amount":` + amount + `,"method":"bank","account":"acc","name":"Name"}`),
			Metadata:    pluginhostsdk.RequestMetadata{Path: "/api/v2/user/invite/withdraw"},
		})
		var answer map[string]any
		require.NoError(t, json.Unmarshal(response.ResponseBody, &answer), "%s", response.ResponseBody)
		return response, answer
	}

	response, answer := withdraw("40")
	require.EqualValues(t, 200, response.StatusCode, "%v", answer)
	require.Empty(t, stub.operation, "a native route does not reach the legacy handler")
	require.Len(t, recorder.calls, 1)
	want := &kernelsubscriberv1.AdjustBalanceRequest{
		RequestId: "affiliate.withdraw:1", UserId: 2, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION,
		AmountCents: -40, Reason: "commission withdrawal",
	}
	require.True(t, proto.Equal(want, recorder.calls[0]), "debit %v", recorder.calls[0])
	var stored native.Withdrawal
	require.NoError(t, db.Take(&stored, 1).Error)
	require.Equal(t, 0, stored.Status, "pending once debited")
	require.Equal(t, stored.CreatedAt, stored.UpdatedAt)

	// The view still shows 100, the kernel holds 20: the kernel's refusal
	// wins and the reservation is removed.
	response, answer = withdraw("30")
	require.EqualValues(t, 400, response.StatusCode)
	require.Equal(t, "insufficient balance", answer["error"])
	require.Len(t, recorder.calls, 2)
	require.Equal(t, "affiliate.withdraw:2", recorder.calls[1].GetRequestId())
	var count int64
	require.NoError(t, db.Model(&native.Withdrawal{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
