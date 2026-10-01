// Package subscriptioncompat proves the subscription package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (times the handlers take from their clock masked) and the
// same resulting rows.
//
// The native side reads the plan, entitlement, membership and node views the
// kernel publishes (packagestore.EnsureKernelAPIViews) and writes only the
// adopted subscription tables. Membership changes reach the real
// KernelSubscriber server (internal/kernelsubscriber) in process over gRPC,
// on the native side's database, so both sides end with the same
// memberships, request ledger and change log.
package subscriptioncompat

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
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
	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	mirror "github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// expiryBase is what seeded expiries are relative to: one value for the
// whole run, so the two sides store the same expiries.
var expiryBase = time.Now().Unix()

func route(method, pattern, routeID string, legacy func(*handler.SubscriptionAdminHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID,
		// The legacy handler is built per request: its service keeps the
		// database it was built with, which the harness sets up per case.
		Legacy: func(c *gin.Context) { legacy(handler.NewSubscriptionAdminHandler(), c) },
		Models: []any{
			&model.Plan{}, &model.User{}, &model.Node{}, &model.NodeProtocol{}, &model.SubscriptionGroup{},
			&model.SubscriptionTemplate{}, &model.UserSubscriptionGroup{}, &model.PlanSubscriptionGroup{},
		},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// subscriptionHost is the identity the kernel serves the subscription host
// as.
var subscriptionHost = packagebridge.HostIdentity{PackageID: "subscription", Version: "4.0.0", Generation: 1}

// groupsOnly authorizes what the subscription package's signed release
// declares of KernelSubscriber: the membership family, for the
// subscription host.
type groupsOnly struct{}

func (groupsOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == subscriptionHost && capability == service.CapabilitySubscriberGroups {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelSubscriber serves the kernel's KernelSubscriber on db in process and
// returns a client for it, as the subscription host gets one over its
// bridge.
func kernelSubscriber(t *testing.T, db *gorm.DB) kernelsubscriberv1.KernelSubscriberClient {
	t.Helper()
	server := &kernelsubscriber.Server{DB: db, Authorizer: groupsOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server.For(subscriptionHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelsubscriberv1.NewKernelSubscriberClient(conn)
}

// membershipRoute is a route that changes subscription group membership:
// the native side calls KernelSubscriber, and both databases have the
// subscriber request ledger and change log.
func membershipRoute(t *testing.T, method, pattern, routeID string, legacy func(*handler.SubscriptionAdminHandler, *gin.Context)) packagecompat.Route {
	r := route(method, pattern, routeID, legacy)
	r.Models = append(r.Models, &model.SubscriberRequest{}, &model.SubscriberChange{})
	r.Native = func(db *gorm.DB) pluginhostsdk.NativeHandler {
		service := &native.Service{
			Open:       func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
			Subscriber: kernelSubscriber(t, db),
		}
		return service.Handlers()[routeID]
	}
	return r
}

func ptr[T any](value T) *T { return &value }

// seed writes users, plans, groups, templates, nodes and their protocols,
// the links between them, then the kernel views. Expiries and node reports
// are relative to now (expiries to expiryBase), so the handlers' clocks see
// them as seeded. Users 1
// and 2 are active subscribers; 3 is banned, 4 expired and 5 out of
// traffic.
func seed(t testing.TB, db *gorm.DB) {
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.Plan{
		{ID: 1, Name: "Basic", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "Pro", CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "Bare", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	user := func(id uint, u, d int64) model.User {
		return model.User{
			ID: id, Email: fmt.Sprintf("user%d@example.test", id), Token: fmt.Sprintf("token-%d", id), UUID: fmt.Sprintf("uuid-%d", id),
			U: u, D: d, IsAdmin: map[bool]int{true: 1}[id == 1], TransferEnable: 1 << 30, CreatedAt: seeded, UpdatedAt: seeded,
		}
	}
	users := []model.User{user(1, 0, 0), user(2, 100, 200), user(3, 1000, 2000), user(4, 7, 9), user(5, 50, 0)}
	users[2].Banned, users[3].ExpiredAt, users[4].TransferEnable = 1, ptr(expiryBase-3600), 50
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{
		{ID: 1, Name: "default", Priority: 0, Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "premium", Description: ptr("fast nodes"), Priority: 10, Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "retired", Priority: 5, Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 4, Name: "empty", Priority: 10, Enable: 1, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	// The kernel model's column default turns a zero into 1 on create.
	require.NoError(t, db.Model(&model.SubscriptionGroup{}).Where("id = ?", 3).UpdateColumn("enable", 0).Error)
	template := func(id, group uint, name string, sort int) model.SubscriptionTemplate {
		return model.SubscriptionTemplate{
			ID: id, GroupID: group, Name: name, Type: "vless", Enable: 1, Sort: sort, Server: name + ".example.test", Port: 443,
			Transport: "tcp", CreatedAt: seeded, UpdatedAt: seeded,
		}
	}
	templates := []model.SubscriptionTemplate{
		template(1, 1, "hk", 2), template(2, 1, "jp", 1), template(3, 2, "us", 0), template(4, 2, "sg", 0), template(5, 3, "old", 0),
	}
	templates[0].ServerName, templates[0].TLS, templates[0].ALPN = ptr("hk.example.test"), 1, ptr("h2")
	templates[2].TLS, templates[2].RealityPublicKey, templates[2].RealityShortID = 2, ptr("pbk"), ptr("sid")
	templates[2].TransportSettings, templates[2].TemplateJSON = ptr(`{"path":"/ws"}`), `{"name":"{{.Email}}"}`
	templates[3].Type, templates[3].SSCipher, templates[3].Tags = "shadowsocks", ptr("2022-blake3-aes-128-gcm"), ptr(`["sg"]`)
	require.NoError(t, db.Create(&templates).Error)
	require.NoError(t, db.Create(&[]model.PlanSubscriptionGroup{
		{ID: 1, PlanID: 1, GroupID: 1, CreatedAt: seeded}, {ID: 2, PlanID: 2, GroupID: 1, CreatedAt: seeded},
		{ID: 3, PlanID: 2, GroupID: 2, CreatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.UserSubscriptionGroup{
		{ID: 1, UserID: 2, GroupID: 1, CreatedAt: seeded},
		{ID: 2, UserID: 2, GroupID: 2, ExpireAt: ptr(expiryBase + 86400), TransferEnable: ptr(int64(1 << 30)), CreatedAt: seeded},
		{ID: 3, UserID: 3, GroupID: 2, ExpireAt: ptr(expiryBase - 86400), CreatedAt: seeded},
		{ID: 4, UserID: 4, GroupID: 2, ExpireAt: ptr(expiryBase + 3600), NextRenewPrice: ptr(int64(990)), CreatedAt: seeded},
		{ID: 5, UserID: 5, GroupID: 3, CreatedAt: seeded},
	}).Error)
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 1, Name: "edge-1", Host: "edge-1.example.test", APIKey: "k1", Secret: "s1", LastCheckAt: ptr(now - 60), CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "edge-2", Host: "edge-2.example.test", APIKey: "k2", Secret: "s2", LastCheckAt: ptr(now - 3600), CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "edge-3", Host: "edge-3.example.test", APIKey: "k3", Secret: "s3", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	protocol := func(id, node uint) model.NodeProtocol {
		return model.NodeProtocol{
			ID: id, NodeID: node, Name: fmt.Sprintf("p%d", id), Type: model.ProtocolVLESS, Port: 440 + int(id),
			RealitySettings: ptr(`{"private_key":"secret"}`), CreatedAt: seeded, UpdatedAt: seeded,
		}
	}
	require.NoError(t, db.Create(&[]model.NodeProtocol{protocol(1, 1), protocol(2, 1), protocol(3, 2), protocol(4, 3), protocol(5, 1)}).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (1, 1), (1, 3), (2, 2), (2, 4), (2, 1)").Error)
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
	syncSequences(t, db)
}

// empty has the tables and views but no rows.
func empty(t testing.TB, db *gorm.DB) { require.NoError(t, packagestore.EnsureKernelAPIViews(db)) }

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{
		"v2_plan", "v2_user", "v2_subscription_group", "v2_subscription_template", "v2_plan_subscription_group",
		"v2_user_subscription_group", "v2_node", "v2_node_protocol",
	} {
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

// state is everything the routes can change, and the node and membership
// tables they must not.
func state(t testing.TB, db *gorm.DB) any {
	var groups []model.SubscriptionGroup
	require.NoError(t, db.Order("id").Find(&groups).Error)
	groupRows := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		groupRows = append(groupRows, map[string]any{
			"id": g.ID, "name": g.Name, "description": g.Description, "priority": g.Priority, "enable": g.Enable,
			"created_at": clockTime(g.CreatedAt), "updated_at": clockTime(g.UpdatedAt),
		})
	}
	var templates []model.SubscriptionTemplate
	require.NoError(t, db.Order("id").Find(&templates).Error)
	templateRows := make([]map[string]any, 0, len(templates))
	for _, tpl := range templates {
		created, updated := tpl.CreatedAt, tpl.UpdatedAt
		tpl.CreatedAt, tpl.UpdatedAt = time.Time{}, time.Time{}
		templateRows = append(templateRows, map[string]any{"row": tpl, "created_at": clockTime(created), "updated_at": clockTime(updated)})
	}
	var planGroups []model.PlanSubscriptionGroup
	require.NoError(t, db.Order("id").Find(&planGroups).Error)
	planRows := make([]map[string]any, 0, len(planGroups))
	for _, pg := range planGroups {
		planRows = append(planRows, map[string]any{"id": pg.ID, "plan_id": pg.PlanID, "group_id": pg.GroupID, "created_at": clockTime(pg.CreatedAt)})
	}
	var links []struct {
		SubscriptionGroupID uint
		NodeProtocolID      uint
	}
	require.NoError(t, db.Table("v2_subscription_group_node_protocols").Order("subscription_group_id, node_protocol_id").Find(&links).Error)
	var members []model.UserSubscriptionGroup
	require.NoError(t, db.Order("id").Find(&members).Error)
	var nodes, protocols int64
	require.NoError(t, db.Model(&model.Node{}).Count(&nodes).Error)
	require.NoError(t, db.Model(&model.NodeProtocol{}).Count(&protocols).Error)
	return map[string]any{
		"groups": groupRows, "templates": templateRows, "plan_groups": planRows, "links": links,
		"members": len(members), "nodes": nodes, "protocols": protocols,
	}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Principal = admin
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Principal = admin
		c.Snapshot = state
		packagecompat.RunWrite(t, r, c)
	}
}

func TestStaticListsParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/subscription/formats", "subscription.admin.subscription.formats.get",
		(*handler.SubscriptionAdminHandler).GetSubscriptionFormats),
		[]packagecompat.Case{{Name: "formats", Path: "/api/v2/admin/subscription/formats"}})
	read(t, route("GET", "/api/v2/admin/subscription/protocols", "subscription.admin.subscription.protocols.get",
		(*handler.SubscriptionAdminHandler).GetProtocolTypes),
		[]packagecompat.Case{{Name: "protocol types", Path: "/api/v2/admin/subscription/protocols"}})
}

func TestGroupReadsParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/subscription/groups", "subscription.admin.subscription.groups.get",
		(*handler.SubscriptionAdminHandler).GetGroups),
		[]packagecompat.Case{
			{Name: "by priority then id", Path: "/api/v2/admin/subscription/groups"},
			{Name: "no groups", Path: "/api/v2/admin/subscription/groups", Seed: empty},
		})
	group := func(id string) string { return "/api/v2/admin/subscription/groups/" + id }
	read(t, route("GET", "/api/v2/admin/subscription/groups/:id", "subscription.admin.subscription.groups.id.get",
		(*handler.SubscriptionAdminHandler).GetGroup),
		[]packagecompat.Case{
			{Name: "with its templates", Path: group("1")},
			{Name: "with a description", Path: group("2")},
			{Name: "without templates", Path: group("4")},
			{Name: "unknown", Path: group("99")},
			{Name: "zero", Path: group("0")},
			{Name: "not a number", Path: group("x")},
			{Name: "a condition", Path: group("1%20OR%201=1")},
			{Name: "beyond 32 bits", Path: group("4294967297")},
		})
	templates := func(id string) string { return "/api/v2/admin/subscription/groups/" + id + "/templates" }
	read(t, route("GET", "/api/v2/admin/subscription/groups/:id/templates", "subscription.admin.subscription.groups.id.templates.get",
		(*handler.SubscriptionAdminHandler).GetTemplates),
		[]packagecompat.Case{
			{Name: "by sort then id", Path: templates("1")},
			{Name: "settings and keys", Path: templates("2")},
			{Name: "none", Path: templates("4")},
			{Name: "unknown group", Path: templates("99")},
			{Name: "not a number", Path: templates("x")},
		})
	template := func(id string) string { return "/api/v2/admin/subscription/templates/" + id }
	read(t, route("GET", "/api/v2/admin/subscription/templates/:id", "subscription.admin.subscription.templates.id.get",
		(*handler.SubscriptionAdminHandler).GetTemplate),
		[]packagecompat.Case{
			{Name: "a template", Path: template("3")},
			{Name: "another", Path: template("4")},
			{Name: "unknown", Path: template("99")},
			{Name: "not a number", Path: template("-1")},
		})
}

func TestLinkReadsParity(t *testing.T) {
	plans := func(id string) string { return "/api/v2/admin/subscription/plans/" + id + "/groups" }
	read(t, route("GET", "/api/v2/admin/subscription/plans/:plan_id/groups", "subscription.admin.subscription.plans.plan_id.groups.get",
		(*handler.SubscriptionAdminHandler).GetPlanGroups),
		[]packagecompat.Case{
			{Name: "two groups", Path: plans("2")},
			{Name: "one group", Path: plans("1")},
			{Name: "none", Path: plans("3")},
			{Name: "unknown plan", Path: plans("99")},
			{Name: "plan zero", Path: plans("0")},
			{Name: "not a number", Path: plans("x")},
		})
	users := func(id string) string { return "/api/v2/admin/subscription/users/" + id + "/groups" }
	read(t, route("GET", "/api/v2/admin/subscription/users/:user_id/groups", "subscription.admin.subscription.users.user_id.groups.get",
		(*handler.SubscriptionAdminHandler).GetUserGroups),
		[]packagecompat.Case{
			{Name: "unexpired and without expiry", Path: users("2")},
			{Name: "an expired one is left out", Path: users("3")},
			{Name: "an expiring one", Path: users("4")},
			{Name: "a disabled group is listed", Path: users("5")},
			{Name: "none", Path: users("1")},
			{Name: "unknown user", Path: users("99")},
			{Name: "not a number", Path: users("x")},
		})
}

func TestStatsParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/subscription/stats", "subscription.admin.subscription.stats.get",
		(*handler.SubscriptionAdminHandler).GetGroupStats),
		[]packagecompat.Case{
			{Name: "members, traffic, templates, protocols, online nodes and plans", Path: "/api/v2/admin/subscription/stats"},
			{Name: "no groups", Path: "/api/v2/admin/subscription/stats", Seed: empty},
		})
}

func TestGroupWritesParity(t *testing.T) {
	stamped := []string{"data.created_at", "data.updated_at"}
	create := route("POST", "/api/v2/admin/subscription/groups", "subscription.admin.subscription.groups.post",
		(*handler.SubscriptionAdminHandler).CreateGroup)
	path := "/api/v2/admin/subscription/groups"
	write(t, create, []packagecompat.Case{
		{Name: "a group", Path: path, Mask: stamped, Body: []byte(`{"name":"new","description":"d","priority":3,"enable":1}`)},
		{Name: "zero enable takes the default", Path: path, Mask: stamped, Body: []byte(`{"name":"new","enable":0}`)},
		{Name: "an explicit id and times", Path: path, Body: []byte(`{"id":40,"name":"new","created_at":"2020-01-02T03:04:05Z","updated_at":"2020-01-02T03:04:05Z"}`),
			Mask: []string{"data.updated_at"}},
		{Name: "nested associations are dropped", Path: path, Mask: stamped,
			Body: []byte(`{"name":"new","protocols":[{"id":1},{"name":"x","node":{"name":"n"}}],"templates":[{"id":1},{"name":"t"}]}`)},
		{Name: "a duplicate name", Path: path, Body: []byte(`{"name":"default"}`)},
		{Name: "a name of the wrong type", Path: path, Body: []byte(`{"name":1}`)},
		{Name: "templates of the wrong type", Path: path, Body: []byte(`{"name":"new","templates":"x"}`)},
		{Name: "a nested protocol type of the wrong type", Path: path, Body: []byte(`{"name":"new","protocols":[{"type":1}]}`)},
		{Name: "a nested node status of the wrong type", Path: path, Body: []byte(`{"name":"new","protocols":[{"node":{"status":"x"}}]}`)},
		{Name: "a nested time that does not parse", Path: path, Body: []byte(`{"name":"new","templates":[{"created_at":"x"}]}`)},
		{Name: "a body that does not parse", Path: path, Body: []byte(`{"name":`)},
		{Name: "no body", Path: path},
		{Name: "a body that is not an object", Path: path, Body: []byte(`[1]`)},
	})
	update := route("PUT", "/api/v2/admin/subscription/groups/:id", "subscription.admin.subscription.groups.id.put",
		(*handler.SubscriptionAdminHandler).UpdateGroup)
	group := func(id string) string { return "/api/v2/admin/subscription/groups/" + id }
	write(t, update, []packagecompat.Case{
		{Name: "every column", Path: group("2"), Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"premium+","description":null,"priority":1,"enable":1}`)},
		{Name: "missing fields become zero", Path: group("1"), Mask: []string{"data.updated_at"}, Body: []byte(`{"name":"default"}`)},
		{Name: "the path id wins", Path: group("1"), Mask: []string{"data.updated_at"}, Body: []byte(`{"id":2,"name":"renamed"}`)},
		{Name: "an unknown id creates the group", Path: group("50"), Mask: stamped, Body: []byte(`{"name":"fifty","enable":1}`)},
		{Name: "id zero creates a group", Path: group("0"), Mask: stamped, Body: []byte(`{"name":"zero"}`)},
		{Name: "nested associations are dropped", Path: group("2"), Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"premium","protocols":[{"id":3}],"templates":[{"id":1,"name":"moved"}]}`)},
		{Name: "a duplicate name", Path: group("2"), Body: []byte(`{"name":"default"}`)},
		{Name: "a priority of the wrong type", Path: group("2"), Body: []byte(`{"priority":"high"}`)},
		{Name: "not a number", Path: group("x"), Body: []byte(`{"name":"x"}`)},
		{Name: "an invalid body before the id", Path: group("x"), Body: []byte(`{"name":`)},
		{Name: "no body", Path: group("1")},
	})
}

func TestTemplateWritesParity(t *testing.T) {
	stamped := []string{"data.created_at", "data.updated_at"}
	create := route("POST", "/api/v2/admin/subscription/groups/:id/templates", "subscription.admin.subscription.groups.id.templates.post",
		(*handler.SubscriptionAdminHandler).CreateTemplate)
	templates := func(id string) string { return "/api/v2/admin/subscription/groups/" + id + "/templates" }
	full := `{"name":"tw","type":"trojan","server":"tw.example.test","port":8443,"server_name":"tw.example.test","tls":1,` +
		`"tls_fingerprint":"chrome","alpn":"h2,http/1.1","transport":"ws","transport_settings":"{\"path\":\"/ws\"}","enable":1,"sort":3,` +
		`"protocol_settings":"{\"flow\":\"\"}","template_json":"{\"name\":\"{{.Email}}\"}","tags":"[\"tw\"]"}`
	write(t, create, []packagecompat.Case{
		{Name: "every field", Path: templates("1"), Mask: stamped, Body: []byte(full)},
		{Name: "defaults", Path: templates("2"), Mask: stamped, Body: []byte(`{"name":"min"}`)},
		{Name: "the path group wins over the body", Path: templates("2"), Mask: stamped, Body: []byte(`{"name":"x","group_id":1}`)},
		{Name: "a nested group is dropped", Path: templates("2"), Mask: stamped,
			Body: []byte(`{"name":"x","group":{"id":1,"name":"default","protocols":[{"name":"p","node":{"name":"n"}}]}}`)},
		// SQLite stores it, PostgreSQL refuses it (foreign key): the body's
		// times keep the answer free of clock times on both.
		{Name: "an unknown group", Path: templates("99"),
			Body: []byte(`{"name":"orphan","created_at":"2020-01-02T03:04:05Z","updated_at":"2020-01-02T03:04:05Z"}`)},
		{Name: "a port of the wrong type", Path: templates("1"), Body: []byte(`{"port":"443"}`)},
		{Name: "a nested group of the wrong type", Path: templates("1"), Body: []byte(`{"group":"default"}`)},
		{Name: "not a number", Path: templates("x"), Body: []byte(full)},
		{Name: "no body", Path: templates("1")},
	})
	update := route("PUT", "/api/v2/admin/subscription/templates/:id", "subscription.admin.subscription.templates.id.put",
		(*handler.SubscriptionAdminHandler).UpdateTemplate)
	template := func(id string) string { return "/api/v2/admin/subscription/templates/" + id }
	write(t, update, []packagecompat.Case{
		{Name: "some fields", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"name":"us-2","port":8443,"tls":true,"enable":false}`)},
		{Name: "a pointer field to null", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"reality_public_key":null}`)},
		{Name: "move to another group", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"group_id":1}`)},
		{Name: "a fractional sort", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"sort":2.7}`)},
		{Name: "the id and times are kept", Path: template("3"), Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"x","id":9,"ID":9,"Id":9,"created_at":"2001-01-01T00:00:00Z","CreatedAt":"2001-01-01T00:00:00Z","UPDATED_AT":"2001-01-01T00:00:00Z"}`)},
		{Name: "only protected keys", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"id":9}`)},
		{Name: "a field name", Path: template("3"), Mask: []string{"data.updated_at"}, Body: []byte(`{"Name":"by field name"}`)},
		{Name: "an unknown column", Path: template("3"), Body: []byte(`{"bogus":1}`)},
		{Name: "the group relation", Path: template("3"), Body: []byte(`{"Group":{"name":"x"}}`)},
		{Name: "an empty object", Path: template("3"), Body: []byte(`{}`)},
		{Name: "null", Path: template("3"), Body: []byte(`null`)},
		{Name: "unknown", Path: template("99"), Body: []byte(`{"name":"x"}`)},
		{Name: "not a number", Path: template("x"), Body: []byte(`{"name":"x"}`)},
		{Name: "a body that is not an object", Path: template("3"), Body: []byte(`[1]`)},
		{Name: "no body", Path: template("3")},
	})
	remove := route("DELETE", "/api/v2/admin/subscription/templates/:id", "subscription.admin.subscription.templates.id.delete",
		(*handler.SubscriptionAdminHandler).DeleteTemplate)
	write(t, remove, []packagecompat.Case{
		{Name: "a template", Path: template("2")},
		{Name: "unknown", Path: template("99")},
		{Name: "zero", Path: template("0")},
		{Name: "not a number", Path: template("x")},
	})
}

