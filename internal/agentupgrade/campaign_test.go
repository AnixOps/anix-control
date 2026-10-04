package agentupgrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateBatches(t *testing.T) {
	require.NoError(t, ValidateBatches(DefaultBatches()))
	require.NoError(t, ValidateBatches([]Batch{{Percent: 1, MinDurationSeconds: 1800}, {Percent: 100, MinDurationSeconds: 3600}}))
	for name, batches := range map[string][]Batch{
		"none":            nil,
		"canary too big":  {{Percent: 10, MinDurationSeconds: 1800}, {Percent: 100, MinDurationSeconds: 1800}},
		"not increasing":  {{Percent: 5, MinDurationSeconds: 1800}, {Percent: 5, MinDurationSeconds: 1800}, {Percent: 100, MinDurationSeconds: 1800}},
		"short":           {{Percent: 5, MinDurationSeconds: 1799}, {Percent: 100, MinDurationSeconds: 1800}},
		"not to 100":      {{Percent: 5, MinDurationSeconds: 1800}, {Percent: 50, MinDurationSeconds: 1800}},
		"over 100":        {{Percent: 5, MinDurationSeconds: 1800}, {Percent: 101, MinDurationSeconds: 1800}},
		"too many":        make([]Batch, MaxBatches+1),
		"single over cap": {{Percent: 100, MinDurationSeconds: 1800}},
	} {
		assert.ErrorIs(t, ValidateBatches(batches), ErrInvalid, name)
	}
}

func TestBatchBounds(t *testing.T) {
	assert.Equal(t, []int{0, 2, 10, 40}, BatchBounds(40, DefaultBatches()))
	assert.Equal(t, []int{0, 1, 1, 3}, BatchBounds(3, DefaultBatches()))
	assert.Equal(t, []int{0, 1, 1, 1}, BatchBounds(1, DefaultBatches()))
	assert.Equal(t, []int{0, 5, 25, 100}, BatchBounds(100, DefaultBatches()))
	assert.Equal(t, []int{0, 51, 251, 1001}, BatchBounds(1001, DefaultBatches()))
}

func TestCandidatesAreDeterministicAndExcluded(t *testing.T) {
	db := openSQLite(t)
	f := newFixture(t, db)
	f.addProxy(1, `["edge"]`)
	f.addProxy(2, "")
	f.addProxy(3, "core, edge")
	f.addForward(1, `["core"]`)
	f.addForward(2, "")
	disabled := f.addForward(3, "")
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", disabled.ID).Update("enabled", false).Error)
	// Seen only on UniProxy: not an AnixOps Agent on the stream.
	require.NoError(t, db.Create(&model.Node{ID: 9, Name: "third-party", APIKey: "key-9", Status: model.NodeStatusOnline}).Error)
	f.seen(agentcontrol.NodeKindProxy, 9, model.AgentTransportUniProxy)
	// The API-key stream counts.
	require.NoError(t, db.Create(&model.Node{ID: 10, Name: "apikey", APIKey: "key-10", Status: model.NodeStatusOnline}).Error)
	f.seen(agentcontrol.NodeKindProxy, 10, model.AgentTransportAPIKeyStream)

	nodes, err := Candidates(context.Background(), db, Exclude{})
	require.NoError(t, err)
	names := make([]string, 0, len(nodes))
	for index, node := range nodes {
		names = append(names, node.String())
		if index > 0 {
			assert.Less(t, OrderKey(nodes[index-1]), OrderKey(node), "canary order is the name hash")
		}
	}
	assert.ElementsMatch(t, []string{"proxy-1", "proxy-2", "proxy-3", "proxy-10", "forward-1", "forward-2"}, names)
	again, err := Candidates(context.Background(), db, Exclude{})
	require.NoError(t, err)
	assert.Equal(t, nodes, again)
	assert.Equal(t, OrderKey(agentcontrol.AgentNode{Kind: "proxy", ID: 1}), OrderKey(agentcontrol.AgentNode{Kind: "proxy", ID: 1}))

	excluded, err := Candidates(context.Background(), db, Exclude{Nodes: []string{"proxy-2"}, Tags: []string{"edge"}})
	require.NoError(t, err)
	names = names[:0]
	for _, node := range excluded {
		names = append(names, node.String())
	}
	assert.ElementsMatch(t, []string{"proxy-10", "forward-1", "forward-2"}, names)
}

