package wire

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const node = "forward-11"

func capabilities() *forwardv1.NodeCapabilities {
	return &forwardv1.NodeCapabilities{
		KernelVersion: "6.12.0", Cgroup: "v2", Ipv6: true, AgentVersion: "4.2.0",
		Engines: []*forwardv1.EngineCapabilities{{
			Engine: forwardv1.Engine_ENGINE_NFTABLES, Version: "nft 1.1.3", Available: true, Ipv6: true, Udp: true,
			Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
			LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
			BandwidthLimit: true, Quota: true, MaxConns: true,
		}},
	}
}

func TestHelloCapabilityRoundTrip(t *testing.T) {
	capability, err := HelloCapability(capabilities())
	require.NoError(t, err)
	assert.Equal(t, agentcontrol.CapabilityForward, capability.GetName())
	assert.Equal(t, agentcontrol.CapabilityVersionV1, capability.GetVersion())
	assert.Contains(t, capability.GetAttributes()[AttributeNodeCapabilities], `"kernel_version":"6.12.0"`, "proto field names")

	hello := []*agentv1pb.Capability{{Name: "agent.ping", Version: "v1"}, capability}
	caps, listed, err := NodeCapabilitiesFromHello(hello, node)
	require.NoError(t, err)
	require.True(t, listed)
	want := capabilities()
	want.NodeRef = node
	assert.True(t, proto.Equal(want, caps), "node_ref is filled in")

	_, listed, err = NodeCapabilitiesFromHello(hello[:1], node)
	require.NoError(t, err)
	assert.False(t, listed)
	// forward at another version is not forward.v1.
	_, listed, _ = NodeCapabilitiesFromHello([]*agentv1pb.Capability{{Name: agentcontrol.CapabilityForward, Version: "v2"}}, node)
	assert.False(t, listed)

	// A newer Agent's fields are ignored.
	raw := `{"node_ref":"forward-11","engines":[{"engine":"ENGINE_GOST","available":true,"future":1}],"future":{"x":1}}`
	caps, _, err = NodeCapabilitiesFromHello([]*agentv1pb.Capability{forwardCapability(raw)}, node)
	require.NoError(t, err)
	assert.Equal(t, forwardv1.Engine_ENGINE_GOST, caps.GetEngines()[0].GetEngine())
}

func forwardCapability(raw string) *agentv1pb.Capability {
	return &agentv1pb.Capability{Name: agentcontrol.CapabilityForward, Version: agentcontrol.CapabilityVersionV1,
		Attributes: map[string]string{AttributeNodeCapabilities: raw}}
}

