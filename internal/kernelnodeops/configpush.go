package kernelnodeops

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Configuration push on the Agent Control stream (config.v1, A2-3;
// node-ops-service.md section 5.5). The kernel sends a node's agent the
// stored desired configuration as a ConfigSnapshot, and records the
// agent's ConfigStatus in v4_kernel_node_config_status. The listener
// (internal/grpc) decides when to send; these functions are the kernel's
// side of it, shared by the listener and node.sync.

// ConfigSnapshotOf returns the snapshot of a stored desired configuration:
// its revision, hash, format and document, as stored.
func ConfigSnapshotOf(row model.KernelNodeDesiredConfig) *agentv1pb.ConfigSnapshot {
	return &agentv1pb.ConfigSnapshot{
		ConfigRevision: row.Revision, ConfigHash: row.ConfigHash, Format: row.Format, ConfigJson: []byte(row.ConfigJSON),
	}
}

// RefreshDesiredConfig rebuilds node's desired configuration from its rows
// and stores it when it changed or none was stored, through
// BuildDesiredConfig and StoreDesiredConfig, the functions SyncNode uses.
// An unchanged configuration is not written. changed reports whether the
// stored revision grew (or the row was created). ErrNodeGone when the node
// does not exist.
func RefreshDesiredConfig(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, now time.Time) (model.KernelNodeDesiredConfig, bool, error) {
	cfg, err := BuildDesiredConfig(db, node)
	if err != nil {
		return model.KernelNodeDesiredConfig{}, false, err
	}
	stored, found, err := LoadDesiredConfig(ctx, db, node)
	if err != nil {
		return model.KernelNodeDesiredConfig{}, false, err
	}
	if found && stored.ConfigHash == cfg.Hash {
		return stored, false, nil
	}
	return StoreDesiredConfig(ctx, db, cfg, now)
}

// maxConfigStatusError bounds the agent's error text that is stored.
const maxConfigStatusError = 1024

// ConfigVerdict judges an agent's ConfigStatus against the node's desired
// configuration (model.ConfigVerdict*): applied or failed when it names
// the desired revision and hash, stale when it names an older revision,
// mismatch otherwise (the desired revision with another hash, a revision
// the kernel never stored, or no desired configuration).
func ConfigVerdict(desired model.KernelNodeDesiredConfig, found bool, status *agentv1pb.ConfigStatus) string {
	switch {
	case !found:
		return model.ConfigVerdictMismatch
	case status.GetConfigRevision() < desired.Revision:
		return model.ConfigVerdictStale
	case status.GetConfigRevision() != desired.Revision || status.GetConfigHash() != desired.ConfigHash:
		return model.ConfigVerdictMismatch
	case status.GetApplied():
		return model.ConfigVerdictApplied
	}
	return model.ConfigVerdictFailed
}

// configErrorCodePattern is the form of an error code: lowercase words
// joined by "_", at most 64 bytes (agentcontrol, "Error codes").
var configErrorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ConfigStatusErrorCode is the error_code of a status that was not
// applied, as the kernel keeps it: empty for an applied status and for a
// code that is not of the error code form. Codes the contract does not
// define (agentcontrol.ConfigErrorCodes) are kept, for a newer Agent.
func ConfigStatusErrorCode(status *agentv1pb.ConfigStatus) string {
	code := status.GetErrorCode()
	if status.GetApplied() || !configErrorCodePattern.MatchString(code) {
		return ""
	}
	return code
}

// RecordConfigStatus records a ConfigStatus the node's agent sent on
// session sessionID and returns the verdict on it (ConfigVerdict). Every
// status is recorded as the node's last report; only an applied one that
// names the desired revision and hash moves the node's applied revision.
func RecordConfigStatus(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, sessionID string, status *agentv1pb.ConfigStatus, now time.Time) (string, error) {
	if status == nil {
		return "", errors.New("config status is required")
	}
	desired, found, err := LoadDesiredConfig(ctx, db, node)
	if err != nil {
		return "", err
	}
	verdict := ConfigVerdict(desired, found, status)
	now = now.UTC()
	reportedError := truncateUTF8(status.GetError(), maxConfigStatusError)
	reportedErrorCode := ConfigStatusErrorCode(status)
	reportedHash := truncateUTF8(status.GetConfigHash(), 64)
	row := model.KernelNodeConfigStatus{
		NodeKind: node.Kind, NodeID: uint64(node.ID), SessionID: truncateUTF8(sessionID, 64),
		ReportedRevision: status.GetConfigRevision(), ReportedHash: reportedHash, ReportedApplied: status.GetApplied(),
		ReportedError: reportedError, ReportedErrorCode: reportedErrorCode, Verdict: verdict, ReportedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	updates := map[string]any{
		"session_id": row.SessionID, "reported_revision": row.ReportedRevision, "reported_hash": row.ReportedHash,
		"reported_applied": row.ReportedApplied, "reported_error": row.ReportedError, "reported_error_code": reportedErrorCode, "verdict": verdict,
		"reported_at": now, "updated_at": now,
	}
	if verdict == model.ConfigVerdictApplied {
		row.AppliedRevision, row.AppliedHash, row.AppliedAt = status.GetConfigRevision(), reportedHash, &now
		updates["applied_revision"], updates["applied_hash"], updates["applied_at"] = row.AppliedRevision, row.AppliedHash, now
	}
	err = db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_kind"}, {Name: "node_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error
	if err != nil {
		return "", err
	}
	return verdict, nil
}

// LoadConfigStatus returns what the node's agent last reported about its
// configuration.
func LoadConfigStatus(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode) (model.KernelNodeConfigStatus, bool, error) {
	var rows []model.KernelNodeConfigStatus
	if err := db.WithContext(ctx).Where("node_kind = ? AND node_id = ?", node.Kind, node.ID).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelNodeConfigStatus{}, false, err
	}
	if len(rows) == 0 {
		return model.KernelNodeConfigStatus{}, false, nil
	}
	return rows[0], true, nil
}

// ConfigLaggingNodes counts the nodes whose agent reported a configuration
// status and whose applied revision is behind the desired one.
func ConfigLaggingNodes(ctx context.Context, db *gorm.DB) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Table(model.KernelNodeConfigStatus{}.TableName() + " AS s").
		Joins("JOIN " + model.KernelNodeDesiredConfig{}.TableName() + " AS d ON d.node_kind = s.node_kind AND d.node_id = s.node_id").
		Where("s.applied_revision < d.revision").Count(&count).Error
	return count, err
}

