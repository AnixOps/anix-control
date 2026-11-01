package routesubscription

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "routes.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.ForwardNode{}, &model.KernelForwardRoute{},
		&model.KernelForwardAllocation{}, &model.ForwardRouteSubscription{}, &model.OperationLog{}))
	// KFC JP: Shadowsocks on 11111; the Shanghai forwarder newnb-1488.
	require.NoError(t, db.Create(&model.Node{ID: 50, Name: "KFC JP", Host: "104.251.233.29"}).Error)
	require.NoError(t, db.Create(&[]model.NodeProtocol{
		{ID: 501, NodeID: 50, Name: "SS", Type: model.ProtocolShadowsocks, Port: 11111, Enable: 1},
		{ID: 502, NodeID: 50, Name: "WG", Type: model.ProtocolWireGuard, Port: 443, Enable: 1},
	}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "newnb-1488", Host: "sh.example.com", Port: 7000, Enabled: true}).Error)
	return db
}

func storeRoute(t *testing.T, db *gorm.DB, route *forwardv1.Route, enforced string) {
	t.Helper()
	encoded, err := protojson.Marshal(route)
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, db.Save(&model.KernelForwardRoute{ID: route.GetId(), Owner: "admin", Revision: 1, RouteJSON: string(encoded), Enforced: enforced, CreatedAt: now, UpdatedAt: now}).Error)
}

func kfcRoute(port uint32) *forwardv1.Route {
	return &forwardv1.Route{
		Id: "r1", Owner: "admin", Name: "KFC-JP",
		Listen:  &forwardv1.Listen{Port: 15301, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP},
		Hops:    []*forwardv1.Hop{{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, NodeRefs: []string{"forward-1"}}},
		Targets: []*forwardv1.Target{{Host: "104.251.233.29", Port: port}},
	}
}

func TestPublishOffersTheProtocolAtTheEntry(t *testing.T) {
	db := newDB(t)
	s := New(db)
	ctx := context.Background()

	// The target port has no protocol: the route cannot be published.
	storeRoute(t, db, kfcRoute(11113), "")
	status, err := s.Get(ctx, "r1")
	require.NoError(t, err)
	require.False(t, status.Published)
	require.Nil(t, status.Protocol)
	require.Equal(t, ReasonNoProtocol, status.Reason)
	_, err = s.Set(ctx, "r1", true, "", Actor{UserID: 1, Name: "admin"})
	require.ErrorIs(t, err, ErrInvalid)

	storeRoute(t, db, kfcRoute(11111), "")
	require.NoError(t, db.Create(&model.KernelForwardAllocation{RouteID: "r1", HopIndex: 0, NodeRef: "forward-1", Port: 15301, Mark: 1, UpdatedAt: time.Now()}).Error)
	status, err = s.Set(ctx, "r1", true, " 上海 9929 ", Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.True(t, status.Published)
	require.Equal(t, "上海 9929", status.DisplayName)
	require.Equal(t, uint(501), status.Protocol.ID)
	require.Equal(t, "KFC JP", status.Protocol.NodeName)
	require.Equal(t, []Entry{{NodeRef: "forward-1", NodeName: "newnb-1488", Host: "sh.example.com", Port: 15301}}, status.Entries)
	require.Empty(t, status.Reason)

	offers, err := Offers(db)
	require.NoError(t, err)
	require.Equal(t, map[uint][]Offer{501: {{RouteID: "r1", Label: "上海 9929", Host: "sh.example.com", Port: 15301}}}, offers)

	// Paused routes are not offered; they stay published.
	paused := kfcRoute(11111)
	paused.Paused = true
	storeRoute(t, db, paused, "")
	offers, err = Offers(db)
	require.NoError(t, err)
	require.Empty(t, offers)
	storeRoute(t, db, kfcRoute(11111), "quota")
	offers, err = Offers(db)
	require.NoError(t, err)
	require.Empty(t, offers)

	// A UDP-only route cannot carry Shadowsocks.
	udp := kfcRoute(11111)
	udp.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	storeRoute(t, db, udp, "")
	status, err = s.Get(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, ReasonL4Mismatch, status.Reason)

	// Withdrawn.
	storeRoute(t, db, kfcRoute(11111), "")
	status, err = s.Set(ctx, "r1", false, "", Actor{})
	require.NoError(t, err)
	require.False(t, status.Published)
	offers, err = Offers(db)
	require.NoError(t, err)
	require.Empty(t, offers)

	var logs []model.OperationLog
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 2)

	_, err = s.Get(ctx, "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Set(ctx, "r1", true, "a|b", Actor{})
	require.ErrorIs(t, err, ErrInvalid)
}

// A route whose last hop runs on the proxy node itself reaches its
// protocol at a loopback target.
func TestLoopbackTargetOnTheExitNode(t *testing.T) {
	db := newDB(t)
	route := kfcRoute(443)
	route.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	route.Listen.Address = "203.0.113.9"
	route.Hops = []*forwardv1.Hop{
		{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, NodeRefs: []string{"forward-1"}},
		{Role: forwardv1.HopRole_HOP_ROLE_EXIT, NodeRefs: []string{"proxy-50"}},
	}
	route.Targets = []*forwardv1.Target{{Host: "127.0.0.1", Port: 443}}
	storeRoute(t, db, route, "")
	status, err := New(db).Get(context.Background(), "r1")
	require.NoError(t, err)
	require.Equal(t, uint(502), status.Protocol.ID)
	require.Empty(t, status.Reason)
	require.Equal(t, []Entry{{NodeRef: "forward-1", NodeName: "newnb-1488", Host: "203.0.113.9", Port: 15301}}, status.Entries)
}

func TestOffersWithoutTheTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	offers, err := Offers(db)
	require.NoError(t, err)
	require.Empty(t, offers)
}

// A route with an entry hostname (entry HA) is offered once, at the name.
func TestEntryHostname(t *testing.T) {
	db := newDB(t)
	route := kfcRoute(11111)
	route.Listen.EntryHostname = "JP.Example.com."
	route.Hops[0].NodeRefs = []string{"forward-1", "forward-2"}
	storeRoute(t, db, route, "")
	status, err := New(db).Get(context.Background(), "r1")
	require.NoError(t, err)
	require.Equal(t, []Entry{{NodeName: "JP.Example.com", Host: "jp.example.com", Port: 15301}}, status.Entries)
}

func TestEntriesFollowTheRouteOverrideThenTheNodeNAT(t *testing.T) {
	w := &world{
		nodes:       map[uint]model.Node{1: {ID: 1, Name: "lq", Host: "10.0.0.2"}},
		forwards:    map[uint]model.ForwardNode{},
		allocations: map[string]map[string]uint32{},
		nat: map[uint][]model.NodeEntry{1: {{Kind: model.NodeEntryNAT, Enabled: true, Host: "198.51.100.7",
			PortMap: `[{"from":24000,"to":24010,"external":31000}]`}}},
	}
	route := &forwardv1.Route{
		Listen: &forwardv1.Listen{Port: 24005},
		Hops:   []*forwardv1.Hop{{NodeRefs: []string{"proxy-1"}}},
	}
	if got := w.entries(route, "r1"); len(got) != 1 || got[0].Host != "198.51.100.7" || got[0].Port != 31005 {
		t.Fatalf("NAT entry: %+v", got)
	}
	route.Labels = map[string]string{"entry-public/proxy-1": "203.0.113.9:25001"}
	if got := w.entries(route, "r1"); len(got) != 1 || got[0].Host != "203.0.113.9" || got[0].Port != 25001 {
		t.Fatalf("route override: %+v", got)
	}
}
