package kernelforward

import (
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gproto "google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func TestReportAhead(t *testing.T) {
	h := func(c string) string { return strings.Repeat(c, 64) }
	for _, tc := range []struct {
		name             string
		stored, reported planner.Generation
		ahead            bool
		to               uint64
	}{
		{"behind", planner.Generation{Generation: 5, StateHash: h("a")}, planner.Generation{Generation: 4, StateHash: h("b")}, false, 0},
		{"converged", planner.Generation{Generation: 5, StateHash: h("a")}, planner.Generation{Generation: 5, StateHash: h("a")}, false, 0},
		{"same generation, no hash", planner.Generation{Generation: 5, StateHash: h("a")}, planner.Generation{Generation: 5}, false, 0},
		{"no state on the node", planner.Generation{}, planner.Generation{}, false, 0},
		{"higher, other hash", planner.Generation{Generation: 2, StateHash: h("a")}, planner.Generation{Generation: 9, StateHash: h("b")}, true, 10},
		{"higher, same hash", planner.Generation{Generation: 2, StateHash: h("a")}, planner.Generation{Generation: 9, StateHash: h("a")}, true, 9},
		{"higher, no hash", planner.Generation{Generation: 2, StateHash: h("a")}, planner.Generation{Generation: 9}, true, 10},
		{"same generation, other hash", planner.Generation{Generation: 3, StateHash: h("a")}, planner.Generation{Generation: 3, StateHash: h("b")}, true, 4},
		{"no stored state", planner.Generation{}, planner.Generation{Generation: 7, StateHash: h("b")}, true, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.ahead, reportAhead(tc.stored, tc.reported))
			if tc.ahead {
				assert.Equal(t, tc.to, recovered(tc.stored, tc.reported))
			}
		})
	}
}

// storedState answers a node's state row and checks that the state's
// encoding carries the row's generation.
func (f *fixture) storedState(node agentcontrol.AgentNode) model.KernelForwardNodeState {
	f.t.Helper()
	var row model.KernelForwardNodeState
	require.NoError(f.t, f.db.First(&row, "node_ref = ?", node.String()).Error)
	state := f.state(node)
	require.Equal(f.t, row.Generation, state.GetGeneration(), "the encoded state carries the row's generation")
	require.Equal(f.t, row.StateHash, state.GetStateHash())
	return row
}

func (f *fixture) reportHolding(node agentcontrol.AgentNode, observed time.Time, generation uint64, hash string) ReportResult {
	f.t.Helper()
	result, err := f.service.RecordReport(f.ctx, node, &forwardv1.NodeForwardReport{
		NodeRef: node.String(), Generation: generation, StateHash: hash, Applied: true,
	}, observed, observed)
	require.NoError(f.t, err)
	return result
}

// After a Control database reset the Agents hold higher generations than
// Control stamps: the first report ahead moves the node's generation past
// it, keeping its hops, and pushes the node; later plans stamp above it.
func TestGenerationRecovery(t *testing.T)         { runGenerationRecovery(t, openSQLite(t)) }
func TestPostgresGenerationRecovery(t *testing.T) { runGenerationRecovery(t, openPostgres(t)) }

func runGenerationRecovery(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(0))
	listener := listen(t)
	before := f.storedState(entry)
	hops := f.state(entry).GetHops()
	require.NotEmpty(t, hops)
	baseReport, basePlan := recoveries[RecoveryReport].Load(), recoveries[RecoveryPlan].Load()

	// Behind and converged reports change nothing.
	at := start.Add(time.Minute)
	assert.False(t, f.reportHolding(entry, at, before.Generation-1, strings.Repeat("c", 64)).Recovered)
	assert.False(t, f.reportHolding(entry, at.Add(time.Second), before.Generation, before.StateHash).Recovered)
	assert.Empty(t, listener.take())
	assert.Equal(t, before, f.storedState(entry))

	// The Agent holds generation 40 with other hops (the state Control
	// stamped before its database was reset).
	at = at.Add(time.Minute)
	result := f.reportHolding(entry, at, 40, strings.Repeat("d", 64))
	assert.True(t, result.Stored)
	assert.True(t, result.Recovered)
	after := f.storedState(entry)
	assert.EqualValues(t, 41, after.Generation)
	assert.Equal(t, before.StateHash, after.StateHash, "the hops are kept")
	assert.Equal(t, hops, f.state(entry).GetHops())
	assert.Equal(t, []string{"forward-11"}, listener.take(), "the node is pushed")
	assert.Equal(t, baseReport+1, recoveries[RecoveryReport].Load())

	// An older report observed before it is dropped and recovers nothing;
	// the Agent applying 41 converges.
	assert.False(t, f.reportHolding(entry, at.Add(-time.Second), 90, "").Stored)
	assert.False(t, f.reportHolding(entry, at.Add(time.Minute), 41, after.StateHash).Recovered)
	assert.EqualValues(t, 41, f.storedState(entry).Generation)
	assert.Empty(t, listener.take())

	// The same generation holding the same hops only lifts the stored one.
	at = at.Add(2 * time.Minute)
	exitBefore := f.storedState(exit)
	assert.True(t, f.reportHolding(exit, at, 17, exitBefore.StateHash).Recovered)
	assert.EqualValues(t, 17, f.storedState(exit).Generation)
	assert.Equal(t, []string{"forward-12"}, listener.take())

	// The same generation with other hops moves one above it.
	assert.True(t, f.reportHolding(exit, at.Add(time.Second), 17, strings.Repeat("e", 64)).Recovered)
	assert.EqualValues(t, 18, f.storedState(exit).Generation)
	listener.take()

	// The next plan that changes the hops stamps above the recovered
	// generations.
	updated := gproto.Clone(route).(*forwardv1.Route)
	updated.Listen.Port = 30443
	_, err := f.service.UpdateRoute(f.ctx, "update-1", updated, route.GetRevision())
	require.NoError(t, err)
	assert.EqualValues(t, 42, f.storedState(entry).Generation)
	assert.EqualValues(t, 18, f.storedState(exit).Generation, "the exit hop did not change")
	assert.Equal(t, basePlan, recoveries[RecoveryPlan].Load(), "nothing was ahead at plan time")

	var body strings.Builder
	WritePrometheus(&body, db)
	assert.Contains(t, body.String(), "# TYPE anixops_forward_generation_recoveries_total counter\n")
	assert.Contains(t, body.String(), `anixops_forward_generation_recoveries_total{reason="report"} `)
}

