package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type ProductionAcceptance struct {
	Release struct {
		ControlImageDigest  string `yaml:"control_image_digest"`
		ControlCommit       string `yaml:"control_commit"`
		AgentArtifactSHA256 string `yaml:"agent_artifact_sha256"`
		AgentCommit         string `yaml:"agent_commit"`
	} `yaml:"release"`
	Database struct {
		BackupReference    string `yaml:"backup_reference"`
		RestoreRehearsedAt string `yaml:"restore_rehearsed_at"`
		SchemaMigratedAt   string `yaml:"schema_migrated_at"`
		SchemaVerifiedAt   string `yaml:"schema_verified_at"`
	} `yaml:"database"`
	Notifications struct {
		SMTPDeliveredAt     string `yaml:"smtp_delivered_at"`
		SMTPMessageID       string `yaml:"smtp_message_id"`
		TelegramDeliveredAt string `yaml:"telegram_delivered_at"`
		TelegramMessageID   string `yaml:"telegram_message_id"`
	} `yaml:"notifications"`
	ExternalMonitoring struct {
		Provider                  string `yaml:"provider"`
		MonitorID                 string `yaml:"monitor_id"`
		OutageAlertAt             string `yaml:"outage_alert_at"`
		RecoveryAlertAt           string `yaml:"recovery_alert_at"`
		EmailAndTelegramConfirmed bool   `yaml:"email_and_telegram_confirmed"`
	} `yaml:"external_monitoring"`
	Canary struct {
		NodeIDs                  []uint `yaml:"node_ids"`
		StartedAt                string `yaml:"started_at"`
		CompletedAt              string `yaml:"completed_at"`
		WebSocketReconnectPassed bool   `yaml:"websocket_reconnect_passed"`
		AgentRestartPassed       bool   `yaml:"agent_restart_passed"`
		DuplicateDeliveryPassed  bool   `yaml:"duplicate_delivery_passed"`
		SignedInstallPassed      bool   `yaml:"signed_install_passed"`
		InvalidSignatureRejected bool   `yaml:"invalid_signature_rejected"`
		ManualRollbackPassed     bool   `yaml:"manual_rollback_passed"`
		EvidenceReference        string `yaml:"evidence_reference"`
	} `yaml:"canary"`
	Security struct {
		ForgedNodeRejected    bool   `yaml:"forged_node_rejected"`
		OversizeEventRejected bool   `yaml:"oversize_event_rejected"`
		RedactionVerified     bool   `yaml:"redaction_verified"`
		EvidenceReference     string `yaml:"evidence_reference"`
	} `yaml:"security"`
	Network struct {
		Regions           []string `yaml:"regions"`
		DeviceClasses     []string `yaml:"device_classes"`
		VerifiedAt        string   `yaml:"verified_at"`
		EvidenceReference string   `yaml:"evidence_reference"`
	} `yaml:"network"`
	Handoff struct {
		Owner                   string   `yaml:"owner"`
		Technicians             []string `yaml:"technicians"`
		OwnerChannelsVerifiedAt string   `yaml:"owner_channels_verified_at"`
		RunbookReviewedAt       string   `yaml:"runbook_reviewed_at"`
		ApprovedBy              string   `yaml:"approved_by"`
		ApprovedAt              string   `yaml:"approved_at"`
	} `yaml:"handoff"`
}

func ValidateProductionAcceptanceFile(path string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- operator explicitly supplies the local evidence path.
	if err != nil {
		return fmt.Errorf("read production acceptance file: %w", err)
	}
	var evidence ProductionAcceptance
	if err := yaml.Unmarshal(data, &evidence); err != nil {
		return fmt.Errorf("parse production acceptance file: %w", err)
	}
	return evidence.Validate()
}

