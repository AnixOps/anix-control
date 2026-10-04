package agentupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Worker timing.
const (
	// DefaultInterval is how often the worker evaluates the campaign.
	DefaultInterval = 5 * time.Second
	// ConfigGrace is how long a node that reconnected with the target
	// version and negotiated config.v1 has to report a failed
	// ConfigStatus before it counts as succeeded (it keeps being watched
	// for the rest of its batch).
	ConfigGrace = time.Minute
	// ackTimeout bounds the wait for an Agent's acknowledgement.
	ackTimeout = 10 * time.Second
	// operationMargin is taken off the reconnect timeout for the
	// operation's deadline, so the Agent stops before Control gives up.
	operationMargin = time.Minute
	// maxConcurrentOffers bounds the dispatches of one evaluation.
	maxConcurrentOffers = 16
	// Operation id prefixes of the agent.upgrade operations.
	upgradeOperationPrefix  = "agent-upgrade-"
	rollbackOperationPrefix = "agent-rollback-"
)

// Worker drives the active campaign. It runs in the singleton-worker
// process, which holds the Agent Control streams it dispatches on.
type Worker struct {
	Service *Service
	// Streams returns the Agent Control streams; a nil result skips the
	// evaluation.
	Streams func() agentstreams.Streams
	// Interval defaults to DefaultInterval.
	Interval time.Duration
}

var observeOnce sync.Once

// Run evaluates the active campaign every Interval until ctx ends. It
// registers HandleObserved with the streams once per process.
func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	if streams := w.streams(); streams != nil {
		observeOnce.Do(func() { streams.OnObserved(w.HandleObserved) })
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Agent upgrade campaign evaluation failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) streams() agentstreams.Streams {
	if w.Streams == nil {
		return nil
	}
	return w.Streams()
}

