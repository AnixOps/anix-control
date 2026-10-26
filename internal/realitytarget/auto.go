package realitytarget

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Automatic selection (phase 3): for each proxy node that has it enabled,
// the singleton worker probes the node's current targets and the built-in
// candidates every AutoInterval. A Reality protocol whose target failed in
// two runs in a row (AutoFailRuns), the second AutoRecheckDelay after the
// first, moves to the recommended target. A probe that gave no answer
// (offline, timeout) never counts as a failure, and a target that still
// passes is never replaced for a better score: switching changes the SNI
// every client has, so the worker only replaces what is broken.
const (
	// AutoInterval is the time between two runs of a node.
	AutoInterval = 6 * time.Hour
	// AutoRecheckDelay is the time to the run that confirms a failure.
	AutoRecheckDelay = 15 * time.Minute
	// AutoRetryDelay is the time to the next run when the node's Agent
	// could not probe.
	AutoRetryDelay = 30 * time.Minute
	// AutoFailRuns is the runs in a row a target must fail in.
	AutoFailRuns = 2
	// AutoTick is how often the worker looks for nodes that are due.
	AutoTick = 5 * time.Minute
	// AutoAuditAction is the operation log action of a switch.
	AutoAuditAction = "reality_target_auto_switch"
)

// AutoState is a node's automatic selection, as the API answers it.
type AutoState struct {
	Enabled      bool       `json:"enabled"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	LastSwitchAt *time.Time `json:"last_switch_at,omitempty"`
	FailStreak   int        `json:"fail_streak"`
	LastOutcome  string     `json:"last_outcome,omitempty"`
}

// AutoState answers node nodeID's automatic selection; a node without a row
// has it off.
func (s *Service) AutoState(ctx context.Context, nodeID uint) (AutoState, error) {
	var row model.RealityTargetAuto
	err := s.DB.WithContext(ctx).Where("node_id = ?", nodeID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AutoState{}, nil
	}
	if err != nil {
		return AutoState{}, err
	}
	return autoState(row), nil
}

func autoState(row model.RealityTargetAuto) AutoState {
	return AutoState{
		Enabled: row.Enabled, NextRunAt: utc(row.NextRunAt), LastRunAt: utc(row.LastRunAt),
		LastSwitchAt: utc(row.LastSwitchAt), FailStreak: row.FailStreak, LastOutcome: row.LastOutcome,
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// SetAuto turns node nodeID's automatic selection on or off. Turning it on
// makes the node due at once and starts its failure count afresh.
func (s *Service) SetAuto(ctx context.Context, nodeID uint, enabled bool) (AutoState, error) {
	row := model.RealityTargetAuto{NodeID: nodeID, Enabled: enabled}
	updates := []string{"enabled", "updated_at"}
	if enabled {
		row.NextRunAt, row.FailStreak = nil, 0
		updates = append(updates, "next_run_at", "fail_streak")
	}
	err := s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns(updates),
	}).Create(&row).Error
	if err != nil {
		return AutoState{}, err
	}
	return s.AutoState(ctx, nodeID)
}

// AutoWorker runs the automatic selection in the singleton worker.
type AutoWorker struct {
	Service *Service
	// Ready reports whether the node's Agent can probe from this process
	// (connected here, with reality.probe).
	Ready func(node agentcontrol.AgentNode) bool
	// Update writes a protocol (service.NodeService.UpdateProtocol).
	Update UpdateProtocol
	// Tick defaults to AutoTick.
	Tick time.Duration
}

// Run runs the due nodes every Tick until ctx ends.
func (w *AutoWorker) Run(ctx context.Context) {
	tick := w.Tick
	if tick <= 0 {
		tick = AutoTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		if err := w.RunDue(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("reality target auto selection failed", "component", "reality-target", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunDue runs each enabled node whose next run is due, one at a time.
func (w *AutoWorker) RunDue(ctx context.Context) error {
	now := w.Service.now().UTC()
	var rows []model.RealityTargetAuto
	err := w.Service.DB.WithContext(ctx).
		Where("enabled = ? AND (next_run_at IS NULL OR next_run_at <= ?)", true, now).
		Order("node_id").Find(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.RunNode(ctx, row.NodeID); err != nil {
			slog.Warn("reality target auto selection of a node failed", "component", "reality-target", "node_id", row.NodeID, "error", err)
		}
	}
	return nil
}

// RunNode runs node nodeID once: it probes, and switches the protocols whose
// target failed AutoFailRuns times in a row.
func (w *AutoWorker) RunNode(ctx context.Context, nodeID uint) error {
	s := w.Service
	var row model.RealityTargetAuto
	if err := s.DB.WithContext(ctx).Where("node_id = ?", nodeID).First(&row).Error; err != nil {
		return err
	}
	if !row.Enabled {
		return nil
	}
	now := s.now().UTC()
	row.LastRunAt = &now
	next := now.Add(AutoInterval)

	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)} // #nosec G115 -- node ids are uint32 on the wire.
	current, err := s.current(nodeID)
	switch {
	case err != nil:
		return err
	case len(current) == 0:
		row.FailStreak, row.LastOutcome = 0, "no Reality protocol"
		return w.save(ctx, row, next)
	case w.Ready != nil && !w.Ready(node):
		row.LastOutcome = "the Agent cannot probe now (offline, on another Control, or without reality.probe)"
		return w.save(ctx, row, now.Add(AutoRetryDelay))
	}

	targets := make([]string, 0, len(current))
	for _, target := range current {
		if candidate, ok := currentCandidate(target); ok {
			targets = append(targets, candidate.String())
		}
	}
	candidates, err := Candidates(targets, true)
	if err != nil {
		return err
	}
	report, err := s.Run(ctx, nodeID, candidates, DefaultTimeout)
	if err != nil {
		return err
	}

	var broken []CurrentTarget
	for _, target := range current {
		candidate, ok := currentCandidate(target)
		if !ok {
			continue
		}
		if verdict := targetVerdict(report, candidate); verdict == VerdictFailed {
			broken = append(broken, target)
		}
	}
	if len(broken) == 0 {
		row.FailStreak, row.LastOutcome = 0, "every current target passed"
		return w.save(ctx, row, next)
	}
	row.FailStreak++
	names := make([]string, 0, len(broken))
	for _, target := range broken {
		names = append(names, target.ServerName)
	}
	if row.FailStreak < AutoFailRuns {
		row.LastOutcome = fmt.Sprintf("current target failed (%s); rechecking before a switch", strings.Join(names, ", "))
		return w.save(ctx, row, now.Add(AutoRecheckDelay))
	}
	if report.Recommended == nil {
		row.LastOutcome = fmt.Sprintf("current target failed (%s) and no candidate passed", strings.Join(names, ", "))
		return w.save(ctx, row, next)
	}
	best := Candidate{ServerName: report.Recommended.ServerName, Port: report.Recommended.Port}
	var switched []string
	var failures []string
	for _, target := range broken {
		result, err := s.Apply(ctx, nodeID, ApplyRequest{ProtocolID: target.ProtocolID, Target: best}, w.Update)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", target.Name, err))
			continue
		}
		switched = append(switched, fmt.Sprintf("%s %s → %s", target.Name, result.Previous.ServerName, best.ServerName))
		s.auditSwitch(ctx, nodeID, result, report.Recommended.Score)
	}
	if len(switched) > 0 {
		row.LastSwitchAt = &now
		row.FailStreak = 0
	}
	row.LastOutcome = "switched " + strings.Join(switched, "; ")
	if len(switched) == 0 {
		row.LastOutcome = "switch failed"
	}
	if len(failures) > 0 {
		row.LastOutcome += "; failed: " + strings.Join(failures, "; ")
	}
	return w.save(ctx, row, next)
}

// save stores the run's state with its next run.
func (w *AutoWorker) save(ctx context.Context, row model.RealityTargetAuto, next time.Time) error {
	row.NextRunAt = &next
	if len(row.LastOutcome) > 500 {
		row.LastOutcome = row.LastOutcome[:500]
	}
	return w.Service.DB.WithContext(ctx).Model(&model.RealityTargetAuto{}).Where("node_id = ?", row.NodeID).Updates(map[string]any{
		"next_run_at": row.NextRunAt, "last_run_at": row.LastRunAt, "last_switch_at": row.LastSwitchAt,
		"fail_streak": row.FailStreak, "last_outcome": row.LastOutcome, "updated_at": w.Service.now().UTC(),
	}).Error
}

// auditSwitch writes the operation log row of a switch (module
// reality_target, no user: Control did it).
func (s *Service) auditSwitch(ctx context.Context, nodeID uint, result *ApplyResult, score int) {
	content := fmt.Sprintf(`{"node_id":%d,"protocol_id":%d,"from":%q,"to":%q,"score":%d,"automatic":true}`,
		nodeID, result.Applied.ProtocolID, result.Previous.Dest, result.Applied.Dest, score)
	targetID := nodeID
	row := model.OperationLog{
		Username: "control", Action: AutoAuditAction, Module: "reality_target",
		TargetType: "proxy_node", TargetID: &targetID, Content: content, Status: 1,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		slog.Warn("reality target auto switch audit row not written", "component", "reality-target", "error", err)
	}
}

// currentCandidate is the candidate a protocol's target probes as: its
// dest when that is host:port, else its server name on 443.
func currentCandidate(target CurrentTarget) (Candidate, bool) {
	if target.Dest != "" {
		if candidate, err := ParseCandidate(target.Dest); err == nil {
			return candidate, true
		}
	}
	if target.ServerName != "" {
		if candidate, err := ParseCandidate(target.ServerName); err == nil {
			return candidate, true
		}
	}
	return Candidate{}, false
}

// targetVerdict is candidate's verdict in report, VerdictError when this
// run has none for it.
func targetVerdict(report *Report, candidate Candidate) string {
	for _, target := range report.Targets {
		if target.ServerName == candidate.ServerName && target.Port == candidate.Port {
			if target.Error != "" {
				return VerdictError
			}
			return target.Verdict
		}
	}
	return VerdictError
}
