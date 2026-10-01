// Package forwardcompat proves the forward package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL: the
// same bytes (times the handlers take from their clock masked) and the same
// resulting rows.
//
// The native side reads the forward node, runtime settings, user directory
// and entitlement views the kernel publishes
// (packagestore.EnsureKernelAPIViews), and resets a subscriber's traffic
// through the real KernelSubscriber server (internal/kernelsubscriber) in
// process over gRPC, on the native side's database, so both sides end with
// the same counters, request ledger and change log.
package forwardcompat

import (
	"context"
	"fmt"
	"net"
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
	"github.com/AnixOps/anix-control/v4/packages/forward/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

var (
	admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	// alice holds permissions for alpha (speed limit "fast"), beta
	// (disabled) and gamma (a disabled tunnel), and four forwards.
	alice = pluginhostsdk.Principal{ActorID: 2}
	// bob holds a permission for alpha and two forwards.
	bob = pluginhostsdk.Principal{ActorID: 3}
	// carol holds nothing.
	carol = pluginhostsdk.Principal{ActorID: 4}
)

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// forwardHost is the identity the kernel serves the forward host as.
var forwardHost = packagebridge.HostIdentity{PackageID: "forward", Version: "4.0.0", Generation: 1}

// trafficOnly authorizes what the forward package's signed release
// declares: the traffic family, for the forward host.
type trafficOnly struct{}

func (trafficOnly) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == forwardHost && capability == service.CapabilitySubscriberTraffic {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// kernelSubscriber serves the kernel's KernelSubscriber on db in process and
// returns a client for it, as the forward host gets one over its bridge.
func kernelSubscriber(t *testing.T, db *gorm.DB) kernelsubscriberv1.KernelSubscriberClient {
	t.Helper()
	server := &kernelsubscriber.Server{DB: db, Authorizer: trafficOnly{}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server.For(forwardHost))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelsubscriberv1.NewKernelSubscriberClient(conn)
}

var models = []any{
	&model.Plan{}, &model.User{}, &model.ForwardNode{}, &model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{},
	&model.ForwardPortBinding{}, &model.SpeedLimit{}, &model.ForwardLatencyBucket{}, &model.ForwardRule{},
	&model.ForwardRuntimeJob{}, &model.SystemConfig{}, &model.SubscriberRequest{}, &model.SubscriberChange{},
}

func route(t *testing.T, method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy, Models: models,
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open:       func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Subscriber: kernelSubscriber(t, db),
			}
			handler, ok := service.Handlers()[routeID]
			require.True(t, ok, "no native handler for %s", routeID)
			return handler
		},
	}
}

// forwardRoute is a route of the kernel's ForwardHandler. The legacy
// handlers are built per request: their services keep the database they
// were built with, which the harness sets up per case.
func forwardRoute(t *testing.T, method, pattern, routeID string, legacy func(*handler.ForwardHandler, *gin.Context)) packagecompat.Route {
	return route(t, method, pattern, routeID, func(c *gin.Context) { legacy(handler.NewForwardHandler(), c) })
}

func ptr[T any](value T) *T { return &value }

