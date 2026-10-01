package identitycompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// directoryDay is a past day the directory's users were created on.
var directoryDay = time.Date(2026, 8, 3, 9, 30, 0, 0, time.UTC)

// earlyToday is a creation time on the current UTC day, which the
// statistics count as new today on both sides of a case.
var earlyToday = time.Now().UTC().Truncate(24 * time.Hour).Add(time.Second)

// directoryUser is an extra subscriber of the directory.
func directoryUser(id uint, email string, created time.Time, change func(*model.User)) model.User {
	user := model.User{ID: id, Email: email, Password: "x", UUID: "uuid-" + email, Token: "token-" + email, CreatedAt: created, UpdatedAt: created}
	if change != nil {
		change(&user)
	}
	return user
}

// withDirectory adds subscribers of every kind to seedUsers' five: plans,
// entitlements, expiries (past, future, zero), a banned and expired user,
// staff, e-mails with LIKE wildcards and mixed case, and creation times with
// ties, so ordering and paging have something to prove.
func withDirectory(t testing.TB, db *gorm.DB) {
	withPlan(t, db)
	require.NoError(t, db.Create(&model.Plan{ID: 8, Name: "Lite", GroupID: 5, TransferEnable: 5}).Error)
	plan7, plan8, group4, group5 := uint(7), uint(8), uint(4), uint(5)
	speed, devices, future, zero := int64(10), 2, later, int64(0)
	users := []model.User{
		directoryUser(6, "carol@example.test", directoryDay.Add(4*time.Hour), func(u *model.User) {
			u.PlanID, u.GroupID, u.ExpiredAt = &plan7, &group4, &future
			u.TransferEnable, u.U, u.D, u.SpeedLimit, u.DeviceLimit = 1000, 10, 20, &speed, &devices
			u.Balance, u.CommissionBalance, u.FlowResetTime = 100, 30, 5
		}),
		directoryUser(7, "dave_x@example.test", directoryDay.Add(3*time.Hour), func(u *model.User) {
			u.PlanID, u.GroupID, u.ExpiredAt, u.Banned = &plan8, &group5, &expiredAt, 1
		}),
		directoryUser(8, "Erin@Example.test", directoryDay.Add(3*time.Hour), func(u *model.User) {
			u.PlanID, u.ExpiredAt = &plan7, &future
		}),
		directoryUser(9, "frank%off@example.test", directoryDay.Add(3*time.Hour), func(u *model.User) {
			u.ExpiredAt = &zero
		}),
		directoryUser(10, "grace@staff.test", earlyToday, func(u *model.User) {
			u.IsStaff, u.PlanID = 1, &plan8
		}),
		directoryUser(11, "heidi@example.test", directoryDay.Add(-24*time.Hour), func(u *model.User) {
			u.Banned = 1
		}),
	}
	require.NoError(t, db.Create(&users).Error)
	if db.Name() == "postgres" {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence('v2_user', 'id'), (SELECT MAX(id) FROM v2_user))").Error)
	}
	// seedUsers' five: the administrator first, the member and the banned
	// user created in the same instant.
	for id, created := range map[uint]time.Time{
		1: directoryDay.Add(-48 * time.Hour), 2: directoryDay.Add(time.Hour), 3: directoryDay.Add(time.Hour),
		4: directoryDay.Add(2 * time.Hour), 5: earlyToday,
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", id).UpdateColumns(map[string]any{"created_at": created, "updated_at": created}).Error)
	}
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).UpdateColumns(map[string]any{
		"plan_id": 7, "group_id": 4, "transfer_enable": 1000, "u": 100, "d": 300, "expired_at": later, "balance": 250, "remark_content": "vip",
	}).Error)
}

// withManyUsers adds more subscribers than the largest page holds, some
// created in the same instant.
func withManyUsers(t testing.TB, db *gorm.DB) {
	users := make([]model.User, 0, 120)
	for i := uint(0); i < 120; i++ {
		users = append(users, directoryUser(100+i, fmt.Sprintf("many%03d@example.test", i), directoryDay.Add(time.Duration(i/3)*time.Minute), nil))
	}
	require.NoError(t, db.CreateInBatches(&users, 60).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id <= ?", 5).UpdateColumns(map[string]any{"created_at": earlyToday, "updated_at": earlyToday}).Error)
}

// emptyDirectory seeds no user at all.
func emptyDirectory(testing.TB, *gorm.DB) {}

func runDirectory(t *testing.T, route packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seedUsers(nil, withDirectory)
		}
		c.Principal = admin
		packagecompat.RunRead(t, route, c)
	}
}

