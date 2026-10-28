package kernelforward

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
)

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

	gost := f.create("migrate-1", gostRoute(0))
	nft := f.create("migrate-2", twoHop(0))
	ha := gostRoute(0)
	ha.Hops[1].NodeRefs = []string{exit.String(), "forward-13"}
	ha.Policy = &forwardv1.Policy{NextHop: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER}
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 13).Update("enabled", true).Error)
	f.hello(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 13}, caps)
	blocked := f.create("migrate-3", ha)
	var port uint32
	for _, allocation := range f.allocations() {
		if allocation.RouteID == gost.GetId() && allocation.HopIndex == 0 {
			port = allocation.Port
		}
	}
	require.NotZero(t, port)

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