func TestStartRefuses(t *testing.T) {
	db := openSQLite(t)
	f := newFixture(t, db)
	ctx := context.Background()
	_, err := f.service.Start(ctx, StartRequest{TargetVersion: "v4.2.0", ControlVersion: "v4.2.0", Artifacts: testArtifacts()})
	assert.ErrorIs(t, err, ErrNoNodes)
	f.addProxy(1, "")
	for name, request := range map[string]StartRequest{
		"not a tag":          {TargetVersion: "4.2.0", Artifacts: testArtifacts()},
		"newer than Control": {TargetVersion: "v4.3.0", ControlVersion: "v4.2.0", Artifacts: testArtifacts()},
		"no artifacts":       {TargetVersion: "v4.2.0"},
		"bad batches":        {TargetVersion: "v4.2.0", Artifacts: testArtifacts(), Batches: []Batch{{Percent: 100, MinDurationSeconds: 60}}},
		"bad exclusion":      {TargetVersion: "v4.2.0", Artifacts: testArtifacts(), Exclude: Exclude{Nodes: []string{"node-1"}}},
	} {
		_, err := f.service.Start(ctx, request)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	campaign := f.start(StartRequest{Actor: Actor{UserID: 7, Name: "root@example.com", IP: "198.51.100.1"}})
	_, err = f.service.Start(ctx, StartRequest{TargetVersion: "v4.2.0", Artifacts: testArtifacts()})
	assert.ErrorIs(t, err, ErrActiveCampaign)
	var audit model.OperationLog
	require.NoError(t, db.Where("action = ?", AuditActionStart).First(&audit).Error)
	assert.Contains(t, audit.Content, campaign.ID)
	assert.Equal(t, "agent_upgrade", audit.Module)
}

func TestCampaignSucceeds(t *testing.T)         { runCampaignSucceeds(t, openSQLite(t)) }
func TestPostgresCampaignSucceeds(t *testing.T) { runCampaignSucceeds(t, openPostgres(t)) }

// 40 nodes in batches of 2, 8 and 30; one failure in 30 (3.3%) does not
// roll back.
func runCampaignSucceeds(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	for id := uint(1); id <= 20; id++ {
		f.streams.connect(f.addProxy(id, ""), "v4.1.0", start.Add(-time.Hour), "upgrade.v1")
		f.streams.connect(f.addForward(id, ""), "v4.1.0", start.Add(-time.Hour), "upgrade.v1")
	}
	campaign := f.start(StartRequest{})
	view := f.campaign(campaign.ID)
	require.Equal(t, 40, view.Total)
	require.Equal(t, []int{2, 8, 30}, []int{view.Batches[0].Nodes, view.Batches[1].Nodes, view.Batches[2].Nodes})

	f.tick()
	canaries := f.batchNodes(campaign.ID, 0)
	require.Len(t, f.streams.operations(), 2)
	for _, node := range canaries {
		sent := f.lastOperation(node)
		assert.Equal(t, agentcontrol.OperationKindAgentUpgrade, sent.operation.GetKind())
		assert.Equal(t, agentcontrol.UpgradeActionUpgrade, sent.request.Action)
		assert.Equal(t, "v4.2.0", sent.request.TargetVersion)
		assert.Equal(t, "v4.1.0", sent.request.PreviousVersion)
		assert.Equal(t, campaign.ID, sent.request.CampaignID)
		assert.Len(t, sent.request.Artifacts, 2)
		assert.Equal(t, start.Add(ReconnectTimeout-operationMargin).UnixMilli(), sent.operation.GetDeadlineUnixMs())
		assert.Equal(t, model.AgentUpgradeNodeOffered, f.nodeState(campaign.ID, node).State)
	}
	f.tick() // no second offer while in flight
	require.Len(t, f.streams.operations(), 2)

	for _, node := range canaries {
		f.upgrade(node, "v4.2.0")
		assert.Equal(t, model.AgentUpgradeNodeUpgrading, f.nodeState(campaign.ID, node).State)
	}
	f.tick()
	for _, node := range canaries {
		row := f.nodeState(campaign.ID, node)
		assert.Equal(t, model.AgentUpgradeNodeSucceeded, row.State)
		assert.Equal(t, "v4.1.0", row.FromVersion)
		assert.NotNil(t, row.ReconnectedAt)
	}
	// The canary batch lasts 30 minutes.
	f.clock.Advance(20 * time.Minute)
	f.tick()
	assert.Equal(t, 0, f.campaign(campaign.ID).CurrentBatch)
	f.clock.Advance(10 * time.Minute)
	f.tick()
	assert.Equal(t, 1, f.campaign(campaign.ID).CurrentBatch)

	f.tick()
	second := f.batchNodes(campaign.ID, 1)
	require.Len(t, f.streams.operations(), 10)
	for _, node := range second {
		f.upgrade(node, "v4.2.0")
	}
	f.tick()
	f.clock.Advance(MinBatchDuration)
	f.tick()
	require.Equal(t, 2, f.campaign(campaign.ID).CurrentBatch)

	f.tick()
	last := f.batchNodes(campaign.ID, 2)
	require.Len(t, f.streams.operations(), 40)
	for index, node := range last {
		if index == 0 {
			sent := f.lastOperation(node)
			f.worker.HandleObserved(node, &agentv1pb.ObservedState{
				OperationId: sent.operation.GetOperationId(), Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED,
				Message: "signature does not verify", StateJson: []byte(`{"error_code":"upgrade_signature_invalid"}`),
			})
			continue
		}
		f.upgrade(node, "v4.2.0")
	}
	f.tick()
	failed := f.nodeState(campaign.ID, last[0])
	assert.Equal(t, model.AgentUpgradeNodeFailed, failed.State)
	assert.Equal(t, CodeApplyFailed, failed.ErrorCode)
	assert.Contains(t, failed.Error, "upgrade_signature_invalid")
	view = f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeRunning, view.Status)
	assert.Equal(t, 30, view.Batches[2].Offered)
	assert.Equal(t, 1, view.Batches[2].Failed)

	f.clock.Advance(MinBatchDuration)
	f.tick()
	view = f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeSucceeded, view.Status)
	assert.NotNil(t, view.FinishedAt)
	assert.Nil(t, view.ActiveSlot)
	assert.Equal(t, 39, view.States[model.AgentUpgradeNodeSucceeded])
	var finished int64
	require.NoError(t, db.Model(&model.OperationLog{}).Where("action = ?", AuditActionFinish).Count(&finished).Error)
	assert.Equal(t, int64(1), finished)

	// The slot is free again.
	f.start(StartRequest{})
}