// Tick evaluates the active campaign once.
func (w *Worker) Tick(ctx context.Context) error {
	db := w.Service.DB.WithContext(ctx)
	var campaign model.AgentUpgradeCampaign
	if err := db.Where("active_slot IS NOT NULL").First(&campaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	streams := w.streams()
	if streams == nil {
		return nil
	}
	var batches []Batch
	if err := json.Unmarshal([]byte(campaign.BatchesJSON), &batches); err != nil || len(batches) == 0 {
		return fmt.Errorf("campaign %s: batches: %v", campaign.ID, err)
	}
	var nodes []model.AgentUpgradeNode
	if err := db.Where("campaign_id = ? AND batch = ?", campaign.ID, campaign.CurrentBatch).Order("order_key").Find(&nodes).Error; err != nil {
		return err
	}
	now := w.Service.now()
	for index := range nodes {
		if err := w.evaluate(ctx, &campaign, &nodes[index], streams, now); err != nil {
			return err
		}
	}
	if campaign.Status == model.AgentUpgradeRollingBack {
		return w.rollBack(ctx, &campaign, nodes, streams, now)
	}
	stats := batchStats(nodes)
	if stats.exceeds(campaign.FailureThresholdPercent) {
		reason := fmt.Sprintf("batch %d: %d of %d offered nodes failed (more than %d%%)", campaign.CurrentBatch+1, stats.failed, stats.offered, campaign.FailureThresholdPercent)
		return w.update(ctx, &campaign, AuditActionRollback, func(c *model.AgentUpgradeCampaign) {
			startRollback(c, now, CodeBatchFailed, reason)
		})
	}
	if campaign.Status != model.AgentUpgradeRunning {
		return nil
	}
	if stats.pending > 0 {
		// The next evaluation sees what the offers changed.
		w.offer(ctx, &campaign, nodes, streams, now)
		return nil
	}
	if stats.inFlight > 0 {
		return nil
	}
	// Every node of the batch has settled: the batch passes once it has
	// lasted its minimum duration (an empty batch passes at once).
	batch := batches[campaign.CurrentBatch]
	started := campaign.CreatedAt
	if campaign.BatchStartedAt != nil {
		started = *campaign.BatchStartedAt
	}
	if len(nodes) > 0 && now.Sub(started) < time.Duration(batch.MinDurationSeconds)*time.Second {
		return nil
	}
	if campaign.CurrentBatch+1 >= len(batches) {
		return w.update(ctx, &campaign, AuditActionFinish, func(c *model.AgentUpgradeCampaign) {
			finish(c, now, model.AgentUpgradeSucceeded, "", fmt.Sprintf("every batch passed (%s)", c.TargetVersion))
		})
	}
	return w.update(ctx, &campaign, "", func(c *model.AgentUpgradeCampaign) {
		c.CurrentBatch++
		c.BatchStartedAt = &now
	})
}

// stats counts a batch's nodes.
type stats struct {
	pending, inFlight, succeeded, failed, skipped, offered int
}

func batchStats(nodes []model.AgentUpgradeNode) stats {
	var s stats
	for _, node := range nodes {
		switch node.State {
		case model.AgentUpgradeNodePending:
			s.pending++
		case model.AgentUpgradeNodeOffered, model.AgentUpgradeNodeUpgrading:
			s.inFlight++
		case model.AgentUpgradeNodeSucceeded:
			s.succeeded++
		case model.AgentUpgradeNodeFailed, model.AgentUpgradeNodeRolledBack:
			s.failed++
		case model.AgentUpgradeNodeSkipped:
			s.skipped++
		}
	}
	s.offered = s.inFlight + s.succeeded + s.failed
	return s
}

// exceeds applies H19: more than threshold percent of the batch's offered
// nodes failed. Skipped and pending nodes do not count.
func (s stats) exceeds(threshold int) bool {
	return s.offered > 0 && s.failed*100 > threshold*s.offered
}

// evaluate closes the loop of one node: an offered or upgrading node
// succeeds when a session that connected after the offer reports the
// target version, and fails when it reports another after the hand-off or
// when the reconnect timeout passes. A succeeded node is watched for the
// rest of its batch: a failed ConfigStatus or a reconnect with another
// version fails it.
func (w *Worker) evaluate(ctx context.Context, campaign *model.AgentUpgradeCampaign, node *model.AgentUpgradeNode, streams agentstreams.Streams, now time.Time) error {
	agentNode := agentcontrol.AgentNode{Kind: node.NodeKind, ID: uint32(node.NodeID)} // #nosec G115 -- stored from a 32-bit agentcontrol.AgentNode.
	session, connected := streams.Session(agentNode)
	switch node.State {
	case model.AgentUpgradeNodeOffered, model.AgentUpgradeNodeUpgrading:
		if node.OfferedAt == nil {
			return nil
		}
		if connected && session.ConnectedAt.After(*node.OfferedAt) && session.SessionID != node.SessionID {
			if agentcontrol.SameAgentVersion(session.AgentVersion, campaign.TargetVersion) {
				failed, err := w.configFailed(ctx, agentNode, session.ConnectedAt)
				if err != nil {
					return err
				}
				if failed != "" {
					return w.fail(ctx, node, now, CodeConfigFailed, failed)
				}
				if negotiated(session, agentcontrol.CapabilityConfig) && now.Sub(session.ConnectedAt) < ConfigGrace {
					return nil
				}
				reconnected := session.ConnectedAt
				return w.updateNode(ctx, node, []string{node.State}, func(n *model.AgentUpgradeNode) {
					n.State, n.ReconnectedAt, n.FinishedAt = model.AgentUpgradeNodeSucceeded, &reconnected, &now
				})
			}
			if node.HandedOffAt != nil && session.ConnectedAt.After(*node.HandedOffAt) {
				return w.fail(ctx, node, now, CodeReverted, fmt.Sprintf("the Agent reconnected with %s after the hand-off, not %s", session.AgentVersion, campaign.TargetVersion))
			}
		}
		if now.Sub(*node.OfferedAt) > time.Duration(campaign.ReconnectTimeoutSeconds)*time.Second {
			return w.fail(ctx, node, now, CodeReconnectTimeout, fmt.Sprintf("no reconnect with %s within %d seconds", campaign.TargetVersion, campaign.ReconnectTimeoutSeconds))
		}
	case model.AgentUpgradeNodeSucceeded:
		if campaign.Status == model.AgentUpgradeRollingBack {
			return nil
		}
		if connected && !agentcontrol.SameAgentVersion(session.AgentVersion, campaign.TargetVersion) {
			return w.fail(ctx, node, now, CodeReverted, fmt.Sprintf("the Agent now reports %s, not %s", session.AgentVersion, campaign.TargetVersion))
		}
		since := node.CreatedAt
		if node.ReconnectedAt != nil {
			since = *node.ReconnectedAt
		}
		failed, err := w.configFailed(ctx, agentNode, since)
		if err != nil {
			return err
		}
		if failed != "" {
			return w.fail(ctx, node, now, CodeConfigFailed, failed)
		}
	}
	return nil
}

func negotiated(session agentstreams.Session, capability string) bool {
	for _, name := range session.NegotiatedCapabilities {
		if name == capability+"."+agentcontrol.CapabilityVersionV1 {
			return true
		}
	}
	return false
}

// configFailed returns the error of a failed ConfigStatus the node's
// Agent reported at or after since, or "".
func (w *Worker) configFailed(ctx context.Context, node agentcontrol.AgentNode, since time.Time) (string, error) {
	var status model.KernelNodeConfigStatus
	err := w.Service.DB.WithContext(ctx).Where("node_kind = ? AND node_id = ?", node.Kind, node.ID).First(&status).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if status.Verdict != model.ConfigVerdictFailed || status.ReportedAt.Before(since) {
		return "", nil
	}
	message := "the Agent could not apply its configuration after the upgrade"
	if status.ReportedError != "" {
		message += ": " + status.ReportedError
	}
	return message, nil
}

// offer sends agent.upgrade to the batch's pending nodes. A node that is
// offline stays pending until its batch's minimum duration has passed,
// then is skipped; one whose Agent does not negotiate upgrade.v1 is
// skipped; one that already runs the target succeeds without an offer.
func (w *Worker) offer(ctx context.Context, campaign *model.AgentUpgradeCampaign, nodes []model.AgentUpgradeNode, streams agentstreams.Streams, now time.Time) {
	var artifacts []agentcontrol.UpgradeArtifact
	if err := json.Unmarshal([]byte(campaign.ArtifactsJSON), &artifacts); err != nil {
		log.Printf("Agent upgrade campaign %s: artifacts: %v", campaign.ID, err)
		return
	}
	var batches []Batch
	_ = json.Unmarshal([]byte(campaign.BatchesJSON), &batches)
	batchEnd := now
	if campaign.BatchStartedAt != nil && campaign.CurrentBatch < len(batches) {
		batchEnd = campaign.BatchStartedAt.Add(time.Duration(batches[campaign.CurrentBatch].MinDurationSeconds) * time.Second)
	}
	semaphore := make(chan struct{}, maxConcurrentOffers)
	var wg sync.WaitGroup
	for index := range nodes {
		node := &nodes[index]
		if node.State != model.AgentUpgradeNodePending {
			continue
		}
		agentNode := agentcontrol.AgentNode{Kind: node.NodeKind, ID: uint32(node.NodeID)} // #nosec G115 -- stored from a 32-bit agentcontrol.AgentNode.
		session, connected := streams.Session(agentNode)
		switch {
		case !connected:
			if !now.Before(batchEnd) {
				w.logged(w.skip(ctx, node, now, CodeOffline, "the node's Agent was not connected during its batch"))
			}
			continue
		case agentcontrol.SameAgentVersion(session.AgentVersion, campaign.TargetVersion):
			w.logged(w.updateNode(ctx, node, []string{model.AgentUpgradeNodePending}, func(n *model.AgentUpgradeNode) {
				n.State, n.FromVersion, n.FinishedAt = model.AgentUpgradeNodeSucceeded, session.AgentVersion, &now
				n.ReconnectedAt = &session.ConnectedAt
			}))
			continue
		case !negotiated(session, agentcontrol.CapabilityUpgrade):
			w.logged(w.skip(ctx, node, now, CodeUnsupported, fmt.Sprintf("the Agent (%s) does not negotiate upgrade.v1: re-run the installer to upgrade it", session.AgentVersion)))
			continue
		}
		request := agentcontrol.UpgradeRequest{
			Schema: agentcontrol.UpgradeSchemaV1, CampaignID: campaign.ID, Action: agentcontrol.UpgradeActionUpgrade,
			TargetVersion: campaign.TargetVersion, Artifacts: artifacts,
		}
		if releaseTag(session.AgentVersion) != "" {
			request.PreviousVersion = releaseTag(session.AgentVersion)
		}
		semaphore <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()
			w.logged(w.dispatch(ctx, campaign, node, agentNode, session, request, upgradeOperationPrefix, now, streams))
		}()
	}
	wg.Wait()
}

