package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	forwardingPluginID       = "nftables-forward"
	forwardingCanaryDuration = 72 * time.Hour
	maxForwardingNodeCount   = 1_000_000
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type ForwardingAcceptance struct {
	Release struct {
		ControlCommit string `yaml:"control_commit"`
		AgentCommit   string `yaml:"agent_commit"`
		PluginID      string `yaml:"plugin_id"`
		PluginVersion string `yaml:"plugin_version"`
		PackageSHA256 string `yaml:"package_sha256"`
	} `yaml:"release"`
	Staging struct {
		Environment               string `yaml:"environment"`
		RestoreRehearsedAt        string `yaml:"restore_rehearsed_at"`
		VerifiedAt                string `yaml:"verified_at"`
		IPv4TCPPassed             bool   `yaml:"ipv4_tcp_passed"`
		IPv4UDPPassed             bool   `yaml:"ipv4_udp_passed"`
		IPv6TCPPassed             bool   `yaml:"ipv6_tcp_passed"`
		IPv6UDPPassed             bool   `yaml:"ipv6_udp_passed"`
		PartialNodeRollbackPassed bool   `yaml:"partial_node_rollback_passed"`
		ControlRestartPassed      bool   `yaml:"control_restart_passed"`
		AgentRestartPassed        bool   `yaml:"agent_restart_passed"`
		EvidenceReference         string `yaml:"evidence_reference"`
	} `yaml:"staging"`
	LegacyFallback struct {
		RehearsedAt        string `yaml:"rehearsed_at"`
		RecoveryVerifiedAt string `yaml:"recovery_verified_at"`
		EvidenceReference  string `yaml:"evidence_reference"`
	} `yaml:"legacy_fallback"`
	Canary struct {
		NodeIDs           []uint `yaml:"node_ids"`
		StartedAt         string `yaml:"started_at"`
		CompletedAt       string `yaml:"completed_at"`
		TrafficHealthy    bool   `yaml:"traffic_healthy"`
		RollbackPassed    bool   `yaml:"rollback_passed"`
		EvidenceReference string `yaml:"evidence_reference"`
	} `yaml:"canary"`
	Rollout struct {
		ManagedNodeCount uint                    `yaml:"managed_node_count"`
		Steps            []ForwardingRolloutStep `yaml:"steps"`
	} `yaml:"rollout"`
	Operator struct {
		ApprovedBy      string `yaml:"approved_by"`
		ApprovedAt      string `yaml:"approved_at"`
		ChangeReference string `yaml:"change_reference"`
	} `yaml:"operator"`
}

type ForwardingRolloutStep struct {
	Percentage        uint   `yaml:"percentage"`
	NodeIDs           []uint `yaml:"node_ids"`
	StartedAt         string `yaml:"started_at"`
	CompletedAt       string `yaml:"completed_at"`
	EvidenceReference string `yaml:"evidence_reference"`
}

func ValidateForwardingAcceptanceFile(path string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- operator explicitly supplies the local evidence path.
	if err != nil {
		return fmt.Errorf("read forwarding acceptance file: %w", err)
	}

	var evidence ForwardingAcceptance
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&evidence); err != nil {
		return fmt.Errorf("parse forwarding acceptance file: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("parse forwarding acceptance file: exactly one YAML document is required")
		}
		return fmt.Errorf("parse forwarding acceptance file: %w", err)
	}
	return evidence.Validate()
}