func TestPlanLinkWritesParity(t *testing.T) {
	assign := route("POST", "/api/v2/admin/subscription/plans/:plan_id/groups", "subscription.admin.subscription.plans.plan_id.groups.post",
		(*handler.SubscriptionAdminHandler).AssignGroupToPlan)
	plans := func(id string) string { return "/api/v2/admin/subscription/plans/" + id + "/groups" }
	write(t, assign, []packagecompat.Case{
		{Name: "a new link", Path: plans("3"), Body: []byte(`{"group_id":2}`)},
		{Name: "an existing link is kept once", Path: plans("2"), Body: []byte(`{"group_id":2}`)},
		{Name: "unknown plan", Path: plans("99"), Body: []byte(`{"group_id":2}`)},
		{Name: "unknown group", Path: plans("1"), Body: []byte(`{"group_id":99}`)},
		{Name: "no group", Path: plans("1"), Body: []byte(`{}`)},
		{Name: "a group of the wrong type", Path: plans("1"), Body: []byte(`{"group_id":"2"}`)},
		// The driver refuses an id beyond int64: a database error, not a
		// missing record.
		{Name: "a group id the database refuses", Path: plans("1"), Body: []byte(`{"group_id":18446744073709551615}`)},
		{Name: "not a number", Path: plans("x"), Body: []byte(`{"group_id":2}`)},
		{Name: "no body", Path: plans("1")},
	})
	remove := route("DELETE", "/api/v2/admin/subscription/plans/:plan_id/groups/:group_id",
		"subscription.admin.subscription.plans.plan_id.groups.group_id.delete", (*handler.SubscriptionAdminHandler).RemoveGroupFromPlan)
	link := func(plan, group string) string {
		return "/api/v2/admin/subscription/plans/" + plan + "/groups/" + group
	}
	write(t, remove, []packagecompat.Case{
		{Name: "a link", Path: link("2", "1")},
		{Name: "no such link", Path: link("1", "2")},
		{Name: "unknown plan", Path: link("99", "1")},
		{Name: "unknown group", Path: link("1", "99")},
		{Name: "plan not a number", Path: link("x", "1")},
		{Name: "group not a number", Path: link("1", "x")},
	})
}

