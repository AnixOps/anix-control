package kernelforward

import (
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// targetStep answers the stage 1 step about the route's target.
func targetStep(t *testing.T, answer *forwardv1.DiagnoseRouteResponse) *forwardv1.DiagnoseStep {
	t.Helper()
	var found []*forwardv1.DiagnoseStep
	for _, step := range steps(answer, forwardv1.ProbeKind_PROBE_KIND_CONFIG, "") {
		if step.GetTarget() != "" {
			found = append(found, step)
		}
	}
	require.Len(t, found, 1)
	return found[0]
}

// A target on a proxy node is checked against the node's protocols: the
// KFC-JP route pointed at :11113 while the node's Shadowsocks listened on
// 11111 and 11112.
func TestDiagnoseTargetProtocol(t *testing.T) {
	resetDiagnosesForTest()
	f := newFixture(t, openSQLite(t))
	require.NoError(t, f.db.Create(&model.Node{ID: 50, Name: "KFC JP", Host: "198.51.100.10", Port: 443, APIKey: "kfc-key"}).Error)
	require.NoError(t, f.db.Create(&[]model.NodeProtocol{
		{ID: 501, NodeID: 50, Name: "ss", Type: model.ProtocolShadowsocks, Port: 11111, Enable: 1},
		{ID: 502, NodeID: 50, Name: "ss2022", Type: model.ProtocolShadowsocks, Port: 11112, Enable: 1},
	}).Error)

	// No protocol on the target's port.
	route := f.create("t1", twoHop(31001))
	step := targetStep(t, f.diagnose(route.GetId(), 0))
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_FAILED, step.GetResult().GetStatus())
	assert.Equal(t, CodeTargetNoProtocol, step.GetResult().GetCode())
	assert.Equal(t, "198.51.100.10:443", step.GetTarget())
	assert.Contains(t, step.GetResult().GetMessage(), "11111, 11112")
	assert.Contains(t, step.GetResult().GetMessage(), "KFC JP")

	// Shadowsocks on the port, over a TCP route.
	resetDiagnosesForTest()
	tcp := twoHop(31002)
	tcp.Targets[0].Port = 11111
	route = f.create("t2", tcp)
	step = targetStep(t, f.diagnose(route.GetId(), 0))
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_OK, step.GetResult().GetStatus())
	assert.Equal(t, CodeTargetProtocol, step.GetResult().GetCode())

	// The same over a UDP-only route: Shadowsocks clients need TCP.
	resetDiagnosesForTest()
	udp := twoHop(31003)
	udp.Targets[0].Port = 11111
	udp.Listen.Protocol = forwardv1.L4Protocol_L4_PROTOCOL_UDP
	route = f.create("t3", udp)
	step = targetStep(t, f.diagnose(route.GetId(), 0))
	assert.Equal(t, forwardv1.ProbeStatus_PROBE_STATUS_FAILED, step.GetResult().GetStatus())
	assert.Equal(t, CodeTargetL4Mismatch, step.GetResult().GetCode())

	// A target that is no proxy node's address adds no step.
	resetDiagnosesForTest()
	other := twoHop(31004)
	other.Targets[0].Host = "203.0.113.99"
	route = f.create("t4", other)
	for _, s := range steps(f.diagnose(route.GetId(), 0), forwardv1.ProbeKind_PROBE_KIND_CONFIG, "") {
		assert.Empty(t, s.GetTarget())
	}
}
