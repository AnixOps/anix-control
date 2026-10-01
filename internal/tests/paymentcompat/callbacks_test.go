package paymentcompat

// The callback routes: provider signatures checked with the gateways'
// secrets, the payment recorded in the package's tables, and the order it
// pays completed by the kernel. The native side calls the real KernelOrder
// server (internal/kernelorder) in process over gRPC, on the native side's
// database; the legacy side runs the kernel's handlers, which complete the
// order in their own transaction through the same function
// (service.CompleteOrderPaymentTx). Both must end with the same payment
// records, gateway statistics, orders, subscribers, subscription groups,
// request ledger and change log.
//
// No test calls a payment provider: the PayPal webhook's verification goes
// through http.DefaultTransport on both sides (the kernel's paypalHTTPClient
// and the package's default client set no transport), which these tests
// replace with a fake PayPal API. Its calls are part of the compared state.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5" // #nosec G501 -- EPay's public callback protocol uses MD5 signatures.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kernelorder"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// The kernel's server registers its payment gateways (EPay).
	_ "github.com/AnixOps/anix-control/v4/internal/payment/gateways"
)

// paymentHost is the identity the kernel serves the payment host as.
var paymentHost = packagebridge.HostIdentity{PackageID: "payment", Version: "4.0.0", Generation: 1}

// completionOnly authorizes what the payment package's signed release
// declares of KernelOrder, for the payment host.
type completionOnly struct{}

