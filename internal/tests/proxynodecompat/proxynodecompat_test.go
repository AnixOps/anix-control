// Package proxynodecompat proves the proxy-node package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (times the handlers take from their clock masked) and,
// where a route writes, the same resulting rows.
//
// The load balancer routes and the node logs read the adopted
// v2_load_balancer and v2_node_log tables. The node statistics, and whether
// a node exists, come from the kernel view kapi_node_status_v1 over v2_node
// (packagestore.EnsureKernelAPIViews), so every node case also proves the
// view shows what the handlers count.
package proxynodecompat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.Node{}, &model.NodeProtocol{}, &model.NodeLog{}, &model.LoadBalancer{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// loadBalancers and nodes build the legacy handler per request: their
// services take the kernel's database when they are built, and the harness
// sets up a database per case.
func loadBalancers(method func(*handler.LoadBalancerHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewLoadBalancerHandler(), c) }
}

func nodes(method func(*handler.NodeHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewNodeHandler(), c) }
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_node", "v2_node_log", "v2_load_balancer"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

func seedViews(t testing.TB, db *gorm.DB) {
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

// seedLoadBalancers writes load balancers in two groups with weights as an
// object, as text that is not JSON, as an array and none, and both flags
// off on one (Create would write their defaults instead).
func seedLoadBalancers(t testing.TB, db *gorm.DB) {
	rows := []model.LoadBalancer{
		{ID: 1, Name: "edge-hk", GroupID: 1, Strategy: "round-robin", HealthCheck: true, CheckInterval: 60, CheckTimeout: 10, NodeWeights: `{"1":3,"2":1}`, Enabled: true},
		{ID: 2, Name: "edge-jp", GroupID: 1, Strategy: "weight", HealthCheck: true, CheckInterval: 30, CheckTimeout: 5, NodeWeights: "not json", Enabled: true},
		{ID: 3, Name: "edge-us", GroupID: 2, Strategy: "latency", HealthCheck: true, CheckInterval: 120, CheckTimeout: 20, NodeWeights: `[1,2]`, Enabled: true},
		{ID: 4, Name: "edge-eu", GroupID: 3, Strategy: "random", HealthCheck: true, CheckInterval: 60, CheckTimeout: 10, Enabled: true},
	}
	for i := range rows {
		rows[i].CreatedAt = seeded.Add(time.Duration(i) * time.Hour)
		rows[i].UpdatedAt = seeded.Add(time.Duration(i)*time.Hour + time.Minute)
	}
	require.NoError(t, db.Create(&rows).Error)
	require.NoError(t, db.Model(&model.LoadBalancer{}).Where("id = ?", 4).UpdateColumns(map[string]any{"health_check": false, "enabled": false}).Error)
	syncSequences(t, db)
}

// loadBalancerRows is v2_load_balancer after a request, without the times
// the handlers take from their clock.
func loadBalancerRows(t testing.TB, db *gorm.DB) any {
	var rows []struct {
		ID            uint
		Name          string
		GroupID       uint
		Strategy      string
		HealthCheck   bool
		CheckInterval int
		CheckTimeout  int
		NodeWeights   string
		Enabled       bool
		CreatedAt     *time.Time
	}
	require.NoError(t, db.Model(&model.LoadBalancer{}).Order("id").Find(&rows).Error)
	return rows
}

// loadBalancerRowsWithoutCreation also leaves out the creation time, which a
// created row takes from the clock.
func loadBalancerRowsWithoutCreation(t testing.TB, db *gorm.DB) any {
	var rows []struct {
		ID            uint
		Name          string
		GroupID       uint
		Strategy      string
		HealthCheck   bool
		CheckInterval int
		CheckTimeout  int
		NodeWeights   string
		Enabled       bool
	}
	require.NoError(t, db.Model(&model.LoadBalancer{}).Order("id").Find(&rows).Error)
	return rows
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, snapshot func(testing.TB, *gorm.DB) any, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		c.Snapshot = snapshot
		packagecompat.RunWrite(t, r, c)
	}
}

func TestLoadBalancerListRouteParity(t *testing.T) {
	path := "/api/v2/admin/loadbalancers"
	read(t, route("GET", path, "proxy.loadbalancer.get", loadBalancers((*handler.LoadBalancerHandler).ListLoadBalancers)), []packagecompat.Case{
		{Name: "newest first", Path: path, Seed: seedLoadBalancers},
		{Name: "one group", Path: path + "?group_id=1", Seed: seedLoadBalancers},
		{Name: "a group without load balancers", Path: path + "?group_id=9", Seed: seedLoadBalancers},
		{Name: "a negative group is every group", Path: path + "?group_id=-4", Seed: seedLoadBalancers},
		{Name: "a group that is not a number is every group", Path: path + "?group_id=abc", Seed: seedLoadBalancers},
		{Name: "no load balancers", Path: path},
	})
}

func TestLoadBalancerDetailRouteParity(t *testing.T) {
	pattern := "/api/v2/admin/loadbalancers/:id"
	path := func(id string) string { return "/api/v2/admin/loadbalancers/" + id }
	read(t, route("GET", pattern, "proxy.loadbalancer.id.get", loadBalancers((*handler.LoadBalancerHandler).GetLoadBalancer)), []packagecompat.Case{
		{Name: "weights as an object", Path: path("1"), Seed: seedLoadBalancers},
		{Name: "weights that are not JSON", Path: path("2"), Seed: seedLoadBalancers},
		{Name: "weights as an array", Path: path("3"), Seed: seedLoadBalancers},
		{Name: "no weights, both flags off", Path: path("4"), Seed: seedLoadBalancers},
		{Name: "missing", Path: path("99"), Seed: seedLoadBalancers},
		{Name: "id that is not a number", Path: path("abc"), Seed: seedLoadBalancers},
		{Name: "id beyond 32 bits", Path: path("4294967297"), Seed: seedLoadBalancers},
	})
}

