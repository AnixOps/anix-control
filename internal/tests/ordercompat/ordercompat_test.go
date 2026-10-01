// Package ordercompat proves the order package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL: the
// same bytes (times the handlers take from their clock and random trade
// numbers masked) and the same resulting rows.
//
// The native side reads the plan and user views the kernel publishes
// (packagestore.EnsureKernelAPIViews), and completes orders through the
// real KernelSubscriber server (internal/kernelsubscriber) in process over
// gRPC, on the native side's database, so both sides end with the same
// subscriber rows, request ledger and change log.
package ordercompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kernelsubscriber"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/order/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

var (
	admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	// buyer holds plan 2 until 1_900_000_000.
	buyer = pluginhostsdk.Principal{ActorID: 2}
	// newcomer holds no plan.
	newcomer = pluginhostsdk.Principal{ActorID: 3}
	// lapsed held plan 1, which expired.
	lapsed = pluginhostsdk.Principal{ActorID: 4}
)

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// paidToday is when order 2 was paid: today, for the revenue statistics.
// It is taken once, so both sides of a case seed the same time, which the
// order answers show.
var paidToday = time.Now().Unix()

// orderHost is the identity the kernel serves the order host as.
var orderHost = packagebridge.HostIdentity{PackageID: "order", Version: "4.0.0", Generation: 1}

// entitlementsOnly authorizes what the order package's signed release
// declares: the entitlement family, for the order host.
type entitlementsOnly struct{}

func (entitlementsOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == orderHost && capability == service.CapabilitySubscriberEntitlements {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelSubscriber serves the kernel's KernelSubscriber on db in process and
// returns a client for it, as the order host gets one over its bridge.
func kernelSubscriber(t *testing.T, db *gorm.DB) kernelsubscriberv1.KernelSubscriberClient {
	t.Helper()
	server := &kernelsubscriber.Server{DB: db, Authorizer: entitlementsOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server.For(orderHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelsubscriberv1.NewKernelSubscriberClient(conn)
}

func route(t *testing.T, method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{
			&model.Plan{}, &model.User{}, &model.Order{}, &model.Coupon{}, &model.SubscriptionGroup{},
			&model.PlanSubscriptionGroup{}, &model.UserSubscriptionGroup{}, &model.SubscriberRequest{}, &model.SubscriberChange{},
		},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open:       func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Subscriber: kernelSubscriber(t, db),
			}
			return service.Handlers()[routeID]
		},
	}
}

// The legacy handlers are built per request: their services keep the
// database they were built with, which the harness sets up per case.
func admins(method func(*handler.AdminHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewAdminHandler(), c) }
}

func orders(method func(*handler.OrderHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewOrderHandler(), c) }
}

func ptr[T any](value T) *T { return &value }