func TestProtocolLinkWritesParity(t *testing.T) {
	link := route("POST", "/api/v2/admin/subscription/groups/:id/protocols", "subscription.admin.subscription.groups.id.protocols.post",
		(*handler.SubscriptionAdminHandler).UpdateGroupProtocols)
	protocols := func(id string) string { return "/api/v2/admin/subscription/groups/" + id + "/protocols" }
	write(t, link, []packagecompat.Case{
		{Name: "replace some", Path: protocols("2"), Body: []byte(`{"protocol_ids":[1,3,5]}`)},
		{Name: "duplicates", Path: protocols("1"), Body: []byte(`{"protocol_ids":[3,3,2]}`)},
		{Name: "the same set", Path: protocols("1"), Body: []byte(`{"protocol_ids":[3,1]}`)},
		{Name: "clear", Path: protocols("2"), Body: []byte(`{"protocol_ids":[]}`)},
		{Name: "a group without links", Path: protocols("4"), Body: []byte(`{"protocol_ids":[4]}`)},
		{Name: "an unknown protocol", Path: protocols("1"), Body: []byte(`{"protocol_ids":[1,99]}`)},
		{Name: "an unknown group", Path: protocols("99"), Body: []byte(`{"protocol_ids":[1]}`)},
		{Name: "group zero", Path: protocols("0"), Body: []byte(`{"protocol_ids":[1]}`)},
		{Name: "no list", Path: protocols("1"), Body: []byte(`{}`)},
		{Name: "a null list", Path: protocols("1"), Body: []byte(`{"protocol_ids":null}`)},
		{Name: "a negative id", Path: protocols("1"), Body: []byte(`{"protocol_ids":[-1]}`)},
		{Name: "a fractional id", Path: protocols("1"), Body: []byte(`{"protocol_ids":[1.5]}`)},
		{Name: "an id the database refuses", Path: protocols("1"), Body: []byte(`{"protocol_ids":[18446744073709551615]}`)},
		{Name: "not a number", Path: protocols("x"), Body: []byte(`{"protocol_ids":[1]}`)},
		{Name: "no body", Path: protocols("1")},
	})
}

