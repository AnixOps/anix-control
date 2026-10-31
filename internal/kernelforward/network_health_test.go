package kernelforward

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/forwardddns"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// networkStatus records proxy-21's network status at now: the plan it runs
// and the route's port forward with targets.
func networkStatus(t *testing.T, f *fixture, db *gorm.DB, network *networkmodel.Service, routeID string, state networkv1.ComponentState, targets ...*networkv1.ForwardTargetHealth) {
	t.Helper()
	deviceStatus(t, f, db, network, proxy, "203.0.113.21/24", routeID, state, targets...)
}

// deviceStatus records node's network status at now, with the public
// interface address address.
func deviceStatus(t *testing.T, f *fixture, db *gorm.DB, network *networkmodel.Service, node agentcontrol.AgentNode, address, routeID string, state networkv1.ComponentState, targets ...*networkv1.ForwardTargetHealth) {
	t.Helper()
	require.NoError(t, network.RecordHello(f.ctx, node, &networkv1.DeviceCapabilities{}))
	var device model.NetworkDevice
	require.NoError(t, db.First(&device, "node_ref = ?", node.String()).Error)
	status := &networkv1.NetworkStatus{
		DeviceRef: node.String(), Generation: device.Generation,
		Interfaces:   []*networkv1.InterfaceFact{{Name: "eth0", Addresses: []string{"10.0.0.2/24", address}}},
		PortForwards: []*networkv1.PortForwardStatus{{Id: routeID, State: state, Targets: targets}},
	}
	require.NoError(t, network.RecordStatus(f.ctx, node, status, f.clock.Now()))
}