// seedWith writes users, forward nodes (with API tokens), tunnels,
// permissions, speed limits, forwards, latency buckets, legacy rules and
// the given runtime settings, then the kernel views.
func seedWith(settings map[string]string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		user := func(id uint, speed *int64) model.User {
			return model.User{
				ID: id, Email: fmt.Sprintf("user%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id),
				SpeedLimit: speed, TransferEnable: 1 << 40, U: int64(id) * 1000, D: int64(id) * 3000,
				IsAdmin: map[bool]int{true: 1}[id == 1], CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.User{user(1, nil), user(2, ptr(int64(2048))), user(3, nil), user(4, ptr(int64(0)))}).Error)

		node := func(id uint, name, kind, host string, enabled bool, status int) model.ForwardNode {
			return model.ForwardNode{
				ID: id, Name: name, Type: kind, Host: host, Port: 8000 + int(id), APIPort: 9000 + int(id),
				APIToken: fmt.Sprintf("node-%d-secret-token", id), MetricsPort: 9100, Region: "HK", ISP: "isp", Datacenter: "dc",
				Bandwidth: 1000, Status: status, LastCheck: seeded.Add(time.Duration(id) * time.Minute), Latency: int(id),
				Load: 0.25, Uptime: 99.5, Tags: `["edge"]`, Weight: 2, MaxConn: 100, Enabled: enabled,
				TotalUpload: int64(id) * 100, TotalDownload: int64(id) * 1000, CurrentConn: int(id),
				CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		nodes := []model.ForwardNode{
			node(1, "relay-a", "relay", " 198.51.100.1 ", true, 1),
			node(2, "exit-b", "exit", "198.51.100.2", true, 0),
			node(3, "relay-off", "relay", "198.51.100.3", true, 1),
			node(4, "relay-mixed-case", "Relay ", "198.51.100.4", true, 1),
			node(5, "exit-e", "exit", "198.51.100.5", true, 1),
		}
		// GORM writes no zero value over a column default, so a disabled
		// node is disabled after its creation.
		require.NoError(t, db.Create(&nodes).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 3).UpdateColumn("enabled", false).Error)

		tunnel := func(id uint, name string, in uint, out *uint, kind int, status int) model.ForwardTunnel {
			return model.ForwardTunnel{
				ID: id, Name: name, InNodeID: in, OutNodeID: out, InIP: fmt.Sprintf("198.51.100.%d", in), OutIP: "198.51.100.2",
				InNodePortSta: ptr(20000), InNodePortEnd: ptr(20100), Type: kind, Flow: 2, Protocol: "tcp", TrafficRatio: 1.5,
				InterfaceName: "eth0", TCPListenAddr: "0.0.0.0", UDPListenAddr: "[::]", Status: status,
				CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
			}
		}
		tunnels := []model.ForwardTunnel{
			tunnel(1, "alpha", 1, ptr(uint(1)), 1, 1),
			tunnel(2, "beta", 1, ptr(uint(2)), 2, 1),
			tunnel(3, "gamma", 1, ptr(uint(1)), 1, 1),
			tunnel(4, "delta", 0, ptr(uint(1)), 1, 1),
			tunnel(5, "epsilon", 1, ptr(uint(1)), 1, 1),
			tunnel(6, "zeta", 1, nil, 2, 1),
			tunnel(7, "Alpha", 1, ptr(uint(1)), 1, 1),
		}
		tunnels[1].Protocol = "tls"
		tunnels[1].TrafficRatio = 0
		require.NoError(t, db.Create(&tunnels).Error)
		require.NoError(t, db.Model(&model.ForwardTunnel{}).Where("id = ?", 3).UpdateColumn("status", 0).Error)

		require.NoError(t, db.Create(&[]model.SpeedLimit{
			{ID: 1, CreatedTime: 1, UpdatedTime: 2, Status: 1, Name: "fast", Speed: 5000, TunnelID: 1, TunnelName: "alpha"},
			{ID: 2, CreatedTime: 1, UpdatedTime: 2, Status: 1, Name: "slow", Speed: 100, TunnelID: 2, TunnelName: "beta"},
		}).Error)

		permission := func(id, user, tunnel uint, speed *uint, inFlow int64) model.ForwardUserTunnel {
			return model.ForwardUserTunnel{
				ID: id, UserID: user, TunnelID: tunnel, Flow: 10, Num: 5, InFlow: inFlow, OutFlow: 7, FlowResetTime: 3,
				ExpTime: 1893456000000, SpeedID: speed, Status: 1, CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.ForwardUserTunnel{
			permission(1, 2, 1, ptr(uint(1)), 5),
			permission(2, 2, 2, nil, 900000),
			permission(3, 3, 1, ptr(uint(2)), 0),
			permission(4, 2, 3, nil, 0),
		}).Error)
		// PostgreSQL keeps the foreign keys GORM creates, so rows that
		// outlived their user, tunnel or node exist only on SQLite.
		orphans := db.Name() != "postgres"
		if orphans {
			require.NoError(t, db.Create(&[]model.ForwardUserTunnel{permission(5, 99, 4, ptr(uint(9)), 0)}).Error)
		}
		require.NoError(t, db.Model(&model.ForwardUserTunnel{}).Where("id = ?", 2).UpdateColumn("status", 0).Error)

		synced := seeded.Add(30 * time.Minute)
		forward := func(id, owner, tunnel uint, remote, strategy string, inx int) model.Forward {
			return model.Forward{
				ID: id, UserID: owner, UserName: fmt.Sprintf("user%d@example.test", owner), Name: fmt.Sprintf("forward-%d", id),
				TunnelID: tunnel, InPort: 20000 + int(id), RemoteAddr: remote, InterfaceName: "eth1", Strategy: strategy,
				Status: 1, RuntimeBackend: "gost", RuntimeStatus: 2, RuntimeMessage: "synced", RuntimeLastSyncAt: &synced,
				InFlow: int64(id) * 100, OutFlow: int64(id) * 10, Inx: inx,
				CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
			}
		}
		forwards := []model.Forward{
			forward(1, 2, 1, "203.0.113.1:443", "round", 1),
			forward(2, 2, 1, "203.0.113.1:443,203.0.113.2:443", "round", 0),
			forward(3, 3, 1, "203.0.113.3:443", "hash", 0),
			forward(4, 2, 2, "203.0.113.4:443,203.0.113.5:443", "bogus", 2),
			forward(6, 2, 1, "203.0.113.7:443", "fifo", 0),
		}
		forwards[4].RuntimeLastSyncAt = nil
		require.NoError(t, db.Create(&forwards).Error)
		if orphans {
			require.NoError(t, db.Create(&[]model.Forward{forward(5, 3, 99, "203.0.113.6:443", "fifo", 1)}).Error)
		}

		bucket := func(id uint, kind string, target uint, host string, at time.Time, success int) model.ForwardLatencyBucket {
			return model.ForwardLatencyBucket{
				ID: id, TargetKey: fmt.Sprintf("%s:%d:%s:443", kind, target, host), TargetType: kind, TargetID: target,
				Label: fmt.Sprintf("%s-%d", kind, target), Host: host, Port: 443, BucketAt: at, IntervalSeconds: 60,
				SampleCount: 10, SuccessCount: success, MinRTT: 1, AvgRTT: float64(id) + 0.5, MaxRTT: 9, P95RTT: 8, LossPct: 2.5,
				CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.ForwardLatencyBucket{
			bucket(1, "tunnel_node", 1, "198.51.100.1", seeded, 10),
			bucket(2, "tunnel_node", 1, "198.51.100.1", seeded.Add(time.Minute), 0),
			bucket(3, "forward_node", 1, "198.51.100.1", seeded, 10),
			bucket(4, "tunnel_node", 0, "198.51.100.9", seeded, 10),
			// Other targets with the same ids, which the ingress latencies
			// leave out.
			bucket(5, "node", 1, "198.51.100.10", seeded, 3),
			bucket(6, "forward", 1, "203.0.113.1", seeded, 4),
			bucket(7, "forward_node", 0, "198.51.100.11", seeded, 5),
		}).Error)
		for id := uint(8); id < 16; id++ {
			require.NoError(t, db.Create(ptr(bucket(id, "node", 1, fmt.Sprintf("198.51.100.%d", 100+id), seeded, 1))).Error)
		}

		expires := seeded.Add(48 * time.Hour)
		rule := func(id uint, owner *uint, relay, exit uint, port int) model.ForwardRule {
			return model.ForwardRule{
				ID: id, Name: fmt.Sprintf("rule-%d", id), Enabled: true, RelayNodeID: relay, ListenPort: port, Protocol: "tcp",
				ExitNodeID: exit, TargetHost: "203.0.113.9", TargetPort: 8443, UserID: owner, AllowedIPs: "10.0.0.0/8",
				SpeedLimit: ptr(int64(512)), TrafficLimit: ptr(int64(1 << 30)), ExpireTime: &expires, Upload: 5, Download: 6,
				Connections: 1, TotalConns: 9, Remark: "remark", CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.ForwardRule{
			rule(1, ptr(uint(2)), 1, 2, 30001), rule(2, ptr(uint(3)), 1, 5, 30002), rule(3, nil, 1, 2, 30003),
		}).Error)
		if orphans {
			require.NoError(t, db.Create(&[]model.ForwardRule{rule(4, ptr(uint(2)), 99, 5, 30004)}).Error)
		}

		configs := []model.SystemConfig{
			{ID: 1, Key: "forward.runtime.nodex.token", Value: "nodex-secret", CreatedAt: seeded, UpdatedAt: seeded},
		}
		next := uint(2)
		for key, value := range settings {
			configs = append(configs, model.SystemConfig{ID: next, Key: key, Value: value, CreatedAt: seeded, UpdatedAt: seeded})
			next++
		}
		require.NoError(t, db.Create(&configs).Error)

		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		syncSequences(t, db)
	}
}

var seed = seedWith(nil)

func empty(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1}).Error)
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{
		"v2_user", "v2_forward_node", "v2_forward_tunnel", "v2_forward_user_tunnel", "v2_forward", "v2_speed_limit",
		"v2_forward_latency_bucket", "v2_forward_rule", "v2_system_config",
	} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// clockTime is a time the handlers take from their clock: masked unless it
