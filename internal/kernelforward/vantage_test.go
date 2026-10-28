package kernelforward

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// vantageReport stores a fresh report of node carrying vantage results:
// each listed entry address with its probe state.
func (d *dnsFixture) vantageReport(node agentcontrol.AgentNode, bindingID uint64, states map[string]forwardv1.HealthState) {
	d.t.Helper()
	state := d.state(node)
	report := &forwardv1.NodeForwardReport{NodeRef: node.String(), Generation: state.GetGeneration(), StateHash: state.GetStateHash(), Applied: true}
	for _, probe := range state.GetVantageProbes() {
		if probe.GetBindingId() != bindingID {
			continue
		}
		if health, ok := states[probe.GetAddress()]; ok {
			report.Vantage = append(report.Vantage, &forwardv1.VantageResult{
				BindingId: bindingID, RouteId: probe.GetRouteId(), NodeRef: probe.GetNodeRef(), Address: probe.GetAddress(),
				Port: probe.GetPort(), State: health, CheckedAtUnixMs: d.clock.Now().UnixMilli(),
			})
		}
	}
	encoded, err := jsonWrite.Marshal(report)
	require.NoError(d.t, err)
	now := d.clock.Now()
	require.NoError(d.t, d.db.Save(&model.KernelForwardNodeReport{
		NodeRef: node.String(), Generation: report.Generation, StateHash: report.StateHash, Applied: true,
		ReportJSON: string(encoded), ObservedAt: now, ReceivedAt: now,
	}).Error)
}

// Vantage probes (L4): the nodes with the binding's label get one probe per
// published entry address, an address more of them find down than up
// leaves the DNS, and clearing the selector removes the probes.
func TestEntryHAVantageProbes(t *testing.T)         { runEntryHAVantage(t, openSQLite(t)) }
func TestPostgresEntryHAVantageProbes(t *testing.T) { runEntryHAVantage(t, openPostgres(t)) }

func runEntryHAVantage(t *testing.T, db *gorm.DB) {
	d := newDNSFixture(t, db)
	provider := d.provider()
	binding, err := d.service.CreateDNSBinding(d.ctx, "binding-1", &forwardv1.DnsBinding{
		RouteId: d.route.GetId(), ProviderId: provider.GetId(), Zone: "example.com", Vantage: " region = cn ",
	})
	require.NoError(t, err)
	require.Equal(t, "region=cn", binding.GetVantage())
	// No node carries the label yet: no probes.
	require.Empty(t, d.state(exit).GetVantageProbes())

	before := d.state(exit).GetGeneration()
	_, err = d.service.SetNodeSettings(d.ctx, exit, NodeSettings{Labels: map[string]string{"region": "cn"}})
	require.NoError(t, err)
	state := d.state(exit)
	require.Greater(t, state.GetGeneration(), before, "the probes bump the vantage node's generation")
	probes := state.GetVantageProbes()
	require.Len(t, probes, 2)
	for i, want := range []struct{ ref, address string }{{"forward-11", "192.0.2.11"}, {"forward-14", "192.0.2.14"}} {
		require.Equal(t, binding.GetId(), probes[i].GetBindingId())
		require.Equal(t, d.route.GetId(), probes[i].GetRouteId())
		require.Equal(t, want.ref, probes[i].GetNodeRef())
		require.Equal(t, want.address, probes[i].GetAddress())
		require.EqualValues(t, 31000, probes[i].GetPort())
		require.EqualValues(t, VantageInterval.Milliseconds(), probes[i].GetIntervalMs())
		require.EqualValues(t, VantageTimeout.Milliseconds(), probes[i].GetTimeoutMs())
	}
	// Entry nodes never probe their own route.
	require.Empty(t, d.state(entry).GetVantageProbes())

	d.tick(entry, entry2)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11,192.0.2.14"}, d.dns.take())

	// The vantage node cannot reach forward-14: it leaves on the third
	// evaluation, like any other bad verdict.
	down := map[string]forwardv1.HealthState{
		"192.0.2.11": forwardv1.HealthState_HEALTH_STATE_HEALTHY,
		"192.0.2.14": forwardv1.HealthState_HEALTH_STATE_UNHEALTHY,
	}
	for i := 0; i < 3; i++ {
		d.clock.Advance(DNSEvaluateInterval)
		d.report(entry, false)
		d.report(entry2, false)
		d.vantageReport(exit, binding.GetId(), down)
		require.NoError(t, d.ha.Evaluate(d.ctx))
	}
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11"}, d.dns.take())
	nodes := d.status().GetNodes()
	require.Equal(t, dnsReasonVantageUnreachable, nodes[1].GetReason())
	require.Equal(t, []string{"192.0.2.14"}, nodes[1].GetUnreachable())
	require.EqualValues(t, 1, nodes[1].GetVantageNodes())
	require.Empty(t, nodes[0].GetUnreachable())

	// Clearing the selector removes the probes and the vantage verdicts.
	_, err = d.service.UpdateDNSBinding(d.ctx, "clear", &forwardv1.DnsBinding{Id: binding.GetId()})
	require.NoError(t, err)
	require.Empty(t, d.state(exit).GetVantageProbes())
	require.Empty(t, d.status().GetBinding().GetVantage())
	for i := 0; i < 3; i++ {
		d.clock.Advance(DNSEvaluateInterval)
		d.report(entry, false)
		d.report(entry2, false)
		d.vantageReport(exit, binding.GetId(), down)
		require.NoError(t, d.ha.Evaluate(d.ctx))
	}
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11,192.0.2.14"}, d.dns.take())
	var rows int64
	require.NoError(t, db.Model(&model.KernelForwardDNSVantage{}).Count(&rows).Error)
	require.Zero(t, rows)
}

func TestCheckVantage(t *testing.T) {
	for selector, want := range map[string]string{
		"":            "",
		"  ":          "",
		"region=cn":   "region=cn",
		" isp = ct ":  "isp=ct",
		"a.b/c-d=x y": "a.b/c-d=x y",
	} {
		got, problem := checkVantage(selector)
		require.Nil(t, problem, selector)
		require.Equal(t, want, got, selector)
	}
	for _, selector := range []string{"region", "=cn", "region=", "region=a=b", "-x=cn", "region=c\nn", "region=" + string(make([]byte, 200))} {
		_, problem := checkVantage(selector)
		require.NotNil(t, problem, selector)
		require.Equal(t, "binding.vantage", problem.GetField(), selector)
	}
}

func TestUnreachableNeedsMoreDownThanUp(t *testing.T) {
	views := map[string]*vantageView{
		"192.0.2.1": {up: 1, down: 1, nodes: map[string]bool{"a": true, "b": true}},
		"192.0.2.2": {up: 1, down: 2, nodes: map[string]bool{"a": true, "b": true, "c": true}},
	}
	kept, down, vantageNodes := unreachable(views, []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"})
	require.Equal(t, []string{"192.0.2.1", "192.0.2.3"}, kept)
	require.Equal(t, []string{"192.0.2.2"}, down)
	require.Equal(t, 3, vantageNodes)
}
