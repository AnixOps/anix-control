// Package agentupgrade runs staged Agent upgrades (forward-sdk.md section 9,
// O4; owner decision H19): Control, never the node, decides when an Agent
// upgrades. A campaign pushes one signed Agent release to the enrolled
// nodes in batches (5% → 25% → 100% by default, at least 30 minutes each),
// canaries first in a deterministic order, as the agent.upgrade operation
// on the Agent Control stream of every Agent that negotiated upgrade.v1.
// A node succeeds when its Agent reconnects with the target version (and
// its configuration still applies); a batch in which more than 5% of the
// offered nodes fail — refused, failed to apply, or did not reconnect with
// the new version within 10 minutes — is rolled back: the campaign stops
// and the upgraded nodes of that batch are told to reinstate their
// previous release.
//
// The campaign lives in two protected tables
// (v4_kernel_agent_upgrade_campaign, v4_kernel_agent_upgrade_node); the
// Worker drives it in the singleton-worker process, which holds the Agent
// streams.
package agentupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

// The H19 rules.
const (
	// FailureThresholdPercent: a batch rolls back when more than this share
	// of its offered nodes fail.
	FailureThresholdPercent = 5
	// ReconnectTimeout: an offered node must reconnect with the target
	// version within this time.
	ReconnectTimeout = 10 * time.Minute
	// MinBatchDuration is the shortest a batch may last.
	MinBatchDuration = 30 * time.Minute
	// FirstBatchMaxPercent caps the canary batch.
	FirstBatchMaxPercent = 5
	// MaxBatches bounds a campaign's batches.
	MaxBatches = 10
)

// DefaultBatches are H19's batches: 5%, 25% and 100% of the nodes
// (cumulative), 30 minutes each.
func DefaultBatches() []Batch {
	return []Batch{
		{Percent: 5, MinDurationSeconds: int(MinBatchDuration / time.Second)},
		{Percent: 25, MinDurationSeconds: int(MinBatchDuration / time.Second)},
		{Percent: 100, MinDurationSeconds: int(MinBatchDuration / time.Second)},
	}
}

// Batch is one step of a campaign: the nodes up to Percent of the
// campaign's nodes (cumulative), which run for at least
// MinDurationSeconds before the next batch starts.
type Batch struct {
	Percent            int `json:"percent"`
	MinDurationSeconds int `json:"min_duration_seconds"`
}

// Exclude names nodes (proxy-<id>, forward-<id>) and node tags a campaign
// leaves out.
type Exclude struct {
	Nodes []string `json:"nodes"`
	Tags  []string `json:"tags"`
}

// Errors of the campaign API; handlers map them to answers.
var (
	ErrInvalid        = errors.New("invalid agent upgrade campaign")
	ErrActiveCampaign = errors.New("another agent upgrade campaign is active")
	ErrNoNodes        = errors.New("no node can be upgraded")
	ErrNotFound       = errors.New("agent upgrade campaign not found")
	ErrState          = errors.New("the campaign is not in a state that allows this")
)

// Error codes recorded on campaigns and nodes.
const (
	CodeBatchFailed       = "batch_failed"
	CodeAborted           = "aborted"
	CodeRejected          = "rejected"
	CodeApplyFailed       = "apply_failed"
	CodeReconnectTimeout  = "reconnect_timeout"
	CodeReverted          = "reverted"
	CodeConfigFailed      = "config_apply_failed"
	CodeUnsupported       = "upgrade_unsupported"
	CodeOffline           = "node_offline"
	CodeRollbackFailed    = "rollback_failed"
	CodeRollbackUnconfirm = "rollback_unconfirmed"
)

// Audit actions (v2_operation_log, module agent_upgrade).
const (
	auditModule         = "agent_upgrade"
	AuditActionStart    = "agent_upgrade_start"
	AuditActionPause    = "agent_upgrade_pause"
	AuditActionResume   = "agent_upgrade_resume"
	AuditActionAbort    = "agent_upgrade_abort"
	AuditActionRollback = "agent_upgrade_rollback"
	AuditActionFinish   = "agent_upgrade_finish"
)