// ConfigApplied reports whether the node's agent applied row, as the kernel
// verified it (an applied ConfigStatus naming its revision and hash).
func ConfigApplied(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, row model.KernelNodeDesiredConfig) (bool, error) {
	return appliedDesired(ctx, db, node, row)
}

// appliedDesired reports whether the node's agent applied row, as the
// kernel verified it.
func appliedDesired(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, row model.KernelNodeDesiredConfig) (bool, error) {
	status, found, err := LoadConfigStatus(ctx, db, node)
	if err != nil || !found {
		return false, err
	}
	return status.AppliedRevision == row.Revision && status.AppliedHash == row.ConfigHash, nil
}

func truncateUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := text[:limit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return strings.ToValidUTF8(cut, "")
}

// configFollower fans the ConfigStatus reports of one ConfigStreams out to
// the node syncs waiting for their snapshot's answer. One follower
// subscribes per ConfigStreams, once.
type configFollower struct {
	mu      sync.Mutex
	waiters map[agentcontrol.AgentNode][]chan agentstreams.ConfigStatusReport
}

var (
	configFollowersMu sync.Mutex
	configFollowers   = map[agentstreams.ConfigStreams]*configFollower{}
)

func configFollowerFor(streams agentstreams.ConfigStreams) *configFollower {
	configFollowersMu.Lock()
	defer configFollowersMu.Unlock()
	if f, ok := configFollowers[streams]; ok {
		return f
	}
	f := &configFollower{waiters: map[agentcontrol.AgentNode][]chan agentstreams.ConfigStatusReport{}}
	configFollowers[streams] = f
	streams.OnConfigStatus(f.deliver)
	return f
}