// releaseTag returns version as a v-prefixed release tag, or "".
func releaseTag(version string) string {
	tag := "v" + strings.TrimPrefix(strings.TrimSpace(version), "v")
	if (agentcontrol.UpgradeRequest{Schema: agentcontrol.UpgradeSchemaV1, CampaignID: "x", Action: agentcontrol.UpgradeActionRollback, TargetVersion: tag}).Validate() != nil {
		return ""
	}
	return tag
}

// dispatch sends one agent.upgrade (an upgrade or a rollback) and records
// the acknowledgement.
func (w *Worker) dispatch(ctx context.Context, campaign *model.AgentUpgradeCampaign, node *model.AgentUpgradeNode, agentNode agentcontrol.AgentNode, session agentstreams.Session, request agentcontrol.UpgradeRequest, prefix string, now time.Time, streams agentstreams.Streams) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	operationID := prefix + uuid.NewString()
	deadline := now.Add(time.Duration(campaign.ReconnectTimeoutSeconds)*time.Second - operationMargin)
	dispatchCtx, cancel := context.WithTimeout(ctx, ackTimeout)
	defer cancel()
	ack, err := streams.Dispatch(dispatchCtx, agentNode, &agentv1pb.DesiredOperation{
		OperationId: operationID, Kind: agentcontrol.OperationKindAgentUpgrade, PayloadJson: payload, DeadlineUnixMs: deadline.UnixMilli(),
	})
	rollback := prefix == rollbackOperationPrefix
	switch {
	case errors.Is(err, agentstreams.ErrCapabilityMissing):
		if rollback {
			return w.updateNode(ctx, node, []string{node.State}, func(n *model.AgentUpgradeNode) {
				n.ErrorCode, n.Error, n.RollbackSentAt = CodeRollbackFailed, "the Agent does not negotiate upgrade.v1", &now
			})
		}
		return w.skip(ctx, node, now, CodeUnsupported, "the Agent does not negotiate upgrade.v1")
	case err != nil:
		// Offline, busy or a lost acknowledgement: tried again on the next
		// evaluation (a replayed operation is idempotent on the Agent).
		return nil
	}
	if rollback {
		return w.updateNode(ctx, node, []string{node.State}, func(n *model.AgentUpgradeNode) {
			n.RollbackOperationID, n.RollbackSentAt = operationID, &now
			if !ack.GetAccepted() {
				n.ErrorCode, n.Error = CodeRollbackFailed, truncate("the Agent refused the rollback: "+ack.GetError(), 1024)
			}
		})
	}
	if !ack.GetAccepted() {
		return w.updateNode(ctx, node, []string{model.AgentUpgradeNodePending}, func(n *model.AgentUpgradeNode) {
			n.State, n.OperationID, n.SessionID, n.FromVersion = model.AgentUpgradeNodeFailed, operationID, session.SessionID, session.AgentVersion
			n.OfferedAt, n.FinishedAt = &now, &now
			n.ErrorCode, n.Error = CodeRejected, truncate("the Agent refused the upgrade: "+ack.GetError(), 1024)
		})
	}
	return w.updateNode(ctx, node, []string{model.AgentUpgradeNodePending}, func(n *model.AgentUpgradeNode) {
		n.State, n.OperationID, n.SessionID, n.FromVersion, n.OfferedAt = model.AgentUpgradeNodeOffered, operationID, session.SessionID, session.AgentVersion, &now
	})
}

