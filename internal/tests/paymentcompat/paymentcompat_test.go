// Package paymentcompat proves the payment package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL: the
// same bytes (times the handlers take from their clock and random trade
// numbers and addresses masked) and the same resulting rows.
//
// The native side reads orders through the kernel view kapi_order_billing_v1
// (packagestore.EnsureKernelAPIViews). No test calls a payment provider: the
// fiat routes are stubs on both sides, and the callbacks (callbacks_test.go)
// verify PayPal deliveries against a fake PayPal API.
package paymentcompat

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var (
	admin  = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	buyer  = pluginhostsdk.Principal{ActorID: 2}
	other  = pluginhostsdk.Principal{ActorID: 3}
	public = pluginhostsdk.Principal{}
)

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.Plan{}, &model.User{}, &model.Order{}, &model.PaymentGateway{}, &model.PaymentRecord{}, &model.Payment{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// The legacy handlers are built per request: their services keep the
// database they were built with, which the harness sets up per case.
func gateways(method func(*handler.PaymentGatewayHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewPaymentGatewayHandler(), c) }
}

func payments(method func(*handler.PaymentHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewPaymentHandler(), c) }
}

func ptr[T any](value T) *T { return &value }

func at(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC) }

// seed writes subscribers, orders, gateways, payment records and payment
// methods, then the kernel views.
func seed(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&model.Plan{ID: 1, Name: "Basic", CreatedAt: seeded, UpdatedAt: seeded}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1},
		{ID: 2, Email: "buyer@example.test", Token: "t2", UUID: "u2"},
		{ID: 3, Email: "other@example.test", Token: "t3", UUID: "u3"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Order{
		{ID: 1, UserID: 2, PlanID: 1, Period: "month", TradeNo: "T1", TotalAmount: 10000, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, UserID: 2, PlanID: 1, Period: "month", TradeNo: "T2", TotalAmount: 5000, Status: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, UserID: 3, PlanID: 1, Period: "month", TradeNo: "T3", TotalAmount: 7000, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 4, UserID: 2, PlanID: 1, Period: "month", TradeNo: "T4", TotalAmount: 50, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 5, UserID: 2, PlanID: 1, Period: "month", TradeNo: "T5", TotalAmount: 1234, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.PaymentGateway{
		{
			ID: 1, Name: "EPay", Type: "epay", Enabled: true, Config: `{"api_url":"https://pay.example.test","pid":"1001","key":"merchant-key"}`,
			FeeRate: 0.01, FeeFixed: 0.5, MinAmount: 1, MaxAmount: 10000, Sort: 2, Description: "EPay", TotalOrders: 3, TotalAmount: 152.5,
			CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 2, Name: "Stripe", Type: "stripe", Config: `{"publishable_key":"pk_test","secret_key":"sk_test","webhook_secret":"whsec"}`,
			MinAmount: 1, MaxAmount: 10000, Sort: 1, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 3, Name: "Alipay", Type: "alipay", Enabled: true, Config: `{"app_id":"a1","private_key":"alipay-private","public_key":"alipay-public"}`,
			MinAmount: 1, MaxAmount: 10000, Sort: 3, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{
			ID: 4, Name: "PayPal", Type: "paypal", Enabled: true, Icon: "paypal.svg",
			Config:  `{"client_id":"cid","client_secret":"paypal-secret","nested":{"api_key":"deep"},"sandbox_mode":true}`,
			FeeRate: 0.02, MinAmount: 5, MaxAmount: 500, CreatedAt: seeded, UpdatedAt: seeded,
		},
		{ID: 5, Name: "Broken", Type: "epay", Config: `key=raw-secret`, MinAmount: 1, MaxAmount: 10000, Sort: 4, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.PaymentRecord{
		{
			ID: 1, TradeNo: "PAY20260901080000aaaa0001", GatewayID: 1, GatewayType: "epay", GatewayTradeNo: "EP1", UserID: 2,
			Amount: 100, FeeAmount: 1.5, ActualAmount: 101.5, Currency: "CNY", Status: 1, PaidAt: ptr(at(1, 9)),
			NotifyData: "money=101.50", ClientIP: "198.51.100.7", OrderID: ptr(uint(1)), CreatedAt: at(1, 8), UpdatedAt: at(1, 9),
		},
		{
			ID: 2, TradeNo: "X40220260902080000bbbb0002", GatewayType: "crypto", Provider: "x402", UserID: 2, Amount: 100, ActualAmount: 0.0001,
			Currency: "ETH", Status: 0, TxHash: ptr("0xhash"), WalletAddress: ptr("0xwallet"), Network: ptr("sepolia"), OrderID: ptr(uint(1)),
			CreatedAt: at(2, 8), UpdatedAt: at(2, 8),
		},
		{
			ID: 3, TradeNo: "PAY20260903080000cccc0003", GatewayID: 1, GatewayType: "epay", UserID: 3, Amount: 70, ActualAmount: 71.2,
			Currency: "CNY", Status: 2, CancelledAt: ptr(at(3, 9)), CreatedAt: at(3, 8), UpdatedAt: at(3, 9),
		},
		{
			ID: 4, TradeNo: "PAY20260904080000dddd0004", GatewayID: 4, GatewayType: "paypal", UserID: 3, Amount: 20, ActualAmount: 20.4,
			Currency: "CNY", Status: 3, PaidAt: ptr(at(4, 8)), RefundedAt: ptr(at(5, 8)), CreatedAt: at(4, 8), UpdatedAt: at(5, 8),
		},
		{
			ID: 5, TradeNo: "FIAT20260805080000eeee0005", GatewayType: "fiat", Provider: "stripe", UserID: 2, Amount: 50, ActualAmount: 50,
			Currency: "USD", Status: 4, CancelledAt: ptr(at(6, 8)), CreatedAt: time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC), UpdatedAt: at(6, 8),
		},
		{
			ID: 6, TradeNo: "PAY20260815080000ffff0006", GatewayID: 1, GatewayType: "epay", UserID: 2, Amount: 50, FeeAmount: 1, ActualAmount: 51,
			Currency: "CNY", Status: 1, PaidAt: ptr(time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)),
			CreatedAt: time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.Payment{
		{ID: 1, Name: "Stripe", Method: "fiat", Provider: "stripe", Sort: 2, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "PayPal", Method: "fiat", Provider: "paypal", Sort: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "Alipay", Method: "fiat", Provider: "alipay", Sort: 1, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	// enable defaults to 1 on insert; PayPal's method is off.
	require.NoError(t, db.Model(&model.Payment{}).Where("id = ?", 2).UpdateColumn("enable", 0).Error)
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
	syncSequences(t, db)
}

// noMethods is seed without enabled payment methods.
func noMethods(t testing.TB, db *gorm.DB) {
	seed(t, db)
	require.NoError(t, db.Model(&model.Payment{}).Where("1 = 1").UpdateColumn("enable", 0).Error)
}

func empty(t testing.TB, db *gorm.DB) { require.NoError(t, packagestore.EnsureKernelAPIViews(db)) }

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_plan", "v2_user", "v2_order", "v2_payment_gateway", "v2_payment_record", "v2_payment"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockTime is a time the handlers take from their clock: masked unless it
// was seeded.
func clockTime(value time.Time) string {
	if d := time.Since(value); d > -10*time.Minute && d < 10*time.Minute {
		return "<now>"
	}
	return value.UTC().Format(time.RFC3339)
}

func clockTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return clockTime(*value)
}

// gatewayState is v2_payment_gateway after a write, configurations as
// stored: a secret sent back as the placeholder keeps its stored value.
func gatewayState(t testing.TB, db *gorm.DB) any {
	var rows []model.PaymentGateway
	require.NoError(t, db.Order("id").Find(&rows).Error)
	state := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		state = append(state, map[string]any{
			"id": row.ID, "name": row.Name, "type": row.Type, "enabled": row.Enabled, "icon": row.Icon, "config": row.Config,
			"fee_rate": row.FeeRate, "fee_fixed": row.FeeFixed, "min_amount": row.MinAmount, "max_amount": row.MaxAmount,
			"sort": row.Sort, "description": row.Description, "total_orders": row.TotalOrders, "total_amount": row.TotalAmount,
			"created_at": clockTime(row.CreatedAt), "updated_at": clockTime(row.UpdatedAt),
		})
	}
	return state
}

var (
	generatedTradeNo = regexp.MustCompile(`^(PAY|X402|FIAT)\d{14}[0-9a-f]{8}$`)
	generatedWallet  = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
)

// recordState is v2_payment_record after a write, with the orders, which no
// payment route changes; generated trade numbers and addresses are compared
// by format.
func recordState(t testing.TB, db *gorm.DB) any {
	var rows []model.PaymentRecord
	require.NoError(t, db.Order("id").Find(&rows).Error)
	state := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		tradeNo := row.TradeNo
		if row.ID > 6 {
			require.Regexp(t, generatedTradeNo, tradeNo)
			tradeNo = tradeNo[:len(tradeNo)-22] + "<generated>"
		}
		var wallet any = row.WalletAddress
		if row.WalletAddress != nil && generatedWallet.MatchString(*row.WalletAddress) {
			wallet = "<generated>"
		}
		state = append(state, map[string]any{
			"id": row.ID, "trade_no": tradeNo, "gateway_id": row.GatewayID, "gateway_type": row.GatewayType,
			"gateway_trade_no": row.GatewayTradeNo, "provider": row.Provider, "user_id": row.UserID, "amount": row.Amount,
			"fee_amount": row.FeeAmount, "actual_amount": row.ActualAmount, "currency": row.Currency, "status": row.Status,
			"paid_at": clockTimePtr(row.PaidAt), "cancelled_at": clockTimePtr(row.CancelledAt), "refunded_at": clockTimePtr(row.RefundedAt),
			"notify_data": row.NotifyData, "client_ip": row.ClientIP, "tx_hash": row.TxHash, "wallet_address": wallet,
			"network": row.Network, "order_id": row.OrderID, "created_at": clockTime(row.CreatedAt), "updated_at": clockTime(row.UpdatedAt),
		})
	}
	var orders []struct {
		ID     uint
		Status int
		PaidAt *int64
	}
	require.NoError(t, db.Model(&model.Order{}).Order("id").Find(&orders).Error)
	return map[string]any{"records": state, "orders": orders, "gateways": gatewayState(t, db)}
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

func TestGatewayRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/payment/gateways", "payment.admin.payment.gateways.get", gateways((*handler.PaymentGatewayHandler).ListGateways)), []packagecompat.Case{
		{Name: "every gateway by sort, secrets redacted", Path: "/api/v2/admin/payment/gateways", Principal: admin},
		{Name: "no gateways", Path: "/api/v2/admin/payment/gateways", Principal: admin, Seed: empty},
	})
	created := []string{"data.created_at", "data.updated_at"}
	write(t, route("POST", "/api/v2/admin/payment/gateways", "payment.admin.payment.gateways.post", gateways((*handler.PaymentGatewayHandler).CreateGateway)), gatewayState, []packagecompat.Case{
		{Name: "create with a configuration object", Path: "/api/v2/admin/payment/gateways", Principal: admin, Mask: created,
			Body: []byte(`{"name":"EPay 2","type":"epay","icon":"e.svg","config":{"api_url":"https://pay2.example.test","pid":"2002","key":"k2"},` +
				`"fee_rate":0.03,"fee_fixed":1,"min_amount":2,"max_amount":300,"sort":7,"description":"second"}`)},
		{Name: "create with a configuration string", Path: "/api/v2/admin/payment/gateways", Principal: admin, Mask: created,
			Body: []byte(`{"name":"Stripe 2","type":"stripe","config":"{\"secret_key\":\"sk2\",\"publishable_key\":\"pk2\"}"}`)},
		{Name: "a placeholder secret is stored empty", Path: "/api/v2/admin/payment/gateways", Principal: admin, Mask: created,
			Body: []byte(`{"name":"Copy","type":"epay","config":{"pid":"3","key":"********"}}`)},
		{Name: "create without a configuration", Path: "/api/v2/admin/payment/gateways", Principal: admin, Mask: created,
			Body: []byte(`{"name":"Bare","type":"usdt"}`)},
		{Name: "a type that cannot be created", Path: "/api/v2/admin/payment/gateways", Principal: admin, Body: []byte(`{"name":"X","type":"x402"}`)},
		{Name: "no name", Path: "/api/v2/admin/payment/gateways", Principal: admin, Body: []byte(`{"type":"epay"}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/payment/gateways", Principal: admin, Body: []byte(`{"name":"X","type":"epay","fee_rate":"high"}`)},
		{Name: "no body", Path: "/api/v2/admin/payment/gateways", Principal: admin},
	})
	write(t, route("PUT", "/api/v2/admin/payment/gateways/:id", "payment.admin.payment.gateways.id.put", gateways((*handler.PaymentGatewayHandler).UpdateGateway)), gatewayState, []packagecompat.Case{
		{Name: "a redacted configuration keeps its secrets", Path: "/api/v2/admin/payment/gateways/4", Principal: admin, Mask: created,
			Body: []byte(`{"config":{"client_id":"cid2","client_secret":"********","nested":{"api_key":"********"},"sandbox_mode":false}}`)},
		{Name: "a new secret replaces the stored one", Path: "/api/v2/admin/payment/gateways/1", Principal: admin, Mask: created,
			Body: []byte(`{"name":"EPay renamed","config":{"api_url":"https://pay.example.test","pid":"1001","key":"rotated"},"fee_rate":0,"sort":9,"description":"d"}`)},
		{Name: "limits only", Path: "/api/v2/admin/payment/gateways/2", Principal: admin, Mask: created, Body: []byte(`{"min_amount":3,"max_amount":30,"fee_fixed":0.25,"icon":"s.svg"}`)},
		{Name: "an unreadable configuration stays hidden", Path: "/api/v2/admin/payment/gateways/5", Principal: admin, Mask: created, Body: []byte(`{"name":"Still broken"}`)},
		{Name: "an enabled gateway of a type without a callback", Path: "/api/v2/admin/payment/gateways/3", Principal: admin, Body: []byte(`{"name":"Alipay 2"}`)},
		{Name: "a type that cannot be set", Path: "/api/v2/admin/payment/gateways/1", Principal: admin, Body: []byte(`{"type":"paypal"}`)},
		{Name: "unknown gateway", Path: "/api/v2/admin/payment/gateways/99", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "id that is not a number", Path: "/api/v2/admin/payment/gateways/x", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "id that is a condition", Path: "/api/v2/admin/payment/gateways/0%20OR%201=1", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "no body", Path: "/api/v2/admin/payment/gateways/1", Principal: admin},
	})
	write(t, route("DELETE", "/api/v2/admin/payment/gateways/:id", "payment.admin.payment.gateways.id.delete", gateways((*handler.PaymentGatewayHandler).DeleteGateway)), gatewayState, []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/payment/gateways/5", Principal: admin},
		{Name: "unknown gateway", Path: "/api/v2/admin/payment/gateways/99", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/payment/gateways/x", Principal: admin},
	})
	write(t, route("POST", "/api/v2/admin/payment/gateways/:id/toggle", "payment.admin.payment.gateways.id.toggle.post", gateways((*handler.PaymentGatewayHandler).ToggleGateway)), gatewayState, []packagecompat.Case{
		{Name: "enable", Path: "/api/v2/admin/payment/gateways/2/toggle", Principal: admin, Body: []byte(`{"enabled":true}`)},
		{Name: "flip without a body", Path: "/api/v2/admin/payment/gateways/1/toggle", Principal: admin},
		{Name: "flip with an empty object", Path: "/api/v2/admin/payment/gateways/2/toggle", Principal: admin, Body: []byte(`{}`)},
		{Name: "disable", Path: "/api/v2/admin/payment/gateways/3/toggle", Principal: admin, Body: []byte(`{"enabled":false}`)},
		{Name: "enable a type without a callback", Path: "/api/v2/admin/payment/gateways/3/toggle", Principal: admin, Body: []byte(`{"enabled":true}`)},
		{Name: "enabled that is not a boolean", Path: "/api/v2/admin/payment/gateways/1/toggle", Principal: admin, Body: []byte(`{"enabled":"yes"}`)},
		{Name: "flip an unknown gateway", Path: "/api/v2/admin/payment/gateways/99/toggle", Principal: admin},
		{Name: "enable an unknown gateway", Path: "/api/v2/admin/payment/gateways/99/toggle", Principal: admin, Body: []byte(`{"enabled":true}`)},
		{Name: "disable an unknown gateway", Path: "/api/v2/admin/payment/gateways/99/toggle", Principal: admin, Body: []byte(`{"enabled":false}`)},
		{Name: "malformed body", Path: "/api/v2/admin/payment/gateways/1/toggle", Principal: admin, Body: []byte(`{"enabled":`)},
		{Name: "id beyond 32 bits", Path: "/api/v2/admin/payment/gateways/4294967296/toggle", Principal: admin},
	})
	read(t, route("GET", "/api/v2/user/payment/channels", "payment.user.payment.channels.get", gateways((*handler.PaymentGatewayHandler).GetChannels)), []packagecompat.Case{
		{Name: "enabled gateways of live types", Path: "/api/v2/user/payment/channels", Principal: buyer},
		{Name: "no gateways", Path: "/api/v2/user/payment/channels", Principal: buyer, Seed: empty},
	})
}

func TestRecordRoutesParity(t *testing.T) {
	records := route("GET", "/api/v2/admin/payment/records", "payment.admin.payment.records.get", gateways((*handler.PaymentGatewayHandler).ListPaymentRecords))
	read(t, records, []packagecompat.Case{
		{Name: "newest first", Path: "/api/v2/admin/payment/records", Principal: admin},
		{Name: "a page", Path: "/api/v2/admin/payment/records?page=2&page_size=2", Principal: admin},
		{Name: "a page size beyond the limit", Path: "/api/v2/admin/payment/records?page=0&page_size=1000", Principal: admin},
		{Name: "an empty page", Path: "/api/v2/admin/payment/records?page=&page_size=", Principal: admin},
		{Name: "by status name", Path: "/api/v2/admin/payment/records?status=paid", Principal: admin},
		{Name: "by status number", Path: "/api/v2/admin/payment/records?status=2", Principal: admin},
		{Name: "failed is cancelled", Path: "/api/v2/admin/payment/records?status=failed", Principal: admin},
		{Name: "an unknown status filters nothing", Path: "/api/v2/admin/payment/records?status=lost", Principal: admin},
		{Name: "by gateway type", Path: "/api/v2/admin/payment/records?gateway_type=epay&status=1", Principal: admin},
		{Name: "no records", Path: "/api/v2/admin/payment/records", Principal: admin, Seed: empty},
	})
	stats := route("GET", "/api/v2/admin/payment/stats", "payment.admin.payment.stats.get", gateways((*handler.PaymentGatewayHandler).GetPaymentStats))
	read(t, stats, []packagecompat.Case{
		{Name: "a range with every record", Path: "/api/v2/admin/payment/stats?start=2026-08-01&end=2026-09-30", Principal: admin},
		{Name: "a range with some", Path: "/api/v2/admin/payment/stats?start=2026-09-02&end=2026-09-30", Principal: admin},
		{Name: "the last 30 days", Path: "/api/v2/admin/payment/stats", Principal: admin},
		{Name: "an invalid start", Path: "/api/v2/admin/payment/stats?start=yesterday", Principal: admin},
		{Name: "an empty start", Path: "/api/v2/admin/payment/stats?start=", Principal: admin},
		{Name: "an invalid end", Path: "/api/v2/admin/payment/stats?start=2026-08-01&end=2026-13-01", Principal: admin},
		{Name: "no records", Path: "/api/v2/admin/payment/stats?start=2026-08-01&end=2026-09-30", Principal: admin, Seed: empty},
	})
	user := route("GET", "/api/v2/user/payment/records", "payment.user.payment.records.get", gateways((*handler.PaymentGatewayHandler).GetUserRecords))
	read(t, user, []packagecompat.Case{
		{Name: "the caller's records", Path: "/api/v2/user/payment/records", Principal: buyer},
		{Name: "another caller's", Path: "/api/v2/user/payment/records", Principal: other},
		{Name: "a page", Path: "/api/v2/user/payment/records?page=2&page_size=2", Principal: buyer},
		{Name: "page zero", Path: "/api/v2/user/payment/records?page=0&page_size=2", Principal: buyer},
		{Name: "a caller without records", Path: "/api/v2/user/payment/records", Principal: admin},
	})
}

func TestStatusRoutesParity(t *testing.T) {
	userStatus := route("GET", "/api/v2/user/payment/status/:trade_no", "payment.user.payment.status.trade_no.get", gateways((*handler.PaymentGatewayHandler).GetPaymentStatus))
	read(t, userStatus, []packagecompat.Case{
		{Name: "the caller's payment", Path: "/api/v2/user/payment/status/PAY20260901080000aaaa0001", Principal: buyer},
		{Name: "another user's payment", Path: "/api/v2/user/payment/status/PAY20260903080000cccc0003", Principal: buyer},
		{Name: "unknown payment", Path: "/api/v2/user/payment/status/NOPE", Principal: buyer},
	})
	publicStatus := route("GET", "/api/v2/payment/status/:trade_no", "payment.payment.status.trade_no.get", payments((*handler.PaymentHandler).GetPaymentStatus))
	read(t, publicStatus, []packagecompat.Case{
		{Name: "a crypto payment", Path: "/api/v2/payment/status/X40220260902080000bbbb0002", Principal: public},
		{Name: "a paid payment", Path: "/api/v2/payment/status/PAY20260901080000aaaa0001", Principal: public},
		{Name: "a refunded payment", Path: "/api/v2/payment/status/PAY20260904080000dddd0004", Principal: public},
		{Name: "an expired payment", Path: "/api/v2/payment/status/FIAT20260805080000eeee0005", Principal: public},
		{Name: "unknown payment", Path: "/api/v2/payment/status/NOPE", Principal: public},
	})
	methods := route("GET", "/api/v2/payment/methods", "payment.payment.methods.get", payments((*handler.PaymentHandler).GetPaymentMethods))
	read(t, methods, []packagecompat.Case{
		{Name: "the enabled methods", Path: "/api/v2/payment/methods", Principal: public},
		{Name: "crypto while no method is enabled", Path: "/api/v2/payment/methods", Principal: public, Seed: noMethods},
		{Name: "no methods", Path: "/api/v2/payment/methods", Principal: public, Seed: empty},
	})
	check := route("GET", "/api/v2/payment/x402/check/:id", "payment.payment.x402.check.id.get", payments((*handler.PaymentHandler).X402CheckPayment))
	read(t, check, []packagecompat.Case{
		{Name: "by trade number", Path: "/api/v2/payment/x402/check/X40220260902080000bbbb0002", Principal: buyer},
		{Name: "by id", Path: "/api/v2/payment/x402/check/1", Principal: buyer},
		{Name: "an expired payment", Path: "/api/v2/payment/x402/check/5", Principal: buyer},
		{Name: "another user's by trade number", Path: "/api/v2/payment/x402/check/PAY20260903080000cccc0003", Principal: buyer},
		{Name: "another user's by id", Path: "/api/v2/payment/x402/check/4", Principal: buyer},
		{Name: "an id beyond 64 bits wraps around", Path: "/api/v2/payment/x402/check/18446744073709551617", Principal: buyer},
		{Name: "unknown id", Path: "/api/v2/payment/x402/check/99", Principal: buyer},
		{Name: "unknown trade number", Path: "/api/v2/payment/x402/check/NOPE", Principal: buyer},
	})
}

func TestCreatePaymentRoutesParity(t *testing.T) {
	create := route("POST", "/api/v2/user/payment/create", "payment.user.payment.create.post", gateways((*handler.PaymentGatewayHandler).CreatePayment))
	traded := []string{"data.trade_no"}
	write(t, create, recordState, []packagecompat.Case{
		{Name: "a top-up with the gateway's fee", Path: "/api/v2/user/payment/create", Principal: buyer, Mask: traded, Body: []byte(`{"gateway_id":1,"amount":10}`)},
		{Name: "the caller's order in full", Path: "/api/v2/user/payment/create", Principal: buyer, Mask: traded, Body: []byte(`{"gateway_id":1,"amount":100,"order_id":1}`)},
		{Name: "an order to the cent", Path: "/api/v2/user/payment/create", Principal: buyer, Mask: traded, Body: []byte(`{"gateway_id":1,"amount":12.34,"order_id":5}`)},
		{Name: "less than the order's total", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":1,"order_id":1}`)},
		{Name: "more than the order's total", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":101,"order_id":1}`)},
		{Name: "another user's order", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":70,"order_id":3}`)},
		{Name: "a paid order", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":50,"order_id":2}`)},
		{Name: "an unknown order", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":50,"order_id":99}`)},
		{Name: "an order below the gateway's minimum", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":1,"order_id":4}`)},
		{Name: "a disabled gateway", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":2,"amount":10}`)},
		{Name: "a gateway type without a callback", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":3,"amount":10}`)},
		{Name: "below the gateway's minimum", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":4,"amount":2}`)},
		{Name: "above the gateway's maximum", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":4,"amount":501}`)},
		{Name: "an unknown gateway", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":99,"amount":10}`)},
		{Name: "an amount below one", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":1,"amount":0.5}`)},
		{Name: "wrong field type", Path: "/api/v2/user/payment/create", Principal: buyer, Body: []byte(`{"gateway_id":"one","amount":10}`)},
		{Name: "no body", Path: "/api/v2/user/payment/create", Principal: buyer},
	})
	x402 := route("POST", "/api/v2/payment/x402/create", "payment.payment.x402.create.post", payments((*handler.PaymentHandler).X402CreatePayment))
	write(t, x402, recordState, []packagecompat.Case{
		{Name: "the caller's order", Path: "/api/v2/payment/x402/create", Principal: buyer,
			Mask: []string{"data.trade_no", "data.wallet_address", "data.expires_at", "data.qr_code"}, Body: []byte(`{"order_id":1,"token":"USDC","network":"base-sepolia"}`)},
		{Name: "without a token or network", Path: "/api/v2/payment/x402/create", Principal: buyer,
			Mask: []string{"data.trade_no", "data.wallet_address", "data.expires_at", "data.qr_code"}, Body: []byte(`{"order_id":5}`)},
		{Name: "another user's order", Path: "/api/v2/payment/x402/create", Principal: buyer, Body: []byte(`{"order_id":3,"token":"ETH"}`)},
		{Name: "a paid order", Path: "/api/v2/payment/x402/create", Principal: buyer, Body: []byte(`{"order_id":2,"token":"ETH"}`)},
		{Name: "an unknown order", Path: "/api/v2/payment/x402/create", Principal: buyer, Body: []byte(`{"order_id":99,"token":"ETH"}`)},
		{Name: "no order", Path: "/api/v2/payment/x402/create", Principal: buyer, Body: []byte(`{"token":"ETH"}`)},
		{Name: "no body", Path: "/api/v2/payment/x402/create", Principal: buyer},
	})
	fiat := route("POST", "/api/v2/payment/fiat/create", "payment.payment.fiat.create.post", payments((*handler.PaymentHandler).FiatCreatePayment))
	write(t, fiat, recordState, []packagecompat.Case{
		{Name: "a simulated Stripe checkout", Path: "/api/v2/payment/fiat/create", Principal: buyer,
			Mask: []string{"data.trade_no", "data.checkout_url", "data.session_id"}, Body: []byte(`{"order_id":1,"provider":"stripe"}`)},
		{Name: "a simulated PayPal order", Path: "/api/v2/payment/fiat/create", Principal: buyer,
			Mask: []string{"data.trade_no", "data.approve_url", "data.order_id"}, Body: []byte(`{"order_id":5,"provider":"paypal"}`)},
		{Name: "an unknown provider", Path: "/api/v2/payment/fiat/create", Principal: buyer, Body: []byte(`{"order_id":1,"provider":"alipay"}`)},
		{Name: "another user's order", Path: "/api/v2/payment/fiat/create", Principal: buyer, Body: []byte(`{"order_id":3,"provider":"stripe"}`)},
		{Name: "a paid order", Path: "/api/v2/payment/fiat/create", Principal: buyer, Body: []byte(`{"order_id":2,"provider":"stripe"}`)},
		{Name: "no provider", Path: "/api/v2/payment/fiat/create", Principal: buyer, Body: []byte(`{"order_id":1}`)},
		{Name: "no body", Path: "/api/v2/payment/fiat/create", Principal: buyer},
	})
}
