// Package affiliatecompat proves the affiliate package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (times the handlers take from their clock masked) and the
// same resulting rows.
//
// The native side reads the referral, entitlement and settings views the
// kernel publishes (packagestore.EnsureKernelAPIViews), and changes
// commission balances through the real KernelSubscriber server
// (internal/kernelsubscriber) in process over gRPC, on the native side's
// database, so both sides end with the same withdrawals, balances and
// request ledger.
package affiliatecompat

import (
	"context"
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
	"github.com/AnixOps/anix-control/v4/packages/affiliate/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

var (
	admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	// alice invited bob, carol and dave and holds 500 in commission.
	alice = pluginhostsdk.Principal{ActorID: 2}
	// bob invited eve and frank and holds 50.
	bob = pluginhostsdk.Principal{ActorID: 3}
	// carol has no commission.
	carol = pluginhostsdk.Principal{ActorID: 4}
)

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// affiliateHost is the identity the kernel serves the affiliate host as.
var affiliateHost = packagebridge.HostIdentity{PackageID: "affiliate", Version: "4.0.0", Generation: 1}

// balanceOnly authorizes what the affiliate package's signed release
// declares: the balance family, for the affiliate host.
type balanceOnly struct{}

func (balanceOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == affiliateHost && capability == service.CapabilitySubscriberBalance {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelSubscriber serves the kernel's KernelSubscriber on db in process and
// returns a client for it, as the affiliate host gets one over its bridge.
func kernelSubscriber(t *testing.T, db *gorm.DB) kernelsubscriberv1.KernelSubscriberClient {
	t.Helper()
	server := &kernelsubscriber.Server{DB: db, Authorizer: balanceOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server.For(affiliateHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelsubscriberv1.NewKernelSubscriberClient(conn)
}

func route(t *testing.T, method, pattern, routeID string, legacy func(*handler.InviteHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID,
		// The legacy handlers are built per request: their services keep
		// the database they were built with, which the harness sets up per
		// case.
		Legacy: func(c *gin.Context) { legacy(handler.NewInviteHandler(), c) },
		Models: []any{
			&model.Plan{}, &model.User{}, &model.CommissionRecord{}, &model.CommissionWithdraw{}, &model.InviteConfig{},
			&model.SystemConfig{}, &model.SubscriberRequest{},
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

func ptr[T any](value T) *T { return &value }

// settingsValue is the affiliate frontend settings row.
const settingsValue = `{"code_prefix":" AFF ","code_length":6,"withdraw_fee":1.5,"withdraw_methods":["bank","alipay"]}`

// seedWith writes users, commissions, withdrawals and, as asked, the invite
// configuration and the frontend settings, then the kernel views.
func seedWith(config bool, settings *string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		user := func(id uint, inviter *uint, commission int64) model.User {
			return model.User{
				ID: id, Email: fmt.Sprintf("user%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id),
				InviteUserID: inviter, CommissionBalance: commission, Balance: 7, IsAdmin: map[bool]int{true: 1}[id == 1],
				CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.User{
			user(1, nil, 0), user(2, nil, 500), user(3, ptr(uint(2)), 50), user(4, ptr(uint(2)), 0), user(5, ptr(uint(2)), 0),
			user(6, ptr(uint(3)), 3), user(7, ptr(uint(3)), 0), user(8, ptr(uint(6)), 0),
		}).Error)
		commission := func(id, owner, order, from uint, amount float64, status int) model.CommissionRecord {
			return model.CommissionRecord{
				ID: id, UserID: owner, OrderID: order, FromUserID: from, Amount: amount, Type: 1, Status: status, Remark: fmt.Sprintf("order %d", order),
				CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.CommissionRecord{
			commission(1, 2, 10, 3, 12.5, 1), commission(2, 2, 11, 4, 30, 2), commission(3, 2, 12, 5, 7.25, 0),
			commission(4, 3, 13, 6, 20, 1), commission(5, 3, 14, 7, 5, 3), commission(6, 6, 15, 8, 1, 1),
		}).Error)
		processed := seeded.Add(time.Hour)
		withdrawal := func(id, owner uint, amount float64, method string, status int, at *time.Time) model.CommissionWithdraw {
			return model.CommissionWithdraw{
				ID: id, UserID: owner, Amount: amount, Method: method, Account: fmt.Sprintf("account-%d", id), Name: fmt.Sprintf("name %d", id),
				Status: status, ProcessedAt: at, CreatedAt: seeded.Add(time.Duration(10+id) * time.Minute), UpdatedAt: seeded,
			}
		}
		rows := []model.CommissionWithdraw{
			withdrawal(1, 2, 100, "alipay", 0, nil), withdrawal(2, 2, 50, "wechat", 1, &processed), withdrawal(3, 3, 20, "bank", 2, &processed),
			// Stored before amounts had to be whole.
			withdrawal(4, 3, 1.99, "alipay", 0, nil),
			// Its user no longer exists.
			withdrawal(5, 99, 40, "bank", 0, nil),
			// The native module's reservation.
			withdrawal(6, 2, 25, "alipay", -1, nil),
			withdrawal(7, 0, 10, "alipay", 0, nil),
		}
		rows[2].Remark = "rejected before"
		require.NoError(t, db.Create(&rows).Error)
		if config {
			require.NoError(t, db.Create(&model.InviteConfig{
				ID: 1, Enabled: true, AutoGenerate: true, CodeCount: 3, CodeExpireDays: 30, CommissionEnabled: true, CommissionType: 2,
				CommissionRate: 0.15, CommissionFixed: 4.5, CommissionMinAmount: 10, FirstOrderBonus: 2, FirstTrafficBonus: 1 << 30,
				CreatedAt: seeded, UpdatedAt: seeded,
			}).Error)
		}
		require.NoError(t, db.Create(&model.SystemConfig{ID: 1, Key: "smtp.password", Value: "smtp-secret", CreatedAt: seeded, UpdatedAt: seeded}).Error)
		if settings != nil {
			require.NoError(t, db.Create(&model.SystemConfig{
				ID: 2, Key: packagestore.InviteSettingsKey, Value: *settings, Type: "json", Group: "invite", CreatedAt: seeded, UpdatedAt: seeded,
			}).Error)
		}
		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		syncSequences(t, db)
	}
}

var seed = seedWith(true, ptr(settingsValue))

// withLedger seeds a request already in the subscriber ledger.
func withLedger(requestID string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seed(t, db)
		require.NoError(t, db.Create(&model.SubscriberRequest{
			RequestID: requestID, Method: "adjust_balance", UserID: 2, Result: `{"balance":0}`, CreatedAt: seeded,
		}).Error)
	}
}

func empty(t testing.TB, db *gorm.DB) { require.NoError(t, packagestore.EnsureKernelAPIViews(db)) }

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_user", "v2_commission_record", "v2_commission_withdraw", "v2_invite_config", "v2_system_config"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockTime is a time the handlers take from their clock: masked unless it
// was seeded.
func clockTime(value time.Time) string {
	if !value.IsZero() && value.Sub(seeded) >= 0 && value.Sub(seeded) <= 2*time.Hour {
		return value.UTC().Format(time.RFC3339)
	}
	if d := time.Since(value); d > -10*time.Minute && d < 10*time.Minute {
		return "<now>"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func clockTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return clockTime(*value)
}

// state is everything the affiliate routes can change: the withdrawals,
// the users' balances, the subscriber request ledger and the invite
// configuration.
func state(t testing.TB, db *gorm.DB) any {
	var withdrawals []model.CommissionWithdraw
	require.NoError(t, db.Order("id").Find(&withdrawals).Error)
	rows := make([]map[string]any, 0, len(withdrawals))
	for _, w := range withdrawals {
		rows = append(rows, map[string]any{
			"id": w.ID, "user_id": w.UserID, "amount": w.Amount, "method": w.Method, "account": w.Account, "name": w.Name,
			"status": w.Status, "remark": w.Remark, "processed_at": clockTimePtr(w.ProcessedAt),
			"created_at": clockTime(w.CreatedAt), "updated_at": clockTime(w.UpdatedAt),
		})
	}
	var users []struct {
		ID                uint
		Balance           int64
		CommissionBalance int64
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	var requests []model.SubscriberRequest
	require.NoError(t, db.Order("request_id").Find(&requests).Error)
	ledger := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		ledger = append(ledger, map[string]any{
			"request_id": request.RequestID, "method": request.Method, "user_id": request.UserID, "result": request.Result,
			"created_at": clockTime(request.CreatedAt),
		})
	}
	var configs []model.InviteConfig
	require.NoError(t, db.Order("id").Find(&configs).Error)
	configRows := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		configRows = append(configRows, map[string]any{
			"id": cfg.ID, "enabled": cfg.Enabled, "auto_generate": cfg.AutoGenerate, "code_count": cfg.CodeCount,
			"code_expire_days": cfg.CodeExpireDays, "commission_enabled": cfg.CommissionEnabled, "commission_type": cfg.CommissionType,
			"commission_rate": cfg.CommissionRate, "commission_fixed": cfg.CommissionFixed, "commission_min": cfg.CommissionMinAmount,
			"first_order_bonus": cfg.FirstOrderBonus, "first_traffic_bonus": cfg.FirstTrafficBonus,
			"created_at": clockTime(cfg.CreatedAt), "updated_at": clockTime(cfg.UpdatedAt),
		})
	}
	return map[string]any{"withdrawals": rows, "users": users, "ledger": ledger, "configs": configRows}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Snapshot = state
		packagecompat.RunWrite(t, r, c)
	}
}

func TestUserListsParity(t *testing.T) {
	read(t, route(t, "GET", "/api/v2/user/invite/commissions", "affiliate.user.invite.commissions.get", (*handler.InviteHandler).GetCommissionRecords),
		[]packagecompat.Case{
			{Name: "the caller's commissions newest first", Path: "/api/v2/user/invite/commissions", Principal: alice},
			{Name: "another inviter's", Path: "/api/v2/user/invite/commissions", Principal: bob},
			{Name: "none", Path: "/api/v2/user/invite/commissions", Principal: carol},
			{Name: "a second page", Path: "/api/v2/user/invite/commissions?page=2&page_size=2", Principal: alice},
			{Name: "page zero", Path: "/api/v2/user/invite/commissions?page=0", Principal: alice},
			{Name: "page size zero", Path: "/api/v2/user/invite/commissions?page_size=0", Principal: alice},
			{Name: "a negative page size", Path: "/api/v2/user/invite/commissions?page_size=-1", Principal: alice},
			{Name: "a page that is not a number", Path: "/api/v2/user/invite/commissions?page=x&page_size=", Principal: alice},
			{Name: "no data", Path: "/api/v2/user/invite/commissions", Principal: alice, Seed: empty},
		})
	read(t, route(t, "GET", "/api/v2/user/invite/withdrawals", "affiliate.user.invite.withdrawals.get", (*handler.InviteHandler).GetWithdrawRecords),
		[]packagecompat.Case{
			{Name: "the caller's withdrawals with a reservation", Path: "/api/v2/user/invite/withdrawals", Principal: alice},
			{Name: "another user's", Path: "/api/v2/user/invite/withdrawals", Principal: bob},
			{Name: "none", Path: "/api/v2/user/invite/withdrawals", Principal: carol},
			{Name: "a second page", Path: "/api/v2/user/invite/withdrawals?page=2&page_size=1", Principal: alice},
			{Name: "a negative page", Path: "/api/v2/user/invite/withdrawals?page=-1", Principal: alice},
		})
}

func TestAdminReadsParity(t *testing.T) {
	read(t, route(t, "GET", "/api/v2/admin/invite/withdrawals", "affiliate.admin.invite.withdrawals.get", (*handler.InviteHandler).GetWithdrawals),
		[]packagecompat.Case{
			{Name: "every withdrawal", Path: "/api/v2/admin/invite/withdrawals", Principal: admin},
			{Name: "pending", Path: "/api/v2/admin/invite/withdrawals?status=pending", Principal: admin},
			{Name: "status 0", Path: "/api/v2/admin/invite/withdrawals?status=0", Principal: admin},
			{Name: "approved", Path: "/api/v2/admin/invite/withdrawals?status=Approved", Principal: admin},
			{Name: "status 2", Path: "/api/v2/admin/invite/withdrawals?status=2", Principal: admin},
			{Name: "reservations", Path: "/api/v2/admin/invite/withdrawals?status=-1", Principal: admin},
			{Name: "an unknown status filters nothing", Path: "/api/v2/admin/invite/withdrawals?status=bogus", Principal: admin},
			{Name: "page bounds", Path: "/api/v2/admin/invite/withdrawals?page=0&page_size=500", Principal: admin},
			{Name: "a second page", Path: "/api/v2/admin/invite/withdrawals?page=2&page_size=3", Principal: admin},
			{Name: "no withdrawals", Path: "/api/v2/admin/invite/withdrawals", Principal: admin, Seed: empty},
		})
	read(t, route(t, "GET", "/api/v2/admin/invite/stats", "affiliate.admin.invite.stats.get", (*handler.InviteHandler).GetInviteStats),
		[]packagecompat.Case{
			{Name: "inviters, commissions and withdrawals", Path: "/api/v2/admin/invite/stats", Principal: admin},
			{Name: "no data", Path: "/api/v2/admin/invite/stats", Principal: admin, Seed: empty},
		})
}

func TestAdminConfigParity(t *testing.T) {
	config := route(t, "GET", "/api/v2/admin/invite/config", "affiliate.admin.invite.config.get", (*handler.InviteHandler).GetConfig)
	created := []string{"data.created_at", "data.updated_at"}
	write(t, config, []packagecompat.Case{
		{Name: "stored configuration and settings", Path: "/api/v2/admin/invite/config", Principal: admin},
		{Name: "no configuration creates the default", Path: "/api/v2/admin/invite/config", Principal: admin, Mask: created,
			Seed: seedWith(false, ptr(settingsValue))},
		{Name: "no settings", Path: "/api/v2/admin/invite/config", Principal: admin, Seed: seedWith(true, nil)},
		{Name: "empty settings", Path: "/api/v2/admin/invite/config", Principal: admin, Seed: seedWith(true, ptr(""))},
		{Name: "settings that do not parse", Path: "/api/v2/admin/invite/config", Principal: admin, Seed: seedWith(true, ptr(`{"code_length":"x"}`))},
		{Name: "partial settings", Path: "/api/v2/admin/invite/config", Principal: admin, Seed: seedWith(true, ptr(`{"withdraw_fee":2.5,"code_prefix":"  "}`))},
		{Name: "null settings", Path: "/api/v2/admin/invite/config", Principal: admin, Seed: seedWith(true, ptr("null"))},
	})
}

func TestProcessWithdrawalParity(t *testing.T) {
	process := route(t, "POST", "/api/v2/admin/invite/withdrawals/:id/process", native.ProcessRouteID, (*handler.InviteHandler).ProcessWithdraw)
	decided := []string{"data.processed_at", "data.updated_at"}
	path := func(id string) string { return "/api/v2/admin/invite/withdrawals/" + id + "/process" }
	write(t, process, []packagecompat.Case{
		{Name: "approve", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"approve":true,"remark":"paid out"}`)},
		{Name: "reject refunds", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"approve":false,"remark":"wrong account"}`)},
		{Name: "approved alias", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"approved":"yes"}`)},
		{Name: "status text", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"status":"rejected"}`)},
		{Name: "status number", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"status":2}`)},
		{Name: "status digit text", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"status":"1"}`)},
		{Name: "approve wins over status", Path: path("1"), Principal: admin, Mask: decided, Body: []byte(`{"approve":0,"status":1}`)},
		{Name: "approve that is not boolean", Path: path("1"), Principal: admin, Body: []byte(`{"approve":"maybe"}`)},
		{Name: "approved that is not boolean", Path: path("1"), Principal: admin, Body: []byte(`{"approved":2}`)},
		{Name: "an unknown status", Path: path("1"), Principal: admin, Body: []byte(`{"status":3}`)},
		{Name: "no decision", Path: path("1"), Principal: admin, Body: []byte(`{"remark":"x"}`)},
		{Name: "a fractional withdrawal is refunded its whole part", Path: path("4"), Principal: admin, Mask: decided, Body: []byte(`{"approve":false}`)},
		{Name: "a rejection without its user", Path: path("5"), Principal: admin, Mask: decided, Body: []byte(`{"approve":false}`)},
		{Name: "a rejection for user zero", Path: path("7"), Principal: admin, Mask: decided, Body: []byte(`{"approve":false}`)},
		{Name: "rejecting twice refunds once", Path: path("1"), Principal: admin, Body: []byte(`{"approve":false}`),
			Warmup: [][]byte{[]byte(`{"approve":false}`)}},
		{Name: "a refund already in the ledger is not paid again", Path: path("1"), Principal: admin, Mask: decided,
			Body: []byte(`{"approve":false}`), Seed: withLedger("affiliate.withdraw.refund:1")},
		{Name: "already approved", Path: path("2"), Principal: admin, Body: []byte(`{"approve":false}`)},
		{Name: "already rejected", Path: path("3"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "a reservation", Path: path("6"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "unknown withdrawal", Path: path("99"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "withdrawal zero", Path: path("0"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "id that is not a number", Path: path("x"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "id that is a condition", Path: path("0%20OR%201=1"), Principal: admin, Body: []byte(`{"approve":false}`)},
		{Name: "id beyond 32 bits", Path: path("4294967297"), Principal: admin, Body: []byte(`{"approve":true}`)},
		{Name: "an invalid body before the id", Path: path("x"), Principal: admin, Body: []byte(`{"approve":`)},
		{Name: "no body", Path: path("1"), Principal: admin},
		{Name: "a body that is not an object", Path: path("1"), Principal: admin, Body: []byte(`[1]`)},
	})
}

func TestWithdrawParity(t *testing.T) {
	withdraw := route(t, "POST", "/api/v2/user/invite/withdraw", native.WithdrawRouteID, (*handler.InviteHandler).RequestWithdraw)
	created := []string{"data.created_at", "data.updated_at"}
	body := func(amount string) []byte {
		return []byte(`{"amount":` + amount + `,"method":"bank","account":"6222 0000","name":"Alice"}`)
	}
	write(t, withdraw, []packagecompat.Case{
		{Name: "a withdrawal", Path: "/api/v2/user/invite/withdraw", Principal: alice, Mask: created, Body: body("100")},
		{Name: "the whole balance", Path: "/api/v2/user/invite/withdraw", Principal: alice, Mask: created, Body: body("500")},
		{Name: "a whole amount written with a fraction", Path: "/api/v2/user/invite/withdraw", Principal: bob, Mask: created, Body: body("50.0")},
		{Name: "more than the balance", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("501")},
		{Name: "a huge amount", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("1e300")},
		{Name: "below the minimum", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("9")},
		{Name: "a fractional amount", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("10.5")},
		{Name: "a negative amount", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("-5")},
		{Name: "a zero amount", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("0")},
		{Name: "an amount that is a string", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body(`"100"`)},
		{Name: "an unknown method", Path: "/api/v2/user/invite/withdraw", Principal: alice,
			Body: []byte(`{"amount":100,"method":"paypal","account":"a","name":"n"}`)},
		{Name: "no account", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: []byte(`{"amount":100,"method":"bank","name":"n"}`)},
		{Name: "no body", Path: "/api/v2/user/invite/withdraw", Principal: alice},
		{Name: "a body that does not parse", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: []byte(`{"amount":`)},
		{Name: "no commission", Path: "/api/v2/user/invite/withdraw", Principal: carol, Body: body("10")},
		{Name: "an unknown user", Path: "/api/v2/user/invite/withdraw", Principal: pluginhostsdk.Principal{ActorID: 99}, Body: body("10")},
		{Name: "the second of two withdrawals overdraws", Path: "/api/v2/user/invite/withdraw", Principal: bob, Body: body("30"),
			Warmup: [][]byte{body("30")}},
		{Name: "no configuration creates the default minimum", Path: "/api/v2/user/invite/withdraw", Principal: alice, Mask: created,
			Body: body("10"), Seed: seedWith(false, nil)},
		{Name: "the default minimum", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("9"), Seed: seedWith(false, nil)},
		{Name: "a debit id already in the ledger", Path: "/api/v2/user/invite/withdraw", Principal: alice, Body: body("100"),
			Seed: withLedger("affiliate.withdraw:8")},
	})
}