func TestCampaignRollsBack(t *testing.T)         { runCampaignRollsBack(t, openSQLite(t)) }
func TestPostgresCampaignRollsBack(t *testing.T) { runCampaignRollsBack(t, openPostgres(t)) }

// One node of eight that does not reconnect within 10 minutes (12.5%)
// rolls the batch back: its upgraded nodes are told to reinstate v4.1.0,
// later batches are never offered.
func runCampaignRollsBack(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	for id := uint(1); id <= 40; id++ {
		f.streams.connect(f.addProxy(id, ""), "v4.1.0", start.Add(-time.Hour), "upgrade.v1")
	}
	campaign := f.start(StartRequest{})
	f.tick()
	for _, node := range f.batchNodes(campaign.ID, 0) {
		f.upgrade(node, "v4.2.0")
	}
	f.tick()
	f.clock.Advance(MinBatchDuration)
	f.tick()
	require.Equal(t, 1, f.campaign(campaign.ID).CurrentBatch)
	f.tick()
	second := f.batchNodes(campaign.ID, 1)
	silent := second[3]
	for _, node := range second {
		if node != silent {
			f.upgrade(node, "v4.2.0")
		}
	}
	f.tick()
	assert.Equal(t, model.AgentUpgradeRunning, f.campaign(campaign.ID).Status)
	f.clock.Advance(ReconnectTimeout + time.Second)
	f.tick()
	row := f.nodeState(campaign.ID, silent)
	assert.Equal(t, model.AgentUpgradeNodeFailed, row.State)
	assert.Equal(t, CodeReconnectTimeout, row.ErrorCode)
	view := f.campaign(campaign.ID)
	require.Equal(t, model.AgentUpgradeRollingBack, view.Status)
	assert.Equal(t, CodeBatchFailed, view.ErrorCode)
	assert.Contains(t, view.StatusReason, "1 of 8")

	sentBefore := len(f.streams.operations())
	f.tick()
	rollbacks := f.streams.operations()[sentBefore:]
	require.Len(t, rollbacks, 7, "every upgraded node of the batch, not the silent one")
	for _, sent := range rollbacks {
		assert.Equal(t, agentcontrol.UpgradeActionRollback, sent.request.Action)
		assert.Equal(t, "v4.1.0", sent.request.TargetVersion)
		assert.Empty(t, sent.request.Artifacts)
		assert.NotEqual(t, silent, sent.node)
	}
	f.tick() // waiting for the rollbacks to reconnect
	assert.Equal(t, model.AgentUpgradeRollingBack, f.campaign(campaign.ID).Status)
	for _, sent := range rollbacks {
		f.clock.Advance(time.Second)
		f.streams.connect(sent.node, "v4.1.0", f.clock.Now(), "upgrade.v1")
	}
	f.tick()
	view = f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeRolledBack, view.Status)
	assert.Nil(t, view.ActiveSlot)
	assert.Equal(t, 7, view.Batches[1].States[model.AgentUpgradeNodeRolledBack])
	assert.Equal(t, 1, view.Batches[1].States[model.AgentUpgradeNodeFailed])
	assert.Equal(t, 2, view.Batches[0].States[model.AgentUpgradeNodeSucceeded], "earlier batches stay upgraded")
	assert.Equal(t, 30, view.Batches[2].States[model.AgentUpgradeNodePending])
	for _, node := range f.batchNodes(campaign.ID, 2) {
		for _, sent := range f.streams.operations() {
			assert.NotEqual(t, node, sent.node, "a later batch is never offered")
		}
	}
	var rollback model.OperationLog
	require.NoError(t, db.Where("action = ?", AuditActionRollback).First(&rollback).Error)
	assert.Equal(t, "system", rollback.Username)
}

