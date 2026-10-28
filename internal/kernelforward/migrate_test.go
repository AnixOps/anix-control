package kernelforward

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// legacyGost stores route as a gost route kept from before gost was
// removed: it is created on nftables, then rewritten in place.
func (f *fixture) legacyGost(requestID string, route *forwardv1.Route) *forwardv1.Route {
	f.t.Helper()
	nft := proto.Clone(route).(*forwardv1.Route)
	for _, hop := range nft.Hops {
		hop.Engine = forwardv1.Engine_ENGINE_NFTABLES
		if len(hop.NodeRefs) > 1 {
			hop.NodeRefs = hop.NodeRefs[:1]
		}
	}
	nft.Policy = nil
	created := f.create(requestID, nft)
	stored := proto.Clone(route).(*forwardv1.Route)
	stored.Id, stored.Revision = created.GetId(), created.GetRevision()
	stored.CreatedAtUnixMs, stored.UpdatedAtUnixMs = created.GetCreatedAtUnixMs(), created.GetUpdatedAtUnixMs()
	encoded, err := encodeRoute(stored)
	require.NoError(f.t, err)
	require.NoError(f.t, f.db.Model(&model.KernelForwardRoute{}).Where("id = ?", stored.GetId()).Update("route_json", encoded).Error)
	return stored
}

// Control takes no new gost hop, on create or update.
func TestGostRoutesAreRefused(t *testing.T) {
	db := openSQLite(t)
	migrate(t, db)
	f := newFixture(t, db)
	caps := nftCaps(forwardv1.Engine_ENGINE_NFTABLES, forwardv1.Engine_ENGINE_GOST)
	f.hello(entry, caps)
	f.hello(exit, caps)
	_, err := f.service.CreateRoute(f.ctx, "gost-create", gostRoute(0))
	var refusal *RefusedError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "hops[0].engine", refusal.Violations[0].Field)
	require.Contains(t, refusal.Violations[0].Message, "ENGINE_GOST was removed")

	created := f.create("nft-create", twoHop(0))
	update := gostRoute(0)
	update.Id = created.GetId()
	_, err = f.service.UpdateRoute(f.ctx, "gost-update", update, created.GetRevision())
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "hops[0].engine", refusal.Violations[0].Field)
}

func gostRoute(port uint32) *forwardv1.Route {
	route := twoHop(port)
	for _, hop := range route.Hops {
		hop.Engine = forwardv1.Engine_ENGINE_GOST
	}
	return route
}

