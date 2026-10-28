package kernelforward

import (
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// networkFixture is the fixture with a network model of proxy-21 alone,
// whose internet egress is eg-hk.
func networkFixture(t *testing.T) (*fixture, *gorm.DB, *networkmodel.Service) {
	t.Helper()
	db := openSQLite(t)
	migrate(t, db)
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	f := newFixture(t, db)
	network := networkmodel.New(db)
	_, err := network.Replace(f.ctx, &networkmodel.Document{
		Lines: []networkmodel.Line{{ID: "ln-hk", Node: "proxy-21", Interface: "eth0", Families: []string{"v4"}, GatewayV4: "203.0.113.1"}},
		Egresses: []networkmodel.Egress{
			{ID: "eg-hk", Line: "ln-hk", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "auto"},
		},
	}, nil, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)
	return f, db, network
}

// networkRoute fills route in as a one-hop ENGINE_NETWORK route on
// proxy-21 leaving by eg-hk.
func networkRoute(route *forwardv1.Route) *forwardv1.Route {
	route.Owner, route.Name = "admin", "hk-ssh"
	route.Hops = []*forwardv1.Hop{{
		Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NETWORK,
		NodeRefs: []string{proxy.String()}, NetworkEgressId: "eg-hk",
	}}
	if route.Listen == nil {
		route.Listen = &forwardv1.Listen{Port: 20022, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP}
	}
	if route.Targets == nil {
		route.Targets = []*forwardv1.Target{{Host: "198.51.100.10", Port: 22}}
	}
	return route
}

// An ENGINE_NETWORK route is no NodeForwardState: it is a port forward of
// its device's NetworkPlan, which changes with the route.
func TestNetworkRouteIsAPortForward(t *testing.T) {
	f, db, network := networkFixture(t)
	var device model.NetworkDevice
	require.NoError(t, db.First(&device, "node_ref = ?", proxy.String()).Error)
	before := device.Generation

	route := f.create("network-1", networkRoute(&forwardv1.Route{
		Listen:  &forwardv1.Listen{Port: 20022, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 22, Weight: 3}, {Host: "198.51.100.11", Port: 22}},
		Policy: &forwardv1.Policy{
			Target: forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER, ProxyProtocol: forwardv1.ProxyProtocol_PROXY_PROTOCOL_V1,
		},
		Limits: &forwardv1.Limits{MaxConns: 50},
	}))
	_, found, err := f.service.State(f.ctx, proxy.String())
	require.NoError(t, err)
	require.False(t, found, "no NodeForwardState")

	plan, found, err := network.DevicePlan(f.ctx, proxy.String())
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, plan.GetPortForwards(), 1)
	forward := plan.GetPortForwards()[0]
	require.Equal(t, route.GetId(), forward.GetId())
	require.Equal(t, "eg-hk", forward.GetEgressId())
	require.Equal(t, "::", forward.GetListen().GetAddress())
	require.Equal(t, []string{"tcp", "udp"}, forward.GetProtocols())
	require.Len(t, forward.GetTargets(), 2)
	require.Equal(t, uint32(3), forward.GetTargets()[0].GetWeight())
	require.Equal(t, "failover", forward.GetPolicy().GetBalance())
	require.Equal(t, "v1", forward.GetPolicy().GetProxyProtocol())
	require.Equal(t, uint32(50), forward.GetPolicy().GetMaxConns())
	require.NoError(t, db.First(&device, "node_ref = ?", proxy.String()).Error)
	require.Equal(t, before+1, device.Generation, "the route changed the device's plan")

	// Deleting the route takes the port forward away in the same plan.
	require.NoError(t, f.service.DeleteRoute(f.ctx, "network-2", route.GetId()))
	plan, _, err = network.DevicePlan(f.ctx, proxy.String())
	require.NoError(t, err)
	require.Empty(t, plan.GetPortForwards())
	require.NoError(t, db.First(&device, "node_ref = ?", proxy.String()).Error)
	require.Equal(t, before+2, device.Generation)
}

func TestNetworkRouteIsValidated(t *testing.T) {
	db := openSQLite(t)
	migrate(t, db)
	f := newFixture(t, db)
	_, err := f.service.CreateRoute(f.ctx, "network-1", &forwardv1.Route{
		Owner: "admin", Name: "mixed",
		Listen: &forwardv1.Listen{Port: 20022, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{
			{Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES, NodeRefs: []string{entry.String()}},
			{
				Role: forwardv1.HopRole_HOP_ROLE_EXIT, Engine: forwardv1.Engine_ENGINE_NETWORK, NodeRefs: []string{proxy.String()},
				Ingress:         &forwardv1.LinkTransport{Security: forwardv1.LinkSecurity_LINK_SECURITY_NETWORK, NetworkTransportId: "tr-hk"},
				NetworkEgressId: "eg-hk",
			},
		},
		Targets: []*forwardv1.Target{{Host: "198.51.100.10", Port: 22}},
	})
	require.ErrorContains(t, err, "forward route refused")
}

// The port forwards' bytes meter the route's entry hop: a lower value
// starts a new epoch, an older status is skipped, and reaching the quota
// pauses the route, which takes its port forward away.
func TestNetworkCountersMeterTheEntry(t *testing.T) {
	f, _, network := networkFixture(t)
	route := f.create("network-1", networkRoute(&forwardv1.Route{Limits: &forwardv1.Limits{QuotaBytes: 1000}}))
	at := start.Add(time.Minute)
	record := func(at time.Time, up, down uint64) ReportResult {
		t.Helper()
		result, err := f.service.RecordNetworkCounters(f.ctx, proxy, []*networkv1.PortForwardStatus{
			{Id: route.GetId(), UpBytes: up, DownBytes: down}, {Id: "01JUNKNOWNR0UTE00000000000", UpBytes: 1},
		}, at)
		require.NoError(t, err)
		return result
	}
	used := func() uint64 {
		t.Helper()
		stats, err := f.service.RouteStats(f.ctx, route.GetId())
		require.NoError(t, err)
		var sum uint64
		for _, counter := range stats {
			sum += counter.GetUpBytes() + counter.GetDownBytes()
		}
		return sum
	}
	require.Equal(t, 1, record(at, 300, 300).Metered)
	require.Equal(t, 0, record(at.Add(-time.Second), 10, 10).Metered, "an older status is skipped")
	require.Equal(t, 1, record(at.Add(time.Minute), 100, 100).Metered, "the kernel restarted")
	require.Equal(t, uint64(800), used())
	plan, _, err := network.DevicePlan(f.ctx, proxy.String())
	require.NoError(t, err)
	require.Len(t, plan.GetPortForwards(), 1)

	result := record(at.Add(2*time.Minute), 400, 400)
	require.True(t, result.Replanned)
	require.Equal(t, uint64(1400), used())
	plan, _, err = network.DevicePlan(f.ctx, proxy.String())
	require.NoError(t, err)
	require.Empty(t, plan.GetPortForwards(), "over its quota")
}
