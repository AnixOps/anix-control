package proxynodecompat

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/proxy-node/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The node routes run through the whole path a deployment runs
// (packagecompat.RunKernelRead and RunKernelWrite): the kernel's gateway
// expands the handles an answer shows, the package host reads v2_node, which
// its lease adopts since the node credential split finalized it, and the
// kernel retires deleted nodes. The answers and, for the deletion, the rows,
// the kernel's secrets and credentials after it are the ones recorded in
// testdata while the routes still matched the kernel's legacy handlers.

// now is the clock the seeds' report times are relative to: a node's shown
// status depends on how long ago it checked in. The recorded answers hold 0
// for the report times (volatile).
var now = time.Now().Unix()

func volatile() []string {
	return []string{strconv.FormatInt(now-30, 10), strconv.FormatInt(now-7200, 10)}
}

// wireGuard holds two key pairs generated for this test and used nowhere
// else: fixed, so the recorded answers hold them.
var wireGuard = struct{ private, public, otherPrivate, otherPublic string }{
	private: "xFgrZYz2S27LdAUfmgvMBP9EVh6BYwOlMP5PgnlNy/Q=", public: "p1T3JyjXGiS31F8mQjgZ/jHyPzC/BcEdwPDQ0pdD5gU=",
	otherPrivate: "HgjMzty/73P8n/d1cSgyDaMp7tKkz+KjFg2lUwZOl5M=", otherPublic: "lcuwvHt27KTECRVvIHPzkpV3dgqLPM8frdRVjIVOqk4=",
}

func wireGuardRawConfig(private, public string) string {
	return fmt.Sprintf(`{"type":"wireguard","server_port":51820,"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24",`+
		`"server_private_key":%q,"server_public_key":%q,"tunnel_type":"quic","log":{"level":"info"}}`, private, public)
}

var adminModels = []any{
	&model.Node{}, &model.NodeProtocol{}, &model.NodeLog{}, &model.WireGuardPeer{}, &model.SubscriptionGroup{},
	&model.User{}, &model.ForwardNode{}, &model.AgentCertificate{}, &model.AgentEnrollment{},
}

func capabilities() []string {
	return []string{service.CapabilityNodeOpsNodeConfig}
}

// kernelRoute is a proxy-node route on the kernel's path.
func kernelRoute(method, pattern, routeID string) packagecompat.KernelRoute {
	models := append(append(append([]any{}, adminModels...), packagecompat.NodeSplitModels()...), append(model.KernelNodeOperationModels(), &model.EgressRoute{}, &model.NodeExitPolicy{})...)
	return packagecompat.KernelRoute{
		Route:     packagecompat.Route{Method: method, Pattern: pattern, RouteID: routeID, Models: models},
		PackageID: "proxy-node", Capabilities: capabilities(),
		NativeService: func(db *gorm.DB, nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
			return (&native.Service{
				Open:    func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Leased:  packagecompat.Lease("proxy-node"),
				NodeOps: nodeOps,
			}).Handlers()[routeID]
		},
	}
}

// seedFleet writes nodes 1 to 6: online, offline, pending, disabled, in
// two groups, with raw configurations (a WireGuard one with its keys, one
// that is not JSON), protocols with secrets, a WireGuard protocol with its
// users' peers and group links, and agent certificates.
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
	syncTables(t, db, "v2_node", "v2_node_protocol", "v2_wireguard_peer", "v2_subscription_group", "v2_user")
}

func wireGuardSettings(private, public string) string {
	return fmt.Sprintf(`{"cidr":"10.8.0.0/24","server_address":"10.8.0.1/24","server_private_key":%q,"server_public_key":%q,"tunnel_type":"quic",`+
		`"relay":{"server":"relay.example.test","server_port":8443}}`, private, public)
}

// seedFinalized is seedFleet with every table the routes read finalized.
func seedFinalized(t testing.TB, db *gorm.DB) {
	seedFleet(t, db)
	packagecompat.FinalizeNodeSplit(t, db, nodesecrets.TableNode, nodesecrets.TableNodeProtocol, nodesecrets.TableWireGuardPeer)
}

