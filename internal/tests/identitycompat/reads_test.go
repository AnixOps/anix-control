package identitycompat

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// later is an expiry ten and a half days ahead, far from a day boundary, so
// both sides count the same days remaining.
var later = time.Now().Add(10*24*time.Hour + 12*time.Hour).Unix()

// seededAt fixes the rows' times, which each side's seed would otherwise
// take from its own clock.
var seededAt = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// withSubscription gives the member a plan, traffic and an expiry, the
// banned user some traffic, and every row fixed times.
func withSubscription(t testing.TB, db *gorm.DB) {
	withPlan(t, db)
	require.NoError(t, db.Model(&model.Plan{}).Where("id = ?", 7).UpdateColumns(map[string]any{"created_at": seededAt, "updated_at": seededAt}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).UpdateColumns(map[string]any{
		"plan_id": 7, "group_id": 4, "transfer_enable": 1000, "u": 100, "d": 300, "expired_at": later,
		"balance": 250, "remark_content": "vip",
	}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 3).UpdateColumns(map[string]any{"u": 5, "transfer_enable": 20}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("1 = 1").UpdateColumns(map[string]any{"created_at": seededAt, "updated_at": seededAt}).Error)
}

// withGrants restricts the ticket plugin to an access group the member is
// in.
func withGrants(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.AutoMigrate(&model.AccessGroup{}, &model.AccessGroupUser{}, &model.ResourceGrant{}))
	require.NoError(t, db.Create(&model.AccessGroup{ID: 1, ScopeID: "default", Name: "support", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.AccessGroupUser{GroupID: 1, UserID: 2}).Error)
	require.NoError(t, db.Create(&model.ResourceGrant{GroupID: 1, ResourceType: "plugin_api", ResourceID: "ticket", Permissions: `["ticket.view"]`}).Error)
}

func userRoute(pattern, routeID string, legacy func(*handler.UserHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: "GET", Pattern: pattern, RouteID: routeID, Models: Models,
		Legacy: func(c *gin.Context) { legacy(handler.NewUserHandler(), c) },
		Native: nativeRoute(routeID),
	}
}

func runReads(t *testing.T, route packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seedUsers(nil, nil)
		}
		packagecompat.RunRead(t, route, c)
	}
}

func TestProfileParity(t *testing.T) {
	route := userRoute("/api/v2/user/profile", native.ProfileRouteID, (*handler.UserHandler).GetProfile)
	path := "/api/v2/user/profile"
	runReads(t, route, []packagecompat.Case{
		{Name: "member", Path: path, Principal: member},
		{Name: "administrator is unrestricted", Path: path, Principal: admin},
		{Name: "member with plugin grants", Path: path, Principal: member, Seed: seedUsers(nil, withGrants)},
		{Name: "administrator with a restricted plugin", Path: path, Principal: admin, Seed: seedUsers(nil, withGrants)},
		{Name: "banned member", Path: path, Principal: pluginhostsdk.Principal{ActorID: 3}},
		{Name: "unknown user", Path: path, Principal: nobody},
	})
}

func TestDashboardParity(t *testing.T) {
	route := userRoute("/api/v2/user/dashboard", native.DashboardRouteID, (*handler.UserHandler).GetDashboard)
	path := "/api/v2/user/dashboard"
	computed := []string{"data.subscription.cached_at"}
	runReads(t, route, []packagecompat.Case{
		{Name: "member with a plan", Path: path, Principal: member, Mask: computed, Seed: seedUsers(nil, withSubscription)},
		{Name: "member without a plan", Path: path, Principal: member, Mask: computed},
		{Name: "expired", Path: path, Principal: pluginhostsdk.Principal{ActorID: 4}, Mask: computed},
		{Name: "banned member with traffic", Path: path, Principal: pluginhostsdk.Principal{ActorID: 3}, Mask: computed, Seed: seedUsers(nil, withSubscription)},
		{Name: "unknown user", Path: path, Principal: nobody},
	})
}

func TestAdminUserDetailParity(t *testing.T) {
	route := adminRoute("GET", "/api/v2/admin/users/:id", native.AdminUserRouteID, (*handler.AdminHandler).GetUser)
	seed := seedUsers(nil, withSubscription)
	runReads(t, route, []packagecompat.Case{
		{Name: "member with a plan", Path: "/api/v2/admin/users/2", Principal: admin, Seed: seed},
		{Name: "banned without a plan", Path: "/api/v2/admin/users/3", Principal: admin, Seed: seed},
		{Name: "administrator", Path: "/api/v2/admin/users/1", Principal: admin, Seed: seed},
		{Name: "mfa member", Path: "/api/v2/admin/users/5", Principal: admin, Seed: seed},
		{Name: "unknown", Path: "/api/v2/admin/users/99", Principal: admin, Seed: seed},
		{Name: "id zero", Path: "/api/v2/admin/users/0", Principal: admin, Seed: seed},
		{Name: "invalid id", Path: "/api/v2/admin/users/abc", Principal: admin, Seed: seed},
		{Name: "id beyond 32 bits", Path: "/api/v2/admin/users/4294967296", Principal: admin, Seed: seed},
	})
}
