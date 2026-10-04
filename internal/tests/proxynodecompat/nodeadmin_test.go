package proxynodecompat

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The node, registration key and load balancer check routes run through
// the whole path a deployment runs (packagecompat.RunKernelRead and
// RunKernelWrite): the kernel's gateway seals the secrets an administrator
// types into handles and expands the ones an answer shows, the package
// host reads v2_node, which its lease adopts once the node credential
// split finalized it, and the kernel stores raw configuration secrets,
// retires deleted nodes, issues and revokes registration keys and checks
// forward nodes. Both sides start from the same rows; the answers are the
// same bytes and the rows, the kernel's secrets and credentials end the
// same.

// now is the clock the seeds' report times are relative to, one value for
// the run, so both sides store the same.
var now = time.Now().Unix()

var wireGuard = func() (pair struct{ private, public, otherPrivate, otherPublic string }) {
	var err error
	if pair.private, pair.public, err = service.GenerateWireGuardKeypair(); err != nil {
		panic(err)
	}
	if pair.otherPrivate, pair.otherPublic, err = service.GenerateWireGuardKeypair(); err != nil {
		panic(err)
	}
	return pair
}()

func wireGuardRawConfig(private, public string) string {
	return fmt.Sprintf(`{"type":"wireguard","server_port":51820,"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24",`+
		`"server_private_key":%q,"server_public_key":%q,"tunnel_type":"quic","log":{"level":"info"}}`, private, public)
}

var adminModels = []any{
	&model.Node{}, &model.NodeProtocol{}, &model.NodeLog{}, &model.LoadBalancer{}, &model.WireGuardPeer{}, &model.SubscriptionGroup{},
	&model.User{}, &model.AuthorizedKey{}, &model.ForwardNode{}, &model.AgentCertificate{}, &model.AgentEnrollment{},
}

func capabilities() []string {
	return []string{service.CapabilityNodeOpsNodeConfig, service.CapabilityNodeOpsCredentials, service.CapabilityNodeOpsDiagnose}
}

// kernelRoute is a proxy-node route on the kernel's path.
func kernelRoute(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.KernelRoute {
	models := append(append(append([]any{}, adminModels...), packagecompat.NodeSplitModels()...), model.KernelNodeOperationModels()...)
	return packagecompat.KernelRoute{
		Route:     packagecompat.Route{Method: method, Pattern: pattern, RouteID: routeID, Models: models, Legacy: legacy},
		PackageID: "proxy-node", Capabilities: capabilities(),
		NativeService: func(db *gorm.DB, nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
			return (&native.Service{
				Open:    func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Leased:  packagecompat.Lease(db, "proxy-node"),
				NodeOps: nodeOps,
			}).Handlers()[routeID]
		},
	}
}

func nodeHandler(method func(*handler.NodeHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewNodeHandler(), c) }
}