// membershipState is state plus the shared subscriber state a membership
// change writes: v2_user_subscription_group, the request ledger and the
// change log, with clock times masked and, for requests that carry no key
// or request id, the random request ids too.
func membershipState(maskRequestIDs bool) func(t testing.TB, db *gorm.DB) any {
	return func(t testing.TB, db *gorm.DB) any {
		var members []model.UserSubscriptionGroup
		require.NoError(t, db.Order("id").Find(&members).Error)
		memberRows := make([]map[string]any, 0, len(members))
		for _, m := range members {
			memberRows = append(memberRows, map[string]any{
				"id": m.ID, "user_id": m.UserID, "group_id": m.GroupID, "expire_at": m.ExpireAt, "transfer_enable": m.TransferEnable,
				"next_renew_price": m.NextRenewPrice, "created_at": clockTime(m.CreatedAt),
			})
		}
		var requests []model.SubscriberRequest
		require.NoError(t, db.Order("created_at, request_id").Find(&requests).Error)
		ledger := make([]map[string]any, 0, len(requests))
		for index, request := range requests {
			id := request.RequestID
			if maskRequestIDs {
				id = fmt.Sprintf("<request %d>", index+1)
			}
			ledger = append(ledger, map[string]any{
				"request_id": id, "method": request.Method, "user_id": request.UserID, "result": request.Result,
				"created_at": clockTime(request.CreatedAt),
			})
		}
		var changes []struct {
			UserID  uint
			Deleted bool
		}
		require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
		return map[string]any{"tables": state(t, db), "members": memberRows, "ledger": ledger, "changes": changes}
	}
}