// seed writes plans, subscription groups, subscribers, coupons and orders,
// then the kernel views.
func seed(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{
		{ID: 5, Name: "group 5", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 7, Name: "group 7", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 9, Name: "group 9", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.Plan{
		{ID: 1, GroupID: 1, TransferEnable: 10, Name: "Basic", Show: 1, Renew: 1, MonthPrice: ptr(int64(1000)), YearPrice: ptr(int64(10000)), CreatedAt: seeded, UpdatedAt: seeded},
		{
			ID: 2, GroupID: 2, TransferEnable: 100, SpeedLimit: ptr(int64(200)), DeviceLimit: ptr(5), Name: "Pro", Show: 1, Renew: 1,
			MonthPrice: ptr(int64(3000)), QuarterPrice: ptr(int64(8000)), HalfYearPrice: ptr(int64(15000)), YearPrice: ptr(int64(28000)),
			TwoYearPrice: ptr(int64(50000)), ThreeYearPrice: ptr(int64(70000)), OnetimePrice: ptr(int64(99000)), CreatedAt: seeded, UpdatedAt: seeded,
		},
		{ID: 3, GroupID: 3, TransferEnable: 1, Name: "Hidden", Renew: 1, OnetimePrice: ptr(int64(500)), CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	// Plan 2 grants groups 9 and 7, in that order.
	require.NoError(t, db.Create(&[]model.PlanSubscriptionGroup{
		{ID: 1, PlanID: 2, GroupID: 9, CreatedAt: seeded}, {ID: 2, PlanID: 2, GroupID: 7, CreatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1},
		{
			ID: 2, Email: "buyer@example.test", Token: "t2", UUID: "u2", PlanID: ptr(uint(2)), GroupID: ptr(uint(2)), TransferEnable: 100 << 30,
			U: 100, D: 200, ExpiredAt: ptr(int64(1_900_000_000)), SpeedLimit: ptr(int64(200)), DeviceLimit: ptr(5),
		},
		{ID: 3, Email: "newcomer@example.test", Token: "t3", UUID: "u3", U: 5, D: 6},
		{ID: 4, Email: "lapsed@example.test", Token: "t4", UUID: "u4", PlanID: ptr(uint(1)), GroupID: ptr(uint(1)), ExpiredAt: ptr(int64(1_600_000_000))},
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{ID: 1, UserID: 2, GroupID: 5, CreatedAt: seeded}).Error)
	coupon := func(id uint, code string, kind, value int, limit *int, used int, start, end int64) model.Coupon {
		return model.Coupon{
			ID: id, Code: code, Name: "coupon " + code, Type: kind, Value: value, LimitUse: limit, UseCount: used,
			StartedAt: start, EndedAt: end, CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
		}
	}
	require.NoError(t, db.Create(&[]model.Coupon{
		coupon(1, "PCT20", 1, 20, ptr(10), 3, 1_600_000_000, 2_000_000_000),
		coupon(2, "FIX500", 2, 500, nil, 0, 1_600_000_000, 2_000_000_000),
		coupon(3, "OVER", 2, 999_999, nil, 0, 1_600_000_000, 2_000_000_000),
		coupon(4, "EXPIRED", 1, 50, nil, 0, 1_600_000_000, 1_700_000_000),
		coupon(5, "FUTURE", 1, 50, nil, 0, 2_000_000_000, 2_100_000_000),
		coupon(6, "USEDUP", 1, 50, ptr(1), 1, 1_600_000_000, 2_000_000_000),
		coupon(7, "MINUS", 1, 50, ptr(-1), 4, 1_600_000_000, 2_000_000_000),
		coupon(8, "ZERO", 1, 10, ptr(0), 9, 1_600_000_000, 2_000_000_000),
	}).Error)
	order := func(id, user, plan uint, period string, status int, total int64, paidAt *int64) model.Order {
		return model.Order{
			ID: id, UserID: user, PlanID: plan, Type: 1, Period: period, TradeNo: fmt.Sprintf("T%d", id), TotalAmount: total,
			DiscountAmount: ptr(int64(0)), Status: status, PaidAt: paidAt, CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
		}
	}
	require.NoError(t, db.Create(&[]model.Order{
		order(1, 2, 2, "month", 0, 3000, nil),
		order(2, 3, 1, "month", 1, 1000, &paidToday),
		order(3, 3, 2, "year", 3, 28000, ptr(int64(1_700_000_000))),
		order(4, 4, 1, "month", 2, 1000, nil),
		// Marked paid: a renewal of the plan the buyer holds, a first plan,
		// a hidden plan with an unknown period.
		order(5, 2, 2, "quarter", 0, 8000, nil),
		order(6, 3, 2, "month", 0, 3000, nil),
		order(7, 4, 3, "weekly", 0, 500, nil),
		order(8, 3, 1, "onetime", 3, 1000, ptr(int64(1_700_000_000))),
	}).Error)
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
	syncSequences(t, db)
}

// seedOrphans adds orders whose plan or buyer no longer exists. PostgreSQL
// enforces the kernel's foreign keys, so they are dropped first.
func seedOrphans(t testing.TB, db *gorm.DB) {
	seed(t, db)
	if db.Name() == "postgres" {
		for _, constraint := range []string{"fk_v2_order_user", "fk_v2_order_plan"} {
			require.NoError(t, db.Exec("ALTER TABLE v2_order DROP CONSTRAINT IF EXISTS "+constraint).Error)
		}
	}
	require.NoError(t, db.Create(&[]model.Order{
		{ID: 20, UserID: 99, PlanID: 1, Period: "month", TradeNo: "T20", TotalAmount: 1000, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 21, UserID: 3, PlanID: 404, Period: "month", TradeNo: "T21", TotalAmount: 1000, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 22, UserID: 0, PlanID: 1, Period: "month", TradeNo: "T22", TotalAmount: 1000, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	syncSequences(t, db)
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_plan", "v2_user", "v2_order", "v2_coupon", "v2_user_subscription_group", "v2_subscription_group", "v2_plan_subscription_group"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockTime is a time the handlers take from their clock: masked unless it
// was seeded.
func clockTime(value time.Time) string {
	if !value.IsZero() && value.Sub(seeded) >= 0 && value.Sub(seeded) <= time.Hour {
		return value.UTC().Format(time.RFC3339)
	}
	if d := time.Since(value); d > -10*time.Minute && d < 10*time.Minute {
		return "<now>"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// clockUnix masks a Unix time the handlers derive from their clock: now,
// or now plus a period.
func clockUnix(value int64) any {
	now := time.Now()
	for _, ref := range []struct {
		label string
		at    time.Time
	}{
		{"<now>", now}, {"<now+30d>", now.Add(30 * 24 * time.Hour)}, {"<now+1mo>", now.AddDate(0, 1, 0)},
		{"<now+3mo>", now.AddDate(0, 3, 0)}, {"<now+12mo>", now.AddDate(0, 12, 0)}, {"<now+1200mo>", now.AddDate(0, 1200, 0)},
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

// coupons is v2_coupon after a write.
func coupons(t testing.TB, db *gorm.DB) any {
	var rows []model.Coupon
	require.NoError(t, db.Order("id").Find(&rows).Error)
	state := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		state = append(state, map[string]any{
			"id": row.ID, "code": row.Code, "name": row.Name, "type": row.Type, "value": row.Value, "limit_use": row.LimitUse,
			"limit_use_with": row.LimitUseWith, "limit_period": row.LimitPeriod, "use_count": row.UseCount,
			"started_at": clockUnix(row.StartedAt), "ended_at": clockUnix(row.EndedAt),
			"created_at": clockTime(row.CreatedAt), "updated_at": clockTime(row.UpdatedAt),
		})
	}
	return state
}

// ordersAndCoupons is v2_order and v2_coupon after a write; the trade
// numbers of new orders are random and only their format is compared.
func ordersAndCoupons(t testing.TB, db *gorm.DB) any {
	var rows []model.Order
	require.NoError(t, db.Order("id").Find(&rows).Error)
	state := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		tradeNo := row.TradeNo
		if len(tradeNo) == 22 {
			_, err := time.ParseInLocation("20060102150405", tradeNo[:14], time.Local)
			require.NoError(t, err, "trade number %s", tradeNo)
			require.Regexp(t, `^[A-Z0-9]{8}$`, tradeNo[14:])
			tradeNo = "<trade number>"
		}
		state = append(state, map[string]any{
			"id": row.ID, "invite_user_id": row.InviteUserID, "user_id": row.UserID, "plan_id": row.PlanID, "coupon_id": row.CouponID,
			"payment_id": row.PaymentID, "type": row.Type, "period": row.Period, "trade_no": tradeNo, "callback_no": row.CallbackNo,
			"total_amount": row.TotalAmount, "discount_amount": row.DiscountAmount, "surplus_amount": row.SurplusAmount,
			"refund_amount": row.RefundAmount, "balance": row.Balance, "surplus_order": row.SurplusOrder, "status": row.Status,
			"commission_status": row.CommissionStatus, "commission_balance": row.CommissionBalance, "paid_at": clockUnixPtr(row.PaidAt),
			"created_at": clockTime(row.CreatedAt), "updated_at": clockTime(row.UpdatedAt),
		})
	}
	return map[string]any{"orders": state, "coupons": coupons(t, db)}
}

// completion is everything marking an order paid can change: the orders,
// the subscribers' entitlements and subscription groups, the subscriber
// request ledger (v4_kernel_subscriber_request) and change log
// (v4_kernel_subscriber_change), and the plans the views read.
func completion(t testing.TB, db *gorm.DB) any {
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
		Banned         int
		Balance        int64
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	userState := make([]map[string]any, 0, len(users))
	for _, user := range users {
		userState = append(userState, map[string]any{
			"id": user.ID, "plan_id": user.PlanID, "group_id": user.GroupID, "transfer_enable": user.TransferEnable, "u": user.U, "d": user.D,
			"speed_limit": user.SpeedLimit, "device_limit": user.DeviceLimit, "expired_at": clockUnixPtr(user.ExpiredAt),
			"banned": user.Banned, "balance": user.Balance,
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
		var result struct {
			ExpiresAt *int64 `json:"expires_at"`
		}
		require.NoError(t, json.Unmarshal([]byte(request.Result), &result))
		ledger = append(ledger, map[string]any{
			"request_id": request.RequestID, "method": request.Method, "user_id": request.UserID,
			"expires_at": clockUnixPtr(result.ExpiresAt), "created_at": clockTime(request.CreatedAt),
		})
	}
	var changes []struct {
		ID        uint64
		UserID    uint
		Deleted   bool
		CreatedAt time.Time
	}
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
	changeState := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		changeState = append(changeState, map[string]any{"id": change.ID, "user_id": change.UserID, "deleted": change.Deleted, "created_at": clockTime(change.CreatedAt)})
	}
	return map[string]any{
		"orders": ordersAndCoupons(t, db), "users": userState, "groups": groups, "ledger": ledger, "changes": changeState,
	}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, snapshot func(testing.TB, *gorm.DB) any, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		if c.Snapshot == nil {
			c.Snapshot = snapshot
		}
		packagecompat.RunWrite(t, r, c)
	}
}

func empty(t testing.TB, db *gorm.DB) { require.NoError(t, packagestore.EnsureKernelAPIViews(db)) }

func TestCouponRoutesParity(t *testing.T) {
	read(t, route(t, "GET", "/api/v2/admin/coupon", "order.admin.coupon.get", handler.NewAdminCouponHandler().GetCoupons), []packagecompat.Case{
		{Name: "every coupon newest first", Path: "/api/v2/admin/coupon", Principal: admin},
		{Name: "no coupons", Path: "/api/v2/admin/coupon", Principal: admin, Seed: empty},
	})
	created := []string{"data.data.created_at", "data.data.updated_at"}
	write(t, route(t, "POST", "/api/v2/admin/coupon", "order.admin.coupon.post", handler.NewAdminCouponHandler().CreateCoupon), coupons, []packagecompat.Case{
		{Name: "create", Path: "/api/v2/admin/coupon", Principal: admin, Mask: created,
			Body: []byte(`{"code":"NEW10","name":"New","type":1,"value":10,"limit_use":5,"started_at":1700000000,"ended_at":1900000000}`)},
		{Name: "create with the default window", Path: "/api/v2/admin/coupon", Principal: admin,
			Mask: append([]string{"data.data.started_at", "data.data.ended_at"}, created...), Body: []byte(`{"code":"NOW","name":"Now","type":2,"value":300}`)},
		{Name: "a taken code", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"PCT20","name":"Again","type":1,"value":5}`)},
		{Name: "a value of zero", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"Z","name":"Zero","type":1,"value":0}`)},
		{Name: "an unknown type", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"T","name":"Type","type":3,"value":5}`)},
		{Name: "a negative limit", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"L","name":"Limit","type":1,"value":5,"limit_use":-1}`)},
		{Name: "a code too long", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"` + string(make65()) + `","name":"Long","type":1,"value":5}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/coupon", Principal: admin, Body: []byte(`{"code":"W","name":"Wrong","type":"one","value":5}`)},
		{Name: "no body", Path: "/api/v2/admin/coupon", Principal: admin},
	})
	write(t, route(t, "DELETE", "/api/v2/admin/coupon/:id", "order.admin.coupon.id.delete", handler.NewAdminCouponHandler().DeleteCoupon), coupons, []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/coupon/4", Principal: admin},
		{Name: "unknown coupon", Path: "/api/v2/admin/coupon/99", Principal: admin},
		{Name: "coupon zero", Path: "/api/v2/admin/coupon/0", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/coupon/x", Principal: admin},
		{Name: "id that is a condition", Path: "/api/v2/admin/coupon/0%20OR%201=1", Principal: admin},
	})
	read(t, route(t, "POST", "/api/v2/user/coupon/check", "order.user.coupon.check.post", handler.NewCouponHandler().CheckCoupon), []packagecompat.Case{
		{Name: "a percentage coupon", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"PCT20","plan_id":2}`)},
		{Name: "a fixed coupon", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"FIX500","plan_id":1}`)},
		{Name: "an expired coupon", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"EXPIRED","plan_id":1}`)},
		{Name: "a coupon not started", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"FUTURE","plan_id":1}`)},
		{Name: "a used up coupon", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"USEDUP","plan_id":1}`)},
		{Name: "a negative limit is no limit here", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"MINUS","plan_id":1}`)},
		{Name: "an unknown code", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"NOPE","plan_id":1}`)},
		{Name: "no plan", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"code":"PCT20"}`)},
		{Name: "no code", Path: "/api/v2/user/coupon/check", Principal: buyer, Body: []byte(`{"plan_id":1}`)},
		{Name: "no body", Path: "/api/v2/user/coupon/check", Principal: buyer},
	})
}