func (e ForwardingAcceptance) Validate() error {
	var problems []string
	require := func(ok bool, message string) {
		if !ok {
			problems = append(problems, message)
		}
	}
	requireText := func(value, field string) {
		require(nonPlaceholderEvidence(value), field+" is required and must not be a placeholder")
	}
	requireTime := func(value, field string) time.Time {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
		require(err == nil, field+" must be RFC3339")
		return parsed
	}

	require(hexLength(e.Release.ControlCommit, 40), "release.control_commit must be a 40-character lowercase Git commit")
	require(hexLength(e.Release.AgentCommit, 40), "release.agent_commit must be a 40-character lowercase Git commit")
	require(strings.TrimSpace(e.Release.PluginID) == forwardingPluginID, "release.plugin_id must be nftables-forward")
	require(semanticVersionPattern.MatchString(strings.TrimSpace(e.Release.PluginVersion)), "release.plugin_version must be a semantic version")
	require(hexLength(e.Release.PackageSHA256, 64), "release.package_sha256 must be 64 lowercase hex characters")

	require(strings.TrimSpace(e.Staging.Environment) == "staging", "staging.environment must be staging")
	restoreAt := requireTime(e.Staging.RestoreRehearsedAt, "staging.restore_rehearsed_at")
	stagingVerifiedAt := requireTime(e.Staging.VerifiedAt, "staging.verified_at")
	require(restoreAt.IsZero() || stagingVerifiedAt.IsZero() || !stagingVerifiedAt.Before(restoreAt), "staging verification must not precede restore rehearsal")
	require(e.Staging.IPv4TCPPassed, "staging.ipv4_tcp_passed must be true")
	require(e.Staging.IPv4UDPPassed, "staging.ipv4_udp_passed must be true")
	require(e.Staging.IPv6TCPPassed, "staging.ipv6_tcp_passed must be true")
	require(e.Staging.IPv6UDPPassed, "staging.ipv6_udp_passed must be true")
	require(e.Staging.PartialNodeRollbackPassed, "staging.partial_node_rollback_passed must be true")
	require(e.Staging.ControlRestartPassed, "staging.control_restart_passed must be true")
	require(e.Staging.AgentRestartPassed, "staging.agent_restart_passed must be true")
	requireText(e.Staging.EvidenceReference, "staging.evidence_reference")

	fallbackAt := requireTime(e.LegacyFallback.RehearsedAt, "legacy_fallback.rehearsed_at")
	fallbackRecoveredAt := requireTime(e.LegacyFallback.RecoveryVerifiedAt, "legacy_fallback.recovery_verified_at")
	require(stagingVerifiedAt.IsZero() || fallbackAt.IsZero() || !fallbackAt.Before(stagingVerifiedAt), "legacy fallback rehearsal must follow staging verification")
	require(fallbackAt.IsZero() || fallbackRecoveredAt.IsZero() || fallbackRecoveredAt.After(fallbackAt), "legacy fallback recovery must follow its rehearsal")
	requireText(e.LegacyFallback.EvidenceReference, "legacy_fallback.evidence_reference")

	requireUniqueNodeIDs(e.Canary.NodeIDs, "canary.node_ids", &problems)
	canaryStartedAt := requireTime(e.Canary.StartedAt, "canary.started_at")
	canaryCompletedAt := requireTime(e.Canary.CompletedAt, "canary.completed_at")
	require(fallbackRecoveredAt.IsZero() || canaryStartedAt.IsZero() || !canaryStartedAt.Before(fallbackRecoveredAt), "canary must start after legacy fallback recovery")
	require(canaryStartedAt.IsZero() || canaryCompletedAt.IsZero() || !canaryCompletedAt.Before(canaryStartedAt.Add(forwardingCanaryDuration)), "canary must run for at least 72 hours")
	require(e.Canary.TrafficHealthy, "canary.traffic_healthy must be true")
	require(e.Canary.RollbackPassed, "canary.rollback_passed must be true")
	requireText(e.Canary.EvidenceReference, "canary.evidence_reference")

	require(e.Rollout.ManagedNodeCount > 0, "rollout.managed_node_count must be greater than zero")
	require(e.Rollout.ManagedNodeCount <= maxForwardingNodeCount, "rollout.managed_node_count exceeds the supported maximum")
	expectedPercentages := [...]uint{1, 5, 25, 100}
	require(len(e.Rollout.Steps) == len(expectedPercentages), "rollout.steps must contain exactly the 1, 5, 25, and 100 percent stages")
	var previousNodes map[uint]struct{}
	previousCompletedAt := canaryCompletedAt
	for index, step := range e.Rollout.Steps {
		field := fmt.Sprintf("rollout.steps[%d]", index)
		if index < len(expectedPercentages) {
			require(step.Percentage == expectedPercentages[index], field+".percentage is out of order")
		}
		nodes := requireUniqueNodeIDs(step.NodeIDs, field+".node_ids", &problems)
		if e.Rollout.ManagedNodeCount > 0 && e.Rollout.ManagedNodeCount <= maxForwardingNodeCount && index < len(expectedPercentages) {
			expectedCount := (e.Rollout.ManagedNodeCount*expectedPercentages[index] + 99) / 100
			require(uint(len(step.NodeIDs)) == expectedCount, fmt.Sprintf("%s.node_ids must contain %d nodes", field, expectedCount))
		}
		for nodeID := range previousNodes {
			_, present := nodes[nodeID]
			require(present, fmt.Sprintf("%s.node_ids must retain node %d from the previous stage", field, nodeID))
		}
		startedAt := requireTime(step.StartedAt, field+".started_at")
		completedAt := requireTime(step.CompletedAt, field+".completed_at")
		require(previousCompletedAt.IsZero() || startedAt.IsZero() || !startedAt.Before(previousCompletedAt), field+" must not overlap the previous gate")
		require(startedAt.IsZero() || completedAt.IsZero() || completedAt.After(startedAt), field+".completed_at must follow started_at")
		requireText(step.EvidenceReference, field+".evidence_reference")
		previousNodes = nodes
		previousCompletedAt = completedAt
	}

	requireText(e.Operator.ApprovedBy, "operator.approved_by")
	approvedAt := requireTime(e.Operator.ApprovedAt, "operator.approved_at")
	require(previousCompletedAt.IsZero() || approvedAt.IsZero() || !approvedAt.Before(previousCompletedAt), "operator approval must follow the 100 percent rollout stage")
	requireText(e.Operator.ChangeReference, "operator.change_reference")

	if len(problems) > 0 {
		return fmt.Errorf("forwarding acceptance is incomplete: %s", strings.Join(problems, "; "))
	}
	return nil
}

func requireUniqueNodeIDs(nodeIDs []uint, field string, problems *[]string) map[uint]struct{} {
	nodes := make(map[uint]struct{}, len(nodeIDs))
	if len(nodeIDs) == 0 {
		*problems = append(*problems, field+" must contain at least one node")
	}
	for _, nodeID := range nodeIDs {
		if nodeID == 0 {
			*problems = append(*problems, field+" must not contain node zero")
			continue
		}
		if _, exists := nodes[nodeID]; exists {
			*problems = append(*problems, fmt.Sprintf("%s contains duplicate node %d", field, nodeID))
			continue
		}
		nodes[nodeID] = struct{}{}
	}
	return nodes
}