func (e ProductionAcceptance) Validate() error {
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

	require(sha256DigestPattern.MatchString(strings.TrimSpace(e.Release.ControlImageDigest)), "release.control_image_digest must be sha256:<64 lowercase hex>")
	require(hexLength(e.Release.ControlCommit, 40), "release.control_commit must be a 40-character Git commit")
	require(hexLength(e.Release.AgentArtifactSHA256, 64), "release.agent_artifact_sha256 must be 64 lowercase hex characters")
	require(hexLength(e.Release.AgentCommit, 40), "release.agent_commit must be a 40-character Git commit")

	requireText(e.Database.BackupReference, "database.backup_reference")
	restoreAt := requireTime(e.Database.RestoreRehearsedAt, "database.restore_rehearsed_at")
	migratedAt := requireTime(e.Database.SchemaMigratedAt, "database.schema_migrated_at")
	verifiedAt := requireTime(e.Database.SchemaVerifiedAt, "database.schema_verified_at")
	require(restoreAt.IsZero() || migratedAt.IsZero() || !restoreAt.After(migratedAt), "database restore rehearsal must not be after schema migration")
	require(migratedAt.IsZero() || verifiedAt.IsZero() || !verifiedAt.Before(migratedAt), "database schema verification must not precede migration")

	smtpAt := requireTime(e.Notifications.SMTPDeliveredAt, "notifications.smtp_delivered_at")
	requireText(e.Notifications.SMTPMessageID, "notifications.smtp_message_id")
	telegramAt := requireTime(e.Notifications.TelegramDeliveredAt, "notifications.telegram_delivered_at")
	requireText(e.Notifications.TelegramMessageID, "notifications.telegram_message_id")

	requireText(e.ExternalMonitoring.Provider, "external_monitoring.provider")
	requireText(e.ExternalMonitoring.MonitorID, "external_monitoring.monitor_id")
	outageAt := requireTime(e.ExternalMonitoring.OutageAlertAt, "external_monitoring.outage_alert_at")
	recoveryAt := requireTime(e.ExternalMonitoring.RecoveryAlertAt, "external_monitoring.recovery_alert_at")
	require(outageAt.IsZero() || recoveryAt.IsZero() || recoveryAt.After(outageAt), "external monitoring recovery must follow its outage alert")
	require(e.ExternalMonitoring.EmailAndTelegramConfirmed, "external_monitoring.email_and_telegram_confirmed must be true")

	require(len(e.Canary.NodeIDs) > 0, "canary.node_ids must contain at least one node")
	canaryStart := requireTime(e.Canary.StartedAt, "canary.started_at")
	canaryEnd := requireTime(e.Canary.CompletedAt, "canary.completed_at")
	require(canaryStart.IsZero() || canaryEnd.IsZero() || canaryEnd.After(canaryStart), "canary.completed_at must follow canary.started_at")
	require(e.Canary.WebSocketReconnectPassed, "canary.websocket_reconnect_passed must be true")
	require(e.Canary.AgentRestartPassed, "canary.agent_restart_passed must be true")
	require(e.Canary.DuplicateDeliveryPassed, "canary.duplicate_delivery_passed must be true")
	require(e.Canary.SignedInstallPassed, "canary.signed_install_passed must be true")
	require(e.Canary.InvalidSignatureRejected, "canary.invalid_signature_rejected must be true")
	require(e.Canary.ManualRollbackPassed, "canary.manual_rollback_passed must be true")
	requireText(e.Canary.EvidenceReference, "canary.evidence_reference")

	require(e.Security.ForgedNodeRejected, "security.forged_node_rejected must be true")
	require(e.Security.OversizeEventRejected, "security.oversize_event_rejected must be true")
	require(e.Security.RedactionVerified, "security.redaction_verified must be true")
	requireText(e.Security.EvidenceReference, "security.evidence_reference")

	require(len(e.Network.Regions) >= 2, "network.regions must contain at least two regions")
	require(len(e.Network.DeviceClasses) > 0, "network.device_classes must contain at least one device class")
	requireTime(e.Network.VerifiedAt, "network.verified_at")
	requireText(e.Network.EvidenceReference, "network.evidence_reference")

	requireText(e.Handoff.Owner, "handoff.owner")
	require(len(e.Handoff.Technicians) > 0 || nonPlaceholderEvidence(e.Handoff.Owner), "handoff must name an owner or technician")
	requireTime(e.Handoff.OwnerChannelsVerifiedAt, "handoff.owner_channels_verified_at")
	requireTime(e.Handoff.RunbookReviewedAt, "handoff.runbook_reviewed_at")
	requireText(e.Handoff.ApprovedBy, "handoff.approved_by")
	approvedAt := requireTime(e.Handoff.ApprovedAt, "handoff.approved_at")
	require(approvedAt.IsZero() || smtpAt.IsZero() || !approvedAt.Before(smtpAt), "handoff approval must follow SMTP verification")
	require(approvedAt.IsZero() || telegramAt.IsZero() || !approvedAt.Before(telegramAt), "handoff approval must follow Telegram verification")
	require(approvedAt.IsZero() || canaryEnd.IsZero() || !approvedAt.Before(canaryEnd), "handoff approval must follow canary completion")

	if len(problems) > 0 {
		return fmt.Errorf("production acceptance is incomplete: %s", strings.Join(problems, "; "))
	}
	return nil
}

func nonPlaceholderEvidence(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	return trimmed != "" && !strings.Contains(lower, "replace") && !strings.Contains(lower, "todo") && !strings.Contains(lower, "example")
}

func hexLength(value string, length int) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != length {
		return false
	}
	for _, char := range trimmed {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