// was seeded.
func clockTime(value time.Time) string {
	if !value.IsZero() && value.Sub(seeded) >= 0 && value.Sub(seeded) <= 72*time.Hour {
		return value.UTC().Format(time.RFC3339)
	}
	if d := time.Since(value); d > -10*time.Minute && d < 10*time.Minute {
		return "<now>"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// state is everything the native routes can change: forwards, tunnels,
// permissions, the subscribers' counters, the request ledger and the change
// log.
func state(t testing.TB, db *gorm.DB) any {
	var forwards []model.Forward
	require.NoError(t, db.Order("id").Find(&forwards).Error)
	forwardRows := make([]map[string]any, 0, len(forwards))
	for _, f := range forwards {
		forwardRows = append(forwardRows, map[string]any{
			"id": f.ID, "user_id": f.UserID, "tunnel_id": f.TunnelID, "in_port": f.InPort, "status": f.Status, "inx": f.Inx,
			"in_flow": f.InFlow, "out_flow": f.OutFlow, "created_at": clockTime(f.CreatedAt), "updated_at": clockTime(f.UpdatedAt),
		})
	}
	var tunnels []model.ForwardTunnel
	require.NoError(t, db.Order("id").Find(&tunnels).Error)
	tunnelRows := make([]map[string]any, 0, len(tunnels))
	for _, tunnel := range tunnels {
		tunnelRows = append(tunnelRows, map[string]any{
			"id": tunnel.ID, "name": tunnel.Name, "in_node_id": tunnel.InNodeID, "out_node_id": tunnel.OutNodeID, "in_ip": tunnel.InIP,
			"out_ip": tunnel.OutIP, "in_node_port_sta": tunnel.InNodePortSta, "in_node_port_end": tunnel.InNodePortEnd,
			"type": tunnel.Type, "flow": tunnel.Flow, "protocol": tunnel.Protocol, "traffic_ratio": tunnel.TrafficRatio,
			"interface_name": tunnel.InterfaceName, "tcp_listen_addr": tunnel.TCPListenAddr, "udp_listen_addr": tunnel.UDPListenAddr,
			"status": tunnel.Status, "created_at": clockTime(tunnel.CreatedAt), "updated_at": clockTime(tunnel.UpdatedAt),
		})
	}
	var permissions []model.ForwardUserTunnel
	require.NoError(t, db.Order("id").Find(&permissions).Error)
	permissionRows := make([]map[string]any, 0, len(permissions))
	for _, p := range permissions {
		permissionRows = append(permissionRows, map[string]any{
			"id": p.ID, "user_id": p.UserID, "tunnel_id": p.TunnelID, "flow": p.Flow, "num": p.Num, "in_flow": p.InFlow,
			"out_flow": p.OutFlow, "flow_reset_time": p.FlowResetTime, "exp_time": p.ExpTime, "speed_id": p.SpeedID,
			"status": p.Status, "created_at": clockTime(p.CreatedAt), "updated_at": clockTime(p.UpdatedAt),
		})
	}
	var users []struct {
		ID   uint
		U, D int64
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
	var changes []struct {
		UserID  uint
		Deleted bool
	}
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
	var nodeTokens []string
	require.NoError(t, db.Model(&model.ForwardNode{}).Order("id").Pluck("api_token", &nodeTokens).Error)
	return map[string]any{
		"forwards": forwardRows, "tunnels": tunnelRows, "permissions": permissionRows, "users": users, "ledger": ledger,
		"changes": changes, "node_tokens": nodeTokens,
	}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Snapshot = state
		packagecompat.RunWrite(t, r, c)
	}
}

func TestForwardListParity(t *testing.T) {
	for _, r := range []packagecompat.Route{
		forwardRoute(t, "POST", "/api/v2/forward/list", "forward.forward.list.post", (*handler.ForwardHandler).ListPanelForwards),
		forwardRoute(t, "POST", "/api/v2/admin/forward/list", "forward.admin.forward.list.post", (*handler.ForwardHandler).ListPanelForwards),
	} {
		read(t, r, []packagecompat.Case{
			{Name: r.RouteID + " the caller's forwards in order", Path: r.Pattern, Principal: alice},
			{Name: r.RouteID + " another user's", Path: r.Pattern, Principal: bob},
			{Name: r.RouteID + " none", Path: r.Pattern, Principal: carol},
			{Name: r.RouteID + " an administrator sees every forward", Path: r.Pattern, Principal: admin},
			{Name: r.RouteID + " no data", Path: r.Pattern, Principal: admin, Seed: empty},
		})
	}
}

func TestForwardOrderParity(t *testing.T) {
	for _, r := range []packagecompat.Route{
		forwardRoute(t, "POST", "/api/v2/forward/update-order", "forward.forward.update_order.post", (*handler.ForwardHandler).UpdatePanelForwardOrder),
		forwardRoute(t, "POST", "/api/v2/admin/forward/update-order", "forward.admin.forward.update_order.post", (*handler.ForwardHandler).UpdatePanelForwardOrder),
	} {
		write(t, r, []packagecompat.Case{
			{Name: r.RouteID + " the caller's forwards", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":[{"id":1,"inx":5},{"id":2,"inx":-1}]}`)},
			{Name: r.RouteID + " another user's forward", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":[{"id":1,"inx":5},{"id":3,"inx":1}]}`)},
			{Name: r.RouteID + " an administrator orders anyone's", Path: r.Pattern, Principal: admin, Body: []byte(`{"forwards":[{"id":1,"inx":5},{"id":3,"inx":1}]}`)},
			{Name: r.RouteID + " an unknown forward", Path: r.Pattern, Principal: admin, Body: []byte(`{"forwards":[{"id":99,"inx":1}]}`)},
			{Name: r.RouteID + " a forward twice", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":[{"id":1,"inx":1},{"id":1,"inx":2}]}`)},
			{Name: r.RouteID + " an empty list", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":[]}`)},
			{Name: r.RouteID + " no list", Path: r.Pattern, Principal: alice, Body: []byte(`{}`)},
			{Name: r.RouteID + " a list that is not a list", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":{"id":1}}`)},
			{Name: r.RouteID + " no body", Path: r.Pattern, Principal: alice},
			{Name: r.RouteID + " a body that does not parse", Path: r.Pattern, Principal: alice, Body: []byte(`{"forwards":`)},
		})
	}
}

func TestAdminTunnelListParity(t *testing.T) {
	r := forwardRoute(t, "POST", "/api/v2/admin/tunnel/list", "forward.admin.tunnel.list.post", (*handler.ForwardHandler).ListPanelAdminTunnels)
	read(t, r, []packagecompat.Case{
		{Name: "every tunnel", Path: r.Pattern, Principal: admin},
		{Name: "no tunnels", Path: r.Pattern, Principal: admin, Seed: empty},
	})
}

// The backend settings each case runs with.
var (
	nodeXOn      = map[string]string{"forward.runtime.nodex_mode": "true"}
	nodeXOff     = map[string]string{"forward.runtime.nodex_mode": "off", "forward.runtime_backend": "gost"}
	ansibleOwn   = map[string]string{"forward.runtime.nodex_mode": "0", "forward.runtime.ansible.backend": "iptables_ansible"}
	cleanAgent   = map[string]string{"forward.runtime_backend": " Clean_Agent "}
	iptables     = map[string]string{"forward.runtime_backend": "iptables_ansible"}
	badBackend   = map[string]string{"forward.runtime_backend": "nodex"}
	badNodeXMode = map[string]string{"forward.runtime.nodex_mode": "maybe"}
)

func TestTunnelsForForwardsParity(t *testing.T) {
	for _, r := range []packagecompat.Route{
		forwardRoute(t, "POST", "/api/v2/tunnel/user/tunnel", "forward.tunnel.user.tunnel.post", (*handler.ForwardHandler).ListPanelTunnels),
		forwardRoute(t, "POST", "/api/v2/admin/tunnel/user/tunnel", "forward.admin.tunnel.user.tunnel.post", (*handler.ForwardHandler).ListPanelTunnels),
	} {
		read(t, r, []packagecompat.Case{
			{Name: r.RouteID + " the caller's tunnels", Path: r.Pattern, Principal: alice},
			{Name: r.RouteID + " another user's", Path: r.Pattern, Principal: bob},
			{Name: r.RouteID + " none", Path: r.Pattern, Principal: carol},
			{Name: r.RouteID + " no caller", Path: r.Pattern, Principal: pluginhostsdk.Principal{}},
			{Name: r.RouteID + " an administrator", Path: r.Pattern, Principal: admin},
			{Name: r.RouteID + " NodeX mode", Path: r.Pattern, Principal: admin, Seed: seedWith(nodeXOn)},
			{Name: r.RouteID + " NodeX mode off", Path: r.Pattern, Principal: admin, Seed: seedWith(nodeXOff)},
			{Name: r.RouteID + " the Ansible backend", Path: r.Pattern, Principal: admin, Seed: seedWith(ansibleOwn)},
			{Name: r.RouteID + " clean agents", Path: r.Pattern, Principal: alice, Seed: seedWith(cleanAgent)},
			{Name: r.RouteID + " iptables", Path: r.Pattern, Principal: admin, Seed: seedWith(iptables)},
			{Name: r.RouteID + " an unknown backend", Path: r.Pattern, Principal: admin, Seed: seedWith(badBackend)},
			{Name: r.RouteID + " an unknown NodeX mode", Path: r.Pattern, Principal: alice, Seed: seedWith(badNodeXMode)},
			{Name: r.RouteID + " no data", Path: r.Pattern, Principal: admin, Seed: empty},
		})
	}
}

func TestTunnelCreateParity(t *testing.T) {
	r := forwardRoute(t, "POST", "/api/v2/admin/tunnel/create", "forward.admin.tunnel.create.post", (*handler.ForwardHandler).CreatePanelTunnel)
	created := []string{"data.createdTime", "data.updatedTime"}
	ok := func(name, request string, settings map[string]string) packagecompat.Case {
		return packagecompat.Case{Name: name, Path: r.Pattern, Principal: admin, Body: []byte(request), Mask: created, Seed: seedWith(settings)}
	}
	refused := func(name, request string, settings map[string]string) packagecompat.Case {
		return packagecompat.Case{Name: name, Path: r.Pattern, Principal: admin, Body: []byte(request), Seed: seedWith(settings)}
	}
	write(t, r, []packagecompat.Case{
		ok("port forwarding", `{"name":" omega ","inNodeId":1,"type":1,"flow":1,"interfaceName":" eth9 ","tcpListenAddr":" 0.0.0.0 "}`, nil),
		ok("tunnel forwarding", `{"name":"omega","inNodeId":1,"outNodeId":2,"type":2,"flow":2,"trafficRatio":2.5}`, nil),
		ok("tunnel forwarding with a protocol", `{"name":"omega","inNodeId":4,"outNodeId":5,"type":2,"flow":2,"protocol":" mtls "}`, nil),
		ok("a ratio of zero is one", `{"name":"omega","inNodeId":1,"type":1,"flow":2,"trafficRatio":0}`, nil),
		ok("the largest ratio", `{"name":"omega","inNodeId":1,"type":1,"flow":2,"trafficRatio":100}`, nil),
		refused("a ratio too large", `{"name":"omega","inNodeId":1,"type":1,"flow":2,"trafficRatio":100.5}`, nil),
		refused("a negative ratio", `{"name":"omega","inNodeId":1,"type":1,"flow":2,"trafficRatio":-1}`, nil),
		refused("the same node twice", `{"name":"omega","inNodeId":1,"outNodeId":1,"type":2,"flow":2}`, nil),
		refused("no exit node", `{"name":"omega","inNodeId":1,"type":2,"flow":2}`, nil),
		refused("an exit node that is a relay", `{"name":"omega","inNodeId":1,"outNodeId":4,"type":2,"flow":2}`, nil),
		refused("an entry node that is an exit", `{"name":"omega","inNodeId":2,"type":1,"flow":2}`, nil),
		refused("a disabled entry node", `{"name":"omega","inNodeId":3,"type":1,"flow":2}`, nil),
		refused("an unknown entry node", `{"name":"omega","inNodeId":99,"type":1,"flow":2}`, nil),
		refused("an unknown exit node", `{"name":"omega","inNodeId":1,"outNodeId":99,"type":2,"flow":2}`, nil),
		refused("no entry node", `{"name":"omega","type":1,"flow":2}`, nil),
		refused("a name in use", `{"name":" alpha ","inNodeId":1,"type":1,"flow":2}`, nil),
		refused("no name", `{"name":"  ","inNodeId":1,"type":1,"flow":2}`, nil),
		refused("an unknown type", `{"name":"omega","inNodeId":1,"type":3,"flow":2}`, nil),
		refused("an unknown flow", `{"name":"omega","inNodeId":1,"type":1,"flow":0}`, nil),
		ok("Ansible: the entry node runs it", `{"name":"omega","inNodeId":1,"type":1,"flow":2,"protocol":"tls"}`, ansibleOwn),
		ok("Ansible: the execution node", `{"name":"omega","outNodeId":4,"type":1,"flow":2}`, nodeXOff),
		ok("clean agents: the execution node wins", `{"name":"omega","inNodeId":5,"outNodeId":1,"type":1,"flow":2}`, cleanAgent),
		refused("Ansible: tunnel forwarding", `{"name":"omega","inNodeId":1,"outNodeId":2,"type":2,"flow":2}`, ansibleOwn),
		refused("Ansible: no node", `{"name":"omega","type":1,"flow":2}`, ansibleOwn),
		refused("Ansible: an exit node", `{"name":"omega","inNodeId":2,"type":1,"flow":2}`, iptables),
		refused("Ansible: a disabled node", `{"name":"omega","inNodeId":3,"type":1,"flow":2}`, cleanAgent),
		refused("an unknown backend", `{"name":"omega","inNodeId":1,"type":1,"flow":2}`, badBackend),
		refused("an unknown NodeX mode", `{"name":"omega","inNodeId":1,"type":1,"flow":2}`, badNodeXMode),
		refused("a type that is a string", `{"name":"omega","inNodeId":1,"type":"1","flow":2}`, nil),
		refused("no body", ``, nil),
		refused("a body that does not parse", `{"name":`, nil),
	})
}

func TestTunnelDeleteParity(t *testing.T) {
	r := forwardRoute(t, "POST", "/api/v2/admin/tunnel/delete", "forward.admin.tunnel.delete.post", (*handler.ForwardHandler).DeletePanelTunnel)
	write(t, r, []packagecompat.Case{
		{Name: "an unused tunnel", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":5}`)},
		{Name: "a tunnel with forwards", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1}`)},
		{Name: "a tunnel with permissions only", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":4}`)},
		{Name: "a disabled tunnel with a permission", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":3}`)},
		{Name: "an unknown tunnel", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":99}`)},
		{Name: "tunnel zero", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":0}`)},
		{Name: "an id that is a string", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":"5"}`)},
		{Name: "a negative id", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":-5}`)},
		{Name: "no body", Path: r.Pattern, Principal: admin},
	})
}

func TestUserTunnelAssignParity(t *testing.T) {
	for _, r := range []packagecompat.Route{
		forwardRoute(t, "POST", "/api/v2/tunnel/user/assign", "forward.tunnel.user.assign.post", (*handler.ForwardHandler).AssignPanelUserTunnel),
		forwardRoute(t, "POST", "/api/v2/admin/tunnel/user/assign", "forward.admin.tunnel.user.assign.post", (*handler.ForwardHandler).AssignPanelUserTunnel),
	} {
		write(t, r, []packagecompat.Case{
			{Name: r.RouteID + " a permission", Path: r.Pattern, Principal: admin,
				Body: []byte(`{"userId":3,"tunnelId":2,"flow":20,"num":3,"flowResetTime":1,"expTime":1893456000000,"speedId":2}`)},
			{Name: r.RouteID + " without a speed limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4,"tunnelId":1,"speedId":0}`)},
			{Name: r.RouteID + " on a disabled tunnel", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4,"tunnelId":3}`)},
			{Name: r.RouteID + " twice", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":2,"tunnelId":1}`)},
			{Name: r.RouteID + " an unknown user", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":99,"tunnelId":1}`)},
			{Name: r.RouteID + " an unknown tunnel", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4,"tunnelId":99}`)},
			{Name: r.RouteID + " an unknown speed limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4,"tunnelId":1,"speedId":9}`)},
			{Name: r.RouteID + " another tunnel's speed limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4,"tunnelId":1,"speedId":2}`)},
			{Name: r.RouteID + " no user", Path: r.Pattern, Principal: admin, Body: []byte(`{"tunnelId":1}`)},
			{Name: r.RouteID + " no tunnel", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4}`)},
			{Name: r.RouteID + " a user id that is a string", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":"4","tunnelId":1}`)},
			{Name: r.RouteID + " no body", Path: r.Pattern, Principal: admin},
		})
	}
}

func TestUserTunnelListParity(t *testing.T) {
	for _, r := range []packagecompat.Route{
		forwardRoute(t, "POST", "/api/v2/tunnel/user/list", "forward.tunnel.user.list.post", (*handler.ForwardHandler).ListPanelUserTunnels),
		forwardRoute(t, "POST", "/api/v2/admin/tunnel/user/list", "forward.admin.tunnel.user.list.post", (*handler.ForwardHandler).ListPanelUserTunnels),
	} {
		// The list raises a permission's traffic to its forwards' totals.
		write(t, r, []packagecompat.Case{
			{Name: r.RouteID + " a user's permissions", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":2}`)},
			{Name: r.RouteID + " another user's", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":3}`)},
			{Name: r.RouteID + " a user without a row", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":99}`)},
			{Name: r.RouteID + " a user without permissions", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":4}`)},
			{Name: r.RouteID + " no user", Path: r.Pattern, Principal: admin, Body: []byte(`{}`)},
			{Name: r.RouteID + " a user id that is a string", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":"2"}`)},
			{Name: r.RouteID + " no body", Path: r.Pattern, Principal: admin},
			{Name: r.RouteID + " no data", Path: r.Pattern, Principal: admin, Body: []byte(`{"userId":1}`), Seed: empty},
		})
	}
}

func TestMultiIngressParity(t *testing.T) {
	r := forwardRoute(t, "GET", "/api/v2/admin/forward/observability/multi-ingress", "forward.admin.forward.observability.multi_ingress.get",
		(*handler.ForwardHandler).GetObservabilityMultiIngress)
	path := func(query string) string { return r.Pattern + query }
	read(t, r, []packagecompat.Case{
		{Name: "a forward's ingress latencies", Path: path("?targetId=1"), Principal: admin},
		{Name: "a tunnel without an exit node", Path: path("?targetId=4"), Principal: admin},
		{Name: "a forward whose tunnel is gone", Path: path("?targetId=5"), Principal: admin},
		{Name: "an unknown forward", Path: path("?targetId=99"), Principal: admin},
		{Name: "no target", Path: path(""), Principal: admin},
		{Name: "target zero", Path: path("?targetId=0"), Principal: admin},
		{Name: "a target that is not a number", Path: path("?targetId=x"), Principal: admin},
		{Name: "a target beyond 32 bits", Path: path("?targetId=4294967297"), Principal: admin},
		{Name: "no buckets", Path: path("?targetId=1"), Principal: admin, Seed: func(t testing.TB, db *gorm.DB) {
			seed(t, db)
			require.NoError(t, db.Where("1 = 1").Delete(&model.ForwardLatencyBucket{}).Error)
		}},
	})
}

func TestStatsParity(t *testing.T) {
	r := forwardRoute(t, "GET", "/api/v2/admin/forward/stats", "forward.admin.forward.stats.get", (*handler.ForwardHandler).GetForwardStats)
	read(t, r, []packagecompat.Case{
		{Name: "enabled nodes", Path: r.Pattern, Principal: admin},
		{Name: "no nodes", Path: r.Pattern, Principal: admin, Seed: empty},
	})
}

func TestUserRulesParity(t *testing.T) {
	r := forwardRoute(t, "GET", "/api/v2/user/forward/rules", "forward.user.forward.rules.get", (*handler.ForwardHandler).GetUserRules)
	read(t, r, []packagecompat.Case{
		{Name: "the caller's rules and their nodes", Path: r.Pattern, Principal: alice},
		{Name: "another user's", Path: r.Pattern, Principal: bob},
		{Name: "none", Path: r.Pattern, Principal: carol},
		{Name: "no data", Path: r.Pattern, Principal: carol, Seed: empty},
	})
}

// Neither side shows a node's API token to a user.
func TestUserRulesShowNoNodeToken(t *testing.T) {
	r := forwardRoute(t, "GET", "/api/v2/user/forward/rules", "forward.user.forward.rules.get", (*handler.ForwardHandler).GetUserRules)
	r.Native = func(db *gorm.DB) pluginhostsdk.NativeHandler {
		handler := (&native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}).Handlers()[r.RouteID]
		return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
			response, err := handler(ctx, request)
			require.NoError(t, err)
			require.NotContains(t, string(response.Body), "secret-token")
			require.Contains(t, string(response.Body), `"api_token":""`)
			return response, err
		}
	}
	packagecompat.RunRead(t, r, packagecompat.Case{Name: "no token", Path: r.Pattern, Principal: alice, Seed: seed})
}

func TestResetParity(t *testing.T) {
	r := route(t, "POST", "/api/v2/user/reset", native.ResetRouteID, func(c *gin.Context) { handler.NewAdminHandler().ResetCompatFlow(c) })
	key := map[string]string{"Idempotency-Key": "reset-1"}
	write(t, r, []packagecompat.Case{
		{Name: "a subscriber's traffic", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":2,"type":1}`), RequestHeaders: key},
		{Name: "a retried reset applies once", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":2,"type":1}`), RequestHeaders: key,
			Warmup: [][]byte{[]byte(`{"id":2,"type":1}`)}},
		{Name: "the request id when there is no key", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":3,"type":1}`),
			RequestHeaders: map[string]string{"X-Request-Id": "request-7"}},
		{Name: "an unknown subscriber", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":99,"type":1}`), RequestHeaders: key},
		{Name: "a permission's traffic", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1,"type":2}`)},
		{Name: "a permission of a user without a row", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":5,"type":2}`)},
		{Name: "an unknown permission", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":99,"type":2}`)},
		{Name: "an unknown type", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1,"type":3}`)},
		{Name: "a negative type", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1,"type":-1}`)},
		{Name: "no type", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1}`)},
		{Name: "id zero", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":0,"type":1}`)},
		{Name: "an id that is a string", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":"2","type":1}`)},
		{Name: "no body", Path: r.Pattern, Principal: admin},
	})
}

// The legacy handler and the native one record a subscriber reset under the
// same ledger id, so a retry is applied once whichever side serves it.
func TestResetRequestIDsMatch(t *testing.T) {
	for _, token := range []string{"reset-1", "", strings.Repeat("k", 300)} {
		require.Equal(t, service.ForwardUserResetRequestID(7, token), native.ResetRequestID(7, token))
	}
	require.LessOrEqual(t, len(native.ResetRequestID(4294967295, "x")), 128)
}

// Every route the host serves natively has a parity test above.
func TestEveryNativeRouteIsCovered(t *testing.T) {
	covered := map[string]bool{
		"forward.forward.list.post": true, "forward.admin.forward.list.post": true, "forward.forward.update_order.post": true,
		"forward.admin.forward.update_order.post": true, "forward.admin.tunnel.list.post": true, "forward.admin.tunnel.create.post": true,
		"forward.admin.tunnel.delete.post": true, "forward.tunnel.user.tunnel.post": true, "forward.admin.tunnel.user.tunnel.post": true,
		"forward.tunnel.user.assign.post": true, "forward.admin.tunnel.user.assign.post": true, "forward.tunnel.user.list.post": true,
		"forward.admin.tunnel.user.list.post": true, "forward.admin.forward.observability.multi_ingress.get": true,
		"forward.admin.forward.stats.get": true, "forward.user.forward.rules.get": true, native.ResetRouteID: true,
	}
	handlers := (&native.Service{Subscriber: kernelsubscriberv1.NewKernelSubscriberClient(nil)}).Handlers()
	require.Len(t, handlers, len(covered))
	for routeID := range handlers {
		require.True(t, covered[routeID], routeID)
	}
}
