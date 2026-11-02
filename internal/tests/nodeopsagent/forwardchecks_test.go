package nodeopsagent

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/stretchr/testify/assert"
)

// forwardCheckCapabilities are an Agent that runs the forward checks.
var forwardCheckCapabilities = append(append([]string(nil), fakeagent.DefaultCapabilities...), "diag")

func (f *fixture) forwardChecks() kernelnodeops.ForwardChecks {
	return kernelnodeops.ForwardChecks{Sources: func() kernelnodeops.AgentSources {
		return kernelnodeops.AgentSources{Streams: f.control.Streams}
	}}
}

// The route diagnosis lists each node with whether its Agent is connected
// and advertises agent.diagnostic and diag.v1.
func TestForwardCheckVantage(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f *fixture) {
		checks := f.forwardChecks()
		assert.Equal(t, kernelforward.NodeVantage{Note: "the node's Agent is not connected"}, checks.Vantage(f.forwardNode()))

		old := f.forwardAgent(fakeagent.Script{AgentVersion: "4.1.9"})
		vantage := checks.Vantage(f.forwardNode())
		assert.True(t, vantage.Connected)
		assert.False(t, vantage.Capable)
		assert.Contains(t, vantage.Note, "diag.v1")
		old.Disconnect()

		f.forwardAgent(fakeagent.Script{Capabilities: forwardCheckCapabilities, AgentVersion: "4.2.1"})
		assert.Eventually(t, func() bool {
			v := checks.Vantage(f.forwardNode())
			return v.Connected && v.Capable
		}, awaitTimeout, settleInterval)
	})
}