// Service is the campaign store and its rules.
type Service struct {
	DB *gorm.DB
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Actor is who changes a campaign: an administrator (UserID, Name, IP) or
// the command line (UserID 0).
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// StartRequest starts a campaign.
type StartRequest struct {
	TargetVersion string
	// ControlVersion is the running Control's release (H25: the Agent
	// release is Control's); a newer target is refused.
	ControlVersion string
	// Artifacts are the verified release assets
	// (agentinstall.UpgradeArtifacts).
	Artifacts []agentcontrol.UpgradeArtifact
	// Batches default to DefaultBatches.
	Batches []Batch
	Exclude Exclude
	Reason  string
	Actor   Actor
}

// ValidateBatches checks batches against H19: 1 to MaxBatches, cumulative
// percentages strictly increasing and ending at 100, the first at most
// FirstBatchMaxPercent, each at least MinBatchDuration.
func ValidateBatches(batches []Batch) error {
	if len(batches) == 0 || len(batches) > MaxBatches {
		return fmt.Errorf("%w: 1 to %d batches", ErrInvalid, MaxBatches)
	}
	previous := 0
	for index, batch := range batches {
		if batch.Percent <= previous || batch.Percent > 100 {
			return fmt.Errorf("%w: batch percentages are cumulative, strictly increasing, at most 100", ErrInvalid)
		}
		if index == 0 && batch.Percent > FirstBatchMaxPercent {
			return fmt.Errorf("%w: the first (canary) batch takes at most %d%% of the nodes", ErrInvalid, FirstBatchMaxPercent)
		}
		if time.Duration(batch.MinDurationSeconds)*time.Second < MinBatchDuration {
			return fmt.Errorf("%w: every batch lasts at least %d seconds", ErrInvalid, int(MinBatchDuration/time.Second))
		}
		previous = batch.Percent
	}
	if previous != 100 {
		return fmt.Errorf("%w: the last batch must reach 100%%", ErrInvalid)
	}
	return nil
}

// OrderKey is a node's place in the canary order: the hex SHA-256 of its
// name, so the same nodes are the canaries of every campaign.
func OrderKey(node agentcontrol.AgentNode) string {
	sum := sha256.Sum256([]byte("anixops-agent-upgrade:" + node.String()))
	return hex.EncodeToString(sum[:])
}

// BatchBounds splits total nodes, ordered by OrderKey, into the batches:
// batch i takes the nodes from bounds[i] to bounds[i+1]. A cumulative
// share rounds up, so the canary batch has at least one node.
func BatchBounds(total int, batches []Batch) []int {
	bounds := make([]int, len(batches)+1)
	for index, batch := range batches {
		end := int(math.Ceil(float64(total) * float64(batch.Percent) / 100))
		if end < bounds[index] {
			end = bounds[index]
		}
		if end > total || index == len(batches)-1 {
			end = total
		}
		bounds[index+1] = end
	}
	return bounds
}

// candidate is a node a campaign may include.
type candidate struct {
	node agentcontrol.AgentNode
	tags []string
}

// Candidates lists the nodes a campaign includes: enabled proxy and
// forward nodes whose Agent was seen on the Agent Control stream
// (v4_kernel_agent_transport, mtls-stream or apikey-stream), minus the
// excluded nodes and tags, in canary order.
func Candidates(ctx context.Context, db *gorm.DB, exclude Exclude) ([]agentcontrol.AgentNode, error) {
	var seen []model.AgentTransport
	if err := db.WithContext(ctx).Where("transport IN ?", []string{model.AgentTransportMTLSStream, model.AgentTransportAPIKeyStream}).
		Find(&seen).Error; err != nil {
		return nil, err
	}
	proxyIDs, forwardIDs := map[uint]bool{}, map[uint]bool{}
	for _, row := range seen {
		switch row.NodeKind {
		case agentcontrol.NodeKindProxy:
			proxyIDs[row.NodeID] = true
		case agentcontrol.NodeKindForward:
			forwardIDs[row.NodeID] = true
		}
	}
	var candidates []candidate
	if len(proxyIDs) > 0 {
		var nodes []model.Node
		if err := db.WithContext(ctx).Select("id", "status", "tags").Where("id IN ?", keys(proxyIDs)).Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if node.Status == model.NodeStatusDisabled {
				continue
			}
			tags := ""
			if node.Tags != nil {
				tags = *node.Tags
			}
			candidates = append(candidates, candidate{node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(node.ID)}, tags: parseTags(tags)}) // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
		}
	}
	if len(forwardIDs) > 0 {
		var nodes []model.ForwardNode
		if err := db.WithContext(ctx).Select("id", "enabled", "tags").Where("id IN ?", keys(forwardIDs)).Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if !node.Enabled {
				continue
			}
			candidates = append(candidates, candidate{node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(node.ID)}, tags: parseTags(node.Tags)}) // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
		}
	}
	excludedNodes := map[string]bool{}
	for _, name := range exclude.Nodes {
		excludedNodes[strings.TrimSpace(name)] = true
	}
	excludedTags := map[string]bool{}
	for _, tag := range exclude.Tags {
		excludedTags[strings.TrimSpace(tag)] = true
	}
	nodes := make([]agentcontrol.AgentNode, 0, len(candidates))