func TestLoadBalancerCreateRouteParity(t *testing.T) {
	path := "/api/v2/admin/loadbalancers"
	created := []string{"data.created_at", "data.updated_at"}
	write(t, route("POST", path, "proxy.loadbalancer.post", loadBalancers((*handler.LoadBalancerHandler).CreateLoadBalancer)), loadBalancerRowsWithoutCreation, []packagecompat.Case{
		{Name: "full body with weights", Path: path, Seed: seedLoadBalancers, Mask: created,
			Body: []byte(`{"name":"edge-sg","group_id":4,"strategy":"least-load","health_check":true,"check_interval":15,"check_timeout":3,"enabled":true,"weights":{"7": 2, "8":1}}`)},
		{Name: "weights as text", Path: path, Seed: seedLoadBalancers, Mask: created,
			Body: []byte(`{"name":"edge-sg","node_weights":"{\"7\":2}"}`)},
		{Name: "text weights win over weights", Path: path, Seed: seedLoadBalancers, Mask: created,
			Body: []byte(`{"name":"edge-sg","weights":{"1":1},"node_weights":"{\"2\":2}"}`)},
		{Name: "null weights", Path: path, Seed: seedLoadBalancers, Mask: created,
			Body: []byte(`{"name":"edge-sg","weights":null}`)},
		{Name: "flags off are written as their defaults", Path: path, Seed: seedLoadBalancers, Mask: created,
			Body: []byte(`{"name":"edge-sg","health_check":false,"enabled":false}`)},
		{Name: "empty object", Path: path, Mask: created, Body: []byte(`{}`)},
		{Name: "unknown strategy", Path: path, Seed: seedLoadBalancers, Body: []byte(`{"name":"x","strategy":"fastest"}`)},
		{Name: "name too long", Path: path, Seed: seedLoadBalancers, Body: []byte(fmt.Sprintf(`{"name":"%0256d"}`, 0))},
		{Name: "negative interval", Path: path, Seed: seedLoadBalancers, Body: []byte(`{"name":"x","check_interval":-5}`)},
		{Name: "negative group", Path: path, Seed: seedLoadBalancers, Body: []byte(`{"name":"x","group_id":-1}`)},
		{Name: "not JSON", Path: path, Seed: seedLoadBalancers, Body: []byte(`{"name":`)},
		{Name: "no body", Path: path, Seed: seedLoadBalancers},
	})
}

func TestLoadBalancerUpdateRouteParity(t *testing.T) {
	pattern := "/api/v2/admin/loadbalancers/:id"
	path := func(id string) string { return "/api/v2/admin/loadbalancers/" + id }
	updated := []string{"data.updated_at"}
	write(t, route("PUT", pattern, "proxy.loadbalancer.id.put", loadBalancers((*handler.LoadBalancerHandler).UpdateLoadBalancer)), loadBalancerRows, []packagecompat.Case{
		{Name: "rename and regroup", Path: path("1"), Seed: seedLoadBalancers, Mask: updated,
			Body: []byte(`{"name":"edge-hk2","group_id":5,"strategy":"weighted-random"}`)},
		{Name: "turn both flags off", Path: path("1"), Seed: seedLoadBalancers, Mask: updated,
			Body: []byte(`{"health_check":false,"enabled":false}`)},
		{Name: "turn both flags on", Path: path("4"), Seed: seedLoadBalancers, Mask: updated,
			Body: []byte(`{"health_check":true,"enabled":true}`)},
		{Name: "replace weights", Path: path("2"), Seed: seedLoadBalancers, Mask: updated,
			Body: []byte(`{"weights":{"3":9}}`)},
		{Name: "zero values keep the stored ones", Path: path("3"), Seed: seedLoadBalancers, Mask: updated,
			Body: []byte(`{"name":"","group_id":0,"check_interval":0,"check_timeout":0,"weights":null}`)},
		{Name: "empty object", Path: path("3"), Seed: seedLoadBalancers, Mask: updated, Body: []byte(`{}`)},
		{Name: "unknown strategy", Path: path("1"), Seed: seedLoadBalancers, Body: []byte(`{"strategy":"fastest"}`)},
		{Name: "not JSON", Path: path("1"), Seed: seedLoadBalancers, Body: []byte(`[`)},
		{Name: "missing", Path: path("99"), Seed: seedLoadBalancers, Body: []byte(`{"name":"x"}`)},
		{Name: "id that is not a number", Path: path("x"), Seed: seedLoadBalancers, Body: []byte(`{"name":"x"}`)},
	})
}

func TestLoadBalancerDeleteRouteParity(t *testing.T) {
	pattern := "/api/v2/admin/loadbalancers/:id"
	path := func(id string) string { return "/api/v2/admin/loadbalancers/" + id }
	write(t, route("DELETE", pattern, "proxy.loadbalancer.id.delete", loadBalancers((*handler.LoadBalancerHandler).DeleteLoadBalancer)), loadBalancerRows, []packagecompat.Case{
		{Name: "existing", Path: path("2"), Seed: seedLoadBalancers},
		{Name: "missing succeeds", Path: path("99"), Seed: seedLoadBalancers},
		{Name: "id that is not a number", Path: path("-1"), Seed: seedLoadBalancers},
	})
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