func make65() []byte {
	code := make([]byte, 65)
	for i := range code {
		code[i] = 'A'
	}
	return code
}

func TestOrderStatsParity(t *testing.T) {
	read(t, route(t, "GET", "/api/v2/admin/orders/stats", "order.admin.orders.stats.get", admins((*handler.AdminHandler).GetOrderStats)), []packagecompat.Case{
		{Name: "counts and revenue", Path: "/api/v2/admin/orders/stats", Principal: admin},
		{Name: "no orders", Path: "/api/v2/admin/orders/stats", Principal: admin, Seed: empty},
	})
}

func TestOrderStatusRoutesParity(t *testing.T) {
	write(t, route(t, "PUT", "/api/v2/admin/orders/:id/status", "order.admin.orders.id.status.put", admins((*handler.AdminHandler).UpdateOrderStatus)), ordersAndCoupons, []packagecompat.Case{
		{Name: "paid sets the payment time", Path: "/api/v2/admin/orders/1/status", Principal: admin, Body: []byte(`{"status":1}`)},
		{Name: "completed grants nothing", Path: "/api/v2/admin/orders/1/status", Principal: admin, Body: []byte(`{"status":3}`)},
		{Name: "cancelled", Path: "/api/v2/admin/orders/2/status", Principal: admin, Body: []byte(`{"status":2}`)},
		{Name: "pending fails the required binding", Path: "/api/v2/admin/orders/2/status", Principal: admin, Body: []byte(`{"status":0}`)},
		{Name: "an unknown status", Path: "/api/v2/admin/orders/1/status", Principal: admin, Body: []byte(`{"status":4}`)},
		{Name: "unknown order", Path: "/api/v2/admin/orders/99/status", Principal: admin, Body: []byte(`{"status":2}`)},
		{Name: "order zero", Path: "/api/v2/admin/orders/0/status", Principal: admin, Body: []byte(`{"status":2}`)},
		{Name: "id that is not a number", Path: "/api/v2/admin/orders/x/status", Principal: admin, Body: []byte(`{"status":2}`)},
		{Name: "id that is a condition", Path: "/api/v2/admin/orders/0%20OR%201=1/status", Principal: admin, Body: []byte(`{"status":2}`)},
		{Name: "no body", Path: "/api/v2/admin/orders/1/status", Principal: admin},
	})
	write(t, route(t, "POST", "/api/v2/admin/orders/:id/cancel", "order.admin.orders.id.cancel.post", admins((*handler.AdminHandler).CancelOrder)), ordersAndCoupons, []packagecompat.Case{
		{Name: "cancel a pending order", Path: "/api/v2/admin/orders/1/cancel", Principal: admin},
		{Name: "cancel a completed order", Path: "/api/v2/admin/orders/3/cancel", Principal: admin},
		{Name: "unknown order", Path: "/api/v2/admin/orders/99/cancel", Principal: admin},
		{Name: "order zero", Path: "/api/v2/admin/orders/0/cancel", Principal: admin},
		{Name: "id beyond 32 bits", Path: "/api/v2/admin/orders/4294967296/cancel", Principal: admin},
	})
}