next:
	for _, c := range candidates {
		if excludedNodes[c.node.String()] {
			continue
		}
		for _, tag := range c.tags {
			if excludedTags[tag] {
				continue next
			}
		}
		nodes = append(nodes, c.node)
	}
	sort.Slice(nodes, func(i, j int) bool { return OrderKey(nodes[i]) < OrderKey(nodes[j]) })
	return nodes, nil
}

func keys(set map[uint]bool) []uint {
	out := make([]uint, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// parseTags reads a node's tags: a JSON array of strings, else a comma
// separated list.
func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		return tags
	}
	for _, tag := range strings.Split(raw, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func validateExclude(exclude Exclude) error {
	if len(exclude.Nodes) > 1000 || len(exclude.Tags) > 100 {
		return fmt.Errorf("%w: at most 1000 excluded nodes and 100 excluded tags", ErrInvalid)
	}
	for _, name := range exclude.Nodes {
		if _, err := agentcontrol.ParseAgentNode(strings.TrimSpace(name)); err != nil {
			return fmt.Errorf("%w: excluded node %q is not proxy-<id> or forward-<id>", ErrInvalid, name)
		}
	}
	for _, tag := range exclude.Tags {
		if strings.TrimSpace(tag) == "" || len(tag) > 64 {
			return fmt.Errorf("%w: excluded tags are 1 to 64 bytes", ErrInvalid)
		}
	}
	return nil
}

// Start validates and records a campaign with its nodes and their batches.
// Only one campaign is active at a time.
func (s *Service) Start(ctx context.Context, request StartRequest) (model.AgentUpgradeCampaign, error) {
	target := strings.TrimSpace(request.TargetVersion)
	if !semver.IsValid(target) || !strings.HasPrefix(target, "v") {
		return model.AgentUpgradeCampaign{}, fmt.Errorf("%w: target_version %q is not a release tag", ErrInvalid, target)
	}
	if control := "v" + strings.TrimPrefix(strings.TrimSpace(request.ControlVersion), "v"); semver.IsValid(control) && semver.Compare(target, control) > 0 {
		return model.AgentUpgradeCampaign{}, fmt.Errorf("%w: target_version %s is newer than this Control (%s): the Agent release is Control's (H25)", ErrInvalid, target, control)
	}
	if len(request.Artifacts) == 0 {
		return model.AgentUpgradeCampaign{}, fmt.Errorf("%w: the release has no verified artifacts", ErrInvalid)
	}
	probe := agentcontrol.UpgradeRequest{
		Schema: agentcontrol.UpgradeSchemaV1, CampaignID: "probe", Action: agentcontrol.UpgradeActionUpgrade,
		TargetVersion: target, Artifacts: request.Artifacts,
	}
	if err := probe.Validate(); err != nil {
		return model.AgentUpgradeCampaign{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	batches := request.Batches
	if len(batches) == 0 {
		batches = DefaultBatches()
	}
	if err := ValidateBatches(batches); err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	if err := validateExclude(request.Exclude); err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	if len(request.Reason) > 512 {
		return model.AgentUpgradeCampaign{}, fmt.Errorf("%w: reason is at most 512 bytes", ErrInvalid)
	}
	artifactsJSON, err := json.Marshal(request.Artifacts)
	if err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	batchesJSON, err := json.Marshal(batches)
	if err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	excludeJSON, err := json.Marshal(request.Exclude)
	if err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	now := s.now()
	slot := model.AgentUpgradeActiveSlot
	campaign := model.AgentUpgradeCampaign{
		ID: uuid.NewString(), TargetVersion: target, ArtifactsJSON: string(artifactsJSON), BatchesJSON: string(batchesJSON),
		FailureThresholdPercent: FailureThresholdPercent, ReconnectTimeoutSeconds: int(ReconnectTimeout / time.Second),
		ExcludeJSON: string(excludeJSON), Status: model.AgentUpgradeRunning, ActiveSlot: &slot, BatchStartedAt: &now,
		Reason: strings.TrimSpace(request.Reason), CreatedBy: request.Actor.UserID, Actor: truncate(request.Actor.Name, 100),
		CreatedAt: now, UpdatedAt: now,
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&model.AgentUpgradeCampaign{}).Where("active_slot IS NOT NULL").Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrActiveCampaign
		}
		nodes, err := Candidates(ctx, tx, request.Exclude)
		if err != nil {
			return err
		}
		if len(nodes) == 0 {
			return fmt.Errorf("%w: no enabled node's Agent was seen on the Agent Control stream (after exclusions)", ErrNoNodes)
		}
		if err := tx.Create(&campaign).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrActiveCampaign
			}
			return err
		}
		bounds := BatchBounds(len(nodes), batches)
		rows := make([]model.AgentUpgradeNode, 0, len(nodes))
		for batch := range batches {
			for _, node := range nodes[bounds[batch]:bounds[batch+1]] {
				rows = append(rows, model.AgentUpgradeNode{
					CampaignID: campaign.ID, NodeKind: node.Kind, NodeID: uint64(node.ID), Batch: batch, OrderKey: OrderKey(node),
					State: model.AgentUpgradeNodePending, CreatedAt: now, UpdatedAt: now,
				})
			}
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return err
		}
		return writeAudit(tx, request.Actor, AuditActionStart, map[string]any{
			"campaign_id": campaign.ID, "target_version": target, "nodes": len(nodes), "batches": batches,
			"exclude": request.Exclude, "reason": campaign.Reason,
		})
	})
	if err != nil {
		return model.AgentUpgradeCampaign{}, err
	}
	return campaign, nil
}

