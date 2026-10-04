package kernelforward

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func counter(routeID string, hop uint32, epoch string, up, down uint64) *forwardv1.Counters {
	return &forwardv1.Counters{
		RouteId: routeID, HopIndex: hop, CounterEpoch: epoch, UpBytes: up, DownBytes: down,
		UpPackets: up / 10, DownPackets: down / 10, TotalConns: up / 100, ActiveConns: uint32(down / 1000), // #nosec G115 -- test values.
	}
}

func (f *fixture) report(node agentcontrol.AgentNode, observed time.Time, generation uint64, counters ...*forwardv1.Counters) ReportResult {
	f.t.Helper()
	result, err := f.service.RecordReport(f.ctx, node, &forwardv1.NodeForwardReport{
		NodeRef: node.String(), Generation: generation, Applied: true, Counters: counters,
	}, observed, observed)
	require.NoError(f.t, err)
	return result
}

func (f *fixture) stats(routeID string) map[string]*forwardv1.Counters {
	f.t.Helper()
	counters, err := f.service.RouteStats(f.ctx, routeID)
	require.NoError(f.t, err)
	out := map[string]*forwardv1.Counters{}
	for _, c := range counters {
		out[c.GetNodeRef()] = c
	}
	return out
}

// The traffic ledger: growth within an epoch is added once, a decrease adds
// nothing, a reset or a re-created hop adds its new epoch in full, and an
// older report is dropped whole.
func TestLedgerEpochs(t *testing.T)         { runLedgerEpochs(t, openSQLite(t)) }
func TestPostgresLedgerEpochs(t *testing.T) { runLedgerEpochs(t, openPostgres(t)) }

func runLedgerEpochs(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(0))
	id := route.GetId()
	at := start.Add(time.Minute)

	result := f.report(entry, at, 2, counter(id, 0, "e1", 100, 1000))
	assert.True(t, result.Stored)
	assert.Equal(t, 1, result.Metered)
	assert.EqualValues(t, 100, f.stats(id)["forward-11"].GetUpBytes())

	// The same values again: nothing grows.
	at = at.Add(time.Minute)
	assert.Zero(t, f.report(entry, at, 2, counter(id, 0, "e1", 100, 1000)).Metered)
	// Growth within the epoch adds the difference.
	at = at.Add(time.Minute)
	assert.Equal(t, 1, f.report(entry, at, 2, counter(id, 0, "e1", 150, 1500)).Metered)
	// A field that went down adds nothing and keeps the stored value; the
	// other field's growth counts.
	at = at.Add(time.Minute)
	f.report(entry, at, 2, counter(id, 0, "e1", 120, 1600))
	stats := f.stats(id)["forward-11"]
	assert.EqualValues(t, 150, stats.GetUpBytes())
	assert.EqualValues(t, 1600, stats.GetDownBytes())
	// A reset: the new epoch counts in full.
	at = at.Add(time.Minute)
	f.report(entry, at, 2, counter(id, 0, "e2", 10, 20))
	stats = f.stats(id)["forward-11"]
	assert.EqualValues(t, 160, stats.GetUpBytes())
	assert.EqualValues(t, 1620, stats.GetDownBytes())
	assert.EqualValues(t, 16, stats.GetUpPackets())
	assert.Empty(t, stats.GetCounterEpoch(), "summed over epochs")
	assert.EqualValues(t, 0, stats.GetActiveConns(), "the latest epoch's")
	// An older report is dropped whole, counters too.
	result = f.report(entry, at.Add(-30*time.Second), 2, counter(id, 0, "e3", 999, 999))
	assert.False(t, result.Stored)
	assert.Zero(t, result.Metered)
	assert.EqualValues(t, 160, f.stats(id)["forward-11"].GetUpBytes())
	// The hop re-created: another epoch, and the exit's own hop.
	at = at.Add(time.Hour)
	f.report(entry, at, 2, counter(id, 0, "e2", 30, 20), counter(id, 0, "e3", 5, 5))
	f.report(exit, at, 2, counter(id, 1, "x1", 7, 9))
	stats = f.stats(id)["forward-11"]
	assert.EqualValues(t, 185, stats.GetUpBytes(), "150 + 30 + 5")
	assert.EqualValues(t, 1625, stats.GetDownBytes(), "1600 + 20 + 5")
	assert.EqualValues(t, 1, f.stats(id)["forward-12"].GetHopIndex())
	assert.EqualValues(t, 7, f.stats(id)["forward-12"].GetUpBytes())

	// The hourly ledger holds the same growth, split by hour.
	traffic, err := f.service.Traffic(f.ctx, id, start, at.Add(time.Hour))
	require.NoError(t, err)
	var up, down uint64
	hours := map[time.Time]bool{}
	for _, bucket := range traffic {
		if bucket.HopIndex == 0 {
			up, down = up+bucket.UpBytes, down+bucket.DownBytes
		}
		hours[bucket.HourStart.UTC()] = true
	}
	assert.EqualValues(t, 185, up)
	assert.EqualValues(t, 1625, down)
	assert.Len(t, hours, 2)

	// A counter beyond the columns' range is refused.
	_, err = f.service.RecordReport(f.ctx, entry, &forwardv1.NodeForwardReport{
		NodeRef: "forward-11", Counters: []*forwardv1.Counters{{RouteId: id, CounterEpoch: "e9", UpBytes: math.MaxUint64}},
	}, at, at)
	require.ErrorIs(t, err, ErrInvalidReport)
	_, err = f.service.RouteStats(f.ctx, "01JF1A000000000000000000ZZ")
	require.ErrorIs(t, err, ErrNotFound)
}

