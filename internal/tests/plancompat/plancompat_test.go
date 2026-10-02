// Package plancompat proves the plan package's native routes answer exactly
// as the kernel's legacy handlers, on SQLite and PostgreSQL: the same bytes
// (times the handlers take from their clock masked) and the same resulting
// rows.
//
// The native assignment reaches the real KernelSubscriber server
// (internal/kernelsubscriber) in process over gRPC, on the native side's
// database, so both sides end with the same subscriber rows, request ledger
// and change log.
package plancompat

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
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/plan/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

var (
	admin  = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	member = pluginhostsdk.Principal{ActorID: 2}
)

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// planHost is the identity the kernel serves the plan host as.
var planHost = packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 1}

// entitlementsOnly authorizes what the plan package's signed release
// declares: the entitlement family, for the plan host.
type entitlementsOnly struct{}

func (entitlementsOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == planHost && capability == service.CapabilitySubscriberEntitlements {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelSubscriber serves the kernel's KernelSubscriber on db in process and
// returns a client for it, as the plan host gets one over its bridge.
func kernelSubscriber(t *testing.T, db *gorm.DB) kernelsubscriberv1.KernelSubscriberClient {
	t.Helper()
	server := &kernelsubscriber.Server{DB: db, Authorizer: entitlementsOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server.For(planHost))
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
			&model.Plan{}, &model.User{}, &model.Event{}, &model.UserSubscriptionGroup{},
			&model.SubscriberRequest{}, &model.SubscriberChange{},
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

// The legacy admin handler is built per request: its services keep the
// database they were built with, which the harness sets up per case.
func admins(method func(*handler.AdminHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewAdminHandler(), c) }
}

func ptr[T any](value T) *T { return &value }

func seedUsers(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.Plan{
		{ID: 1, GroupID: 1, TransferEnable: 10, Name: "Basic", Content: ptr("basic"), Show: 1, Sort: ptr(2), Renew: 1, MonthPrice: ptr(int64(1000)), CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1},
		{
			ID: 2, Email: "two@example.test", Token: "t2", UUID: "u2", PlanID: ptr(uint(1)), GroupID: ptr(uint(1)), TransferEnable: 10 << 30,
			U: 100, D: 200, ExpiredAt: ptr(int64(1_800_000_000)), SpeedLimit: ptr(int64(50)), DeviceLimit: ptr(2),
		},
		{ID: 3, Email: "three@example.test", Token: "t3", UUID: "u3", U: 5, D: 6},
		{ID: 4, Email: "four@example.test", Token: "t4", UUID: "u4", Banned: 1},
	}).Error)
	require.NoError(t, db.Create(&model.SubscriptionGroup{ID: 9, Name: "group 9", CreatedAt: seeded, UpdatedAt: seeded}).Error)
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{ID: 1, UserID: 2, GroupID: 9, CreatedAt: seeded}).Error)
	syncSequences(t, db)
}

