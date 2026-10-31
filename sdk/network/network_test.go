package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func contractDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "contracts", "network", "v1"))
	require.NoError(t, err)
	return dir
}

func readPlan(t *testing.T, name string) *networkv1.NetworkPlan {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractDir(t), name))
	require.NoError(t, err)
	plan, err := DecodePlan(raw)
	require.NoError(t, err)
	return plan
}

var goldenPlans = []string{"plan-anx-line-ingress.json", "plan-anx-line-egress.json", "plan-dual-line-single-box.json"}

// TestGoldenPlans: every golden plan passes Check, is generation 1, and
// carries the hash HashOf computes, so the Agent and the kernel can check
// their own hashing against the same files.
func TestGoldenPlans(t *testing.T) {
	for _, name := range goldenPlans {
		t.Run(name, func(t *testing.T) {
			plan := readPlan(t, name)
			require.NoError(t, Check(plan))
			want := proto.Clone(plan).(*networkv1.NetworkPlan)
			Stamp(want, Generation{})
			require.Equal(t, uint64(1), plan.GetGeneration())
			require.Equal(t, want.GetHash(), plan.GetHash(), "a changed golden plan needs its new hash")
		})
	}
}

func TestGoldenCapabilitiesAndStatus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractDir(t), "capabilities-anixcore.json"))
	require.NoError(t, err)
	caps := &networkv1.DeviceCapabilities{}
	require.NoError(t, unmarshal.Unmarshal(raw, caps))
	require.NoError(t, CheckDeviceCapabilities(caps, "proxy-1"))

	for _, name := range goldenPlans[:2] {
		plan := readPlan(t, name)
		plan.DeviceRef = "proxy-1"
		require.Empty(t, Unsupported(plan, caps), name)
	}
	// The single box uses no transport but local, yet the device below
	// runs anx_line only: local is missing.
	only := &networkv1.DeviceCapabilities{Kinds: []*networkv1.KindSupport{
		{Component: networkv1.Component_COMPONENT_TRANSPORT, Kind: TransportAnxLine, Versions: []uint32{1}},
	}}
	require.Contains(t, Unsupported(readPlan(t, "plan-dual-line-single-box.json"), only), "COMPONENT_TRANSPORT local v1")

	raw, err = os.ReadFile(filepath.Join(contractDir(t), "status-anx-line-ingress.json"))
	require.NoError(t, err)
	status, err := DecodeStatus(raw, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, readPlan(t, "plan-anx-line-ingress.json").GetHash(), status.GetPlanHash())
	_, err = DecodeStatus(raw, "proxy-2")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestHelloCapabilityRoundTrip(t *testing.T) {
	caps := &networkv1.DeviceCapabilities{DeviceRef: "proxy-1", Kinds: []*networkv1.KindSupport{
		{Component: networkv1.Component_COMPONENT_TRANSPORT, Kind: TransportAnxLine, Versions: []uint32{1}, Executor: "anixcore"},
	}}
	capability, err := HelloCapability(caps)
	require.NoError(t, err)
	require.Equal(t, agentcontrol.CapabilityNetwork, capability.GetName())

	got, listed, err := DeviceCapabilitiesFromHello([]*agentv1pb.Capability{capability}, "proxy-1")
	require.NoError(t, err)
	require.True(t, listed)
	require.True(t, proto.Equal(caps, got))

	_, listed, err = DeviceCapabilitiesFromHello([]*agentv1pb.Capability{capability}, "proxy-2")
	require.True(t, listed)
	require.ErrorIs(t, err, ErrInvalid)

	_, listed, err = DeviceCapabilitiesFromHello(nil, "proxy-1")
	require.NoError(t, err)
	require.False(t, listed)

	bare := &agentv1pb.Capability{Name: agentcontrol.CapabilityNetwork, Version: agentcontrol.CapabilityVersionV1}
	_, listed, err = DeviceCapabilitiesFromHello([]*agentv1pb.Capability{bare}, "proxy-1")
	require.True(t, listed)
	require.ErrorIs(t, err, ErrInvalid)

	for name, bad := range map[string]*networkv1.DeviceCapabilities{
		"unknown component": {Kinds: []*networkv1.KindSupport{{Kind: "local", Versions: []uint32{1}}}},
		"bad kind":          {Kinds: []*networkv1.KindSupport{{Component: networkv1.Component_COMPONENT_EGRESS, Kind: "Hop", Versions: []uint32{1}}}},
		"no versions":       {Kinds: []*networkv1.KindSupport{{Component: networkv1.Component_COMPONENT_EGRESS, Kind: "hop"}}},
		"twice": {Kinds: []*networkv1.KindSupport{
			{Component: networkv1.Component_COMPONENT_EGRESS, Kind: "hop", Versions: []uint32{1}},
			{Component: networkv1.Component_COMPONENT_EGRESS, Kind: "hop", Versions: []uint32{2}},
		}},
		"routing range": {RoutingFirst: 30000, RoutingLast: 29999},
	} {
		require.ErrorIs(t, CheckDeviceCapabilities(bad, ""), ErrInvalid, name)
	}
}