func writeMembership(t *testing.T, r packagecompat.Route, maskRequestIDs bool, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Principal = admin
		c.Snapshot = membershipState(maskRequestIDs)
		packagecompat.RunWrite(t, r, c)
	}
}

func requestID(id string) map[string]string { return map[string]string{"X-Request-ID": id} }

func TestUserGroupGrantParity(t *testing.T) {
	grant := membershipRoute(t, "POST", "/api/v2/admin/subscription/users/:user_id/groups", native.GrantUserGroupRouteID,
		(*handler.SubscriptionAdminHandler).AssignGroupToUser)
	users := func(id string) string { return "/api/v2/admin/subscription/users/" + id + "/groups" }
	later := time.Now().Add(30 * 24 * time.Hour).Unix()
	expiring := func(group int) []byte { return []byte(fmt.Sprintf(`{"group_id":%d,"expire_at":%d}`, group, later)) }
	writeMembership(t, grant, false, []packagecompat.Case{
		{Name: "a new group with every field", Path: users("2"), RequestHeaders: requestID("g-1"),
			Body: []byte(fmt.Sprintf(`{"group_id":3,"expire_at":%d,"transfer_enable":1073741824,"next_renew_price":1500}`, later))},
		{Name: "a new group without fields", Path: users("1"), RequestHeaders: requestID("g-2"), Body: []byte(`{"group_id":4}`)},
		{Name: "a disabled group", Path: users("1"), RequestHeaders: requestID("g-3"), Body: []byte(`{"group_id":3}`)},
		{Name: "an existing group: only the given fields are set", Path: users("2"), RequestHeaders: requestID("g-4"),
			Body: []byte(`{"group_id":2,"next_renew_price":500}`)},
		{Name: "an existing group: a new expiry", Path: users("2"), RequestHeaders: requestID("g-5"), Body: expiring(2)},
		{Name: "an existing group: null fields are kept", Path: users("2"), RequestHeaders: requestID("g-6"),
			Body: []byte(`{"group_id":2,"expire_at":null,"transfer_enable":null}`)},
		{Name: "an expired membership of a banned subscriber", Path: users("3"), RequestHeaders: requestID("g-7"), Body: expiring(2)},
		{Name: "an expired subscriber", Path: users("4"), RequestHeaders: requestID("g-8"), Body: []byte(`{"group_id":1}`)},
		{Name: "a subscriber out of traffic", Path: users("5"), RequestHeaders: requestID("g-9"), Body: []byte(`{"group_id":1}`)},
		{Name: "negative values are stored as given", Path: users("2"), RequestHeaders: requestID("g-10"),
			Body: []byte(`{"group_id":4,"expire_at":-1,"transfer_enable":-1,"next_renew_price":-5}`)},
		{Name: "a retried request applies once", Path: users("1"), RequestHeaders: requestID("g-11"),
			Warmup: [][]byte{[]byte(`{"group_id":2}`)}, Body: []byte(`{"group_id":2}`)},
		{Name: "the idempotency key wins over the request id", Path: users("1"),
			RequestHeaders: map[string]string{"Idempotency-Key": "grant-1", "X-Request-ID": "g-12"},
			Warmup:         [][]byte{expiring(2)}, Body: expiring(2)},
		{Name: "a key reused for other values is another grant", Path: users("1"), RequestHeaders: map[string]string{"Idempotency-Key": "grant-2"},
			Warmup: [][]byte{[]byte(`{"group_id":2}`)}, Body: expiring(2)},
		{Name: "unknown user", Path: users("99"), RequestHeaders: requestID("g-13"), Body: []byte(`{"group_id":1}`)},
		{Name: "unknown user and group", Path: users("99"), Body: []byte(`{"group_id":99}`)},
		{Name: "user zero", Path: users("0"), Body: []byte(`{"group_id":1}`)},
		{Name: "unknown group", Path: users("2"), RequestHeaders: requestID("g-14"), Body: []byte(`{"group_id":99}`)},
		{Name: "a group beyond 32 bits", Path: users("2"), Body: []byte(`{"group_id":4294967297}`)},
		// The driver refuses an id beyond int64: a database error, not a
		// missing record.
		{Name: "a group id the database refuses", Path: users("2"), Body: []byte(`{"group_id":18446744073709551615}`)},
		{Name: "no group", Path: users("2"), Body: []byte(`{}`)},
		{Name: "group zero", Path: users("2"), Body: []byte(`{"group_id":0}`)},
		{Name: "a group of the wrong type", Path: users("2"), Body: []byte(`{"group_id":"2"}`)},
		{Name: "an expiry of the wrong type", Path: users("2"), Body: []byte(`{"group_id":2,"expire_at":"soon"}`)},
		{Name: "user not a number", Path: users("x"), Body: []byte(`{"group_id":1}`)},
		{Name: "user beyond 32 bits", Path: users("4294967297"), Body: []byte(`{"group_id":1}`)},
		{Name: "a body that does not parse", Path: users("2"), Body: []byte(`{"group_id":`)},
		{Name: "no body", Path: users("2")},
	})
	// Without an idempotency key or request id every request is a grant of
	// its own, as in v2; the ids are random on both sides.
	writeMembership(t, grant, true, []packagecompat.Case{
		{Name: "two requests are two grants", Path: users("2"), Warmup: [][]byte{[]byte(`{"group_id":3}`)}, Body: expiring(3)},
	})
}