// The latest report per node: health by route, hop errors, and how far
// each node runs behind its desired generation.
func TestReportsAndConvergence(t *testing.T)         { runReportsAndConvergence(t, openSQLite(t)) }
func TestPostgresReportsAndConvergence(t *testing.T) { runReportsAndConvergence(t, openPostgres(t)) }

func runReportsAndConvergence(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(0))
	id := route.GetId()
	nodes, err := NodeConvergence(f.ctx, db)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.False(t, nodes[0].Reported)
	assert.EqualValues(t, 2, nodes[0].Lag(), "never reported: the desired generation")
	assert.False(t, nodes[0].Converged())

	at := start.Add(time.Minute)
	_, err = f.service.RecordReport(f.ctx, entry, &forwardv1.NodeForwardReport{
		NodeRef: "forward-11", Generation: 1, StateHash: strings.Repeat("a", 64), Applied: false,
		Errors: []*forwardv1.HopError{{RouteId: id, Engine: forwardv1.Engine_ENGINE_NFTABLES, Message: "nft: no table"}},
		Health: []*forwardv1.UpstreamHealth{
			{RouteId: id, Address: "192.0.2.12", Port: DefaultPortFirst, State: forwardv1.HealthState_HEALTH_STATE_HEALTHY, RttUs: 900},
			{RouteId: "01JF1A000000000000000000ZZ", Address: "198.51.100.1", Port: 1},
		},
		Counters: []*forwardv1.Counters{counter(id, 0, "e1", 1, 1)},
	}, at, at)
	require.NoError(t, err)
	latest, found, err := f.service.LatestReport(f.ctx, "forward-11")
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, latest.GetCounters(), "counters go to the ledger")
	assert.Equal(t, at.UnixMilli(), latest.GetObservedAtUnixMs())
	_, found, err = f.service.LatestReport(f.ctx, "forward-12")
	require.NoError(t, err)
	assert.False(t, found)

	health, err := f.service.RouteHealth(f.ctx, id)
	require.NoError(t, err)
	require.Len(t, health, 1)
	assert.EqualValues(t, 900, health[0].GetRttUs())
	_, err = f.service.RouteHealth(f.ctx, "01JF1A000000000000000000ZZ")
	require.ErrorIs(t, err, ErrNotFound)

	nodes, err = NodeConvergence(f.ctx, db)
	require.NoError(t, err)
	assert.Equal(t, "forward-11", nodes[0].NodeRef)
	assert.True(t, nodes[0].Reported)
	assert.EqualValues(t, 1, nodes[0].Lag())
	assert.Equal(t, 1, nodes[0].HopErrors)

	var body strings.Builder
	WritePrometheus(&body, db)
	text := body.String()
	assert.Contains(t, text, "anixops_forward_nodes 2\n")
	assert.Contains(t, text, "anixops_forward_lagging_nodes 2\n")
	assert.Contains(t, text, "anixops_forward_unreported_nodes 1\n")
	assert.Contains(t, text, "anixops_forward_generation_lag_max 2\n")
	assert.Contains(t, text, "anixops_forward_hop_errors 1\n")
	assert.Contains(t, text, "anixops_forward_plan_refused 0\n")

	at = at.Add(time.Minute)
	f.report(entry, at, 2)
	f.report(exit, at, 2)
	nodes, err = NodeConvergence(f.ctx, db)
	require.NoError(t, err)
	for _, node := range nodes {
		assert.True(t, node.Converged(), node.NodeRef)
		assert.Zero(t, node.Lag(), node.NodeRef)
	}
	body.Reset()
	WritePrometheus(&body, db)
	assert.Contains(t, body.String(), "anixops_forward_lagging_nodes 0\n")
	body.Reset()
	WritePrometheus(&body, openBare(t))
	assert.Empty(t, body.String(), "no tables, no gauges")
	WritePrometheus(&body, nil)
	assert.Empty(t, body.String())
}