func TestHelloCapabilityRefusals(t *testing.T) {
	cases := map[string]string{
		"missing":            "",
		"oversize":           `{"kernel_version":"` + strings.Repeat("x", MaxNodeCapabilitiesBytes) + `"}`,
		"malformed":          `{"engines":`,
		"wrong node":         `{"node_ref":"forward-12"}`,
		"unspecified engine": `{"engines":[{"engine":"ENGINE_UNSPECIFIED"}]}`,
		"unknown engine":     `{"engines":[{"engine":9}]}`,
		"twice":              `{"engines":[{"engine":"ENGINE_GOST"},{"engine":"ENGINE_GOST"}]}`,
		"unknown strategy":   `{"engines":[{"engine":"ENGINE_GOST","strategies":[42]}]}`,
		"unknown security":   `{"engines":[{"engine":"ENGINE_GOST","link_securities":[42]}]}`,
		"long version":       `{"engines":[{"engine":"ENGINE_GOST","version":"` + strings.Repeat("v", MaxTextBytes+1) + `"}]}`,
		"long reason":        `{"engines":[{"engine":"ENGINE_GOST","unavailable_reason":"` + strings.Repeat("r", MaxTextBytes+1) + `"}]}`,
		"long kernel":        `{"kernel_version":"` + strings.Repeat("k", MaxTextBytes+1) + `"}`,
	}
	many := `{"engines":[`
	for i := 0; i <= MaxEngines; i++ {
		if i > 0 {
			many += ","
		}
		many += `{"engine":"ENGINE_GOST"}`
	}
	cases["too many engines"] = many + `]}`
	for name, raw := range cases {
		capability := forwardCapability(raw)
		if raw == "" {
			capability.Attributes = nil
		}
		_, listed, err := NodeCapabilitiesFromHello([]*agentv1pb.Capability{capability}, node)
		assert.True(t, listed, name)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	_, err := HelloCapability(nil)
	require.ErrorIs(t, err, ErrInvalid)
	tooMany := capabilities()
	tooMany.Engines[0].Strategies = make([]forwardv1.BalanceStrategy, len(forwardv1.BalanceStrategy_name)+1)
	_, err = HelloCapability(tooMany)
	require.ErrorIs(t, err, ErrInvalid)
	notUTF8 := capabilities()
	notUTF8.Cgroup = "\xff"
	require.ErrorIs(t, CheckNodeCapabilities(notUTF8, ""), ErrInvalid)
	big := capabilities()
	for i := 0; i < MaxEngines-1; i++ {
		big.Engines = append(big.Engines, &forwardv1.EngineCapabilities{Engine: forwardv1.Engine(100 + i)})
	}
	_, err = HelloCapability(big)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestStateMemberRoundTrip(t *testing.T) {
	state := &forwardv1.NodeForwardState{NodeRef: node, Generation: 7, StateHash: strings.Repeat("a", 64), Hops: []*forwardv1.NodeHop{{
		RouteId: "01JF1A000000000000000000A1", Role: forwardv1.HopRole_HOP_ROLE_ENTRY, Engine: forwardv1.Engine_ENGINE_NFTABLES,
		Listen: &forwardv1.Listen{Port: 30001, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP}, Mark: 1,
	}}}
	member, err := StateMember(state)
	require.NoError(t, err)
	assert.Equal(t, "7", member["generation"], "uint64 as a protojson string")
	document, err := json.Marshal(map[string]any{"kind": "forward", NodeConfigMember: member})
	require.NoError(t, err)
	again, err := json.Marshal(map[string]any{NodeConfigMember: member, "kind": "forward"})
	require.NoError(t, err)
	assert.Equal(t, document, again, "the same state always encodes the same bytes")

	decoded, found, err := StateFromNodeConfig(NodeConfigFormat, document)
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, proto.Equal(state, decoded))

	_, found, err = StateFromNodeConfig("anixops.nodeconfig/v1", document)
	require.NoError(t, err)
	assert.False(t, found, "v1 carries no forwarding")
	_, found, err = StateFromNodeConfig(NodeConfigFormat, []byte(`{"kind":"forward"}`))
	require.NoError(t, err)
	assert.False(t, found)
	_, _, err = StateFromNodeConfig(NodeConfigFormat, []byte(`[]`))
	require.ErrorIs(t, err, ErrInvalid)
	_, found, err = StateFromNodeConfig(NodeConfigFormat, []byte(`{"forward":{"generation":"x"}}`))
	require.ErrorIs(t, err, ErrInvalid)
	assert.True(t, found)
}

func report() *forwardv1.NodeForwardReport {
	return &forwardv1.NodeForwardReport{
		NodeRef: node, Generation: 3, StateHash: strings.Repeat("b", 64), Applied: true, ObservedAtUnixMs: 1_800_000_000_000,
		Errors: []*forwardv1.HopError{{RouteId: "01JF1A000000000000000000A1", HopIndex: 1, Engine: forwardv1.Engine_ENGINE_GOST, Message: "bind: in use"}},
		Counters: []*forwardv1.Counters{
			{RouteId: "01JF1A000000000000000000A1", CounterEpoch: "n1", UpBytes: 10, DownBytes: 20},
			{RouteId: "01JF1A000000000000000000A1", NodeRef: node, CounterEpoch: "n2", UpBytes: 1},
		},
		Health: []*forwardv1.UpstreamHealth{{RouteId: "01JF1A000000000000000000A1", Address: "192.0.2.12", Port: 30000, State: forwardv1.HealthState_HEALTH_STATE_HEALTHY}},
	}
}

func TestReportRoundTrip(t *testing.T) {
	packaged, err := Report(report())
	require.NoError(t, err)
	assert.True(t, IsReport(packaged))
	assert.Equal(t, ReportPluginID, packaged.GetPluginId())
	assert.Equal(t, ReportKind, packaged.GetKind())
	assert.Equal(t, ReportVersion, packaged.GetVersion())
	assert.EqualValues(t, 1_800_000_000_000, packaged.GetObservedAtUnixMs())
	require.NoError(t, agentcontrol.ValidatePackageReportKind(packaged.GetKind()))
	decoded, err := DecodeReport(packaged.GetPayloadJson(), node)
	require.NoError(t, err)
	assert.True(t, proto.Equal(report(), decoded))

	assert.False(t, IsReport(&agentv1pb.PackageReport{PluginId: "machine-telemetry", Kind: ReportKind}))
	assert.False(t, IsReport(&agentv1pb.PackageReport{PluginId: ReportPluginID, Kind: "systemd.services"}))
	_, err = DecodeReport(packaged.GetPayloadJson(), "forward-12")
	require.ErrorIs(t, err, ErrInvalid, "another stream's node")
	_, err = DecodeReport([]byte(`{`), node)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = DecodeReport(make([]byte, agentcontrol.MaxPackageReportPayloadBytes+1), node)
	require.ErrorIs(t, err, ErrInvalid)
	decoded, err = DecodeReport([]byte(`{"node_ref":"forward-11","future":true}`), node)
	require.NoError(t, err, "unknown fields are ignored")
	assert.Equal(t, node, decoded.GetNodeRef())

	huge := report()
	for i := 0; i < 3000; i++ {
		huge.Health = append(huge.Health, &forwardv1.UpstreamHealth{RouteId: "01JF1A000000000000000000A1", Address: strings.Repeat("h", 200)})
	}
	_, err = Report(huge)
	require.ErrorIs(t, err, ErrInvalid, "over the PackageReport cap")
}

func TestReportRefusals(t *testing.T) {
	cases := map[string]func(r *forwardv1.NodeForwardReport){
		"no node":      func(r *forwardv1.NodeForwardReport) { r.NodeRef = "" },
		"bad hash":     func(r *forwardv1.NodeForwardReport) { r.StateHash = "ABC" },
		"counter node": func(r *forwardv1.NodeForwardReport) { r.Counters[0].NodeRef = "forward-12" },
		"no epoch":     func(r *forwardv1.NodeForwardReport) { r.Counters[0].CounterEpoch = "" },
		"long epoch": func(r *forwardv1.NodeForwardReport) {
			r.Counters[0].CounterEpoch = strings.Repeat("e", MaxEpochBytes+1)
		},
		"epoch not UTF-8":   func(r *forwardv1.NodeForwardReport) { r.Counters[0].CounterEpoch = "\xff" },
		"duplicate counter": func(r *forwardv1.NodeForwardReport) { r.Counters[1].CounterEpoch = "n1" },
		"bad route id":      func(r *forwardv1.NodeForwardReport) { r.Counters[0].RouteId = "r-1" },
		"long route id":     func(r *forwardv1.NodeForwardReport) { r.Counters[0].RouteId = strings.Repeat("a", 65) },
		"hop out of range":  func(r *forwardv1.NodeForwardReport) { r.Counters[0].HopIndex = 8 },
		"error route":       func(r *forwardv1.NodeForwardReport) { r.Errors[0].RouteId = "" },
		"error engine":      func(r *forwardv1.NodeForwardReport) { r.Errors[0].Engine = 9 },
		"error message":     func(r *forwardv1.NodeForwardReport) { r.Errors[0].Message = strings.Repeat("m", MaxTextBytes+1) },
		"health route":      func(r *forwardv1.NodeForwardReport) { r.Health[0].HopIndex = 99 },
		"health address":    func(r *forwardv1.NodeForwardReport) { r.Health[0].Address = strings.Repeat("a", 254) },
		"health port":       func(r *forwardv1.NodeForwardReport) { r.Health[0].Port = 70000 },
		"health state":      func(r *forwardv1.NodeForwardReport) { r.Health[0].State = 9 },
		"too many errors": func(r *forwardv1.NodeForwardReport) {
			r.Errors = make([]*forwardv1.HopError, MaxReportEntries+1)
		},
	}
	for name, mutate := range cases {
		r := report()
		mutate(r)
		require.ErrorIs(t, CheckReport(r, node), ErrInvalid, name)
	}
	require.ErrorIs(t, CheckReport(nil, node), ErrInvalid)
	require.ErrorIs(t, CheckReport(report(), ""), ErrInvalid)
	_, err := Report(&forwardv1.NodeForwardReport{})
	require.ErrorIs(t, err, ErrInvalid)
}