func TestAdminUserListParity(t *testing.T) {
	route := adminRoute("GET", "/api/v2/admin/users", native.AdminUsersRouteID, (*handler.AdminHandler).GetUserList)
	list := func(query string) string { return "/api/v2/admin/users" + query }
	runDirectory(t, route, []packagecompat.Case{
		{Name: "first page", Path: list("")},
		{Name: "pages split users created together", Path: list("?page=2&page_size=3")},
		{Name: "last page", Path: list("?page=4&page_size=3")},
		{Name: "beyond the last page", Path: list("?page=9&page_size=3")},
		{Name: "one per page", Path: list("?page=7&page_size=1")},
		{Name: "page size zero is the default", Path: list("?page_size=0")},
		{Name: "page size above the maximum", Path: list("?page_size=1000")},
		{Name: "negative page size", Path: list("?page_size=-5")},
		{Name: "page zero", Path: list("?page=0&page_size=2")},
		{Name: "negative page", Path: list("?page=-3&page_size=2")},
		{Name: "page that does not parse", Path: list("?page=abc&page_size=xyz")},
		{Name: "empty page", Path: list("?page=&page_size=")},
		{Name: "email substring", Path: list("?email=example")},
		{Name: "email exact", Path: list("?email=carol@example.test")},
		{Name: "email no match", Path: list("?email=nobody")},
		{Name: "email underscore is a wildcard", Path: list("?email=e_x")},
		{Name: "email percent is a wildcard", Path: list("?email=%25off")},
		{Name: "email case", Path: list("?email=ERIN")},
		{Name: "email empty", Path: list("?email=")},
		{Name: "plan", Path: list("?plan_id=7")},
		{Name: "other plan", Path: list("?plan_id=8")},
		{Name: "unknown plan", Path: list("?plan_id=99")},
		{Name: "plan that does not parse", Path: list("?plan_id=abc")},
		{Name: "plan beyond 32 bits", Path: list("?plan_id=4294967296")},
		{Name: "plan empty", Path: list("?plan_id=")},
		{Name: "active", Path: list("?status=active")},
		{Name: "expired", Path: list("?status=expired")},
		{Name: "banned", Path: list("?status=banned")},
		{Name: "all", Path: list("?status=all")},
		{Name: "unknown status lists everyone", Path: list("?status=frozen")},
		{Name: "repeated status takes the first", Path: list("?status=banned&status=active")},
		{Name: "active on a plan", Path: list("?status=active&plan_id=7")},
		{Name: "expired e-mail", Path: list("?status=expired&email=example")},
		{Name: "banned on a plan", Path: list("?status=banned&plan_id=8")},
		{Name: "every filter paged", Path: list("?status=active&plan_id=7&email=example&page=2&page_size=1")},
		{Name: "many users, first page", Path: list(""), Seed: seedUsers(nil, withManyUsers)},
		{Name: "many users, page size zero is the default", Path: list("?page_size=0&page=2"), Seed: seedUsers(nil, withManyUsers)},
		{Name: "many users, page size above the maximum", Path: list("?page_size=500"), Seed: seedUsers(nil, withManyUsers)},
		{Name: "many users, second full page", Path: list("?page_size=100&page=2"), Seed: seedUsers(nil, withManyUsers)},
		{Name: "member list", Path: list(""), Seed: seedUsers(nil, withSubscription)},
		{Name: "no users", Path: list(""), Seed: emptyDirectory},
		{Name: "no users beyond the first page", Path: list("?page=3"), Seed: emptyDirectory},
	})
}

func TestAdminUserStatsParity(t *testing.T) {
	route := adminRoute("GET", "/api/v2/admin/users/stats", native.AdminUserStatsRouteID, (*handler.AdminHandler).GetUserStats)
	runDirectory(t, route, []packagecompat.Case{
		{Name: "directory", Path: "/api/v2/admin/users/stats"},
		{Name: "created today", Path: "/api/v2/admin/users/stats", Seed: seedUsers(nil, nil)},
		{Name: "created on another day", Path: "/api/v2/admin/users/stats", Seed: seedUsers(nil, withSubscription)},
		{Name: "no users", Path: "/api/v2/admin/users/stats", Seed: emptyDirectory},
	})
}