// seedFleet writes nodes 1 to 6: online, offline, pending, disabled, in
// two groups, with raw configurations (a WireGuard one with its keys, one
// that is not JSON), protocols with secrets, a WireGuard protocol with its
// users' peers and group links, agent certificates, and registration keys.
func seedFleet(t testing.TB, db *gorm.DB) {
	recent, old := now-30, now-7200
	group := func(id uint) *uint { return &id }
	raw := func(text string) *string { return &text }
	nodes := []model.Node{
		{ID: 1, Name: "edge-hk", Host: "hk.example.test", Port: 443, Status: model.NodeStatusOnline, LastCheckAt: &recent, GroupID: group(1),
			Sort: 2, RawConfig: raw(wireGuardRawConfig(wireGuard.private, wireGuard.public)), TotalUpload: 1 << 30, Tags: raw(`["hk"]`)},
		{ID: 2, Name: "edge-jp", Host: "jp.example.test", Port: 443, Status: model.NodeStatusOnline, LastCheckAt: &old, GroupID: group(1), Sort: 1,
			RawConfig: raw(`{"log":{"level":"debug"},"api":{"token":"fake-raw-token"}}`)},
		{ID: 3, Name: "pending", Host: "p.example.test", Port: 8443, Status: model.NodeStatusPending, GroupID: group(2), RawConfig: raw("not json")},
		{ID: 4, Name: "disabled", Host: "d.example.test", Port: 443, Status: model.NodeStatusDisabled, LastCheckAt: &recent},
		{ID: 5, Name: "pending-online", Host: "po.example.test", Port: 443, Status: model.NodeStatusPending, LastCheckAt: &recent, GroupID: group(2)},
		{ID: 6, Name: "bare", Host: "bare.example.test", Port: 443, Status: model.NodeStatusOffline},
	}
	for i := range nodes {
		nodes[i].APIKey, nodes[i].APIKeyHash, nodes[i].Secret = fmt.Sprintf("fake-key-%d", i+1), fmt.Sprintf("hash-%d", i+1), fmt.Sprintf("fake-secret-%d", i+1)
		nodes[i].CreatedAt, nodes[i].UpdatedAt = seeded, seeded.Add(time.Minute)
	}
	require.NoError(t, db.Create(&nodes).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "admin@example.test", Token: "token-1", UUID: "uuid-1", CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	settings := wireGuardSettings(wireGuard.otherPrivate, wireGuard.otherPublic)
	protocols := []model.NodeProtocol{
		{ID: 1, NodeID: 1, Name: "reality", Type: model.ProtocolVLESS, Port: 443, TLS: 2, Sort: 1,
			RealitySettings: raw(`{"dest":"www.example.test:443","private_key":"fake-reality-private","public_key":"pub"}`)},
		{ID: 2, NodeID: 1, Name: "wg", Type: model.ProtocolWireGuard, Port: 51821, Settings: &settings},
		{ID: 3, NodeID: 2, Name: "plain", Type: model.ProtocolVMess, Port: 10086, Transport: raw("ws"), TransportSettings: raw(`{"path":"/ws"}`)},
	}
	for i := range protocols {
		protocols[i].CreatedAt, protocols[i].UpdatedAt = seeded, seeded.Add(time.Minute)
	}
	require.NoError(t, db.Create(&protocols).Error)
	require.NoError(t, db.Create(&model.WireGuardPeer{ID: 1, NodeProtocolID: 2, UserID: 1, PeerIP: "10.8.0.2", PrivateKey: "fake-peer-private",
		PublicKey: "peer-pub", PresharedKey: "fake-psk", CreatedAt: seeded, UpdatedAt: seeded}).Error)
	require.NoError(t, db.Create(&model.SubscriptionGroup{ID: 1, Name: "default", Enable: 1, CreatedAt: seeded, UpdatedAt: seeded}).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (1, 1), (1, 2), (1, 3)").Error)
	expires := now + 86400
	require.NoError(t, db.Create(&[]model.AuthorizedKey{
		{ID: 1, Name: "ansible", Key: "fake-registration-key-1", KeyHash: "kh1", Used: 2, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "expiring", Key: "fake-registration-key-2", KeyHash: "kh2", ExpireAt: &expires, CreatedAt: seeded.Add(time.Hour), UpdatedAt: seeded.Add(time.Hour)},
	}).Error)
	syncTables(t, db, "v2_node", "v2_node_protocol", "v2_wireguard_peer", "v2_subscription_group", "v2_user", "v2_authorized_key")
}

func wireGuardSettings(private, public string) string {
	return fmt.Sprintf(`{"cidr":"10.8.0.0/24","server_address":"10.8.0.1/24","server_private_key":%q,"server_public_key":%q,"tunnel_type":"quic",`+
		`"relay":{"server":"relay.example.test","server_port":8443}}`, private, public)
}

// seedFinalized is seedFleet with every table the routes read finalized.
func seedFinalized(t testing.TB, db *gorm.DB) {
	seedFleet(t, db)
	packagecompat.FinalizeNodeSplit(t, db, nodesecrets.TableNode, nodesecrets.TableNodeProtocol, nodesecrets.TableWireGuardPeer,
		nodesecrets.TableAuthorizedKey)
}

// fleetState is what the routes can change: the nodes as stored, the
// kernel's secrets and credentials for them, the protocols, peers and
// links that go with a node, the registration keys and the agent
// certificates. Times the handlers take from their clock read as
// "<clock>".
func fleetState(t testing.TB, db *gorm.DB) any {
	var nodes []model.Node
	require.NoError(t, db.Order("id").Find(&nodes).Error)
	nodeRows := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		created, updated := node.CreatedAt, node.UpdatedAt
		node.CreatedAt, node.UpdatedAt = time.Time{}, time.Time{}
		nodeRows = append(nodeRows, map[string]any{"row": node, "api_key": node.APIKey, "secret": node.Secret, "created_at": clock(created), "updated_at": clock(updated)})
	}
	var protocols []struct{ ID, NodeID uint }
	require.NoError(t, db.Model(&model.NodeProtocol{}).Order("id").Find(&protocols).Error)
	var secrets []struct {
		Scope, ColumnName, JSONPointer, Value string
		OwnerID                               uint
	}
	require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("scope <> ?", "legacy_original").Order("scope, owner_id, column_name, json_pointer").Find(&secrets).Error)
	var credentials []struct {
		SubjectKind, Kind, Status string
		SubjectID                 uint
		HasValue                  bool
	}
	require.NoError(t, db.Model(&model.NodeCredential{}).Select("subject_kind, kind, status, subject_id, value <> '' AS has_value").
		Order("subject_kind, subject_id, kind, status").Find(&credentials).Error)
	var keys []struct {
		ID       uint
		Name     string
		Used     int
		HasKey   bool
		ExpireAt *int64
	}
	require.NoError(t, db.Model(&model.AuthorizedKey{}).Select("id, name, used, key <> '' AS has_key, expire_at").Order("id").Find(&keys).Error)
	for i := range keys {
		// A key issued by the request ends a duration after the clock the
		// request read; only the seeded end is compared.
		if keys[i].ExpireAt != nil && *keys[i].ExpireAt != now+86400 {
			clocked := int64(-1)
			keys[i].ExpireAt = &clocked
		}
	}
	var peers, links int64
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Count(&peers).Error)
	require.NoError(t, db.Table("v2_subscription_group_node_protocols").Count(&links).Error)
	return map[string]any{"nodes": nodeRows, "protocols": protocols, "secrets": secrets, "credentials": credentials, "keys": keys, "peers": peers, "links": links}
}

