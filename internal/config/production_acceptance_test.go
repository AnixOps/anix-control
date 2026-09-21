package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func completeProductionAcceptance() ProductionAcceptance {
	var value ProductionAcceptance
	value.Release.ControlImageDigest = "sha256:" + strings.Repeat("a", 64)
	value.Release.ControlCommit = strings.Repeat("b", 40)
	value.Release.AgentArtifactSHA256 = strings.Repeat("c", 64)
	value.Release.AgentCommit = strings.Repeat("d", 40)
	value.Database.BackupReference = "backup-20260922-001"
	value.Database.RestoreRehearsedAt = "2026-09-22T01:00:00Z"
	value.Database.SchemaMigratedAt = "2026-09-22T02:00:00Z"
	value.Database.SchemaVerifiedAt = "2026-09-22T02:05:00Z"
	value.Notifications.SMTPDeliveredAt = "2026-09-22T03:00:00Z"
	value.Notifications.SMTPMessageID = "smtp-message-001"
	value.Notifications.TelegramDeliveredAt = "2026-09-22T03:01:00Z"
	value.Notifications.TelegramMessageID = "telegram-message-001"
	value.ExternalMonitoring.Provider = "uptime-provider"
	value.ExternalMonitoring.MonitorID = "monitor-001"
	value.ExternalMonitoring.OutageAlertAt = "2026-09-22T04:00:00Z"
	value.ExternalMonitoring.RecoveryAlertAt = "2026-09-22T04:10:00Z"
	value.ExternalMonitoring.EmailAndTelegramConfirmed = true
	value.Canary.NodeIDs = []uint{7}
	value.Canary.StartedAt = "2026-09-22T05:00:00Z"
	value.Canary.CompletedAt = "2026-09-22T07:00:00Z"
	value.Canary.WebSocketReconnectPassed = true
	value.Canary.AgentRestartPassed = true
	value.Canary.DuplicateDeliveryPassed = true
	value.Canary.SignedInstallPassed = true
	value.Canary.InvalidSignatureRejected = true
	value.Canary.ManualRollbackPassed = true
	value.Canary.EvidenceReference = "change-20260922-canary"
	value.Security.ForgedNodeRejected = true
	value.Security.OversizeEventRejected = true
	value.Security.RedactionVerified = true
	value.Security.EvidenceReference = "security-test-20260922"
	value.Network.Regions = []string{"cn-east", "eu-west"}
	value.Network.DeviceClasses = []string{"linux-amd64"}
	value.Network.VerifiedAt = "2026-09-22T07:30:00Z"
	value.Network.EvidenceReference = "network-test-20260922"
	value.Handoff.Owner = "primary-oncall"
	value.Handoff.OwnerChannelsVerifiedAt = "2026-09-22T08:00:00Z"
	value.Handoff.RunbookReviewedAt = "2026-09-22T08:10:00Z"
	value.Handoff.ApprovedBy = "release-approver"
	value.Handoff.ApprovedAt = "2026-09-22T09:00:00Z"
	return value
}

func TestProductionAcceptanceAcceptsCompleteEvidence(t *testing.T) {
	require.NoError(t, completeProductionAcceptance().Validate())
}

func TestProductionAcceptanceRejectsMissingRealWorldGates(t *testing.T) {
	value := completeProductionAcceptance()
	value.Notifications.TelegramMessageID = "REPLACE-ME"
	value.Canary.ManualRollbackPassed = false
	value.Network.Regions = []string{"one-region"}

	err := value.Validate()
	require.Error(t, err)
	require.ErrorContains(t, err, "telegram_message_id")
	require.ErrorContains(t, err, "manual_rollback_passed")
	require.ErrorContains(t, err, "at least two regions")
}