func TestPlanMemberRoundTrip(t *testing.T) {
	plan := readPlan(t, "plan-anx-line-ingress.json")
	member, err := PlanMember(plan)
	require.NoError(t, err)
	document, err := json.Marshal(map[string]any{"version": 1, NodeConfigMember: member})
	require.NoError(t, err)
	got, found, err := PlanFromNodeConfig(document)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, proto.Equal(plan, got))
	require.Equal(t, plan.GetHash(), HashOf(got))

	_, found, err = PlanFromNodeConfig([]byte(`{"version":1}`))
	require.NoError(t, err)
	require.False(t, found)
	_, _, err = PlanFromNodeConfig([]byte(`[]`))
	require.ErrorIs(t, err, ErrInvalid)
}

func TestStamp(t *testing.T) {
	plan := readPlan(t, "plan-anx-line-egress.json")
	g := Stamp(plan, Generation{})
	require.Equal(t, uint64(1), g.Generation)
	require.Equal(t, g, Stamp(plan, g), "an unchanged plan keeps its generation")
	plan.Egresses[0].Source = "203.0.113.21"
	next := Stamp(plan, g)
	require.Equal(t, uint64(2), next.Generation)
	require.NotEqual(t, g.Hash, next.Hash)
}

func TestStatusReportRoundTrip(t *testing.T) {
	status := &networkv1.NetworkStatus{DeviceRef: "proxy-1", Components: []*networkv1.ComponentStatus{
		{Component: networkv1.Component_COMPONENT_EGRESS, Id: "eg-jp", State: networkv1.ComponentState_COMPONENT_STATE_APPLIED},
	}}
	report, err := StatusReport(status)
	require.NoError(t, err)
	require.True(t, IsStatusReport(report))
	got, err := DecodeStatus(report.GetPayloadJson(), "proxy-1")
	require.NoError(t, err)
	require.True(t, proto.Equal(status, got))

	status.Components[0].Id = "bad id"
	_, err = StatusReport(status)
	require.ErrorIs(t, err, ErrInvalid)
}

// An anx-server hybrid key (X25519 and ML-KEM-768) runs past MaxTextBytes
// in its text form; the status carries it, and the dialing end's plan.
func TestHybridServerKeyFits(t *testing.T) {
	key := "anx1-" + strings.Repeat("a", 2440)
	status := &networkv1.NetworkStatus{DeviceRef: "proxy-1", PublicKeys: []*networkv1.PublicKey{{Kind: TransportAnxLine, Key: key}}}
	_, err := StatusReport(status)
	require.NoError(t, err)

	plan := readPlan(t, "plan-anx-line-ingress.json")
	plan.Transports[0].GetAnxLine().ServerPublic = key
	require.NoError(t, Check(plan))

	status.PublicKeys[0].Key = strings.Repeat("a", MaxKeyBytes+1)
	_, err = StatusReport(status)
	require.ErrorIs(t, err, ErrInvalid)
}