func clock(value time.Time) string {
	if value.Sub(seeded) >= 0 && value.Sub(seeded) <= 2*time.Hour {
		return value.UTC().Format(time.RFC3339)
	}
	return "<clock>"
}

func syncTables(t testing.TB, db *gorm.DB, tables ...string) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range tables {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

func kernelRead(t *testing.T, r packagecompat.KernelRoute, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Principal == (pluginhostsdk.Principal{}) {
			c.Principal = admin
		}
		if c.Seed == nil {
			c.Seed = seedFinalized
		}
		packagecompat.RunKernelRead(t, r, c)
	}
}

func kernelWrite(t *testing.T, r packagecompat.KernelRoute, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Principal == (pluginhostsdk.Principal{}) {
			c.Principal = admin
		}
		if c.Seed == nil {
			c.Seed = seedFinalized
		}
		if c.Snapshot == nil {
			c.Snapshot = fleetState
		}
		packagecompat.RunKernelWrite(t, r, c)
	}
}

func TestNodeListParity(t *testing.T) {
	path := "/api/v2/admin/nodes"
	kernelRead(t, kernelRoute("GET", path, native.NodesRouteID, nodeHandler((*handler.NodeHandler).GetNodes)), []packagecompat.Case{
		{Name: "every node by sort, statuses from their last check", Path: path},
		{Name: "page 2 of 4", Path: path + "?page=2&page_size=4"},
		{Name: "a page size over the limit", Path: path + "?page_size=1000"},
		{Name: "pagination that is not a number", Path: path + "?page=x&page_size=-3"},
		{Name: "a status", Path: path + "?status=1"},
		{Name: "a status that is not a number", Path: path + "?status=x"},
		{Name: "a group", Path: path + "?group_id=1"},
		{Name: "a group that is not a number", Path: path + "?group_id=abc"},
		{Name: "a search in names and hosts", Path: path + "?search=jp"},
		{Name: "a search without results", Path: path + "?search=nothing"},
		{Name: "before finalize the kernel answers", Path: path, Seed: seedFleet, Fallback: true},
	})
}

func TestNodeDetailParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id }
	kernelRead(t, kernelRoute("GET", pattern, native.NodeRouteID, nodeHandler((*handler.NodeHandler).GetNode)), []packagecompat.Case{
		{Name: "with its protocols and raw configuration, masked", Path: path("1")},
		{Name: "a raw configuration that is not JSON", Path: path("3")},
		{Name: "a disabled node keeps its status", Path: path("4")},
		{Name: "an unknown node", Path: path("99")},
		{Name: "not a number", Path: path("x")},
		{Name: "beyond 32 bits", Path: path("4294967297")},
		{Name: "before finalize the kernel answers", Path: path("1"), Seed: seedFleet, Fallback: true},
	})
}

func TestNodeDeleteParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id }
	kernelWrite(t, kernelRoute("DELETE", pattern, native.DeleteNodeRouteID, nodeHandler((*handler.NodeHandler).DeleteNode)), []packagecompat.Case{
		{Name: "with its protocols, peers, links and credentials", Path: path("1")},
		{Name: "with a protocol", Path: path("2")},
		{Name: "without protocols", Path: path("6")},
		{Name: "an unknown node", Path: path("99")},
		{Name: "not a number", Path: path("x")},
		{Name: "before finalize the kernel answers", Path: path("1"), Seed: seedFleet, Fallback: true},
	})
}

func TestRawConfigReadParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/raw-config"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id + "/raw-config" }
	kernelRead(t, kernelRoute("GET", pattern, native.RawConfigRouteID, nodeHandler((*handler.NodeHandler).GetNodeRawConfig)), []packagecompat.Case{
		{Name: "a WireGuard configuration, masked", Path: path("1")},
		{Name: "a token, masked", Path: path("2")},
		{Name: "a configuration that is not JSON", Path: path("3")},
		{Name: "none", Path: path("6")},
		{Name: "an unknown node", Path: path("99")},
		{Name: "not a number", Path: path("x")},
		{Name: "before finalize the kernel answers", Path: path("1"), Seed: seedFleet, Fallback: true},
	})
}

func TestRawConfigUpdateParity(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id/raw-config"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id + "/raw-config" }
	body := func(config string) []byte { return []byte(`{"raw_config":` + config + `}`) }
	quoted := func(config string) []byte { return []byte(`{"raw_config":` + strconv.Quote(config) + `}`) }
	kernelWrite(t, kernelRoute("PUT", pattern, native.UpdateRawConfigRouteID, nodeHandler((*handler.NodeHandler).UpdateNodeRawConfig)), []packagecompat.Case{
		{Name: "a typed WireGuard key pair", Path: path("6"), Body: body(wireGuardRawConfig(wireGuard.otherPrivate, wireGuard.otherPublic))},
		{Name: "as a JSON string", Path: path("6"), Body: quoted(wireGuardRawConfig(wireGuard.otherPrivate, wireGuard.otherPublic))},
		{Name: "the placeholder keeps the private key", Path: path("1"), Body: body(wireGuardRawConfig(service.NodeSecretPlaceholder, wireGuard.public))},
		{Name: "a kept private key that does not match a new public key", Path: path("1"), Body: body(wireGuardRawConfig(service.NodeSecretPlaceholder, wireGuard.otherPublic))},
		{Name: "a typed private key that does not match its public key", Path: path("6"), Body: body(wireGuardRawConfig(wireGuard.otherPrivate, wireGuard.public))},
		{Name: "a configuration without secrets replaces one with", Path: path("2"), Body: body(`{"log":{"level":"warning"}}`)},
		{Name: "null clears it", Path: path("1"), Body: body("null")},
		{Name: "an array is refused", Path: path("2"), Body: body(`[1,2]`)},
		{Name: "a string that is not an object is refused", Path: path("2"), Body: quoted("[1]")},
		{Name: "an unknown node is nothing to update", Path: path("99"), Body: body(`{"log":{}}`)},
		{Name: "an unknown node with a placeholder", Path: path("99"), Body: body(`{"token":"********"}`)},
		{Name: "no configuration", Path: path("2"), Body: []byte(`{}`)},
		// The gateway binds handles to the node the path names, and must
		// read the body: such requests are the kernel's legacy handler's.
		{Name: "a node id that is not a number", Path: path("x"), Body: body(`{}`), Fallback: true},
		{Name: "not JSON", Path: path("2"), Body: []byte(`{"raw_config":`), Fallback: true},
		{Name: "before finalize the kernel answers", Path: path("6"), Seed: seedFleet, Fallback: true,
			Body: body(wireGuardRawConfig(wireGuard.otherPrivate, wireGuard.otherPublic))},
	})
}