// rollBack tells the batch's upgraded nodes to reinstate their previous
// release and ends the campaign rolled_back when every one of them runs
// another version than the target, or after twice the reconnect timeout
// (in-flight upgrades settle, then rollbacks reconnect); unconfirmed nodes
// keep rollback_unconfirmed.
func (w *Worker) rollBack(ctx context.Context, campaign *model.AgentUpgradeCampaign, nodes []model.AgentUpgradeNode, streams agentstreams.Streams, now time.Time) error {
	pending := 0
	for index := range nodes {
		node := &nodes[index]
		switch node.State {
		case model.AgentUpgradeNodeOffered, model.AgentUpgradeNodeUpgrading:
			pending++
			continue
		case model.AgentUpgradeNodeSucceeded, model.AgentUpgradeNodeFailed:
		default:
			continue
		}
		agentNode := agentcontrol.AgentNode{Kind: node.NodeKind, ID: uint32(node.NodeID)} // #nosec G115 -- stored from a 32-bit agentcontrol.AgentNode.
		session, connected := streams.Session(agentNode)
		upgraded := connected && agentcontrol.SameAgentVersion(session.AgentVersion, campaign.TargetVersion)
		switch {
		case node.RollbackSentAt == nil && upgraded:
			previous := releaseTag(node.FromVersion)
			if previous == "" {
				w.logged(w.updateNode(ctx, node, []string{node.State}, func(n *model.AgentUpgradeNode) {
					n.ErrorCode, n.Error, n.RollbackSentAt = CodeRollbackFailed, "the version the node ran before is unknown", &now
				}))
				continue
			}
			pending++
			w.logged(w.dispatch(ctx, campaign, node, agentNode, session, agentcontrol.UpgradeRequest{
				Schema: agentcontrol.UpgradeSchemaV1, CampaignID: campaign.ID, Action: agentcontrol.UpgradeActionRollback,
				TargetVersion: previous, PreviousVersion: releaseTag(campaign.TargetVersion),
			}, rollbackOperationPrefix, now, streams))
		case node.RollbackSentAt != nil && node.ErrorCode != CodeRollbackFailed:
			if connected && !upgraded && session.ConnectedAt.After(*node.RollbackSentAt) {
				w.logged(w.updateNode(ctx, node, []string{node.State}, func(n *model.AgentUpgradeNode) {
					n.State, n.FinishedAt = model.AgentUpgradeNodeRolledBack, &now
				}))
				continue
			}
			pending++
		case node.RollbackSentAt == nil && !connected && node.State == model.AgentUpgradeNodeSucceeded:
			// Upgraded but offline: wait for it within the deadline.
			pending++
		}
	}
	started := now
	if campaign.RollbackStartedAt != nil {
		started = *campaign.RollbackStartedAt
	}
	deadline := started.Add(2 * time.Duration(campaign.ReconnectTimeoutSeconds) * time.Second)
	if pending > 0 && now.Before(deadline) {
		return nil
	}
	if pending > 0 {
		if err := w.Service.DB.WithContext(ctx).Model(&model.AgentUpgradeNode{}).
			Where("campaign_id = ? AND batch = ? AND state IN ? AND rollback_sent_at IS NOT NULL AND error_code <> ?", campaign.ID, campaign.CurrentBatch,
				[]string{model.AgentUpgradeNodeSucceeded, model.AgentUpgradeNodeFailed}, CodeRollbackFailed).
			Updates(map[string]any{"error_code": CodeRollbackUnconfirm, "error": "the Agent did not reconnect with its previous release in time", "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return w.update(ctx, campaign, AuditActionFinish, func(c *model.AgentUpgradeCampaign) {
		finish(c, now, model.AgentUpgradeRolledBack, "", "")
	})
}

// HandleObserved records the progress an Agent reports for an
// agent.upgrade operation (registered with the streams by Run).
func (w *Worker) HandleObserved(node agentcontrol.AgentNode, observed *agentv1pb.ObservedState) {
	id := observed.GetOperationId()
	if !strings.HasPrefix(id, upgradeOperationPrefix) && !strings.HasPrefix(id, rollbackOperationPrefix) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.observe(ctx, node, observed); err != nil {
		log.Printf("Agent upgrade: recording %s of %s: %v", id, node, err)
	}
}

func (w *Worker) observe(ctx context.Context, agentNode agentcontrol.AgentNode, observed *agentv1pb.ObservedState) error {
	id := observed.GetOperationId()
	column := "operation_id"
	if strings.HasPrefix(id, rollbackOperationPrefix) {
		column = "rollback_operation_id"
	}
	var node model.AgentUpgradeNode
	err := w.Service.DB.WithContext(ctx).Where(column+" = ? AND node_kind = ? AND node_id = ?", id, agentNode.Kind, agentNode.ID).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var state agentcontrol.UpgradeState
	_ = json.Unmarshal(observed.GetStateJson(), &state)
	now := w.Service.now()
	message := truncate(observed.GetMessage(), 900)
	code := state.ErrorCode
	if column == "rollback_operation_id" {
		if observed.GetPhase() == agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED || observed.GetPhase() == agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED {
			return w.updateNode(ctx, &node, []string{node.State}, func(n *model.AgentUpgradeNode) {
				n.ErrorCode, n.Error = CodeRollbackFailed, truncate(strings.TrimSpace("the rollback failed: "+code+" "+message), 1024)
			})
		}
		return nil
	}
	inFlight := []string{model.AgentUpgradeNodeOffered, model.AgentUpgradeNodeUpgrading}
	switch observed.GetPhase() {
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_ACCEPTED, agentv1pb.ObservedPhase_OBSERVED_PHASE_APPLYING:
		return w.updateNode(ctx, &node, []string{model.AgentUpgradeNodeOffered}, func(n *model.AgentUpgradeNode) {
			n.State = model.AgentUpgradeNodeUpgrading
		})
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_SUCCEEDED:
		if state.Phase == agentcontrol.UpgradePhaseCurrent {
			return w.updateNode(ctx, &node, inFlight, func(n *model.AgentUpgradeNode) {
				n.State, n.ReconnectedAt, n.FinishedAt = model.AgentUpgradeNodeSucceeded, &now, &now
			})
		}
		return w.updateNode(ctx, &node, inFlight, func(n *model.AgentUpgradeNode) {
			n.State, n.HandedOffAt = model.AgentUpgradeNodeUpgrading, &now
		})
	case agentv1pb.ObservedPhase_OBSERVED_PHASE_FAILED, agentv1pb.ObservedPhase_OBSERVED_PHASE_SUPERSEDED:
		if code == "" {
			code = CodeApplyFailed
		}
		return w.fail(ctx, &node, now, CodeApplyFailed, strings.TrimSpace(code+": "+message))
	}
	return nil
}

func (w *Worker) fail(ctx context.Context, node *model.AgentUpgradeNode, now time.Time, code, message string) error {
	return w.updateNode(ctx, node, []string{model.AgentUpgradeNodeOffered, model.AgentUpgradeNodeUpgrading, model.AgentUpgradeNodeSucceeded}, func(n *model.AgentUpgradeNode) {
		n.State, n.ErrorCode, n.Error, n.FinishedAt = model.AgentUpgradeNodeFailed, code, truncate(message, 1024), &now
	})
}

func (w *Worker) skip(ctx context.Context, node *model.AgentUpgradeNode, now time.Time, code, message string) error {
	return w.updateNode(ctx, node, []string{model.AgentUpgradeNodePending}, func(n *model.AgentUpgradeNode) {
		n.State, n.ErrorCode, n.Error, n.FinishedAt = model.AgentUpgradeNodeSkipped, code, truncate(message, 1024), &now
	})
}

// updateNode changes a node row only while it is in one of the states
// from (the observed-state handler and the evaluation race); node is
// updated in place when the row changed.
func (w *Worker) updateNode(ctx context.Context, node *model.AgentUpgradeNode, from []string, change func(*model.AgentUpgradeNode)) error {
	next := *node
	change(&next)
	next.UpdatedAt = w.Service.now()
	result := w.Service.DB.WithContext(ctx).Model(&model.AgentUpgradeNode{}).Where("id = ? AND state IN ?", node.ID, from).
		Select("state", "from_version", "operation_id", "session_id", "rollback_operation_id", "error_code", "error",
			"offered_at", "handed_off_at", "reconnected_at", "finished_at", "rollback_sent_at", "updated_at").
		Updates(&next)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		*node = next
	}
	return nil
}

// update changes the campaign while it still has the status it was read
// with, and audits automatic transitions as the system.
func (w *Worker) update(ctx context.Context, campaign *model.AgentUpgradeCampaign, action string, change func(*model.AgentUpgradeCampaign)) error {
	before := campaign.Status
	next := *campaign
	change(&next)
	next.UpdatedAt = w.Service.now()
	return w.Service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentUpgradeCampaign{}).Where("id = ? AND status = ? AND current_batch = ?", campaign.ID, before, campaign.CurrentBatch).
			Select("*").Omit("id", "created_at").Updates(&next)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		*campaign = next
		if action == "" {
			return nil
		}
		return writeAudit(tx, Actor{}, action, map[string]any{
			"campaign_id": next.ID, "from": before, "to": next.Status, "batch": next.CurrentBatch + 1,
			"error_code": next.ErrorCode, "reason": next.StatusReason,
		})
	})
}

func (w *Worker) logged(err error) {
	if err != nil {
		log.Printf("Agent upgrade campaign: %v", err)
	}
}