func (completionOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == paymentHost && capability == service.CapabilityOrderComplete {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelOrder serves the kernel's KernelOrder on db in process and returns
// a client for it, as the payment host gets one over its bridge.
func kernelOrder(t *testing.T, db *gorm.DB) kernelorderv1.KernelOrderClient {
	t.Helper()
	server := &kernelorder.Server{DB: db, Authorizer: completionOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelorderv1.RegisterKernelOrderServer(grpcServer, server.For(paymentHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelorderv1.NewKernelOrderClient(conn)
}

var callbackModels = []any{
	&model.SubscriptionGroup{}, &model.Plan{}, &model.PlanSubscriptionGroup{}, &model.User{}, &model.UserSubscriptionGroup{},
	&model.Order{}, &model.PaymentGateway{}, &model.PaymentRecord{}, &model.Payment{}, &model.SubscriberRequest{}, &model.SubscriberChange{},
}

func callbackRoute(t *testing.T, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: "POST", Pattern: pattern, RouteID: routeID, Legacy: legacy, Models: callbackModels,
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open:   func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Orders: kernelOrder(t, db),
			}
			return service.Handlers()[routeID]
		},
	}
}

// Gateway secrets of the seed.
const (
	epayKey      = "merchant-key"
	stripeSecret = "whsec_test"
	x402Secret   = "x402-secret"
)

// callbackSeed writes subscribers, plans, orders, gateways and payment
// records in every state a callback meets.
//
// Orders: 1 the buyer's renewal of plan 2 (30.00); 2 another user's; 3 a
// newcomer's first plan 2 (80.00); 4 the buyer's completed order; 5 the
// buyer's cancelled order; 6 the buyer's order paid by another payment.
//
// Payment records, by trade number: PAYE* EPay, X* x402, F* Stripe, P*
// PayPal. *1 pays order 1 in full. PAYE8, X5, F4 and P3 are paid records
// whose order 1 is still pending: the state a failure between the
// package's two steps leaves.
func callbackSeed(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{
		{ID: 5, Name: "group 5", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 7, Name: "group 7", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 9, Name: "group 9", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.Plan{
		{ID: 1, GroupID: 1, TransferEnable: 10, Name: "Basic", Show: 1, Renew: 1, MonthPrice: ptr(int64(1000)), CreatedAt: seeded, UpdatedAt: seeded},
		{
			ID: 2, GroupID: 2, TransferEnable: 100, SpeedLimit: ptr(int64(200)), DeviceLimit: ptr(5), Name: "Pro", Show: 1, Renew: 1,
			MonthPrice: ptr(int64(3000)), QuarterPrice: ptr(int64(8000)), CreatedAt: seeded, UpdatedAt: seeded,
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.PlanSubscriptionGroup{
		{ID: 1, PlanID: 2, GroupID: 9, CreatedAt: seeded}, {ID: 2, PlanID: 2, GroupID: 7, CreatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1},
		{
			ID: 2, Email: "buyer@example.test", Token: "t2", UUID: "u2", PlanID: ptr(uint(2)), GroupID: ptr(uint(2)), TransferEnable: 100 << 30,
			U: 100, D: 200, ExpiredAt: ptr(int64(1_900_000_000)), SpeedLimit: ptr(int64(200)), DeviceLimit: ptr(5),
		},
		{ID: 3, Email: "other@example.test", Token: "t3", UUID: "u3"},
		{ID: 4, Email: "newcomer@example.test", Token: "t4", UUID: "u4", U: 5, D: 6},
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{ID: 1, UserID: 2, GroupID: 5, CreatedAt: seeded}).Error)
	order := func(id, user, plan uint, period string, status int, total int64, paidAt *int64) model.Order {
		return model.Order{
			ID: id, UserID: user, PlanID: plan, Type: 1, Period: period, TradeNo: fmt.Sprintf("T%d", id), TotalAmount: total,
			DiscountAmount: ptr(int64(0)), Status: status, PaidAt: paidAt, CreatedAt: seeded, UpdatedAt: seeded,
		}
	}
	require.NoError(t, db.Create(&[]model.Order{
		order(1, 2, 2, "month", 0, 3000, nil),
		order(2, 3, 1, "month", 0, 1000, nil),
		order(3, 4, 2, "quarter", 0, 8000, nil),
		order(4, 2, 1, "month", 3, 1000, ptr(int64(1_700_000_000))),
		order(5, 2, 1, "month", 2, 1000, nil),
		order(6, 2, 2, "month", 1, 3000, ptr(int64(1_700_000_000))),
	}).Error)
	require.NoError(t, db.Create(&[]model.PaymentGateway{
		{
			ID: 1, Name: "EPay", Type: "epay", Enabled: true, Config: `{"api_url":"https://pay.example.test","pid":"1001","key":"` + epayKey + `"}`,
			FeeRate: 0.01, MinAmount: 1, MaxAmount: 10000, TotalOrders: 3, TotalAmount: 152.5, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 2, Name: "Stripe", Type: "stripe", Enabled: true, Config: `{"publishable_key":"pk_test","secret_key":"sk_test","webhook_secret":"` + stripeSecret + `"}`,
			MinAmount: 1, MaxAmount: 10000, TotalOrders: 1, TotalAmount: 9, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 3, Name: "PayPal", Type: "paypal", Enabled: true, Config: `{"client_id":"cid","client_secret":"paypal-secret","webhook_id":"WH-1","sandbox_mode":true}`,
			MinAmount: 1, MaxAmount: 10000, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 4, Name: "x402", Type: "x402", Enabled: true, Config: `{"webhook_secret":"` + x402Secret + `","accept_tokens":["ETH ","usdc"]}`,
			MinAmount: 1, MaxAmount: 10000, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{ID: 5, Name: "Old EPay", Type: "epay", Config: `{"key":"old-key"}`, MinAmount: 1, MaxAmount: 10000, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	record := func(id uint, tradeNo string, gateway uint, gatewayType, provider string, user uint, amount, actual float64, currency string, order *uint) model.PaymentRecord {
		return model.PaymentRecord{
			ID: id, TradeNo: tradeNo, GatewayID: gateway, GatewayType: gatewayType, Provider: provider, UserID: user, Amount: amount,
			FeeAmount: actual - amount, ActualAmount: actual, Currency: currency, OrderID: order, CreatedAt: at(1, 8), UpdatedAt: at(1, 8),
		}
	}
	paid := func(r model.PaymentRecord) model.PaymentRecord {
		r.Status, r.PaidAt, r.GatewayTradeNo, r.NotifyData, r.UpdatedAt = 1, ptr(at(1, 9)), "GW-EARLIER", "earlier", at(1, 9)
		return r
	}
	crypto := func(r model.PaymentRecord) model.PaymentRecord {
		r.WalletAddress, r.Network = ptr("0xwallet"), ptr("sepolia")
		return r
	}
	one, two, three, four, five, six := ptr(uint(1)), ptr(uint(2)), ptr(uint(3)), ptr(uint(4)), ptr(uint(5)), ptr(uint(6))
	require.NoError(t, db.Create(&[]model.PaymentRecord{
		record(1, "PAYE1", 1, "epay", "", 2, 30, 30.3, "CNY", one),
		record(2, "PAYE2", 1, "epay", "", 2, 10, 10, "CNY", two),
		record(3, "PAYE3", 1, "epay", "", 2, 1, 1, "CNY", one),
		record(4, "PAYE4", 1, "epay", "", 2, 10, 10, "CNY", four),
		record(5, "PAYE5", 1, "epay", "", 2, 10, 10, "CNY", five),
		record(6, "PAYE6", 1, "epay", "", 2, 50, 50, "CNY", nil),
		record(7, "PAYE7", 1, "epay", "", 4, 80, 80, "CNY", three),
		paid(record(8, "PAYE8", 1, "epay", "", 2, 30, 30, "CNY", one)),
		record(9, "PAYE9", 1, "epay", "", 2, 30, 30, "CNY", six),
		crypto(record(10, "X1", 0, "crypto", "x402", 2, 30, 0.00003, "ETH", one)),
		crypto(record(11, "X2", 0, "crypto", "x402", 2, 30, 0.00003, "USDT", one)),
		crypto(record(12, "X3", 0, "crypto", "x402", 2, 10, 0.00001, "ETH", two)),
		crypto(record(13, "X4", 0, "crypto", "x402", 2, 30, 0.00003, "CNY", one)),
		paid(crypto(record(14, "X5", 0, "crypto", "x402", 2, 30, 0.00003, "ETH", one))),
		record(15, "F1", 0, "fiat", "stripe", 2, 30, 30, "USD", one),
		record(16, "F2", 0, "fiat", "stripe", 4, 80, 80, "USD", three),
		record(17, "F3", 0, "fiat", "stripe", 2, 10, 10, "USD", two),
		paid(record(18, "F4", 0, "fiat", "stripe", 2, 30, 30, "USD", one)),
		record(19, "P1", 3, "fiat", "paypal", 2, 30, 30, "USD", one),
		record(20, "P2", 3, "fiat", "paypal", 2, 10, 10, "USD", four),
		paid(record(21, "P3", 3, "fiat", "paypal", 2, 30, 30, "USD", one)),
	}).Error)
	syncCallbackSequences(t, db)
}

// callbackSeedWith is callbackSeed with changes applied after it.
func callbackSeedWith(statements ...string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		callbackSeed(t, db)
		for _, statement := range statements {
			require.NoError(t, db.Exec(statement).Error, statement)
		}
	}
}

// orphanSeed adds the buyer's pending order 7 of a plan that no longer
// exists, paid in full by PAYE10. PostgreSQL enforces the kernel's foreign
// key from the order to its plan, so it is dropped first.
func orphanSeed(t testing.TB, db *gorm.DB) {
	callbackSeed(t, db)
	if db.Name() == "postgres" {
		require.NoError(t, db.Exec("ALTER TABLE v2_order DROP CONSTRAINT IF EXISTS fk_v2_order_plan").Error)
	}
	require.NoError(t, db.Create(&model.Order{
		ID: 7, UserID: 2, PlanID: 404, Type: 1, Period: "month", TradeNo: "T7", TotalAmount: 1000, CreatedAt: seeded, UpdatedAt: seeded,
	}).Error)
	require.NoError(t, db.Create(&model.PaymentRecord{
		ID: 22, TradeNo: "PAYE10", GatewayID: 1, GatewayType: "epay", UserID: 2, Amount: 10, ActualAmount: 10, Currency: "CNY",
		OrderID: ptr(uint(7)), CreatedAt: at(1, 8), UpdatedAt: at(1, 8),
	}).Error)
	syncCallbackSequences(t, db)
}

// syncCallbackSequences moves PostgreSQL's id sequences past explicitly
// seeded ids, so both sides create the next row with the same id.
func syncCallbackSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{
		"v2_subscription_group", "v2_plan", "v2_plan_subscription_group", "v2_user", "v2_user_subscription_group", "v2_order",
		"v2_payment_gateway", "v2_payment_record",
	} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockUnix masks a Unix time the handlers derive from their clock: now,
// or now plus an order's period.
func clockUnix(value int64) any {
	now := time.Now()
	for _, ref := range []struct {
		label string
		at    time.Time
	}{
		{"<now>", now}, {"<now+1mo>", now.AddDate(0, 1, 0)}, {"<now+3mo>", now.AddDate(0, 3, 0)},
	} {
		if d := value - ref.at.Unix(); d > -600 && d < 600 {
			return ref.label
		}
	}
	return value
}

func clockUnixPtr(value *int64) any {
	if value == nil {
		return nil
	}
	return clockUnix(*value)
}

// callbackState is everything a callback can change: the payment records,
// the gateways' statistics, the orders, the subscribers' entitlements and
// subscription groups, the subscriber request ledger
// (v4_kernel_subscriber_request: "payment:<trade_no>" and "order:<id>")
// and change log (v4_kernel_subscriber_change).
func callbackState(t testing.TB, db *gorm.DB) any {
	var records []model.PaymentRecord
	require.NoError(t, db.Order("id").Find(&records).Error)
	recordRows := make([]map[string]any, 0, len(records))
	for _, row := range records {
		recordRows = append(recordRows, map[string]any{
			"id": row.ID, "trade_no": row.TradeNo, "gateway_id": row.GatewayID, "gateway_type": row.GatewayType,
			"gateway_trade_no": row.GatewayTradeNo, "provider": row.Provider, "user_id": row.UserID, "amount": row.Amount,
			"fee_amount": row.FeeAmount, "actual_amount": row.ActualAmount, "currency": row.Currency, "status": row.Status,
			"paid_at": clockTimePtr(row.PaidAt), "cancelled_at": clockTimePtr(row.CancelledAt), "refunded_at": clockTimePtr(row.RefundedAt),
			"notify_data": row.NotifyData, "client_ip": row.ClientIP, "tx_hash": row.TxHash, "wallet_address": row.WalletAddress,
			"network": row.Network, "order_id": row.OrderID, "created_at": clockTime(row.CreatedAt), "updated_at": clockTime(row.UpdatedAt),
		})
	}
	var gatewayRows []model.PaymentGateway
	require.NoError(t, db.Order("id").Find(&gatewayRows).Error)
	gatewayStats := make([]map[string]any, 0, len(gatewayRows))
	for _, row := range gatewayRows {
		gatewayStats = append(gatewayStats, map[string]any{
			"id": row.ID, "enabled": row.Enabled, "config": row.Config, "total_orders": row.TotalOrders, "total_amount": row.TotalAmount,
			"updated_at": clockTime(row.UpdatedAt),
		})
	}
	var orders []model.Order
	require.NoError(t, db.Order("id").Find(&orders).Error)
	orderRows := make([]map[string]any, 0, len(orders))
	for _, row := range orders {
		orderRows = append(orderRows, map[string]any{
			"id": row.ID, "user_id": row.UserID, "plan_id": row.PlanID, "status": row.Status, "paid_at": clockUnixPtr(row.PaidAt),
			"total_amount": row.TotalAmount, "commission_status": row.CommissionStatus, "updated_at": clockTime(row.UpdatedAt),
		})
	}
	var users []struct {
		ID             uint
		PlanID         *uint
		GroupID        *uint
		TransferEnable int64
		U              int64
		D              int64
		SpeedLimit     *int64
		DeviceLimit    *int
		ExpiredAt      *int64
		Balance        int64
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	userRows := make([]map[string]any, 0, len(users))
	for _, user := range users {
		userRows = append(userRows, map[string]any{
			"id": user.ID, "plan_id": user.PlanID, "group_id": user.GroupID, "transfer_enable": user.TransferEnable, "u": user.U, "d": user.D,
			"speed_limit": user.SpeedLimit, "device_limit": user.DeviceLimit, "expired_at": clockUnixPtr(user.ExpiredAt), "balance": user.Balance,
		})
	}
	var groups []struct {
		ID      uint
		UserID  uint
		GroupID uint
	}
	require.NoError(t, db.Model(&model.UserSubscriptionGroup{}).Order("id").Find(&groups).Error)
	var requests []model.SubscriberRequest
	require.NoError(t, db.Order("request_id").Find(&requests).Error)
	ledger := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(request.Result), &result))
		if expires, ok := result["expires_at"].(float64); ok {
			result["expires_at"] = clockUnix(int64(expires))
		}
		ledger = append(ledger, map[string]any{
			"request_id": request.RequestID, "method": request.Method, "user_id": request.UserID, "result": result,
			"created_at": clockTime(request.CreatedAt),
		})
	}
	var changes []struct {
		ID        uint64
		UserID    uint
		Deleted   bool
		CreatedAt time.Time
	}
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
	changeRows := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		changeRows = append(changeRows, map[string]any{"id": change.ID, "user_id": change.UserID, "deleted": change.Deleted, "created_at": clockTime(change.CreatedAt)})
	}
	return map[string]any{
		"records": recordRows, "gateways": gatewayStats, "orders": orderRows, "users": userRows, "groups": groups,
		"ledger": ledger, "changes": changeRows,
	}
}

// expect wraps a snapshot with checks on the native side's own rows that
// parity alone cannot show, since both sides complete orders through the
// same kernel function: which orders are completed and which are not.
func expect(orderStatuses map[uint]int, outcomes map[string]string) func(testing.TB, *gorm.DB) any {
	return func(t testing.TB, db *gorm.DB) any {
		for id, want := range orderStatuses {
			var order model.Order
			require.NoError(t, db.Take(&order, id).Error)
			require.Equal(t, want, order.Status, "order %d", id)
		}
		for tradeNo, want := range outcomes {
			var row model.SubscriberRequest
			err := db.Take(&row, "request_id = ?", service.OrderPaymentRequestID(tradeNo)).Error
			if want == "" {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound, "payment %s completes no order", tradeNo)
				continue
			}
			require.NoError(t, err, "payment %s", tradeNo)
			require.Contains(t, row.Result, `"outcome":"`+want+`"`, "payment %s", tradeNo)
		}
		return callbackState(t, db)
	}
}

func runCallbacks(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = callbackSeed
		}
		if c.Snapshot == nil {
			c.Snapshot = callbackState
		}
		packagecompat.RunWrite(t, r, c)
	}
}

// epayPath is a signed EPay notification for /payment/callback/epay.
func epayPath(params map[string]string, key string) string {
	values := url.Values{}
	for name, value := range params {
		values.Set(name, value)
	}
	keys := make([]string, 0, len(values))
	for name, value := range values {
		if name == "sign" || name == "sign_type" || value[0] == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var signed strings.Builder
	for i, name := range keys {
		if i > 0 {
			signed.WriteByte('&')
		}
		signed.WriteString(name + "=" + values.Get(name))
	}
	signed.WriteString(key)
	sum := md5.Sum([]byte(signed.String())) // #nosec G401 -- EPay's public callback protocol uses MD5 signatures.
	if _, preset := params["sign"]; !preset {
		values.Set("sign", hex.EncodeToString(sum[:]))
	}
	values.Set("sign_type", "MD5")
	return "/api/v2/payment/callback/epay?" + values.Encode()
}

func epayPaid(tradeNo, money string) map[string]string {
	return map[string]string{"pid": "1001", "out_trade_no": tradeNo, "trade_no": "EP-" + tradeNo, "trade_status": "TRADE_SUCCESS", "money": money}
}

func TestPaymentCallbackParity(t *testing.T) {
	r := callbackRoute(t, "/api/v2/payment/callback/:type", native.CallbackRouteID, gateways((*handler.PaymentGatewayHandler).PaymentCallback))
	completed := map[uint]int{1: 3}
	untouched := map[uint]int{1: 0}
	runCallbacks(t, r, []packagecompat.Case{
		{Name: "a signed payment renews the buyer's plan", Path: epayPath(epayPaid("PAYE1", "30.30"), epayKey),
			Snapshot: expect(completed, map[string]string{"PAYE1": "completed"})},
		{Name: "a newcomer's first plan with its subscription groups", Path: epayPath(epayPaid("PAYE7", "80.00"), epayKey),
			Snapshot: expect(map[uint]int{3: 3}, map[string]string{"PAYE7": "completed"})},
		{Name: "a repeat answers fail and grants once", Path: epayPath(epayPaid("PAYE1", "30.30"), epayKey), Warmup: [][]byte{nil},
			Snapshot: expect(completed, map[string]string{"PAYE1": "completed"})},
		{Name: "a signed amount that is not the payment's", Path: epayPath(epayPaid("PAYE1", "0.01"), epayKey),
			Snapshot: expect(untouched, map[string]string{"PAYE1": ""})},
		{Name: "a forged signature", Path: epayPath(map[string]string{"pid": "1001", "out_trade_no": "PAYE1", "trade_no": "EP", "trade_status": "TRADE_SUCCESS", "money": "30.30", "sign": "deadbeefdeadbeefdeadbeefdeadbeef"}, epayKey),
			Snapshot: expect(untouched, nil)},
		{Name: "signed with another key", Path: epayPath(epayPaid("PAYE1", "30.30"), "old-key"), Snapshot: expect(untouched, nil)},
		{Name: "a pending notification changes nothing", Path: epayPath(map[string]string{"pid": "1001", "out_trade_no": "PAYE1", "trade_no": "EP", "trade_status": "WAIT_BUYER_PAY"}, epayKey),
			Snapshot: expect(untouched, nil)},
		{Name: "an unknown trade number", Path: epayPath(epayPaid("NOPE", "30.30"), epayKey)},
		{Name: "a gateway type without a callback", Path: strings.Replace(epayPath(epayPaid("PAYE1", "30.30"), epayKey), "/epay?", "/alipay?", 1)},
		{Name: "no enabled EPay gateway", Path: epayPath(epayPaid("PAYE1", "30.30"), epayKey), Seed: callbackSeedWith("UPDATE v2_payment_gateway SET enabled = false WHERE id = 1"),
			Snapshot: expect(untouched, nil)},
		{Name: "another user's order is left unchanged", Path: epayPath(epayPaid("PAYE2", "10.00"), epayKey),
			Snapshot: expect(map[uint]int{2: 0}, map[string]string{"PAYE2": "refused"})},
		{Name: "a payment short of the order's total", Path: epayPath(epayPaid("PAYE3", "1.00"), epayKey),
			Snapshot: expect(untouched, map[string]string{"PAYE3": "refused"})},
		{Name: "an order completed before", Path: epayPath(epayPaid("PAYE4", "10.00"), epayKey),
			Snapshot: expect(map[uint]int{4: 3}, map[string]string{"PAYE4": "refused"})},
		{Name: "a cancelled order", Path: epayPath(epayPaid("PAYE5", "10.00"), epayKey),
			Snapshot: expect(map[uint]int{5: 2}, map[string]string{"PAYE5": "refused"})},
		{Name: "an order paid by another payment", Path: epayPath(epayPaid("PAYE9", "30.00"), epayKey),
			Snapshot: expect(map[uint]int{6: 1}, map[string]string{"PAYE9": "refused"})},
		{Name: "a top-up pays no order", Path: epayPath(epayPaid("PAYE6", "50.00"), epayKey), Snapshot: expect(untouched, map[string]string{"PAYE6": ""})},
		{Name: "a repeat completes an order left pending", Path: epayPath(epayPaid("PAYE8", "30.00"), epayKey),
			Snapshot: expect(completed, map[string]string{"PAYE8": "completed"})},
		{Name: "a plan that no longer exists leaves the order paid", Path: epayPath(epayPaid("PAYE10", "10.00"), epayKey), Seed: orphanSeed,
			Snapshot: expect(map[uint]int{7: 1}, map[string]string{"PAYE10": "paid"})},
	})
}

// x402Body is a signed x402 confirmation.
func x402Body(fields map[string]any, secret string) []byte {
	sign := map[string]string{}
	for _, name := range []string{"trade_no", "tx_hash", "status", "amount", "token"} {
		value, _ := fields[name].(string)
		sign[name] = value
	}
	block, _ := fields["block_number"].(int)
	confirmations, _ := fields["confirmations"].(int)
	sign["block_number"], sign["confirmations"] = strconv.Itoa(block), strconv.Itoa(confirmations)
	keys := make([]string, 0, len(sign))
	for name := range sign {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+sign[name])
	}
	if _, preset := fields["signature"]; !preset {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(strings.Join(parts, "&")))
		fields["signature"] = hex.EncodeToString(mac.Sum(nil))
	}
	body, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return body
}

func x402Confirmed(tradeNo, amount, token string) map[string]any {
	return map[string]any{
		"trade_no": tradeNo, "tx_hash": "0xtx-" + tradeNo, "block_number": 42, "confirmations": 3, "status": "confirmed",
		"amount": amount, "token": token,
	}
}

func with(fields map[string]any, name string, value any) map[string]any {
	fields[name] = value
	return fields
}

func TestX402CallbackParity(t *testing.T) {
	r := callbackRoute(t, "/api/v2/payment/x402/callback", native.X402CallbackRouteID, payments((*handler.PaymentHandler).X402Callback))
	path := "/api/v2/payment/x402/callback"
	completed := map[uint]int{1: 3}
	untouched := map[uint]int{1: 0}
	runCallbacks(t, r, []packagecompat.Case{
		{Name: "the payment's token and amount", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003000", "eth"), x402Secret),
			Snapshot: expect(completed, map[string]string{"X1": "completed"})},
		{Name: "more than asked", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.5", "ETH"), "status", "success"), x402Secret),
			Snapshot: expect(completed, map[string]string{"X1": "completed"})},
		{Name: "a repeat is already processed", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003", "ETH"), x402Secret), Warmup: [][]byte{nil},
			Snapshot: expect(completed, map[string]string{"X1": "completed"})},
		{Name: "another token", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003", "USDC"), x402Secret),
			Snapshot: expect(untouched, map[string]string{"X1": ""})},
		{Name: "a token the gateway does not accept", Path: path, Body: x402Body(x402Confirmed("X2", "0.00003", "USDT"), x402Secret),
			Snapshot: expect(untouched, map[string]string{"X2": ""})},
		{Name: "less than asked", Path: path, Body: x402Body(x402Confirmed("X1", "0.00002999", "ETH"), x402Secret),
			Snapshot: expect(untouched, map[string]string{"X1": ""})},
		{Name: "an amount that is not a decimal", Path: path, Body: x402Body(x402Confirmed("X1", "3e-5", "ETH"), x402Secret)},
		{Name: "a payment created without a token", Path: path, Body: x402Body(x402Confirmed("X4", "0.00003", "CNY"), x402Secret),
			Snapshot: expect(untouched, map[string]string{"X4": ""})},
		{Name: "another user's order is left unchanged", Path: path, Body: x402Body(x402Confirmed("X3", "0.00001", "ETH"), x402Secret),
			Snapshot: expect(map[uint]int{2: 0}, map[string]string{"X3": "refused"})},
		{Name: "a repeat completes an order left pending", Path: path, Body: x402Body(x402Confirmed("X5", "0.00003", "ETH"), x402Secret),
			Snapshot: expect(completed, map[string]string{"X5": "completed"})},
		{Name: "not yet confirmed", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.00003", "ETH"), "confirmations", 0), x402Secret)},
		{Name: "a failed transfer cancels the payment", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.00003", "ETH"), "status", "failed"), x402Secret)},
		{Name: "a transfer still confirming", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.00003", "ETH"), "status", "confirming"), x402Secret)},
		{Name: "a forged signature", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.00003", "ETH"), "signature", "00ff"), x402Secret)},
		{Name: "signed with another secret", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003", "ETH"), "guess")},
		{Name: "no signature", Path: path, Body: x402Body(with(x402Confirmed("X1", "0.00003", "ETH"), "signature", ""), x402Secret)},
		{Name: "an unknown trade number", Path: path, Body: x402Body(x402Confirmed("NOPE", "0.00003", "ETH"), x402Secret)},
		{Name: "no x402 gateway secret", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003", "ETH"), ""),
			Seed: callbackSeedWith(`UPDATE v2_payment_gateway SET config = '{"accept_tokens":["ETH"]}' WHERE id = 4`)},
		{Name: "an unreadable x402 configuration", Path: path, Body: x402Body(x402Confirmed("X1", "0.00003", "ETH"), x402Secret),
			Seed: callbackSeedWith(`UPDATE v2_payment_gateway SET config = '{"webhook_secret":"x402-secret","confirm_blocks":"six"}' WHERE id = 4`)},
		{Name: "a body that is not JSON", Path: path, Body: []byte(`{"trade_no":`)},
		{Name: "a field of the wrong type", Path: path, Body: []byte(`{"trade_no":"X1","block_number":"x"}`)},
		{Name: "no body", Path: path},
	})
}

// stripeHeaders sign a Stripe event at now.
func stripeHeaders(body []byte, secret string, at time.Time) map[string]string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(body)))
	return map[string]string{"Stripe-Signature": "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))}
}

func stripeEvent(kind, sessionID string, metadata, reference string) []byte {
	object := map[string]any{"id": sessionID, "status": "complete", "amount_total": 3000, "currency": "usd"}
	if metadata != "" {
		object["metadata"] = map[string]string{"trade_no": metadata}
	}
	if reference != "" {
		object["client_reference_id"] = reference
	}
	body, err := json.Marshal(map[string]any{"id": "evt_" + sessionID, "type": kind, "data": map[string]any{"object": object}})
	if err != nil {
		panic(err)
	}
	return body
}

func TestStripeWebhookParity(t *testing.T) {
	r := callbackRoute(t, "/api/v2/payment/stripe/webhook", native.StripeWebhookRouteID, payments((*handler.PaymentHandler).StripeWebhook))
	path := "/api/v2/payment/stripe/webhook"
	now := time.Now()
	signed := func(name string, body []byte) packagecompat.Case {
		return packagecompat.Case{Name: name, Path: path, Body: body, RequestHeaders: stripeHeaders(body, stripeSecret, now)}
	}
	expecting := func(c packagecompat.Case, orders map[uint]int, outcomes map[string]string) packagecompat.Case {
		c.Snapshot = expect(orders, outcomes)
		return c
	}
	completedEvent := stripeEvent("checkout.session.completed", "cs_1", "F1", "")
	repeat := signed("a repeat is already processed", completedEvent)
	repeat.Warmup = [][]byte{completedEvent}
	repeat.Snapshot = expect(map[uint]int{1: 3}, map[string]string{"F1": "completed"})
	noSecret := signed("no Stripe webhook secret", completedEvent)
	noSecret.Seed = callbackSeedWith("UPDATE v2_payment_gateway SET enabled = false WHERE id = 2")
	stale := packagecompat.Case{Name: "a signature ten minutes old", Path: path, Body: completedEvent,
		RequestHeaders: stripeHeaders(completedEvent, stripeSecret, now.Add(-10*time.Minute))}
	runCallbacks(t, r, []packagecompat.Case{
		expecting(signed("a completed checkout renews the buyer's plan", completedEvent), map[uint]int{1: 3}, map[string]string{"F1": "completed"}),
		expecting(signed("the client reference names the payment", stripeEvent("checkout.session.completed", "cs_2", "", "F2")), map[uint]int{3: 3}, map[string]string{"F2": "completed"}),
		repeat,
		expecting(signed("another user's order is left unchanged", stripeEvent("checkout.session.completed", "cs_3", "F3", "")), map[uint]int{2: 0}, map[string]string{"F3": "refused"}),
		expecting(signed("a repeat completes an order left pending", stripeEvent("checkout.session.completed", "cs_4", "F4", "")), map[uint]int{1: 3}, map[string]string{"F4": "completed"}),
		signed("an unknown trade number", stripeEvent("checkout.session.completed", "cs_5", "NOPE", "")),
		signed("no trade number", stripeEvent("checkout.session.completed", "cs_6", "", "")),
		signed("an expired checkout", stripeEvent("checkout.session.expired", "cs_7", "F1", "")),
		signed("an expired checkout of a paid payment", stripeEvent("checkout.session.expired", "cs_8", "F4", "")),
		signed("a failed asynchronous payment", stripeEvent("checkout.session.async_payment_failed", "cs_9", "", "F1")),
		signed("an expired checkout without a trade number", stripeEvent("checkout.session.expired", "cs_10", "", "")),
		signed("an event it does not handle", stripeEvent("charge.refunded", "ch_1", "F1", "")),
		signed("a signed body that is not an event", []byte(`{"type":`)),
		{Name: "a forged signature", Path: path, Body: completedEvent, RequestHeaders: map[string]string{"Stripe-Signature": "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=deadbeef"}},
		{Name: "no signature", Path: path, Body: completedEvent},
		{Name: "a malformed timestamp", Path: path, Body: completedEvent, RequestHeaders: map[string]string{"Stripe-Signature": "t=yesterday,v1=deadbeef"}},
		stale,
		noSecret,
	})
}

// fakePayPal is PayPal's API, as far as the webhook's verification tells:
// it issues a token for the seeded client credentials and confirms a
// delivery whose transmission signature is "sig-ok". It records every call.
type fakePayPal struct {
	t     *testing.T
	mu    sync.Mutex
	calls []string
}

func (f *fakePayPal) RoundTrip(request *http.Request) (*http.Response, error) {
	body := []byte{}
	if request.Body != nil {
		var err error
		if body, err = io.ReadAll(request.Body); err != nil {
			return nil, err
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, request.Method+" "+request.URL.String()+" "+request.Header.Get("Authorization")+" "+string(body))
	f.mu.Unlock()
	answer := func(code int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	}
	if request.URL.Host != "api-m.sandbox.paypal.com" && request.URL.Host != "api-m.paypal.com" {
		f.t.Errorf("a callback called %s, which is not PayPal's API", request.URL)
		return nil, errors.New("not PayPal")
	}
	switch request.URL.Path {
	case "/v1/oauth2/token":
		if user, password, ok := request.BasicAuth(); !ok || user != "cid" || password != "paypal-secret" {
			return answer(http.StatusUnauthorized, `{"error":"invalid_client"}`)
		}
		return answer(http.StatusOK, `{"access_token":"paypal-test-token"}`)
	case "/v1/notifications/verify-webhook-signature":
		var verify struct {
			TransmissionSig string `json:"transmission_sig"`
			WebhookID       string `json:"webhook_id"`
		}
		if request.Header.Get("Authorization") != "Bearer paypal-test-token" || json.Unmarshal(body, &verify) != nil {
			return answer(http.StatusUnauthorized, `{"name":"AUTHENTICATION_FAILURE"}`)
		}
		if verify.TransmissionSig == "sig-ok" && verify.WebhookID == "WH-1" {
			return answer(http.StatusOK, `{"verification_status":"SUCCESS"}`)
		}
		return answer(http.StatusOK, `{"verification_status":"FAILURE"}`)
	}
	return answer(http.StatusNotFound, `{"name":"RESOURCE_NOT_FOUND"}`)
}

// take returns and forgets the calls recorded so far.
func (f *fakePayPal) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

// installFakePayPal routes http.DefaultTransport to a fake PayPal until the
// test ends.
func installFakePayPal(t *testing.T) *fakePayPal {
	fake := &fakePayPal{t: t}
	previous := http.DefaultTransport
	http.DefaultTransport = fake
	t.Cleanup(func() { http.DefaultTransport = previous })
	return fake
}

func paypalHeaders(signature string) map[string]string {
	return map[string]string{
		"Paypal-Auth-Algo": "SHA256withRSA", "Paypal-Cert-Url": "https://api-m.sandbox.paypal.com/certs/test.pem",
		"Paypal-Transmission-Id": "tx-1", "Paypal-Transmission-Sig": signature, "Paypal-Transmission-Time": "2026-10-01T00:00:00Z",
	}
}

func paypalEvent(kind, customID, value, currency string) []byte {
	resource := map[string]any{"id": "CAP-" + customID, "status": "COMPLETED"}
	if customID != "" {
		resource["custom_id"] = customID
	}
	if value != "" || currency != "" {
		resource["amount"] = map[string]string{"value": value, "currency_code": currency}
	}
	body, err := json.Marshal(map[string]any{"id": "WH-EVT", "event_type": kind, "resource": resource})
	if err != nil {
		panic(err)
	}
	return body
}

func TestPayPalWebhookParity(t *testing.T) {
	fake := installFakePayPal(t)
	r := callbackRoute(t, "/api/v2/payment/paypal/webhook", native.PayPalWebhookRouteID, payments((*handler.PaymentHandler).PayPalWebhook))
	path := "/api/v2/payment/paypal/webhook"
	// Each side's PayPal calls are part of its state: the seed forgets the
	// other side's.
	seeding := func(seed func(testing.TB, *gorm.DB)) func(testing.TB, *gorm.DB) {
		return func(t testing.TB, db *gorm.DB) {
			seed(t, db)
			fake.take()
		}
	}
	state := func(snapshot func(testing.TB, *gorm.DB) any) func(testing.TB, *gorm.DB) any {
		return func(t testing.TB, db *gorm.DB) any {
			return map[string]any{"rows": snapshot(t, db), "paypal": fake.take()}
		}
	}
	// A verified delivery asked PayPal: a token, then the verification.
	asked := func(snapshot func(testing.TB, *gorm.DB) any) func(testing.TB, *gorm.DB) any {
		return func(t testing.TB, db *gorm.DB) any {
			answer := state(snapshot)(t, db).(map[string]any)
			calls, _ := answer["paypal"].([]string)
			require.GreaterOrEqual(t, len(calls), 2, "%v", calls)
			require.True(t, strings.HasPrefix(calls[0], "POST https://api-m.sandbox.paypal.com/v1/oauth2/token Basic "), calls[0])
			require.True(t, strings.HasPrefix(calls[1], "POST https://api-m.sandbox.paypal.com/v1/notifications/verify-webhook-signature Bearer paypal-test-token "), calls[1])
			return answer
		}
	}
	verified := func(name string, body []byte, orders map[uint]int, outcomes map[string]string) packagecompat.Case {
		return packagecompat.Case{Name: name, Path: path, Body: body, RequestHeaders: paypalHeaders("sig-ok"), Snapshot: asked(expect(orders, outcomes))}
	}
	captured := paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P1", "30.00", "USD")
	repeat := verified("a repeat is already processed", captured, map[uint]int{1: 3}, map[string]string{"P1": "completed"})
	repeat.Warmup = [][]byte{captured}
	cases := []packagecompat.Case{
		verified("a completed capture renews the buyer's plan", captured, map[uint]int{1: 3}, map[string]string{"P1": "completed"}),
		repeat,
		verified("a capture in another currency", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P1", "30.00", "EUR"), map[uint]int{1: 0}, map[string]string{"P1": ""}),
		verified("a capture of another amount", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P1", "29.00", "usd"), map[uint]int{1: 0}, map[string]string{"P1": ""}),
		verified("a capture amount that is not a number", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P1", "thirty", "USD"), map[uint]int{1: 0}, nil),
		verified("a capture of an unknown payment", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "NOPE", "30.00", "USD"), nil, nil),
		verified("a capture without a custom id", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "", "30.00", "USD"), nil, nil),
		verified("a capture for an order completed before", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P2", "10.00", "USD"), map[uint]int{4: 3}, map[string]string{"P2": "refused"}),
		verified("a repeat completes an order left pending", paypalEvent("PAYMENT.CAPTURE.COMPLETED", "P3", "30.00", "USD"), map[uint]int{1: 3}, map[string]string{"P3": "completed"}),
		verified("an approved checkout collects nothing", paypalEvent("CHECKOUT.ORDER.APPROVED", "P1", "30.00", "USD"), map[uint]int{1: 0}, map[string]string{"P1": ""}),
		verified("a completed checkout waits for its captures", paypalEvent("CHECKOUT.ORDER.COMPLETED", "P1", "", ""), map[uint]int{1: 0}, nil),
		verified("a denied capture cancels the payment", paypalEvent("PAYMENT.CAPTURE.DENIED", "P1", "", ""), map[uint]int{1: 0}, nil),
		verified("a processing order without a custom id", paypalEvent("CHECKOUT.ORDER.PROCESSING", "", "", ""), nil, nil),
		verified("a verified body that is not an event", []byte(`{"event_type":5}`), nil, nil),
		// The body cannot be sent to PayPal for verification: refused after
		// the token request.
		{Name: "a body that is not JSON", Path: path, Body: []byte(`{"event_type":`), RequestHeaders: paypalHeaders("sig-ok"), Snapshot: state(callbackState)},
		{Name: "PayPal does not confirm the delivery", Path: path, Body: captured, RequestHeaders: paypalHeaders("sig-forged"), Snapshot: asked(expect(map[uint]int{1: 0}, nil))},
		{Name: "no transmission signature", Path: path, Body: captured, RequestHeaders: map[string]string{"Paypal-Transmission-Id": "tx-1"}, Snapshot: state(callbackState)},
		{Name: "no webhook id", Path: path, Body: captured, RequestHeaders: paypalHeaders("sig-ok"), Snapshot: state(callbackState),
			Seed: callbackSeedWith(`UPDATE v2_payment_gateway SET config = '{"client_id":"cid","client_secret":"paypal-secret","sandbox_mode":true}' WHERE id = 3`)},
		{Name: "client credentials PayPal refuses", Path: path, Body: captured, RequestHeaders: paypalHeaders("sig-ok"), Snapshot: state(callbackState),
			Seed: callbackSeedWith(`UPDATE v2_payment_gateway SET config = '{"client_id":"cid","client_secret":"wrong","webhook_id":"WH-1","sandbox_mode":true}' WHERE id = 3`)},
		{Name: "no enabled PayPal gateway", Path: path, Body: captured, RequestHeaders: paypalHeaders("sig-ok"), Snapshot: state(callbackState),
			Seed: callbackSeedWith("UPDATE v2_payment_gateway SET enabled = false WHERE id = 3")},
	}
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = callbackSeed
		}
		c.Seed = seeding(c.Seed)
		packagecompat.RunWrite(t, r, c)
	}
}

// flakyOrders fails the first completion as an unreachable kernel would,
// then reaches the kernel.
type flakyOrders struct {
	kernel kernelorderv1.KernelOrderClient
	failed bool
}

func (f *flakyOrders) CompleteOrderPayment(ctx context.Context, in *kernelorderv1.CompleteOrderPaymentRequest, opts ...grpc.CallOption) (*kernelorderv1.CompleteOrderPaymentResponse, error) {
	if !f.failed {
		f.failed = true
		return nil, status.Error(codes.Unavailable, "kernel unreachable")
	}
	return f.kernel.CompleteOrderPayment(ctx, in, opts...)
}

// A failure between the package's two steps leaves the payment recorded and
// its order pending, never the order paid without the payment; the callback
// fails (the gateway answers 502), the provider delivers it again, and the
// repeat completes the order once.
func TestCallbackConvergesAfterAFailureBetweenItsSteps(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/native.db?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(callbackModels...))
	callbackSeed(t, db)
	orders := &flakyOrders{kernel: kernelOrder(t, db)}
	payment := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }, Orders: orders}
	callback := payment.Handlers()[native.CallbackRouteID]
	query, err := url.ParseQuery(strings.SplitN(epayPath(epayPaid("PAYE1", "30.30"), epayKey), "?", 2)[1])
	require.NoError(t, err)
	request := pluginhostsdk.NativeRequest{
		RouteID: native.CallbackRouteID, Method: "POST",
		Metadata: pluginhostsdk.RequestMetadata{Path: "/api/v2/payment/callback/epay", Query: query, PathParams: map[string]string{"type": "epay"}},
	}

	_, err = callback(context.Background(), request)
	require.Error(t, err, "the callback fails while the kernel is unreachable")
	var record model.PaymentRecord
	require.NoError(t, db.Take(&record, "trade_no = ?", "PAYE1").Error)
	require.Equal(t, model.PaymentStatusPaid, record.Status, "the payment stays recorded")
	var order model.Order
	require.NoError(t, db.Take(&order, 1).Error)
	require.Equal(t, 0, order.Status, "the order is not paid by the first step")

	response, err := callback(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "fail", string(response.Body), "the repeat is answered as an already processed payment")
	require.NoError(t, db.Take(&order, 1).Error)
	require.Equal(t, 3, order.Status, "the repeat completes the order")
	var gateway model.PaymentGateway
	require.NoError(t, db.Take(&gateway, 1).Error)
	require.EqualValues(t, 4, gateway.TotalOrders, "the payment is counted once")

	_, err = callback(context.Background(), request)
	require.NoError(t, err)
	var requests []model.SubscriberRequest
	require.NoError(t, db.Order("request_id").Find(&requests).Error)
	require.Len(t, requests, 2, "payment:PAYE1 and order:1, once")
	require.True(t, bytes.Contains([]byte(requests[1].Result), []byte(`"outcome":"completed"`)), requests[1].Result)
}