// The entry's target checks are the route's health (hop 0, checked by the
// entry device) and feed its latency history.
func TestNetworkRouteHealthAndLatency(t *testing.T) {
	f, db, network := networkFixture(t)
	route := f.create("network-1", networkRoute(&forwardv1.Route{}))

	health, err := f.service.RouteHealth(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Empty(t, health, "no status yet")

	checked := f.clock.Now().Add(-time.Second).UnixMilli()
	target := &networkv1.ForwardTargetHealth{
		Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, RttUs: 2500, CheckedAtUnixMs: checked,
	}
	networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED, target)
	health, err = f.service.RouteHealth(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Len(t, health, 1)
	require.Equal(t, proxy.String(), health[0].GetNodeRef())
	require.Equal(t, uint32(0), health[0].GetHopIndex())
	require.Equal(t, "198.51.100.10", health[0].GetAddress())
	require.Equal(t, forwardv1.HealthState_HEALTH_STATE_HEALTHY, health[0].GetState())
	require.Equal(t, uint32(2500), health[0].GetRttUs())

	// Each check counts once in the latency history, a failed one as a
	// failure.
	forward := &networkv1.PortForwardStatus{Id: route.GetId(), UpBytes: 1, Targets: []*networkv1.ForwardTargetHealth{target}}
	_, err = f.service.RecordNetworkCounters(f.ctx, proxy, []*networkv1.PortForwardStatus{forward}, f.clock.Now())
	require.NoError(t, err)
	_, err = f.service.RecordNetworkCounters(f.ctx, proxy, []*networkv1.PortForwardStatus{forward}, f.clock.Now().Add(time.Second))
	require.NoError(t, err)
	failed := &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_UNHEALTHY,
		ConsecutiveFailures: 1, CheckedAtUnixMs: checked + 500}
	forward.Targets = []*networkv1.ForwardTargetHealth{failed}
	_, err = f.service.RecordNetworkCounters(f.ctx, proxy, []*networkv1.PortForwardStatus{forward}, f.clock.Now().Add(2*time.Second))
	require.NoError(t, err)
	buckets, _, err := f.service.Latency(f.ctx, route.GetId(), time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	require.Equal(t, proxy.String(), buckets[0].GetNodeRef())
	require.Equal(t, uint32(2), buckets[0].GetSamples())
	require.Equal(t, uint32(1), buckets[0].GetFailures())
	require.Equal(t, uint32(2500), buckets[0].GetRttMaxUs())
}

func stepsOf(answer *forwardv1.DiagnoseRouteResponse, kind forwardv1.ProbeKind) []*forwardv1.DiagnoseStep {
	var out []*forwardv1.DiagnoseStep
	for _, step := range answer.GetSteps() {
		if step.GetKind() == kind {
			out = append(out, step)
		}
	}
	return out
}

// An ENGINE_NETWORK route is diagnosed on the network model: the device's
// plan and port forward, the entry's target checks, and a dial of the
// entry's listener from Control.
func TestDiagnoseNetworkRoute(t *testing.T) {
	resetDiagnosesForTest()
	f, db, network := networkFixture(t)
	route := f.create("network-1", networkRoute(&forwardv1.Route{}))
	var mu sync.Mutex
	var dialed []string
	f.service.Probes.Dial = func(_ context.Context, _, address string, _ time.Duration) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialed = append(dialed, address)
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}

	answer, err := f.service.DiagnoseRoute(f.ctx, route.GetId(), 0)
	require.NoError(t, err)
	require.False(t, answer.GetOk())
	config := stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG)
	require.Len(t, config, 1)
	require.Equal(t, CodeNotOnNetwork, config[0].GetResult().GetCode())
	require.NotEmpty(t, answer.GetNodes())
	require.False(t, answer.GetNodes()[0].GetNodeVantage())
	require.Contains(t, dialed, "198.51.100.10:22", "without target health Control dials the targets")

	resetDiagnosesForTest()
	dialed = nil
	networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED,
		&networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, RttUs: 900, CheckedAtUnixMs: 1})
	answer, err = f.service.DiagnoseRoute(f.ctx, route.GetId(), 0)
	require.NoError(t, err)
	require.True(t, answer.GetOk(), "%v", answer.GetSteps())
	config = stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG)
	require.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_OK, config[0].GetResult().GetStatus())
	health := stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH)
	require.Len(t, health, 1)
	require.Equal(t, "198.51.100.10:22", health[0].GetTarget())
	require.Equal(t, CodeHealthy, health[0].GetResult().GetCode())
	require.Equal(t, uint32(900), health[0].GetResult().GetRttUs())
	listen := stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_LISTEN)
	require.Len(t, listen, 1)
	require.Equal(t, "203.0.113.21:20022", listen[0].GetTarget(), "the device's public interface address")
	require.Equal(t, []string{"203.0.113.21:20022"}, dialed, "healthy targets are not dialed from Control")

	// A port forward the kernel could not start, and an open breaker.
	resetDiagnosesForTest()
	networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_ERROR,
		&networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_CIRCUIT_OPEN, ConsecutiveFailures: 3})
	answer, err = f.service.DiagnoseRoute(f.ctx, route.GetId(), 0)
	require.NoError(t, err)
	require.False(t, answer.GetOk())
	require.Equal(t, CodeHopError, stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG)[0].GetResult().GetCode())
	require.Equal(t, CodeCircuitOpen, stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH)[0].GetResult().GetCode())
}

func TestEvaluateNetworkEntry(t *testing.T) {
	now := start
	routeID := "route-1"
	applied := func(targets ...networkv1.TargetHealthState) networkDevice {
		forward := &networkv1.PortForwardStatus{Id: routeID, State: networkv1.ComponentState_COMPONENT_STATE_APPLIED}
		for _, state := range targets {
			forward.Targets = append(forward.Targets, &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: state})
		}
		return networkDevice{Known: true, Negotiated: true, Generation: 4, StatusAt: now.Add(-time.Minute),
			Status: &networkv1.NetworkStatus{Generation: 4, PortForwards: []*networkv1.PortForwardStatus{forward}}}
	}
	good := entryFacts{Addresses: []string{"203.0.113.21"}, Online: true}
	healthy, open := networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, networkv1.TargetHealthState_TARGET_HEALTH_STATE_CIRCUIT_OPEN
	cases := []struct {
		name   string
		device networkDevice
		facts  entryFacts
		want   verdict
		reason string
	}{
		{"healthy", applied(healthy, open), good, verdictGood, dnsReasonHealthy},
		{"no health yet", applied(), good, verdictGood, dnsReasonHealthy},
		{"not on the model", networkDevice{}, good, verdictBad, dnsReasonNotOnNetwork},
		{"no address", applied(healthy), entryFacts{Online: true}, verdictBad, dnsReasonNoAddress},
		{"offline", applied(healthy), entryFacts{Addresses: good.Addresses}, verdictBad, dnsReasonOffline},
		{"targets down", applied(open, networkv1.TargetHealthState_TARGET_HEALTH_STATE_UNHEALTHY), good, verdictBad, dnsReasonUpstreamsDown},
		{"stale", func() networkDevice { d := applied(healthy); d.StatusAt = now.Add(-time.Hour); return d }(), good, verdictBad, dnsReasonReportStale},
		{"never reported", networkDevice{Known: true, Negotiated: true}, good, verdictBad, dnsReasonNeverReported},
		{"converging", func() networkDevice { d := applied(healthy); d.Generation = 5; return d }(), good, verdictNeutral, dnsReasonConverging},
		{"forward down", func() networkDevice {
			d := applied(healthy)
			d.Status.PortForwards[0].State = networkv1.ComponentState_COMPONENT_STATE_DOWN
			return d
		}(), good, verdictBad, dnsReasonHopError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, reason := evaluateNetworkEntry(routeID, c.device, c.facts, now)
			require.Equal(t, c.want, v)
			require.Equal(t, c.reason, reason)
		})
	}
}

