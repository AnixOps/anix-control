// Package machinetelemetrycompat proves the machine-telemetry package's
// native routes answer exactly as the kernel's legacy handlers, on SQLite and
// PostgreSQL: the hourly traffic series and the user traffic ranking. The
// legacy handlers read v2_server_log and v2_user; the native ones read the
// kernel views kapi_traffic_log_v1 and kapi_user_directory_v1 over them, so
// every case also proves the views show what the handlers read.
//
// The answers depend on the current hour, which each side takes from its own
// clock: a run that crosses the top of an hour may see one case differ.
package machinetelemetrycompat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/machine-telemetry/native"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.User{}, &model.TrafficLog{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// stats builds the legacy handler per request on a fresh StatsService,
// which otherwise keeps the database it was first built with.
func stats(method func(*handler.AdminHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		service.ResetStatsServiceForTest()
		method(handler.NewAdminHandler(), c)
	}
}

// seedViews creates the kernel views; the harness has migrated the tables.
func seedViews(t testing.TB, db *gorm.DB) {
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

func seedUsers(t testing.TB, db *gorm.DB) {
	users := make([]model.User, 0, 6)
	for id, name := range map[uint]string{1: "alice", 2: "bob", 3: "carol", 4: "dave", 5: "erin", 6: "frank"} {
		users = append(users, model.User{ID: id, Email: name + "@example.test", Token: "token-" + name, UUID: "uuid-" + name})
	}
	require.NoError(t, db.Create(&users).Error)
}

// seedTraffic writes users and traffic reports relative to the current local
// hour: in it, in earlier hours of the day, a day ago, weeks ago and beyond
// the 30-day window, with rates of 1, 1.5, 0 and -2 (both counted as 1) and
// 2, negative bytes (counted as 0), a sum beyond int64 (clamped), and reports of a user
// without a v2_user row (id 9). Each user's total differs in every window, so
// the ranking, which has no tie-break, is well defined.
func seedTraffic(t testing.TB, db *gorm.DB) {
	seedViews(t, db)
	seedUsers(t, db)
	now := time.Now()
	current := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	hour := int64(3600)
	logs := []model.TrafficLog{
		{UserID: 1, ServerID: 1, ServerType: "vless", U: 1000, D: 5000, Rate: 1, LogAt: current + 5},
		{UserID: 1, ServerID: 2, ServerType: "vless", U: 300, D: 700, Rate: 1.5, LogAt: current - hour + 10},
		{UserID: 2, ServerID: 1, ServerType: "vmess", U: 2500, D: 2500, Rate: 0, LogAt: current - 2*hour},
		{UserID: 2, ServerID: 1, ServerType: "vmess", U: -400, D: 100, Rate: 1, LogAt: current - 2*hour + 59},
		{UserID: 9, ServerID: 3, ServerType: "trojan", U: 10, D: 20, Rate: 2, LogAt: current + 30},
		{UserID: 3, ServerID: 3, ServerType: "trojan", U: 40000, D: 2000, Rate: 1, LogAt: current - 30*hour},
		{UserID: 5, ServerID: 4, ServerType: "vless", U: 9_000_000_000_000_000_000, D: 9_000_000_000_000_000_000, Rate: 1, LogAt: current - 400*hour},
		{UserID: 6, ServerID: 4, ServerType: "vless", U: 777, D: 0, Rate: 1, LogAt: current - 800*hour},
	}
	for i := range logs {
		logs[i].ID = uint(i + 1)
		logs[i].CreatedAt = time.Unix(logs[i].LogAt, 0).UTC()
	}
	require.NoError(t, db.Create(&logs).Error)
	// Create stores the column default (1) for a zero rate; store the zero
	// and a negative rate, which count as 1, as legacy imports may hold them.
	require.NoError(t, db.Model(&model.TrafficLog{}).Where("id = ?", 3).UpdateColumn("rate", 0).Error)
	require.NoError(t, db.Model(&model.TrafficLog{}).Where("id = ?", 4).UpdateColumn("rate", -2).Error)
}

// seedUsersOnly writes users and no traffic.
func seedUsersOnly(t testing.TB, db *gorm.DB) {
	seedViews(t, db)
	seedUsers(t, db)
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func TestHourlyTrafficRouteParity(t *testing.T) {
	path := "/api/v2/admin/traffic/hourly"
	cases := []packagecompat.Case{
		{Name: "last 24 hours", Path: path, Seed: seedTraffic},
		{Name: "no traffic", Path: path, Seed: seedUsersOnly},
		{Name: "no traffic for the user", Path: path + "?user_id=4", Seed: seedTraffic},
	}
	for _, query := range []string{
		"hours=1", "hours=3", "hours=48", "hours=720", "hours=721", "hours=0", "hours=-5", "hours=abc",
		"user_id=1", "user_id=9", "user_id=3&hours=48", "user_id=abc", "user_id=-1", "user_id=4294967296",
		"hours=2&hours=48", "user_id=1&user_id=2",
	} {
		cases = append(cases, packagecompat.Case{Name: query, Path: path + "?" + query, Seed: seedTraffic})
	}
	read(t, route("GET", path, "telemetry.admin.traffic.hourly.get", stats((*handler.AdminHandler).GetHourlyTraffic)), cases)
}

func TestUserTrafficRankingRouteParity(t *testing.T) {
	path := "/api/v2/admin/traffic/user-ranking"
	cases := []packagecompat.Case{
		{Name: "last 24 hours", Path: path, Seed: seedTraffic},
		{Name: "no traffic", Path: path, Seed: seedUsersOnly},
		{Name: "no traffic, every user", Path: path + "?include_zero_users=true", Seed: seedUsersOnly},
		{Name: "no users", Path: path + "?include_zero_users=true", Seed: seedViews},
	}
	for _, query := range []string{
		"hours=1", "hours=48", "hours=720", "hours=5000", "hours=0", "hours=x",
		"limit=1", "limit=2", "limit=0", "limit=-3", "limit=500", "limit=x",
		"include_zero_users=true", "include_zero_users=1", "include_zero_users=yes", "include_zero_users=TRUE",
		"include_zero_users=true&limit=2", "include_zero_users=1&hours=1", "include_zero_users=true&hours=720&limit=5000",
	} {
		cases = append(cases, packagecompat.Case{Name: query, Path: path + "?" + query, Seed: seedTraffic})
	}
	read(t, route("GET", path, "telemetry.admin.traffic.user_ranking.get", stats((*handler.AdminHandler).GetUserTrafficRanking)), cases)
}

// The seed puts traffic in the windows, so the cases compare data and not
// only empty answers: in the last 24 hours alice (1) leads bob (2) and the
// user without a row (9), whose e-mail is empty.
func TestSeedRanksTrafficInTheWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "seed.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TrafficLog{}))
	seedTraffic(t, db)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
	response, err := service.UserTrafficRanking(context.Background(), pluginhostsdk.NativeRequest{})
	require.NoError(t, err)
	var answer struct {
		Data struct {
			List []native.UserTrafficRank `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &answer))
	require.Equal(t, []native.UserTrafficRank{
		{UserID: 1, Email: "alice@example.test", Traffic: 7500},
		{UserID: 2, Email: "bob@example.test", Traffic: 5100},
		{UserID: 9, Email: "", Traffic: 60},
	}, answer.Data.List)
}