// A canary that fails rolls back at once: one of one is over 5%.
func TestCanaryFailureRollsBackAtOnce(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	for id := uint(1); id <= 10; id++ {
		f.streams.connect(f.addProxy(id, ""), "v4.1.0", start, "upgrade.v1")
	}
	campaign := f.start(StartRequest{})
	f.tick()
	canary := f.batchNodes(campaign.ID, 0)[0]
	f.streams.refuse[canary] = "updater unavailable"
	f.streams.sent = nil
	// The refusal was not set when it was offered: refuse the next one by
	// re-offering after a reset of the node.
	require.NoError(t, f.db.Model(&model.AgentUpgradeNode{}).Where("campaign_id = ?", campaign.ID).
		Updates(map[string]any{"state": model.AgentUpgradeNodePending, "offered_at": nil}).Error)
	f.tick()
	row := f.nodeState(campaign.ID, canary)
	assert.Equal(t, model.AgentUpgradeNodeFailed, row.State)
	assert.Equal(t, CodeRejected, row.ErrorCode)
	assert.Contains(t, row.Error, "updater unavailable")
	f.tick()
	view := f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeRollingBack, view.Status)
	f.tick() // nothing to roll back: the canary never upgraded
	assert.Equal(t, model.AgentUpgradeRolledBack, f.campaign(campaign.ID).Status)
}

// Nodes whose Agent does not negotiate upgrade.v1 are skipped without an
// operation; offline nodes stay pending until their batch ends, then are
// skipped. Neither counts towards the failure ratio. An empty batch passes
// at once.
func TestCapabilityGateAndOfflineNodes(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	nodes := []agentcontrol.AgentNode{f.addProxy(1, ""), f.addProxy(2, ""), f.addForward(1, "")}
	campaign := f.start(StartRequest{})
	canary := f.batchNodes(campaign.ID, 0)
	require.Len(t, canary, 1)
	require.Empty(t, f.batchNodes(campaign.ID, 1))
	f.streams.connect(canary[0], "v4.1.0", start, "config.v1", "upgrade.v1")
	f.tick()
	f.upgrade(canary[0], "v4.2.0")
	f.tick()
	// It negotiated config.v1: it has a minute to report a failed status.
	assert.Equal(t, model.AgentUpgradeNodeUpgrading, f.nodeState(campaign.ID, canary[0]).State)
	f.clock.Advance(ConfigGrace)
	f.tick()
	assert.Equal(t, model.AgentUpgradeNodeSucceeded, f.nodeState(campaign.ID, canary[0]).State)
	f.clock.Advance(MinBatchDuration)
	f.tick()
	assert.Equal(t, 1, f.campaign(campaign.ID).CurrentBatch)
	f.tick() // the empty batch passes
	assert.Equal(t, 2, f.campaign(campaign.ID).CurrentBatch)

	last := f.batchNodes(campaign.ID, 2)
	require.Len(t, last, 2)
	old, offline := last[0], last[1]
	f.streams.connect(old, "v3.9.0", f.clock.Now()) // lists no upgrade.v1
	f.tick()
	assert.Len(t, f.streams.operations(), 1, "only the canary was ever sent an operation")
	row := f.nodeState(campaign.ID, old)
	assert.Equal(t, model.AgentUpgradeNodeSkipped, row.State)
	assert.Equal(t, CodeUnsupported, row.ErrorCode)
	assert.Equal(t, model.AgentUpgradeNodePending, f.nodeState(campaign.ID, offline).State)
	f.clock.Advance(MinBatchDuration)
	f.tick()
	row = f.nodeState(campaign.ID, offline)
	assert.Equal(t, model.AgentUpgradeNodeSkipped, row.State)
	assert.Equal(t, CodeOffline, row.ErrorCode)
	f.tick()
	view := f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeSucceeded, view.Status)
	assert.Equal(t, 0, view.Batches[2].Offered)
	assert.Len(t, nodes, 3)
}