// TestCheckRefuses: one broken copy of a golden plan per invariant, each
// refused with a message naming the problem.
func TestCheckRefuses(t *testing.T) {
	cases := []struct {
		name      string
		plan      string
		breakPlan func(p *networkv1.NetworkPlan)
		want      string
	}{
		{"no device", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.DeviceRef = "" }, "device_ref"},
		{"bad hash", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Hash = "XYZ" }, "hash"},
		{"duplicate line", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Lines = append(p.Lines, p.Lines[0]) }, "listed twice"},
		{"bad id", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Lines[0].Id = "a b" }, "must be 1 to 64"},
		{"egress on unknown line", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].LineId = "nope" }, "not a line"},
		{"source family", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].Source = "2001:db8::5" }, "not of family"},
		{"gateway family", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].Gateway = "2001:db8::1" }, "gateway"},
		{"hop kind audience", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].Kind = EgressHop }, "go together"},
		{"health timeout", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].Health.TimeoutMs = 20000 }, "timeout_ms"},
		{"health target", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Egresses[0].Health.Targets = []string{"1.1.1.1"} }, "host:port"},
		{"hop ingress pairing", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[0].Pairing = &networkv1.Pairing{Mode: networkv1.PairingMode_PAIRING_MODE_SAME_LINE}
		}, "no pairing"},
		{"bad peer", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Ingresses[0].AllowedPeers = []string{"x"} }, "allowed peer"},
		{"accept without hop ingress", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].ToIngressId = "" }, "hop ingress"},
		{"accept with peer key", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().ServerPublic = "anx1-key" }, "dialing end"},
		{"accept with bad listen address", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().TcpAddr = "nope" }, "address"},
		{"forward from unknown", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Forwarding[0].FromTransportId = "x" }, "accepting transport"},
		{"forward twice", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) { p.Forwarding = append(p.Forwarding, p.Forwarding[0]) }, "forwarded twice"},
		{"forward to hop egress", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) {
			p.Paths[0].Next = &networkv1.Path_EgressId{EgressId: "eg-ix"}
		}, "internet egress"},
		{"link id", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().LinkId = "7" }, "link_id"},
		{"link key", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().LinkKey = strings.Repeat("A", 64) }, "link_key"},
		{"no server key", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().ServerPublic = "" }, "server_public"},
		{"no carrier", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) {
			a := p.Transports[0].GetAnxLine()
			a.QuicAddr, a.FaketcpAddr, a.TcpAddr = "", "", ""
		}, "carrier address"},
		{"faketcp v6", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().FaketcpAddr = "[2001:db8::20]:8444" }, "IPv4 only"},
		{"carrier name", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].GetAnxLine().CarrierOrder = []string{"c9"} }, "carrier"},
		{"l2 needs server name", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[1].GetAnxLine().ServerName = "" }, "l2"},
		{"settings without kind", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].Kind = TransportWireGuard }, "kind anx_line only"},
		{"dial without hop egress", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].FromEgressId = "" }, "hop egress"},
		{"no peer device", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Transports[0].PeerDeviceRef = "" }, "peer_device_ref"},
		{"path from hop ingress", "plan-anx-line-egress.json", func(p *networkv1.NetworkPlan) {
			p.Paths = []*networkv1.Path{{Id: "p", IngressId: "in-hop", Next: &networkv1.Path_EgressId{EgressId: "eg-jp"}}}
		}, "not a user ingress"},
		{"path without next", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) { p.Paths[0].Next = nil }, "next is required"},
		{"path final mismatch", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) { p.Paths[0].FinalEgressId = "eg-9929" }, "final_egress_id"},
		{"local path without egress", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) { p.Paths[2].FinalEgressId = "" }, "local transport"},
		{"failover to no path", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[0].Pairing.EgressIds = append(p.Ingresses[0].Pairing.EgressIds, "eg-sg")
		}, "final egress"},
		{"failover twice", "plan-anx-line-ingress.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[0].Pairing.EgressIds = []string{"eg-jp", "eg-jp"}
		}, "twice"},
		{"cross takes one", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[1].Pairing.EgressIds = nil
		}, "exactly one"},
		{"user ingress unpaired", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) { p.Ingresses[0].Pairing = nil }, "needs a pairing"},
		{"rules without label", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[0].Pairing = &networkv1.Pairing{Mode: networkv1.PairingMode_PAIRING_MODE_RULES, Rules: []*networkv1.PairingRule{{Match: "domain_suffix:example.com", PathLabel: "jp"}}}
		}, "no path is labelled"},
		{"fallback elsewhere", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[0].Pairing = &networkv1.Pairing{Mode: networkv1.PairingMode_PAIRING_MODE_RULES, FallbackPathId: "pa-9929",
				Rules: []*networkv1.PairingRule{{Match: "domain_suffix:example.com", PathLabel: "jp"}}}
		}, "fallback_path_id"},
		{"listen taken", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[2].Listen = &networkv1.Endpoint{Address: "2001:db8:1::10", Port: 443}
		}, "taken"},
		{"port map length", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[1].PortMap[0].Local.Last = 20020
		}, "differ in length"},
		{"port map overlap", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) {
			p.Ingresses[1].PortMap = append(p.Ingresses[1].PortMap, &networkv1.PortMap{
				Public: &networkv1.PortRange{First: 30010, Last: 30030}, Local: &networkv1.PortRange{First: 21010, Last: 21030},
			})
		}, "overlap"},
		{"local role", "plan-dual-line-single-box.json", func(p *networkv1.NetworkPlan) { p.Transports[0].Kind = TransportWireGuard }, "role local"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := readPlan(t, c.plan)
			c.breakPlan(plan)
			err := Check(plan)
			require.ErrorIs(t, err, ErrInvalid)
			require.Contains(t, err.Error(), c.want)
		})
	}
}