// fleetState is what the routes can change: the nodes as stored, the
// kernel's secrets and credentials for them, the protocols, peers and
// links that go with a node and the agent certificates. Times the handlers take from their clock read as
// "<clock>".
func fleetState(t testing.TB, db *gorm.DB) any {
	var nodes []model.Node
	require.NoError(t, db.Order("id").Find(&nodes).Error)
	nodeRows := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		created, updated := node.CreatedAt, node.UpdatedAt
		node.CreatedAt, node.UpdatedAt = time.Time{}, time.Time{}
		nodeRows = append(nodeRows, map[string]any{"row": node, "created_at": clock(created), "updated_at": clock(updated)})
	}
	var protocols []struct{ ID, NodeID uint }
	require.NoError(t, db.Model(&model.NodeProtocol{}).Order("id").Find(&protocols).Error)
	var secrets []struct {
		Scope, ColumnName, JSONPointer, Value string
		OwnerID                               uint
	}
	require.NoError(t, db.Model(&model.ProtocolSecret{}).Order("scope, owner_id, column_name, json_pointer").Find(&secrets).Error)
	var peers, links int64
	require.NoError(t, db.Model(&model.WireGuardPeer{}).Count(&peers).Error)
	require.NoError(t, db.Table("v2_subscription_group_node_protocols").Count(&links).Error)
	return map[string]any{"nodes": nodeRows, "protocols": protocols, "secrets": secrets, "peers": peers, "links": links}
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
		c.Volatile = volatile()
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
		c.Volatile = volatile()
		if c.Snapshot == nil {
			c.Snapshot = fleetState
		}
		packagecompat.RunKernelWrite(t, r, c)
	}
}

func TestNodeList(t *testing.T) {
	path := "/api/v2/admin/nodes"
	kernelRead(t, kernelRoute("GET", path, native.NodesRouteID), []packagecompat.Case{
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
		{Name: "by name", Path: path + "?sort=name"},
		{Name: "by name descending", Path: path + "?sort=name&order=desc"},
		{Name: "by id", Path: path + "?sort=id&order=desc"},
		{Name: "by the administrator's weight", Path: path + "?sort=sort&order=desc"},
		{Name: "by last check, nodes that never checked in last", Path: path + "?sort=last_check_at&order=asc"},
		{Name: "by last check descending, nodes that never checked in first", Path: path + "?sort=last_check_at&order=desc"},
		{Name: "by load", Path: path + "?sort=cpu_usage&order=desc"},
		{Name: "by online users", Path: path + "?sort=online_users&order=desc"},
		{Name: "by creation time", Path: path + "?sort=created_at"},
		{Name: "by host", Path: path + "?sort=host"},
		{Name: "sorted and paged", Path: path + "?sort=name&order=desc&page=2&page_size=2"},
		{Name: "sorted and filtered", Path: path + "?sort=name&search=jp&status=1"},
		{Name: "empty sort keeps the administrator's order", Path: path + "?sort=&order=desc"},
		{Name: "order without sort keeps the administrator's order", Path: path + "?order=desc"},
		{Name: "the shown status is not a sort column", Path: path + "?sort=status"},
		{Name: "a sort that is an injection", Path: path + "?sort=id%3B%20DROP%20TABLE%20v2_node"},
		{Name: "a direction that is not asc or desc", Path: path + "?sort=id&order=up"},
	})
}

func TestNodeDetail(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id }
	kernelRead(t, kernelRoute("GET", pattern, native.NodeRouteID), []packagecompat.Case{
		{Name: "with its protocols and raw configuration, masked", Path: path("1")},
		{Name: "a raw configuration that is not JSON", Path: path("3")},
		{Name: "a disabled node keeps its status", Path: path("4")},
		{Name: "an unknown node", Path: path("99")},
		{Name: "not a number", Path: path("x")},
		{Name: "beyond 32 bits", Path: path("4294967297")},
	})
}

func TestNodeDelete(t *testing.T) {
	pattern := "/api/v2/admin/nodes/:id"
	path := func(id string) string { return "/api/v2/admin/nodes/" + id }
	kernelWrite(t, kernelRoute("DELETE", pattern, native.DeleteNodeRouteID), []packagecompat.Case{
		{Name: "with its protocols, peers, links and credentials", Path: path("1")},
		{Name: "with a protocol", Path: path("2")},
		{Name: "without protocols", Path: path("6")},
		{Name: "an unknown node", Path: path("99")},
		{Name: "not a number", Path: path("x")},
	})
}