// A node that already runs the target succeeds without an operation (a
// host with two node identities upgraded through the other one).
func TestNodeAlreadyAtTarget(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	node := f.addProxy(1, "")
	f.streams.connect(node, "4.2.0", start, "upgrade.v1")
	campaign := f.start(StartRequest{})
	f.tick()
	assert.Empty(t, f.streams.operations())
	assert.Equal(t, model.AgentUpgradeNodeSucceeded, f.nodeState(campaign.ID, node).State)
}

// The loop closes on the new version from a session opened after the
// offer: a reconnect with the old version after the hand-off fails at once,
// a failed configuration after the reconnect fails, and a replay answered
// "current" succeeds.
func TestNodeOutcomes(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	for id := uint(1); id <= 3; id++ {
		f.streams.connect(f.addProxy(id, ""), "v4.1.0", start, "config.v1", "upgrade.v1")
	}
	campaign := f.start(StartRequest{Batches: []Batch{{Percent: 1, MinDurationSeconds: 1800}, {Percent: 100, MinDurationSeconds: 1800}}})
	f.tick()
	f.clock.Advance(MinBatchDuration)
	canary := f.batchNodes(campaign.ID, 0)[0]
	f.streams.connect(canary, "v4.2.0", f.clock.Now(), "config.v1", "upgrade.v1")
	f.worker.HandleObserved(canary, &agentv1pb.ObservedState{OperationId: f.lastOperation(canary).operation.GetOperationId(), Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED, StateJson: []byte(`{"phase":"current"}`)})
	assert.Equal(t, model.AgentUpgradeNodeSucceeded, f.nodeState(campaign.ID, canary).State)
	f.tick()
	require.Equal(t, 1, f.campaign(campaign.ID).CurrentBatch)
	// Two nodes in a batch of two; pause first so that failures are
	// evaluated but nothing advances.
	f.tick()
	rest := f.batchNodes(campaign.ID, 1)
	reverted, misconfigured := rest[0], rest[1]

	// A stream blip before the hand-off is not a failure.
	f.worker.HandleObserved(reverted, &agentv1pb.ObservedState{OperationId: f.lastOperation(reverted).operation.GetOperationId(), Phase: agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING})
	f.clock.Advance(time.Second)
	f.streams.connect(reverted, "v4.1.0", f.clock.Now(), "config.v1", "upgrade.v1")
	f.tick()
	assert.Equal(t, model.AgentUpgradeNodeUpgrading, f.nodeState(campaign.ID, reverted).State)
	// After the hand-off the old version means the updater failed.
	f.upgrade(reverted, "v4.1.0")
	f.tick()
	row := f.nodeState(campaign.ID, reverted)
	assert.Equal(t, model.AgentUpgradeNodeFailed, row.State)
	assert.Equal(t, CodeReverted, row.ErrorCode)

	f.upgrade(misconfigured, "v4.2.0")
	require.NoError(t, f.db.Create(&model.KernelNodeConfigStatus{
		NodeKind: misconfigured.Kind, NodeID: uint64(misconfigured.ID), SessionID: "s", Verdict: model.ConfigVerdictFailed,
		ReportedError: "unknown field", ReportedAt: f.clock.Now().Add(time.Second), CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
	}).Error)
	f.tick()
	row = f.nodeState(campaign.ID, misconfigured)
	assert.Equal(t, model.AgentUpgradeNodeFailed, row.State)
	assert.Equal(t, CodeConfigFailed, row.ErrorCode)
	assert.Contains(t, row.Error, "unknown field")
}