func TestUserGroupRevokeParity(t *testing.T) {
	revoke := membershipRoute(t, "DELETE", "/api/v2/admin/subscription/users/:user_id/groups/:group_id", native.RevokeUserGroupRouteID,
		(*handler.SubscriptionAdminHandler).RemoveGroupFromUser)
	membership := func(user, group string) string {
		return "/api/v2/admin/subscription/users/" + user + "/groups/" + group
	}
	writeMembership(t, revoke, false, []packagecompat.Case{
		{Name: "a group", Path: membership("2", "1"), RequestHeaders: requestID("r-1")},
		{Name: "an expiring group", Path: membership("2", "2"), RequestHeaders: requestID("r-2")},
		{Name: "an expired membership of a banned subscriber", Path: membership("3", "2"), RequestHeaders: requestID("r-3")},
		{Name: "an expired subscriber", Path: membership("4", "2"), RequestHeaders: requestID("r-4")},
		{Name: "a disabled group of a subscriber out of traffic", Path: membership("5", "3"), RequestHeaders: requestID("r-5")},
		{Name: "a retried request applies once", Path: membership("2", "1"), RequestHeaders: requestID("r-6"), Warmup: [][]byte{nil}},
		{Name: "no such membership", Path: membership("1", "2"), RequestHeaders: requestID("r-7")},
		{Name: "unknown user", Path: membership("99", "1"), RequestHeaders: requestID("r-8")},
		{Name: "unknown group", Path: membership("2", "99"), RequestHeaders: requestID("r-9")},
		{Name: "unknown user and group", Path: membership("99", "99")},
		{Name: "user zero", Path: membership("0", "1")},
		{Name: "group zero", Path: membership("2", "0")},
		{Name: "user not a number", Path: membership("x", "1")},
		{Name: "group not a number", Path: membership("2", "x")},
		{Name: "group beyond 32 bits", Path: membership("2", "4294967297")},
	})
	// A new request after the removal finds no membership.
	writeMembership(t, revoke, true, []packagecompat.Case{
		{Name: "a second request finds none", Path: membership("2", "1"), Warmup: [][]byte{nil}},
	})
}