// await returns a channel that receives every ConfigStatus of the node,
// from any of its sessions, until stop is called. Registering before the
// push means no answer is missed.
func (f *configFollower) await(node agentcontrol.AgentNode) (<-chan agentstreams.ConfigStatusReport, func()) {
	ch := make(chan agentstreams.ConfigStatusReport, 16)
	f.mu.Lock()
	f.waiters[node] = append(f.waiters[node], ch)
	f.mu.Unlock()
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		waiters := f.waiters[node]
		for i, waiter := range waiters {
			if waiter == ch {
				f.waiters[node] = append(waiters[:i:i], waiters[i+1:]...)
				break
			}
		}
		if len(f.waiters[node]) == 0 {
			delete(f.waiters, node)
		}
	}
}

func (f *configFollower) deliver(report agentstreams.ConfigStatusReport) {
	if report.Status == nil {
		return
	}
	f.mu.Lock()
	waiters := append([]chan agentstreams.ConfigStatusReport(nil), f.waiters[report.Node]...)
	f.mu.Unlock()
	for _, waiter := range waiters {
		select {
		case waiter <- report:
		default:
			// A waiter that is 16 reports behind has stopped reading.
		}
	}
}

// configPush is one snapshot pushed by a node sync, and the ConfigStatus
// reports that follow it.
type configPush struct {
	Snapshot  *agentv1pb.ConfigSnapshot
	SessionID string
	Err       error
	reports   <-chan agentstreams.ConfigStatusReport
	stop      func()
}

// pushConfig sends snapshot on the node's session, following the node's
// ConfigStatus reports from before the send.
func pushConfig(ctx context.Context, streams agentstreams.ConfigStreams, node agentcontrol.AgentNode, snapshot *agentv1pb.ConfigSnapshot) *configPush {
	p := &configPush{Snapshot: snapshot}
	p.reports, p.stop = configFollowerFor(streams).await(node)
	sessionID, _, err := streams.PushConfig(ctx, node, snapshot)
	p.SessionID, p.Err = sessionID, err
	if err != nil {
		p.stop()
	}
	return p
}

func (p *configPush) release() {
	if p.stop != nil {
		p.stop()
	}
}

// awaitStatus waits until ctx ends for the agent's answer to the snapshot:
// a ConfigStatus naming its revision and hash, from any session of the
// node (an agent that reconnects is sent the snapshot again and answers on
// its new session). A status for an older revision, or for the snapshot's
// revision with another hash, is not an answer. A status the kernel
// verified as applied at a newer revision is: the node runs a newer
// desired configuration, which supersedes this one.
func (p *configPush) awaitStatus(ctx context.Context) (agentstreams.ConfigStatusReport, error) {
	defer p.release()
	for {
		select {
		case report := <-p.reports:
			if answersSnapshot(p.Snapshot, report) {
				return report, nil
			}
		case <-ctx.Done():
			return agentstreams.ConfigStatusReport{}, ctx.Err()
		}
	}
}

// answersSnapshot tells whether report answers snapshot (awaitStatus).
func answersSnapshot(snapshot *agentv1pb.ConfigSnapshot, report agentstreams.ConfigStatusReport) bool {
	status := report.Status
	switch {
	case status.GetConfigRevision() == snapshot.GetConfigRevision():
		return status.GetConfigHash() == snapshot.GetConfigHash()
	case status.GetConfigRevision() > snapshot.GetConfigRevision():
		return report.Verdict == model.ConfigVerdictApplied
	}
	return false
}