// A DNS binding of an ENGINE_NETWORK route publishes its entry device's
// public address while the device runs the route, and keeps it (degraded)
// when every target is down.
func TestEntryHAOnNetworkRoute(t *testing.T) {
	f, db, network := networkFixture(t)
	fake := &fakeDNS{sets: map[string][]string{}}
	f.service.DNSKEK = func() string { return testKEK }
	f.service.NewDNSProvider = func(string, map[string]string, map[string]string) (forwardddns.Provider, error) { return fake, nil }
	route := f.create("network-1", networkRoute(&forwardv1.Route{
		Listen: &forwardv1.Listen{Port: 20022, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP, EntryHostname: hostnameA},
	}))
	provider, err := f.service.CreateDNSProvider(f.ctx, "provider-1", &forwardv1.DnsProvider{
		Name: "cf", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
	}, map[string]string{forwardddns.CredentialAPIToken: cfSecret})
	require.NoError(t, err)
	_, err = f.service.CreateDNSBinding(f.ctx, "binding-1", &forwardv1.DnsBinding{
		RouteId: route.GetId(), ProviderId: provider.GetId(), Zone: "example.com",
	})
	require.NoError(t, err)
	ha := &EntryHA{Service: f.service, LocalSession: func(agentcontrol.AgentNode) bool { return true }}

	healthy := &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY}
	networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED, healthy)
	require.NoError(t, ha.Evaluate(f.ctx))
	require.Equal(t, []string{"set hk.example.com A 203.0.113.21"}, fake.take())
	status, err := f.service.RouteDNS(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Len(t, status.GetNodes(), 1)
	require.Equal(t, []string{"203.0.113.21"}, status.GetNodes()[0].GetAddresses())
	require.True(t, status.GetNodes()[0].GetInRotation())

	down := &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_CIRCUIT_OPEN}
	for range DNSRemoveAfter {
		f.clock.Advance(DNSEvaluateInterval)
		networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED, down)
		require.NoError(t, ha.Evaluate(f.ctx))
	}
	require.Empty(t, fake.take(), "never emptied")
	status, err = f.service.RouteDNS(f.ctx, route.GetId())
	require.NoError(t, err)
	require.False(t, status.GetNodes()[0].GetInRotation())
	require.Equal(t, dnsReasonUpstreamsDown, status.GetNodes()[0].GetReason())
	require.Equal(t, dnsStateDegraded, status.GetState())
}

