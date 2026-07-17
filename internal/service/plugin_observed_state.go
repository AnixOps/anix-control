package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxNodePluginObservedStates       = 32
	maxNodePluginObservedRuleCounters = 1024
	maxNodePluginObservedRuleIDLength = 96
	maxNodePluginObservedStateAge     = 5 * time.Minute
	maxNodePluginObservedFutureSkew   = time.Minute
	// NodePluginObservedState stores revisions in a signed 64-bit database
	// column, so wire revisions must stay within that representation.
	maxNodePluginObservedRevision = uint64(1<<63 - 1)
)

// NodePluginRuleCounter is the kernel-neutral form of one plugin-owned
// counter. The kernel stores it as opaque, bounded data and does not infer
// quota or billing semantics from it.
type NodePluginRuleCounter struct {
	RuleID  string `json:"rule_id"`
	Packets uint64 `json:"packets"`
	Bytes   uint64 `json:"bytes"`
}

// NodePluginObservedSnapshot is the bounded data the authenticated Agent may
// contribute on its heartbeat. It deliberately has no raw plugin payload or
// error text: those would make the generic observed-state API a secret leak.
type NodePluginObservedSnapshot struct {
	PluginID         string
	Version          string
	DesiredRevision  uint64
	ObservedRevision uint64
	ConfigHash       string
	Health           string
	RulesetSHA256    string
	ObservedAt       time.Time
	RuleCounters     []NodePluginRuleCounter
}

// RecordNodePluginObservedStates stores trusted, monotonic plugin runtime
// snapshots from an authenticated node. A malformed or unauthorized snapshot
// is rejected independently so one bad plugin cannot suppress another
// plugin's heartbeat evidence.
func (s *NodeService) RecordNodePluginObservedStates(nodeID uint, snapshots []NodePluginObservedSnapshot, receivedAt time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("database is not initialized")
	}
	if nodeID == 0 {
		return 0, errors.New("plugin observation node_id is required")
	}
	if len(snapshots) == 0 {
		return 0, nil
	}
	if len(snapshots) > maxNodePluginObservedStates {
		return 0, fmt.Errorf("plugin observations exceed %d entries", maxNodePluginObservedStates)
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}

	accepted := 0
	seen := make(map[string]struct{}, len(snapshots))
	var result error
	for _, snapshot := range snapshots {
		normalized, err := normalizeNodePluginObservedSnapshot(snapshot, receivedAt)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if _, duplicate := seen[normalized.PluginID]; duplicate {
			result = errors.Join(result, fmt.Errorf("plugin observation %q is duplicated", normalized.PluginID))
			continue
		}
		seen[normalized.PluginID] = struct{}{}
		if err := s.authorizeNodePluginObservedState(nodeID, normalized.PluginID, normalized.Version); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := s.persistNodePluginObservedState(nodeID, normalized, receivedAt); err != nil {
			result = errors.Join(result, err)
			continue
		}
		accepted++
	}
	return accepted, result
}

func normalizeNodePluginObservedSnapshot(snapshot NodePluginObservedSnapshot, receivedAt time.Time) (NodePluginObservedSnapshot, error) {
	snapshot.PluginID = strings.TrimSpace(snapshot.PluginID)
	snapshot.Version = strings.TrimSpace(snapshot.Version)
	snapshot.ConfigHash = strings.ToLower(strings.TrimSpace(snapshot.ConfigHash))
	snapshot.RulesetSHA256 = strings.ToLower(strings.TrimSpace(snapshot.RulesetSHA256))
	snapshot.Health = strings.ToLower(strings.TrimSpace(snapshot.Health))
	if !safePluginSegment(snapshot.PluginID) || !safePluginSegment(snapshot.Version) {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation id or version is invalid")
	}
	if snapshot.DesiredRevision == 0 || snapshot.ObservedRevision == 0 || snapshot.ObservedRevision > snapshot.DesiredRevision ||
		snapshot.DesiredRevision > maxNodePluginObservedRevision {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation revision is invalid")
	}
	if !validSHA256Hex(snapshot.ConfigHash) {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation config_hash is invalid")
	}
	if snapshot.Health != "healthy" && snapshot.Health != "unhealthy" && snapshot.Health != "disabled" {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation health is invalid")
	}
	if snapshot.Health == "healthy" && !validSHA256Hex(snapshot.RulesetSHA256) {
		return NodePluginObservedSnapshot{}, errors.New("healthy plugin observation ruleset_sha256 is invalid")
	}
	if snapshot.Health != "healthy" && snapshot.RulesetSHA256 != "" && !validSHA256Hex(snapshot.RulesetSHA256) {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation ruleset_sha256 is invalid")
	}
	if snapshot.ObservedAt.IsZero() || snapshot.ObservedAt.After(receivedAt.Add(maxNodePluginObservedFutureSkew)) || receivedAt.Sub(snapshot.ObservedAt) > maxNodePluginObservedStateAge {
		return NodePluginObservedSnapshot{}, errors.New("plugin observation timestamp is outside the accepted window")
	}
	if len(snapshot.RuleCounters) > maxNodePluginObservedRuleCounters {
		return NodePluginObservedSnapshot{}, fmt.Errorf("plugin observation counters exceed %d entries", maxNodePluginObservedRuleCounters)
	}
	if snapshot.Health != "healthy" && len(snapshot.RuleCounters) != 0 {
		return NodePluginObservedSnapshot{}, errors.New("non-healthy plugin observation must not include counters")
	}
	counters := make([]NodePluginRuleCounter, 0, len(snapshot.RuleCounters))
	seenCounters := make(map[string]struct{}, len(snapshot.RuleCounters))
	for _, counter := range snapshot.RuleCounters {
		counter.RuleID = strings.TrimSpace(counter.RuleID)
		if len(counter.RuleID) == 0 || len(counter.RuleID) > maxNodePluginObservedRuleIDLength || !safePluginSegment(counter.RuleID) {
			return NodePluginObservedSnapshot{}, errors.New("plugin observation counter rule_id is invalid")
		}
		if _, duplicate := seenCounters[counter.RuleID]; duplicate {
			return NodePluginObservedSnapshot{}, fmt.Errorf("plugin observation counter %q is duplicated", counter.RuleID)
		}
		seenCounters[counter.RuleID] = struct{}{}
		counters = append(counters, counter)
	}
	sort.Slice(counters, func(left, right int) bool { return counters[left].RuleID < counters[right].RuleID })
	snapshot.RuleCounters = counters
	return snapshot, nil
}