// seed adds plans on sale, hidden and without a sort order to seedUsers.
func seed(t testing.TB, db *gorm.DB) {
	seedUsers(t, db)
	require.NoError(t, db.Create(&[]model.Plan{
		{
			ID: 2, GroupID: 2, TransferEnable: 100, SpeedLimit: ptr(int64(200)), DeviceLimit: ptr(5), Name: "Pro", Content: ptr("pro"),
			Show: 1, Sort: ptr(1), Renew: 1, ResetPrice: ptr(int64(500)), ResetTrafficMethod: ptr(1), CapacityLimit: ptr(100),
			MonthPrice: ptr(int64(3000)), QuarterPrice: ptr(int64(8000)), HalfYearPrice: ptr(int64(15000)), YearPrice: ptr(int64(28000)),
			TwoYearPrice: ptr(int64(50000)), ThreeYearPrice: ptr(int64(70000)), OnetimePrice: ptr(int64(99000)),
			CreatedAt: seeded, UpdatedAt: seeded.Add(time.Hour),
		},
		{ID: 3, GroupID: 3, TransferEnable: 1, Name: "Hidden", Sort: ptr(0), Renew: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 4, GroupID: 1, Name: "Unsorted", Show: 1, Renew: 1, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	// Renew defaults to 1 on insert; plan 4 does not renew.
	require.NoError(t, db.Model(&model.Plan{}).Where("id = ?", 4).UpdateColumn("renew", 0).Error)
	syncSequences(t, db)
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_plan", "v2_user", "v2_event", "v2_user_subscription_group"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockTime is a time the handlers take from their clock: masked unless it
// is the seeded one.
func clockTime(value time.Time) string {
	if value.Equal(seeded) || value.Equal(seeded.Add(time.Hour)) {
		return value.UTC().Format(time.RFC3339)
	}
	return "<now>"
}

// plans is v2_plan and v2_event after a write.
func plans(t testing.TB, db *gorm.DB) any {
	var rows []model.Plan
	require.NoError(t, db.Order("id").Find(&rows).Error)
	type plan struct {
		model.Plan
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	state := make([]plan, 0, len(rows))
	for _, row := range rows {
		state = append(state, plan{Plan: row, CreatedAt: clockTime(row.CreatedAt), UpdatedAt: clockTime(row.UpdatedAt)})
	}
	return map[string]any{"plans": state, "events": events(t, db)}
}

// events are the v2_event rows, their payloads decoded and clock times
// masked.
func events(t testing.TB, db *gorm.DB) any {
	var rows []model.Event
	require.NoError(t, db.Order("id").Find(&rows).Error)
	type event struct {
		ID        uint
		Type      string
		Status    string
		Payload   map[string]any
		Processed bool
	}
	state := make([]event, 0, len(rows))
	for _, row := range rows {
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(*row.Payload), &payload))
		for _, key := range []string{"created_at", "updated_at"} {
			if raw, ok := payload[key].(string); ok {
				parsed, err := time.Parse(time.RFC3339Nano, raw)
				require.NoError(t, err)
				payload[key] = clockTime(parsed)
			}
		}
		state = append(state, event{ID: row.ID, Type: row.Type, Status: row.Status, Payload: payload, Processed: row.ProcessedAt != nil})
	}
	return state
}

// subscribers is the shared subscriber state an assignment changes: the
// v2_user entitlements, the subscription groups, the request ledger and
// the change log, plus the plan events.
func subscribers(maskRequestIDs bool) func(t testing.TB, db *gorm.DB) any {
	return func(t testing.TB, db *gorm.DB) any {
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
		}
		require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
		var groups []struct {
			UserID  uint
			GroupID uint
		}
		require.NoError(t, db.Model(&model.UserSubscriptionGroup{}).Order("user_id, group_id").Find(&groups).Error)
		var requests []model.SubscriberRequest
		require.NoError(t, db.Order("created_at, request_id").Find(&requests).Error)
		ledger := make([]map[string]any, 0, len(requests))
		for index, request := range requests {
			id := request.RequestID
			if maskRequestIDs {
				id = fmt.Sprintf("<request %d>", index+1)
			}
			ledger = append(ledger, map[string]any{"request_id": id, "method": request.Method, "user_id": request.UserID, "result": request.Result})
		}
		var changes []struct {
			UserID  uint
			Deleted bool
		}
		require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
		return map[string]any{"users": users, "groups": groups, "ledger": ledger, "changes": changes, "events": events(t, db)}
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

func TestPlanReadRoutesParity(t *testing.T) {
	read(t, route(t, "GET", "/api/v2/admin/plans", "plan.admin.plans.get", admins((*handler.AdminHandler).GetPlans)), []packagecompat.Case{
		{Name: "every plan by sort then id", Path: "/api/v2/admin/plans", Principal: admin},
		{Name: "one plan", Path: "/api/v2/admin/plans", Principal: admin, Seed: seedUsers},
		{Name: "no plans", Path: "/api/v2/admin/plans", Principal: admin, Seed: func(testing.TB, *gorm.DB) {}},
	})
	read(t, route(t, "GET", "/api/v2/admin/plans/:id", "plan.admin.plans.id.get", admins((*handler.AdminHandler).GetPlan)), []packagecompat.Case{
		{Name: "plan with every field", Path: "/api/v2/admin/plans/2", Principal: admin},
		{Name: "plan without optional fields", Path: "/api/v2/admin/plans/4", Principal: admin},
		{Name: "unknown plan", Path: "/api/v2/admin/plans/99", Principal: admin},
		{Name: "plan zero", Path: "/api/v2/admin/plans/0", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/plans/x", Principal: admin},
		{Name: "id beyond 32 bits", Path: "/api/v2/admin/plans/4294967296", Principal: admin},
		{Name: "id that is a condition", Path: "/api/v2/admin/plans/0%20OR%201=1", Principal: admin},
	})
	read(t, route(t, "GET", "/api/v2/user/plan", "plan.user.plan.get", handler.NewUserPlanHandler().GetPlans), []packagecompat.Case{
		{Name: "plans on sale by sort", Path: "/api/v2/user/plan", Principal: member},
		{Name: "nothing on sale", Path: "/api/v2/user/plan", Principal: member, Seed: func(testing.TB, *gorm.DB) {}},
	})
}

func TestPlanWriteRoutesParity(t *testing.T) {
	created := []string{"data.created_at", "data.updated_at"}
	write(t, route(t, "POST", "/api/v2/admin/plans", "plan.admin.plans.post", admins((*handler.AdminHandler).CreatePlan)), plans, []packagecompat.Case{
		{Name: "create", Path: "/api/v2/admin/plans", Principal: admin, Mask: created, Body: []byte(`{"group_id":2,"transfer_enable":50,"speed_limit":100,` +
			`"device_limit":3,"name":"New","content":"c","show":1,"sort":5,"renew":1,"reset_price":10,"reset_traffic_method":2,"capacity_limit":7,` +
			`"month_price":1,"quarter_price":2,"half_year_price":3,"year_price":4,"two_year_price":5,"three_year_price":6,"onetime_price":7}`)},
		{Name: "create with defaults", Path: "/api/v2/admin/plans", Principal: admin, Mask: created, Body: []byte(`{"name":"Bare"}`)},
		// The community edition's subscription templates carry no price.
		{Name: "a subscription template without prices", Path: "/api/v2/admin/plans", Principal: admin, Mask: created,
			Body: []byte(`{"name":"Template","group_id":2,"transfer_enable":100,"speed_limit":50,"device_limit":3}`)},
		{Name: "renew zero keeps the column default", Path: "/api/v2/admin/plans", Principal: admin, Mask: created, Body: []byte(`{"name":"No renew","renew":0}`)},
		{Name: "create with an explicit id", Path: "/api/v2/admin/plans", Principal: admin, Mask: created, Body: []byte(`{"id":50,"name":"Fixed"}`)},
		{Name: "create with a taken id", Path: "/api/v2/admin/plans", Principal: admin, Body: []byte(`{"id":2,"name":"Taken"}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/plans", Principal: admin, Body: []byte(`{"transfer_enable":"a lot"}`)},
		{Name: "malformed body", Path: "/api/v2/admin/plans", Principal: admin, Body: []byte(`{"name":`)},
		{Name: "no body", Path: "/api/v2/admin/plans", Principal: admin},
	})
	write(t, route(t, "PUT", "/api/v2/admin/plans/:id", "plan.admin.plans.id.put", admins((*handler.AdminHandler).UpdatePlan)), plans, []packagecompat.Case{
		{Name: "update keeps the creation time", Path: "/api/v2/admin/plans/2", Principal: admin, Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"Pro 2","group_id":3,"transfer_enable":200,"show":0,"sort":9,"month_price":3500,"created_at":"2020-01-01T00:00:00Z"}`)},
		{Name: "a subscription template update without prices", Path: "/api/v2/admin/plans/2", Principal: admin, Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"Template 2","group_id":3,"transfer_enable":200,"speed_limit":0,"device_limit":0}`)},
		{Name: "an empty body clears every field", Path: "/api/v2/admin/plans/1", Principal: admin, Mask: []string{"data.updated_at"}, Body: []byte(`{}`)},
		{Name: "the body cannot move another plan", Path: "/api/v2/admin/plans/3", Principal: admin, Mask: []string{"data.updated_at"}, Body: []byte(`{"id":2,"name":"Moved"}`)},
		{Name: "unknown plan", Path: "/api/v2/admin/plans/99", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "plan zero", Path: "/api/v2/admin/plans/0", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "id that is not a number", Path: "/api/v2/admin/plans/x", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/plans/2", Principal: admin, Body: []byte(`{"sort":"first"}`)},
		{Name: "no body", Path: "/api/v2/admin/plans/2", Principal: admin},
	})
	write(t, route(t, "DELETE", "/api/v2/admin/plans/:id", "plan.admin.plans.id.delete", admins((*handler.AdminHandler).DeletePlan)), plans, []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/plans/3", Principal: admin},
		{Name: "unknown plan", Path: "/api/v2/admin/plans/99", Principal: admin},
		{Name: "plan zero", Path: "/api/v2/admin/plans/0", Principal: admin},
		{Name: "id that is not a number", Path: "/api/v2/admin/plans/x", Principal: admin},
		{Name: "id that is a condition", Path: "/api/v2/admin/plans/0%20OR%201=1", Principal: admin},
	})
}

// A plan a subscriber holds: SQLite does not enforce the kernel's foreign
// key from v2_user, PostgreSQL refuses the delete; both sides alike.
func TestDeletePlanInUseParity(t *testing.T) {
	write(t, route(t, "DELETE", "/api/v2/admin/plans/:id", "plan.admin.plans.id.delete", admins((*handler.AdminHandler).DeletePlan)), plans, []packagecompat.Case{
		{Name: "delete a plan in use", Path: "/api/v2/admin/plans/1", Principal: admin},
	})
}

func TestAssignPlanParity(t *testing.T) {
	assign := route(t, "POST", "/api/v2/admin/plans/:id/assign", native.AssignRouteID, admins((*handler.AdminHandler).AssignPlanToUser))
	requestID := func(id string) map[string]string { return map[string]string{"X-Request-ID": id} }
	write(t, assign, subscribers(false), []packagecompat.Case{
		{Name: "assign with an expiry", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-1"),
			Body: []byte(`{"user_id":3,"expire_at":1893456000}`)},
		{Name: "assign keeps the expiry, limits and subscription groups", Path: "/api/v2/admin/plans/1/assign", Principal: admin, RequestHeaders: requestID("r-2"),
			Body: []byte(`{"user_id":2}`)},
		{Name: "a null expiry is no expiry", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-3"),
			Body: []byte(`{"user_id":2,"expire_at":null}`)},
		{Name: "expiry zero", Path: "/api/v2/admin/plans/4/assign", Principal: admin, RequestHeaders: requestID("r-4"),
			Body: []byte(`{"user_id":3,"expire_at":0}`)},
		{Name: "a banned subscriber", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-5"),
			Body: []byte(`{"user_id":4,"expire_at":1893456000}`)},
		{Name: "a hidden plan", Path: "/api/v2/admin/plans/3/assign", Principal: admin, RequestHeaders: requestID("r-6"),
			Body: []byte(`{"user_id":1}`)},
		{Name: "a retried request applies once", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-7"),
			Warmup: [][]byte{[]byte(`{"user_id":3,"expire_at":1893456000}`)}, Body: []byte(`{"user_id":3,"expire_at":1893456000}`)},
		{Name: "the idempotency key wins over the request id", Path: "/api/v2/admin/plans/2/assign", Principal: admin,
			RequestHeaders: map[string]string{"Idempotency-Key": "assign-3", "X-Request-ID": "r-8"},
			Warmup:         [][]byte{[]byte(`{"user_id":3}`)}, Body: []byte(`{"user_id":3}`)},
		{Name: "a key reused for another expiry is another grant", Path: "/api/v2/admin/plans/2/assign", Principal: admin,
			RequestHeaders: map[string]string{"Idempotency-Key": "assign-3"},
			Warmup:         [][]byte{[]byte(`{"user_id":3}`)}, Body: []byte(`{"user_id":3,"expire_at":1893456000}`)},
		{Name: "unknown subscriber", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-9"), Body: []byte(`{"user_id":99}`)},
		{Name: "subscriber id beyond the contract", Path: "/api/v2/admin/plans/2/assign", Principal: admin, RequestHeaders: requestID("r-10"),
			Body: []byte(`{"user_id":4294967296}`)},
		{Name: "unknown plan", Path: "/api/v2/admin/plans/99/assign", Principal: admin, RequestHeaders: requestID("r-11"), Body: []byte(`{"user_id":2}`)},
		{Name: "unknown plan and subscriber", Path: "/api/v2/admin/plans/99/assign", Principal: admin, Body: []byte(`{"user_id":99}`)},
		{Name: "plan zero", Path: "/api/v2/admin/plans/0/assign", Principal: admin, Body: []byte(`{"user_id":2}`)},
		{Name: "plan id that is not a number", Path: "/api/v2/admin/plans/x/assign", Principal: admin, Body: []byte(`{"user_id":2}`)},
		{Name: "plan id that is a condition", Path: "/api/v2/admin/plans/0%20OR%201=1/assign", Principal: admin, Body: []byte(`{"user_id":2}`)},
		{Name: "no subscriber", Path: "/api/v2/admin/plans/2/assign", Principal: admin, Body: []byte(`{}`)},
		{Name: "subscriber zero", Path: "/api/v2/admin/plans/2/assign", Principal: admin, Body: []byte(`{"user_id":0}`)},
		{Name: "negative subscriber", Path: "/api/v2/admin/plans/2/assign", Principal: admin, Body: []byte(`{"user_id":-1}`)},
		{Name: "expiry that is not a number", Path: "/api/v2/admin/plans/2/assign", Principal: admin, Body: []byte(`{"user_id":2,"expire_at":"soon"}`)},
		{Name: "no body", Path: "/api/v2/admin/plans/2/assign", Principal: admin},
	})
	// Without an idempotency key or request id every request is a grant of
	// its own, as in v2; the ids are random on both sides.
	write(t, assign, subscribers(true), []packagecompat.Case{
		{Name: "two requests are two grants", Path: "/api/v2/admin/plans/2/assign", Principal: admin,
			Warmup: [][]byte{[]byte(`{"user_id":2,"expire_at":1893456000}`)}, Body: []byte(`{"user_id":2,"expire_at":1893456000}`)},
	})
}