func TestAuthKeysParity(t *testing.T) {
	path := "/api/v2/admin/auth-keys"
	kernelRead(t, kernelRoute("GET", path, native.AuthKeysRouteID, nodeHandler((*handler.NodeHandler).GetAuthKeys)), []packagecompat.Case{
		{Name: "newest first, keys masked", Path: path},
		{Name: "none", Path: path, Seed: func(t testing.TB, db *gorm.DB) {
			packagecompat.FinalizeNodeSplit(t, db, nodesecrets.TableAuthorizedKey)
		}},
		{Name: "before finalize the kernel answers", Path: path, Seed: seedFleet, Fallback: true},
	})
	issued := []string{"data.key"}
	kernelWrite(t, kernelRoute("POST", path, native.GenerateAuthKeyRouteID, nodeHandler((*handler.NodeHandler).GenerateAuthKey)), []packagecompat.Case{
		{Name: "a key without an end", Path: path, Body: []byte(`{"name":"rollout"}`), Mask: issued},
		{Name: "a key for 30 days", Path: path, Body: []byte(`{"name":"short","expire_days":30}`), Mask: append(issued, "data.expire_at")},
		{Name: "no name", Path: path, Body: []byte(`{"expire_days":3}`)},
		{Name: "a negative duration", Path: path, Body: []byte(`{"name":"x","expire_days":-1}`)},
		{Name: "a name too long", Path: path, Body: []byte(fmt.Sprintf(`{"name":"%0256d"}`, 0))},
		// A body that is not JSON is served by the legacy handler: the
		// gateway cannot seal what it cannot read.
		{Name: "not JSON", Path: path, Body: []byte(`{"name":`), Fallback: true},
		{Name: "issued before finalize too", Path: path, Body: []byte(`{"name":"early"}`), Mask: issued, Seed: seedFleet,
			Snapshot: func(t testing.TB, db *gorm.DB) any {
				var count int64
				require.NoError(t, db.Model(&model.AuthorizedKey{}).Count(&count).Error)
				return count
			}},
	})
	internal := "/api/v2/internal/auth-keys"
	automation := pluginhostsdk.Principal{ActorID: 0, Admin: false}
	kernelWrite(t, kernelRoute("POST", internal, native.InternalAuthKeyRouteID, nodeHandler((*handler.NodeHandler).InternalGenerateAuthKey)), []packagecompat.Case{
		// The name's min=1 refuses a request without one, so the kernel's
		// default names never apply.
		{Name: "a node name without a name", Path: internal, Body: []byte(`{"node_name":"hk-3","expire_days":1}`), Principal: automation},
		{Name: "automation for a day", Path: internal, Body: []byte(`{"name":"rollout","node_name":"hk-3","expire_days":1}`), Principal: automation,
			Mask: []string{"data.key", "data.expire_at"}},
		{Name: "automation with a name", Path: internal, Body: []byte(`{"name":"ci"}`), Principal: automation, Mask: []string{"data.key"}},
		{Name: "a node name too long", Path: internal, Body: []byte(fmt.Sprintf(`{"node_name":"%0256d"}`, 0)), Principal: automation},
	})
	keyPath := func(id string) string { return "/api/v2/admin/auth-keys/" + id }
	kernelWrite(t, kernelRoute("DELETE", "/api/v2/admin/auth-keys/:id", native.DeleteAuthKeyRouteID, nodeHandler((*handler.NodeHandler).DeleteAuthKey)), []packagecompat.Case{
		{Name: "a key", Path: keyPath("1")},
		{Name: "an unknown key", Path: keyPath("99")},
		{Name: "not a number", Path: keyPath("x")},
	})
}