func TestGroupDeleteParity(t *testing.T) {
	remove := membershipRoute(t, "DELETE", "/api/v2/admin/subscription/groups/:id", native.DeleteGroupRouteID,
		(*handler.SubscriptionAdminHandler).DeleteGroup)
	group := func(id string) string { return "/api/v2/admin/subscription/groups/" + id }
	writeMembership(t, remove, false, []packagecompat.Case{
		{Name: "members, templates, plan and protocol links", Path: group("2"), RequestHeaders: requestID("d-1")},
		{Name: "one active member", Path: group("1"), RequestHeaders: requestID("d-2")},
		{Name: "a disabled group with an inactive member", Path: group("3"), RequestHeaders: requestID("d-3")},
		{Name: "a group without members or links", Path: group("4"), RequestHeaders: requestID("d-4")},
		{Name: "a retried request finds the group gone", Path: group("2"), RequestHeaders: requestID("d-5"), Warmup: [][]byte{nil}},
		{Name: "unknown", Path: group("99"), RequestHeaders: requestID("d-6")},
		{Name: "zero", Path: group("0")},
		{Name: "not a number", Path: group("x")},
		{Name: "a condition", Path: group("1%20OR%201=1")},
		{Name: "beyond 32 bits", Path: group("4294967297")},
	})
	writeMembership(t, remove, true, []packagecompat.Case{
		{Name: "without a request id", Path: group("2")},
	})
}