// Control is authoritative for quota and expiry: a route whose entry
// traffic reaches its quota, or whose expiry passed, is paused in the
// nodes' states without changing the stored route; raising the quota
// lifts it.
func TestEnforcement(t *testing.T)         { runEnforcement(t, openSQLite(t)) }
func TestPostgresEnforcement(t *testing.T) { runEnforcement(t, openPostgres(t)) }

func runEnforcement(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	limited := twoHop(0)
	limited.Limits = &forwardv1.Limits{QuotaBytes: 1000}
	route := f.create("create-1", limited)
	id := route.GetId()
	listener := listen(t)
	at := start.Add(time.Minute)
	result := f.report(entry, at, 2, counter(id, 0, "e1", 400, 500), counter(id, 1, "e1", 900, 900))
	assert.False(t, result.Replanned, "900 of 1000 bytes on the entry; the exit's do not count")
	result = f.report(entry, at.Add(time.Minute), 2, counter(id, 0, "e1", 500, 500))
	assert.True(t, result.Replanned)
	assert.ElementsMatch(t, []string{"forward-11", "forward-12"}, listener.take())
	assert.True(t, f.state(entry).GetHops()[0].GetPaused())
	assert.True(t, f.state(exit).GetHops()[0].GetPaused())
	var row model.KernelForwardRoute
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	assert.Equal(t, EnforcedQuota, row.Enforced)
	stored, err := f.service.GetRoute(f.ctx, id)
	require.NoError(t, err)
	assert.False(t, stored.GetPaused(), "the stored route is unchanged")
	// More traffic on a paused route replans nothing.
	assert.False(t, f.report(entry, at.Add(2*time.Minute), 2, counter(id, 0, "e1", 600, 600)).Replanned)

	raised := proto.Clone(stored).(*forwardv1.Route)
	raised.Limits.QuotaBytes = 5000
	_, err = f.service.UpdateRoute(f.ctx, "raise", raised, stored.GetRevision())
	require.NoError(t, err)
	assert.False(t, f.state(entry).GetHops()[0].GetPaused())
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	assert.Empty(t, row.Enforced)

	// Expiry: the maintenance pauses the route once its time passed.
	expiring := proto.Clone(raised).(*forwardv1.Route)
	expiring.Revision = 0
	expiring.Limits.ExpiresAtUnixMs = start.Add(2 * time.Hour).UnixMilli()
	_, err = f.service.UpdateRoute(f.ctx, "expiring", expiring, 2)
	require.NoError(t, err)
	listener.take()
	require.NoError(t, f.service.Maintain(f.ctx))
	assert.Empty(t, listener.take(), "not expired yet")
	f.clock.Advance(3 * time.Hour)
	require.NoError(t, f.service.Maintain(f.ctx))
	assert.ElementsMatch(t, []string{"forward-11", "forward-12"}, listener.take())
	assert.True(t, f.state(entry).GetHops()[0].GetPaused())
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	assert.Equal(t, EnforcedExpired, row.Enforced)
	require.NoError(t, f.service.Maintain(f.ctx))
	assert.Empty(t, listener.take(), "an expired route is replanned once")
}