func isUniqueViolation(err error) bool {
	text := strings.ToLower(err.Error())
	return errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(text, "unique") || strings.Contains(text, "duplicate key")
}

// Pause stops offering the upgrade and starting batches. Nodes already
// offered are still evaluated, and a failing batch still rolls back.
func (s *Service) Pause(ctx context.Context, id string, actor Actor) (model.AgentUpgradeCampaign, error) {
	return s.transition(ctx, id, actor, AuditActionPause, func(campaign *model.AgentUpgradeCampaign, now time.Time) error {
		if campaign.Status != model.AgentUpgradeRunning {
			return fmt.Errorf("%w: only a running campaign can be paused (it is %s)", ErrState, campaign.Status)
		}
		campaign.Status, campaign.PausedAt = model.AgentUpgradePaused, &now
		return nil
	})
}

// Resume continues a paused campaign. The time the current batch was
// paused does not count towards its minimum duration.
func (s *Service) Resume(ctx context.Context, id string, actor Actor) (model.AgentUpgradeCampaign, error) {
	return s.transition(ctx, id, actor, AuditActionResume, func(campaign *model.AgentUpgradeCampaign, now time.Time) error {
		if campaign.Status != model.AgentUpgradePaused {
			return fmt.Errorf("%w: only a paused campaign can be resumed (it is %s)", ErrState, campaign.Status)
		}
		if campaign.PausedAt != nil && campaign.BatchStartedAt != nil {
			started := campaign.BatchStartedAt.Add(now.Sub(*campaign.PausedAt))
			campaign.BatchStartedAt = &started
		}
		campaign.Status, campaign.PausedAt = model.AgentUpgradeRunning, nil
		return nil
	})
}