// seedBalancers writes load balancers with health checks on and off and
// forward nodes: one whose endpoint answers, one where nothing listens, in
// several statuses, latencies and loads.
func seedBalancers(listening, closed int) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedFinalized(t, db)
		require.NoError(t, db.Create(&[]model.LoadBalancer{
			{ID: 1, Name: "edge", GroupID: 1, Strategy: "least-load", HealthCheck: true, CheckInterval: 60, CheckTimeout: 5, Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
			{ID: 2, Name: "quiet", GroupID: 2, Strategy: "latency", HealthCheck: true, CheckInterval: 60, CheckTimeout: 5, Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
		}).Error)
		require.NoError(t, db.Model(&model.LoadBalancer{}).Where("id = ?", 2).UpdateColumn("health_check", false).Error)
		require.NoError(t, db.Create(&[]model.ForwardNode{
			{ID: 1, Name: "relay-a", Host: "127.0.0.1", Port: listening, Status: model.ForwardNodeStatusOnline, Latency: 12, Load: 0.5, Enabled: true, APIToken: "fake-forward-token-1", CreatedAt: seeded, UpdatedAt: seeded},
			{ID: 2, Name: "relay-b", Host: "127.0.0.1", Port: closed, Status: model.ForwardNodeStatusOnline, Latency: 31, Load: 0.25, Enabled: true, APIToken: "fake-forward-token-2", CreatedAt: seeded, UpdatedAt: seeded},
			{ID: 3, Name: "relay-c", Host: "127.0.0.1", Port: closed, Status: 0, Latency: 99, Load: 0.9, Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
		}).Error)
		syncTables(t, db, "v2_load_balancer", "v2_forward_node")
	}
}

// forwardState is what a health check records on the forward nodes: their
// status and whether they were checked; the latency a check measures and
// the time it ran differ by run.
func forwardState(t testing.TB, db *gorm.DB) any {
	var nodes []struct {
		ID      uint
		Status  int
		Checked bool
	}
	require.NoError(t, db.Model(&model.ForwardNode{}).Select("id, status, last_check IS NOT NULL AS checked").Order("id").Find(&nodes).Error)
	return nodes
}

func TestLoadBalancerStatsAndCheckParity(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	listening := listener.Addr().(*net.TCPAddr).Port
	nothing, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := nothing.Addr().(*net.TCPAddr).Port
	require.NoError(t, nothing.Close())

	stats := func(id string) string { return "/api/v2/admin/loadbalancers/" + id + "/stats" }
	kernelRead(t, kernelRoute("GET", "/api/v2/admin/loadbalancers/:id/stats", native.LoadBalancerStatsRouteID,
		func(c *gin.Context) { handler.NewLoadBalancerHandler().GetLoadBalancerStats(c) }), []packagecompat.Case{
		{Name: "every forward node", Path: stats("1"), Seed: seedBalancers(listening, closed)},
		{Name: "an unknown load balancer", Path: stats("99"), Seed: seedBalancers(listening, closed)},
		{Name: "not a number", Path: stats("x"), Seed: seedBalancers(listening, closed)},
	})
	check := func(id string) string { return "/api/v2/admin/loadbalancers/" + id + "/check" }
	kernelWrite(t, kernelRoute("POST", "/api/v2/admin/loadbalancers/:id/check", native.LoadBalancerCheckRouteID,
		func(c *gin.Context) { handler.NewLoadBalancerHandler().RunHealthCheck(c) }), []packagecompat.Case{
		{Name: "every forward node checked and recorded", Path: check("1"), Seed: seedBalancers(listening, closed), Snapshot: forwardState},
		{Name: "health checks off", Path: check("2"), Seed: seedBalancers(listening, closed), Snapshot: forwardState},
		{Name: "an unknown load balancer", Path: check("99"), Seed: seedBalancers(listening, closed), Snapshot: forwardState},
		{Name: "not a number", Path: check("x"), Seed: seedBalancers(listening, closed), Snapshot: forwardState},
	})
}
