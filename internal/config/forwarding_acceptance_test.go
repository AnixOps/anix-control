package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func completeForwardingAcceptance() ForwardingAcceptance {
	var value ForwardingAcceptance
	value.Release.ControlCommit = strings.Repeat("a", 40)
	value.Release.AgentCommit = strings.Repeat("b", 40)
	value.Release.PluginID = "nftables-forward"
	value.Release.PluginVersion = "1.2.0"
	value.Release.PackageSHA256 = strings.Repeat("c", 64)
	value.Staging.Environment = "staging"
	value.Staging.RestoreRehearsedAt = "2026-09-01T00:00:00Z"
	value.Staging.VerifiedAt = "2026-09-01T01:00:00Z"
	value.Staging.IPv4TCPPassed = true
	value.Staging.IPv4UDPPassed = true
	value.Staging.IPv6TCPPassed = true
	value.Staging.IPv6UDPPassed = true
	value.Staging.PartialNodeRollbackPassed = true
	value.Staging.ControlRestartPassed = true
	value.Staging.AgentRestartPassed = true
	value.Staging.EvidenceReference = "change-20260901-staging"
	value.LegacyFallback.RehearsedAt = "2026-09-01T02:00:00Z"
	value.LegacyFallback.RecoveryVerifiedAt = "2026-09-01T03:00:00Z"
	value.LegacyFallback.EvidenceReference = "change-20260901-fallback"
	value.Canary.NodeIDs = []uint{1}
	value.Canary.StartedAt = "2026-09-01T04:00:00Z"
	value.Canary.CompletedAt = "2026-09-04T04:00:00Z"
	value.Canary.TrafficHealthy = true
	value.Canary.RollbackPassed = true
	value.Canary.EvidenceReference = "change-20260901-canary"
	value.Rollout.ManagedNodeCount = 100
	value.Rollout.Steps = []ForwardingRolloutStep{
		{Percentage: 1, NodeIDs: nodeRange(1), StartedAt: "2026-09-04T05:00:00Z", CompletedAt: "2026-09-04T06:00:00Z", EvidenceReference: "rollout-1"},
		{Percentage: 5, NodeIDs: nodeRange(5), StartedAt: "2026-09-04T07:00:00Z", CompletedAt: "2026-09-04T08:00:00Z", EvidenceReference: "rollout-5"},
		{Percentage: 25, NodeIDs: nodeRange(25), StartedAt: "2026-09-04T09:00:00Z", CompletedAt: "2026-09-04T10:00:00Z", EvidenceReference: "rollout-25"},
		{Percentage: 100, NodeIDs: nodeRange(100), StartedAt: "2026-09-04T11:00:00Z", CompletedAt: "2026-09-04T12:00:00Z", EvidenceReference: "rollout-100"},
	}
	value.Operator.ApprovedBy = "operations-owner"
	value.Operator.ApprovedAt = "2026-09-04T13:00:00Z"
	value.Operator.ChangeReference = "change-20260904-forwarding"
	return value
}

func nodeRange(count uint) []uint {
	nodes := make([]uint, count)
	for index := range nodes {
		nodes[index] = uint(index + 1)
	}
	return nodes
}

func TestForwardingAcceptanceAcceptsCompleteEvidence(t *testing.T) {
	require.NoError(t, completeForwardingAcceptance().Validate())
}

func TestForwardingAcceptanceRejectsIncompleteGates(t *testing.T) {
	value := completeForwardingAcceptance()
	value.Staging.IPv6UDPPassed = false
	value.LegacyFallback.EvidenceReference = "REPLACE_WITH_REFERENCE"
	value.Canary.TrafficHealthy = false
	value.Operator.ApprovedBy = "TODO"

	err := value.Validate()
	require.ErrorContains(t, err, "staging.ipv6_udp_passed")
	require.ErrorContains(t, err, "legacy_fallback.evidence_reference")
	require.ErrorContains(t, err, "canary.traffic_healthy")
	require.ErrorContains(t, err, "operator.approved_by")
}

func TestForwardingAcceptanceRejectsCanaryShorterThan72Hours(t *testing.T) {
	value := completeForwardingAcceptance()
	value.Canary.CompletedAt = "2026-09-04T03:59:59Z"

	require.ErrorContains(t, value.Validate(), "at least 72 hours")
}

func TestForwardingAcceptanceRejectsInvalidRollout(t *testing.T) {
	tests := map[string]func(*ForwardingAcceptance){
		"wrong stage order": func(value *ForwardingAcceptance) {
			value.Rollout.Steps[1].Percentage = 25
		},
		"wrong node count": func(value *ForwardingAcceptance) {
			value.Rollout.Steps[2].NodeIDs = nodeRange(24)
		},
		"duplicate node": func(value *ForwardingAcceptance) {
			value.Rollout.Steps[1].NodeIDs[4] = 1
		},
		"not a superset": func(value *ForwardingAcceptance) {
			value.Rollout.Steps[1].NodeIDs = []uint{2, 3, 4, 5, 6}
		},
		"overlapping stages": func(value *ForwardingAcceptance) {
			value.Rollout.Steps[1].StartedAt = "2026-09-04T05:30:00Z"
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := completeForwardingAcceptance()
			mutate(&value)
			require.Error(t, value.Validate())
		})
	}
}

func TestForwardingAcceptanceRejectsInvalidOrderingAndRelease(t *testing.T) {
	value := completeForwardingAcceptance()
	value.Release.PluginVersion = "01.2.0"
	value.LegacyFallback.RehearsedAt = "2026-08-31T23:00:00Z"
	value.Operator.ApprovedAt = "2026-09-04T11:30:00Z"

	err := value.Validate()
	require.ErrorContains(t, err, "semantic version")
	require.ErrorContains(t, err, "fallback rehearsal must follow staging")
	require.ErrorContains(t, err, "approval must follow")
}

func TestValidateForwardingAcceptanceFileIsStrict(t *testing.T) {
	value := completeForwardingAcceptance()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "acceptance.yaml")
		require.NoError(t, os.WriteFile(path, data, 0o600))
		require.NoError(t, ValidateForwardingAcceptanceFile(path))
	})

	t.Run("unknown field", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "acceptance.yaml")
		invalid := append(append([]byte(nil), data...), []byte("unknown_gate: true\n")...)
		require.NoError(t, os.WriteFile(path, invalid, 0o600))
		require.ErrorContains(t, ValidateForwardingAcceptanceFile(path), "field unknown_gate not found")
	})

	t.Run("multiple documents", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "acceptance.yaml")
		invalid := append(append([]byte(nil), data...), []byte("---\nrelease: {}\n")...)
		require.NoError(t, os.WriteFile(path, invalid, 0o600))
		require.ErrorContains(t, ValidateForwardingAcceptanceFile(path), "exactly one YAML document")
	})
}

func TestForwardingAcceptanceTemplateFailsClosed(t *testing.T) {
	path := filepath.Join("..", "..", "config", "examples", "forwarding-acceptance.yaml.example")
	err := ValidateForwardingAcceptanceFile(path)
	require.Error(t, err)
	require.ErrorContains(t, err, "forwarding acceptance is incomplete")
}