// The bound and answered types mirror the kernel model: the same type names
// and, for every field the kernel decodes or answers, the same name, JSON
// tag and type (which binding errors print), and for columns the same GORM
// tag.
func TestModelMirrorsTheKernelModel(t *testing.T) {
	pairs := []struct {
		kernel, mirror any
		// absent are kernel relations the mirror leaves out: they are never
		// decoded from a body nor answered.
		absent []string
	}{
		{kernel: model.SubscriptionGroup{}, mirror: mirror.SubscriptionGroup{}},
		{kernel: model.SubscriptionTemplate{}, mirror: mirror.SubscriptionTemplate{}},
		{kernel: model.PlanSubscriptionGroup{}, mirror: mirror.PlanSubscriptionGroup{}, absent: []string{"Plan", "Group"}},
		{kernel: model.NodeProtocol{}, mirror: mirror.NodeProtocol{}},
		{kernel: model.Node{}, mirror: mirror.Node{}},
	}
	for _, pair := range pairs {
		kernel, copied := reflect.TypeOf(pair.kernel), reflect.TypeOf(pair.mirror)
		require.Equal(t, kernel.String(), copied.String())
		var kernelFields, mirrorFields []string
		for i := range kernel.NumField() {
			field := kernel.Field(i)
			if field.Tag.Get("json") == "-" || containsString(pair.absent, field.Name) {
				continue
			}
			kernelFields = append(kernelFields, field.Name)
			other, ok := copied.FieldByName(field.Name)
			require.True(t, ok, "%s.%s", kernel, field.Name)
			require.Equal(t, field.Tag.Get("json"), other.Tag.Get("json"), "%s.%s", kernel, field.Name)
			require.Equal(t, field.Type.String(), other.Type.String(), "%s.%s", kernel, field.Name)
			if isColumn(field.Type) && kernel.Name() != "Node" && kernel.Name() != "NodeProtocol" {
				require.Equal(t, field.Tag.Get("gorm"), other.Tag.Get("gorm"), "%s.%s", kernel, field.Name)
			}
		}
		for i := range copied.NumField() {
			mirrorFields = append(mirrorFields, copied.Field(i).Name)
		}
		require.Equal(t, kernelFields, mirrorFields, "%s fields in order", kernel)
	}
	require.Equal(t, reflect.TypeOf(model.ProtocolType("")).String(), reflect.TypeOf(mirror.ProtocolType("")).String())
	require.Equal(t, reflect.TypeOf(model.NodeStatus(0)).String(), reflect.TypeOf(mirror.NodeStatus(0)).String())
}

func isColumn(typ reflect.Type) bool {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Slice {
		return false
	}
	return typ.Kind() != reflect.Struct || typ == reflect.TypeOf(time.Time{})
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}