// withForward adds a port forward over transport tr-jp to the ingress
// golden plan.
func withForward(t *testing.T) *networkv1.NetworkPlan {
	t.Helper()
	plan := readPlan(t, "plan-anx-line-ingress.json")
	plan.PortForwards = []*networkv1.PortForward{{
		Id: "rt-1", Listen: &networkv1.Endpoint{Address: "0.0.0.0", Port: 20001}, Protocols: []string{"tcp", "udp"},
		Targets: []*networkv1.ForwardTarget{{Host: "10.0.0.5", Port: 22}, {Host: "db.example.com", Port: 5432}},
		Next:    &networkv1.PortForward_TransportId{TransportId: "tr-jp"},
	}}
	return plan
}

func TestPortForwards(t *testing.T) {
	require.NoError(t, Check(withForward(t)))
	full := withForward(t)
	full.PortForwards[0].Targets[0].Weight, full.PortForwards[0].Targets[1].Priority = 2, 1
	full.PortForwards[0].Policy = &networkv1.ForwardPolicy{Balance: "failover", ProxyProtocol: "v2", BandwidthBps: 100_000_000, MaxConns: 500,
		Health:         &networkv1.ForwardHealth{IntervalMs: 5000, TimeoutMs: 2000},
		CircuitBreaker: &networkv1.ForwardBreaker{FailureThreshold: 3, OpenMs: 30000}}
	require.NoError(t, Check(full))

	cases := []struct {
		name      string
		breakPlan func(p *networkv1.NetworkPlan)
		want      string
	}{
		{"bad id", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Id = "a b" }, "must be 1 to 64"},
		{"twice", func(p *networkv1.NetworkPlan) {
			again := proto.Clone(p.PortForwards[0]).(*networkv1.PortForward)
			again.Listen.Port++
			p.PortForwards = append(p.PortForwards, again)
		}, "listed twice"},
		{"listen taken by ingress", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Listen.Port = 8388 }, "taken"},
		{"no protocol", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Protocols = nil }, "protocols"},
		{"bad protocol", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Protocols = []string{"sctp"} }, "protocols"},
		{"protocol twice", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Protocols = []string{"tcp", "tcp"} }, "protocols"},
		{"no target", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Targets = nil }, "targets are required"},
		{"bad target", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Targets[1].Port = 0 }, "targets[1]"},
		{"no next", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Next = nil }, "next is required"},
		{"unknown transport", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Next = &networkv1.PortForward_TransportId{TransportId: "nope"}
		}, "transport_id"},
		{"hop egress", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Next = &networkv1.PortForward_EgressId{EgressId: "eg-ix"}
		}, "internet egress"},
		{"balance", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Policy = &networkv1.ForwardPolicy{Balance: "fastest"}
		}, "balance"},
		{"proxy protocol", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Policy = &networkv1.ForwardPolicy{ProxyProtocol: "v3"}
		}, "proxy_protocol"},
		{"proxy on udp", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Protocols = []string{"udp"}
			p.PortForwards[0].Policy = &networkv1.ForwardPolicy{ProxyProtocol: "v1"}
		}, "UDP only"},
		{"weight", func(p *networkv1.NetworkPlan) { p.PortForwards[0].Targets[0].Weight = 5000 }, "weight"},
		{"health timeout", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Policy = &networkv1.ForwardPolicy{Health: &networkv1.ForwardHealth{IntervalMs: 1000, TimeoutMs: 2000}}
		}, "longer than interval"},
		{"breaker", func(p *networkv1.NetworkPlan) {
			p.PortForwards[0].Policy = &networkv1.ForwardPolicy{CircuitBreaker: &networkv1.ForwardBreaker{FailureThreshold: 1000}}
		}, "failure_threshold"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := withForward(t)
			c.breakPlan(plan)
			err := Check(plan)
			require.ErrorIs(t, err, ErrInvalid)
			require.Contains(t, err.Error(), c.want)
		})
	}
}