func TestPauseResumeAbort(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	for id := uint(1); id <= 4; id++ {
		f.streams.connect(f.addProxy(id, ""), "v4.1.0", start, "upgrade.v1")
	}
	ctx := context.Background()
	admin := Actor{UserID: 1, Name: "root"}
	campaign := f.start(StartRequest{})
	_, err := f.service.Resume(ctx, campaign.ID, admin)
	assert.ErrorIs(t, err, ErrState)
	_, err = f.service.Pause(ctx, campaign.ID, admin)
	require.NoError(t, err)
	f.tick()
	assert.Empty(t, f.streams.operations(), "a paused campaign offers nothing")
	f.clock.Advance(10 * time.Minute)
	resumed, err := f.service.Resume(ctx, campaign.ID, admin)
	require.NoError(t, err)
	assert.Equal(t, start.Add(10*time.Minute), resumed.BatchStartedAt.UTC(), "paused time does not count")
	f.tick()
	assert.Len(t, f.streams.operations(), 1)

	_, err = f.service.Pause(ctx, "missing", admin)
	assert.ErrorIs(t, err, ErrNotFound)
	aborted, err := f.service.Abort(ctx, campaign.ID, admin, false)
	require.NoError(t, err)
	assert.Equal(t, model.AgentUpgradeAborted, aborted.Status)
	assert.Nil(t, aborted.ActiveSlot)
	_, err = f.service.Abort(ctx, campaign.ID, admin, false)
	assert.ErrorIs(t, err, ErrState)
	f.tick() // an ended campaign is not evaluated
	assert.Len(t, f.streams.operations(), 1)

	// Abort with rollback rolls the current batch back.
	second := f.start(StartRequest{})
	f.tick()
	canary := f.batchNodes(second.ID, 0)[0]
	f.upgrade(canary, "v4.2.0")
	f.tick()
	rolling, err := f.service.Abort(ctx, second.ID, admin, true)
	require.NoError(t, err)
	assert.Equal(t, model.AgentUpgradeRollingBack, rolling.Status)
	sent := len(f.streams.operations())
	f.tick()
	require.Len(t, f.streams.operations(), sent+1)
	assert.Equal(t, agentcontrol.UpgradeActionRollback, f.lastOperation(canary).request.Action)
	f.clock.Advance(time.Second)
	f.streams.connect(canary, "v4.1.0", f.clock.Now(), "upgrade.v1")
	f.tick()
	view := f.campaign(second.ID)
	assert.Equal(t, model.AgentUpgradeRolledBack, view.Status)
	assert.Equal(t, CodeAborted, view.ErrorCode)
	var audits int64
	require.NoError(t, f.db.Model(&model.OperationLog{}).Where("module = ?", "agent_upgrade").Count(&audits).Error)
	assert.GreaterOrEqual(t, audits, int64(7))
}

// A rollback the Agent cannot confirm ends the campaign after twice the
// reconnect timeout, marking the node.
func TestRollbackUnconfirmed(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	node := f.addProxy(1, "")
	f.streams.connect(node, "v4.1.0", start, "upgrade.v1")
	campaign := f.start(StartRequest{})
	f.tick()
	f.upgrade(node, "v4.2.0")
	f.tick()
	_, err := f.service.Abort(context.Background(), campaign.ID, Actor{}, true)
	require.NoError(t, err)
	f.tick()
	assert.Equal(t, agentcontrol.UpgradeActionRollback, f.lastOperation(node).request.Action)
	f.clock.Advance(2*ReconnectTimeout + time.Second)
	f.tick()
	view := f.campaign(campaign.ID)
	assert.Equal(t, model.AgentUpgradeRolledBack, view.Status)
	row := f.nodeState(campaign.ID, node)
	assert.Equal(t, CodeRollbackUnconfirm, row.ErrorCode)
	assert.Equal(t, model.AgentUpgradeNodeSucceeded, row.State)
}

func TestPostgresOneActiveCampaign(t *testing.T) {
	db := openPostgres(t)
	f := newFixture(t, db)
	f.addProxy(1, "")
	f.start(StartRequest{})
	_, err := f.service.Start(context.Background(), StartRequest{TargetVersion: "v4.2.0", Artifacts: testArtifacts()})
	assert.True(t, errors.Is(err, ErrActiveCampaign), err)
	// The unique slot holds even without the check.
	slot := model.AgentUpgradeActiveSlot
	err = db.Create(&model.AgentUpgradeCampaign{ID: "dup", TargetVersion: "v4.2.0", ArtifactsJSON: "[]", BatchesJSON: "[]", ExcludeJSON: "{}",
		Status: model.AgentUpgradeRunning, ActiveSlot: &slot, CreatedAt: start, UpdatedAt: start}).Error
	assert.Error(t, err)
}