// Abort stops a campaign. Without rollback it ends at once (aborted):
// nodes already offered finish their upgrade on their own. With rollback
// the current batch is rolled back first, as after a failure.
func (s *Service) Abort(ctx context.Context, id string, actor Actor, rollback bool) (model.AgentUpgradeCampaign, error) {
	return s.transition(ctx, id, actor, AuditActionAbort, func(campaign *model.AgentUpgradeCampaign, now time.Time) error {
		switch campaign.Status {
		case model.AgentUpgradeRunning, model.AgentUpgradePaused:
		case model.AgentUpgradeRollingBack:
			if rollback {
				return nil
			}
		default:
			return fmt.Errorf("%w: the campaign already ended (%s)", ErrState, campaign.Status)
		}
		if rollback {
			startRollback(campaign, now, CodeAborted, "aborted by an administrator with rollback")
			return nil
		}
		finish(campaign, now, model.AgentUpgradeAborted, CodeAborted, "aborted by an administrator")
		return nil
	})
}

func (s *Service) transition(ctx context.Context, id string, actor Actor, action string, change func(*model.AgentUpgradeCampaign, time.Time) error) (model.AgentUpgradeCampaign, error) {
	var campaign model.AgentUpgradeCampaign
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&campaign).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		before := campaign.Status
		now := s.now()
		if err := change(&campaign, now); err != nil {
			return err
		}
		campaign.UpdatedAt = now
		if err := tx.Select("*").Where("id = ? AND status = ?", campaign.ID, before).Updates(&campaign).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, action, map[string]any{"campaign_id": campaign.ID, "from": before, "to": campaign.Status})
	})
	return campaign, err
}

func startRollback(campaign *model.AgentUpgradeCampaign, now time.Time, code, reason string) {
	campaign.Status, campaign.RollbackStartedAt, campaign.PausedAt = model.AgentUpgradeRollingBack, &now, nil
	campaign.ErrorCode, campaign.StatusReason = code, truncate(reason, 1024)
}

func finish(campaign *model.AgentUpgradeCampaign, now time.Time, status, code, reason string) {
	campaign.Status, campaign.ActiveSlot, campaign.FinishedAt, campaign.PausedAt = status, nil, &now, nil
	if code != "" || campaign.ErrorCode == "" {
		campaign.ErrorCode = code
	}
	if reason != "" {
		campaign.StatusReason = truncate(reason, 1024)
	}
}

func writeAudit(tx *gorm.DB, actor Actor, action string, content map[string]any) error {
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	name := actor.Name
	if name == "" && actor.UserID == 0 {
		name = "system"
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(name, 100), Action: action, Module: auditModule, TargetType: "agent_upgrade_campaign",
		Content: string(data), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