// The gost routes move onto the network model in one go: a trusted
// private line gets a bare anx_line transport with the hop components it
// needs, the exit an internet egress, the entry keeps its allocated port;
// nftables routes and routes the model cannot carry stay.
func TestMigrateGostRoutes(t *testing.T) {
	db := openSQLite(t)
	migrate(t, db)
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	f := newFixture(t, db)
	caps := nftCaps(forwardv1.Engine_ENGINE_NFTABLES, forwardv1.Engine_ENGINE_GOST)
	f.hello(entry, caps)
	f.hello(exit, caps)
	require.NoError(t, db.Model(&model.KernelForwardNode{}).Where("node_ref IN ?", []string{entry.String(), exit.String()}).
		Update("labels_json", `{"link":"iepl"}`).Error)
	f.service.NetworkKEK = func() string { return testKEK }
	network := f.service.network(db)
	_, err := network.Replace(f.ctx, &networkmodel.Document{Lines: []networkmodel.Line{
		{ID: "ln-sh", Node: entry.String(), Interface: "eth0", Families: []string{"v4"}, GatewayV4: "192.0.2.1"},
		{ID: "ln-hk", Node: exit.String(), Interface: "eth0", Families: []string{"v4"}, GatewayV4: "192.0.2.1"},
	}}, nil, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)

	// Routes stored before gost was removed: created on nftables, then
	// rewritten in place, as Control no longer takes a gost hop.
	gost := f.legacyGost("migrate-1", gostRoute(0))
	nft := f.create("migrate-2", twoHop(0))
	ha := gostRoute(0)
	ha.Hops[1].NodeRefs = []string{exit.String(), "forward-13"}
	ha.Policy = &forwardv1.Policy{NextHop: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER}
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 13).Update("enabled", true).Error)
	f.hello(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 13}, caps)
	blocked := f.legacyGost("migrate-3", ha)
	var port uint32
	for _, allocation := range f.allocations() {
		if allocation.RouteID == gost.GetId() && allocation.HopIndex == 0 {
			port = allocation.Port
		}
	}
	require.NotZero(t, port)

	// The routes left on gost are not planned, show why, and keep their
	// ports.
	_, err = f.service.Replan(f.ctx, "test")
	require.NoError(t, err)
	var row model.KernelForwardRoute
	require.NoError(t, db.Where("id = ?", gost.GetId()).First(&row).Error)
	require.Equal(t, EnforcedGostRemoved, row.Enforced)
	for _, hop := range f.state(entry).GetHops() {
		require.NotEqual(t, gost.GetId(), hop.GetRouteId(), "a gost route is not planned")
	}
	held := false
	for _, allocation := range f.allocations() {
		if allocation.RouteID == gost.GetId() && allocation.HopIndex == 0 {
			held = allocation.Port == port && allocation.ReleasedAt == nil
		}
	}
	require.True(t, held, "a gost route keeps its entry port")

	dry, err := f.service.MigrateGostRoutes(f.ctx, true, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)
	require.False(t, dry.Applied)
	require.Len(t, dry.Routes, 1)
	require.Equal(t, gost.GetId(), dry.Routes[0].ID)
	require.Equal(t, []string{"mig-tr-forward-11-forward-12"}, dry.Routes[0].Transports)
	require.Equal(t, "mig-eg-internet-forward-12", dry.Routes[0].Egress)
	require.Len(t, dry.Blocked, 1)
	require.Equal(t, blocked.GetId(), dry.Blocked[0].ID)
	require.Contains(t, dry.Blocked[0].Reasons[0], "served by 2 nodes")
	require.Equal(t, 1, dry.Untouched)
	require.ElementsMatch(t, []string{
		"egress mig-eg-hop-forward-11", "ingress mig-in-hop-forward-12",
		"transport mig-tr-forward-11-forward-12", "egress mig-eg-internet-forward-12",
	}, dry.Added)
	stored, err := f.service.GetRoute(f.ctx, gost.GetId())
	require.NoError(t, err)
	require.Equal(t, forwardv1.Engine_ENGINE_GOST, stored.GetHops()[0].GetEngine(), "a dry run changes nothing")

	applied, err := f.service.MigrateGostRoutes(f.ctx, false, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)
	require.True(t, applied.Applied)
	stored, err = f.service.GetRoute(f.ctx, gost.GetId())
	require.NoError(t, err)
	require.Equal(t, forwardv1.Engine_ENGINE_NETWORK, stored.GetHops()[1].GetEngine())
	require.Equal(t, port, stored.GetListen().GetPort(), "the clients keep their port")
	require.Equal(t, "mig-tr-forward-11-forward-12", stored.GetHops()[1].GetIngress().GetNetworkTransportId())
	stored, err = f.service.GetRoute(f.ctx, nft.GetId())
	require.NoError(t, err)
	require.Equal(t, forwardv1.Engine_ENGINE_NFTABLES, stored.GetHops()[0].GetEngine())

	view, err := network.Get(f.ctx)
	require.NoError(t, err)
	require.Len(t, view.Document.Transports, 1)
	require.True(t, view.Document.Transports[0].Trusted)
	require.Equal(t, "anx_line", view.Document.Transports[0].Kind)
	require.Equal(t, "192.0.2.12", view.Document.Ingresses[0].PublicHost)
	require.Len(t, view.Pending, 1, "the exit has not reported its ANX-Line key")
	require.Equal(t, gost.GetId(), view.Pending[0].Forward)

	require.NoError(t, network.SetPublicKeys(f.ctx, exit.String(), map[string]string{"anx_line": "anx1-exit"}))
	plan, _, err := network.DevicePlan(f.ctx, entry.String())
	require.NoError(t, err)
	require.Len(t, plan.GetPortForwards(), 1)
	require.Equal(t, port, plan.GetPortForwards()[0].GetListen().GetPort())
	require.NotEmpty(t, f.state(entry).GetHops(), "the nftables route stays")

	// Nothing is left to move; the blocked route stays blocked.
	again, err := f.service.MigrateGostRoutes(f.ctx, false, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)
	require.Empty(t, again.Routes)
	require.Len(t, again.Blocked, 1)
}