// A node with a report ahead and no stored state (its plans were refused
// since the reset) gets no state from the report; the next accepted plan
// stamps it above the reported generation.
func TestGenerationRecoveryWithoutState(t *testing.T) {
	runGenerationRecoveryWithoutState(t, openSQLite(t))
}
func TestPostgresGenerationRecoveryWithoutState(t *testing.T) {
	runGenerationRecoveryWithoutState(t, openPostgres(t))
}

func runGenerationRecoveryWithoutState(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	require.NoError(t, db.Where("node_ref = ?", "forward-11").Delete(&model.KernelForwardNodeState{}).Error)
	result := f.reportHolding(entry, start.Add(time.Minute), 25, strings.Repeat("d", 64))
	assert.True(t, result.Stored)
	assert.False(t, result.Recovered)
	_, found, err := f.service.State(f.ctx, "forward-11")
	require.NoError(t, err)
	assert.False(t, found, "a report never makes a state")
	_, err = f.service.ResetNode(f.ctx, "forward-11")
	require.ErrorIs(t, err, ErrNoState)

	basePlan := recoveries[RecoveryPlan].Load()
	f.create("create-1", twoHop(0))
	assert.EqualValues(t, 26, f.storedState(entry).Generation)
	assert.EqualValues(t, 2, f.storedState(exit).Generation, "a node without a report stamps as before")
	assert.Equal(t, basePlan+1, recoveries[RecoveryPlan].Load())
}

// ResetNode moves a node above its stored and reported generations,
// keeping its hops, and pushes it.
func TestResetNode(t *testing.T)         { runResetNode(t, openSQLite(t)) }
func TestPostgresResetNode(t *testing.T) { runResetNode(t, openPostgres(t)) }

func runResetNode(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	f.create("create-1", twoHop(0))
	listener := listen(t)
	before := f.storedState(entry)
	base := recoveries[RecoveryOperator].Load()

	reset, err := f.service.ResetNode(f.ctx, "forward-11")
	require.NoError(t, err)
	assert.Equal(t, NodeReset{NodeRef: "forward-11", Previous: before.Generation, Generation: before.Generation + 1, StateHash: before.StateHash}, reset)
	assert.Equal(t, before.Generation+1, f.storedState(entry).Generation)
	assert.Equal(t, []string{"forward-11"}, listener.take())
	assert.Equal(t, base+1, recoveries[RecoveryOperator].Load())

	// A report behind the stored generation counts too: the operator's
	// generation is above both.
	f.reportHolding(entry, start.Add(time.Minute), 1, strings.Repeat("c", 64))
	require.NoError(t, db.Model(&model.KernelForwardNodeReport{}).Where("node_ref = ?", "forward-11").Update("generation", 70).Error)
	reset, err = f.service.ResetNode(f.ctx, "forward-11")
	require.NoError(t, err)
	assert.EqualValues(t, 70, reset.Reported)
	assert.EqualValues(t, 71, reset.Generation)
	assert.EqualValues(t, 71, f.storedState(entry).Generation)
	listener.take()

	for _, ref := range []string{"forward-99", "proxy-21"} {
		_, err = f.service.ResetNode(f.ctx, ref)
		require.ErrorIs(t, err, ErrNoState, ref)
	}
	for _, ref := range []string{"", "forward", "forward-0", "panel-1", "forward-011"} {
		_, err = f.service.ResetNode(f.ctx, ref)
		require.ErrorIs(t, err, ErrInvalidRequest, ref)
	}
	assert.Empty(t, listener.take())
}
