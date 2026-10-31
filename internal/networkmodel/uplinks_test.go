package networkmodel

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func dualUplinks() NodeSettings {
	return NodeSettings{Uplinks: []Uplink{
		{Name: "cn2", Label: "CN2", Interface: "eth0", SourceV4: "10.0.0.2", GatewayV4: "10.0.0.1"},
		{Name: "cu", Label: "9929", Interface: "eth1", SourceV4: "10.0.1.2", GatewayV4: "10.0.1.1"},
	}}
}

// uplinkStatus is a status whose uplink egresses towards proxy-2 report
// rtts (0: down).
func uplinkStatus(rtts map[string]uint32) *networkv1.NetworkStatus {
	status := &networkv1.NetworkStatus{DeviceRef: "proxy-1"}
	for uplink, rtt := range rtts {
		c := &networkv1.ComponentStatus{Component: networkv1.Component_COMPONENT_EGRESS, Id: uplinkEgressID("proxy-1", uplink, "proxy-2", "v4"),
			State: networkv1.ComponentState_COMPONENT_STATE_APPLIED, RttMs: rtt}
		if rtt == 0 {
			c.State = networkv1.ComponentState_COMPONENT_STATE_DOWN
		}
		status.Components = append(status.Components, c)
	}
	return status
}

func entryEgress(t *testing.T, s *Service) string {
	t.Helper()
	plan, found, err := s.DevicePlan(context.Background(), "proxy-1")
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, network.Check(plan))
	return plan.GetTransports()[0].GetFromEgressId()
}

// A dual-NIC entry probes the next device from each uplink, starts on the
// fastest, switches only to a clearly faster one held for a while, leaves
// a down one at once, and follows a pinned choice.
func TestUplinksProbeAndSwitch(t *testing.T) {
	s := newService(t)
	now := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	seenAgents(t, s, 1, 2)
	ctx := context.Background()
	s.Forwards = func(*gorm.DB) ([]Forward, error) { return []Forward{autoForward()}, nil }
	_, err := s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)
	require.NoError(t, s.SetPublicKeys(ctx, "proxy-2", map[string]string{"anx_line": "anx1-japan"}))

	_, err = s.SetNodeSettings(ctx, "proxy-1", dualUplinks(), Actor{Name: "admin"})
	require.NoError(t, err)
	plan, _, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.NoError(t, network.Check(plan))
	egresses := map[string]*networkv1.Egress{}
	for _, e := range plan.GetEgresses() {
		egresses[e.GetId()] = e
	}
	cn2, cu := egresses["auto-proxy-1-up-cn2-proxy-2"], egresses["auto-proxy-1-up-cu-proxy-2"]
	require.NotNil(t, cn2)
	require.NotNil(t, cu)
	require.Equal(t, "10.0.1.2", cu.GetSource())
	require.Equal(t, []string{"h.example.com:24443"}, cu.GetHealth().GetTargets())
	lines := map[string]*networkv1.Line{}
	for _, l := range plan.GetLines() {
		lines[l.GetId()] = l
	}
	require.Equal(t, "eth1", lines[cu.GetLineId()].GetInterface())
	require.Equal(t, "10.0.1.1", lines[cu.GetLineId()].GetGatewayV4())
	require.Equal(t, cn2.GetId(), entryEgress(t, s), "before any probe: the first uplink")

	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	record := func(rtts map[string]uint32) {
		t.Helper()
		require.NoError(t, s.RecordStatus(ctx, node, uplinkStatus(rtts), now))
	}
	record(map[string]uint32{"cn2": 80, "cu": 30})
	require.Equal(t, cu.GetId(), entryEgress(t, s), "the first probes: the fastest")

	// Not clearly faster: stays.
	record(map[string]uint32{"cn2": 22, "cu": 30})
	require.Equal(t, cu.GetId(), entryEgress(t, s))
	// Clearly faster, but not for long enough.
	for range uplinkStreak {
		record(map[string]uint32{"cn2": 10, "cu": 30})
	}
	require.Equal(t, cu.GetId(), entryEgress(t, s), "held for uplinkHold")
	now = now.Add(uplinkHold)
	record(map[string]uint32{"cn2": 10, "cu": 30})
	require.Equal(t, cn2.GetId(), entryEgress(t, s))

	// Down: left at once.
	record(map[string]uint32{"cn2": 0, "cu": 30})
	require.Equal(t, cu.GetId(), entryEgress(t, s))

	view, err := s.NodeNetwork(ctx, "proxy-1")
	require.NoError(t, err)
	require.Len(t, view.UplinkHops, 1)
	hop := view.UplinkHops[0]
	require.Equal(t, "proxy-2", hop.To)
	require.Equal(t, "n", hop.ToName)
	require.Equal(t, UplinkAuto, hop.Choice)
	require.Equal(t, "cu", hop.Active)
	require.Equal(t, "h.example.com:24443", hop.Target)
	require.Contains(t, hop.Reason, "cn2 is down")
	require.Len(t, hop.Probes, 2)

	// Pinned: the choice, whatever the probes say.
	pinned := dualUplinks()
	pinned.UplinkChoices = map[string]string{"proxy-2": "cn2"}
	_, err = s.SetNodeSettings(ctx, "proxy-1", pinned, Actor{})
	require.NoError(t, err)
	require.Equal(t, cn2.GetId(), entryEgress(t, s))
	record(map[string]uint32{"cn2": 0, "cu": 30})
	require.Equal(t, cn2.GetId(), entryEgress(t, s))
	got, err := s.NodeSettings(ctx, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"proxy-2": "cn2"}, got.UplinkChoices)
	require.Len(t, got.Uplinks, 2)

	// No uplinks: the device's own hop egress again.
	_, err = s.SetNodeSettings(ctx, "proxy-1", NodeSettings{}, Actor{})
	require.NoError(t, err)
	require.Equal(t, "auto-proxy-1-hop", entryEgress(t, s))
}