// listedKeys are the fields of a listed user.
var listedKeys = []string{
	"balance", "banned", "commission_balance", "created_at", "d", "device_limit", "email", "expired_at", "flowResetTime",
	"group_id", "id", "is_admin", "is_staff", "plan_id", "speed_limit", "transfer_enable", "u",
}

// The list shows no credential: each user has exactly the directory's
// fields, and no subscription token, proxy uuid or other column of the row.
func TestAdminUserListShowsNoCredentials(t *testing.T) {
	_, service, _ := controlWithIdentity(t)
	listed := data(t, answer(t, service, native.AdminUsersRouteID, admin, nil, nil))
	users, ok := listed["list"].([]any)
	require.True(t, ok, "%v", listed)
	require.Len(t, users, 5)
	for _, entry := range users {
		user := entry.(map[string]any)
		var keys []string
		for key := range user {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		require.Equal(t, listedKeys, keys)
	}
}

// search answers the native list for a query string: its total and users.
func search(t *testing.T, service *native.Service, query string) (float64, []map[string]any) {
	t.Helper()
	values, err := url.ParseQuery(query)
	require.NoError(t, err)
	response, err := service.Handlers()[native.AdminUsersRouteID](context.Background(), pluginhostsdk.NativeRequest{
		RouteID: native.AdminUsersRouteID, Principal: admin, Metadata: pluginhostsdk.RequestMetadata{Query: values},
	})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &decoded), "%s", response.Body)
	listed := data(t, decoded)
	var users []map[string]any
	for _, entry := range listed["list"].([]any) {
		users = append(users, entry.(map[string]any))
	}
	return listed["total"].(float64), users
}

func ids(users []map[string]any) []float64 {
	out := []float64{}
	for _, user := range users {
		out = append(out, user["id"].(float64))
	}
	return out
}

// The directory answers identity's accounts, not Control's projection of
// them: here identity renamed and promoted the member and banned the MFA
// member without projecting any of it, and has no account for the banned
// subscriber, who keeps the fields Control holds. Filters, statuses and
// counts all follow identity.
func TestDirectoryTakesTheAccountFromIdentity(t *testing.T) {
	db, service, stores := controlWithIdentity(t)
	ctx := context.Background()
	renamed, yes := "identity@example.test", true
	_, err := stores.Accounts.Update(ctx, 2, account.Changes{Email: &renamed, IsAdmin: &yes})
	require.NoError(t, err)
	_, err = stores.Accounts.Update(ctx, 5, account.Changes{Banned: &yes})
	require.NoError(t, err)
	require.NoError(t, stores.Accounts.Delete(ctx, 3))
	var projected []model.User
	require.NoError(t, db.Order("id").Find(&projected).Error)
	require.Equal(t, "member@example.test", projected[1].Email, "nothing was projected")
	require.Equal(t, 0, projected[4].Banned)

	total, users := search(t, service, "email=identity@")
	require.EqualValues(t, 1, total)
	require.Equal(t, []float64{2}, ids(users))
	require.Equal(t, renamed, users[0]["email"])
	require.EqualValues(t, 1, users[0]["is_admin"])
	require.EqualValues(t, 250, users[0]["balance"], "entitlements are Control's")
	total, _ = search(t, service, "email=member@")
	require.Zero(t, total, "the projected e-mail is not identity's")

	total, users = search(t, service, "status=banned")
	require.EqualValues(t, 2, total)
	require.Equal(t, []float64{5, 3}, ids(users))
	require.Equal(t, "banned@example.test", users[1]["email"], "without an account, Control's fields")
	_, users = search(t, service, "status=active")
	require.Equal(t, []float64{2, 1}, ids(users))
	_, users = search(t, service, "status=expired")
	require.Equal(t, []float64{4}, ids(users))

	stats := data(t, answer(t, service, native.AdminUserStatsRouteID, admin, nil, nil))
	require.Equal(t, map[string]any{
		"total_users": 5.0, "active_users": 2.0, "expired_users": 1.0, "banned_users": 2.0, "today_new_users": 0.0,
	}, stats)
}

// Pages of a search neither repeat nor skip a user, though every user was
// created in the same instant: ties go by id, newest first.
func TestDirectoryPagesAreStable(t *testing.T) {
	_, service, _ := controlWithIdentity(t)
	var seen []float64
	for page := 1; page <= 3; page++ {
		total, users := search(t, service, "page_size=2&page="+strconv.Itoa(page))
		require.EqualValues(t, 5, total)
		seen = append(seen, ids(users)...)
	}
	require.Equal(t, []float64{5, 4, 3, 2, 1}, seen)
}