// authorizeNodePluginObservedState requires the exact enabled assignment and
// a currently verifiable official Agent release. Merely knowing an official
// plugin ID is not enough to inject runtime health into the kernel.
func (s *NodeService) authorizeNodePluginObservedState(nodeID uint, pluginID, version string) error {
	var assignmentCount int64
	if err := s.db.Model(&model.NodeServiceAssignment{}).
		Where("node_id = ? AND plugin_id = ? AND desired_version = ? AND enabled = ? AND delete_pending = ?", nodeID, pluginID, version, true, false).
		Count(&assignmentCount).Error; err != nil {
		return fmt.Errorf("authorize plugin observation assignment: %w", err)
	}
	if assignmentCount == 0 {
		return fmt.Errorf("plugin observation %s@%s is not assigned to node", pluginID, version)
	}
	var plugin model.Plugin
	if err := s.db.First(&plugin, "id = ? AND official = ? AND publisher = ?", pluginID, true, "AnixOps").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("plugin observation %s is not an official plugin", pluginID)
		}
		return fmt.Errorf("load plugin observation catalog entry: %w", err)
	}
	var release model.PluginRelease
	if err := s.db.First(&release, "plugin_id = ? AND version = ?", pluginID, version).Error; err != nil {
		return fmt.Errorf("load plugin observation release: %w", err)
	}
	manifest, err := VerifyStoredPluginRelease(s.db, release, nil)
	if err != nil {
		return fmt.Errorf("verify plugin observation release %s@%s: %w", pluginID, version, err)
	}
	if !manifestSupportsTarget(*manifest, "agent") || !containsPluginCapability(manifest.Capabilities, "kernel.observed-state") {
		return fmt.Errorf("plugin observation release %s@%s is not authorized for kernel observed state", pluginID, version)
	}
	return nil
}

func (s *NodeService) persistNodePluginObservedState(nodeID uint, snapshot NodePluginObservedSnapshot, receivedAt time.Time) error {
	desiredRevision, desiredOK := nodePluginObservedRevision(snapshot.DesiredRevision)
	observedRevision, observedOK := nodePluginObservedRevision(snapshot.ObservedRevision)
	if !desiredOK || !observedOK {
		return errors.New("plugin observation revision is invalid")
	}
	counters, err := json.Marshal(snapshot.RuleCounters)
	if err != nil {
		return fmt.Errorf("encode plugin observation counters: %w", err)
	}
	state := model.NodePluginObservedState{
		NodeID: nodeID, PluginID: snapshot.PluginID, Version: snapshot.Version,
		DesiredRevision: desiredRevision, ObservedRevision: observedRevision,
		ConfigHash: snapshot.ConfigHash, Health: snapshot.Health, RulesetSHA256: snapshot.RulesetSHA256,
		CountersJSON: string(counters), ObservedAt: snapshot.ObservedAt, ReceivedAt: receivedAt, UpdatedAt: receivedAt,
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_id"}, {Name: "plugin_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"version", "desired_revision", "observed_revision", "config_hash", "health", "ruleset_sha256",
				"counters_json", "observed_at", "received_at", "updated_at",
			}),
			Where: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "excluded.observed_at >= observed_at"}}},
		}).Create(&state)
		if result.Error != nil {
			return fmt.Errorf("persist plugin observation %s: %w", snapshot.PluginID, result.Error)
		}
		return nil
	})
}

func nodePluginObservedRevision(revision uint64) (int64, bool) {
	if revision > maxNodePluginObservedRevision {
		return 0, false
	}
	return int64(revision), true // #nosec G115 -- the Agent revision is bounded by the signed database column range above.
}