func TestMarkOrderPaidParity(t *testing.T) {
	paid := route(t, "POST", "/api/v2/admin/orders/:id/paid", native.MarkPaidRouteID, admins((*handler.AdminHandler).MarkOrderPaid))
	write(t, paid, completion, []packagecompat.Case{
		{Name: "a renewal extends the current expiry", Path: "/api/v2/admin/orders/5/paid", Principal: admin},
		{Name: "a first plan starts now with its subscription groups", Path: "/api/v2/admin/orders/6/paid", Principal: admin},
		{Name: "an unknown period starts and ends now", Path: "/api/v2/admin/orders/7/paid", Principal: admin},
		{Name: "a completed order is granted again once", Path: "/api/v2/admin/orders/8/paid", Principal: admin},
		{Name: "marking twice grants once", Path: "/api/v2/admin/orders/6/paid", Principal: admin, Warmup: [][]byte{nil}},
		{Name: "a cancelled order", Path: "/api/v2/admin/orders/4/paid", Principal: admin},
		{Name: "unknown order", Path: "/api/v2/admin/orders/99/paid", Principal: admin},
		{Name: "order zero", Path: "/api/v2/admin/orders/0/paid", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/orders/x/paid", Principal: admin},
		{Name: "id that is a condition", Path: "/api/v2/admin/orders/0%20OR%201=1/paid", Principal: admin},
		{Name: "a buyer who no longer exists", Path: "/api/v2/admin/orders/20/paid", Principal: admin, Seed: seedOrphans},
		{Name: "a plan that no longer exists", Path: "/api/v2/admin/orders/21/paid", Principal: admin, Seed: seedOrphans},
		{Name: "an order without a buyer", Path: "/api/v2/admin/orders/22/paid", Principal: admin, Seed: seedOrphans},
	})
}

func TestSaveOrderParity(t *testing.T) {
	save := route(t, "POST", "/api/v2/user/order/save", "order.user.order.save.post", orders((*handler.OrderHandler).SaveOrder))
	created := []string{"data.trade_no", "data.created_at", "data.updated_at"}
	write(t, save, ordersAndCoupons, []packagecompat.Case{
		{Name: "a new purchase", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":2,"period":"half_year"}`)},
		{Name: "a renewal", Path: "/api/v2/user/order/save", Principal: buyer, Mask: created, Body: []byte(`{"plan_id":2,"period":"two_year"}`)},
		{Name: "an upgrade", Path: "/api/v2/user/order/save", Principal: lapsed, Mask: created, Body: []byte(`{"plan_id":2,"period":"three_year"}`)},
		{Name: "a hidden plan", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":3,"period":"onetime"}`)},
		{Name: "a percentage coupon", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":2,"period":"year","coupon_id":1}`)},
		{Name: "a fixed coupon", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":2}`)},
		{Name: "a coupon above the price", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":3}`)},
		{Name: "an expired coupon", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":4}`)},
		{Name: "a used up coupon", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":6}`)},
		{Name: "a negative limit applies no coupon", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":7}`)},
		{Name: "a zero limit is unlimited", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":8}`)},
		{Name: "an unknown coupon is kept without a discount", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":99}`)},
		{Name: "coupon zero", Path: "/api/v2/user/order/save", Principal: newcomer, Mask: created, Body: []byte(`{"plan_id":1,"period":"month","coupon_id":0}`)},
		{Name: "a period the plan does not sell", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":1,"period":"quarter"}`)},
		{Name: "each unsold period", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":3,"period":"month"}`)},
		{Name: "an unknown period", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":1,"period":"weekly"}`)},
		{Name: "an unknown plan", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":99,"period":"month"}`)},
		{Name: "an unknown buyer", Path: "/api/v2/user/order/save", Principal: pluginhostsdk.Principal{ActorID: 99}, Body: []byte(`{"plan_id":1,"period":"month"}`)},
		{Name: "no plan", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"period":"month"}`)},
		{Name: "no period", Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":1}`)},
		{Name: "no body", Path: "/api/v2/user/order/save", Principal: newcomer},
	})
	for _, period := range []string{"month", "quarter", "half_year", "year", "two_year", "three_year"} {
		write(t, save, ordersAndCoupons, []packagecompat.Case{
			{Name: "the hidden plan does not sell " + period, Path: "/api/v2/user/order/save", Principal: newcomer, Body: []byte(`{"plan_id":3,"period":"` + period + `"}`)},
		})
	}
}

// The order list and detail answers carry the order, its plan's id and name
// and, for an administrator, its buyer's id and e-mail: the native side
// reads them from kapi_plan_name_v1 and kapi_user_directory_v1. A user sees
// only their own orders.
func TestOrderListParity(t *testing.T) {
	list := route(t, "GET", "/api/v2/admin/orders", native.AdminOrdersRouteID, admins((*handler.AdminHandler).GetOrderList))
	read(t, list, []packagecompat.Case{
		{Name: "every order newest first", Path: "/api/v2/admin/orders", Principal: admin},
		{Name: "a page", Path: "/api/v2/admin/orders?page=2&page_size=3", Principal: admin},
		{Name: "a page beyond the last", Path: "/api/v2/admin/orders?page=9", Principal: admin},
		{Name: "page zero", Path: "/api/v2/admin/orders?page=0&page_size=2", Principal: admin},
		{Name: "a page size beyond the maximum", Path: "/api/v2/admin/orders?page_size=1000", Principal: admin},
		{Name: "a page size that is not a number", Path: "/api/v2/admin/orders?page_size=x", Principal: admin},
		{Name: "empty page parameters", Path: "/api/v2/admin/orders?page=&page_size=", Principal: admin},
		{Name: "by buyer", Path: "/api/v2/admin/orders?user_id=3", Principal: admin},
		{Name: "by a buyer id that does not parse", Path: "/api/v2/admin/orders?user_id=x", Principal: admin, Seed: seedOrphans},
		{Name: "by status", Path: "/api/v2/admin/orders?status=0", Principal: admin},
		{Name: "by a status that does not parse", Path: "/api/v2/admin/orders?status=paid", Principal: admin},
		{Name: "by type", Path: "/api/v2/admin/orders?type=1", Principal: admin},
		{Name: "by trade number", Path: "/api/v2/admin/orders?trade_no=T3", Principal: admin},
		{Name: "by the start of the e-mail", Path: "/api/v2/admin/orders?email=newcomer", Principal: admin},
		{Name: "by the middle of the e-mail", Path: "/api/v2/admin/orders?email=comer@", Principal: admin},
		{Name: "by an e-mail no buyer has", Path: "/api/v2/admin/orders?email=nobody", Principal: admin},
		{Name: "an e-mail with a wildcard", Path: "/api/v2/admin/orders?email=%25example", Principal: admin},
		{Name: "every filter", Path: "/api/v2/admin/orders?user_id=3&status=0&type=1&trade_no=T6&email=newcomer", Principal: admin},
		{Name: "orders whose plan or buyer no longer exists", Path: "/api/v2/admin/orders", Principal: admin, Seed: seedOrphans},
		{Name: "no orders", Path: "/api/v2/admin/orders", Principal: admin, Seed: empty},
	})
	mine := route(t, "GET", "/api/v2/user/order", native.UserOrdersRouteID, orders((*handler.OrderHandler).GetOrders))
	read(t, mine, []packagecompat.Case{
		{Name: "the caller's orders", Path: "/api/v2/user/order", Principal: buyer},
		{Name: "another caller's orders", Path: "/api/v2/user/order", Principal: newcomer},
		{Name: "a caller without orders", Path: "/api/v2/user/order", Principal: admin},
		{Name: "query parameters cannot widen the list", Path: "/api/v2/user/order?user_id=2&email=buyer&status=0", Principal: newcomer},
		{Name: "a page", Path: "/api/v2/user/order?page=2&page_size=2", Principal: newcomer},
		{Name: "a page size beyond the maximum", Path: "/api/v2/user/order?page_size=1000", Principal: newcomer},
		{Name: "a negative page size", Path: "/api/v2/user/order?page_size=-1", Principal: newcomer},
		{Name: "page size zero", Path: "/api/v2/user/order?page_size=0", Principal: newcomer},
		{Name: "a negative page", Path: "/api/v2/user/order?page=-2&page_size=1", Principal: newcomer},
		{Name: "no caller", Path: "/api/v2/user/order", Principal: pluginhostsdk.Principal{}, Seed: seedOrphans},
		{Name: "an order whose plan no longer exists", Path: "/api/v2/user/order", Principal: newcomer, Seed: seedOrphans},
		{Name: "a caller whose account is gone", Path: "/api/v2/user/order", Principal: pluginhostsdk.Principal{ActorID: 99}, Seed: seedOrphans},
		{Name: "no orders", Path: "/api/v2/user/order", Principal: buyer, Seed: empty},
	})
}

func TestOrderDetailParity(t *testing.T) {
	detail := route(t, "GET", "/api/v2/admin/orders/:id", native.AdminOrderRouteID, admins((*handler.AdminHandler).GetOrder))
	read(t, detail, []packagecompat.Case{
		{Name: "an order with its plan and buyer", Path: "/api/v2/admin/orders/1", Principal: admin},
		{Name: "a completed order", Path: "/api/v2/admin/orders/3", Principal: admin},
		{Name: "unknown order", Path: "/api/v2/admin/orders/99", Principal: admin},
		{Name: "order zero", Path: "/api/v2/admin/orders/0", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/orders/x", Principal: admin},
		{Name: "id that is a condition", Path: "/api/v2/admin/orders/0%20OR%201=1", Principal: admin},
		{Name: "id beyond 32 bits", Path: "/api/v2/admin/orders/4294967296", Principal: admin},
		{Name: "a buyer who no longer exists", Path: "/api/v2/admin/orders/20", Principal: admin, Seed: seedOrphans},
		{Name: "a plan that no longer exists", Path: "/api/v2/admin/orders/21", Principal: admin, Seed: seedOrphans},
		{Name: "an order without a buyer", Path: "/api/v2/admin/orders/22", Principal: admin, Seed: seedOrphans},
	})
	mine := route(t, "GET", "/api/v2/user/order/:id", native.UserOrderRouteID, orders((*handler.OrderHandler).GetOrderDetail))
	read(t, mine, []packagecompat.Case{
		{Name: "the caller's order", Path: "/api/v2/user/order/1", Principal: buyer},
		{Name: "the caller's completed order", Path: "/api/v2/user/order/3", Principal: newcomer},
		{Name: "another user's order", Path: "/api/v2/user/order/2", Principal: buyer},
		{Name: "an administrator's own route shows only their orders", Path: "/api/v2/user/order/1", Principal: admin},
		{Name: "unknown order", Path: "/api/v2/user/order/99", Principal: buyer},
		{Name: "order zero", Path: "/api/v2/user/order/0", Principal: buyer},
		{Name: "id that is not a number", Path: "/api/v2/user/order/x", Principal: buyer},
		{Name: "id that is a condition", Path: "/api/v2/user/order/1%20OR%201=1", Principal: buyer},
		{Name: "id beyond 32 bits", Path: "/api/v2/user/order/4294967297", Principal: buyer},
		{Name: "an order without a buyer and no caller", Path: "/api/v2/user/order/22", Principal: pluginhostsdk.Principal{}, Seed: seedOrphans},
		{Name: "the caller's order whose plan no longer exists", Path: "/api/v2/user/order/21", Principal: newcomer, Seed: seedOrphans},
		{Name: "a caller whose account is gone", Path: "/api/v2/user/order/20", Principal: pluginhostsdk.Principal{ActorID: 99}, Seed: seedOrphans},
	})
}
