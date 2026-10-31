// Package proxynodecompat proves the proxy-node package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (times the handlers take from their clock masked) and,
// where a route writes, the same resulting rows.
//
// The node logs read the adopted v2_node_log table. The node statistics, and whether
// a node exists, come from the kernel view kapi_node_status_v1 over v2_node
// (packagestore.EnsureKernelAPIViews), so every node case also proves the
// view shows what the handlers count.
package proxynodecompat

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/proxy-node/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.Node{}, &model.NodeProtocol{}, &model.NodeLog{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// nodes builds the legacy handler per request: its service takes the
// kernel's database when it is built, and the harness sets up a database
// per case.
func nodes(method func(*handler.NodeHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewNodeHandler(), c) }
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_node", "v2_node_log"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

func seedViews(t testing.TB, db *gorm.DB) {
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

// seedNodes writes nodes that checked in recently, long ago and never, in
// every status, with traffic, and the view over them. A pending node that
// checked in recently counts both online and pending, as in the kernel.
func seedNodes(t testing.TB, db *gorm.DB) {
	seedViews(t, db)
	now := time.Now().Unix()
	recent, old := now-60, now-3600
	rows := []model.Node{
		{ID: 1, Name: "online", Host: "a.example.test", APIKey: "k1", APIKeyHash: "h1", Secret: "s1", Status: model.NodeStatusOnline, LastCheckAt: &recent, TotalUpload: 1 << 40, TotalDownload: 3 << 40},
		{ID: 2, Name: "stale", Host: "b.example.test", APIKey: "k2", APIKeyHash: "h2", Secret: "s2", Status: model.NodeStatusOnline, LastCheckAt: &old, TotalUpload: 500, TotalDownload: 700},
		{ID: 3, Name: "pending", Host: "c.example.test", APIKey: "k3", APIKeyHash: "h3", Secret: "s3", Status: model.NodeStatusPending},
		{ID: 4, Name: "pending-online", Host: "d.example.test", APIKey: "k4", APIKeyHash: "h4", Secret: "s4", Status: model.NodeStatusPending, LastCheckAt: &recent, TotalUpload: 1},
		{ID: 5, Name: "disabled", Host: "e.example.test", APIKey: "k5", APIKeyHash: "h5", Secret: "s5", Status: model.NodeStatusDisabled, LastCheckAt: &old},
	}
	require.NoError(t, db.Create(&rows).Error)
	syncSequences(t, db)
}

func TestNodeStatsRouteParity(t *testing.T) {
	path := "/api/v2/admin/nodes/stats"
	read(t, route("GET", path, "proxy.admin.nodes.stats.get", nodes((*handler.NodeHandler).GetNodeStats)), []packagecompat.Case{
		{Name: "every status", Path: path, Seed: seedNodes},
		{Name: "no nodes", Path: path, Seed: seedViews},
	})
}

// seedNodeLogs writes the nodes and logs of two nodes: levels as the kernel
// stores them, structured fields that are an object, an array, a number,
// not JSON or absent, and logs with and without the time the node logged
// them, which orders them before the time they were stored.
func seedNodeLogs(t testing.TB, db *gorm.DB) {
	seedNodes(t, db)
	at := func(minutes int) *time.Time { v := seeded.Add(time.Duration(minutes) * time.Minute); return &v }
	logs := []model.NodeLog{
		{ID: 1, NodeID: 1, Level: "info", Source: "xray", Message: "started", TraceID: "t-1", FieldsJSON: `{"pid":42,"version":"1.8.4"}`, LoggedAt: at(1), CreatedAt: seeded.Add(10 * time.Minute)},
		{ID: 2, NodeID: 1, Level: "warning", Source: "xray", Message: "slow upstream <edge>", TraceID: "t-2", FieldsJSON: `[1,2,3]`, LoggedAt: at(5), CreatedAt: seeded.Add(11 * time.Minute)},
		{ID: 3, NodeID: 1, Level: "error", Source: "agent", Message: "config reload failed", TraceID: "t-3", FieldsJSON: `12345678901234567890`, CreatedAt: seeded.Add(3 * time.Minute)},
		{ID: 4, NodeID: 1, Level: "debug", Source: "agent", Message: "heartbeat", FieldsJSON: `not json`, LoggedAt: at(5), CreatedAt: seeded.Add(12 * time.Minute)},
		{ID: 5, NodeID: 1, Level: "info", Source: "wireguard", Message: "peer added", TraceID: "agent-trace", LoggedAt: at(7), CreatedAt: seeded.Add(13 * time.Minute)},
		{ID: 6, NodeID: 2, Level: "error", Source: "xray", Message: "other node", LoggedAt: at(8), CreatedAt: seeded.Add(14 * time.Minute)},
	}
	for i := range logs {
		logs[i].UpdatedAt = logs[i].CreatedAt
	}
	require.NoError(t, db.Create(&logs).Error)
	syncSequences(t, db)
}

func TestNodeLogsRouteParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/logs"
	path := func(id, query string) string { return "/api/v2/admin/nodes/" + id + "/logs" + query }
	read(t, route("GET", pattern, "proxy.admin.nodes.id.logs.get", nodes((*handler.NodeHandler).GetNodeLogs)), []packagecompat.Case{
		{Name: "newest first", Path: path("1", ""), Seed: seedNodeLogs},
		{Name: "level as reported", Path: path("1", "?level=WARN"), Seed: seedNodeLogs},
		{Name: "level fatal is error", Path: path("1", "?level=%20fatal%20"), Seed: seedNodeLogs},
		{Name: "unknown level", Path: path("1", "?level=trace"), Seed: seedNodeLogs},
		{Name: "source", Path: path("1", "?source=%20agent%20"), Seed: seedNodeLogs},
		{Name: "search in message, source and trace", Path: path("1", "?search=agent"), Seed: seedNodeLogs},
		{Name: "search and level", Path: path("1", "?search=e&level=info"), Seed: seedNodeLogs},
		{Name: "second page", Path: path("1", "?page=2&page_size=2"), Seed: seedNodeLogs},
		{Name: "page past the end", Path: path("1", "?page=9&page_size=2"), Seed: seedNodeLogs},
		{Name: "page size above the limit", Path: path("1", "?page_size=1000"), Seed: seedNodeLogs},
		{Name: "page and page size below one", Path: path("1", "?page=-1&page_size=0"), Seed: seedNodeLogs},
		{Name: "page that is not a number", Path: path("1", "?page=x&page_size=y"), Seed: seedNodeLogs},
		{Name: "node without logs", Path: path("3", ""), Seed: seedNodeLogs},
		{Name: "missing node", Path: path("99", ""), Seed: seedNodeLogs},
		{Name: "id that is not a number", Path: path("abc", ""), Seed: seedNodeLogs},
		{Name: "no nodes", Path: path("1", ""), Seed: seedViews},
	})
}