func TestPortForwardStatus(t *testing.T) {
	status := &networkv1.NetworkStatus{DeviceRef: "proxy-1", PortForwards: []*networkv1.PortForwardStatus{
		{Id: "rt-1", State: networkv1.ComponentState_COMPONENT_STATE_APPLIED, UpBytes: 10, DownBytes: 20},
	}}
	require.NoError(t, CheckStatus(status, "proxy-1"))
	status.PortForwards[0].Id = "a b"
	require.ErrorIs(t, CheckStatus(status, "proxy-1"), ErrInvalid)
	status.PortForwards[0].Id = "rt-1"
	status.PortForwards[0].State = 99
	require.ErrorIs(t, CheckStatus(status, "proxy-1"), ErrInvalid)
	status.PortForwards[0].State = networkv1.ComponentState_COMPONENT_STATE_APPLIED
	status.PortForwards[0].Targets = []*networkv1.ForwardTargetHealth{
		{Host: "203.0.113.9", Port: 443, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, RttUs: 1200, CheckedAtUnixMs: 1},
	}
	require.NoError(t, CheckStatus(status, "proxy-1"))
	for _, broken := range []func(*networkv1.ForwardTargetHealth){
		func(target *networkv1.ForwardTargetHealth) { target.Port = 0 },
		func(target *networkv1.ForwardTargetHealth) { target.Host = "" },
		func(target *networkv1.ForwardTargetHealth) { target.State = 99 },
		func(target *networkv1.ForwardTargetHealth) { target.CheckedAtUnixMs = -1 },
	} {
		target := proto.Clone(status.PortForwards[0].Targets[0]).(*networkv1.ForwardTargetHealth)
		broken(target)
		bad := proto.Clone(status).(*networkv1.NetworkStatus)
		bad.PortForwards[0].Targets = []*networkv1.ForwardTargetHealth{target}
		require.ErrorIs(t, CheckStatus(bad, "proxy-1"), ErrInvalid)
	}
}

func TestUnsupportedRoutingRange(t *testing.T) {
	plan := readPlan(t, "plan-anx-line-egress.json")
	caps := &networkv1.DeviceCapabilities{RoutingFirst: 30000, RoutingLast: 30999, Kinds: []*networkv1.KindSupport{
		{Component: networkv1.Component_COMPONENT_INGRESS, Kind: IngressDirect, Versions: []uint32{1}},
		{Component: networkv1.Component_COMPONENT_EGRESS, Kind: EgressNATDirect, Versions: []uint32{1}},
		{Component: networkv1.Component_COMPONENT_TRANSPORT, Kind: TransportAnxLine, Versions: []uint32{1}},
	}}
	require.Equal(t, []string{
		"egress eg-jp rule priority 20001 outside 30000–30999",
		"egress eg-jp table 20001 outside 30000–30999",
	}, Unsupported(plan, caps))
}