// A route with two entry devices: each runs the port forward by an
// internet egress of its own, reports its own target checks, is diagnosed
// and is judged for the entry HA DNS name on its own.
func TestNetworkRouteSeveralEntries(t *testing.T) {
	resetDiagnosesForTest()
	f, db, network := networkFixture(t)
	second := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 22}
	require.NoError(t, db.Create(&model.Node{ID: 22, Name: "proxy-2", Host: "proxy2.example.com", Port: 443, APIKey: "proxy2-key", Status: model.NodeStatusOnline}).Error)
	view, err := network.Get(f.ctx)
	require.NoError(t, err)
	doc := view.Document
	doc.Lines = append(doc.Lines, networkmodel.Line{ID: "ln-hk2", Node: second.String(), Interface: "eth0", Families: []string{"v4"}, GatewayV4: "203.0.113.1"})
	doc.Egresses = append(doc.Egresses, networkmodel.Egress{ID: "eg-hk2", Line: "ln-hk2", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "auto"})
	_, err = network.Replace(f.ctx, &doc, &view.Revision, networkmodel.Actor{Name: "admin"})
	require.NoError(t, err)

	fake := &fakeDNS{sets: map[string][]string{}}
	f.service.DNSKEK = func() string { return testKEK }
	f.service.NewDNSProvider = func(string, map[string]string, map[string]string) (forwardddns.Provider, error) { return fake, nil }
	route := networkRoute(&forwardv1.Route{
		Listen: &forwardv1.Listen{Port: 20022, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP, EntryHostname: hostnameA},
	})
	route.Hops[0].NodeRefs = []string{proxy.String(), second.String()}
	route = f.create("network-1", route)
	for node, egress := range map[string]string{proxy.String(): "eg-hk", second.String(): "eg-hk2"} {
		plan, found, err := network.DevicePlan(f.ctx, node)
		require.NoError(t, err)
		require.True(t, found, node)
		require.Len(t, plan.GetPortForwards(), 1, node)
		require.Equal(t, egress, plan.GetPortForwards()[0].GetEgressId(), node)
	}

	healthy := &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, RttUs: 700, CheckedAtUnixMs: 1}
	down := &networkv1.ForwardTargetHealth{Host: "198.51.100.10", Port: 22, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_CIRCUIT_OPEN, ConsecutiveFailures: 3}
	report := func(secondTarget *networkv1.ForwardTargetHealth) {
		networkStatus(t, f, db, network, route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED, healthy)
		deviceStatus(t, f, db, network, second, "203.0.113.22/24", route.GetId(), networkv1.ComponentState_COMPONENT_STATE_APPLIED, secondTarget)
	}
	report(healthy)
	health, err := f.service.RouteHealth(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Len(t, health, 2)
	require.Equal(t, []string{proxy.String(), second.String()}, []string{health[0].GetNodeRef(), health[1].GetNodeRef()})

	var mu sync.Mutex
	var dialed []string
	f.service.Probes.Dial = func(_ context.Context, _, address string, _ time.Duration) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialed = append(dialed, address)
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	answer, err := f.service.DiagnoseRoute(f.ctx, route.GetId(), 0)
	require.NoError(t, err)
	require.True(t, answer.GetOk(), "%v", answer.GetSteps())
	require.Len(t, stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG), 2)
	require.Len(t, stepsOf(answer, forwardv1.ProbeKind_PROBE_KIND_HEALTH), 2)
	require.ElementsMatch(t, []string{"203.0.113.21:20022", "203.0.113.22:20022"}, dialed)

	provider, err := f.service.CreateDNSProvider(f.ctx, "provider-1", &forwardv1.DnsProvider{
		Name: "cf", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
	}, map[string]string{forwardddns.CredentialAPIToken: cfSecret})
	require.NoError(t, err)
	_, err = f.service.CreateDNSBinding(f.ctx, "binding-1", &forwardv1.DnsBinding{
		RouteId: route.GetId(), ProviderId: provider.GetId(), Zone: "example.com",
	})
	require.NoError(t, err)
	ha := &EntryHA{Service: f.service, LocalSession: func(agentcontrol.AgentNode) bool { return true }}
	require.NoError(t, ha.Evaluate(f.ctx))
	require.Equal(t, []string{"set hk.example.com A 203.0.113.21,203.0.113.22"}, fake.take())

	// The second entry's checks fail: it leaves rotation, the first stays.
	for range DNSRemoveAfter {
		f.clock.Advance(DNSEvaluateInterval)
		report(down)
		require.NoError(t, ha.Evaluate(f.ctx))
	}
	require.Equal(t, []string{"set hk.example.com A 203.0.113.21"}, fake.take())
	status, err := f.service.RouteDNS(f.ctx, route.GetId())
	require.NoError(t, err)
	require.Len(t, status.GetNodes(), 2)
	for _, node := range status.GetNodes() {
		require.Equal(t, node.GetNodeRef() == proxy.String(), node.GetInRotation(), node.GetNodeRef())
	}
}