func TestParseUplinkEgress(t *testing.T) {
	uplink, to, family, ok := parseUplinkEgress("proxy-62", uplinkEgressID("proxy-62", "cn2", "proxy-50", "v6"))
	require.True(t, ok)
	require.Equal(t, []string{"cn2", "proxy-50", "v6"}, []string{uplink, to, family})
	_, to, family, ok = parseUplinkEgress("proxy-62", "auto-proxy-62-up-cu-forward-3")
	require.True(t, ok)
	require.Equal(t, []string{"forward-3", "v4"}, []string{to, family})
	for _, id := range []string{"auto-proxy-62-hop", "auto-proxy-6-up-cu-proxy-50", "auto-proxy-62-up-cu-x"} {
		_, _, _, ok := parseUplinkEgress("proxy-62", id)
		require.False(t, ok, id)
	}
}

func TestCheckUplinks(t *testing.T) {
	for name, settings := range map[string]NodeSettings{
		"bad name":      {Uplinks: []Uplink{{Name: "CN-2", Interface: "eth0", SourceV4: "10.0.0.2"}}},
		"twice":         {Uplinks: []Uplink{{Name: "a", Interface: "eth0", SourceV4: "10.0.0.2"}, {Name: "a", Interface: "eth1", SourceV4: "10.0.1.2"}}},
		"no source":     {Uplinks: []Uplink{{Name: "a", Interface: "eth0"}}},
		"no interface":  {Uplinks: []Uplink{{Name: "a", SourceV4: "10.0.0.2"}}},
		"bad default":   {Uplinks: []Uplink{{Name: "a", Interface: "eth0", SourceV4: "10.0.0.2"}}, UplinkDefault: "b"},
		"bad choice":    {Uplinks: []Uplink{{Name: "a", Interface: "eth0", SourceV4: "10.0.0.2"}}, UplinkChoices: map[string]string{"proxy-2": "b"}},
		"bad next node": {UplinkChoices: map[string]string{"jp": "auto"}},
	} {
		_, err := checkSettings(settings)
		var invalid *Invalid
		require.ErrorAs(t, err, &invalid, name)
	}
	ok, err := checkSettings(NodeSettings{Uplinks: []Uplink{{Name: " CN2 ", Interface: "eth0", SourceV6: "2001:db8::2"}},
		UplinkDefault: "cn2", UplinkChoices: map[string]string{"proxy-2": "auto", "proxy-3": ""}})
	require.NoError(t, err)
	require.Equal(t, "cn2", ok.Uplinks[0].Name)
	require.Equal(t, map[string]string{"proxy-2": "auto"}, ok.UplinkChoices)
}

func TestNextChoice(t *testing.T) {
	now := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)
	up := func(name string, rtt uint32) UplinkProbe { return UplinkProbe{Uplink: name, State: "up", RTTMs: rtt} }
	unknown := []UplinkProbe{{Uplink: "a", State: "unknown"}, {Uplink: "b", State: "unknown"}}
	got, switched := nextChoice(activeUplink{Uplink: "a", Since: now}, unknown, now)
	require.False(t, switched)
	require.Equal(t, "a", got.Uplink)
	// A faster one that falls back loses its streak.
	current := activeUplink{Uplink: "a", Since: now.Add(-time.Hour)}
	current, _ = nextChoice(current, []UplinkProbe{up("a", 100), up("b", 50)}, now)
	require.Equal(t, 1, current.Streak)
	current, _ = nextChoice(current, []UplinkProbe{up("a", 100), up("b", 95)}, now)
	require.Zero(t, current.Streak)
}
