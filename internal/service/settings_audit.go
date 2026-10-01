package service

import (
	"encoding/json"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// SettingsAuditActor is who changed a setting, as the audit trail records
// it. The kernel's handlers take it from the request's context; the
// KernelSettings contract from the calling package's Actor.
type SettingsAuditActor struct {
	UserID    *uint
	Username  string
	IP        string
	UserAgent string
}

// AuditUsername is the username the audit trail records for userID when
// the request names none: the user's e-mail, which the kernel's handlers
// record for a signed-in administrator. A request the package bridge
// relays to a legacy handler, and a KernelSettings call, carry the actor's
// id only. An unknown user, or a failed lookup, records none.
func AuditUsername(db *gorm.DB, userID *uint) string {
	if db == nil || userID == nil || *userID == 0 {
		return ""
	}
	var emails []string
	if err := db.Model(&model.User{}).Where("id = ?", *userID).Limit(1).Pluck("email", &emails).Error; err != nil || len(emails) == 0 {
		return ""
	}
	return strings.TrimSpace(emails[0])
}

// SystemConfigAuditInput is the audit entry of a system configuration
// change: which key, its group and type, whether it is a secret and has a
// value, and whether the stored secret was kept. For a value with masked
// fields (the SMTP password of the e-mail configuration) it also names
// them and those that hold a value. It never holds the value.
func SystemConfigAuditInput(actor SettingsAuditActor, action string, cfg *model.SystemConfig, preserveExisting bool) *OperationLogInput {
	return &OperationLogInput{
		UserID: actor.UserID, Username: actor.Username, Action: action, Module: "system",
		TargetType: "system_config", TargetID: settingsAuditTarget(cfg.ID),
		Content: SystemConfigAuditContent(cfg, preserveExisting), IP: actor.IP, UserAgent: actor.UserAgent, Status: 1,
	}
}

// SystemConfigAuditContent is the content of a system configuration audit
// entry.
func SystemConfigAuditContent(cfg *model.SystemConfig, preserveExisting bool) string {
	if cfg == nil {
		return ""
	}
	fields := map[string]any{
		"key":               cfg.Key,
		"group":             cfg.Group,
		"type":              cfg.Type,
		"sensitive":         IsSensitiveSystemConfigKey(cfg.Key),
		"has_value":         strings.TrimSpace(cfg.Value) != "",
		"preserve_existing": preserveExisting,
	}
	// The names avoid the words the audit log answer redacts.
	if masked := SystemConfigMaskedFields(cfg.Key); len(masked) > 0 {
		fields["masked_fields"] = masked
		fields["masked_fields_with_value"] = maskedFieldsWithValue(cfg.Key, cfg.Value)
	}
	content, err := json.Marshal(fields)
	if err != nil {
		return cfg.Key
	}
	return string(content)
}

// BackupConfigAuditInput is the audit entry of a backup configuration
// change: every field but the S3 keys, whether each S3 key has a value, and
// which of them were kept.
func BackupConfigAuditInput(actor SettingsAuditActor, action string, cfg *model.BackupConfig, preservedSensitiveFields []string) *OperationLogInput {
	return &OperationLogInput{
		UserID: actor.UserID, Username: actor.Username, Action: action, Module: "system",
		TargetType: "backup_config", TargetID: settingsAuditTarget(cfg.ID),
		Content: BackupConfigAuditContent(cfg, preservedSensitiveFields), IP: actor.IP, UserAgent: actor.UserAgent, Status: 1,
	}
}

// BackupConfigAuditContent is the content of a backup configuration audit
// entry.
func BackupConfigAuditContent(cfg *model.BackupConfig, preservedSensitiveFields []string) string {
	if cfg == nil {
		return ""
	}
	if preservedSensitiveFields == nil {
		preservedSensitiveFields = []string{}
	}
	content, err := json.Marshal(map[string]any{
		"enabled":                    cfg.Enabled,
		"auto_backup":                cfg.AutoBackup,
		"schedule":                   cfg.Schedule,
		"retention_days":             cfg.RetentionDays,
		"backup_database":            cfg.BackupDatabase,
		"backup_files":               cfg.BackupFiles,
		"storage_type":               cfg.StorageType,
		"storage_path":               cfg.StoragePath,
		"s3_bucket":                  cfg.S3Bucket,
		"s3_region":                  cfg.S3Region,
		"s3_endpoint":                cfg.S3Endpoint,
		"s3_access_key_has_value":    strings.TrimSpace(cfg.S3AccessKey) != "",
		"s3_secret_key_has_value":    strings.TrimSpace(cfg.S3SecretKey) != "",
		"preserved_sensitive_fields": preservedSensitiveFields,
	})
	if err != nil {
		return cfg.StorageType
	}
	return string(content)
}

func settingsAuditTarget(id uint) *uint {
	if id == 0 {
		return nil
	}
	return &id
}
